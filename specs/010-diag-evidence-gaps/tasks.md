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
> - **禁止编写或运行 UI 单测**。本特性**不为任何界面行补测试**，不产生 UI 测试，也不产生手动 UI Todo。US3 整条就是为这条设的。
> - **禁止用 computer use / 自动点击做验收**。
> - **保留该改动需要的非 UI 检查**：Go 包内测试（含 `-race`）、迁移 lint、`node:test` 静态检查、既有边界与合同检查、`go build ./cmd/server`。
> - **不跑全量套件**（`pnpm test`、`make test`），只跑最窄的有用检查。
> - 未运行的检查记「按策略未执行，等待用户验证」，**绝不记为通过**。**DB 背书用例在没有 `LORETIDE_DIAG_TEST_DATABASE_URL` 时会跳过，跳过记为未执行，不记为通过。**
> - **本特性不改任何生产代码。** 实施中若发现某条行为必须改生产代码才能测，**停下来报告**（FR-019），不得自行扩范围。

## Phase 1: Setup（基线）

- [x] T001 记录基线：`go test -race ./internal/content/diagnostics -count=1 -v` 的 PASS/SKIP/FAIL 三个计数；`ls packages/views/content/diagnostics/` 的测试文件数（SC-007 的比对基数）；`git rev-parse origin/app-main` — *基线，无 FR 映射*
- [x] T002 确认 `LORETIDE_DIAG_TEST_DATABASE_URL` 已设且诊断包用例无 SKIP；**若不可用则停下来报告**——FR-007 与 FR-010 都需要真实事务，无库时它们只会被跳过 — **FR-018**
- [x] T003 [P] 留存 `git diff --stat origin/app-main -- server/` 的空基线，供 T026 比对「生产代码零改动」 — **FR-014、SC-008**

## Phase 2: User Story 1 - 对照表说实话 (Priority: P1) 🎯 先做

**Goal**: 已有测试的行指到测试；关不掉的行写明缺功能；自相矛盾的行改掉。

**Independent Test**: quickstart §2 的 SC-001 / SC-002 两行。

- [x] T004 [US1] 复核 DIAG-02「交付私有正文脱敏」：确认 `TestModelOutputSamplesNeverSurviveSanitize` 的 13 字段负例与 `TestLogRegressionSanitizeRules` 的三处 `Message` 断言确实覆盖该行，记录覆盖到哪、没覆盖哪 — **FR-001、FR-002**
- [x] T005 [US1] 复核 DIAG-12「下载保存服务端原始字节与文件名」：确认 `TestContentDiagnosticExportDownloadNamesTheFileItWantsSaved` 覆盖 `Content-Disposition`、原样 bundle 与 wire key 名；**前端保存路径仍无测试**这一点要单独记下 — **FR-001、FR-002、FR-004**
- [x] T006 [US1] `docs/development/diagnostics-acceptance-mapping.md`：把 T004、T005 两行的类型改为「自动测试已通过」并写入**测试函数名**；DIAG-12 那行的备注保留「前端保存路径仍无测试，按原则 II 不补」 — **FR-002、FR-004、SC-001、SC-002**
- [x] T007 [US1] 同文件：DIAG-03「交付级别配置」按 Q1 = A 改写——证据列改为 `Severity` 四值枚举收敛 + `limits.go` 两项配置边界并引用对应测试；**备注明写「按级别过滤写入」在生产代码中不存在，属缺失功能** — **FR-003、SC-002**
- [x] T008 [US1] 同文件：把「按级别过滤写入」登记为**后续任务**（§5 或等价位置），写明它是功能缺口不是测试缺口 — **FR-003、FR-014**

**Checkpoint**: US1 可独立交付——对照表不再误导读者，即使一条新测试都还没写。

## Phase 3: User Story 2 - 六条真实缺口各有一条定向测试 (Priority: P1)

**Goal**: 六条缺口各有一条会因实现被删而变红的测试。

**Independent Test**: quickstart §4 的六处变异。

### 迁移约束（无库）

- [x] T009 [US2] `server/internal/migrations/content_constraints_test.go`（**新增，先写**）：判定逻辑写成**接受「文件名 → 内容」映射的纯函数**，夹具用合成映射——**不在 `server/migrations/` 里造假文件**，造了会被既有的编号唯一与 up/down 配对两条 lint 扫到，两套检查会打架。**先跑一次确认它红** — **FR-006、SC-004**
- [x] T010 [US2] 同文件：实现 R1（无 `REFERENCES`/`FOREIGN KEY`）、R2（无 `CASCADE`）、R3（`CREATE INDEX` 必须 `CONCURRENTLY`）、R4（含并发索引的文件语句数为 1）；范围为编号 **≥ 468** 且文件名含 `content_` — **FR-006**
- [x] T011 [US2] 同文件：判定前剥掉 `--` 行注释、`/* */` 块注释与字符串字面量；补一条**注释不误报**的负例（`-- FOREIGN KEY` 在注释里不得变红） — **FR-006**
- [x] T012 [US2] 同文件：失败信息**点名文件与违反的规则**；补 R1 / R3 / R4 三条负例夹具各自断言点名内容 — **FR-006、SC-004**
- [x] T013 [US2] 对当前仓库跑一次：`468`～`476` 共 12 个文件全部通过 — **FR-006**

### 回写 trace 与读路径只读（DB 背书）

- [x] T014 [US2] `server/internal/content/diagnostics/readpath_test.go`（**新增，先写**）：沿用 `testStore` 夹具；回写用例走「`Simulate` → `CommitRun` → 从 `content_operation_audit` 读回」，断言读回事件的 `trace_id` / `operation_id` **等于** `run.Events[0]` 的对应值——**断言相等，不是断言非空**（非空断言在「复制成了另一个随机 id」时依然绿） — **FR-007**
- [x] T015 [US2] 同文件：只读用例对 diagnostics 四张表取调用前后 `count(*)` 快照，断言 `Query` / `Runs` / `GetRun` 前后行数一致 — **FR-010、SC-006**
- [x] T016 [US2] 两条用例在无 `LORETIDE_DIAG_TEST_DATABASE_URL` 时**跳过而非失败**，与既有 DB 背书用例同形 — **FR-018**

### 队列统计与样本不足（无库）

- [x] T017 [US2] `server/internal/content/diagnostics/overview_metrics_test.go`（**新增，先写**）：`QueueWait` 用例——队列事件时长被累加，**且非队列事件的时长不计入**。缺后半条时，把 `component=="queue"` 判断删掉依然绿 — **FR-008**
- [x] T018 [US2] 同文件：`P95` 阈值**两侧各一条**——19 个样本为 nil、20 个样本非 nil。只验一侧等于没验边界 — **FR-009、SC-005**

### 不自动上传（静态检查）

- [x] T019 [US2] `scripts/check-diagnostics-no-upload.test.mjs`（**新增，先写**）：正例（当前仓库通过）；负例 `fetch("https://…")` 与 `sendBeacon` 各一条，断言点名文件与命中的原语；**不误报**用例（4 处 `refetch()` + 一处 `api.fetchRaw()` 不得变红）；清单过时用例（少一个文件即失败）。夹具为合成映射 — **FR-011b、FR-011c、FR-012、SC-003a、SC-003b**
- [x] T020 [US2] `scripts/check-diagnostics-no-upload.mjs`（**新增**）：导出 `check(files)` 纯函数 + CLI 入口，与 `check-diagnostics-contract.mjs` 同形；匹配按**词边界**（`.` 也排除在前缀里，`api.fetchRaw(` 不是原生 `fetch`） — **FR-011、FR-011c**
- [x] T021 [US2] 同文件：扫描范围写成**显式四文件清单**，**排除 `packages/core/api/client.ts`**（下载经 `api.contentDiagnosticDownload` → `fetchRaw` 是合法路径）；清单内任一文件不存在即失败 — **FR-011a、FR-012**
- [x] T022 [US2] `package.json`：新增 `check:diagnostics-no-upload`，`&&` 串联自测与扫描，与既有 `check:diagnostics-contract` 同形 — **FR-011**
- [x] T023 [US2] `docs/development/diagnostics-acceptance-mapping.md`：把六条新测试对应的六行类型改为「自动测试已通过」并写入测试名/脚本名 — **FR-002、SC-001**

## Phase 4: User Story 3 - 界面行保持手动 (Priority: P2)

- [x] T024 [US3] 核对**新增 UI 单测数为 0**、`packages/views/content/diagnostics/` 下测试文件数与 T001 基线一致；在交付记录中逐条列出保持手动的界面行（DIAG-06 ×3、DIAG-09 ×6、DIAG-13 ×2、DIAG-12 预览、DIAG-10 模型参数快照）及依据 — **FR-016、SC-007**
- [x] T025 [US3] 在交付记录中**显式列出不在范围的两行及原因**：DIAG-02 验证嵌套字段（`Event` 扁平，造不出真实嵌套负例）、DIAG-10 不复制媒体（已被 `specs/008` FR-012/013 认领） — **FR-015**

## Phase 5: Polish 与交付证据

> 顺序要求：**先跑检查（T027），再写证据（T028）**。证据引用的是实际运行结果，不是预期结果。

- [x] T026 核对不改生产代码：`git diff --stat origin/app-main -- server/` 去掉 `_test.go` 后无任何改动。**若实施过程中曾出现「不改生产代码就测不了」的情形，交付记录 MUST 记下是哪一条、当时停下来报告的内容与裁决**，MUST NOT 自行改动后事后说明 — **FR-014、FR-019、SC-008**
- [x] T027 运行 quickstart §1 全套并记录退出码（诊断包用 `-v` 报 PASS/SKIP/FAIL 三个计数）；**另做 §4 的 6 处变异验证**，每处改完即还原 — **FR-013、SC-003、SC-003a、SC-003b、SC-010**
- [x] T028 `docs/development/diagnostics-acceptance-mapping.md`：§4.1 计数按第 2 节各行**重新统计**；引用 T027 的**实际**结果 — **FR-005、SC-009**
- [x] T029 核对边界与不变量：改动文件 ⊆ plan.md → Source Code 清单；`server/migrations` 文件数不变；CI 工作流 `on:` 段未改；`pkg/executionpolicy` 无改动 — **FR-017**
- [x] T030 准备 PR 正文：改动与用途、实际命令与退出码（含 SKIP 计数）、变异验证结果、**逐行核实结论表**（2 条已有测试 / 1 条缺功能 / 6 条新测试）、不在范围的两行及原因、UI 影响（无）、手动 UI Todo（无）、回滚 — *交付，无独立 FR*

## Dependencies

- T001～T003 → 全部；**T002 不通过则停下来报告**，不要在无库状态下继续
- **US1（T004～T008）→ US2**：核实的产出是「该写哪几条」，它是 US2 的输入
- 迁移约束：T009（先写，确认红）→ T010 → T011 → T012 → T013
- DB 背书：T014（先写，确认红）→ T015 → T016
- 无库用例：T017、T018 可并行
- 静态检查：T019（先写，确认红）→ T020 → T021 → T022
- T023 依赖 T013、T016、T018、T022
- US3：T024 依赖 T001；T025 可随时
- Polish：T026 依赖全部实现任务；**T027 → T028**；T029 依赖全部；T030 最后

## Parallel Example

```text
并行组 A（Setup）：T003 基线留存 | T001/T002 顺序跑
并行组 B（US2 内部，四组互不重叠）：
  迁移约束 T009～T013 | DB 背书 T014～T016 | 无库用例 T017/T018 | 静态检查 T019～T022
```

## Implementation Strategy

1. **US1 先做且可独立交付**：对照表不再误导读者，即使一条新测试都还没写。
2. **US2 的四组互不重叠**，可并行给不同执行者；T023 是它们的汇合点。
3. **US3 不产出代码**，只产出交付记录里的两段说明与两个计数核对。
4. 任务总数 30；新增 3 个 Go 测试文件 + 2 个脚本文件，改动对照表与 `package.json`。
5. **本特性不改生产代码、无页面改动、无手动 UI Todo**。若发现必须改生产代码才能测，**停下来报告**。
