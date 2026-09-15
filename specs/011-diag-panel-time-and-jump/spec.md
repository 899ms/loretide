# Feature Specification: 瀑布时间语义、真实时钟偏差与可达的追踪跳转

**Feature Branch**: `claude/spec-011-diag-panel-time-and-jump`

**Created**: 2026-09-15

**Status**: Draft

**Input**: 主任务 2026-09-15 在真实浏览器执行 `docs/development/manual-ui-runbook.md`，64 条中 **4 条未通过**：`006-W-2`、`006-W-4`、`006-W-9`、`008-J-1`（并附带发现 `008-O-1/O-2/O-3` 在该路径上不可达）。全部落在 `content/diagnostics` 边界内。

这是**缺陷修复**，不是新功能。三个缺陷各自独立，可分别交付。

---

## 缺陷与根因（均已对代码核实）

### 缺陷一 —— 瀑布时间轴整体右移一个步长（`006-W-2` / `006-W-9`，最重要）

**现象**：run `93edf840` 的六条技术事件，数据库里 `occurred_at` = 13 / 21 / 34 / 45 / 65 / 71 ms，`duration` = 13 / 8 / 13 / 11 / 20 / 6 ms。真实链条**首尾相接、零间隙**。面板却画成 0-13 / 8-16 / 21-34 / 32-43 / 52-72 / 58-64，**凭空出现 5ms 与 9ms 的假等待间隙**，且 executor 看起来嵌在 daemon 里面。

**根因（一处语义分歧，两端各自自洽）**：

| 位置 | 行为 |
|---|---|
| `server/internal/content/diagnostics/simulator.go:38-39` | 先 `now = now.Add(duration)`，**再**写 `Occurred: now` —— 所以 `occurred_at` 是该步的**结束**时刻 |
| `packages/core/content/diagnostics/trace-waterfall.ts:72` | 注释写 "the earliest parseable **start**"，按 `[occurred, occurred+duration]` 画 —— 把它当**开始**时刻 |

两边都没错，错的是**没人把 `occurred_at` 的语义写下来**。生产者写结束时刻、消费者读开始时刻，于是每根条子整体右移自己的时长，相邻步骤之间就长出等于「前一步时长减去时间差」的假间隙。

**裁决（主任务）**：**改生产者，不改瀑布算法。** 把 `occurred_at` 的语义钉死为**「该事件所代表步骤的开始时刻」**，`simulator.go` 改为**步进前**盖 `Occurred`。理由：开始时刻是 tracing 领域的通行语义（span start + duration），瀑布算法、`STREAM_EVENT_CAP` 折叠、`clockSkew` 判定全部建立在它之上；改消费者要动的面积大得多，且会让 `occurred_at` 与 OpenTelemetry 的 span 语义背离。

### 缺陷二 —— `clock_skew` 场景不产生时钟偏差标注（`006-W-4`）

**现象**：跑 `clock_skew` 场景的运行，瀑布里**不出现** `text093`「时钟偏差：此处不可按时间解读」。

**根因**：`trace-waterfall.ts:134` 只在**子 span 偏移 < 父 span 偏移**时置 `anomaly = "clockSkew"`；而 `simulator.go:36` 的 `clock_skew` 分支**只写错误码 `CLOCK_SKEW` 与一条复现缺口**，时间戳仍然单调递增。**两者永远碰不上**——判定条件与造数据条件互不相交，所以这条路径从交付之日起就没有被走过。

**修法**：让 `clock_skew` 场景**真的**把该步的 `Occurred` 往前挪，早于其父 span，使 `clockSkew` 真实触发。`FR-014`「不夹取」必须仍然成立——偏差是**标注**出来的，不是被纠正掉的。

### 缺陷三 —— 「查看追踪」落点可达性（`008-J-1`，并附带发现 `008-O-1/O-2/O-3` 不可达）

**现象 A（措辞不符）**：点「查看追踪」后落在**单次追踪**页（`index.tsx:617` `setTab(3)`），而 `specs/008` 的 `manual-ui-todo.md` 与 runbook 写的是「切到**技术日志**页」。

**裁决（主任务）**：**改规格不改落点。** 单次追踪本就是正确目标——对象与版本卡片与事件表都在该分支下。

**现象 B（真正的 bug，附带发现）**：`index.tsx:1039` 的渲染条件是 `tab === 3 && !runId && trace`。而 `onTrace` **必然**执行 `setRunId(jump.filter.runId)`。于是：

> 走「查看追踪」进入单次追踪页时，`runId` 非空 → 该分支**永不渲染** → `008-O-1/O-2/O-3` 的「对象与版本」卡片在这条路径上**永远看不到**。

主任务只能靠**刷新页面手输 Trace**（使 `runId` 为空）才看到它。也就是说 runbook 设计的 `J-1 → O-1` 连贯路径**在实现里是断的**。

**修法**：放宽该条件，使单次追踪在**有 `runId` 时**同时呈现瀑布 **与** 对象与版本卡片 + 事件表。具体编排由实现决定，但必须让 `J-1 → O-1` 连贯。

---

## User Scenarios & Testing *(mandatory)*

### User Story 1 —— 瀑布时间轴如实反映真实时序（P1，缺陷一）

排障的人打开一次运行的瀑布，想知道**时间花在哪一步**。如果两步之间真的有等待，他要看见间隙；如果没有，他**不能**看见间隙——否则他会去查一个不存在的延迟。

**Why this priority**：这是 `006-W-2` / `006-W-9` 两条未通过项的共同根因，且它让瀑布**主动误导**：假间隙比没有瀑布更糟。

**Independent Test**：Go 侧断言一次模拟运行的相邻步骤首尾相接；core 侧用真实形状（连续链）断言 `buildTraceWaterfall` 产出零行间空隙。二者都无需界面。

**Acceptance Scenarios**：

1. **Given** 一次模拟运行，**When** 检查其相邻事件，**Then** `start[i+1] == start[i] + duration[i]`，**0 容差**。
2. **Given** 该运行的事件喂给瀑布推导，**When** 检查相邻行，**Then** 不存在任何正的行间空隙。
3. **Given** `occurred_at` 的语义，**When** 查阅合同，**Then** 它被明确写为「该事件所代表步骤的**开始**时刻」，生产者与消费者引用同一句话。

---

### User Story 2 —— `clock_skew` 场景真的显示时钟偏差（P2，缺陷二）

跑 `clock_skew` 故障场景的人，期望在瀑布上看到「此处不可按时间解读」的标注——这正是该场景存在的理由。

**Why this priority**：`006-W-4` 未通过。缺陷一修完后时间轴才可信，这条才有意义，故排第二。

**Independent Test**：断言 `clock_skew` 场景产出的运行经 `buildTraceWaterfall` 后**存在** `clockSkew` 标注。

**Acceptance Scenarios**：

1. **Given** `clock_skew` 场景的一次运行，**When** 求其瀑布，**Then** 至少一行的 `anomaly` 为 `clockSkew`。
2. **Given** 同一次运行，**When** 检查被标注行的偏移，**Then** 它**早于**其父 span 且**未被夹取**到父区间内（FR-014 仍成立）。
3. **Given** 其余 15 个场景，**When** 求其瀑布，**Then** **不得**出现 `clockSkew` —— 偏差是该场景独有的。

---

### User Story 3 —— 从「查看追踪」能连贯走到对象与版本（P1，缺陷三）

排障的人在操作时间线点「查看追踪」，落到单次追踪页，**在同一屏**看到：这次追踪的瀑布、它touch 的对象与版本、以及构成它的事件列表。不需要刷新页面，也不需要手输追踪编号。

**Why this priority**：它让 `008-O-1/O-2/O-3` 三条手动条目**从不可达变为可达**——在此之前它们无法被诚实地验收。

**Independent Test**：渲染条件的改动本身按原则 II 不写自动测试，进手动清单；但「跳转产出什么」已由既有的 `describeTraceJump` 纯函数测试覆盖，本特性不改它。

**Acceptance Scenarios**：

1. **Given** 从操作时间线点「查看追踪」进入单次追踪页，**When** 该页渲染，**Then** 瀑布、对象与版本卡片、事件表**同时可见**。
2. **Given** 手输追踪编号（`runId` 为空）进入，**When** 该页渲染，**Then** 对象与版本卡片与事件表**仍然可见**（不得为修新路径而破坏旧路径）。
3. **Given** 只从运行选择器选了运行、没有追踪编号，**When** 该页渲染，**Then** **不**显示对象与版本卡片——没有追踪范围时归拢无意义。

---

### Edge Cases

- 单步运行（只有一个事件）：无相邻对，首尾相接断言应**空过**而非报错。
- `duration` 为 0 的步骤：下一步的开始时刻与本步相同，属**零间隙**，不得判为异常。
- `clock_skew` 的偏移使某事件的 `occurred_at` 早于运行的 `created_at`：允许，瀑布基线取**最早**事件，负偏移不应出现。
- 追踪编号存在但该追踪的事件已过保留期：对象与版本卡片显示「未记录版本」或不显示，**不得**崩溃或显示上一次追踪的残留。

## Requirements *(mandatory)*

### Functional Requirements

**缺陷一 —— 时间语义**

- **FR-001**: `occurred_at` 的语义 MUST 被明确定义为**「该事件所代表步骤的开始时刻」**，并写入本特性合同与 `specs/006/contracts/trace-waterfall.md`。
- **FR-002**: 模拟器 MUST 在**步进前**盖 `Occurred`，使 `occurred_at` 为开始时刻。
- **FR-003**: 系统 MUST 有 Go 测试断言一次模拟运行的相邻步骤**首尾相接**：`start[i+1] == start[i] + duration[i]`，**0 容差**。
- **FR-004**: 系统 MUST 有 core 纯函数测试，用**真实形状**（连续链）断言 `buildTraceWaterfall` 不产生任何行间空隙。
- **FR-005**: 本特性 MUST NOT 修改瀑布算法的时间处理（`trace-waterfall.ts` 的基线、偏移与时长计算）。

**缺陷二 —— 真实时钟偏差**

- **FR-006**: `clock_skew` 场景 MUST 产生**真实的**乱序时间戳——该步的 `Occurred` 早于其父 span。
- **FR-007**: 系统 MUST 有测试断言该场景产出的运行经 `buildTraceWaterfall` 后**存在** `clockSkew` 标注。
- **FR-008**: 偏差 MUST 只被**标注**、不被**纠正**（FR-014「不夹取」仍成立）。
- **FR-009**: 其余场景 MUST NOT 产生 `clockSkew` 标注。

**缺陷三 —— 可达的跳转落点**

- **FR-010**: 单次追踪页 MUST 在**有追踪编号**时同时呈现瀑布、对象与版本卡片、事件表，使 `J-1 → O-1` 路径连贯。
- **FR-011**: 手输追踪编号（无运行编号）的既有路径 MUST 继续可用。
- **FR-012**: 无追踪编号时 MUST NOT 显示对象与版本卡片。
- **FR-013**: `specs/008/manual-ui-todo.md` 与 `docs/development/manual-ui-runbook.md` 中 `J-1`、`J-4` 的措辞 MUST 由「技术日志」改为「单次追踪」，**两份都改**，并注明「2026-09-15 实施期修正，原措辞与实现不符」。

**贯穿约束**

- **FR-014**: MUST NOT 新增任何前端数据读取路径（事件查询已携带 `trace_id`）。
- **FR-015**: MUST NOT 修改 `server/internal/daemon/` 或任何上游 Multica 代码。
- **FR-016**: MUST NOT 新增 UI 单测（原则 II）；界面行为进手动清单。
- **FR-017**: 每条**有自动覆盖**的新规则 MUST 各做一处变异验证——即缺陷一的时间语义与缺陷二的时钟偏差。**缺陷三没有变异验证，而且不可能有**：它改的是界面渲染条件，按原则 II 没有自动测试可以被变红。它的验收只能是用户复验 `008-J-1` 与 `008-O-1`，这一点 MUST 在 PR 正文写明，不得让「变异验证 N 处」的说法掩盖它。

### Key Entities

- **诊断事件**：携带 `occurred_at`（本特性后明确为**开始**时刻）与 `duration_ms`。
- **瀑布行**：由事件推导，带 `startOffsetMs` / `durationMs` / `anomaly`。
- **时钟偏差标注**：`anomaly = "clockSkew"`，表示「此处不可按时间解读」。

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: run `93edf840` 形状的数据经修复后，瀑布相邻行的间隙为 **0**（修复前为 5ms 与 9ms 的假间隙）。
- **SC-002**: `clock_skew` 场景产出的运行，**100%** 出现 `clockSkew` 标注；其余 **15 个场景 0%** 出现。
- **SC-003**: 从「查看追踪」进入单次追踪页，瀑布、对象与版本、事件表三者**同屏可见**，无需刷新或手输编号。
- **SC-004**: **两条**可自动覆盖的新规则各有一处变异验证使对应用例变红。第三条（跳转落点）**无自动覆盖**，其验收由用户复验承担——见 FR-017。
- **SC-005**: 新增 UI 单测数为 **0**；上游与 daemon 代码改动行数为 **0**。

## Assumptions

- 主任务的浏览器观察与数据库证据为准；本规格的根因已逐条对代码核实。
- 缺陷一改生产者不改消费者，是主任务裁决，不再重新论证。
- 缺陷三改规格不改落点，是主任务裁决；真正要修的是渲染条件。
- 现有 `trace-waterfall.test.ts` 的夹具按「`occurred` 即开始」构造，**与新语义一致，不必改**。
- 三条缺陷彼此独立；缺陷二在语义上依赖缺陷一先修（否则「早于父 span」的判断建立在错误的时间基准上）。
