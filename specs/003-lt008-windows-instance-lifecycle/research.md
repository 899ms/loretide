# Research: 003 Windows 实例生命周期入口的可核验性

Phase 0。Technical Context 无 NEEDS CLARIFICATION；以下为技术决策。

## D1. 组件身份判定

- **Decision**：
  - supervisor：`data/windows/supervisor.pid` → `Win32_Process` 命令行含 `local-windows-supervisor.mjs` 且 `ExecutablePath` 为 node。
  - api：`data/windows/api.pid` → `ExecutablePath` 等于 `<checkout>\data\windows\api.exe`。
  - web：`data/windows/web.pid` → node 且命令行含 `<checkout>\apps\web\node_modules\next`。
  - postgres：`pg_ctl status` 退出码 + 数据目录 `F:\loretide-runtime\pg`。
  - 「归属 checkout」= 上述路径前缀与 `$root` 一致；不一致即「非本实例」。
- **Rationale**：监督进程已写各子进程 pid 文件；用可执行路径与命令行比对比端口更可靠。
- **Alternatives considered**：只看端口 `OwningProcess`——无法区分本实例与另一 checkout 的同名进程。

## D2. 端口占用判定

- **Decision**：`Get-NetTCPConnection -State Listen -LocalPort 15332,18000,13000`；`OwningProcess` 不在本实例 pid 集合内 → 「非本实例进程占用」。`status` 标红并退出非 0；`start` 前置检查失败。
- **Rationale**：FR-002、FR-004；无需第三方工具。

## D3. 构建标识

- **Decision**：`start` 时执行 `git -C $root rev-parse --short HEAD` 写入 `data/windows/build.txt`（连同 `api.exe` 的 `LastWriteTime`）；`status` 读取并显示。监督进程可选读取该文件注入 `LORETIDE_BUILD`，替代当前硬编码 `windows-local-migration`——此项为可选，不作为验收。
- **Rationale**：FR-001「构建标识」；`make status` 在 Linux 侧显示 commit。
- **Alternatives considered**：从 `/health` 读 build——需要 API 已起且返回该字段，不覆盖未启动场景。

## D4. `start` 前置检查顺序

- **Decision**：secrets.json 存在 → api.exe 存在 → postgres 可启 → 端口非本实例占用 → 陈旧 pid 清理 → 启动。任一失败即退出非 0，输出 `[<component>] <原因>；下一步：<动作>`。
- **Rationale**：FR-004；先查最便宜且最常见的缺失。

## D5. `stop` 等待

- **Decision**：写 `stop` 文件后，轮询 api/web/supervisor pid 是否存活，间隔 1 秒，默认上限 `-TimeoutSec 30`；超时列出仍存活的组件与 pid，退出非 0，不强杀。实例未运行 → 退出 0。
- **Rationale**：FR-006；紧接 `build` 需要 `api.exe` 已释放。

## D6. `-Json` 输出

- **Decision**：`status -Json` 输出单个对象（见 contracts/status-json.md），字段与人类可读行一一对应；`ConvertTo-Json -Depth 4 -Compress`。
- **Rationale**：FR-003；agent 校验。

## D7. 测试形式

- **Decision**：`scripts/local-windows.test.ps1` 用 Pester 风格的最小自写断言（不引入 Pester 依赖）：以环境变量指向临时状态目录与伪 `pg_ctl`，验证前置缺失时退出码与输出组件名、陈旧 pid 清理、`-Json` 可解析。真实启动仅在手动清单。
- **Rationale**：与 `scripts/dev-env.test.sh` 同类；constitution II。
- **Alternatives considered**：引入 Pester——新增依赖，超范围。

## D8. 单实例文档

- **Decision**：`native-windows.md` 新增「限制」段：同一机器只支持一个实例；第二个 checkout `start` 会在端口检查处失败并指出占用进程；多实例隔离另立任务。追加更新记录行。
- **Rationale**：FR-008、FR-010；clarify 2026-09-14。
