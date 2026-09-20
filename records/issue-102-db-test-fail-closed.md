# Issue #102 —— 测试入口 fail-closed（Phase 1）

**分支** `claude/spec-023-db-test-fail-closed` ｜ **base** `app-main` @ `af0c77d`（#100 合入后）
**日期** 2026-09-20

## 做了什么

两处整包数据库 `TestMain`（`server/internal/handler/handler_test.go`、`server/cmd/server/integration_test.go`）不再有默认 DSN、不再回退 `DATABASE_URL`、不再 exit-0 跳过。缺任何一项必需配置就在**连接前**以 **exit 2** 退出。

## 不在本次范围

- **346 个测试文件的 build-tag 分类**（Phase 2，另开 Issue）。因此这两个包的无库用例目前**完全不跑**——见下「残留限制」。
- 其他使用数据库的包。本次只修这两个整包级静默退出的入口。

## 证据

全部在云端容器自带的 PostgreSQL 上取得（`127.0.0.1`，本容器内），独立库 + 最小权限角色，**未指向任何容器外地址**。

### 无配置 → 非零退出，且没有连接尝试

用不可达地址 `192.0.2.1`（RFC 5737 测试网段）配 30 秒 `connect_timeout` 作通用 `DATABASE_URL`，不给 opt-in：

```
$ env -i PATH=... DATABASE_URL="postgres://multica:multica@192.0.2.1:5432/multica?sslmode=disable&connect_timeout=30" ./handler.test
refusing to run the handler database suite: LORETIDE_DB_TESTS=1 is required: these suites need an isolated database and never choose one for you
exit=2  elapsed_ms=22
```

22 毫秒。若曾发起连接，30 秒超时会让它挂住。`cmd/server` 同样 exit 2、同样的一行。

（`go test` 会把二进制的退出码归一成 1；上面直接运行 `go test -c` 产出的二进制，看到的是真实的 2。）

### 配置齐全但不符 → 非零退出，且不跑用例

| 情形 | 结果 |
|---|---|
| 期望库名不符 | exit 2 `connected to database "loretide_db_test_h_…", expected "loretide_db_test_wrong"` |
| 期望角色不符 | exit 2 `connected as role "loretide_role_h_…", expected "loretide_role_wrong"` |
| 角色是 superuser | exit 2 `the connected role holds rolsuper; a test role must hold none of the privileged attributes` |
| 环境里的 suite 与二进制不符 | exit 2 `LORETIDE_DB_TEST_SUITE=cmd-server but this binary is the handler suite` |

shell 侧 `verify-db-test-target.sh` 对同样的错配同样 exit 2，并打印期望与实际的九元组（不含 URL）。

### 配置正确 → 逐测试 JSON 证据

同一隔离库上迁移后：

```
internal/handler (全新库) : 逐测试终态 pass=3801 skip=47 fail=0
cmd/server                : 逐测试终态 pass=489  skip=0  fail=0
```

那 47 个 skip 是**既有**的、依赖本作业不提供的服务（Redis 等）的用例，不是本次要消除的整包跳过。CI 的证据脚本把它们逐个列名，而不是并进一个总数。

**「跑了零个用例」必须判红**，用 CI 里那段 node 脚本对一份只含 skip 的事件流实测：

```
$ node <该脚本> allskip.jsonl handler
Error: handler: no test reported a pass; a suite that ran nothing is not a green suite
exit=1
```

**一次必须说明的复用污染**：在同一个本地库上反复跑全量时，`TestOldRevisionWithoutProfileReadsAsEmptyPendingProfile`（用固定 `revision_id`）与 `TestCommentSourceContextLifecycle` 会因上一轮残留而失败。换成**全新库**后 0 failed（上表即全新库结果）。CI 每次新建库，不会出现这种状态；本地反复跑同一个库会。

### 纯逻辑用例

`internal/testutil/dbtest` 的 `LoadRequired` 不做任何 I/O，所以它强制的每一条规则都有不需要数据库的用例：opt-in 精确值、每个必需变量、空白值、suite 比对、标识符形状、URL 的路由参数 / 多主机 / socket / 未评审参数、以及**任何错误都不带口令**。

## 残留限制（不要读成已解决）

1. **默认路径不再执行这两个包。** 这是真实缺口，Phase 2 才关上。
2. **这里的任何检查都不是隔离性证明。** 环境变量、库名前缀、loopback、标记行都可伪造。隔离边界是 CI 每次新建库与角色的那台 runner；本次只是让进程在无法证明自己指向它时拒绝运行。
3. **所有权检查**只说明角色拥有目标库，不说明它不拥有别的库。
4. **shell 校验与 Go 校验不是两个独立证明**：它们读同一份环境，能满足一个的人也能满足另一个。它们是同一个错误的两道防线。
5. **DB 作业不在 `ci.yml` 的 `backend` 聚合门里**（那个门只看 backend-tests / backend-agent-tests / sqlc-check / go-vulnerability-scan）。要让它成为合并门槛，需要在分支保护里单独加。
6. `server/scripts/verify-handler-test-db.sh` **保留未动**，因为 PR #100 的分支作业仍在调用它；#100 合入后应切到 `verify-db-test-target.sh` 并删除旧脚本。

## 与 PR #100 的关系

本分支在 #100 合入 `app-main`（`af0c77d`）后 rebase，并**就地把它的分支专用作业 `issue-98-account-profile-db` 改成通用作业 `db-suites`**：

| 原来 | 现在 |
|---|---|
| `if:` 限定 `codex/issue-98-account-expression-profile` 一个分支 | 无分支条件，随 workflow 的 `push`/`pull_request` 触发 |
| 一个库跑两个包 | matrix 两条腿，**各一 runner、各一库一角色**（名字带 suite slug） |
| 点名 10 个用例 | **整包**，经 `scripts/test-go-db.sh` |
| 断言「恰好 10 个顶层 pass」 | 逐测试终态统计，**零 pass 即红**，skip 逐个列名 |
| 传通用 `DATABASE_URL` + 两个旧变量 | 传完整 `LORETIDE_DB_TEST_*`；通用 `DATABASE_URL` 只在迁移那一行、且不导出 |

派生 / 校验 / 最小权限 provisioning / `always()` 精确回收这四段逻辑是**它的**，原样保留。

#100 的迁移 482 schema 契约检查也保留了，挂在 handler 那条腿上（它需要一个迁移过的库，而这是最便宜的现成位置）。
