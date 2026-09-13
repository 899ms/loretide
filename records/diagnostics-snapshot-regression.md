# Issue #15 — DIAG-SNAPSHOT-TEST-01：快照深拷贝与复现缺口回归

- Issue: https://github.com/899ms/loretide/issues/15
- 执行者：Antigravity (session `8c8b18e1-a29a-4594-ab20-2e24f3630e66`)
- 任务分支：`task/issue-15-snapshot-regression`（基线 origin/app-main `5716a4f15aa9ffc20b5a84137795d9938fed9c5a`）
- 工作目录：独立 git worktree `F:/GJ/内容创作工作台/app/.agents/worktrees/issue-15-snapshot-regression`
- PR 目标：`app-main`（Draft）

## 修改的文件（严格限定在 Issue 文件边界内）

1. `server/internal/content/diagnostics/snapshot_regression_test.go`（新增）
   - `TestSnapshotRegressionCloneIsolation`: 覆盖切片（Required, Excluded, Grants）与字典（Hashes）双向深拷贝隔离，标量、版本及配置字段完整性，空集合与 nil 容错。
   - `TestSnapshotRegressionReproductionGaps`: 覆盖基线一致、文件缺失、hash 变化、授权撤回及多类缺口同时发生；按排序后的缺口内容（而非仅数量）断言，规避 map 遍历无序。
   - `TestSnapshotRegressionInputImmutability`: 验证缺口计算绝不修改输入快照、当前文件 hash 字典和授权字典，已撤权授权不会被旧 Grants 覆盖复活。
   - `TestSnapshotRegressionJSONRoundtrip`: 验证 Snapshot 全部字段序列化往返保真，以及对未知拓展字段的向前兼容解析。
2. `docs/development/diagnostics-snapshot-regression.md`（新增）
3. `records/diagnostics-snapshot-regression.md`（新增）

## 现有覆盖与新增覆盖差异

- **原有情况**：
  - `contract_test.go` 中的 `TestSnapshotImmutableAndRevocation` 仅对单个 hash 键做简单的变异检查，且 `ReproductionGaps` 仅断言了 `len == 2`，没有检查各个缺口的具体类型及 ID 内容，未覆盖 `FILE_CHANGED`、多重授权撤权、空集合切片以及双向隔离。
- **新增回归保证**：
  - 明确断言深拷贝后对原对象的切片/字典修改不影响副本，对副本的修改也不回溯影响原对象。
  - 针对 `ReproductionGaps` 建立表驱动用例，全面覆盖 6 种场景（基线、缺失、哈希变动、撤权、组合并发缺口、空配置），对缺口列表做排序后内容精准比对。
  - 强化输入不可变性断言，证明计算过程绝不产生副作用。
  - 保证 JSON 往返全字段无损。

## 实际测试执行结果

在 `server/` 目录下执行定向回归测试：

```bash
# 1. 定向功能回归测试
go test -v ./internal/content/diagnostics -run TestSnapshotRegression -count=1
# 结果：PASS（4 个顶级测试用例，8 个子用例，0.120s）

# 2. 并发与竞态检测
go test -race -v ./internal/content/diagnostics -run TestSnapshotRegression -count=1
# 结果：PASS（2.244s）

# 3. 静态代码分析
go vet ./internal/content/diagnostics
# 结果：通过，无警告或报错
```

## 约束与边界确认

- 未修改任何生产代码（`contract.go` 等保持原状）。
- 未触碰其他 Issue 的文件，未触碰数据库、外部服务或端口。
- 禁止 UI 单元测试与 computer use 验收：无任何 UI 改动，未编写或运行 UI 测试。
- 数据库集成测试与 HTTP 导出测试归属 Issue #2，不在此任务伪造或引入。

## 回滚方法

- 可直接删除分支 `task/issue-15-snapshot-regression` 或 revert 对应 commit。
