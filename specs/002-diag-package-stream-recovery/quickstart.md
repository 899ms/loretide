# Quickstart: 002 验证指引

前置：Windows 实例已启动（`./scripts/local-windows.ps1 start`）；已登录为 workspace owner；`pnpm install` 完成。

## 自动检查（可执行部分）

```bash
pnpm typecheck
pnpm --filter @multica/core test -- content/diagnostics
(cd server && go test ./internal/handler -run 'ContentDiagnostic' -count=1)
pnpm check:content-boundaries      # views 未引入平台 API
```

预期：全部通过；新增用例名可映射到 FR-001～FR-008。

## 手动验证（用户执行，见 manual-ui-todo.md）

1. **轮换不可见（SC-001）**：打开 `/loretide-dev-check/diagnostics` 实时流，观察 3 分钟。预期无断线提示。网络面板可见每 ~25 秒一次新的 `stream?after=...` 请求，且最后一行含 `"rotate":true`。
2. **过滤透传（SC-002）**：设置 `severity=error`，网络面板核对重连请求的查询串含 `severity=error&after=N`。
3. **真实断线**：`./scripts/local-windows.ps1 stop` 后 ≤10 秒出现断线提示；`start` 后 ≤5 秒清除；过滤仍为 error。
4. **下载保真（SC-003）**：点击下载；用 `curl -X POST -H "X-Workspace-ID: <ws>" -H "Authorization: Bearer <token>" "http://127.0.0.1:18000/api/content-diagnostics/export?run_id=<id>" -o expected.json`；`cmp expected.json <下载文件>` 无差异；文件名与响应头一致。
5. **非授权下载（SC-004）**：修改查询串加 `account_id=x`，界面显示拒绝与 `next_action`，无文件生成。
6. **缺口保留**：清理技术日志后续读，缺口提示出现；等待若干正常批次，提示仍在。
7. **200 条标示**：触发 >200 条事件后，界面显示「仅显示最近 200 条」。

## 未执行项的记录方式

任何未执行的手动项在交付记录中标「按策略未执行，等待用户验证」，不标通过。
