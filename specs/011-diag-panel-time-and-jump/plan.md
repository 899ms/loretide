# Implementation Plan: 瀑布时间语义、真实时钟偏差与可达的追踪跳转

**Branch**: `claude/spec-011-diag-panel-time-and-jump` | **Date**: 2026-09-15 | **Spec**: [spec.md](./spec.md)

## Summary

三个独立缺陷，全部在 `content/diagnostics` 边界内：

1. **时间语义分歧**（P1）—— 生产者写结束时刻、消费者读开始时刻。改生产者一行的语句顺序，并把语义写进合同。
2. **`clock_skew` 空转**（P2）—— 判定条件与造数据条件互不相交。让场景真的造出乱序时间戳。
3. **跳转落点不可达**（P1）—— 渲染条件 `!runId` 与 `onTrace` 必设 `runId` 互斥，导致对象与版本卡片在该路径上永不出现。放宽条件。

**改动极小、语义极重**：缺陷一是一行语句顺序，缺陷三是一个布尔条件，但两者都会改变读者对界面的解读。因此测试与合同的分量远大于代码的分量。

## Technical Context

**Language/Version**: Go 1.26；TypeScript 5 strict

**Primary Dependencies**: 现有依赖，不新增

**Storage**: PostgreSQL（现有表）。**无迁移、无 schema 变更**

**Testing**: Go `testing`；`vitest`，纯函数一律 `// @vitest-environment node`

**Project Type**: Existing monorepo. Do not re-derive this.

**Constraints**:
- **不改瀑布算法的时间处理**（FR-005）——本特性只改生产者与合同
- **不新增前端数据读取路径**（FR-014）——事件查询已携带 `trace_id`，已核实
- **不碰 `server/internal/daemon/` 与任何上游 Multica 代码**（FR-015）
- **不新增 UI 单测**（FR-016，原则 II）
- 现有 `trace-waterfall.test.ts` 夹具与新语义一致，**不改**

## Constitution Check

| 原则 | 判定 | 说明 |
|---|---|---|
| I. CLAUDE.md 权威 | PASS | 遵循测试分层与包边界 |
| II. 无 UI 单测 | PASS | 缺陷三的渲染条件改动**不写自动测试**，进手动清单；缺陷一二的断言都在纯函数与 Go 层 |
| III. 模块边界 | PASS | 不新增跨包依赖；`linkage.ts` / `trace-waterfall.ts` 不动 |
| IV. 状态分离 | PASS | 缺陷三只改渲染条件，不改查询、不改 store |
| V. 数据库 | PASS（不适用） | 无迁移、无 schema 变更 |
| VI. 响应解析 | PASS | 不改 schema、不改端点 |
| VII. UI 复用 | PASS | 缺陷三**只放宽一个布尔条件**，不新增控件、不改样式 |
| VIII. 范围 | PASS | 严格限于 4 条未通过项及其附带发现；其余不碰 |
| IX. 真实执行器禁用 | PASS | 全部走模拟器 |
| X. 勾选不等于验收 | PASS | 手动条目仍「待用户验证」；需用户复验的条目在 PR 正文单列 |

**无停止条件触发。**

## Project Structure

### Source Code（改动必须限于此清单）

```text
server/internal/content/diagnostics/
├── simulator.go                    # 改：Occurred 步进前盖（缺陷一）；clock_skew 造真实偏移（缺陷二）
└── span_timing_test.go             # 新增：首尾相接 + clock_skew 触发 + 其余场景不触发

packages/core/content/diagnostics/
└── trace-waterfall.test.ts         # 改：新增真实形状的零间隙用例与 clock_skew 用例（夹具本身不改）

packages/views/content/diagnostics/
└── index.tsx                       # 改：放宽 tab===3 的渲染条件（缺陷三）

specs/006-diag-trace-waterfall-regression/contracts/
└── trace-waterfall.md              # 改：钉死 occurred_at 语义

specs/008-diag-linkage-and-invariants/
└── manual-ui-todo.md               # 改：J-1 / J-4 措辞

docs/development/
└── manual-ui-runbook.md            # 改：J-1 / J-4 措辞
```

**明确不动**：`trace-waterfall.ts`（算法）、`linkage.ts`、`server/internal/daemon/`、迁移、CI、locales（无新文案）。

**Structure Decision**：沿用既有分层。时间语义的断言放在**生产者所在的包**（`internal/content/diagnostics`），因为要断言的是「模拟器产出的形状」；瀑布侧的零间隙断言放在 core 的既有测试文件里，与它已有的不变量并列。

## Complexity Tracking

无违规。

一处需要记录的**取舍**：缺陷二让 `clock_skew` 场景产生真实乱序时间戳，这意味着该场景的事件**不再单调递增**。这是有意的——它正是该场景要模拟的现象。缺陷一的「首尾相接」断言因此必须**排除** `clock_skew` 场景，否则两条规则互相矛盾。这一点在合同与测试注释中写明，不留给读代码的人去猜。
