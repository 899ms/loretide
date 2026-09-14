# Tasks: 诊断 HTTP 追踪贯通与请求脱敏

**Input**: Design documents from `/specs/005-diag-trace-and-sanitize/`

**Prerequisites**: plan.md, spec.md（含 2026-09-14 Clarifications）、research.md、data-model.md、contracts/（三份）、quickstart.md

**Tests**: 行为性改动，按 `CLAUDE.md` Testing「先在正确的包写失败的测试」。测试层：Go 中间件（纯 `httptest`）、Go handler（`testutil.Call` + `dbfx`）、Go 纯规则与派发（无数据库）、core 契约（`// @vitest-environment node`）。**不写 UI 单测**。

> ## Loretide testing policy（不可协商）
>
> - **禁止编写或运行 UI 单测**，本地与 CI 皆然；不得靠改名、改扩展名或重新归类规避。
> - **禁止用 computer use / 自动点击做验收**。本功能预计无页面改动；若实施中改到页面，列出受影响的屏幕、步骤与预期，交用户手动确认后才算通过。
> - **保留该改动需要的非 UI 检查**：Go 测试、core 契约测试、模块边界检查、`pnpm typecheck`、构建。
> - **不跑全量套件**（`pnpm test`、`make test`），只跑最窄的有用检查。
> - 未运行的检查记「按策略未执行，等待用户验证」，**绝不记为通过**。
> - `server/internal/handler/TestMain` 在无数据库时 `os.Exit(0)`：`go test` 会打印 `ok` 但零用例执行。handler 相关任务必须带 `-v` 确认真实执行，否则记为未运行。

## Format: `[ID] [P?] [Story] Description`

## Phase 1: Setup（基线）

- [ ] T001 记录基线：`(cd server && GOTOOLCHAIN=auto go test ./internal/content/diagnostics ./internal/middleware -count=1)` 与 `(cd server && GOTOOLCHAIN=auto go test ./internal/handler -run ContentDiagnostic -count=1 -v)`，`pnpm --filter @multica/core exec vitest run content/diagnostics/contract`。**handler 一行必须看 `-v` 输出确认非零用例**；若本机无 PostgreSQL，先起库并 `go run ./cmd/migrate up`，否则把该行记为未运行
- [ ] T002 确认 `pnpm check:content-boundaries` 存在；不存在则改用 `node --test scripts/check-content-boundaries.test.mjs && node scripts/check-content-boundaries.mjs`，记录基线退出码

## Phase 2: Foundational（阻塞全部故事）

- [ ] T003 [P] `server/internal/content/diagnostics/contract.go`：`Event` 增 `Route string \`json:"route,omitempty"\`` 与 `Upstream string \`json:"upstream_trace,omitempty"\``（data-model.md）。不改任何既有字段
- [ ] T004 [P] `packages/core/content/diagnostics/contract.ts`：`eventSchema` 增 `route` / `upstream_trace`（`z.string().optional().default("")`），transform 为 `route` / `upstreamTrace`
- [ ] T005 [P] `packages/core/content/diagnostics/contract.test.ts`（先写，先失败）：新字段缺省为 `""`；带值时正确 transform；缺 `event_id` 的畸形事件仍走 `parseWithFallback` 兜底。文件头保持 `// @vitest-environment node`

## Phase 3: User Story 1 - 一个 trace id 串起 HTTP 入口到 daemon (Priority: P1) 🎯 MVP

**Goal**: 三段共用一个 trace；用户凭据的入站 `traceparent` 不被采信；daemon 路径采信；记录范围不变。

**Independent Test**: quickstart §1～§4；自动 T006、T008、T012。

- [ ] T006 [US1] `server/internal/middleware/trace_test.go`（**新增，先写**）：表驱动、纯 `httptest`、无数据库。用例——(a) 无 `traceparent` 时建本实例 trace；(b) 带合法 `traceparent` 且非 daemon 时**不**采信为父级，仅存为候选；(c) 格式非法时既不采信也不留存候选；(d) 采样位为「不采样」仍建 trace；(e) 中间件不改变响应状态码与 body。映射 FR-001、FR-004a、FR-015
- [ ] T007 [US1] `server/internal/middleware/trace.go`（**新增**）：为每请求建本实例 span context 并写入 `r.Context()`；校验入站 `traceparent`（与 `Unpack` 同口径），通过则存为候选父级。**不 import `content/diagnostics`**（research D5：避免跨内容根依赖，否则需改 `scripts/content-boundaries.json` 的 `adapters`）
- [ ] T008 [US1] `server/internal/middleware/trace_test.go`：补 daemon 采信用例——`X-Actor-Source` 为 machine credential 时候选被提升为父级，trace-id 取自入站值。映射 FR-004、FR-016
- [ ] T009 [US1] `server/internal/middleware/daemon_auth.go`：在 `DaemonAuth` 认证成功之后调用提升动作（一处调用，不改认证逻辑本身）。**判定只依据认证结果，不读任何自称身份的请求头或请求体**
- [ ] T010 [US1] `server/cmd/server/router.go`：把 trace 传播中间件挂进公共栈（`chimw.RequestID` 一带，认证之前）。**不改 `DiagnosticTrace` 的挂载位置**——记录范围保持在 `/api/content-diagnostics`（FR-015）
- [ ] T011 [US1] `server/internal/handler/content_diagnostics.go`：`DiagnosticTrace` 保留 `Child(ctx)` 调用不变（此时父级已有效，会沿用 TraceID 并新开 span）；改为写入 `Route`；未被采信的候选写入 `Upstream`。映射 FR-002、FR-003
- [ ] T012 [US1] `server/internal/handler/content_diagnostics_test.go`（先写）：(a) 一次模拟运行的 HTTP 入口事件、队列消息与 daemon 回写三处 `trace_id` 相同；(b) 带用户凭据 `traceparent` 的请求，其事件 `trace_id` ≠ 入站值且 `upstream_trace` = 入站值；(c) 非法 `traceparent` 时 `upstream_trace` 为空且请求成功。映射 FR-002、FR-004、FR-004a
- [ ] T013 [US1] `server/internal/daemon/client.go`：四处 `http.NewRequestWithContext`（713、1179、1215、1248）经统一 helper 注入 `traceparent`。队列与 WS 两段**不动**（research D6）

**Checkpoint**: US1 可独立交付；quickstart §1～§4 可手动核对。

## Phase 4: User Story 2 - 记录请求身份而不泄漏 (Priority: P1)

**Goal**: 事件能说出「是哪个接口」，凭据头 / 路径参数 / 查询串一律不进入技术日志与导出包。

**Independent Test**: quickstart §5；自动 T014。

- [ ] T014 [US2] `server/internal/content/diagnostics/log_regression_test.go`（先写）：负例为主——`Authorization` / `authorization`（大小写变体）/ `Cookie` 不出现名称与取值；路径参数取值不出现；查询串键名与取值均不出现；同名多值不拼接；超长输入整体丢弃而非截断。映射 FR-005、FR-006、FR-008
- [ ] T015 [US2] `server/internal/content/diagnostics/log.go`：新增请求头准入 allowlist（大小写不敏感比对）与路径/URL 安全形状规则，接入既有 `Sanitize()`。**不改**既有 `oneOf` / `token` / `hexID` 的行为（FR-012）
- [ ] T016 [US2] `server/internal/handler/content_diagnostics.go`：`Route` 取 chi `RouteContext().RoutePattern()` + 方法；取不到时写 `""`，**不回退到实际路径**（research D3、contracts/request-sanitization.md）
- [ ] T017 [US2] `server/internal/handler/content_diagnostics_test.go`：断言导出包（预览与下载两条路径）中 `route` 为路由模板形状，且凭据头 / 路径参数 / 查询串取值出现次数为 0，`redacted` 为真。映射 FR-007、FR-008

## Phase 5: User Story 3 - 事务内登记 + 提交后派发 (Priority: P2)

**Goal**: 回滚不派发、提交必派发或可见待派发、失败可见、幂等、有界；进程退出丢失是**写明的限制**。

**Independent Test**: quickstart §6；自动 T018。

- [ ] T018 [US3] `server/internal/content/diagnostics/dispatch_test.go`（**新增，先写**）：(a) 事务回滚 → 派发 0 次且无「已成功」痕迹；(b) 事务提交 → 派发 1 次或可见待派发；(c) 派发失败 → 计数上升、业务操作照常成功；(d) 相同 `idempotencyKey` 重复派发不产生重复效果；(e) 队列超界按「丢弃可见」处理。映射 FR-009、FR-010、FR-011
- [ ] T019 [US3] `server/internal/content/diagnostics/dispatch.go`（**新增**）：`Register(tx, item)` + `Dispatch()`；登记项四元组 `{id, kind, payload, idempotencyKey}`；进程内有界队列；失败计入既有 `LogBuffer.Errors` / `Dropped`，**不新增指标名**。**不新增表、不新增迁移**（FR-017）
- [ ] T020 [US3] `server/internal/content/diagnostics/dispatch_test.go`：补可替换性用例——用一个替身实现调用同一签名，证明调用方不依赖进程内存储（contracts/transactional-dispatch.md「可替换性」）
- [ ] T021 [US3] `server/internal/content/diagnostics/dispatch.go`：确认 `Store.Audit` 的既有事务内写入与失败即回滚行为**未被改动**，并在文件注释写明「关键审计不走本接口」。映射 FR-010、FR-012

## Phase 6: Polish 与交付证据

- [ ] T022 [P] 在 `trace_test.go`、`log_regression_test.go`、`dispatch_test.go`、`content_diagnostics_test.go`、`contract.test.ts` 顶部注释列出用例 → FR 编号映射（FR-001～FR-013）
- [ ] T023 `docs/development/diagnostics-acceptance-mapping.md`：§4.3 的三行「实现缺失」回写闭合证据（DIAG-02 请求头 / 路径 URL 脱敏、DIAG-04 outbox 接口、DIAG-05 HTTP trace 传播）。**同时写明 outbox 为进程内、不跨重启**（FR-010a），并如实保留其余 12 条未闭合条目。**不得声称任何 D13-V 条目整体通过**（FR-014）
- [ ] T024 运行 quickstart 的自动检查全套并记录退出码：三条 `go test`（handler 那条带 `-v` 确认非零用例）、`pnpm --filter @multica/core exec vitest run content/diagnostics/contract`、`pnpm typecheck`、边界检查。任一未运行的记「按策略未执行」
- [ ] T025 `git diff --stat` 核对改动文件 ⊆ plan.md → Source Code 清单；超出即回退或在 PR 中单列说明
- [ ] T026 准备 PR 正文：改动与用途、实际命令与退出码、闭合了 §4.3 哪三行、未闭合哪几条、**outbox 不跨重启的限制**、UI 影响（预计无）、手动 UI Todo（预计无）、回滚：撤销本 PR；声明本功能未新增任何外发网络目标（FR-013）

## Dependencies

- T001、T002 → 全部
- Foundational T003～T005 → 全部故事；三者互相并行
- US1：T006 → T007 → T008 → T009；T010 依赖 T007；T011 依赖 T003、T010；T012 依赖 T011；T013 独立于 T006～T012，可并行
- US2：T014 → T015；T016 依赖 T003、T011；T017 依赖 T015、T016
- US3：T018 → T019 → T020 → T021；整条链独立于 US1/US2，可并行
- Polish 依赖全部；T023 依赖 T024 的实际结果（先跑检查再写证据）

## Parallel Example

```text
并行组 A（Foundational）：T003 contract.go | T004 contract.ts | T005 contract.test.ts
并行组 B（US1 内）：T006 中间件测试 | T013 daemon 出站注入
并行组 C（跨故事）：US3 整条链（T018～T021） | US2 的 T014/T015
```

## Implementation Strategy

1. **MVP = Foundational + US1**：先让三段连通、来源区分正确、记录范围不变。quickstart §1～§4 通过即可开 Draft PR。
2. **US2 紧随其后**，与 US1 共用 `content_diagnostics.go`，文件重叠多，不建议并行给两个执行者。
3. **US3 与前两者文件不重叠**，可由另一执行者并行推进。
4. 任务总数 26；若单 PR 手写文件超过约 11 个的量级，按 `tasks/plan.md` 的完成标准拆两个 PR：PR-A = Foundational + US1 + US2，PR-B = US3 + 证据回写。
5. **本功能预计无页面改动**。若实施中发现必须改页面，停下来报告并按 constitution 原则 II 转手动清单，不得自行写 UI 单测。
