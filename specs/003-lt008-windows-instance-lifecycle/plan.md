# Implementation Plan: Windows 实例生命周期入口的可核验性

**Branch**: `003-lt008-windows-instance-lifecycle` | **Date**: 2026-09-14 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `/specs/003-lt008-windows-instance-lifecycle/spec.md`

## Summary

在现有 `scripts/local-windows.ps1` 上扩展：`status` 逐组件报告状态 / pid / 归属 checkout / 构建标识 / 健康结果并支持 `-Json`；`start` 做前置检查（`api.exe`、`secrets.json`、三个端口的非本实例占用）并清理陈旧 pid；`stop` 等待进程退出并按超时报告；文档写明单实例。不改监督进程的执行策略，不做多实例隔离（clarify 已定）。

## Technical Context

**Language/Version**: PowerShell 7+（`pwsh`）；Node.js（监督进程，不改逻辑）

**Primary Dependencies**: `pg_ctl`（`F:\loretide-runtime\pgsql\bin`）、`Get-CimInstance Win32_Process`、`Get-NetTCPConnection`、`Invoke-WebRequest`、`git rev-parse`。无新增

**Storage**: `data/windows/`（pid、日志、`stop` 信号、`secrets.json`、新增 `build.txt`）；已被 `.gitignore` 的 `data/` 排除

**Testing**: 新增 `scripts/local-windows.test.ps1`（与 Linux 侧 `scripts/dev-env.test.sh` 同类：对脚本的退出码与输出做非 UI 检查）；其余按手动清单

**Target Platform**: Windows 11，本机单实例

**Project Type**: Existing monorepo (Go backend + Next.js web + Electron desktop + Expo mobile + shared packages). Do not re-derive this.

**Performance Goals**: `status` 在实例未运行时 ≤ 3 秒返回（不等待 HTTP 超时）；`stop` 默认等待上限 30 秒

**Constraints**: 不输出密钥；`LORETIDE_EXECUTION_POLICY=disabled` 保持；不强杀进程；不改端口分配（单实例）

**Scale/Scope**: 脚本 1 个（约 25 → 120 行）；测试脚本 1 个（新）；文档 1 个

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| 原则 | 适用 | 判定 |
|---|---|---|
| I. CLAUDE.md 权威 | 是 | 通过：`native-windows.md` 更新，`CLAUDE.md` 不改 |
| II. 无 UI 单测 / 无自动 UI 验收 | 是 | 通过：测试仅对脚本退出码与输出；浏览器可达由用户手验 |
| III. 模块边界 | 否 | N/A（脚本，不触碰 packages/server 代码） |
| IV. 状态分离 | 否 | N/A |
| V. 数据库 | 否 | N/A（不改 schema；`stop` 不动 PostgreSQL） |
| VI. API 解析 | 否 | N/A |
| VII. UI 复用 | 否 | N/A |
| VIII. 范围 | 是 | 通过：单实例；不重写监督进程；隔离另立 |
| IX. 执行器禁用 | 是 | 通过：FR-009 明确保持 disabled |
| X. 勾选 ≠ 验收 | 是 | 通过：手动项由用户确认 |

**Stop Conditions**：无触发。→ 进入 Phase 0。

**Post-design re-check**：通过。新增文件 `scripts/local-windows.test.ps1` 与 Linux 侧 `dev-env.test.sh` 同类，不是新测试类型。

## Project Structure

### Documentation (this feature)

```text
specs/003-lt008-windows-instance-lifecycle/
├── plan.md
├── research.md
├── quickstart.md
├── contracts/status-json.md
├── checklists/requirements.md
└── tasks.md
```

（无 data-model.md：无数据实体；状态文件形状见 contracts/status-json.md。）

### Source Code (repository root)

```text
scripts/local-windows.ps1              # status 扩展 + -Json；start 前置检查与陈旧 pid 清理；stop 等待
scripts/local-windows-supervisor.mjs   # 不改逻辑；仅在启动时读取 build.txt 注入 LORETIDE_BUILD（可选，见 research D3）
scripts/local-windows.test.ps1         # 新增：退出码 / 输出契约检查（不启动真实服务时用桩）
docs/development/native-windows.md     # 行为说明 + 单实例限制 + 更新记录
```

**Structure Decision**: 单脚本内扩展，用函数拆分 `Get-InstanceStatus`、`Test-StartPreconditions`、`Wait-InstanceStop`；不引入模块文件。

## Complexity Tracking

无违反项。
