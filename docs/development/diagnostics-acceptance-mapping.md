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
| **自动测试已通过** | 存在自动测试，且本次实际运行并通过（命令与退出码见第 6 节） |
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
| 交付请求头脱敏规则 | `log.go` `headerAdmission()` 三档逐名准入 + 四条后缀模式（`*-token`/`*-secret`/`*-key`/`*-password`），未列入者默认不记；`headersPresent()` 只记名称 | 自动测试已通过 | `TestHeaderAdmissionTiers`、`TestHeadersPresentRecordsNamesOnly`（feature 005） |
| 交付路径/URL 脱敏规则 | `log.go` `requestRoute()` / `safeRoute()`：只记注册时的路由模板 + 方法 + 状态码，不记原始路径、查询串取值与键名、路径哈希；无模板时留空而非回退 | 自动测试已通过 | `TestRequestRouteKeepsThePatternAndNothingElse`、`TestSanitizeDropsWholeValuesRatherThanTrimmingThem`、`TestContentDiagnosticExportCarriesNoRequestSecrets`（feature 005） |
| 交付私有正文脱敏 | `Event.safe_message` 为固定枚举文案，不承载自由正文 | 自动测试已通过 | `TestModelOutputSamplesNeverSurviveSanitize` 把模型输出样本**逐一塞进 13 个字段**（`Step`/`Build`/`Version`/`Message`/`Next`/`Code`/`Component`/`Action`/`Outcome`/`Severity`/`ActorKind`/`Route`/`Upstream`）逐字段断言不残留；`TestLogRegressionSanitizeRules` 另有三处断言 `Message` 被错误码固定文案覆盖，其中一处**注入的正是一段恶意自由文本**。该行原备注要的「正文被塞入其他字段的负例」即此。**测试早于本次核实就存在（feature 005 / #32），本次只补引用**（feature 010） |
| 供全部 sink 及导出复用 | `SlogHandler` 与 `Store.Technical` 均经 `Sanitize()`；`Export` 复用同一 `Event` | 自动测试已通过 | `TestLogRegressionSlogHandlerLeakingPrevention` |
| 验证嵌套字段 | `TestLogRegressionSanitizeRules` 覆盖字段级规则 | 代码存在但无测试 | 未见**嵌套**结构的负例；`Event` 为扁平结构，嵌套场景不适用但卡片明确要求 |
| 验证异常文本 | `TestSecretsNeverEnterTechnicalLog` | 自动测试已通过 | — |
| 验证模型输出样例 | `log_regression_test.go`：5 组真实形状的模型输出样例（回显的 system prompt、带 key 的 provider stdout、provider 栈回溯、含嵌套凭据的 tool-call、带令牌的 markdown 回调 URL）× 13 个 `Sanitize` 负责的字段，断言序列化后的事件不含任一敏感串、且不含换行 | 自动测试已通过 | `TestModelOutputSamplesNeverSurviveSanitize`（65 个子用例）、`TestModelOutputThroughSlogHandlerLeaksNothing`。业务身份字段（`Workspace`/`Account`/`Actor`/`ObjectID`）按设计透传、不在断言范围，见 `TestLogRegressionSanitizeRules` 的「business identity fields preserved」用例 |
| 不能只测试顶层 API key | `TestLogRegressionSanitizeRules` 覆盖多字段 | 自动测试已通过 | — |
| 不记录真实凭据 | `TestSecretsNeverEnterTechnicalLog`、`TestLogRegressionSlogHandlerLeakingPrevention` | 自动测试已通过 | — |

### DIAG-03 · 技术日志采集与有界存储（验收 D13-V05/11）

| 条目 | 证据 | 类型 | 备注 |
|---|---|---|---|
| 交付结构化 slog 适配 | `log.go` `SlogHandler` | 自动测试已通过 | `TestLogRegressionSlogHandlerLeakingPrevention` |
| 交付有界缓冲 | `log.go` `LogBuffer{capacity, Dropped, Errors}` | 自动测试已通过 | `TestLogRegressionBufferCapacities`、`TestLogRegressionConcurrentAppendAndEvents` |
| 交付滚动存储/索引 | `store.go` `PruneTechnical()`（按 `received_at` 与 `sequence` 双条件裁剪）；迁移 `468`～`473` | 自动测试已通过 | `TestPostgresAuditRollbackIsolationAndRetention` 覆盖保留裁剪 |
| 交付级别配置 | `Event.severity` 受 `Sanitize` 的四值枚举收敛（`log.go` `oneOf("debug","info","warn","error")`）；`limits.go` `ConfigureLimits` 的 `MaxLogs`(100..100000) 与 `Retention`(1..90 天) | 自动测试已通过 | `TestLogRegressionSanitizeRules`（非法级别收敛为 `unknown`）、`TestDiagnosticLimitsRejectInvalidConfiguration`（两项配置的边界与拒绝非法值）。**缺口性质更正（feature 010）**：卡片要求的「**按级别过滤写入**」在生产代码中**不存在**——`ConfigureLimits` 只有上述两项，全仓没有任何按 `severity` 过滤写入的分支。**缺的是功能不是测试**，已登记为后续任务，见 §4.3；写测试无法闭合它 |
| 交付保留配置 | `store.go` `Store.Retention` / `Store.MaxLogs`；`limits.go` 校验 | 自动测试已通过 | `TestDiagnosticLimitsRejectInvalidConfiguration` |
| 交付丢弃可见状态 | `Metrics.Dropped` / `Metrics.SinkErrors` 进入 `Overview` | 自动测试已通过 | `TestPostgresFullScenariosExportAndHealth` |
| 验证写失败 | `Store.Technical` 失败时 `s.Log.Errors.Add(1)` 而不返回错误 | 自动测试已通过 | handler 侧 `TestContentDiagnosticWritesCoordinateWithWorkspaceDelete/technical_is_dropped_after_delete_commits` |
| 验证磁盘满模拟 | `store_diskfull_test.go`：在隔离 schema 上用触发器抛 SQLSTATE `53100`（PostgreSQL 自身的 `disk_full`）驱动真实 INSERT 路径，断言有界（内存缓冲与行数均不增长）、可见（`sink_errors` 与 `dropped` 各按失败次数递增）、不拖垮业务（`CommitRun` / `Runs` / `Query` 仍成功），并验证存储恢复后写入恢复且计数停止 | 自动测试已通过 | `TestTechnicalSinkOnFullStorageStaysBoundedVisibleAndNonFatal`、`TestAuditOnFullStorageRejectsTheWriteInsteadOfCounting`（关键审计满盘时整体拒绝而非计数吞掉）。需 `LORETIDE_DIAG_TEST_DATABASE_URL`，未配置时 `t.Skip` |
| 验证截断 | `TestLogRegressionBufferCapacities` | 自动测试已通过 | — |
| 验证持续写入 | `TestLogRegressionConcurrentAppendAndEvents`（`-race`） | 自动测试已通过 | — |
| sink 失败不拖垮普通请求 | `Technical()` 无返回值，失败仅计数 | 自动测试已通过 | 同「验证写失败」行；浏览器侧对照为手动项 V11-10 |

### DIAG-04 · 操作审计存储与一致性（验收 D13-V02/11）

| 条目 | 证据 | 类型 | 备注 |
|---|---|---|---|
| 交付追加审计 | `store.go` `appendAudit()` / `Audit()` | 自动测试已通过 | `TestPostgresAuditRollbackIsolationAndRetention` |
| 交付事务接口 | `withWorkspaceWrite()`（pgx 事务 + `WorkspaceWriteGuard`）；`CommitRun(..., failAudit bool)` | 自动测试已通过 | handler 侧 5 个 `TestContentDiagnosticWritesCoordinateWithWorkspaceDelete` 子用例 |
| 交付 outbox 接口 | `dispatch.go` `Outbox` 接口；两个实现：`MemoryOutbox`（进程内）与 `PostgresOutbox` + `Drainer`（落库，`dispatch_postgres.go` / `drain.go`，表 `content_dispatch_outbox`，迁移 474～476） | 自动测试已通过 | `TestOutbox*` 7 个（feature 005）+ `TestPostgresOutbox*` 8 个、`TestDrainer*` 6 个、`TestCommitRunWithDispatch*` 2 个（feature 009），全部在真实 PostgreSQL 上实际跑过。**「带限制」已去掉**：进程重启不再丢未派发项，`TestPostgresOutboxSurvivesTheProcess` 是 `TestMemoryOutboxDoesNotSurviveTheProcess` 的反面断言，两条并存。关键审计仍走 `Store.Audit` 的事务内路径，**未改动** |
| 交付授权查询 | `Store.Query(ctx, scope, filter)`；`Scope.Allows()` | 自动测试已通过 | `TestContentDiagnosticsAuthAndFaultGate` 五个子用例 |
| 技术日志清理不清除审批证据 | `PruneTechnical()` 只 `DELETE FROM content_technical_log` | 自动测试已通过 | `TestPostgresAuditRollbackIsolationAndRetention` 明确断言审计独立保留 |
| 验证成功/失败/回滚事件 | `CommitRun` 的 `failAudit` 分支 | 自动测试已通过 | `TestPostgresAuditRollbackIsolationAndRetention` |
| 验证重复事件 | `Receiver.Receive()` 去重返回 `DUPLICATE` | 自动测试已通过 | `TestSimulatorRegressionReceiverContracts`、`TestWebSocketQueuePropagationDuplicateAndRevocation` |
| 关键审计失败不得伪报业务成功 | `CommitRun` 审计失败即整体回滚 | 自动测试已通过 | `TestContentDiagnosticWritesCoordinateWithWorkspaceDelete/run_and_audit_are_rejected_after_delete_commits` |
| 遵守无 FK / 并发索引迁移约束 | 迁移 `468_content_diagnostics`～`476_content_dispatch_outbox_due` | 自动测试已通过 | `TestContentMigrationConstraints`（feature 010）逐文件核对编号 ≥ 468 且文件名含 `content_` 的 **18 个迁移文件**：R1 无 `REFERENCES`/`FOREIGN KEY`、R2 无 `CASCADE`、R3 每条 `CREATE INDEX` 必须 `CONCURRENTLY`、R4 含并发索引的文件语句数为 1。判定前剥掉注释与字符串，另有四条负例夹具与一条「注释不误报」用例。**#48 记录的那次人工核对由此变成每次 CI 都跑的断言**（CI 的 `-run` 过滤已加入该测试名，触发段未动） |

### DIAG-05 · 贯通异步追踪（验收 D13-V03）

| 条目 | 证据 | 类型 | 备注 |
|---|---|---|---|
| 交付 HTTP trace 传播 | `middleware/trace.go` `Trace`（全部 API 路由，建本实例 trace、回传 `X-Diagnostic-Trace`）与 `AdoptDaemonTrace`（仅认证后的 daemon 路径采信入站 `traceparent`）；`daemon/client.go` 出站注入 | 自动测试已通过 | `TestTracePropagationAtTheBoundary`、`TestAdoptDaemonTraceOnlyAfterDaemonAuthentication`、`TestContentDiagnosticTraceRunsThroughQueueAndDaemon`（HTTP→queue→daemon 三段同 trace）、`TestTracePropagatesWithoutRecordingOutsideDiagnostics`（传播全局、记录不变）（feature 005） |
| 交付队列消息 trace 传播 | `transport.go` `DecodeQueuedEnvelope()` | 自动测试已通过 | `TestWebSocketQueuePropagationDuplicateAndRevocation` |
| 交付 WebSocket/模拟 daemon 传播 | `transport.go` `ServeSimulationTransport()` | 自动测试已通过 | 同上 |
| 交付结果回写 trace | `Run.Events` 携带 `operation_id`/`trace_id`；`CommitRun` 在同一事务内把它们复制进审计事件 | 自动测试已通过 | `TestCommittedRunWritesBackItsOperationAndTraceIDs`（feature 010，DB 背书）走完「模拟 → 提交落库 → 读回」，断言读回的审计事件的 `trace_id`/`operation_id` **等于**运行自身的值。**断言相等而非非空**——`CommitRun` 落库前执行 `saved.Events = nil`，运行行不存事件，复制若产生一个新随机 id，非空断言依然会绿，而面板上那条追踪将指向不存在的地方 |
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
| 交付队列统计 | `Metrics.QueueWait` | 自动测试已通过 | `TestOverviewQueueWaitCountsOnlyQueueEvents`（feature 010，DB 背书）：3 条 `component=queue`、40ms 的事件 → `QueueWait=120`；再写入 4 条 500ms 的非队列事件后 `QueueWait` **保持 120**，同时样本数升至 7（证明那 4 条确实入库）。**反面断言是关键**——只验正面时，删掉 `component=="queue"` 判断改为全部累加依然会绿 |
| 交付耗时/错误/丢弃统计 | `Metrics{Count,Errors,P50,P95,Retries,Cancelled,Dropped,SinkErrors}` | 自动测试已通过 | `TestPostgresFullScenariosExportAndHealth` |
| 未连接执行器明确未验证 | 无心跳时 `Status = "unverified"`，`Reason = "No real component heartbeat; simulation is separate"` | 自动测试已通过 | 同上；这是 D13-V01「不把未接入显示成功」的关键实现 |
| 验证组件断开 | 心跳过期分支 | 自动测试已通过 | 同上 |
| 验证慢响应 | `Metrics.P50` / `P95`；场景 `slow` | 自动测试已通过 | `TestEverySimulatedFaultAndDeterministicTime` |
| 验证样本不足 | `Metrics.P95` 为 `*int64`（可空，样本不足时为 null） | 自动测试已通过 | `TestOverviewP95IsNullUntilThereAreEnoughSamples`（feature 010，DB 背书）**在阈值两侧各断言一次**：19 个样本 → `P95` 为 null；第 20 个样本写入后 → 非 null。只验一侧等于没验边界——只验 null 侧时把阈值改成 1000 也绿，只验非 null 侧时恒返回一个值也绿 |
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
| 查询不能直接修改业务状态 | `Query`/`Runs`/`GetRun` 均为只读 SQL | 自动测试已通过 | `TestReadPathsDoNotWrite`（feature 010，DB 背书）对 diagnostics **四张表**取调用前后 `count(*)` 快照，断言三条读路径跑完后行数一致。**数行数而不是开只读事务**：只读事务证明的是「那个事务只读」，而这三个方法自己开连接，外层设置管不到它们 |
| D13-V05 浏览器矩阵 | `specs/002-diag-package-stream-recovery/manual-ui-todo.md` V05-1～V05-15 | **无证据** | 15 条全部标「待用户验证」，本次未执行 |

### DIAG-09 · 完整浏览器诊断面板（验收 D13-V01～05）

| 条目 | 证据 | 类型 | 备注 |
|---|---|---|---|
| 交付概览 | `index.tsx` + `text028 = 组件健康`、`text029 = 实例：`、`text030 = · 构建：` | 代码存在但无测试 | views 层无测试文件；按原则 II 不补 UI 单测 |
| 交付操作时间线 | 面板 `audit` 标签；`Event` 时间线渲染 | 代码存在但无测试 | — |
| 交付日志筛选 | `text057 = 清除筛选`、`text052/053 = 开始/结束时间` | 代码存在但无测试 | 筛选**序列化**已由 core 契约测试覆盖，**界面**未测 |
| 交付实时流 | `text058 = 实时日志 · 最近 200 条` | 代码存在但无测试 | 状态机已由 `stream-state.test.ts` 覆盖，界面接线未测 |
| 交付 trace 瀑布 | `packages/core/content/diagnostics/trace-waterfall.ts` `buildTraceWaterfall`：由 `parent_span_id` 构层级、由 `occurred_at` 相对最早值定位、按 `duration_ms` 定宽、超 `STREAM_EVENT_CAP`(200) 时保留含 `error_code` 的 span 及其完整祖先链后折叠。`trace-waterfall.test.ts` 20 条用例覆盖契约七条不变量与边界表八行 | 自动测试已通过 | **派生逻辑**已测；**界面渲染**（缩进、偏移、折叠交互）无自动测试，按原则 II 交手动项 `specs/006` W-1 ～ W-9。2026-09-14 由 #37 交付 |
| 交付运行步骤入口 | `text089 = 查看运行`、`text060 = 选择运行` | 代码存在但无测试 | — |
| 支持复制追踪编号 | `text068 = 复制追踪编号` | 代码存在但无测试 | — |
| 支持关联跳转 | `packages/core/content/diagnostics/linkage.ts` `describeTraceJump(event)`：产出「一组技术日志筛选」或「不可跳转的理由」互斥二选一；追踪编号为空或非 32 位十六进制时恒为 `unavailable`。`linkage.test.ts` 10 条覆盖六行边界表与四条不变量 | 自动测试已通过 | **派生逻辑**已测；面板 `onTrace` 消费它，不可跳转时按钮置灰并说明。**界面呈现**无自动测试，按原则 II 交手动项 `specs/008` J-1 ～ J-4。2026-09-14 由 #008 交付，同时修掉「空追踪编号静默跳到全量日志」 |
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
| 不复制媒体 | `snapshot_media_test.go` 3 条：16 字段清单逐项比对名称与类型、集合元素只许字符串、全结构无字节容器 | 自动测试已通过 | **2026-09-14 由 008 闭合**（FR-012 / FR-013，#50 的第 6 项已预先登记归属）。此前记「由结构保证；未见显式负例」——现在有了：变异验证中加 `Blob []byte` 变红，**加一个合法 `string` 字段也变红**，所以新增字段必须有人显式确认它不是媒体 |
| 日志中不保存完整私有资料 | `safe_message` 固定枚举 | 自动测试已通过 | `TestSecretsNeverEnterTechnicalLog` |

### DIAG-11 · 模拟执行器与固定故障场景（验收 D13-V07/10）

| 条目 | 证据 | 类型 | 备注 |
|---|---|---|---|
| 交付可控时间 | `Simulate()` 虚拟时钟；面板 `text080 = 固定种子与虚拟时钟` | 自动测试已通过 | `TestSimulatorRegressionDeterministicTimeAndIdentifiers` |
| 交付可控种子 | `Simulate(..., seed, ...)`；面板 `text082 = 固定种子` | 自动测试已通过 | 同上 |
| 交付需求 §7 全部故障场景 | `simulator.go` `Scenarios` 共 **16** 个：`normal` `slow` `timeout` `cancel` `reconnect` `duplicate` `late` `file_missing` `file_changed` `denied` `database` `model_auth` `model_quota` `schema` `search` `clock_skew` | 自动测试已通过 | `TestEverySimulatedFaultAndDeterministicTime`、`TestSimulatorRegressionScenarioContracts`。**对表已完成**（主任务 2026-09-14 于 `app-main` `c9cc63eca` 核对）：`docs/13` §7 列举的 **15 项全部在这 16 个之内**，多出的一个是 `clock_skew`。**超集不是缺口**——多一个场景不会让「全部列举故障可独立复现」不成立 |
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
| 下载保存服务端原始字节与文件名 | handler 设 `Content-Disposition`；`apps/web/platform/content-diagnostics.ts` | 自动测试已通过 | `TestContentDiagnosticExportDownloadNamesTheFileItWantsSaved` 断言 `Content-Disposition` 以 `attachment;` 开头且含 `filename=`、响应体是服务端原样的 bundle、`redacted` 为 true、保留服务端的 wire key 名。**类型更正（feature 010）**：该行此前类型记「代码存在但无测试」而备注已写着「已通过」，**类型与备注自相矛盾**；测试早于本次核实就存在，本次只改类型与引用。前端保存路径的测试 `content-diagnostics.test.ts` 仍按 constitution 原则 II 不执行，**那一段没有自动覆盖** |
| 非授权 run/account 拒绝且不生成文件 | `TestContentDiagnosticExportRefusesUngrantedAccount` | 自动测试已通过 | 「不生成文件」的**浏览器侧**行为为手动项 V08-7 |
| 200 条上限标示 | core `STREAM_EVENT_CAP`；`mergeEvents` 上限 | 自动测试已通过 | core「keeps the newest page of events」 |
| 验证跨账号导出拒绝 | `TestContentDiagnosticExportRefusesUngrantedAccount` | 自动测试已通过 | — |
| 验证密钥和正文负例 | `TestSecretsNeverEnterTechnicalLog`、`TestLogRegressionSanitizeRules` | 自动测试已通过 | 字段级已测；**诊断包成品**的负例为手动项 V08-10 |
| 日志清理不删批准记录 | `PruneTechnical()` | 自动测试已通过 | `TestPostgresAuditRollbackIsolationAndRetention` |
| 不自动上传诊断包 | 代码中导出路径无外发请求 | 自动测试已通过 | `pnpm check:diagnostics-no-upload`（feature 010）静态核对导出链路**显式四文件**（`apps/web/platform/content-diagnostics.ts`、`packages/views/content/diagnostics/index.tsx`、`packages/core/content/diagnostics/queries.ts`、诊断页 `page.tsx`）不出现 `fetch`/`XMLHttpRequest`/`sendBeacon`/`new WebSocket`。**`packages/core/api/client.ts` 刻意排除**——下载经 `api.contentDiagnosticDownload` → `fetchRaw` 走共享客户端，那是合法同源调用。匹配按**词边界**（面板现有 4 处 `refetch()`，朴素子串匹配会全部误报）。清单内任一文件缺失即失败，不会静默变绿。手动项 V08-8 保留 |
| D13-V08/V11 浏览器矩阵 | `manual-ui-todo.md` V08-1～V08-11、V11-1～V11-11 | **无证据** | 22 条全部「待用户验证」 |

### DIAG-13 · 回归关联与诊断端到端验收（验收 D13-V09/V12 及全部 D13 基础场景）

| 条目 | 证据 | 类型 | 备注 |
|---|---|---|---|
| 交付面板中的故障复现入口 | 面板 `text079 = 固定故障复现`、`text083 = 运行模拟场景` | 代码存在但无测试 | — |
| 交付回归结果入口 | 面板 `text085 = 回归结果`、`text086`（「通过表示模拟结果符合该故障预期」） | 代码存在但无测试 | — |
| 故障/场景/模块/提交关联 | `packages/core/content/diagnostics/regression.ts` `describeRunLinkage(run, readableRunIds)`：四项定位各带可用性状态，`originalRun` 分 `self` / `linkable` / `unreadable` / `missing`（后者按 `/^[0-9a-f]{32}$/` 判定）。`regression.test.ts` 8 条用例，含「`self` 与 `missing` 的返回结果不相等」 | 自动测试已通过 | 从字段齐备变为**关联关系被断言**。浏览器跳转 **需手动** `specs/006` L-1 ～ L-5。2026-09-14 由 #37 交付 |
| CI 模拟检查 | `.github/workflows/loretide-content.yml` 第 80 行 `go test -race ./internal/content/diagnostics -count=1` | 自动测试已通过 | CI 仅在 push/PR 到 `app-main` 时触发 |
| 浏览器验证报告 | 两份 `manual-ui-todo.md` 存在，但 57 条全部「待用户验证」 | **无证据** | 报告尚未产生 |
| 验证修复前故障用例失败、修复后通过 | `Evaluate(run)`：`Actual == Expected` → `passed`，否则 `failed` | 自动测试已通过 | `TestSimulatorRegressionScenarioContracts`。但**「修复前失败→修复后通过」的成对证据**未见 |
| 未执行不显示通过 | `regression.ts` `describeRegressionVerdict`：四态判定，后端初始值 `"not_run"`（`simulator.go:15`）与空串同判为「未运行」；`regression.test.ts` 含不变量用例「遍历 7 × 5 组输入，断言除 `passed` 行外无一返回 `passed`」 | 自动测试已通过 | 面板此前把原始 token `not_run` 直接渲染进判定列；现为显式四态。文案 `text007` 是列表为空提示，与本条是两件事。2026-09-14 由 #37 交付 |
| 浏览器可从操作记录定位技术原因 | — | **无证据** | 需浏览器手验 |
| 浏览器可导出脱敏包 | 手动项 V08-2、V08-3 | **无证据** | 待用户验证 |
| 公共接入合同 | `docs/development/diagnostics-onboarding-contract.md`（第 2 节 7 行三列「要做什么 / 公共入口 / 怎么算做到了」）+ `scripts/check-diagnostics-contract.mjs`（26 用例）+ PR 模板两条 + `loretide-content.yml` 一个 run 步骤 | 自动测试已通过 | 合同第二列的 **23 个公共入口**由检查的提供方规则 **P2** 机器核对——改名或删掉其中一个，检查变红；另有一条用例反向核对这 23 个名字都出现在合同文本里，两边不会单方面漂移。**检查只证明痕迹存在**，不证明接入语义正确，合同第 5 节写明了这一点 |

## 3. D13-V01～V12 逐子句对照

标注口径：**已自动化**（附用例名）｜**需手动**（附 `specs/002-diag-package-stream-recovery/manual-ui-todo.md` 条目号）｜**无覆盖**。

| ID | 子句 | 状态 |
|---|---|---|
| **V01** | 浏览器概览区分健康/不可用/未配置/未知/未验证 | **需手动**（无对应条目）｜服务端状态机已自动化：`TestPostgresFullScenariosExportAndHealth`（`unverified`/`unavailable`/`unknown` 三分支） |
| V01 | 及最后心跳 | 已自动化 `TestPostgresFullScenariosExportAndHealth`（`LastSeen`）；浏览器展示 **需手动** V11-4 |
| V01 | 不将未接入 Codex 或远程组件显示成功 | 已自动化 `TestPostgresFullScenariosExportAndHealth`（无心跳 → `unverified`） |
| **V02** | 同一操作日志可跳到技术追踪 | 已自动化（派生层）`describeTraceJump`，`linkage.test.ts` 10 条：追踪齐备→技术日志筛选；追踪为空→`unavailable` 且**断言不返回 filter**；不退化为按运行号跳转；清空其余筛选与游标。浏览器 **需手动** `specs/008` J-1 ～ J-4。**2026-09-14**：此前记「无覆盖」不准确，跳转当时已存在于面板内联 setter，缺的是可测性与空值处理 |
| V02 | 可跳到对象版本 | 已自动化（派生层）`describeObjectVersions`，`linkage.test.ts` 8 条：按对象类型+标识归拢、去重保序、计数守恒、同标识不同类型不合并、无版本时不伪造。**限于已取回的那一页事件**（服务端筛选无对象维度，不新增读取路径），跨页看不全，浏览器 **需手动** `specs/008` O-1 ～ O-3 |
| V02 | 人工/Agent/系统可区分 | 已自动化 — `Event.actor_kind` + `oneOf()` 白名单，`TestLogRegressionSanitizeRules` |
| V02 | 失败不产生虚假成功审计 | 已自动化 `TestContentDiagnosticWritesCoordinateWithWorkspaceDelete/run_and_audit_are_rejected_after_delete_commits`、`TestPostgresAuditRollbackIsolationAndRetention` |
| **V03** | 从请求跨队列到模拟 daemon/工具/回写的 trace 连续 | 已自动化。HTTP 入口：`middleware/trace.go` `Trace` 全局挂载（`router.go:1295`），`TestTracePropagationAtTheBoundary` / `TestTraceMintsADistinctTracePerRequest`；队列 + WS：`TestWebSocketQueuePropagationDuplicateAndRevocation`；端到端：handler 侧 `TestContentDiagnosticTraceRunsThroughQueueAndDaemon`、`TestContentDiagnosticBoundaryReportsTheTraceItRanUnder`。**2026-09-14 更新**：此前记「HTTP 一段无覆盖」，已由 #30 闭合 |
| V03 | 重试独立 attempt | 已自动化 `TestTracePropagationAndUntrustedIdentity` |
| V03 | 重复去重 | 已自动化 `TestSimulatorRegressionReceiverContracts` |
| V03 | 迟到与缺口可见 | 已自动化 `TestEverySimulatedFaultAndDeterministicTime`（`late`）；缺口 UI **需手动** V11-5～V11-7 |
| **V04** | 授权/文件/网络/模型/schema/数据库错误可区分 | 已自动化 `TestEverySimulatedFaultAndDeterministicTime`（16 场景覆盖全部六类） |
| V04 | 提供有效下一动作 | 已自动化（判定层）`next_action_test.go` 4 条：16 个场景码**逐码写明期望动作**（新增未决码即失败，不靠「结果在集合内」——那样的断言对默认分支恒真）、`check_registered_file` 分支、默认与空码回退非空、动作与 `retryable` 成对。四处变异验证均变红。**界面呈现仍无覆盖**（原则 II） |
| V04 | 不泄漏敏感堆栈 | 已自动化 `TestSecretsNeverEnterTechnicalLog` |
| **V05** | 按品牌账号授权过滤 | 已自动化 `TestContentDiagnosticsAuthAndFaultGate/account`、`/cross-workspace`；浏览器 **需手动** V05-11、V05-12 |
| V05 | 分页 | **需手动** V05-13（服务端分页已由 `TestPostgresAuditRollbackIsolationAndRetention` 覆盖） |
| V05 | 暂停 | 已自动化 `stream-state.test.ts`「does not reconnect while paused」；浏览器 **需手动** V05-7 |
| V05 | 断线恢复 | 已自动化 `stream-state.test.ts`（退避/rotate/续读）；浏览器 **需手动** V05-1～V05-6、V05-9、V05-10 |
| V05 | 保留期 | 已自动化 `TestPostgresAuditRollbackIsolationAndRetention`；浏览器 **需手动** V05-14、V11-3、V11-5～V11-7 |
| V05 | 丢弃提示 | 已自动化 `TestLogRegressionBufferCapacities`（丢弃计数）；浏览器 **需手动** V05-15、V11-2 |
| **V06** | 输入快照固定版本和授权 | 已自动化 `TestSnapshotImmutableAndRevocation`、`TestSnapshotRegressionInputImmutability` |
| V06 | 旧文件缺失明确不可完整复现 | 已自动化 `TestSnapshotRegressionReproductionGaps` |
| V06 | 不新建自动媒体快照 | 已自动化 `snapshot_media_test.go` 3 条：16 字段清单逐项比对名称与类型、集合元素只许字符串、全结构无字节容器。变异验证：加 `Blob []byte` 变红，**加一个合法 `string` 字段也变红**——字段清单相对「只查类型」的价值正在于此 |
| **V07** | 固定数据与全部列举故障可独立复现 | 已自动化 `TestEverySimulatedFaultAndDeterministicTime`、`TestSimulatorRegressionDeterministicTimeAndIdentifiers`。**「全部列举」的对表已完成**（主任务 2026-09-14 于 `app-main` `c9cc63eca` 核对）：`docs/13` §7 的 15 项全部落在 `Scenarios` 的 16 个里，多出 `clock_skew` |
| V07 | 隔离实例之外注入被拒绝 | 已自动化 `TestTransportNonTestDenied`、`TestSimulatorRegressionRejectionAndSecurity` |
| V07 | 不产生真实发布/审批/指标 | 已自动化 `TestSimulationCannotAuthorizeOrReplayHumanActions` |
| **V08** | 诊断包可预览导出内容 | **需手动** V08-2、V08-3 |
| V08 | 凭据/私有正文不泄漏 | 已自动化（字段级）`TestSecretsNeverEnterTechnicalLog`、`TestLogRegressionSanitizeRules`；**成品包**级 **需手动** V08-6 |
| V08 | 敏感字段负例检查 | 已自动化（字段级）`TestLogRegressionSanitizeRules`；**需手动** V08-10 |
| V08 | 跨账号导出拒绝 | 已自动化 `TestContentDiagnosticExportRefusesUngrantedAccount`；浏览器 **需手动** V08-7、V08-11 |
| **V09** | 回归结果能定位原故障 | 已自动化 `regression.test.ts` 的 `describeRunLinkage` 四态（`self` / `linkable` / `unreadable` / `missing`），含「`self` 与 `missing` 不得相等」断言；浏览器跳转 **需手动** `specs/006` L-1 ～ L-3、L-5 |
| V09 | 定位输入场景 | 已自动化 `TestSimulatorRegressionScenarioContracts`（`Run.Scenario`） |
| V09 | 定位模块 | 已自动化 `regression.test.ts`「marks empty scenario, module and build as missing」「passes present locators through unchanged」；浏览器 **需手动** `specs/006` L-1、L-4 |
| V09 | 定位代码版本 | 已自动化 `TestSimulatorRegressionDeterministicTimeAndIdentifiers`（`Run.Build`） |
| V09 | 未运行/失败不会被当通过 | 已自动化 `regression.test.ts` 的四态真值表六行 + 不变量用例「遍历 7 × 5 组输入，断言除 `passed` 行外无一返回 `passed`」；浏览器 **需手动** `specs/006` V-1 ～ V-5。**2026-09-14 更新**：此前「未运行」态只有面板文案，现由 `describeRegressionVerdict` 覆盖 |
| **V10** | 偏好不受临时恢复影响 | 已自动化 `reproduce_preference_test.go` 3 条，原运行偏好用**非夹具值** `strict-sources-only`（夹具恒为 `"all"`，用它比对分不清「真的没改」与「碰巧相等」），复现后重读原运行断言偏好与其余快照输入均未变。**注**：该性质目前**由存储层无运行更新路径保证**（`CommitRun` 拒绝重复 `run_id`），并非复现逻辑自觉 |
| V10 | 查看日志不触发模型重跑 | 已自动化 `TestTransportNonTestDenied`（非测试环境拒绝）；查询路径只读，**无显式用例** |
| V10 | 真实重跑需重新校验 | **无覆盖**（真实执行器保持禁用，此条尚不可验证） |
| V10 | 不重放人类动作 | 已自动化 `TestSimulationCannotAuthorizeOrReplayHumanActions` |
| **V11** | 日志 sink 失败可见 | 已自动化 `Metrics.SinkErrors` + `TestPostgresFullScenariosExportAndHealth`；浏览器 **需手动** V11-1 |
| V11 | 存储满可见 | 已自动化 `store_diskfull_test.go`：`TestTechnicalSinkOnFullStorageStaysBoundedVisibleAndNonFatal`（技术日志有界、可见、不致命）、`TestAuditOnFullStorageRejectsTheWriteInsteadOfCounting`（审计按事务拒绝而非计数）；浏览器 **需手动** V11-2、V11-3。**2026-09-14 更新**：此前记「磁盘满模拟无覆盖」，已由 #32 闭合 |
| V11 | 心跳过期可见 | 已自动化 `TestPostgresFullScenariosExportAndHealth`；浏览器 **需手动** V11-4 |
| V11 | 有界 | 已自动化 `TestLogRegressionBufferCapacities`、`TestLogRegressionConcurrentAppendAndEvents`；浏览器 **需手动** V11-8、V11-9 |
| V11 | 普通技术日志失败不拖垮业务 | 已自动化 `TestContentDiagnosticWritesCoordinateWithWorkspaceDelete/technical_is_dropped_after_delete_commits`；浏览器 **需手动** V11-10 |
| V11 | 关键审计失败按事务拒绝 | 已自动化 `.../run_and_audit_are_rejected_after_delete_commits`、`/standalone_audit_is_rejected_after_delete_commits`、`TestDeleteWorkspace_PurgesContentDiagnosticsAtomically`；浏览器 **需手动** V11-11 |
| **V12** | 每个后续功能交付时有操作、错误、trace、故障和回归证据 | 已自动化（流程层）——`docs/development/diagnostics-onboarding-contract.md` 第 2 节逐面写明入口与判定，`pnpm check:diagnostics-contract` 对每个**已落地**模块判 E1/E2/E3 并点名缺项（26 用例，含三条「分别只缺一条」负例）。合同在模块落地时才生效：今天 12 个模块中 11 个无目录，检查对它们沉默 |
| V12 | 真实 Codex 及远程阶段分别追加实测 | **无覆盖**，且在执行器禁用期间**不可能通过**（constitution 原则 IX，`executionpolicy.Check()` 恒返回 `ErrDisabled`）。合同第 4 节把这一栏固定记为「未执行」，使它不会被静默跳过 |
| V12 | 模拟不代替真实通过 | 已自动化（口径层）`Overview` 的 `unverified` 状态 + 面板 `text026`；**流程级已补**——合同第 4 节把证据分「模拟可得 / 仅真实执行器可得」两栏，PR 模板新增一条勾选项要求分列填写。两处都无自动测试，属流程约定 |

## 4. 汇总

### 4.1 每张卡片的三类计数

计数由第 2 节各表逐行统计得出（按「类型」列），非手工累加。

| 卡片 | 自动测试已通过 | 代码存在但无测试 | 无证据 | 合计 |
|---|---:|---:|---:|---:|
| DIAG-01 | 10 | 0 | 0 | 10 |
| DIAG-02 | 9 | 1 | 0 | 10 |
| DIAG-03 | 11 | 0 | 0 | 11 |
| DIAG-04 | 9 | 0 | 0 | 9 |
| DIAG-05 | 11 | 0 | 0 | 11 |
| DIAG-06 | 8 | 3 | 0 | 11 |
| DIAG-07 | 12 | 0 | 0 | 12 |
| DIAG-08 | 14 | 0 | 1 | 15 |
| DIAG-09 | 4 | 6 | 3 | 13 |
| DIAG-10 | 10 | 1 | 0 | 11 |
| DIAG-11 | 9 | 0 | 0 | 9 |
| DIAG-12 | 11 | 1 | 1 | 13 |
| DIAG-13 | 5 | 2 | 3 | 10 |
| **合计** | **123** | **14** | **8** | **145** |

### 4.2 D13-V01～V12 状态

| ID | 状态 | 说明 |
|---|---|---|
| D13-V01 | **部分** | 服务端状态机全部已自动化；浏览器概览区分为手动，未执行 |
| D13-V02 | **部分** | 四条子句的**派生层全部已自动化**（跳到技术追踪、跳到对象版本由 `linkage.test.ts` 覆盖；审计一致性与角色区分此前已有）；**浏览器侧仍全部手动**（`specs/008` J-1 ～ J-4、O-1 ～ O-3，7 条待用户验证）。**2026-09-14**：两条此前记「无覆盖」的子句由 008 闭合 |
| D13-V03 | **部分** | 四条子句全部已自动化（HTTP 入口、队列、WS、attempt / 去重 / 迟到）；缺口 UI 与浏览器连续性仍为手动。**2026-09-14**：HTTP 一段由 #30 闭合 |
| D13-V04 | **部分** | 六类错误区分与**下一动作有效性判定**均已自动化（`next_action_test.go`，含穷尽性与成对断言）；**界面呈现无覆盖**，按原则 II 只能手动。**2026-09-14**：下一动作一条由 008 闭合 |
| D13-V05 | **部分** | 服务端与 core 状态机已自动化；浏览器 15 条全部待用户验证 |
| D13-V06 | **部分** | 三条子句全部已自动化，含「不新建自动媒体快照」的字段清单断言。**2026-09-14**：由 008 闭合。仍记「部分」——复现清单的浏览器呈现未验，且新发现「复现不还原原始输入」（见 §5 后续条目） |
| D13-V07 | **部分** | 三条子句均已自动化，**且「全部列举故障」与 `docs/13` §7 的对表已完成**（15 项全覆盖，`Scenarios` 多一个 `clock_skew`）。仍记「部分」而非「满足」的唯一原因是 **V07 没有对应的浏览器手动清单**，见 §4.3 第 1 条 |
| D13-V08 | **部分** | 字段级脱敏与跨账号拒绝已自动化；成品包预览/负例 11 条待用户验证 |
| D13-V09 | **部分** | 五条子句全部由 `regression.test.ts` 的纯函数覆盖（定位原故障 / 场景 / 模块 / 代码版本 / 未运行不当通过）；**浏览器侧仍全部手动**（`specs/006` V-1 ～ V-6、L-1 ～ L-5，11 条待用户验证）。**2026-09-14**：由「未满足」改为「部分」，因 #37 补齐了三条此前无覆盖的子句 |
| D13-V10 | **部分** | 四条子句中**三条已自动化**（偏好不受复现影响、查看日志不触发重跑、不重放人类动作）；第四条「真实重跑需重新校验」因执行器按原则 IX 保持禁用**尚不可验证**。**2026-09-14**：由「未满足」改为「部分」，因 008 闭合了此前无覆盖的「偏好不受临时恢复影响」 |
| D13-V11 | **部分** | 服务端有界性、事务拒绝与磁盘满均已自动化；浏览器 11 条待用户验证。**2026-09-14**：磁盘满由 #32 闭合 |
| D13-V12 | **部分** | 第一、三条子句已由接入合同与 `check:diagnostics-contract` 覆盖；**第二条子句（真实 Codex 及远程阶段实测）在执行器禁用期间不可能通过**，因此本条不得记为整体满足 |

**无一条 D13-V 达到「完全满足」。** 12 条**全部为「部分」**，不再有「未满足」——2026-09-14 由 008 把最后一条 V10 从「未满足」提到「部分」。

**「没有未满足」不等于接近达标。** 所有「部分」都卡在同一处：服务端与纯函数层的自动覆盖已相当完整，而**浏览器侧 64 条手动条目一条未执行**。按 constitution 原则 II，界面行为只能由用户确认——这是 DG-01 出口的结构性约束，不是可以靠再写测试消除的缺口。

### 4.3 结论

**DG-01 当前不满足，缺口如下：**

1. **D13-V01～V11 未用完整模拟场景验证**——浏览器手动条目现有 **64 条**，全部处于「待用户验证」，一条未执行：`specs/002-diag-package-stream-recovery/manual-ui-todo.md` 37 条（V05 / V08 / V11）、`specs/006-diag-trace-waterfall-regression/manual-ui-todo.md` 20 条（瀑布 W-1 ～ W-9、四态 V-1 ～ V-6、关联 L-1 ～ L-5）、`specs/008-diag-linkage-and-invariants/manual-ui-todo.md` 7 条（跳转 J-1 ～ J-4、对象版本 O-1 ～ O-3）。三份清单合计覆盖 V02 / V05 / V08 / V09 / V11 五条；**V01、V03、V04、V06、V07、V10 仍没有对应的手动清单**。
2. **D13-V12 的公共接入合同已交付，但 V12 仍不整体满足**——接入合同与 PR 层检查已落地（三条子句中第一、三条有覆盖），但第二条子句「真实 Codex 及远程阶段分别追加实测」在执行器禁用期间**不可能通过**。合同的职责是让这件事**可见**（第 4 节固定记为「未执行」），不是让它通过。
3. **8 个条目完全无证据**，逐条列出（全部是浏览器手动矩阵尚未执行）。下表保留已闭合行的历史，删除线即表示不再计入：

   | 卡片 | 条目 | 性质 |
   |---|---|---|
   | ~~DIAG-02~~ | ~~交付请求头脱敏规则~~ | **已闭合**（feature 005） |
   | ~~DIAG-02~~ | ~~交付路径/URL 脱敏规则~~ | **已闭合**（feature 005） |
   | ~~DIAG-02~~ | ~~验证模型输出样例~~ | **已闭合**（2026-09-14 测试补齐） |
   | ~~DIAG-03~~ | ~~验证磁盘满模拟~~ | **已闭合**（2026-09-14 测试补齐） |
   | ~~DIAG-04~~ | ~~交付 outbox 接口~~ | **已闭合**（feature 005 交接口 + 进程内实现；**feature 009 补上落库实现与排水器，限制取消**） |
   | ~~DIAG-05~~ | ~~交付 HTTP trace 传播~~ | **已闭合**（feature 005） |
   | DIAG-08 | D13-V05 浏览器矩阵 | 手动未执行 |
   | DIAG-09 | 验证从操作到失败步骤定位 | 手动未执行 |
   | DIAG-09 | 验证分页/暂停 | 手动未执行 |
   | DIAG-09 | 验证断线提示 | 手动未执行 |
   | DIAG-12 | D13-V08/V11 浏览器矩阵 | 手动未执行 |
   | DIAG-13 | 浏览器验证报告 | 手动未执行 |
   | DIAG-13 | 浏览器可从操作记录定位技术原因 | 手动未执行 |
   | DIAG-13 | 浏览器可导出脱敏包 | 手动未执行 |

   （历史）其中影响面最大的是 **DIAG-05 的 HTTP trace 传播未接线**，它直接让 D13-V03「从请求跨队列到模拟 daemon」的首段断裂。该项已于 2026-09-14 闭合。

   > **2026-09-14 更新（一）`specs/005-diag-trace-and-sanitize` 实施后**：**4 行实现缺失已闭合**（DIAG-02 两条、DIAG-04 一条、DIAG-05 一条），见各卡片表内的证据列。
   >
   > DIAG-04 当时的闭合**带限制**：交付的是接口与进程内实现，进程退出会丢失未派发项。**该限制已于 2026-09-14 更新（四）取消**，见下。
   >
   > **2026-09-14 更新（二）测试缺口补齐**：**2 行测试缺失已闭合**——DIAG-02「验证模型输出样例」与 DIAG-03「验证磁盘满模拟」，用例名见各卡片表。两者都只加测试，未改生产代码。
   >
   > 剩余 **8 条无证据全部是浏览器手动矩阵**（DIAG-08 一条、DIAG-09 三条、DIAG-12 一条、DIAG-13 三条），按 constitution 原则 II 只能由用户手动验收。
   >
   > **§4.1 计数同步**：更新（一）当时只改了卡片表、未同步 §4.1，本次一并按第 2 节各行重新统计，结果为 **108 / 28 / 8**（此前表中为 102 / 28 / 14）。
   >
   > **前两次更新都不改动任何 D13-V 的状态**：§4.2 的 12 条按原样。已有代码不等于已验收，浏览器矩阵未逐项通过之前 DG-01 仍不满足。
   >
   > **2026-09-14 更新（三）`specs/007-diag-delivery-contract` 实施后**：DIAG-13 新增一行「公共接入合同」（自动测试已通过），§4.1 合计随之为 **109 / 28 / 8**（共 145）。
   >
   > 这是**唯一一次改动某条 D13-V 状态**的更新：**D13-V12 由「未满足」改为「部分」**。改的理由只有一条——第一、三条子句现在有了可执行的覆盖（合同文本 + 静态检查 + PR 模板两条勾选项）。**它没有变成「满足」，而且在当前政策下也不可能变成「满足」**：第二条子句要求真实 Codex 与远程阶段实测，而真实执行器按 constitution 原则 IX 保持禁用。
   >
   > **这一项刻意留着不闭合。** 合同第 4 节要求这一栏固定写「未执行（constitution 原则 IX）」，不写「N/A」、不留空、不用「暂不适用」代替——这三种写法都会在汇总时被当成没有问题。任何汇总口径若能在真实执行器没跑的情况下显示全绿，那个口径是错的。
   >
   > **本次交付不改动任何生产代码**：新增两个脚本文件与一份合同文档，改动 `package.json`、PR 模板、CI 工作流各一处，本文件回写。`server/` 与 `packages/` 下无一行改动，无新增迁移。
   >
   > **2026-09-15 更新（五）`specs/010-diag-evidence-gaps` 实施后**：§2 中**非界面**的「代码存在但无测试」行全部处理完毕，**9 行**转为「自动测试已通过」，§4.1 按第 2 节逐行重新统计为 **121 / 16 / 8**（总数仍 145）。
   >
   > **其中 3 行没有写新测试**：DIAG-02「交付私有正文脱敏」与 DIAG-12「下载保存服务端原始字节与文件名」**早就有测试，只是本文件没引用**（前者是 #32 的 13 字段负例，后者是 handler 侧的下载用例）；DIAG-03「交付级别配置」按它**今天真正交付的东西**重新界定并引用既有测试，卡片要求的「按级别过滤写入」另记为功能缺口（第 5 条）。**按原样「补 8 条测试」会产出 2 条重复用例与 1 条测不到目标的用例。**
   >
   > **另 6 行由新测试支撑**：`TestContentMigrationConstraints`（迁移约束，18 个文件，四条规则）、`TestCommittedRunWritesBackItsOperationAndTraceIDs`（回写链路）、`TestOverviewQueueWaitCountsOnlyQueueEvents`、`TestOverviewP95IsNullUntilThereAreEnoughSamples`、`TestReadPathsDoNotWrite`、`pnpm check:diagnostics-no-upload`（静态检查）。
   >
   > **一处此前就存在的计数偏差已改正**：§4.1 原记 113 / 24 / 8，而按第 2 节逐行统计实为 **112 / 25 / 8**——DIAG-04 的「遵守无 FK / 并发索引迁移约束」行在 §2 里一直是「代码存在但无测试」，§4.1 却已把它算作已通过。本次该行**真的**有了测试，两边就此对齐；本文件的计数从此以第 2 节逐行统计为准。
   >
   > **本次交付不改动任何生产代码**：新增 3 个 Go 测试文件、2 个脚本文件，改动 `package.json`、CI 的 `-run` 过滤（`on:` 段逐字节未动）与本文件。`server/` 下非 `_test.go` 文件零改动，无新增迁移，**未为任何界面行补自动测试**。
   >
   > **2026-09-14 更新（四）`specs/009-diag-durable-outbox` 实施后**：DIAG-04「交付 outbox 接口」行的 **「带限制」取消**，由「代码存在但无测试」转为「自动测试已通过」，§4.1 合计 **112 / 25 / 8 → 113 / 24 / 8**（总数仍 145）。
   >
   > 落库实现（`PostgresOutbox`）在调用方的事务里写记录，排水器（`Drainer`）按行级租约认领并派发，进程重启后未派发项不丢也不重复。`TestMemoryOutboxDoesNotSurviveTheProcess` 与 `TestPostgresOutboxSurvivesTheProcess` **两条并存、互为反面**——前者不是被删掉了，而是主语被点明：`MemoryOutbox` 仍为无数据库场景保留，它的限制依然真实。
   >
   > **一条不声称的保证**：派发已发出、成功标记尚未落库时崩溃，下一轮会重发。没有分布式事务就关不掉这个窗口，所以它由接收方的幂等吸收，**不记为本实现单方面保证的 exactly-once**。代码注释与规格 FR-011 都这么写。
   >
   > **本次不改动 `Store.Audit` 与 `Store.CommitRun` 的语义**：新增的同事务入口 `CommitRunWithDispatch` 与它们并列，`CommitRun` 的事务体被原样提取为私有函数供两者共用。

4. **14 个条目「代码存在但无测试」**（feature 010 后为 16，**008 再转化 2 项**：DIAG-09「支持关联跳转」与 DIAG-10「不复制媒体」）——**剩下的 14 项里有 13 项是浏览器界面行**：DIAG-06（3）、DIAG-09（6）、DIAG-13（2）、DIAG-12 预览（1）、DIAG-10 模型参数快照（1）。按 constitution 原则 II 这些不补 UI 单测，只能由用户手动验收，因此 **DG-01 的出口天然依赖一份尚不存在的完整浏览器验收报告**。另外 1 项是非界面行（DIAG-02「验证嵌套字段」），见第 6 条。
5. **一项功能缺口，不是测试缺口**（feature 010 登记）：**DIAG-03 卡片要求的「按级别过滤写入」在生产代码中不存在**。`limits.go` 的 `ConfigureLimits` 只有 `MaxLogs` 与 `Retention` 两项，全仓没有任何按 `severity` 过滤写入的分支。写测试无法闭合它——测试只能固定现状，而现状里没有这个功能。**需要一个实现任务**，不属于补证据的范围。
6. **两项非界面行留给后续 spec**（feature 010 显式排除）：
   - DIAG-02「验证嵌套字段」：`Event` 为扁平结构，造不出真实的嵌套负例，宜与 `specs/008` 的快照字段清单一并处理；
   - ~~DIAG-10「不复制媒体」~~ —— **已闭合**：`specs/008` 的 FR-012 / FR-013 已实施（`snapshot_media_test.go`），该行已转为「自动测试已通过」。原文：**已被 `specs/008-diag-linkage-and-invariants` 的 FR-012 / FR-013 认领**（008 尚未实施），由 010 再写一遍会与之撞车。
7. **对表：两项均已完成**：
   - ~~`simulator.go` 的 16 个场景是否等于 `docs/13` §7 的完整列举~~ —— **已完成**（主任务 2026-09-14 于 `app-main` `c9cc63eca` 核对）：§7 列举 **15 项，全部在 `Scenarios` 之内**，多出的一个是 `clock_skew`。`Scenarios` 是 §7 的**超集**，不构成缺口。
   - ~~迁移 `468`～`473` 是否满足「无 FK、索引全部 `CONCURRENTLY`」~~ —— **已完成**（主任务 2026-09-14 于 `app-main` 核对）：六个 up/down 文件无 `REFERENCES` / `CASCADE`，5 条索引全部 `CREATE [UNIQUE] INDEX CONCURRENTLY IF NOT EXISTS`。~~仓库迁移测试仍不检查这两项，属人工核对~~ —— **不再是人工核对**：feature 010 的 `TestContentMigrationConstraints` 每次 CI 都跑，范围已扩到 `468`～`476` 共 **18 个文件**。

## 5. 剩余实现缺口（下一份 spec 的输入）

本节只列**实现缺口**：D13-V 中仍无自动覆盖、且**不是**「浏览器手动条目未执行」也**不是**「政策阻断」的子句。每条给出为什么算缺口与代码位置。

排除两类，因为它们不是写代码能消除的：

- **手动未执行**——57 条浏览器条目按原则 II 只能由用户确认，再写多少测试也不会让它们变成已验证。
- **政策阻断**——V10「真实重跑需重新校验」与 V12「真实 Codex 及远程阶段实测」要求真实执行器，而 constitution 原则 IX 保持其禁用。这两条在政策改变前**不可能**通过，不属于可规划的缺口。

| # | D13-V 子句 | 为什么算缺口 | 代码位置 |
|---|---|---|---|
| G1 | **V02** 同一操作日志可跳到技术追踪 | **已闭合（派生层）**。`describeTraceJump` 把「从一条事件推导去哪里看」做成纯函数，`linkage.test.ts` 10 条覆盖契约六行边界表与四条不变量；面板 `onTrace` 改为消费它，空/非法追踪编号时按钮置灰并说明。**2026-09-14 修正**：本行原写「路径本身没有派生逻辑」是**错的**——跳转当时已存在于 `index.tsx:531` 的一串内联 setter，真正的缺口是它按原则 II 不可测，且追踪编号为空时会静默跳到未筛选的全量日志。界面呈现仍 **需手动** `specs/008` J-1 ～ J-4 | `packages/core/content/diagnostics/linkage.ts`；`packages/views/content/diagnostics/index.tsx` |
| G2 | **V02** 可跳到对象版本 | **已闭合（派生层，带范围限制）**。`describeObjectVersions` 在**已取回的事件**内按 `objectType`+`objectId` 归拢版本，去重保序并计数，`linkage.test.ts` 8 条覆盖六行边界表与两条不变量；面板在单次追踪页显示对象与版本，无版本时显示「未记录版本」。**不新增数据读取路径**（服务端筛选无对象维度，Q2 裁决 A），代价是**跨页同对象事件看不全**，界面有范围说明，手动条目 O-3 专验这一点 | `packages/core/content/diagnostics/linkage.ts` |
| G3 | **V04** 提供有效下一动作（界面呈现） | **已闭合（判定层）**。`next_action_test.go` 4 条：场景码穷尽性（按码逐一写明期望动作，新增未决码即失败）、`check_registered_file` 分支、默认与空码回退、动作与可重试成对。四处变异验证均变红。**2026-09-14 修正**：本行原写「没有错误码→可执行动作的映射函数」是**错的**——映射一直在 `log.go:99-100` 的生产代码里，且 `log_regression_test.go` 已断言其中 4 个分支；真正的缺口是穷尽性、`check_registered_file` 分支与两字段一致性。**界面呈现仍无自动覆盖**（原则 II） | `server/internal/content/diagnostics/log.go:99-100` |
| G4 | **V06** 不新建自动媒体快照 | **已闭合**。`snapshot_media_test.go` 3 条：16 字段清单逐项比对名称与类型、集合元素只许是字符串、全结构无字节容器。变异验证：加 `Blob []byte` 变红，**加一个合法的 `string` 字段也变红**——这正是字段清单相对「只查类型」的价值 | `server/internal/content/diagnostics/contract.go` `Snapshot` |
| G5 | **V10** 偏好不受临时恢复影响 | **已闭合**。`reproduce_preference_test.go` 3 条，原运行的偏好用**非夹具值** `strict-sources-only`，复现后重读原运行断言偏好与其余快照输入均未变。**2026-09-14 修正**：本行原写「全仓没有任何测试引用它」是**错的**——Go 字段名是 `Preference`（`saved_preference` 只是 JSON 标签），`simulator_test.go:8` 与 `snapshot_regression_test.go:48` 都引用了；真正的缺口是那些断言用的是夹具默认值 `"all"`，分不清「真的没改」与「碰巧相等」 | `server/internal/content/diagnostics/contract.go` `Snapshot.Preference` |
| G6 | **V02 / V09** 跨层关联的端到端断言 | **已闭合**。`content_diagnostics_linkage_test.go` 3 条，从**真实复现**出发逐跳走「审计事件 →(trace_id) 技术事件 →(run_id) 运行 →(original_run_id) 原故障运行」，断言标识符对得上；另有 FR-018 负例证明空标识符不被当通配（该查询是 `($n='' OR ...)` 形式，空值即不加条件） | `server/internal/handler/content_diagnostics_linkage_test.go` |

> **2026-09-14 更新（五）`specs/008-diag-linkage-and-invariants` 实施后**：**G1 ～ G6 六项全部已闭合**，判定依据是实跑用例并对每项做了变异验证，不是「代码写了」。
>
> 同时**修正本节此前三处与代码不符的描述**（G1、G3、G5，见各行内的「2026-09-14 修正」）。这三处都是**把已经存在的东西写成了不存在**：G1 的跳转、G3 的映射、G5 的字段引用当时都在仓库里。成因是写这三行时**只查了名字没查实现**——G5 最典型，Go 字段名 `Preference` 与 JSON 标签 `saved_preference` 不同名，按 JSON 名检索就什么也搜不到。G2、G4、G6 三行经核实**描述准确**，未改。
>
> 这三处修正**没有缩小 008 的范围**：每项的工作从「从零实现」变成「补上真正缺的那一半」，六项照做不误。

**新增后续条目（本次不修，原则 VIII）**：**复现并不还原原始输入。** `Service.Run` 在 `original != ""` 时只 `GetRun` 读出原运行的首个 `operation_id` 与最大 `attempt`，`Simulate` 随后构造的是**固定夹具快照**（`simulator.go:14`），从不还原原运行记录的那份输入。也就是说「固定故障复现」是「按同一场景新跑一次并建立链接」，不是「用当时的输入重放」。这关系到 D13-V06 / V10 的「复现清单」语义，**不在 §5 六项之内**，交主任务排期。`TestReproduceProducesANewRunLinkedToTheOriginal` 已把这条现状**断言下来**，改变它会使该用例变红，从而必须同步更新本条与 `specs/008` 的 Assumptions。

**顺带查明的一条事实**（支持上条）：`CommitRun` 对已存在的 `run_id` 直接拒绝，存储层**没有任何运行更新路径**。G5 的第一次变异验证正是因此没能变红——写回被静默拒绝了。换用直接 SQL `UPDATE` 才使断言变红。所以「复现不回写偏好」目前**由存储层的无更新路径保证**，而不是由复现逻辑自觉；一旦将来加入运行更新能力，这条性质就只剩 `reproduce_preference_test.go` 在守。

**这一节不构成验收结论。** 它只说明：即使 57 条手动条目全部通过，上表六项仍然没有自动覆盖，DG-01 的证据链在这些位置仍是空的。

## 6. 本文件的证据是怎么来的

本节按更新批次分块，每块注明当时的 `app-main` 提交。下表是**初版**（`app-main` `58b5e1a23`）的命令，Linux 容器 + 本地 PostgreSQL 16.13（`initdb` 新建，端口 15433，非开发库）；后续批次沿用同一套环境。

| 命令 | 退出码 | 结果 |
|---|---:|---|
| `go test ./internal/content/diagnostics -count=1 -race -v` | 0 | 22 个用例全部 PASS，**无 SKIP**（已设 `LORETIDE_DIAG_TEST_DATABASE_URL`，两个 Postgres 集成用例真实执行） |
| `go run ./cmd/migrate up` | 0 | 迁移至 `473_content_log_scope` |
| `go test ./internal/handler -count=1 -run 'TestContentDiagnostic\|TestDeleteWorkspace_PurgesContentDiagnostics' -v` | 0 | 8 个顶层用例 + 11 个子用例全部 PASS |
| `pnpm --filter @multica/core exec vitest run content/diagnostics/contract.test.ts content/diagnostics/stream-state.test.ts` | 0 | 2 文件 15 用例全部通过 |

**2026-09-14 更新（四）的命令**（在 `app-main` `8f60b607b` 上实际执行，PostgreSQL 16.13 本地实例、独立库 `loretide_diag_009`）：

| 命令 | 退出码 | 结果 |
|---|---:|---|
| `go test -race ./internal/content/diagnostics -count=1 -v` | 0 | **53 PASS / 0 SKIP / 0 FAIL**（基线 37 PASS）。`LORETIDE_DIAG_TEST_DATABASE_URL` 已设，DB 背书用例真实执行，**无一条跳过** |
| `go test ./internal/migrations -run 'TestMigrationNumericPrefixesAreUnique\|TestMigrationFilesHaveMatchingDirections' -count=1` | 0 | 迁移编号唯一、`.up`/`.down` 成对 |
| `go build ./cmd/server` | 0 | — |
| `pnpm check:content-boundaries` | 0 | 3539 files; 12 registered modules |
| `pnpm check:diagnostics-contract` | 0 | checked 1 landed module; skipped 11 not yet landed |

**变异验证**（6 处，每处改完即还原）：认领条件去掉 `delivered_at IS NULL`、去掉租约过期判断、去掉 `SKIP LOCKED`、部分唯一索引改成全表唯一、非事务句柄改为静默接受、排水中去掉清理——**六处全部变红**。

> `SKIP LOCKED` 那一处**第一轮没有变红**：租约本身已经保证了正确性（第二个排水器会在第一个提交后重读该行并跳过），`SKIP LOCKED` 保证的是不阻塞。这是一处真实的测试缺口，已补 `TestDrainerDoesNotBlockOnARowAnotherTransactionHolds` 关掉——补完后该变异变红。代码注释也随之改成了变异实际证明的说法。

**2026-09-14 更新（三）的命令**（在 `app-main` `47f818aa3` 上实际执行）：

| 命令 | 退出码 | 结果 |
|---|---:|---|
| `node --test scripts/check-diagnostics-contract.test.mjs` | 0 | 26 用例全部 PASS，无 SKIP |
| `node scripts/check-diagnostics-contract.mjs` | 0 | `checked 1 landed module; skipped 11 not yet landed` |
| `pnpm check:diagnostics-contract` | 0 | 同上（自测 + 扫描串联） |
| `pnpm check:content-boundaries` | 0 | `3530 files; 12 registered modules`，未受影响 |
| `pnpm typecheck` | 0 | 9/9 任务，**全部命中 turbo 缓存**——本次未改动任何 TypeScript 文件，缓存命中是预期结果，不作为新的类型检查证据 |

**变异验证**（SC-004，7 处，每处改完即还原）：删掉 E1 / E2 / E3 任一条判定、删掉 `expires` 必填校验、删掉到期比较、把未落地模块的沉默改成报错、把 `NewID` 加进 E2 认可列表——七次均有用例变红（分别为 5 / 5 / 3 / 2 / 2 / 12 / 1 条）。「写完看绿」不算验证。

**2026-09-14 更新（四）本次刷新的命令**（在 `app-main` `30ecd87d0` 上实际执行，同一套本地 PostgreSQL 16.13 / 端口 15433）：

| 命令 | 退出码 | 结果 |
|---|---:|---|
| `pnpm check:content-boundaries` | 0 | 13 自测用例 PASS；`3534 files; 12 registered modules` |
| `pnpm check:diagnostics-contract` | 0 | 26 自测用例 PASS；`checked 1 landed module; skipped 11 not yet landed` |
| `go test ./internal/content/diagnostics ./internal/middleware -count=1 -race -v` | 0 | diagnostics **37 PASS / 0 SKIP**（已设 `LORETIDE_DIAG_TEST_DATABASE_URL`，四个 Postgres 集成用例真实执行）；middleware **60 PASS / 17 SKIP**——跳过的 17 条是 Redis 限速与 PAT/daemon 认证用例，需要本地未起的 Redis，**与 trace 无关**，6 条 trace 用例（`TestTracePropagationAtTheBoundary` 等）全部 PASS。#32 的 `TestTechnicalSinkOnFullStorageStaysBoundedVisibleAndNonFatal`、`TestAuditOnFullStorageRejectsTheWriteInsteadOfCounting` 与 7 个 `TestOutbox*`、`TestModelOutputSamplesNeverSurviveSanitize` 等脱敏用例均按用例名逐条确认执行 |
| `go test ./internal/handler -count=1 -run 'TestContentDiagnostic\|TestDeleteWorkspace_PurgesContentDiagnostics' -v` | 0 | **14 PASS / 0 SKIP**，含 `TestContentDiagnosticTraceRunsThroughQueueAndDaemon`、`TestContentDiagnosticBoundaryReportsTheTraceItRanUnder`、`TestContentDiagnosticExportCarriesNoRequestSecrets`；已按 `-v` 的用例名逐条核对，未依赖包级退出码（见本节末尾的陷阱说明） |
| `pnpm --filter @multica/core exec vitest run content/diagnostics/{contract,stream-state,trace-waterfall,regression}.test.ts` | 0 | 4 文件 56 用例全部通过（**按文件逐个指定**，不用 `content/diagnostics/` 目录通配——通配会把原则 II 排除的 `queries.test.tsx` 一并跑掉），含 #37 新增的 `trace-waterfall.test.ts`（20）与 `regression.test.ts`（18） |

本次刷新只改本文件，未改任何生产代码、测试或脚本。§4.1 的 13 行计数由脚本按第 2 节各表的「类型」列重新统计，未手工累加；结果 **112 / 25 / 8（共 145）**，与上一版相比自动化 +3、无测试 −3，合计不变——这三项来自 #37 把 DIAG-09 一行与 DIAG-13 两行从「代码存在但无测试」转为「自动测试已通过」。

**2026-09-14 更新（五）008 实施的命令**（在 `app-main` `3fd8268` 之上实际执行，同一套本地 PostgreSQL 16.13 / 端口 15433）：

| 命令 | 退出码 | 结果 |
|---|---:|---|
| `pnpm check:content-boundaries` | 0 | 自测 PASS；新增的 `linkage.ts` 未越界 |
| `pnpm check:diagnostics-no-upload` | 0 | `4 files checked; 1 excluded by design` |
| `pnpm check:diagnostics-contract` | 0 | 26 自测 PASS；`checked 1 landed module; skipped 11 not yet landed` |
| `pnpm typecheck --force` | 0 | 9/9 任务，**0 cached**——全部真实执行，不是缓存命中。见下方「基线缺陷」 |
| `go test ./internal/content/diagnostics -count=1 -race -v` | 0 | **67 PASS / 0 SKIP / 0 FAIL**（已设 `LORETIDE_DIAG_TEST_DATABASE_URL`，Postgres 集成用例真实执行）。含本次新增的 `TestNextAction*` 4 条、`TestSnapshot{HasNoMediaPayload,CollectionsCarryOnlyText,HasNoByteContainerAnywhere}` 3 条、`TestReproduce*` 3 条，用例名逐条核对 |
| `go test ./internal/handler -run 'TestContentDiagnostic\|TestDeleteWorkspace_PurgesContentDiagnostics' -v` | 0 | **17 PASS / 0 SKIP / 0 FAIL**，含本次新增的 `TestContentDiagnosticLinkage*` 3 条 |
| `pnpm --filter @multica/core exec vitest run …5 个文件` | 0 | **5 文件 74 用例**全部通过，含新增 `linkage.test.ts` 18 条 |
| `pnpm --filter @multica/views exec vitest run locales/parity.test.ts` | 0 | 160 用例通过（四语言键齐） |

vitest **逐文件指定**（`contract` / `stream-state` / `trace-waterfall` / `regression` / `linkage`），不用 `content/diagnostics/` 目录通配——通配会把原则 II 排除的 `queries.test.tsx` 一并跑掉。

**本次重跑是在 rebase 到 `app-main` `50733c7`（含 #47 #48 #49 #50 #52）之后进行的**，诊断包用例数由 47 升到 67 是 #50 带入的新用例，不是本特性新增。

**重跑期间本机 PostgreSQL 实例一度消失**（容器临时目录被清），当时 `internal/handler` 打印 `ok`、退出码 **0**、**一个用例都没跑**——正是本节末尾那条陷阱的实况。重建实例并跑完迁移后才得到上表数字，`h3` 日志中无 `Skipping tests` 行。**这再次说明：handler 包的证据必须看用例名，不能看退出码。**

**变异验证**（本次 7 处，每处改完即还原，均确认变红）：

| # | 改动 | 变红用例 |
|---|---|---|
| M1 | 删掉 `check_registered_file` 分支 | 穷尽性 + 该分支两条 |
| M2 | 给 `Scenarios` 加一个未映射的错误码 | 穷尽性 |
| M3 | 把某可重试码的 `Retryable` 翻成 `false` | 动作/可重试成对 |
| M4 | 把默认 `Next` 改成空串 | 三条 |
| M5 | 给 `Snapshot` 加 `Blob []byte` | 字段清单 + 字节容器 |
| M6 | 给 `Snapshot` 加一个**合法的** `string` 字段 | 字段清单（这正是它相对「只查类型」的价值） |
| M7 | 让复现用直接 SQL 回写原运行的偏好 | 偏好与快照输入两条 |

**M7 的第一次尝试没能变红**，值得记录：当时用 `CommitRun` 回写，而它对已存在的 `run_id` 直接拒绝，写入被静默吞掉。换成直接 `UPDATE` 才变红。**这本身是一条发现**——「复现不回写偏好」目前是由存储层没有运行更新路径保证的，不是复现逻辑自觉。

**「M2 的第一版断言没能变红」也必须记录**：最初的穷尽性断言写成「结果落在已知动作集合内」，而未映射的错误码会落到默认分支 `inspect_trace`，它**本来就在集合内**——该断言对它要抓的那个情况恒为真。改成**逐码写明期望动作**的对照表后才真正变红。写完看绿而不做变异验证，就会把这种断言当成覆盖。

**基线缺陷（非本次引入，已顺手修复）**：`pnpm typecheck` **自 #37 合并起在 `app-main` 上一直是红的**，被 turbo 缓存掩盖——#37 当时的记录就写着「9/9 任务全部命中缓存」，缓存命中掩盖了真实的类型错误。原因是 `trace-waterfall.test.ts` 的构造辅助函数没有设置 `route` / `status` / `headersPresent` / `upstreamTrace`，而 `DiagnosticEvent` 要求这四个字段（zod 的 `.optional().default("")` 在 transform 之后产出必填 `string`）。修法与本次新增文件所需的改动完全相同：在辅助函数里补上这四个字段的零值。该文件**不在 008 的 plan Source Code 清单内**，作为清单外改动在 PR 正文单列。

**未执行**：

- `packages/core/content/diagnostics/queries.test.tsx` 与 `apps/web/platform/content-diagnostics.test.ts`——UI 单测，按 constitution 原则 II 与 `loretide-content.yml` 的显式排除，CI 不跑，本次也不跑。
- 全量 `pnpm test` / `make test` / Playwright——本任务为纯文档，不做全量验证。
- 三份 `manual-ui-todo.md` 合计 **64** 条浏览器条目（`specs/002` 37 条、`specs/006` 20 条、`specs/008` 7 条）——需真实 Windows 实例与用户操作。

**一处需要注意的陷阱**：`server/internal/handler/handler_test.go` 的 `TestMain` 在数据库连不上时执行 `os.Exit(0)`，即**整个 handler 测试包会以退出码 0 "通过"而实际一个用例都没跑**。本文件引用的 handler 证据均已通过 `-v` 输出逐条确认用例真实执行，未依赖包级退出码。任何后续复核请同样使用 `-v` 核对用例名，不要只看退出码。
