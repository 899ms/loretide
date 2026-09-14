# Research: 009 DIAG-04 持久 outbox

Phase 0。Technical Context 无 NEEDS CLARIFICATION（三处已由 2026-09-14 主任务裁决）。以下为落地前的技术决策，全部核实自 `app-main` @ `3fd8268`。

## D1. 表的形状：抄同仓已通过评审的 `seat_capacity_outbox`，不抄它的表

- **Decision**：新表 `content_dispatch_outbox`，列取自 `seat_capacity_outbox`（迁移 `415`）的同一组概念——`attempt_count`、`next_attempt_at`、`claimed_until`、`delivered_at`、`last_error`、`dead_lettered_at`、`created_at`/`updated_at`——但**独立建表**，前缀 `content_` 与 diagnostics 已有三张表一致。
- **Rationale**：这套列与 `FOR UPDATE SKIP LOCKED` 的配合已经在本仓运行且被评审过（`pkg/db/generated/seat_capacity.sql.go:14-31`），没有理由重新发明。同时 FR-021 禁止复用别的模块的表——`seat_capacity_outbox` 归 `seatcapacity` 模块，只借形状。
- **Alternatives considered**：复用 `seat_capacity_outbox` 并加一个 `source` 列——省一张表，但把两个模块的写入混进同一张表，违反表归属规则，且它的 `action` 列带 `CHECK` 白名单，加诊断用的动作要改别人的约束。否决。

## D2. 认领：`FOR UPDATE SKIP LOCKED` + 行级租约

- **Decision**：认领一批到期记录用 `SELECT … WHERE dead_lettered_at IS NULL AND delivered_at IS NULL AND next_attempt_at <= now() AND (claimed_until IS NULL OR claimed_until <= now()) ORDER BY next_attempt_at, created_at FOR UPDATE SKIP LOCKED LIMIT n`，然后把 `claimed_until` 推到「现在 + 租约时长」。
- **Rationale**：FR-009a 要求行级租约且租约过期可被重新认领。`SKIP LOCKED` 解决**同一瞬间**的并发（FR-012 前半），`claimed_until` 解决**认领方失联**（FR-012 后半）——两者缺一不可：只有行锁的话，持锁进程被 kill 后锁随连接释放、记录立刻可领，但没有「这条正在被谁处理」的持久证据；只有租约的话，两个排水器会在同一毫秒读到同一行。
- **为什么租约不足以保证「只派发一次」**：租约过期后重新认领是**合法**的，此时前一个认领方可能已经派发成功却没来得及标记。所以「恰好一次」不能只靠租约，必须叠 D3 的幂等。

## D3. 「恰好一次」的真实含义与落点

- **Decision**：分成两层——**登记层去重**由 `idempotency_key` 上的**部分唯一索引**加 `ON CONFLICT DO NOTHING` 保证；**派发层不重复**由「派发成功后写 `delivered_at`」加「认领条件排除 `delivered_at IS NOT NULL`」保证；两者都无法覆盖的窗口（派发成功、标记前崩溃）由**派发目标自身幂等**兜底，合同写明这一点。
- **Rationale**：这是诚实的说法。任何「派发 + 标记」的组合在没有分布式事务时都留一个窗口，声称纯粹的 exactly-once 是假的。SC-001/SC-003 测的是「在本实现可控的范围内恰好一次」，而 FR-009a 后半句「派发本身 MUST 幂等」正是为这个窗口写的。
- **Alternatives considered**：两阶段提交 / 把派发放进事务——派发是对外动作，放进事务会让一个慢的接收方拖住数据库事务，且失败就回滚已提交的业务。否决。

## D4. 幂等键为空怎么办（FR-013）

- **Decision**：**部分唯一索引** `… (idempotency_key) WHERE idempotency_key <> ''`。空键不参与去重，与 `MemoryOutbox` 现有行为（`if d.IdempotencyKey != "" && o.seen[...]`）逐字一致。
- **Rationale**：两种实现对同一输入必须给同样的答案，否则「换实现调用方不用改」是假的。把空键规则交给一个**索引**而不是一段代码，还顺带让规则在 schema 里可读。
- **Alternatives considered**：空键一律拒绝——比现有实现更严，会让已有调用方（将来的）在切换实现时行为改变，否决；空键当作「每次都是新记录」并写全表唯一索引——空字符串只能存在一条，是错的。否决。

## D5. 插入用 `ON CONFLICT DO NOTHING`，与模块既有写法一致

- **Decision**：`INSERT INTO content_dispatch_outbox(...) VALUES(...) ON CONFLICT DO NOTHING`。
- **Rationale**：`Store.Technical` 对 `content_technical_log` 就是这么写的（`store.go:122`），且 `470`/`471`/`469` 三个迁移都是先建唯一索引再靠 `ON CONFLICT` 去重。重复登记因此是一个**静默的 no-op**，而不是一个会把调用方事务打挂的错误——与 `MemoryOutbox` 的「重复键跳过」语义相同。

## D6. `tx any` 的断言（FR-004）

- **Decision**：`tx.(pgx.Tx)`，失败返回错误（沿用模块既有的 `ErrUnavailable` 一类，不新增错误类型），不暂存、不静默接受。
- **Rationale**：Q3 = A。这里唯一值得写下来的是**为什么不把接口签名收窄成 `pgx.Tx`**：那会让 `MemoryOutbox` 失去无数据库可测性，而硬约束要求保留它。`any` 的代价就是这一次断言，代价明确、位置单一。

## D7. 排水器的两段触发与生命周期（FR-008、FR-009、FR-009b）

- **Decision**：
  1. **就地**：`Settle(ctx, tx, true)` 在事务提交后被调用，直接派发**本事务**登记的那些记录（按 id 取回，避免再扫一遍表）；
  2. **周期**：一个 `Drainer`，`Start(ctx)` 起一个 goroutine 按间隔扫描，`ctx` 取消即停。间隔由构造参数传入，**默认值定义在 diagnostics 包内的常量**，真实取值由 `cmd/server/router.go` 读环境变量后传进来。
- **Rationale**：`cmd/server/router.go:443` 已经是这个模式——`diagnostics.NewService(..., os.Getenv("LORETIDE_BUILD"), os.Getenv("APP_ENV")=="development" && ...)`，环境变量在 `cmd/server` 读、值传进模块。沿用它，模块本身不读环境变量，测试直接传一个很小的间隔（FR-009b、SC-012）。
- **为什么不做独立后台服务**：Q2 = A 明确「不新增独立后台服务」。`Drainer` 的持有者是已经存在的服务装配点，多一个 goroutine，不多一个部署单元。

## D8. 上限与终点（FR-018、FR-019）

- **Decision**：`attempt_count` 到上限即写 `dead_lettered_at` 并停止认领；载荷超过上限在 `Register` 阶段按**丢弃**语义处理并计 `Dropped`，不写表。上限均为构造参数，带包内默认值。
- **Rationale**：`MemoryOutbox` 现有的「超容量即丢弃并计 `Dropped`」就是同一个决定的内存版；载荷上限放在 `Register` 而不是排水时，是因为一条写不出去的记录不该先占住表空间。
- **Alternatives considered**：无限重试 + 告警——本仓没有告警通道，等于无限重试。否决。

## D9. 计数映射（FR-017）

- **Decision**：**派发失败 → `LogBuffer.Errors`；丢弃（载荷超限、认领后发现已死信等）→ `LogBuffer.Dropped`**。不新增字段、不新增指标名。
- **Rationale**：`MemoryOutbox.count(drop)` 已经是这个映射，`Overview` 把两者暴露为 `sink_errors` 与 `dropped`（`service.go:6`、`service.go:17`）。沿用它，`Overview` 的字段集合在本特性前后逐字段一致（SC-005）。

## D10. FR-005 的新入口挂在哪次业务写入上

- **Decision**：挂在 **`Store.CommitRun`** 这条路径的同形入口上——它是 diagnostics 模块**自己**的业务写入（写 `content_diagnostic_run` 并在同一事务内 `appendAudit`），完全落在本模块的表内，满足 FR-005a。**`Store.Audit` 与 `Store.CommitRun` 现有签名与行为一字不动**，新增的是一个并列入口。
- **Rationale**：Q1 = A 加补充约束。`CommitRun` 已经在一个事务里做两件事（写运行 + 写审计），再加一次 `Register` 是同一事务边界内的第三件事，不需要新的事务形状；而 `Audit` 是「关键审计」路径，`dispatch.go` 头注释明写关键审计要继续走它、不走 outbox。
- **需要的最小改动**：`withWorkspaceWrite` 今天是私有且只在提交前运行；新入口需要一个「提交成功后」的时点来调 `Settle`。做法是在新入口内部先 `withWorkspaceWrite`（其中 `Register`），返回 nil 后再 `Settle(ctx, tx, true)`——**注意 `tx` 在提交后已不可用**，所以 `Settle` 的 `tx` 参数在落库实现里只作为**分组键**，不再用于执行 SQL。这一点必须写进合同，否则实现会试图在已提交的事务上执行语句。

## D11. 清理（FR-020）

- **Decision**：已派发与已死信的记录按模块既有 `Retention`（7 天）清理，复用 `PruneTechnical` 同形的写法，不引入新的定时机制——挂在周期扫描的同一次唤醒里。
- **Rationale**：模块已有保留期口径，再造一个清理节奏会出现两个互相不知道的定时器。

## D12. 测试落点

- **Decision**：
  - 纯逻辑（键规则、上限判定、计数映射、`tx` 断言失败）→ `dispatch_test.go` 同包无库用例；
  - 需要真实事务与重启语义（FR-006、FR-010、FR-011、FR-012、SC-001~SC-003、SC-010）→ **DB 背书用例**，沿用 `store_integration_test.go` 的 `LORETIDE_DIAG_TEST_DATABASE_URL` 门槛；
  - `MemoryOutbox` 既有 7 个用例**不改语义**，只把 `TestOutboxDoesNotSurviveTheProcess` 改名点明主语（FR-014）。
- **Rationale**：CI 已经在 `loretide-content.yml` 里用一个最小权限库跑 `go test -race ./internal/content/diagnostics`，DB 背书用例进得去，不需要新的 CI 步骤。
- **不写 UI 单测**：本特性无页面改动。
