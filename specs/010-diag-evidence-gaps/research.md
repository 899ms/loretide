# Research: 010 补齐「代码存在但无测试」的定向证据

Phase 0。Technical Context 无 NEEDS CLARIFICATION（三处已由 2026-09-15 主任务裁决）。以下为落地前的技术决策，全部核实自 `app-main` @ `af02b6e`。

## D1. 先改对照表，再补测试

- **Decision**：US1（核实与更正）在 US2（补测试）**之前**完成并单独成一组任务。
- **Rationale**：逐行核实的结论是点名的 8 条里有 2 条已有测试、1 条写测试关不掉。先补测试再核实，会先写出 2 条重复用例，再把它们删掉。**核实的产出是「该写哪几条」，它是 US2 的输入而不是并行项。**
- **Alternatives considered**：并行做——省不了时间（核实是读代码，补测试是写代码，瓶颈都在同一个人），却让重复劳动有机会发生。否决。

## D2. 迁移约束测试：扫文本，不连数据库

- **Decision**：一条纯 Go 测试，读 `server/migrations/` 的文件内容做判定，**不需要数据库**。放 `server/internal/migrations/`，与既有两条 lint（`TestMigrationNumericPrefixesAreUnique`、`TestMigrationFilesHaveMatchingDirections`）并列。
- **Rationale**：约束本身是**文件内容的属性**（有没有 `REFERENCES`、索引是不是 `CONCURRENTLY`、是不是单语句），不是运行时行为。连库执行一遍反而测不出「文件里写没写」——一个已经建好的索引看不出它当初是不是并发建的。放既有 lint 旁边，CI 现有的那条 `go test ./internal/migrations -run 'TestMigration...'` 只需在名字过滤里加一个即可，**不改工作流触发条件**。
- **范围下界 `468`**：`468` 是 diagnostics 模块第一个迁移。不设下界会把全仓 1000+ 个历史迁移扫进来，其中别的模块的 FK 会立刻变成本模块的红线——那是把别人的包袱记在自己账上。
- **文件名含 `content_`**：diagnostics 的四张表全部以 `content_` 前缀命名（`content_diagnostic_run`、`content_operation_audit`、`content_technical_log`、`content_dispatch_outbox`），迁移文件名沿用同一前缀。

## D3. 迁移扫描要跳过注释与字符串

- **Decision**：判定前先剥掉 SQL 注释（`--` 行注释与 `/* */` 块注释）与字符串字面量，再匹配 `REFERENCES` / `FOREIGN KEY` / `CASCADE`。
- **Rationale**：Edge Case 里写明的误报风险。一条写着「-- 这里刻意不加 FOREIGN KEY」的注释会让检查变红，而它恰恰在说这个约束被遵守了。
- **可复用的先例**：`scripts/check-content-boundaries.mjs` 的 `extractGoImports` 就是为同一类问题写的词法器（跳过注释、字符串、原始字符串、rune 字面量）。SQL 侧要自己写，但逻辑面小得多。
- **Alternatives considered**：直接正则全文匹配——会被注释误报，且「索引是不是单语句」也需要数语句数，正则数分号同样会被字符串里的分号骗到。否决。

## D4. 回写 trace 测试：走完「模拟 → 提交 → 读回」

- **Decision**：DB 背书测试。`Simulate()` 产出运行 → `Store.CommitRun` 落库 → 从 `content_operation_audit` 读回那条审计事件 → 断言它的 `trace_id` / `operation_id` **等于** `run.Events[0]` 的对应值。
- **Rationale**：`CommitRun` 在落库前执行 `saved.Events = nil`，**运行行本身不存事件**；事件走同一事务内的 `appendAudit`，其 `Trace` / `Operation` 从 `run.Events[0]` 复制。所以要验的是这一次复制有没有丢，而这只能从库里读回来验。
- **关键**：断言 **`==` 而不是「两者非空」**。非空断言在「复制成了另一个随机 id」时依然绿——那正是最可能出的错。
- **Alternatives considered**：只断言 `GetRun` 返回的运行携带 trace——运行行不存事件，这条断言无对象。否决。

## D5. 只读断言：前后行数快照

- **Decision**：调用 `Query` / `Runs` / `GetRun` 前后，各对 diagnostics 模块的四张表取一次 `count(*)`，断言前后一致。
- **Rationale**：「不修改业务状态」的可观察形式就是「行数没变」。做法不依赖任何新角色、新连接、新权限，任何人读得懂。
- **为什么不用只读事务/只读角色**：把读路径塞进一个 `SET TRANSACTION READ ONLY` 的事务里，测的是「这个事务只读」而不是「这几个方法只读」——方法内部自己 `Begin` 的话，外层设置根本管不到它。只读角色则要新建角色与授权，为一条断言引入一套权限配置。否决。
- **边界**：读路径可能触发 `PruneTechnical` 之类的顺带清理。已核实 `Query` / `Runs` / `GetRun` 三个方法体内**没有**任何 `Exec`（`store.go:219`、`224`、`268`、`292` 全是 `SELECT`），所以行数快照不会被自身的清理搅乱。

## D6. 队列统计与样本不足：纯内存，不需要库

- **Decision**：两条都只需要构造 `Event` 切片与一个 `Service`，走 `Overview` 的计算路径，**不需要数据库**。
- **Rationale**：`QueueWait` 与 `P95` 都是 `Overview` 在内存里算出来的（`service.go:16-17`）。`Overview` 确实要读库拿事件，但断言的是**算法**，用一个可控的事件集合喂进去就够。
- **`P95` 的阈值两侧都要验**：`len(durations)>=20` 是硬阈值。只验 19（null）不验 20（非 null），把条件写成 `>=1000` 也一样绿；只验 20 不验 19，写成恒为非 nil 也一样绿。**两侧各一条断言，缺一条这个边界就没被钉住。**
- **`QueueWait` 的反面也要验**：只验「队列事件被累加」时，把 `if e.Component=="queue"` 删掉（改成全部累加）依然绿。必须同时有一条**非队列事件不计入**的断言。

## D7. 静态检查：显式文件清单 + 词边界 + 正负例

- **Decision**：`node:test` 静态检查，与 `scripts/check-diagnostics-contract.mjs` 同形（导出纯函数 + CLI 入口），扫 **FR-011a 列的四个文件**，匹配 `fetch` / `XMLHttpRequest` / `sendBeacon` / `new WebSocket`。
- **排除 `packages/core/api/client.ts`**（主任务约束）：下载经 `api.contentDiagnosticDownload` → `fetchRaw` 走共享客户端，那是**合法的同源 API 调用**。把它判红等于把正常下载判成违规。
- **词边界（FR-011c）**：已实测 `packages/views/content/diagnostics/index.tsx` 有 **4 处 `refetch(`**（React Query 的重新取数）。`fetch(` 的朴素子串匹配得 **4 个误报**，`(?<![\w.])fetch\(` 式的词边界匹配得 **0 个**。这条不写进规格，实施阶段会先看到 4 条假红线。
- **清单过时要失败（FR-012）**：断言清单里每个文件都存在。一个扫不到文件的检查会永远绿——那比没有检查更糟，因为它看起来在工作。
- **Alternatives considered**：目录通配扫 `packages/views/content/diagnostics/**`——将来该目录加入一个正当需要 `fetch` 的文件时，检查会逼着作者去改检查而不是去想清楚；显式清单让「范围变了」这件事在 diff 里看得见。否决（也与主任务约束一致）。

## D8. 已有测试的引用怎么写才算数

- **Decision**：对照表引用测试时，MUST 写**测试函数名**，且该测试的断言要真的覆盖该行说的事；覆盖不全时写明覆盖到哪、没覆盖哪。
- **Rationale**：SC-001 要求「按引用的测试名去仓库里找，能找到且对得上」。写「已有测试覆盖」而不写名字，等于把核实工作留给下一个读者。
- **本特性会用到的两处引用**：
  - DIAG-02 私有正文脱敏 → `TestModelOutputSamplesNeverSurviveSanitize`（13 字段负例）+ `TestLogRegressionSanitizeRules`（`Message` 被固定文案覆盖，含恶意文本注入）；
  - DIAG-12 下载文件名与原始字节 → `TestContentDiagnosticExportDownloadNamesTheFileItWantsSaved`（`Content-Disposition`、原样 bundle、wire key 名）。**同时写明前端保存路径仍无测试且按原则 II 不补。**

## D9. DIAG-03 怎么写才不是粉饰

- **Decision**（Q1 = A）：该行的证据列改为它**今天真正交付**的两件事（`Severity` 四值枚举收敛、`limits.go` 两项可配置且拒非法值）并引用对应测试；备注列**明写**「卡片要求的『按级别过滤写入』在生产代码中不存在，属**缺失功能**，单列后续任务」。
- **Rationale**：把「缺功能」写成「已通过」是粉饰；把它继续写成「代码存在但无测试」则是误导——会让人去写一条根本写不出的测试。**第三种写法才是实话**：这部分有证据，那部分没有功能。
- **类型怎么记**：证据列写的两件事都有测试，因此类型可记「自动测试已通过」，**但备注必须带缺失功能的说明**，否则计数会掩盖缺口。这一点由 FR-003 与 SC-002 共同约束。

## D10. 测试落点

| 测试 | 落点 | 要库 |
|---|---|---|
| 迁移约束 | `server/internal/migrations/` | 否 |
| 回写 trace | `server/internal/content/diagnostics/`（DB 背书） | **是** |
| 队列统计 | `server/internal/content/diagnostics/` | 否 |
| 样本不足 P95 | `server/internal/content/diagnostics/` | 否 |
| 读路径只读 | `server/internal/content/diagnostics/`（DB 背书） | **是** |
| 不自动上传 | `scripts/`（`node:test`） | 否 |

DB 背书用例沿用 `store_integration_test.go` 的 `testStore` 夹具与 `LORETIDE_DIAG_TEST_DATABASE_URL` 门槛。CI 已在 `loretide-content.yml` 用最小权限库跑 `go test -race ./internal/content/diagnostics`，**不需要新增 CI 步骤**；迁移 lint 那条 `-run` 过滤加一个名字即可，**触发条件不动**。

**不写 UI 单测**：本特性不为任何界面行补测试。
