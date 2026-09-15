# Feature Specification: 诊断模拟器的数据形态

**Feature Branch**: `claude/spec-012-diag-simulator-shapes`

**Created**: 2026-09-15

**Status**: Draft

**Input**: User description: "扩充 `server/internal/content/diagnostics` 的模拟器与固定夹具，使诊断手动验收矩阵中因缺少数据形态而无法执行的条目变为可执行：并发 span、孤儿 span、单 span 运行、>200 span 运行、not_run / failed / 冲突的回归结果、空 module 或 build、无版本追踪与跨页同对象、可注入的 sink 失败与缓冲丢弃。"

**Traces to**: `docs/development/manual-ui-runbook.md` 的结果登记表（2026-09-15 实跑：18 通过 / 4 未通过 / 42 未执行）；`specs/002` / `specs/006` / `specs/008` 三份 `manual-ui-todo.md` 中标注「缺少对应数据」的条目。

**依赖**：本特性与 `specs/011` 都改动 `simulator.go`。**实施排在 011 合入之后**，本 PR 只产出规格。

## Current State（以代码为准）

核实自 `app-main` @ `a6bf4ff`。以下逐条给出**今天为什么造不出**，依据是代码而不是推测。

### 0. 先说一个数目对不上的地方

任务描述说 42 条未执行里有 **19 条**是「模拟器造不出数据形态」，但 (a)～(h) 实际点名了 **15 条**：

```text
W-3, W-5, W-6, W-7, W-8, V-1, V-3, V-5, L-4, L-5, O-2, O-3, V11-1, V11-2, V05-15
```

差 4 条。本规格覆盖点名的 15 条；**另 4 条是哪几条需要主任务补全**（**Q1**）。不补全的风险不是漏做，而是交付后仍有 4 条卡在同一个原因上，而没人知道是哪 4 条。

**主任务 2026-09-15 裁决（Q1 已结）**：那 4 条是 `008-J-4`、`002-V05-14`、`002-V08-11`、`002-V11-08`。预检报告的 19 条口径包含了它们，但**没有一条是数据形态**：

| 条目 | 归属 | 处理 |
|---|---|---|
| `008-J-4`（过期追踪） | 保留期路径 | 与 `006-L-5` 同一处理：`LORETIDE_DIAG_RETENTION_DAYS=1` |
| `002-V05-14`（保留期外记录） | 保留期路径 | 同上 |
| `002-V08-11`（他人 workspace 的 `run_id`） | 授权路径 | 需 `handler-tests` 工作区的成员身份；不是数据形态 |
| `002-V11-08`（> 200 条实时事件） | **已被覆盖** | `shape_deep` 一次写 250 个事件，归入「已覆盖」 |

因此最终账是 **已覆盖 14 条**（13 + `002-V11-08`）、**移出 4 条**（`006-L-5`、`008-O-2` 与两条保留期路径）、**另有 1 条 `002-V08-11` 转授权路径**，合计 19 条，**仍不可达 0 条**。造法写在 `docs/development/manual-ui-runbook.md` 的 `§3.1` 与 `§3.2`。

### 1. 瀑布形态：`Simulate` 的结构决定了 span 只能是一条直线

`simulator.go` 的循环每步做两件事：

```text
child, parent := Child(ctx)      // parent = 上一步的 span
...
ctx = transport                  // 下一步的父 = 这一步的 child
```

外加 `now = now.Add(duration)` 在写事件**之前**推进虚拟时钟。两者合起来决定了：

| 条目 | 要的形态 | 今天为什么造不出 |
|---|---|---|
| **006-W-3** | 两个**非父子且时间重叠**的 span | **父子关系是一条链**：每个 span 都是下一个的父，所以任意两个 span 都是祖先／后代关系，**没有一对是「非父子」**。这一条单独就排除了 W-3 |

> **实施期修正（T001，基线 `7a779d6`，011 合入后）**：本表初稿还写了第二条理由——「时间逐步累加，区间首尾相接、**永不重叠**」。011 之后这句**不再普遍成立**：`Occurred` 改为步骤的**开始**时刻，`clock_skew` 会把该步的开始挪到父 span 之前，于是它可以与某个**祖先**的区间重叠。
>
> **W-3 的结论不变**，因为它要的是「**非父子**且重叠」，而链式拓扑让每一对都是祖先／后代。变的是理由的条数：从两条独立理由收窄为一条。`TestSimulatedStepsAreContiguousInTime` 与 `TestOnlyClockSkewProducesOutOfOrderTime` 精确锁住了这个边界。
| **006-W-5** | 父 span 指向**本次运行之外** | `parent` 只来自本次运行内上一步的 `Child(ctx)`；`i==0` 时置空。**没有任何路径能写入一个外部 span id** |
| **006-W-6** | **只有单个 span** 的运行 | `steps` 是固定的 9 个组件。提前中断只发生在 `stepCode != ""` 时，而全部故障点都落在 `daemon`(i=4) / `executor`(i=5) / `tool`(i=6) / `result`(i=7)。**最短的运行是 6 个事件**（`model_auth`），没有任何场景能停在第 1 步 |
| **006-W-7 / W-8** | span 数 **> 200** | 同上，`len(steps) == 9` 是**硬上限**。今天的运行最多 9 个 span |

#### 1a. `006-W-7` 还有第二道卡点，在读取一侧

只把 span 造够是不够的。逐环节核实：

| 环节 | 代码 | 数值 |
|---|---|---|
| 运行详情取事件 | `store.go` `GetRun` → `s.Query(ctx, scope, Filter{Run: id, Limit: 100})` | 100 |
| `Query` 的上限夹取 | `store.go` `if f.Limit < 1 \|\| f.Limit > 100 { f.Limit = 50 }` | 硬上限 100 |
| 瀑布折叠阈值 | `trace-waterfall.ts` `cap = options.cap ?? STREAM_EVENT_CAP`；`contract.ts` `STREAM_EVENT_CAP = 200` | 200 |
| 折叠条件 | `trace-waterfall.ts` `if (order.length > cap)` | `order.length > 200` |
| 面板传入 | `index.tsx:351` `buildTraceWaterfall(run.events, ...)`；`run = d.detail.data`（`queries.ts:18` → `runs?run_id=`） | `run.events` ≤ 100 |

`100 > 200` 永远为假。**即使运行有 250 个 span，`006-W-7` 的「另有 N 条」折叠提示也出不来**，`006-W-8`（点开折叠）随之无从谈起。只做「产出 250 个 span」这一半，交付后这两条**仍然跑不了**——正是 SC-001 禁止的结果。因此 FR-005 同时覆盖读取一侧。


### 2. 回归判定：三种状态在正常路径上都写不进库

`Run.Regression` 初值是 `"not_run"`（`simulator.go:15`），但 `Service.Run` 在提交前**无条件**调用 `Evaluate`：

```text
Evaluate(&run); if err = s.Store.CommitRun(...)      // service.go:25
```

`Evaluate` 只写 `passed` / `failed`（`simulator.go` 末行）。四态判定在前端 `packages/core/content/diagnostics/regression.ts`：

```text
NEVER_EVALUATED = {"", "not_run"}      → not_run
raw == "failed"                        → failed
raw == "passed" && status ∈ {completed, failed} → passed
其余（含未知枚举、passed 但 status 不是已完成）→ undecidable
```

| 条目 | 要的状态 | 今天为什么造不出 |
|---|---|---|
| **006-V-1** | `not_run` **落库** | `Evaluate` 无条件覆盖初值。`not_run` 只在内存里存在一瞬，**永远不会被写进 `content_diagnostic_run`** |
| **006-V-3** | `failed` | `Evaluate` 判 `run.Actual != run.Expected` 才写 failed。而模拟器对每个场景都**按定义产出它自己的 Expected**——逐场景核对过（含 `duplicate` 的两次 `Receive`、`clock_skew` 不中断循环、`database` 在 `Service.Run` 里补写 `Actual`），**没有一个场景会不相等**。failed 意味着模拟器自身坏了，不是可构造的数据 |
| **006-V-5** | `undecidable`（界面「无法判定」） | 需要 `passed` 配上一个不在 `{completed, failed}` 里的 status，或一个未知的 regression 枚举值。`Simulate` 只写 `completed` / `failed`，`Evaluate` 只写 `passed` / `failed`。**两个集合都封闭，交集为空** |

### 3. 关联信息：`module` 恒非空，`build` 已可空

```text
Module: "diagnostics"        // 硬编码
Build:  safeToken(build)     // 来自 os.Getenv("LORETIDE_BUILD")
```

| 条目 | 结论 |
|---|---|
| **006-L-4**（`module` 或 `build` 为空） | **一半今天就能做**：不设 `LORETIDE_BUILD` 时 `build` 为空。`module` 硬编码为 `"diagnostics"`，**没有任何输入能让它为空**。条目原文是「`module` **或** `build` 为空」，因此严格说今天可执行——但只能验到 `build` 那一半，`module` 那一半的呈现从未被看过 |

### 4. 对象与版本

```text
Version: "1"                 // Simulate 里硬编码，Sanitize 的 safeToken 会原样保留
ObjectID: run.ID             // 一次运行的全部事件共享同一个 ObjectID
```

技术日志页的分页大小是 **25**（`packages/views/content/diagnostics/index.tsx:574` 的 `limit: "25"`），且是常量、界面不可调。

| 条目 | 结论 |
|---|---|
| **008-O-2**（追踪内事件**都没有版本**） | **今天已经可以做，不需要动模拟器**。面板的「触发页面错误（测试）」按钮走 `Service.ClientError`，它构造的事件**不设 `Version`**（`service.go:34`），且 `Trace` 是一个全新 id——即一条只含一个无版本事件的追踪，正是 O-2 要的形态。建议移出本特性范围，只改 runbook 备注（**Q3**） |
| **008-O-3**（同一对象事件**跨多页**） | 造不出。一次运行最多 9 个事件共享同一个 `ObjectID`，而分页是 25。**9 < 25**，同一对象的事件永远落在同一页。条目原文提示「调小分页」，但分页大小是常量，界面没有这个开关 |

### 5. 计数器：今天只能靠破坏性操作或荒唐的重复

`LogBuffer` 容量 **1000**（`store.go:37`）。两个计数器的全部增长点：

| 计数器 | 增长点 | 触发条件 |
|---|---|---|
| `Dropped` | `LogBuffer.Append` 满环丢弃（`log.go:105`）；`Store.Technical` 写失败（`store.go:191`）；outbox 溢出 | 满环需要 **1000 条事件 ≈ 112 次运行**；写失败需要让表不可写 |
| `Errors` | `Store.Technical` 写失败（`store.go:190`）；`PruneTechnical` 失败（`service.go:25`）；outbox 派发失败 | 均需要真实的写入失败 |

| 条目 | 结论 |
|---|---|
| **002-V11-1**（`sink_errors > 0`）、**002-V11-2** / **002-V05-15**（`dropped > 0`） | 今天只有两条路：**跑 112 次模拟**把环填满（只能得到 `dropped`，得不到 `sink_errors`），或**让某张表不可写**——后者正是 runbook `§9` 的破坏性操作，跑完整个实例的数据作废。所以这三条实际上被绑在了 `§9` 上，而 `§9` 又要求「跑完重建实例」，于是它们与 `§6` 概览的其余条目**无法在同一轮里完成** |

### 6. 隔离门禁今天已经存在，但只覆盖 `Simulate`

`Simulate` 首行即 `if !testEnabled || !scope.Allows(...) { return Run{}, ErrDenied }`，`testEnabled` 由 `cmd/server/router.go:443` 传入：

```text
os.Getenv("APP_ENV") == "development" && os.Getenv("LORETIDE_DIAGNOSTICS_TEST") == "1"
```

**瀑布形态、回归状态、关联信息、对象版本这四类都经过 `Simulate`，天然继承这道门。** 但第 5 类（sink 失败与缓冲丢弃）的注入点在 `Store` / `LogBuffer` 一侧，**不经过 `Simulate`，没有现成的门**（**Q2**）。

### 7. `Scenarios` 与 `docs/13` §7 今天的对表关系

`Scenarios` 共 **16** 项，`docs/13` §7 列举 **15** 项，关系是 **§7 ⊆ Scenarios**，多出的一项是 `clock_skew`（对照表已记，主任务 2026-09-14 于 `c9cc63eca` 核对）。

`Scenario` 结构只有两个字段：

```text
type Scenario struct{ ID string; Expected string }
```

**没有任何字段能表达「这一项是业务故障」还是「这一项是诊断自验专用的数据形态」。** 新增项一旦混进同一个列表，§7 的对表口径就从「§7 ⊆ Scenarios」变成一句说不清的话（**Q2**）。

`Scenarios` 经 `Overview` 下发给前端，是**对外形状**，加字段要走 API 兼容规则。

### 不在本功能范围

不碰 `server/internal/daemon` 与任何上游 Multica 代码；不改真实执行器的禁用状态；不削弱 `Sanitize` 白名单、审计写失败即整体回滚两条既有不变量；不写 UI 单测；不改 CI 触发条件；不新增前端读取路径。

## Clarifications

### Session 2026-09-15（待主任务裁决，已按推荐值暂定，不阻塞）

- **Q1**（覆盖范围）：任务描述说 19 条，(a)～(h) 实际点名 15 条，差 4 条。→ **暂定 A**：本特性覆盖**点名的 15 条**，并在交付记录里列出「已覆盖 15 条」与「未点名的 4 条待主任务补全」。见 FR-001。
  - **已裁决（2026-09-15，012 合入后）**：4 条是 `008-J-4`、`002-V05-14`、`002-V08-11`、`002-V11-08`。前三条分属保留期与授权路径，不由模拟器承担；`002-V11-08` 已被 `shape_deep` 覆盖。详见 Current State 第 0 节。**Q1 关闭。**
- **Q2**（新形态在 `Scenarios` 里怎么标注）：硬约束要求新形态出现在 `Scenarios` 里，又要求 §7 对表口径不变糊。→ **暂定 A**：给 `Scenario` 增加一个**分类字段**（业务故障 / 诊断自验），使「哪些属于 §7」成为**结构里的事实而不是文档里的约定**，并让对表可以被机器核对。代价是 `Scenarios` 是对外形状，要同步前端解析与畸形响应测试。见 FR-012、FR-013。
- **Q3**（第 5 类的注入门禁 + 两项移出）：sink 失败与缓冲丢弃的注入点不经过 `Simulate`，没有现成的门；另有两条不该由模拟器承担。→ **暂定 A**：注入点**复用同一个判定**（`APP_ENV=development && LORETIDE_DIAGNOSTICS_TEST=1`），由服务装配处一次性决定并注入，**模块内不读环境变量**；**006-L-5 与 008-O-2 移出本特性**，理由见 FR-020、FR-021。见 FR-015、FR-016。
  - **plan 阶段的修正（research D5）**：把 sink 失败本身做成一个 shape 场景后，它走 `Simulate` 的既有门禁，**不需要装配处注入、不需要新开关、不需要新环境变量**，恢复也不需要动作。Q3 问的「不经过 `Simulate` 的注入点没有现成的门」这个问题因此消失。FR-015 / FR-016 的要求不变，只是被平凡满足。主任务若坚持原暂定值，装配注入同样可行，代价是多一个开关和一条会被忘记的人工关闭步骤。

## User Scenarios & Testing *(mandatory)*

### User Story 1 - 验收矩阵里「缺少对应数据」的条目变成可跑 (Priority: P1)

跑手动矩阵的人，按 runbook 造一遍数据，就能把此前标「缺少对应数据」的条目挨个走完，不再需要猜「这个形态要怎么弄出来」。

**Why this priority**: 这是本特性存在的唯一理由。当前 42 条未执行里有近三成卡在同一个原因上——**不是没时间，是没有那个数据**。不解决它，DG-01 的出口永远差这一口，而且差的是同一口。

**Independent Test**: 在一个隔离实例上，按 runbook 的造数步骤跑一遍，逐条确认目标形态出现在界面上（并发条、孤儿标注、单行瀑布、折叠提示、「未运行」、「无法判定」、「未记录」、跨页版本卡片、非零计数）。

**Acceptance Scenarios**:

1. **Given** 一个隔离实例，**When** 按 runbook 造数，**Then** 本特性覆盖的每一条都能得到它要的数据形态，无一条仍需标「缺少对应数据」。
2. **Given** 某一条形态在实施后仍造不出，**When** 交付，**Then** 交付记录 MUST 写明是哪一条、为什么，MUST NOT 悄悄留着旧备注。
3. **Given** 一条被移出范围的条目，**When** 读 runbook，**Then** 它的备注说明的是**移出的理由**，不是「缺少对应数据」。

---

### User Story 2 - 这些形态只在隔离实例里可达 (Priority: P1)

生产路径拿不到任何一个新形态：注入不了故障，伪造不了回归结果，改不了 span 的父子关系。

**Why this priority**: 与 US1 同为 P1，且**优先级更硬**。本特性做的事情本质上是「让系统能产出反常数据」，这正是隔离规则要挡住的（出处：constitution 的 Development Workflow 一节与 `docs/development/ai-collaboration.md`，加上原则 IX「真实执行器保持禁用」；任务描述记作「原则 VII」有误，原则 VII 是 UI 复用规则，见 plan.md）。一个能在生产伪造 `passed` 的开关，比 19 条跑不了的验收项危险得多。

**Independent Test**: 在 `testEnabled` 为假、或环境变量不满足的条件下，逐个入口尝试构造新形态，全部被拒；并有负例测试固定这一点。

**Acceptance Scenarios**:

1. **Given** `testEnabled` 为假，**When** 请求任一新形态，**Then** 被拒绝，且 MUST NOT 产生任何一条记录。
2. **Given** 生产配置（`APP_ENV` 非 development 或 `LORETIDE_DIAGNOSTICS_TEST` 非 `1`），**When** 尝试注入 sink 失败或缓冲丢弃，**Then** 注入点不存在或不生效。
3. **Given** 任一新形态，**When** 它写出的事件经过既有脱敏，**Then** 脱敏白名单**未被放宽**，审计写失败仍然整体回滚。

---

### User Story 3 - 对表口径不会因为新增场景而变糊 (Priority: P2)

读 `Scenarios` 列表的人，能一眼分清哪几项是 `docs/13` §7 要求的业务故障，哪几项是诊断自验专用的数据形态。

**Why this priority**: §7 的对表刚在 2026-09-14 完成（「§7 的 15 项 ⊆ Scenarios 的 16 项」）。本特性会往同一个列表里加东西，**如果不同时说清分类，那次对表的结论第二天就作废了**——而它是 D13-V07 记「部分」而非「未满足」的依据之一。

**Independent Test**: 按分类过滤 `Scenarios`，业务故障那一类应当仍然**完全覆盖** §7 的 15 项；诊断自验那一类与 §7 无交集。

**Acceptance Scenarios**:

1. **Given** 新增形态后的 `Scenarios`，**When** 按分类过滤，**Then** 业务故障类仍完全覆盖 §7 的 15 项。
2. **Given** 同一列表，**When** 读诊断自验类，**Then** 每一项都能说出它服务于哪一条验收条目，MUST NOT 出现无人认领的项。
3. **Given** 对照表的 §7 对表结论，**When** 本特性交付后重读，**Then** 结论仍然成立且措辞已更新，MUST NOT 停留在「16 项」这个过时数字上。

---

### Edge Cases

- 新形态与既有场景同名或语义重叠：场景 id 是前端下拉的值，重名会让用户选到意外的东西。
- `>200 span` 的运行会产生 200+ 条事件，落库与技术日志的容量、分页、保留期都会被它影响：一次这样的运行会把 1000 条的环占掉五分之一。
- 孤儿 span 的 `parent_span_id` 必须是**合法格式但不属于本次运行**；`Sanitize` 对不合法的 span id 会清空，清空后就不是孤儿而是顶层。
- 注入 sink 失败后，**失败状态要能恢复**：否则该实例剩下的验收全部在失败态下进行。
- `not_run` 落库后，既有的「回归结果」读取路径要能接住它——前端已支持（`NEVER_EVALUATED` 含 `not_run`），但后端读取与导出路径未被验证过。
- 并发 span 的时间重叠必须**可见地**重叠，而不是差几毫秒：验收要的是人眼能看出来。
- 同一对象跨页需要该对象的事件数 **> 25**（当前分页常量）；若将来分页可调，形态的构造条件也要跟着说清。

## Requirements *(mandatory)*

### 覆盖范围（US1）

- **FR-001**: 交付 MUST 覆盖任务描述点名的条目，并 MUST 在交付记录中逐条列出「已覆盖 / 已移出 / 仍不可达」三类；数目与描述不符时 MUST 写明差异（Q1 = A 暂定）。
- **FR-002**: 系统 MUST 能产出**两个非父子且时间可见重叠**的 span（`006-W-3`）。
- **FR-003**: 系统 MUST 能产出 `parent_span_id` 指向**本次运行之外**的 span，且该 id MUST 是合法格式（否则会被既有脱敏清空而退化为顶层节点）（`006-W-5`）。
- **FR-004**: 系统 MUST 能产出**只含单个 span** 的运行（`006-W-6`）。
- **FR-005**: 系统 MUST 能产出 span 数 **> 200** 且**失败步骤位于深层**的运行（`006-W-7` / `W-8`）。**产出不够用**：运行详情读取路径 `GetRun` → `Store.Query(Filter{Run:id, Limit:100})` 把该次运行的事件截在 **100** 条，而瀑布的折叠阈值是 `STREAM_EVENT_CAP = 200`，`100 > 200` 永远为假。因此本条 MUST 同时让运行详情**取全该次运行的事件**（同一端点、同一响应形状，不构成新增读取路径），否则折叠提示永不出现、`006-W-7` / `W-8` 仍不可执行。见 plan.md research D2。
- **FR-006**: 系统 MUST 能把 `regression = not_run` 的运行**写进库**，而不只是内存态（`006-V-1`）。
- **FR-007**: 系统 MUST 能产出 `regression = failed` 的运行（`006-V-3`）。
- **FR-008**: 系统 MUST 能产出被前端判定为**无法判定**的运行（`006-V-5`）。判定规则以 `describeRegressionVerdict` 为准，MUST NOT 为此改动该规则。
- **FR-009**: 系统 MUST 能产出 `module` 为空的运行（`006-L-4` 中今天无法验证的那一半）。
- **FR-010**: 系统 MUST 能产出同一对象事件数 **> 当前分页大小**的情形，使其跨页（`008-O-3`）。
- **FR-011**: 系统 MUST 提供可注入的 sink 写入失败与缓冲丢弃，使 `sink_errors` 与 `dropped` 大于 0（`002-V11-1` / `002-V11-2` / `002-V05-15`），且 MUST 能**恢复**到正常状态，MUST NOT 要求为此重建实例。

### 分类与对表（US3）

- **FR-012**: 新增形态 MUST 出现在 `Scenarios` 列表中。
- **FR-013**: `Scenarios` 的每一项 MUST 可判定它属于**业务故障**还是**诊断自验专用形态**，且该判定 MUST 是结构里的事实而非仅文档约定（Q2 = A 暂定：增加分类字段）。因 `Scenarios` 是对外形状，前端解析 MUST 同步更新并 MUST 附一条畸形响应测试。
- **FR-014**: 交付 MUST 更新 `docs/13` §7 的对表说明：业务故障类 MUST 仍完全覆盖 §7 的 15 项；诊断自验类 MUST 逐项说明它服务于哪一条验收条目。

### 隔离与不变量（US2）

- **FR-015**: 全部新形态 MUST 只在隔离实例语境下可达，判定沿用既有的 `testEnabled`（`APP_ENV=development && LORETIDE_DIAGNOSTICS_TEST=1`）（Q3 = A 暂定）。
- **FR-016**: 不经过 `Simulate` 的注入点（sink 失败、缓冲丢弃）MUST 由服务装配处一次性决定并注入其可用性，**模块内 MUST NOT 读环境变量**。
- **FR-017**: MUST 有**负例测试**证明隔离实例之外无法注入故障、无法伪造回归结果——隔离规则（constitution Development Workflow + `docs/development/ai-collaboration.md`，及原则 IX）MUST 对每一类新形态**各有一条**断言。
- **FR-018**: 本特性 MUST NOT 放宽 `Sanitize` 的白名单、MUST NOT 改变「审计写失败即整体回滚」、MUST NOT 改变真实执行器的禁用状态。
- **FR-019**: 本特性 MUST NOT 改动 `server/internal/daemon` 或任何上游 Multica 代码。

### 显式排除

- **FR-020**: **`006-L-5`（原故障运行不可读）MUST 移出本特性**。它要的是「被引用的运行已被保留期裁剪或跨工作区不可读」，那是**保留期与授权**的行为，不是模拟器能构造的数据形态——让模拟器造一个"不可读的运行"，等于让它写一条本不该存在的记录。该条目 MUST 改为由保留期配置（`LORETIDE_DIAG_RETENTION_DAYS` 最小 1 天）或跨工作区引用来验证，并在 runbook 中改写备注。
- **FR-021**: **`008-O-2`（追踪内事件都没有版本）MUST 移出本特性**。核实结果是**它今天已经可以做**：面板的「触发页面错误（测试）」按钮走 `Service.ClientError`，该事件不设 `Version` 且自带全新 `Trace`，正是该条目要的形态。为它增加模拟器形态是重复劳动。该条目 MUST 只更新 runbook 备注（Q3 = A 暂定）。

### 文档回写

- **FR-022**: 交付 MUST 更新 `docs/development/manual-ui-runbook.md` 与三份 `manual-ui-todo.md` 中相关条目的备注：由「缺少对应数据」改为**可执行的造数步骤**；被移出的条目改为**移出理由与替代验证方式**，MUST NOT 保留已失效的旧备注。
- **FR-023**: 本特性 MUST NOT 新增 UI 单测；界面呈现仍由手动矩阵验收。

### Key Entities

- **数据形态（Shape）**：一种为验证界面呈现而存在的、正常业务流程不会产生的记录形状。它不是业务故障。
- **场景分类（Scenario kind）**：`Scenarios` 每一项的归属——业务故障（对着 `docs/13` §7）或诊断自验专用形态。
- **注入点（Injection point）**：让 sink 写入失败或缓冲丢弃的可控开关，带隔离门禁与恢复路径。

## Success Criteria *(mandatory)*

- **SC-001**: 本特性覆盖的每一条验收条目，在隔离实例上按 runbook 的造数步骤都能得到目标形态；仍需标「缺少对应数据」的条目数为 **0**。
- **SC-002**: `Scenarios` 中按分类过滤出的业务故障项，**完全覆盖** `docs/13` §7 的 15 项；诊断自验项与 §7 的交集为 **0**。
- **SC-003**: 诊断自验类的每一项都能指到它服务的验收条目编号，指不到的项数为 **0**。
- **SC-004**: 隔离实例之外，每一类新形态各有一条负例断言证明其不可达；缺断言的类数为 **0**。
- **SC-005**: 注入 sink 失败后可恢复到正常状态，恢复后 `sink_errors` 不再增长——验证不需要重建实例。
- **SC-006**: `Sanitize` 白名单的成员数量与内容在本特性前后**逐项一致**；审计回滚与执行器禁用的既有断言全部仍然通过。
- **SC-007**: `server/internal/daemon` 与上游 Multica 代码的改动行数为 **0**。
- **SC-008**: 新增 UI 单测数为 **0**。
- **SC-009**: runbook 与三份 `manual-ui-todo.md` 中，本特性覆盖或移出的条目，其备注**没有一条**仍写着「缺少对应数据」。
- **SC-010**: `Scenarios` 是对外形状，其解析在前端有对应的畸形响应测试。

## UI Impact

**无新增 UI 单测。** 本特性改变的是**能被界面显示的数据**，界面代码本身不在范围内；呈现是否正确由手动矩阵验收——这正是本特性要让那些条目变得可跑的原因。

## Assumptions

- 手动矩阵的条目措辞以三份 `manual-ui-todo.md` 为准，runbook 是它们的执行顺序（`docs/development/spec-kit-workflow.md` 的口径）。
- 分页大小当前是常量 25；若实施期间它变成可配置，`008-O-3` 的构造条件要跟着改，但「事件数必须大于分页大小」这条不变。
- 真实执行器在本特性期间保持禁用；新增形态全部是模拟数据，不涉及真实执行。
- 本特性与 `specs/011` 都动 `simulator.go`，**实施排在 011 合入之后**，以 011 合入后的 `simulator.go` 为基线重新核实 Current State 第 1 节。
- 三处 Q1/Q2/Q3 已按推荐值暂定并写入 FR，主任务裁决若不同，改动范围各自局限在少数 FR 与其对应任务。
