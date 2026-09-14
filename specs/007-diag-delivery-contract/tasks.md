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
> - **禁止编写或运行 UI 单测**。本功能**无页面改动**，不产生任何 UI 测试，也不产生手动 UI Todo。
> - **禁止用 computer use / 自动点击做验收**。
> - **保留该改动需要的非 UI 检查**：脚本自测、仓库扫描、既有边界检查、`pnpm typecheck`。
> - **不跑全量套件**（`pnpm test`、`make test`），只跑最窄的有用检查。
> - 未运行的检查记「按策略未执行，等待用户验证」，**绝不记为通过**。
> - **本功能不改生产代码**（clarify FR-003）。若实施中发现必须改，停下来问，不要自行扩范围。

## Phase 1: Setup（基线）

- [ ] T001 记录基线：`pnpm check:content-boundaries` 退出码；`ls server/internal/content/`（确认只有 `diagnostics`）；`ls server/migrations | wc -l`（SC-007 的比对基数）；`git rev-parse origin/app-main` — *基线，无 FR 映射*
- [ ] T002 记录 `.github/workflows/loretide-content.yml` 的 `on:` 段现状（逐字节留存，供 T018 比对） — **SC-005**

## Phase 2: Foundational（阻塞全部故事）

- [ ] T003 [P] `scripts/diagnostics-contract.json`（**新增**）：`{"version":1,"exemptions":[]}`。三字段规则见 data-model.md；空数组是正确的初始状态——今天没有任何模块需要豁免 — **FR-012**
- [ ] T004 [P] 确认 `scripts/content-boundaries.json` **只读不写**：本功能全程不修改它（T019 用 `git diff` 复核） — **FR-015**

## Phase 3: User Story 1 - 新模块作者知道要做哪几件事 (Priority: P1) 🎯 MVP

**Goal**: 一份逐项可判定的接入合同，每项指向真实公共入口。

**Independent Test**: quickstart §5；拿合同第三列对照 `diagnostics` 模块逐条核对。

- [ ] T005 [US1] `docs/development/diagnostics-onboarding-contract.md`（**新增**）：服务端条目表，每行三列「要做什么 / 公共入口 / 怎么算做到了」，入口取自 `contracts/onboarding-contract.md` 的核实结果 — **FR-001、FR-002**
- [ ] T006 [US1] 同文件：**错误码枚举**那一行第二列写「暂无公共入口（`log.go` 的 `codes` 未导出）」，第三列写「向诊断包申请导出，导出前由人工审查」，并注明导出为**后续任务**。**不得**要求模块引用私有符号 — **FR-003**
- [ ] T007 [US1] 同文件：新增前端两根一节（消费诊断错误对象经 `parseWithFallback`、呈现 `next_action`、不泄漏正文），**显著注明当前无静态检查、由人工审查**，前端侧检查列为后续任务 — **FR-016a**
- [ ] T008 [US1] 同文件：新增**静态检查边界声明**一节——检查只证明痕迹存在，不证明语义正确，也不证明真实执行器跑过；通过检查 ≠ 接入合格 — **FR-002**

**Checkpoint**: US1 可独立交付——合同文本本身就有价值，即使检查尚未落地。

## Phase 4: User Story 2 - PR 层挡住「新模块没接诊断」 (Priority: P1)

**Goal**: 缺项被点名报出；未落地模块沉默；脚本有自己的负例。

**Independent Test**: quickstart §1～§3；自动 T009～T013。

- [ ] T009 [US2] `scripts/check-diagnostics-contract.test.mjs`（**新增，先写**）：正例（`diagnostics` 模块通过）；**三条缺项负例**——分别只缺 E1 / E2 / E3，各自断言错误文本点名**对应那一条**；未落地模块无输出；只用 `NewID` 不算 E2。夹具为**合成文件映射**，不在仓库造假目录 — **FR-007、FR-011、FR-011a、SC-002、SC-003**
- [ ] T010 [US2] `scripts/check-diagnostics-contract.mjs`（**新增**）：导出 `check(files, config)` 纯函数；E1 词法扫描 import（跳过注释与字符串）、E2 匹配三类调用点之一、E3 扫 `_test.go`；**只读 `server/internal/content/`** — **FR-004、FR-011、FR-016**
- [ ] T011 [US2] 同文件：未落地模块（目录下无 `.go`）**完全跳过**，不产出任何输出；CLI 摘要报告「检查 N 个已落地 / 跳过 M 个未落地」 — **FR-005、SC-003**
- [ ] T012 [US2] 同文件：缺项输出**点名到条**（`<module>: missing <E1|E2|E3> — <说明>`）；CLI 入口走目录、设退出码；与 `check-content-boundaries.mjs` 同构、**无新增依赖** — **FR-006、FR-008、FR-011a**
- [ ] T013 [US2] `scripts/check-diagnostics-contract.test.mjs`：补豁免用例——登记后通过；登记缺 `reason` / `where` / 未知模块名则**配置无效并失败**（不降级为告警） — **FR-012**
- [ ] T014 [US2] `package.json`：新增 `check:diagnostics-contract`，`&&` 串联自测与扫描，与既有 `check:content-boundaries` 同形 — **FR-008**
- [ ] T015 [US2] `.github/PULL_REQUEST_TEMPLATE.md`：新增一条交付检查项，措辞可判定，与既有存储所有权项（第 41 行）并列 — **FR-010**
- [ ] T016 [US2] `.github/workflows/loretide-content.yml`：在既有两条边界检查步骤之后**新增一条 run 步骤**。**`on:` 段一字不动**。*（位置暂定，主任务未答；若裁定 CI 接入另立任务，删除本条即可）* — **FR-009**

## Phase 5: User Story 3 - 模拟通过 ≠ 真的通过 (Priority: P1)

**Goal**: 模拟证据与真实执行器证据分列，后者在禁用期间一律「未执行」。

**Independent Test**: quickstart §6。

- [ ] T017 [US3] `docs/development/diagnostics-onboarding-contract.md`：新增「模拟不代替真实通过」一节，证据分两栏（模拟可得 / 仅真实执行器可得），后者填「未执行（constitution 原则 IX）」 — **FR-013**
- [ ] T018 [US3] `.github/PULL_REQUEST_TEMPLATE.md` 与合同检查表：确认**不存在**一个可在真实执行器未跑的情况下达成的「全绿」状态——逐项复核措辞 — **FR-014**

## Phase 6: Polish 与交付证据

> 顺序要求：**先跑检查（T020），再写证据（T021）**。证据引用的是实际运行结果，不是预期结果。

- [ ] T019 [P] 在 `check-diagnostics-contract.test.mjs` 顶部注释列出用例 → FR 编号映射（FR-004～FR-014） — *证据整理，无独立 FR*
- [ ] T020 运行 quickstart 的自动检查全套并记录退出码：`node --test` 自测、仓库扫描、`pnpm check:diagnostics-contract`、`pnpm check:content-boundaries`、`pnpm typecheck`。**另做变异验证**：删掉 E1/E2/E3 任一条规则，确认对应负例变红，随后还原 — **SC-002、SC-004**
- [ ] T021 `docs/development/diagnostics-acceptance-mapping.md`：回写 D13-V12 三行与 DIAG-13「公共接入合同」，引用 T020 的**实际**结果。**明写第二条子句（真实 Codex 实测）在执行器禁用期间不可能通过，因此 D13-V12 不标整体通过**；§4.1 / §4.3 计数按第 2 节各行重新统计 — **FR-017、SC-006**
- [ ] T022 核对边界与不变量：`git diff --stat` 确认改动文件 ⊆ plan.md → Source Code 清单；`scripts/content-boundaries.json` 无改动；`server/migrations` 文件数与基线一致；`loretide-content.yml` 的 `on:` 段与 T002 留存逐字节一致 — **FR-015、SC-005、SC-007**
- [ ] T023 准备 PR 正文：改动与用途、实际命令与退出码、变异验证结果、闭合了 D13-V12 的哪几条 / 哪条闭合不了及原因、两处「暂定待确认」（CI 接入、豁免文件位置）、UI 影响（无）、手动 UI Todo（无）、回滚 — **FR-017**

## Dependencies

- T001、T002 → 全部
- Foundational T003、T004 → US2
- US1：T005 → T006 / T007 / T008（同一文件，顺序写）
- US2：T009 → T010 → T011 → T012；T013 依赖 T003、T012；T014 依赖 T012；T015、T016 依赖 T014
- US3：T017 依赖 T005（同一文件）；T018 依赖 T015、T017
- Polish：T019 依赖 US2；**T020 → T021**；T022 依赖全部；T023 最后

## Parallel Example

```text
并行组 A（Foundational）：T003 豁免配置 | T004 只读复核
并行组 B（跨故事）：US1 的 T005～T008（文档） | US2 的 T009/T010（脚本）
```

## Implementation Strategy

1. **MVP = US1 + US2**：合同文本 + 能报缺项的检查。两者都到位，D13-V12 第一条子句才算有覆盖。
2. **US3 与 US1 共用同一份合同文档**，不建议并行给两个执行者。
3. **US2 的脚本与 US1 的文档文件不重叠**，可并行。
4. 任务总数 23；单 PR 手写文件约 8 个，无需拆分。
5. **本功能无页面改动、无手动 UI Todo**。若实施中发现必须改页面或改生产代码，**停下来报告**，不得自行扩范围。
