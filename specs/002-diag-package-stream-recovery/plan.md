# Implementation Plan: 诊断包下载保真与实时流断线恢复

**Branch**: `002-diag-package-stream-recovery` | **Date**: 2026-09-14 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `/specs/002-diag-package-stream-recovery/spec.md`

## Summary

四处闭合：(1) 服务端流在 25 秒计划结束的最后一页携带 `rotate: true`，客户端据此静默续连、携带全部过滤条件与游标、真实断线才退避重连、授权失败停止重连；(2) 下载改走 `fetchRaw` 取原始响应字节与 `Content-Disposition` 文件名，不再经 zod 转换后重序列化；(3) 缺口提示保留、200 条上限标示；(4) 契约 / Go handler / 纯函数层测试补齐，并交付 D13-V05 / V08 / V11 手动验收清单。只做 Web，不动存储、模拟器与面板布局。

## Technical Context

**Language/Version**: Go 1.26（server）；TypeScript strict（packages/core、packages/views、apps/web）

**Primary Dependencies**: chi、`net/http` flusher（既有）；TanStack Query、zod、`parseWithFallback`（既有）。无新增依赖

**Storage**: 既有诊断表；本功能不改 schema、不加迁移

**Testing**: Go：`testutil.Call(h, req).Want(status).JSON(&out)`；TS：`packages/core/content/diagnostics/*.test.ts(x)`（纯函数与契约用 `// @vitest-environment node`）。不写 UI 单测

**Target Platform**: Web（Next.js）。桌面端未挂载诊断页，不在范围

**Project Type**: Existing monorepo (Go backend + Next.js web + Electron desktop + Expo mobile + shared packages). Do not re-derive this.

**Performance Goals**: 计划轮换的续连间隙 ≤ 1 秒且无可见状态变化；真实断线退避 2s→30s

**Constraints**: 25 秒轮换与每批成员资格重查保留；`views` 不引入平台 API；下载不自动上传；不改 `Scope.Accounts` 语义

**Scale/Scope**: 后端 2 个文件；core 3 个文件；views 1 个文件；web platform 1 个文件；测试 2～3 个文件；1 份清单

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| 原则 | 适用 | 判定 |
|---|---|---|
| I. CLAUDE.md 权威 | 是 | 通过 |
| II. 无 UI 单测 / 无自动 UI 验收 | 是 | 通过：流状态机与合并逻辑抽为纯函数在 core 测；页面行为进 manual-ui-todo.md |
| III. 模块边界 | 是 | 通过：下载仍经 `download` prop 注入；`views/content/diagnostics` 不 import 平台 API（FR-012）；检查器会验证 |
| IV. 状态分离 | 是 | 通过：事件仍写入 Query cache；流状态（连接态、gap、过滤）属客户端视图状态，留在 hook 内部 `useState`/`useRef`；过滤条件由诊断页既有的本地 state 持有并传入 hook（既有模式，本功能不新增状态；`CLAUDE.md` 要求 filters 归 Zustand，迁移另立任务） |
| V. 数据库 | 否 | N/A（无 schema 变更） |
| VI. API 解析 | 是 | 通过：`pageSchema` 新增 `rotate` 为可选默认 false，经 `parseWithFallback`；新增畸形响应测试 |
| VII. UI 复用 | 是 | 通过：提示与标示复用现有诊断页组件与 token，不新建控件 |
| VIII. 范围 | 是 | 通过：不动存储 / 模拟器 / 布局；桌面端另立 |
| IX. 执行器禁用 | 是 | 通过：不触碰 |
| X. 勾选 ≠ 验收 | 是 | 通过：manual-ui-todo.md 初始为待用户验证 |

**Stop Conditions**：无触发。→ 进入 Phase 0。

**Post-design re-check**：通过。新增字段向后兼容（旧客户端忽略 `rotate`，旧服务端不发则客户端按真实断线处理——行为与现状相同，不更差）。

## Project Structure

### Documentation (this feature)

```text
specs/002-diag-package-stream-recovery/
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── manual-ui-todo.md            # 实施时生成（FR-009）
├── contracts/
│   ├── stream-page.md
│   └── export-download.md
├── checklists/requirements.md
└── tasks.md
```

### Source Code (repository root)

```text
server/internal/content/diagnostics/store.go          # Page 结构增加 Rotate（轮换收尾标记）
server/internal/handler/content_diagnostics.go        # Stream: deadline 分支发最后一页并置 rotate=true
server/internal/handler/content_diagnostics_test.go   # + 轮换页、过滤透传、拒绝路径用例

packages/core/api/client.ts                           # + contentDiagnosticDownload(query, signal): Promise<Response>（fetchRaw）
packages/core/content/diagnostics/contract.ts         # pageSchema + rotate；导出 STREAM_EVENT_CAP=200；filter→query 序列化纯函数；Content-Disposition 文件名解析纯函数
packages/core/content/diagnostics/stream-state.ts     # 新增：nextStreamState / computeBackoff 纯函数状态机（research D2）
packages/core/content/diagnostics/stream-state.test.ts # 新增：// @vitest-environment node，覆盖全部转移
packages/core/content/diagnostics/queries.ts          # useDiagnosticStream(wsId, enabled, filter)：调用状态机；副作用（fetch / 定时器 / visibilitychange）；download 改为 fetchRaw → {blob, filename}
packages/core/content/diagnostics/contract.test.ts    # + rotate 解析、畸形响应、文件名解析、过滤序列化

packages/views/content/diagnostics/index.tsx          # download prop 签名改为 ({blob, filename})；gap 提示保留；「仅显示最近 200 条」标示
apps/web/platform/content-diagnostics.ts              # downloadDiagnosticBundle({blob, filename})：直接保存 Blob，用服务端文件名
apps/web/platform/content-diagnostics.test.ts         # 随 download 签名变化的平台接入测试（非 UI 单测）
```

**Structure Decision**: 全部在既有文件内改；状态机与解析逻辑抽为 `contract.ts` 的纯函数以便在 node 环境测试（constitution II）。不新建目录。

## Complexity Tracking

无违反项。
