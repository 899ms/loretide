# Contract: HTTP trace 传播

## 适用范围

**传播**：全部 API 路由（公共中间件层）。**记录**：仅 `/api/content-diagnostics` 路由组写技术事件——传播变全局不改变记录范围（spec FR-015）。

## Request

可选请求头 `traceparent`，W3C Trace Context 格式：`00-<32 hex trace-id>-<16 hex span-id>-<2 hex flags>`。

## 服务端行为

| 来源 | 判定依据 | 入站 `traceparent` 的处置 |
|---|---|---|
| 本实例 daemon | `DaemonAuth` 通过后由认证中间件写入的 `X-Actor-Source`（客户端提供的值已被剥离） | **采信为父级**；本段 trace 沿用其 trace-id |
| 浏览器 / 任何用户凭据 | 同上 | **不采信**。仅在该请求写技术事件时以 `upstream_trace` 关联属性记录 |
| 格式非法 / 校验不通过 | 与 `Unpack` 一致的校验 | **既不采信也不留存**；不使请求失败；拒绝可见 |
| 缺失 | — | 正常路径，建本实例 trace |

无论哪一行，服务端**总是**为请求建立一个本实例 trace，请求绝不因 `traceparent` 而失败。

## Response

- `X-Diagnostic-Trace: <32 hex trace-id>` —— **既有头，形状不变**，安装版客户端可能已依赖。
- 本功能不在响应上新增 `traceparent`。

## 下游传播

| 载体 | 机制 | 是否本功能改动 |
|---|---|---|
| 队列 / WebSocket | `Pack(ctx,…)` 把 ctx 中的 span context 注入 `Envelope.Carrier` | **否**——既有代码。边界修好后自动携带正确的 trace |
| daemon HTTP 出站 | `server/internal/daemon/client.go` 注入 `traceparent` 请求头 | 是 |

## Client obligations

- 不得依赖服务端采信自己发出的 `traceparent`；服务端返回的 `X-Diagnostic-Trace` 才是本次请求的权威标识。
- 不得把 trace 标识当作凭据或授权依据。

## 不变量

- `upstream_trace` **MUST NOT** 作为查询键、**MUST NOT** 用于跨 workspace 关联、**MUST NOT** 参与授权判定。
- 采样位为「不采样」不影响 trace-id 的建立与传播。
- 非诊断路由经过传播中间件后，响应形状、状态码与错误语义**不变**，也不因此产生技术事件。

## Compatibility

- 旧客户端不发 `traceparent`：走「缺失」一行，行为与今天相同。
- 旧服务端（无本功能）收到 `traceparent`：忽略，行为与今天相同。
- 两个方向都不劣于现状。
