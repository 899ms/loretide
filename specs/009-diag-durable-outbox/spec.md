# Feature Specification: DIAG-04 持久 outbox

**Feature Branch**: `claude/spec-009-diag-durable-outbox`

**Created**: 2026-09-14

**Status**: Draft

**Input**: User description: "DIAG-04 持久 outbox：把 `server/internal/content/diagnostics/dispatch.go` 的 `Outbox` 接口换成落库实现——记录在业务事务内与业务写入同事务写入 outbox 表，提交后由进程内排水器派发，进程重启后未派发项不丢失且不重复派发（幂等键）；让 `TestOutboxDoesNotSurviveTheProcess` 这条限制断言转为反向断言；`MemoryOutbox` 保留给无数据库场景与测试。"

**Traces to**: `docs/development/diagnostics-acceptance-mapping.md` §2 DIAG-04「交付 outbox 接口」行的**带限制**闭合、§4.3 第 3 条的 `~~DIAG-04~~` 行、以及 2026-09-14 更新（一）里「持久落库版本列为后续任务」。

## Current State（以代码为准）

核实自 `app-main` @ `3fd8268`。

### 1. 今天的 `dispatch.go`

| 元素 | 现状 |
|---|---|
| `DispatchItem` | `{ID, Kind, Payload []byte, IdempotencyKey}`。注释明写这四部分「正是持久实现需要的：寻址的 id、路由的 kind、载荷、以及让重试不生效两次的幂等键」 |
| `Outbox` 接口 | `Register(ctx, tx any, item) error`、`Settle(ctx, tx any, committed bool) int`。`tx` **刻意是 `any` 而非 `pgx.Tx`**，注释说明理由是让调用方传自己存储层的句柄、并让契约可以在无数据库时被验证 |
| `MemoryOutbox` | 按 `tx` 分桶暂存；每事务容量上限，超出即丢弃并计数；`IdempotencyKey` 去重（进程内 `seen` map）；`Deliver` 为 `nil` 时视为「已派发」的空操作 |
| 文件头注释 | 一整段 **`LIMIT, DELIBERATE`**：明写进程退出会丢记录、这是被规格接受的限制（005 的 FR-010a）、`TestOutboxDoesNotSurviveTheProcess` 断言它，并明说「持久版本是另一个任务，下面的接口形状就是为了让它替换时调用方一行不改」 |

### 2. 关键事实：**今天没有任何生产调用方**

全仓检索 `server/` 下对 diagnostics 包 `Outbox` / `MemoryOutbox` 的引用，**非测试文件为 0**；唯一引用点是 `internal/content/diagnostics/dispatch_test.go`。

（检索命中的 `SeatCapacityOutbox` 属于 `internal/seatcapacity` 里另一套早已存在的持久 outbox，与本模块无关，见 §6。）

**这改变了本特性的形状**：需求说「记录在业务事务内与业务写入同事务写入 outbox 表」，但**今天没有任何业务写入在调用它**。落库实现本身可以交付并被测试覆盖，「与业务写入同事务」这句话却没有真实事务可挂靠，除非本特性同时接一个调用点。这是 **Q1**，不是实施细节。

### 3. 事务的形状

`Store` 的写入都经过一个私有助手：

```text
Store.withWorkspaceWrite(ctx, workspaceID, func(tx pgx.Tx) error)
  → pool.Begin → guard.LockForContentDiagnosticWrite(ctx, tx, workspaceID) → write(tx) → tx.Commit
```

`Store.Audit` 走它（`store.go:110-115`），`Store.Technical` 也走它但失败只计数。**这个助手是私有的，且回调只在提交前运行**——「提交后派发」需要一个提交之后的时点，当前形状里没有。

`Outbox` 的 `tx any` 与这里的 `pgx.Tx` 之间需要一次类型断言；断言失败怎么办是 **Q3**。

### 4. 计数与暴露：只有两个计数器

- `LogBuffer` 的全部计数器就是 `Dropped` 与 `Errors`（`log.go:103`）。
- `Overview` 通过 `Metrics.Dropped`（JSON `dropped`）与 `Metrics.SinkErrors`（JSON `sink_errors`）暴露它们（`service.go:6`、`service.go:17`）。
- `MemoryOutbox.count(drop)` 的既有映射：**溢出 → `Dropped`；派发失败 → `Errors`**。

硬约束「不新增指标名」意味着排水器的失败与丢弃**只能落进这两个**，并沿用同一映射。

### 5. 迁移与表的既有约定

- diagnostics 的三张表由迁移 `468_content_diagnostics` 建立：`content_diagnostic_run`、`content_operation_audit`、`content_technical_log`。**均无外键、无级联**（逐文件核实）。
- 并发索引单独成文件、单语句：`472_content_audit_scope.up.sql`、`473_content_log_scope.up.sql`，形如 `CREATE INDEX CONCURRENTLY IF NOT EXISTS …`。
- 当前最大迁移编号 **473**，`server/migrations/` 共 1004 个文件。
- 仓库已有迁移 lint：`TestMigrationNumericPrefixesAreUnique`（编号不得重复）、`TestMigrationFilesHaveMatchingDirections`（`.up` / `.down` 必须成对）。

### 6. 同仓已有的持久 outbox 先例（只借形状，不复用表）

`seat_capacity_outbox`（迁移 `415`）已经是一套可用的持久 outbox：

- 列：`attempt_count`、`next_attempt_at`、`lease_token`、`delivered_at`、`last_error`、`dead_lettered_at`、`created_at`/`updated_at`；
- 领取：`FOR UPDATE SKIP LOCKED` + 租约续期（`pkg/db/generated/seat_capacity.sql.go:14-31`）。

它归 `seatcapacity` 模块所有。按表归属规则本特性 **MUST NOT** 复用或读写它，只把它当作已在本仓通过评审的形状参考。

### 7. 那条要被反转的断言

`dispatch_test.go:149` 的 `TestOutboxDoesNotSurviveTheProcess`：新建一个 outbox 代表「重启后的进程」，断言它 `Settle` 出 0 条、`Deliver` 一条都没收到。

注意它断言的主语是 **`MemoryOutbox`**，而 `MemoryOutbox` **按硬约束要保留**。因此「转为反向断言」不能就地改这一条的含义，否则 `MemoryOutbox` 的这条真实限制就没有断言了。处理方式见 FR-014。

### 不在本功能范围

不新增任何前端数据读取路径；不碰执行闸门（`pkg/executionpolicy`）；不改 `Store.Audit` 既有的「审计写失败即整体回滚」语义；不新增指标名；不复用或读写其他模块的表；不改 `MemoryOutbox` 的对外行为。

## Clarifications

### Session 2026-09-14（主任务已裁决）

- **Q1**（调用点）：本特性是否同时接一个生产调用点，好让「与业务写入同事务」有真实事务可验证？→ **A**：新增一个 `Store` 上的入口，在**同一个事务**里完成一次业务写入与一次 `Register`，提交后结算；`Store.Audit` 的行为一字不动。**补充约束**：该入口只能承载 **diagnostics 模块自己的业务写入**（例如一次模拟运行的提交），MUST NOT 触碰其他模块的表。见 FR-005、FR-005a。
- **Q2**（排水器的归属与触发）：谁运行排水器、以什么节奏？→ **A**：两段式——提交后**就地**排水本事务的记录（低延迟），外加进程内**周期扫描**认领遗留与租约过期的记录（这才是重启恢复的来源）。不新增独立后台服务。**补充约束**：周期扫描的认领用**行级租约**（如 `claimed_until`），租约过期即可被重新认领；派发本身必须幂等；**扫描间隔可配置、有默认值，测试中可缩短**。见 FR-008、FR-009、FR-009a、FR-009b。
- **Q3**（`tx any` 拿不到真事务时）：落库实现在类型断言失败时怎么办？→ **A**：**失败关闭**——`Register` 返回错误，调用方的事务随之回滚。「以为写进了 outbox 其实没写」比直接失败危险得多。见 FR-004。

## User Scenarios & Testing *(mandatory)*

### User Story 1 - 进程重启后，已提交事务的派发不会凭空消失 (Priority: P1)

一次业务写入提交成功，但进程在派发之前退出。进程重启后，那条记录仍然存在并被派发；它**只被派发一次**，即使重启发生在派发的任何一步。

**Why this priority**: 这是 DIAG-04 唯一未闭合的部分，也是当前实现在代码注释里自认的限制。没有它，「提交了就一定会派发」这句话在任何一次部署、崩溃或重启面前都不成立。

**Independent Test**: 在一个真实数据库上注册并提交一条记录，不运行排水器就丢弃该进程的全部内存状态，用一个全新的实现实例排水，断言这条记录被派发且只派发一次。

**Acceptance Scenarios**:

1. **Given** 一条记录随业务事务提交、尚未派发，**When** 丢弃全部进程内状态后由新实例排水，**Then** 该记录被派发，派发次数为 1。
2. **Given** 一条记录已经派发成功，**When** 新实例再次排水，**Then** 它不被再次派发。
3. **Given** 业务事务回滚，**When** 任何实例排水，**Then** 该记录不存在、永不派发。
4. **Given** 两个排水器同时运行，**When** 两者都去认领同一批记录，**Then** 每条记录只被其中一个派发，总派发次数仍为 1。

---

### User Story 2 - 派发失败不拖垮业务，但也不被静默吞掉 (Priority: P1)

派发目标不可用时，业务操作已经提交、不能被回退。失败必须被重试，重试不能无限占用资源，而且必须出现在运维已经在看的地方。

**Why this priority**: 与 US1 同为 P1 且互补。一个「不丢记录」但会把失败悄悄堆积到看不见的地方的实现，只是把丢失换成了积压。

**Independent Test**: 让派发目标持续失败，断言记录保留、重试次数增长、失败计入既有计数器且能在 `Overview` 读到；超过上限后记录进入终止状态而不是无限重试。

**Acceptance Scenarios**:

1. **Given** 派发目标返回错误，**When** 排水器处理该记录，**Then** 业务事务的结果不受影响，记录保留待重试，失败计入既有的 `sink_errors`。
2. **Given** 同一条记录连续失败到上限，**When** 再次排水，**Then** 它进入终止状态并停止重试，且该事实可被观察到。
3. **Given** 排水过程中产生任何失败或丢弃，**When** 读取 `Overview`，**Then** 数字出现在既有的 `dropped` / `sink_errors` 两个字段里，**没有新增任何指标名**。

---

### User Story 3 - 没有数据库时，行为退化得可预期 (Priority: P2)

单元测试、无数据库的部署与本地无库运行仍然可以使用内存实现；两种实现对调用方是同一个接口，且各自的限制都有断言写着。

**Why this priority**: 硬约束要求保留 `MemoryOutbox`。如果保留后没人说清「什么时候用哪个、各自保证什么」，调用方会在不知情的情况下拿到一个不持久的实现。

**Independent Test**: 用同一段调用方代码分别跑两种实现；断言接口一致，并断言两条互为反面的限制各自成立。

**Acceptance Scenarios**:

1. **Given** 同一段调用方代码，**When** 分别对内存实现与落库实现运行，**Then** 两者都满足接口契约，调用方无需改动。
2. **Given** 内存实现，**When** 模拟进程重启，**Then** 未派发记录丢失——这条限制**仍有断言**。
3. **Given** 落库实现，**When** 模拟进程重启，**Then** 未派发记录**不丢失**——这是与上一条互为反面的新断言。

---

### Edge Cases

- 业务事务提交成功，但提交后的就地派发还没开始进程就退了：记录必须由周期扫描接手。
- 排水器认领了记录后自己死掉：租约必须能过期，记录必须能被另一个排水器重新认领，且不产生第二次实际派发。
- 同一个幂等键在不同事务里出现两次：第二次必须不产生第二次派发。
- 幂等键为空：现有内存实现把空键视为「不去重」。落库实现必须给出同样可判定的答案，不能让空键变成一条绕过去重的暗路。
- 载荷超大：表里存一条无上限的载荷会把诊断库变成一个不受控的写入池。
- 派发目标一直不可用：记录必须有一个终点，不能永远滚动重试。
- 已派发记录的清理：模块已有 7 天保留期与 `PruneTechnical`；outbox 表不能成为唯一一张只增不减的表。
- 无数据库时构造落库实现：必须明确失败，不能悄悄退化成内存行为。

## Requirements *(mandatory)*

### Functional Requirements

**接口与实现选择**

- **FR-001**: 系统 MUST 保留现有 `Outbox` 接口的方法集与语义，使既有调用方代码无需改动即可换用落库实现。
- **FR-002**: 系统 MUST 保留 `MemoryOutbox` 作为无数据库场景与测试的实现，其对外行为 MUST NOT 改变。
- **FR-003**: 系统 MUST 让「用哪个实现」是调用方的显式选择，MUST NOT 让落库实现在数据库不可用时自动退化为内存行为。
- **FR-004**: 落库实现在收到的事务句柄不是一个可用的数据库事务时 MUST 失败关闭：返回错误、不暂存、不静默接受。（Q3 = A）

**同事务写入**

- **FR-005**: 系统 MUST 提供一条可被端到端验证的路径，在**同一个事务**内完成一次业务写入与一次记录登记，并在该事务提交后结算。（Q1 = A：新增入口承载，`Store.Audit` 行为不变）
- **FR-005a**: FR-005 的新入口 MUST 只承载 **diagnostics 模块自己的业务写入**（例如一次模拟运行的提交），MUST NOT 读写任何其他模块的表。若某个真实调用场景需要跨模块写入，MUST 停下来报告，不得自行扩范围。
- **FR-006**: 业务事务回滚时，登记的记录 MUST 不存在于表中，MUST NOT 在任何后续时点被派发。
- **FR-007**: 记录的写入 MUST 与业务写入共享同一个事务边界，MUST NOT 通过另开连接或另起事务写入——否则「提交即登记」在崩溃窗口内不成立。

**排水与重启恢复**

- **FR-008**: 事务提交后，系统 MUST 就地尝试派发该事务登记的记录。
- **FR-009**: 系统 MUST 另有一条周期性的扫描路径，认领并派发**任何**尚未派发的记录，不依赖登记它的那个进程仍然存活。（Q2 = A）
- **FR-009a**: 周期扫描的认领 MUST 基于**行级租约**（形如 `claimed_until`）：认领即写入租约期限，租约**过期后该记录 MUST 可被重新认领**；在此基础上派发本身 MUST 幂等，使「重新认领」永远不会变成「第二次实际派发」。
- **FR-009b**: 扫描间隔 MUST 可配置且 MUST 有默认值，MUST NOT 硬编码；测试 MUST 能把它缩短到不必真实等待默认间隔。
- **FR-010**: 进程重启后，已提交但未派发的记录 MUST 仍被派发。
- **FR-011**: 任何记录 MUST 最多被实际派发一次，无论重启发生在哪一步、也无论同时有几个排水器在跑——**但有一个实现挡不住的窗口必须写明**：派发已发出、成功标记尚未落库时进程崩溃，下一轮会重发。该窗口没有分布式事务就关不掉，MUST 由 FR-009a 要求的「派发本身幂等」在接收方吸收，MUST NOT 被表述为本实现单方面保证的 exactly-once。除该窗口外的一切情形（并发排水器、租约过期重领、进程在派发前退出）MUST 由本实现自己挡住。
- **FR-012**: 多个排水器并发时 MUST NOT 出现两个排水器同时处理同一条记录；认领 MUST 是排他的，且认领方失联后记录 MUST 能被重新认领。
- **FR-013**: 幂等键为空的记录 MUST 有一个明确且可判定的处理规则，该规则 MUST 在规格与断言中写明，MUST NOT 成为绕过去重的隐式通道。

**断言的反转**

- **FR-014**: 系统 MUST 新增一条与 `TestOutboxDoesNotSurviveTheProcess` 互为反面的断言，证明落库实现在进程重启后不丢记录且不重复派发。原断言 MUST 保留并改为明确以 `MemoryOutbox` 为主语——它的限制依然真实，删掉它等于删掉对保留下来的那个实现的限制记录。
- **FR-015**: `dispatch.go` 文件头的 `LIMIT, DELIBERATE` 段 MUST 更新：那条限制不再是整个特性的限制，而是内存实现的限制。

**失败、上限与可见性**

- **FR-016**: 派发失败 MUST NOT 影响已提交的业务操作的结果。
- **FR-017**: 派发失败与记录丢弃 MUST 只计入既有的 `LogBuffer.Errors` 与 `LogBuffer.Dropped`，沿用既有映射（失败→`Errors`，丢弃→`Dropped`），MUST NOT 新增任何指标名或新的暴露字段。
- **FR-018**: 记录 MUST 有重试上限与失败后的终止状态，MUST NOT 无限重试；进入终止状态 MUST 可被观察到。该可观察性 MUST 走既有的技术日志事件，MUST NOT 新增计数器或指标名——复用 `Errors` 会把「又失败一次」与「不再重试了」混成同一个数字，那正是要区分的两件事。
- **FR-019**: 记录载荷 MUST 有大小上限；超限 MUST 按既有的丢弃语义处理并计数，MUST NOT 无界写入。
- **FR-020**: 已派发记录 MUST 有清理路径，与模块既有的保留期口径一致，MUST NOT 让该表成为只增不减的表。

**边界与归属**

- **FR-021**: 新增的表 MUST 归 diagnostics 模块所有，MUST NOT 复用或读写其他模块的表（含 `seat_capacity_outbox`）。
- **FR-022**: 新增迁移 MUST NOT 包含外键、级联删除或级联更新；索引 MUST 使用 `CREATE INDEX CONCURRENTLY`，且每个并发索引 MUST 单独成一个单语句迁移文件；`.up` 与 `.down` MUST 成对。
- **FR-023**: 本特性 MUST NOT 新增任何面向前端的数据读取路径、查询参数或端点。
- **FR-024**: 本特性 MUST NOT 改动执行闸门（`pkg/executionpolicy`）及其调用点。
- **FR-025**: 本特性 MUST NOT 新增 UI 单测；本特性无页面改动，不产生手动 UI 验收项。

### Key Entities

- **待派发记录（Outbox record）**：一条其存在由业务事务决定、只能在该事务提交后发送的记录。含寻址标识、路由类别、载荷、幂等键，以及派发状态所需的尝试次数、下次尝试时间、认领租约、终止标记与时间戳。
- **认领（Claim）**：一个排水器对一批记录的排他占用，带过期时间，使认领方失联后记录可被重新认领而不产生重复派发。
- **幂等键（Idempotency key）**：让同一条逻辑记录无论被重试几次都只生效一次的键。
- **排水器（Drainer）**：把已提交未派发的记录取出并派发的进程内组件，有就地与周期两种触发。

## Success Criteria *(mandatory)*

- **SC-001**: 一条已提交未派发的记录，在丢弃全部进程内状态后由新实例排水，被派发且**恰好一次**（本用例不构造 FR-011 写明的「已派发未标记」崩溃窗口）。
- **SC-002**: 业务事务回滚后，该事务登记的记录数为 0，任何后续排水的派发数为 0。
- **SC-003**: 两个排水器并发处理同一批记录时，任一记录的实际派发次数为 1。
- **SC-004**: 派发目标持续失败时，业务操作的结果不变，记录保留，`sink_errors` 增长；达到上限后停止重试。
- **SC-005**: 新增与派发相关的指标名数量为 **0**——`Overview` 的字段集合在本特性前后逐字段一致。
- **SC-006**: `MemoryOutbox` 的对外行为在本特性前后一致：其既有用例不做语义改动即全部通过。
- **SC-007**: 「重启后丢失」与「重启后不丢失」两条断言同时存在且同时通过，主语分别是内存实现与落库实现。
- **SC-008**: 新增迁移中外键、`REFERENCES`、`CASCADE` 的出现次数为 0；非并发索引创建次数为 0；每个并发索引所在迁移文件的语句数为 1；`.up`/`.down` 成对，仓库既有迁移 lint 通过。
- **SC-009**: 本特性新增的面向前端读取路径数为 0；`pkg/executionpolicy` 的改动行数为 0。
- **SC-010**: 一条租约已过期的记录能被另一个排水器重新认领并完成派发，其实际派发总次数仍为 1。
- **SC-011**: FR-005 新增入口触及的表全部属于 diagnostics 模块；它读写其他模块表的次数为 0。
- **SC-012**: 扫描间隔可由测试设定为一个远小于默认值的值，相关用例的运行时间不随默认间隔增长。

## UI Impact

**无。** 本特性交付的是服务端实现、一张表与相应迁移、以及测试；不触及任何页面，也不改变 `Overview` 已暴露的字段集合。

## Assumptions

- 诊断模块的写入继续使用既有的工作区写入保护协议（提交前先取工作区写锁），本特性不改这条链路的顺序。
- 「业务写入」在本特性语境下指诊断模块自己的写入；本特性不为其他模块的写入接线。
- 落库实现所需的数据库能力不超出仓库既有用法（同仓已有一套持久 outbox 在用同类机制），不引入新的数据库扩展或新的外部依赖。
- 本特性不改变审计写入「失败即整体回滚」的语义；outbox 是那条链路之外的第二类记录，不是它的替代。
- Q1/Q2/Q3 已由主任务裁决为 A 并连同两条补充约束写入 FR（FR-005a、FR-009a、FR-009b）。
