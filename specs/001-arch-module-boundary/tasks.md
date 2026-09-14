# Tasks: 模块边界检查的验收闭合与本地入口

**Input**: Design documents from `/specs/001-arch-module-boundary/`

**Prerequisites**: plan.md, spec.md, research.md, contracts/verify-entry.md, quickstart.md

**Tests**: 本功能不新增测试文件；仅当验收对照（T009）发现负例缺口时，向既有 `scripts/check-content-boundaries.test.mjs` 追加用例。

> **Loretide testing policy**: 无 UI；不写 UI 单测；所有验证为 `node --test` 与命令退出码。未执行的检查记「未执行」，不记通过。

**Organization**: 按用户故事分组；US1 是最小可交付单元。

## Format: `[ID] [P?] [Story] Description`

## Phase 1: Setup（基线）

- [x] T001 记录基线：在仓库根运行 `node --version`、`node --test scripts/check-content-boundaries.test.mjs`、`node scripts/check-content-boundaries.mjs`，把 Node 版本、13 个用例结果、扫描结果与耗时写入 `specs/001-arch-module-boundary/baseline.txt`（不入库，仅 PR 正文引用）

## Phase 2: Foundational

无（不改检查器、不改配置）。

## Phase 3: User Story 1 - 本地一条命令跑出与 CI 相同的边界检查 (Priority: P1) 🎯 MVP

**Goal**: `pnpm check:content-boundaries` 在任一 worktree 得到与 CI 相同的结果与退出码。

**Independent Test**: quickstart §1–§3。

- [x] T002 [US1] 在 `package.json` 的 `scripts` 中、`check:ui-radii` 之后新增 `"check:content-boundaries": "node --test scripts/check-content-boundaries.test.mjs && node scripts/check-content-boundaries.mjs"`
- [x] T003 [US1] 在 `scripts/check.sh` 的 typecheck 步骤之后插入 `pnpm check:content-boundaries`，失败即退出（沿用该脚本现有的失败处理方式）
- [x] T004 [P] [US1] 在 `CLAUDE.md` → Verification → Useful checks 代码块中增加一行 `pnpm check:content-boundaries`
- [x] T005 [P] [US1] 在 `docs/development/content-boundary-checks.md` 开头「It runs in … via」段落之后，补一段本地入口说明：命令、与 CI 逐字相同、Node ≥ 22（CI 为 22）
- [x] T006 [US1] 执行 quickstart §1（通过 + 计时）、§2（负例失败与恢复）、§3（`grep` 命中 check.sh），把命令、退出码、输出片段追加到 `baseline.txt`；另附本次 CI（`loretide-content.yml`）运行链接作为 Linux 侧证据（FR-002）

**Checkpoint**: US1 完成即可独立交付。

## Phase 4: User Story 2 - PR 作者确认存储所有权 (Priority: P2)

**Goal**: PR 模板含存储所有权检查项。

**Independent Test**: quickstart §4。

- [x] T007 [P] [US2] 在 `.github/PULL_REQUEST_TEMPLATE.md` Checklist 中、「I have added or updated tests where applicable」之后插入 research D3 的检查项原文
- [x] T008 [US2] 同步修改 `.github/pull_request_template.md`（当前两文件内容相同，`cmp` 确认改后仍相同）（主任务确认：仅一个模板文件，任务书误记）

## Phase 5: User Story 3 - 主任务凭证据回写 ARCH-01/02 状态 (Priority: P3)

**Goal**: 逐条证据节。

**Independent Test**: quickstart §5。

- [x] T009 [US3] 核查 Go 私有入口覆盖（research D4）：阅读 `scripts/check-content-boundaries.test.mjs` 第 7 行与第 36 行用例，确认 Go 分组导入中的跨模块非公开路径是否被判为违规；结论写入 T010 的表格「备注」列；若未覆盖，只记录，不改规则
- [x] T010 [US3] 在 `docs/development/content-boundary-checks.md` 末尾新增 `## Acceptance mapping (ARCH-01/02)`，表格 6 行：ARCH-01 交付 / 验收 / 验证，ARCH-02 交付 / 验收 / 验证；每行列「满足与否 / 证据（命令 + 退出码 + 文件或用例名）/ 备注」，并写明检查器为静态导入检查、不覆盖 SQL 所有权与 content 根目录以外代码（FR-006）
- [x] T011 [P] [US3] 运行 quickstart §5 的模块集合比对，把 12 个模块名与 docs/12 §2 的 11 个 + `diagnostics` 的对照结果写入同一节（预期差异 0）

## Phase 6: Polish

- [x] T012 `git diff --stat` 确认改动文件 ⊆ {package.json, scripts/check.sh, CLAUDE.md, docs/development/content-boundary-checks.md, .github/PULL_REQUEST_TEMPLATE.md, .github/pull_request_template.md}；超出即回退
- [x] T013 再次运行 `pnpm check:content-boundaries`，退出 0；准备 PR 正文：改动文件与用途、实际命令与退出码、未验证项（如 Go 私有入口未覆盖）、UI 影响：无、手动 UI Todo：无、回滚：撤销本 PR 提交

## Dependencies

- T001 → 全部
- US1（T002–T006）独立可交付；T004、T005 可与 T002/T003 并行
- US2（T007–T008）与 US1 无依赖，可并行
- US3（T009–T011）依赖 US1 完成（证据需引用入口命令的实测结果）；T011 可与 T009 并行
- Polish 依赖全部

## Parallel Example

```text
并行组 A（不同文件）：T002 package.json | T004 CLAUDE.md | T005 content-boundary-checks.md | T007 PR 模板
并行组 B：T009 用例核查 | T011 模块集合比对
```

## Implementation Strategy

1. 先交付 US1（MVP）：入口命令 + 文档一行；本地实测通过后即可开 Draft PR。
2. US2 一并进同一 PR（2 个模板文件）。
3. US3 在 US1 实测后补证据节。
4. 全部改动预计 6 个文件，符合任务卡「1～5 个手写文件」的量级（多出的是重复的 PR 模板文件）。
