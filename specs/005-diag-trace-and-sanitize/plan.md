# Implementation Plan: 诊断 HTTP 追踪贯通与请求脱敏

**Branch**: `005-diag-trace-and-sanitize` | **Date**: 2026-09-14 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `/specs/005-diag-trace-and-sanitize/spec.md`

## Summary

三处闭合，一条主线：**让 HTTP 边界的 `context.Context` 携带正确的 span context**，其余各段自动成立。

1. **传播**：新增全局中间件为每个 API 请求建立本实例 trace；由于队列与 WS 两段已经用 `Pack(ctx,…)` 从 ctx 注入 carrier，边界一修好，`请求 → 队列 → daemon` 的连续性无需改动那两段即成立。父级采信的判定放在认证之后（`X-Actor-Source` 由认证中间件写入，客户端值被剥离），只有 daemon 的 machine credential 路径能把入站 `traceparent` 提升为父级。
2. **脱敏**：把「请求头准入」与「路径/URL 安全形状」从副作用变成两条具名、可复用的规则，放在既有 `Sanitize()` 同一处，供全部 sink 与导出共用。
3. **派发接口**：`事务内登记 + 提交后派发`，进程内实现、不落库、不新增迁移；签名按可被持久实现替换设计。

记录范围不变：只有 `/api/content-diagnostics` 路由组写技术事件（clarify FR-015）。

## Technical Context

**Language/Version**: Go 1.26（server，`go.mod` 要求；容器用 `GOTOOLCHAIN=auto`）；TypeScript strict（`packages/core`）

**Primary Dependencies**: `go.opentelemetry.io/otel/trace` + `propagation`（既有，`contract.go` 已用）、chi 中间件栈（既有）。TS 侧 zod + `parseWithFallback`（既有）。**无新增依赖**

**Storage**: 无 schema 变更、无迁移。技术事件与审计以 `payload` JSONB 列整体存储，`Event` 增字段不触及列定义

**Testing**: Go 用 `testutil.Call(h, req).Want(status).JSON(&out)` 与 `dbfx`；纯函数与中间件用表驱动 `go test`；TS 契约用 `// @vitest-environment node`。**不写 UI 单测**

**Target Platform**: Go 服务端为主；`packages/core` 仅随线上字段更新 schema

**Project Type**: Existing monorepo (Go backend + Next.js web + Electron desktop + Expo mobile + shared packages). Do not re-derive this.

**Performance Goals**: 全局中间件对每请求增加的工作限于一次 header 读取与一次 span context 构造，无 I/O、无锁竞争、无分配放大

**Constraints**: 传播全局、记录模块内；入站 `traceparent` 默认不采信；不新增表与迁移；不改 `Pack`/`Unpack` 线上形状；不改既有 `Sanitize` 已通过的字段级规则

**Scale/Scope**: server 中间件 1 新文件 + 1 测试；handler 2 文件；diagnostics 包 3 文件 + 测试；daemon 客户端 1 文件；core 2 文件；文档 1 份。预计手写文件约 11 个

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| 原则 | 适用 | 判定 |
|---|---|---|
| I. CLAUDE.md 权威 | 是 | 通过：后端约定（`gofmt`/`go vet`/检查错误）、测试落点、UUID 规则均按 `CLAUDE.md` |
| II. 无 UI 单测 / 无自动 UI 验收 | 是 | 通过：本功能全部落在服务端与 core 契约层；无页面改动，不新增 UI 单测。若实施中被迫改页面，转手动清单 |
| III. 模块边界 | 是 | 通过：新规则留在 `server/internal/content/diagnostics`（content 根内）；中间件在 `server/internal/middleware`（非 content 根，属既有适配层，需确认是否要进 `content-boundaries.json` 的 `adapters`——见 research D5） |
| IV. 状态分离 | 是 | 通过：无前端状态改动；新增字段经 Query 缓存既有路径 |
| V. 数据库 | 是 | 通过：**无 schema 变更、无迁移**。clarify FR-017 明确不落库，正是为了不引入新表 |
| VI. API 解析 | 是 | 通过：`Event` 增字段 → `packages/core/content/diagnostics/contract.ts` 同步加可选字段并经 `parseWithFallback`；新增畸形响应测试 |
| VII. UI 复用 | 否 | N/A：无页面改动 |
| VIII. 范围 | 是 | 通过：只做 `§4.3` 三行；其余 12 条缺口在 spec 中显式列为不在范围 |
| IX. 执行器禁用 | 是 | 通过：不触碰执行器路径；`daemon/client.go` 只加出站 header，不改调用语义 |
| X. 勾选 ≠ 验收 | 是 | 通过：交付只声称闭合这三行，不声称任何 D13-V 整体通过（FR-014） |

**Stop Conditions**：无触发。→ 进入 Phase 0。

**Post-design re-check**：通过。两处需要记录的风险已写入 Complexity Tracking：全局中间件触及所有路由（clarify 明确要求），以及 outbox 接口不落库带来的进程退出丢失（FR-010a 已把它写成验收项而非隐含假设）。

## Project Structure

### Documentation (this feature)

```text
specs/005-diag-trace-and-sanitize/
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/
│   ├── trace-propagation.md
│   ├── request-sanitization.md
│   └── transactional-dispatch.md
├── checklists/requirements.md
└── tasks.md
```

### Source Code (repository root)

```text
server/internal/middleware/trace.go                       # 新增：全局传播中间件（建 trace、校验入站值、暂存候选父级）
server/internal/middleware/trace_test.go                  # 新增：表驱动，覆盖建 trace / 不采信 / 非法值 / 不改响应
server/internal/middleware/daemon_auth.go                 # 认证后提升候选父级的挂点（只加一处调用）
server/cmd/server/router.go                               # 全局栈挂 trace 传播；记录范围不变

server/internal/handler/content_diagnostics.go            # DiagnosticTrace 改为沿用 ctx 中的 span，不再自行 Child 新开；写入请求身份
server/internal/handler/content_diagnostics_test.go       # + 连续性、来源区分、记录范围不变、请求身份形状用例

server/internal/content/diagnostics/contract.go            # Event 增请求身份与关联属性字段（可选，omitempty）
server/internal/content/diagnostics/log.go                 # 新增请求头准入规则与路径/URL 安全形状规则，接入 Sanitize
server/internal/content/diagnostics/log_regression_test.go # + 请求头 / 路径 / 查询串负例
server/internal/content/diagnostics/dispatch.go            # 新增：事务内登记 + 提交后派发接口（进程内实现）
server/internal/content/diagnostics/dispatch_test.go       # 新增：回滚不派发 / 提交派发 / 失败可见 / 幂等键 / 有界

server/internal/daemon/client.go                           # 出站请求注入 traceparent

packages/core/content/diagnostics/contract.ts              # eventSchema 增可选字段，经 parseWithFallback
packages/core/content/diagnostics/contract.test.ts         # + 新字段缺省与畸形响应兜底

docs/development/diagnostics-acceptance-mapping.md         # §4.3 三行的闭合证据回写（FR-014）
```

**Structure Decision**: 不新建目录。传播放 `middleware/`（它是 HTTP 适配层，content 根之外），规则与派发接口放 `content/diagnostics/`（模块自有能力），两者通过 `context.Context` 相接，不产生反向依赖。

## Complexity Tracking

| 违反项 | 为什么需要 | 被拒绝的更简方案 |
|---|---|---|
| 全局中间件触及所有 API 路由 | clarify FR-015 明确要求传播全局；只挂诊断路由组则业务链路仍无法追踪 | 只挂 `/api/content-diagnostics`：范围更小，但主任务已判定不满足需要 |
| 父级采信分两段（全局建 trace + 认证后提升） | 全局栈在认证之前运行，此处拿不到 `X-Actor-Source`，而 FR-016 要求判定基于已完成的认证结果 | 单个全局中间件直接读请求头判断来源：会让客户端自称身份决定信任，正是 FR-016 禁止的 |
| 派发接口不落库 | clarify FR-017 明确要求先定接口、不引入新表与迁移 | 持久 outbox：更可靠，但超出本功能范围，已列为后续任务；限制由 FR-010a 显式承担 |
