# DIAG-01～13 验收证据对照（面向 DG-01）

## 1. 这份文件是什么

**这是证据对照，不是验收。** 本文件只回答一个问题：`tasks/diagnostics.md` 里 DIAG-01～13 的每一条「交付」「验证」，在当前应用代码里能找到什么证据。

- **勾选与 DG-01 判定不在本文件、也不在本仓库完成。** 按 `.specify/memory/constitution.md` 原则 X，勾选任务卡与判定检查点由主任务在文档仓库（分支 `main`）执行。本文件不勾任何卡片，不声明任何一项「通过」。
- **证据基线**：应用仓库 `app-main` 提交 `58b5e1a2382fe17e32c1047572a5051baf689c4e`。此后的改动不在本文件覆盖范围内。
- **卡片与验收矩阵原文**：文档仓库 `main` 的 `tasks/diagnostics.md` 与 `docs/13-完整开发诊断与操作日志需求.md` 第 9 节。本文件按提交 `7926540cc` 时点的 `origin/main` 读取，未 checkout 该分支。
- **写法约定**：宁可写「无证据」，不写推断。代码里看得见但没有测试固定的能力，一律记「代码存在但无测试」，不记「已通过」。

### 证据类型三档

| 类型 | 含义 |
|---|---|
| **自动测试已通过** | 存在自动测试，且本次实际运行并通过（命令与退出码见第 5 节） |
| **代码存在但无测试** | 实现可在代码中定位，但没有自动测试固定该行为；或测试存在却按 Loretide 政策不执行（UI 单测） |
| **无证据** | 在应用代码中找不到对应实现，或找到的部分不足以支撑该条 |

### 关于「测试存在但不执行」

`packages/core/content/diagnostics/queries.test.tsx` 与 `apps/web/platform/content-diagnostics.test.ts` 两个文件存在于仓库，但按 `.github/workflows/loretide-content.yml` 的显式注释与 constitution 原则 II，**CI 不执行**，本次也未执行。凡依赖它们的条目一律记「代码存在但无测试」，不记「已通过」。

## 2. DIAG-01～13 逐条对照

### DIAG-01 · 诊断合同与模块接口（验收 D13-V02/04）

| 条目 | 证据 | 类型 | 备注 |
|---|---|---|---|
| 交付 operation/trace/span/attempt 合同 | `server/internal/content/diagnostics/contract.go`：`Event`（含 `operation_id`/`trace_id`/`span_id`/`parent_span_id`/`attempt`）、`Envelope`、`Child()`、`Pack()`/`Unpack()`；`TestTracePropagationAndUntrustedIdentity` | 自动测试已通过 | — |
| 交付审计/技术日志合同 | `contract.go` `Event` 单一事件类型；`store.go` `appendAudit` / `Technical` 两条写入路径 | 自动测试已通过 | 由 `TestPostgresAuditRollbackIsolationAndRetention` 覆盖两路独立性 |
| 交付错误码合同 | `contract.go` `Event.error_code`；`simulator.go` `Scenarios` 的 16 个 `expected_code`；`ProviderCode()` | 自动测试已通过 | `TestSimulatorRegressionScenarioContracts` |
| 交付组件状态合同 | `service.go` `Component{Name,Status,LastSeen,Version,Reason}` | 自动测试已通过 | `TestPostgresFullScenariosExportAndHealth` |
| 交付可注入接口 | `store.go` `WorkspaceWriteGuard` 接口；`NewStore(pool, guard)`；`NewService(store, build, enabled)` | 自动测试已通过 | 测试以 `isolatedWorkspaceWriteGuard` 注入 |
| 明确数据所有权 | `scripts/content-boundaries.json` 将 `diagnostics` 登记为模块并声明依赖；`pnpm check:content-boundaries` 扫描通过 | 自动测试已通过 | 入口由已合并 PR #19 提供 |
| 验证序列化 | `TestSnapshotRegressionJSONRoundtrip`；core 侧 `contract.test.ts`「rejects malformed success and absent run evidence」 | 自动测试已通过 | — |
| 验证错误分类 | `ClassifyTransport()`；`TestSimulatorRegressionScenarioContracts` | 自动测试已通过 | — |
| 验证未知字段与调用者兼容 | core `contract.test.ts`「accepts additive fields without inventing results」 | 自动测试已通过 | — |
| 验证无全局业务依赖或私有 SQL 入口 | `check-content-boundaries.mjs` 的「公开入口」「未登记模块」规则；13 个自测用例 | 自动测试已通过 | 静态导入检查，**不覆盖 SQL 层所有权**（见 `content-boundary-checks.md` 限制节） |

### DIAG-02 · 脱敏与日志字段策略（验收 D13-V08）

| 条目 | 证据 | 类型 | 备注 |
|---|---|---|---|
| 交付凭据字段准入与脱敏 | `log.go` `Sanitize()`、`safeToken()`、`oneOf()` | 自动测试已通过 | `TestLogRegressionSanitizeRules` |
| 交付请求头脱敏规则 | 未在 `Sanitize()` 中找到按请求头名称的准入/脱敏分支 | **无证据** | `Event` 无请求头字段；脱敏按字段白名单而非按 header 名 |
| 交付路径/URL 脱敏规则 | 未找到针对路径或 URL 的专门脱敏分支 | **无证据** | — |
| 交付私有正文脱敏 | `Event.safe_message` 为固定枚举文案，不承载自由正文 | 代码存在但无测试 | 「不承载正文」由结构保证，未见针对「正文被塞入其他字段」的负例 |
| 供全部 sink 及导出复用 | `SlogHandler` 与 `Store.Technical` 均经 `Sanitize()`；`Export` 复用同一 `Event` | 自动测试已通过 | `TestLogRegressionSlogHandlerLeakingPrevention` |
| 验证嵌套字段 | `TestLogRegressionSanitizeRules` 覆盖字段级规则 | 代码存在但无测试 | 未见**嵌套**结构的负例；`Event` 为扁平结构，嵌套场景不适用但卡片明确要求 |
| 验证异常文本 | `TestSecretsNeverEnterTechnicalLog` | 自动测试已通过 | — |
| 验证模型输出样例 | 未找到以模型输出为输入的脱敏负例 | **无证据** | — |
| 不能只测试顶层 API key | `TestLogRegressionSanitizeRules` 覆盖多字段 | 自动测试已通过 | — |
| 不记录真实凭据 | `TestSecretsNeverEnterTechnicalLog`、`TestLogRegressionSlogHandlerLeakingPrevention` | 自动测试已通过 | — |

### DIAG-03 · 技术日志采集与有界存储（验收 D13-V05/11）

| 条目 | 证据 | 类型 | 备注 |
|---|---|---|---|
| 交付结构化 slog 适配 | `log.go` `SlogHandler` | 自动测试已通过 | `TestLogRegressionSlogHandlerLeakingPrevention` |
| 交付有界缓冲 | `log.go` `LogBuffer{capacity, Dropped, Errors}` | 自动测试已通过 | `TestLogRegressionBufferCapacities`、`TestLogRegressionConcurrentAppendAndEvents` |
| 交付滚动存储/索引 | `store.go` `PruneTechnical()`（按 `received_at` 与 `sequence` 双条件裁剪）；迁移 `468`～`473` | 自动测试已通过 | `TestPostgresAuditRollbackIsolationAndRetention` 覆盖保留裁剪 |
| 交付级别配置 | `Event.severity`；`limits.go` | 代码存在但无测试 | 未见「按级别过滤写入」的配置路径与测试 |
| 交付保留配置 | `store.go` `Store.Retention` / `Store.MaxLogs`；`limits.go` 校验 | 自动测试已通过 | `TestDiagnosticLimitsRejectInvalidConfiguration` |
| 交付丢弃可见状态 | `Metrics.Dropped` / `Metrics.SinkErrors` 进入 `Overview` | 自动测试已通过 | `TestPostgresFullScenariosExportAndHealth` |
| 验证写失败 | `Store.Technical` 失败时 `s.Log.Errors.Add(1)` 而不返回错误 | 自动测试已通过 | handler 侧 `TestContentDiagnosticWritesCoordinateWithWorkspaceDelete/technical_is_dropped_after_delete_commits` |
| 验证磁盘满模拟 | 未找到磁盘满或存储写满的模拟测试 | **无证据** | `store_integration_test.go` 出现 `disk` 字样但非磁盘满场景 |
| 验证截断 | `TestLogRegressionBufferCapacities` | 自动测试已通过 | — |
| 验证持续写入 | `TestLogRegressionConcurrentAppendAndEvents`（`-race`） | 自动测试已通过 | — |
| sink 失败不拖垮普通请求 | `Technical()` 无返回值，失败仅计数 | 自动测试已通过 | 同「验证写失败」行；浏览器侧对照为手动项 V11-10 |

### DIAG-04 · 操作审计存储与一致性（验收 D13-V02/11）

| 条目 | 证据 | 类型 | 备注 |
|---|---|---|---|
| 交付追加审计 | `store.go` `appendAudit()` / `Audit()` | 自动测试已通过 | `TestPostgresAuditRollbackIsolationAndRetention` |
| 交付事务接口 | `withWorkspaceWrite()`（pgx 事务 + `WorkspaceWriteGuard`）；`CommitRun(..., failAudit bool)` | 自动测试已通过 | handler 侧 5 个 `TestContentDiagnosticWritesCoordinateWithWorkspaceDelete` 子用例 |
| 交付 outbox 接口 | 全仓检索 `outbox` 无命中 | **无证据** | 卡片明确写「事务/outbox接口」；当前只有事务，没有 outbox |
| 交付授权查询 | `Store.Query(ctx, scope, filter)`；`Scope.Allows()` | 自动测试已通过 | `TestContentDiagnosticsAuthAndFaultGate` 五个子用例 |
| 技术日志清理不清除审批证据 | `PruneTechnical()` 只 `DELETE FROM content_technical_log` | 自动测试已通过 | `TestPostgresAuditRollbackIsolationAndRetention` 明确断言审计独立保留 |
| 验证成功/失败/回滚事件 | `CommitRun` 的 `failAudit` 分支 | 自动测试已通过 | `TestPostgresAuditRollbackIsolationAndRetention` |
| 验证重复事件 | `Receiver.Receive()` 去重返回 `DUPLICATE` | 自动测试已通过 | `TestSimulatorRegressionReceiverContracts`、`TestWebSocketQueuePropagationDuplicateAndRevocation` |
| 关键审计失败不得伪报业务成功 | `CommitRun` 审计失败即整体回滚 | 自动测试已通过 | `TestContentDiagnosticWritesCoordinateWithWorkspaceDelete/run_and_audit_are_rejected_after_delete_commits` |
| 遵守无 FK / 并发索引迁移约束 | 迁移 `468_content_diagnostics`～`473_content_log_scope` | 代码存在但无测试 | 本次**未**逐文件核对迁移中是否无 `REFERENCES`、索引是否全部 `CONCURRENTLY`；仓库另有 `TestMigrationNumericPrefixesAreUnique` / `TestMigrationFilesHaveMatchingDirections`，但二者不检查这两项约束 |

### DIAG-05 · 贯通异步追踪（验收 D13-V03）

| 条目 | 证据 | 类型 | 备注 |
|---|---|---|---|
| 交付 HTTP trace 传播 | `transport.go` 注释称 `DecodeQueuedEnvelope` 是「HTTP 与 WS 适配器共用的合同」，但 `server/internal/handler/` 与 `server/internal/middleware/` 中检索 `Pack(`/`Unpack(`/`traceparent`/`TraceContext` **均无命中** | **无证据** | 合同存在，HTTP 一侧未接线 |
| 交付队列消息 trace 传播 | `transport.go` `DecodeQueuedEnvelope()` | 自动测试已通过 | `TestWebSocketQueuePropagationDuplicateAndRevocation` |
| 交付 WebSocket/模拟 daemon 传播 | `transport.go` `ServeSimulationTransport()` | 自动测试已通过 | 同上 |
| 交付结果回写 trace | `Run.Events` 携带 `operation_id`/`trace_id` | 代码存在但无测试 | 未见「回写链路」独立用例 |
| 交付重试 attempt | `Envelope.Attempt`；`Event.attempt` | 自动测试已通过 | `TestTracePropagationAndUntrustedIdentity` |
| 交付去重序号 | `Envelope.Sequence`；`Receiver` | 自动测试已通过 | `TestSimulatorRegressionReceiverContracts` |
| 验证父子链 | `Child()`；`TestTracePropagationAndUntrustedIdentity` | 自动测试已通过 | — |
| 验证取消 | 场景 `cancel` → `CANCELLED`；`ClassifyTransport` | 自动测试已通过 | `TestEverySimulatedFaultAndDeterministicTime` |
| 验证重复/迟到 | 场景 `duplicate`/`late` | 自动测试已通过 | 同上 |
| 验证格式非法或伪造 ID | `Unpack()` 拒绝非法 carrier | 自动测试已通过 | `TestTracePropagationAndUntrustedIdentity`（用例名含 `UntrustedIdentity`） |
| ID 不作为授权 | `ServeSimulationTransport` 每条消息重查 `authorize()`，不信任消息内 ID | 自动测试已通过 | `TestWebSocketQueuePropagationDuplicateAndRevocation` 覆盖撤销 |

### DIAG-06 · 前后端结构化错误接入（验收 D13-V04）

| 条目 | 证据 | 类型 | 备注 |
|---|---|---|---|
| 交付统一错误响应 | handler 返回 `{code, trace_id, next_action}` 形状；core `describeDiagnosticError()` 以 zod 校验该形状 | 自动测试已通过 | `TestContentDiagnosticExportRefusesUngrantedAccount`、`TestContentDiagnosticsRejectsMalformedCursorAndSensitiveInput` |
| 交付浏览器错误边界 | `packages/views/content/diagnostics/index.tsx` `class DiagnosticBoundary`；`recordDiagnosticClientError()` | 代码存在但无测试 | `packages/views/content/diagnostics/` 下无测试文件；按 constitution 原则 II 不补 UI 单测 |
| 交付请求错误关联 | `describeDiagnosticError()` 返回 `traceId`；面板文案 `text091 = 追踪编号：` | 代码存在但无测试 | 展示路径无自动测试 |
| 交付步骤及下一动作 | `Event.step`、`Event.next_action`；面板 `text075 = · 下一动作` | 自动测试已通过 | 契约层已测；展示层未测 |
| 诊断详情按权限展示 | `Scope.Allows()` + handler 授权门 | 自动测试已通过 | `TestContentDiagnosticsAuthAndFaultGate` |
| 验证权限错误 | 场景 `denied` → `AUTHORIZATION_DENIED` | 自动测试已通过 | `TestEverySimulatedFaultAndDeterministicTime` |
| 验证文件错误 | 场景 `file_missing` / `file_changed` | 自动测试已通过 | 同上 |
| 验证网络错误 | 场景 `reconnect` → `NETWORK_UNAVAILABLE` | 自动测试已通过 | 同上 |
| 验证 schema 错误 | 场景 `schema` → `OUTPUT_SCHEMA` | 自动测试已通过 | 同上 |
| 验证未捕获 UI 异常的可定位记录 | `DiagnosticBoundary` + `uiFault = 触发页面错误（测试）` 按钮 | 代码存在但无测试 | 需浏览器手验；未列入 `specs/002` 手动清单 |
| 不泄漏敏感堆栈 | `safe_message` 固定枚举 | 自动测试已通过 | `TestSecretsNeverEnterTechnicalLog` |

### DIAG-07 · 健康、心跳与运行指标（验收 D13-V01/11）

| 条目 | 证据 | 类型 | 备注 |
|---|---|---|---|
| 交付组件健康汇总 | `service.go` `Overview()` 汇总 `web/files/daemon/executor/search` 五组件 | 自动测试已通过 | `TestPostgresFullScenariosExportAndHealth` |
| 交付心跳过期 | `Overview()`：`now.Sub(*c.LastSeen) > 30s` → `unavailable` / `heartbeat expired` | 自动测试已通过 | 同上 |
| 交付版本 | `Component.Version`；`Overview.Build` | 自动测试已通过 | 同上 |
| 交付队列统计 | `Metrics.QueueWait` | 代码存在但无测试 | 字段存在；未见断言其取值来源的用例 |
| 交付耗时/错误/丢弃统计 | `Metrics{Count,Errors,P50,P95,Retries,Cancelled,Dropped,SinkErrors}` | 自动测试已通过 | `TestPostgresFullScenariosExportAndHealth` |
| 未连接执行器明确未验证 | 无心跳时 `Status = "unverified"`，`Reason = "No real component heartbeat; simulation is separate"` | 自动测试已通过 | 同上；这是 D13-V01「不把未接入显示成功」的关键实现 |
| 验证组件断开 | 心跳过期分支 | 自动测试已通过 | 同上 |
| 验证慢响应 | `Metrics.P50` / `P95`；场景 `slow` | 自动测试已通过 | `TestEverySimulatedFaultAndDeterministicTime` |
| 验证样本不足 | `Metrics.P95` 为 `*int64`（可空，样本不足时为 null） | 代码存在但无测试 | 未见专门断言「样本不足 → P95 为 null」的用例 |
| 验证时钟偏差 | `Overview()`：`LastSeen` 超前 5 秒 → `unknown` / `clock skew`；场景 `clock_skew` | 自动测试已通过 | `TestEverySimulatedFaultAndDeterministicTime` |
| 验证有限指标标签 | `oneOf()` 限定组件名白名单 | 自动测试已通过 | `TestLogRegressionSanitizeRules` |
| 不默认触发真实模型探测 | `Service.Enabled` 门禁；`transport.go` 非测试环境拒绝 | 自动测试已通过 | `TestTransportNonTestDenied`、`TestSimulationCannotAuthorizeOrReplayHumanActions` |

### DIAG-08 · 授权诊断查询与实时流（验收 D13-V05；卡片标注部分交付，PR #21）

| 条目 | 证据 | 类型 | 备注 |
|---|---|---|---|
| 交付授权过滤 | `Store.Query` + `Scope.Allows()`；handler 授权门 | 自动测试已通过 | `TestContentDiagnosticsAuthAndFaultGate`（`unauthenticated`/`machine`/`cross-workspace`/`account`/`owner`） |
| 交付分页 | `Page{...has_more}`；`Filter` 游标 | 自动测试已通过 | `TestPostgresAuditRollbackIsolationAndRetention` |
| 交付流式游标 | handler `ContentDiagnosticStream`；core `stream-state.ts` | 自动测试已通过 | `TestContentDiagnosticStreamAppliesRequestedFilter`；core「keeps the cursor across pause」 |
| 交付去重 | core `mergeEvents()` | 自动测试已通过 | core「de-duplicates by event id, orders by sequence」 |
| 交付缺口状态 | `gap` 字段；core「keeps a reported gap until the user clears it」 | 自动测试已通过 | — |
| 计划轮换对用户不可见 | handler `rotate=true` 收尾页；core「treats a rotate page as a planned handover」 | 自动测试已通过 | `TestContentDiagnosticStreamClosesPlannedWindowWithRotate` |
| 重连携带过滤条件与游标 | core「carries every filter key and the cursor into the stream query」 | 自动测试已通过 | — |
| 退避重连 | core「backs off from two seconds to a thirty second ceiling」 | 自动测试已通过 | — |
| 授权失败停止重连 | core「stops reconnecting after an authorization failure」；handler 撤销成员资格时无 rotate 结束 | 自动测试已通过 | `TestContentDiagnosticStreamEndsWithoutRotateWhenMembershipIsRevoked` |
| 验证跨品牌/账号拒绝 | `TestContentDiagnosticsAuthAndFaultGate/account` 与 `/cross-workspace` | 自动测试已通过 | 当前 `Scope.Accounts` 为空集合，任何非空 `account_id` 被拒 |
| 验证暂停续读 | core「does not reconnect while paused」「keeps the cursor across pause」 | 自动测试已通过 | — |
| 验证游标过期 | `TestContentDiagnosticsRejectsMalformedCursorAndSensitiveInput`；面板 `text054 = 游标已过期，部分技术日志已清理。` | 自动测试已通过 | — |
| 验证乱序 | core「orders by sequence」 | 自动测试已通过 | — |
| 查询不能直接修改业务状态 | `Query`/`Runs`/`GetRun` 均为只读 SQL | 代码存在但无测试 | 未见「只读」的显式断言用例 |
| D13-V05 浏览器矩阵 | `specs/002-diag-package-stream-recovery/manual-ui-todo.md` V05-1～V05-15 | **无证据** | 15 条全部标「待用户验证」，本次未执行 |

### DIAG-09 · 完整浏览器诊断面板（验收 D13-V01～05）

| 条目 | 证据 | 类型 | 备注 |
|---|---|---|---|
| 交付概览 | `index.tsx` + `text028 = 组件健康`、`text029 = 实例：`、`text030 = · 构建：` | 代码存在但无测试 | views 层无测试文件；按原则 II 不补 UI 单测 |
| 交付操作时间线 | 面板 `audit` 标签；`Event` 时间线渲染 | 代码存在但无测试 | — |
| 交付日志筛选 | `text057 = 清除筛选`、`text052/053 = 开始/结束时间` | 代码存在但无测试 | 筛选**序列化**已由 core 契约测试覆盖，**界面**未测 |
| 交付实时流 | `text058 = 实时日志 · 最近 200 条` | 代码存在但无测试 | 状态机已由 `stream-state.test.ts` 覆盖，界面接线未测 |
| 交付 trace 瀑布 | 未在面板中找到瀑布图或层级可视化实现 | **无证据** | 面板有 `text006 = 查看追踪` 跳转与 `attempt` 文案，但未见瀑布视图 |
| 交付运行步骤入口 | `text089 = 查看运行`、`text060 = 选择运行` | 代码存在但无测试 | — |
| 支持复制追踪编号 | `text068 = 复制追踪编号` | 代码存在但无测试 | — |
| 支持关联跳转 | `text006 = 查看追踪` | 代码存在但无测试 | — |
| 验证从操作到失败步骤定位 | — | **无证据** | 需浏览器手验；未列入 `specs/002` 手动清单 |
| 验证分页/暂停 | 手动清单 V05-13（分页）、V05-7（暂停） | **无证据** | 待用户验证 |
| 验证断线提示 | 手动清单 V05-4～V05-6 | **无证据** | 待用户验证 |
| 验证开发权限 | `TestContentDiagnosticsAuthAndFaultGate` | 自动测试已通过 | 接口层已测；界面层未测 |
| 不依赖 Electron | 诊断页仅挂载于 `apps/web/app/[workspaceSlug]/(dashboard)/diagnostics/page.tsx`；桌面端未挂载 | 自动测试已通过 | `pnpm check:content-boundaries` 禁止 `electron` 导入（用例 1 负例） |

### DIAG-10 · 输入快照与复现清单（验收 D13-V06/10）

| 条目 | 证据 | 类型 | 备注 |
|---|---|---|---|
| 交付版本快照 | `contract.go` `Snapshot`；`Run.Build` | 自动测试已通过 | `TestSnapshotRegressionJSONRoundtrip` |
| 交付授权快照 | `ReproductionGaps(s, current, grants)` 的 `grants` 入参 | 自动测试已通过 | `TestSnapshotImmutableAndRevocation` |
| 交付文件哈希快照 | `Snapshot` 字段 | 自动测试已通过 | `TestSnapshotRegressionReproductionGaps` |
| 交付模型参数快照 | 面板 `text020 = 模型温度 / 超时` | 代码存在但无测试 | 展示文案存在；未见针对模型参数字段的独立断言 |
| 交付复现缺口清单 | `ReproductionGaps()`；`Run.Gaps`；面板 `text023 = 复现缺口：` | 自动测试已通过 | `TestSnapshotRegressionReproductionGaps` |
| 用模拟输入先验证 | `Simulate()` 生成 `Snapshot` | 自动测试已通过 | `TestEverySimulatedFaultAndDeterministicTime` |
| 验证配置不可回写历史 | `CloneSnapshot()` 深拷贝 | 自动测试已通过 | `TestSnapshotRegressionCloneIsolation`、`TestSnapshotRegressionInputImmutability` |
| 验证文件缺失/变化 | 场景 `file_missing` / `file_changed` | 自动测试已通过 | `TestSnapshotRegressionReproductionGaps` |
| 验证授权撤销 | `TestSnapshotImmutableAndRevocation` | 自动测试已通过 | — |
| 不复制媒体 | `Snapshot` 只存哈希与标识，无二进制字段 | 代码存在但无测试 | 由结构保证；未见显式负例 |
| 日志中不保存完整私有资料 | `safe_message` 固定枚举 | 自动测试已通过 | `TestSecretsNeverEnterTechnicalLog` |

### DIAG-11 · 模拟执行器与固定故障场景（验收 D13-V07/10）

| 条目 | 证据 | 类型 | 备注 |
|---|---|---|---|
| 交付可控时间 | `Simulate()` 虚拟时钟；面板 `text080 = 固定种子与虚拟时钟` | 自动测试已通过 | `TestSimulatorRegressionDeterministicTimeAndIdentifiers` |
| 交付可控种子 | `Simulate(..., seed, ...)`；面板 `text082 = 固定种子` | 自动测试已通过 | 同上 |
| 交付需求 §7 全部故障场景 | `simulator.go` `Scenarios` 共 **16** 个：`normal` `slow` `timeout` `cancel` `reconnect` `duplicate` `late` `file_missing` `file_changed` `denied` `database` `model_auth` `model_quota` `schema` `search` `clock_skew` | 自动测试已通过 | `TestEverySimulatedFaultAndDeterministicTime`、`TestSimulatorRegressionScenarioContracts`。**未核对**该 16 个是否等于 `docs/13` §7 的完整列举——§7 原文不在本次读取范围（只读了第 9 节），需主任务对表 |
| 交付隔离复现入口 | handler `ContentDiagnosticSimulate`；`Service.Enabled` 门禁 | 自动测试已通过 | `TestContentDiagnosticsAuthAndFaultGate` |
| 不默认调用真实模型 | `LORETIDE_EXECUTION_POLICY=disabled`；`transport.go` 非测试环境拒绝 | 自动测试已通过 | `TestTransportNonTestDenied`；另有 `pkg/executionpolicy` 门禁测试在 CI |
| 验证场景可重复 | `TestSimulatorRegressionDeterministicTimeAndIdentifiers` | 自动测试已通过 | — |
| 验证取消/迟到/数据库失败可定位 | 场景 `cancel` / `late` / `database` | 自动测试已通过 | `TestEverySimulatedFaultAndDeterministicTime` |
| 非测试环境注入拒绝 | `TestTransportNonTestDenied`、`TestSimulatorRegressionRejectionAndSecurity` | 自动测试已通过 | — |
| 不能执行人工批准或发布动作 | `TestSimulationCannotAuthorizeOrReplayHumanActions` | 自动测试已通过 | — |

### DIAG-12 · 诊断包与保留策略（验收 D13-V08/11；卡片标注部分交付，PR #21）

| 条目 | 证据 | 类型 | 备注 |
|---|---|---|---|
| 交付脱敏导出清单 | `service.go` `Export{Manifest, Run, Audit, Technical, Redacted, Limits}` | 自动测试已通过 | `TestPostgresFullScenariosExportAndHealth` |
| 交付预览 | 面板 `text077 = 预览导出清单`、`text076 = 先预览，再下载到本机。不自动上传诊断包。` | 代码存在但无测试 | 预览**界面**路径无自动测试 |
| 交付权限检查 | handler 导出授权门 | 自动测试已通过 | `TestContentDiagnosticExportRefusesUngrantedAccount` |
| 交付审计与技术日志独立保留/清理 | `PruneTechnical()` 只删技术日志；面板 `text047 = 条；审计独立保留。` | 自动测试已通过 | `TestPostgresAuditRollbackIsolationAndRetention` |
| 交付容量状态 | `Overview.Capacity`、`Overview.RetentionDays` | 自动测试已通过 | `TestPostgresFullScenariosExportAndHealth` |
| 下载保存服务端原始字节与文件名 | handler 设 `Content-Disposition`；`apps/web/platform/content-diagnostics.ts` | 代码存在但无测试 | handler 侧 `TestContentDiagnosticExportDownloadNamesTheFileItWantsSaved` **已通过**；但前端保存路径的测试 `content-diagnostics.test.ts` 按政策不执行 |
| 非授权 run/account 拒绝且不生成文件 | `TestContentDiagnosticExportRefusesUngrantedAccount` | 自动测试已通过 | 「不生成文件」的**浏览器侧**行为为手动项 V08-7 |
| 200 条上限标示 | core `STREAM_EVENT_CAP`；`mergeEvents` 上限 | 自动测试已通过 | core「keeps the newest page of events」 |
| 验证跨账号导出拒绝 | `TestContentDiagnosticExportRefusesUngrantedAccount` | 自动测试已通过 | — |
| 验证密钥和正文负例 | `TestSecretsNeverEnterTechnicalLog`、`TestLogRegressionSanitizeRules` | 自动测试已通过 | 字段级已测；**诊断包成品**的负例为手动项 V08-10 |
| 日志清理不删批准记录 | `PruneTechnical()` | 自动测试已通过 | `TestPostgresAuditRollbackIsolationAndRetention` |
| 不自动上传诊断包 | 代码中导出路径无外发请求 | 代码存在但无测试 | 手动项 V08-8 用 Network 面板核对 |
| D13-V08/V11 浏览器矩阵 | `manual-ui-todo.md` V08-1～V08-11、V11-1～V11-11 | **无证据** | 22 条全部「待用户验证」 |

### DIAG-13 · 回归关联与诊断端到端验收（验收 D13-V09/V12 及全部 D13 基础场景）

| 条目 | 证据 | 类型 | 备注 |
|---|---|---|---|
| 交付面板中的故障复现入口 | 面板 `text079 = 固定故障复现`、`text083 = 运行模拟场景` | 代码存在但无测试 | — |
| 交付回归结果入口 | 面板 `text085 = 回归结果`、`text086`（「通过表示模拟结果符合该故障预期」） | 代码存在但无测试 | — |
| 故障/场景/模块/提交关联 | `Run{Scenario, Module, Build, Original}` 四字段齐备 | 代码存在但无测试 | 字段存在；未见断言四者关联关系的用例 |
| CI 模拟检查 | `.github/workflows/loretide-content.yml` 第 80 行 `go test -race ./internal/content/diagnostics -count=1` | 自动测试已通过 | CI 仅在 push/PR 到 `app-main` 时触发 |
| 浏览器验证报告 | `manual-ui-todo.md` 存在，但 37 条全部「待用户验证」 | **无证据** | 报告尚未产生 |
| 验证修复前故障用例失败、修复后通过 | `Evaluate(run)`：`Actual == Expected` → `passed`，否则 `failed` | 自动测试已通过 | `TestSimulatorRegressionScenarioContracts`。但**「修复前失败→修复后通过」的成对证据**未见 |
| 未执行不显示通过 | 面板 `text007 = 暂无记录。未执行不表示已通过。` | 代码存在但无测试 | 文案存在；未见断言该状态的用例 |
| 浏览器可从操作记录定位技术原因 | — | **无证据** | 需浏览器手验 |
| 浏览器可导出脱敏包 | 手动项 V08-2、V08-3 | **无证据** | 待用户验证 |

## 3. D13-V01～V12 逐子句对照

标注口径：**已自动化**（附用例名）｜**需手动**（附 `specs/002-diag-package-stream-recovery/manual-ui-todo.md` 条目号）｜**无覆盖**。

| ID | 子句 | 状态 |
|---|---|---|
| **V01** | 浏览器概览区分健康/不可用/未配置/未知/未验证 | **需手动**（无对应条目）｜服务端状态机已自动化：`TestPostgresFullScenariosExportAndHealth`（`unverified`/`unavailable`/`unknown` 三分支） |
| V01 | 及最后心跳 | 已自动化 `TestPostgresFullScenariosExportAndHealth`（`LastSeen`）；浏览器展示 **需手动** V11-4 |
| V01 | 不将未接入 Codex 或远程组件显示成功 | 已自动化 `TestPostgresFullScenariosExportAndHealth`（无心跳 → `unverified`） |
| **V02** | 同一操作日志可跳到技术追踪 | **无覆盖**（面板 `text006 = 查看追踪` 存在，无测试、无手动条目） |
| V02 | 可跳到对象版本 | **无覆盖**（`Event.object_version` 字段存在，跳转路径无证据） |
| V02 | 人工/Agent/系统可区分 | 已自动化 — `Event.actor_kind` + `oneOf()` 白名单，`TestLogRegressionSanitizeRules` |
| V02 | 失败不产生虚假成功审计 | 已自动化 `TestContentDiagnosticWritesCoordinateWithWorkspaceDelete/run_and_audit_are_rejected_after_delete_commits`、`TestPostgresAuditRollbackIsolationAndRetention` |
| **V03** | 从请求跨队列到模拟 daemon/工具/回写的 trace 连续 | 部分已自动化 `TestWebSocketQueuePropagationDuplicateAndRevocation`（队列+WS）；**HTTP 一段无覆盖**（handler/middleware 无 trace 传播） |
| V03 | 重试独立 attempt | 已自动化 `TestTracePropagationAndUntrustedIdentity` |
| V03 | 重复去重 | 已自动化 `TestSimulatorRegressionReceiverContracts` |
| V03 | 迟到与缺口可见 | 已自动化 `TestEverySimulatedFaultAndDeterministicTime`（`late`）；缺口 UI **需手动** V11-5～V11-7 |
| **V04** | 授权/文件/网络/模型/schema/数据库错误可区分 | 已自动化 `TestEverySimulatedFaultAndDeterministicTime`（16 场景覆盖全部六类） |
| V04 | 提供有效下一动作 | 已自动化（契约层）`contract.test.ts`；**界面呈现无覆盖** |
| V04 | 不泄漏敏感堆栈 | 已自动化 `TestSecretsNeverEnterTechnicalLog` |
| **V05** | 按品牌账号授权过滤 | 已自动化 `TestContentDiagnosticsAuthAndFaultGate/account`、`/cross-workspace`；浏览器 **需手动** V05-11、V05-12 |
| V05 | 分页 | **需手动** V05-13（服务端分页已由 `TestPostgresAuditRollbackIsolationAndRetention` 覆盖） |
| V05 | 暂停 | 已自动化 `stream-state.test.ts`「does not reconnect while paused」；浏览器 **需手动** V05-7 |
| V05 | 断线恢复 | 已自动化 `stream-state.test.ts`（退避/rotate/续读）；浏览器 **需手动** V05-1～V05-6、V05-9、V05-10 |
| V05 | 保留期 | 已自动化 `TestPostgresAuditRollbackIsolationAndRetention`；浏览器 **需手动** V05-14、V11-3、V11-5～V11-7 |
| V05 | 丢弃提示 | 已自动化 `TestLogRegressionBufferCapacities`（丢弃计数）；浏览器 **需手动** V05-15、V11-2 |
| **V06** | 输入快照固定版本和授权 | 已自动化 `TestSnapshotImmutableAndRevocation`、`TestSnapshotRegressionInputImmutability` |
| V06 | 旧文件缺失明确不可完整复现 | 已自动化 `TestSnapshotRegressionReproductionGaps` |
| V06 | 不新建自动媒体快照 | **无覆盖**（结构上无二进制字段，但无显式负例） |
| **V07** | 固定数据与全部列举故障可独立复现 | 已自动化 `TestEverySimulatedFaultAndDeterministicTime`、`TestSimulatorRegressionDeterministicTimeAndIdentifiers`。**注**：「全部列举」与 `docs/13` §7 的对表未做 |
| V07 | 隔离实例之外注入被拒绝 | 已自动化 `TestTransportNonTestDenied`、`TestSimulatorRegressionRejectionAndSecurity` |
| V07 | 不产生真实发布/审批/指标 | 已自动化 `TestSimulationCannotAuthorizeOrReplayHumanActions` |
| **V08** | 诊断包可预览导出内容 | **需手动** V08-2、V08-3 |
| V08 | 凭据/私有正文不泄漏 | 已自动化（字段级）`TestSecretsNeverEnterTechnicalLog`、`TestLogRegressionSanitizeRules`；**成品包**级 **需手动** V08-6 |
| V08 | 敏感字段负例检查 | 已自动化（字段级）`TestLogRegressionSanitizeRules`；**需手动** V08-10 |
| V08 | 跨账号导出拒绝 | 已自动化 `TestContentDiagnosticExportRefusesUngrantedAccount`；浏览器 **需手动** V08-7、V08-11 |
| **V09** | 回归结果能定位原故障 | **无覆盖**（`Run.Original` 字段存在，无用例、无手动条目） |
| V09 | 定位输入场景 | 已自动化 `TestSimulatorRegressionScenarioContracts`（`Run.Scenario`） |
| V09 | 定位模块 | **无覆盖**（`Run.Module` 字段存在，无断言） |
| V09 | 定位代码版本 | 已自动化 `TestSimulatorRegressionDeterministicTimeAndIdentifiers`（`Run.Build`） |
| V09 | 未运行/失败不会被当通过 | 部分已自动化 `Evaluate()` + `TestSimulatorRegressionScenarioContracts`；「未运行」态仅有面板文案 `text007`，**无覆盖** |
| **V10** | 偏好不受临时恢复影响 | **无覆盖** |
| V10 | 查看日志不触发模型重跑 | 已自动化 `TestTransportNonTestDenied`（非测试环境拒绝）；查询路径只读，**无显式用例** |
| V10 | 真实重跑需重新校验 | **无覆盖**（真实执行器保持禁用，此条尚不可验证） |
| V10 | 不重放人类动作 | 已自动化 `TestSimulationCannotAuthorizeOrReplayHumanActions` |
| **V11** | 日志 sink 失败可见 | 已自动化 `Metrics.SinkErrors` + `TestPostgresFullScenariosExportAndHealth`；浏览器 **需手动** V11-1 |
| V11 | 存储满可见 | **需手动** V11-2、V11-3。**磁盘满模拟无覆盖**（见 DIAG-03） |
| V11 | 心跳过期可见 | 已自动化 `TestPostgresFullScenariosExportAndHealth`；浏览器 **需手动** V11-4 |
| V11 | 有界 | 已自动化 `TestLogRegressionBufferCapacities`、`TestLogRegressionConcurrentAppendAndEvents`；浏览器 **需手动** V11-8、V11-9 |
| V11 | 普通技术日志失败不拖垮业务 | 已自动化 `TestContentDiagnosticWritesCoordinateWithWorkspaceDelete/technical_is_dropped_after_delete_commits`；浏览器 **需手动** V11-10 |
| V11 | 关键审计失败按事务拒绝 | 已自动化 `.../run_and_audit_are_rejected_after_delete_commits`、`/standalone_audit_is_rejected_after_delete_commits`、`TestDeleteWorkspace_PurgesContentDiagnosticsAtomically`；浏览器 **需手动** V11-11 |
| **V12** | 每个后续功能交付时有操作、错误、trace、故障和回归证据 | **无覆盖**（这是对**后续**交付的流程要求，当前无接入合同与检查机制） |
| V12 | 真实 Codex 及远程阶段分别追加实测 | **无覆盖**（真实执行器保持禁用） |
| V12 | 模拟不代替真实通过 | 已自动化（口径层）`Overview` 的 `unverified` 状态 + 面板 `text026`；**无流程级检查** |

## 4. 汇总

### 4.1 每张卡片的三类计数

计数由第 2 节各表逐行统计得出（按「类型」列），非手工累加。

| 卡片 | 自动测试已通过 | 代码存在但无测试 | 无证据 | 合计 |
|---|---:|---:|---:|---:|
| DIAG-01 | 10 | 0 | 0 | 10 |
| DIAG-02 | 5 | 2 | 3 | 10 |
| DIAG-03 | 9 | 1 | 1 | 11 |
| DIAG-04 | 7 | 1 | 1 | 9 |
| DIAG-05 | 9 | 1 | 1 | 11 |
| DIAG-06 | 8 | 3 | 0 | 11 |
| DIAG-07 | 10 | 2 | 0 | 12 |
| DIAG-08 | 13 | 1 | 1 | 15 |
| DIAG-09 | 2 | 7 | 4 | 13 |
| DIAG-10 | 9 | 2 | 0 | 11 |
| DIAG-11 | 9 | 0 | 0 | 9 |
| DIAG-12 | 9 | 3 | 1 | 13 |
| DIAG-13 | 2 | 4 | 3 | 9 |
| **合计** | **102** | **27** | **15** | **144** |

### 4.2 D13-V01～V12 状态

| ID | 状态 | 说明 |
|---|---|---|
| D13-V01 | **部分** | 服务端状态机全部已自动化；浏览器概览区分为手动，未执行 |
| D13-V02 | **部分** | 审计一致性已自动化；「跳到技术追踪 / 对象版本」两条子句无覆盖 |
| D13-V03 | **部分** | 队列 + WebSocket 已自动化；**HTTP 一段无覆盖** |
| D13-V04 | **部分** | 六类错误区分已自动化；界面呈现与下一动作无覆盖 |
| D13-V05 | **部分** | 服务端与 core 状态机已自动化；浏览器 15 条全部待用户验证 |
| D13-V06 | **部分** | 前两条子句已自动化；「不新建自动媒体快照」无显式负例 |
| D13-V07 | **部分** | 三条子句均已自动化；但「全部列举故障」与 `docs/13` §7 的对表**未做** |
| D13-V08 | **部分** | 字段级脱敏与跨账号拒绝已自动化；成品包预览/负例 11 条待用户验证 |
| D13-V09 | **未满足** | 「定位原故障」「定位模块」「未运行不当通过」三条无覆盖 |
| D13-V10 | **未满足** | 四条子句中三条无覆盖；真实重跑因执行器禁用尚不可验证 |
| D13-V11 | **部分** | 服务端有界性与事务拒绝已自动化；浏览器 11 条待用户验证；磁盘满模拟无覆盖 |
| D13-V12 | **未满足** | 流程级接入合同与检查机制均无覆盖 |

**无一条 D13-V 达到「完全满足」。** 12 条中 9 条为「部分」，3 条为「未满足」。

### 4.3 结论

**DG-01 当前不满足，缺口如下：**

1. **D13-V01～V11 未用完整模拟场景验证**——37 条浏览器手动条目（`manual-ui-todo.md` V05/V08/V11）全部处于「待用户验证」，且该清单只覆盖 V05/V08/V11 三条，V01～V04、V06、V07、V09、V10 没有对应的手动清单。
2. **D13-V12 的公共接入合同未通过**——无接入合同、无流程检查，三条子句全部无覆盖。
3. **15 个条目完全无证据**，逐条列出（其中 6 条是浏览器手动矩阵尚未执行，9 条是实现缺失）：

   | 卡片 | 条目 | 性质 |
   |---|---|---|
   | DIAG-02 | 交付请求头脱敏规则 | 实现缺失 |
   | DIAG-02 | 交付路径/URL 脱敏规则 | 实现缺失 |
   | DIAG-02 | 验证模型输出样例 | 测试缺失 |
   | DIAG-03 | 验证磁盘满模拟 | 测试缺失 |
   | DIAG-04 | 交付 outbox 接口 | 实现缺失（卡片明文要求「事务/outbox接口」） |
   | DIAG-05 | 交付 HTTP trace 传播 | 实现缺失 |
   | DIAG-08 | D13-V05 浏览器矩阵 | 手动未执行 |
   | DIAG-09 | 交付 trace 瀑布 | 实现缺失 |
   | DIAG-09 | 验证从操作到失败步骤定位 | 手动未执行 |
   | DIAG-09 | 验证分页/暂停 | 手动未执行 |
   | DIAG-09 | 验证断线提示 | 手动未执行 |
   | DIAG-12 | D13-V08/V11 浏览器矩阵 | 手动未执行 |
   | DIAG-13 | 浏览器验证报告 | 手动未执行 |
   | DIAG-13 | 浏览器可从操作记录定位技术原因 | 手动未执行 |
   | DIAG-13 | 浏览器可导出脱敏包 | 手动未执行 |

   其中影响面最大的是 **DIAG-05 的 HTTP trace 传播未接线**（`handler/`、`middleware/` 中检索不到 `Pack`/`Unpack`/`traceparent`），它直接让 D13-V03「从请求跨队列到模拟 daemon」的首段断裂——队列与 WebSocket 两段都已自动化，唯独入口一段没有。

4. **27 个条目「代码存在但无测试」**——其中 DIAG-09（7 项）与 DIAG-13（4 项）集中在浏览器面板层。按 constitution 原则 II 这些不补 UI 单测，只能由用户手动验收，因此 **DG-01 的出口天然依赖一份尚不存在的完整浏览器验收报告**。
5. **两项对表未做，需主任务在文档仓库完成**：
   - `simulator.go` 的 16 个场景是否等于 `docs/13` §7 的完整列举（本次只读了 §9，未读 §7）；
   - 迁移 `468`～`473` 是否满足「无 FK、索引全部 `CONCURRENTLY`」（仓库现有迁移测试不检查这两项）。

## 5. 本文件的证据是怎么来的

全部命令在 `app-main` `58b5e1a23` 上实际执行，Linux 容器 + 本地 PostgreSQL 16.13（`initdb` 新建，端口 15433，非开发库）。

| 命令 | 退出码 | 结果 |
|---|---:|---|
| `go test ./internal/content/diagnostics -count=1 -race -v` | 0 | 22 个用例全部 PASS，**无 SKIP**（已设 `LORETIDE_DIAG_TEST_DATABASE_URL`，两个 Postgres 集成用例真实执行） |
| `go run ./cmd/migrate up` | 0 | 迁移至 `473_content_log_scope` |
| `go test ./internal/handler -count=1 -run 'TestContentDiagnostic\|TestDeleteWorkspace_PurgesContentDiagnostics' -v` | 0 | 8 个顶层用例 + 11 个子用例全部 PASS |
| `pnpm --filter @multica/core exec vitest run content/diagnostics/contract.test.ts content/diagnostics/stream-state.test.ts` | 0 | 2 文件 15 用例全部通过 |

**未执行**：

- `packages/core/content/diagnostics/queries.test.tsx` 与 `apps/web/platform/content-diagnostics.test.ts`——UI 单测，按 constitution 原则 II 与 `loretide-content.yml` 的显式排除，CI 不跑，本次也不跑。
- 全量 `pnpm test` / `make test` / Playwright——本任务为纯文档，不做全量验证。
- `manual-ui-todo.md` 的 37 条浏览器条目——需真实 Windows 实例与用户操作。

**一处需要注意的陷阱**：`server/internal/handler/handler_test.go` 的 `TestMain` 在数据库连不上时执行 `os.Exit(0)`，即**整个 handler 测试包会以退出码 0 "通过"而实际一个用例都没跑**。本文件引用的 handler 证据均已通过 `-v` 输出逐条确认用例真实执行，未依赖包级退出码。任何后续复核请同样使用 `-v` 核对用例名，不要只看退出码。
