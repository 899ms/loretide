# Quickstart: 005 验证指引

前置：应用实例已启动（原生 Windows 用 `./scripts/local-windows.ps1 start`）；已登录为 workspace owner；`pnpm install` 完成。Go 工具链用 `GOTOOLCHAIN=auto`（`go.mod` 要求 1.26）。

## 自动检查（可执行部分）

```bash
# 传播中间件：纯 httptest，不需要数据库
(cd server && GOTOOLCHAIN=auto go test ./internal/middleware -run Trace -count=1 -v)

# 脱敏规则与派发接口：不需要数据库
(cd server && GOTOOLCHAIN=auto go test ./internal/content/diagnostics -run 'Sanitize|LogRegression|Dispatch' -count=1 -v)

# handler 三段连续性：需要 PostgreSQL
(cd server && GOTOOLCHAIN=auto go test ./internal/handler -run ContentDiagnostic -count=1 -v)

# core 契约
pnpm --filter @multica/core exec vitest run content/diagnostics/contract

pnpm typecheck
pnpm check:content-boundaries       # 无此脚本时：node --test scripts/check-content-boundaries.test.mjs && node scripts/check-content-boundaries.mjs
```

> ⚠️ `server/internal/handler/TestMain` 在无数据库时 `os.Exit(0)`，`go test` 会打印 `ok` 却**一个用例都没跑**。handler 那一行必须带 `-v` 看真实用例，或先起本地 PostgreSQL 并跑完迁移（`go run ./cmd/migrate up`）。这一点在 002 交付时踩过，写在这里以免重复。

预期：全部通过；新增用例名可映射到 FR-001～FR-013。

## 手动核对（开发者执行，非 UI 验收）

1. **三段连续（SC-001/002）**：打开诊断页触发一次模拟运行；从技术日志取该运行的 `trace_id`，确认 HTTP 入口事件、队列消息与 daemon 回写三处相同。
2. **不采信用户 traceparent（SC-003）**：
   ```bash
   curl -i -H "X-Workspace-ID: <ws>" -H "Authorization: Bearer <token>" \
        -H "traceparent: 00-11111111111111111111111111111111-2222222222222222-01" \
        "http://127.0.0.1:18000/api/content-diagnostics/overview"
   ```
   响应的 `X-Diagnostic-Trace` **不等于** `1111…`；该请求成功（与不带该头时一致）。
3. **非法值不失败**：把上面的 `traceparent` 换成 `garbage`，响应仍为 200，且 `upstream_trace` 不留存该值。
4. **记录范围不变**：打一个非诊断业务路由（例如 `GET /api/workspaces`），确认响应**带** `X-Diagnostic-Trace`，但 `content_technical_log` **没有**新增行。
5. **脱敏负例（SC-004）**：带 `Authorization`、`X-Api-Key`、`User-Agent`、路径参数与 `?token=abc` 请求诊断端点，导出诊断包后 `grep` 包内容：`abc`、令牌串、查询串**键名**、`User-Agent` 取值出现次数均为 0；请求身份只出现为 `<METHOD> <路由模板>` 加状态码。
6. **派发边界（SC-005）**：构造事务回滚场景确认派发 0 次；构造提交后派发失败确认失败计数上升且业务操作成功。

## 未执行项的记录方式

任何未执行的项在交付记录中标「按策略未执行，等待用户验证」，不标通过。本功能预计无页面改动；若实施中改到页面，按 constitution 原则 II 转入手动 UI 清单，不写 UI 单测。
