# Issue #4 — CI-01：补齐诊断前端回归与数据库最小权限

- Issue: https://github.com/899ms/loretide/issues/4
- 执行者：Claude (Opus 4.8) — Claude Code 会话 `26a4a9e9-f381-4563-8524-33475f9214dd`
- 分支：`task/issue-4-ci-diagnostics`（基线 app-main `bb3a4b4616bf48a52e075bbe5e23e38112bbf6aa`）
- 工作目录：独立 git worktree `loretide-development-04017b`
- PR 目标：`app-main`（Draft）

## 修改的文件（严格限定在 Issue 文件边界内）

1. `.github/workflows/loretide-content.yml` — 修改现有 workflow：
   - 新增执行 `packages/core/content/diagnostics/queries.test.tsx`
     与 `apps/web/platform/content-diagnostics.test.ts`（此前只跑 `contract.test.ts`）。
   - 用专用数据库 `loretide_diag_ci` + 最小权限登录角色
     `loretide_diag`（`NOSUPERUSER NOCREATEDB NOCREATEROLE`）替代原来直接用
     `postgres` 超级用户；管理员仅用于初始化，集成测试以受限角色连接。
   - 新增失败即停止的身份/权限核验步骤（`false false false true`，否则 `exit 1`）。
   - 随机生成角色密码、`::add-mask::` 掩码；日志不输出密码或完整连接串。
   - 保留原有检查：模块边界、执行器拒绝门禁、迁移不变量、Web typecheck、API build，
     并对 Go 步骤加 `set -euo pipefail` 确保失败不被吞掉。
2. `docs/development/ci-diagnostics.md` — 新增：workflow 说明、最小权限模型、
   密钥处理、本地复现步骤。
3. `records/issue-4-ci-diagnostics.md` — 本交付记录。

未修改：生产代码、任何已有测试、其它 workflow、环境启动脚本、`tests/diagnostics-acceptance/`。
未改动 #1/#2 范围。

## 设计要点与证据

- 集成测试 `store_integration_test.go` 从 `LORETIDE_DIAG_TEST_DATABASE_URL`
  取连接串，自建临时 schema、跑迁移 468–473、清理时 DROP。核对迁移
  468–473 无 `CREATE EXTENSION` / `SUPERUSER` / `CREATE ROLE` 等特权 DDL，
  故受限角色（仅拥有单个数据库）足以执行，无需集群级权限。
- 该测试文件属于“已有测试”，未修改；仅通过环境变量把它指向受限角色。
- 本地未安装依赖，且 Issue 明确要求以真实 CI 为验收（不能用本地成功替代），
  故未在本机执行；YAML 与所有 `run` 脚本已通过 `bash -n` 与 YAML 解析校验。

## 真实 CI 运行证据

<!-- 推送并创建 Draft PR 后，按 head SHA 记录真实 GitHub Actions run -->

- PR：<待补>
- head SHA：<待补>
- Actions run：<待补>
- 各检查结果：<待补>

## 跳过项 / 未完成项

- 本机未运行测试（无 node_modules；验收以 CI 为准）。
- 不涉及真实执行器、VPS、Docker、开发业务数据库、branch protection。

## 回滚方法

- 仅 revert 本任务对 CI/文档的改动：
  `git revert <本任务提交>` 或在 PR 关闭后丢弃分支 `task/issue-4-ci-diagnostics`。
- workflow 可直接恢复为基线 `bb3a4b461` 的版本；无数据迁移、无生产变更需回滚。
