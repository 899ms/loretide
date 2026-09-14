# Implementation Plan: DIAG-04 持久 outbox

**Branch**: `claude/spec-009-diag-durable-outbox` | **Date**: 2026-09-14 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `/specs/009-diag-durable-outbox/spec.md`

## Summary

把 `dispatch.go` 里自认「进程退出就丢」的 `MemoryOutbox` 补上一个落库的同门实现，让「事务提交了就一定会派发」这句话在重启面前也成立。

四件东西：

1. **一张表** `content_dispatch_outbox` 与三个迁移（建表 + 两个并发索引，各自单语句单文件）。
2. **一个落库实现** `PostgresOutbox`，与 `MemoryOutbox` 实现同一个 `Outbox` 接口：`Register` 在调用方的事务里插入，`Settle` 在提交后派发本事务那批。
3. **一个排水器** `Drainer`：周期扫描、行级租约认领、失败退避、到上限进死信、顺带按保留期清理。它才是重启恢复的来源。
4. **一个真实调用点**：diagnostics 模块自己的模拟运行提交路径，在同一个事务里写运行、写审计、登记记录，提交后结算——让「与业务写入同事务」有真实事务可验证（Q1 = A、FR-005a）。

**一条必须说清的边界**：`Settle` 在事务**提交之后**被调用，此时 `tx` 已不可用。落库实现里 `tx` 参数只当**分组键**，不再用于执行 SQL——这条写进合同，否则实现会试图在已提交的事务上跑语句（research D10）。

## Technical Context

**Language/Version**: Go 1.26.6（`server/go.mod`）；无前端改动

**Primary Dependencies**: **无新增**。`pgx/v5` 与 `pgxpool` 已是模块直接依赖（`store.go`）

**Storage**: PostgreSQL。新增 **1 张表**、**2 个索引**、**3 个迁移文件对**（`.up`/`.down` 成对，编号自 474 起）

**Testing**: Go 测试，同包。无库用例走 `dispatch_test.go`；需要真实事务的走 DB 背书用例，门槛沿用 `LORETIDE_DIAG_TEST_DATABASE_URL`（`store_integration_test.go:21`）。**不写 UI 单测**（无页面改动）

**Target Platform**: 服务端

**Project Type**: Existing monorepo (Go backend + Next.js web + Electron desktop + Expo mobile + shared packages). Do not re-derive this.

**Performance Goals**: 常态派发延迟 = 提交后就地排水，约等于一次派发调用；遗留记录的延迟上界 = 扫描间隔（可配置，默认见 data-model）

**Constraints**: 无外键、无级联；并发索引单语句单文件；表归 diagnostics；排水失败只计入既有 `LogBuffer.Dropped` / `Errors`；`Overview` 字段集合前后逐字段一致；不新增前端读取路径；不碰执行闸门；`MemoryOutbox` 与 `Store.Audit` / `Store.CommitRun` 的既有行为一字不动

**Scale/Scope**: 新增约 2 个 Go 文件 + 2 个测试文件 + 6 个迁移文件；改动 `dispatch.go`（注释与一条断言改名）、`store.go`（新增一个入口）、`cmd/server/router.go`（装配与间隔取值）

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| 原则 | 适用 | 判定 |
|---|---|---|
| I. CLAUDE.md 权威 | 是 | 通过：表名前缀、迁移写法、测试落点、提交前缀均按 `CLAUDE.md` 与模块既有文件 |
| II. 无 UI 单测 / 无自动 UI 验收 | 是 | 通过：本特性无页面改动，无 UI 单测，无手动 UI 项 |
| III. 模块边界 | 是 | 通过：新表与新代码全在 `server/internal/content/diagnostics/`；**不读写其他模块的表**（FR-005a、FR-021）；环境变量在 `cmd/server` 读、值传进模块，沿用 `router.go:443` 的既有模式 |
| IV. 状态分离 | 否 | N/A：无前端状态 |
| V. 数据库 | **是，且是重点** | 通过：**无外键、无级联**；两个索引均 `CONCURRENTLY` 且各自单语句单文件；`.up`/`.down` 成对；仓库既有迁移 lint（`TestMigrationNumericPrefixesAreUnique`、`TestMigrationFilesHaveMatchingDirections`）覆盖编号与配对 |
| VI. API 解析 | 否 | N/A：不新增端点，不改响应形状 |
| VII. UI 复用 | 否 | N/A：无页面 |
| VIII. 范围 | 是 | 通过：只做 DIAG-04 那一行的持久化；调用点只接 diagnostics 自己的业务写入；前端读取路径、其他模块接线均明确排除 |
| IX. 执行器禁用 | 是 | 通过：不触碰 `pkg/executionpolicy` 及其调用点（SC-009 用改动行数为 0 钉住） |
| X. 勾选 ≠ 验收 | 是 | 通过：mapping 的 DIAG-04 行只在实测跑过后按实际结果回写 |

**Stop Conditions**：无触发。→ 进入 Phase 0。

**Post-design re-check**：通过。两处需要记录的张力见 Complexity Tracking。

## Project Structure

### Documentation (this feature)

```text
specs/009-diag-durable-outbox/
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/
│   ├── outbox.md            # Register / Settle 的落库语义，含 tx 只作分组键
│   └── drainer.md           # 认领、租约、退避、死信、清理与配置
├── checklists/requirements.md
└── tasks.md
```

### Source Code (repository root)

```text
server/migrations/474_content_dispatch_outbox.up.sql                      # 新增：建表（无 FK、无级联）
server/migrations/474_content_dispatch_outbox.down.sql                    # 新增
server/migrations/475_content_dispatch_outbox_idempotency.up.sql          # 新增：部分唯一索引，单语句
server/migrations/475_content_dispatch_outbox_idempotency.down.sql        # 新增
server/migrations/476_content_dispatch_outbox_due.up.sql                  # 新增：认领用索引，单语句
server/migrations/476_content_dispatch_outbox_due.down.sql                # 新增
server/internal/content/diagnostics/dispatch_postgres.go                  # 新增：PostgresOutbox
server/internal/content/diagnostics/drain.go                              # 新增：Drainer
server/internal/content/diagnostics/dispatch_postgres_test.go             # 新增：DB 背书用例
server/internal/content/diagnostics/drain_test.go                         # 新增：租约/退避/死信/清理
server/internal/content/diagnostics/dispatch.go                           # 改：LIMIT 段改写、限制断言点明主语
server/internal/content/diagnostics/dispatch_test.go                      # 改：断言改名 + 新增反向断言的对照
server/internal/content/diagnostics/store.go                              # 改：新增同事务入口（Audit/CommitRun 不动）
server/cmd/server/router.go                                               # 改：装配 PostgresOutbox 与 Drainer，读间隔环境变量
docs/development/diagnostics-acceptance-mapping.md                        # 改：DIAG-04 行与 §4.3 回写
```

**Structure Decision**: 不新建目录、不新建包。落库实现与排水器与 `dispatch.go` 并列放在 diagnostics 包内——它们是同一个接口的第二个实现和它的驱动，拆包只会让 `Outbox` 的两个实现分处两地。迁移与既有 `468`～`473` 并列。

## Complexity Tracking

| 张力 | 为什么接受 | 被拒绝的更简方案 |
|---|---|---|
| **「恰好一次」在派发成功、标记前崩溃的窗口里做不到** | 没有分布式事务就没有真正的 exactly-once。规格因此把保证拆成三层（登记去重 / 已派发不再认领 / 派发目标自身幂等），并要求合同写明第三层的责任在接收方。声称 exactly-once 而不写这个窗口，才是真正的风险 | 把派发放进事务：一个慢接收方会拖住数据库事务，失败还会回滚已提交的业务。否决 |
| **新增一个真实调用点（Q1 = A）** | 不接调用点，「与业务写入同事务」就只能靠测试夹具自造事务，是纸面承诺。接在模块自己的模拟运行提交上，既有真实事务又不越出模块边界 | 接在 `Store.Audit` 上：那是关键审计路径，`dispatch.go` 头注释明写关键审计不走 outbox。否决 |
| **保留 `MemoryOutbox` 造成两个实现两套限制** | 硬约束要求保留，且无库场景确实需要它。代价是两条互为反面的断言必须同时存在（FR-014）——这不是冗余，删掉任何一条都会让对应实现的限制失去记录 | 删掉 `MemoryOutbox`：与硬约束冲突，且 `dispatch_test.go` 的无库用例全部失去被测对象。否决 |
