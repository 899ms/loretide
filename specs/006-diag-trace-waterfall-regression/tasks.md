# Tasks: 诊断 trace 瀑布视图与回归关联

**Input**: Design documents from `/specs/006-diag-trace-waterfall-regression/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/trace-waterfall.md, contracts/regression-verdict.md, quickstart.md, manual-ui-todo.md

**Tests**: 本功能**要求**测试，但只要 node 环境纯函数测试。新增两个 `.test.ts`，UI 单测 0 个。

> ## Loretide testing policy (NON-NEGOTIABLE)
>
> - **不写、不跑 UI 单测**，本地与 CI 皆然；不得靠改名、改扩展名或重新归类绕过。
> - **不用 computer use 或自动点击做验收**。页面行为列入 `manual-ui-todo.md`，由用户确认后才算数。
> - 保留本次范围需要的非 UI 检查：纯函数测试、`pnpm check:content-boundaries`、`pnpm typecheck`、i18n parity。
> - 不跑可能拉进 UI 测试的全量命令。跑最窄有用的检查。
> - 未执行的检查记「按策略未执行，等待用户验证」，**绝不记为通过**。
> - 测试放置遵循 `CLAUDE.md` → Testing：共享逻辑进 `packages/core/*.test.ts`；不需要 DOM 的 `.test.ts` 首行写 `// @vitest-environment node`。
> - 每个行为只有一个权威层。纯函数矩阵不在 DOM 挂载里重跑。

**Organization**: 按用户故事分组。US1（瀑布）与 US2/US3（回归）互不依赖，可并行。

**Traceability**: 每条任务末尾以 `→` 标注其覆盖的 FR / SC 编号。21 个需求键（FR-001～FR-014、SC-001～SC-007）全部有至少一条任务，对照见文末覆盖表。

## Format: `[ID] [P?] [Story] Description → FR/SC`

## Phase 1: Setup（基线）

- [ ] T001 记录基线：运行 `pnpm --filter @multica/core exec vitest run content/diagnostics/contract.test.ts content/diagnostics/stream-state.test.ts`、`pnpm check:content-boundaries`、`pnpm typecheck`，把命令与退出码写入 `specs/006-diag-trace-waterfall-regression/baseline.txt`（不入库，内容进 PR 正文）。确认改动前这三项均为绿，避免把既有失败算到本功能头上 → SC-007

## Phase 2: Foundational（阻塞全部故事）

无需要先行的基础设施。`STREAM_EVENT_CAP` 已导出（`contract.ts:23`），`runSchema` / `eventSchema` 字段已齐备，两条故事可直接开始。

## Phase 3: User Story 1 - 瀑布层级与时间轴 (Priority: P1) 🎯 MVP

**Goal**: `buildTraceWaterfall` 把扁平事件变成带层级、带时间偏移、带折叠的行，视图只负责画。

**Independent Test**: quickstart §1 的 `trace-waterfall.test.ts` 全绿；页面侧 manual-ui-todo W-1 ～ W-9。

### Tests for User Story 1（先写，且必须先失败）⚠️

- [ ] T002 [P] [US1] 新建 `packages/core/content/diagnostics/trace-waterfall.test.ts`，首行 `// @vitest-environment node`。按 `contracts/trace-waterfall.md` 的七条不变量各写至少一条用例：终止性（成环 / 自引用）、层级完整性（无悬空片段）、失败可见性（折叠后错误 span 及祖先链仍在）、非负偏移、无 NaN、不夹取（clockSkew 位置照实）、数量守恒。此时应全部失败 → FR-001, FR-002, FR-003, FR-004, FR-014, SC-003, SC-006
- [ ] T003 [P] [US1] 在同一文件补边界输入用例，覆盖 `contracts/trace-waterfall.md` 边界表**全部七行**：空输入、单 span、全 0 耗时、**负 `durationMs` → `invalidDuration` 且宽度不为负**、孤儿父、成环、超 cap。负耗时一行是本次整改新增（analyze C2），此前 data-model 定义了该异常却无人测它 → FR-001, FR-004, FR-014

### Implementation for User Story 1

- [ ] T004 [US1] 新建 `packages/core/content/diagnostics/trace-waterfall.ts`，实现 `buildTraceWaterfall(events, options?)`。按 research D1 两趟算法：`Map<spanId, event>` 建索引，父链上溯用 `Set` 检测环（终止性由 Set 容量上界保证）；按 D2 以最小 `occurredAt` 为基准算 `startOffsetMs`，`Date.parse` 失败置 `invalidTime` 且偏移取 0（禁止 NaN）；负 `durationMs` 归一为 0 并置 `invalidDuration`；按 D3 折叠：必保留集 = 含 `errorCode` 的 span ∪ 其完整祖先链，再按 `startOffsetMs` 补足至 cap，其余按最近可见祖先聚合为 `collapsed` 行。`anomaly` 按 data-model 五级优先级取首个命中。默认 cap 引用 `STREAM_EVENT_CAP`，**不写第二个字面量 200** → FR-001, FR-002, FR-003, FR-004, FR-009, FR-014
- [ ] T005 [US1] 在 `packages/core/content/diagnostics/index.ts` 导出 `buildTraceWaterfall` 与 `WaterfallRow` / `WaterfallResult` 类型 → FR-009
- [ ] T006 [US1] 跑 T002 + T003 至全绿：`pnpm --filter @multica/core exec vitest run content/diagnostics/trace-waterfall.test.ts` → SC-003, SC-006
- [ ] T007 [US1] 改写 `packages/views/content/diagnostics/index.tsx` 的 trace 标签页（现约 686–730 行）：改为消费 `buildTraceWaterfall` 的 `rows`，用 `depth` 做缩进、用 `startOffsetMs` 做左偏移、用 `durationMs` 做宽度；`anomaly` 显示对应标注；`collapsed` 行显示「另有 N 条」并可展开（本地 `useState`，按 research D5 不进 Zustand）。复用既有 `Progress` / `SettingsCard`，用语义 token 与 `--text-*` 字号；改动前读 `docs/development/design/README.md`。**不写任何 UI 单测** → FR-001, FR-002, FR-004, FR-011, FR-014
- [ ] T008 [P] [US1] 在四语言 `packages/views/locales/{en,zh-Hans,ja,ko}/common.json` 的 `diagnostics` 下新增瀑布文案键：时钟偏差、父 span 不在本次运行内、成环异常、无效时间、无效耗时、另有 N 条、本次运行没有可展示的 span。中文对照 `conventions.zh.mdx` 术语表 → FR-001, FR-004, FR-014
- [ ] T009 [US1] **SC-001 的验收路径核对（不得自动化）**：确认 `manual-ui-todo.md` 的 **W-9** 存在、指向 SC-001、且措辞包含「10 秒内」「不需要打开原始 JSON」两个可判定条件。实现完成后把 W-9 交用户在浏览器计时确认。**MUST NOT** 用任何纯函数测试宣布 SC-001 通过——纯函数能证明偏移算得对，证明不了人能否在 10 秒内看出来（analyze C1） → SC-001
- [ ] T010 [US1] 跑 `pnpm --filter @multica/views exec vitest run locales/parity.test.ts` 确认四语言齐全 → FR-011

**Checkpoint**: US1 可独立交付——瀑布可用，回归关联未动。

## Phase 4: User Story 2 - 四态回归判定 (Priority: P1)

**Goal**: 未评估的运行显示「未运行」而不是空白，且任何不确定都不倒向「通过」。

**Independent Test**: `regression.test.ts` 的四态真值表全绿；页面侧 manual-ui-todo V-1 ～ V-6。

### Tests for User Story 2（先写，且必须先失败）⚠️

- [ ] T011 [P] [US2] 新建 `packages/core/content/diagnostics/regression.test.ts`，首行 `// @vitest-environment node`。按 `contracts/regression-verdict.md` 真值表五行各一条用例（空串 → `not_run`；`passed` + 状态正常 → `passed`；`passed` + 状态未完成 → `undecidable`；`failed` → `failed`；未知值 → `undecidable`）。另加一条**不变量用例**：遍历一组构造输入，断言除真值表第二行外没有任何输入返回 `passed`。此时应全部失败 → FR-005, SC-002

### Implementation for User Story 2

- [ ] T012 [US2] 新建 `packages/core/content/diagnostics/regression.ts`，实现 `describeRegressionVerdict(run)`：按真值表映射四态，`rawRegression` 原样带出原值，`basisAvailable` 反映 `expectedCode` / `actualCode` 是否都非空。按 research D4，未知取值与状态冲突统一落 `undecidable` → FR-005, FR-006, FR-009
- [ ] T013 [US2] 在 `index.ts` 导出 `describeRegressionVerdict` 与 `RegressionVerdict` 类型 → FR-009
- [ ] T014 [US2] 跑 T011 至全绿 → SC-002
- [ ] T015 [US2] 改 `index.tsx` 运行列表的回归结果列（现约 899 行 `{r.regression}`）：改为显示四态标签；`passed` 同时显示 `expectedCode` / `actualCode` 作为判定依据；`undecidable` 同时显示原值。**不得**在概览区增加聚合计数 → FR-005, FR-006, FR-011, FR-013
- [ ] T016 [P] [US2] 四语言新增四态文案键，并沿用面板既有 `text086` 口径——「通过」旁保留「表示模拟结果符合该故障预期」的限定语，避免读成「功能可用」 → FR-005

**Checkpoint**: US2 可独立交付——即使 US1 与 US3 未做，「未运行」也不再显示成空白。

## Phase 5: User Story 3 - 回归关联与跳转 (Priority: P2)

**Goal**: 四项定位各自带可用性状态，原故障可页内跳转。

**Independent Test**: `regression.test.ts` 中 `describeRunLinkage` 的四种组合；页面侧 manual-ui-todo L-1 ～ L-5。

### Tests for User Story 3（先写，且必须先失败）⚠️

- [ ] T017 [P] [US3] 在 `regression.test.ts` 追加 `describeRunLinkage` 用例：`originalRunId` 为空 → `self`；非空且在 `readableRunIds` 中 → `linkable`；非空但不在集合中 → `unreadable`；格式不合法 → `missing`；`scenario` / `module` / `build` 空串 → `missing`。外加一条断言：`self` 与 `missing` 的返回结果不相等 → FR-007, SC-004

### Implementation for User Story 3

- [ ] T018 [US3] 在 `regression.ts` 实现 `describeRunLinkage(run, readableRunIds)`，按 data-model 的 `originalRun.state` 四态判定；函数**不发起任何请求**，可读集合由调用方提供 → FR-007, FR-009, FR-012
- [ ] T019 [US3] 在 `index.ts` 导出 `describeRunLinkage` 与 `RunLinkage` 类型 → FR-009
- [ ] T020 [US3] 跑 T017 至全绿 → SC-004
- [ ] T021 [US3] 改 `index.tsx` 运行列表：四项定位各自渲染其状态；`linkable` 时点击原故障**在页内切换**到该运行详情（复用既有「选择运行 / 查看运行」路径，**不新增路由**）；`unreadable` 标注不可读及原因；`self` 标「本身即原故障」；`missing` 标「未记录」。`readableRunIds` 由当前已取回的运行列表构造 → FR-007, FR-008, FR-011
- [ ] T022 [P] [US3] 四语言新增关联文案键：本身即原故障、原故障运行不可读、未记录 → FR-007

**Checkpoint**: 三条故事均可独立验证。

## Phase 6: Polish

- [ ] T023 跑 quickstart §1 ～ §6 全部命令并记录退出码：两个纯函数测试、无新增 `.test.tsx` 核对、`pnpm check:content-boundaries`、`pnpm typecheck`、i18n parity、`git diff --stat -- server/` 为空 → FR-010, FR-011, SC-002, SC-003, SC-004, SC-006, SC-007
- [ ] T024 **范围与授权核对**：①`git diff --stat` 确认改动文件 ⊆ {`packages/core/content/diagnostics/trace-waterfall.ts`、`trace-waterfall.test.ts`、`regression.ts`、`regression.test.ts`、`index.ts`、`packages/views/content/diagnostics/index.tsx`、`packages/views/locales/{en,zh-Hans,ja,ko}/common.json`}，超出即回退；②特别确认 `server/` 与任何迁移目录的 diff 为空；③**确认未新增任何 `api.*` 调用、查询入口或 `Scope` 过滤放宽**——三个纯函数都不发请求，`readableRunIds` 由调用方从已取回数据构造，本功能不应新增一条数据读取路径（analyze C3：FR-012 此前无任务验证） → FR-010, FR-012
- [ ] T025 核对 `manual-ui-todo.md` 的 **20 条**与实际实现一一对应（若实现过程中页面行为有变，更新清单条目而不是删掉它），全部保持「待用户验证」 → FR-011, SC-005
- [ ] T026 准备 PR 正文：改动文件与用途、实际命令与退出码、未验证项（20 条手动条目未执行、UI 层无自动测试、SC-001 只能由 W-9 人工确认）、UI 影响：有页面改动、手动 UI Todo：20 条、回滚：撤销本 PR 提交 → FR-011, SC-005

## 需求覆盖表

| 需求 | 任务 |
|---|---|
| FR-001 层级 | T002, T003, T004, T007, T008 |
| FR-002 时间轴 | T002, T004, T007 |
| FR-003 终止性 | T002, T004 |
| FR-004 折叠 | T002, T003, T004, T007, T008 |
| FR-005 四态 | T011, T012, T015, T016 |
| FR-006 判定依据 | T012, T015 |
| FR-007 四项定位 | T017, T018, T021, T022 |
| FR-008 页内跳转 | T021 |
| FR-009 纯函数可测 | T004, T005, T012, T013, T018, T019 |
| FR-010 不改 Go | T023, T024 |
| FR-011 无 UI 单测 | T007, T010, T015, T021, T023, T025, T026 |
| FR-012 授权口径 | T018, **T024** |
| FR-013 无概览聚合 | T015 |
| FR-014 不夹取 | T002, T003, T004, T007, T008 |
| SC-001 10 秒可读 | **T009**（人工，W-9） |
| SC-002 四态用例 | T011, T014, T023 |
| SC-003 异常有限步返回 | T002, T006, T023 |
| SC-004 四项定位组合 | T017, T020, T023 |
| SC-005 UI 单测 0 | T025, T026 |
| SC-006 折叠后失败可见 | T002, T006, T023 |
| SC-007 边界检查 | T001, T023 |

21 个需求键全部有覆盖。

## Dependencies & Execution Order

- T001 → 全部
- US1（T002–T010）与 US2（T011–T016）**无相互依赖**，可完全并行
- US3（T017–T022）依赖 US2 的 `regression.ts` 文件已存在（T012），因为两个函数同文件；逻辑上不依赖
- T008 / T016 / T022 都改同一批 locale 文件，**不可并行**，需串行或合并为一次改动
- T009 只核对清单条目，可在 US1 任意时点做；W-9 的**执行**在实现完成后交用户
- Polish（T023–T026）依赖全部

### Within Each User Story

- 测试先写并先失败（T002/T003 → T004；T011 → T012；T017 → T018）
- 纯函数先于视图接线（T004 → T007；T012 → T015；T018 → T021）
- 视图接线后再补文案，避免键名反复

## Parallel Example

```text
并行组 A（不同文件）：T002+T003 瀑布测试 | T011 回归测试
并行组 B（前两者完成后）：T004 瀑布实现 | T012 判定实现
串行：T008 → T016 → T022（同一批 locale 文件，避免冲突）
```

## Implementation Strategy

1. **MVP = US1 或 US2 任一**。两者独立：US1 让瀑布可读，US2 消除唯一可能造成错误验收结论的缺口。若只能做一个，**先做 US2**——它修的是「空白被读成通过」，比看不清层级更危险。
2. US3 紧随 US2，同文件、同测试文件，增量小。
3. 三条故事全部完成后跑 Polish，再开 PR。
4. 全部改动预计 7 个手写文件（2 实现 + 2 测试 + 1 导出 + 1 视图 + 4 locale 按一组算），符合任务卡量级；单 PR。
