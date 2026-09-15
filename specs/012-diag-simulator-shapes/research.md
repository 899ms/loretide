# Phase 0 Research: 诊断模拟器的数据形态

核实基线：`app-main` @ `a6bf4ff`。每条决定都给出「读了哪段代码」，不给推测。

---

## D1 — shape 场景的构造点：早分流，不进步进循环

**决定**：`Simulate` 在参数校验之后、进入 `steps` 循环之前对 shape 场景早分流，构造逻辑放在新文件 `shapes.go`。业务故障场景的代码路径逐字节不变。

**为什么**：`simulator.go` 的循环每一轮做两件事，而这两件事**正是那些形态造不出来的原因**：

```text
child, parent := Child(ctx)   // 父 = 上一步的 span
ctx = transport               // 下一步的父 = 这一步的 child     → 父子成链
now = now.Add(duration)       // 写事件之前推进                  → 区间首尾相接
```

想在循环里造并发 span，要打断 `ctx = transport` 的传递；想造孤儿，要在 `i==0` 之外再开一个置父分支；想造 250 个 span，要让 `steps` 变长。三处改动叠在一起，等于把循环改成一台通用事件发生器，而它现在承载的是**16 个业务故障场景的确定性语义**——`TestEverySimulatedFaultAndDeterministicTime` 与 `TestSimulatorRegressionScenarioContracts` 锁的就是这份语义。

**拒绝的替代**：给循环加 `shape` 分支。拒绝理由不是「不好看」，是**业务故障场景的回归测试会开始为 shape 的改动而变红**，两套语义共用一份断言面。

**代价**：`shapes.go` 与 `simulator.go` 各自构造事件，`Event` 的字段填写有重复。接受——重复的是字段赋值，不是规则；规则（`Sanitize`）仍然只有一份，两边都要经过它。

---

## D2 — `>200 span` 的真正卡点在读取，不在产出（改变了 FR-005 的做法）

**核实**：

| 环节 | 代码 | 数值 |
|---|---|---|
| 运行详情取事件 | `store.go` `GetRun` → `s.Query(ctx, scope, Filter{Run: id, Limit: 100})` | 100 |
| `Query` 的上限夹取 | `store.go` `if f.Limit < 1 \|\| f.Limit > 100 { f.Limit = 50 }` | 硬上限 100 |
| 瀑布折叠阈值 | `trace-waterfall.ts` `const cap = options.cap ?? STREAM_EVENT_CAP`；`contract.ts` `STREAM_EVENT_CAP = 200` | 200 |
| 折叠条件 | `trace-waterfall.ts` `if (order.length > cap)` | `order.length > 200` |
| 面板传入 | `index.tsx:351` `buildTraceWaterfall(run.events, expanded ? {cap: run.events.length} : {})`；`run = d.detail.data`（`queries.ts:18` → `runs?run_id=`） | `run.events` ≤ 100 |

**结论**：`100 > 200` 永远为假。**即使运行有 250 个 span，`006-W-7` 的「另有 N 条」折叠提示也出不来**，`006-W-8`（点开折叠）随之无从谈起。只做「让模拟器产出 250 个 span」这一半，交付后这两条仍然跑不了——正是 SC-001 禁止的结果。

**决定**：`GetRun` 用既有游标在内部翻页，把该次运行的事件取全，上限设一个明确的天花板（建议 500，**写进 contracts**）。

**为什么是这个做法**：

- 端点不变、响应形状不变、前端不变 → **不构成「新增前端读取路径」**。
- `Store.Query` 的 100 上限不动 → 对外分页边界（`diagnosticFilter` 里 `f.Limit > 100` 直接 `ErrConflict`）原样保留。
- 天花板是必须的：没有它，一次异常大的运行会让运行详情无界增长。500 给了 250 个 span 两倍余量。

**拒绝的替代**：

1. 调低 `buildTraceWaterfall` 的默认 cap 到 50。**这是改产品行为去迁就验收**。cap 200 与 `STREAM_EVENT_CAP` 同源，是流的事件上限，改它会连带改流的语义。
2. 放宽 `Store.Query` 的 100 上限到 500。面更大：那个夹取同时服务对外的 `/events` 与 `/stream`，放宽等于放宽公开分页边界。
3. 认账，记 `006-W-7`/`W-8` 仍不可达。与 SC-001 直接冲突。

**顺带记录，不在本特性范围**：`Service.Export` 的 `Filter{Limit: 100}` 有同样的截断。一次 250 span 的运行导出后只带 100 条技术事件。这是**已存在的行为**，本特性不改（原则 VIII），但 contracts 要写明这个已知界限，免得下一个人以为导出是全量。

---

## D3 — `Scenario.Kind`：分类是结构里的事实

**决定**：`Scenario` 增加 `Kind string \`json:"kind"\``，取值 `"fault"`（现有 16 项）与 `"shape"`（新增项）。

**为什么不是文档约定**：`docs/13` §7 的对表结论（「§7 的 15 项 ⊆ `Scenarios` 的 16 项」）是 `diagnostics-acceptance-mapping.md` 里 D13-V07 记「部分」而非「未满足」的依据之一。往同一个列表里加 9 项之后，那句话如果只靠文档补一句「其中 9 项不算」，**结论就没法被机器核对了**。有了 `Kind`，SC-002 变成一条可执行断言：按 `Kind == "fault"` 过滤出的 id 集合，等于本特性之前的 16 项。

**对外形状的后果**（`CLAUDE.md` → API Compatibility，constitution 原则 VI）：`Scenarios` 经 `Service.Overview` 下发，前端 `packages/core/content/diagnostics/contract.ts` 的 `overviewSchema` 有对应解析：

```text
scenarios: z.array(z.object({id: z.string(), expected_code: z.string()})
  .transform(s => ({id: s.id, expectedCode: s.expected_code})))
```

新字段在前端**必须可缺省**：装好的桌面端会连上更旧的后端，那时响应里没有 `kind`，而 `parseWithFallback` 一旦整体解析失败，概览会连组件状态和指标一起丢。解析写成可缺省并落一个默认值（`"fault"`），语义上也对——旧后端只有业务故障场景。

**必须附的测试**（FR-013 / SC-010）：缺 `kind` 的响应、`kind` 为非字符串的响应，两者都不得让 `overviewSchema` 整体失败。

---

## D4 — 孤儿 span 的 `parent_span_id` 必须活过 `Sanitize`

**核实**：`log.go` 的 `Sanitize` 对 `Trace`/`Span`/`Parent`/`Operation`/`Run`/`ID` 逐个做

```text
if *p != "" && !hexID.MatchString(*p) { *p = "" }      // hexID = ^[a-f0-9]{16,32}$
```

**决定**：孤儿的父 id 用一个**固定的、合法的 16 位小写十六进制字面量**，并断言它不出现在该次运行的任何 `Span` 里。

**为什么必须固定而不是随机**：随机生成有碰撞的可能（16 位十六进制，碰撞概率极低但非零），而碰撞的后果不是报错，是**那一条静悄悄变成正常的父子关系**，`006-W-5` 于是验了个假形态。固定字面量把碰撞变成一条可断言的事实。

**为什么必须合法**：不合法会被 `Sanitize` 清空，清空后 `buildTraceWaterfall` 的 `resolveParent` 走 `if (!event.parentSpanId) return {parent: "", anomaly: null}` —— 该 span 变成**顶层节点且无异常标注**，而 `006-W-5` 要的是「显示为顶层并标注『父 span 不在本次运行内』」。两者在界面上只差一个标注，**跑矩阵的人极可能把它记成通过**。这是本特性里最容易悄悄失败的一处。

---

## D5 — sink 失败做成场景，不另开注入点（推翻 spec Q3 的暂定值）

**规格 Q3 的暂定值**：注入点复用 `APP_ENV=development && LORETIDE_DIAGNOSTICS_TEST=1`，由服务装配处一次性决定并注入，模块内不读环境变量。

**核实后的更简做法**：`Store.CommitRun(ctx, scope, run, failAudit bool)` **已经是这个模式**：

```text
if failAudit { if _, err := tx.Exec(ctx, "SELECT 1 / 0"); err != nil { return ErrUnavailable } }
```

它的唯一 `true` 调用方是 `Service.Run` 里 `scenario == "database"` 的分支，而 `Service.Run` 先调 `Simulate`，`Simulate` 首行就是门禁。**故障注入被一个场景 id 携带着穿过既有的门，没有第二道开关。**

**决定**：sink 写入失败照此办理——`Store.Technical` 增加一个失败开关参数，唯一的 `true` 来源是 `Service.Run` 里 `Kind == "shape"` 且该 shape 声明了 sink 失败。

**结果**：

- **不新增环境变量、不新增装配参数、不新增注入 API。** FR-016（「模块内 MUST NOT 读环境变量」）被平凡满足——本特性一个 `os.Getenv` 都不加。
- **恢复不需要动作**（FR-011 / SC-005）：失败只作用于那一次运行的写入，下一次运行照常。不需要重建实例，也不需要「关掉开关」这个会被忘记的步骤。
- Q3 提的「不经过 `Simulate` 的注入点没有现成的门」这个问题**消失了**，因为不再有不经过 `Simulate` 的注入点。

**主任务裁决若坚持 Q3 = A**：装配注入的做法同样能满足 FR-011/FR-015/FR-016，代价是多一个开关和一条「记得关掉」的人工步骤。本 plan 按上述更简做法推进，差异已在此记录。

**`dropped` 的两条来源，说清楚**：`store.go` 的失败路径同时 `Log.Errors.Add(1)` 与 `Log.Dropped.Add(1)`；`log.go:105` 的满环丢弃只加 `Dropped`。

- `002-V11-1`（`sink_errors > 0`）：由 sink 失败 shape 直接满足。
- `002-V11-2`（`dropped > 0`，原文括注「缓冲区溢出丢弃」）与 `002-V05-15`：由同一个 shape 满足**计数**，但它走的是写失败路径而非满环路径。**这个差别要写进 runbook 备注**，不能含糊——括注说的是来源，条目验的是「指标显示真实数值」，两者不冲突，但记录必须诚实。
- 满环路径顺带也变得可及：`LogBuffer` 容量 1000，一次 `shape_deep` 运行写 250 条，**4 次运行即可撑满环**，而今天需要约 112 次普通模拟。runbook 可以把它写成一个可选步骤。

---

## D6 — 三种回归状态各自的真实机制

`Run.Regression` 初值 `"not_run"`，但 `service.go:25` 无条件 `Evaluate(&run)`，而 `Evaluate` 只写 `passed` / `failed`。前端四态判定在 `regression.ts`：

```text
NEVER_EVALUATED = {"", "not_run"}                       → not_run
raw == "failed"                                         → failed
raw == "passed" && status ∈ {completed, failed}         → passed
其余                                                     → undecidable
```

| 条目 | 机制 | 为什么用这个机制 |
|---|---|---|
| `006-V-1` `not_run` | 该 shape 声明 `SkipEvaluate`，`Service.Run` 跳过 `Evaluate`，初值 `not_run` 落库 | 唯一能让 `not_run` 进库的办法就是别覆盖它 |
| `006-V-3` `failed` | **不做任何覆盖**：让该 shape 的 `Expected` 与它实际产出的 `Actual` 不相等，`Evaluate` 自己写出 `failed` | 走真实判定路径，验的才是真的判定。直接把 `Regression` 写成 `"failed"` 会让这条验收变成「界面能显示一个我们塞进去的字符串」 |
| `006-V-5` `undecidable` | `Regression` 为 `passed`，`Status` 为 `{completed, failed}` 之外的值（如 `running`） | `describeRegressionVerdict` 不改（FR-008）。`Run.Status` 不经 `Sanitize`（`Sanitize` 作用于 `Event`），可以承载这个值 |

**`SkipEvaluate` 放在哪**：放进 shape 表，由 `Service.Run` 按场景 id 查表。**不给 `Run` 加字段**——`Run` 整体序列化给面板，加字段就是又一次对外形状变更，还要再配一轮 zod 与畸形响应测试。

---

## D7 — `shape_deep` 的代价：写入慢，必须写进 runbook

**核实** `store.go` 的 `Technical`：每次调用开一个事务（含 `withWorkspaceWrite` 的工作区锁）、500ms 超时，末尾**各跑一次** `PruneTechnical`（一条 `DELETE ... WHERE received_at < $1 OR sequence < ...`）。`service.go` 的 `Run` 对 `run.Events` 逐条调用它，之后再 `PruneTechnical` 一次。

250 个事件 = 250 个事务 + 251 次 prune。开发实例上这是**秒级**操作。

**决定**：不做批量写入优化（原则 VIII：那是既有行为，不是本特性要修的缺陷），但 **runbook 的造数步骤必须写明这一步会明显变慢**。跑矩阵的人看到界面停住会以为挂了，然后刷新，然后拿到一个写了一半的运行——这比慢本身更糟。

**容量核对**：`MaxLogs` 10000、保留期 7 天。250 条一次、几次运行，不会触发裁剪，也不会把矩阵里其他条目要看的数据挤掉。

---

## D8 — `module` 为空能落库，`build` 那一半今天就能验

**核实**：`Sanitize` 只处理 `Event`；`Run` 的 `Module` / `Build` 不经过它。`commitRunWrite` 直接 `json.Marshal(saved)` 落 `content_diagnostic_run`。所以 `Module: ""` 能原样进库。

`006-L-4` 的原文是「`module` **或** `build` 为空」。`build` 来自 `os.Getenv("LORETIDE_BUILD")`，不设即为空——**这一半今天就能验**。本特性补的是 `module` 那一半。runbook 备注要把这个区别写出来，否则交付后没人知道这条到底验了哪一半。

---

## D9 — 负例的形状：每一类各一条，不是总共一条

FR-017 / SC-004 要求「每一类新形态各有一条断言」。**逐个场景一条**，而不是「shape 类整体一条」：一条整体断言只能证明当前这批被挡住，下一个人加第 10 个 shape 时它不会变红。

断言面：

1. `testEnabled == false` → 每个 shape id 调 `Simulate` 返回 `ErrDenied`，**且没有任何记录产生**（查库确认 `content_diagnostic_run` 无新行）。
2. `scope.Allows` 为假 → 同上。
3. 业务故障类的 id 集合，在本特性前后**逐项一致**（SC-002），且 shape 类与 `docs/13` §7 的 15 项交集为 0。
4. `Sanitize` 的允许值表（`codes`、`oneOf` 的各组允许值、`headerValueAdmitted`、`headerPresenceAdmitted`、`headerDenied`、`headerDeniedSuffix`）成员**逐项一致**（SC-006）。

第 4 条要找既有测试。`contract_test.go` / `log_test.go` 里若已有等价断言，**只在对照表里补引用，不重复写**（010 定下的口径）。

---

## D10 — constitution 引用更正

任务描述与规格 US2 / FR-017 把「隔离实例之外注入被拒绝」记作 **constitution 原则 VII**。核实 `.specify/memory/constitution.md`：**原则 VII 是「UI reuses Multica, it does not reinvent it」**。

这条规则的实际出处：constitution 的 **Development Workflow** 一节（隔离：一个任务、一个执行者、一个 worktree、自己的数据库与端口）与它指向的 `docs/development/ai-collaboration.md`，加上**原则 IX**（真实执行器保持禁用）。

**需求不变，引文更正。** 规格已同步。记在这里是因为：下一个读规格的人如果照 VII 去翻 constitution，会翻到一条不相干的 UI 规则，然后开始怀疑负例测试到底该断言什么。
