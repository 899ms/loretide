# Data Model: 002 诊断包下载保真与实时流断线恢复

无数据库变更。以下为线上契约与客户端状态的形状变化。

## Page（线上，NDJSON 每行 / events 响应）

| 字段 | 类型 | 变化 |
|---|---|---|
| events | Event[] | 不变 |
| cursor | number ≥ 0 | 不变 |
| gap | boolean | 不变 |
| has_more | boolean | 不变 |
| **rotate** | boolean，可选，默认 false | **新增**：仅计划轮换的最后一页为 true |

客户端 `pageSchema`：`rotate: z.boolean().optional().default(false)`；transform 后字段名 `rotate`。

## Export（线上，POST /export 响应）

形状不变。变化在客户端处理：下载路径不再解析为对象，保存原始字节。

## Stream state（客户端，hook 内部）

```text
status:   connecting | connected | reconnecting | disconnected | paused | denied
gap:      boolean（粘性，wsId 变化或用户清除才复位）
notice:   string
filter:   DiagnosticFilter（kind, traceId, runId, component, errorCode, severity, from, until）
cursor:   number
backoffMs: number（0 | 2000 | 4000 | … | 30000）
```

转移（摘要，完整见 research.md D2）：

| 当前 | 事件 | 下一状态 | 副作用 |
|---|---|---|---|
| connected | page(rotate=false) | connected | 写入事件；backoff=0；清 notice |
| connected | page(rotate=true) | connected | 写入事件；立即重连（延迟 0） |
| connected | end / error(非 403/404) | disconnected | notice=断线；backoff=min(2×, 30s)；定时重连 |
| disconnected | open | connected | 清 notice |
| any | error(403/404) | denied | notice=授权失败 + next_action；停止重连 |
| any | pause | paused | 中止连接；保留 cursor |
| paused | resume | connecting | 从 cursor 续读 |
| any | wsId 变化 | connecting | cursor=0；gap=false；filter 保留 |
| any | filter 变化 | connecting | cursor=0 |

## DownloadResult（客户端）

```text
{ blob: Blob, filename: string }
```

`filename` 来自 `Content-Disposition: attachment; filename="..."`，解析失败回退 `loretide-diagnostics.json`。
