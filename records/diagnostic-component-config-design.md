# #239 诊断组件配置事实来源与状态合同设计记录

**日期**：2026-09-22
**基线**：`79510d5329856056c61583d3b874a6874e25dd88` (`app-main`)
**范围**：只读源码后的设计记录；本提交不改生产代码、配置、数据库、服务或 UI。
**关联规格**：[specs/032-diagnostic-component-config-contract/spec.md](../specs/032-diagnostic-component-config-contract/spec.md)

## 结论

当前 `Service.Overview` 只有心跳 map，没有组件配置事实。它把 `api` 直接写成
`healthy`，每次 overview 都为 `database` 做连通性检查；`web/files/daemon/executor/search`
没有诊断模块心跳时统一显示 `unverified`。因此，**没有心跳仅表示没有该诊断心跳，绝不表示未配置**。

建议后续实施将「主机已知的配置事实」和「运行/连通性证据」分开采集，再由一个纯函数归约为
页面状态。配置事实只可由拥有该配置生命周期的宿主显式注册；心跳永远不能创建或覆盖配置事实。

## 现有事实来源追踪

| 组件 | 现有配置入口 / 生命周期 | 现有心跳或活性生产者 | 现在能诚实断言的状态 | 设计后的事实所有者 |
|---|---|---|---|---|
| `api` | `server/cmd/server/main.go`：启动完成 DB 连接、构造 router 与 `newMainHTTPServer`，随后 `srv.ListenAndServe`。`server/cmd/server/router.go:NewRouterWithOptions` 创建诊断 Service。 | 无诊断 `api` 心跳；`Overview` 自己以当前时间写入 `healthy`。 | 当前是合成状态，不是监听成功或请求活性证据。 | `cmd/server` 的启动/监听生命周期；监听失败必须留在 `unknown`，不得预置健康。 |
| `database` | `main.go` 从启动配置取得连接串，调用 `newDBPool`，并在服务前 `pool.Ping`；失败即退出。 | `server/internal/content/diagnostics/service.go:Overview` 调用 `Store.Check`，这是一次当前连通性探测，非配置事实。 | 已有启动成功与单次可用性证据，但 overview 未保存配置事实。 | `cmd/server` 的已解析、已建立 pool（仅布尔/种类，不泄漏连接串）；`Store.Check` 仍只拥有活性。 |
| `web` | 未找到服务器端的 web 部署/诊断开关注册。`packages/core/content/diagnostics/queries.ts:useDiagnostics` 仅在诊断页挂载时每 10 秒请求客户端端点。 | 该请求到 `handler.ContentDiagnosticClient` 后调用 `ContentDiagnostics.Heartbeat("web", "browser", ...)`。 | 只证明一个已授权浏览器最近发过请求；没有请求是 `unverified`，并非未配置。 | 未来 web 部署/客户端能力注册器；在未有可审计注册器前保持 `unknown`。 |
| `files` | `router.go:NewRouterWithOptions` 先尝试 `storage.NewS3StorageFromEnv()`，再尝试 `storage.NewLocalStorageFromEnv()`，结果作为 `store` 传入 `handler.New`；两者可都为 nil。 | 没有诊断 `files` 心跳。 | 存储构造结果是唯一已有的配置事实，但当前未交给 diagnostics。 | Router 装配点；只发布 `configured/unconfigured` 和已允许的存储类别，绝不发布 bucket、路径或凭据。 |
| `daemon` | 诊断模块无 daemon 配置入口。`router.go` 装配 daemon hub 仅表示 server 可接收协议，不等于存在或启用了 daemon。 | `handler.DaemonHeartbeat`、`HandleDaemonWSHeartbeat` 与 `HeartbeatScheduler` 更新 `agent_runtime.last_seen_at` / Redis liveness；语义是逐 runtime、逐 workspace 活性。 | 这是另一套 runtime 生命周期，不能直接填入全局 diagnostics 的 `daemon`。 | 未来从已注册 runtime 配置投影出的、带范围的 daemon adapter；没有显式空注册表事实时为 `unknown`。 |
| `executor` | `server/pkg/executionpolicy.Check` 无条件返回 `ErrDisabled`；注释明确尚无可开启的受审查 adapter。 | 无诊断 executor 心跳。 | 这是明确的 **disabled**（策略关闭），不是 `unconfigured`。 | execution-policy adapter；只有经审查的真实执行器注册后才可报告配置，禁止用 simulator 代替。 |
| `search` | 在 diagnostics 与 router 装配中未发现 Loretide-owned 的 search 配置入口。 | 无诊断 search 心跳。 | 必须为 `unknown`，不得将缺失源码或心跳显示为未配置。 | 后续实际 search host 的注册器；在其出现前不声明未配置。 |

## 关键边界

- `diagnostics.Service.Heartbeat` 目前接受 `web/files/daemon/executor/search`，但代码库中生产调用只有
  `ContentDiagnosticClient → Heartbeat("web", "browser", ...)`。它必须继续是活性通道，不能兼任配置 API。
- daemon 的 Redis/数据库心跳、运行时租约和离线恢复属于 `agent_runtime`。若未来适配到诊断页，适配器必须
  先定义「一个还是多个 runtime、哪个 workspace、注册记录是否仍存在」；不得把一次 runtime 心跳冒充为
  全局 daemon 配置。
- `simulation_enabled` 仅来自开发诊断模拟开关；模拟 run 或模拟心跳不得写入配置事实，更不得令真实组件变绿。
- 任何配置事实只含枚举、时间、来源和安全的版本/原因码；不复制环境变量、连接串、文件路径、bucket 或凭据。

## 建议实施切片（均须主控审查后另开卡）

1. **卡 A：服务端合同和 api/database/files 注册（P1）**
   - 预计：`server/internal/content/diagnostics/*`、`server/cmd/server/main.go`、`router.go`、
     `packages/core/content/diagnostics/contract.ts` 及对应非 UI 纯函数测试。
   - 依赖：本规格的字段、优先级和兼容规则；不接触真实执行器。
   - 回滚：revert 该卡的代码提交；新增响应字段为可选，旧客户端保持现有展示。

2. **卡 B：daemon/executor/search 的具名事实提供者与页面呈现（P2）**
   - 预计：runtime 配置投影/adaptor、执行策略 adapter、前端 contract/诊断概览及非 UI 纯函数测试。
   - 依赖：卡 A 的注册接口；daemon 的范围语义和任何 search host 必须先获主控裁定。
   - 回滚：revert 该卡；没有提供者时统一安全回落 `unknown`，不回填模拟或历史心跳。

## 验证与交接

本卡允许且应执行的核验：源码路径/符号交叉搜索、Markdown 链接与 `git diff --check`。本卡**未**运行
数据库、迁移、服务、端口探测、UI 单测、真实执行器、浏览器自动化，也未读取本机配置值或向 Multica 发消息。
UI 人工验收、后续代码实现及两张实施卡的拆分均留给主控裁定。
