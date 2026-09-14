# Spec Kit 试点交付与 cloud 主控记录

日期：2026-09-14。主任务：Claude 桌面会话（用户授权为总指挥）。执行者：Claude Code cloud 会话 × 2。

## 本轮产出

| 项 | 位置 | 状态 |
| --- | --- | --- |
| Spec Kit 基线：`.specify/` 适配模板、薄 constitution、`.claude/skills/speckit-*`、`.gitignore` 白名单 | app 分支 `chore/spec-kit-baseline` @ `d7211755a` | PR [#20](https://github.com/899ms/loretide/pull/20) → app-main，CI 由其触发 |
| specs/001–004：spec / clarifications / plan / research / contracts / quickstart / tasks | 同上 | 随 #20 |
| 001 ARCH-01/02 验收闭合与本地入口 | PR [#19](https://github.com/899ms/loretide/pull/19) → 基线分支 | 审查通过，待合并 |
| 002 DIAG-12/08 下载保真与流断线恢复 | PR [#21](https://github.com/899ms/loretide/pull/21) → 基线分支 | 审查通过，待合并；26 条手动 UI 项待用户本机验证 |
| 003 LT-008 Windows 实例生命周期 | PR [#23](https://github.com/899ms/loretide/pull/23) → 基线分支 | 审查通过，待合并；主任务在本机真机验证（测试 92/92；冷态 start / 幂等 / status / stop 全流程通过） |

合并顺序：#19 → #21 → #23 → #20。合并动作由用户执行（Claude Code 自动模式将 `gh pr merge` 保留给人）。

## 状态回写

`tasks/architecture.md` ARCH-01/02、`tasks/diagnostics.md` DIAG-08/12 改为「已交付 / 部分交付，待合并验收」并附 PR 证据；未勾选——勾选留待合并与用户验收（constitution 原则 X）。

## 过程中确认的事实（供后续复用）

1. **`claude --cloud` 对本仓库始终走打包而非克隆**，三次尝试（linked worktree、普通 `.git` 目录的完整克隆）均如此，且因仓库 146 MiB 超过 100 MB 上限退化为无远端、无历史的压扁快照，会话无法推送。GitHub 侧 Claude App 授权范围已包含 `899ms/loretide`，与授权无关。**结论：本仓库的 cloud 会话一律从 claude.ai/code 网页创建**（仓库选择器 → 分支 → 权限 Auto → 模型），网页创建的会话正常克隆、带 `gh` 与推送凭据。
2. 网页创建后，主控通过 `claude -p "<指令>" --cloud <session_id>` 下指令；通过 Chrome 标签页 `get_page_text` 可读取 cloud 会话对话，无需用户转述。
3. 云端 VM：Ubuntu 24.04，Node 22 / pnpm 10.28.2 / Go 1.24.7（go.mod 要求 1.26，`GOTOOLCHAIN=auto` 可用）、PostgreSQL 16、无 `gh`（会话改用 GitHub MCP 建 PR）、无 PowerShell。
4. `server/internal/handler/TestMain` 在无数据库时 `os.Exit(0)`，`go test` 会打印 `ok` 但零用例执行；002 会话自行启动本地 PostgreSQL 后才真实执行。后续 Go 相关任务书需明示此点。
5. 任务卡与代码脱节（评估 F01）在四个功能上全部应验：ARCH 检查器与 CI、LT-008 启停脚本、DIAG 下载均已存在；规格以「Current State（以代码为准）」开头只覆盖缺口。
6. `.github/PULL_REQUEST_TEMPLATE.md` 只有一个文件；Windows 上 `ls` 显示大小写两份是 NTFS 不区分大小写所致。
7. 003 的三个真实缺陷（`pg_ctl start` 子进程继承管道导致 `start` 挂死、`start` 不确认 supervisor 存活、`next dev` 子 worker 被判为端口冲突）云端 61 个桩测试全绿却一个都没抓到，只有 Windows 真机跑出来。涉及本机运行时的任务，主任务必须在真机复跑 quickstart，不能以桩测试通过为准。
8. 主任务给执行者的修复意见应描述**要达到的行为**而非具体 API：第一轮给出的 `Start-Process -Wait` 在 Windows 上等待整个后代进程树，把执行者引向了同样挂死的实现。

## 未完成

- 文档仓库阶段 A 的校准（评估 F01–F03：docs/11、docs/04、docs/13 过时状态句）未开始。
- Issue 登记未做；三个 PR 正文均写「Issue 待主任务登记」。
- 004 LT-009 被 LT-008 / ARCH-02 / DG-01 门禁挡住，规格与计划已备。
- 三个作废 cloud 会话（`session_0172…`、`session_01PS…`、`session_01W7…`）待归档。

本次 UI 影响：无；手动 UI Todo：002 的 26 条见 app `specs/002-diag-package-stream-recovery/manual-ui-todo.md`，待用户本机执行。

## 第二日（2026-09-14 下午～09-15 凌晨）追加

### 合入清单（app 仓库 app-main，均经主任务本机复核后 squash 合并）

| PR | 内容 |
| --- | --- |
| #30 / #32 | specs/005：HTTP trace 传播、请求头/URL 脱敏、进程内 outbox；DIAG-02/03 测试缺口 |
| #35 | specs/007：诊断接入合同 + `check:diagnostics-contract` 进 CI + 豁免须带到期日期 |
| #37 | specs/006：trace 层级瀑布、折叠、回归四态判定与四项定位 |
| #40 / #48 | 验收对照表刷新（新增 §5 剩余实现缺口 G1–G6）、迁移对表 |
| #42 | daemon 三包 114 例失败分诊：108 例为执行闸门（`policytest.SkipIfExecutionGated` + `loretide_gate_open` 构建约束），6 例环境；0 真实回归 |
| #43 / #44 / #49 | specs/008（G1–G6）、009（持久 outbox）、010（非 UI 证据缺口）规格 |
| #45 | specs/009：`content_dispatch_outbox` 表（迁移 474～476）+ 租约排水器；真实 PostgreSQL 53 用例 |
| #47 | `docs/development/spec-kit-workflow.md`（流程规范 + 三条铁律） |
| #50 | specs/010：对照表 121/16/8，`TestContentMigrationConstraints`、`check:diagnostics-no-upload` |
| #52 | `docs/development/manual-ui-runbook.md`：57 条手动项一次跑完的顺序与登记表 |

文档仓库：#31/#34/#36/#38/#39/#41/#46/#51 状态回写。累计 #19～#52 共 34 个。

### 新增教训

9. **turbo 缓存命中的 `pnpm typecheck` 不是证据。** #37 的 PR 正文写「9/9 全部命中缓存」，主任务复核时也只跑了带缓存的 `pnpm typecheck`，两边都绿；实际 `packages/core` 的 `tsc --noEmit` 自 #37 起在 app-main 就是红的（测试夹具缺四个必填字段）。规则：PR 正文出现「命中缓存」，主任务必须用 `--force` 或单包 `tsc --noEmit` 复跑。
10. **规格被事实推翻时在实现 PR 内修正并单列「规格修正」**，不遮掉也不另开 PR；006（`"not_run"` 字面值）、007（提供方无法 import 自己）、009（`sequence` 列、`SKIP LOCKED` 与租约的真实关系）三次都按此处理。
11. **实现任务默认回勾 tasks.md**，未真正执行的项保留并注明；005 与 007 两次遗漏后写进 workflow 文档。
12. **cloud 会话会触发账号用量限额**（016nc 在 impl 008 T032 处停下），未推送的工作留在容器里；主任务应在派发时要求「阶段性 push」，并在限额恢复后先推送再继续。
13. **Windows 专有失败只能在 Windows 分诊**：`-overlay` 摘掉执行闸门后四组用例结果不变，证明与闸门无关（真实 codex-cli 在 PATH、路径进 JSON 转义、symlink 特权、HOME 未隔离）。

### 当前状态

- DG-01 剩余瓶颈只有用户手动矩阵：002 的 37 条 + 006 的 20 条（`docs/development/manual-ui-runbook.md` 给出一次跑完的顺序），008 交付后再加 7 条。
- 进行中：impl 008（会话 016nc，WIP 已推送）。
- 后续项：排水器接入服务级优雅停机；Windows 专有 daemon 失败 98 例分诊（需 Windows 主机）；`Sanitize()` 对 ObjectID/Workspace/Account/Actor 的设计问题。
