# Feature Specification: 内容对象的空间授权入口（LT-010）

**Feature Branch**: `claude/spec-014-lt010-workspace-authorization`

**Created**: 2026-09-15

**Status**: Draft

**Input**: `tasks/todo.md` LT-010——「给后续内容服务提供基于认证主体的品牌权限校验，定义无权读取/写入的统一行为」。

**Traces to**: NFR-03；AC-001；D10-V06；`docs/12` §2 workspace-core。

**SOP**: §3 经营底座链条第二块；LT-011（账号配置）的前置。

## Current State（以代码为准，2026-09-15 于 `app-main` `2bb8ff5` 核实）

**授权这件事已经有一套在跑，但只服务诊断一个模块。**

`server/internal/handler/content_diagnostics.go` 的 `diagnosticScope` 是唯一的入口，顺序固定：

| # | 步骤 | 失败时 |
|---|---|---|
| 1 | 服务未装配 | 503 `diagnostics unavailable` |
| 2 | `isMachineCredentialActor(r)` | 拒绝（403 诊断错误对象） |
| 3 | `resolveWorkspaceID(r)` | —— |
| 4 | `requireUserID(w,r)` | 401 |
| 5 | `getWorkspaceMember(ctx, actor, workspace)` | **404**，body `{error:"workspace not found", code:"AUTHORIZATION_DENIED", trace_id, next_action:"check_authorization"}` |
| 6 | `roleAllowed(member.Role,"owner","admin")` | **403** 诊断错误对象 |
| 7 | `Scope{Workspace,Actor,Accounts:[]}` + `scope.Allows(workspace, account_id)` | **403** 诊断错误对象 |

**必须记下来的两点**：

1. **第 5 步与第 6/7 步的状态码不同**（404 vs 403），但**错误码相同**（都是 `AUTHORIZATION_DENIED`）。第 5 步是内联 `writeJSON`，不走 `diagnosticError`。
2. `diagnosticError` 产出的错误对象经 `diagnostics.Sanitize`，形状为 `{error, code, trace_id, component, retryable, next_action}`——**正文一律不含对象字段**，这就是「拒绝不泄漏对象正文」的既有实现。

**其它既有事实**：

- `Scope.Allows(workspace, account)`（`contract.go`）：actor 或 workspace 为空、或 workspace 不匹配 → false；account 为空 → true；否则必须在 `Accounts` 白名单内。**目前 `Accounts` 恒为空切片**，注释写明「真实账号权限待账号域交付」。
- `getWorkspaceMember` **每次都查库，没有缓存**（`handler.go:951`）。所以成员关系一旦变化，下一次调用立即生效。（有缓存的是 daemon 路径 `RequireDaemonWorkspaceAccess`，不是本卡涉及的用户路径。）
- 诊断实时流**每批重查成员与角色**（`ContentDiagnosticStream` 循环内），撤销会关闭在跑的流——这是「并发归属变化后旧授权失效」的既有先例。
- `server/internal/content/` 下**只有 `diagnostics` 一个已落地模块**；`scripts/content-boundaries.json` 注册了 12 个模块，其中 **`workspace-core` 已声明可依赖 `diagnostics`**。
- 上游 `member` 表与 `getWorkspaceMember` 是既有事实，本卡**只读不改**。

**因此 LT-010 的实际增量是**：把这套判定从 `diagnosticScope` 里提取成 **content 层可复用的助手**，让后续内容服务共用同一套「谁能读写这个空间的对象」与「拒绝长什么样」；**不是再造一份**。诊断改为调用共享助手，**行为逐字节不变并有测试自证**。

## Clarifications

### Session 2026-09-15

三项均已由主任务裁决，**全部取 A**，与本会话的推荐值一致，正文无需回改。

- **Q1 助手放哪？** → **A（已裁决）**：新建 `server/internal/content/workspace-core/`。裁决同时指明：**本特性因此是接入合同真正生效的第一个新模块目录**——E1/E2/E3 必须满足，`pnpm check:diagnostics-contract` 必须过且报 **`checked 2 landed modules`**；**拒绝判定要经诊断记一条审计或技术事件，不得为了过检查塞一个无意义的调用**。
- **Q2 拒绝响应统一成什么形状？** → **A（已裁决）**：助手返回**类型化判定**；诊断现有 403/404 **逐字节不变并有测试自证**；新模块规范映射为**无权与不存在同为 404**，响应体用诊断错误对象形状，**不带对象正文**。
- **Q3 是否允许缓存成员关系？** → **A（已裁决）**：不缓存，每次判定都查；**并发归属变化用例就靠这条成立**。

详见文末「待澄清问题」。

## User Scenarios & Testing *(mandatory)*

### User Story 1 —— 后续内容服务有一个共用的授权入口（P1）

写第二个内容模块的人，不需要重新想一遍「怎么判断这个人能不能读这个空间的对象」，也不需要抄一遍 `diagnosticScope`。他调用一个助手，拿到一个明确的判定结果。

**Why this priority**：这是本卡的全部目的。没有它，第二个模块会抄一份，第三个模块会抄一份走样的，「统一行为」就永远不存在。

**Independent Test**：助手可在 Go 测试里直接调用，给定成员关系与请求主体，断言判定结果；无需 HTTP、无需界面。

**Acceptance Scenarios**：

1. **Given** 用户是空间成员且角色够，**When** 求判定，**Then** 允许，并带回解析出的空间与主体。
2. **Given** 用户不是该空间成员，**When** 求判定，**Then** 拒绝，理由为「非成员」。
3. **Given** 用户是成员但角色不够，**When** 求判定，**Then** 拒绝，理由为「角色不足」，**与非成员是不同的理由**（调用方可据此决定响应，但**默认映射到同一响应**——见 US3）。

---

### User Story 2 —— 诊断改用共享助手，行为一个字节都没变（P1）

诊断模块已经在生产上按现有规则拒绝与放行。提取助手**不能**改变它的任何一个响应。

**Why this priority**：这是提取型改动的唯一风险。行为变了就不是提取，是回归。

**Independent Test**：诊断既有的授权用例全部保持通过；另加用例钉住「非成员 404、角色不足 403、账号不匹配 403」三种状态码与错误码组合。

**Acceptance Scenarios**：

1. **Given** 非成员请求诊断接口，**When** 提取前后各跑一次，**Then** 状态码与 body 完全相同（404 + `AUTHORIZATION_DENIED`）。
2. **Given** 成员但角色不足，**Then** 403 + `AUTHORIZATION_DENIED`，body 为经 `Sanitize` 的诊断错误对象。
3. **Given** `account_id` 不在白名单，**Then** 403 + `AUTHORIZATION_DENIED`。

---

### User Story 3 —— 拒绝不泄漏任何东西，包括「它存在」（P1）

被拒绝的人拿不到对象正文，**也拿不到「这个对象存在」这条信息**。无权访问与根本不存在，回同一个响应。

**Why this priority**：AC-001 与任务卡验收都写明「拒绝结果不泄漏对象正文」；「关联不存在」被单列为验证项，说明存在性本身也是要保护的。

**Independent Test**：对同一接口分别用「他人空间的真实 id」与「一个不存在的 id」请求，断言两次响应**逐字节相同**。

**Acceptance Scenarios**：

1. **Given** 传入他人空间的真实 `workspace_id`，**When** 请求，**Then** 拒绝，body 不含名称、设置或任何对象字段。
2. **Given** 传入一个根本不存在的 `workspace_id`，**When** 请求，**Then** **与上一条完全相同的响应**——攻击者无法据此判断哪个存在。
3. **Given** 任一拒绝响应，**When** 检查 body，**Then** 只含 `{error, code, trace_id, component, retryable, next_action}` 白名单字段。

---

### User Story 4 —— 归属变了，旧授权立刻失效（P2）

用户被移出空间后，他手上正在进行的请求与下一次请求都拿不到数据。

**Why this priority**：任务卡把「并发归属变化」单列为验证项。机制已存在（每次查库、流每批重查），本故事把它变成可复核的断言。

**Independent Test**：判定通过后删除成员行，再次判定即拒绝；无需真实并发。

**Acceptance Scenarios**：

1. **Given** 用户此刻是成员且判定通过，**When** 成员行被删除，**Then** 下一次判定拒绝。
2. **Given** 用户角色被从 admin 降为 member，**When** 下一次判定，**Then** 按新角色拒绝。
3. **Given** 判定助手，**When** 连续调用两次，**Then** 每次都真的查了成员关系（不缓存，Q3-A）。

---

### Edge Cases

- `workspace_id` 格式非法（非 UUID）：与「无权」同一响应，不得回 400 暴露「格式对了但没权限 / 格式就不对」的差别。
- 主体缺失（未认证）：仍是 401，**不**并入 404——未认证与无权是不同的事，合并会让登录态失效表现为「资源不存在」。
- 机器凭据主体：沿用诊断现有的拒绝（`isMachineCredentialActor`）。
- 空间存在但调用方传了另一个空间的对象 id：属调用方的对象归属校验，助手只答「这个主体能不能进这个空间」；**边界写明**，避免调用方误以为助手替它查了对象归属。

## Requirements *(mandatory)*

### Functional Requirements

**助手本体**

- **FR-001**: MUST 提供一个可复用的授权助手，输入为「请求主体 + 目标空间」，输出为**类型化判定**（允许 / 拒绝 + 理由）。
- **FR-002**: 拒绝理由 MUST 至少区分：非成员、角色不足、主体缺失、空间标识不可用。理由供调用方记录与排障，**不**直接决定响应。
- **FR-003**: 助手 MUST NOT 自己写 HTTP 响应；响应形状由调用方按映射决定（Q2-A）。

**统一拒绝行为**

- **FR-004**: MUST 提供一个**规范映射**，供新内容模块使用：非成员、角色不足、空间不存在、空间标识非法 → **同一响应**（404 + `AUTHORIZATION_DENIED`）。
- **FR-005**: 拒绝响应 body MUST 只含 `{error, code, trace_id, component, retryable, next_action}`，经 `diagnostics.Sanitize` 产出，**不含任何对象字段**。
- **FR-006**: 未认证 MUST 仍为 401，MUST NOT 并入 404。
- **FR-007**: 诊断模块 MUST 沿用它现有的状态码映射（非成员 404、角色不足 403、账号不匹配 403），**行为逐字节不变**，并有测试自证。

**归属变化**

- **FR-008**: 助手 MUST 在每次判定时读取当时的成员关系，MUST NOT 缓存（Q3-A）。
- **FR-009**: 成员被移除或角色被降低后，下一次判定 MUST 拒绝。

**边界**

- **FR-010**: 助手只判定「主体对空间的权限」，MUST NOT 承担对象归属校验；该边界 MUST 写入契约。
- **FR-011**: MUST NOT 新增数据库外键或级联；预期**零迁移**。
- **FR-012**: MUST NOT 修改上游 Multica 代码与 `server/internal/daemon/`；上游 workspace 只读。
- **FR-013**: 新建的 `server/internal/content/workspace-core/` MUST 满足接入合同 E1/E2/E3，`pnpm check:diagnostics-contract` MUST 退出 0 且报 **`checked 2 landed modules`**（本特性是该合同生效后的**第一个**新模块目录）。
- **FR-013a**: E2 的满足 MUST 来自**真实的接入**——拒绝判定经诊断记一条技术事件；MUST NOT 为通过检查而塞入一个不产生记录的调用。
- **FR-014**: 本卡**无界面改动**；MUST NOT 新增 UI 单测。

### Key Entities

- **授权主体**：已认证的用户标识（`actor`）。机器凭据主体单独拒绝。
- **目标空间**：`workspace_id`。由调用方解析后交给助手，助手不负责解析来源。
- **成员关系**：上游 `member` 表的 `(workspace_id, user_id, role)`，只读。
- **判定结果**：允许（带 actor 与 workspace）或拒绝（带理由）。

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: 第二个内容模块接入授权**无需复制** `diagnosticScope` 的任何一行——调用助手即可。
- **SC-002**: 诊断的授权行为在提取前后**完全一致**：三种拒绝组合（404/403/403 + `AUTHORIZATION_DENIED`）各有用例钉住。
- **SC-003**: 「他人空间的真实 id」与「不存在的 id」两次请求的响应**逐字节相同**。
- **SC-004**: 成员被移除后下一次判定拒绝，用例覆盖。
- **SC-005**: 五条核心负例各有一个用例，且**先写并确认失败**：同用户跨品牌、另一身份、传入他人 `workspace_id`、关联不存在、并发归属变化。
- **SC-006**: 每条不变量各有一处变异验证使对应用例变红。
- **SC-007**: 新增 UI 单测数 **0**；迁移数 **0**；上游与 daemon 改动行数 **0**。

## UI Impact

**无。** 本卡是服务端授权助手，无界面改动，因此不产生手动 UI 条目、不新增 UI 单测。

> **SOP 可操作阶段的界面规则**（用户 2026-09-15 确认）：不在 UI/UX 美观上花时间，界面整套继承上游 Multica 的设计系统与设计 token（`packages/ui` 组件、`--text-*` 字号、语义色、间距圆角），只做「把功能挂到既有组件上」——不新增控件、不调样式、不引入第二套视觉语言。**本卡无界面改动，故无适用项**；若实施中发现确需界面改动，按此规则执行并在 PR 正文只说明复用了哪些既有组件。

## Assumptions

- 上游 `member` 表与 `getWorkspaceMember` 只读复用，不改模型。
- `Scope.Accounts` 目前恒为空、真实账号权限待账号域（LT-011）交付；本卡**不**提前实现账号级授权，只保留接口形状。
- 「统一行为」指新模块的规范映射；诊断因已在生产上按现有形状被验过（`002-V05-11`），**保持不变**而非被统一改造。
- 对象归属校验（某对象是否属于该空间）属调用方，不在本助手内。

## 待澄清问题（已按推荐值暂定，不阻塞）

### Q1 授权助手放在哪个目录？

任务卡写「预计涉及 `server/internal/service` 权限助手」，但本卡硬约束又要求「不碰上游 Multica 代码」。`server/internal/service/` 是上游目录。

| 选项 | 做法 | 含义 |
|---|---|---|
| **A（推荐，已暂定）** | 新建 `server/internal/content/workspace-core/` | 该模块名**已在 `content-boundaries.json` 注册**且**已声明可依赖 `diagnostics`**，E1 自然满足；与 `docs/12` §2 对齐；边界检查覆盖得到。代价：需满足 E1/E2/E3 |
| B | 放 `server/internal/service/` | 贴合任务卡字面，但在上游目录里新增 Loretide 代码，与「不碰上游」冲突，且内容边界检查覆盖不到 |
| C | 留在 `content/diagnostics/` 内导出 | 改动最小，但让「授权」永久挂在诊断模块下，第三个模块会依赖 diagnostics 才能鉴权，方向错了 |

### Q2 拒绝响应统一成什么形状？

现状：非成员 **404**、角色不足 **403**、账号不匹配 **403**，三者错误码都是 `AUTHORIZATION_DENIED`。验收要求「关联不存在 → 与无权同一响应」。

| 选项 | 做法 | 含义 |
|---|---|---|
| **A（推荐，已暂定）** | 助手只返回类型化判定；**诊断沿用现有 404/403**（逐字节不变），助手另提供**新模块用的规范映射**（无权与不存在同为 404） | 既满足「统一行为」（对新模块），又不回归已验过的诊断行为 |
| B | 全部统一为 404 | 「统一」最彻底，但**改变诊断现行行为**，与「行为逐字节不变」直接冲突，且 `002-V05-11` 验过的 403 会失效 |
| C | 全部统一为 403 | 同样改变现行行为，且 403 比 404 更泄漏（承认资源存在） |

### Q3 是否允许缓存成员关系？

现状用户路径**每次查库**；daemon 路径有缓存。

| 选项 | 做法 | 含义 |
|---|---|---|
| **A（推荐，已暂定）** | 不缓存，每次判定都查 | 「并发归属变化后旧授权立刻失效」自然成立，可断言。代价：每次判定一次查询 |
| B | 允许短 TTL 缓存 | 少查库，但撤销有延迟窗口，与验收项直接冲突，且需要额外的失效路径 |
