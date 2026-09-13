# Feature Specification: Windows 实例生命周期入口的可核验性

**Feature Branch**: `003-lt008-windows-instance-lifecycle`

**Created**: 2026-09-14

**Status**: Draft

**Input**: User description: "LT-008：为当前 Windows 实例提供可核验的启动 / 状态 / 停止方式。以现有 scripts/local-windows.ps1 为基础闭合验收缺口，不重写为另一套脚本。"

**Traces to**: `tasks/todo.md` LT-008；T-01；NFR-04；`docs/development/native-windows.md`。

## Current State（以代码为准）

任务卡写「预计涉及 scripts/dev-start.ps1 / dev-status.ps1 / dev-stop.ps1（拟新增）」，这个前提已过时。当前 Windows 入口是：

- `scripts/local-windows.ps1 {start|stop|status|build}`（25 行）+ `scripts/local-windows-supervisor.mjs`（进程监督，崩溃 3 秒后自动拉起）。
- `start`：检查并按需启动 PostgreSQL（端口 15332）；按 `data/windows/supervisor.pid` 判断监督进程是否已在运行（同时校验命令行含脚本名），已在运行则退出；否则以隐藏窗口启动 node 监督进程。
- `stop`：写 `data/windows/stop` 文件请求应用优雅停止；PostgreSQL 保留运行。
- `status`：打印 `pg_ctl status`，再对 `http://localhost:18000/health` 与 `http://localhost:13000` 各发一次请求打印状态码。
- `build`：`go build` 生成 `data/windows/api.exe`。
- 监督进程从 `data/windows/secrets.json` 读数据库密码与 JWT；端口 18000 / 13000 / 15332 硬编码；`LORETIDE_EXECUTION_POLICY=disabled` 固定。

对照任务卡三条验收：

| 验收 | 现状 |
|---|---|
| 重复启动不重复占端口 | 基本满足：pid + 命令行校验；但 pid 文件陈旧（进程已死）时会正常启动，pid 被无关进程复用时靠命令行匹配排除 |
| 停止只影响本实例且保留数据 | 满足数据保留；「只影响本实例」在单实例下成立，第二个 checkout 会因硬编码端口互相干扰 |
| 失败指出具体组件 | 不满足：`status` 只报 URL 可达与否；`start` 只对 PostgreSQL 抛错，`api.exe` 缺失或 `secrets.json` 缺失时监督进程反复崩溃只写日志，命令行看不到 |

另有一项与 `CLAUDE.md` 的 Linux `make status` 不对等：Windows `status` 不显示各组件 pid、所属 checkout、构建标识，无法「证明这是你的进程」。

## Clarifications

### Session 2026-09-14

- Q: 同一台 Windows 机器上第二个 worktree 的端口/数据库隔离是否纳入？ → A: 不纳入。本功能只保证单实例；文档写明第二实例不支持；第二个 checkout 的 start 在端口检查处按 FR-004 失败并指出占用者；隔离机制另立任务。

## User Scenarios & Testing *(mandatory)*

### User Story 1 - status 证明每个组件是谁、属于谁 (Priority: P1)

开发者或 agent 运行 `status`，看到 postgres / supervisor / api / web 每一项：运行或停止、pid、所属 checkout 路径、构建标识、健康检查结果；不属于本 checkout 的同端口进程被明确标出，而不是显示成「可用」。

**Why this priority**: 这是「可核验」的核心。当前 `status` 无法区分「我的 api 在跑」和「另一个目录的 api 占着 18000」。`CLAUDE.md` 已把这个能力作为 Linux 侧的基线要求。

**Independent Test**: 启动后运行 `status`，每组件一行且 pid 与 `Get-Process` 一致；用无关进程占住 13000 后运行 `status`，web 行显示「端口被非本实例进程占用」而非「200」。

**Acceptance Scenarios**:

1. **Given** 实例已启动，**When** 运行 `status`，**Then** 输出四个组件各一行，含状态、pid、checkout 路径、构建标识、健康结果；退出码 0。
2. **Given** 实例未启动，**When** 运行 `status`，**Then** 四行均为停止，不发起无意义请求等待超时；退出码非 0 但非异常。
3. **Given** 端口 18000 被另一进程占用，**When** 运行 `status`，**Then** api 行标明「非本实例进程 pid N 占用」；退出码非 0。
4. **Given** agent 需要机器可读结果，**When** 运行 `status -Json`，**Then** 输出单个 JSON 对象，字段与人类可读输出一一对应。

---

### User Story 2 - start 在前置缺失时立即失败并指出组件 (Priority: P1)

运行 `start` 时若 `api.exe` 未构建、`secrets.json` 缺失、目标端口被非本实例进程占用，脚本立即以非零退出并说明缺哪个组件、下一步做什么；不启动半套服务留下一个反复崩溃的监督进程。

**Why this priority**: 任务卡明确要求「失败指出具体组件」；当前失败被吞进日志文件，agent 和人都要翻日志才知道。

**Independent Test**: 删除 `data/windows/api.exe` 后运行 `start`，退出码非 0，输出含「api.exe 缺失，先运行 build」；恢复后 `start` 成功。

**Acceptance Scenarios**:

1. **Given** `api.exe` 不存在，**When** 运行 `start`，**Then** 不启动监督进程，退出非 0，输出指出 api 组件与 `build` 动作。
2. **Given** `secrets.json` 不存在，**When** 运行 `start`，**Then** 同上，指出 secrets 组件与文档位置；输出不包含任何密钥内容。
3. **Given** 端口 13000 被非本实例进程占用，**When** 运行 `start`，**Then** 退出非 0，指出 web 组件与占用 pid。
4. **Given** `supervisor.pid` 指向已不存在的进程，**When** 运行 `start`，**Then** 清理陈旧 pid 文件后正常启动。
5. **Given** 实例已在运行，**When** 再次运行 `start`，**Then** 报告已运行并退出 0，不新增监听端口。

---

### User Story 3 - stop 等待并确认，不只是请求 (Priority: P2)

运行 `stop` 后脚本等待 api / web 进程实际退出（有上限），报告每个组件的结果；PostgreSQL 保持运行；重复 `stop` 幂等。

**Why this priority**: 「停止只影响本实例且保留数据」需要可验证：当前 `stop` 写完文件就返回，无法知道进程是否真的退了，紧接着 `build` 会因 `api.exe` 被占用失败。

**Independent Test**: `stop` 后 `Get-Process` 无 api.exe / next 进程；`pg_ctl status` 仍为 running；随后 `build` 成功。

**Acceptance Scenarios**:

1. **Given** 实例运行中，**When** 运行 `stop`，**Then** 脚本等待至 api、web、supervisor 退出或达到超时，输出每组件结果；PostgreSQL 状态不变。
2. **Given** 超时仍有进程未退，**When** 等待结束，**Then** 输出未退出组件与 pid，退出非 0，不强杀。
3. **Given** 实例未运行，**When** 运行 `stop`，**Then** 报告无需停止，退出 0。

---

### Edge Cases

- `supervisor.pid` 存在但进程是无关程序复用了该 pid：现有命令行匹配处理；`status` 也需用同一判定。
- PostgreSQL 在运行但数据目录不是 `F:\loretide-runtime\pg`：`status` 标出数据目录不匹配。
- `/health` 返回 200 但 web 返回 500：两行各自报告，不合并成「可用」。
- 监督进程在崩溃循环中（api 每 3 秒重启）：`status` 显示 supervisor 运行、api 反复重启（pid 变化或启动时间极近）。
- 脚本在非 PowerShell 7 下运行：明确最低版本，不静默失败。

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: `status` MUST 对 postgres、supervisor、api、web 各输出一行：状态、pid、所属 checkout 路径、构建标识（`LORETIDE_BUILD` 或 commit）、健康检查结果。
- **FR-002**: `status` MUST 区分「本实例进程」与「非本实例进程占用同端口」，后者不得显示为健康。
- **FR-003**: `status` MUST 提供 `-Json` 输出，字段与人类可读输出对应，供 agent 校验。
- **FR-004**: `start` MUST 在启动监督进程前检查 `api.exe`、`secrets.json` 存在以及三个端口未被非本实例进程占用；任一失败即退出非 0，输出组件名与下一步动作。
- **FR-005**: `start` MUST 幂等：已运行 → 报告并退出 0；陈旧 pid 文件 → 清理后启动。
- **FR-006**: `stop` MUST 等待 api、web、supervisor 退出，超时上限可配置且有默认值；超时后列出未退出组件，退出非 0，不强杀；PostgreSQL 不受影响；未运行时幂等退出 0。
- **FR-007**: 所有失败输出 MUST 指向具体组件与对应日志文件路径；MUST NOT 输出 `secrets.json` 内容或任何密钥。
- **FR-008**: `docs/development/native-windows.md` MUST 与实际行为一致，并追加更新记录（日期 / 改了什么 / 验证证据）。
- **FR-009**: 脚本 MUST NOT 改变监督进程的执行策略（`LORETIDE_EXECUTION_POLICY=disabled` 保持）。
- **FR-010**: `native-windows.md` MUST 写明同一台机器同时只支持一个实例；第二个 checkout 运行 `start` 时 MUST 在端口占用检查处按 FR-004 失败并指出占用进程，不得静默覆盖。

### Key Entities

- **实例**：一个 checkout 对应的 postgres + supervisor + api + web 四组件集合；身份由 checkout 路径 + pid + 构建标识确定。
- **状态目录**：`data/windows/`，含 pid 文件、日志、`stop` 信号文件、`secrets.json`（已被 `.gitignore` 排除）。

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: 手动全流程「start → 浏览器访问 → 重复 start → status → stop → start」通过，每步输出与本规格一致；`netstat` 前后对比重复 start 未新增监听。
- **SC-002**: 三种前置缺失（api.exe、secrets.json、端口占用）各自使 `start` 退出非 0 且输出指出对应组件；恢复后成功。
- **SC-003**: `stop` 后 `pg_ctl status` 为 running，且一张业务表行数与停止前一致；紧接 `build` 成功。
- **SC-004**: `status -Json` 输出能被 `ConvertFrom-Json` 解析，四组件字段齐全。
- **SC-005**: 以上验证以命令 + 退出码 + 输出片段记录在交付记录中；未执行的项标「未执行」。

## UI Impact

脚本变更，无页面改动。

手动 UI Todo（由用户确认）：
1. `start` 成功后，浏览器打开 `http://localhost:13000/loretide-dev-check/diagnostics`，页面可达且诊断概览显示 web 组件健康。

## Assumptions

- PostgreSQL 二进制与数据目录位置沿用 `F:\loretide-runtime`，不在本功能内参数化。
- 本功能不引入新的运行时依赖；端口占用检测使用 PowerShell 内置能力。
- Linux 侧 `make up/status/down` 不改动；两侧输出字段语义对齐但不要求格式相同。
- 同一台机器上的第二个 checkout 隔离不纳入本功能（clarify 2026-09-14）；隔离机制另立任务。
