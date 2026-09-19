# Issue #99 数据库测试入口失败封闭提案

状态：仅供主任务审查，**未实施、未运行**。

## 目标

消除 `handler` / `cmd/server` 测试在变量缺失时连接
`localhost:5432/multica` 的隐式 fallback。数据库写入前必须同时满足显式 opt-in、独立测试
库来源、命名、连接身份与最小权限校验；任何不匹配都失败，不降级到业务/开发库。

## 最小安全补丁（阶段 A）

1. `server/internal/handler/handler_test.go` 与 `server/cmd/server/integration_test.go` 不再读取
   通用 `DATABASE_URL`，也不再含默认 URL；只读取新的专用变量，例如
   `LORETIDE_HANDLER_TEST_DATABASE_URL`。
2. 再要求一个显式布尔 opt-in，例如 `LORETIDE_RUN_HANDLER_DB_TESTS=1`。URL 与 opt-in 必须
   同时存在；只存在其一视为配置错误并非零退出。两者都不存在时不连接数据库，并明确输出
   “DB suite not opted in”。
3. 在任何 cleanup/setup SQL 之前执行统一只读 gate：
   - URL 来源只能是上述专用变量；不读取或继承 `DATABASE_URL`。
   - URL host 必须是 loopback；database 名匹配 `^loretide_handler_test_[a-z0-9_]+$`；
     禁止 host/database query override，允许项白名单化。
   - 协议查询核对 `current_database()`、`current_user` 与 URL 期望值一致。
   - 角色必须 `NOSUPERUSER`、`NOCREATEDB`、`NOCREATEROLE`、`NOREPLICATION`、
     `NOBYPASSRLS`，且只拥有该测试库。
   - 推荐由环境预置不可伪装的测试库 sentinel，并核对随机 run identity；测试进程本身不
     创建/删除数据库。
4. gate 失败必须 exit non-zero，并只输出脱敏原因；不得打印 URL、密码或原始连接错误中的
   凭据。
5. 阶段 A 可暂时沿用“未 opt-in 时整个包不执行”的现状以先阻断误连，但这仍可能造成
   非 DB 测试静默缺席，不能作为最终验收质量方案。

建议把 gate 放在 `server/internal/testutil` 的小型公共 helper 中，由两个 `TestMain` 复用；
URI/策略部分做纯函数单测，协议身份部分只在审查批准的独立库中运行。

## 后续证据质量补丁（阶段 B，非最小补丁）

- 将 DB-free 测试与 DB integration 分离，或让每个 DB 用例显式调用 `requireTestDB`，避免
  未 opt-in 时 `TestMain` 提前退出而把路由挂载等无 DB 测试一起吞掉。
- CI 对应期望用例名收集逐项 `run/pass/skip/fail` 事件；零测试、任何 skip、缺少期望用例
  都失败。
- 把 suite 固定 email/slug 改成每次运行唯一 token，并以本次捕获 ID 清理；禁止 setup 前按
  固定业务键广泛删除。cleanup 错误需要可见，且保留可恢复的脱敏日志。
- `handler` 与 `cmd/server` 最好使用各自独立数据库或至少串行执行，避免共享全局行/锁。

## CI 影响

现有常规 CI 的 `DATABASE_URL` 指向名为 `multica` 的共享 service database，且使用 service
初始角色；它不满足上述专用来源、命名与最小权限 gate。实施前必须由独立变更在 job 内预置
专用非超级用户/专用数据库、运行迁移、导出专用变量，再运行两个包。不能为保持旧 CI 绿色
而给 fallback 或通用 `DATABASE_URL` 开后门。

## 当前两项超范围 diff（等待裁定，不纳入实现）

### `.github/workflows/loretide-content.yml`

- 新增 `pnpm --filter @multica/core exec vitest run content/topic-planning/contract.test.ts`。
- 把已有独立 DB URL 同时写入 `LORETIDE_TOPIC_TEST_DATABASE_URL`。
- 新增 `go test -race ./internal/content/topic-planning -count=1`。

净变化：3 行新增。该文件不在 Issue #99 原始范围，当前未提交、未推送。

### `scripts/test-local-windows.ps1`

- `ValidateSet` 新增 `topic`。
- 环境保存/恢复名单加入 `LORETIDE_TOPIC_TEST_DATABASE_URL`。
- 将已核验的本机隔离 URL 同时赋给 topic 专用变量。
- 包选择由二分支改为 `switch`，新增 `topic -> ./internal/content/topic-planning`。

该 diff 只扩展已有独立本机入口，但仍是原 Issue 范围外改动；当前未提交、未推送，也未运行
其 `topic` suite。

## 审查决策点

1. 是否把阶段 A 做成独立安全 Issue/PR，而不是混入 #99。
2. 两个测试包共用一个专用 env/database，还是分别使用独立资源。
3. 未 opt-in 时暂时整包 skip，还是先完成阶段 B 后再切断 fallback。
4. 当前两项超范围 diff 是移出 #99、另立 PR，还是经明确授权纳入。
