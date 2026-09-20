# Feature Specification: §3.3 导入少量历史资产——已发表作品的粘贴导入

**Feature Branch**: `claude/spec-031-historical-import`

**Created**: 2026-09-20

**Status**: Draft（含 6 个 clarify，推荐值已暂定，不阻塞）

**Input**: Issue #210。SOP §3.3 原文见下。

---

## 原文依据

SOP §3.3：

> 优先导入已认可作品、常用参考资料、典型用户问题及可用的真实历史反馈。系统保留它们的「历史导入」标识。历史内容不知道采用了哪个原稿版本时，创建「发布后快照」，不虚构版本链。没有历史数据也可开始。推荐依据来自 IP 方向与现有素材，并明确显示「暂无个人表现数据」。

这段话一共给了六件事，本卡逐条对照：

| 原文 | 落点 | 本卡是否新做 |
| --- | --- | --- |
| 已认可作品 | work-editor 作品 + 文档 + 一版正文；review-delivery 发布记录 | **是**（主体） |
| 常用参考资料、典型用户问题 | source-inbox 条目（`historical_import=true`） | **否，已落地**（见 Current State） |
| 可用的真实历史反馈 | feedback-learning 指标 / 摘录，绑到导入产生的发布记录 | **部分**：模块已落地，缺的是「有一条可绑的发布记录」 |
| 「历史导入」标识 | 素材侧已有；作品与发布记录侧**没有** | **是** |
| 「发布后快照」、不虚构版本链 | 今天无处安放（见 Current State 缺口 C） | **是** |
| 「暂无个人表现数据」 | 全仓零命中 | **是** |

---

## Current State（以代码为准，2026-09-20 / `app-main` @ `169fce2`）

### 024 work-editor：版本的来源与动作，调用方定不了

`server/internal/content/work-editor/contract.go`：

```go
var Sources = []Source{SourceGenerated, SourceEdited, SourceAdopted} // generated / edited / adopted
var Actions = []Action{ActionSaved, ActionRestored, ActionAdopted}   // saved / restored / adopted
```

`Source` 的注释写的是「**Exactly three, per SOP 7.1**」；`Action` 没有同样的声明。`server/internal/content/work-editor/contract_test.go:24` 把两组值逐字钉死。迁移 `500_content_artifact_version.up.sql` 第 31–32 行各有一个 `CHECK (... IN (...))` 兜底。

更要紧的是 `version.go` 的这条注释：

> The source and action are chosen by the code above, not by a caller.

`SaveVersion` / `RestoreVersion` / `AdoptVersion` 三个方法各自写死一对 `(source, action)`，`appendVersion` 是唯一写版本的路径。**请求体里没有、也不可能有 `action` 字段。** 所以「版本 `action=imported`」不是加一个枚举值就行，它要一个新的 store 方法、一条新的写路径，以及那条 `CHECK` 一起改——这是 Q1。

### 024 work-editor：作品必须挂在一张选题卡上

`store.go:173`：

```go
if workspaceID == "" || actor == "" || work.TopicCardID == "" {
    return Work{}, ErrInvalid
}
```

迁移 `494_content_work.up.sql` 的 `topic_card_id text NOT NULL`。同一张表里 `snapshot_id` 的注释是「`''` when the work was not started from a snapshot. **A real state, not a missing value**」——即这张表已经承认「某个来源位是空的」是正常状态，但 `topic_card_id` 没拿到这个待遇。

历史作品没有选题卡：它先有了正文，再有的一切。这是 Q3。

### 025 review-delivery：发布记录不需要交付任务，但需要一个链接

`publication.go` 的 `RecordRequest` 里 `DeliveryTaskID` 是普通字段，`Record` 不校验它非空；迁移 `513` 是 `delivery_task_id text NOT NULL DEFAULT ''`。**所以「无审核请求、无交付任务的发布记录」今天就写得进去**，不需要改 025 的存储。

`ValidatePublicationRecord`（`states.go:183`）对 `reported_published` 要求 `page_url_or_content_id` 非空。导入一条历史发布因此**必须**给出链接或平台内容 id，这不是本卡加的限制，是 025 已有的。

`Record` 会用 `s.Artifacts.ResolveVersion(ctx, wsID, artifactID, "")` 反查 `work_id`，artifact 不存在就是 `ErrNotFound`。**导入的顺序被这条锁死**：作品 → 文档 → 版本 → 发布记录。

`VersionMatch` 是 `matched / differs / unknown`，注释写明 `unknown` 来自 SOP 9.1 且「is not a failure and must not block anything」。历史导入天然是 `unknown`——**这正是「不虚构版本链」在 025 已有的表达**。

`Channel` 只有四个：`xiaohongshu / wechat_mp / douyin / shipinhao`，加一个是迁移。历史作品发在别处的，今天记不下来（见 Out of Scope）。

### 025 发布记录没有 `version_id`，而 027 靠两跳去反查

`PublicationRecord` 的字段里**没有 `version_id`**。027 的 `ManualMetric.VersionID` 注释：

> VersionID is resolved on READ, **two hops through the delivery task and the review request**. `""` means it could not be resolved - a history entry with no delivery task, for one - and that is never a reason to refuse the recording.

027 已经预见到了这种行——但它给出的答案是「答不出来，照记」。对本卡不够：§3.3 要的是**创建「发布后快照」**，也就是让「这条发布对应的是这份正文」有个明确的落点，而不是一个解析不出来的空串。这是 Q4。

### 027 feedback-learning：指标绑发布记录，已可用

`ManualMetric.PublicationRecordID` 必填，`Platform` 四个，`Metric` 十一个（无 `other`），`Value` 是 `*int64`——`nil` 是「平台不给我看」，指向 0 是「我确认过是 0」。`FeedbackExcerpt` 同样绑发布记录，把「别人说的」和「运营者的解读」分成两列。

**本卡一旦造出发布记录，历史反馈就有地方挂了，027 一行都不用改。**

`PendingRegistration` 是派生的，`Due` 是 specs/029 的三值答案。

### 028 source-inbox：`historical_import` 端到端已落地

`Source.HistoricalImport bool`，`NewSource.HistoricalImport` 可由调用方设；`packages/core/content/source-inbox/draft.ts` 有 `historicalImport`，`packages/views/content/source-inbox/index.tsx:386` 有勾选框，列表行显示 `contentSources.historicalLabel`。

**§3.3 的「常用参考资料」「典型用户问题」两路本卡不必再做**，用现成的收件箱录入即可。本卡要对齐的是它的口径：布尔、创建时确定、此后不可改。

### 026 今日工作台第五项

`specs/026-today-dashboard/spec.md` 的 FR-005b：第五项「待补录的反馈」列**已发布、却一条人工指标都没录**的发布记录，数据源 `GET /api/content-feedback/pending`；经 specs/029 修订后只列**观察天数已过**的，没设天数或没有发布时间的仍在列并写明「答不出来」。

后果要写清楚：**本卡导入的每一条历史作品，都会立刻出现在第五项里**（它是发布记录，且没有指标）。这是对的——历史反馈确实待补录——但如果导入二十条，工作台第五项会被历史内容淹没。这是 Q6 的一半。

### 「暂无个人表现数据」：全仓零命中

`grep -rn "暂无个人表现数据\|noPerformanceData"` 在 `packages/` 与 `apps/web` 下无命中。今天没有任何地方说这句话，也没有任何地方需要说——因为「推荐依据」本身（EP-04c 候选自动生成）没落地，026 的 FR-011 明确写着「候选自动生成暂不可用」。

### 登记表：没有 `historical-import` 模块

`scripts/content-boundaries.json` 的 `modules` 有 12 个，**没有** `historical-import`。已落地八个：`diagnostics`、`workspace-core`、`ip-profile`、`topic-planning`、`work-editor`、`review-delivery`、`feedback-learning`、`source-inbox`。

导入要同时写 work-editor 与 review-delivery 两个模块的对象。看依赖表，**`review-delivery` 已经依赖 `work-editor` 与 `source-inbox`**，所以「导入编排放进 review-delivery」在依赖方向上是通的；放进 `work-editor` 则不通（它不依赖 review-delivery）。第三条路是 web 适配器串既有端点、一个模块都不新增。这是 Q2。

### 可复用的既有模式

- **受控集**：Go 枚举权威 → 400 诊断式错误（`FieldError` 指名字段）；DB `CHECK` 兜底。
- **只插不改 + 守卫用例**：扫模块源码里的 `UPDATE <table>` / `DELETE FROM <table>`，**并同时断言存在 `INSERT`**（少了后半条，空模块也是绿的）。
- **迁移**：下一个可用号 **532**（现有最大 531）。R5 建表不内联 `PRIMARY KEY`/`UNIQUE`；R6 每个建索引的 up 登记进 `server/cmd/migrate/main.go` 的 `concurrentIndexCleanups`，名字逐字节一致，建表迁移不登记；每个并发索引单语句单文件。
- **第 12 步**：每个带路径参数的端点，要有一条用例穿过真实中间件且**路径参数值与上下文值不同**。
- **栅栏**：写事务第一条语句取 `LockForContentDiagnosticWrite`。

---

## Clarifications

六个问题。每个都给了推荐值，**推荐值已写进下面的 FR，裁决前不阻塞**；裁决若不同，改的是 FR 而不是整份规格的骨架。

### Q1 — 版本的 `action` 加不加 `imported`？

今天的写路径由代码选定 `(source, action)`，调用方给不了。三个选项：

- **A（推荐）**：`Action` 加第四个值 `imported`，`Source` 保持 `edited` 三值不变。理由：`Source` 的注释自己声明了「Exactly three, per SOP 7.1」，动它等于推翻一句写死的话；`Action` 没有这个声明。而且「导入」确实不是「保存」——把它记成 `saved` 会让版本历史说「有人在 2026 年写下了这篇 2024 年的文章」。代价：改 `contract.go`、`contract_test.go:24`、迁移 500 的 `CHECK`（新迁移 `ALTER ... DROP CONSTRAINT / ADD CONSTRAINT`），加一个 `ImportVersion` 方法。
- **B**：复用 `action=saved`，「导入」只记在作品层的标识上。代价小，但版本行本身说不出自己的来历，且 §3.3 说的是「系统**保留**标识」——保留在哪一层是我们定的，落在作品层不算违规。
- **C**：`Source` 也加 `imported`。**不推荐**，直接和那句注释冲突。

### Q2 — 导入编排归谁？

- **A（推荐）**：**web 适配器串既有端点，不新增模块**。理由：需要的五个端点全部已存在（见 FR-020），新增一个模块意味着新的登记项、新的依赖边和一套新的存储层，而它自己一张表都不建。
- **B**：放进 `review-delivery`（依赖方向通，它已依赖 work-editor）。好处是**一个事务**，A 做不到。
- **C**：新建 `historical-import` 模块，依赖 `["workspace-core", "work-editor", "review-delivery", "source-inbox", "diagnostics"]`。

**A 有一个必须写进规格的代价**：五个 POST 串起来不是原子的，中途失败会留下「有作品没发布记录」的半成品。FR-022 / FR-023 处理它。若裁决 B，FR-022 / FR-023 换成「一个事务，失败即整体回滚」，其余 FR 不动。

### Q3 — 历史作品没有选题卡，怎么办？

- **A（推荐）**：放宽 `content_work.topic_card_id` 允许 `''`，语义与同表的 `snapshot_id` 完全一致——「A real state, not a missing value」。改 `store.go:173` 的守卫与 024 契约注释。代价：024 已落地契约的一处放宽。
- **B**：导入时先建一张 `draft` 选题卡。**不推荐**：那张卡是编出来的，它会出现在今日工作台「值得写的选题」里，让运营者看到一条根本不需要写的选题——正是「不虚构」要防的事，只是换了个位置。
- **C**：每个工作区一张合成的「历史导入」选题卡，所有导入挂它。比 B 干净，但它仍是一条不存在的选题卡，且要额外处理「它不能被列进第一区块」。

### Q4 — 「发布后快照」放哪？

- **A（推荐）**：给 `content_publication_record` 加一列 `version_id text NOT NULL DEFAULT ''`。它就是「发布后快照」：这条发布对应的是这份正文，直接指向，没有中间的审核请求与交付任务，**因此也没有虚构的版本链**。附带好处是 027 的两跳反查多了一条直达短路，`ManualMetric.VersionID` 对导入行不再是空串。
- **B**：新建一张 `content_post_publication_snapshot` 表。表达更显式，但它和「发布记录 + 一个 version_id」承载的信息完全一样，多一张表多一套读写。
- **C**：读时从「该 artifact 只有一版」推出来。**不推荐**：一旦有人事后在导入的文档上再存一版，这个推断就错了，而且错得无声。

三个选项都不虚构版本链：导入的文档**只有一个版本**，快照指向它本身。

### Q5 — 「历史导入」标识落在哪几行？

- **A（推荐）**：`content_work` 与 `content_publication_record` 各加一列布尔，创建时确定、此后不可改（口径与 028 一致）。作品侧回答「这篇是导入的」，发布记录侧回答「这个数字是历史的」——后者是 027 日后做聚合时唯一能把历史与新发布分开的依据，**没有它，第一次算「本月表现」就会把三年前的数据算进去**。
- **B**：只加在 `content_work`。发布记录侧靠 `work_id` 反查。省一列，但让「这条指标是不是历史的」变成一次连表。
- **C**：版本行上也加。不推荐，导入的文档只有一版，信息重复。

### Q6 — 「暂无个人表现数据」显示在哪、条件是什么？

原文这句话挂在「推荐依据来自 IP 方向与现有素材」后面，而**推荐（EP-04c）没落地**，026 的 FR-011 已经写着「候选自动生成暂不可用」。

- **A（推荐）**：现在就显示，落在**账号页**与**今日工作台「值得写的选题」区块的说明行**，条件是「该范围内一条人工指标都没有」（`feedback-learning` 的指标行数为 0）。理由：这句话是对运营者说的「别把下面的东西当成数据支撑的结论」，它在推荐落地前就成立，而且 EP-04c 落地时它已经在了，不用再补。
- **B**：等 EP-04c 一起做。规格里只写清条件，不做界面。
- **C**：只在账号页显示。

另外，**FR-028 处理导入把今日工作台第五项淹没的问题**（Current State 最后一节）：本卡不改 026 的判定，只要求第五项的每一条能看出它是历史导入的。

---

## User Scenarios & Testing *(mandatory)*

### User Story 1 - 把一篇已经发出去的文章搬进来（Priority: P1）

运营者手上有一篇两年前发在小红书、当时反响不错的文章。他打开导入页，粘贴正文，选渠道，填发布时间、平台账号与链接，提交。系统建出一件作品、一份文档、一个版本，以及一条「已报告发布」的发布记录，并把它们都标成历史导入。

**Why this priority**：这是 §3.3 的主句，也是本卡唯一新做的主体流程。没有它，后面四个故事都没有可挂的对象。

**Independent Test**：导入一条，然后在作品页能打开它的正文，在发布记录列表里能看到它，两处都写着「历史导入」。收件箱、指标、工作台都不必动。

**Acceptance Scenarios**:

1. **Given** 一个工作区，**When** 提交一次完整的导入（正文 + 渠道 + 发布时间 + 账号 + 链接），**Then** 产生 1 件作品、1 份文档、**恰好 1 个版本**、1 条发布记录，且发布记录的 `version_match` 是 `unknown`、`delivery_task_id` 为空。
2. **Given** 同一次导入，**When** 查看该文档的版本列表，**Then** **只有一条**，没有任何「从某版恢复 / 采用某版」的行——**没有虚构的版本链**。
3. **Given** 导入时没有填链接或平台内容 id，**When** 提交，**Then** 被拒绝并指名 `page_url_or_content_id`（025 对 `reported_published` 的既有要求，不是本卡新加的）。
4. **Given** 导入产生的发布记录，**When** 读它，**Then** 声明者（`declared_by`）是导入者，`actor_id` 取自会话而非请求体。
5. **Given** 一次导入，**When** 查看它产生的任何一个对象，**Then** 没有任何字段是系统推断出来的：渠道、发布时间、账号、链接、正文全部来自表单（宪法 IX）。

---

### User Story 2 - 说清楚这是历史的，不是刚写的（Priority: P1）

作品列表、发布记录列表、今日工作台第五项里，导入进来的东西一眼能认出来是历史导入。运营者不会把三年前的发布当成上周的成果。

**Why this priority**：§3.3 原文明确要求「系统保留它们的『历史导入』标识」。少了它，导入就是往干净的数据里掺入来历不明的行。

**Independent Test**：导入一条、正常新建一件作品，两者在同一个列表里的显示能区分开。

**Acceptance Scenarios**:

1. **Given** 一件导入的作品与一件正常创建的作品，**When** 打开作品列表，**Then** 前者有「历史导入」标识，后者没有。
2. **Given** 一条导入的发布记录，**When** 读它，**Then** 它自己带着历史导入标识，**不需要**调用方回查作品才知道。
3. **Given** 一条导入的发布记录，**When** 尝试修改它的历史导入标识，**Then** 改不了——发布记录本就只插不改，标识随行写死。
4. **Given** 一件导入的作品，**When** 尝试把它的历史导入标识改成 false，**Then** 被拒绝（口径与 028 的 `historical_import` 一致：创建时确定、此后不可改）。

---

### User Story 3 - 不知道用的是哪一版，就说不知道（Priority: P2）

历史作品的正文是从平台上复制回来的，运营者不知道当初的原稿经过几轮修改。系统不去猜：发布记录指向的就是导入的那一版正文，版本匹配标成 `unknown`，并且不造任何审核请求与交付任务。

**Why this priority**：§3.3 里「不虚构版本链」是一条禁令。禁令没有正面产出，但违反它的代价是整个版本历史失去可信度。

**Independent Test**：导入一条，查它的审核请求与交付任务列表，都是空的；查版本匹配，是 `unknown`。

**Acceptance Scenarios**:

1. **Given** 一次导入，**When** 查该作品的审核请求，**Then** 零条；**When** 查交付任务，**Then** 零条。
2. **Given** 一次导入产生的发布记录，**When** 读它的「发布后快照」，**Then** 它直接指向导入的那一版正文，中间没有任何审核请求或交付任务。
3. **Given** 一次导入产生的发布记录，**When** 读 `version_match`，**Then** 是 `unknown`，且这不阻塞任何后续操作（025 既有语义）。
4. **Given** 一次导入，**When** 之后有人在这份文档上正常存一版新的，**Then** 发布记录的「发布后快照」**仍然指向导入的那一版**，不跟着最新版跑。

---

### User Story 4 - 把当年的数据和评论也补上（Priority: P2）

运营者手上还有当年的阅读数、点赞数，以及几条印象深刻的评论。他在导入产生的发布记录上补录指标与摘录，用的是 027 已有的界面。

**Why this priority**：§3.3 说的是「可用的真实历史反馈」。模块已落地，缺的只是一条可绑的发布记录——本卡的第一个故事刚好造出来。

**Independent Test**：在一条导入的发布记录上录一条指标，读回来不变。不需要任何 027 的代码改动。

**Acceptance Scenarios**:

1. **Given** 一条导入的发布记录，**When** 录一条指标，**Then** 成功，且这条指标能看出它绑的是一条历史导入的发布。
2. **Given** 当年只知道阅读数、不知道曝光数，**When** 录入，**Then** 曝光留空（`null`），**MUST NOT** 存成 0（027 既有语义：`nil` 是不知道，0 是确认为零）。
3. **Given** 一条导入的发布记录，**When** 录一条反馈摘录，**Then** 「别人说的」与「运营者的解读」分成两列存，与 027 一致。
4. **Given** 一条导入的发布记录上已经录了指标，**When** 打开今日工作台第五项，**Then** 它不再出现在「待补录」里（026/027 既有判定，本卡不改）。

---

### User Story 5 - 参考资料和用户问题走收件箱（Priority: P3）

运营者把常看的几篇参考文章和几个典型用户问题粘进素材收件箱，勾上「历史导入」。

**Why this priority**：§3.3 提到了这两类，但**它们今天就能做**（028 端到端已落地，包括勾选框）。本卡把它写进规格是为了说明「这一路不用再建东西」，而不是为了新做。

**Independent Test**：在现有收件箱页面勾上历史导入建一条，列表行显示历史导入标识。**这条今天就能验，不需要本卡的任何实现。**

**Acceptance Scenarios**:

1. **Given** 现有的素材收件箱页面，**When** 勾上「历史导入」粘贴一段参考资料，**Then** 建出的条目带历史导入标识——**无需本卡改动任何代码**。
2. **Given** 本卡交付后，**When** 回看收件箱的历史导入语义，**Then** 与作品侧、发布记录侧的语义一致：布尔、创建时确定、不可改。

---

### User Story 6 - 没有历史数据也能开始（Priority: P3）

一个全新的工作区，一条历史资产都没有导入。运营者照样能用，而且页面明确告诉他「暂无个人表现数据」，不拿空表假装成结论。

**Why this priority**：§3.3 的最后两句：「没有历史数据也可开始」「明确显示『暂无个人表现数据』」。前者是不许把导入做成必经步骤，后者是一句必须说出口的话。

**Independent Test**：新工作区不导入任何东西，打开今日工作台与账号页，两处都不报错，且都写着「暂无个人表现数据」。

**Acceptance Scenarios**:

1. **Given** 一条历史资产都没有的新工作区，**When** 走完新建工作区到写作的全流程，**Then** 没有任何一步要求先导入历史资产。
2. **Given** 该工作区一条人工指标都没有，**When** 打开账号页，**Then** 有一条恒定可见的说明写着「暂无个人表现数据」。
3. **Given** 同上，**When** 打开今日工作台「值得写的选题」区块，**Then** 说明行里同样写着「暂无个人表现数据」，与既有的「候选自动生成暂不可用」并列，**MUST NOT** 互相取代。
4. **Given** 该工作区录入了**任意一条**人工指标，**When** 重新打开两处，**Then** 「暂无个人表现数据」消失，**MUST NOT** 换成一个编出来的摘要。

---

### Edge Cases

- **正文超长**：一篇历史长文超过 `MaxBodyRunes`（200000 runes），导入被拒绝并说明，**MUST NOT** 截断——截断会让「发布后快照」和平台上的实际内容不一致，而这条记录存在的全部意义就是它们一致。
- **同一篇导入两次**：两条独立的作品与发布记录。本卡**不做**跨作品去重（028 的去重是按素材内容哈希，作品侧没有对应机制）；重复的代价由运营者承担，系统不擅自合并——与 R-011「内容相同不删除独立的收藏上下文与批注」同一态度。
- **渠道不在四个之内**（知乎、B 站、公众号之外的平台）：今天记不下来。见 Out of Scope 第 2 条。
- **发布时间填了未来的时间**：本卡不发明校验规则。025 的 `published_at` 是可空时间，没有上界；加一条「不能是未来」是 025 的事。
- **导入中途失败**（作品建好了，发布记录没建成）：见 FR-022 / FR-023。**绝不静默**。
- **导入二十条历史作品**：今日工作台第五项会一次多出二十条待补录。本卡不改 026 的判定（FR-028），但要求每条能看出是历史导入的。
- **导入的作品被拿去走正常的审核 / 交付流程**：允许，且不影响已有的发布记录——「发布后快照」钉在导入的那一版上（US3 场景 4）。
- **跨工作区**：把别的工作区的 artifact id 拼进导入请求，得到与「不存在」同形的拒绝（025 `Record` 的既有行为）。

---

## Requirements *(mandatory)*

### Functional Requirements

**范围与性质**

- **FR-001**: 导入 MUST 全部由人粘贴 / 填写。系统 MUST NOT 抓取任何平台页面、MUST NOT 持有任何平台凭据、MUST NOT 调用任何模型（宪法 IX，与 025 / 027 / 028 的同一条禁令一致）。
- **FR-002**: 导入 MUST NOT 是使用系统的前置步骤。一条历史资产都不导入，全流程 MUST 可用（§3.3「没有历史数据也可开始」）。
- **FR-003**: 本卡 MUST 是 web-only，MUST NOT 涉及桌面端与移动端。
- **FR-004**: 本卡 MUST NOT 支持文件导入（属 W-03）与 CSV 批量导入（027 已有的指标粘贴除外）。

**一次导入产生什么**

- **FR-005**: 一次成功的导入 MUST 恰好产生：1 件作品（work）、1 份文档（artifact）、**恰好 1 个版本**、1 条发布记录。MUST NOT 产生审核请求，MUST NOT 产生交付任务。
- **FR-006**: 导入表单的必填项 MUST 是：正文、渠道、发布时间、平台账号、链接或平台内容 id。标题可由运营者填写，缺省时的取法见 FR-016。
- **FR-007**: 产生的发布记录 MUST 是 `reported_published`，`declared_by` MUST 是导入者，`actor_id` MUST 取自会话而非请求体（025 既有约束）。
- **FR-008**: 产生的发布记录的 `version_match` MUST 是 `unknown`，且这 MUST NOT 阻塞任何后续操作（SOP 9.1 + 025 既有语义）。
- **FR-009**: 产生的发布记录的 `delivery_task_id` MUST 为空串。系统 MUST NOT 为了凑一条发布记录而造一个交付任务。

**不虚构版本链**

- **FR-010**: 导入产生的文档 MUST 只有一个版本。系统 MUST NOT 生成任何 `restored_from` / `adopted_from` 非空的版本，MUST NOT 生成任何声称早于导入时刻的版本行。
- **FR-011**: 「发布后快照」MUST 直接指向导入产生的那一版正文，中间 MUST NOT 经过审核请求或交付任务。
  - **暂定落点（Q4 = A）**：`content_publication_record` 新增 `version_id`（`text NOT NULL DEFAULT ''`）。
- **FR-012**: 「发布后快照」MUST 在写入时确定、此后 MUST NOT 随该文档的新版本移动。同一份文档之后正常存新版，快照 MUST 仍指向导入的那一版。
- **FR-013**: 快照 MUST NOT 由「该文档只有一版」这类读时推断得出（一旦有了第二版，推断就错，且错得无声）。
- **FR-014**: 版本行记录导入这一动作时，MUST 保持 `source = edited`（历史正文是人写的），动作值见 Q1。
  - **暂定（Q1 = A）**：`Action` 新增第四个值 `imported`；`Source` 保持三值不变。写路径 MUST 与既有三条一样由代码选定 `(source, action)`，MUST NOT 让请求体指定动作。

**历史导入标识**

- **FR-015**: 导入产生的作品与发布记录 MUST 各自带「历史导入」标识，语义与 028 完全一致：**布尔、创建时确定、此后不可改**。
  - **暂定（Q5 = A）**：两处各加一列布尔。
- **FR-015a**: 发布记录 MUST 自己带这个标识，MUST NOT 要求读方回查作品才能判断。理由写进规格是因为它有具体后果：日后做表现聚合时，这是唯一能把历史数字与新发布分开的依据。
- **FR-016**: 运营者没填标题时，标题 MUST 取自正文的可读前缀，MUST NOT 留空、MUST NOT 编造一个概括（那是模型的活）。
- **FR-017**: 导入产生的作品在作品列表里 MUST 与正常创建的作品可区分。
- **FR-018**: 任何路径 MUST NOT 能把已存在对象的历史导入标识由 true 改为 false 或反之。

**编排与失败**

- **FR-019**: 本卡 MUST NOT 新建数据库表。（新增列不在此列——FR-011 / FR-015 各需要一列。）
- **FR-020**: 导入 MUST 只使用已落地的写入口。清单：`POST /api/content-works`、`POST /api/content-works/{id}/artifacts`、该文档的版本写入口（FR-014）、`POST /api/content-publications`；历史反馈另走 `POST /api/content-metrics` 与 027 的摘录入口。
- **FR-021**: 导入编排的归属见 Q2。
  - **暂定（Q2 = A）**：web 适配器按序调用既有端点，**不新增内容模块**，`scripts/content-boundaries.json` 的 `modules` 依赖表 MUST NOT 被修改；新增的页面按既有惯例登记进 `adapters`。
- **FR-022**: 在 Q2 = A 下，编排**不是原子的**。任何一步失败时，导入 MUST 停在该步，MUST NOT 继续后面的步骤，并 MUST 向运营者说明**已经建出了什么、还差什么**。MUST NOT 只显示一句「导入失败」。
- **FR-023**: 在 Q2 = A 下，半成品 MUST 是可收拾的：已建出的作品 MUST 是一件正常的作品（可打开、可编辑、可后续补一条发布记录），MUST NOT 处于任何需要特殊清理的中间态。系统 MUST NOT 自动删除已建出的对象——删除是运营者的决定。
  - 若裁决 Q2 = B（放进 `review-delivery`，一个事务），FR-022 / FR-023 MUST 改写为「失败整体回滚，不留任何半成品」，其余 FR 不变。

**「暂无个人表现数据」**

- **FR-024**: 当作用范围内**一条人工指标都没有**时，系统 MUST 显示「暂无个人表现数据」。判定 MUST 只看指标行数是否为 0，MUST NOT 引入任何时间窗口或阈值。
- **FR-025**: 该说明的位置见 Q6。
  - **暂定（Q6 = A）**：账号页与今日工作台「值得写的选题」区块的说明行各一条。工作台那条 MUST 与既有的「候选自动生成暂不可用（EP-04c 未落地）」**并列显示**，MUST NOT 取代它——两句话说的是两件事。
- **FR-026**: 有了任意一条人工指标后，该说明 MUST 消失，且 MUST NOT 被替换成任何系统生成的表现摘要（EP-04c 未落地，宪法 IX）。
- **FR-027**: 该说明 MUST 是派生的，MUST NOT 落任何存储列。

**与已落地模块的边界**

- **FR-028**: 本卡 MUST NOT 修改 026 第五项「待补录的反馈」的判定，也 MUST NOT 把历史导入的发布记录排除在外——它们确实待补录。但第五项的每一条 MUST 能看出它是否为历史导入，否则一次导入二十条会让这个区块读不懂。
- **FR-029**: 本卡 MUST NOT 修改 027 的任何契约。指标与摘录绑发布记录的既有路径 MUST 原样可用。
- **FR-030**: 本卡 MUST NOT 修改 028 的任何契约。「常用参考资料」「典型用户问题」两路 MUST 走现有的素材收件箱，本卡 MUST NOT 为它们新建入口。
- **FR-031**: 本卡 MUST NOT 扩大 025 的 `Channel` 受控集。
- **FR-032**: 对 024 / 025 契约的改动（FR-011 / FR-014 / FR-015 各自可能带来的列与枚举变更）MUST 各自伴随受控集用例、DB `CHECK` 与迁移；受控集的 Go 值、`contract_test.go` 的清单与迁移 `CHECK` 三者 MUST 逐字一致。

### Key Entities

- **导入的作品（Work）**：一件历史作品的容器。带历史导入标识；选题卡位见 Q3（推荐允许为空，语义同 `snapshot_id`）。
- **导入的文档（Artifact）与它唯一的版本**：正文本身。版本的 `source` 是 `edited`，动作见 Q1。
- **发布记录（PublicationRecord，025 既有）**：`reported_published`，声明者是导入者，`delivery_task_id` 空，`version_match = unknown`，新增「发布后快照」指向与历史导入标识两项。
- **发布后快照**：「这条发布对应的是这份正文」这一事实。不是新对象，是发布记录上的一个确定指向（Q4 = A）。
- **历史反馈（027 既有）**：绑在上述发布记录上的手录指标与反馈摘录。本卡不新建。
- **历史素材（028 既有）**：`historical_import = true` 的收件箱条目。本卡不新建。

---

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: 一次完整导入之后，该作品的版本列表**恰好一条**，且 `restored_from` 与 `adopted_from` 都为空。
- **SC-002**: 一次完整导入之后，该作品的审核请求数为 0、交付任务数为 0。
- **SC-003**: 导入产生的发布记录的 `version_match` 读回来是 `unknown`，`delivery_task_id` 读回来是空串。
- **SC-004**: 缺链接 / 缺平台内容 id 的导入被拒绝，且错误指名 `page_url_or_content_id`（而非一句「参数错误」）。
- **SC-005**: 导入之后再在同一份文档上正常存一版，发布记录的「发布后快照」读回来**仍是导入的那一版**。
- **SC-006**: 导入的作品与正常创建的作品，在同一个列表里的历史导入标识不同；发布记录侧**不查作品**即可读出该标识。
- **SC-007**: 任何请求都无法把已存在对象的历史导入标识改掉，被拒绝时错误指名该字段。
- **SC-008**: 正文超过 200000 runes 的导入被拒绝，且**没有任何对象被建出来**。
- **SC-009**: 在导入产生的发布记录上录一条指标成功，且曝光留空时读回来是 `null` 而不是 0。
- **SC-010**: 一条人工指标都没有的工作区，账号页与今日工作台两处各有一条恒定可见的「暂无个人表现数据」；录入任意一条指标后两处都消失，且**没有出现任何系统生成的表现摘要**。
- **SC-011**: 今日工作台的「值得写的选题」区块同时显示「候选自动生成暂不可用」与「暂无个人表现数据」两句，互不取代。
- **SC-012**: 一条历史资产都不导入，从新建工作区到写出第一篇作品的全流程无任何一步要求先导入。
- **SC-013**（Q2 = A 下）：把第四步（建发布记录）人为置为失败，运营者看到的提示写明「作品与文档已建出、发布记录未建成」，且那件作品随后能正常打开与编辑。
- **SC-014**: `scripts/content-boundaries.json` 的 `modules` 依赖表在本卡前后**逐字节不变**（Q2 = A 下）。
- **SC-015**: 受控集三处（Go 值、`contract_test.go` 清单、迁移 `CHECK`）对本卡改动后的集合逐字一致。

---

## Out of Scope（本卡明确不做，且写明它们何时出现）

1. **文件导入**（上传 doc / pdf / 图片）：属 W-03。它落地时，导入表单多一个来源，本卡的对象模型不变。
2. **四渠道之外的平台**：025 的 `Channel` 只有四个，扩它是一次迁移加一次受控集变更，属 025 的后续卡。在此之前，发在其他平台的历史作品只能选一个近似渠道并在备注里写明——**规格不为此发明一个 `other` 值**（与 027 的指标集同一态度：给了逃生舱，第一个被塞进去的就是本该在清单里的名字）。
3. **跨作品的历史内容去重**：028 的去重按素材内容哈希，作品侧没有对应机制。同一篇导入两次是两条独立的记录。
4. **批量导入**：本卡一次一条。批量要处理部分失败、进度与回滚，那是另一张卡；027 已有的指标批量粘贴不受影响。
5. **历史表现的聚合与排名**：SOP 10.2 的事。本卡只保证 FR-015a 的标识存在，让日后的聚合分得开历史与新发布。
6. **EP-04c 候选自动生成**：「推荐依据来自 IP 方向与现有素材」的推荐本身没落地。本卡只做「暂无个人表现数据」这句话，不做推荐。
7. **「发布时间不能是未来」这类校验**：025 的 `published_at` 今天没有上界，加上界是 025 的事。

### 一条由裁决直接推出的已知限制

若 Q2 裁决为 A（web 适配器），**导入不是原子的**。这不是疏漏，是选 A 的代价，FR-022 / FR-023 把它变成运营者看得见、收拾得了的状态而不是静默的半成品。裁决 B 会消掉这条限制，代价是新增一条 `review-delivery → 导入编排` 的职责。

---

## Assumptions

1. **导入者就是声明者**：发布记录的 `declared_by` 取导入者本人。历史作品是他自己发的，这个假设在 §3.3 的语境下成立；若是替别人导入，他可以改 `declared_by` 的文本（025 里它本就是自由文本）。
2. **历史正文是人写的**：所以版本的 `source` 是 `edited` 而不是 `generated`。即便当年用过模型辅助，我们也无从知道，而 `generated` 的注释写明「Nothing in this phase can produce it」。
3. **一件历史作品对应一份文档**：不拆正文与渠道稿。历史作品已经发出去了，它就是那一份。
4. **「暂无个人表现数据」的范围是工作区**（Q6 = A 下，账号页那条可收窄到账号）。收窄到账号需要指标按账号过滤，027 的 `ManualMetric` 有 `AccountID`，做得到。
5. **导入量是「少量」**：§3.3 的标题就是「导入少量历史资产」。本卡不为几百条的规模做分页、后台任务或进度条。
6. **本卡不需要新的鉴权边界**：走的全是既有端点，每一个都已经过 `workspace-core.Authorize`。
