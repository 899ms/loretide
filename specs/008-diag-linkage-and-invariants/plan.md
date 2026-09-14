# Implementation Plan: 诊断关联跳转与不变量断言

**Branch**: `claude/spec-008-diag-linkage-invariants` | **Date**: 2026-09-14 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/008-diag-linkage-and-invariants/spec.md`

## Summary

闭合对照表 §5 的六项实现缺口。经代码核实，六项的真实形态与 §5 的描述并不一致（见 spec.md「规格修正」），因此本计划的重点是**补上每项真正缺的那一半**，而不是从零实现：

- **G1 / G2（跳转）**：跳转已在面板里以内联 setter 的形式存在。把「从一条事件推导出去哪里看」抽成 `packages/core/` 的纯函数，使其可在 node 环境下断言（原则 II 下界面本身不可测），顺带修掉「追踪编号为空时静默跳到全量日志」这个会误导读者的缺陷。G2 按 Q2-A 只在**已取回的事件**范围内归拢对象版本，**不新增任何数据读取路径**。
- **G3 / G4 / G5 / G6（断言）**：纯测试工作，不改生产代码。分别补齐下一动作映射的**穷尽性**、快照**无媒体载荷**的字段清单断言、复现**不回写偏好**的具名断言、以及一条**从真实运行出发**逐跳走通的跨层链条断言。

四项 clarify 全部裁决为 A，正文无需回改。

## Technical Context

**Language/Version**: Go 1.26（server）；TypeScript 5 strict（packages）

**Primary Dependencies**: 现有依赖，**不新增**。Go 侧 `pgx`、标准库 `testing`/`reflect`；TS 侧 `zod`、`vitest`

**Storage**: PostgreSQL（现有 `content_diagnostic_*` 表）。**无迁移、无 schema 变更**

**Testing**: Go `testing`（含 `internal/testutil` 的 `dbfx` / `testutil.Call`）；`vitest`，纯函数一律 `// @vitest-environment node`

**Target Platform**: Linux 服务端 + Web（桌面端未挂载诊断页）

**Project Type**: Existing monorepo (Go backend + Next.js web + Electron desktop + Expo mobile + shared packages). Do not re-derive this.

**Performance Goals**: 无新指标。两个新纯函数在一页事件（上限 `STREAM_EVENT_CAP` = 200）上求值，与既有 `buildTraceWaterfall` 同数量级

**Constraints**:
- **不新增数据读取路径**（FR-019、Q2-A）。现有事件查询只支持追踪 / 组件 / 严重级 / 错误码 / 运行 / 时间 / 游标七个维度，本特性只用这些
- **不新增 UI 单测**（原则 II、FR-020）。界面行为进 `manual-ui-todo.md`
- **不改生产代码**，G3 ～ G6 四项；G1 / G2 的生产改动限于「把既有逻辑挪进纯函数 + 修一处缺陷」

**Scale/Scope**: 2 个新纯函数、1 个新 TS 测试文件、4 个新 Go 测试文件、1 处面板改写、4 语言各 ~6 个新文案键、1 份手动清单

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| 原则 | 本计划如何满足 | 判定 |
|---|---|---|
| **I. CLAUDE.md 权威** | 遵循 `CLAUDE.md` 的测试分层（纯函数进 `.test.ts` 并标 node 环境）、包边界、locales 四语言同时加 | PASS |
| **II. 无 UI 单测、无自动浏览器验收** | 这正是 G1 存在的原因：跳转推导挪到 `core` 才能被测。面板本身不测，界面条目全部进 `manual-ui-todo.md` 标「待用户验证」 | PASS |
| **III. 模块边界** | 新纯函数放 `packages/core/content/diagnostics/`，不引 `react-dom` / `localStorage` / `process.env` / UI 库；`views` 只消费，方向仍是 `views -> core` | PASS |
| **IV. 服务端/客户端状态分离** | 不新增查询、不新增 store。跳转推导是**纯函数**，产出的筛选条件由面板既有的 `useState` 承接，不引入新的状态所有权 | PASS |
| **V. 数据库无外键无级联** | 无迁移、无 schema 变更、无新索引 | PASS（不适用） |
| **VI. API 响应解析不强转** | 不新增端点、不改 schema。G2 消费的是**已经过 `eventSchema` 解析**的事件 | PASS |
| **VII. UI 复用 Multica** | 面板改动限于调用纯函数并按结果禁用按钮 / 显示说明，复用既有 `Button` 的 `disabled` 与既有排版，不新建控件。动手前读 `docs/development/design/README.md` | PASS（见下方风险） |
| **VIII. 范围即所领任务** | 严格限于 §5 六项。实施中发现的相邻问题（见 research.md R4「复现不还原原始输入」）记为后续，不在本特性修 | PASS |
| **IX. 真实执行器保持禁用** | 全部测试走模拟器，不解析也不执行任何 agent CLI，不触碰凭据 | PASS |
| **X. 勾选不等于验收** | `tasks.md` 的勾选只表示交付；手动条目一律「待用户验证」；对照表状态改写须以**实跑用例**为依据 | PASS |

**Stop Conditions 检查**：constitution 可读且无占位符；权威源可读；文件范围已在下方 Source Code 清单中界定；无需靠弱化规则来过闸。**无停止条件触发。**

**风险（非违规）**：原则 VII 要求改 Web 页面前读设计文档。G1 的界面改动会新增一个「不可跳转」的呈现，须复用既有的禁用态与说明文案样式，不得自造控件；这一点在 `tasks.md` 中作为显式前置步骤。

## Project Structure

### Documentation (this feature)

```text
specs/008-diag-linkage-and-invariants/
├── plan.md              # 本文件
├── spec.md
├── research.md          # Phase 0
├── data-model.md        # Phase 1
├── quickstart.md        # Phase 1
├── contracts/
│   ├── trace-jump.md
│   └── next-action.md
├── checklists/requirements.md
├── manual-ui-todo.md    # 浏览器条目（原则 II）
└── tasks.md             # /speckit-tasks 产出，本命令不创建
```

### Source Code (repository root)

**实施 PR 的改动必须限于下列清单。** 清单外的文件一律不改。

```text
packages/core/content/diagnostics/
├── linkage.ts              # 新增：describeTraceJump / describeObjectVersions（G1 / G2）
└── linkage.test.ts         # 新增：node 环境，二者的规范层（G1 / G2）

packages/views/content/diagnostics/
└── index.tsx               # 修改：onTrace 改为消费 describeTraceJump；新增对象版本呈现与不可跳转态

packages/views/locales/{en,zh-Hans,ja,ko}/common.json
                            # 修改：diagnostics 段新增文案键，四语言同时加

server/internal/content/diagnostics/
├── next_action_test.go        # 新增：下一动作穷尽性与封闭性（G3）
├── snapshot_media_test.go     # 新增：快照字段清单、无媒体载荷（G4）
└── reproduce_preference_test.go  # 新增：复现不回写偏好（G5）

server/internal/handler/
└── content_diagnostics_linkage_test.go  # 新增：跨层链条端到端（G6）

docs/development/
└── diagnostics-acceptance-mapping.md    # 修改：§5 三处描述修正 + 各行状态刷新
```

**明确不动**：`server/migrations/`（无迁移）、`server/internal/content/diagnostics/` 的任何**生产** `.go` 文件（G3 ～ G6 是纯测试）、`scripts/`、CI 工作流、`packages/ui/`、桌面与移动端。

**Structure Decision**：沿用既有 `content/diagnostics` 模块的分层——派生逻辑在 `packages/core/content/diagnostics/`（与 `trace-waterfall.ts`、`regression.ts` 并列，同样是「因为原则 II 禁止 UI 单测，所以凡需要测试证明的逻辑必须放在测试够得着的地方」），界面在 `packages/views/content/diagnostics/`，后端不变量断言就地放在被断言代码所在的包内。**不新建任何目录层级。**

## Complexity Tracking

> 无 Constitution Check 违规，本节为空。

一处需要记录的**取舍**（非违规）：G2 按 Q2-A 只在已取回的事件范围内归拢对象版本，因此**跨页的同对象事件看不全**。更完整的做法是给服务端加对象维度筛选，但那会新增一条数据读取路径，被 FR-019 与 Q2 裁决排除。该限制已写入 spec.md 的 Assumptions，并作为 O-3 条目进入 `manual-ui-todo.md`，使它可见而不是被悄悄吞掉。
