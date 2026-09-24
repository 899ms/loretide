# #239 诊断组件配置事实来源与状态合同设计记录

**日期**：2026-09-22
**审计历史基线**：`79510d5329856056c61583d3b874a6874e25dd88` (`app-main`)
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
- 事实注册必须按来源分区并带该来源内单调 generation：`server_boot` 的 `api/database` 与
  `router_storage` 的 `files` 各自原子替换，不能互相清空。缺项是校验错误；失去事实只能显式报
  `unknown`，不能把缺项解释成 `unconfigured`。执行策略另以只读 `ExecutionFact` 注入当前的
  `disabled`，没有任何接口可把它启用；浏览器客户端永远拿不到私有 registrar。

## 可派发实施卡草案（主控预占后才可实现）

两张卡的**正式实施基线**必须是本设计 PR 合并后由主控锁定的 `app-main` 提交；派卡时将该实际 SHA
写入卡片。`79510d5329856056c61583d3b874a6874e25dd88` 仅保留为本设计的审计历史基线，不能作为未来
实施分支的假定起点。本 PR 当前设计头为 `d21a93743c278cc589c0a285d892a3dbe2526d26`。它们只覆盖
`api/database/files` 的事实合同。
`daemon`、`search`、`web` 能力注册、`executor_host`、真实执行器启用和任何 UI 改动均不在这两张卡内。

### A1（S）纯内存事实注册与归约器

**目标与依赖**：在不改 HTTP、router、Store 或 wire response 的前提下，实现本规格的来源分区、generation
和纯归约规则。依赖仅为本记录和 `specs/032-diagnostic-component-config-contract/spec.md`；不依赖数据库、
启动配置、服务或 A2。

**预计文件和真实符号**：

| 操作 | 文件 | 符号 / 边界 |
|---|---|---|
| 新增 | `server/internal/content/diagnostics/component_facts.go` | 新的未导出 `componentFactRegistry`、`sourceSnapshot`、`executionFact`、`registerSourceSnapshot`、`reduceComponentStatus`；仅使用内存 map、`sync` 和 `time`。 |
| 新增 | `server/internal/content/diagnostics/component_facts_test.go` | `TestComponentFactRegistry...` 与 `TestReduceComponentStatus...`；只构造 registry 和值对象。 |
| 可选小改 | `server/internal/content/diagnostics/service.go` | 只增加 registry 字段/构造，不改变 `Service.Overview`、`Store.Check`、`Store.Query` 或现有 `Heartbeat` 的响应行为。 |

**完整验收矩阵**：

| 案例 | 断言 |
|---|---|
| 来源域 | `server_boot` 只能完整提交 `api,database`；`router_storage` 只能完整提交 `files`；`execution_policy` 组件域为空且 execution fact 必填。越域、缺项、重复组件均拒绝。 |
| 分区与代际 | `server_boot@g7` + `router_storage@g3` 后，`router_storage@g4` 仅替换 files；`router_storage@g2` 拒绝；相同 generation 仅完全相同快照幂等。 |
| 未知/未配置 | 无事实或显式 `unknown` 归约为 unknown；只有明确事实才归约为 unconfigured。 |
| 活性冲突 | configured + 无心跳为 unverified；configured + 新鲜/过期心跳为 healthy/unavailable；unconfigured + 新鲜心跳仍为 unconfigured，带安全冲突原因。 |
| 执行策略 | execution-policy 的 disabled 覆盖展示；缺 execution source 时默认 unknown；不存在可写入 enabled 的入口。 |
| 隔离与并发 | 模拟输入无法取得 registrar；并发 reader 只看见完整来源分区，且不丢失其他来源。 |

**无数据库核查方式**：从 `server`（唯一含 `go.mod` 的目录；仓库根没有 `go.mod/go.work`）运行。本卡的测试
不得创建 `diagnostics.Store`，更不得调用 `NewStore`、`Service.Overview`、`Store.Check` 或 `Store.Query`。
在 `internal/content/diagnostics` 未出现 `TestMain` 的前提下，只运行新测试名称，例如
`go test ./internal/content/diagnostics -run '^(TestComponentFactRegistry|TestReduceComponentStatus)' -count=1`。
另以 `rg` 确认新文件不导入 `pgx`/`pgxpool`、不引用 `Store`。这两项均不打开数据库连接。

**兼容、回滚和禁止范围**：A1 不变更 JSON、页面或既有 overview 行为，故旧前后端无 wire 风险。回滚仅 revert
A1 提交。禁止改 `server/cmd/server/*`、`server/internal/handler/*`、`packages/core/*`、`packages/views/*`，禁止
迁移、服务、端口、真实执行器、模拟事实写入或 UI 单测。

### A2（M）受控宿主接入与可选响应合同

**前置依赖**：A1 已合并且其纯测试为绿；主控明确预占此卡。只接入已有且可确认的
`server_boot`（api/database）、`router_storage`（files）和当前只读 `execution_policy=disabled`。不为 web、daemon、
search 或 executor configuration 发明来源。

**预计文件和真实符号**：

| 操作 | 文件 | 符号 / 边界 |
|---|---|---|
| 修改 | `server/internal/content/diagnostics/service.go` | `NewService` 持有 A1 registry；`Overview` 从纯 reducer 取得组件行。数据库现有 `Store.Check` 仍只提供活性，不得读取或回显 DSN。 |
| 修改 | `server/cmd/server/router.go` | `NewRouterWithOptions` 在 `storage.NewS3StorageFromEnv` / `storage.NewLocalStorageFromEnv` 的既有选择结果处注册 `router_storage` 的 files 事实（仅 configured/unconfigured 与允许的类别）。 |
| 修改 | `server/cmd/server/main.go` | `main` 已在 `pool.Ping` 后调用 `NewRouterWithOptions` 并取得 `h`；通过 `h.ContentDiagnostics` 注入 `server_boot` 和当前 `executionpolicy.Check` 的只读 disabled 事实。api 的事实不得伪装为监听成功：在没有显式 listener-ready 生命周期改造前只能是配置已知、活性 unknown。 |
| 可选、后置 | `packages/core/content/diagnostics/contract.ts` | 仅以 `.default("unknown")` 等方式解析新增可选 `config_state`、`health_state`、`execution_state`、安全原因/时间字段；不改变旧字段或触碰视图。 |
| 可选、后置 | `packages/core/content/diagnostics/contract.test.ts` | 旧 wire payload 缺失新增字段时安全回落；只测 parser，不是 UI 测试。 |

**完整验收矩阵**：

| 案例 | 断言 |
|---|---|
| 宿主来源 | pool 成功建立后的 boot 事实不含连接值；S3/local 成功或两者都无的装配结果分别生成 files 的 configured/unconfigured；现有 policy 只产生 disabled。 |
| 不作伪 | listener 尚未有 ready 回调时 api 已配置但无活性证据，health 为 unverified（以 spec §3 为准）；`Store.Check` 成功/失败仅改变 database health，不改变 database config。 |
| 无来源组件 | web/daemon/search 和 executor configuration 在本卡仍为 unknown；现有 browser/daemon heartbeat 不能改变其配置。 |
| 响应兼容 | 若可选 wire 字段落地，旧客户端忽略字段；新 parser 面对旧后端缺字段回落 unknown/not_applicable，既有 `status`/metrics/scenarios 完整。 |
| 回归边界 | `ContentDiagnosticClient → Heartbeat("web", ...)` 未变为 registrar；`executionpolicy.Check` 没有 enable 分支；simulation 不写事实。 |

**无数据库核查方式**：从 `server`（唯一含 `go.mod` 的目录）运行，且不得执行
`go test ./cmd/server`，该包有数据库集成 `TestMain`。可以执行 A1 的纯测试和核心 parser 测试；对宿主文件
只做 `go test -c -o <已验证的临时目录>/server.test ./cmd/server`（只编译，不执行测试二进制）及源码断言：
`main` 的注册在已取得 `h` 后、`router` 的 files 注册紧邻现有 storage 选择，且注册调用没有原始配置参数。
编译产物必须置于临时目录并在核验后移除。禁止执行 `main`、`Overview` 的数据库路径或任何本机服务。

**兼容、回滚和禁止范围**：新增 wire 字段必须可选；在未加入 parser 字段时本卡可只落服务端内部行为。回滚仅
revert A2 提交，恢复当前响应形状和合成状态，不迁移、不回填、不清理历史。禁止页面/翻译/视觉改动、UI 单测、
computer use、daemon/runtime 查询、search、真实执行器、配置值读取、数据库/端口/服务运行。

### 卡 B：继续搁置

daemon 的 workspace 聚合与显式空注册表、search 的真实宿主、web 的可信部署清单以及 `executor_host` 的受审查
配置来源尚未裁定。本卡只保留设计，不得因 A1/A2 创建来源、访问 runtime 数据、启用执行器或扩展 UI。

## 验证与交接

本卡允许且应执行的核验：源码路径/符号交叉搜索、Markdown 链接与 `git diff --check`。本卡**未**运行
数据库、迁移、服务、端口探测、UI 单测、真实执行器、浏览器自动化，也未读取本机配置值或向 Multica 发消息。
UI 人工验收、后续代码实现及两张实施卡的拆分均留给主控裁定。
