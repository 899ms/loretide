# Spec Kit 试点交付与 cloud 主控记录

日期：2026-09-14。主任务：Claude 桌面会话（用户授权为总指挥）。执行者：Claude Code cloud 会话 × 2。

## 本轮产出

| 项 | 位置 | 状态 |
| --- | --- | --- |
| Spec Kit 基线：`.specify/` 适配模板、薄 constitution、`.claude/skills/speckit-*`、`.gitignore` 白名单 | app 分支 `chore/spec-kit-baseline` @ `d7211755a` | PR [#20](https://github.com/899ms/loretide/pull/20) → app-main，CI 由其触发 |
| specs/001–004：spec / clarifications / plan / research / contracts / quickstart / tasks | 同上 | 随 #20 |
| 001 ARCH-01/02 验收闭合与本地入口 | PR [#19](https://github.com/899ms/loretide/pull/19) → 基线分支 | 审查通过，待合并 |
| 002 DIAG-12/08 下载保真与流断线恢复 | PR [#21](https://github.com/899ms/loretide/pull/21) → 基线分支 | 审查通过，待合并；26 条手动 UI 项待用户本机验证 |
| 003 LT-008 Windows 实例生命周期 | cloud 会话进行中 | 分支 `claude/spec-003-lt008-windows-instance-lifecycle` |

合并顺序：#19 → #21 → #20。合并动作由用户执行（Claude Code 自动模式将 `gh pr merge` 保留给人）。

## 状态回写

`tasks/architecture.md` ARCH-01/02、`tasks/diagnostics.md` DIAG-08/12 改为「已交付 / 部分交付，待合并验收」并附 PR 证据；未勾选——勾选留待合并与用户验收（constitution 原则 X）。

## 过程中确认的事实（供后续复用）

1. **`claude --cloud` 对本仓库始终走打包而非克隆**，三次尝试（linked worktree、普通 `.git` 目录的完整克隆）均如此，且因仓库 146 MiB 超过 100 MB 上限退化为无远端、无历史的压扁快照，会话无法推送。GitHub 侧 Claude App 授权范围已包含 `899ms/loretide`，与授权无关。**结论：本仓库的 cloud 会话一律从 claude.ai/code 网页创建**（仓库选择器 → 分支 → 权限 Auto → 模型），网页创建的会话正常克隆、带 `gh` 与推送凭据。
2. 网页创建后，主控通过 `claude -p "<指令>" --cloud <session_id>` 下指令；通过 Chrome 标签页 `get_page_text` 可读取 cloud 会话对话，无需用户转述。
3. 云端 VM：Ubuntu 24.04，Node 22 / pnpm 10.28.2 / Go 1.24.7（go.mod 要求 1.26，`GOTOOLCHAIN=auto` 可用）、PostgreSQL 16、无 `gh`（会话改用 GitHub MCP 建 PR）、无 PowerShell。
4. `server/internal/handler/TestMain` 在无数据库时 `os.Exit(0)`，`go test` 会打印 `ok` 但零用例执行；002 会话自行启动本地 PostgreSQL 后才真实执行。后续 Go 相关任务书需明示此点。
5. 任务卡与代码脱节（评估 F01）在四个功能上全部应验：ARCH 检查器与 CI、LT-008 启停脚本、DIAG 下载均已存在；规格以「Current State（以代码为准）」开头只覆盖缺口。
6. `.github/PULL_REQUEST_TEMPLATE.md` 只有一个文件；Windows 上 `ls` 显示大小写两份是 NTFS 不区分大小写所致。

## 未完成

- 文档仓库阶段 A 的校准（评估 F01–F03：docs/11、docs/04、docs/13 过时状态句）未开始。
- Issue 登记未做；三个 PR 正文均写「Issue 待主任务登记」。
- 004 LT-009 被 LT-008 / ARCH-02 / DG-01 门禁挡住，规格与计划已备。
- 三个作废 cloud 会话（`session_0172…`、`session_01PS…`、`session_01W7…`）待归档。

本次 UI 影响：无；手动 UI Todo：002 的 26 条见 app `specs/002-diag-package-stream-recovery/manual-ui-todo.md`，待用户本机执行。
