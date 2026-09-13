# Contract: /api/content-diagnostics/export

## GET（预览）

Query：`run_id`（必需）、`account_id`（可选；当前 `Scope.Accounts` 为空，任何非空值 → 403）。
响应 `200` JSON Export：`{manifest[], run, audit, technical, redacted, limits[]}`。客户端经 `exportSchema` 转换后用于预览。

## POST（下载）

同样的 query；body 为空对象或空。
响应：

- `200`
- `Content-Type: application/json`
- `Content-Disposition: attachment; filename="loretide-diagnostics.json"`（文件名可含 run_id，由服务端决定）
- body：与 GET 相同形状的 JSON 字节。

**客户端义务**：以原始字节保存（`Response.blob()`），文件名取自 `Content-Disposition`，解析失败回退 `loretide-diagnostics.json`；MUST NOT 解析为对象后重新序列化。

## Errors（GET / POST 同）

`403` / `404` / `409` / `503`，body 为诊断错误对象：
`{error, code, trace_id, component, retryable, next_action}`。客户端显示 `next_action`，不生成文件。

## Guarantees

- 导出内容已脱敏（`redacted: true`）；被裁剪项列在 `limits`。
- 不自动上传到任何外部地址。
