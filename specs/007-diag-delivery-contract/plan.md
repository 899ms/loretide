# Implementation Plan: 诊断接入合同与交付检查

**Branch**: `007-diag-delivery-contract` | **Date**: 2026-09-14 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `/specs/007-diag-delivery-contract/spec.md`

## Summary

三件东西，一条主线：**让「接入诊断」这件事从口头约定变成可判定的**。

1. **合同文本**（`docs/development/diagnostics-onboarding-contract.md`）：新 content 模块必须接入的诊断面逐项写明，每项给「要做什么 / 公共入口 / 怎么算做到了」。错误码枚举那一项写成「向诊断包申请导出」——它今天私有，合同不能要求引用私有符号（clarify FR-003）。
2. **静态检查**（`scripts/check-diagnostics-contract.mjs`）：与既有 `check-content-boundaries.mjs` 同形——导出一个纯函数 `check(files, config)`，CLI 入口负责走目录。判定 E1/E2/E3 三条，缺哪条报哪条，只管 `server/internal/content/`，对尚无目录的模块沉默。
3. **交付检查表**：PR 模板一条勾选项 + 合同里的「模拟 ≠ 真实」一节，把模拟证据与真实执行器证据分列。

**为什么检查能测**：既有检查器把 `check(files, config)` 导出为纯函数，测试用**合成的文件映射**当夹具（`{'server/internal/content/work-editor/a.go': '...'}`）。本功能照搬——三个「分别只缺一条」的夹具都是内存里的字符串，不需要在仓库里造假模块目录。

## Technical Context

**Language/Version**: Node 22（脚本与测试，`node:test`）；无 Go 代码改动

**Primary Dependencies**: **无新增**。既有检查器只用 `node:fs` / `node:path` / `typescript`（TS 解析）；本检查只需处理 Go 源码，连 `typescript` 都不需要

**Storage**: 无。不新增迁移，不碰数据库

**Testing**: `node --test scripts/check-diagnostics-contract.test.mjs`，夹具为合成文件映射。**不写 UI 单测**（本功能无页面）

**Target Platform**: 仓库脚本 + CI

**Project Type**: Existing monorepo (Go backend + Next.js web + Electron desktop + Expo mobile + shared packages). Do not re-derive this.

**Performance Goals**: 与既有边界检查同量级（全仓扫描约 2 秒）；本检查只读 `server/internal/content/`，更快

**Constraints**: 不改生产代码（clarify FR-003）；不改 `content-boundaries.json` 的模块图与依赖规则；不改 CI 触发条件；对尚无目录的 11 个模块必须沉默

**Scale/Scope**: 新增 4 个文件（合同、脚本、脚本测试、豁免配置），改 3 个既有文件（`package.json`、PR 模板、mapping），CI 一个步骤。预计手写文件 7～8 个

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| 原则 | 适用 | 判定 |
|---|---|---|
| I. CLAUDE.md 权威 | 是 | 通过：脚本形态、测试落点、提交前缀均按 `CLAUDE.md` |
| II. 无 UI 单测 / 无自动 UI 验收 | 是 | 通过：本功能无页面改动，无 UI 单测，无手动 UI 项 |
| III. 模块边界 | 是 | 通过：新脚本在 `scripts/`，不属任何 content 根，不产生跨根 import；**不改既有边界检查器的任何规则** |
| IV. 状态分离 | 否 | N/A：无前端状态 |
| V. 数据库 | 否 | N/A：**无迁移、无 schema 变更**（SC-007 把它钉为验收条件） |
| VI. API 解析 | 否 | N/A：无 API 改动 |
| VII. UI 复用 | 否 | N/A：无页面 |
| VIII. 范围 | 是 | 通过：只做 D13-V12 三子句与 DIAG-13 接入合同；前端侧检查、错误码导出均显式列为后续任务 |
| IX. 执行器禁用 | 是 | **本功能的核心之一**：合同明写真实执行器禁用期间相关证据一律记「未执行」，FR-014 禁止出现「未跑却全绿」的状态 |
| X. 勾选 ≠ 验收 | 是 | 通过：FR-017 明禁声称 D13-V12 整体通过——第二条子句在执行器禁用期间不可能通过 |

**Stop Conditions**：无触发。→ 进入 Phase 0。

**Post-design re-check**：通过。唯一需要记录的张力是「一个今天几乎空转的检查」，已在 Complexity Tracking 说明。

## Project Structure

### Documentation (this feature)

```text
specs/007-diag-delivery-contract/
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/
│   ├── onboarding-contract.md      # 合同文本要写成什么样
│   └── check-cli.md                # 检查脚本的输入 / 输出 / 退出码
├── checklists/requirements.md
└── tasks.md
```

### Source Code (repository root)

```text
docs/development/diagnostics-onboarding-contract.md   # 新增：接入合同正文（含前端两根的要求与「无静态检查」声明）
scripts/check-diagnostics-contract.mjs                # 新增：导出 check(files, config) + CLI 入口
scripts/check-diagnostics-contract.test.mjs           # 新增：node:test，合成夹具，含三条缺项负例
scripts/diagnostics-contract.json                     # 新增：豁免登记（位置已确认，见 spec Clarifications 第 5 条）
package.json                                          # + check:diagnostics-contract 入口
.github/PULL_REQUEST_TEMPLATE.md                      # + 一条交付检查项
.github/workflows/loretide-content.yml                # + 一个 run 步骤（触发条件一字不动）
docs/development/diagnostics-acceptance-mapping.md    # D13-V12 与 DIAG-13 对应行回写
```

**Structure Decision**: 不新建目录。脚本与既有检查器并列放 `scripts/`，合同与 mapping 并列放 `docs/development/`。**不碰 `scripts/content-boundaries.json`**——它是 ARCH-01/02 的交付物，本功能只读它的模块列表。

## Complexity Tracking

| 张力 | 为什么接受 | 被拒绝的更简方案 |
|---|---|---|
| 检查今天几乎空转（12 个模块只有 1 个会被判定） | 这正是现在加的理由。等第一个新模块落地时再加，那个 PR 要同时背「实现模块」与「引入新红线」，最可能的结果是当场删掉检查。现在加，它先在 `diagnostics` 上跑绿，新模块落地时自然生效 | 推迟到有模块时再做：省事，但把检查的引入时机放在了它最容易被拒绝的时刻 |
| 判定用三条静态条件，而非「真的接入了」 | 静态检查只能证明痕迹存在，不能证明语义正确。三条（import + 调用点 + 测试）把「装样子」的成本抬到接近真接入，同时保留 FR-012 的豁免出口给合理的例外 | 只查 import：一个 `diagnostics.NewID()` 就能骗过；查全部调用语义：静态分析做不到，且会误伤把审计集中在 handler 层的正当设计 |
| 新增一个豁免配置文件 | 没有豁免出口时，唯一绕过办法是删检查——那比没有检查更糟。主任务已确认位置，并要求豁免必带**到期日期**：无期限的豁免等同于永久豁免，与删掉检查效果相同 | 复用 `content-boundaries.json` 的 `adapters`：更少文件，但要改 ARCH-01/02 的交付物 |
