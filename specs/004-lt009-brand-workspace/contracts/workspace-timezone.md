# Contract: 工作区时区（`settings["loretide.timezone"]`）

## POST /api/workspaces（创建）

Body 新增可选 `settings`：

```json
{ "name": "品牌 A", "slug": "brand-a", "settings": { "loretide.timezone": "Asia/Shanghai" } }
```

- `settings.loretide.timezone` 合法 IANA 名 → 201，响应 `settings` 含该键。
- 非法（如 `"Mars/Olympus"`）→ 400 `{ "error": "invalid timezone" }`，不创建。
- 缺失 → 201，响应 `settings.loretide.timezone = "Asia/Shanghai"`。

## PATCH /api/workspaces/{id}（更新）

Body `settings` 已存在。含 `loretide.timezone` 时同上校验；不含时该键不变；其余键原样透传。

## GET /api/workspaces/{id} 与 GET /api/workspaces（读取）

响应 `settings.loretide.timezone` 始终存在；数据库缺该键时响应补 `"Asia/Shanghai"`（不回写）。

## 授权

以上接口沿用既有成员校验：非成员 → 403/404，响应 body 不含 `name` / `settings` / 任何对象字段。

## 客户端

- 读取：`getWorkspaceTimezone(ws)`，缺失 / 非法 → `"Asia/Shanghai"`。
- 写入：`useUpdateWorkspace().mutate({ id, data: { settings: { "loretide.timezone": tz } } })`；**服务端 `UpdateWorkspace` 对 `settings` 是整体替换**（`handler/workspace.go:403-405` 直接 marshal 请求中的 `settings` 写入），因此客户端 MUST 先读取当前 `settings`、用 `withWorkspaceTimezone(settings, tz)` 合并后整体发送；只发单键会清空其他设置。

## 兼容

- 旧客户端：忽略该键，不受影响。
- 新客户端对旧服务端：响应缺该键 → 读取器兜底默认。
