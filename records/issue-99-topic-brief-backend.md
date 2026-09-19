# Issue #99：选题卡与冻结简报后端首切片

## 任务与边界

- 仓库：`899ms/loretide`
- Issue：`#99`
- 工作分支：`task/issue-99-topic-brief-backend`
- 起点：`dd16162f47e5e822ef9d14d00f66decff93f3219`
- 目标分支：`app-main`
- 只做 PR 1：迁移、后端模块、HTTP 路由、删除链、core 网络契约与非 UI 测试。
- 未做：页面、UI 测试、computer use、模型调用、文件/素材/知识卡读取、输入快照、候选选题、必用/排除、真实 AI executor、服务重启。

## 关键实现决定

- 新建 `content_topic_card` 与 append-only `content_brief_revision`，不用上游 `issue`。
- 账号引用可空；非空时只经 `ip-profile` 的公开 `Get` 验证同品牌归属。
- 迁移使用 483～489，避开在途 021 的 482；无外键、无级联。建表不声明会隐式建索引的 `PRIMARY KEY`，两个稳定 id 唯一索引、版本唯一索引和两个工作区索引各自使用单文件 `CONCURRENTLY`。
- `revision_id` 是稳定引用键，`revision` 只是每张卡内的人读计数。
- `start` 先取得工作区删除锁，在一个事务里写审计、锁选题卡、创建首版简报、更新状态；重复 `start` 返回同一首版。
- 未执行 `start` 的草稿不能直接追加简报，避免先写 revision 1 后再开始时撞唯一索引。
- diagnostics 最小扩界：新增 `Store.AuditTx`。调用方拥有事务并负责提交/回滚；`AuditTx` 复用工作区写锁、授权校验与脱敏，不直接跨模块写诊断表。
- 技术日志在业务事务先回滚后写，避免新日志事务等待调用方仍持有的工作区锁。
- topic 路由位于认证层内、通用成员中间件外，确保所有端点（包括非成员拒绝）真正经过 `workspace-core.Authorize`；越权通过 `RefusalStatus` / `RefusalBody` 隐藏资源存在性并留下技术事件。
- core 在网络边界把 `snake_case` 转为 `camelCase`；读取状态使用开放字符串并由带 `default` 的标签函数兼容未来服务端状态。列表、卡片、版本列表和指定版本均有 query hook；动作成功后失效整个工作区的 topic query 前缀，避免列表状态或首版简报缓存陈旧。

## 文件与用途

- `server/migrations/483_*`～`489_*`：两表、两个稳定 id 唯一索引、简报版本唯一索引和两个工作区索引。
- `server/internal/content/topic-planning/contract.go`：Go 领域形状、状态、动作与错误。
- `server/internal/content/topic-planning/store.go`：事务写入、幂等开始、append-only 简报、跨品牌隔离与诊断接入。
- `server/internal/content/topic-planning/store_integration_test.go`：独立 schema 的真实 PostgreSQL 行为验证；无专用数据库变量时 SKIP，并发写入用例使用仓库 Go 版本支持的 `WaitGroup.Go`。
- `server/internal/content/topic-planning/store_test.go`：不依赖数据库的失败技术事件形状测试。
- `server/internal/content/diagnostics/store.go`、`audit_tx_test.go`：事务感知审计入口及提交/回滚/越权验证。
- `server/internal/handler/content_topic.go`：HTTP 适配、授权、错误映射与路径参数读取。
- `server/cmd/server/router.go`、`content_topic_routes_test.go`：真实路由挂载与中间件路径用例。
- `server/pkg/db/queries/workspace_delete.sql`、生成物与删除测试：品牌删除同事务清理两张新表。
- `packages/core/content/topic-planning/*`：zod 响应解析、状态默认分支与 TanStack Query 接口。
- `packages/core/api/client.ts`、`packages/core/package.json`：原始 JSON transport 与子路径导出。
- `server/internal/migrations/content_constraints_test.go`：原有 R1～R4 继续覆盖 468 及之后的 content 迁移；新增 R5 从本功能起始编号 483 起拦截会隐式建非并发索引的 `PRIMARY KEY/UNIQUE` 约束，不追溯误报 477/479 的历史主键。
- `scripts/content-boundaries.json`、诊断合同检查与文档：登记适配器和 `AuditTx` 公共入口。
- `.github/workflows/loretide-content.yml`、`scripts/test-local-windows.ps1`：工作区中已有独立测试入口改动，但不属于 Issue #99 交付，必须从本次 review/提交中排除。
- `specs/022-ep04-topic-brief/manual-ui-todo.md`：PR 1 无页面的手验边界。

## 当前验证状态

- 当前最终静态差异上**没有执行**测试、迁移、数据库连接、服务操作、CI、提交或推送；因此本节没有任何“通过”结论。
- 仅完成非执行性整理：对本轮修改的 Go 文件运行了 `gofmt`，`git diff --check` 退出 0。
- 尝试调用仓库本地 `prettier` 只为格式化三个 topic-planning TS 文件，但当前环境没有该命令（`prettier is not recognized`）；未产生格式化工具证据，也未触发测试。
- Windows 换行转换使 `git status` 显示多份 `server/pkg/db/generated/*.go` 已修改；按 Git 语义差异核对，生成目录中只有 `models.go` 与 `workspace_delete.sql.go` 有内容变化，其余是状态噪声。本轮未重置、清理或覆盖这些用户工作区文件。
- 较早阶段曾运行 core/静态检查及 sqlc 生成，但它们早于本轮权限、迁移和 camelCase 修正，只能视为历史过程记录，不能作为当前差异的验收证据。
- 较早阶段的 Go 目标命令触发了不安全的 TestMain 本机数据库回退；详见 `records/issue-99-local-database-incident.md`。这些结果全部作废，不得计入验收。

## 未执行测试与检查（当前差异）

- DB-free Go 用例：`TestReadFailureProducesSanitizedTechnicalEvent`、`TestAuditTxFailsClosedWithMissingDependencies`、`TestBriefStoreHasNoUpdateOrDeletePath`、`TestFixtureSchemaNameIsSafe`、`TestContentTopicEndpointsAreMounted`、`TestContentTopicEndpointsRejectUnauthenticatedCallers`。现有包级 TestMain 是否会在选中这些测试前连接数据库尚未被 fail-closed 改造，故本轮未执行。
- 专用 diagnostics PostgreSQL 用例：`TestAuditTxSharesTheCallersCommitAndRollback`、`TestAuditTxRefusesUnauthorizedScopeWithoutWriting`。
- 专用 topic PostgreSQL 用例：`TestTopicCardsActionsAndBriefsRoundTrip`、`TestTopicCardFieldsAllowHonestNoneValues`、`TestAppendBriefRequiresTheStartAction`、`TestConcurrentBriefAppendsReceiveDistinctRevisions`、`TestDecisionActionsDoNotTouchAccountConfiguration`、`TestCreateRejectsAnAccountFromAnotherWorkspace`、`TestAuditFailureRollsBackTheTopicWrite`。
- 常规 backend PostgreSQL/真实路由用例：`TestContentTopicEndpointsHideTheWorkspaceFromNonMembers`、`TestContentTopicPathIDSurvivesTheRealMiddleware`、`TestDeleteWorkspace_PurgesContentDiagnosticsAtomically`、`TestWorkspaceDeletionManifestCoversPublicSchema`。
- core 契约用例 5 条：缺字段回退、错类型回退、未来状态保留、状态标签默认分支、畸形简报版本回退。
- 迁移与生成：483～489 全量 up、重复 up 幂等、down、迁移约束检查、sqlc 重新生成/差异核对均未在当前差异上执行。
- 静态/类型检查：core 指定测试、core typecheck、全仓 `pnpm typecheck --force`、`check:content-boundaries`、`check:diagnostics-contract`、`check:diagnostics-no-upload` 均未在当前差异上执行。

## 未完成与诚实边界

- `LORETIDE_TOPIC_TEST_DATABASE_URL`、`LORETIDE_DIAG_TEST_DATABASE_URL` 与隔离的 `DATABASE_URL` 均未获准用于本轮验证，因此迁移、事务、删除链和真实路由只能标记为未执行。
- 后续验证必须先落实 `records/issue-99-test-entry-fail-closed-proposal.md` 的隔离入口，并逐条保留实际执行/通过/跳过证据；不能再把包级命令的汇总输出代替逐测试证据。
- GitHub-hosted 的四库四角色、迁移生命周期与 JSON 无-skip 门禁方案见 `records/issue-99-github-hosted-isolated-validation-plan.md`；它仅待审，不构成运行或 CI 改动授权。
- 条件迁移恢复：部署前按 483～489 顺序核对两张表、全部列与五个 `indisvalid AND indisready` 索引。若 `schema_migrations` 已记录版本但对象缺失，先备份并停止相关写入；对缺表按原编号顺序手工执行对应建表 SQL，对无效/缺失索引先 `DROP INDEX CONCURRENTLY IF EXISTS` 再执行对应 `.up.sql`，复核对象与唯一性后才恢复写入。此恢复流程本轮未执行。
- FR-018 只交付结构保证：旧简报只插不改且有稳定 `brief_revision_id`。某次运行继续读取其钉住版本的行为，须等 `agent-workflow` 落地后再断言。
- UI 验收留到 PR 2。
