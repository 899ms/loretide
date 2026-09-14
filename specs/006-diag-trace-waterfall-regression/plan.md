# Implementation Plan: 诊断 trace 瀑布视图与回归关联

**Branch**: `006-diag-trace-waterfall-regression` | **Date**: 2026-09-14 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `/specs/006-diag-trace-waterfall-regression/spec.md`

## Summary

四个纯函数进 `packages/core/content/diagnostics/`，一个视图改写在 `packages/views/content/diagnostics/index.tsx`，一份手动 UI 清单。纯函数负责层级构造、时间轴计算、四态判定与四项定位；视图只消费它们的输出。Clarifications 已定：不动任何 Go 代码，不新增路由，不做概览聚合，上限 200 且按错误码保留祖先链。

## Technical Context

**Language/Version**: TypeScript 5.x（strict）；React 19（views 层）；Node 22（测试运行时）

**Primary Dependencies**: 无新增。复用 `zod`（已有 schema）、`vitest`（已有）、`@multica/ui` 的 `Progress` / `Table` / `Badge` 等既有组件

**Storage**: N/A。本功能不读写数据库，不新增迁移，不改 `Event` / `Run` 字段集合（FR-010）

**Testing**: `vitest`，全部为 `// @vitest-environment node` 的纯函数测试，置于 `packages/core/content/diagnostics/*.test.ts`。UI 单测 0 个（FR-011、constitution 原则 II）

**Target Platform**: Web（`apps/web` 的 `/{workspaceSlug}/diagnostics`）。桌面端未挂载诊断页，不在范围内

**Project Type**: Existing monorepo (Go backend + Next.js web + Electron desktop + Expo mobile + shared packages). Do not re-derive this.

**Performance Goals**: 层级构造对 200 个 span 在单次渲染前完成，不引入可感知延迟；构造复杂度 O(n)，不得出现按父链重复遍历的 O(n²)

**Constraints**: 不改 Go 代码；不新增路由；不加概览指标；不写 UI 单测；输出不得包含密钥（沿用既有 `safe_message` 口径）

**Scale/Scope**: 4 个纯函数 + 1 个视图文件改写 + 2 个测试文件 + 1 份手动清单 + 少量 i18n 文案键

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| 原则 | 适用 | 判定 |
|---|---|---|
| I. CLAUDE.md 权威 | 是 | 通过：遵循 Package Boundaries 与 Testing 两节；`CLAUDE.md` 不改 |
| II. 无 UI 单测 / 无自动 UI 验收 | 是 | 通过：判定与构造逻辑全部下沉为 node 环境纯函数；页面行为进 `manual-ui-todo.md`，未执行记「待用户验证」。这是本功能的主要设计约束，见 Structure Decision |
| III. 模块边界 | 是 | 通过：纯函数落在 `packages/core/content/diagnostics/`，只被 `packages/views/` 消费；方向为 `views -> core`，不反向。`core` 不引入 `react-dom` / `localStorage` / `process.env` / UI 库 |
| IV. 状态分离 | 是 | 通过：瀑布与判定是**派生视图**，不是服务器状态也不是持久客户端状态。折叠/展开是短暂 UI 状态，留在组件内，不进 Zustand，不持久化 |
| V. 数据库 | 否 | N/A（FR-010 明确不碰） |
| VI. API 解析 | 是 | 通过：不新增端点。纯函数消费的是**已经过 `runSchema` / `eventSchema` 解析后**的对象，不直接吃网络 JSON；未知 `regression` 取值按 FR-005 落到「无法判定」，等价于枚举 switch 的 `default` 分支 |
| VII. UI 复用 | 是 | 通过：复用既有 `Progress` / `Table` / `Badge` / `SettingsCard`，用语义 token 与 `--text-*` 字号；不手搓新控件。改动前读 `docs/development/design/README.md` |
| VIII. 范围 | 是 | 通过：只做瀑布层级 + 时间轴 + 四态判定 + 四项定位。对照表列出的其余缺口（DIAG-05 HTTP trace、DIAG-04 outbox、DIAG-02 请求头脱敏、DIAG-03 磁盘满）明确不做；概览聚合按 Q5 排除 |
| IX. 执行器禁用 | 是 | 通过：不触碰执行策略，不新增任何执行路径 |
| X. 勾选 ≠ 验收 | 是 | 通过：本功能只交付实现与手动清单；DIAG-09 / D13-V09 的勾选与对照表更新由主任务在文档仓库进行 |

**Stop Conditions**：constitution 可读；权威来源可读；文件范围明确（见 Source Code）；无需削弱任何规则。→ 可进入 Phase 0。

**Post-design re-check（Phase 1 后）**：通过。设计未引入新依赖、新测试类型或新路由。唯一需要留意的是原则 IV——折叠状态刻意留在组件本地而非 store，理由见 research D5。

## Project Structure

### Documentation (this feature)

```text
specs/006-diag-trace-waterfall-regression/
├── spec.md
├── plan.md
├── research.md
├── data-model.md
├── contracts/
│   ├── trace-waterfall.md
│   └── regression-verdict.md
├── quickstart.md
├── manual-ui-todo.md
├── checklists/requirements.md
└── tasks.md              # /speckit-tasks 生成
```

### Source Code (repository root)

```text
packages/core/content/diagnostics/
├── trace-waterfall.ts            # 新增：buildTraceWaterfall（层级 + 时间轴 + 折叠）
├── trace-waterfall.test.ts       # 新增：node 环境纯函数测试
├── regression.ts                 # 新增：describeRegressionVerdict / describeRunLinkage
├── regression.test.ts            # 新增：node 环境纯函数测试
└── index.ts                      # 改：导出新函数与类型
# contract.ts 不改：STREAM_EVENT_CAP=200 已导出并被 mergeEvents 与面板使用，瀑布直接引用

packages/views/content/diagnostics/
└── index.tsx                     # 改：trace 标签页改为层级瀑布；运行列表接四态判定与四项定位跳转

packages/views/locales/*/common.json   # 改：新增 diagnostics 文案键（四态、时钟偏差、折叠、未记录、本身即原故障）
```

**不在改动范围内**：`server/` 全部；任何迁移；`apps/web/` 的路由与平台接线（FR-008 页内切换，无需新增）；`packages/ui/`（复用既有组件，不新增原语）。

**Structure Decision**: 逻辑与呈现分离是本功能的核心结构决定，原因是 constitution 原则 II：UI 不写单测，所以**凡是需要被测试证明正确的东西都必须不在 UI 里**。层级构造、时间轴偏移、折叠选择、四态判定、四项定位全部是确定性的纯数据变换，下沉到 `core` 后可在 node 环境逐条断言；`index.tsx` 只剩「把已算好的行渲染出来」，其正确性交给手动清单。这样既满足原则 II，又不会让「不写 UI 单测」变成「这部分没人验证」。

## Complexity Tracking

无违反项。四个函数分两个文件而非一个，是因为瀑布与回归关联是两条独立的验收线（US1 与 US2/US3），分开后任一条可单独交付与回退。

## Phase 0 输出

见 [research.md](research.md)：D1 层级构造算法与终止性、D2 时间轴基准与时钟偏差、D3 折叠选择策略、D4 四态判定的取值来源、D5 折叠状态的归属、D6 上限常量的复用方式。

## Phase 1 输出

- [data-model.md](data-model.md)：三个派生实体的字段与取值域（均不持久化）
- [contracts/trace-waterfall.md](contracts/trace-waterfall.md)：`buildTraceWaterfall` 的输入输出契约与不变量
- [contracts/regression-verdict.md](contracts/regression-verdict.md)：`describeRegressionVerdict` / `describeRunLinkage` 的契约与四态真值表
- [quickstart.md](quickstart.md)：验证指引（自动部分）
- [manual-ui-todo.md](manual-ui-todo.md)：页面行为逐条，初始全部「待用户验证」
