# Manual UI Todo: 002 诊断包下载保真与实时流断线恢复

**状态**：全部条目初始为「待用户验证」。按 `.specify/memory/constitution.md` 原则 II，UI 验收不写单测、不做自动点击；未执行的条目记「按策略未执行，等待用户验证」，不记通过。

**页面**：Web `/{workspaceSlug}/diagnostics`（例：`/loretide-dev-check/diagnostics`）。需以 workspace owner / admin 身份登录。桌面端未挂载诊断页，本功能不含桌面端（spec FR-012）。

**前置**：本地实例已启动（`./scripts/local-windows.ps1 start`）；浏览器开发者工具的 Network 面板可用。

## 验收来源说明（需主任务确认）

规格要求「逐句列出 D13-V05、V08、V11」。本分支 `chore/spec-kit-baseline` 中不存在 `docs/13` 与 `tasks/diagnostics.md`，因此无法引用这三项的**原文逐句**。下表按 `spec.md` 对三项的引述拆分：

- **D13-V05**：「断线恢复…可验证」
- **D13-V08**：「诊断包可预览导出内容，凭据 / 私有正文不泄漏」
- **D13-V11**：「日志 sink 失败 / 存储满 / 心跳过期可见且有界」

主任务补齐原文后，请核对下表是否覆盖每一句；如有缺口，条目在此追加。

## D13-V05 断线恢复可验证

| # | 页面 / 位置 | 操作 | 预期 | 对应 | 用户确认 |
|---|---|---|---|---|---|
| V05-1 | 诊断页 → 技术日志 | 点「连接 / 继续实时流」，连续观察 3 分钟（≥6 次服务端 25 秒轮换） | 状态行始终显示 `connected`；全程无任何断线提示 Alert | FR-001 / SC-001 | 待用户验证 |
| V05-2 | 同上 + Network 面板 | 观察 3 分钟内的 `stream?...` 请求 | 每约 25 秒出现一条新 `stream` 请求；上一条的最后一行 NDJSON 含 `"rotate":true` | FR-001 | 待用户验证 |
| V05-3 | 技术日志筛选区 | 设 `severity=error`，保持实时流连接 | 新到事件均为 error；Network 面板中每次续连的查询串都含 `severity=error` 且 `after=<上次游标>` | FR-002 / SC-002 | 待用户验证 |
| V05-4 | 同上 | `./scripts/local-windows.ps1 stop` 停止 API | ≤10 秒内出现断线提示「实时流已断开，正在从上次游标重新连接。」或「实时流连接失败…」 | FR-003 / SC-001 | 待用户验证 |
| V05-5 | 同上 + Network 面板 | 保持 API 停止，观察重连请求的时间间隔 | 间隔按 2s → 4s → 8s → 16s → 30s 递增，之后稳定在 30s，不再每 2 秒重试 | FR-003 | 待用户验证 |
| V05-6 | 同上 | `start` 恢复 API | ≤5 秒清除断线提示；状态回到 `connected`；筛选仍为 `severity=error`；事件从停机前游标续读，无重复、无跳号 | FR-002 / FR-003 / SC-001 | 待用户验证 |
| V05-7 | 同上 | 点「暂停实时流」，等待若干秒后再点「连接 / 继续实时流」 | 状态变 `paused` 后不再发起请求；继续后从暂停时游标续读，期间事件（保留期内）不丢失 | spec US1 §4 | 待用户验证 |
| V05-8 | 同上 | 流连接期间，在另一浏览器/账号把当前用户的 workspace 角色从 owner/admin 降为 member | 流关闭并显示授权失败提示，提示中含 `next_action`（如 `check_authorization`）；**不再自动重连**（Network 面板无新 `stream` 请求） | FR-004 / spec US1 §5 | 待用户验证 |
| V05-9 | 同上 | 流连接中切到其他标签页 ≥1 分钟（触发浏览器定时器节流）后切回 | 回到前台后立即续连一次，不需等待完整退避；状态恢复 `connected` | spec Edge Cases | 待用户验证 |
| V05-10 | 同上 | 流连接中切换 workspace | 事件列表清空、游标归零、缺口提示复位；不残留上一个 workspace 的事件 | FR-007 | 待用户验证 |

## D13-V08 诊断包可预览导出内容，凭据 / 私有正文不泄漏

| # | 页面 / 位置 | 操作 | 预期 | 对应 | 用户确认 |
|---|---|---|---|---|---|
| V08-1 | 诊断页 → 故障复现 | 选场景 `normal`，运行一次模拟，记下 `run_id` | 生成运行记录，跳到「运行详情」 | 前置 | 待用户验证 |
| V08-2 | 诊断页 → 诊断导出 | 选中该 run，点「预览导出内容」 | 显示 manifest 列表与 `limits` 裁剪说明 | FR-005（预览路径） | 待用户验证 |
| V08-3 | 同上 | 点「下载脱敏诊断包」 | 浏览器保存一个文件；文件名与响应头 `Content-Disposition: attachment; filename="..."` 一致（Network 面板核对） | FR-005 / SC-003 | 待用户验证 |
| V08-4 | 文本编辑器 | 打开下载的文件 | 键名为**线上格式**：`run_id` / `workspace_id` / `has_more`（**不是** `runId` / `workspaceId` / `hasMore`）；`"redacted": true`；`limits` 列出被裁剪项 | FR-005 / SC-003 | 待用户验证 |
| V08-5 | 终端 | `curl -X POST -H "X-Workspace-ID: <ws>" -H "Authorization: Bearer <token>" "http://127.0.0.1:18000/api/content-diagnostics/export?run_id=<id>" -o expected.json` 然后 `cmp expected.json <下载文件>` | `cmp` 无输出（逐字节一致） | FR-005 / SC-003 | 待用户验证 |
| V08-6 | 同上 | 检查下载文件内容 | 无源文本、无凭据 / token、无模型 provider stdout、无媒体副本；敏感字段为脱敏值 | D13-V08 | 待用户验证 |
| V08-7 | 诊断页 → 诊断导出 | 通过修改请求查询串加 `account_id=x`（或用他人 workspace 的 `run_id`）触发下载 | 返回 403 / 404 的诊断错误对象；界面显示「导出失败，未生成文件：<CODE> · <next_action>」；**浏览器未生成任何文件** | FR-006 / SC-004 | 待用户验证 |
| V08-8 | Network 面板（全程） | 在预览与下载全过程中观察外发请求 | 除本实例 API 外无任何外部地址请求；诊断包不自动上传 | FR-011 | 待用户验证 |
| V08-9 | 诊断页 | 下载过程中让会话过期（清除 token）后再点下载 | 返回授权错误对象；不生成半个文件 | spec Edge Cases | 待用户验证 |

## D13-V11 日志 sink 失败 / 存储满 / 心跳过期可见且有界

| # | 页面 / 位置 | 操作 | 预期 | 对应 | 用户确认 |
|---|---|---|---|---|---|
| V11-1 | 诊断页 → 概览 | 在 `sink_errors > 0` 的状态下查看指标区 | `sink_errors` 指标可见并显示真实数值；组件状态不显示为全部健康 | spec US3 §3 | 待用户验证 |
| V11-2 | 诊断页 → 概览 | 在 `dropped > 0`（缓冲区溢出丢弃）的状态下查看指标区 | `dropped` 指标可见并显示真实数值 | spec US3 §3 | 待用户验证 |
| V11-3 | 诊断页 → 概览 | 查看 `retention_days` 与 `capacity` | 如实显示；保留期为 0 或容量为 0 的异常配置照实展示，不伪装为正常 | spec Edge Cases | 待用户验证 |
| V11-4 | 诊断页 → 概览 | 停止 web 心跳（关闭诊断页 ≥30 秒后由另一入口查看概览） | 心跳过期的组件状态不显示为 `healthy` | D13-V11 | 待用户验证 |
| V11-5 | 诊断页 → 技术日志（实时流） | 清理技术日志（或等待保留期裁剪）后从旧游标续读，使服务端返回 `gap=true` | 状态行出现「· 检测到保留期缺口」与缺口提示 | FR-007 | 待用户验证 |
| V11-6 | 同上 | 缺口出现后继续接收若干正常批次 | 缺口提示**仍然保留**，不因后续正常批次自动消失 | FR-007 | 待用户验证 |
| V11-7 | 同上 | 点「清除缺口提示」 | 缺口标示与提示消失；后续再出现缺口时重新出现 | FR-007 | 待用户验证 |
| V11-8 | 同上 | 触发 >200 条实时事件 | 显示「仅显示最近 200 条」；列表条数有界，不无限增长 | FR-008 | 待用户验证 |
| V11-9 | 同上 | 构造同一批内含重复 `event_id` 与乱序 `sequence` 的事件 | 每个 `event_id` 只出现一次；显示顺序按 `sequence` 单调递增 | FR-008 | 待用户验证 |

## 已自动化的部分（无需手动重复）

以下已由自动测试固定，手动验收不必重复，仅在上表相关条目失败时作为定位参考：

| 层 | 文件 | 覆盖 |
|---|---|---|
| Go handler | `server/internal/handler/content_diagnostics_test.go` | FR-001 计划轮换收尾页 `rotate=true` / 撤销成员资格时无 rotate 结束；FR-002 过滤键透传到 `Store.Query`；FR-005 下载响应带 `Content-Disposition`；FR-006 非授权 `account_id` 返回诊断错误对象 |
| core 纯函数 | `packages/core/content/diagnostics/stream-state.ts` 的 `stream-state.test.ts` | FR-001 rotate 静默续连；FR-002 workspace / filter 变化归零游标；FR-003 退避 2/4/8/16/30/30 与成功归零；FR-004 403/404 → denied 且停止重连；FR-007 缺口粘性 |
| core 契约 | `packages/core/content/diagnostics/contract.test.ts` | FR-001 `rotate` 缺省 false 与畸形响应兜底；FR-002 `streamQuery` 序列化；FR-005 `Content-Disposition` 文件名解析；FR-008 `mergeEvents` 去重 / 排序 / 200 条上限 |
| web 平台接线 | `apps/web/platform/content-diagnostics.test.ts` | FR-005 保存的是服务端原始字节与服务端文件名 |

## 按策略未执行

以下项按 constitution 原则 II 不做自动化，本次交付**未执行**，等待用户验证：上表 V05-1 ～ V11-9 全部条目。
