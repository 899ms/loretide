---

description: "Task list template for feature implementation"
---

# Tasks: [FEATURE NAME]

**Input**: Design documents from `/specs/[###-feature-name]/`

**Prerequisites**: plan.md (required), spec.md (required for user stories), research.md, data-model.md, contracts/

**Tests**: The examples below include test tasks. Tests are OPTIONAL - only include them if explicitly requested in the feature specification.

> ## Loretide testing policy (NON-NEGOTIABLE - overrides the generic examples below)
>
> - **Never write or run UI unit tests.** Applies to local runs and CI alike. Do not work around it by renaming files, changing extensions, or reclassifying a UI test as something else.
> - **Never use computer use or automated browser clicking for acceptance.** When a task touches UI, list the affected screens/controls, the exact steps, and the expected result as a manual Todo for the user to verify. Do not mark it passed until the user confirms.
> - **Do keep the non-UI checks** the changed scope needs: contract tests, Go tests, permission and module-boundary checks, `pnpm typecheck`, and the build.
> - **Do not run whole-suite commands** that may pull in UI tests. Run the narrowest useful check.
> - A check you did not run is recorded as "not run per policy, awaiting user verification" - never as passed.
> - Test placement follows `CLAUDE.md` -> Testing: shared logic in `packages/core/*.test.ts`, shared UI in `packages/views/*.test.tsx`, platform wiring in `apps/web/`, E2E in `e2e/*.spec.ts`, backend in `server/` Go tests. A `.test.ts` needing no DOM starts with `// @vitest-environment node`.
> - Give each behavior ONE canonical layer. Do not re-run a helper's matrix through a DOM mount.

**Organization**: Tasks are grouped by user story to enable independent implementation and testing of each story.

## Format: `[ID] [P?] [Story] Description`


> ## Loretide testing policy（不可协商）
>
> - **禁止编写或运行 UI 单测**。本特性**无页面改动**，不产生任何 UI 测试，也不产生手动 UI Todo。
> - **禁止用 computer use / 自动点击做验收**。
> - **保留该改动需要的非 UI 检查**：包内 Go 测试（含 `-race`）、迁移 lint、`go build ./cmd/server`、既有边界与合同检查。
> - **不跑全量套件**（`pnpm test`、`make test`），只跑最窄的有用检查。
> - 未运行的检查记「按策略未执行，等待用户验证」，**绝不记为通过**。**DB 背书用例在没有 `LORETIDE_DIAG_TEST_DATABASE_URL` 时会跳过，跳过记为未执行，不记为通过。**
> - 本特性**不碰执行闸门**（`pkg/executionpolicy`）。实施中若发现必须碰，停下来问。

## Phase 1: Setup（基线）

- [x] T001 记录基线：`ls server/migrations | wc -l`（SC-008 的比对基数，当前 1004）；`ls server/migrations | tail -3`（确认最大编号为 473）；`git rev-parse origin/app-main`；`go test -race ./internal/content/diagnostics -count=1 -v` 的用例名与条数 — *基线，无 FR 映射*
- [x] T002 逐字节留存 `server/internal/content/diagnostics/service.go` 中 `Metrics` 结构体那一行，供 **T020** 比对「`Overview` 字段集合前后一致」 — **FR-017、SC-005**
- [x] T003 [P] 确认 `LORETIDE_DIAG_TEST_DATABASE_URL` 已设且迁移可跑；若不可用，**停下来报告**——本特性的核心验收（FR-006/FR-010/FR-011/FR-012）全部需要真实事务，无库时它们只会被跳过 — *前置，无 FR 映射*

## Phase 2: Foundational（阻塞全部故事）

- [x] T004 [P] `server/migrations/474_content_dispatch_outbox.up.sql` 与 `.down.sql`（**新增**）：按 data-model 建表 `content_dispatch_outbox`，13 列。**无 `FOREIGN KEY` / `REFERENCES` / `CASCADE`**；`.down` 为 `DROP TABLE IF EXISTS` — **FR-021、FR-022、SC-008**
- [x] T005 [P] `server/migrations/475_content_dispatch_outbox_idempotency.up.sql` 与 `.down.sql`（**新增**）：`CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS … (idempotency_key) WHERE idempotency_key <> '';`。**文件内只有这一条语句**——PostgreSQL 拒绝在事务或多语句字符串里建并发索引 — **FR-013、FR-022、SC-008**
- [x] T006 [P] `server/migrations/476_content_dispatch_outbox_due.up.sql` 与 `.down.sql`（**新增**）：`CREATE INDEX CONCURRENTLY IF NOT EXISTS … (next_attempt_at, created_at) WHERE delivered_at IS NULL AND dead_lettered_at IS NULL;`。同样单语句单文件 — **FR-022、SC-008**
- [x] T007 跑 `go run ./cmd/migrate up` 与迁移 lint（`TestMigrationNumericPrefixesAreUnique`、`TestMigrationFilesHaveMatchingDirections`），确认迁到 476 且 lint 通过 — **FR-022、SC-008**

## Phase 3: User Story 1 - 重启后不丢、且只派发一次 (Priority: P1) 🎯 MVP

**Goal**: 已提交未派发的记录在进程重启后仍被派发，且恰好一次。

**Independent Test**: quickstart §2 的 SC-001/SC-002/SC-003/SC-010 四行。

- [x] T008 [US1] `server/internal/content/diagnostics/dispatch_postgres_test.go`（**新增，先写**）：DB 背书用例骨架与夹具——建临时 schema/清理、构造真实事务、一个可控 `Deliver`（可计数、可令其失败）。**先跑一次确认它红**（实现尚不存在） — **FR-007、SC-001**
- [x] T009 [US1] `server/internal/content/diagnostics/dispatch_postgres.go`（**新增**）：`PostgresOutbox` 的 `Register`——`tx.(pgx.Tx)` 断言失败即返回错误（**不暂存、不静默**）；用**传进来的那个 tx** 执行 `INSERT … ON CONFLICT DO NOTHING`；记下本组 id — **FR-004、FR-007、FR-013**
- [x] T010 [US1] 同文件：载荷超限在 `Register` 阶段判定——不写表、计 `Dropped`、**返回 nil**（返回 error 会让一条超大诊断记录打挂调用方的业务事务） — **FR-019、FR-017**
- [x] T011 [US1] 同文件：`Settle`——`committed=false` 不派发且清分组；`committed=true` 就地派发本组。**`tx` 在此已提交、不可用，只作分组键，不得用它执行任何 SQL**；二次结算派发 0 条 — **FR-008、FR-016**
- [x] T012 [US1] `server/internal/content/diagnostics/drain.go`（**新增**）：`Drainer` 的认领——`FOR UPDATE SKIP LOCKED` + 写 `claimed_until`；认领条件排除 `delivered_at IS NOT NULL` 与 `dead_lettered_at IS NOT NULL`；租约过期即可重新认领 — **FR-009、FR-009a、FR-012**
- [x] T013 [US1] 同文件：派发结果落库——成功写 `delivered_at` 并清租约；失败 `attempt_count++`、退避推后 `next_attempt_at`、写**已脱敏的短** `last_error`、计 `Errors`、清租约 — **FR-011、FR-016、FR-017**
- [x] T014 [US1] 同文件：`Start(ctx)` 周期唤醒与 `ctx` 取消即停（返回前结束当前一轮，不留悬挂 goroutine）；间隔/租约/上限/批量均为构造参数，默认值为包内常量，**模块内不读环境变量** — **FR-009b**
- [x] T015 [US1] `dispatch_postgres_test.go`：补 FR-006 与 FR-010——回滚后表内记录数为 0；提交后不跑就地排水、丢弃实例、新建 `Drainer` 跑一轮，断言派发 1 次，再跑一轮断言 0 次 — **FR-006、FR-010、FR-011、SC-001、SC-002**
- [x] T016 [US1] `server/internal/content/diagnostics/drain_test.go`（**新增**）：并发两个 `Drainer` 跑同一批，断言任一记录实际派发次数为 1；租约设毫秒级、认领后不派发直接放手，等过期再跑一轮，断言仍只派发 1 次 — **FR-012、SC-003、SC-010**

**Checkpoint**: US1 可独立交付——落库实现 + 排水器本身就闭合了 DIAG-04 的「带限制」那条限制。

## Phase 4: User Story 2 - 失败不拖垮业务，也不被静默吞掉 (Priority: P1)

**Goal**: 派发失败可重试、有终点、且只出现在既有的两个计数里。

**Independent Test**: quickstart §2 的 SC-004 行与 §3。

- [x] T017 [US2] `drain.go`：`attempt_count` 达上限即写 `dead_lettered_at` 并停止认领；进入死信**可被观察到**——走**既有技术日志事件**，**不得新增计数器或指标名**（复用 `Errors` 会把「又失败一次」与「不再重试了」混成同一个数字），也不得只写进表里没人看的一列 — **FR-018、FR-017**
- [x] T018 [US2] `drain.go`：把 `delivered_at` / `dead_lettered_at` 早于模块既有 `Retention`（7 天）的行删掉，**挂在同一次唤醒里**，不另起定时器 — **FR-020**
- [x] T019 [US2] `drain_test.go`：接收方恒失败——业务操作结果不变、记录保留、`sink_errors` 增长；重试上限设为 1～2，断言到达死信后不再认领；断言清理会删掉过保留期的已派发行 — **FR-016、FR-017、FR-018、FR-020、SC-004**
- [x] T020 [US2] 核对**不新增指标名**：`Metrics` 结构体与 `Overview` 的字段集合与 T002 留存逐字段一致；派发失败→`Errors`、丢弃→`Dropped` 的映射与 `MemoryOutbox.count` 相同 — **FR-017、SC-005**

## Phase 5: User Story 3 - 两个实现、两条互为反面的限制 (Priority: P2)

**Goal**: 无库场景仍可用，且两种实现各自的限制都有断言写着。

**Independent Test**: quickstart §2 与 §6 的变异行。

- [x] T021 [US3] `server/internal/content/diagnostics/dispatch.go`：改写文件头 `LIMIT, DELIBERATE` 段——那条限制**不再是整个特性的限制，而是 `MemoryOutbox` 的限制**；指向落库实现 — **FR-015**
- [x] T022 [US3] `server/internal/content/diagnostics/dispatch_test.go`：`TestOutboxDoesNotSurviveTheProcess` **改名点明主语**（如 `TestMemoryOutboxDoesNotSurviveTheProcess`），**语义不变**。就地反转会悄悄删掉对保留下来那个实现的限制记录 — **FR-014、FR-002、SC-006**
- [x] T023 [US3] `dispatch_postgres_test.go`：新增与上一条**互为反面**的断言（落库实现重启后不丢、不重复），并在注释里互相指名 — **FR-014、SC-007**
- [x] T024 [US3] `dispatch_postgres.go`：无数据库时 `PostgresOutbox` **明确失败**，不退化为内存行为；`MemoryOutbox` 对外行为一字不改，其既有 7 个用例不做语义改动即通过 — **FR-002、FR-003、SC-006**
- [x] T025 [US3] **接口未动的证据**：`var _ Outbox = (*PostgresOutbox)(nil)`；`Outbox` 接口的方法集与签名与 `origin/app-main` 逐字一致（`git diff` 该接口块应无输出）；既有 `TestOutboxCallersDependOnTheInterfaceOnly` **不做任何改动即通过**——它就是「换实现调用方一行不改」这句话的现成断言 — **FR-001**

## Phase 6: 调用点（Q1 = A）

- [x] T026 `server/internal/content/diagnostics/store.go`：新增一个与 `CommitRun` 并列的入口，在**同一个事务**内完成 diagnostics 自己的业务写入与一次 `Register`，提交返回后再 `Settle(ctx, tx, true)`。**`Store.Audit` 与 `Store.CommitRun` 的签名与行为一字不动** — **FR-005**
- [x] T027 同入口：只触碰 `content_*` 表，**不读写任何其他模块的表**（含 `seat_capacity_outbox`）；`grep -n "seat_capacity" server/internal/content/diagnostics/` 应无输出 — **FR-005a、FR-021、SC-011**
- [x] T028 `server/cmd/server/router.go`：装配 `PostgresOutbox` 与 `Drainer`，间隔等取值由此处读环境变量后传入，沿用 `router.go:443` 的既有模式；确认模块内没有 `os.Getenv` — **FR-009b、SC-012**

## Phase 7: Polish 与交付证据

> 顺序要求：**先跑检查（T030），再写证据（T031）**。证据引用的是实际运行结果，不是预期结果。

- [x] T029 [P] 在 `dispatch_postgres_test.go` 与 `drain_test.go` 顶部注释列出用例 → FR/SC 编号映射 — *证据整理，无独立 FR*
- [x] T030 运行 quickstart §1 全套并记录退出码：`go test -race ./internal/content/diagnostics -count=1 -v`（**核对无 SKIP**）、迁移 lint、`go build ./cmd/server`、`pnpm check:content-boundaries`、`pnpm check:diagnostics-contract`。**另做 quickstart §6 的 5 处变异验证**，每处改完即还原 — **SC-001～SC-004、SC-010**
- [x] T031 `docs/development/diagnostics-acceptance-mapping.md`：把 DIAG-04「交付 outbox 接口」行的**带限制**去掉并引用 T030 的**实际**结果；§4.3 第 3 条的 `~~DIAG-04~~` 行与 2026-09-14 更新（一）里「持久落库版本列为后续任务」一并回写；§4.1 计数按第 2 节各行重新统计 — **FR-014、SC-006、SC-007**
- [x] T032 核对边界与不变量：`git diff --stat` 确认改动文件 ⊆ plan.md → Source Code 清单；`server/pkg/executionpolicy/` 无改动；新增迁移中 `references|foreign key|cascade` 计数为 0、并发索引文件语句数为 1、迁移文件数 1004 → 1010；新增前端读取路径为 0；**新增 UI 单测数为 0、手动 UI Todo 数为 0**（本特性无页面改动） — **FR-022、FR-023、FR-024、FR-025、SC-008、SC-009**
- [x] T033 准备 PR 正文：改动与用途、实际命令与退出码（含是否有 SKIP）、变异验证结果、DIAG-04 从「带限制」变成什么、UI 影响（无）、手动 UI Todo（无）、回滚 — *交付，无独立 FR*

## Dependencies

- T001～T003 → 全部；**T003 不通过则停下来报告**，不要在无库状态下继续
- Foundational T004、T005、T006 可并行 → T007 → US1/US2
- US1：T008（先写，确认红）→ T009 → T010 → T011 → T012 → T013 → T014；T015 依赖 T011、T014；T016 依赖 T012、T014
- US2：T017、T018 依赖 T013；T019 依赖 T017、T018；T020 依赖 T002、T013
- US3：T021、T022 可并行；T023 依赖 T015；T024 依赖 T009；T025 依赖 T009
- Phase 6：T026 依赖 T009/T011；T027 依赖 T026；T028 依赖 T014、T026
- Polish：T029 依赖 US1/US2；**T030 → T031**；T032 依赖全部；T033 最后

## Parallel Example

```text
并行组 A（Foundational）：T004 建表 | T005 幂等索引 | T006 认领索引
并行组 B（跨故事）：US3 的 T021/T022（注释与改名） | US1 的 T012～T014（排水器）
```

## Implementation Strategy

1. **MVP = US1**：落库实现 + 排水器。它单独就闭合了 DIAG-04 那条「带限制」。
2. **US2 与 US1 共用 `drain.go`**，不建议并行给两个执行者。
3. **Phase 6 的调用点依赖 US1 全部完成**——没有可用的实现就没有可挂的调用点。
4. 任务总数 33；单 PR 新增约 4 个 Go 文件 + 6 个迁移文件，改动 4 个既有文件。
5. **本特性无页面改动、无手动 UI Todo**。若实施中发现必须改页面、必须碰执行闸门、或必须跨模块写表，**停下来报告**，不得自行扩范围。
