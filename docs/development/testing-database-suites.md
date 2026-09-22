# 数据库测试套件

## 这份文件是什么

`server/internal/handler` 与 `server/cmd/server` 是两个由 Go `TestMain` **fail-closed** 的整包级数据库测试套件：没有完整配置就**非零退出**，**不连接任何数据库**，**不跳过**。

`server/internal/content/topic-planning` 是第三个、只经 `scripts/test-go-db.sh` 调度的 CI store 套件。其历史夹具在直接 `go test` 时若缺少 `LORETIDE_TOPIC_TEST_DATABASE_URL` 会**skip**，并不拥有与前两个包相同的 Go 入口守卫；wrapper 则在启动该包前要求完整的 `LORETIDE_DB_TEST_*` 合同，并仅向子进程映射已验证的 URL。CI 还要求本卡指定的 store 用例实际 `pass`，不能由纯逻辑用例的 package pass 掩盖 skip。

这份文件说明为什么是这样，以及怎么跑它们。

## 为什么

两处 `TestMain` 原本这样写：

```go
dbURL := os.Getenv("DATABASE_URL")
if dbURL == "" {
    dbURL = "postgres://multica:multica@localhost:5432/multica?sslmode=disable"
}
pool, err := pgxpool.New(ctx, dbURL)
if err != nil {
    fmt.Printf("Skipping tests: ...")
    os.Exit(0)     // ← 绿灯
}
```

一个设计带来两种失败：

1. **绿灯掩盖整包跳过。** `ok ... 0.015s` 在任何汇总口径里都和「通过」长得一样，而它实际上一个用例都没跑。这在本仓库被当成通过**不止一次**。
2. **误连开发者本机数据库。** 2026-09-19 一天之内发生 **6 次**。

所以改成：**没有默认值、没有回退、没有跳过。**

## 必需的环境变量

全部必需，**缺一即非零退出（exit 2），且在打开 socket 之前**：

| 变量 | 含义 |
|---|---|
| `LORETIDE_DB_TESTS` | 必须**恰好**是 `1`。`true` / `yes` / `0` 都不算 |
| `LORETIDE_DB_TEST_DATABASE_URL` | 目标库的完整 URL（**不是** `DATABASE_URL`） |
| `LORETIDE_DB_TEST_DATABASE` | 期望连上的库名 |
| `LORETIDE_DB_TEST_ROLE` | 期望连上的角色名 |
| `LORETIDE_DB_TEST_RUN_ID` | 本次运行的标识（CI 用 `run_id_attempt`） |
| `LORETIDE_DB_TEST_SUITE` | `handler`、`cmd-server` 或 `topic-planning`；wrapper 固定它，前两者也由 Go 守卫比对 |

**通用 `DATABASE_URL` 永远不会被读取。** 读它就等于让每一个导出过它的开发者 shell 变成一次隐式 opt-in——而那正是要消除的问题。

## URL 的限制

`LORETIDE_DB_TEST_DATABASE_URL` 被拒绝的情形：

- **多主机**（`a,b`）：pgx 可能连到一台这里没检查过的服务器，身份比对就失去意义；
- **Unix socket**（空 host 或百分号编码的绝对路径）：provisioner 可以用它做管理通道，测试角色的 URL 契约另行评审；
- **路由类查询参数**：`host` / `hostaddr` / `port` / `user` / `dbname` / `database` / `service` / `servicefile` / `target_session_attrs`——它们能改变实际到达哪里，而库名比对是针对调用方**声称**的那个库做的；
- **未经评审的查询参数**：libpq 认识的参数很多，没人考虑过的那个恰恰是会出事的那个，所以白名单之外一律拒绝。

**任何错误信息都不会打印 URL**，它带着口令。诊断输出只说 suite / 库名 / 角色名 / run id。

## 连上之后还查什么

一次往返，九个值一起比：

- `current_database()` 与 `LORETIDE_DB_TEST_DATABASE` 相等；
- `current_user` 与 `LORETIDE_DB_TEST_ROLE` 相等；
- `rolsuper` / `rolcreatedb` / `rolcreaterole` / `rolreplication` / `rolbypassrls` **全为 false**；
- 该角色**拥有**这个库。

**这些都不是隔离性的证明。** 环境变量、库名前缀、loopback 地址、标记行，任何能设置它们的人都能伪造。它们挡住的是**普通的配置错误**——没设的变量、旧的 shell、抄错的命令行。真正的隔离边界是 CI 每次运行新建一个最小权限角色与库的那台 runner；这里只是拒绝在无法证明自己指向它的时候运行。

所有权检查说明「该角色拥有**这个**库」，**不说明**它不拥有别的库。

## 怎么跑

### 本机

```bash
# 自己建一个隔离库和最小权限角色（不要指向开发库）
psql "$ADMIN_URL" -v ON_ERROR_STOP=1 <<'SQL'
CREATE ROLE loretide_db_role_local LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE
  NOREPLICATION NOBYPASSRLS PASSWORD '…';
CREATE DATABASE loretide_db_test_local OWNER loretide_db_role_local;
SQL
psql "$ADMIN_URL/loretide_db_test_local" -c \
  "CREATE EXTENSION IF NOT EXISTS pgcrypto; CREATE EXTENSION IF NOT EXISTS pg_trgm;"

export LORETIDE_DB_TEST_DATABASE_URL="postgres://loretide_db_role_local:…@127.0.0.1:5432/loretide_db_test_local?sslmode=disable"
export LORETIDE_DB_TESTS=1
export LORETIDE_DB_TEST_DATABASE=loretide_db_test_local
export LORETIDE_DB_TEST_ROLE=loretide_db_role_local
export LORETIDE_DB_TEST_RUN_ID=local_$(date +%s)

# 迁移只在这一条命令里看得见通用 DATABASE_URL，且不导出
(cd server && DATABASE_URL="$LORETIDE_DB_TEST_DATABASE_URL" go run ./cmd/migrate up)

bash server/scripts/verify-db-test-target.sh      # 先让 shell 侧拒绝明显的错配
bash scripts/test-go-db.sh --suite handler
bash scripts/test-go-db.sh --suite cmd-server
bash scripts/test-go-db.sh --suite topic-planning
```

`scripts/test-go-db.sh` **不会补任何默认值**。缺变量就退出，且不调用 `go`——一个会填默认值的 wrapper 等于把刚拆掉的东西装回去。这个结论适用于经 wrapper 调度的三个 suite；不要把直接运行 `go test ./internal/content/topic-planning` 的 fixture skip 误读为 fail-closed 通过。

### CI

`.github/workflows/loretide-content.yml` 的 `db-suites` 作业：三个 suite 各一个 runner、各一库一角色，名字从 `GITHUB_RUN_ID` / `GITHUB_RUN_ATTEMPT` 派生并在每个动库的步骤里**重新派生并校验**（要删库的那一步必须自己算出它被允许删的名字）。跑完 `if: always()` 精确回收。`topic-planning` 仅在 wrapper 子进程中把已验证的 URL 映射为其历史夹具变量 `LORETIDE_TOPIC_TEST_DATABASE_URL`，夹具仍会再建/删自己的 schema。

证据是**逐测试**的 JSON 事件，不是包级的一行 `ok`：**零个 pass 直接判红**——「跑了零个用例」正是过去被读成成功的那个状态。`topic-planning` 还必须报告 `TestPatchBodyChangesOnlyItsFiveFieldsAndLeavesFrozenObjectsUntouched`、`TestPatchBodyAuditFailureRollsBackCardWrite` 与 `TestPatchBodyConcurrentDifferentFieldsDoNotOverwriteEachOther` 为 `pass`；skip、缺失或非 pass 都判红。

## 默认路径现在缺了什么

`scripts/test-go.sh` 把这两个包**排除**在常规包列表之外。

这是一个**真实的覆盖缺口**，不是「它们的无库用例照样在跑」：这两个包的 346 个测试文件尚未分类（Phase 2，另开 Issue），所以默认 wrapper **完全不执行**它们。

这个缺口是刻意的，而且比它替换掉的东西小——被替换掉的是**一个报绿而什么都没跑的套件**。Phase 2 给数据库相关的测试文件加 `dbtest` 构建标签、无库的文件保持无标签之后，默认 wrapper 才能重新包含它们并真的跑到无库子集。

## 相关文件

| 文件 | 作用 |
|---|---|
| `server/internal/testutil/dbtest/guard.go` | 配置校验与连上后的身份核对 |
| `server/internal/testutil/dbtest/scope.go` | 每次运行唯一的夹具名（清理仍按 id） |
| `server/scripts/verify-db-test-target.sh` | Go 启动前的同一套检查 |
| `scripts/test-go-db.sh` | 专用入口 |
| `scripts/test-go.sh` | 通用入口，排除 handler 与 cmd/server 两个包 |
| `.github/workflows/loretide-content.yml` 的 `db-suites` | 三个隔离 DB 作业 |
