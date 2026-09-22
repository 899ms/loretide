# Feature Specification: 选题卡引用素材条目（022 契约扩展）

**Feature Branch**: `claude/spec-030-topic-source-refs`

**Created**: 2026-09-20

**Updated**: 2026-09-21（按主控裁决回写：§5.2 原文替换转述、引用归属更正、字段改名、三条 clarify 全部落定）

**Status**: **无待裁决项**。裁决全文见 PR #207 评论。

**Input**: Issue #204 + 主控裁决（PR #207 评论，2026-09-21，含 §5.2 原文）。把 §4 的素材收件箱（028 / #176 / #182）与 §5.2 的选题卡（022 / #107 / #119 / #133）连起来。028 的裁决 **Q2 = A** 把这件事单独留成一张卡，这就是那张卡。

**模块**：`topic-planning`（写方）+ `source-inbox`（被引方）。登记表 `scripts/content-boundaries.json` **一个字不改**——理由见 Current State 第 3 节。

---

## 原文依据

### §5.2「一张选题卡必须说清楚」（原文，裁决贴回）

> 一张选题卡必须说清楚：
>
> - 写给谁，解决什么具体问题，拟表达什么判断。
> - **为什么适合这个 IP，引用哪些素材和过去的经营结论。**
> - 为什么是现在：时效窗口、内容计划或常青价值；没有时效依据就直接说明。
> - 与已有作品的关系，是新角度、更新还是高度重复。
> - **证据是否充分，存在什么缺口，需要多少研究或制作投入。**
> - 适合哪些渠道，有什么表现参考及不可比较因素。
> - 推荐动作：今天推进、补证后推进、加入储备、更新旧作、暂缓。
>
> 用户可以开始、收藏、暂缓、放弃或提出自定义选题。暂缓和放弃支持快捷原因及自由备注；这些是偏好信号，单次操作不永久改变 IP 定位。

### 一处更正：引用挂在第 2 项，不是第 4 项

**Issue #204 把引用挂在「与已有作品的关系」与「证据缺口」两项下，那是转述错误。** 原文摆在这里，三件事一目了然：

| 项 | 原文 | Go 字段 | 本卡 |
|---|---|---|---|
| 第 2 项 | 为什么适合这个 IP，**引用哪些素材**和过去的经营结论 | `IPFit` / `ip_fit` | **「引用哪些素材」五个字就在这一项里。这是本卡的主场。** |
| 第 4 项 | **与已有作品的关系**，是新角度、更新还是高度重复 | `ExistingContentRelation` | **指的是作品，不是素材。本卡一个字不碰。** |
| 第 5 项 | **证据是否充分，存在什么缺口**，需要多少研究或制作投入 | `EvidenceGapsAndInvestment` | 「证据」可以是素材，所以这一项也能引。 |

初稿按 Issue 的转述把引用挂在第 4、5 两项，**第 4 项是错的**。这一节记下更正，是因为它改的不只是字段名——它改的是「引用在回答哪个问题」。第 2 项问的是「这个选题为什么适合这个 IP」，答案里的素材是**立论依据**；第 5 项问的是「证据够不够、缺什么」，答案里的素材是**证据**。两者都不是「和已经发出去的东西什么关系」。

**「过去的经营结论」不做。** 第 2 项原文还有这半句，指的是 §10.3 的经营结论（Learning）。027 的 FR-023 明写「本卡 MUST NOT 产生『待采纳 / 已采纳经营结论』，也 MUST NOT 为它建表」，全仓至今没有这个实体。**本卡不为它留字段**——留一个永远是空的字段，和 023 的 `required_sources` 恒空是同一种谎，而那件事 023 是拿一条负例挡住的，不是拿一个空字段圆过去的。裁决已确认。

### §4 步骤 4 与 R-011（原文，来源 `specs/028-source-inbox-manual/spec.md:27,38`，028 裁决贴回）

| 步骤 | 人做什么 | 系统做什么 | 产出 |
|---|---|---|---|
| 4 归类与判断 | 采纳分类，**说明为何收藏** | 建议摘要、实体、观点、问题与**关联资料** | **原文、自动摘要和个人判断分开保存** |

> **R-011**：收件箱支持批量标签、归档、**关联已有来源**、筛选和去重提示；内容相同不删除独立的收藏上下文与批注。

028 明写「**关联已有来源**（R-011 的第三项）**本卡不做**」（`specs/028/manual-ui-todo.md:68`）。本卡做的是**选题卡 → 素材条目**这一个方向；素材条目之间互相关联仍然不做。

**注意 §4 步骤 4「系统做什么」一列里的「关联资料」是模型生成的**，028 的 FR-001 与 SC-004 把它钉死为 0 条。本卡做的是**人手选的**引用，与那一列无关——这一点必须在页面上分得开，否则 028 花了一整张卡守住的「0 条系统生成的关联资料」会被本卡的界面偷偷推翻。

---

## Current State（以代码为准，2026-09-20 于 `app-main` `10b7f75` 核实）

这一节每一条都给了文件与行号，在 `10b7f75` 上可复现。没核实的没有写进来。

### 1. 那两项今天是自由文本，没有任何结构

`server/internal/content/topic-planning/contract.go`：

```go
IPFit                     string `json:"ip_fit"`                        // 第 2 项，第 62 行
EvidenceGapsAndInvestment string `json:"evidence_gaps_and_investment"`  // 第 5 项，第 65 行
```

建表迁移 `server/migrations/483_content_topic_card.up.sql` 里也是两个 `text NOT NULL`。**没有任何位置可以放一个素材条目 id。**

（第 4 项 `ExistingContentRelation`（第 64 行）同样是自由文本，本卡**不动它**。）

### 2. **选题卡的七项今天根本改不了**（Issue 没提，这是本卡最要紧的一条）

`content_topic_card` 全仓只有**两条** UPDATE 语句，都在 `server/internal/content/topic-planning/store.go`：

| 行号 | 改哪些列 | 入口 |
|---|---|---|
| 278 | `account_id` | `SetAccount`（#133） |
| 415 | `status` / `decision_reason` / `decision_note` / `started_brief_revision_id` | 四个动作 |

`server/cmd/server/router.go:1890-1905` 的路由与之对齐：`POST /{id}/account`、`POST /{id}/actions`，**没有任何 PATCH 或 PUT 能改卡的正文七项**。

**后果**：引用做成卡上的列，今天唯一能写进去的时刻是**创建那一刻**。「给一张已经建好的卡补一条素材」需要一条**新的写入路径**——它不是本卡的可选项，是必需品。

`SetAccount` 的注释已经把这条新路径该长什么样写好了（`store.go:239-243`）：

> 它自己一个入口，而不是四个动作上的一个字段：那四个改的是卡的去向，这个改的是它写给谁，合在一起会让「收藏」能悄悄把一张卡改投给别的账号。

同一个道理对引用成立：`save` 不该顺手改掉这张卡引了哪些素材。**裁决已定：照 `SetAccount` 的形状单开一个端点 `POST /{id}/sources`。**

### 3. 登记表里 `topic-planning` 不依赖 `source-inbox`，而且不必改

`scripts/content-boundaries.json`：

```text
"source-inbox":    ["workspace-core", "diagnostics"]
"knowledge-base":  ["workspace-core", "source-inbox", "diagnostics"]
"topic-planning":  ["workspace-core", "ip-profile", "knowledge-base", "diagnostics"]
```

`topic-planning → knowledge-base → source-inbox` 是**二跳**，登记表不允许直接 import。

**但这不构成障碍**，已有两个先例：

- **#133 的账号校验**：`topic-planning` 声明了 `ip-profile`，所以 `store.go:26` 的 `AccountReader` 端口可以直接用 `ipprofile.Account` / `ipprofile.ErrNotFound`。
- **#197 的观察时点**（上周刚落地）：`feedback-learning` 的 `Observation` 端口由 handler 适配器回答。

本卡与 #197 同形但更严：端口签名**只用字符串**，不出现任何 `sourceinbox` 类型，于是 `topic-planning` 对 `source-inbox` **零 import**，登记表 `modules` 不动。

`adapters` 也不动——`server/internal/handler/content_topic.go` **已经在册**（登记表 `adapters` 第 7 行）。

### 4. #133 的校验先例：端口 + 适配器 + **栅栏事务内**校验

`server/internal/content/topic-planning/store.go:218-237`：

```go
// checkAccount refuses an account that is not this brand's.
// ... A foreign or missing account is ErrNotFound, the same answer a foreign
// card gets, so a refusal cannot be used to find out which account ids exist.
```

`SetAccount` 的注释（`store.go:245-247`）写明了为什么校验在事务里：

> 整件事跑在工作区删除栅栏里，品牌校验也在内：**在栅栏外答出来的校验，可能在工作区删除之前答完、在删除之后才把写入应用上去。**

**本卡的素材校验照抄这三条**：端口注入、栅栏事务内、外来或不存在一律 `ErrNotFound`。

### 5. source-inbox 的条目 id 稳定、状态三态、**没有删除路径**

- `server/migrations/517_content_source_id_unique_idx.up.sql`：`source_id` 唯一索引。
- `server/internal/content/source-inbox/contract.go:81-86`：`inbox` / `organized` / `archived`。
- 全仓 `DELETE FROM content_source` **只有一处**：`server/pkg/db/queries/workspace_delete.sql:399`，即品牌删除链。

**所以一条引用一旦建立，被引方不会消失，只会换状态。** `archived` 是「归档」，不是「删除」——一条引用不因为对方被归档而失效，页面要显示它的状态而不是把它丢掉。这是本卡的一个陷阱位：把「归档」当「没了」，卡上的引用会无声地少掉一条。

### 6. 023 的 `required_sources` 恒空，而且有一条按字段分列的负例钉着

`server/internal/content/topic-planning/snapshot.go:86-89` 写空，`snapshot_test.go:90` 的 `TestNoSourcelessFieldIsInvented` 逐字段断言，`required_sources` 那一行的归属写的是 `EP-04d / W-03`。失败信息是：

> 填它必须是对这条规则的**刻意修改**，不是一个安静的默认值。

同一个文件还给了**另一条出路**（`snapshot.go:45-57`）：

```go
// StoredSnapshot ... The extensions are declared rather than squeezed into one
// of the sixteen. Each of those sixteen already means something else, and
// borrowing one would make "aligned with diagnostics.Snapshot" a half-truth.
AutoPrecheck          bool `json:"auto_precheck"`
UsesNeutralExpression bool `json:"uses_neutral_expression"`
```

**这段注释就是 Q3 裁决的依据**：要冻结的东西与那十六项里任何一项含义不同，它就该是第三个扩展字段，而不是借用 `required_sources`。

### 7. 022 的 A6 守卫管的是简报表，**本卡碰不到它**

守卫在 `server/internal/content/topic-planning/store_integration_test.go:468` `TestBriefStoreHasNoUpdateOrDeletePath`，扫 `store.go` 里是否出现 `UPDATE CONTENT_BRIEF_REVISION` / `DELETE FROM CONTENT_BRIEF_REVISION`，并要求 `INSERT` 存在。

**它只针对简报版本表。** 选题卡本来就是可改的（第 2 节的两条 UPDATE 就在那儿），所以：

- 引用放在**卡**上 → A6 无关；
- 引用放进**简报版本**上 → 要给 append-only 表开一条 UPDATE，**A6 当场红**，023 已经为同样的理由拒绝过一次（`specs/023/contracts/start-snapshot.md:25`：「那条守卫不容破例」）；
- 引用**冻进快照**（INSERT 一行新快照）→ A6 无关，快照表也是只插。

### 8. 页面两边都已经有了地方挂

- `packages/views/content/topic-planning/index.tsx`（1105 行）：列表 + 新建表单 + 详情面板的主从布局，详情走 `useContentTopic(wsId, topicCardId)`。
- `packages/views/content/source-inbox/index.tsx`（702 行）：收件箱，`useContentSources(wsId, status, tag)` 已经**带状态筛选**（`packages/core/content/source-inbox/queries.ts:29`）。

**选条目的下拉不必新造数据源**：`useContentSources(wsId, "inbox")` 与 `useContentSources(wsId, "organized")` 今天就能用。

### 9. 卡上的集合列已有形状：`channels` 是 `jsonb`，不是 `text[]`

`483_content_topic_card.up.sql`：`channels jsonb NOT NULL`。`store.go:57-74` 的 `encodeStrings` / `decodeStrings` 是 `json.Marshal` / `json.Unmarshal` 一对。

**注意 `normalizeStrings`（`store.go:57`）只把 nil 变成 `[]`，不去重、不去空、不截断。** 引用列表如果照抄它，「同一条素材被加两次」与「一条空字符串 id」都会存进去。本卡给引用自己的归一化规则（FR-008）。

同一张表里 `jsonb` 是本地先例（裁决据此选了 jsonb 数组）；`content_source.tags` 用的是 `text[]`，是另一张表的先例。

### 10. 迁移与既有规则

- 下一个可用号 **532**（现有最大 531，`server/migrations/`）。本卡用 **532 / 533** 两个加列迁移。
- R1–R6 与工作区删除清单（`server/internal/handler/workspace_delete_manifest_test.go:66` 已有 `content_topic_card`）。**加列不动清单。**
- 栅栏：写事务第一条语句取 `LockForContentDiagnosticWrite`。
- **第 12 步**：`POST /{id}/sources` 带路径参数，必须有一条穿过真实 router 与中间件、且**路径参数值与上下文里的工作区 id 不同**的用例，取参数只用 `chi.URLParam`。

---

## 裁决记录（主控 2026-09-21，PR #207 评论）

| 问题 | 裁决 | 与我的推荐 |
|---|---|---|
| **Q1 存储形态** | **B（两列），但按原文改名与归属**：`fit_source_ids`（第 2 项「引用哪些素材」）、`evidence_source_ids`（第 5 项「证据…缺口」）。两个加列迁移，jsonb 数组，无外键。不做反向显示，故不选 C。「过去的经营结论」未落地，不留字段 | 采纳 B，**归属更正**——我按 Issue 的转述挂在第 4、5 项，第 4 项是错的 |
| **Q2 校验** | **A**：只用字符串的端口 + handler 适配器，栅栏事务内校验，外来 / 不存在 404 同形（照 #133）。附带条**接受**：`archived` 可被引用、选择器默认不列 | 与推荐一致 |
| **Q3 冻结** | **B**：`topic-planning` 扩展字段进 `StoredSnapshot`，**023 负例一行不改** | 与推荐一致 |
| **新写入路径** | US2 定 P1 **接受**；照 `SetAccount` 单开端点 **`POST /{id}/sources`**，不挂四个动作上；其余五项仍不可编辑，记 Out of Scope | 与推荐一致 |
| **反向显示** | **不做**，写 Out of Scope | 与推荐一致 |

---

## User Scenarios & Testing *(mandatory)*

### User Story 1 - 说清楚这个选题为什么适合这个 IP，并把依据挂上去 (Priority: P1)

运营者要写一张选题卡。「为什么适合这个 IP」这一栏，他想说的不只是一段话——他上周在收件箱里存了三条同题材的材料，**那三条就是他这么判断的依据**。他在这一栏下面选中它们，卡建出来之后，谁打开这张卡都能直接点进那三条素材。

**Why this priority**: 这是 §5.2 第 2 项原文里的「引用哪些素材」五个字，是整张卡的理由。不做这一个故事，收件箱与选题卡仍然是两个互不知道对方存在的抽屉。单做它已经有用：一张说得出立论依据的选题卡。

**Independent Test**: 建一张卡时在「为什么适合这个 IP」下挂两条素材，读回来这两条还在、顺序不变、标题能显示。「证据缺口」留空不影响保存。

**Acceptance Scenarios**:

1. **Given** 收件箱里有三条本品牌的素材，**When** 新建选题卡并在「为什么适合这个 IP」下选中其中两条，**Then** 卡创建成功，详情里列出那两条的标题与状态。
2. **Given** 一张卡两栏都没挂任何素材，**When** 保存，**Then** 成功——**引用不是必填**，§5.2 没说它是。
3. **Given** 一条素材被选了两次，**When** 保存，**Then** 存下来的是一条，不是两条。
4. **Given** 一张卡，**When** 看「与已有作品的关系」那一栏，**Then** 它**没有**素材选择器——那一项问的是与作品的关系，不是素材（原文第 4 项）。

### User Story 2 - 给一张已经建好的卡补一条素材 (Priority: P1)

卡是三天前建的，今天他才收到那份关键的材料。他打开卡，在「证据是否充分、存在什么缺口」下把这条新素材加进去。**卡的状态、它关联的账号、它的决定原因，一个都没动。**

**Why this priority**: 与 US1 同为 P1，因为 Current State 第 2 节的发现：**今天这张卡建完就改不了了**。没有这个故事，引用只能在创建那一秒填对，这在实际使用里等于做不了。

**Independent Test**: 对一张已存在的卡调用 `POST /{id}/sources`，只有引用变了；再读一次卡，`status` / `account_id` / `decision_reason` / 七项正文与之前逐字节相同。

**Acceptance Scenarios**:

1. **Given** 一张 `status=draft`、已关联账号 X 的卡，**When** 给它加一条素材引用，**Then** 引用生效，且 `status` 仍是 `draft`、账号仍是 X。
2. **Given** 一张卡的某一栏引了三条，**When** 提交一个只含其中两条的新列表，**Then** 第三条被移除——这个入口是**整列替换**，与 `SetAccount` 同形。
3. **Given** 四个动作之一（开始 / 收藏 / 暂缓 / 放弃），**When** 执行它，**Then** 卡的引用**一条不变**——动作不碰引用，引用入口不碰状态。
4. **Given** 一次请求只给了 `fit_source_ids`，**When** 提交，**Then** `evidence_source_ids` **不变**；反之亦然。两栏各自独立替换。

### User Story 3 - 引用不会指向别的品牌，也不会指向不存在的东西 (Priority: P1)

他在 A 品牌的卡上，不可能挂上 B 品牌的素材；手打一个不存在的 id 也不行。系统的回答和「这张卡不是你的」**是同一句话**，不会因此泄露哪些 id 存在。

**Why this priority**: P1 而不是 P2，因为这是一条正确性要求而不是体验要求。一条没校验过的引用最终会在某个读路径上被解析成标题，那里漏一次品牌过滤就是跨品牌读取。

**Independent Test**: 用 B 品牌的 `source_id` 去写 A 品牌的卡 → 404；用一个随机字符串 → 404；两个 404 的响应体**无法区分**。

**Acceptance Scenarios**:

1. **Given** B 品牌的一条素材，**When** 在 A 品牌的卡上引用它，**Then** 404，且**卡没有被部分写入**（栅栏事务内，全有或全无）。
2. **Given** 一个不存在的 id，**When** 引用它，**Then** 404，响应体与上一条**逐字节相同**。
3. **Given** 一次提交里两条有效、一条外来，**When** 提交，**Then** 整次拒绝，**有效的那两条也没写进去**。
4. **Given** 一次提交里 `fit_source_ids` 全部合法、`evidence_source_ids` 含一条外来，**When** 提交，**Then** 整次拒绝，**两栏都没写**。

### User Story 4 - 归档的素材还在卡上 (Priority: P2)

他三个月前归档了一条素材。今天打开当初引用它的那张卡，**那条引用还在**，标着「已归档」。他一眼看出这条依据老了，但不会以为自己从没引过它。

**Why this priority**: P2，因为它不阻塞前三个故事，但不做就是一个静默的数据丢失——而且是最难发现的那种：卡上少了一条，没有任何地方会说少了什么。

**Independent Test**: 引用一条素材 → 把它改成 `archived` → 重新读卡，引用仍在，状态显示为「已归档」。

**Acceptance Scenarios**:

1. **Given** 一张卡引了一条 `organized` 的素材，**When** 那条素材被归档，**Then** 卡上的引用**仍在**，并显示「已归档」。
2. **Given** 一条已归档的素材，**When** 打开选择器，**Then** 它**默认不出现在候选里**（选择器列的是还在用的）。
3. **Given** 一条已归档素材的 id，**When** 直接提交给端点，**Then** **接受**——归档不是删除，校验只问「是不是本品牌的、在不在」。
4. **Given** 品牌被删除，**When** 删除链跑完，**Then** 卡与素材一起消失——没有孤儿引用，因为没有孤儿卡。

### User Story 5 - 冻结的简报说得出它当时引了什么 (Priority: P3)

他上个月「开始」了一张卡，冻了一份简报。今天卡上的引用已经换过两轮。他打开当时那份简报的输入快照，**看到的是当月那三条，不是今天这五条**。

**Why this priority**: P3，因为前四个故事不依赖它。但 Q3 已裁决为 B，所以本卡要做。

**Independent Test**: 引用三条 → 开始 → 改成五条 → 读那次开始的快照，里面是三条。

**Acceptance Scenarios**:

1. **Given** 卡上两栏共引了三条，**When** 执行「开始」，**Then** 这次的输入快照按**两栏分别**记下那三条。
2. **Given** 快照已冻结，**When** 卡上的引用改成五条，**Then** 快照里**仍是三条**。
3. **Given** 一张一条都没引的卡，**When** 开始，**Then** 快照里那两项是**空数组，不是 null**（照 `snapshot.go:86-89` 既有口径）。
4. **Given** 任意一次开始，**When** 读快照的 `required_sources`，**Then** 它**仍然是空数组**——本卡不碰它（FR-016）。

### Edge Cases

- **一次提交 200 条引用**：要有上限，否则一张卡可以把整个收件箱挂上去，详情页变成一页 id。
- **空字符串 id / 只有空白的 id**：不是「没引用」，是一条坏数据。归一化时丢掉（FR-008），不是存进去。
- **同一条素材在两栏都被引**：合法。它既是「为什么适合这个 IP」的依据，也是「证据」，这是两栏各自的答案，不是重复。
- **卡被「放弃」之后还能不能改引用**：能。放弃是决定，不是锁；`SetAccount` 今天也不看状态。
- **素材条目被编辑（`organize`）改了标题**：卡上显示的是**当前标题**，不是引用时的标题——引用存的是 id，不是快照。快照那一份由 US5 负责。
- **旧客户端**：装好的桌面端会拿到两个它不认识的字段；反过来，新页面对着旧后端会拿不到这两个字段，**必须读成「没有引用」而不是报错，更不能读成「有但空」**（宪法 VI）。
- **请求里只给了一栏**：另一栏保持不变，不是被清空（US2 场景 4）。这是「缺省 ≠ 清空」，与本仓反复出现的「未设 ≠ 零值」同一族。

---

## Requirements *(mandatory)*

### Functional Requirements

**存储与契约**

- **FR-001**: 选题卡 MUST 能结构化地记录它引用了哪些素材条目，引用以 `source_id` 为键，分成**两列**：`fit_source_ids`（§5.2 第 2 项「为什么适合这个 IP，**引用哪些素材**」）与 `evidence_source_ids`（§5.2 第 5 项「**证据是否充分，存在什么缺口**」）。两列都是 `jsonb` 数组，照同表 `channels` 的既有形状（Current State 第 9 节）。
- **FR-001a**: 本卡 MUST NOT 给 §5.2 第 4 项「与已有作品的关系」（`existing_content_relation`）增加任何引用字段——那一项问的是与**作品**的关系，不是素材。
- **FR-001b**: 本卡 MUST NOT 为 §5.2 第 2 项里的「过去的经营结论」留字段。§10.3 的经营结论未落地（027 的 FR-023 明写不建表），一个恒空的字段是一条没人做过的声明。
- **FR-002**: 引用 MUST NOT 使用外键（R1）；MUST NOT 级联（R2）。跨表关系与清理在应用层显式处理。
- **FR-003**: 迁移 MUST 是 **532** 与 **533** 两个加列文件，各一列。加列 MUST NOT 改 `workspace_delete_manifest_test.go`（卡本身已在清单里），MUST NOT 新增索引，因此 R3 / R4 / R5 / R6 在本卡**无新增对象需要遵守**——这一条 MUST 在交付时逐句复核，不是默认成立。
- **FR-004**: 本卡 MUST NOT 修改 `content_brief_revision` 的任何列，MUST NOT 给它增加 UPDATE 路径——022 的 A6 守卫（`store_integration_test.go:468`）MUST 保持绿，且**不被改写**。
- **FR-005**: §5.2 其余五项与四个动作的行为 MUST 一字不变。本卡 MUST NOT 改 `status` 受控集、MUST NOT 改 `decision_reason` / `decision_note` 的语义。

**写入路径**

- **FR-006**: MUST 提供 **`POST /api/content-topics/{id}/sources`**，形状照 `SetAccount`（`store.go:239`）：自己的端点、整列替换、不碰状态与账号。MUST NOT 把引用做成四个动作上的一个字段。
- **FR-006a**: 请求体的两个键 MUST 各自可缺省。**缺省 = 不改这一栏**，MUST NOT 读作「清空这一栏」；清空 MUST 由显式的空数组表达。
- **FR-007**: 该入口 MUST 在工作区删除栅栏事务内执行，栅栏 MUST 是事务的第一条语句；校验 MUST 在同一事务内（Current State 第 4 节）。
- **FR-008**: 写入前 MUST 归一化每一栏：**去重**（保留首次出现的顺序）、**丢弃空白 id**、**长度上限**（**每栏 50 条**，超过即 400 并指名是哪一栏）。MUST NOT 直接复用 `normalizeStrings`（`store.go:57`）——它只把 nil 变 `[]`，三件事一件都不做。
- **FR-009**: 空列表与「没有这个字段」MUST 读作同一件事（「没引用」），并 MUST 以**空数组**而不是 null 写出与读回，照 `snapshot.go:86-89` 的既有口径。
- **FR-009a**: 创建选题卡（`POST /api/content-topics`）MUST 也接受这两个键，走**同一套**归一化与校验规则——US1 是建卡时就挂上。

**校验**

- **FR-010**: 每一个被引用的 `source_id` MUST 属于**同一个品牌**且 MUST 存在。校验由 `topic-planning` 的一个**只用字符串**的端口回答，适配器在 `handler/content_topic.go`（已在 `adapters` 登记表内）。
- **FR-011**: 该端口的签名 MUST NOT 出现任何 `source-inbox` 的类型，使 `topic-planning` 对 `source-inbox` 保持**零 import**，`scripts/content-boundaries.json` 的 `modules` MUST NOT 改动。
- **FR-012**: 外来品牌的 id 与不存在的 id MUST 得到**同一个** `ErrNotFound`（404），响应体 MUST 无法区分二者（照 `checkAccount` 的 `store.go:218-223`）。
- **FR-013**: 一次提交里只要有一条不合法，整次 MUST 拒绝；MUST NOT 部分写入，**两栏都不写**。
- **FR-014**: `archived` 的条目 MUST 可以被引用（校验只问品牌与存在性），已建立的引用 MUST NOT 因为对方被归档而失效或被移除（Current State 第 5 节）。

**冻结**

- **FR-015**: 执行「开始」时，MUST 把卡上当时的两栏引用冻进输入快照的**两个新的 topic-planning 扩展字段**，照 `StoredSnapshot` 既有的 `auto_precheck` / `uses_neutral_expression` 的做法（`snapshot.go:45-57`）。两栏 MUST 分开记录，MUST NOT 合并成一个列表。
- **FR-016**: MUST NOT 往 `Snapshot.required_sources` 或 `Snapshot.excluded_sources` 里写任何东西；023 的 `TestNoSourcelessFieldIsInvented`（`snapshot_test.go:90`）MUST 保持**一行不改且全绿**。
- **FR-017**: 快照冻结后，卡上引用的任何变化 MUST NOT 改变已冻结的快照。

**接口与前端**

- **FR-018**: 新端点 MUST 挂到 `server/cmd/server/router.go`，且 MUST 有一条**穿过真实 router 与中间件**的用例；路径参数值 MUST 与上下文里的工作区 id 不同；取参数 MUST 只用 `chi.URLParam`（工作流第 12 步）。
- **FR-019**: 响应 MUST 经 zod schema 与 `parseWithFallback` 解析，MUST NOT 强制转型（宪法 VI）。MUST 有一条**畸形响应用例**。
- **FR-020**: 缺失的 / 类型不对的引用字段 MUST 读作「没有引用」的**空数组**，MUST NOT 使整张卡或整个列表解析失败。
- **FR-021**: 选题卡新建表单与详情 MUST 在「为什么适合这个 IP」与「证据是否充分」**两栏各自**提供素材选择器，候选 MUST 只列**本品牌**的条目，且 MUST 默认排除 `archived`。
- **FR-022**: 详情里每条引用 MUST 显示它的**标题与整理状态**，MUST NOT 只显示一个 id。标题为空时 MUST 有一个说得出是哪条的回落，MUST NOT 显示空白行。
- **FR-023**: 界面 MUST 写明这些引用是**人手选的**，MUST NOT 与 §4 步骤 4「系统做什么」一列的「关联资料」混同；028 的 SC-004（0 条系统生成的关联资料）MUST 保持成立。
- **FR-024**: 本卡 MUST NOT 调用任何 AI 执行器，MUST NOT 产生任何由系统生成的关联建议（宪法 IX，与 028 FR-001 同口径）。

**诊断与权限**

- **FR-025**: 引用的写入与拒绝 MUST 走 `workspace-core.Authorize`，MUST 与既有卡端点同一口径；每次拒绝 MUST 记一条技术诊断事件。
- **FR-026**: 引用写入 MUST 在同一事务内写审计事件，口径照 `SetAccount` 的 `link-account`，动作名 `link-sources`。
- **FR-027**: `pnpm check:diagnostics-contract` MUST 保持绿；本卡不新增落地模块（`topic-planning` 与 `source-inbox` 都已落地，共八个）。

**不做**

- **FR-028**: 本卡 MUST NOT 做「这条素材被哪些选题卡引用」的反向视图。
- **FR-029**: 本卡 MUST NOT 做素材条目之间的互相关联（R-011 的「关联已有来源」，028 已明写不做）。
- **FR-030**: 本卡 MUST NOT 写 UI 单测，MUST NOT 用 computer use 做验收；UI 验收 MUST 是 `specs/030-topic-source-refs/manual-ui-todo.md` 里的手动清单（宪法 II）。
- **FR-031**: 本卡 MUST NOT 让 §5.2 其余五项变成可编辑的——那是另一张卡（Out of Scope 第 6 条）。

### Key Entities

- **选题卡的立论素材引用**（`fit_source_ids`）：§5.2 第 2 项的「引用哪些素材」。一张卡指向零到多条素材条目的有序、去重列表。
- **选题卡的证据素材引用**（`evidence_source_ids`）：§5.2 第 5 项「证据是否充分，存在什么缺口」的答案里那些素材。与上一项**分开存**：同一条素材可以只属于其中一个，也可以两个都属于。
- **素材条目**（`source-inbox` 拥有，本卡只读）：`source_id` 稳定，状态 `inbox` / `organized` / `archived`，**没有删除路径**（品牌删除除外）。
- **输入快照的引用扩展**：一次「开始」冻下来的两份引用列表。它们是 `StoredSnapshot` 的**第三、第四个扩展字段**，不是 `diagnostics.Snapshot` 十六项中的任何一项。

---

## Success Criteria *(mandatory)*

- **SC-001**: 一张选题卡能说出它在「为什么适合这个 IP」与「证据是否充分」两栏各引了哪几条素材，两份**分得开**；「与已有作品的关系」那一栏**没有**引用字段。
- **SC-002**: 一张已经建好的卡能补引用，补完之后它的状态、账号与七项正文**逐字节未变**。
- **SC-003**: 引用入口执行一次，卡的 `status` 与 `account_id` **零变化**；四个动作各执行一次，卡的引用**零变化**。两个方向各有一条用例。
- **SC-003a**: 请求只给一栏时，另一栏**零变化**；显式给空数组时，那一栏清空。两条各一个用例。
- **SC-004**: 跨品牌与不存在两种 id 各拒绝一次，两次响应体**逐字节相同**；拒绝后卡上**零部分写入**。
- **SC-005**: 一次提交含一条非法 id 时，同批的合法 id **0 条**被写入，**两栏都是 0 条**。
- **SC-006**: 同一条素材提交两次，存下来 **1 条**；空白 id 提交，存下来 **0 条**；51 条的提交被拒绝并**指名是哪一栏**。
- **SC-007**: 引用一条素材后将其归档，卡上的引用数**不变**，且该行显示「已归档」；把已归档 id 直接提交给端点，**接受**。
- **SC-008**: 022 的 A6 守卫（`TestBriefStoreHasNoUpdateOrDeletePath`）**一行未改且绿**；023 的 `TestNoSourcelessFieldIsInvented` **一行未改且绿**，`required_sources` 与 `excluded_sources` 仍是空。
- **SC-009**: 冻结后改卡上的引用，已冻结快照里的那两份**不变**；快照里两份**分开记录**。
- **SC-010**: `scripts/content-boundaries.json` 的 `modules` 与 `adapters` **均零改动**；`pnpm check:content-boundaries` 与 `pnpm check:diagnostics-contract` 全绿；落地模块数仍是 **8**。
- **SC-011**: `POST /{id}/sources` 在 `router.go` 里 `grep` 得到，且有一条路径参数值 ≠ 上下文工作区 id、穿过真实中间件的用例（工作流第 12 步）。
- **SC-012**: 畸形响应用例覆盖三种：字段缺失、类型不对、整条响应不是对象；三种都读成**空数组**，**0 次**解析异常，**0 次**整列表消失。
- **SC-013**: 页面上**0 条**由系统生成的关联建议；「人手选的」这句说明恒定可见（028 SC-004 仍成立）。
- **SC-014**: 两个加列迁移逐条复核 R1–R6 的适用性：**0 个** `REFERENCES` / `FOREIGN KEY`、**0 个** `CASCADE`、**0 个**新索引、`workspace_delete_manifest_test.go` **零改动**。up / down 各实测一次。
- **SC-015**: 手验清单 `manual-ui-todo.md` 覆盖两栏各自的选择器、第 4 项没有选择器、归档条目的显示、跨品牌不可选、四语言；全部条目状态为「未执行」，由主任务在浏览器逐条验收。

---

## Assumptions

- 品牌 = 工作区，与 LT-009 同一口径。
- §5.2 原文由裁决贴回（PR #207 评论），本规格的每一条要求都能指回其中一句；指不回去的写在「我补的地方」。
- `knowledge-base` 仍未落地，本卡**不经过它**——登记表声明的是允许方向，不是必须路径（022 的 Assumptions 已经这么记过一次）。
- §10.3 的经营结论（Learning）未落地，027 的 FR-023 明写不为它建表；本卡因此不为第 2 项原文里的「过去的经营结论」留位置。
- `agent-workflow` 仍未落地，所以「运行继续引用它开始时的那一份」在本卡仍然只能由**结构**保证（简报与快照都只插不改），**行为断言无处可写**，与 022 的 FR-018 同一口径。
- 素材条目**不会消失**（无删除路径），所以本卡不设计「引用指向的东西没了」这条路径。若将来 `source-inbox` 开出删除，那是那张卡要处理的事，本卡在这里留一句话，不留一段代码。
- 桌面端未接线（与 `/topics`、`/sources` 今天一致），本卡只做 Web。

---

## Out of Scope

1. **「这条素材被哪些选题卡引用」的反向视图**（裁决：不做）。
2. **素材条目之间的互相关联**（R-011 第三项）——028 已明写不做，本卡不捡。
3. **模型生成的关联资料建议**（§4 步骤 4「系统做什么」一列）——宪法 IX，028 FR-001 / SC-004。
4. **给简报版本加引用列**——要给 append-only 表开 UPDATE，022 的 A6 不容破例（023 已为同样理由拒绝过一次）。
5. **`Snapshot.required_sources` / `excluded_sources` 的真正填充**——那是 EP-04d / W-03 的活（`snapshot_test.go:102` 记的归属）。本卡按 Q3=B **不碰它们**。
6. **选题卡其余五项的编辑能力**。Current State 第 2 节发现七项今天都改不了；本卡只为引用开一条入口，**不顺手把另外五项也变成可编辑的**（宪法 VIII，裁决已确认）。这是一个真实的缺口，应当单独立卡。
7. **「过去的经营结论」的引用**（§5.2 第 2 项后半句）——§10.3 未落地，无物可引。
8. **桌面端接线**。

---

## 我补的地方（原文没有）

逐条列出，便于日后追溯。

1. **独立的引用写入入口 `POST /{id}/sources`**（FR-006）。SOP 不谈端点。依据是 Current State 第 2 节的代码事实加 `SetAccount` 注释里已经写下的理由。**裁决已接受。**
2. **每栏 50 条上限**（FR-008）。原文无据，是一个防止详情页变成 id 列表的工程值。
3. **「缺省 ≠ 清空」**（FR-006a）。原文无据。理由是本仓反复出现的同一族问题：把「没说」读成「设成空的」，不一致时没有任何东西会报警。
4. **归档的条目可引、已建立的引用不失效**（FR-014）。原文只说收件箱支持归档，没说归档对引用意味着什么。**裁决已接受。**
5. **冻结走扩展字段而不是 `required_sources`**（FR-015、FR-016）。依据是 `snapshot.go:45-57` 的注释与 023 的负例，不是 SOP。**裁决已接受。**
6. **界面要写明「人手选的」**（FR-023）。原文没这句；它是为了不让本卡的界面把 028 守住的「0 条系统生成的关联资料」在视觉上推翻。

---

## 后续切片：Issue #238 已建卡五项正文编辑（2026-09-22）

FR-031 与 Out of Scope 第 6 条在 030 的交付范围内仍然成立：030 没有把
引用端点顺手扩成整卡编辑。Issue #238 是该条明确要求的**另一张卡**，只交付
后端合同、存储、路由和定向测试；页面编辑控件另行立卡与人工验收。

- 专用入口是 `PATCH /api/content-topics/{id}/body`。它仅接受
  `audience_problem_judgment`、`ip_fit`、`timing`、`existing_content_relation`、
  `evidence_gaps_and_investment` 五个字符串键；返回更新后的完整选题卡。
- 每个键独立部分更新：省略保留原值，显式 `""` 清空。`null`、非字符串、未知键、
  空对象或多余 JSON 值一律 `400`，且不写卡或审计成功事件。
- `channels`、`recommended_action`、账号、两组引用、状态、
  `decision_reason`、`decision_note` 不属于该入口；简报版本与开始快照仍是
  append-only、不可由该入口变更。
- 写入和审计在同一个工作区删除栅栏事务内完成。存储层以单条条件 `UPDATE` 做部分
  更新，不能先读整卡再全量覆写，因此两个请求分别改不同字段不会互相覆盖。
- 同 030 的授权口径：路径卡 id 在 `chi.URLParam` 读取，工作区来自既有授权上下文；
  不同工作区或无权访问继续是既有的 `404` 形状。
