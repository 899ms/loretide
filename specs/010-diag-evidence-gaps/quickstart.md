# Quickstart: 010 验证指引

前置：Go 1.26（`GOTOOLCHAIN=auto`）、Node 22、一个**独立**的 PostgreSQL。**无前端改动、无需启动应用、无生产代码改动、无 UI 验收项。**

## 0. 数据库（两条用例需要）

```bash
export LORETIDE_DIAG_TEST_DATABASE_URL='postgres://…/…?sslmode=disable'
```

**没设这个变量时，回写 trace 与读路径只读两条用例会跳过。跳过记「未执行」，不记通过**（FR-018）。带库跑时 `-v` 输出里的 SKIP 计数应为 **0**。

## 1. 自动检查

```bash
# 迁移约束（无库）
(cd server && go test ./internal/migrations -run 'TestContentMigrationConstraints' -count=1 -v)

# 诊断包全部用例（含两条 DB 背书新用例）
(cd server && go test -race ./internal/content/diagnostics -count=1 -v)

# CI 那条迁移 lint，加了新名字之后仍要过
(cd server && go test ./internal/migrations -run 'TestMigrationNumericPrefixesAreUnique|TestMigrationFilesHaveMatchingDirections|TestContentMigrationConstraints' -count=1)

# 静态检查：自测 + 仓库扫描
pnpm check:diagnostics-no-upload

# 既有检查不受影响
pnpm check:content-boundaries
pnpm check:diagnostics-contract
(cd server && go build ./cmd/server)
```

预期：全部退出码 0；诊断包 `-v` 输出**不出现 SKIP**（出现即说明库没接上，见第 0 步）。

## 2. 逐条核对验收

| 验的是 | 怎么看 |
|---|---|
| **SC-001 引用对得上** | 拿对照表被改的每一行，按它引用的测试名 `grep -rn "func <TestName>" server/`，找得到且该测试确实断言这一行说的事 |
| **SC-002 无自相矛盾** | 搜对照表里类型为「代码存在但无测试」而备注含「已通过」的行，应为 0 |
| **SC-004 迁移负例点名** | 夹具注入一个含 `REFERENCES` 的 `content_` 迁移 → 变红且失败信息里有该文件名与 R1 |
| **SC-005 P95 两侧** | 19 个样本 → `P95` 为 nil；20 个样本 → 非 nil。两条断言都在 |
| **SC-006 只读** | 把 `Query` / `Runs` / `GetRun` 任一改成带写入 → 只读用例变红 |
| **SC-003a 静态检查正负例** | 当前仓库退出 0；夹具塞 `fetch("https://…")` 或 `sendBeacon` → 非 0 且点名 |
| **SC-003b 无误报** | 夹具里 4 处 `refetch()` 与一处 `api.fetchRaw()` → 不变红 |
| **SC-007 无 UI 单测** | `ls packages/views/content/diagnostics/*.test.* 2>/dev/null \| wc -l` 与改动前一致 |

## 3. 不改生产代码（SC-008）

```bash
git diff --stat origin/app-main -- server/ | grep -v "_test.go"
```

预期：**除测试文件外无任何 `server/` 改动**。出现即违反 FR-014 与硬约束。

## 4. 变异验证（SC-003，6 处）

| 变异 | 应变红的用例 |
|---|---|
| 迁移约束去掉 R1（FK 判定） | 迁移约束用例的 R1 负例 |
| 迁移约束去掉 R3（CONCURRENTLY 判定） | R3 负例 |
| 回写时把 `Trace` 换成一个新随机 id | 回写 trace 用例（**这条是「断言相等而非非空」的意义所在**） |
| `QueueWait` 去掉 `component=="queue"` 判断（全部累加） | 队列统计的「非队列事件不计入」断言 |
| `P95` 的阈值从 `>=20` 改成 `>=1` | 样本不足用例的 null 侧 |
| 静态检查去掉词边界（改朴素子串） | 「不误报」用例（4 处 `refetch` 会被命中） |

每处改完即还原。**删掉规则会红才算断言有效。**

## 5. 未执行项的记录方式

任何未执行的检查在交付记录中标「按策略未执行，等待用户验证」，**不标通过**。特别地：

- DB 背书用例在没有 `LORETIDE_DIAG_TEST_DATABASE_URL` 时会跳过，**跳过记为未执行**；
- 界面行一条测试都不补，交付记录里逐条写明保持手动及依据；
- 本特性**无手动 UI 项**——没有页面改动。
