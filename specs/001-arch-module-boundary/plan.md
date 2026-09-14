# Implementation Plan: 模块边界检查的验收闭合与本地入口

**Branch**: `001-arch-module-boundary` | **Date**: 2026-09-14 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `/specs/001-arch-module-boundary/spec.md`

## Summary

检查器与 CI 已存在，本功能只补三样：一条仓库级本地入口（`pnpm check:content-boundaries`，并接入 `scripts/check.sh` 使 `make check` 覆盖）、PR 模板的存储所有权检查项、以及一份逐条对应 ARCH-01/02 验收项的证据节。不改检查器规则、不改 `content-boundaries.json`、不改 CI 触发条件。

## Technical Context

**Language/Version**: Node.js（CI 为 Node 22，本机 v24.14；脚本为 ESM `.mjs`）；bash（`scripts/check.sh`）

**Primary Dependencies**: 无新增。复用 `node --test` 与现有 `scripts/check-content-boundaries.{mjs,test.mjs}`

**Storage**: N/A

**Testing**: 现有 13 个 `node --test` 用例；本功能不新增测试文件，只在需要时向该文件追加用例

**Target Platform**: 开发机（Windows PowerShell / Linux shell）与 GitHub Actions

**Project Type**: Existing monorepo (Go backend + Next.js web + Electron desktop + Expo mobile + shared packages). Do not re-derive this.

**Performance Goals**: 入口命令 60 秒内完成（SC-001）

**Constraints**: 不改 `loretide-content.yml` 触发条件；不改检查器规则；不改模块集合；不得让「扫描通过」掩盖「自测失败」

**Scale/Scope**: 4～5 个文件的小改动：`package.json`、`scripts/check.sh`、`.github/PULL_REQUEST_TEMPLATE.md`、`docs/development/content-boundary-checks.md`、`CLAUDE.md`（一行）

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| 原则 | 适用 | 判定 |
|---|---|---|
| I. CLAUDE.md 权威 | 是 | 通过：只在 Verification 段加一行引用，不改规则 |
| II. 无 UI 单测 / 无自动 UI 验收 | 是 | 通过：无 UI；测试仅为 `node --test` 纯函数用例 |
| III. 模块边界 | 是 | 通过：本功能就是边界检查的入口，不放宽任何规则（FR-007） |
| IV. 状态分离 | 否 | N/A |
| V. 数据库 | 否 | N/A（存储所有权只进检查表，不进 CI，clarify 已定） |
| VI. API 解析 | 否 | N/A |
| VII. UI 复用 | 否 | N/A |
| VIII. 范围 | 是 | 通过：不扩展检查器覆盖范围，不顺手修 architecture.md（文档仓库） |
| IX. 执行器禁用 | 是 | 通过：不触碰 |
| X. 勾选 ≠ 验收 | 是 | 通过：证据节只提供证据，回写由主任务做 |

**Stop Conditions**：constitution 已就绪；权威来源可读；文件范围明确；无需削弱规则。→ 可进入 Phase 0。

**Post-design re-check (Phase 1 后)**：通过，设计未引入新文件类型或新依赖。

## Project Structure

### Documentation (this feature)

```text
specs/001-arch-module-boundary/
├── plan.md
├── research.md
├── quickstart.md
├── contracts/verify-entry.md
├── checklists/requirements.md
└── tasks.md               # /speckit-tasks 生成
```

（无 data-model.md：本功能不涉及数据实体。）

### Source Code (repository root)

```text
package.json                                  # + scripts.check:content-boundaries
scripts/check.sh                              # + 调用该入口，使 make check 覆盖
scripts/check-content-boundaries.mjs          # 不改
scripts/check-content-boundaries.test.mjs     # 仅在验收对照发现负例缺口时追加用例
.github/PULL_REQUEST_TEMPLATE.md              # + 存储所有权检查项
docs/development/content-boundary-checks.md   # + 「验收对照」节
CLAUDE.md                                     # Verification → Useful checks 加一行
```

**Structure Decision**: 全部落在已有文件；不新建目录，不新建脚本。入口命令用 `package.json` script 而非 Makefile 目标，因为 Windows 本机无 `make`（`docs/development/windows.md`）。

## Complexity Tracking

无违反项。
