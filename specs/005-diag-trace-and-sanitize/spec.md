# Feature Specification: 诊断 HTTP 追踪贯通与请求脱敏

**Feature Branch**: `005-diag-trace-and-sanitize`

**Created**: 2026-09-14

**Status**: Draft

**Input**: User description: "诊断 HTTP 追踪贯通与请求脱敏——闭合 docs/development/diagnostics-acceptance-mapping.md §4.3 列出的三处实现缺失：(1) DIAG-05 HTTP 一段的 trace 传播未接线（handler/middleware 无 Pack/Unpack/traceparent），导致 D13-V03「从请求跨队列到 daemon 的 trace 连续」不成立；(2) DIAG-02 请求头脱敏规则与路径/URL 脱敏规则缺失（D13-V08）；(3) DIAG-04 卡片明文要求的事务/outbox 接口缺失（D13-V02/V11）。以当前代码为准写 Current State，只覆盖缺口，不重建已有能力。"

**Traces to**: `docs/development/diagnostics-acceptance-mapping.md` §4.3 第 3 条的三行「实现缺失」；DIAG-02、DIAG-04、DIAG-05；D13-V02、V03、V08、V11。

## Current State（以代码为准）

本节逐条核实自 `app-main` @ `0bd37da87`。**这三处都不是「没有代码」，而是「合同已有、某一段没接线」或「规则只以副作用形式存在、不可复用」。**

### 1. trace 的 HTTP 一段

- **合同已存在**：`server/internal/content/diagnostics/contract.go` 有 `Pack(ctx, operation, attempt, sequence) Envelope` 与 `Unpack(ctx, Envelope)`，内部用 W3C `propagation.TraceContext{}` 注入/提取；`Child(ctx)` 在父 span 有效时沿用其 `TraceID`，否则**随机生成一个新的 TraceID**。
- **队列与 WebSocket 两段已接线**：`transport.go` 的 `DecodeQueuedEnvelope()` 与 `ServeSimulationTransport()` 都调用 `Unpack`；`simulator.go` 调用 `Pack`。两段均有自动测试（`TestWebSocketQueuePropagationDuplicateAndRevocation`）。
- **HTTP 一段没接线**：`server/internal/handler/` 与 `server/internal/middleware/` 中检索 `Pack(` / `Unpack(` / `traceparent` / `TraceContext` **均无命中**。唯一的 HTTP 侧入口 `handler.DiagnosticTrace`（`content_diagnostics.go:54`）直接 `diagnostics.Child(r.Context())`——而 `r.Context()` 里没有从请求头提取出来的 span context，所以 `Child` 每次都看到「无效父级」，**每个 HTTP 请求都开一条全新的 trace**。入站 `traceparent` 被丢弃，出站不写 `traceparent`。
- **响应确实带 trace**，但格式是自有的 `X-Diagnostic-Trace: <32 位 trace id>`，不是 W3C `traceparent`，且只有 trace id、没有 span id 与采样位，无法作为下一跳的父级。
- **operation 也断在这一跳**：该中间件写技术事件时用 `Operation: diagnostics.NewID()`，每请求一个新 operation id，与客户端或上游的 operation 无关。
- **挂载范围**：`DiagnosticTrace` 只挂在 `/api/content-diagnostics` 路由组（`server/cmd/server/router.go:1868`），业务路由不经过它。

**后果**：D13-V03「从请求跨队列到模拟 daemon/工具/回写的 trace 连续」的首段断裂。队列与 WS 两段能连上，是因为它们共用同一个 `Envelope`；HTTP 入口既不接收也不传递，链路从第一跳就断了。

### 2. 请求头与路径/URL 的脱敏规则

- **现状是「靠白名单顺带挡住」**：`log.go` 的 `Sanitize()` 对扁平的 `Event` 逐字段做准入——枚举字段走 `oneOf()`，自由文本字段走 `safeToken()`（不匹配 `^[a-zA-Z0-9_.:-]{0,100}$` 就整体换成 `[redacted]`），ID 字段必须匹配 `^[a-f0-9]{16,32}$`。`SlogHandler` 只取 `error_code` / `component` / `trace_id` 三个属性，其余全部丢弃。
- **所以今天不会泄漏**：路径与 URL 落在 `Step`/`Build`/`Version` 里会被 `safeToken` 整体 `[redacted]`（`TestLogRegressionSanitizeRules` 已断言），请求头根本没有字段可落。
- **但没有可复用的规则**：`Event` **没有** header / path / method / url 任何字段；不存在「按请求头名称准入」的分支，也不存在「把 URL 削成安全形状」的函数。现有做法只有一种结果——**整体丢弃**。
- **这正是第 1 项的前置**：HTTP 一段接线后，技术事件需要记录「这是哪个请求」才有诊断价值。今天没有任何安全形状可用：记全路径会带上路径参数与查询串，不记则事件无法定位。因此两处缺口必须一起闭合，不能只做 trace。

### 3. 事务 / outbox 接口

- **事务一半已交付且有测试**：`store.go` 的 `withWorkspaceWrite()`（pgx 事务 + `WorkspaceWriteGuard`）与 `CommitRun(..., failAudit bool)`；`TestPostgresAuditRollbackIsolationAndRetention` 与 handler 侧 5 个 `TestContentDiagnosticWritesCoordinateWithWorkspaceDelete` 子用例覆盖「审计失败整体回滚」「技术日志失败只计数不拖垮业务」。
- **outbox 一半没有**：全仓检索 `outbox` 在诊断域无命中；唯一命中是 `seat_capacity_outbox`（迁移 `415`～`419`），属席位计费域，与诊断无关，**不可直接复用**。
- **缺口的实质**：今天「事务内提交」与「事务外投递」之间没有合同。`Store.Technical()` 在事务外写，失败只计数；`Audit()` 在事务内写，失败即回滚。介于两者之间的情形——**必须随业务事务一起决定生死、但要在提交之后才能投递**的记录——没有承载它的接口。

### 不在本功能范围

`§4.3` 另外 12 条缺口（`DIAG-02` 模型输出样例负例、`DIAG-03` 磁盘满模拟、`DIAG-09` trace 瀑布、6 条浏览器手动矩阵、`DIAG-13` 三条浏览器验证）不在本功能内。本功能不重建 `Pack`/`Unpack`/`Sanitize`/`withWorkspaceWrite`，不改诊断面板布局，不改模拟器场景集合。

## User Scenarios & Testing *(mandatory)*

### User Story 1 - 一个 trace id 串起从 HTTP 入口到 daemon 的整条链路 (Priority: P1)

开发者在诊断页看到一条失败记录，拿它的 trace id 去查，能看到同一条 trace 上依次出现 HTTP 入口、队列消息、模拟 daemon 回写——中间不断、不改 id。上游若已带自己的 trace，链路并入而不是另起一条。

**Why this priority**: D13-V03 的首段。队列与 WS 两段已自动化，唯独入口一段断裂，导致整条验收项无法成立——修好这一跳，V03 的「连续」子句才第一次可验证。

**Independent Test**: 构造一个带 `traceparent` 的请求打到诊断端点，读响应里的 trace 标识，再从该请求触发的模拟运行里取事件的 `trace_id`，三者相同；不带 `traceparent` 的请求自行开一条新 trace，其 id 同样贯穿到 daemon 段。

**Acceptance Scenarios**:

1. **Given** 请求带合法 `traceparent`，**When** 经过诊断路由，**Then** 该请求写出的技术事件、它触发的队列消息与 daemon 回写共用同一 `trace_id`，且父子关系可还原。
2. **Given** 请求不带 `traceparent`，**When** 经过诊断路由，**Then** 服务端生成一条新 trace，其 id 同样贯穿后续各段；响应告知调用方该 id。
3. **Given** 请求带格式非法或被伪造的 `traceparent`，**When** 经过诊断路由，**Then** 按与 `Unpack` 一致的方式拒绝采信，改为新开一条 trace，并记录一次可见的拒绝，不返回错误、不中断请求。
4. **Given** 同一 operation 的重试，**When** 第二次请求到达，**Then** 两次共用 operation 标识、各自有独立 attempt，可区分而不混淆。
5. **Given** 一次请求在处理中被取消或超时，**When** 记录结束事件，**Then** 该事件仍落在同一 trace 上，错误分类与既有六类一致。

---

### User Story 2 - 技术日志能说出「是哪个请求」，且不泄漏路径参数、查询串与请求头 (Priority: P1)

开发者看到一条技术事件时，能知道它来自哪个接口；同时诊断包里不出现凭据、令牌、路径中的标识、查询串内容或未准入的请求头。

**Why this priority**: 与 US1 同为 P1 且互为前提——不先定下安全形状，US1 接线后记录请求身份就会把敏感内容带进技术日志与导出包，直接违反 D13-V08。

**Independent Test**: 用带凭据请求头、路径参数与查询串的请求打到诊断端点；读回技术事件与导出包，其中出现的是可复用的安全形状，凭据/令牌/查询串内容一律不出现。

**Acceptance Scenarios**:

1. **Given** 请求带 `Authorization`、`Cookie` 等凭据类请求头，**When** 记录技术事件，**Then** 事件与导出包中都不出现其值，也不出现可还原原值的片段。
2. **Given** 请求带路径参数与查询串，**When** 记录技术事件，**Then** 记录的是不含具体取值的安全形状；未准入的部分整体不记，不做「截断后保留前缀」这类部分保留。
3. **Given** 一个未在准入名单上的请求头，**When** 记录技术事件，**Then** 该头既不出现名称也不出现取值。
4. **Given** 脱敏后的事件进入导出包，**When** 预览与下载，**Then** 两条路径得到同一套脱敏结果，`redacted` 为真。
5. **Given** 新增的规则被其他 sink 复用，**When** 任一 sink 写入，**Then** 走同一套规则，不存在绕过路径。

---

### User Story 3 - 需要随事务定生死、提交后才投递的记录有明确合同 (Priority: P2)

当一条诊断记录必须与业务事务同生共死，但只能在提交之后才能对外投递时，有一个接口承载它：事务回滚则该记录不存在，事务提交则该记录必被投递或可见地进入重试，不会静默丢失，也不会在回滚后仍被投递。

**Why this priority**: DIAG-04 卡片明文要求「事务/outbox接口」，事务一半已交付并有测试，outbox 一半无任何证据。它是 D13-V02「失败不产生虚假成功审计」与 D13-V11「关键审计失败按事务拒绝」的补齐项，但不像 US1/US2 那样阻塞 V03，故列 P2。

**Independent Test**: 让业务事务回滚，确认该记录未被投递也不可见；让事务提交但投递失败，确认记录不丢失且失败可见（计数或状态），业务操作本身不被拖垮。

**Acceptance Scenarios**:

1. **Given** 记录在事务内登记，**When** 事务回滚，**Then** 该记录不存在、不被投递，且不产生任何「已成功」的痕迹。
2. **Given** 记录在事务内登记，**When** 事务提交，**Then** 该记录必被投递，或进入可见的待投递状态；不静默丢弃。
3. **Given** 提交后投递失败，**When** 失败发生，**Then** 失败可见（可计数或可查状态），业务操作照常成功，与既有「技术日志失败只计数」的行为一致。
4. **Given** 同一记录被重复投递，**When** 接收方处理，**Then** 与既有 `Receiver` 去重语义一致，不产生重复业务效果。
5. **Given** 关键审计写入失败，**When** 事务结束，**Then** 仍按既有行为整体拒绝，不因引入本接口而被降级为「只计数」。

---

### Edge Cases

- 入站 `traceparent` 合法但采样位为「不采样」：链路仍需连续，不能因采样位丢弃 trace id。
- 同一请求内多次进入被记录的边界：各段有独立 span，不共用同一个 span id。
- 请求头名称大小写与重复出现：准入判定不受大小写影响；同名多值不得因拼接而泄漏未准入部分。
- 路径中出现凭据样式的片段（例如把令牌写进路径）：按未准入处理，不因为「它在路径里」就当作安全。
- 极长 URL 或超多请求头：记录有界，超出部分整体丢弃而非截断保留。
- 投递方长时间不可用：待投递记录不得无界增长，需与既有有界存储/丢弃可见的口径一致。
- 事务提交成功但进程随即退出：重启后该记录仍应可投递或可见，不静默消失。

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: 系统 MUST 在 HTTP 边界提取入站 trace 上下文，并把它作为后续段落的父级；提取失败或不可信时 MUST 新开一条 trace 而不是让链路断裂。
- **FR-002**: 系统 MUST 让 HTTP 入口、队列消息与 daemon 回写共用同一 trace 标识，且各段有独立 span 与可还原的父子关系。
- **FR-003**: 系统 MUST 向调用方回传本次请求的 trace 标识，使调用方能据此发起下一跳或提交问题报告。
- **FR-004**: 不可信或格式非法的入站 trace 上下文 MUST NOT 被采信，且 MUST NOT 使请求失败；拒绝本身 MUST 可见。
- **FR-005**: 系统 MUST 提供按请求头名称的准入规则；未准入的请求头 MUST NOT 以任何形式（名称、取值、片段、长度）进入技术日志或导出包。
- **FR-006**: 系统 MUST 提供路径与 URL 的脱敏规则，产出一个不含具体取值的安全形状；查询串内容 MUST NOT 进入技术日志或导出包。
- **FR-007**: FR-005 与 FR-006 的规则 MUST 是全部 sink 与导出复用的同一套规则，MUST NOT 存在绕过路径。
- **FR-008**: 脱敏 MUST 保持「整体丢弃优于部分保留」，MUST NOT 输出可还原原值的片段。
- **FR-009**: 系统 MUST 提供一个接口，使一条记录能在业务事务内登记、在提交之后才被投递；事务回滚时该记录 MUST NOT 存在也 MUST NOT 被投递。
- **FR-010**: 事务提交后的投递失败 MUST 可见且不丢失记录，MUST NOT 拖垮业务操作；关键审计失败 MUST 保持既有的整体拒绝行为，不得降级。
- **FR-011**: 待投递记录 MUST 有界，超出容量的处置 MUST 与既有「丢弃可见」的口径一致。
- **FR-012**: 本功能 MUST NOT 改变既有 `Pack`/`Unpack` 的线上形状、既有 `Sanitize` 已通过的字段级规则、既有事务与回滚行为；新增字段 MUST 向后兼容。
- **FR-013**: 本功能 MUST NOT 新增任何外发网络目标，MUST NOT 把诊断内容发往外部地址。
- **FR-014**: 交付 MUST 说明本功能覆盖了 `§4.3` 的哪三行、未覆盖哪几行，并 MUST NOT 声称任何 D13-V 条目已整体通过。
- **FR-015**: HTTP trace 传播的挂载范围 MUST 明确并有据可查 [NEEDS CLARIFICATION: 只覆盖 `/api/content-diagnostics` 路由组（`DiagnosticTrace` 当前挂载点），还是覆盖全部 API 路由？前者范围小、与现状一致；后者让 V03 对业务链路也成立，但触及所有路由的中间件栈]。
- **FR-016**: 入站 `traceparent` 的信任口径 MUST 明确 [NEEDS CLARIFICATION: 采信客户端提供的 `traceparent` 作为父级（链路真正连续，但调用方可自选或伪造 trace id 并借此关联他人记录），还是边界一律新开 trace、只把入站值作为旁证记录（不可被外部操纵，但跨系统链路在此处断开）？]。
- **FR-017**: outbox 的形态 MUST 明确 [NEEDS CLARIFICATION: 落库的持久 outbox（新表 + 投递方，跨进程重启可靠，但需新迁移并受无外键/索引 `CONCURRENTLY` 约束），还是不落库的事务内登记 + 提交后派发接口（无新表、范围小，但进程退出即丢失）？]。

### Key Entities

- **Trace 上下文**：一次请求在链路上的身份，含 trace 标识、span 标识、父 span 标识、采样位；跨 HTTP、队列、WebSocket 三种载体传递。
- **请求身份（脱敏后）**：技术事件中用来回答「这是哪个请求」的安全形状，不含路径参数取值、查询串内容与未准入请求头。
- **准入名单**：请求头名称的准入规则集合，与既有字段级白名单同一口径——列举允许项，其余一律不记。
- **待投递记录**：在业务事务内登记、提交后才投递的记录；有生死与事务绑定、投递失败可见、容量有界三个属性。

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: 带 `traceparent` 的请求触发的模拟运行，其 HTTP 入口事件、队列消息与 daemon 回写三段的 trace 标识完全一致（自动测试断言，非人工比对）。
- **SC-002**: 不带 `traceparent` 的请求同样产出贯穿三段的单一 trace 标识；调用方可从响应取得该标识。
- **SC-003**: 伪造或非法的入站 trace 上下文 100% 不被采信，且这类请求的成功率与不带该头时一致（不因拒绝而失败）。
- **SC-004**: 以包含凭据头、路径参数与查询串的请求做负例，技术事件与导出包中这些取值的出现次数为 0。
- **SC-005**: 事务回滚场景下待投递记录的投递次数为 0；事务提交场景下投递次数为 1 或记录处于可见的待投递状态，二者必居其一。
- **SC-006**: 新增自动测试全部通过且用例名可映射到 FR 编号；`pnpm typecheck`、相关 Go 测试与模块边界检查通过。
- **SC-007**: 交付文档逐行说明 `§4.3` 三行缺口的闭合证据，并如实列出仍未闭合的其余条目。

## UI Impact

预计无。本功能作用于服务端边界与存储合同，不改诊断面板布局与既有交互。若实施中发现必须改动页面，按 constitution 原则 II 与 VII 处理：不写 UI 单测，改动进手动验收清单由用户确认。

## Assumptions

- 诊断存储、模拟器、面板布局不改；`Pack`/`Unpack`/`Child` 的线上形状不改。
- 既有 `X-Diagnostic-Trace` 响应头保留，不因引入标准头而移除（installed 客户端可能已依赖）。
- `Scope.Accounts` 仍为空集合的现状不变，本功能不触碰账号域权限。
- 真实执行器保持禁用；本功能不涉及执行器路径。
- 队列与 WebSocket 两段已通过的行为与测试不动，本功能只补 HTTP 一段。
- 席位计费域的 `seat_capacity_outbox` 不被复用，也不被改动。
- `§4.3` 中「测试缺失」与「手动未执行」的条目不在本功能内，另行安排。
