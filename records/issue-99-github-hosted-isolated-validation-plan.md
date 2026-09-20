# Issue #99：GitHub-hosted 隔离验证申请方案（仅方案，未执行）

## 结论与授权边界

本文件是供主任务逐项审阅的**未来 GitHub-hosted 验证方案**，不是 CI
配置，也不授权任何测试、迁移、数据库、服务、提交、推送或 GitHub 操作。

在获得新的明确批准前，以下事情仍然禁止：

- 连接任何本地数据库，或运行任意 `go test` / `go run ./cmd/migrate`；
- 修改、触发或复用 `.github/workflows/loretide-content.yml`；
- 修改或运行 `scripts/test-local-windows.ps1`；
- UI 测试、computer use、服务启动、提交、推送、创建 PR 或手工触发 CI。

当前可访问 worktree 中没有 Issue #98 的隔离方案或批准记录；不能把未找到
的先例当作授权。即便后续找到 #98，其批准也**不自动适用于 #99**。

## 审阅快照与精确实现白名单

快照工作树：`task/issue-99-topic-brief-backend`，HEAD
`dd16162f47e5e822ef9d14d00f66decff93f3219`。本方案撰写时，Git 的语义
差异由下列 **46 个 #99 实现/交付文件**组成；候选提交若增删任一项，必须
重新审阅白名单，而不是让验证工作流静默扩大范围。

```text
docs/development/diagnostics-onboarding-contract.md
packages/core/api/client.ts
packages/core/package.json
packages/core/content/topic-planning/contract.test.ts
packages/core/content/topic-planning/contract.ts
packages/core/content/topic-planning/index.ts
packages/core/content/topic-planning/queries.ts
scripts/check-diagnostics-contract.mjs
scripts/content-boundaries.json
server/cmd/server/content_topic_routes_test.go
server/cmd/server/router.go
server/internal/content/diagnostics/audit_tx_test.go
server/internal/content/diagnostics/store.go
server/internal/content/topic-planning/contract.go
server/internal/content/topic-planning/store.go
server/internal/content/topic-planning/store_integration_test.go
server/internal/content/topic-planning/store_test.go
server/internal/handler/content_topic.go
server/internal/handler/workspace_delete_diagnostics_test.go
server/internal/handler/workspace_delete_manifest_test.go
server/internal/migrations/content_constraints_test.go
server/migrations/483_content_topic_card.down.sql
server/migrations/483_content_topic_card.up.sql
server/migrations/484_content_topic_card_workspace_idx.down.sql
server/migrations/484_content_topic_card_workspace_idx.up.sql
server/migrations/485_content_brief_revision.down.sql
server/migrations/485_content_brief_revision.up.sql
server/migrations/486_content_brief_revision_unique_idx.down.sql
server/migrations/486_content_brief_revision_unique_idx.up.sql
server/migrations/487_content_brief_revision_workspace_idx.down.sql
server/migrations/487_content_brief_revision_workspace_idx.up.sql
server/migrations/488_content_topic_card_id_unique_idx.down.sql
server/migrations/488_content_topic_card_id_unique_idx.up.sql
server/migrations/489_content_brief_revision_id_unique_idx.down.sql
server/migrations/489_content_brief_revision_id_unique_idx.up.sql
server/pkg/db/generated/models.go
server/pkg/db/generated/workspace_delete.sql.go
server/pkg/db/queries/workspace_delete.sql
specs/022-ep04-topic-brief/contracts/topic-card-and-brief.md
specs/022-ep04-topic-brief/manual-ui-todo.md
specs/022-ep04-topic-brief/plan.md
specs/022-ep04-topic-brief/spec.md
specs/022-ep04-topic-brief/tasks.md
records/issue-99-local-database-incident.md
records/issue-99-test-entry-fail-closed-proposal.md
records/issue-99-topic-brief-backend.md
```

本文件 `records/issue-99-github-hosted-isolated-validation-plan.md` 是审阅
产物，不属于上面的实现候选提交；若要提交它，也需把它单独加入白名单。

以下现有差异明确**不在 #99 实现白名单、不得随验证一起带入**：

```text
.github/workflows/loretide-content.yml
scripts/test-local-windows.ps1
```

`git status` 显示的其余 `server/pkg/db/generated/*.go` 修改是 Windows 换行
状态噪声；Git 语义差异只有白名单中的 `models.go` 与
`workspace_delete.sql.go`。不得以 reset、checkout、clean 或覆盖去处理它们。

## 迁移基线与 482 关系

- 当前候选中的新迁移严格为 483～489，共 7 对 up/down 文件：两张表和五个
  独立的 `CONCURRENTLY` 索引。
- 当前 checkout 的 `server/migrations/` 在 481 后直接出现 483；并没有
  482 文件。`content_constraints_test.go` 出现的 `482_content_thing_idx`
  仅为内存负例夹具，不是迁移文件。
- 交付记录说明 482 被在途 021 预留，因此 #99 从 483 开始。当前可访问 Git
  worktree/分支中没有可检查的 482 实现，不能假称它已经验证。
- 执行前必须固定 PR head 和其 `origin/app-main` merge-base，并在干净的
  GitHub checkout 重新列举 `server/migrations/48[0-9]_*.sql`。若届时 482
  已出现、483～489 的编号冲突、或 merge-base 发生迁移重排，停止验证并要求
  重新审阅编号/白名单；不得自行改号、rebase 或把 482 当作已验证。
- 全量 `up` 用运行时 `migrations.Files("up")` 的词典序；全量 `down` 反序。
  483～489 的精确生命周期另用专用测试入口验证，避免把历史上无 `.down.sql`
  的迁移误解释成 #99 回滚失败。

## 拟申请的验证基础设施（单独授权、单独 PR）

先单独审阅一个新的 workflow 文件，例如
`.github/workflows/issue-99-topic-brief-isolated-validation.yml`。它只能有：

- `pull_request` 到 `app-main` 的窄路径触发，以及显式 `workflow_dispatch`；
- `permissions: { contents: read }`；不使用仓库 secrets、云账号、外网服务、
  部署凭据或 Docker socket；不调用任何应用外部 API；
- GitHub-hosted Ubuntu runner 的本机 PostgreSQL；不复用或改动现有
  `loretide-content.yml` / Windows 脚本；
- 仅 checkout 的候选 commit、验证本方案列出的命令、上传已脱敏 JSON 结果和
  清理证明为短期 artifact。

该 workflow 文件不在当前 #99 白名单中。批准验证基础设施不等于批准提交或
运行 #99；批准 #99 也不等于批准改动现有 workflow。

## 临时数据库、角色与身份门禁

每个 GitHub run 生成只含数字和下划线的 run suffix，例如
`${GITHUB_RUN_ID}_${GITHUB_RUN_ATTEMPT}`。管理员 `postgres` 只在**预置**和
**最终清理**两个步骤使用；测试步骤绝不使用管理员 URL。

为同一 run 建立四个不同数据库及对应不同角色：

| 用途 | 数据库/角色前缀 | 允许的连接变量 |
|---|---|---|
| 全量首次 up | `loretide_i99_full_` / `loretide_i99_full_` | `DATABASE_URL`，仅该步骤 |
| 全量重复 up | `loretide_i99_repeat_` / `loretide_i99_repeat_` | `DATABASE_URL`，仅该步骤 |
| 483～489 生命周期 | `loretide_i99_lifecycle_` / `loretide_i99_lifecycle_` | `DATABASE_URL`，仅该步骤 |
| #99 测试 suites | `loretide_i99_suite_` / `loretide_i99_suite_` | `DATABASE_URL`、`LORETIDE_DIAG_TEST_DATABASE_URL`、`LORETIDE_TOPIC_TEST_DATABASE_URL`，仅 suite 步骤 |

每个角色必须是该 run 新建、只拥有自己的一个数据库，并且是：

```text
LOGIN NOINHERIT NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS
```

它可在自己拥有的数据库中创建/删除临时 schema 与表，这是 topic/diagnostics
fixture 的必要最小能力；不能创建数据库、角色、扩展或访问其他数据库。随机
密码和 URL 要先 `::add-mask::`，仅经 `$GITHUB_ENV` 传给需要它的后续步骤，
日志不得打印用户名、库名、host 或 URL。

在任一迁移/测试前，分别以四个非管理员角色执行只读身份检查，要求：

```text
rolsuper=false rolinherit=false rolcreatedb=false rolcreaterole=false
rolreplication=false rolbypassrls=false database_owner=true
```

并确认 `current_database()` 精确等于该步骤被分配的数据库。任一值不符、连接
变量缺失、连接到默认 `multica`、或 URL 未掩码，均 `exit 1`；不回退到
`localhost:5432/multica`，不扩大权限重试。

若全量历史迁移需要特定扩展，管理员只能在**对应临时数据库**中预置仓库已声明
且 runner 镜像已有的扩展，然后再次以最小角色验证扩展可见。扩展缺失或要求更
高权限时，整个 gate 失败并记录需要的扩展；不得临时授予 superuser、CREATEDB、
CREATEROLE 或网络访问。

`always()` 清理步骤由管理员按已生成的精确标识符执行：先终止仅属于该临时
数据库的会话，再 `DROP DATABASE IF EXISTS`，随后 `DROP ROLE IF EXISTS`。清理
后以管理员只读查询确认四个数据库和四个角色均不存在；任一残留让 job 失败。
清理日志只输出成功/失败布尔值和 run suffix，不输出凭据。

## 不同数据库上的迁移门禁

所有命令在 `server/` 下运行，并显式设置该步骤的 `DATABASE_URL`；不得依赖
`cmd/migrate` 在变量缺失时回退的本机默认连接串。

1. **full（干净库）**：`go run ./cmd/migrate up`。要求退出 0；读取
   `schema_migrations`，其版本集合必须与候选 checkout 的全部 `.up.sql`
   文件集合完全相等；查询 483/485 两表与五个索引，五个索引均
   `indisvalid AND indisready AND indislive`。条件迁移的“SQL skipped”只能
   是 runner 已声明的条件结果，不能被当作测试 SKIP。
2. **repeat（另一干净库）**：首次 `up` 后记录 migration ledger、表/索引
   形状和索引有效性；第二次 `up` 必须退出 0，ledger 与形状逐项不变。第二次
   runner 输出的 `skip (already applied)` 是本门的预期幂等行为，不属于测试
   JSON 的 SKIP 豁免。
3. **lifecycle（第三个独立库）**：新增且先审阅
   `server/cmd/migrate/migrate_topic_brief_lifecycle_test.go`，其中唯一测试
   `TestIssue99TopicBriefMigrationLifecycle` 使用 `runMigrations` 和私有
   schema/ledger，严格执行 483,484,485,486,487,488,489 的 up → 重复 up →
   489..483 down → 再 up。它逐步断言 ledger、两表存在性、五个索引的
   `indisvalid/indisready/indislive`，并在 down 后断言两表和全部五个索引均
   不存在。该新测试必须在 `DATABASE_URL` 缺失/连接失败时 `Fatal`，不得
   `Skip`。

这三门分别使用不同 DB/role，不能把一次成功拿来证明另一次。全量 down 不作为
#99 的验收命令：runner 只枚举存在 `.down.sql` 的历史文件，历史不可逆版本会令
“全库 down 后 ledger 为空”成为错误指标；#99 的七个可逆文件由 lifecycle 门精确
证明。

## TestMain/连接来源与先决修复

| suite | 当前连接来源 | 当前问题 | 执行前必须满足 |
|---|---|---|
| `./cmd/server` | `TestMain` 读取 `DATABASE_URL`，缺失时回退 `postgres://multica:multica@localhost:5432/multica`；连接/`Ping` 失败会 `os.Exit(0)` | 假绿且可能触及本机库 | 先批准并实现 `issue-99-test-entry-fail-closed-proposal.md`；只接受显式 suite DB URL，缺失/连接失败非零退出 |
| `./internal/handler` | 同上 | 同上 | 同上；fixture 只能位于 suite DB |
| `./internal/content/diagnostics` | `LORETIDE_DIAG_TEST_DATABASE_URL`；每例创建 `diag_test_*` schema | 变量缺失时 `Skip` | workflow 必须提供 suite DB URL；JSON gate 禁止任何 Skip |
| `./internal/content/topic-planning` | `LORETIDE_TOPIC_TEST_DATABASE_URL`；每例创建 `topic_test_*` schema | 变量缺失时 `Skip` | workflow 必须提供 suite DB URL；JSON gate 禁止任何 Skip |
| `./internal/migrations` | 不连接 DB | 无数据库入口 | 仍执行 JSON 无 skip gate |
| core contract | Vitest node 环境，无 DB/UI | 无数据库入口 | 仅 node contract 文件，JSON summary 无 pending/todo/skip |

`records/issue-99-test-entry-fail-closed-proposal.md` 是独立变更，必须单独审阅
和合入；本方案不会把它悄悄合入 #99。未满足该先决条件时，cmd/server 与 handler
两组不得执行，即使 GitHub runner 有临时数据库。

## 精确 suites、JSON 通过与无 Skip 门禁

每个 Go 命令使用 `go test -json -count=1 -run '^(... )$'` 写入独立 NDJSON
artifact；shell 启用 `set -euo pipefail`。一个拟新增的、单独审阅的
`scripts/verify-issue-99-test-json.mjs` 必须读取每个 artifact 并要求：

- 每个列出的 leaf test 恰有 `Action:"pass"`；
- 没有任何 `Action:"skip"`、`Action:"fail"`、包级 fail 或缺失的期望测试；
- `go test` 进程退出 0；未知额外执行测试或子测试也使门禁失败，防止 `-run`
  正则漂移扩大范围。

Core 的 Vitest JSON 同样要求总数=5、passed=5、failed=0、pending=0、todo=0、
skipped=0。门禁输出只列 suite、测试名、PASS/SKIP/FAIL 计数与 artifact SHA-256，
不输出数据库身份或 URL。

| suite | 精确测试集合 |
|---|---|
| `./internal/migrations` | `TestContentMigrationConstraints`；`TestContentMigrationConstraintsCatchViolations`；`TestContentMigrationConstraintsIgnoreCommentsAndStrings`；`TestContentMigrationConstraintsScopeExcludesOtherModules` |
| `./internal/content/diagnostics` | `TestAuditTxSharesTheCallersCommitAndRollback`；`TestAuditTxRefusesUnauthorizedScopeWithoutWriting`；`TestAuditTxFailsClosedWithMissingDependencies` |
| `./internal/content/topic-planning` | `TestReadFailureProducesSanitizedTechnicalEvent`；`TestTopicCardsActionsAndBriefsRoundTrip`；`TestTopicCardFieldsAllowHonestNoneValues`；`TestAppendBriefRequiresTheStartAction`；`TestConcurrentBriefAppendsReceiveDistinctRevisions`；`TestDecisionActionsDoNotTouchAccountConfiguration`；`TestCreateRejectsAnAccountFromAnotherWorkspace`；`TestAuditFailureRollsBackTheTopicWrite`；`TestBriefStoreHasNoUpdateOrDeletePath`；`TestFixtureSchemaNameIsSafe` |
| `./cmd/server` | `TestContentTopicEndpointsAreMounted`；`TestContentTopicEndpointsRejectUnauthenticatedCallers`；`TestContentTopicEndpointsHideTheWorkspaceFromNonMembers`；`TestContentTopicPathIDSurvivesTheRealMiddleware` |
| `./internal/handler` | `TestDeleteWorkspace_PurgesContentDiagnosticsAtomically`；`TestWorkspaceDeletionManifestCoversPublicSchema` |
| `./cmd/migrate` | 拟新增的 `TestIssue99TopicBriefMigrationLifecycle` |
| `@multica/core` | `falls back when a required field is missing`；`falls back when a field has the wrong type`；`preserves a future server status without blanking the card`；`keeps the default branch for a future server status`；`rejects malformed brief versions instead of casting network JSON` |

除上述 suites 外，验证还需以非 UI、无数据库副作用的独立步骤运行：

```text
pnpm --filter @multica/core exec vitest run content/topic-planning/contract.test.ts --reporter=json
pnpm --filter @multica/core typecheck
pnpm check:content-boundaries
pnpm check:diagnostics-contract
make sqlc
git diff --exit-code -- server/pkg/db/generated
go build ./cmd/server
git diff --check
```

`make sqlc` 是当前仓库 CI 的既有入口；workflow PR 仍须以该 checkout 的
Makefile/lockfile 核实其可复现性。若入口缺失或要求额外未批准工具，停止而非
猜测安装或下载工具。

## 最终成功条件与回传格式

只有同时满足以下条件，#99 才可被报告为“GitHub-hosted 验证完成”：

1. 白名单门精确匹配候选提交，两个明确排除文件没有进入提交；
2. 四个临时 DB/role 都通过身份门，且清理后全部不存在；
3. full、repeat、lifecycle 三个迁移门全部通过；
4. 上表所有测试都在 JSON 中 PASS，零 SKIP；
5. core 类型检查、边界/诊断合同、sqlc 差异、server build 与 diff 检查均通过；
6. 结果 artifact 含候选 SHA、merge-base、迁移文件清单、每个 suite 的 PASS/SKIP/
   FAIL 计数、索引有效性布尔值、清理布尔值；不含任何凭据或连接串。

回传必须逐条标注实际执行命令、退出码、PASS/SKIP/FAIL 计数和未执行原因。任一
前置条件、权限、身份、清理或无-skip 门失败时，立即停止后续依赖步骤，报告失败
证据，不自动修复、放宽门禁或改动范围。

## 需要主任务明确批准的最小事项

1. 将 fail-closed TestMain 修复作为独立变更审阅并合入；
2. 新建独立 GitHub-hosted 验证 workflow 与 JSON gate 脚本；
3. 新建 `TestIssue99TopicBriefMigrationLifecycle` 的迁移生命周期测试；
4. 在批准后的 candidate SHA 上触发该 workflow；
5. 如未来 482 合入或白名单变动，先重新审阅本方案。

在上述五项均未被明确批准前，本文件只是一份可审查计划。
