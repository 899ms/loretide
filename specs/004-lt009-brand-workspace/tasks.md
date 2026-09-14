# Tasks: 品牌空间的时区属性与切换隔离

**Input**: Design documents from `/specs/004-lt009-brand-workspace/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/workspace-timezone.md, quickstart.md

**Blocked by**: LT-008、ARCH-02、DG-01。T001 是门禁：未满足则停在规划，不开始实施。

**Tests**: 行为性改动，先写失败测试。层：Go handler（`testutil.Call`）、core 纯函数（`// @vitest-environment node`）、locales parity（既有）。**不写 UI 单测**；表单与设置页进手动 UI Todo。

> **Loretide testing policy**: 禁止 UI 单测；禁止自动点击验收；未执行手动项记「按策略未执行，等待用户验证」。

## Format: `[ID] [P?] [Story] Description`

## Phase 1: Setup（门禁与基线）

- [ ] T001 核对前置：LT-008、ARCH-02、DG-01 在 `tasks/todo.md` / `tasks/architecture.md` / `tasks/diagnostics.md` 均已由主任务标为完成；任一未完成 → 停止，向主任务报告，不进入 Phase 2
- [ ] T002 记录基线：`(cd server && go test ./internal/handler -run Workspace -count=1)`、`pnpm --filter @multica/core test -- workspace`、`pnpm --filter @multica/views test -- locales/parity` 通过

## Phase 2: Foundational（阻塞全部故事）

- [ ] T003 [P] 新建 `packages/core/workspace/timezone.ts`：`TIMEZONE_SETTINGS_KEY`、`DEFAULT_TIMEZONE = "Asia/Shanghai"`、`isValidTimezone(name)`（`Intl.DateTimeFormat` try/catch）、`getWorkspaceTimezone(ws)`、`withWorkspaceTimezone(settings, tz)`（research D3 / D5）；在 `packages/core/workspace/index.ts` 导出
- [ ] T004 [P] 新建 `packages/core/workspace/timezone.test.ts`（`// @vitest-environment node`，先写）：合法值原样；缺 `settings` / `settings` 为 null / 为字符串 / 键非字符串 / 非法名 → 默认；`isValidTimezone("Asia/Shanghai")=true`、`("Mars/Olympus")=false`；`withWorkspaceTimezone({a:1}, tz)` 保留 `a` 且写入时区键，`withWorkspaceTimezone(null, tz)` 得到只含时区键的对象
- [ ] T005 [P] 核对 `server/cmd/server/main.go`（或入口）是否已 `import _ "time/tzdata"`；未导入则加上（Windows 开发机 `time.LoadLocation` 需要，research D2）
- [ ] T006 `server/internal/handler/workspace_test.go`（先写）：Create 带合法 `settings.loretide.timezone` → 201 且响应含该值；非法 → 400 不创建；不带 → 响应为 `Asia/Shanghai`；Update 非法 → 400 且原值不变；Update 只改其他 `settings` 键时时区键不变（透传）；Get 历史数据无该键 → 响应补默认
- [ ] T007 `server/internal/handler/workspace.go`：新增 `validateTimezoneSetting(settings any) error`（取 `loretide.timezone`，`time.LoadLocation`）；`CreateWorkspaceRequest` 增加 `Settings any \`json:"settings"\`` 并在 Create 中校验、写入；`UpdateWorkspace` 在写入前校验；`workspaceToResponse` 在 `settings` 缺该键或非字符串时填 `Asia/Shanghai`（只补响应，不回写）
- [ ] T008 `packages/core/workspace/mutations.ts`：确认无 `useUpdateWorkspace`；新增 `useUpdateWorkspace()`（`api.updateWorkspace(id, data)`，`onSettled` invalidate 工作区查询键，键名以 `queries.ts` 为准；不做乐观更新，research D5）；`packages/core/api/client.ts` 的 `createWorkspace` 请求体类型增加 `settings?: Record<string, unknown>`

## Phase 3: User Story 1 - 创建时设置时区，刷新与重登后保持 (Priority: P1) 🎯 MVP

**Goal**: 创建 / 设置页可设可改；默认 Asia/Shanghai；非法 400。

**Independent Test**: quickstart 手动 §1；自动 T004、T006。

- [ ] T009 [P] [US1] `packages/views/locales/{en,ja,ko,zh-Hans}/onboarding.json`：增加 `step_workspace.timezone_label`、`step_workspace.timezone_hint`；`packages/views/locales/{en,ja,ko,zh-Hans}/settings.json`：增加 `workspace.timezone_label`、`workspace.timezone_default_badge`、`workspace.timezone_invalid`；zh-Hans 用「工作区」「时区」「默认」
- [ ] T010 [US1] `packages/views/onboarding/steps/step-workspace.tsx`：用 `packages/ui` 既有 Select/Combobox（无则 `pnpm ui:add combobox`，overwrite 一律 `n`）加时区选择，候选来自 `Intl.supportedValuesOf("timeZone")`（不可用时回退常用列表 + 手输），默认预选 `DEFAULT_TIMEZONE`；提交体加 `settings: { [TIMEZONE_SETTINGS_KEY]: tz }`
- [ ] T011 [US1] `packages/views/settings/components/workspace-tab.tsx`：显示 `getWorkspaceTimezone(workspace)`；未显式设置（`settings` 无该键）时显示「默认」标记；修改后经 `useUpdateWorkspace` 保存（服务端整体替换 `settings`：用 `withWorkspaceTimezone(workspace.settings, tz)` 合并后整体发送，见 contracts；只发单键会清空其他设置）；非法值显示 `timezone_invalid`
- [ ] T012 [US1] `pnpm --filter @multica/views test -- locales/parity` 通过（四语言键一致）

**Checkpoint**: US1 可独立交付并手动验证。

## Phase 4: User Story 2 - 切换后不显示前一工作区数据 (Priority: P1)

**Goal**: 既有机制的可复核验收。

- [ ] T013 [US2] 按 quickstart 手动 §2 抽样 5 个空间范围接口与诊断流，把响应 `workspace_id` 核对结果写入交付记录；发现任何 `wsId` 缺失的查询键或未按 `workspace_id` 过滤的接口 → 作为独立缺陷报告主任务，不在本功能内修

## Phase 5: User Story 3 - 非成员拒绝且不泄漏 (Priority: P2)

- [ ] T014 [US3] `workspace_test.go`（先写）：非成员 GET `/api/workspaces/{A}` → 403/404 且 body 不含 `name` / `settings`；非成员 PATCH `settings.loretide.timezone` → 403/404 且 A 的时区不变；伪造 `X-Workspace-ID` 请求任一空间范围接口 → 拒绝（沿用既有 `getWorkspaceMember`，预期无需改代码，测试固定行为）

## Phase 6: User Story 4 - 术语一致 (Priority: P3)

- [ ] T015 [US4] 检查 T009 的 zh-Hans 文案与既有创建 / 切换 / 设置三处均用「工作区」；`grep -rn "品牌空间" packages/views/locales/zh-Hans/` 为空；PR 检查表勾选「中文产品文案已对照 conventions.zh.mdx」

## Phase 7: Polish

- [ ] T016 运行 `pnpm typecheck`、`pnpm --filter @multica/core test -- workspace`、`pnpm --filter @multica/views test -- locales/parity`、`(cd server && go test ./internal/handler -run Workspace -count=1)`、`pnpm check:content-boundaries`；全部通过
- [ ] T017 `git diff --stat` 确认文件 ⊆ plan.md 结构清单；超出即回退
- [ ] T018 准备 PR 正文：改动与用途、实际命令与退出码、UI 影响（创建表单 / 设置页）、手动 UI Todo 4 项（quickstart）、未执行项、回滚：撤销本 PR；`Related to #<编号>`

## Dependencies

- T001（门禁）→ 全部；T002 → 全部
- Foundational：T003/T004/T005 并行；T006 → T007；T008 依赖 T003（用常量）
- US1：T009 并行；T010 依赖 T003、T008；T011 依赖 T003、T008；T012 依赖 T009
- US2：T013 依赖 US1 部署到本地实例
- US3：T014 独立于 US1（可与 Foundational 并行）
- US4：T015 依赖 T009
- Polish 依赖全部

## Parallel Example

```text
并行组 A：T003 timezone.ts | T004 timezone.test.ts | T005 tzdata 导入 | T009 locales ×8
并行组 B：T006 Go 测试 | T014 非成员测试
并行组 C（Foundational 后）：T010 创建表单 | T011 设置页
```

## Implementation Strategy

1. T001 门禁不过就停：本功能的价值在 W-02 之后，前置未完成时提前实施会与 LT-008 / DG-01 的改动冲突。
2. MVP = Foundational + US1；US3 的测试可以提前写（固定既有行为）。
3. US2 是验收动作不是代码，随 US1 一起做。
4. 约 9 个手写文件 + 8 个 locale JSON；按 `tasks/plan.md` 完成标准，若主任务要求，拆为 PR-A（后端 + core：T003–T008、T014）与 PR-B（views + locales：T009–T012、T015）。
