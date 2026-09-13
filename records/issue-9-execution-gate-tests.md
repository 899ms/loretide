# Issue #9 — SEC-TEST-01：执行器禁用门禁回归加固

- Issue: https://github.com/899ms/loretide/issues/9
- 执行者：Claude (Opus 4.8) — Claude Code 会话 `26a4a9e9-f381-4563-8524-33475f9214dd`
- 分支：`task/issue-9-execution-gate-tests`（基线 app-main `c1934e002355dc8d5be2badadd6b5bdce402ee43`）
- 工作目录：独立 git worktree `loretide-development-04017b`
- PR 目标：`app-main`（Draft）

## 修改的文件（严格限定在 Issue 文件边界内）

1. `server/pkg/executionpolicy/policy_test.go` — 新增 `TestLoretideErrDisabledMatchesThroughWrapping`：
   锁定 `ErrDisabled` sentinel 经 `%w` 包裹后仍可被 `errors.Is` 匹配（agent/execenv 门禁依赖此契约），
   且无关错误不被误配。保留原有 `TestConfiguredPolicy*` / `TestMissingPolicy*`。
2. `server/pkg/agent/loretide_policy_test.go` — 重写为 `TestLoretideGateRejectsEveryRegisteredFactory`：
   枚举真实注册表 `SupportedTypes`（25 个协议族）与 `BuiltinRuntimes`（内置运行时），
   对 `New` / `NewRuntime` / 生产路由 `ResolveBackend` 断言 `errors.Is(err, ErrDisabled)` 且 backend 为 nil；
   另含未注册类型（证明门禁先于类型校验）；覆盖 env 未设置/disabled/enabled/unknown。
3. `server/internal/daemon/execenv/loretide_policy_test.go` — 重写为两个测试：
   - `Prepare`：用“其余字段全部合法”的合成参数（WorkspacesRoot/WorkspaceID/TaskID/Provider 均设），
     断言 `errors.Is(err, ErrDisabled)`、env 为 nil、且 WorkspacesRoot 未被创建（无文件副作用）。
   - `Reuse`：传入**已存在**的 WorkDir（门禁在 `os.Stat` 之前），断言返回 nil 且未创建
     `multica-config` sidecar。覆盖 env 矩阵。
4. `docs/development/execution-gate-tests.md`（新增）、`records/issue-9-execution-gate-tests.md`（新增）。

未修改：`policy.go` 及任何生产模块、CI/权限/白名单/数据库/服务/端口。未碰 #1/#2 相关文件。
全部合成参数 + `t.TempDir`，不访问用户 HOME、不读真实模型凭据、不调真实提供者 CLI、不启服务。

## 前后断言差异（为何 err!=nil 不够）

- 旧 agent 测试仅 `if _, err := New(provider, Config{}); err == nil { fatal }`——任何非 nil 错误即满足，
  门禁被移除后若因“unknown agent type”等无关原因失败，测试仍绿。
- 旧 execenv 测试 `Prepare(PrepareParams{WorkspacesRoot: root})` 缺 WorkspaceID/TaskID：门禁被绕过时
  会返回“workspace ID is required”，`err == nil` 检查照样通过——正是 Issue 指出的弱断言。
- 新测试统一断言 `errors.Is(err, executionpolicy.ErrDisabled)`（Reuse 按真实契约断言返回 nil），
  并对 `Prepare`/`Reuse` 增加“无文件副作用”核验；agent 侧改为枚举真实注册表而非硬编码子集。

## 实际验证（本地，命令/数量/结果）

定向 go test（从 server/ 运行，明确 -run 范围）：

```
go test ./pkg/executionpolicy -run 'TestConfiguredPolicy|TestMissingPolicy|TestLoretideErrDisabledMatchesThroughWrapping' -count=1 -v
  → PASS：TestLoretideErrDisabledMatchesThroughWrapping、TestConfiguredPolicy(4 子用例)、TestMissingPolicy
go test ./pkg/agent -run 'TestLoretideGateRejectsEveryRegisteredFactory' -count=1 -v
  → PASS
go test ./internal/daemon/execenv -run 'TestLoretidePrepareRejectsWithErrDisabledAndNoSideEffects|TestLoretideReuseReturnsNilWithoutSideEffects' -count=1 -v
  → PASS（2 个测试）
go vet ./pkg/executionpolicy ./pkg/agent ./internal/daemon/execenv → 无输出，退出 0
```

隔离变异验证（不触发真实 CLI，不提交生产改动）：临时将 `Check()` 改为 `return nil`，重跑上述测试，
全部 6 项**失败**——executionpolicy 报 “gate failed open”/“missing configuration enabled execution”；
agent 报 “New(claude) … returned a non-nil backend while execution is disabled”；execenv 报
“Prepare returned a non-nil environment”（并打印 `prepared env root=…` 证明产生了文件副作用）与
“Reuse returned a non-nil environment”。随后 `git checkout -- server/pkg/executionpolicy/policy.go`
还原，`git status` 确认 policy.go 不在改动列表，再次全部 PASS。证明测试能识别门禁被绕过，而非只重复常量实现。

模块边界：改动文件均不在内容模块根（server/internal/content、packages/*/content）之下，
不影响 `check-content-boundaries` 扫描；真实执行器仍禁用（未改 policy.go）。

## 未覆盖 / 限制

- 仅静态/接口层门禁回归；不代表真实执行器运行时越权已被证明（对齐 LT-004 说明）。
- 未运行完整 agent 集成测试（`agentintegration` build tag + `MULTICA_RUN_REAL_AGENT_SMOKE=1`），
  按 Issue 要求不触发真实 CLI。
- 未发现需要生产代码改动才能安全测试的缺口；如后续需要在门禁前更早的层加断言，交主任务裁定。

## 真实 CI 运行证据

<!-- 推送并创建 Draft PR 后按 head SHA 补齐 -->

- PR：<待补>
- head SHA：<待补>
- Actions run：<待补>

## 回滚方法

- 仅 revert 本任务测试/文档提交：`git revert <本任务提交>`，或关闭 PR 后丢弃分支
  `task/issue-9-execution-gate-tests`。无生产/CI/数据变更需回滚。
