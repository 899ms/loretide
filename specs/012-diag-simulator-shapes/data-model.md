# Phase 1 Data Model: 诊断模拟器的数据形态

**不新增表，不新增迁移。** 所有形态写进既有的 `content_diagnostic_run`、`content_technical_log`、`content_operation_audit`。constitution 原则 V 因此不适用于本特性。

---

## 1. `Scenario`（对外形状，改动）

```text
type Scenario struct {
    ID       string `json:"id"`
    Expected string `json:"expected_code"`
    Kind     string `json:"kind"`          // 新增
}
```

| 字段 | 取值 | 规则 |
|---|---|---|
| `Kind` | `"fault"` \| `"shape"` | 每一项**必须**是二者之一。`"fault"` 对应 `docs/13` §7 的业务故障；`"shape"` 是诊断自验专用的数据形态 |

**不变量**

- **INV-1**：`Kind == "fault"` 的 id 集合，本特性前后**逐项一致**——即仍然恰好是原来的 16 项（`normal` `slow` `timeout` `cancel` `reconnect` `duplicate` `late` `file_missing` `file_changed` `denied` `database` `model_auth` `model_quota` `schema` `search` `clock_skew`）。
- **INV-2**：`Kind == "shape"` 的 id 与 `docs/13` §7 的 15 项**交集为空**。
- **INV-3**：每个 `Kind == "shape"` 的项都能指到它服务的验收条目编号（下表第 3 节），指不到的项数为 0。
- **INV-4**：`shape_` 前缀是**可读性约定**，`Kind` 才是规则。任何代码 **MUST NOT** 通过解析 id 前缀来判断分类——今天两者一致，明天改名就不一致了。

**对外形状的后果**：见 `contracts/scenario-kind.md`。

---

## 2. Shape 表（模块内部，不对外）

`shapes.go` 里的一张查表，键是场景 id：

```text
type shape struct {
    ID           string   // = Scenario.ID
    Items        []string // 服务的验收条目编号，INV-3 靠它成立
    SkipEvaluate bool     // true → Service.Run 不调 Evaluate
    FailSink     bool     // true → Service.Run 让该次运行的技术日志写入失败
    Status       string   // 非空则覆盖 Run.Status
    Module       string   // 非空则覆盖 Run.Module；显式空值由 EmptyModule 表达
    EmptyModule  bool     // true → Run.Module = ""
}
```

**为什么 `EmptyModule` 是独立布尔而不是让 `Module: ""` 表示空**：零值歧义。`Module: ""` 在 Go 里既是「不覆盖」也是「覆盖为空」，而 `006-L-4` 要的恰恰是后者。用一个布尔把两种意思分开，比在注释里解释一个零值可靠。

**为什么这张表不进 `Run`**：`Run` 整体序列化给面板。加字段 = 又一次对外形状变更 = 又一轮 zod 同步与畸形响应测试。查表在模块内部，对外不可见。

---

## 3. 九个形态

| # | 场景 id | 服务的条目 | 产出 | 关键不变量 |
|---|---|---|---|---|
| 1 | `shape_concurrent` | `006-W-3` | 两个 span 共用同一个父，且时间区间**可见地重叠** | 重叠必须是**人眼可分辨**的量级（建议 ≥ 100ms 且重叠段 ≥ 该 span 时长的 1/4），不是差几毫秒。二者**互不为父子** |
| 2 | `shape_orphan` | `006-W-5` | 一个 span 的 `Parent` 是合法格式但不属于本次运行 | 父 id 是**固定的 16 位小写十六进制字面量**，必须通过 `hexID`，且**断言它不出现在本次运行的任何 `Span` 里**（research D4） |
| 3 | `shape_single_span` | `006-W-6` | 恰好 1 个事件的运行 | `len(run.Events) == 1`；该事件 `Parent == ""` |
| 4 | `shape_deep` | `006-W-7`、`006-W-8`、`008-O-3` | **> 200** 个事件，层级嵌套，**含错误码的 span 位于深层**且其各级父节点齐全 | 见下方「4. `shape_deep` 的额外约束」 |
| 5 | `shape_not_run` | `006-V-1` | `regression = "not_run"` **落库** | `SkipEvaluate = true`。库里 `content_diagnostic_run` 的 payload 里 `regression` 字面值为 `not_run` |
| 6 | `shape_regression_failed` | `006-V-3` | `regression = "failed"` | **不覆盖**：`Expected != Actual`，由 `Evaluate` 自己写出 failed（research D6） |
| 7 | `shape_undecidable` | `006-V-5` | 前端判为「无法判定」 | `Regression = "passed"` 且 `Status ∉ {completed, failed}`。**MUST NOT 改动 `describeRegressionVerdict`**（FR-008） |
| 8 | `shape_no_module` | `006-L-4`（`module` 那一半） | `Run.Module == ""` | `EmptyModule = true`。`build` 那一半今天已可验（不设 `LORETIDE_BUILD`），备注要写清区别（research D8） |
| 9 | `shape_sink_failure` | `002-V11-1`、`002-V11-2`、`002-V05-15` | `sink_errors > 0` 且 `dropped > 0` | `FailSink = true`。**恢复不需要动作**：失败只作用于该次运行的写入（research D5） |

**覆盖核对**：9 个形态覆盖 13 条；`006-L-5` 与 `008-O-2` 显式移出（FR-020 / FR-021）；13 + 2 = **15**，等于任务描述 (a)～(h) 点名的条数。任务描述所说的 19 条与此差 4 条，待主任务补全（spec Q1）。

---

## 4. `shape_deep` 的额外约束

这一条是九个里唯一一个产出跨越了「读取」而不只是「写入」的。

| 约束 | 值 | 来源 |
|---|---|---|
| 事件数 | **> 200**，建议 250 | 折叠条件是 `order.length > cap`，`cap = STREAM_EVENT_CAP = 200` |
| 运行详情必须取全 | `GetRun` 内部翻页，天花板 **500** | research D2。100 条的截断会让折叠提示永不出现 |
| 失败 span 的位置 | 深层，且**其各级父节点齐全** | `006-W-7` 要验的是「折叠状态下失败步骤及其各级父节点仍可见」。父链断了就验不到这条 |
| 同对象跨页 | 250 个事件共享同一个 `ObjectID`（`run.ID`），技术日志分页 25 → 约 10 页 | `008-O-3` 要「同一对象的事件跨多页」。`index.tsx:574` 的 `limit: "25"` 是常量 |
| 写入耗时 | 秒级，**runbook 必须写明** | research D7：250 个事务 + 251 次 prune |

**已知界限，不在本特性范围**：`Service.Export` 用 `Filter{Limit: 100}` 取审计与技术事件，一次 250 span 的运行**导出后只带 100 条技术事件**。这是既有行为（原则 VIII），本特性不改，但 contracts 与 runbook 都要写明，免得下一个人以为导出是全量。

---

## 5. 读取路径的改动边界

| 位置 | 改动 | 不改 |
|---|---|---|
| `Store.GetRun` | 用既有 `Filter.After` 游标内部翻页，直到该运行事件取尽或达 500 条天花板 | 端点、响应形状、`Run` 结构 |
| `Store.Query` | — | **100 的上限夹取原样保留**。它同时服务对外的 `/events` 与 `/stream` |
| `handler.diagnosticFilter` | — | **`f.Limit > 100 → ErrConflict` 原样保留**。对外分页边界不放宽 |
| `Store.Technical` | 增加失败开关参数，唯一 `true` 来源是 `Service.Run` 的 shape 分支 | 成功路径、`Sanitize`、计数器语义 |
| `packages/core/.../contract.ts` | `overviewSchema` 的 `scenarios` 增加**可缺省**的 `kind` | 其余字段、`transform` 的输出键名 |

**没有新增端点，没有新增查询参数，没有新增前端请求。**

---

## 6. 与既有不变量的关系（FR-018 / SC-006）

| 不变量 | 本特性的影响 | 怎么证明 |
|---|---|---|
| `Sanitize` 白名单 | **零影响**。shape 事件与业务故障事件走同一个 `Sanitize` | 允许值表成员逐项一致的断言（research D9 第 4 条）。既有测试若已覆盖，只补对照表引用，不重写 |
| 审计写失败即整体回滚 | **零影响**。`shape_sink_failure` 让**技术日志**写入失败，不是审计 | 该 shape 跑完后 `content_operation_audit` 里有该次运行的审计行 |
| 真实执行器保持禁用 | **零影响**。全部形态是虚拟时间下的构造数据 | 既有的执行闸门测试不变红；无新增默认测试解析 agent CLI |
| 隔离门禁 | **加固**：9 个 shape 各一条负例 | research D9 第 1、2 条 |
