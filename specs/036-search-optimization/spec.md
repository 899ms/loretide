# Feature Specification: 平台搜索优化（BO-05 / R-060）

**Feature Branch**: `claude/spec-036-search-optimization`

**Created**: 2026-09-25

**Status**: **待裁决**。主控派单的八条决定（D1–D8）已写进各条 FR；Q1～Q8 是留给主控的问题，每题给了推荐值，见文末「待裁决问题」。

**Input**: 文档仓库 `docs/14-品牌全案运营扩展需求.md` 的 R-060、「工作流与技术落点」、补充验收 D14-V09、V10、V15；`tasks/brand-operations.md` 的 BO-05（「实施前拆分搜索主题/意图、优化建议与版本采用、人工搜索观测和 D14-V09/10/15 测试」）；主控派单（2026-09-25）。

**模块**（每一部分分开写；落点问题见 Q1）：

| 部分 | 模块 | 依据 |
|---|---|---|
| 搜索主题（问题、关键词/主题组、意图、来源、关联） | **`topic-planning`** | 文档 14：「搜索主题由选题领域维护」 |
| 优化建议、比较、采用/放弃的决定与效果 | **`topic-planning`**（Q1 推荐 A；备选 B 放进 `work-editor`） | 文档 14「优化建议关联作品候选版本」；D3「经作品编辑器公开接口、由 handler 适配器」 |
| 采用时写新作品版本 | **`work-editor`**（新增公开函数 `ApplyBody`） | 文档 12 §2：`work-editor` 独占「内容版本」 |
| 搜索指标与排名观察 | **`feedback-learning`** | 文档 14：「观测指标复用反馈领域」 |
| 跨模块读写 | `server/internal/handler/content_search_*.go` 三个适配器文件 | D5 |

`modules` 依赖表**一个字不改**；需要追加的 `adapters` 条目写在 plan.md「主控决定」，由各实施 PR 自带。

**本卡只出规格，不含实现。** 实施拆成四个 PR，见 `tasks.md`。

**命名**：代码前缀 `search_`（Go 文件）/ `content_search_`（表、handler 文件），路由 `/api/content-search`，页面 `/{workspaceSlug}/search-optimization`，i18n 命名空间 `search_optimization`。

---

## 需求原文

**下面是文档 14 的原句，不是转述。** 每个受控集、每条「系统不做什么」都要能指回其中一句或主控的 D1–D8；指不回去的，在「本规格补的设计」一节单独列出。

**R-060**

> - 针对所选中国平台、账号和经营目标，整理用户搜索问题、关键词/主题组与搜索意图，关联资料、候选选题和内容简报。
> - 支持人工输入关键词、已有客户提问及授权素材；联网研究沿用本地/联网/全部范围、来源和执行预算。没有搜索数据接口时不得虚构搜索量、竞争度或排名。
> - 检查标题、正文、话题及平台允许配置的描述是否准确回答目标问题，输出可编辑建议、依据和修改差异。不得以关键词堆砌替代内容质量，不保证搜索排名。
> - 建议必须经过用户采用；采用后产生新作品版本，使旧预检报告按既有规则过期，已批准版本不自动继承审批。
> - 支持手工登记平台提供的搜索曝光、搜索来源访问等实际可用指标，以及带时间、查询词、观察条件和证据的排名观察。缺少指标明确显示未知；个别观察不代表稳定或全平台排名。

**工作流与技术落点（摘录）**

> - 数据库不新增外键/级联；必要关系由应用事务校验。新增索引遵守现有独立并发索引迁移约束。
> - 搜索主题由选题领域维护，优化建议关联作品候选版本；观测指标复用反馈领域。研究→选题→渠道稿优化→人工审核→手工搜索表现记录→AI 分析，不新增自动平台操作。

**既有规则（「旧预检报告按既有规则过期」指的是这两句）**

> 内容修改只使旧报告相对当前稿过期，不每次保存调用模型；无变化保存不制造过期。（文档 11）
>
> 内容变化产生新版本，原批准仍属于旧快照，交付时重新检查有效性。（文档 12 §3）

**补充验收**

| 编号 | 验收要求 |
| --- | --- |
| D14-V09 | 搜索主题、意图、来源可追溯；无数据时搜索量/竞争度/排名显示未知，本地模式不联网 |
| D14-V10 | 搜索优化建议可比较、采用和放弃；采用产生新版本并使旧报告过期，不能沿用旧终审；搜索观察保留窗口与条件 |
| D14-V15 | 浏览器完成关键词→优化候选→审核→手工反馈及成本/线索/成交→复盘→人工采纳；拒绝建议不修改预算、配置或记忆 |

---

## 主控已定（2026-09-25，派单要点）

| # | 决定 | 落在哪几条 FR |
|---|---|---|
| **D1** | 真实 AI 执行器保持禁用（宪法 IX），本版**不联网**。交付：(a) 人工维护的搜索主题：问题、关键词/主题组、意图与来源，关联资料、选题卡和简报，由选题领域拥有；(b) 人写的优化建议，覆盖标题、正文、话题及平台允许的描述，每条带依据与相对目标作品版本的差异，可比较、采用或放弃；(c) 手工登记平台提供的搜索指标（搜索曝光、搜索来源访问，只登记平台给出的）与排名观察，带时间、查询词、观察条件与证据，复用反馈领域。AI 生成建议与联网研究（含本地/联网/全部范围与预算）是后续卡。本地模式从不联网 | FR-001～FR-019、FR-030～FR-041、FR-070～FR-078 |
| **D2** | D14-V09：没有数据时，搜索量、竞争度、排名显示「未知」，不伪造；单次观察绝不呈现为稳定排名或全平台排名 | FR-014、FR-072、FR-075、FR-076、FR-080～FR-084 |
| **D3** | D14-V10：采用一条建议，经 handler 适配器调用作品编辑器的公开接口，产生一个新作品版本；旧预检报告按既有规则过期，已批准版本不把批准带到新版本，旧报告不用于终审。放弃不改任何东西 | FR-050～FR-060 |
| **D4** | 不做关键词堆砌：规格写明产品从不承诺排名，建议的目标是准确回答目标问题 | FR-031、FR-036、FR-037、FR-081 |
| **D5** | 模块边界：模块之间不互相 import；所有跨模块调用经 handler 适配器；不改 `modules` 表；需要时给出适配器方案。每一部分写明落点，落点作为待裁决问题并给推荐 | 文首「模块」表；FR-101、FR-102；Q1；plan.md |
| **D6** | 数据库无外键、无级联，遵守 R1–R6（`server/internal/migrations/content_constraints_test.go`）；索引单独、单语句、并发；**迁移号不预留**，合入时改号；写路径先取删除栅栏；已删除的工作区在**每一个**适配器里都映射为 `ErrNotFound`（035 PR 1 曾把「不存在」映射成存储错误）；带路径参数的端点有真实中间件用例且必须挂进 `router.go` | FR-100～FR-108 |
| **D7** | 界面沿用上游 Multica 设计系统；不写 UI 单测、不用 computer use；附 `manual-ui-todo.md`；D14-V15 只能部分自动化，写明哪几段 | FR-110～FR-115；SC-014；「D14-V15 覆盖说明」 |
| **D8** | 本地迁移测试固定写 `DATABASE_URL='postgres://none:none@127.0.0.1:1/none?sslmode=disable' go test ./cmd/migrate ./internal/migrations`；每张卡列「远程验收」（`~/loretide-ci/lt-verify.sh <branch> all`） | tasks.md |

---

## Current State（以代码为准，2026-09-25 / `app-main` @ `8bd4d55`）

### 1. 预检（EP-06）在代码里不存在

仓库里没有预检报告表、没有预检运行、没有「报告过期」的实现。019（LT-015）只落地了品牌开关 `loretide.auto_precheck`（工作区 settings 键）；`topic-planning/snapshot.go:37` 把它记进启动快照，注释写明「触发预检是 EP-06」。文档 11 与文档 12 §3 写下了规则（见「需求原文」），代码里还没有承载它的对象。

所以 D3 的「旧预检报告过期」在本版只能落成**结构性保证**，不能落成「把某份报告标成过期」的写操作：

- 采用一定产生一个**新的** `version_id`，而且正文与基础版本**一定不同**（FR-056）——这满足「内容修改使旧报告过期」与「无变化保存不制造过期」两半；
- 本卡写进合同：EP-06 的报告 MUST 以 `version_id` 为键，「过期」= 报告的 `version_id` ≠ 该文档最新版本（contract §8）。EP-06 落地时照此实现，就自动覆盖本卡产生的版本。

### 2. 作品版本只版本化「正文」

`work-editor/contract.go`：`ArtifactVersion` 只有 `Body`；作品标题 `Work.Title` 与文档标题 `Artifact.Title` 是可改的普通列，**不进版本**。「话题」「描述」没有任何结构化字段。所以 R-060 的「标题、正文、话题及平台允许配置的描述」四项，在本版只能都是**目标文档正文里的文字**（Q4）。渠道稿（`channel_draft`）本来就是一个渠道的完整稿子，标题行、话题标签、描述都在正文里。

### 3. `work-editor` 写版本的唯一入口已支持幂等

`work-editor/version.go:100` 的 `appendVersion` 是写版本的唯一路径：取删除栅栏 → 审计 → `FOR UPDATE` 锁文档 → 取号 → 插入 → 把编辑副本改成 `saved`；可选的 `idempotency.Request` 在同一事务里 `Claim` / `Complete`，重放时直接返回上次的版本（031 历史导入用）。公开的四个入口 `SaveVersion`、`RestoreVersion`、`AdoptVersion`、`ImportVersion` 都**不能**「以给定正文写一个新版本，并核对基础版本」：

- `SaveVersion` 保存的是编辑副本。采用建议若先 `PatchArtifact` 再 `SaveVersion`，两步之间有窗口，而且会覆盖人没保存的草稿；
- 没有一个入口核对「基础版本仍是最新」。

因此本卡要给 `work-editor` **新增一个公开函数** `ApplyBody`（contract §7.4）。它只是 `appendVersion` 的又一个意图，受 `TestTheVersionTableHasNoUpdateOrDeletePath`、`TestSourceAndActionAreNeverReadFromInput`（`work-editor/guards_test.go:111,140`）约束。

`Actions` 受控集是 `saved / restored / adopted / imported`；`imported` 是迁移 535 用「一条 `ALTER TABLE` 同时 DROP 与 ADD CHECK」加进来的（Q3 照此加 `suggestion_applied`）。前端 `packages/core/content/work-editor/contract.ts:30` 的 `VERSION_ACTIONS` 与 `packages/views/content/work-editor/index.tsx:690` 的动作文案要同步。

### 4. 审批按版本回答，新版本没有批准

`review-delivery/review.go:291` 的 `LatestReviewStatusFor(ws, actor, artifactID, versionID)` 按**版本**回答；它的注释说明，这个函数让「系统不得把『已通过』自动套在新版本上」可以观察：一个版本批准后，下一个版本读回来是「没有审核请求」。`Submit`（`review.go:42`）冻结的是提交时指定的那个版本。交付任务通过审核请求绑定旧版本（`states.go:240` `ShouldHold` 比较 `VersionID`）。本卡**不改** `review-delivery`，只用它的公开读接口在用例里断言（SC-006）。

顺带发现：`HoldForChangedTarget`（`delivery.go:249`）在整个服务端**没有调用方**——内容变化后，交付任务不会自动转 `held`。它交付的仍是被批准的旧版本（这本身是对的），但「提醒有新版本」这一步不存在。记为后续项，本卡不修（宪法 VIII）。

### 5. 反馈领域的指标集是故意封闭的

`feedback-learning/contract.go` 的 `Metrics` 是 SOP 10.1 的**恰好十一项**，注释写明没有 `other`；027 的 `TestThereIsNoSixthControlledSet` 守着受控集的数目；035 的表现维度按 `Metrics` 泛型遍历（035 spec Current State §3）。把「搜索曝光」「搜索来源访问」加进 `Metrics`，会同时改变 027 的合同和 035 已生成报告的复算基础（Q2）。

`ManualMetric.Value` 是指针：nil = 未知，0 = 确认为零。本卡的搜索指标照抄这条。

`feedback-learning` 的包级守卫（035 spec Current State §3）同样约束本卡：SQL 里不许 `SUM(`、`AVG(`、`GROUP BY`，`count(` 只许一处；不外发、不调模型；新受控集要登记。

### 6. 选题领域已有的公开读写

| 需要 | 已有 | 位置 |
|---|---|---|
| 核实素材（资料）存在 | `SourceReader.Exists`，handler 的 `topicSourceReader` 实现 | `topic-planning/sources.go`、`handler/content_topic.go:60-80` |
| 核实账号存在、读平台 | `AccountReader.Get`（`topic-planning` 可以 import `ip-profile`，依赖表里有） | `topic-planning/store.go:26` |
| 选题卡、简报 | `Store.Get`、`GetBrief`（同模块，可直接读表） | `store.go:559,800` |
| 平台受控集 | `ipprofile.Platforms`（八个） | `ip-profile/account.go:22-33` |
| 删除栅栏 | `Store.Guard`，`begin()` 第一句取 | `store.go:89` |

`topic-planning` **不能** import `work-editor`、`feedback-learning`、`source-inbox`。

### 7. 跨模块适配器的接线方式与一个已知的坑

照 034/035：模块定义只用本模块类型与字符串的小接口，handler 文件里的适配器实现它，在 `h.xxxStore()` 里注入（`handler/content_roi_records.go:27-95`、`handler/content_opdiag_reports.go`）。

**已删除工作区的映射**：工作区删除提交后，第一个读到「没有」的可能是任何一个适配器。035 的 `opdiagReadError`（`handler/content_opdiag_reports.go:97-110`）把 `workspacecore.ErrNotFound`、`reviewdelivery.ErrNotFound`、`feedbacklearning.ErrNotFound` 统一映射为 `feedbacklearning.ErrNotFound`；它的注释说明，否则被栅栏挡住的写会变成 503，而不是与其他写一致的 404。本卡三个适配器文件的**每一个**方法都要走同一种映射，并各有一条「工作区删除后」用例（FR-104）。

另一个方向的既有问题：027 的 `feedbackPublications.Resolve`（`handler/content_metric.go:83-93`）把**任何**查询错误（含数据库故障）都映射成 `ErrNotFound`，会把存储故障报告成 404。本卡复用它核实发布记录，但不改它，记为后续项。

### 8. 迁移、删除链、路由、页面登记

- R1 无外键、R2 无级联、R3 索引 `CONCURRENTLY`、R4 并发索引迁移单语句、R5 建表不含 `PRIMARY KEY` / `UNIQUE`、R6 并发索引登记进 `server/cmd/migrate/main.go` 的 `concurrentIndexCleanups`。
- 当前 `app-main` 最大迁移号 **584**。**本卡不预留编号**（D6）。
- 新表进 `server/internal/handler/workspace_delete_manifest_test.go` 与 `server/pkg/db/queries/workspace_delete.sql` 的同一条 CTE 链，`make sqlc`。
- 路由照 `/api/content-operating-diagnosis`（`server/cmd/server/router.go:2102`）挂在 `RequireWorkspaceMember` 组**之外**，handler 内 `workspacecore.Authorize`，`h.DiagnosticTrace`。
- 新页面登记 `packages/core/paths/paths.ts`、`packages/core/paths/route-icons.ts`、`packages/views/layout/route-icon-components.tsx`、`packages/core/diagnostics/diagnostic-context.ts`（同 034/035 的页面 PR）。

---

## User Scenarios & Testing *(mandatory)*

### User Story 1 - 整理一个搜索主题：问题、关键词、意图，查得到从哪来 (Priority: P1)

小张负责品牌的小红书主号，经营目标是「门店到店预约」。他新建搜索主题「羊绒大衣怎么洗」：平台小红书，账号选主号，经营目标写「让想买羊绒大衣的人知道能到店护理」；用户问题两条（「羊绒大衣能机洗吗」「羊绒大衣起球怎么办」），关键词五个；意图选「解决具体问题」；来源选「已有客户提问」，说明写「9 月私信里有 6 个人问过」。他再把这个主题关联到一张现有选题卡和它的第 2 版简报，并引用一份授权的面料资料。页面上显示「搜索量：未知」「竞争度：未知」「排名：未知（还没有观察记录）」，旁边写「本版没有搜索数据接口」。

**Why this priority**：R-060 第 1、2 条；D14-V09「可追溯」「显示未知」。

**Independent Test**：建一个主题，读回全部字段与第 1 修订；改一次关键词，得到第 2 修订，第 1 修订逐字节不变；`search_volume`、`competition` 都是 `{status: "unknown", reason: "no_data_source"}`。

**Acceptance Scenarios**：

1. **Given** 一个合法的主题，**When** 创建，**Then** 得到 `theme_id` 与修订 1，`origin`、`origin_note`、`source_ids`、`recorded_by`、`created_at` 全部读得回。
2. **Given** 来源为「已有客户提问」，**When** `origin_note` 为空，**Then** 400 点名 `origin_note`；来源为「授权素材」而 `source_ids` 为空，**Then** 400 点名 `source_ids`。
3. **Given** 问题与关键词都为空，**When** 创建，**Then** 400 点名 `questions`。
4. **Given** 引用了不存在（或属于别的品牌）的素材、选题卡或简报，**When** 创建，**Then** 400 点名对应字段（`source_ids` / `topic_card_ids` / `brief_revision_ids`），不泄露它是否在别处存在。
5. **Given** 请求体带 `search_volume`、`competition`、`rank`、`scope`、`budget` 之类的字段，**When** 提交，**Then** 400 点名该字段（严格解码）——本版既不接受人填的搜索量，也没有联网范围与预算。
6. **Given** 任何主题，**When** 读取，**Then** `search_volume` 与 `competition` 恒为「未知」，`data_origin = manual_only`；库里**没有**这两列。
7. **Given** 修改主题，**When** 带的 `base_revision` 不是当前修订，**Then** 409 点名 `base_revision`；归档也是一条新修订（`voided = true`），旧修订保留。

---

### User Story 2 - 写一条优化建议：对准问题、写明依据、看得到改了哪几行 (Priority: P1)

小张打开作品「羊绒护理指南」的小红书渠道稿（当前第 3 版），写一条建议：对准主题「羊绒大衣怎么洗」的问题「羊绒大衣能机洗吗」，涉及「标题」「正文」「话题」三项，依据写「原稿第一段没有正面回答能不能机洗，读者要翻到第四段；话题里没有『羊绒护理』」，然后贴上修改后的整篇稿子。保存后，页面显示相对第 3 版的差异：删了哪几行、加了哪几行。页面固定提示「建议的目标是准确回答目标问题；不要用堆砌关键词代替内容质量；本工作台不保证搜索排名」。

**Why this priority**：R-060 第 3 条；D4。

**Independent Test**：以第 3 版为基础建建议，读回的 `diff` 与测试里用同一纯函数算出的逐字节相同；`proposed_body` 与基础版本正文完全相同 → 400。

**Acceptance Scenarios**：

1. **Given** 一条建议，**When** 保存，**Then** 记下 `work_id`、`artifact_id`、`base_version_id`、`theme_id` 与当时主题的修订号 `theme_revision`、`target_question`、`aspects`、`rationale`、`proposed_body`、`author_kind = human`。
2. **Given** `proposed_body` 与基础版本正文逐字节相同，**When** 保存，**Then** 400 点名 `proposed_body`（没有改动的建议不是建议，也会让采用变成「无变化保存」）。
3. **Given** `aspects` 为空或含四项之外的值，**When** 保存，**Then** 400 点名 `aspects`。
4. **Given** `rationale` 为空，**When** 保存，**Then** 400 点名 `rationale`——每条建议必须有依据。
5. **Given** `theme_id` 指向已归档的主题，**When** 新建建议，**Then** 400 点名 `theme_id`。
6. **Given** `base_version_id` 不属于该文档，**When** 保存，**Then** 与「不存在」同形拒绝。
7. **Given** 一条建议，**When** 读取，**Then** `diff` 是按行的差异（`equal` / `delete` / `insert`）加上插入与删除行数；`base_is_current` 读时派生（基础版本是否仍是该文档最新版本）。
8. **Given** 建议的任何响应，**When** 反射扫字段名，**Then** 没有 `score`、`rank`、`density`、`keyword_count`、`seo_*` 一类字段（FR-036）。

---

### User Story 3 - 并排比较几条建议 (Priority: P2)

同一份渠道稿第 3 版上有三条建议（小张一条、同事两条）。他勾上三条点「比较」：三栏并排，每栏是该建议相对第 3 版的差异、它对准的问题、涉及的项和依据。系统不排序、不打分，按创建时间排列。

**Acceptance Scenarios**：

1. **Given** 2～4 条建议，**When** 比较，**Then** 返回每条的当前修订、相对各自基础版本的差异、`base_is_current`，以及 `same_base`（是否同一基础版本）；按 `created_at`、再按 `suggestion_id` 排列。
2. **Given** 1 条或 5 条，**When** 比较，**Then** 400 点名 `ids`。
3. **Given** 比较，**When** 完成，**Then** 所有表行数不变（比较是只读的）。
4. **Given** 比较结果，**When** 反射扫，**Then** 没有任何「推荐哪条」「得分」「排名」字段。

---

### User Story 4 - 采用一条建议：出现新版本，旧批准不带过去 (Priority: P1)

第 3 版已经审核通过。小张采用了其中一条建议。作品出现第 4 版，版本记录写「由搜索优化建议采用」；第 4 版的审核状态是「未提交」，不是「已通过」；他得重新提交审核。同一第 3 版上的另外两条建议变成「基础版本已不是最新」，不能再采用——要采用就得基于第 4 版重写。

**Why this priority**：R-060 第 4 条；D3；D14-V10。

**Independent Test**：第 3 版已批准 → 采用 → 文档最新版本变成第 4 版，正文 = 建议的 `proposed_body`；`LatestReviewStatusFor(第 4 版)` 返回「没有」；第 3 版的审核请求仍是 `approved` 且仍指向第 3 版。

**Acceptance Scenarios**：

1. **Given** 建议的基础版本是文档最新版本、编辑副本已保存，**When** 采用，**Then** 依次留下：决定（`adopt`，谁、何时、哪一修订）→ 经 `work-editor.ApplyBody` 写出的新版本（`source = edited`，`action = suggestion_applied`，正文 = `proposed_body`）→ 效果 `done` 指向新 `version_id`。
2. **Given** 文档的最新版本已不是建议的基础版本，**When** 采用，**Then** 409 点名 `base_version_id`，**不写决定**，也没有调用任何写接口。
3. **Given** 文档的编辑副本有没保存的改动（`draft_status = working`），**When** 采用，**Then** 409 点名 `draft_status`，不写决定——采用不能覆盖人没保存的字。
4. **Given** 基础版本已批准，**When** 采用成功，**Then** 新版本读回「没有审核请求」；旧版本的审核请求、交付任务、发布记录一个字节都不变（SC-006）。
5. **Given** 新版本，**When** 提交审核，**Then** 产生一条新的 `pending` 审核请求；系统**没有**任何把旧批准、旧报告复制或关联到新版本的路径（守卫，SC-007）。
6. **Given** 请求带的 `revision` 不是建议的当前修订，**When** 采用，**Then** 409 点名 `revision`——人采用的必须是他看到的那一版。
7. **Given** 采用成功，**When** 查看同一基础版本上的其他建议，**Then** 它们 `base_is_current = false`，采用按钮不可用；它们的记录不变。
8. **Given** 已有决定的建议，**When** 再提交决定，**Then** 409 点名 `suggestion_id`。

---

### User Story 5 - 采用中途失败，重试不会多出一个版本 (Priority: P2)

写版本时数据库断了一下：建议显示「已采用，写版本失败」，可以重试。另一种情况：版本写成了，但记录效果时失败，建议显示「已采用，结果未记录」；小张点重试，系统凭这条建议的幂等键拿回刚才那个版本，**不会**再写一个第 5 版。

**Acceptance Scenarios**：

1. **Given** 决定已写、`ApplyBody` 返回存储错误，**When** 查看，**Then** 效果 `failed`，`failure_code = storage`；重试成功后第二条效果 `done`。
2. **Given** 决定已写、版本已写、效果未写（测试钩子注入），**When** 重试，**Then** `ApplyBody` 凭幂等键 `search-suggestion:<suggestion_id>` 重放上次的版本；文档版本数恰好比采用前多 1；效果 `done` 指向那个版本。
3. **Given** 同一状态下两个重试并发，**When** 完成，**Then** 文档版本数仍恰好多 1，最多一条 `done` 效果。
4. **Given** 已有 `done` 效果的决定，**When** 重试，**Then** 409 点名 `decision_id`。
5. **Given** 决定写完后，别人抢先保存了新版本（写版本时基础版本已变），**When** `ApplyBody` 返回「基础版本已变」，**Then** 效果 `failed`，`failure_code = base_moved`。这是终局：重试会得到同样的结果，界面提示「请基于最新版本重写建议」。

---

### User Story 6 - 放弃一条建议：什么都不变 (Priority: P1)

小张放弃同事写的一条建议，写了一句原因。建议显示「已放弃」和放弃人、时间。作品版本、选题卡、简报、搜索主题、账号配置都没变。

**Why this priority**：D3「放弃不改任何东西」；D14-V15「拒绝建议不修改预算、配置或记忆」。

**Acceptance Scenarios**：

1. **Given** 放弃，**When** 完成，**Then** 只多一条决定（`abandon`）与一条审计；作品版本数、编辑副本、选题卡数、简报修订数、主题修订数、账号配置版本数全部不变。
2. **Given** 放弃路径，**When** 执行，**Then** 假适配器记录到**零次**写调用（`ApplyBody` 零次）。
3. **Given** 基础版本已不是最新的建议，**When** 放弃，**Then** 可以放弃（放弃不要求基础版本是最新）。
4. **Given** 已放弃的建议，**When** 想改主意，**Then** 只能新建一条建议（可以从它复制内容）；旧建议与旧决定保持原样。

---

### User Story 7 - 手工登记搜索指标：平台没给的就是未知 (Priority: P1)

第 4 版发布后第 7 天，小张在小红书后台看到这条笔记「搜索曝光 1,240」，但后台没有「搜索来源访问」这一项。他登记一条搜索曝光（统计窗口「发布后 7 天累计」，证据「笔记数据页截图 0928-1.png」）；搜索来源访问不登记。页面上这条发布记录显示「搜索曝光 1240（发布后 7 天累计）」「搜索来源访问：未知」。

**Why this priority**：R-060 第 5 条「只登记平台提供的」「缺少指标明确显示未知」；D14-V10「保留窗口」。

**Acceptance Scenarios**：

1. **Given** 一条发布记录，**When** 登记搜索曝光，**Then** 保存 `metric = search_impression`、`value`、`unit`、`stat_window`（必填）、`sampled_at`、`evidence_note`、`recorded_by`、`source_type = manual`。
2. **Given** 值为 nil，**When** 保存，**Then** 存为「未知」；为 0 时存为「确认为零」，两者读回不同。
3. **Given** 某指标从未登记，**When** 读取该发布记录的搜索指标，**Then** 那一项不出现在列表里；界面按「未登记 = 未知」显示，**不**显示 0。
4. **Given** `stat_window` 为空，**When** 保存，**Then** 400 点名 `stat_window`。
5. **Given** 发布记录不存在或属于别的品牌，**When** 保存，**Then** 与「不存在」同形拒绝。
6. **Given** `metric` 不是两项之一（例如 `search_rank`、`search_volume`），**When** 保存，**Then** 400 点名 `metric`。

---

### User Story 8 - 登记一次排名观察：每一次都只是一次 (Priority: P1)

10 月 2 日 21:30，小张用自己的手机、未登录、定位上海，在小红书搜「羊绒大衣能机洗吗」，按「综合」排序，这条笔记在第 7 位；他登记一条排名观察，证据写截图文件名。第二天在北京用同事的账号搜，前 30 条里没看到，他登记一条「前 30 条内未找到」。主题页的「排名」一栏列出这两次观察，每条带时间、查询词、条件、证据，旁边固定写「单次观察，不代表稳定排名或全平台排名」。**没有**「当前排名」「平均排名」「最好排名」。

**Why this priority**：R-060 第 5 条；D2；D14-V10「搜索观察保留窗口与条件」。

**Acceptance Scenarios**：

1. **Given** 一次观察，**When** 保存，**Then** 记下 `platform`、`query`、`observed_at`、`conditions`、`result_kind`、`position` 或 `scanned_depth`、`evidence_note`，以及可选的 `theme_id`、`publication_record_id`、`account_id`。
2. **Given** `result_kind = position`，**When** `position` 缺失或小于 1，**Then** 400 点名 `position`；`not_found` 而 `scanned_depth` 缺失或小于 1，**Then** 400 点名 `scanned_depth`；两者同时给 → 400 点名多出的那个。
3. **Given** `conditions`、`evidence_note`、`query` 任一为空，**When** 保存，**Then** 400 点名该字段。
4. **Given** 一个主题有多次观察，**When** 读取，**Then** 按 `observed_at` 倒序逐条列出；响应里**不存在**任何合并多次观察的字段（均值、最好、把最近一次当作「当前排名」）。
5. **Given** 主题没有任何观察，**When** 显示，**Then** 「排名：未知（还没有观察记录）」。
6. **Given** 一条观察记错了，**When** 作废，**Then** 产生一条 `voided` 修订，旧修订保留；列表默认不显示已作废的。

---

### User Story 9 - 旁观者看得到这一切都是人录的 (Priority: P3)

品牌负责人打开搜索优化页，所有主题、建议、指标、观察都标「人工」；页面上没有「AI 生成建议」「联网研究」「一键优化」按钮，只有一行灰字「AI 建议与联网研究将在后续版本提供；本版不联网」。

**Acceptance Scenarios**：

1. **Given** 任一建议，**When** 读取，**Then** `author_kind = human`；本版不存在别的取值。
2. **Given** 任一主题、指标、观察，**When** 读取，**Then** `data_origin = manual_only`。
3. **Given** 本卡的全部 Go 源文件，**When** 守卫扫描，**Then** 没有 `net/http` 客户端调用、没有任何执行器或模型包的 import（FR-005）。

---

### Edge Cases

- **主题的平台与账号不一致**：账号是抖音号、主题平台选小红书 → 400 点名 `platform`（主题挂了账号时，平台必须等于账号的平台）。
- **主题平台不在四个交付渠道里**（知乎、微博、B 站、快手）：主题可以建；这个平台登记不了搜索指标与排名观察（反馈领域的平台集是四个交付渠道），页面在指标与观察区写「该平台没有可登记的发布渠道」，不是 0（Q8）。
- **建议的目标文档被删除**：没有文档删除接口，只有工作区删除；工作区删除后一切拒绝，与「不存在」同形。
- **主题在建议之后被归档或改写**：建议里存的是当时的 `theme_revision` 与 `target_question` 原文，读建议时照常显示；页面另外标出「主题已有新修订 / 已归档」（读时派生）。
- **同一文档两条建议并发采用**：两个都通过预检查，先到的写出新版本；后到的在 `ApplyBody` 的行锁之后发现基础版本已变 → 效果 `failed/base_moved`。两个决定都在，一个 `done`、一个 `failed`，版本数只多 1。
- **建议所基于的版本不是最新，但内容恰好和最新一样**（例如有人恢复了旧版本）：仍按 `version_id` 判断为「基础版本已变」，不比正文——版本身份是审核与交付的依据，正文相同不代表是同一个版本。
- **正文很长**：差异按行计算；`proposed_body` 上限与 `work-editor.MaxBodyRunes`（200000 rune）相同。
- **关键词写法**：只做 NFC 与首尾空白规范化，去重按规范化后的逐字比较；不做大小写折叠、同义合并或分词。
- **观察时间在未来**：`observed_at` 比服务器时间晚 10 分钟以上 → 400 点名 `observed_at`（留出设备时钟的小误差）。
- **同一查询词同一时刻两次登记**：两条都存（可能是两台设备）；不去重，不合并。

---

## Requirements *(mandatory)*

### Functional Requirements

**不联网、不调模型（D1）**

- **FR-001**：本卡的所有路径 MUST NOT 发起任何出站网络请求、调用模型或启动执行器；主题、建议、指标、观察全部由人录入。
- **FR-002**：请求体里出现 `scope`、`budget`、`online`、`research_*` 一类联网研究字段 MUST 400 点名该字段（严格解码）；本版没有「本地 / 联网 / 全部」范围。
- **FR-003**：主题、指标、观察的响应 MUST 带 `data_origin = manual_only`（受控集，本版恰好一项）；建议的 `author_kind` MUST 是受控集，本版恰好一项 `human`。
- **FR-004**：AI 建议与联网研究是后续卡；本卡只在合同里留挂接键（contract §9），MUST NOT 为它们建表或留空列。
- **FR-005**：守卫用例扫本卡的全部 Go 源文件（`topic-planning/search_*.go`、`feedback-learning/search_*.go`、`work-editor/apply.go`、`handler/content_search_*.go`）：没有 `net/http` 的客户端调用，没有 `agent-workflow` / `agent-gateway` / 执行器包的 import。

**搜索主题（`topic-planning`，D1-a）**

- **FR-010**：搜索主题 MUST 修订制：`theme_id` + `revision`，只插不改；当前状态 = 最大修订；归档 = `voided = true` 的新修订。
- **FR-011**：主题字段 MUST 是：`name`（必填）、`platform`（`ipprofile.Platforms` 八项之一）、`account_id`（可空；非空时经 `AccountReader` 核实存在，且其平台 MUST 等于 `platform`）、`business_goal`（经营目标，自由文本）、`questions`（用户搜索问题，0～50 条）、`keywords`（关键词/主题组，0～100 个）、`intent`（受控集，contract §3）、`origin`（受控集恰好三项：`manual_keyword` / `customer_question` / `authorized_material`，对应 R-060「人工输入关键词、已有客户提问及授权素材」）、`origin_note`、`source_ids`（授权素材）、`topic_card_ids`、`brief_revision_ids`、`note`。
- **FR-012**：`questions` 与 `keywords` MUST 至少一项非空；`customer_question` MUST 有非空 `origin_note`；`authorized_material` MUST 有至少一个 `source_ids`。
- **FR-013**：`source_ids` MUST 经 `SourceReader.Exists` 逐个核实；`topic_card_ids`、`brief_revision_ids` MUST 在本模块逐个核实属于本工作区。任一不通过 MUST 400 点名该字段，不说明它在别处是否存在。
- **FR-014**：主题 MUST NOT 有搜索量、竞争度、排名的存储列。读主题时 `search_volume` 与 `competition` MUST 读时派生为 `{status: "unknown", reason: "no_data_source"}`；请求体带这些字段 MUST 400（严格解码）（Q5）。
- **FR-015**：主题列表 MUST 支持按 `platform`、`account_id`、`topic_card_id`（关联了该卡的主题）、`include_archived` 过滤；排序固定为 `name`、再按 `theme_id`，MUST NOT 按任何数值排序。
- **FR-016**：主题的修订历史 MUST 可读（每一修订的全部字段与 `recorded_by`、`created_at`），这是 D14-V09「来源可追溯」的承载。
- **FR-017**：关键词与问题 MUST 只做 NFC 与首尾空白规范化、去掉空项、按规范化结果去重（保留首次出现的顺序）；MUST NOT 做大小写折叠、同义合并、分词或扩写。
- **FR-018**：选题卡与简报的读路径 MUST NOT 因本卡而改变响应形状；「这张卡关联了哪些主题」由主题列表按 `topic_card_id` 过滤回答（FR-015），不在选题卡响应里加字段。
- **FR-019**：意图 `intent` 是人的判断；系统 MUST NOT 根据问题或关键词推断意图。

**优化建议（D1-b；D4）**

- **FR-030**：建议 MUST 修订制：`suggestion_id` + `revision`。一条建议 MUST 指向一个文档的一个版本：`work_id`、`artifact_id`、`base_version_id`（经适配器核实三者一致且存在）；这三项在修订之间 MUST NOT 改变（改基础版本 = 新建一条建议）。
- **FR-031**：建议 MUST 对准一个主题：`theme_id`（新建时 MUST 未归档）、服务端写入的 `theme_revision`（当时的当前修订）、`target_question`（必填，自由文本，≤500 rune）。
- **FR-032**：`aspects` MUST 是受控集 `title` / `body` / `topics` / `description` 的非空子集，不重复。四项都是目标文档**正文里的文字**；MUST NOT 修改 `Work.Title`、`Artifact.Title` 或任何非版本化字段（Q4）。
- **FR-033**：`rationale`（依据）MUST 非空；可选 `evidence_source_ids`（授权素材，同 FR-013 核实）。
- **FR-034**：`proposed_body` MUST 是修改后的整篇正文，≤ `work-editor.MaxBodyRunes`；与基础版本正文逐字节相同 MUST 400 点名 `proposed_body`。
- **FR-035**：建议的差异 `diff` MUST 由服务端纯函数 `DiffLines(base, proposed)` 读时计算（按 `\n` 切行的最短编辑脚本，输出 `equal` / `delete` / `insert` 三种操作与插入、删除行数），确定性：同一输入逐字节相同。基础版本正文经适配器读取（版本只插不改，所以读时计算与存副本等价）。
- **FR-036**：建议、比较、主题的任何请求与响应 MUST NOT 有分数、排名预测、关键词密度、关键词出现次数、「SEO 得分」一类字段；反射用例扫字段名。系统 MUST NOT 计算关键词在正文里出现的次数（这会把人往堆砌引）。
- **FR-037**：页面 MUST 在建议区固定显示：建议的目标是准确回答目标问题；不以关键词堆砌替代内容质量；本工作台不保证搜索排名。
- **FR-038**：建议的当前状态 MUST 读时派生（contract §5）：`open`、`adopted`、`adopt_failed`、`adopt_unrecorded`、`abandoned`；另派生 `base_is_current`（基础版本是否仍是该文档最新版本）与 `theme_changed`（主题是否已有更新修订或已归档）。都 MUST NOT 落存储。
- **FR-039**：建议的修改 MUST 带 `base_revision`，不符 → 409；已有决定的建议 MUST NOT 再写修订（409 点名 `suggestion_id`）。
- **FR-040**：比较 MUST 是只读端点：2～4 个 `suggestion_id`，返回每条的当前修订、`diff`、`base_is_current` 与整体 `same_base`；按 `created_at`、`suggestion_id` 排列；MUST NOT 排序打分或给出推荐。
- **FR-041**：建议列表 MUST 支持按 `work_id`、`artifact_id`、`theme_id`、派生状态过滤。

**采用与放弃（D3；D14-V10）**

- **FR-050**：对一条建议，用户 MUST 能做且只能做一次决定：`adopt` 或 `abandon`，带可选说明与建议的当前 `revision`（不符 → 409 点名 `revision`）。决定只插不改；同一建议已有决定 → 409 点名 `suggestion_id`（唯一索引兜底）。
- **FR-051**：`abandon` MUST 只写决定与审计；MUST NOT 调用任何适配器的写方法，MUST NOT 写本模块的效果表。放弃不要求基础版本是最新。
- **FR-052**：`adopt` 的**预检查**（写决定之前）MUST 经适配器读文档当前状态：最新版本 = `base_version_id`（否则 409 点名 `base_version_id`）、编辑副本 `draft_status = saved`（否则 409 点名 `draft_status`）。预检查失败 MUST NOT 写决定。
- **FR-053**：`adopt` 通过预检查后 MUST 按顺序：① 写决定（本模块事务）；② 经适配器调用 `work-editor.ApplyBody`，带幂等键 `search-suggestion:<suggestion_id>`；③ 写效果：成功 `done` 带新 `version_id`，失败 `failed` 带原因码（contract §3 `EffectFailures`）。② 与 ③ 之间的失败由重试收敛（FR-055）（Q6）。
- **FR-054**：`ApplyBody` MUST 在一个事务里：取删除栅栏 → 幂等 `Claim`（同键重放直接返回上次的版本，不再核对下面几项）→ 审计 → 锁文档 → 核对最新版本 = `BaseVersionID`（否则 `ErrBaseMoved`）→ 核对 `draft_status = saved`（否则 `ErrDraftUnsaved`）→ 核对正文与基础版本不同（否则 `ErrNoChange`）→ 按 `appendVersion` 同一套规则取号并插入新版本（`source = edited`，`action = suggestion_applied`，Q3）→ 编辑副本改为新正文、`saved` → 幂等 `Complete`。
- **FR-055**：没有效果记录、或最新效果为 `failed` 的采用决定 MUST 可重试（`POST /decisions/{decisionId}/retry`），重试用同一个幂等键；已有 `done` 效果 → 409 点名 `decision_id`。一条建议 MUST 至多产生一个新版本，无论重试几次、是否并发（SC-004）。
- **FR-056**：采用产生的版本 MUST 有新的 `version_id`，且正文与基础版本不同。这是本版对「使旧预检报告按既有规则过期」的全部承担（Current State §1）：报告按 `version_id` 绑定，新 `version_id` 出现即旧报告相对当前稿过期；正文不同保证这不是「无变化保存」。
- **FR-057**：本卡 MUST NOT 写 `review-delivery` 的任何东西：不复制、不关联、不修改审核请求、交付任务或发布记录；新版本的审核状态 MUST 读回「没有审核请求」（`LatestReviewStatusFor` 返回 `false`）。要交付新版本，必须重新提交审核。
- **FR-058**：采用 MUST NOT 修改选题卡、简报、主题、账号配置、经营规则或任何设置；采用的唯一跨模块写是 `ApplyBody`。
- **FR-059**：每个决定、效果 MUST 在同一事务写审计（`AuditTx`）；审计失败整体回滚。
- **FR-060**：`work-editor` 的版本历史 MUST 能看出一个版本来自建议采用：`action = suggestion_applied`；前端版本列表的动作文案 MUST 覆盖这个新值（zod 枚举同步，未知值走兜底）。版本行里 MUST NOT 存建议 id（`work-editor` 不知道搜索优化存在）；「这个版本来自哪条建议」由建议的效果记录回答。

**搜索指标与排名观察（`feedback-learning`，D1-c；D2）**

- **FR-070**：搜索指标 MUST 存在本卡新表 `content_search_metric`（Q2），字段与 027 的 `content_manual_metric` 同形：`publication_record_id`（必填，经既有 `feedbackPublications` 核实）、`platform`（反馈领域四项）、`account_id`、`metric`、`value`（指针：nil = 未知，0 = 确认为零）、`unit`、`stat_window`（**必填**）、`sampled_at`、`evidence_note`、`recorded_by`、`source_type`（服务端写，本版只有 `manual`）。只插不改。
- **FR-071**：`metric` MUST 是受控集**恰好两项**：`search_impression`（搜索曝光）、`search_visit`（搜索来源访问）。平台后台没有给出的指标就不登记；MUST NOT 有 `search_volume`、`search_rank`、`competition` 或 `other`。
- **FR-072**：读一条发布记录的搜索指标 MUST 原样列出每一条采样；MUST NOT 合计、平均或只返回「最新值」。界面对没有任何采样的指标显示「未知」，对 nil 显示「未知」，对 0 显示 0。
- **FR-073**：排名观察 MUST 修订制（`observation_id` + `revision`，作废是新修订），字段：`platform`（四项）、`account_id`（可空，经适配器核实）、`query`（必填，≤200 rune，NFC + 去首尾空白后保存）、`theme_id`（可空，经适配器向 `topic-planning` 核实存在）、`publication_record_id`（可空，经 `feedbackPublications` 核实）、`observed_at`（必填，不得晚于服务器时间 10 分钟以上）、`conditions`（观察条件，必填）、`result_kind`、`position` / `scanned_depth`、`evidence_note`（必填）。
- **FR-074**：`result_kind` MUST 是受控集**恰好两项**：`position`（出现在第 `position` 位，`position ≥ 1`）、`not_found`（在前 `scanned_depth` 条内没有找到，`scanned_depth ≥ 1`）；两个整数字段 MUST 恰好给一个。
- **FR-075**：观察列表 MUST 按 `observed_at` 倒序、再按 `observation_id` 逐条返回；MUST NOT 有均值、中位数、最好名次、「当前排名」或任何跨观察的合并字段；每条观察 MUST 带规则 id `rank.single_observation`，前端译为「单次观察，不代表稳定排名或全平台排名」。
- **FR-076**：主题没有任何未作废的观察时，界面 MUST 显示「排名：未知（还没有观察记录）」；这是页面按观察列表为空得出的，不是存储的值。
- **FR-077**：`feedback-learning` 的 SQL MUST 继续满足 027 包级守卫（不聚合）；两个新受控集登记进 `TestThereIsNoSixthControlledSet`。
- **FR-078**：`feedback-learning` MUST NOT import `topic-planning`；`theme_id` 的核实经 handler 适配器（contract §7.3）。

**未知与不承诺（D2；D14-V09）**

- **FR-080**：「未知」MUST 是显式的受控原因（`no_data_source`、`no_observation`、`not_recorded`），MUST NOT 用 0、空字符串、`null` 数字或省略来表示。
- **FR-081**：任何响应、规则 id、四语言文案 MUST NOT 出现排名承诺或排名预测：「保证排名」「上首页」「提升排名」「排名第一」及英文 `guarantee`、`boost ranking`、`top rank`（守卫扫 Go 字符串字面量与本页 i18n 键；只放行 `optimization.no_ranking_promise` 这一条否定句）。
- **FR-082**：单次观察 MUST NOT 被任何界面元素呈现为「排名」本身（例如主题卡片上的一个大号数字）；排名一栏只能是观察列表或「未知」。
- **FR-083**：搜索指标与排名观察 MUST NOT 与 027 的普通指标、034 的 ROI、035 的诊断合并计算；本卡不改那三处的任何计算。
- **FR-084**：D14-V09「本地模式不联网」在本版的含义：本版只有本地（人工）模式，FR-001 与 FR-005 保证没有任何联网路径。

**授权与共同约束（D5、D6）**

- **FR-100**：所有端点 MUST 先经 `workspace-core.Authorize`（成员资格），拒绝与「不存在」同形，同类拒绝体逐字节相同（`trace_id` 除外）。
- **FR-101**：跨模块的读写 MUST 只经 handler 适配器：模块定义小接口（只用本模块类型与字符串），`handler/content_search_*.go` 实现。`topic-planning` MUST NOT import `work-editor` / `feedback-learning`；`feedback-learning` MUST NOT import `topic-planning`；`work-editor` MUST NOT 新增任何内容模块 import。守卫用例扫 import，`pnpm check:content-boundaries` 也会挡。
- **FR-102**：模块 SQL MUST NOT 读写别的模块的表。
- **FR-103**：每条写路径 MUST 在同一事务里先取工作区删除栅栏，再写业务行，再写审计。
- **FR-104**：每个适配器方法 MUST 把「目标不存在」与「工作区已删除 / 不属于调用者」（`workspacecore.ErrNotFound`、对方模块的 `ErrNotFound` / `ErrInvalid`）映射为**调用方模块的** `ErrNotFound`，其余错误映射为 `ErrStorage`；每个适配器方法各有一条「工作区删除已提交后」的用例，断言 HTTP 404 而不是 503。
- **FR-105**：新表 MUST 遵守 R1–R6；每张表有一个以 `workspace_id` 打头的索引；MUST 进删除清单与删除链。
- **FR-106**：新表 MUST 只插不改（修订制或纯追加）；守卫用例扫本卡源文件：没有针对这些表的 `UPDATE`、`ON CONFLICT DO UPDATE`、删除链之外的 `DELETE`，且每张表都有 `INSERT INTO`。
- **FR-107**：带路径参数的端点 MUST 各有一条穿过真实 router 与中间件、路径参数值 ≠ 上下文值的用例；所有新端点 MUST 挂进 `server/cmd/server/router.go`，并有路由存在性用例（`server/cmd/server/content_search_routes_test.go`）。
- **FR-108**：迁移号不预留；每个实施 PR 合入前把自己的迁移改号为紧接当时 `app-main` 最大号之后的连续号。

**界面（D7）**

- **FR-110**：新页面 `/{workspaceSlug}/search-optimization`，分「搜索主题」「优化建议」「搜索表现」三个区块，只用既有 Multica 组件与 `--text-*` 字号，四语言。
- **FR-111**：MUST NOT 写 UI 单测；界面项进 `manual-ui-todo.md`，一律记「未执行」。
- **FR-112**：「未知」MUST 用中性的次要文字色，不用报错色，也不用红黄绿表示好坏。
- **FR-113**：建议的差异 MUST 用上游已有的删除/插入语义 token；上游没有时，用删除线与下划线表示，不新建颜色。
- **FR-114**：页面从作品编辑器跳转进来时带 `work_id` 与 `artifact_id` 过滤；作品编辑器页只加一个链接，不嵌入建议区（宪法 VIII）。
- **FR-115**：页面上 MUST NOT 有「AI 生成建议」「联网研究」「一键优化」「预测排名」一类入口；只放一行说明「AI 建议与联网研究将在后续版本提供；本版不联网」。

### Key Entities

- **搜索主题修订（Search Theme Revision）**：`topic-planning`。一个主题组：平台、账号、经营目标、问题、关键词、意图、来源与依据、关联的素材/选题卡/简报。
- **建议修订（Suggestion Revision）**：`topic-planning`。对准一个主题的一个问题，指向一个文档的一个版本，带涉及项、依据与修改后的整篇正文。
- **决定（Decision）**：`topic-planning`。对一条建议的采用或放弃，一条建议一个。只插。
- **效果（Effect）**：`topic-planning`。采用之后真正发生了什么：写出了哪个版本，或失败原因。只插。
- **差异（Diff，值对象）**：读时计算的按行编辑脚本。
- **搜索指标（Search Metric）**：`feedback-learning`。一次人工采样：发布记录、指标、值（可未知）、统计窗口、证据。只插。
- **排名观察修订（Rank Observation Revision）**：`feedback-learning`。一次人工观察：平台、查询词、时间、条件、结果、证据。
- **应用正文（`work-editor.ApplyBody`，新公开入口）**：以给定正文、核对基础版本，写一个新版本。

---

## Success Criteria *(mandatory)*

- **SC-001**（D14-V09 可追溯）：主题创建 → 修订 → 归档，三个修订逐字节可读；来源三种各一条样例（含 FR-012 的三条 400）；引用别的品牌的素材 / 选题卡 / 简报各一条，拒绝不泄露存在性。
- **SC-002**（D14-V09 未知）：任一主题读回 `search_volume`、`competition` 为 `unknown/no_data_source`；请求体带 `search_volume`、`rank`、`scope`、`budget` 各一条 400；反射扫主题类型无这些存储字段；迁移里没有这些列。
- **SC-003**（D14-V10 比较）：同一基础版本三条建议 → 比较返回三条、`same_base = true`、按创建时间排列、无推荐字段；比较前后全部表行数不变。
- **SC-004**（D14-V10 采用，真实 DB）：采用 → 文档版本数 +1、新版本 `action = suggestion_applied`、正文 = `proposed_body`、效果 `done`；「版本已写、效果未写」后重试 → 版本数仍只 +1；两个重试并发 → 同样只 +1；已 `done` 再重试 → 409。
- **SC-005**（D14-V10 预检查）：基础版本不是最新 → 409 `base_version_id` 且决定表行数不变；编辑副本未保存 → 409 `draft_status` 且不写；决定之后被抢先保存 → 效果 `failed/base_moved`，版本数只多抢先的那一个。
- **SC-006**（D14-V10 不沿用旧终审，真实 DB，handler 包）：第 3 版提交审核并批准、建交付任务 → 采用建议得第 4 版 → `LatestReviewStatusFor(第 4 版)` 为「没有」；第 3 版的审核请求、交付任务、发布记录逐字节不变；提交第 4 版得到新的 `pending` 请求。
- **SC-007**（D14-V10 旧报告）：守卫：本卡源文件不写 `content_review_*`、`content_delivery_*`、`content_publication_*`；`ApplyBody` 产生的 `version_id` 与基础版本不同、正文不同（FR-056 的两半各一条断言）。预检报告本身**无法测试**（EP-06 未实现），在 checklist 里如实记录。
- **SC-008**（D3 放弃；D14-V15 后半）：放弃 → 除决定与审计外，本卡全部表行数不变；作品版本数、编辑副本、选题卡数、简报修订数、主题修订数、账号配置版本数不变；假适配器零次写调用。
- **SC-009**（D14-V10 观察保留窗口与条件）：搜索指标缺 `stat_window` → 400；排名观察缺 `conditions` / `evidence_note` / `observed_at` 各一条 400；读回逐字段相同。
- **SC-010**（D2）：同一主题三次观察 → 列表三条，按时间倒序；反射扫观察与主题的响应类型，无 `avg`、`best`、`current_rank`、`rank` 一类汇总字段；nil 指标读回 nil，0 读回 0。
- **SC-011**（D4）：反射扫建议、比较、主题的请求与响应类型，无 `score`、`density`、`keyword_count`、`seo`；守卫扫 Go 字符串字面量与四语言本页文案，无排名承诺词（FR-081）。
- **SC-012**（D5、D6）：迁移规则 R1–R6 全绿；删除工作区后每张新表该工作区行数为 0；删除已提交后每条写路径被栅栏拒绝且不留半条记录；**每个适配器方法**一条「删除后 → 404」用例；每个带路径参数的端点有穿过真实中间件、参数 ≠ 上下文的用例；路由存在性用例覆盖每个新端点；import 守卫覆盖三个模块。
- **SC-013**（D1）：守卫：本卡 Go 源文件无出站请求、无执行器 import；`author_kind` 恰好 `human`，`data_origin` 恰好 `manual_only`。
- **SC-014**（D14-V15，**部分**）：一条服务端链路用例（handler 包，真实库）：建主题 → 建选题卡与作品、存第 1 版 → 以第 1 版写建议 → 采用得第 2 版 → 提交第 2 版审核并批准 → 建交付任务、登记发布记录 → 登记一条搜索曝光与一条排名观察（关联主题与发布记录）→ 按主题读观察、按发布记录读指标，都能追到第 2 版。另一条：放弃一条建议，D14-V15「拒绝建议不修改预算、配置或记忆」中本卡能触及的部分全部不变（SC-008）。「手工反馈及成本/线索/成交 → 复盘 → 人工采纳」由 034/035 的链路覆盖；浏览器全程由 `manual-ui-todo.md` 的 036-U-30 覆盖，记「未执行」。

---

## D14-V15 覆盖说明（D7）

| 链路一段 | 由谁交付 | 本卡 |
|---|---|---|
| **关键词 → 搜索主题** | **036** | **闭合**（人工；联网研究是后续卡） |
| **主题 → 优化候选（建议）→ 采用为新版本** | **036** | **闭合** |
| 新版本 → 人工审核 → 交付 → 发布记录 | 024 / 025 | 只读；SC-006、SC-014 串起来 |
| **发布记录 → 手工搜索指标与排名观察** | **036** | **闭合** |
| 手工反馈（普通指标、摘录） | 027 | 不动 |
| 成本 / 线索 / 成交 → ROI 复盘 | 034 | 不动 |
| 经营诊断 → 人工采纳 | 035 | 不动 |
| 「拒绝建议不修改预算、配置或记忆」 | 035（诊断建议）、036（搜索建议的放弃） | 本卡的放弃：SC-008 |
| AI 分析（工作流末端「→ AI 分析」） | 后续卡（宪法 IX） | 不做 |

**自动化能证明的**：本卡三段各自的服务端契约；SC-014 把「主题 → 建议 → 采用 → 审核 → 发布记录 → 搜索观测」串成一条服务端用例；放弃不改任何东西（SC-008）。

**只能手验的**：浏览器从搜索主题一路点到审核、登记搜索表现，再去 ROI 复盘与经营诊断页（036-U-30）。

**不适用 / 未执行**：「AI 分析」与「联网研究」本版没有；D14-V09「本地模式不联网」以守卫证明（SC-013），真实联网另验。

---

## Assumptions

1. **数据都是人录的**。本卡不接平台接口、不抓数据、不调模型、不联网。
2. **一个品牌 = 一个工作区**；工作区成员都可以建主题、写建议、做决定、登记观测，与 027 / 034 / 035 相同。
3. **标题、话题、描述都在文档正文里**（Q4）。本卡不改作品或文档的标题列。
4. **规模**：一个品牌几十到几百个主题；一个文档同时开着的建议几条；观察每月几十到几百条。列表在 Go 里过滤，不需要为 jsonb 数组建 GIN 索引。
5. **预检报告不存在**（Current State §1）。本卡的「使旧报告过期」只做到结构性保证与合同约定。

---

## Out of Scope（后续卡）

- **AI 生成建议**、**联网研究**（本地 / 联网 / 全部范围、来源、执行预算）、**AI 分析**搜索表现（D1）。
- 搜索量、竞争度的数据接口；人工录入搜索量（Q5=A 时）。
- 搜索指标的 CSV 批量导入（027 有 `csv_import`；本卡只做单条手工登记）。
- 把搜索指标接进 035 经营诊断的表现维度（FR-083）。
- 作品标题、话题、描述的结构化版本字段（Q4=A 时）。
- 预检运行与报告（EP-06）。
- 修 `HoldForChangedTarget` 没有调用方（Current State §4）。
- 修 027 `feedbackPublications` 把存储错误映射成 404（Current State §7）。
- 作品编辑器页内嵌建议区（FR-114 只加链接）。

---

## 本规格补的设计（原文没有直接给，改起来的代价写在后面）

1. **建议 = 修改后的整篇正文 + 读时差异**（FR-034、FR-035）。原文说「输出可编辑建议、依据和修改差异」；存整篇正文让采用变成「写一个版本」这一件事，差异由正文与不可变的基础版本算出，不会与正文不一致。代价：长文每条建议多存一份正文。
2. **一条建议一个决定**（FR-050）。比 035 的「一个修订一个决定」简单：搜索建议没有「改完再决定」的需要，放弃后要重来就新建。代价：改主意要复制一条。
3. **采用前先预检查，再记决定**（FR-052）。把「基础版本已变」「有没保存的草稿」这两种最常见的失败挡在决定之前，不留一个注定失败的采用决定；剩下的只有竞态（US5 场景 5）。
4. **`base_version_id` 按版本身份比较，不比正文**（Edge Cases）。审核与交付都认版本身份；正文相同的两个版本在审核上仍是两个版本。
5. **意图受控集**（contract §3 `SearchIntents`，Q7）。原文只说「搜索意图」；给六项（含「未分类」）是为了能按意图筛选；不从文字推断（FR-019）。
6. **排名观察的结果只有两种**：第几位，或「前 N 条内没找到」（FR-074）。「没找到」必须带看了多少条，否则「没找到」与「没看」分不开。
7. **排名观察的平台只用四个交付渠道**（Q8）。反馈领域的平台集就是这四个，发布记录也只在这四个渠道上；主题可以建在另外四个平台上，但那里登记不了观测。
8. **`stat_window` 对搜索指标必填**（FR-070）。027 里它可以空；D14-V10 明写「搜索观察保留窗口」，所以本卡收紧。

---

## 待裁决问题（主控请逐条裁定；每题的推荐值已经写进上面的 FR，裁定不同则相应 FR 要改）

| # | 问题 | 选项 | 推荐 | 影响 |
|---|---|---|---|---|
| **Q1** | 建议、决定、效果放哪个模块（D5） | **A** `topic-planning`，与搜索主题同处；采用经 handler 适配器调 `work-editor.ApplyBody`，三步 + 幂等键收敛 / **B** `work-editor`：文档 12 §2 写它独占「候选修改」，采用在一个事务里完成，没有跨模块窗口；主题经适配器只读核实 | **A**。D3 明写「经作品编辑器公开接口、由 handler 适配器」，A 正是这个形状；文档 14 把搜索主题放在选题领域，建议紧挨着它，按主题查建议、比较、依据都在同一模块。`appendVersion` 已支持幂等键，A 的跨模块窗口用现成机制收敛。B 的好处是原子，代价是 `work-editor` 要长出「主题」「问题」「依据」这些搜索概念，还要一个新适配器反向核实主题 | 文首「模块」表；FR-030～FR-060；contract §1.2～§1.4、§7；plan.md「主控决定」第 1 条 |
| **Q2** | 搜索指标存哪 | **A** `feedback-learning` 新表 `content_search_metric`，自己的两项受控集 / **B** 把两项加进 027 的 `Metrics`（十一 → 十三），复用 `content_manual_metric` | **A**。027 的 `Metrics` 注释写明是 SOP 10.1 的恰好十一项、没有 `other`；035 的表现维度按 `Metrics` 遍历，B 会改变 035 已生成报告的输入集合与复算基础。A 同形照抄，读写代码可以共用一半 | FR-070～FR-072；contract §1.5 |
| **Q3** | 采用写出的版本用什么动作 | **A** `work-editor` 的 `Actions` 加 `suggestion_applied`（一个 CHECK 迁移，照 535）/ **B** 沿用 `saved` | **A**。SOP 7.1 要「来源与动作记录」；记成 `saved` 会让版本历史说「某人手写保存了这一版」，事实是采用了一条建议。代价是 `work-editor` 一个迁移和前端枚举同步 | FR-054、FR-060；contract §7.4 |
| **Q4** | 标题、话题、描述怎么表示 | **A** 都是目标文档正文里的文字，`aspects` 只是标签 / **B** 给 `work-editor` 加结构化、版本化的标题/话题/描述字段 | **A**。今天版本只有正文（Current State §2），渠道稿本来就把这些写在正文里；B 是 `work-editor` 的大改，会波及 024、025、031 的读写与页面 | FR-032；Assumptions 3 |
| **Q5** | 搜索量与竞争度 | **A** 不存，恒为「未知 / 没有数据来源」，请求带了就 400 / **B** 允许人工录入并附证据 | **A**。R-060：「没有搜索数据接口时不得虚构」；人从第三方工具抄的数值口径各异，一旦存了就会被当成数据用。等数据接口或联网研究落地时再定 | FR-014；SC-002 |
| **Q6** | 采用的三步顺序 | **A** 预检查 → 记决定 → `ApplyBody`（幂等键）→ 记效果（同 035 Q3）/ **B** 先 `ApplyBody` 再在一个事务里记决定与效果 | **A**。先记决定，保证「采用有动作记录」在任何失败下成立；B 会出现「版本已写、建议仍显示未决定」的窗口，此时若有人点放弃，就得反查 `work-editor` 才能发现矛盾 | FR-052～FR-055 |
| **Q7** | 搜索意图 | **A** 受控集六项：`learn` 了解、`solve` 解决具体问题、`compare` 比较选择、`buy` 购买/预约/到店、`find` 找品牌/账号/门店、`unclassified` 未分类 / **B** 自由文本 | **A**。能筛选、能在页面上一致显示；有「未分类」，人不必硬选 | FR-011、FR-019；contract §3 |
| **Q8** | 指标与观察的平台集 | **A** 反馈领域的四个交付渠道；主题可在八个账号平台上建 / **B** 观察也放开到八个平台 | **A**。发布记录只在四个渠道上；B 要给 `feedback-learning` 加一个与 027 不同的平台集，并让没有发布记录的观察成为常态 | FR-073；Edge Cases |

另有一条规模问题留给派单：PR 2 若审查不便，可拆成 2a（建议、比较、放弃，本模块全部迁移）与 2b（`work-editor.ApplyBody`、动作迁移、采用与重试）。见 plan.md。
