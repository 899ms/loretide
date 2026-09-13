# Contract: GET /api/content-diagnostics/stream

NDJSON 实时流。每行一个 Page JSON。

## Request

Query：`after`（必需，上次游标，首次 0）+ 与 `/events` 相同的过滤键：`kind, trace_id, run_id, component, error_code, severity, from, until`（RFC3339）。Header：`X-Workspace-ID`；须为 owner/admin；机器凭据拒绝。

## Response

- `200`，`Content-Type: application/x-ndjson`，`Cache-Control: no-store`。
- 每秒最多一行；每行为 Page：`{events, cursor, gap, has_more, rotate?}`。
- 服务端在约 25 秒后结束连接；**结束前最后一行 `rotate: true`**。这是计划轮换，不是错误。
- 成员资格每批重查；撤销后连接直接结束（无 rotate 行）。

## Client obligations

- 收到 `rotate: true`：保持已连接状态，不提示，立即以 `after=<该行 cursor>` 与相同过滤键重连。
- 连接结束且最后一行无 `rotate`，或连接异常：视为真实断线，退避重连（2s→30s），显示断线提示；首个成功行清除提示。
- 连接前的 HTTP 403/404：停止重连，显示 `next_action`。
- `gap: true`：显示缺口提示并保留。

## Compatibility

- 旧客户端忽略 `rotate`，行为与现状相同。
- 旧服务端不发 `rotate`，新客户端按真实断线处理，行为与现状相同。
