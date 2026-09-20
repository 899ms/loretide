# Feature Specification: 今日工作台（SOP §2 入口页）

**Feature Branch**: `claude/spec-026-today-dashboard`

**Created**: 2026-09-20

**Status**: Draft（含 3 个待裁决项）

**Input**: Issue #155 — 「SOP §2：日常使用从"今日工作台"开始。那里应同时呈现值得写的选题、正在推进的作品、需要处理的审核、到期交接和待补充的经营底座。」web-only、只读聚合已落地对象、不调模型、首版不新增表。

---

## Current State（以代码为准，2026-09-20 / `app-main` @ `9f1c75a`）

这一节是本规格的事实基础。每条都从代码读出，不是从 Issue 转述。

### 已落地的内容模块

`scripts/content-boundaries.json` 登记 12 个模块，其中 **6 个已落地**（有目录）：

| 模块 | server | core | views |
|---|---|---|---|
| `diagnostics` | 有 | 有 | 有 |
| `workspace-core` | 有 | — | — |
| `ip-profile` | 有 | 有 | 有 |
| `topic-planning` | 有 | 有 | 有 |
| `work-editor` | 有 | 有 | 有 |
| `review-delivery` | 有 | 有 | **无**（025 的页面 PR 尚未落地） |

未落地：`source-inbox`、`knowledge-base`、`agent-workflow`、`project-collab`、`feedback-learning`、`agent-gateway`。

**`project-collab` 目前是空的**——它在登记表里已有依赖（`workspace-core`、`topic-planning`、`work-editor`、`review-delivery`、`diagnostics`），但没有任何代码。这直接影响 Q1。

### 今天可用的读端点（`server/cmd/server/router.go`）

```
GET  /api/content-accounts                          列账号
GET  /api/content-accounts/{id}/profile             表达配置 + readiness{can_start, missing[]}
GET  /api/content-topics?account_id=                列选题卡
GET  /api/content-topics/{id}                       卡详情
GET  /api/content-topics/{id}/briefs                简报版本
GET  /api/content-topics/{id}/briefs/{rid}/snapshots 某版的开始记录
GET  /api/content-works?topic_card_id=              列作品
GET  /api/content-works/{id}/artifacts              列文档（含 draft_status）
GET  /api/content-works/{id}/artifacts/{aid}/versions 版本历史
GET  /api/content-reviews?artifact_id=&status=      列审核请求
GET  /api/content-deliveries?artifact_id=&status=   列交付待办（含派生 due / pending_registration）
GET  /api/content-publications?artifact_id=         列发布记录
```

`content-topics` / `content-works` / `content-reviews` / `content-deliveries` / `content-publications` 在通用 member 中间件**之外**，各自到达 `workspace-core.Authorize`；`content-accounts` / `content-diagnostics` 在 `RequireWorkspaceMember` **之内**。

### 五个区块与现有数据的对应，以及三处缺口

| SOP §2 区块 | 数据源 | 今天能不能直接拿到 |
|---|---|---|
| 值得写的选题 | `content-topics` | **口径不符**，见缺口 A |
| 正在推进的作品 | `content-works` + 每个作品的 `artifacts.draft_status` | **需要 N+1**，见缺口 B |
| 需要处理的审核 | `content-reviews?status=pending`（也含 `changes_requested`） | 能，一次请求 |
| 到期交接 / 待登记发布 | `content-deliveries` 的派生 `due` / `pending_registration` | 能，一次请求 |
| 待补充的经营底座 | `content-accounts` + 每个账号的 `profile.readiness` | **需要 N+1**，见缺口 C |

**缺口 A — 选题状态口径与 Issue 不符（必须裁决）。**
Issue 写「只展示人工创建且状态为 **proposed/shortlisted** 的卡」。但 `server/internal/content/topic-planning/contract.go` 里的 `Status` 只有五个值：

```go
StatusDraft    Status = "draft"
StatusStarted  Status = "started"
StatusSaved    Status = "saved"
StatusDeferred Status = "deferred"
StatusDropped  Status = "dropped"
```

**`proposed` 和 `shortlisted` 在代码里不存在。** 我不替裁决把它们映射成 `draft`——见 Q2。另外 `TopicCard` **没有任何「谁创建的 / 是否 AI 生成」字段**，所以「只展示人工创建的」今天既无法筛选也无法验证；在 EP-04c 落地前它恒真（一切都是人建的），但这是**巧合成立，不是被保证的**。

**缺口 B — 作品的 working 状态要 N+1 次请求。**
`draft_status`（`working` / `saved`）在**文档**上，不在作品上。`ListContentWorks` 只接 `topic_card_id`，不接状态筛选，也不返回文档摘要。所以「正在推进的作品」= 列作品（1 次）+ 逐个作品列文档（N 次）。

**缺口 C — 账号就绪判定要 N+1 次请求。**
`ListContentAccounts` 返回的是账号本身；`readiness{can_start, missing[]}` 只在 `GET /api/content-accounts/{id}/profile` 上（`content_account_profile.go:21`）。所以「待补充的经营底座」= 列账号（1 次）+ 逐个账号取 profile（N 次）。

就绪判定在两侧都有实现且已做 parity：Go `ipprofile.ProfileReadiness`、TS `profileReadiness`（021 交付）。

### 页面现状

`apps/web/app/[workspaceSlug]/(dashboard)/` 下已有 `accounts` / `topics` / `diagnostics` 三个内容页。**没有 `today`，也没有工作区首页落地页**——`(dashboard)` 只有 `layout.tsx` 和 `loading.tsx`。

`today` **不需要**进 `server/internal/handler/reserved_slugs.json`：保留 slug 管的是**工作区之前**的根路由，而 `/{slug}/today` 在 `[workspaceSlug]` 之下，与 `/topics`、`/accounts` 同级。

### 本检出没有文档仓库

`docs/` 下只有 `assets/` 和 `development/`，**没有 docs/12**。所以「今日工作台归哪个模块」无法从代码或文档判定，按派单列为 clarify（Q1）。

---

## User Scenarios & Testing *(mandatory)*

### User Story 1 - 一屏看清今天要做什么（Priority: P1）

运营者早上打开工作区，第一眼就在一个页面上看到五件事各有多少、分别是哪些：可以动手写的选题、正在写但还没定稿的文档、等我批的审核、已经到点该交接的和交接完还没登记结果的、以及哪个账号的底座还缺东西。每一项都能点进它原本所属的页面继续处理。

**Why this priority**: 这是 SOP §2 的整句话。没有它，运营者要轮流打开四个页面才能知道今天有没有事——工作台不存在时，五个区块的数据全都已经有了，缺的只是「同时呈现」。

**Independent Test**: 在一个有数据的工作区打开工作台，五个区块各显示正确的条目与计数；只做这一个故事就已经可用。

**Acceptance Scenarios**:

1. **Given** 工作区里有 1 张 `draft` 选题卡、1 份 `working` 文档、1 条 `pending` 审核、1 条已到期的交付、1 个缺「每周投入」的账号，**When** 打开工作台，**Then** 五个区块各显示 1 条，且每条都写明它是哪一条（标题 / 账号名 / 渠道），不是只有一个数字。
2. **Given** 某区块一条都没有，**When** 打开工作台，**Then** 该区块显示「今天没有」的空态，**不是**隐藏整个区块，也不是一直转圈。
3. **Given** 工作台上某条选题卡，**When** 点它，**Then** 进入 `/topics` 并选中该卡；**不在工作台上就地改状态**。
4. **Given** 后端某个区块的请求失败，**When** 打开工作台，**Then** 该区块单独显示失败并可重试，**其余四个区块照常显示**——一个区块坏掉不能让整页空白。

---

### User Story 2 - 知道哪件事为什么在这里（Priority: P2）

每一条都写明它被列出来的**理由**，而不是只把对象罗列一遍：交付待办写「计划时间已过 2 天，尚未交接」，账号写「缺：每周投入、内容方向」，文档写「编辑中，最近一版是 3 天前」。

**Why this priority**: 工作台的价值是「不用点进去就知道要不要管它」。只给标题的列表仍然要求逐条点开，等于没省事。但没有它，US1 仍然可用。

**Independent Test**: 逐条核对理由文字与它在原页面上的状态一致。

**Acceptance Scenarios**:

1. **Given** 一条 `scheduled` 且计划时间已过的交付，**When** 在工作台看它，**Then** 理由是「已到期未交接」，与 `GET /api/content-deliveries` 为该条返回的 `due` 一致（025 的页面尚未落地，见 Assumptions 6）。
2. **Given** 一个 `can_start=false` 的账号，**When** 在工作台看它，**Then** 缺项**逐项点名**（用账号页同一套字段名），不是一句「配置不完整」。

---

### User Story 3 - 说清楚什么还没有（Priority: P3）

工作台明确标注「候选选题的自动生成暂不可用」，并说明现在列出的是人工创建的卡；不假装这是一份由系统推荐出来的清单。

**Why this priority**: SOP §2 的「值得写的选题」原意包含系统推荐。EP-04c 不在，留白会被读成没做完，伪造推荐则是宪法 IX 禁止的。

**Independent Test**: 该区块有一条恒定可见的说明，文案里写明原因。

**Acceptance Scenarios**:

1. **Given** EP-04c 未落地，**When** 打开选题区块，**Then** 有一条说明写明「候选自动生成暂不可用」并说明现在列的是什么，**且没有任何伪造的推荐条目**。

---

### Edge Cases

- 工作区一条数据都没有（新工作区）：五个区块各自空态，页面不报错。
- 账号很多（如 50 个）时，「待补充的经营底座」不能因为逐个取 profile 而让整页卡住——见 Assumptions 里的 N+1 取舍。
- 一条交付同时「已到期」且「待登记」在逻辑上不可能（`due` 要求 `scheduled`，`pending_registration` 要求 `handed_off`），但界面不能假设互斥到崩溃。
- 某个账号的 profile 请求 404 / 503：该账号在底座区块里标为「读取失败」，不静默从列表里消失。
- 用户在另一个标签页处理掉一条审核后回到工作台：工作台显示的是上次读取的结果，需要能刷新；**不做实时推送**。
- 跨品牌：切换工作区后工作台只显示当前工作区的对象；把别的工作区的 id 拼进任何读端点得到与「不存在」同形的拒绝。

---

## Requirements *(mandatory)*

### Functional Requirements

**范围与性质**

- **FR-001**: 工作台 MUST 是**只读聚合**：它不创建、不修改、不删除任何对象，所有动作都是跳转到对象原本所属的页面。
- **FR-002**: 工作台 MUST NOT 调用任何 AI 执行器，MUST NOT 产生任何推断出来的内容（宪法 IX）。
- **FR-003**: 首版 MUST NOT 新增数据库表。
- **FR-004**: 工作台的数据 MUST 全部来自已落地模块的读端点；不得读取未落地模块（`source-inbox`、`knowledge-base`、`agent-workflow`、`feedback-learning`、`agent-gateway`）。

**五个区块**

- **FR-005**: 工作台 MUST 同时呈现五个区块：值得写的选题、正在推进的作品、需要处理的审核、到期交接与待登记发布、待补充的经营底座。五个区块 MUST 同时可见，不得折叠掉其中任何一个作为默认。
- **FR-006**: 「需要处理的审核」MUST 列出状态为 `pending` 与 `changes_requested` 的审核请求。`approved` / `rejected` / `cancelled` 不列。
- **FR-007**: 「到期交接与待登记发布」MUST 使用 `content-deliveries` 在读时派生的 `due` 与 `pending_registration`，MUST NOT 自行重算到期规则，也 MUST NOT 把「已交接」当成「已发布」。
- **FR-008**: 「正在推进的作品」MUST 列出至少有一份文档处于 `working` 的作品，并显示是哪一份文档。
- **FR-009**: 「待补充的经营底座」MUST 列出 `readiness.can_start = false` 的账号，并**逐项列出** `readiness.missing[]`，使用与账号页相同的字段名。
- **FR-010**: 「值得写的选题」的状态口径见 [NEEDS CLARIFICATION: Q2]。在裁决前，本规格按「列出尚未开始的卡」编写，具体状态值由 Q2 确定。
- **FR-011**: 「值得写的选题」区块 MUST 有一条恒定可见的说明，写明「候选自动生成暂不可用（EP-04c 未落地）」，并说明现在列出的是人工创建的卡。MUST NOT 出现任何伪造的推荐条目。

**每条的可读性**

- **FR-012**: 每个区块的每一条 MUST 能被识别到具体对象（标题 / 账号名 / 渠道 + 文档名），MUST NOT 只显示一个计数。
- **FR-013**: 每一条 MUST 写明它被列出的理由，且理由 MUST 与该对象在其原页面上的状态一致。
- **FR-014**: 每一条 MUST 可跳转到该对象原本所属的页面。工作台上 MUST NOT 提供就地改状态的操作。

**失败与空态**

- **FR-015**: 每个区块 MUST 独立加载、独立失败。任一区块失败 MUST NOT 让其余区块无法显示。
- **FR-016**: 每个区块为空时 MUST 显示明确空态，MUST NOT 隐藏区块，MUST NOT 停在加载态。
- **FR-017**: 单条记录读取失败时 MUST 在其所在区块内标注失败，MUST NOT 静默从列表中消失（否则「今天没有事」和「读不到」看起来一样）。

**边界与授权**

- **FR-018**: 所有读取 MUST 限定在当前工作区。越权访问 MUST 返回与「不存在」同形的拒绝（404），MUST NOT 透露对象存在。
- **FR-019**: 若新增任何只读端点，每个带路径参数的端点 MUST 有一条用例穿过真实中间件，且路径参数值与上下文值不同（工作流第 12 步）。
- **FR-020**: 工作台 MUST NOT 引入实时推送；数据新鲜度由页面自身的读取与用户可见的刷新决定。

**归属与路由**

- **FR-021**: 读模型的模块归属见 [NEEDS CLARIFICATION: Q1]。无论裁决结果如何，MUST 通过 `pnpm check:content-boundaries`，MUST NOT 为了让某个模块 import 另一个而修改 `scripts/content-boundaries.json` 的依赖表（除非裁决明确要求）。
- **FR-022**: 页面路由见 [NEEDS CLARIFICATION: Q3]。
- **FR-023**: 页面 MUST 只挂既有组件（宪法 VII），MUST NOT 新增控件或新依赖，MUST 四语言（en / zh-Hans / ja / ko）。
- **FR-024**: MUST NOT 写 UI 单测（宪法 II）；界面项进 `manual-ui-todo.md`。非 UI 的聚合与判定逻辑 MUST 有 node 测试。

### Key Entities

工作台**不定义新实体**。它呈现的是已有对象的一个视图：

- **选题卡**（`topic-planning`）：`status`、`account_id`、`started_brief_revision_id`。
- **作品 / 文档**（`work-editor`）：文档的 `draft_status`（`working` / `saved`）、最近版本。
- **审核请求**（`review-delivery`）：`status`。
- **交付待办**（`review-delivery`）：`status`、`scheduled_at`，以及**读时派生**的 `due`、`pending_registration`——这两个不存库，按 SOP 9.2「不推断平台状态」。
- **账号与表达配置**（`ip-profile`）：`readiness{can_start, missing[]}`。

唯一新增的概念是**「工作台读模型」**：把上述对象按五个区块组织起来的一个纯读结果。它是否成为一个服务端对象，取决于 Q1。

---

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: 运营者在**一个页面**上就能判断今天有没有事要做，不需要打开其他页面——五个区块全部可见。
- **SC-002**: 五个区块中任意一个的数据源不可用时，其余四个仍然可用（可通过逐个模拟失败验证，5 次中 5 次）。
- **SC-003**: 工作台列出的每一条，其状态与理由都与该对象在原页面上显示的一致（逐条人工核对，0 处不一致）。
- **SC-004**: 工作台不产生任何写操作——整页操作一遍后，五类对象的数据与操作前逐字节相同（除跳转外无副作用）。
- **SC-005**: 「候选自动生成暂不可用」的说明在选题区块恒定可见，且该区块内 0 条伪造推荐。
- **SC-006**: 空工作区打开工作台：5 个区块 5 个空态，0 个加载态残留，0 个报错。

---

## Assumptions

以下是本规格在派单未明确处所做的默认判断。**每条都可被主任务推翻。**

1. **读取时机**：工作台在打开时读取一次，用户可手动刷新。不做轮询、不做 WebSocket 推送（FR-020）。理由是五类数据都不是秒级变化的，而推送会把工作台变成一个需要维护缓存一致性的东西。
2. **N+1 的取舍（重要）**：「正在推进的作品」与「待补充的经营底座」今天都需要 1+N 次请求（缺口 B、C）。**首版接受 N+1**，并对区块设一个可见的条数上限（建议 10 条，超出显示「还有 N 条」并跳转到原页面）。
   替代方案是新增两个只读聚合端点。我**不默认选它**，因为派单写的是「必要时只加只读端点」而不是「先加端点」，而上限 10 条让 N+1 在真实规模下不成为问题。**若主任务认为账号/作品规模会大到 N+1 不可接受，请在 Q1 一并裁决**。
3. **「人工创建」今天无法验证**：`TopicCard` 没有来源字段（缺口 A）。首版按「全部卡都是人工创建」处理，并在 FR-011 的说明里写清楚。EP-04c 落地时**必须**加来源字段，否则这个筛选无处可依。
4. **不含桌面端**：与 `/topics`、`/accounts`、`/diagnostics` 一致，首版 web-only，桌面端不接线、侧边栏入口由主任务另行决定。
5. **审核区块不区分「我要批的」和「别人要批的」**：`ReviewRequest` 有 `requested_by` 与 `decided_by`，但**没有「指派给谁审」的字段**，所以「等我批的」今天无法判定；首版列的是工作区内全部待处理审核。
6. **`review-delivery` 的页面尚未落地**（Current State），所以「跳转到原页面」对审核与交付两个区块**暂时无处可跳**。首版这两个区块的条目**不可点**，并写明「对应页面尚未落地」；等 025 页面 PR 落地后再接上。**这是一个真实的功能缺口，不是遗漏。**
7. **计数与列表同源**：区块标题上的计数就是列表的条数（或「上限内 N 条 / 共 M 条」），不单独算一份。

---

## 待裁决（clarify）

### Q1：工作台读模型归哪个模块？

**Context**: FR-021。本检出没有 docs/12，无法判定归属（Current State）。

| 选项 | 做法 | 代价 |
|---|---|---|
| **A** | 新建 `project-collab` 模块作为跨模块读模型：一个 `GET /api/content-today`，服务端聚合五个区块 | 登记表里 `project-collab` 已经依赖那四个模块，**不用改依赖表**；一次请求解决缺口 B、C 的 N+1；但要落地一个至今为空的模块，且工作台的口径会固化在服务端 |
| **B**（推荐暂定） | 不新建模块：各模块用**已有的**读端点，由 web 适配器页组合（与 #152 的插槽注入同一口径） | 首版零新增服务端代码、零新增端点、零新增表；但缺口 B、C 的 N+1 留在前端，且五个区块的口径散在页面里 |
| **C** | 折中：各模块各加**一个只读汇总端点**（如 `GET /api/content-accounts/readiness`、`GET /api/content-works?draft_status=working`），仍由适配器组合 | 消掉 N+1 且不新建模块；但要动四个上游模块的端点面 |

**我的推荐是 B**，理由：派单写「首版不新增表；必要时只加只读端点」，B 是唯一一个连端点都不用加的；而 #152 刚确立了「跨模块组合在适配器里做」的先例。**但 B 把工作台的口径放在页面层，如果之后桌面端也要工作台，这套口径要复制一遍**——这是 B 的真实代价，请一并考虑。

### Q2：「值得写的选题」列哪些状态？

**Context**: 缺口 A。Issue 写 `proposed` / `shortlisted`，**代码里没有这两个值**，只有 `draft` / `started` / `saved` / `deferred` / `dropped`。

| 选项 | 口径 | 含义 |
|---|---|---|
| **A**（推荐暂定） | 只列 `draft` | 「还没开始的卡」。`started` 已经在写了（属于作品区块），`saved` / `deferred` / `dropped` 是已决策的 |
| **B** | 列 `draft` + `deferred` | 把「暂缓」的也捞回来，避免它们被永久遗忘 |
| **C** | 在 022 的 `Status` 里**新增** `proposed` / `shortlisted` 两个值 | 贴合 Issue 原文，但这是改 022 已落地的受控枚举 + 一次迁移，**远超本卡范围**，且首版说了不新增表 |

**我的推荐是 A**。C 我不会自行采纳——改一个已落地模块的状态枚举需要单独一张卡。

### Q3：路由放哪里？

**Context**: FR-022。`(dashboard)` 下今天**没有**工作区首页落地页。

| 选项 | 路由 | 含义 |
|---|---|---|
| **A**（推荐暂定） | 新增 `/{slug}/today` | 与 `/topics`、`/accounts` 同级；不需要进保留 slug 表（它在 `[workspaceSlug]` 之下）；侧边栏是否加入口另议 |
| **B** | 作为工作区默认落地页（进入工作区先看到它） | 最贴合 SOP §2「日常使用从这里开始」；但会改变现有进入工作区的行为，属于动上游导航 |
| **C** | 先做 `/{slug}/today`，落地稳定后再由主任务决定要不要设为默认落地页 | 两步走 |

**我的推荐是 C**：先按 A 交付，默认落地页单独决定。把 B 放进本卡会让一个只读页面 PR 顺手改掉所有人进入工作区的行为。
