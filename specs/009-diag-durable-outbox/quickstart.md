# Quickstart: 009 验证指引

前置：Go 1.26（`GOTOOLCHAIN=auto`）、一个可写的 PostgreSQL、迁移已跑到最新。**无前端改动、无需启动应用、无 UI 验收项。**

## 0. 准备数据库

```bash
# 与 store_integration_test.go:21 用的是同一个环境变量
export LORETIDE_DIAG_TEST_DATABASE_URL='postgres://…/…?sslmode=disable'
(cd server && go run ./cmd/migrate up)   # 应跑到 476
```

**没设这个变量时 DB 背书用例会跳过**，跳过不算通过——交付记录里按「按策略未执行」记，不记为 PASS。

## 1. 自动检查

```bash
# 包内全部用例（含 MemoryOutbox 既有 7 个、PostgresOutbox 与 Drainer 新增用例）
(cd server && go test -race ./internal/content/diagnostics -count=1 -v)

# 迁移 lint：编号唯一、.up/.down 成对
(cd server && go test ./internal/migrations -run 'TestMigrationNumericPrefixesAreUnique|TestMigrationFilesHaveMatchingDirections' -count=1)

# 编译
(cd server && go build ./cmd/server)

# 既有边界与合同检查不受影响
pnpm check:content-boundaries
pnpm check:diagnostics-contract
```

预期：全部退出码 0；`-v` 输出里 **不出现 SKIP**（出现即说明数据库没接上，见第 0 步）。

## 2. 逐条核对验收（开发者执行，非 UI 验收）

| 验的是 | 怎么看 |
|---|---|
| **SC-001 重启不丢且只发一次** | 提交后**不跑**就地排水，丢弃实例，新建 `Drainer` 跑一轮 → 派发 1 次；再跑一轮 → 派发 0 次 |
| **SC-002 回滚不留痕** | 业务事务回滚后 `SELECT count(*) FROM content_dispatch_outbox WHERE item_id = …` 为 0 |
| **SC-003 并发只发一次** | 两个 `Drainer` 同跑一批，派发计数总和 = 记录条数 |
| **SC-004 失败不拖垮业务 + 有终点** | 接收方恒失败：业务操作仍成功、记录保留、`sink_errors` 增长；到上限后 `dead_lettered_at` 非空且不再认领 |
| **SC-010 租约过期可重领** | 租约设毫秒级，认领后不派发直接放手，等过期再跑一轮 → 仍只派发 1 次 |
| **SC-011 不越模块** | `git diff` 确认新入口触及的表只有 `content_*`；`grep -n "seat_capacity" server/internal/content/diagnostics/` 无输出 |
| **SC-012 间隔可缩短** | 用例传一个远小于默认值的间隔，用例耗时不随默认间隔增长 |

## 3. 不新增指标名（SC-005）

```bash
# Overview / Metrics 的字段集合必须逐字段一致
(cd server && git diff origin/app-main -- internal/content/diagnostics/service.go | grep -E '^[+-]' | grep -v '^[+-][+-]')
```

预期：**`Metrics` 结构体所在行无任何增删**。出现新字段即违反 FR-017。

## 4. 迁移硬约束（SC-008）

```bash
cd server/migrations
grep -iEc 'references|foreign key|cascade' 474_*.sql 475_*.sql 476_*.sql   # 每个都应为 0
grep -c 'CONCURRENTLY' 475_content_dispatch_outbox_idempotency.up.sql      # 1
grep -c ';' 475_content_dispatch_outbox_idempotency.up.sql                 # 1（单语句）
grep -c ';' 476_content_dispatch_outbox_due.up.sql                         # 1（单语句）
ls 47[456]_*.sql | wc -l                                                    # 6（三对）
```

## 5. 执行闸门未被触碰（SC-009）

```bash
git diff origin/app-main --stat -- server/pkg/executionpolicy/   # 应无输出
```

## 6. 变异验证（不是「写完看绿」）

| 变异 | 预期变红的断言 |
|---|---|
| 认领条件去掉 `delivered_at IS NULL` | 「不重复派发」 |
| 去掉 `claimed_until` 的过期判断 | 「租约过期可重新认领」 |
| 去掉 `SKIP LOCKED` | 并发用例（可能表现为超时或重复） |
| 部分唯一索引改成全表唯一索引 | 「空幂等键不去重」 |
| `Register` 改成另开连接写入 | 「回滚后表里没有行」 |

每处改完即还原。**删掉规则会红才算断言有效。**

## 未执行项的记录方式

任何未执行的检查在交付记录中标「按策略未执行，等待用户验证」，**不标通过**。特别地：DB 背书用例在没有 `LORETIDE_DIAG_TEST_DATABASE_URL` 时会跳过，**跳过记为未执行**。本特性无手动 UI 项——没有页面改动。
