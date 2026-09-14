# Contract: `Drainer`（周期排水器）

重启恢复的真正来源。就地排水只覆盖「本进程还活着」的情形；**FR-010 只能由这里满足**。

## 生命周期

| 条目 | 契约 |
|---|---|
| 归属 | 进程内组件，由已有的服务装配点持有。**MUST NOT 新增独立后台服务**（Q2 = A） |
| 启动 | `Start(ctx)` 起一个 goroutine 按间隔唤醒 |
| 停止 | `ctx` 取消即停；MUST 在返回前结束正在进行的一轮，不留悬挂 goroutine |
| 配置 | 间隔、租约时长、重试上限、每轮认领条数均为**构造参数**，默认值是包内常量。真实取值由 `cmd/server/router.go` 读环境变量后传入。**模块内 MUST NOT 读环境变量**（FR-009b） |

## 一轮唤醒做的事

### 1. 认领

```sql
SELECT … FROM content_dispatch_outbox
WHERE delivered_at IS NULL
  AND dead_lettered_at IS NULL
  AND next_attempt_at <= now()
  AND (claimed_until IS NULL OR claimed_until <= now())
ORDER BY next_attempt_at, created_at
FOR UPDATE SKIP LOCKED
LIMIT <batch>
```
随即把 `claimed_until` 推到 `now() + <lease>`。

| 条目 | 契约 |
|---|---|
| 并发 | `SKIP LOCKED` MUST 保证同一瞬间两个排水器不取到同一行（FR-012 前半） |
| 失联 | `claimed_until` 过期后该行 MUST 可被重新认领（FR-009a、FR-012 后半） |
| 为什么两者都要 | 只有行锁：持锁进程被 kill 后锁随连接释放，但没有「这条正在被谁处理」的持久证据；只有租约：两个排水器会在同一毫秒读到同一行 |
| 已派发 / 已死信 | MUST 不被认领（认领条件已排除） |

### 2. 派发

| 结果 | 动作 |
|---|---|
| 成功 | 写 `delivered_at`，清 `claimed_until` |
| 失败 | `attempt_count++`；`next_attempt_at` 按退避推后；写**已脱敏的短** `last_error`；计 `Errors`；清 `claimed_until` |
| 失败且 `attempt_count` 达上限 | 写 `dead_lettered_at`，停止重试（FR-018） |

**接收方必须幂等**：认领租约过期后重新认领是合法的，此时前一个认领方可能已经派发成功却没来得及写 `delivered_at`。这个窗口任何不用分布式事务的 outbox 都关不掉，合同因此把它显式交给接收方（FR-009a 后半句、data-model「三层保证」第三层）。

### 3. 清理

`delivered_at` 或 `dead_lettered_at` 早于模块既有 `Retention`（7 天）的行 MUST 被删除（FR-020）。挂在同一次唤醒里，**MUST NOT 另起定时器**——两个互相不知道的定时器比一个慢的更难排查。

## 可观察性

| 事件 | 落点 |
|---|---|
| 派发失败 | `LogBuffer.Errors` → `Overview.sink_errors` |
| 丢弃 | `LogBuffer.Dropped` → `Overview.dropped` |
| 进入死信 | MUST 走**既有技术日志事件**，MUST NOT 新增计数器或指标名，MUST NOT 只写进表里没人看的一列。复用 `Errors` 会把「又失败一次」与「不再重试了」混成同一个数字 |

**MUST NOT 新增任何指标名或 `Overview` 字段**（FR-017、SC-005）。

## 可测性

| 要验证的 | 怎么测 |
|---|---|
| 重启不丢（FR-010、SC-001） | 提交后不跑就地排水，丢弃实例，新建 `Drainer` 跑一轮 |
| 不重复（FR-011、SC-001） | 同上再跑一轮，断言派发总次数仍为 1 |
| 并发只发一次（FR-012、SC-003） | 两个 `Drainer` 同时跑同一批 |
| 租约过期可重领（FR-009a、SC-010） | 租约设为毫秒级，认领后不派发直接放手，等过期再跑一轮 |
| 上限与死信（FR-018、SC-004） | 重试上限设为 1～2，接收方恒失败 |
| 间隔可缩短（FR-009b、SC-012） | 传一个远小于默认值的间隔，断言用例不随默认间隔变慢 |

以上除「间隔可缩短」外均需真实事务，走 DB 背书用例，门槛沿用 `LORETIDE_DIAG_TEST_DATABASE_URL`（`store_integration_test.go:21`）。CI 已经在 `loretide-content.yml` 里用一个最小权限库跑 `go test -race ./internal/content/diagnostics`，**不需要新增 CI 步骤**。
