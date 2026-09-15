# Data Model: 009 DIAG-04 持久 outbox

**一张新表、两个索引、三个迁移文件对。** 无外键、无级联、无 schema 变更以外的数据读取路径。

## 表 `content_dispatch_outbox`（新增，迁移 474）

前缀 `content_` 与 diagnostics 已有三张表一致（`content_diagnostic_run`、`content_operation_audit`、`content_technical_log`），表归 diagnostics 模块所有。

| 列 | 类型 | 规则 |
|---|---|---|
| `sequence` | `bigint GENERATED ALWAYS AS IDENTITY` | **实施期新增**。行的稳定标识。**`item_id` 不唯一**——空幂等键时同一个 id 可以有多行——所以 `Settle` 在事务提交后按「本事务登记了哪几行」把记录取回来时，没有它就没有可寻址的行。与模块既有两张表（`content_operation_audit`、`content_technical_log`）的 `sequence` 写法一致 |
| `item_id` | `text NOT NULL` | `DispatchItem.ID`。寻址用；调用方给什么就存什么 |
| `kind` | `text NOT NULL` | `DispatchItem.Kind`。路由用。**不设 `CHECK` 白名单**——白名单会让新增一种 kind 需要改迁移，而 kind 的合法性是调用方的事 |
| `payload` | `bytea NOT NULL` | `DispatchItem.Payload`。**大小上限在 `Register` 阶段判定**，超限即丢弃并计数，不写表（FR-019） |
| `idempotency_key` | `text NOT NULL DEFAULT ''` | 空串表示不去重，与 `MemoryOutbox` 现有行为一致（FR-013） |
| `workspace_id` | `text NOT NULL DEFAULT ''` | 跟随模块既有三张表的列形；用于排障与按工作区清理，**不开放给前端查询**（FR-023）。来源见下方 `DispatchItem.Workspace` |
| `attempt_count` | `integer NOT NULL DEFAULT 0` | 已尝试派发次数 |
| `next_attempt_at` | `timestamptz NOT NULL DEFAULT now()` | 早于它不认领；失败后按退避推后 |
| `claimed_until` | `timestamptz` | **行级租约**（FR-009a）。NULL 或已过期即可被认领 |
| `delivered_at` | `timestamptz` | 非 NULL 即已派发，永不再认领 |
| `dead_lettered_at` | `timestamptz` | 非 NULL 即终止，停止重试（FR-018） |
| `last_error` | `text` | 最近一次失败的原因。**必须是已脱敏的短文本**，不得写入原始错误对象 |
| `created_at` | `timestamptz NOT NULL DEFAULT now()` | 登记时间；清理与排序用 |
| `updated_at` | `timestamptz NOT NULL DEFAULT now()` | 最近一次状态变更 |

**无 `FOREIGN KEY`、无 `REFERENCES`、无 `ON DELETE` / `ON UPDATE` 级联**（constitution 原则 V、FR-022）。`workspace_id` 与工作区表的关系由应用代码负责，与模块既有三张表的做法一致。

### 实施期修正：工作区删除时随之清除

本文件原先只说 `workspace_id` 「用于排障与按工作区清理」，**没有说清工作区被删除时这张表怎么办**。缺的不是一句话，而是一条判定：`server/internal/handler` 的 `TestWorkspaceDeletionManifestCoversPublicSchema` 要求 `public` 下每张表在删除清单里有显式归属，这张表落地后该测试在 `app-main` 上一直报 `unclassified=[content_dispatch_outbox]`。

**主任务 2026-09-15 裁决：随工作区一起清除，不保留。** 表带 `workspace_id`、归 diagnostics 模块所有，而工作区一旦删除，未派发项**已经没有接收方**——留着它们不是「待派发」，是永远派不出去的残留。处理方式与 `content_diagnostic_run` / `content_operation_audit` / `content_technical_log` 完全一致：

- 删除清单登记为 `workspaceDelete`；
- 工作区删除事务里加一条 `DELETE FROM content_dispatch_outbox WHERE workspace_id = $1::text`，与上述三张表同一个 CTE 链、同一个事务；
- **仍然不加外键、不加级联**——行由应用代码删除，原则 V 不变。

因此该表与那三张表共享同一条原子性保证：工作区删除失败回滚时，未派发项**原样留在表里**，不会出现「工作区还在、队列已空」的中间态。

### 实施期修正：`DispatchItem` 增加 `Workspace` 字段

本文件原先列了 `workspace_id` 列，但 `DispatchItem` 只有 `ID` / `Kind` / `Payload` / `IdempotencyKey` 四个字段，**没有任何东西可以填它**。两条出路：给 `DispatchItem` 加一个字段，或从表里删掉这一列。删列是对本文件更大的偏离，因此选前者：

```text
DispatchItem{ ID, Kind, Payload, IdempotencyKey, Workspace }
```

- **`Outbox` 接口的方法集与签名一字未改**，FR-001 仍然成立——`TestOutboxCallersDependOnTheInterfaceOnly` 不做任何改动即通过。
- `MemoryOutbox` **忽略**该字段，对外行为不变（FR-002、SC-006）。
- 该字段**不参与记录的身份**：去重仍然只看 `IdempotencyKey`。
- 值由 `Store.CommitRunWithDispatch` 用运行所属工作区**覆盖**写入，调用方在 item 里放什么都不作数——入口拥有 scoping，这样一条记录不可能被登记到别的工作区名下。

## 索引（各自单语句、单文件）

| 迁移 | 语句 | 为什么 |
|---|---|---|
| 475 | `CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS content_dispatch_outbox_idempotency ON content_dispatch_outbox (idempotency_key) WHERE idempotency_key <> '';` | **部分**唯一索引。把「空键不去重」这条规则交给 schema 而不是一段代码；配合 `ON CONFLICT DO NOTHING`，重复登记是静默 no-op，与 `MemoryOutbox` 的跳过语义相同 |
| 476 | `CREATE INDEX CONCURRENTLY IF NOT EXISTS content_dispatch_outbox_due ON content_dispatch_outbox (next_attempt_at, created_at) WHERE delivered_at IS NULL AND dead_lettered_at IS NULL;` | 认领查询的排序与过滤。**部分**索引：已派发与已死信的行不该占认领索引 |

对应 `.down.sql` 为 `DROP INDEX CONCURRENTLY IF EXISTS …` / `DROP TABLE IF EXISTS …`，与 `473`、`468` 的 down 写法一致。

> **一条实施时必须遵守的约束**：PostgreSQL 拒绝在事务或多语句字符串里建并发索引，所以 475 与 476 **各自一个文件、文件内只有一条语句**。仓库的迁移执行器在显式事务之外运行迁移文件，正是为此。

## 状态机

```text
                 Register（调用方事务内，ON CONFLICT DO NOTHING）
                          │
                          ▼
                     [ pending ]  delivered_at NULL, dead_lettered_at NULL
                     │        │
        认领（就地/周期）│        │ 租约过期 → 回到可认领
                     ▼        │
                  [ claimed ] ─┘  claimed_until = now + lease
                     │
         ┌───────────┴───────────┐
         │ 派发成功               │ 派发失败
         ▼                       ▼
   [ delivered ]           attempt_count++，next_attempt_at 退避
   delivered_at = now      Errors +1
   永不再认领                     │
                       达到上限 ──┴──► [ dead-lettered ]
                                        dead_lettered_at = now
                                        停止重试

   业务事务回滚 → 行根本不存在（FR-006）
   delivered / dead-lettered 满 Retention → 清理（FR-020）
```

## 「恰好一次」的三层保证

| 层 | 机制 | 覆盖什么 |
|---|---|---|
| 登记去重 | 部分唯一索引 + `ON CONFLICT DO NOTHING` | 同一幂等键被登记两次 |
| 不重复认领 | 认领条件含 `delivered_at IS NULL`；`SKIP LOCKED` + `claimed_until` | 并发排水器、认领方失联后的重新认领 |
| 派发幂等 | **接收方责任**，合同写明 | 派发成功但标记前崩溃的窗口 |

**第三层必须写在合同里**：前两层挡不住「已经发出去、还没写 `delivered_at` 就崩溃」，任何不用分布式事务的 outbox 都挡不住。声称 exactly-once 而不说这个窗口，等于把风险藏起来。

## 配置项（FR-009b）

| 项 | 默认值定义在 | 真实取值来自 | 测试 |
|---|---|---|---|
| 扫描间隔 | diagnostics 包内常量 | `cmd/server/router.go` 读环境变量后传入 | 直接传一个远小于默认值的间隔 |
| 租约时长 | 同上 | 同上 | 同上，可设为毫秒级以验证过期后重新认领 |
| 重试上限 | 同上 | 同上 | 设为 1～2 以快速到达死信 |
| 载荷上限 | 同上 | 同上 | 设为很小以验证丢弃与计数 |
| 每次认领条数 | 同上 | 同上 | — |

**模块内不读环境变量**：`cmd/server/router.go:443` 已经是这个模式（`diagnostics.NewService(..., os.Getenv("LORETIDE_BUILD"), ...)`），沿用它。

## 计数映射（FR-017，不新增指标名）

| 事件 | 计数器 | `Overview` 字段 |
|---|---|---|
| 派发失败 | `LogBuffer.Errors` | `sink_errors` |
| 载荷超限丢弃、认领到已死信记录等丢弃 | `LogBuffer.Dropped` | `dropped` |

**没有第三个计数器，没有新字段。** `Overview` 的字段集合在本特性前后逐字段一致（SC-005）。

## 不产生的东西

- 不新增 sqlc 查询（模块既有写法是包内裸 SQL，`store.go` 全文如此）。
- 不新增前端可见的读取路径、查询参数或端点（FR-023）。
- 不改 `Overview`、`Metrics`、`Event`、`Run`、`Page` 任何一个结构体的字段。
- 不读写任何其他模块的表，含 `seat_capacity_outbox`（FR-021）。
