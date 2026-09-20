# 数据库测试套件

## 这份文件是什么

`server/internal/handler` 与 `server/cmd/server` 是两个**整包级**的数据库测试套件。它们的入口是 **fail-closed** 的：没有完整配置就**非零退出**，**不连接任何数据库**，**不跳过**。

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
| `LORETIDE_DB_TEST_SUITE` | `handler` 或 `cmd-server`，**由代码固定并与环境比对** |

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
```

`scripts/test-go-db.sh` **不会补任何默认值**。缺变量就退出，且不调用 `go`——一个会填默认值的 wrapper 等于把刚拆掉的东西装回去。

### CI

`.github/workflows/loretide-content.yml` 的 `db-suites` 作业：两个 suite 各一个 runner、各一库一角色，名字从 `GITHUB_RUN_ID` / `GITHUB_RUN_ATTEMPT` 派生并在每个动库的步骤里**重新派生并校验**（要删库的那一步必须自己算出它被允许删的名字）。跑完 `if: always()` 精确回收。

证据是**逐测试**的 JSON 事件，不是包级的一行 `ok`：**零个 pass 直接判红**——「跑了零个用例」正是过去被读成成功的那个状态。

## 两种模式

这两个包的测试文件按 `dbtest` 构建约束分成两半：

- **无标签（默认）**：不触及数据库的测试。`go test ./...`、`scripts/test-go.sh` 与通用 CI 作业跑的就是这一半，**不需要任何 `LORETIDE_DB_TEST_*` 配置**，也不会连接任何数据库。两处 DB `TestMain` 都带 `dbtest` 标签，所以默认构建里根本没有它，包不会 fail-closed 退出。
- **`-tags=dbtest`**：上面那一半，加上所有触及数据库的测试与 `TestMain`。只有 `scripts/test-go-db.sh` 会带这个标签，且必须先满足上面的完整配置契约。

「触及数据库」= 可达 `TestMain` 初始化的那几个包级根符号（`testHandler` / `testPool` / `testUserID` / `testWorkspaceID` / `testRuntimeID` / `dbfx` 等）。判定是对整包求引用闭包，**由编译器兜底**：无标签构建里只要还剩一处引用了被标签挡掉的符号就编译不过。混合文件被拆成 `X_test.go`（无标签）与 `X_dbtest_test.go`（带标签），**不用整包 skip 做选择**。

逐文件的分类与依据见 `records/issue-114-dbtest-classification-handler.md`。

`scripts/test-go-db.sh` 固定带 `-tags=dbtest`：少了这个标签，一个数据库作业会建好库、连上、然后只跑无库那一半并报绿。

### 现状

`internal/handler` 已分类，默认路径会跑它的无库测试。**`cmd/server` 仍被 `scripts/test-go.sh` 排除**，它的 53 个测试文件是 Phase 2 的后半段；在那之前它的无库测试确实没有被默认路径执行——这是一个已知的、正在收口的缺口，不是「它们照样在跑」。

## 相关文件

| 文件 | 作用 |
|---|---|
| `server/internal/testutil/dbtest/guard.go` | 配置校验与连上后的身份核对 |
| `server/internal/testutil/dbtest/scope.go` | 每次运行唯一的夹具名（清理仍按 id） |
| `server/scripts/verify-db-test-target.sh` | Go 启动前的同一套检查 |
| `scripts/test-go-db.sh` | 专用入口，固定 `-tags=dbtest` |
| `scripts/test-go.sh` | 通用入口，跑无标签（无库）那一半；仍排除 `cmd/server` |
| `.github/workflows/loretide-content.yml` 的 `db-suites` | 两个隔离 DB 作业 |
