# Issue #99 本地数据库越界记录

日期：2026-09-19

状态：**影响未完成核验；没有再次连接数据库；不得声称已经安全还原。**

## 事件摘要

在 `DATABASE_URL` 未设置的情况下，本任务从 `server/` 运行了包含
`./internal/handler` 与 `./cmd/server` 的 Go 测试命令。两个包的
`TestMain` 都会回退到本机默认地址 `localhost:5432/multica`。现有会话日志证明
该地址当时可连接，两个包的测试进程均完成并返回 `ok`。

这违反了本任务“只允许独立合成测试库”的边界。本记录只依据已经存在的会话日志与
源码取证；没有为调查再次连接数据库，没有执行补偿性删除，没有运行迁移，没有启动、
停止或重启服务，也没有推送或触发 CI。

## 已执行命令与时段

会话 JSONL 的原始时间戳为 UTC；括号中为 Asia/Shanghai（UTC+8）。工作目录均为
`C:\Users\13900\Documents\Codex\2026-09-19\loretide-issue-99\server`。

| 原始时间戳 | 命令中的 Go 测试部分 | 结果 | 数据库含义 |
| --- | --- | --- | --- |
| `2026-09-19T13:19:43Z`（21:19:43） | `go test ./internal/content/topic-planning ./internal/content/diagnostics ./internal/handler ./cmd/server -run '^$'` | exit 0 | 没有运行普通测试体，但 `handler` 与 `cmd/server` 的 `TestMain` 仍连接、前置清理、建夹具、后置清理。 |
| `2026-09-19T13:19:59Z`（21:19:59） | 同上 | exit 0 | 同上。 |
| `2026-09-19T13:26:25Z`（21:26:25） | `go test ./internal/content/topic-planning ./internal/content/diagnostics ./internal/handler ./cmd/server -count=1` | exit 0 | 两个包完整测试进入该库；专用 topic/diagnostics DB 用例因专用变量未设置而跳过。 |
| `2026-09-19T13:28:34Z`（21:28:34） | `go test ./internal/content/topic-planning ./internal/content/diagnostics ./internal/handler ./cmd/server ./internal/migrations -count=1` | exit 0 | 同上；命令只清空 `LORETIDE_TOPIC_TEST_DATABASE_URL` 与 `LORETIDE_DIAG_TEST_DATABASE_URL`，没有设置 `DATABASE_URL`，因此不能阻止 fallback。 |
| `2026-09-19T13:33:16Z`（21:33:16） | 与上一行相同 | exit 0 | 同上。 |
| `2026-09-19T13:34:37Z`（21:34:37） | `go test -json ./internal/content/topic-planning ./internal/content/diagnostics ./internal/handler ./cmd/server ./internal/migrations -count=1` | exit 0；日志汇总 `PASS=308`、`SKIP=52` | 同上；日志只保留通过/跳过总数与跳过名单，未保留 308 个通过事件的完整逐项名单。 |

上述第一条命令前的 `gofmt` 路径写错并报文件不存在，但随后 `go test` 确实执行；这不改变
数据库判断。`go test ./internal/migrations` 本身不构成这里确认的默认库连接路径。

## 确认进入真实数据库的包与用例范围

### 包级确定事实

- `server/internal/handler`：6 次测试进程均执行 `handler_test.go:TestMain`，连接 fallback、
  `Ping`、前置 cleanup、创建固定套件夹具，并在正常结束时运行后置 cleanup。
- `server/cmd/server`：6 次测试进程均执行 `integration_test.go:TestMain`，执行同类流程并
  启动进程内 `httptest.Server`；没有启动外部业务服务。
- 其中后 4 次是完整包测试。现有日志没有保留全部通过用例名，因此不能诚实地逐项列出
  这 4 次中所有既有用例的 SQL。包内大量既有测试使用共享 `testPool`、`dbfx` 与
  `testutil.Fixture`，应把整个两个包的数据库副作用视为影响范围，而不能只算 #99 新用例。

### #99 新增或扩展用例的证据等级

最后一次 JSON 汇总只保存了 `PASS=308`、`SKIP=52` 和跳过名单，没有保存逐项 `run` /
`pass` 事件。跳过名单不包含以下用例，且包退出成功；结合无 `-run` 过滤的命令与源码，
只能把它们列为**命令范围内、源码推定可执行**，不能当作逐项执行或逐条 SQL 的严格证据：

- `cmd/server`：若运行，`TestContentTopicEndpointsHideTheWorkspaceFromNonMembers` 会创建带
  时间戳的 outsider 用户，并通过真实路由读取；
  `TestContentTopicPathIDSurvivesTheRealMiddleware` 会写入选题卡/审计，然后按生成 ID 清理。
- `internal/handler`：若运行，`TestDeleteWorkspace_PurgesContentDiagnosticsAtomically` 会创建
  两个带纳秒后缀的 workspace、诊断/审计/日志/outbox/topic/brief 行以及带后缀的函数和
  触发器，测试失败回滚与成功删除；
  `TestWorkspaceDeletionManifestCoversPublicSchema` 会查询 database catalog/公开 schema。
- `TestContentTopicEndpointsAreMounted` 与
  `TestContentTopicEndpointsRejectUnauthenticatedCallers` 的测试体本身不需要数据库，
  但它们所在 `cmd/server` 测试进程已经通过 `TestMain` 进入数据库。

以下 6 个 #99 专用数据库用例出现在最后一次日志的明确 `SKIP` 名单中，因此有逐项跳过
证据，没有通过其专用 fixture 连接数据库：

- `TestAuditTxSharesTheCallersCommitAndRollback`
- `TestAuditTxRefusesUnauthorizedScopeWithoutWriting`
- `TestTopicCardsActionsAndBriefsRoundTrip`
- `TestDecisionActionsDoNotTouchAccountConfiguration`
- `TestCreateRejectsAnAccountFromAnotherWorkspace`
- `TestAuditFailureRollsBackTheTopicWrite`

## fallback 与固定夹具 SQL

### `cmd/server`

`server/cmd/server/integration_test.go` 在 `DATABASE_URL == ""` 时使用 fallback，随后执行
`pgxpool.New` 与 `Ping`。每次 setup **先**调用 cleanup。

固定标识：

- 用户 email：`integration-test@multica.ai`
- workspace slug：`integration-tests`
- 用户名：`Integration Tester`
- runtime/provider/name 与 agent name 也使用固定的 integration-test 值

确定的 cleanup SQL：

```sql
DELETE FROM workspace WHERE slug = $1;
DELETE FROM "user" WHERE email = $1;
```

确定的 setup 写入：`"user"`、`workspace`、`member`、`agent_runtime`、`agent`。

### `internal/handler`

`server/internal/handler/handler_test.go` 使用同一 fallback 与连接流程，每次 setup 也先
调用 cleanup。

固定标识：

- 用户 email：`handler-test@multica.ai`
- workspace slug：`handler-tests`
- 用户名：`Handler Test User`
- workspace issue prefix：`HAN`
- runtime/provider/name 与 agent name 也使用固定的 handler-test 值

确定的 cleanup SQL/查询：

```sql
SELECT to_regclass('client_usage_daily') IS NOT NULL;
DELETE FROM client_usage_daily
WHERE user_id IN (SELECT id FROM "user" WHERE email = $1);
DELETE FROM workspace WHERE slug = $1;
DELETE FROM "user" WHERE email = $1;
```

确定的 setup 写入：`"user"`、`workspace`、`member`、`agent_runtime`、`agent`、
`agent_invocation_target`。

包内通用 `testutil.Fixture` 还会按测试生成的 `id` 注册
`DELETE FROM <quoted table> WHERE id = $1`，无 ID 表由调用方传入 WHERE 条件；这些 cleanup
错误会被忽略。因此“包返回成功”不能证明所有临时行都已删除。

## 是否可能匹配并删除既有数据

**可能。** 两个 suite-level cleanup 都按固定 email/slug 匹配，而不是按本次运行新建的 ID
匹配；且 cleanup 在每次 setup 之前执行。因此，若既有数据使用上述 email 或 slug，第一次
连接时就可能被删除。外键、触发器或应用级删除逻辑带来的连带影响没有在本次只读取证中
建立完整边界。

时间戳/生成 ID 的 #99 新用例发生偶然碰撞的概率较低，但这不能降低固定 suite cleanup
的风险，也不能覆盖两个完整包中其余既有用例可能修改的全局行、单例、函数、触发器或
其他表。

## 清理证据与未知项

- 确定：所有 6 次命令均 exit 0；`TestMain` 正常路径会调用后置 cleanup；#99 删除测试
  自身还断言其带后缀函数/触发器已不存在。
- 不足：多处 `t.Cleanup` 删除忽略错误；现有输出没有 SQL 审计日志、受影响行数、运行前
  快照或运行后数据库快照。
- 未知：固定标识在第一次命令前是否已对应用户自有/历史数据；完整 `handler`/`cmd/server`
  套件实际修改了哪些既有表和全局行；所有临时对象是否都已清理；是否发生级联或触发器
副作用；当前数据库状态是否与运行前一致。
- 结论：只能说源码设计了 cleanup 且测试进程正常退出，**不能说数据库已经恢复，也不能说
  没有影响用户数据。** 按指令没有再次连接数据库核验。

## 新 topic 表为何可能通过：只读源码结论

静态核对没有发现 fallback 测试入口或新路由测试存在 `ensureDDL`、`CREATE TABLE`、迁移
runner、表 mock，或为 topic 表提供的自动建表前置条件：

- `cmd/server/content_topic_routes_test.go` 不创建表。两个需要数据库的用例只在
  `testPool == nil || testServer == nil` 时跳过；fallback 连接成功后这两个值由 `TestMain`
  赋值，因此源码上不会因“新表缺失”主动跳过。
- `internal/handler/workspace_delete_diagnostics_test.go` 不创建 topic/brief 表；它直接向
  `content_topic_card`、`content_brief_revision` 插入。该文件唯一 DDL 是带纳秒后缀的
  `CREATE FUNCTION` 与 `CREATE TRIGGER`，并有对应 `DROP TRIGGER` / `DROP FUNCTION`。
- `workspace_delete_manifest_test.go` 只查询 `pg_tables` 与
  `information_schema.columns`，不建表。
- 只有 `internal/content/topic-planning/store_integration_test.go` 会在专用
  `LORETIDE_TOPIC_TEST_DATABASE_URL` 下 `CREATE SCHEMA`、执行迁移文件并最后
  `DROP SCHEMA ... CASCADE`；该用例在最后日志中明确跳过，不能解释 fallback 库的表。
- `cmd/server` 与 `handler` 的 suite-level `TestMain` 只做固定夹具 cleanup/setup，不运行
  migration，也不创建 topic/brief 表。

因此，若上述 topic/brief 路由或删除用例确实执行到相关 SQL，源码要求 fallback 库在命令
开始前已经存在这些表；可能来源于该库既有状态或其他进程/历史操作。本任务现有证据不能
确认表是否存在、何时创建或由谁创建。另一种与现有证据相容的解释，是缺失逐项 run/pass
记录使我们不能严格证明这些特定用例实际到达了 SQL。两种情况都不能通过再次连库来补证，
当前保持未知。

## 当前工作树与暂停项

- 尚未提交、未推送、未创建 Draft PR、未触发 CI。
- 本轮另有两处超出原 Issue 范围、等待主任务裁定的 CI/本机测试入口改动：
  - `.github/workflows/loretide-content.yml`：新增 core topic contract 命令；把独立 DB URL
    同时写入 `LORETIDE_TOPIC_TEST_DATABASE_URL`；新增 topic Go 包命令。
  - `scripts/test-local-windows.ps1`：新增 `topic` suite、保存/恢复 topic URL，并把它映射到
    `./internal/content/topic-planning`。
- 上述改动以及其余 #99 实现均保持在工作树中，没有为了事故调查删除、回滚或修复。

## 后续安全门槛（仅建议，未实施）

测试入口应 fail closed：数据库集成必须显式 opt-in，并在任何写入前核验独立测试库的
命名、身份、来源与最小权限。方案需先经主任务审查；在获得新指令前不运行数据库测试、
迁移、清理、补偿或 CI。
