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

## Phase 7: 实机回归（主任务 2026-09-14 在真实 Windows 实例上发现）

主任务用 pwsh 7.6.6 在真机跑完整流程，发现三处云端桩测覆盖不到的缺陷。以下为修复与其验证层级。

- [x] R001 [HIGH] `start` 在需要拉起 PostgreSQL 时挂死：`Invoke-PgCtl` 用 `$out = & pg_ctl ... 2>&1` 捕获管道，`pg_ctl start` 拉起的 `postgres.exe` 继承重定向句柄并持有到自身退出，PowerShell 等管道关闭即等数据库退出（实测 postgres ready 后 5 分钟未返回；杀 pwsh 后 postgres 仍在）。修复：新增 `Start-PostgresServer`，用 `Start-Process -Wait -PassThru -WindowStyle Hidden` 取 `ExitCode`，不接管道，保留 `-l postgres.log`；`status` 分支仍用 `Invoke-PgCtl` 捕获。**验证层级：Windows 实机验证**——句柄继承死锁无法用桩复现
- [x] R002 [MEDIUM] `start` 未确认 supervisor 存活即报成功：supervisor 因 `secrets.json` ENOENT 立即退出（错误只进 `supervisor.err.log`），`start` 仍打印 `Local supervisor started` 并返回 0，第二次 `start` 又重复启动。修复：`Start-SupervisorProcess` 后 `Wait-SupervisorAlive` 最多轮询 10 秒，要求 `supervisor.pid` 存在、进程存活且命令行匹配；否则输出 `[supervisor]` + `err.log` 路径并退出 1。启动前先删除旧 pid 文件，避免陈旧 pid 让等待假通过。**验证层级：桩测覆盖**（`Start-SupervisorProcess` 桩不写 pid → 退出 1）+ Windows 实机复验
- [x] R003 [MEDIUM] Next.js 子进程被误判为端口冲突：`web.pid=15872` 是 `next dev` 父进程，13000 的实际监听者是子 worker `pid 44772`，`status` 报 `conflict ... not this instance`、`ok=false`、`exit 1`，而实例一切正常。修复：新增 `Test-ListenerIsOurs`——监听者 pid 在已知组件 pid 集合内、或命令行/可执行路径位于 `$Root` 之内（与 `Test-ProcessOwned` 同一判定）、或父进程链（上溯 5 层，带自环保护）含已知组件 pid，三者任一成立即非冲突。`Get-ProcessInfo` 增取 `ParentProcessId`。**验证层级：桩测覆盖**（子 worker 命令行含 `$Root` → 非冲突、`ok=true`；仅靠父链的后代 → 非冲突；另一 checkout 的 next → 仍是冲突；父链自环 → 不挂起）+ Windows 实机复验

- [x] R004 [HIGH] R001 的第一版修法本身仍会挂死：`Start-Process -PassThru -Wait` 在 Windows 上等待的是**整个进程树**，而 `postgres.exe` 是 `pg_ctl` 的子进程，等价于等数据库退出——与最初的捕获管道是同一个坑的两条路径（主任务实测：postgres 15:02:39 已 ready，`start` 5 分钟未返回）。修复：去掉 `-Wait`，改 `$proc.WaitForExit(60000)`，只等 `pg_ctl` 自身；60 秒未返回记 `ExitCode=124` 并报失败。另新增 `Wait-PostgresReady`：`pg_ctl` 返回 0 不等于服务器可接受连接，故用 `Invoke-PgCtl status`（不派生子进程，可安全捕获）轮询最多 30 秒作为就绪判据，超时报 `[postgres]` + `postgres.log` 路径并退出 1。**验证层级：桩测覆盖**——本轮找到了让桩能触达的办法：在隔离子作用域内遮蔽 `Start-Process` 这个 cmdlet 本身（函数优先于 cmdlet 解析），于是真实的 `Start-PostgresServer` 会调到桩，桩记录**是否被传入 `-Wait`**，这正是缺陷本身；并断言不阻塞（Stopwatch < 5s）、`WaitForExit` 上限 60000、`pg_ctl` 不返回时报 124。仍需 Windows 实机复验

四组回归用例均已对未修复代码验证过会失败（R001+R004 合计 5 条、R002 5 条、R003 3 条、就绪轮询 3 条），非空断言。

R001 的教训值得记一笔：第一版修法只把「等待对象」从管道换成了进程树，两者都把「等 pg_ctl 返回」误写成「等 postgres 退出」。当时的桩测只断言调用形态（有 `Start-Process`、无 `2>&1`），恰好对新写法成立，所以没拦住。现在的断言改为记录实际传参，形态断言只作为补充。

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
