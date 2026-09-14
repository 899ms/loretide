# Contract: 迁移约束测试

## 形态

一条纯 Go 测试，放 `server/internal/migrations/`，与既有 `TestMigrationNumericPrefixesAreUnique` / `TestMigrationFilesHaveMatchingDirections` 并列。**不需要数据库。**

约束是**文件内容的属性**，不是运行时行为——一个已经建好的索引看不出它当初是不是并发建的，所以连库执行一遍反而测不出这件事。

## 范围

```text
server/migrations/<N>_*content_*.{up,down}.sql   其中 N >= 468
```

当前命中 12 个文件（`468`～`476` 的六组）。

**下界 `468` 不是随意取的**：它是 diagnostics 模块第一个迁移。不设下界会把全仓 1000+ 个历史迁移扫进来，别的模块的 FK 立刻变成本模块的红线——那是把别人的包袱记在自己账上。

## 规则

| ID | 规则 | 说明 |
|---|---|---|
| R1 | 不得出现 `REFERENCES` / `FOREIGN KEY` | constitution 原则 V |
| R2 | 不得出现 `CASCADE` | 同上 |
| R3 | 每条 `CREATE [UNIQUE] INDEX` 必须带 `CONCURRENTLY` | 同上 |
| R4 | 含并发索引的文件语句数必须为 1 | PostgreSQL 拒绝在事务或多语句字符串里建并发索引 |

**判定前 MUST 剥掉**：`--` 行注释、`/* */` 块注释、字符串字面量。一条写着 `-- 这里刻意不加 FOREIGN KEY` 的注释恰恰在说约束被遵守了，字面匹配会把它判成违规；数分号数到字符串里的分号同样会误判 R4。

## 失败信息

每条违规一行，**必须点名文件与违反的规则**：

```text
474_content_dispatch_outbox.up.sql: R1 foreign key reference is not allowed (matched "REFERENCES")
475_content_dispatch_outbox_idempotency.up.sql: R3 index must be created CONCURRENTLY
```

只说「迁移不合规」不满足合同——作者需要知道改哪个文件的哪一行。

## 可测性

| 要验的 | 怎么验 |
|---|---|
| 正例 | 当前仓库的 12 个文件全部通过 |
| R1 负例 | 合成夹具：一段带 `REFERENCES` 的建表语句 → 变红并点名 R1 |
| R3 负例 | 合成夹具：`CREATE INDEX` 不带 `CONCURRENTLY` → 变红并点名 R3 |
| R4 负例 | 合成夹具：一个文件里两条语句其中一条是并发索引 → 变红并点名 R4 |
| 注释不误报 | 合成夹具：`-- FOREIGN KEY` 出现在注释里 → **不**变红 |

**夹具用内存中的「文件名 → 内容」映射，不在 `server/migrations/` 里造假文件**——造了会被迁移编号唯一性与 up/down 配对两条既有 lint 扫到，两套检查会互相打架。这要求判定逻辑写成可接受映射的纯函数，CLI/测试入口再去读盘。

## 接进 CI

CI 现有这一条：

```text
go test ./internal/migrations -run 'TestMigrationNumericPrefixesAreUnique|TestMigrationFilesHaveMatchingDirections' -count=1
```

**只在 `-run` 的名字过滤里加一个新测试名**，工作流的 `on:` 段一字不动。
