# Issue #251：诊断概览组件原因码显示为本地化说明

## 任务与边界

- 仓库：`899ms/loretide`；Issue：`#251`
- 工作分支：`claude/diagnostics-reason-text`，起点 `efa964f`（origin/app-main）
- 只改页面显示与四种语言文案；未改服务端、样式、`scripts/content-boundaries.json`、其他页面；未写 UI 单测，未用浏览器或 computer use，未连接数据库。

## 改动

- `packages/views/content/diagnostics/index.tsx`：新增固定原因码列表 `componentReasonCodes` 与判断函数；组件行说明改为：
  - 已知码 → `t($.diagnostics.reasons[code])`
  - 未知码 → 原样显示
  - 空 reason 且 `status === "healthy"` → `t($.diagnostics.reasonHealthy)`（「当前连接可用」）
  - 空 reason 且非 healthy → 不显示
- `packages/views/locales/{zh-Hans,en,ja,ko}/common.json`：diagnostics 段新增 `reasonHealthy` 与 `reasons.*`（10 个码），四种语言键集相同。

## 原因码来源

`server/internal/content/diagnostics/component_facts.go`、`service.go` 中会进入组件 `reason` 的固定码：
config_unknown、liveness_unverified、config_unconfigured、storage_not_configured、heartbeat_without_configuration、heartbeat_expired、clock_skew、execution_disabled、database_connectivity_unavailable、heartbeat_missing_timestamp。
`storage_s3`、`storage_local`、`server_boot` 只出现在已配置时的 `config_reason`，不会进入 `reason`；心跳上报的自带原因（`safeToken`）不是固定码，按未知码原样显示。

## 验证（worktree 根目录）

- `pnpm typecheck --force`：9 successful，0 cached
- `pnpm check:content-boundaries`：pass 13 / fail 0
- `pnpm check:diagnostics-contract`：pass 26 / fail 0
- `pnpm check:diagnostics-no-upload`：pass 7 / fail 0
- `pnpm --filter @multica/views exec vitest run content/diagnostics locales rich-content/package-exports.test.ts`：3 文件 174 测试通过（locales/parity、locales/mcp、rich-content/package-exports；views 下 content/diagnostics 无测试文件）

## 待用户手验

见 PR 的「手验 Todo」。
