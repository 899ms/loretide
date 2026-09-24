# #245 诊断 A2：宿主配置事实接入与兼容响应记录

**日期**：2026-09-22（Codex 起草）/ 2026-09-25（Claude-F5.1 接续完成，Multica GLN-13）  
**基线**：`cea2bb43befa3b24aa70af86f5d3dcbea3a81f81`（`app-main`）  
**分支**：`codex/diagnostic-component-facts-a2`  
**范围**：仅 #245 A2；不启动本机服务、不连接本机数据库、不运行迁移、端口、浏览器或真实执行器。

## 已实现

| 文件 | 关联与用途 |
|---|---|
| `server/internal/content/diagnostics/component_facts.go` | 给既有纯归约器增加数据库探测失败的明确、只读活性状态；仍不产生配置事实。 |
| `server/internal/content/diagnostics/service.go` | 新增三个窄的内部宿主入口：启动、存储、只读 disabled 策略；overview 将配置事实、心跳和数据库连通性分开归约，并以可选字段返回。没有通用 source/component 写入 API。 |
| `server/cmd/server/router.go` | 在现有 S3/local 存储选择后，仅登记 `s3`、`local` 或未配置；不传 bucket、路径或凭据。 |
| `server/cmd/server/main.go` | 在 `NewRouterWithOptions` 返回 handler 后登记 `server_boot` 的 api/database 事实；仅在既有 `executionpolicy.Check()` 返回 `ErrDisabled` 时登记 disabled。没有 listener-ready 断言。 |
| `packages/core/content/diagnostics/contract.ts` | 为 `config_state`、`health_state`、`execution_state`、`config_observed_at`、`config_reason` 提供旧 wire payload 的安全默认值；既有字段保持不变。 |
| `server/internal/content/diagnostics/component_facts_test.go` | 覆盖配置与活性分离、未知来源、未配置 files、disabled executor 和敏感 storage 类别拒绝。 |
| `packages/core/content/diagnostics/contract.test.ts` | 覆盖旧 overview 缺少新增字段时的安全解析，以及新字段的保留。 |

## 事实与兼容性结果

- `api`：`configured` 但在没有 listener-ready 生命周期的情况下为 `unverified`，绝不预置为 healthy。
- `database`：启动配置事实与 `Store.Check` 的当前连通性分开；探测只影响 health/status，不回显连接值。
- `files`：无存储为明确 `unconfigured`；配置时仅暴露 `s3` 或 `local` 类别。
- `web`、`daemon`、`search`：没有宿主来源时保持 `unknown`；浏览器 heartbeat 只能提供 health evidence。
- `executor`：当前策略只能产生 `disabled`，不存在 enabled 写入接口；非 executor 的 execution 字段为 `not_applicable`。

## 验证

| 检查 | 结果 |
|---|---|
| `go test ./internal/content/diagnostics -run '^(TestComponentFactRegistry|TestReduceComponentStatus|TestNewServiceBuildsFactRegistryWithoutStore|TestHostFactsStaySeparateFromLiveness|TestStorageFactsRejectUnknownCategory|TestStorageFactsExposeOnlyAllowedCategory)$' -count=1` | 通过（纯内存，不调用 Store、数据库或服务）。 |
| `go test -c -o <validated temp>/server.test.exe ./cmd/server` | 通过；仅编译，未执行二进制或 `cmd/server` 测试。 |
| `git diff --check` | 通过。 |
| 注册点静态搜索 | 仅命中 `cmd/server/main.go`、`cmd/server/router.go`、诊断服务及测试；无 handler/HTTP 写入路由。 |
| `go vet ./internal/content/diagnostics/... ./cmd/server` / `go build ./cmd/server` | 通过（2026-09-25）。 |
| `go test ./internal/content/diagnostics/... -count=1` | 通过（整包，含 A1/A2 全部纯测试；包内无 `TestMain`/`init` 数据库副作用）。 |
| `go test ./cmd/migrate ./internal/migrations -count=1` | 通过。 |
| `pnpm install --frozen-lockfile --prefer-offline` 后 `pnpm typecheck --force` | 9/9 包通过；锁文件无改动。 |
| `pnpm run check:content-boundaries` / `check:diagnostics-contract` / `check:diagnostics-no-upload` | 三项通过。 |
| `pnpm --filter @multica/core exec vitest run content/diagnostics` | 6 个文件 92 个用例通过，含旧 wire 缺字段回落与新字段保留两条。 |

## 接续修正（2026-09-25）

- `overviewComponents` 原先无论 `Store.Check` 成败都给 database 活性事实填 `database_connectivity_unavailable`，healthy 行会带上失败原因码；改为只在探测失败时附加，并在 `TestHostFactsStaySeparateFromLiveness` 补断言 healthy 时 `Reason == ""`。
- `origin/app-main` 仍为基线 `cea2bb43b`，rebase 为空操作。
- `service.go` 由原执行者从单行压缩风格改为 gofmt 布局，逻辑改动只在 `Overview`/`overviewComponents` 与三个注册入口；保留该格式化，未再改回。

## 诊断概览潜在影响（未改页面，供人工手验）

`packages/views/content/diagnostics/index.tsx` 未改动，但同一页面在新后端下会呈现如下差异：

- `api` 从 `healthy` 变为 `unverified`，`last_seen` 为 null，页面显示「未收到」；这是规格要求（无 listener-ready 证据不预置 healthy），不是回归。
- `web`/`daemon`/`search` 及无心跳的 `executor` 从 `unverified` 变为 `unknown`；`files` 无存储时为 `unconfigured`，有存储时 `version` 显示 `s3`/`local`。
- `reason` 由英文句子改为固定原因码（`heartbeat_expired`、`execution_disabled`、`database_connectivity_unavailable`），页面原样显示。
- 页面用 `c.reason || "当前连接可用"` 兜底：`unknown`/`unverified` 行 reason 为空时会显示「当前连接可用」，与徽章状态不一致。属于既有呈现逻辑，本卡禁止改页面，需主控裁定是否另开卡处理。
- 新增 `config_state`/`health_state`/`execution_state`/`config_observed_at`/`config_reason` 已进 parser，但页面尚未展示；旧后端缺这些字段时回落 unknown/not_applicable。

## 未覆盖与人工 TODO

- UI 未改动，未运行 UI 单测或 computer use。人工验收时应确认诊断概览能分别呈现 unknown、unconfigured、configured/unverified、healthy、unavailable 与 executor disabled，且旧服务响应仍可解析。
- GitHub 隔离 CI 是数据库相关路径的唯一允许验证场所；本记录不把本地纯测试当作该验证的替代。

## 回滚

回滚本卡只需 revert 本分支对应提交；无迁移、配置改写、服务状态或历史数据回填。
