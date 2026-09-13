# Issue #11 — CI-02：落实禁止 UI 单元测试与用户手动验收规则

- Issue: https://github.com/899ms/loretide/issues/11
- 执行者：Claude (Opus 4.8) — Claude Code 会话 `26a4a9e9-f381-4563-8524-33475f9214dd`
- 分支：`task/issue-11-manual-ui-policy`（基线 app-main `5716a4f15aa9ffc20b5a84137795d9938fed9c5a`）
- 工作目录：独立 git worktree `loretide-development-04017b`
- PR 目标：`app-main`（Draft）

## 修改的文件（严格限定在 Issue 文件边界内）

1. `.github/workflows/loretide-content.yml` — 移除两条 UI 单测执行步骤（**不删除测试源码**），
   并加注释说明策略：
   - 移除 `pnpm --filter @multica/core exec vitest run content/diagnostics/queries.test.tsx`
   - 移除 `pnpm --filter @multica/web exec vitest run platform/content-diagnostics.test.ts`
2. `docs/development/ci-diagnostics.md` — 更新检查清单（去掉两条 UI 测试行），新增
   “UI unit tests are not run in CI (Loretide policy)” 小节；本地复现段把两条 UI 测试标注为
   “developer-local only，CI 不执行”。
3. `docs/development/README.md` — 新增“Testing policy: no UI unit tests in CI, manual UI acceptance”
   小节：按行为（非扩展名）区分 UI/非 UI 测试的判据、说明旧测试未执行是策略而非通过、
   以及“Manual UI verification Todo”模板。
4. `records/issue-11-manual-ui-policy.md` — 本记录。

未改：生产业务代码、其它 workflow、实例/环境脚本、lockfile、测试源码。未碰 #1/#2。

## 分类依据（按行为，实测信号）

| 测试 | 行为信号 | 处置 |
| --- | --- | --- |
| `content/diagnostics/contract.test.ts` | node 环境，无 DOM/React 信号（纯 schema 契约） | **保留 CI** |
| `packages/views locales/parity.test.ts` | `// @vitest-environment node`，无 DOM | **保留 CI** |
| `content/diagnostics/queries.test.tsx` | `@vitest-environment jsdom` + `renderHook` + `@testing-library/react`（React hook） | **移出 CI**（源码保留） |
| `apps/web platform/content-diagnostics.test.ts` | `@vitest-environment jsdom` + `document`/`HTMLAnchorElement`/`URL.createObjectURL`（DOM 下载） | **移出 CI**（源码保留） |

## 保留的非 UI 检查（未因禁 UI 而缩减）

模块边界（单测 + 扫描）、diagnostics contract、locale parity、Web typecheck、
Go 诊断 `-race`、执行器门禁、迁移不变量、数据库最小权限（provision + 身份核验）、
`go build ./cmd/server`。均原样保留。未重跑被移除的 UI 测试。

## 前后差异

- 之前（我在 #4 引入）CI 会执行 `queries.test.tsx`（React hook）与
  `content-diagnostics.test.ts`（DOM 下载），与最新用户规则“禁 UI 单元测试”冲突。
- 现在 CI 不再执行这两条；两测试源码文件保留在仓库，开发者可本地运行。
- 文档明确：CI 日志中不出现这两条测试不代表其通过，而是策略性不执行；UI 由人工验收。

## 验证

- `git diff --stat` 确认仅改动上述四个在册文件。
- workflow YAML 解析通过；所有 `run` 脚本 `bash -n` 通过；确认 0 条 UI 测试 run-step，
  contract/parity/typecheck/DB/Go 检查仍在。
- 真实 CI（PR head）：见下方“真实 CI 运行证据”，核对日志中不执行两条 UI 单测、非 UI 检查保留。
- 未运行 computer use，未新增 UI 单测/自动点击验收，未全量套跑上游测试。

## UI 影响与手动 Todo

- UI 影响：无（仅改 CI 与文档，无运行时 UI 行为变更）。
- 手动 UI Todo：无。
- 模板仅供未来 UI 任务使用（见 `docs/development/README.md`）。

## 真实 CI 运行证据

- PR：https://github.com/899ms/loretide/pull/12 （Draft，base `app-main`）
- head SHA：`f19d99adad7f8e56402d208fedbb889be8d3ac59`
- Actions run（成功）：https://github.com/899ms/loretide/actions/runs/34765284507
  workflow `Loretide content contracts`，conclusion `success`。
- 日志核对（真实日志）：
  - vitest 仅执行 `content/diagnostics/contract.test.ts`（2 passed）与
    `locales/parity.test.ts`（160 passed）。
  - `queries.test.tsx` 与 `platform/content-diagnostics.test.ts` 的执行行数 = **0**（CI 未运行）。
  - 非 UI 检查保留：`Content boundaries passed`、数据库最小权限身份核验（`diagnostics role (...)`）、
    Go 诊断/门禁/迁移、`go build ./cmd/server`、Web typecheck 均在。

（后续仅追加本记录的提交会再触发一次同 workflow 运行，内容等价、同样通过。）

## 未覆盖风险 / 限制

- CI 不再对这两条 UI 测试提供自动回归；相应 UI 行为改为人工验收，存在人工遗漏风险，
  由用户确认承担。
- 分类按当前测试行为判断；若未来某测试同时混合 UI 与纯逻辑，应拆分后让纯逻辑部分进 CI。

## 回滚方法

- 仅 revert 本任务 CI/文档提交：`git revert <本任务提交>`，或关闭 PR 后丢弃分支
  `task/issue-11-manual-ui-policy`。无生产/数据变更需回滚。
