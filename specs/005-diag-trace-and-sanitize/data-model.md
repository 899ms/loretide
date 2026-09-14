# Data Model: 005 诊断 HTTP 追踪贯通与请求脱敏

**无数据库变更、无迁移。** 技术事件与审计事件以整条 `Event` 的 JSON 存入既有 `payload` JSONB 列（`store.go` 的三处 `INSERT INTO content_*(…, payload)`），增字段不触及任何列定义。以下是线上契约与进程内结构的形状变化。

## Event（线上，技术日志 / 审计 / 导出共用）

| 字段 | 类型 | 变化 |
|---|---|---|
| 既有全部字段 | — | 不变 |
| **route** | string，可选，默认 `""` | **新增**：请求身份的前两项，形如 `GET /api/content-diagnostics/events`（路由模板 + 方法）。不含原始路径、查询串取值与键名、路径哈希 |
| **status** | number，可选，默认 `0` | **新增**：请求身份的第三项，响应状态码。与 `route` 合起来构成「路由模板 + 方法 + 状态码」三元组 |
| **headers_present** | string[]，可选，默认 `[]` | **新增**：第二档（只记存在）中**确实出现**的请求头**名称**列表，取值恒为第二档名单的子集。只有名称，没有取值、长度或摘要 |
| **upstream_trace** | string，可选，默认 `""` | **新增**：入站 `traceparent` 的关联属性形式。仅在校验通过且**未**被采信为父级时写入；被采信时不写（此时 `trace_id` 本身即是它）。**不作查询键、不跨 workspace 关联、不用于授权** |

四个字段都带 `omitempty`，旧读者忽略未知字段（`contract.test.ts` 的「accepts additive fields」用例已覆盖该方向）。

客户端 `eventSchema` 相应新增：

```text
route:           z.string().optional().default("")
status:          z.number().optional().default(0)
headers_present: z.array(z.string()).optional().default([])
upstream_trace:  z.string().optional().default("")
```

transform 后为 `route` / `status` / `headersPresent` / `upstreamTrace`，经 `parseWithFallback`。

## Trace 上下文（进程内，跨中间件传递）

```text
traceID:    本实例生成，或（仅 daemon machine credential 路径）取自入站 traceparent
spanID:     每段独立
parentSpan: 取自上一段
sampled:    采样位；不因取值为「不采样」而丢弃 traceID
candidate:  校验通过的入站 traceparent，等待认证后决定是否提升为父级
```

候选父级的生命周期：

| 阶段 | 位置 | 动作 |
|---|---|---|
| 1 | 全局中间件（认证之前） | 建本实例 trace；校验入站 `traceparent`，通过则存为 `candidate`，不通过则丢弃且不留存 |
| 2 | `DaemonAuth` 之后（仅 daemon 组） | 把 `candidate` 提升为父级，`trace_id` 改用它 |
| 3 | 其余全部路由 | `candidate` 不提升；若该请求写技术事件，则以 `upstream_trace` 形式记录 |
| 4 | `Pack(ctx,…)` | 从 ctx 注入 carrier，队列与 WS 两段沿用（**既有代码，不改**） |

## 请求头准入名单（三档）

```text
第一档 记值:     traceparent, tracestate, x-diagnostic-trace, x-request-id,
                 x-workspace-id, content-type, content-length
第二档 只记存在: user-agent, accept          （不记值、不记长度）
第三档 一律不记: authorization, cookie, set-cookie, x-api-key,
                 proxy-authorization, 以及 *-token / *-secret / *-key / *-password
默认:            未列入者不记
多值:            不拼接，整体按未列入处理
优先级:          第三档优先于第一、二档
```

逐名清单与理由见 `contracts/request-sanitization.md`。

## 待派发记录（进程内，不落库）

```text
{ id: string, kind: string, payload: []byte, idempotencyKey: string }
```

| 属性 | 规则 |
|---|---|
| 生死 | 与登记它的事务绑定：回滚则不存在、不派发 |
| 派发 | 提交后触发；失败可见（计入既有 `LogBuffer.Errors` / `Dropped`），不拖垮业务 |
| 幂等 | 同 `idempotencyKey` 重复派发与既有 `Receiver` 去重语义一致 |
| 有界 | 队列有上限，超出按既有「丢弃可见」口径处理 |
| **持久性** | **无**。进程退出丢失未派发项——FR-010a 与 US3 场景 6 已把它写成验收项，不得描述为已保证 |

字段选择理由见 research D7：四元组是持久实现所需的最小集合，因此换成落库实现时调用方不必改。
