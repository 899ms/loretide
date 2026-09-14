# Implementation Plan: 补齐「代码存在但无测试」的定向证据

**Branch**: `claude/spec-010-diag-evidence-gaps` | **Date**: 2026-09-15 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `/specs/010-diag-evidence-gaps/spec.md`

## Summary

对照表 §2 里「代码存在但无测试」的非界面行，一行一行处理。**先核实，再补测试**——逐行核实已经查明点名的 8 条里有 2 条已有测试、1 条写测试关不掉，所以交付物是：

1. **对照表更正 3 行**：2 行只改引用与类型（已有测试）、1 行改写措辞并单列缺失功能（DIAG-03）；
2. **新增 6 条定向测试**：迁移约束、回写 trace、队列统计、样本不足、读路径只读、不自动上传；
3. **1 条后续任务**：「按级别过滤写入」这个**功能**缺口。

**不改任何生产代码**（SC-008 用 `git diff` 行数为 0 钉住）。**不为任何界面行补测试**（SC-007）。

## Technical Context

**Language/Version**: Go 1.26.6；Node 22（`node:test`）。**无生产代码改动**

**Primary Dependencies**: **无新增**。Go 侧用标准库与既有夹具；Node 侧只用 `node:fs` / `node:path`

**Storage**: 无 schema 变更、**无新增迁移**。两条 DB 背书用例读写测试 schema，沿用 `testStore` 的 per-test schema 与清理

**Testing**: Go 测试（其中 2 条需真实事务，门槛 `LORETIDE_DIAG_TEST_DATABASE_URL`）+ 1 个 `node:test` 静态检查。**不写 UI 单测**

**Target Platform**: 服务端与仓库脚本

**Project Type**: Existing monorepo (Go backend + Next.js web + Electron desktop + Expo mobile + shared packages). Do not re-derive this.

**Performance Goals**: 迁移扫描只读 `468` 及之后含 `content_` 的文件（当前 12 个），静态检查只读 4 个文件——两者都在毫秒量级

**Constraints**: 不改生产代码；不改 CI 触发条件；不新增迁移；不碰执行闸门；不新增 UI 单测；静态检查必须显式列文件并排除 `packages/core/api/client.ts`

**Scale/Scope**: 新增 3 个测试文件 + 1 个脚本 + 1 个脚本测试；改动对照表与 `package.json`；预计手写文件 6 个

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| 原则 | 适用 | 判定 |
|---|---|---|
| I. CLAUDE.md 权威 | 是 | 通过：测试落点按 `CLAUDE.md` → Testing（后端进 `server/` Go 测试），脚本与既有检查器并列 |
| II. 无 UI 单测 / 无自动 UI 验收 | **是，且是重点** | 通过：界面行一条不补，SC-007 用「新增 UI 单测数为 0」与「views 下测试文件数不变」双重钉住。**US3 整条就是为这条原则设的** |
| III. 模块边界 | 是 | 通过：Go 测试在被测包内；静态检查在 `scripts/`，不属任何 content 根；不产生跨根 import |
| IV. 状态分离 | 否 | N/A：无前端状态改动 |
| V. 数据库 | 是 | 通过：**无新增迁移、无 schema 变更**；新增的迁移约束测试本身就是在加强原则 V 的执行 |
| VI. API 解析 | 否 | N/A：不改端点、不改响应形状 |
| VII. UI 复用 | 否 | N/A：无页面改动 |
| VIII. 范围 | **是，且需要留意** | 通过：DIAG-03 的缺失功能**只登记不实现**；两条未点名的非界面行显式排除；发现必须改生产代码才能测时**停下来报告**（FR-019） |
| IX. 执行器禁用 | 是 | 通过：不触碰 `pkg/executionpolicy` |
| X. 勾选 ≠ 验收 | 是 | 通过：对照表只按实际运行结果回写；跳过记未执行 |

**Stop Conditions**：无触发。→ 进入 Phase 0。

**Post-design re-check**：通过。三处张力见 Complexity Tracking。

## Project Structure

### Documentation (this feature)

```text
specs/010-diag-evidence-gaps/
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/
│   ├── migration-constraints.md   # 迁移约束测试的范围、规则与失败信息
│   └── no-upload-check.md         # 静态检查的文件清单、原语、正负例
├── checklists/requirements.md
└── tasks.md
```

### Source Code (repository root)

```text
server/internal/migrations/content_constraints_test.go          # 新增：迁移约束（无库）
server/internal/content/diagnostics/overview_metrics_test.go    # 新增：QueueWait 与 P95 阈值（无库）
server/internal/content/diagnostics/readpath_test.go            # 新增：回写 trace + 读路径只读（DB 背书）
scripts/check-diagnostics-no-upload.mjs                         # 新增：导出路径无外发请求的静态检查
scripts/check-diagnostics-no-upload.test.mjs                    # 新增：node:test，正例 + 负例夹具
package.json                                                    # + check:diagnostics-no-upload 入口
docs/development/diagnostics-acceptance-mapping.md              # 3 行更正 + §4.1 计数 + 后续任务登记
```

**Structure Decision**: 不新建目录、不新建包。迁移约束测试与既有两条迁移 lint 同包，CI 那条 `-run` 过滤加一个名字即可、**触发条件不动**；两条诊断测试进被测包；静态检查与 `check-diagnostics-contract.mjs` 并列放 `scripts/`。

## Complexity Tracking

| 张力 | 为什么接受 | 被拒绝的更简方案 |
|---|---|---|
| **交付物与任务描述点名的清单不一致**（8 条点名 → 2 条只改引用、1 条改措辞、6 条新测试，其中 1 条不在点名清单外但由点名行派生） | 逐行核实本来就是任务要求的一步，它的结论就该改变交付物。照着点名清单写 8 条测试，会产出 2 条重复用例和 1 条测不到目标的用例——那不是更忠实，是更省事 | 照单全收写 8 条：省掉核实的判断成本，代价是把重复与无效写进仓库 |
| **DIAG-03 记成「已通过 + 备注缺失功能」而不是「无测试」** | 它今天交付的两件事确实有测试；卡片要求的第三件事没有功能。写成「无测试」会让下一个人去写一条写不出的测试；写成「已通过」而不带备注则会让计数掩盖缺口。两者都要写才是实话 | 继续记「代码存在但无测试」：看起来保守，实际误导 |
| **静态检查用显式文件清单而非目录通配** | 主任务约束，且理由成立：将来该目录加入一个正当需要 `fetch` 的文件时，通配会逼作者去改检查，而清单让「范围变了」在 diff 里看得见。代价是链路拆文件时要手动同步，由 FR-012「清单里任一文件不存在即失败」兜住 | 目录通配：少维护一份清单，多一类会静默失效或误伤的情形 |
