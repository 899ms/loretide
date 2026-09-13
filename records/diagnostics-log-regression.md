# Issue #13 — DIAG-LOG-TEST-01：日志脱敏与有界缓冲并发回归

- Issue: https://github.com/899ms/loretide/issues/13
- 执行者：Antigravity (session `8c8b18e1-a29a-4594-ab20-2e24f3630e66`)
- 任务分支：`task/issue-13-log-regression`（基线 origin/app-main `5716a4f15aa9ffc20b5a84137795d9938fed9c5a`）
- 工作目录：独立 git worktree `F:/GJ/内容创作工作台/app/.agents/worktrees/issue-13-log-regression`
- PR 目标：`app-main`（Draft）

## 修改的文件（严格限定在 Issue 文件边界内）

1. `server/internal/content/diagnostics/log_regression_test.go`（新增）
   - `TestLogRegressionSanitizeRules`: 表驱动覆盖未注册错误码/枚举默认化、非法 trace/span/id 过滤、路径/URL 脱敏替换、合法 token 与业务身份字段保留、安全消息映射。
   - `TestLogRegressionSlogHandlerLeakingPrevention`: 针对 `slog` 嵌套属性、消息体、`WithAttrs`、`WithGroup` 注入多类合成敏感标记，确保序列化输出零泄露，且允许字段正确入库。
   - `TestLogRegressionBufferCapacities`: 覆盖容量 0/负值自动纠偏为 1、容量 1 的 FIFO 淘汰与 `Dropped` 计数、`Events()` 返回副本隔离保护。
   - `TestLogRegressionConcurrentAppendAndEvents`: 验证并发 `Append` 和 `Events` 在 `-race` 下通过，事件总数严格恒等 `len(finalEvents) + Dropped == totalEvents`。
2. `docs/development/diagnostics-log-regression.md`（新增）
3. `records/diagnostics-log-regression.md`（新增）

## 现有覆盖与新增覆盖差异

- **原有情况**：
  - `log_test.go` 仅包含单个用例 `TestSecretsNeverEnterTechnicalLog`，将脱敏与缓冲容量混在一个简短流程中测试，缺少枚举归一化、路径与非法 ID 边界、切片读写隔离和并发 `-race` 保证。
- **新增回归保证**：
  - 全面表驱动验证：细化区分合法业务租户标识与需脱敏的技术消息，防范误将业务字段全数删除或导致脱敏漏网。
  - 防泄漏验证：覆盖 `WithAttrs`、`WithGroup` 与深度嵌套对象的泄漏防范。
  - 边界容量测试：验证 0、负数、极限容量 1 以及普通容量下的淘汰正确性。
  - 并发模型测试：在 `-race` 下进行高并发读写，验证无数据竞争和计数恒等。

## 实际测试执行结果

在 `server/` 目录下执行定向回归测试：

```bash
# 1. 定向功能回归测试
go test -v ./internal/content/diagnostics -run TestLogRegression -count=1
# 结果：PASS（4 个顶级测试用例，12 个子用例，0.129s）

# 2. 并发与竞态检测
go test -race -v ./internal/content/diagnostics -run TestLogRegression -count=1
# 结果：PASS（2.237s）

# 3. 静态代码分析
go vet ./internal/content/diagnostics
# 结果：通过，无警告或报错
```

## 约束与边界确认

- 未修改任何生产代码（`log.go` 等保持原状）。
- 未修改其他任务文件，未触碰数据库、外部服务或端口。
- 禁止 UI 单元测试与 computer use 验收：无任何 UI 改动，未编写或运行 UI 测试。

## 回滚方法

- 可直接删除分支 `task/issue-13-log-regression` 或 revert 对应 commit。
