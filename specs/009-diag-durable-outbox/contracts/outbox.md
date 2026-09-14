# Contract: `PostgresOutbox`（`Outbox` 接口的落库实现）

## 形态

与 `MemoryOutbox` 实现同一个既有接口，**接口本身一字不改**（FR-001）：

```go
Register(ctx context.Context, tx any, item DispatchItem) error
Settle(ctx context.Context, tx any, committed bool) int
```

## `Register`

| 条目 | 契约 |
|---|---|
| 事务 | `tx` MUST 断言为 `pgx.Tx`；断言失败 MUST 返回错误、不暂存、不静默接受（FR-004）。**这是唯一一次断言**，位置单一 |
| 写入 | MUST 用**调用方传进来的这个** `tx` 执行 `INSERT`，MUST NOT 另开连接或另起事务（FR-007）——否则「提交即登记」在崩溃窗口内不成立 |
| 去重 | `INSERT … ON CONFLICT DO NOTHING`，由部分唯一索引兜底。重复键是**静默 no-op**，与 `MemoryOutbox` 的跳过语义相同 |
| 空幂等键 | 不参与去重（部分唯一索引的 `WHERE idempotency_key <> ''`），与 `MemoryOutbox` 逐字一致（FR-013） |
| 载荷超限 | MUST 按丢弃处理：不写表、计 `Dropped`、返回 nil。**返回 nil 而不是 error**——否则一条超大的诊断记录会把调用方的业务事务打挂，而丢一条诊断记录不该有这个代价（FR-019） |
| 回滚 | 调用方事务回滚时行根本不存在，无需任何补偿动作（FR-006） |

## `Settle`

| 条目 | 契约 |
|---|---|
| **`tx` 的角色** | `Settle` 在事务**提交之后**被调用，此时 `tx` **已不可用**。落库实现里 `tx` **只作分组键**，MUST NOT 用它执行任何 SQL。实现取回本组记录用的是 `Register` 时记下的 id 集合，走连接池 |
| `committed == false` | MUST 不派发、MUST 清掉本组的分组记录。行本身随调用方事务回滚消失，实现无需删行 |
| `committed == true` | MUST 就地尝试派发本组记录（FR-008）；返回实际派发条数 |
| 二次结算 | 同一个 `tx` 再次 `Settle` MUST 派发 0 条，与 `MemoryOutbox` 现有行为一致 |
| 失败 | 单条派发失败 MUST NOT 影响其他条，MUST NOT 返回错误给调用方（接口无 error 返回），MUST 计 `Errors` 并按退避留给周期扫描（FR-016、FR-017） |
| 进程在此之前退出 | 记录仍在表里，由周期扫描接手（FR-009、FR-010）——这正是与 `MemoryOutbox` 的分界线 |

## 与 `MemoryOutbox` 的对照

| 行为 | `MemoryOutbox` | `PostgresOutbox` |
|---|---|---|
| 重复幂等键 | 进程内 `seen` map 跳过 | 部分唯一索引 + `ON CONFLICT DO NOTHING` |
| 空幂等键 | 不去重 | 不去重（索引的 `WHERE` 子句） |
| 超出容量 / 载荷超限 | 丢弃 + `Dropped` | 丢弃 + `Dropped` |
| 派发失败 | 计 `Errors`，记录消失 | 计 `Errors`，记录**留在表里等重试** |
| 进程重启 | **未派发项丢失** | **未派发项不丢失** |
| 无数据库 | 正常工作 | MUST 明确失败，MUST NOT 退化为内存行为（FR-003） |

**这张表本身就是 FR-014 的依据**：最后两行互为反面，两条断言因此必须同时存在。

## 不做的事

- MUST NOT 把派发放进调用方的事务——慢接收方会拖住数据库事务，失败还会回滚已提交的业务。
- MUST NOT 在 `Settle` 里做全表扫描——那是周期扫描的职责，就地路径只处理本组。
- MUST NOT 新增指标名或 `Overview` 字段（FR-017、SC-005）。
- MUST NOT 读写其他模块的表（FR-021）。
