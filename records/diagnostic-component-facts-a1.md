# #242 A1 纯内存诊断组件事实：交付记录

**Issue**：#242
**实施基线**：`7e1844db104fbf3a0c0e61466bc613fff8ddc26b` (`app-main`)
**范围**：仅 A1 内存注册/归约器与非 UI Go 测试；不含 A2 宿主接入。

## 变更与用途

| 文件 | 用途 |
|---|---|
| `server/internal/content/diagnostics/component_facts.go` | 私有来源分区 registry、完整域校验、单调 generation、同代幂等/冲突拒绝、值拷贝、并发读快照以及配置/活性/执行状态的纯归约。 |
| `server/internal/content/diagnostics/component_facts_test.go` | 覆盖全部合法来源域、越域/缺项/重复拒绝、代际和分区隔离、输入/输出拷贝、状态优先级、并发读取与 `NewService(nil, ...)` 的内存构造。 |
| `server/internal/content/diagnostics/service.go` | 仅向 `Service` 添加私有 registry 字段并在 `NewService` 构造；不改变 `Overview`、`Heartbeat`、`Store.Check` 或 `Store.Query` 行为。 |

## 实际验证

均从 `server`（模块 `go.mod` 所在目录）运行：

```text
go test -json ./internal/content/diagnostics -run '^(TestComponentFactRegistry.*|TestReduceComponentStatus|TestNewServiceBuildsFactRegistryWithoutStore)$' -count=1
go test -json -race ./internal/content/diagnostics -run '^(TestComponentFactRegistry.*|TestReduceComponentStatus|TestNewServiceBuildsFactRegistryWithoutStore)$' -count=1
go vet ./internal/content/diagnostics
```

两组 JSON 测试均逐项确认所有预期顶层测试为 terminal `pass`，`go vet` 也通过。新 registry 与测试不导入 `pgx`/`pgxpool`，也不构造 `Store` 或调用 `Overview`、`Store.Check`、
`Store.Query`；没有连接数据库、启动服务、迁移、端口探测、UI 单测、computer use 或真实执行器。

## 未覆盖与回滚

A2 的 `main`/router/storage 接入、wire response、parser 和 UI 不在本卡。web、daemon、search、真实执行器
和 Multica 交互也未改动。回滚方式为 revert 本卡提交；A1 不写数据库或外部状态。
