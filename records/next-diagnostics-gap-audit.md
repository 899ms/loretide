# Issue #233 — 诊断软门实现缺口审计

- Issue: https://github.com/899ms/loretide/issues/233
- 基线：`app-main` `01618357fb665fbf99412b696248b85bd846a229`
- 审计分支：`codex/diagnostics-gap-audit`
- 范围：仅核验 DIAG-06、DIAG-07、DIAG-09 与诊断接入合同；本文件不修改生产代码、测试、CI、数据库或环境脚本。

## 方法与边界

任务台账的未勾选项不是“尚未实现”的证据。此审计以 `F:/GJ/内容创作工作台/docs/13-完整开发诊断与操作日志需求.md`、`tasks/diagnostics.md`、现有源码与 CI 合同为准，并逐项区分：

1. 已有实现与已有非 UI 自动化证据；
2. 代码存在但只能由用户完成的浏览器验收；
3. 可以在当前代码中复现的实现缺口。

没有把 #1 的 Windows 独立环境工作或 #2 的浏览器验收/证据补齐重新立卡。没有把旧 TODO、未运行的浏览器矩阵、真实执行器保持禁用，或历史文档的“待验收”表述当成功能缺失。

## 证据矩阵

| 需求 | 当前实现和静态/非 UI 证据 | 结论 |
| --- | --- | --- |
| DIAG-06：结构化错误、错误边界、下一动作和权限 | `server/internal/handler/content_diagnostics.go` 的 `diagnosticError` 为常规诊断失败输出 `code`、`trace_id`、`component`、`retryable`、`next_action`；`packages/core/content/diagnostics/contract.ts` 的 `describeDiagnosticError` 校验并消费该形状；`packages/views/content/diagnostics/index.tsx` 有 `DiagnosticBoundary` 与 `recordDiagnosticClientError`。授权拒绝的接口级断言在 `content_diagnostics_authz_test.go`。 | 主路径已实现；浏览器错误边界呈现仍是用户手验。另见卡 1：两个早退分支绕过了上述合同。 |
| DIAG-07：健康、心跳、版本和有界指标 | `diagnostics.Service.Overview` 汇总 API/数据库及 web/files/daemon/executor/search；无心跳是 `unverified`，过期是 `unavailable`，超前心跳是 `unknown`。`Metrics` 公开队列等待、耗时、错误、重试、取消、丢弃和 sink 错误；`Store.PruneTechnical` 按容量与保留期裁剪，`Technical` 失败只计数。已有相关非 UI 测试包括 `overview_metrics_test.go`、`store_diskfull_test.go`、`log_regression_test.go`。 | 有界存储、失败可见且普通技术日志不拖垮调用的实现已存在；不立卡重做。D13-V01 明列的“未配置”没有数据状态或视图分支，见卡 2。 |
| DIAG-09：面板、日志流、追踪与关联跳转 | `apps/web/app/[workspaceSlug]/(dashboard)/diagnostics/page.tsx` 挂载诊断页；views 页面拥有概览、时间线、筛选、实时流、trace 瀑布、运行、复现、回归与导出标签。`stream-state.ts`、`trace-waterfall.ts`、`linkage.ts` 将可测派生逻辑下沉；`linkage.ts` 对空/非法 trace 明确拒绝跳到无筛选日志。 | 功能代码已存在。面板交互、瀑布可读性、暂停/断线/分页及“从操作定位失败步骤”仍是用户浏览器验收，不得用 UI 测试或本审计冒充通过；不立实现卡。 |
| 诊断接入软门本身 | `docs/development/diagnostics-onboarding-contract.md` 明确 E1/E2/E3 只是接入痕迹，不等于语义、覆盖或真实执行器通过；`.github/workflows/loretide-content.yml` 运行 `pnpm check:diagnostics-contract`，且明确排除 UI 单测。 | 当前静态门保持有效，但不能代替本表中的行为判定或用户验收。 |

## 可实施卡（2 张；均为真实实现缺口）

### D-GAP-01 · 诊断端点的早退响应也必须遵守结构化错误合同

- 规模：S；依赖：无。
- 可复现路径：`diagnosticScope()` 在 `h.ContentDiagnostics == nil` 时调用通用 `writeError(w, 503, "diagnostics unavailable")`；`ContentDiagnosticStream()` 在 `http.Flusher` 不可用时同样调用 `writeError(w, 503, "stream unavailable")`。`writeError` 只返回 `{ "error": "..." }`，绕开 `diagnosticError()` 的稳定 code、trace、组件、可重试标记和下一动作。
- 预期行为：这两个诊断端点的失败应返回与其他诊断失败一致的、已脱敏的结构化对象；至少可被 `describeDiagnosticError()` 读取为稳定 code 和下一动作，并保留/生成允许的 trace 标识。不得在错误体暴露 workspace、账号、请求正文、凭据或内部错误。
- 文件边界：`server/internal/handler/content_diagnostics.go`、该文件的非 UI handler 测试；如现有 core 合同需锁定新响应形状，仅限 `packages/core/content/diagnostics/contract.ts` 及其 node 环境契约测试。不得改环境、迁移、真实执行器或 views 组件。
- 非 UI 验证：为 service-missing 和 non-flushing stream 两条路径分别断言 HTTP 状态与完整结构化字段；运行对应 handler/core 非 UI 测试和 `pnpm check:diagnostics-contract`。若测试需要数据库，只能使用 CI 的隔离测试库，不使用本机业务数据库。
- 人工 Todo：用户在独立实例中停用诊断服务或触发流不可用时，确认错误显示稳定 code/下一动作且不显示敏感细节；此项不能由 UI 单测替代。
- 风险/回滚：对拒绝路径保持既有 404/403 非披露语义；仅 revert 该卡提交即可恢复旧响应。

### D-GAP-02 · 将“未配置”从“未验证”中拆出，完成 D13-V01 状态语义

- 规模：M；依赖：D-GAP-01 无关，可独立实施。
- 可复现路径：`diagnostics.Service.Overview()` 对 web/files/daemon/executor/search 的无心跳一律返回 `unverified`；仓库内没有 `unconfigured` 状态。`overviewSchema` 接受任意字符串，views 只直接展示 `c.status`，因此不存在表示“组件尚未配置”的数据契约或呈现分支。
- 预期行为：概览能稳定区分至少 `healthy`、`unavailable`、`unconfigured`、`unknown`、`unverified`；未配置的组件不能被当作“已配置但未验证”，已配置但尚未收到真实心跳也不能被当作未配置。模拟运行与真实组件状态仍分开，不能因为模拟成功显示真实组件健康。
- 文件边界：`server/internal/content/diagnostics/service.go`（明确组件配置/状态来源）、其非 UI 服务/集成测试、`packages/core/content/diagnostics/contract.ts` 及 node 环境契约测试；如需新增状态说明文案，仅限现有 diagnostics locale 与 `packages/views/content/diagnostics/index.tsx` 的已有 Settings 组合。不得新增控件、改设计 token、改真实执行器、数据库迁移或环境启动脚本。
- 非 UI 验证：覆盖配置缺失、已配置无心跳、过期心跳、未来心跳和健康心跳的互斥状态；断言 schema 兼容旧后端；运行相关 Go/core 非 UI 检查与 CI 的隔离数据库套件。不得运行 UI 单测。
- 人工 Todo：在独立实例中逐一确认五种状态、最后心跳与原因文字可辨，且未连接 Codex/远程组件不显示成功；用户自行完成浏览器验收。
- 风险/回滚：状态从自由字符串收紧前先保持向后兼容；若呈现或旧后端兼容异常，revert 本卡而不改写历史诊断记录。

## 不立卡的事项

- `specs/002-diag-package-stream-recovery/manual-ui-todo.md` 的 37 项和 `specs/006-diag-trace-waterfall-regression/manual-ui-todo.md` 的 20 项是明确标记为待用户执行的浏览器验收，不是生产缺功能，也不属于 #2 的替代实现。
- 真实 Codex/远程执行器依旧禁用；“未执行”必须保持未执行，不能通过模拟或静态扫描改写。
- `docs/development/diagnostics-acceptance-mapping.md` 已登记的 DIAG-03“按严重级别过滤写入”是另一张卡，且不在本 Issue 的 DIAG-06/07/09 范围，故不重复认领。
- overview 指标从最近 100 条技术事件计算是明确的有界读取；需求没有要求无界历史聚合，不将其推断为缺陷。

## 本轮实际核查

仅运行了不连接数据库、不启动服务、不触碰 UI 的静态检查：

| 命令 | 结果 |
| --- | --- |
| `node --test scripts/check-diagnostics-contract.test.mjs` | 26 passed / 0 failed / 0 skipped |
| `node scripts/check-diagnostics-contract.mjs` | passed；checked 9 landed modules，skipped 4 not-yet-landed modules |

未运行：本机数据库、迁移、服务、真实执行器、浏览器验收或任何 UI 单测。CI 的隔离数据库结果仍须按后续实施 PR 的 head SHA 单独记录。

## 回滚

本 PR 只新增本记录；回滚为 `git revert <本审计提交>`。两张后续卡各自独立提交并可独立 revert。
