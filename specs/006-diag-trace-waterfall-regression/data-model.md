# Data Model: 006 诊断 trace 瀑布视图与回归关联

**全部实体均为派生视图，不持久化、不进数据库、不进 Zustand。** 本功能不新增或修改 `Event` / `Run` 的字段集合（FR-010）。下表的「来源」列说明每个字段由哪个已有字段算出。

## WaterfallRow（由一条 `DiagnosticEvent` 派生）

| 字段 | 类型 | 取值域 | 来源 |
|---|---|---|---|
| `eventId` | string | 非空 | `event.eventId`（原样） |
| `spanId` | string | 可为空串 | `event.spanId`（原样） |
| `parentSpanId` | string | 可为空串 | `event.parentSpanId`（原样） |
| `depth` | number | ≥ 0 整数 | 自顶向下遍历赋值；顶层为 0 |
| `startOffsetMs` | number | ≥ 0 | `Date.parse(occurredAt) − 本次运行最小 occurredAt` |
| `durationMs` | number | ≥ 0 | `event.durationMs`，负值归一为 0 并置 `invalidDuration` |
| `anomaly` | enum \| null | `orphan` / `cycle` / `clockSkew` / `invalidTime` / `invalidDuration` / `null` | 见下方判定 |
| `collapsedCount` | number | ≥ 0 | 仅 `kind="collapsed"` 行非 0，表示被折叠的后代条数 |
| `kind` | enum | `span` / `collapsed` | 折叠占位行为 `collapsed` |

### `anomaly` 判定优先级

同一行只带一个 `anomaly`，按以下优先级取第一个命中的：

1. `cycle` — 父链成环（D1 的访问集合命中）
2. `orphan` — `parentSpanId` 非空但不在本次运行的 span 索引内
3. `invalidTime` — `occurredAt` 无法解析为有效时间
4. `clockSkew` — 自身 `occurredAt` 早于父的 `occurredAt`
5. `invalidDuration` — `durationMs` 为负

`parentSpanId` 为空串是正常的根节点，`anomaly` 为 `null`，**不**记为 `orphan`。

## RegressionVerdict（由一条 `DiagnosticRun` 派生）

| 字段 | 类型 | 取值域 | 来源 |
|---|---|---|---|
| `verdict` | enum | `passed` / `failed` / `not_run` / `undecidable` | 见 `contracts/regression-verdict.md` 真值表 |
| `rawRegression` | string | 任意（含空串） | `run.regression`（原样保留，供「无法判定」时复核） |
| `expectedCode` | string | 任意 | `run.expectedCode` |
| `actualCode` | string | 任意 | `run.actualCode` |
| `basisAvailable` | boolean | — | `expectedCode` 与 `actualCode` 是否都非空；`passed` 必须为 `true` 才可展示判定依据（FR-006） |

**不变量**：`verdict === "passed"` 当且仅当 `rawRegression === "passed"` 且 `status` 不指示未完成。任何其他情形一律不是 `passed`（FR-005）。

## RunLinkage（由一条 `DiagnosticRun` 派生）

四项定位，每项独立带可用性状态。

| 字段 | 类型 | 取值域 |
|---|---|---|
| `originalRun` | `{ runId: string; state: enum }` | state: `linkable` / `self` / `unreadable` / `missing` |
| `scenario` | `{ value: string; state: enum }` | state: `present` / `missing` |
| `module` | `{ value: string; state: enum }` | state: `present` / `missing` |
| `build` | `{ value: string; state: enum }` | state: `present` / `missing` |

### `originalRun.state` 的四态含义（不得互相混用，FR-007）

| state | 触发条件 | 界面含义 |
|---|---|---|
| `self` | `run.originalRunId` 为空串 | 「本身即原故障」——这是首次运行，不是断链 |
| `linkable` | `originalRunId` 非空且该运行在当前已取回的运行列表中可见 | 可点击跳转（页内切换，FR-008） |
| `unreadable` | `originalRunId` 非空但目标不在可读集合内（保留期裁剪或越权） | 「原故障运行不可读」+ 原因类别 |
| `missing` | `originalRunId` 非空但取值不合法（非预期格式） | 「未记录」 |

`scenario` / `module` / `build` 的 `missing` 判定统一为「空串」，显示为「未记录」，**不显示为空白，也不推断替代值**。
