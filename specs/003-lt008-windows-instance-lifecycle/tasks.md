# Tasks: Windows 实例生命周期入口的可核验性

**Input**: Design documents from `/specs/003-lt008-windows-instance-lifecycle/`

**Prerequisites**: plan.md, spec.md, research.md, contracts/status-json.md, quickstart.md

**Tests**: 新增 `scripts/local-windows.test.ps1`（与 Linux 侧 `scripts/dev-env.test.sh` 同类）：对脚本退出码与输出做非 UI 检查，用临时状态目录与桩 `pg_ctl`，不启动真实服务。真实全流程按 quickstart 手动执行。

> **Loretide testing policy**: 无 UI 单测；浏览器可达由用户手验；输出不得含密钥。

## Format: `[ID] [P?] [Story] Description`

## Phase 1: Setup（基线）

- [x] T001 记录基线：运行 `./scripts/local-windows.ps1 status` 与 `Get-Content scripts/local-windows.ps1`，把当前输出与 25 行脚本原文保存到 `specs/003-lt008-windows-instance-lifecycle/baseline.txt`（不入库）（`status` 基线输出未执行：执行环境为 Linux 容器，无 pg_ctl.exe / Win32_Process / F:\loretide-runtime；已改为记录脚本原文与 parse 检查）

## Phase 2: Foundational（阻塞全部故事）

- [x] T002 `scripts/local-windows.ps1`：首行加 `#Requires -Version 7.0`；保持现有四个动作行为不变的前提下重构为函数 `Get-InstanceStatus`、`Test-StartPreconditions`、`Wait-InstanceStop`、`Write-ComponentLine`；`param` 增加 `[switch]$Json`、`[int]$TimeoutSec = 30`；允许用环境变量 `LORETIDE_WIN_RUNTIME` / `LORETIDE_WIN_STATE` 覆盖 `$runtime` / `$state`（供测试指向临时目录）
- [x] T003 [P] 新建 `scripts/local-windows.test.ps1`（偏差：未先写失败骨架再实现，脚本与测试同批写出；首次运行确有 1 处失败并据此修正夹具）：最小自写断言（`Assert-Exit`、`Assert-Contains`、`Assert-NotContains`）；`BeforeAll` 建临时状态目录、写桩 `pg_ctl.exe`（批处理或 `.ps1` 桩，按参数返回可控退出码）、伪 `secrets.json`（内容为占位字符串 `PLACEHOLDER_SECRET`）；先写好用例骨架（此时应失败）

## Phase 3: User Story 1 - status 证明每个组件是谁、属于谁 (Priority: P1) 🎯 MVP

**Goal**: 四组件各一行；pid / owned / build / health；`-Json`；冲突标出；退出码按契约。

**Independent Test**: quickstart 手动 §status 段；自动 T006。

- [x] T004 [US1] `Get-InstanceStatus`：按 research D1 判定 postgres / supervisor / api / web 的 state、pid、owned（可执行路径或命令行前缀 == `$root`）；按 D2 用 `Get-NetTCPConnection -State Listen -LocalPort 15332,18000,13000` 找 `OwningProcess` 不在本实例 pid 集合的监听 → `conflicts`；读取 `data/windows/build.txt`（D3）；组件未运行时不发 HTTP 请求；`restarting` 判定：进程 `StartTime` 在 10 秒内，或 pid 与上次 `status` 读取到的 `data/windows/<name>.pid` 不同
- [x] T005 [US1] 输出：无 `-Json` 时每组件一行 + conflicts + `ok=`；`-Json` 时按 `contracts/status-json.md` 输出单个对象（`ConvertTo-Json -Depth 4 -Compress`）；退出码 0 / 1 / 2 按契约
- [x] T006 [US1] `local-windows.test.ps1`：`status -Json` 可 `ConvertFrom-Json` 且含 4 个组件键；全部停止时退出 1 且耗时 < 3s；用桩把 13000 标为「非本实例 pid」时 `conflicts` 非空且 `ok=false`；输出不含 `PLACEHOLDER_SECRET`

**Checkpoint**: US1 可独立交付。

## Phase 4: User Story 2 - start 前置缺失立即失败并指出组件 (Priority: P1)

**Goal**: 缺 api.exe / secrets.json / 端口被占 → 退出非 0 + 组件名 + 下一步；幂等；陈旧 pid 清理。

**Independent Test**: quickstart「前置缺失」段；自动 T008。

- [x] T007 [US2] `Test-StartPreconditions`（research D4 顺序）：secrets.json 存在 → api.exe 存在 → postgres 可启（沿用现有逻辑）→ 三端口无非本实例占用 → `supervisor.pid` 指向的进程不存在或命令行不匹配时删除 pid 文件；任一失败输出 `[<component>] <原因>；日志：data/windows/<name>.log；下一步：<动作>` 并退出非 0；全部通过后 `git -C $root rev-parse --short HEAD` 写入 `data/windows/build.txt`，再按现有方式启动
- [x] T008 [US2] `local-windows.test.ps1`：三种缺失各一例（退出非 0 + 输出含 `api` / `secrets` / `web`）；陈旧 pid 文件被删除；已运行时再次 `start` 退出 0 且输出「已运行」；所有输出不含 `PLACEHOLDER_SECRET`

## Phase 5: User Story 3 - stop 等待并确认 (Priority: P2)

**Goal**: 等待退出、超时报告、postgres 不动、幂等。

**Independent Test**: quickstart 主流程 `stop` 段 + SC-003；自动 T010。

- [x] T009 [US3] `Wait-InstanceStop`：写 `stop` 文件后每秒轮询 api / web / supervisor pid 是否存活，至 `$TimeoutSec`；全部退出 → 输出三行「已退出」退出 0；超时 → 列出仍存活组件与 pid，退出 1，不强杀；实例未运行 → 输出「无需停止」退出 0；不触碰 postgres
- [x] T010 [US3] `local-windows.test.ps1`：未运行时 `stop` 退出 0；用桩 pid（当前 shell 的 pid 作为「不退出」的进程）验证超时路径退出 1 且列出组件

## Phase 6: Polish

- [x] T011 `docs/development/native-windows.md`：更新 Start and stop 段（`-Json`、`-TimeoutSec`、前置检查、退出码）；新增「限制」段写明同一机器只支持一个实例、第二个 checkout 会在端口检查处失败（FR-010）；追加「更新记录」一行（日期 / 改了什么 / 验证证据）
- [x] T012 运行 `pwsh -File scripts/local-windows.test.ps1` 全部通过；运行 quickstart 手动主流程、前置缺失、数据保留，把命令 + 退出码 + 输出片段写入 `baseline.txt`（**未执行**：需真实 Windows + PostgreSQL + 已构建 api.exe；PR 正文附主任务本机验证清单）；`grep -c "LORETIDE_EXECUTION_POLICY:'disabled'" scripts/local-windows-supervisor.mjs` 为 1（FR-009）；浏览器可达项交用户确认
- [x] T013 `git diff --stat` 确认文件 ⊆ {scripts/local-windows.ps1, scripts/local-windows.test.ps1, docs/development/native-windows.md}（`local-windows-supervisor.mjs` 仅当采纳 research D3 可选项时出现）；准备 PR 正文：UI 影响：无页面改动；手动 UI Todo：浏览器可达 1 项；回滚：撤销本 PR

## Dependencies

- T001 → 全部
- T002 → T004、T007、T009；T003 → T006、T008、T010（T002 与 T003 并行）
- US1：T004 → T005 → T006
- US2：T007 → T008（T007 依赖 T004 的 owned/conflict 判定函数）
- US3：T009 → T010（独立于 US2）
- Polish 依赖全部

## Parallel Example

```text
并行组 A：T002 脚本重构 | T003 测试骨架
并行组 B（US1 完成后）：T007 start 前置 | T009 stop 等待
```

## Implementation Strategy

1. MVP = T002 + US1：先让 `status` 说真话，这是后面两个故事的判定基础。
2. US2 与 US3 互不依赖，可并行；都复用 US1 的判定函数。
3. 全部约 3 个手写文件，符合任务卡量级；单 PR。
