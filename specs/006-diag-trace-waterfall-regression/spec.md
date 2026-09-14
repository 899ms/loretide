# Feature Specification: 诊断 trace 瀑布视图与回归关联

**Feature Branch**: `006-diag-trace-waterfall-regression`

**Created**: 2026-09-14

**Status**: Draft

**Input**: User description: "诊断 trace 瀑布视图与回归关联——闭合 `docs/development/diagnostics-acceptance-mapping.md` 中 DIAG-09「交付 trace 瀑布」实现缺失，以及 D13-V09（回归结果能定位原故障、输入场景、模块和代码版本；未运行/失败不会被当通过）三条无覆盖子句；DIAG-13 卡片的「回归关联」部分。"

**Traces to**: `tasks/diagnostics.md` DIAG-09「交付 trace 瀑布」、DIAG-13「故障/场景/模块/提交关联」「未执行不显示通过」；`docs/13` D13-V09；`docs/development/diagnostics-acceptance-mapping.md` 第 4.3 节缺口 3。

## Current State（以代码为准，不以对照表的一句话为准）

`diagnostics-acceptance-mapping.md` 把 DIAG-09「交付 trace 瀑布」记为「无证据」。逐行读过 `packages/views/content/diagnostics/index.tsx` 之后，更准确的说法是：**有一个扁平的耗时条列表，缺的是层级与时间轴**。本规格按后者写，只补缺口。

已核实的事实（`app-main` `0bd37da87`）：

- **数据已齐备，本功能不新增任何持久化字段。**
  - `server/internal/content/diagnostics/contract.go` `Event` 已有 `span_id`、`parent_span_id`、`duration_ms`、`occurred_at`、`attempt`、`step`、`component`。
  - 同文件 `Run` 已有 `regression`、`module`、`build`、`original_run_id`、`scenario`、`seed`、`expected_code`、`actual_code`、`status`。
  - `packages/core/content/diagnostics/contract.ts` 的 `runSchema` / `eventSchema` 已逐字段解析并转成 camelCase，含 `originalRunId`、`regression`、`module`、`build`、`spanId`、`parentSpanId`、`durationMs`。
- **现有 trace 标签页（`index.tsx` 约 686–730 行）渲染的是**：按 `run.events` 数组原序的扁平列表；每行一个 `Progress`，宽度 = `durationMs ÷ max(所有 durationMs)`；`spanId` 与 `parentSpanId` 以纯文本并排打印；序号取数组下标 `i + 1`。
  - 因此**没有**父子缩进，**没有**按开始时刻定位的横轴，**看不出**并发与等待间隙，`parent_span_id` 只是被打印出来而没有被用来构造层级。
- **回归关联现状（`index.tsx` 约 895–906 行）**：运行列表逐列显示 `r.regression`、`r.module`、`r.originalRunId.slice(0, 8) || "—"`。四个定位信息里有三个已显示，但只是文本，没有从回归结果跳回原故障运行的路径。
- **`Evaluate()` 只有两态**：`server/internal/content/diagnostics/simulator.go` 第 47 行 `func Evaluate(run *Run){run.Regression="failed";if run.Actual==run.Expected{run.Regression="passed"}}`。未评估过的 `Run.Regression` 是零值空串，面板直接 `{r.regression}` 渲染成**空单元格**——既不是「通过」也没说「未运行」，是一个沉默的空白。
- **`packages/core/content/diagnostics/` 下没有任何瀑布或判定的纯函数**（检索 `waterfall` / `buildTree` / `span` 无命中）。现有纯函数只有 `mergeEvents`、`streamQuery`、`stream-state.ts` 的状态机。
- 面板已有文案 `text007 = 暂无记录。未执行不表示已通过。`，但那是**列表为空**时的提示，与「某一条运行未评估」是两回事。

**本功能只做三件事**：把 `parent_span_id` 与时间用起来（层级 + 时间轴）；给回归结果一个不会把「未运行」读成「通过」的显式判定；把四个定位信息接成可跳转的关联。**不重建**现有事件列表、不改数据库、不改 `Event`/`Run` 的字段集合。

## User Scenarios & Testing *(mandatory)*

### User Story 1 - 从瀑布图看出一次运行的时间都花在哪 (Priority: P1)

开发者打开某次运行的 trace 标签页，看到的是按父子关系缩进、按开始时刻横向定位的瀑布：哪一步套在哪一步里面、哪两步是并发的、两步之间的空白等了多久，一眼可见。

**Why this priority**: 这是 DIAG-09 唯一被记为实现缺失的交付项，也是 D13-V03「trace 连续、缺口可见」在界面上的落点。扁平等宽条能告诉你「哪步最慢」，但回答不了「为什么这次运行总耗时是 8 秒」——父子与间隙才能。

**Independent Test**: 用一次已保存的模拟运行（例如 `slow` 场景）打开 trace 标签页；缩进层级与 `parent_span_id` 一致，每行左边距对应其相对开始时刻，行间空白对应真实等待。不需要回归关联部分存在即可验证。

**Acceptance Scenarios**:

1. **Given** 一次含父子 span 的运行，**When** 打开 trace 标签页，**Then** 子 span 相对父 span 缩进一级，且不早于父 span 开始、不晚于父 span 结束地显示。
2. **Given** 两个 span 时间区间重叠且互不为父子，**When** 查看瀑布，**Then** 两者在横轴上可见地重叠，而不是被排成前后两行等长条。
3. **Given** 某 span 的 `parent_span_id` 在本次运行的事件集合中不存在（跨运行或已被保留期裁剪），**When** 查看瀑布，**Then** 该 span 作为顶层显示并明确标注「父 span 不在本次运行内」，不被静默丢弃，也不挂到错误的父节点上。
4. **Given** 一次运行只有单个 span，**When** 查看瀑布，**Then** 正常显示单行，不出现除零或空图。

---

### User Story 2 - 回归结果不会把「没跑」显示成「通过」 (Priority: P1)

开发者查看运行列表时，每条回归结果显示的是一个明确的判定：通过 / 未通过 / **未运行** / 无法判定。未评估过的运行显示「未运行」，绝不显示为空白或通过。

**Why this priority**: D13-V09 的末句「未运行/失败不会被当通过」是验收口径本身，也是 constitution 原则 X 在界面上的体现。当前空串渲染成空单元格，是本功能里唯一可能导致**错误验收结论**的缺口——其余缺口只是看不清楚。

**Independent Test**: 构造四种 `Run`（已评估通过、已评估失败、`regression` 为空串、`expected_code` 与 `actual_code` 都为空的无法判定情形），判定函数分别返回四个不同的显式结果，且「未运行」与「无法判定」都不等于通过。可在 node 环境纯函数测试，不需要界面。

**Acceptance Scenarios**:

1. **Given** `regression` 为空串的运行，**When** 查看回归结果，**Then** 显示「未运行」，且该状态在任何聚合计数里都不计入通过。
2. **Given** `regression` 为 `passed` 的运行，**When** 查看，**Then** 显示通过，并同时显示其判定依据（`expected_code` 与 `actual_code`），使「通过」可复核。
3. **Given** `regression` 是既非 `passed` 也非 `failed` 的未知取值（后端新增枚举），**When** 查看，**Then** 显示「无法判定」并原样保留该取值，不猜测、不回落成通过。
4. **Given** 一次通过的运行，**When** 查看其说明文案，**Then** 明确表达「模拟结果符合该故障预期」而非「该功能可用」。

---

### User Story 3 - 从回归结果回到原故障 (Priority: P2)

开发者看到一条回归结果，能就地知道它对应哪次原故障运行、哪个输入场景、哪个模块、哪个代码版本，并能一键跳到原故障运行的详情。

**Why this priority**: D13-V09 前四个子句里「定位原故障」与「定位模块」在对照表中是无覆盖。四个信息中三个已在界面上以文本出现，缺的是**可跳转**与**缺失时的明确标注**，所以排在 P2：没有它 V09 仍不完整，但不会造成错误结论。

**Independent Test**: 对一条带 `original_run_id` 的运行，关联函数返回四个定位项及其可用性；对 `original_run_id` 为空的首次运行，返回「本身即原故障」而非「缺失」。纯函数可测。

**Acceptance Scenarios**:

1. **Given** 一条由某次原故障复现而来的运行，**When** 查看其回归结果，**Then** 可见原故障运行标识、场景、模块、代码版本四项，且原故障可点击跳转到该运行详情。
2. **Given** `original_run_id` 为空的首次运行，**When** 查看，**Then** 标注为「本身即原故障」，不显示为缺失或断链。
3. **Given** `original_run_id` 指向的运行已不可读（保留期裁剪或越权），**When** 查看，**Then** 明确标注「原故障运行不可读」并给出原因类别，不显示成空白，也不让跳转进入错误页。
4. **Given** `module` 或 `build` 为空，**When** 查看，**Then** 该项显示为「未记录」，而不是空白或推断值。

---

### Edge Cases

- **span 父子成环**（`a` 的父是 `b`，`b` 的父是 `a`，数据损坏或伪造）：层级构造必须终止并把环上的节点降级为顶层标注异常，不得无限递归或栈溢出。
- **`occurred_at` 早于父 span 的开始**（时钟偏差，已有 `clock_skew` 场景）：按实际值定位并标注偏差，不夹取、不重排成「看起来正常」。
- **`duration_ms` 为 0 或负数**：0 显示为最小可见宽度；负数按无效处理并标注，不产生负宽度。
- **事件数量很大**（实时流上限为 200 条，历史运行可能更多）：瀑布需要有明确的行数上限与「已截断」提示，与技术日志的 200 条上限口径一致。
- **全部事件 `duration_ms` 相同且为 0**：不得除零；横轴退化时仍显示层级。
- **运行存在但 `events` 为空**（`runSchema` 中 `events` 可为 `null`）：显示「本次运行没有可展示的 span」，不显示空瀑布框。
- **回归判定与运行状态冲突**（`status` 指示未完成但 `regression` 为 `passed`）：以「无法判定」呈现并同时显示两个原始值，不择一相信。

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: 系统 MUST 用 `parent_span_id` 把一次运行的 span 组织成层级，子节点相对父节点缩进；`parent_span_id` 在本次运行内找不到对应 span 的节点 MUST 作为顶层显示并标注原因，不得丢弃或错挂。
- **FR-002**: 系统 MUST 按每个 span 相对本次运行最早开始时刻的偏移量横向定位，使等待间隙与并发重叠可见；宽度 MUST 表示该 span 自身耗时。
- **FR-003**: 层级构造 MUST 对父子成环与自引用保持终止性，环上节点降级为顶层并标注异常。
- **FR-004**: 瀑布 MUST 有明确的行数上限与「已截断」提示，口径与技术日志一致；超限时保留层级正确性，不得只截断成一段无父节点的中间片段而不说明。
- **FR-005**: 系统 MUST 为回归结果提供四态显式判定：通过 / 未通过 / 未运行 / 无法判定。`regression` 为空串 MUST 判为「未运行」；未知取值 MUST 判为「无法判定」并原样保留原值。任何情况下 MUST NOT 把「未运行」或「无法判定」呈现为通过。
- **FR-006**: 显示「通过」时 MUST 同时显示其判定依据（`expected_code` 与 `actual_code`），使该结论可就地复核。
- **FR-007**: 系统 MUST 为每条回归结果给出四项定位：原故障运行、输入场景、模块、代码版本。任一项缺失 MUST 显示为「未记录」；`original_run_id` 为空 MUST 显示为「本身即原故障」，二者不得混为一谈。
- **FR-008**: 原故障运行 MUST 可从回归结果跳转到该运行详情；目标不可读时 MUST 标注不可读及原因类别，不得跳转到错误页或空白页。
- **FR-009**: 层级构造、时间轴计算、四态判定、四项定位 MUST 实现为不依赖 DOM 的纯函数，置于 `packages/core/content/diagnostics/`，可在 node 环境下测试（`// @vitest-environment node`）。
- **FR-010**: 本功能 MUST NOT 新增或修改 `Event` / `Run` 的字段集合，MUST NOT 新增数据库迁移，MUST NOT 改动 `Evaluate()` 以外已通过测试的后端行为。若判定需要「未运行」态而后端无法表达，MUST 在不改变现有 `passed` / `failed` 语义的前提下解决。
- **FR-011**: 本功能 MUST NOT 编写 UI 单元测试，MUST NOT 使用自动点击验收。页面行为 MUST 写入本功能的 `manual-ui-todo.md`，未执行记「待用户验证」。
- **FR-012**: 瀑布与回归关联 MUST 复用现有诊断页的授权口径，MUST NOT 引入新的数据读取入口或放宽 `Scope` 过滤。

### Key Entities

- **Span 行**：由一条 `Event` 派生的展示单元。派生属性：层级深度、相对开始偏移、自身耗时、父节点可达性、异常标注。不持久化。
- **回归判定**：由一条 `Run` 派生的四态结论及其依据（`expected_code` / `actual_code` / `regression` 原值）。不持久化。
- **回归关联**：由一条 `Run` 派生的四项定位及其可用性状态（可跳转 / 本身即原故障 / 不可读 / 未记录）。不持久化。

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: 对一次含至少两层父子与一处并发的运行，开发者能在 10 秒内从瀑布上说出「最慢的一步」与「最长的等待间隙」，无需查看原始 JSON。
- **SC-002**: 四态判定的纯函数用例覆盖全部四态及边界（空串、未知取值、状态冲突），在 node 环境下执行通过；「未运行」与「无法判定」在任何用例中都不返回通过。
- **SC-003**: 层级构造对成环、自引用、孤儿父、空事件集四类异常输入均在有限步内返回，且返回结果可被断言（不抛异常、不无限递归）。
- **SC-004**: 四项定位在「完整」「本身即原故障」「不可读」「未记录」四种组合下各有一条用例，结论互不相同。
- **SC-005**: 本功能新增的自动测试全部为 node 环境纯函数测试，UI 单测数量为 0；页面行为条目 100% 落在 `manual-ui-todo.md` 且初始状态为「待用户验证」。
- **SC-006**: `pnpm check:content-boundaries` 退出 0——新增纯函数位于 `packages/core/content/diagnostics/` 且不引入越界依赖。

## UI Impact

有页面改动：诊断页 trace 标签页的呈现方式，以及运行列表的回归结果列与关联跳转。

按 constitution 原则 II：不写 UI 单测、不做自动点击验收。页面行为逐条写入 `specs/006-diag-trace-waterfall-regression/manual-ui-todo.md`，由用户在本机浏览器验证，未执行记「待用户验证」，不记通过。

## Assumptions

- **数据充分性**：`span_id` / `parent_span_id` / `occurred_at` / `duration_ms` 足以构造层级与时间轴，无需后端补字段。这一点已按 `contract.go` 与 `contract.ts` 核实。
- **「未运行」的表达方式**：后端 `Evaluate()` 保持两态不变，「未运行」由前端从 `regression` 为空串推导。这样不动已通过测试的后端行为（FR-010），代价是「从未评估」与「后端将来显式置空」无法区分——当前后端不会显式置空，故可接受。
- **行数上限**：沿用技术日志既有的 200 条口径，不为瀑布单独定义新阈值。
- **跳转目标**：原故障跳转指向同一诊断页内的运行详情，不新增路由。
- **桌面端不在范围内**：诊断页仅挂载于 Web（`apps/web/app/[workspaceSlug]/(dashboard)/diagnostics/page.tsx`），与 spec 002 的 FR-012 一致。
- **不触碰的相邻缺口**：`diagnostics-acceptance-mapping.md` 列出的其余实现缺失——DIAG-05 的 HTTP trace 传播未接线、DIAG-04 无 outbox、DIAG-02 请求头与路径脱敏、DIAG-03 磁盘满模拟——均**不在本功能范围内**，各自另立任务。本功能不因「顺手」扩大范围（constitution 原则 VIII）。
- **对照表的更新**：本功能完成后 `diagnostics-acceptance-mapping.md` 相关行需要更新，但那是交付后的记录动作，由主任务按原则 X 决定何时进行，不在本功能的实现任务内。
