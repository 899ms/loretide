# Phase 1 Data Model: 诊断关联跳转与不变量断言

**无数据库 schema 变更、无迁移、无新端点。** 本文件描述的是两个新纯函数的输入输出形状，以及四项断言所依据的既有结构。

## 1. 新增派生类型（`packages/core/content/diagnostics/linkage.ts`）

### TraceJump — `describeTraceJump` 的产出

```
TraceJump =
  | { kind: "filter";      filter: TraceJumpFilter }
  | { kind: "unavailable"; reason: TraceJumpBlocker }
```

两分支**互斥**，调用方必须分支处理，无法把「不可跳转」当成一组空筛选用掉——这是 FR-002 的类型级保障。

**TraceJumpFilter**

| 字段 | 来源 | 规则 |
|---|---|---|
| `kind` | 固定 `"technical"` | 跳转目标恒为技术日志 |
| `traceId` | `event.traceId` | 非空，否则走 `unavailable` |
| `runId` | `event.runId` | 可为空串；空则不参与筛选（FR-003、US1 场景 3） |
| `after` | 固定 `0` | 游标归零 |

被**显式清空**的维度：`component`、`severity`、`errorCode`、`from`、`until`。清空是产出的一部分，不是调用方的自觉——FR-003。

**TraceJumpBlocker**

| 值 | 含义 |
|---|---|
| `noTrace` | 事件没有可用追踪编号（原本为空，或被 `Sanitize` 按 `hexID` 清空） |

当前只有一个值。用具名联合而非布尔，是为了将来新增阻断原因时调用方必须处理（服务端枚举 switch 需 `default` 的同一理由）。

### ObjectVersionGroup — `describeObjectVersions` 的产出

```
ObjectVersionGroup = {
  objectType: string
  objectId: string
  versions: ObjectVersionEntry[]   // 按首次出现顺序，去重
  state: "present" | "unversioned" | "unknownObject"
}
```

| `state` | 何时 | 呈现 |
|---|---|---|
| `present` | 该对象至少有一条事件带非空 `objectVersion` | 列出版本 |
| `unversioned` | 该对象的事件都没有记版本 | 「未记录版本」（FR-007），**不伪造** |
| `unknownObject` | 目标事件的 `objectType` 或 `objectId` 为空 | 无法归拢 |

**ObjectVersionEntry**：`{ version: string; firstEventId: string; count: number }`——`firstEventId` 让调用方能定位到具体事件，`count` 说明该版本出现过几次。

**作用域**：只在**传入的事件数组**内归拢。该数组是调用方已经取回并经 `eventSchema` 解析的那一页，**本函数不发起任何请求**（FR-004、FR-019）。跨页同对象事件看不全——Q2-A 的已知代价。

## 2. 既有结构（本特性只读，不修改）

### Event（`server/internal/content/diagnostics/contract.go` / `contract.ts`）

本特性用到的字段，全部**已存在**：

| 字段 | 用于 |
|---|---|
| `trace_id` / `traceId` | G1 跳转键、G6 第一跳 |
| `run_id` / `runId` | G1 附加筛选、G6 第二跳 |
| `object_type` / `object_id` / `object_version` | G2 归拢 |
| `error_code` / `errorCode` | G3 映射输入 |
| `next_action` / `nextAction`、`retryable` | G3 断言对象 |

### Snapshot（G4 / G5 的对象）

G4 断言的**预期字段清单**（16 个，来自 `contract.go` `Snapshot`）：

`ConfigVersion` `PersonaRef` `SOPVersion` `SkillVersion` `RuleVersion` `Executor` `ExecutorVersion` `Scope` `Preference` `Required` `Excluded` `Grants` `Hashes` `Temperature` `Budget` `Timeout`

全部是版本串、引用串、哈希、字符串数组或数值。**无 `[]byte`、无媒体类型、无任意长度载荷。** 清单任一处不匹配（新增、删除、改名、改类型）即断言失败。

### Run（G6 链条的中段与末段）

| 字段 | 用于 |
|---|---|
| `run_id` | 第二跳的落点、第三跳的起点 |
| `original_run_id` | 第三跳；空串表示「本身就是原故障」，非空时须能查到 |
| `snapshot` | G5 的断言对象 |

## 3. 断言关系图（G6）

```
一次真实复现
    │
    ├─ 审计事件 ──(trace_id)──▶ 技术事件        第一跳
    │                              │
    │                        (run_id)           第二跳
    │                              ▼
    │                            运行
    │                              │
    │                     (original_run_id)     第三跳
    │                              ▼
    └────────────────────────▶ 原故障运行
```

每一跳断言**标识符对得上**，而不是各跳分别非空（FR-017）。

**必须显式覆盖的退化**：`store.go:163` 的 SQL 采用 `($4='' OR payload->>'trace_id'=$4)`——空值即不加该条件。因此拿空标识符查询会返回**全部**记录而非零条。FR-018 要求断言空标识符不被当作通配，这条不是理论边界，是 SQL 的实际形状。

## 4. 状态转换

本特性**不引入任何状态机**。两个新函数都是纯函数：同一输入恒等同一输出，不持有状态、不产生副作用、不发请求。四项后端工作全是断言，不改变任何运行时状态。
