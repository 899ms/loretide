# Contract: 九个 shape 场景

每个场景一节：**产出什么**、**怎么算对**、**最容易悄悄失败在哪**。

共同前置：

- 全部经 `Simulate` 的既有门禁 —— `if !testEnabled || !scope.Allows(scope.Workspace, account) { return Run{}, ErrDenied }`。**不新增任何开关、环境变量或装配参数。**
- 全部事件经既有 `Sanitize`。**白名单不放宽。**
- 虚拟时钟基点沿用 `time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)`，同一 `seed` 产出逐字节可复现。

---

## 1. `shape_concurrent` → `006-W-3`

**产出**：两个 span，`Parent` 相同，时间区间重叠。

**算对**：

- 两者 `Parent` 相等且非空；两者互不为对方的 `Parent`。
- 区间重叠：`start_B < start_A + dur_A` 且 `start_A < start_B + dur_B`。
- 重叠**可见**：重叠段 ≥ 二者较短时长的 1/4，且 ≥ 100ms。

**最容易悄悄失败的地方**：重叠只有几毫秒。界面上两根条会挨在一起看不出重叠，跑矩阵的人记「未通过」，然后花半天查一个不存在的渲染缺陷。断言必须锁**重叠量**，不只是「重叠」。

---

## 2. `shape_orphan` → `006-W-5`

**产出**：一个 span 的 `Parent` 是合法格式但不属于本次运行。

**算对**：

- 父 id 通过 `hexID`（`^[a-f0-9]{16,32}$`），用**固定的 16 位小写十六进制字面量**。
- 断言该字面量**不出现在本次运行任何事件的 `Span` 里**。
- 经 `Sanitize` 之后 `Parent` **仍然非空**。

**最容易悄悄失败的地方**：父 id 不合法 → `Sanitize` 清空 → `buildTraceWaterfall` 的 `resolveParent` 走 `if (!event.parentSpanId) return {parent: "", anomaly: null}` → 该 span 变成**顶层且无异常标注**。而 `006-W-5` 要的是「顶层**并标注**『父 span 不在本次运行内』」。界面上两者只差一个标注，**极可能被记成通过**。所以「`Sanitize` 之后仍非空」必须是一条独立断言，不能靠构造时合法就算了。

---

## 3. `shape_single_span` → `006-W-6`

**产出**：恰好 1 个事件的运行。

**算对**：`len(run.Events) == 1`；该事件 `Parent == ""`；`buildTraceWaterfall` 返回 1 行、`collapsed == false`、`anomalies == 0`。

---

## 4. `shape_deep` → `006-W-7`、`006-W-8`、`008-O-3`

**产出**：> 200 个事件（建议 250），层级嵌套，含错误码的 span 位于深层且各级父节点齐全。

**算对**：

- `len(run.Events) > 200`。
- 至少一个事件 `Code != ""`，其 `Parent` 链一路可解析到根，**中间不缺节点**。
- `GetRun` 返回的事件数 **= 运行的实际事件数**（不是 100）。
- `buildTraceWaterfall(run.events, {})` → `collapsed == true` 且存在 `kind == "collapsed"` 的行。
- `buildTraceWaterfall(run.events, {cap: len})` → `collapsed == false`，层级完整，无悬空片段（`006-W-8`）。
- 折叠状态下，带错误码的 span 及其各级父节点**在 `rows` 里**。
- 250 个事件共享同一个 `ObjectID`；技术日志按 25 分页 → 该对象跨约 10 页（`008-O-3`）。

**读取侧的硬前提**：`Store.GetRun` 必须用既有游标内部翻页取全，天花板 500。不做这一步，前四条断言里有三条不成立——详见 research D2。

**已知界限（不改）**：`Service.Export` 的 `Filter{Limit: 100}` 会把导出的技术事件截在 100 条。既有行为，本特性不动，但 runbook 与导出相关条目的备注要写明。

**耗时**：250 个事务 + 251 次 `PruneTechnical`，秒级。runbook **必须**写明这一步会明显变慢，否则跑矩阵的人会刷新，然后拿到一个写了一半的运行。

---

## 5. `shape_not_run` → `006-V-1`

**产出**：`regression = "not_run"` 落库。

**算对**：`content_diagnostic_run` 的 payload 里 `regression` 字面值为 `not_run`（**查库断言，不是查内存中的 `Run`**）；前端 `describeRegressionVerdict` 判为 `not_run`。

**最容易悄悄失败的地方**：断言写在 `Simulate` 的返回值上。`Run` 的初值本来就是 `not_run`，那条断言**今天不改任何代码也是绿的**——它证明不了 `Evaluate` 被跳过了。断言必须落在库里。

---

## 6. `shape_regression_failed` → `006-V-3`

**产出**：`regression = "failed"`。

**算对**：该场景的 `Expected` 与它实际产出的 `Actual` 不相等；`Evaluate` **被正常调用**并写出 `failed`；库里为 `failed`。

**为什么不直接写**：直接把 `Regression` 赋成 `"failed"` 会让这条验收退化成「界面能显示一个我们塞进去的字符串」。走真实判定路径，验的才是真的判定（research D6）。

---

## 7. `shape_undecidable` → `006-V-5`

**产出**：前端判为「无法判定」。

**算对**：`Regression == "passed"` 且 `Status ∉ {completed, failed}`；`describeRegressionVerdict` 返回 `undecidable`；界面同时可见两个原始值（手动验收）。

**MUST NOT**：改动 `describeRegressionVerdict`（FR-008）。这条验收要验的就是那段判定逻辑；为了让它通过而改它，等于把尺子改成被量的东西。

---

## 8. `shape_no_module` → `006-L-4`（`module` 那一半）

**产出**：`Run.Module == ""` 落库。

**算对**：库里 payload 的 `module` 为空字符串；界面显示「未记录」，不是空白、不是推断值。

**备注必须写清**：`006-L-4` 原文是「`module` **或** `build` 为空」。`build` 那一半**今天就能验**（不设 `LORETIDE_BUILD`）。本形态补的是 `module` 那一半。不写清楚，交付后没人知道这条到底验了哪一半。

---

## 9. `shape_sink_failure` → `002-V11-1`、`002-V11-2`、`002-V05-15`

**产出**：`sink_errors > 0` 且 `dropped > 0`。

**算对**：

- 跑完该场景后 `Overview.metrics.sink_errors` 与 `dropped` 均 > 0。
- **恢复不需要动作**：紧接着跑一次普通场景，两个计数**不再增长**，技术日志写入正常（`SC-005`）。
- `content_operation_audit` 里**有**该次运行的审计行 —— 失败的是技术日志，不是审计，「审计写失败即整体回滚」这条不变量未被触碰。

**来源要诚实**：`002-V11-2` 原文括注「缓冲区溢出丢弃」。本形态走的是**写失败**路径（`store.go` 的失败分支同时 `Errors.Add(1)` 与 `Dropped.Add(1)`），不是满环路径（`log.go:105`）。条目验的是「指标显示真实数值」，两者不冲突，但 runbook 备注**必须写明走的是哪条路径**。

**满环路径顺带变得可及**：`LogBuffer` 容量 1000，一次 `shape_deep` 写 250 条，**4 次即可撑满环**（今天需要约 112 次普通模拟）。runbook 可写为可选步骤。

---

## 负例（对每一个场景，各一条）

| 条件 | 期望 |
|---|---|
| `testEnabled == false` | `Simulate` 返回 `ErrDenied`，**且 `content_diagnostic_run` 无新行** |
| `scope.Allows` 为假 | 同上 |

**逐个场景一条，不是整体一条。** 一条整体断言只能证明当前这批被挡住；下一个人加第 10 个 shape 时它不会变红（research D9）。
