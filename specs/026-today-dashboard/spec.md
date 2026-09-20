# Feature Specification: 今日工作台（SOP §2 入口页）

**Feature Branch**: `claude/spec-026-today-dashboard`

**Created**: 2026-09-20

**Status**: Draft（3 个待裁决项已由主控裁决，见「裁决记录」）

**Input**: Issue #155 + 主控裁决（PR #158 评论，2026-09-21）。

SOP §2 原文：「日常使用从"今日工作台"开始。那里应同时呈现**值得写的选题、正在推进的作品、需要处理的审核、到期交接和待补录的反馈**。纯浏览、积累素材、暂缓创作都是有效结果。」

> **Issue #155 里的「待补充的经营底座」是转述有误**，原文第五项是**「待补录的反馈」**。本规格以原文为准。经营底座（021 就绪判定）作为第六个附加区块保留，标题按其本义写「账号配置缺项」，**不冒充 §2 的第五项**。

SOP §11「每天开始」行：用户主要工作是「看选题建议，选定今天推进的一项工作」，工作台应呈现「选题理由、预计投入、审核与交接到期项」。

web-only、只读聚合已落地对象、不调模型、首版不新增表、不新增端点。

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

未落地：`source-inbox`、`knowledge-base`、`agent-workflow`、`project-collab`、`agent-gateway`。（**2026-09-20 更新**：`feedback-learning` 已由 specs/027 实施 PR 1 落地，从本清单移出；本节其余内容仍是 `9f1c75a` 时的快照。）

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

### 六个区块与现有数据的对应，以及三处缺口

§2 原文五项 + 第六个附加区块：

| # | 区块 | 出处 | 数据源 | 今天能不能直接拿到 |
|---|---|---|---|---|
| 1 | 值得写的选题 | §2 原文①，§11 要求带理由与预计投入 | `content-topics` + 账号 `weekly_hours` | **口径不符 + 需要 N+1**，见缺口 A、D |
| 2 | 正在推进的作品 | §2 原文② | `content-works` + 每个作品的 `artifacts.draft_status` | **需要 N+1**，见缺口 B |
| 3 | 需要处理的审核 | §2 原文③ | `content-reviews?status=` | 能，一次请求 |
| 4 | 到期交接与待登记发布 | §2 原文④ | `content-deliveries` 的派生 `due` / `pending_registration` | 能，一次请求 |
| 5 | **待补录的反馈** | §2 原文⑤ | `feedback-learning` | ~~**模块未落地**，见缺口 E~~ **已落地**（specs/027 PR 1，2026-09-20） |
| 6 | 账号配置缺项（附加） | §11「经营底座」，021 就绪判定 | `content-accounts` + 每个账号的 `profile.readiness` | **需要 N+1**，见缺口 C |

**缺口 A — 选题状态口径与 Issue 不符（已由 Q2 裁决）。**
Issue 写「只展示人工创建且状态为 **proposed/shortlisted** 的卡」。但 `server/internal/content/topic-planning/contract.go` 里的 `Status` 只有五个值：

```go
StatusDraft    Status = "draft"
StatusStarted  Status = "started"
StatusSaved    Status = "saved"
StatusDeferred Status = "deferred"
StatusDropped  Status = "dropped"
```

**`proposed` 和 `shortlisted` 在代码里不存在。** 裁决 Q2 = A：**只列 `draft`**，`deferred` 首版不列，**不改 022 的枚举**。另外 `TopicCard` **没有任何「谁创建的 / 是否 AI 生成」字段**，所以「只展示人工创建的」今天既无法筛选也无法验证；在 EP-04c 落地前它恒真（一切都是人建的），但这是**巧合成立，不是被保证的**——见 FR-011a。

**缺口 B — 作品的 working 状态要 N+1 次请求。**
`draft_status`（`working` / `saved`）在**文档**上，不在作品上。`ListContentWorks` 只接 `topic_card_id`，不接状态筛选，也不返回文档摘要。所以「正在推进的作品」= 列作品（1 次）+ 逐个作品列文档（N 次）。

**缺口 C — 账号就绪判定要 N+1 次请求。**
`ListContentAccounts` 返回的是账号本身；`readiness{can_start, missing[]}` 只在 `GET /api/content-accounts/{id}/profile` 上（`content_account_profile.go:21`）。所以「待补充的经营底座」= 列账号（1 次）+ 逐个账号取 profile（N 次）。

就绪判定在两侧都有实现且已做 parity：Go `ipprofile.ProfileReadiness`、TS `profileReadiness`（021 交付）。

**缺口 D — §11 的「预计投入」也要 N+1。**
§11 要求选题区块显示「预计投入」。今天唯一能当它的字段是账号表达配置里的 `weekly_hours`（`ip-profile/profile.go:67`，`HoursField`，带 `value` 与 `status`）——它在**账号**上，不在选题卡上。选题卡只有 `account_id`。所以「每张卡的预计投入」= 列卡（1 次）+ 取相关账号的 profile（N 次，与缺口 C 同源，可合并复用）。

**注意 `weekly_hours` 可能是 `pending` 状态**（021 的 `FieldPending`）。此时它**不是**一个可信的预计投入，必须显示为「未确认」，不能把一个待补充的数字当成承诺。

**缺口 E — 「待补录的反馈」的模块不存在。**
`feedback-learning` 在 `scripts/content-boundaries.json` 里已登记，但 `server/internal/content/`、`packages/core/content/`、`packages/views/content/` 下**都没有该目录**。§2 原文的第五项今天**没有任何数据源**。裁决：该区块**存在但显示「暂不可用（反馈记录接入后）」**，与 024 的三个 AI 入口同一口径——不隐藏、不留白、不伪造。

> **2026-09-20 更新（specs/027 实施 PR 1）：缺口 E 已关闭。** 模块的 server 与 core 两侧已落地，第五项的数据源是 `GET /api/content-feedback/pending`。上面这段保留为当时的事实记录。

### 页面现状

`apps/web/app/[workspaceSlug]/(dashboard)/` 下已有 `accounts` / `topics` / `diagnostics` 三个内容页。**没有 `today`，也没有工作区首页落地页**——`(dashboard)` 只有 `layout.tsx` 和 `loading.tsx`。

`today` **不需要**进 `server/internal/handler/reserved_slugs.json`：保留 slug 管的是**工作区之前**的根路由，而 `/{slug}/today` 在 `[workspaceSlug]` 之下，与 `/topics`、`/accounts` 同级。

### 本检出没有文档仓库

`docs/` 下只有 `assets/` 和 `development/`，**没有 docs/12**。所以「今日工作台归哪个模块」无法从代码或文档判定，按派单列为 clarify（Q1）。

---

## User Scenarios & Testing *(mandatory)*

### User Story 1 - 一屏看清今天要做什么（Priority: P1）

运营者早上打开工作区，第一眼就在一个页面上看到五件事各有多少、分别是哪些：可以动手写的选题、正在写但还没定稿的文档、等我批的审核、已经到点该交接的和交接完还没登记结果的、以及哪个账号的底座还缺东西。每一项都能点进它原本所属的页面继续处理。

**Why this priority**: 这是 SOP §2 的整句话。没有它，运营者要轮流打开四个页面才能知道今天有没有事——工作台不存在时，除「待补录的反馈」外各区块的数据全都已经有了，缺的只是「同时呈现」。

**Independent Test**: 在一个有数据的工作区打开工作台，六个区块各显示正确的条目与计数（第五项显示暂不可用）；只做这一个故事就已经可用。

**Acceptance Scenarios**:

1. **Given** 工作区里有 1 张 `draft` 选题卡、1 份 `working` 文档、1 条 `pending` 审核、1 条已到期的交付、1 个缺「每周投入」的账号，**When** 打开工作台，**Then** 区块 1/2/3/4/6 各显示 1 条，且每条都写明它是哪一条（标题 / 账号名 / 渠道），不是只有一个数字；区块 5「待补录的反馈」显示「暂不可用」。
2. **Given** 某区块一条都没有，**When** 打开工作台，**Then** 该区块显示「今天没有」的空态，**不是**隐藏整个区块，也不是一直转圈。
3. **Given** 工作台上某条选题卡，**When** 点它，**Then** 进入 `/topics` 并选中该卡；**不在工作台上就地改状态**。
4. **Given** 后端某个区块的请求失败，**When** 打开工作台，**Then** 该区块单独显示失败并可重试，**其余区块照常显示**——一个区块坏掉不能让整页空白。

---

### User Story 2 - 知道哪件事为什么在这里（Priority: P2）

每一条都写明它被列出来的**理由**，而不是只把对象罗列一遍：交付待办写「计划时间已过 2 天，尚未交接」，账号写「缺：每周投入、内容方向」，文档写「编辑中，最近一版是 3 天前」。

**Why this priority**: 工作台的价值是「不用点进去就知道要不要管它」。只给标题的列表仍然要求逐条点开，等于没省事。但没有它，US1 仍然可用。

**Independent Test**: 逐条核对理由文字与它在原页面上的状态一致。

**Acceptance Scenarios**:

1. **Given** 一条 `scheduled` 且计划时间已过的交付，**When** 在工作台看它，**Then** 理由是「已到期未交接」，与 `GET /api/content-deliveries` 为该条返回的 `due` 一致（025 的页面尚未落地，见 Assumptions 6）。
2. **Given** 一个 `can_start=false` 的账号，**When** 在工作台看它，**Then** 缺项**逐项点名**（用账号页同一套字段名），不是一句「配置不完整」。
3. **Given** 一张关联账号的 `draft` 卡，**When** 在选题区块看它，**Then** 显示该卡的理由摘要与「预计投入」（该账号的每周可投入时间）；**Given** 该账号的每周投入仍是「待补充」状态，**Then** 预计投入显示为「未确认」，不显示一个看起来像承诺的数字。

---

### User Story 3 - 说清楚什么还没有（Priority: P3）

工作台明确标注两处暂不可用，并说明原因：选题区块的「候选自动生成暂不可用（EP-04c 未落地）」，以及「待补录的反馈」整块「暂不可用（反馈记录接入后）」。不假装选题是系统推荐出来的，也不把不存在的反馈区块悄悄删掉。

**Why this priority**: SOP §2 的「值得写的选题」原意包含系统推荐，「待补录的反馈」则整项没有数据源。留白会被读成没做完，伪造则是宪法 IX 禁止的。这与 024 处理三个 AI 入口的口径一致：**存在、禁用、写明原因**。

**Independent Test**: 两处说明各有一条恒定可见的文案，文案里写明原因。

**Acceptance Scenarios**:

1. **Given** EP-04c 未落地，**When** 打开选题区块，**Then** 有一条说明写明「候选自动生成暂不可用」并说明现在列的是人工创建的 `draft` 卡，**且没有任何伪造的推荐条目**。
2. ~~**Given** `feedback-learning` 未落地，**When** 打开工作台，**Then** 「待补录的反馈」区块**存在**、显示「暂不可用（反馈记录接入后）」~~ **（2026-09-20 起不再适用：模块已落地。）** 现行口径见 FR-005b：该区块列出已发布却一条指标都没录的发布记录，**不隐藏该区块**、**不显示加载中**、**不伪造任何反馈条目**。

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
- **FR-004**: 工作台的数据 MUST 全部来自已落地模块的读端点；MUST NOT 读取未落地模块（`source-inbox`、`knowledge-base`、`agent-workflow`、`project-collab`、`agent-gateway`）。

  > **2026-09-20 更新（specs/027 实施 PR 1）**：`feedback-learning` 已落地（Issue #166），从本名单移出。§2 第五项「待补录的反馈」因此有了数据源，FR-005b 随之改写。改动由 027 而不是 026 做，是主控对 027 Q6 的裁决：把它写成 027 PR 1 的交付要求，而不是留一句「希望有人记得」——两张卡都以为对方会处理，正是这条会掉的地方。

**区块清单**

- **FR-005**: 工作台 MUST 同时呈现 SOP §2 原文的五个区块，顺序与原文一致：①值得写的选题 ②正在推进的作品 ③需要处理的审核 ④到期交接与待登记发布 ⑤待补录的反馈。五项 MUST 同时可见，MUST NOT 默认折叠其中任何一项，**MUST NOT 因为某项没有数据源就把它从页面上去掉**。
- **FR-005a**: 「账号配置缺项」MUST 作为**第六个附加区块**呈现，排在 §2 五项之后。它的标题 MUST 写其本义（账号配置缺项），MUST NOT 使用「待补录的反馈」或任何会让它被误读为 §2 第五项的标题。
- **FR-005b**: 「待补录的反馈」区块 MUST 列出**已发布、却一条人工指标都没录**的发布记录，数据源是 `feedback-learning` 的 `GET /api/content-feedback/pending`，派生口径用 `packages/core/content/feedback-learning` 的 `pendingFeedbackSummary`。每条 MUST 写明是哪一条（渠道 + 发布时间），MUST NOT 只显示一个数字。MUST NOT 隐藏该区块、MUST NOT 停在加载态、MUST NOT 显示任何伪造的反馈条目。

  **这个区块没有「到期」的概念**：判定只有「发了」和「一条都没录」两件事，不含任何时间比较。SOP §3.2 的品牌级「反馈观察时点」尚未落地（属 §3.2 那张卡），所以「过没过」今天无从判断；写一个默认天数等于发明一条 SOP 没说的规则，摆在运营者看不见也改不了的地方。

  > **2026-09-20 更新（specs/027 实施 PR 1）**：本条从「暂不可用（反馈记录接入后）」改写而来。原文保留在此供对照：`feedback-learning` 未落地期间，该区块显示暂不可用并说明原因——不隐藏、不留白、不伪造，与 024 三个 AI 入口同一口径。
- **FR-006**: 「需要处理的审核」MUST 列出状态为 `pending` 与 `changes_requested` 的审核请求。`approved` / `rejected` / `cancelled` 不列。
- **FR-007**: 「到期交接与待登记发布」MUST 使用 `content-deliveries` 在读时派生的 `due` 与 `pending_registration`，MUST NOT 自行重算到期规则，也 MUST NOT 把「已交接」当成「已发布」。
- **FR-008**: 「正在推进的作品」MUST 列出至少有一份文档处于 `working` 的作品，并显示是哪一份文档。
- **FR-009**: 「账号配置缺项」MUST 列出 `readiness.can_start = false` 的账号，并**逐项列出** `readiness.missing[]`，使用与账号页相同的字段名。
- **FR-010**: 「值得写的选题」MUST 只列出状态为 `draft` 的选题卡（裁决 Q2 = A）。`started` / `saved` / `deferred` / `dropped` MUST NOT 列出——`deferred` 首版明确不列。本卡 MUST NOT 修改 `topic-planning` 的 `Status` 枚举。
- **FR-010a**: 每张卡 MUST 显示其**理由摘要**与**预计投入**（SOP §11）。预计投入取该卡所关联账号的每周可投入时间。
- **FR-010b**: 当账号的每周可投入时间处于「待补充」状态时，预计投入 MUST 显示为「未确认」，MUST NOT 把一个未确认的数字呈现为确定值。卡片没有关联账号时，预计投入 MUST 显示为「未关联账号」。
- **FR-011**: 「值得写的选题」区块 MUST 有一条恒定可见的说明，写明「候选自动生成暂不可用（EP-04c 未落地）」，并说明现在列出的是人工创建的 `draft` 卡。MUST NOT 出现任何伪造的推荐条目。
- **FR-011a**: EP-04c 落地时，`TopicCard` **MUST 带来源字段**（人工创建 / 生成），本区块才允许显示生成的卡。在该字段存在之前，本区块 MUST NOT 声称自己做了「只展示人工创建」的筛选——它今天只是恰好成立（见 Assumptions 3）。

**每条的可读性**

- **FR-012**: 每个区块的每一条 MUST 能被识别到具体对象（标题 / 账号名 / 渠道 + 文档名），MUST NOT 只显示一个计数。
- **FR-013**: 每一条 MUST 写明它被列出的理由，且理由 MUST 与该对象在其原页面上的状态一致。
- **FR-014**: 每一条 MUST 可跳转到该对象原本所属的页面，**该页面已落地时**（例外见 FR-022a）。工作台上 MUST NOT 提供就地改状态的操作。

**失败与空态**

- **FR-015**: 每个区块 MUST 独立加载、独立失败。任一区块失败 MUST NOT 让其余区块无法显示。
- **FR-016**: 每个区块为空时 MUST 显示明确空态，MUST NOT 隐藏区块，MUST NOT 停在加载态。
- **FR-017**: 单条记录读取失败时 MUST 在其所在区块内标注失败，MUST NOT 静默从列表中消失（否则「今天没有事」和「读不到」看起来一样）。

**边界与授权**

- **FR-018**: 所有读取 MUST 限定在当前工作区。越权访问 MUST 返回与「不存在」同形的拒绝（404），MUST NOT 透露对象存在。
- **FR-019**: 本卡 MUST NOT 新增任何端点（FR-021），因此**工作流第 12 步在本卡没有适用对象**——这一条在实施 PR 里应当明确记为「不适用，因为没有新增带路径参数的端点」，而不是默默跳过。若裁决之后改为新增端点，第 12 步重新适用。
- **FR-020**: 工作台 MUST NOT 引入实时推送；数据新鲜度由页面自身的读取与用户可见的刷新决定。

**归属与路由**

- **FR-021**: 首版 MUST NOT 新建内容模块，MUST NOT 新增服务端端点（裁决 Q1 = B）：工作台由 web 适配器页组合各模块**已有的**读端点。MUST 通过 `pnpm check:content-boundaries`，MUST NOT 修改 `scripts/content-boundaries.json` 的依赖表。
- **FR-021a**: 所有**派生口径** MUST 实现为 `packages/core` 的纯函数，页面**只负责渲染**。至少包括：什么算「待处理审核」、什么算「到期」、什么算「待登记」、什么算「就绪缺项」、以及每个区块内的**排序**。页面 MUST NOT 自行内联这些判定——否则桌面端将来复用的是页面而不是 core。
- **FR-021b**: 每个区块 MUST 最多显示 10 条。超出时 MUST 显示还有多少条，并提供跳转到该对象原页面查看全部的入口。计数 MUST 与列表同源（「上限内 N 条 / 共 M 条」），MUST NOT 另算一份。
- **FR-022**: 页面路由 MUST 是 `/{workspaceSlug}/today`（裁决 Q3 = C）。本卡 MUST NOT 把它设为工作区默认落地页，MUST NOT 改动现有进入工作区的导航行为；是否设为默认落地页由主任务另行决定。
- **FR-022a**: 审核区块与到期交接区块的条目在 `review-delivery` 页面落地前 MUST 不可点，并 MUST 写明原因（对应页面尚未落地）。MUST NOT 跳到一个不存在的路由。
- **FR-023**: 页面 MUST 只挂既有组件（宪法 VII），MUST NOT 新增控件或新依赖，MUST 四语言（en / zh-Hans / ja / ko）。
- **FR-024**: MUST NOT 写 UI 单测（宪法 II）；界面项进 `manual-ui-todo.md`。非 UI 的聚合与判定逻辑 MUST 有 node 测试。

### Key Entities

工作台**不定义新实体**。它呈现的是已有对象的一个视图：

- **选题卡**（`topic-planning`）：`status`、`account_id`、`started_brief_revision_id`。
- **作品 / 文档**（`work-editor`）：文档的 `draft_status`（`working` / `saved`）、最近版本。
- **审核请求**（`review-delivery`）：`status`。
- **交付待办**（`review-delivery`）：`status`、`scheduled_at`，以及**读时派生**的 `due`、`pending_registration`——这两个不存库，按 SOP 9.2「不推断平台状态」。
- **账号与表达配置**（`ip-profile`）：`readiness{can_start, missing[]}`，以及每周可投入时间（带「已确认 / 待补充」状态）——它是 §11「预计投入」今天唯一的来源。
- **反馈记录**（`feedback-learning`）：~~**尚不存在**~~ **已落地**（specs/027 PR 1，2026-09-20）：人工指标 `content_manual_metric` 与反馈摘录 `content_feedback_excerpt`，第五项读 `GET /api/content-feedback/pending`。

唯一新增的概念是**「工作台读模型」**：把上述对象按六个区块组织起来的一个纯读结果。按裁决 Q1 = B，它**首版不是服务端对象**——它是 `packages/core` 里的一组纯函数（FR-021a），加上一个只做渲染的页面。

---

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: 运营者在**一个页面**上就能判断今天有没有事要做，不需要打开其他页面——§2 的五个区块加第六个附加区块全部可见。
- **SC-002**: 任意一个区块的数据源不可用时，其余区块仍然可用（逐个模拟失败验证，有数据源的 5 个区块 5 次中 5 次）。
- **SC-003**: 工作台列出的每一条，其状态与理由都与该对象在原页面上显示的一致（逐条人工核对，0 处不一致）。
- **SC-004**: 工作台不产生任何写操作——整页操作一遍后，五类对象的数据与操作前逐字节相同（除跳转外无副作用）。
- **SC-005**: 选题区块的「候选自动生成暂不可用」恒定可见并写明原因（0 条伪造推荐）。

  > **2026-09-20 更新（specs/027 实施 PR 1）**：原本是**两处**暂不可用，第二处是「待补录的反馈」整块。`feedback-learning` 落地后它有了真实数据源，只剩选题区块这一处。0 条伪造反馈这一条仍然成立，但它现在由「区块读真实端点」保证，不再由「区块显示暂不可用」保证。
- **SC-007**: 所有派生口径（待处理审核 / 到期 / 待登记 / 就绪缺项 / 区块排序）都有 `packages/core` 的 node 测试覆盖；页面内 0 处内联判定（可通过读代码核对）。
- **SC-008**: 任一区块条目超过 10 条时，显示的是「10 条 + 还有 N 条」而不是全部或截断无提示。
- **SC-006**: 空工作区打开工作台：**六个区块六个空态**，0 个加载态残留，0 个报错。第五项的空态是「没有待补录的反馈」——**那是一个结果，不是一句抱歉**。

  > **2026-09-20 更新（specs/027 实施 PR 1）**：原本是「5 个空态 + 第五项暂不可用」。

---

## Assumptions

以下是本规格在派单与裁决未明确处所做的默认判断。**每条都可被主任务推翻。**

1. **读取时机**：工作台在打开时读取一次，用户可手动刷新。不做轮询、不做 WebSocket 推送（FR-020）。理由是六类数据都不是秒级变化的，而推送会把工作台变成一个需要维护缓存一致性的东西。
2. **N+1 与上限（已裁决）**：区块 1、2、6 今天都需要 1+N 次请求（缺口 B、C、D）。裁决接受 N+1，每区块上限 10 条（FR-021b）。区块 1 的账号 profile 与区块 6 的是同一批数据，**应当复用同一次读取**，不要各取一遍。
3. **「人工创建」今天无法验证**：`TopicCard` 没有来源字段（缺口 A）。EP-04c 落地前全部卡都是人工创建，所以这个筛选**恒真但无从施加**。FR-011a 要求 EP-04c 落地时补上来源字段。
4. **不含桌面端**：与 `/topics`、`/accounts`、`/diagnostics` 一致，首版 web-only，桌面端不接线、侧边栏入口由主任务另行决定。**这正是 FR-021a 要求派生口径放 core 的原因**——桌面端将来复用的应当是 core 的纯函数，不是这个页面。
5. **审核区块不区分「我要批的」和「别人要批的」**：`ReviewRequest` 有 `requested_by` 与 `decided_by`，但**没有「指派给谁审」的字段**，所以「等我批的」今天无法判定；首版列的是工作区内全部待处理审核。
6. **审核与交付两个区块暂时无处可跳**：`review-delivery` 有 server + core，**没有 views**（Current State）。首版这两个区块的条目不可点并写明原因（FR-022a），等 025 的页面 PR 落地后再接上。**这是一个真实的功能缺口，不是遗漏。**
7. **「待补录的反馈」整项没有数据源**：`feedback-learning` 模块不存在（缺口 E）。区块存在但显示暂不可用（FR-005b）。
8. **将来的服务端聚合归 `project-collab`**：若后续规模或桌面端需求让 N+1 不再可接受，服务端聚合端点的归属是 `project-collab`——docs/12 §2 给它的职责是「组织已有业务对象，不直接修改其正文或批准状态」，正合一个只读工作台读模型。届时走 Q1 的选项 A，**登记表不需要修改**（`project-collab` 已经依赖 `topic-planning` / `work-editor` / `review-delivery` / `workspace-core` / `diagnostics`）。
9. **计数与列表同源**：区块标题上的计数就是列表的条数（或「上限内 N 条 / 共 M 条」），不单独算一份。

## 修订

- **[修订 B](./revision-b.md)**（Issue #188）：接入 source-inbox 的快速采集入口与「待整理」区块（§11「随时」行）、侧栏入口与默认落地页方案。其中两条**影响本文件的现有条款**：
  - **FR-005a** 的「第六个附加区块」措辞：若修订 B 的 Q1 裁为 A，附加区块从一个变两个，此处要同步。
  - **FR-005b 已过时**：`feedback-learning` 已于 #177 / #184 落地，第五区块不应再恒显「暂不可用」。修订 B 记录了这一点但**不在那一卡修**，属 027 的工作台接入卡。
  - 修订 B 还报告了一个**已合入的缺陷**：本卡交付的今日工作台上 12 处跳转用 workspace id 拼路径，而路由段是 slug，全部落空（详见 revision-b.md §0）。

## 裁决记录（主控，2026-09-21，PR #158 评论）

三个 clarify 均已裁决，规格已按裁决回写。

| 问题 | 裁决 | 落到规格哪里 |
|---|---|---|
| **Q1** 读模型归属 | **B**：不新建模块、不新增端点，由 web 适配器组合已有读端点。**但派生口径必须放 `packages/core` 纯函数，页面只渲染**；N+1 接受，每区块上限 10 条。将来若需服务端聚合，归属 `project-collab`，走选项 A，不改登记表 | FR-021、FR-021a、FR-021b、Assumptions 2 / 4 / 8 |
| **Q2** 选题状态口径 | **A**：只列 `draft`，`deferred` 首版不列，不改 022 枚举。「只展示人工创建」在 EP-04c 前恒真 → 写成 Assumption + 一条 FR：EP-04c 落地时 `TopicCard` 必须带来源字段 | FR-010、FR-011、FR-011a、Assumptions 3 |
| **Q3** 路由 | **C**：先 `/{workspaceSlug}/today`，默认落地页另议 | FR-022 |

裁决同时纠正了两件事，已写进规格：

1. **Issue #155 的「待补充的经营底座」是转述有误**。SOP §2 原文第五项是**「待补录的反馈」**。规格现按原文五项编排，「账号配置缺项」降为第六个附加区块，标题按本义写，不冒充第五项（FR-005、FR-005a）。
2. **§11 要求选题区块显示「选题理由、预计投入」**。原规格漏了「预计投入」，已补 FR-010a / FR-010b，并记录缺口 D：它今天只能取账号的每周可投入时间，而该字段可能处于「待补充」状态，此时必须显示「未确认」。

`plan.md` / `tasks.md` 按裁决随实施 PR 一并产出（本卡不新增表、不新增端点，规模小）。
