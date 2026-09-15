# Feature Specification: 品牌空间的时区属性与切换隔离

**Feature Branch**: `004-lt009-brand-workspace`

**Created**: 2026-09-14

**Status**: Draft

**Input**: User description: "LT-009：复用 Workspace 完成品牌名称、时区的创建、显示与切换，内容上下文从当前品牌派生。"

**Traces to**: `tasks/todo.md` LT-009；R-001；AC-001；D11-V01；`docs/12` §2 workspace-core。

**Blocked by**（来自任务卡，规格可先写，实施须等）: LT-008、ARCH-02、DG-01。

## Current State（以代码为准）

Multica 上游已有完整的 Workspace：

- 表 `workspace`（单数）：`id, name, slug, description, settings JSONB, created_at, updated_at`，后续迁移加了 `context, repos, issue_prefix, avatar_url`。**没有时区字段。**
- `settings` 是无 schema 的 JSONB，服务端原样 marshal 存入（`handler/workspace.go:403-405`），前端在设置页 workspace-tab 读写。
- API：`ListWorkspaces / GetWorkspace / CreateWorkspace / UpdateWorkspace / LeaveWorkspace / DeleteWorkspace`；`CreateWorkspaceRequest` 字段 `name, slug, description, context, issue_prefix`。
- 前端：`packages/core/workspace`（queries / mutations / hooks / workspace-url）、`packages/views/workspace`（slug、avatar、welcome、no-access）。
- 隔离机制已是硬规则：`CLAUDE.md` 要求所有 workspace 范围查询键含 `wsId`，所有查询按 `workspace_id` 过滤，`X-Workspace-ID` 选择空间；诊断实时流已按 `wsId` 重置游标。
- 术语：`conventions.zh.mdx` 固定 `workspace` → **工作区**；`docs/12` §2 把该模块称为「品牌空间」。两个词指同一对象。

因此 LT-009 的实际增量是：**时区属性**（存储、创建时设置、显示与修改）、**切换隔离的验证**（不是新建机制）、**术语落地**。不是重做 Workspace。

## 规格修正（2026-09-15 实施期）

规格写于 `3ecfaaec5`，实施在 `7a779d6`。T001 逐项重核 Current State，**三处与事实不符**，按 `docs/development/spec-kit-workflow.md` 第 8 步在本实现 PR 内修正：

| # | 规格原文 | 事实 | 处置 |
|---|---|---|---|
| 1 | US3 的 Independent Test：「用户 U2（非 A 成员）GET A 的详情 → 403/404」 | **`GetWorkspace` 处理器本身不做任何成员校验**，直接调用它会返回 200 与完整对象。授权在**路由中间件** `RequireWorkspaceMemberFromURL`（`cmd/server/router.go:1602`）。按规格原文写的测试会「证明」一个并不存在的漏洞 | 测试改为**挂载真实中间件**再验；本节记录该误导性描述 |
| 2 | plan 的 Source Code：`mutations.ts` 新增 `useUpdateWorkspace()`，设置页经它保存 | `workspace-tab.tsx` 现有的保存方式是**直接 `api.updateWorkspace` + 内联 `setQueryData`**（名称/描述、issue 前缀、头像三处皆然）。只为时区引入一个新钩子，会让同一个文件里出现两种保存模式 | **沿用该文件既有模式**；不新增 `useUpdateWorkspace`（CLAUDE.md「优先既有模式而非并行抽象」＋本任务「最小侵入」约束） |
| 3 | contracts 与 tasks 暗示 `CreateWorkspace` 可直接带 settings 落库 | `CreateWorkspaceParams` 是 **sqlc 生成**的，没有 settings 字段；要在建表语句里加就得改 SQL 并重跑 `make sqlc` | 改为**在同一事务内**先 `CreateWorkspace` 再 `UpdateWorkspace` 写 settings——原子性不变，且不动生成代码 |

**另一处事实修正（非规格错误，但规格未预见）**：时区合法性不能只问 `time.LoadLocation`。实测该函数**接受 `""`（静默当作 UTC）与 `"Local"`（服务端自己的时区）**，两者都不是品牌的时区；且 `""` 会让服务端认为「已设置」而客户端读作「未设置」。同时 `Intl.DateTimeFormat` **接受 `"+08:00"` 而 Go 拒绝**。因此两端都显式排除 `""`、`"Local"` 与偏移写法，使前后端对「什么可存」的判断完全一致——这三点各有用例钉住。

---

## Clarifications

### Session 2026-09-14

- Q: 中文界面称呼该对象用「工作区」还是「品牌空间」？ → A: 工作区。沿用术语表与上游文案，零改动；「品牌空间」只用于产品文档描述概念。
- Q: 时区存在哪里？ → A: `settings` JSON 键，键名 `loretide.timezone`；零迁移；服务端对该键做 IANA 校验。
- Q: 时区默认值？ → A: 创建时可选；未填或历史工作区无该键时缺省 `Asia/Shanghai`。

## User Scenarios & Testing *(mandatory)*

### User Story 1 - 创建品牌空间时设置时区，刷新与重登后保持 (Priority: P1)

用户创建一个品牌空间，除名称外设置该品牌的时区；创建 A、B 两个空间各自不同时区；刷新页面、重新登录后各自时区不变；在空间设置中能看到并修改时区。

**Why this priority**: AC-001 要求「创建 A/B 品牌工作区并设置各自时区…刷新仍保存对应设置」。时区是后续排期、发布记录、复盘统计的时间基准，缺了它 W-02 之后的功能没有正确的时间语义。

**Independent Test**: 创建 A（Asia/Shanghai）与 B（America/Los_Angeles），刷新后 GET 各自空间返回对应时区；修改 A 为 Europe/London 后再刷新为新值。

**Acceptance Scenarios**:

1. **Given** 创建空间表单，**When** 填写名称与时区并提交，**Then** 空间创建成功，详情响应含该时区。
2. **Given** 已有空间未设置时区（上游历史数据），**When** 读取详情，**Then** 返回默认时区（见 FR-002），前端不因字段缺失报错。
3. **Given** 提交非法时区名，**When** 创建或修改，**Then** 返回 400 且不写入。
4. **Given** 空间设置页，**When** 修改时区并保存，**Then** 刷新后为新值；已存数据的时间戳不变。

---

### User Story 2 - 切换品牌空间后不显示前一空间的数据 (Priority: P1)

用户从 A 切到 B 后，列表、详情、缓存、实时事件全部只属于 B；再切回 A 亦然。

**Why this priority**: AC-001「切换不混用素材、记忆、搜索、任务、缓存和页面事件」。机制已存在（`wsId` 查询键、`workspace_id` 过滤），本故事是把它变成可复核的验收，不是新建机制。

**Independent Test**: 在 A 创建一条可见对象，切到 B 后该对象不出现在任何列表与详情；诊断实时流游标重置且事件的 `workspace_id` 全为 B。

**Acceptance Scenarios**:

1. **Given** 用户在 A 有数据，**When** 切换到 B，**Then** 所有空间范围接口响应的 `workspace_id` 均为 B，A 的对象不出现。
2. **Given** 诊断实时流在 A 打开，**When** 切换到 B，**Then** 流以 B 重建，游标归零，不混入 A 的事件。
3. **Given** 切回 A，**When** 页面加载，**Then** A 的数据完整显示，无需重新创建。

---

### User Story 3 - 另一身份不能取得空间对象且不泄漏正文 (Priority: P2)

不是空间成员的身份请求该空间任一对象接口，被拒绝，响应不含对象正文。

**Why this priority**: AC-001 后半句「另一个身份请求任一作品/附件/搜索接口均不能取得数据」。LT-010 会建统一授权入口；本故事只验证既有成员校验对新增时区字段与 workspace 详情同样生效。

**Independent Test**: 用户 U2（非 A 成员）GET A 的详情 → 403/404，响应 body 不含 `name`、`settings`、时区。

**Acceptance Scenarios**:

1. **Given** U2 不是 A 成员，**When** U2 请求 A 的详情或更新时区，**Then** 被拒绝，响应无对象字段。
2. **Given** U2 伪造 `X-Workspace-ID: A`，**When** 请求任一空间范围接口，**Then** 被拒绝。

---

### User Story 4 - 中文界面用一致的词称呼品牌空间 (Priority: P3)

中文界面在创建、切换、设置等入口用同一个词称呼这个对象。

**Why this priority**: `conventions.zh.mdx` 已固定 `workspace` → 工作区；`docs/12` 用「品牌空间」。不定下来，文案会两种都出现。

**Independent Test**: 中文 locale 下，创建空间、切换器、设置页三处术语一致，且 PR 检查表的中文文案项被勾选。

**Acceptance Scenarios**:

1. **Given** 中文 locale，**When** 打开创建 / 切换 / 设置三处，**Then** 一律使用「工作区」；不出现「品牌空间」作为界面用词。


---

### Edge Cases

- 时区默认值：新建时未填或历史工作区无该键 → 视为 `Asia/Shanghai`；设置页显示该值并标注「默认」。
- 修改时区不重写历史数据：`created_at` 等一律 UTC 存储，显示时按空间时区转换；本功能不引入「本地时间」列。
- 上游 Multica 后续在 `settings` 里加同名键：时区存放位置需避免与上游冲突（见 Assumptions 与 clarify）。
- 切换空间瞬间仍有 A 的请求在途：响应到达时查询键已是 B，缓存不写入（TanStack Query 键隔离已保证）；不做额外处理。
- 项目（project）与品牌空间：任务卡要求「不混为同一对象」；本功能不触碰 project，只在规格里声明边界。

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: 品牌空间 MUST 具有时区属性，值为 IANA 时区名，服务端校验合法性。
- **FR-002**: 时区在创建时可选；未提供或历史工作区无该键时 MUST 返回 `Asia/Shanghai`，不返回空；设置页 MUST 标注该值为默认。
- **FR-003**: 时区 MUST 可在创建时设置、在空间设置中修改；修改 MUST NOT 改写任何已存时间戳。
- **FR-004**: 空间列表与详情响应 MUST 包含时区；前端 MUST 通过 zod schema + `parseWithFallback` 解析，缺字段时取默认，并新增一条畸形响应测试。
- **FR-005**: 切换空间后，所有空间范围查询键 MUST 含 `wsId`（既有规则），实时连接 MUST 按 `wsId` 重建；验收以实际请求的 `workspace_id` 抽样核对。
- **FR-006**: 非成员对空间对象的读写 MUST 被拒绝，响应 MUST NOT 含对象字段。
- **FR-007**: 时区 MUST 存放在工作区 `settings` JSON 的 `loretide.timezone` 键，不新增列、不做迁移；服务端在创建与更新路径上 MUST 校验该键为合法 IANA 时区名，非法则 400 且不写入；其余 `settings` 键原样透传不受影响。
- **FR-008**: 中文文案 MUST 沿用术语表「工作区」；MUST NOT 引入「品牌空间」作为界面用词（clarify 2026-09-14）。
- **FR-009**: 本功能 MUST NOT 修改 project 实体，MUST NOT 引入独立的「人设 / IP」模型（docs/11 W-02 明确「无独立人设模型」）。

### Key Entities

- **工作区（workspace）**：`id, name, slug, settings{"loretide.timezone": <IANA>}, ...`；一个用户可属于多个工作区；内容上下文由当前工作区派生。
- **成员（member）**：`workspace_id, user_id, role`；授权基础。
- **项目（project）**：独立实体，本功能不触碰。

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: 创建 A / B 各设不同时区，刷新与重新登录后 GET 各自返回设定值；修改后同样保持。
- **SC-002**: 切换到 B 后，抽样 5 个空间范围接口的响应 `workspace_id` 全部为 B；诊断流事件的 `workspace_id` 全部为 B。
- **SC-003**: 非成员请求返回 403/404 且 body 无 `name` / `timezone` / `settings`。
- **SC-004**: `pnpm typecheck` 通过；相关 Go 测试通过；新增时区字段的畸形响应测试存在且通过。
- **SC-005**: 中文 locale 三处入口术语一致（人工核对，记录截图位置由用户提供）。

## UI Impact

有。涉及：创建空间表单（新增时区选择）、空间设置 → 通用（显示与修改时区）、可能的切换器文案。

手动 UI Todo（由用户确认，不做 UI 单测、不做自动点击）：
1. 创建空间表单出现时区选择，默认值符合 clarify 决定；提交后详情显示该时区。
2. 空间设置页显示当前时区，修改保存后刷新为新值。
3. 从 A 切到 B，任务列表、诊断页均只显示 B 的内容；切回 A 恢复。
4. 中文 locale 下创建 / 切换 / 设置三处术语一致。

## Assumptions

- 复用上游 Workspace 的创建 / 更新接口扩展字段，不新建「品牌」实体。
- 时间戳继续 UTC 存储；时区只影响显示与后续排期计算。
- 时区键名 `loretide.timezone` 带前缀，避免与上游未来的 `settings` 键冲突（clarify 2026-09-14）。
- 实施顺序上本功能等待 LT-008、ARCH-02、DG-01；规格与计划可先完成。
- 时区选择控件优先复用 `packages/ui` 已有组件或 `pnpm ui:add` 的 shadcn 组件，不手写。
- IP / 执行配置（AC-001 提到）属于 LT-011 及之后，不在本功能内。
