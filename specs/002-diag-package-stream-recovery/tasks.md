# Tasks: 诊断包下载保真与实时流断线恢复

**Input**: Design documents from `/specs/002-diag-package-stream-recovery/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/stream-page.md, contracts/export-download.md, quickstart.md

**Tests**: 行为性改动，按 `CLAUDE.md` Testing「先在正确的包写失败的测试」。测试层：Go handler（`testutil.Call`）、core 纯函数与契约（`// @vitest-environment node`）。**不写 UI 单测**；页面行为进 `manual-ui-todo.md`。

> **Loretide testing policy**: 禁止 UI 单测（本地与 CI）；禁止自动点击验收；未执行的手动项记「按策略未执行，等待用户验证」。

## Format: `[ID] [P?] [Story] Description`

## Phase 1: Setup（基线）

- [x] T001 运行 `pnpm --filter @multica/core test -- content/diagnostics` 与 `(cd server && go test ./internal/handler -run ContentDiagnostic -count=1)`，记录基线通过

## Phase 2: Foundational（阻塞全部故事）

- [x] T002 [P] `packages/core/content/diagnostics/contract.ts`：`pageSchema` 增加 `rotate: z.boolean().optional().default(false)`；导出 `STREAM_EVENT_CAP = 200` 并让 `mergeEvents` 使用它；新增纯函数 `streamQuery(filter, after): string`（键名 kind / trace_id / run_id / component / error_code / severity / from / until / after）与 `parseContentDispositionFilename(header: string | null): string | null`
- [x] T003 [P] `packages/core/content/diagnostics/contract.test.ts`（先写，先失败）：`rotate` 缺省 false；缺 `cursor` 的畸形页走 `parseWithFallback` 兜底；`streamQuery` 序列化与空过滤；文件名解析（带引号 / 不带 / 缺失 → null）
- [x] T004 `packages/core/api/client.ts`：在 `contentDiagnosticStream` 旁新增 `contentDiagnosticDownload(query: string, signal?: AbortSignal): Promise<Response>`，内部 `fetchRaw` POST `/api/content-diagnostics/export?${query}`，body `{}`
- [x] T005 [P] `server/internal/content/diagnostics/store.go`：`Page` 结构增加 `Rotate bool \`json:"rotate,omitempty"\``

## Phase 3: User Story 1 - 计划轮换不可见、真实断线才提示、续读保持过滤 (Priority: P1) 🎯 MVP

**Goal**: 3 分钟观察无假断线；重连带过滤与游标；退避；denied 停止。

**Independent Test**: quickstart 手动 §1–§3；自动 T006、T008。

- [x] T006 [US1] `server/internal/handler/content_diagnostics_test.go`（先写）：deadline 触发后最后一行 `rotate=true`；查询串 `severity=error&kind=x` 透传到 `Store.Query` 的 Filter；非 owner/admin 成员在流中被撤销后连接结束且无 rotate 行
- [x] T007 [US1] `server/internal/handler/content_diagnostics.go` `ContentDiagnosticStream`：`deadline.C` 分支改为「再查一页 → `page.Rotate=true` → 写出并 flush → return」；其余分支不变
- [x] T008 [US1] 新增 `packages/core/content/diagnostics/stream-state.ts`：`type StreamStatus`、`nextStreamState(state, event)`、`computeBackoff(prevMs)`（2s 起翻倍、上限 30s、成功归零）；转移表见 data-model.md
- [x] T009 [US1] 新增 `packages/core/content/diagnostics/stream-state.test.ts`（`// @vitest-environment node`，先写）：全部转移，含 `page(rotate)`→connected 且延迟 0、`error(403)`→denied、退避序列 2/4/8/16/30/30、pause/resume 保留 cursor、wsId 变化归零、filter 变化归零
- [x] T010 [US1] `packages/core/content/diagnostics/queries.ts`：`useDiagnosticStream(wsId, enabled, filter)` 改为驱动状态机；请求用 `streamQuery(filter, cursor)`；rotate → 不改 notice、立即重连；非 rotate 结束 / 异常 → 退避；HTTP 403/404 → denied 并停止；`visibilitychange` 回前台且 disconnected → 立即重连；返回 `{events, status, gap, notice, clearGap}`
- [x] T011 [US1] `packages/views/content/diagnostics/index.tsx`：把当前过滤条件传入 hook；`denied` 状态显示错误对象的 `next_action`；`reconnecting` 与 `connected` 的显示区分保持既有样式（不新建控件）

**Checkpoint**: US1 可独立交付并手动验证。

## Phase 4: User Story 2 - 下载与服务端字节逐字一致 (Priority: P1)

**Goal**: 下载文件 `cmp` 服务端响应无差异；文件名来自响应头；非授权无文件。

**Independent Test**: quickstart 手动 §4–§5；自动 T012。

- [x] T012 [US2] `content_diagnostics_test.go`（先写）：POST export 带 `account_id=x` → 403 且 body 为诊断错误对象；成功时响应头含 `Content-Disposition: attachment; filename=`
- [x] T013 [US2] `queries.ts` 的 `download` mutation：调用 `api.contentDiagnosticDownload`；`!res.ok` → 解析诊断错误对象并抛出；成功 → `{ blob: await res.blob(), filename: parseContentDispositionFilename(res.headers.get("content-disposition")) ?? "loretide-diagnostics.json" }`
- [x] T014 [US2] `packages/views/content/diagnostics/index.tsx`：`Props.download` 类型改为 `(result: { blob: Blob; filename: string }) => void`；下载失败显示 `next_action`
- [x] T015 [US2] `apps/web/platform/content-diagnostics.ts`：`downloadDiagnosticBundle({blob, filename})` 直接 `URL.createObjectURL(blob)`，`link.download = filename`
- [x] T016 [US2] `apps/web/app/[workspaceSlug]/(dashboard)/diagnostics/page.tsx`：类型随 T014/T015 对齐（一般无需改动，typecheck 确认）

## Phase 5: User Story 3 - 缺口、乱序与重复可见且不误导 (Priority: P2)

- [x] T017 [US3] `queries.ts`：`gap` 粘性——置 true 后只在 `wsId` 变化或 `clearGap()` 时复位；不因后续正常页复位
- [x] T018 [US3] `packages/views/content/diagnostics/index.tsx`：缺口提示旁加「清除」动作；当 `events.length >= STREAM_EVENT_CAP` 显示「仅显示最近 200 条」（复用现有提示样式）
- [x] T019 [P] [US3] `contract.test.ts`：`mergeEvents` 去重 / 按 `sequence` 排序 / 裁剪到 `STREAM_EVENT_CAP` 的用例（已有则补齐边界：重复 id 取后到者、乱序）

## Phase 6: User Story 4 - 手动验收矩阵与测试映射 (Priority: P2)

- [x] T020 [US4] 新建 `specs/002-diag-package-stream-recovery/manual-ui-todo.md`：逐句列出 D13-V05、V08、V11，每句 ≥1 条「页面 / 操作 / 预期 / 用户确认（待验证）」；把 quickstart 手动 §1–§7 映射进去
- [x] T021 [P] [US4] 在 `contract.test.ts`、`stream-state.test.ts`、`content_diagnostics_test.go` 顶部注释列出用例 → FR 编号映射（FR-001～FR-008）

## Phase 7: Polish

- [x] T022 运行 `pnpm typecheck`、`pnpm --filter @multica/core test -- content/diagnostics`、`(cd server && go test ./internal/handler -run ContentDiagnostic -count=1)`、`pnpm check:content-boundaries`（views 未引入平台 API）；全部通过
- [x] T023 `git diff --stat` 确认文件 ⊆ plan.md 结构清单；超出即回退
- [ ] T024 准备 PR 正文：改动与用途、实际命令与退出码、`manual-ui-todo.md` 链接、UI 影响（诊断页状态区 / 下载 / 提示）、手动 UI Todo 清单、未执行项、回滚：撤销本 PR；正文声明本功能未新增任何外发请求（FR-011）

## Dependencies

- T001 → 全部
- Foundational T002–T005 → 全部故事；T002/T003/T005 互相并行，T004 依赖 T002 无（独立）
- US1：T006 → T007；T008 → T009 → T010 → T011
- US2：T012 独立；T013 依赖 T002/T004；T014 依赖 T013；T015 依赖 T014；T016 最后
- US3：T017 依赖 T010；T018 依赖 T017；T019 并行
- US4：T020 依赖 US1–US3 的行为定稿；T021 并行
- Polish 依赖全部

## Parallel Example

```text
并行组 A：T002 contract.ts | T003 contract.test.ts | T005 store.go Page
并行组 B（US1 内）：T006 Go 测试 | T008 状态机 | T009 状态机测试
并行组 C：T012 Go 导出测试 | T019 mergeEvents 测试 | T021 映射注释
```

## Implementation Strategy

1. MVP = Foundational + US1：先让轮换不可见、过滤透传、退避与 denied 正确；手动 §1–§3 通过即可开 Draft PR。
2. US2 下载保真是第二个 P1，与 US1 文件重叠少（client.ts / platform / views 的 download 分支），可紧接着进同一 PR。
3. US3 小改动随 US1 之后。
4. US4 的清单在行为定稿后写，避免返工。
5. 任务总数 24；若单 PR 超过 5 个手写文件的量级（本功能约 9 个），按 `tasks/plan.md` 完成标准拆为两个 PR：PR-A = Foundational + US1 + US3，PR-B = US2 + US4。
