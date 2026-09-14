# Contract: describeTraceJump / describeObjectVersions

`packages/core/content/diagnostics/linkage.ts`。规范层测试：`linkage.test.ts`（`// @vitest-environment node`）。

## describeTraceJump(event) → TraceJump

### 不变量

1. **产出二选一且互斥**：要么 `kind:"filter"`，要么 `kind:"unavailable"`。不存在第三种，也不存在「filter 但字段为空」的中间态。
2. **空追踪编号绝不产生 filter**：`event.traceId === ""` 时恒为 `{kind:"unavailable", reason:"noTrace"}`。这是 FR-002，也是本次要修的缺陷。
3. **清空是产出的一部分**：返回的 filter 不含 `component` / `severity` / `errorCode` / `from` / `until` 任何值，`after` 恒为 `0`。调用方照搬即可，不需要自己记得清。
4. **纯函数**：同一事件恒返回相等结果；不修改入参；不发请求。
5. **只用现有筛选维度**：产出的 filter 字段是既有事件查询已支持的子集（`kind` / `traceId` / `runId` / `after`），不引入任何新查询参数。

### 边界表

| 输入 | 期望 |
|---|---|
| `traceId` 与 `runId` 均非空 | `{kind:"filter", filter:{kind:"technical", traceId, runId, after:0}}` |
| `traceId` 非空、`runId` 空 | 同上，`runId:""`，不参与筛选（FR-003、US1 场景 3） |
| `traceId` 空、`runId` 非空 | `{kind:"unavailable", reason:"noTrace"}`——**不得**退化为按 runId 跳转（Q1 裁决 A 排除了 B） |
| `traceId` 与 `runId` 均空 | `{kind:"unavailable", reason:"noTrace"}` |
| `traceId` 为非法格式（非 32 位十六进制） | 与空串同判：`unavailable`。理由：`Sanitize` 在服务端已会清空，客户端不做第二套格式判定，只认「有没有」 |
| 事件带有上一次筛选残留的无关字段 | 产出不受影响——产出只由该事件决定 |

## describeObjectVersions(events, target) → ObjectVersionGroup

### 不变量

1. **只在传入数组内归拢**，不发请求（FR-004、FR-019）。
2. **去重且保序**：同一版本只出现一次，顺序为首次出现顺序（不是字典序——读者要看的是「先后」）。
3. **不伪造版本**：目标对象的事件都没有版本时，`state:"unversioned"` 且 `versions` 为空数组，绝不填入占位值（FR-007）。
4. **计数守恒**：各 `entry.count` 之和等于数组中 `objectType` + `objectId` 同时匹配且版本非空的事件数。
5. **纯函数**：不修改入参数组。

### 边界表

| 输入 | 期望 |
|---|---|
| 空事件数组 | `versions: []`，`state:"unversioned"` |
| 目标 `objectType` 或 `objectId` 为空 | `state:"unknownObject"`，`versions: []` |
| 目标对象有 3 条事件、2 条带同一版本、1 条带另一版本 | 2 个 entry，count 分别 2 与 1，顺序按首次出现 |
| 目标对象的事件全部 `objectVersion:""` | `state:"unversioned"`，`versions: []` |
| 数组里混有其它对象的事件 | 只归拢 `objectType` 与 `objectId` **同时**匹配的，不按单一维度匹配 |
| 同一 `objectId` 但不同 `objectType` | 视为不同对象，不合并 |

## 面板侧（不写 UI 单测，进手动清单）

- `kind:"filter"` → 按 filter 设置既有 state 并切到技术日志页，行为与今天一致。
- `kind:"unavailable"` → 「查看追踪」置为不可用，并给出说明文案；**不得**切页。
- 对象版本 → `present` 列出；`unversioned` 显示「未记录版本」；`unknownObject` 不显示该区块。

以上三条界面行为按原则 II **不写自动测试**，进 `manual-ui-todo.md`（J-1 ～ J-4、O-1 ～ O-3）。
