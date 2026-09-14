# Feature Specification: 诊断包下载保真与实时流断线恢复

**Feature Branch**: `002-diag-package-stream-recovery`

**Created**: 2026-09-14

**Status**: Draft

**Input**: User description: "DIAG-12/08：诊断包实际下载、实时流断线恢复及关联测试修复。以已有诊断实现为基础闭合 D13-V05 / V08 / V11 的验收缺口。"

**Traces to**: `tasks/diagnostics.md` DIAG-08、DIAG-12；`docs/13` D13-V05、D13-V08、D13-V11；`records/2026-09-13-开发子任务模型选择官方核对.md` 所定「下一项」。

## Current State（以代码为准）

`tasks/diagnostics.md` 头部已说明「已有诊断实现；未勾选表示尚未逐项验收，不能理解为没有代码」。核实结果：

- **接口**：`/api/content-diagnostics/{overview,runs,events,simulate,export,client,stream}`。授权在 `diagnosticScope`：机器凭据拒绝、须为 workspace 的 owner/admin、`Scope.Accounts` 目前为空集合（真实账号权限等账号域交付后接入），因此任何非空 `account_id` 都被拒绝。
- **实时流**：服务端 NDJSON，每秒查询一页并 flush，**25 秒后主动结束**（`deadline`），每批重查成员资格，撤销即关闭。客户端 `useDiagnosticStream` 在流结束或异常后固定 2 秒重连，带上 `after=<上次游标>`；用 `mergeEvents` 去重、按 `sequence` 排序、保留最近 200 条；收到 `gap=true` 显示保留期缺口提示。
- **下载**：GET `export` 为预览，POST 为下载，服务端设置 `Content-Disposition` 但仍以 JSON 响应。Web 端 `apps/web/platform/content-diagnostics.ts` 的 `downloadDiagnosticBundle` 用 Blob + 隐藏链接触发浏览器保存——**下载功能存在**。
- **测试**：前端诊断测试无跳过；Go `store_integration_test.go` 在无独立 PostgreSQL 夹具时 `t.Skip`（环境门控，非缺陷）。
- **保留与容量**：`overview` 返回 `retention_days` / `capacity`；`Store.PruneTechnical` 存在；指标含 `dropped` / `sink_errors`。

**核实出的缺口**：

1. 服务端每 25 秒的**计划轮换**在客户端表现为「实时流已断开，正在从上次游标重新连接」——用户每 25 秒看到一次断线提示，无法区分真实故障。
2. 重连只带 `after=`，**不带原有过滤条件**（kind / run_id / component / error_code / severity / from / until）；`useDiagnosticStream` 的签名也不接收过滤参数。
3. 下载内容是前端把 `exportSchema` **转换后的对象**（键已改为 camelCase）重新 `JSON.stringify`，不是服务端发出的字节；诊断包给开发者定位问题用，键名与线上格式不一致是保真缺陷。服务端的 `Content-Disposition` 因 `fetch` 走 JSON 解析而被忽略。
4. 桌面端已核实：`apps/desktop/src` 未挂载诊断页（无 `DiagnosticsPage` / `content/diagnostics` 引用），因此不存在桌面端下载路径；本功能只做 Web。
5. 重连为固定 2 秒无退避；服务端持续不可用时每 2 秒打一次。
6. D13-V05 / V08 / V11 的浏览器验收矩阵尚未逐项核对（这是「暂不标记总体验收完成」的原因）。

本功能只闭合以上六项，不重写诊断存储、模拟器或面板布局。

## Clarifications

### Session 2026-09-14

- Q: 服务端每 25 秒的计划轮换如何对用户不可见？ → A: 客户端识别轮换：服务端在计划结束的最后一批页携带正常收尾标记，客户端据此静默续连；成员资格重查节奏与撤销生效延迟不变。

## User Scenarios & Testing *(mandatory)*

### User Story 1 - 计划轮换对用户不可见，真实断线才提示，续读保持过滤条件 (Priority: P1)

用户打开实时流并设置过滤条件后持续观察，服务端的周期性轮换不产生任何断线提示；只有服务端真正不可达时才显示断线与重连状态；重连后过滤条件不变、事件从上次游标续读、不重复不丢失。

**Why this priority**: D13-V05 要求「断线恢复…可验证」。当前每 25 秒一次的假断线提示让这一项无法验收，也让真实故障被淹没。

**Independent Test**: 打开流并过滤 `severity=error`，观察 60 秒无断线提示且只出现 error 事件；停止 API 进程后 10 秒内出现断线提示；恢复 API 后提示消失、过滤仍为 error、游标从停机前位置继续。

**Acceptance Scenarios**:

1. **Given** 流已连接，**When** 服务端按计划结束本轮，**Then** 客户端静默续连，状态保持「已连接」，不显示任何提示。
2. **Given** 流已连接且设有过滤条件，**When** 发生续连或真实重连，**Then** 请求携带相同过滤条件与上次游标；新事件仍满足过滤。
3. **Given** API 不可达，**When** 连接失败，**Then** 显示断线提示，重连间隔按退避递增至上限；API 恢复后首个成功批次清除提示。
4. **Given** 用户点击暂停，**When** 再点继续，**Then** 从暂停时游标续读，中间事件不丢失（保留期内）。
5. **Given** 成员资格在流打开期间被撤销，**When** 服务端下一批重查，**Then** 流关闭且客户端显示授权失败，不再自动重连。

---

### User Story 2 - 下载的诊断包与服务端导出内容逐字一致 (Priority: P1)

用户点击下载，浏览器保存一个文件；文件内容就是服务端导出接口返回的字节（线上键名、字段顺序、脱敏结果），与预览显示的内容对应；跨账号的 `run_id` 被拒绝且不产生文件。

**Why this priority**: D13-V08「诊断包可预览导出内容，凭据/私有正文不泄漏」。诊断包的用途是交给开发者复现问题，键名被前端改写后与服务端日志、接口文档对不上。

**Independent Test**: 下载后用文本编辑器打开，键名为 `run_id` / `workspace_id`（线上格式）而非 `runId`；`redacted` 为 true；文件与直接 POST 接口的响应体逐字节相同。

**Acceptance Scenarios**:

1. **Given** 某 run 有权限，**When** 点击下载，**Then** 浏览器保存文件，文件名来自服务端 `Content-Disposition`，内容与接口响应字节一致。
2. **Given** 请求带非授权 `account_id` 或他人 workspace 的 `run_id`，**When** 点击下载，**Then** 返回 403/404 的诊断错误对象，不生成文件，界面显示 `next_action`。
3. **Given** 导出含被脱敏字段，**When** 查看预览与下载文件，**Then** 两者敏感字段均为脱敏值，`limits` 列出被裁剪项。
4. **Given** 桌面端，**When** 查看诊断入口，**Then** 不适用——桌面端未挂载诊断页，本功能不含桌面端；桌面端诊断页与下载适配另立任务。

---

### User Story 3 - 缺口、乱序与重复对用户可见且不误导 (Priority: P2)

当游标超出保留期或存储被裁剪时，界面明确提示存在缺口而不是静默跳过；迟到、乱序、重复的事件去重后按序显示；这些行为有自动测试固定下来。

**Why this priority**: DIAG-08 验证项「游标过期及乱序」；D13-V11「日志 sink 失败 / 存储满 / 心跳过期可见且有界」。现有 `gap` 提示与 `mergeEvents` 已实现，本故事是把验收条件固定为测试，并核对提示文案与真实状态一致。

**Independent Test**: 构造 `gap=true` 页 → 提示出现且在后续无缺口页到达后仍保留（缺口是事实，不应自动消失）；构造乱序与重复事件 → 显示顺序按 `sequence`、无重复。

**Acceptance Scenarios**:

1. **Given** 上次游标早于保留期，**When** 续读，**Then** 首页 `gap=true`，界面显示缺口提示并保留，后续事件正常追加。
2. **Given** 同一批含重复 `event_id` 与乱序 `sequence`，**When** 合并显示，**Then** 每个 `event_id` 只出现一次，顺序单调。
3. **Given** `overview` 报告 `sink_errors > 0` 或 `dropped > 0`，**When** 查看概览，**Then** 对应指标可见，不显示成全部健康。

---

### User Story 4 - D13-V05 / V08 / V11 浏览器验收矩阵作为手动清单交付，关联自动测试补齐 (Priority: P2)

交付一份逐项对应 D13-V05、V08、V11 的手动验收清单，每项写明页面、操作、预期；能自动化的部分（契约、Go handler、流合并逻辑）以现有测试层补齐，不写 UI 单测。

**Why this priority**: `tasks/diagnostics.md` 明确「完整浏览器验收矩阵仍需逐项核对，暂不标记总体验收完成」；这是 DG-01 出口的前置。

**Independent Test**: 清单存在于 `specs/002-.../manual-ui-todo.md`，每项有「用户确认」栏；自动测试在 `packages/core/content/diagnostics/*.test.ts(x)` 与 `server/internal/handler/content_diagnostics_test.go` 中新增用例名可对应到本规格 FR 编号。

**Acceptance Scenarios**:

1. **Given** 本功能交付，**When** 主任务打开清单，**Then** V05 / V08 / V11 每一句验收文本各有至少一条可执行步骤，状态初始为「待用户验证」。
2. **Given** 自动测试运行，**When** `pnpm test --filter @multica/core` 与相关 Go 测试执行，**Then** 新增用例覆盖 FR-001～FR-006，无跳过（环境门控的集成测试除外，单列）。

---

### Edge Cases

- 轮换与真实断线同时发生（轮换后立刻连不上）：按真实断线处理，退避重连。
- 浏览器标签页后台化后计时器被节流：恢复前台时立即续连一次。
- 导出体积超限：服务端 `limits` 列出裁剪项；下载文件仍完整包含 `limits`。
- 下载期间会话过期：返回授权错误对象，不生成半个文件。
- 流打开时切换 workspace：游标归零、过滤重置（已有行为），本功能不改。
- 保留期为 0 或容量为 0 的异常配置：概览如实显示，不伪装为正常。

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: 服务端 MUST 在计划轮换的最后一批页中携带正常收尾标记（`rotate: true`）；客户端收到该标记 MUST 不改变显示状态、不产生提示，并立即以当前过滤条件与游标续连。未带标记的流结束或异常 MUST 按真实断线处理。
- **FR-002**: 续连与重连 MUST 携带用户当前全部过滤条件与上次游标；`useDiagnosticStream` MUST 接收过滤参数。
- **FR-003**: 真实断线的重连 MUST 采用有上限的退避（2s 起、每次翻倍、上限 30s；任一成功页归零）；首个成功批次 MUST 清除断线提示。
- **FR-004**: 授权失败（403/404）MUST 终止自动重连并显示 `next_action`。
- **FR-005**: 下载 MUST 保存服务端导出接口的原始响应字节；文件名取自服务端 `Content-Disposition`；MUST NOT 经过客户端 schema 转换后再序列化。预览路径可继续使用转换后的对象。
- **FR-006**: 下载与预览 MUST 对非授权 `account_id` / 他人 `run_id` 返回诊断错误对象且不生成文件。
- **FR-007**: 缺口提示 MUST 在出现后保留，直到用户手动清除或切换 workspace；MUST NOT 因后续正常批次而自动消失。
- **FR-008**: 事件合并 MUST 按 `event_id` 去重、按 `sequence` 排序；上限 200 条的裁剪 MUST 在界面标示「仅显示最近 200 条」。
- **FR-009**: 交付 MUST 包含 `manual-ui-todo.md`，逐项对应 D13-V05、V08、V11 文本，含页面 / 操作 / 预期 / 用户确认栏。
- **FR-010**: 自动测试 MUST 覆盖 FR-001～FR-008 中可在契约层、Go handler 层、纯函数层验证的部分；MUST NOT 新增 UI 单测；未自动化的项在清单中标「按策略未执行，等待用户验证」。
- **FR-011**: 诊断包 MUST NOT 自动上传到任何外部地址（DIAG-12 验证项）。
- **FR-012**: 本功能 MUST NOT 涉及桌面端；共享视图 `packages/views/content/diagnostics` 的下载入口 MUST 继续通过平台注入的 `download` prop 工作，不引入任何平台 API，以便桌面端日后接入。

### Key Entities

- **诊断事件（Event）**：`event_id, sequence, workspace_id, account_id, kind, component, code, severity, trace, occurred_at`；流按 `sequence` 游标分页。
- **诊断页（Page）**：`events[], cursor, gap, has_more, rotate`（`rotate` 仅在计划轮换的最后一页为 true）。
- **导出包（Export）**：`manifest[], run, audit(Page), technical(Page), redacted, limits[]`。
- **流状态（客户端）**：`connecting / connected / reconnecting / disconnected / paused / denied`，`gap`，`notice`，当前过滤条件，上次游标。

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: 流连续观察 3 分钟（≥6 次服务端轮换）无断线提示；人为停止 API 后 ≤10 秒出现提示，恢复后 ≤5 秒清除。
- **SC-002**: 重连请求的查询串包含停机前的全部过滤键与 `after=<上次游标>`（网络面板核对）。
- **SC-003**: 下载文件与 `curl -X POST .../export?run_id=X` 的响应体 `cmp` 无差异；文件名与 `Content-Disposition` 一致。
- **SC-004**: 非授权下载返回 403/404，浏览器无文件生成。
- **SC-005**: 新增自动测试全部通过，用例名可映射到 FR 编号；`pnpm typecheck` 通过；相关 Go 测试通过。
- **SC-006**: `manual-ui-todo.md` 覆盖 D13-V05 / V08 / V11 全部句子，用户逐项确认后才在记录中标为通过。

## UI Impact

有。涉及诊断页的实时流状态区（提示文案与状态机）、下载按钮行为、缺口提示保留、「仅显示最近 200 条」标示。

手动 UI Todo（由用户确认；不做 UI 单测、不做自动点击）：
1. 打开实时流，观察 3 分钟无断线提示。
2. 设置过滤 `severity=error`，停止 API，出现断线提示；恢复后提示消失，过滤仍为 error。
3. 点击下载，保存文件；用编辑器打开，键名为线上格式，`redacted: true`。
4. 用非授权 `account_id` 请求下载（可通过修改查询串），界面显示拒绝与 `next_action`，无文件。
5. 构造缺口（清理技术日志后续读），缺口提示出现并保留。
6. 概览页在 `sink_errors > 0` 时可见该指标。

## Assumptions

- 诊断存储、模拟器、面板布局不改；本功能只改流状态机、下载路径、提示逻辑与测试。
- 真实账号权限接入等账号域（LT-011）交付后另行处理；本功能沿用 `Scope.Accounts` 为空集合的现状。
- 服务端 25 秒轮换的存在是设计（限制单连接寿命、便于成员资格重查），本功能不取消它，只让它对用户不可见（clarify 2026-09-14：客户端识别收尾标记）。
- Web 端下载改为直接取原始响应流（`fetchRaw`）保存；不再复用 `contentDiagnosticRequest` 的 JSON 路径。
- Go 集成测试的独立 PostgreSQL 夹具按 `docs/development/diagnostics-*-regression.md` 的既有方式提供，本功能不改夹具机制。
