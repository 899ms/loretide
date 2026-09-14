# Phase 0 Research: 诊断关联跳转与不变量断言

全部结论以 `app-main` `c9cc63e` 的代码为依据，逐条注明查证位置。

## R1 — 跳转推导放哪里，才能被断言

**Decision**：在 `packages/core/content/diagnostics/linkage.ts` 新增 `describeTraceJump(event)`，返回「一组筛选」或「一个不可跳转的理由」。面板只消费其结果。

**Rationale**：跳转今天是 `index.tsx:531-541` 的一串 `setRunId` / `setTrace` / `setComponent("")` … 内联 setter。按原则 II 界面不可测，所以这段逻辑目前**在结构上无法被任何测试触及**。把「推导去哪里看」与「把它设进 state」分开，前者就成了纯函数。这与 `trace-waterfall.ts`、`regression.ts` 的既有做法完全一致——那两处的文件头注释写的正是同一条理由。

**Alternatives considered**：
- 在 `views` 里写测试：违反原则 II，且 `packages/views/` 的测试不得 mock `next/*`，不解决根本问题。
- 用 e2e 覆盖：原则 II 明确禁止自动浏览器验收。
- 不抽函数、只补手动条目：G1 仍是「无自动覆盖」，缺口不闭合。

## R2 — 追踪编号为空时的行为（Q1 裁决 A）

**Decision**：返回 `{kind:"unavailable", reason:"noTrace"}`，面板据此禁用按钮并给出说明。

**Rationale**：这是**修一个缺陷**，不是加功能。`Sanitize` 会把不符合 `hexID` 的追踪编号清成空串（`log.go:93`），所以空 `traceId` 是真实可达的状态，不是理论边界。当前实现在这种情况下会 `setTrace("")` 并清掉其它全部筛选——结果是**跳到未经筛选的技术日志**。读者刚点了「查看追踪」，看到一屏日志，最自然的理解就是「这些就是这次故障的追踪」。这比不提供跳转更糟。

**Alternatives considered**：
- 退化为按 `runId` 跳转（Q1-B）：仍给上下文，但范围比追踪宽，可能混入同一运行的其它追踪，读者依然会误读，只是误读得不那么离谱。
- 保持现状（Q1-C）：把已知会误导的路径留着。

## R3 — 对象版本怎么在不新增数据路径的前提下定位（Q2 裁决 A）

**Decision**：`describeObjectVersions(events, target)` 在**调用方已取回的事件数组**内，按 `objectType` + `objectId` 归拢出现过的 `objectVersion`，按首次出现顺序返回。不发任何请求。

**Rationale**：已核实服务端筛选结构 `Filter` 只有 `After` / `Limit` / `Kind` / `Trace` / `Component` / `Severity` / `Code` / `Run` / `From` / `Until`，其 SQL 也只对 `trace_id` / `component` / `severity` / `error_code` / `run_id` / `received_at` 六列做条件（`store.go:163`）。**没有任何对象维度的查询能力**，全仓也没有这样的查询参数。要做服务端跳转就必须新增一条读取路径，被 FR-019 与 Q2 裁决排除。

已取回的事件本身带 `object_type` / `object_id` / `object_version` 三个字段（`contract.go` `Event`，经 `eventSchema` 解析后是 `objectType` / `objectId` / `objectVersion`），所以在客户端归拢是**用现有数据**，不是新数据。

**已知限制**：跨页的同对象事件看不全。这是 Q2-A 的直接代价，已写进 Assumptions 与手动清单 O-3，**不隐藏**。

**Alternatives considered**：
- 新增服务端对象筛选（Q2-B）：语义完整，但新增数据读取路径，须单列论证；裁决为否。
- 只显示不跳转（Q2-C）：G2 实际不闭合。

## R4 — 「临时恢复」在当前代码里是什么（Q4 裁决 A）

**Decision**：指 `Service.Run(..., original)` 这条复现路径。G5 断言：**复现不修改原运行及其快照（含偏好）**。

**Rationale**：全仓检索确认**不存在**任何 restore / revert / 临时恢复流程（三个诊断目录下仅命中测试里的 `vi.restoreAllMocks`）。裁决取 A，即 `docs/13` §7 的「临时恢复」就是从快照复现。

复现路径的实际行为（`service.go:19-25`）：`original != ""` 时 `GetRun` 读出原运行，**只**取其首个事件的 `operation_id` 与最大 `attempt` 用于承接，然后 `Simulate` 生成新运行并 `run.Original = original`。**原运行是只读的**，没有任何写回。

**这使断言非空洞**：可以先落一条快照偏好为**非夹具值**的原运行，再以它为原故障发起复现，然后重读原运行断言其偏好未变。这满足 FR-015「必须用与夹具默认值不同的值」——现有的 `simulator_test.go:8` 恰恰是拿夹具默认值 `"all"` 比对，分不清「真的没改」和「碰巧都等于默认值」。

**记为后续、本特性不修（原则 VIII）**：`Simulate` 每次都构造**固定夹具快照**（`simulator.go:14`），从不还原原运行的快照。也就是说「复现」并不真的还原原始输入，只是新跑一次并建立链接。这关系到 D13-V06 / V10 的「复现清单」语义，但它是**产品行为问题**，不在 §5 六项之内，**本特性不碰**，写进对照表作为新条目供主任务排期。

## R5 — 下一动作的穷尽性怎么断言（Q3 裁决 A）

**Decision**：新增 Go 测试遍历 `Scenarios` 的全部 16 个 `Expected` 码 + 一组已知错误码 + 一个未知码，断言每个码经 `Sanitize` 后的 `Next` 非空且属于测试内声明的已知动作集合；并断言每个分支的 `Next` 与 `Retryable` 成对成立。

**Rationale**：映射已在 `log.go:99-100` 的生产代码里，`log_regression_test.go` 已断言 4 个分支（`inspect_trace` 默认、`retry_simulation`、`check_authorization`、`check_local_client`）。缺的是：
1. **穷尽性**——没有任何测试证明 16 个场景码都落在已知动作上。新增一个错误码会静默落到默认分支 `inspect_trace`，而那未必对。
2. **`check_registered_file` 分支**（`FILE_MISSING` / `FILE_CHANGED`）是唯一未被断言的分支。
3. **`Next` 与 `Retryable` 的一致性**从未被成对断言。

已知动作集合声明在**测试里**而非导出到生产代码：本特性 G3 是纯测试工作，导出一个 `KnownNextActions` 变量属于给生产代码加面积，且「封闭集合」这件事正是测试要主张的，放在测试里更诚实。

**Alternatives considered**：把集合导出到 `log.go` 供测试引用——生产改动，且会让断言退化成「拿实现比实现」，测不出实现改错。

## R6 — 「快照无媒体载荷」怎么断言才会在将来变红

**Decision**：用反射枚举 `Snapshot` 的全部字段名与类型，与测试内的**预期字段清单**逐一比对；清单不匹配即失败。

**Rationale**：`Snapshot` 今天没有二进制字段，这是**结构巧合**而非被断言的性质——`snapshot_regression_test.go` 的四个用例（克隆隔离、复现缺口、输入不可变、JSON 往返）没有一个覆盖它。只断言「当前没有 `[]byte` 字段」是不够的：将来有人加一个 `string` 字段装 base64 图片，这种断言照样绿。字段清单则会在**任何**新增字段时变红，逼人显式确认「这个新字段不是媒体」。

**成本**：加合法的非媒体字段也会触发失败，需要同步清单。这正是它的作用——已写进 spec 的 Edge Cases。

**Alternatives considered**：只查类型不查清单（漏 base64 字符串）；序列化后查体积上界（阈值任意，且大哈希也会误报）。

## R7 — 跨层链条端到端怎么落（G6）

**Decision**：在 `server/internal/handler/` 新增集成测试，走既有的 `testutil.Call` + `dbfx` 路径：真实发起一次复现 → 从审计事件取 `trace_id` 查技术事件 → 从技术事件取 `run_id` 查运行 → 从运行取 `original_run_id` 查原运行，逐跳断言标识符对得上。

**Rationale**：放 handler 层而非 `diagnostics` 包，是因为链条要「从**真实运行**出发」，而运行的落库与事件写入由 `Service.Run` 串起，handler 测试是唯一能覆盖完整串联的现有层。`internal/handler` 已有 `TestContentDiagnosticTraceRunsThroughQueueAndDaemon` 这类集成用例可作范式，且 `CLAUDE.md` 明确要求用 `dbfx` / `testutil.Call` 而非手写 INSERT 与 recorder 四件套。

**必须防的退化**：空标识符不得当通配。已核实 `store.go:163` 的 SQL 用 `($4='' OR payload->>'trace_id'=$4)` 形式——**空值即不加条件**，也就是说拿空 `trace_id` 去查会返回全部事件。断言必须显式覆盖这一点（FR-018），否则「链条走通」可能只是每跳都匹配到了一堆无关记录。

**陷阱（沿用上一轮的记录）**：`server/internal/handler/handler_test.go` 的 `TestMain` 在数据库连不上时 `os.Exit(0)`，整个包会以退出码 0「通过」而一个用例都没跑。G6 的验收必须用 `-v` 核对用例名真实执行。
