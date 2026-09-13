# Research: 001 模块边界检查的验收闭合与本地入口

Phase 0。Technical Context 无 NEEDS CLARIFICATION；以下为本功能的技术决策。

## D1. 本地入口的形式

- **Decision**: `package.json` 新增 `"check:content-boundaries": "node --test scripts/check-content-boundaries.test.mjs && node scripts/check-content-boundaries.mjs"`。
- **Rationale**: 与现有 `check:ui-exports` / `check:ui-radii` 同一命名与形式；`pnpm` 在 Windows 与 Linux 都可用；`&&` 保证自测失败即终止，扫描不会掩盖自测失败（FR-001）。
- **Alternatives considered**: 只加 Makefile 目标——Windows 本机无 make，否决；单独写一个 wrapper 脚本——多一个文件，无收益。

## D2. 接入 `make check`

- **Decision**: 在 `scripts/check.sh` 中追加 `pnpm check:content-boundaries`（位置在 typecheck 之后、Go 测试之前）。
- **Rationale**: `CLAUDE.md` 把 `make check` 列为「broader verification」；Linux/CI 侧应一并覆盖。Windows 用户直接跑 pnpm 脚本，两侧调同一脚本同一配置（FR-002）。
- **Alternatives considered**: 不接入 `make check`——则 Linux 侧仍需手敲，与验收「接入验证命令」不符。

## D3. PR 模板检查项措辞

- **Decision**: 在 Checklist 中、「I have added or updated tests」之后插入：
  `- [ ] If this PR adds or changes a migration, sqlc query, or storage-layer code, every table it touches is owned by the module this PR changes (module list: scripts/content-boundaries.json; ownership table: docs/12 §2 in the documentation repository, branch main); cross-module table access is explained in the PR body`。
- **Rationale**: 措辞指向所有权表；不引入 CI 检查（clarify 2026-09-14）。
- **Alternatives considered**: 放在 PR 正文模板段——检查表更易被审查者扫到。

## D4. Go 侧「私有入口」是否已被检查器覆盖

- **Decision**: 不在本功能内扩展规则。Phase 1 quickstart 中加一条核查：用例 1 已包含 Go 分组导入的私有路径负例（`scripts/check-content-boundaries.test.mjs` 第 36 行「Go grouped/aliased/blank/dot imports」与第 7 行「private … fail」）；若运行后确认覆盖，证据节记「已覆盖」；若不覆盖，证据节记为「未覆盖，需另立任务」，本功能不补规则。
- **Rationale**: FR-007 禁止本功能改检查器规则；ARCH 卡片「不要求重构整个上游」。
- **Alternatives considered**: 直接补 Go 规则——范围蔓延，且与 clarify 精神（不扩范围）不符。

## D5. Node 版本差异

- **Decision**: 入口命令不锁 Node 版本；`docs/development/content-boundary-checks.md` 写明 CI 为 Node 22，本机 ≥22 即可；证据节记录本机实测版本。
- **Rationale**: `node --test` 在 22/24 行为一致；引入 `.nvmrc` 校验超出范围。

## D6. 验收对照节的格式

- **Decision**: 在 `docs/development/content-boundary-checks.md` 末尾新增 `## Acceptance mapping (ARCH-01/02)`，表格列：卡片条目 / 满足与否 / 证据（命令 + 退出码 + 文件或用例名）/ 备注。
- **Rationale**: FR-005、SC-004；主任务据此回写文档仓库，符合 constitution 原则 X。
