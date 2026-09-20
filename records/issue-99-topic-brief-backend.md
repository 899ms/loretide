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

## 验证状态（会话 002，2026-09-20，本容器自带 PostgreSQL）

隔离库 `loretide_issue99`，`DATABASE_URL` / `LORETIDE_TOPIC_TEST_DATABASE_URL` /
`LORETIDE_DIAG_TEST_DATABASE_URL` 三个变量都指向它；不接触开发库或业务库。
下面每一条都是本轮实跑结果，`rebase 到 9d468905c 之后`重跑确认。

### 迁移与生成

| 项 | 结果 |
|---|---|
| 483～489 全量 up | 7 个文件全部 applied |
| 重复 up | 全部 `skip … already applied`，无副作用 |
| down 489→483 | 两张表 + 五个索引 → 0/0；清空 `schema_migrations` 后再 up 成功 |
| 索引状态 | 五个索引 `indisvalid AND indisready` 均为 true |
| `sqlc generate`（v1.31.1） | 生成后 `git status` 干净，生成物与分支内容一致 |
| `go build ./...` / `go vet` | 退出 0 |

### Go 用例（逐条）

DB-free：`TestReadFailureProducesSanitizedTechnicalEvent` PASS（先 FAIL，见下）、
`TestAuditTxFailsClosedWithMissingDependencies` PASS、`TestBriefStoreHasNoUpdateOrDeletePath` PASS、
`TestFixtureSchemaNameIsSafe` PASS、`TestContentTopicEndpointsAreMounted` PASS、
`TestContentTopicEndpointsRejectUnauthenticatedCallers` PASS、
`TestWritesFailClosedWithoutTheWorkspaceFence` PASS（本轮新增）。

diagnostics 真实库：`TestAuditTxSharesTheCallersCommitAndRollback` PASS、
`TestAuditTxRefusesUnauthorizedScopeWithoutWriting` PASS。

topic 真实库：`TestTopicCardsActionsAndBriefsRoundTrip`、`TestTopicCardFieldsAllowHonestNoneValues`、
`TestAppendBriefRequiresTheStartAction`、`TestConcurrentBriefAppendsReceiveDistinctRevisions`、
`TestDecisionActionsDoNotTouchAccountConfiguration`、`TestCreateRejectsAnAccountFromAnotherWorkspace`、
`TestAuditFailureRollsBackTheTopicWrite` 七条全部 PASS。

后端/真实路由：`TestContentTopicEndpointsHideTheWorkspaceFromNonMembers` PASS（先 FAIL，见下）、
`TestContentTopicPathIDSurvivesTheRealMiddleware` PASS、
`TestDeleteWorkspace_PurgesContentDiagnosticsAtomically` PASS、
`TestWorkspaceDeletionManifestCoversPublicSchema` PASS、
`TestContentTopicWritesAreFencedByWorkspaceDeletion` PASS（本轮新增，六个子用例）。

包级计数（`./internal/handler ./cmd/server ./internal/content/...`）：
**4716 PASS / 47 SKIP / 0 FAIL**。47 条 SKIP 全部是 `REDIS_URL` 未配置的既有 Redis 用例，
与本改动无关。`./internal/migrations`：26 PASS / 0 SKIP / 0 FAIL。

### TS 与静态检查

- `pnpm --filter @multica/core exec vitest run content/topic-planning/contract.test.ts`：5 passed（缺字段回退、错类型回退、未来状态保留、状态标签默认分支、畸形简报版本回退）。
- `pnpm typecheck --force`：9 个任务全部成功，**0 cached**。
- `pnpm check:content-boundaries` 退出 0（3610 文件 / 12 登记模块）。
- `pnpm check:diagnostics-contract` 退出 0，落地模块由 3 变 **4**。
- `pnpm check:diagnostics-no-upload` 退出 0。

## 本轮修复的三处

1. **技术事件的 component 被自己抹掉**：`topic-planning` 在构造事件时就调用了
   `diagnostics.Sanitize`，而它的 component 白名单里没有任何内容模块名，于是
   `topic-planning` 被改写成 `unknown`；同时 `Message` / `Next` / `Retryable` 是从
   `reportFailure` 还没写入的 `Code` 推出来的。脱敏是 sink 的职责（`AuditTx` 与
   `Store.Technical` 各自会做），`ip-profile`、`workspace-core` 同样不预先脱敏。
   删掉模块内的两处 `Sanitize` 后 `TestReadFailureProducesSanitizedTechnicalEvent` 由 FAIL 转 PASS。
2. **路由拒绝用例按 component 名匹配**：`TestContentTopicEndpointsHideTheWorkspaceFromNonMembers`
   用 `payload->>'component'='workspace-core'` 找拒绝记录，而 sink 必然把它写成 `unknown`，
   于是 7 条拒绝一条也数不到。改为匹配脱敏后仍然存在、且只有 workspace-core 会同时出现的字段组合
   （`object_type=account` + `action=query` + `step=not_member` + `error_code=AUTHORIZATION_DENIED` + `severity=warn`）。
   变异验证：把 `topicScope` 的 recorder 换成 nil，用例报 `recorded 0 … want 7`。
3. **删除栅栏改为模块自己取**（Issue #104 第 3 项）：见下。

## Issue #104 第 3 项自查

结论：**原实现的四类写入确实都在持锁事务内，但那是审计的副作用，不是模块自己的保证**——
`Create`、`Act`（`start` / `save` / `defer` / `drop`）、`AppendBrief` 都先 `AuditTx`，
而 `AuditTx` 会在调用方事务里取 `LockWorkspaceForContentDiagnosticWrite`。
按裁决「`AuditTx` 经 diagnostics store 持锁只覆盖有审计的路径，不能替代」，本轮把栅栏改为
`topic-planning` 自己持有：`Store.Guard`（`diagnostics.WorkspaceWriteGuard`）在
`begin()` 里、任何选题语句之前取锁；无行 → `ErrNotFound`（边界 404，与外部工作区同形）；
无 guard 则在开事务之前就失败关闭。handler 注入的就是 diagnostics 用的同一个 guard。

新用例 `TestContentTopicWritesAreFencedByWorkspaceDeletion`（`internal/handler`，真实库真实 schema）：
工作区删除提交之后，建卡 / start / save / defer / drop / 追加简报六条写入全部返回 `ErrNotFound`，
且 `content_topic_card`、`content_brief_revision`、`content_operation_audit` 行数一行不增。
变异验证：去掉 `begin()` 里的取锁调用，六个子用例全部 FAIL。

## 未完成与诚实边界

- `records/issue-99-test-entry-fail-closed-proposal.md` 提的失败封闭测试入口**本轮未实施**：
  `handler` / `cmd/server` 的 `TestMain` 仍带 `localhost:5432/multica` 回退（Issue #102 由另一会话处理）。
  本轮靠显式导出三个环境变量指向隔离库来规避，不依赖回退。
- `.github/workflows/loretide-content.yml` 与 `scripts/test-local-windows.ps1` 的超范围 diff 不在本分支，
  也未新增；CI 文件本轮一行未动。
- **脱敏白名单的既有信息损失（不在本 PR 修）**：`Sanitize` 的 component 白名单里没有任何内容模块名，
  因此 `ip-profile`、`workspace-core`、`topic-planning` 的事件在真实 sink 里一律记为 `unknown`。
  放宽白名单会改动 `TestSanitizeEnumAllowlistsAreUnchanged` 钉住的枚举，影响诊断面板的筛选与导出，
  属于跨模块决定，另行裁决。
- `workspace-core` 的拒绝事件没有填 `Occurred`，落库是零值时间。既有行为，非本 PR 引入，未改。
- FR-018 只交付结构保证：旧简报只插不改且有稳定 `brief_revision_id`。某次运行继续读取其钉住版本的
  行为，须等 `agent-workflow` 落地后再断言。
- 条件迁移恢复：部署前按 483～489 顺序核对两张表、全部列与五个 `indisvalid AND indisready` 索引。
  若 `schema_migrations` 已记录版本但对象缺失，先备份并停止相关写入；对缺表按原编号顺序手工执行对应
  建表 SQL，对无效/缺失索引先 `DROP INDEX CONCURRENTLY IF EXISTS` 再执行对应 `.up.sql`，复核对象与
  唯一性后才恢复写入。此恢复流程本轮未执行。
- UI 验收留到 PR 2。
