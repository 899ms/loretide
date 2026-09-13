# Issue #14 — DIAG-SIM-TEST-01：模拟器事件序列与确定性回归

- Issue: https://github.com/899ms/loretide/issues/14
- 执行者：Antigravity (session `8c8b18e1-a29a-4594-ab20-2e24f3630e66`)
- 任务分支：`task/issue-14-simulator-regression`（基线 origin/app-main `5716a4f15aa9ffc20b5a84137795d9938fed9c5a`）
- 工作目录：独立 git worktree `F:/GJ/内容创作工作台/app/.agents/worktrees/issue-14-simulator-regression`
- PR 目标：`app-main`（Draft）

## 修改的文件（严格限定在 Issue 文件边界内）

1. `server/internal/content/diagnostics/simulator_regression_test.go`（新增）
   - `TestSimulatorRegressionReceiverContracts`: 验证 Receiver 对递增序列正常接收、重复序号拒绝为 `DUPLICATE`、乱序拒收为 `LATE_RESULT`、取消后拒收为 `LATE_RESULT` 的独立契约。
   - `TestSimulatorRegressionScenarioContracts`: 对全部 15 种内存场景脱离被测 `Scenarios` 常量定义，独立硬断言出错组件、结果 Code、重试 attempt 递增以及事件序列长度与结果状态。
   - `TestSimulatorRegressionDeterministicTimeAndIdentifiers`: 验证同 seed 下虚拟时间与事件耗时完全一致，而真实 Created 与随机 ID（RunID, EventID, TraceID, OperationID）不发生跨 Run 重复，且单次 Run 内事件关联统一 Operation 与 Trace。
   - `TestSimulatorRegressionRejectionAndSecurity`: 验证 `testEnabled=false`、未授权账号、未知场景被拒绝；明确排除数据库场景真实覆盖并交还 Issue #2，不伪造通过。
2. `docs/development/diagnostics-simulator-regression.md`（新增）
3. `records/diagnostics-simulator-regression.md`（新增）

## 现有覆盖与新增覆盖差异

- **原有情况**：
  - `simulator_test.go` 中的 `TestEverySimulatedFaultAndDeterministicTime` 通过循环 `Scenarios` 并直接断言 `a.Actual == a.Expected`。若被测场景定义中的 `Expected` 与实际逻辑同时错配，该测试依然会错误通过；缺少对出错组件 attribution、attempt 计数及 Receiver 状态机的独立单元验证。
- **新增回归保证**：
  - 独立行为断言：测试用例直接指定字面量预期值（如 `"TIMEOUT"`、`"executor"`），不从生产常量间接推导。
  - 序列契约：验证 `reconnect` 场景 attempt 自动递增、`cancel`/`timeout` 中断终止、`duplicate`/`clock_skew` 容忍后置步执行。
  - 边界隔离：确认数据库场景需要真实持久层支持，严格排除在纯内存单元测试之外，避免弱断言伪造覆盖。

## 实际测试执行结果

在 `server/` 目录下执行定向回归测试：

```bash
# 1. 定向功能回归测试
go test -v ./internal/content/diagnostics -run TestSimulatorRegression -count=1
# 结果：PASS（4 个顶级测试用例，23 个子用例，0.124s）

# 2. 并发与竞态检测
go test -race -v ./internal/content/diagnostics -run TestSimulatorRegression -count=1
# 结果：PASS（2.172s）

# 3. 静态代码分析
go vet ./internal/content/diagnostics
# 结果：通过，无警告或报错
```

## 约束与边界确认

- 未修改任何生产代码（`simulator.go` 等保持原状）。
- 未触碰其他任务文件，未触碰数据库、外部服务或端口。
- 禁止 UI 单元测试与 computer use 验收：无任何 UI 改动，未编写或运行 UI 测试。

## 回滚方法

- 可直接删除分支 `task/issue-14-simulator-regression` 或 revert 对应 commit。
