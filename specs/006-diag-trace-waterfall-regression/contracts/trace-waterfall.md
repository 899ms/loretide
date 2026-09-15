# Contract: `buildTraceWaterfall`

**位置**：`packages/core/content/diagnostics/trace-waterfall.ts`

**消费者**：`packages/views/content/diagnostics/index.tsx` 的 trace 标签页。纯函数，无 DOM 依赖，可在 node 环境测试。

## `occurred_at` 的语义（2026-09-15 由 `specs/011` 钉死）

> **`occurred_at` 是该事件所代表步骤的「开始」时刻。**
> 一个步骤占据的时间区间是 **`[occurred_at, occurred_at + duration_ms)`**。

本合同此前**没有写下这句话**，后果是生产者与消费者各自理解、各自自洽、合起来错：

| 端 | 此前的理解 |
|---|---|
| `simulator.go` | 先 `now = now.Add(duration)` 再盖 `Occurred` → 写的是**结束**时刻 |
| `trace-waterfall.ts:72` | 注释写 "earliest parseable **start**"，按 `[occurred, occurred+duration]` 画 → 读的是**开始**时刻 |

于是每根条子整体右移自己的时长，相邻步骤之间长出假间隙（`006-W-2` / `006-W-9`）。**修的是生产者，不是本算法**——本算法对「开始时刻」的处理一直是对的。

完整根因、修复前后对照与不变量：`specs/011-diag-panel-time-and-jump/contracts/span-timing.md`。

**一条例外**：`clock_skew` 场景**故意**违反「相邻步骤首尾相接」，让某步的 `occurred_at` 早于其父 span。那正是它要模拟的故障，也是 `clockSkew` 标注唯一的真实来源。任何「全部场景时间单调」的断言都必须排除它。

## 签名

```text
buildTraceWaterfall(events: DiagnosticEvent[], options?: { cap?: number }) -> WaterfallResult
```

- `events`：已经过 `eventSchema` 解析的事件数组。**不接受**原始网络 JSON——解析是调用方的责任（constitution 原则 VI）。
- `options.cap`：span 上限，默认取 `STREAM_EVENT_CAP`（=200，来自 `contract.ts`）。仅为测试可注入而存在，生产调用不传。

## 返回

```text
WaterfallResult = {
  rows: WaterfallRow[]        // 已按显示顺序排好，含折叠占位行
  totalSpans: number          // 折叠前的 span 总数
  collapsed: boolean          // 是否发生了折叠
  anomalies: number           // rows 中 anomaly 非 null 的条数
}
```

`rows` 的顺序即渲染顺序：同一父节点下按 `startOffsetMs` 升序，父节点紧邻其子节点之前。

## 不变量（每条都必须有独立用例）

1. **终止性**：对任意输入——含父子成环、自引用、超长父链——函数在有限步内返回，不抛异常、不栈溢出（FR-003）。
2. **层级完整性**：`rows` 中任一 `kind="span"` 行，其父要么也在 `rows` 中，要么该行 `depth === 0`。**不存在悬空片段**（FR-004）。
3. **失败可见性**：任一 `errorCode` 非空的 span 必定出现在 `rows` 中，且其完整祖先链也在 `rows` 中——即使发生折叠（FR-004 / D3）。
4. **非负偏移**：所有 `startOffsetMs >= 0`，且至少有一行为 0（基准点自身）。空输入除外。
5. **无 NaN**：任何 `startOffsetMs` / `durationMs` 均不为 `NaN`，即使 `occurredAt` 无法解析（D2）。
6. **不夹取**：`clockSkew` 行的 `startOffsetMs` 是其真实偏移，**不**被修正到父区间内（FR-014）。
7. **数量守恒**：`totalSpans === events.length`；所有 `collapsed` 行的 `collapsedCount` 之和 + `kind="span"` 行数 === `totalSpans`。

## 边界输入

| 输入 | 期望 |
|---|---|
| `[]` | `rows: []`, `totalSpans: 0`, `collapsed: false`；调用方据此显示「本次运行没有可展示的 span」 |
| 单个 span | 一行，`depth: 0`, `startOffsetMs: 0` |
| 全部 `durationMs` 为 0 | 正常返回，不除零；宽度由调用方按最小可见宽度渲染 |
| 某 span `durationMs` 为负 | 该行 `durationMs` 归一为 0 且 `anomaly: "invalidDuration"`；不得产生负宽度（spec Edge Cases / data-model 异常优先级第 5 级） |
| `parentSpanId` 指向不存在的 span | 该行 `depth: 0`, `anomaly: "orphan"` |
| `a→b→a` 成环 | 环上节点 `depth: 0`, `anomaly: "cycle"`，函数返回 |
| 自引用（`parentSpanId === spanId`） | 同上，按 `cycle` 处理 |
| 超过 `cap` 个 span | `collapsed: true`；不变量 3 仍成立 |

## 保密

输入已是脱敏后的 `Event`（`safe_message` 为固定枚举）。本函数**不新增任何字段拼接**，不会把原本不在 `Event` 中的内容带入输出。
