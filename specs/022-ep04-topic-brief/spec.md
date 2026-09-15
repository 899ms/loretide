# Feature Specification: 手工选题卡与冻结简报（EP-04a）

**Feature Branch**: `claude/spec-022-ep04-topic-brief`

**Created**: 2026-09-15

**Status**: Draft

**Input**: EP-04 拆分方案的首切片，见 [ep04-breakdown.md](./ep04-breakdown.md)。用户 2026-09-15 定 EP-04 不再等 W-03：以粘贴文本与 URL 为输入即可开工。

**SOP 对应**：`docs/01-完整工作流.md` §5.2「一张选题卡必须说清楚」与 §5.3「冻结创作简报」。

**模块**：`topic-planning`。`docs/12` §2 已把「**选题、理由、创作简报**」列为该模块独占维护的数据。

## Current State（以代码为准）

核实自 `app-main` @ `5d6d47f`。

### 1. 模块与边界已经登记好了，目录还是空的

`scripts/content-boundaries.json` 里 **`topic-planning` 已经是注册模块**，依赖声明为 `workspace-core` / `ip-profile` / `knowledge-base` / `diagnostics`。

但 `server/internal/content/` 下**只有三个目录**：`diagnostics`、`ip-profile`、`workspace-core`。`pnpm check:diagnostics-contract` 当前报 `checked 3 landed modules; skipped 9 not yet landed`。

**后果**：本卡新建 `server/internal/content/topic-planning/` 的那一刻，该目录**立刻受接入合同约束**，落地模块数由 3 变 4。合同三条不是交付后再补的东西，是这张卡的一部分。

`knowledge-base` 在依赖声明里，但 EP-04a **不用它**——声明的是允许方向，不是必须使用。

### 2. 版本表的做法已有现成模板

`content_account_revision`（迁移 479）与它的两个 CONCURRENTLY 索引（480 / 481）就是 `ContentBriefRevision` 要照抄的形状：

```text
revision_id  text PRIMARY KEY   -- 被别处引用的键
account_id   text
workspace_id text
revision     bigint             -- 每账号自增，给人读与排序，不是跨表键
created_at   timestamptz
```

迁移注释里已经写清了为什么它 **append-only**：「验收要求旧版本保持可读，且钉了某一版的运行继续看到它钉的那一版。可被 UPDATE 的历史两样都给不了；任何读改写都会悄悄改写过去。」

**这段话对简报版本同样成立**，而且是 §5.3 的原话要求。本卡照抄这个形状，包括「`revision_id` 是被引用的键、`revision` 只是给人读的计数器」这条区分。

### 3. 授权入口已经有了，并且已经在记诊断

`server/internal/content/workspace-core/authz.go:90` 的 `Authorize(ctx, members, recorder, actor, workspace, allowedRoles...)`：

- **每次调用都读成员关系，不缓存**——「被移除的那一刻就出局」；
- 查表出错与「不是成员」**返回同一个拒绝**，使错误不能被用来探测；
- 每次拒绝都写一条技术诊断事件。

`http.go` 的 `RefusalStatus` / `RefusalBody` 负责把拒绝翻成状态码与响应体。**本卡不自己判权限，也不自己造 404。**

### 4. 快照形状已经定了，但本卡还用不上

`diagnostics/contract.go:51` 的 `Snapshot` 有十六个字段，其中 `Required` / `Excluded` / `Scope` / `Preference` 与选题链路直接相关。

**本卡不写快照**——固定输入快照是 EP-04b。这里只需要保证简报版本 id 是一个**能被快照引用的稳定键**。

### 5. 「既有运行继续引用旧版」今天验不了

§5.3 要求「用户修改简报时生成新版本，已有运行继续引用它开始时的版本」。

**今天没有运行**：`agent-workflow` 模块未落地，`server/internal/content/` 下没有它的目录。所以这条要求在本卡只能由**结构**保证——版本表 append-only、`revision_id` 是被引用的键——而**运行侧的断言无处可写**。

**裁决的验收口径**（主任务 2026-09-15）：**结构保证**——简报版本只插不改，运行以 `brief_revision_id` 钉住；**行为断言留 agent-workflow 落地时补**。见 FR-018。

### 6. 上游 `issue` 是什么形状（Q1 的事实依据）

`server/pkg/db/generated/models.go:826` 的 `Issue` 有 **29 个字段**，其中与选题卡无关但会被一并继承的有：`Position`（看板排序）、`ParentIssueID`、`AssigneeType` / `AssigneeID`、`CreatorType` / `CreatorID`、`Stage`、`ProjectID`、`Number`、`OriginType` / `OriginID`、`FirstExecutedAt`、`LastActivityAt`、`Revision`。

它周边还挂着 `issue_dependency`、`issue_label`、`issue_reaction`、`agent_task_queue`（通过 `issue_id` 认领任务）、活动日志与小时级汇总触发器；工作区删除事务里 `LockWorkspaceTaskOwnerIssues` 会对它 `FOR UPDATE` 加写栅栏。

**这些都是真实存在的耦合**，不是假设。见 Q1。

### 不在本功能范围

不调模型；不读本地文件、素材包或知识卡；不做候选选题自动生成（EP-04c）；不做必用 / 排除（EP-04d）；不固定输入快照（EP-04b）；不写 UI 单测；不碰 `server/internal/daemon` 与上游 Multica 其它代码。

## Clarifications

### Session 2026-09-15（**主任务已裁决：Q1 = B，Q2 / Q3 / Q4 = A**）

- **Q1（任务卡点名）复用上游 `issue` 加 content 语义，还是新建 content 表？**
  → **裁决 B：新建 `topic-planning` 自己的表。**

  | | 复用上游 `issue` | 新建 content 表 |
  |---|---|---|
  | 迁移 | 加列或用 `properties` jsonb | 两张新表 + CONCURRENTLY 索引 |
  | 上游改动 | **要**，走 workflow 第 13 步；且不是一次性——每次加选题字段都要再动一次上游表 | 不要 |
  | `docs/12` §2 归属 | **冲突**：§2 把「选题、理由、创作简报」判给 `topic-planning`，而 `issue` 不归任何 content 模块；写它就是跨模块写表，每个 PR 都要解释一次 | 与 §2 一致 |
  | 一并继承的东西 | 看板 `Position`、`issue_dependency` / `issue_label` / `issue_reaction`、`agent_task_queue` 的认领、活动汇总触发器、工作区删除的 `FOR UPDATE` 栅栏 | 无 |
  | 白得的东西 | 看板、标签、评论、指派 | 无（需要时再单独做） |

  **推荐 B 的理由不是「新表更干净」，是 §2 的归属表已经判过了。** 复用 `issue` 要先改 §2，那是文档仓库的决定，不是这张卡能顺手做的。代价诚实写出来：看板与标签这些能力不白得，将来若要「在看板上看选题」得另做。

- **Q2 选题卡挂在品牌上还是品牌 + 账号上？**
  → **裁决 A：挂品牌，账号引用可空。**
  §5.2 有「为什么适合这个 IP」，需要指向账号；但选题在定渠道之前**可能还没选定账号**，强制必填会逼人瞎填。另外 `content_diagnostics.go:36` 的 `diagnosticScope` 今天把 `scope.Accounts` 置空，带 `account_id` 的诊断运行一律 403（`specs/018` 已记），所以账号维度的链路本来也还没通。

- **Q3 暂缓 / 放弃的「偏好信号」存哪里？**
  → **裁决 A：存在选题卡自己这一行上**（状态 + 快捷原因 + 自由备注），**不建单独的信号表、不做任何聚合**。
  §5.2 明写「这些是偏好信号，单次操作不永久改变 IP 定位」。**聚合与采纳属于 `feedback-learning`**（§2：「人工指标、AI 复盘、待采纳 / 已采纳经营结论」）。本卡只存事实。

- **Q4 EP-04a 要不要再拆成「选题卡」与「冻结简报」两张卡？**
  → **裁决 A：不拆，一张卡两个 PR**（存储与接口 PR → 页面 PR，按 SOP 阶段规则）。
  两者的生命周期是连着的——「开始」这个动作同时改卡状态并产出简报首版，拆开会让这个动作横跨两张卡。代价是 EP-04a 比其余三张大一些。

## User Scenarios & Testing *(mandatory)*

### User Story 1 - 把一个想法写成一张说得清楚的选题卡 (Priority: P1)

运营者手打一张选题卡，七件事逐项写下来；写不出的那项能如实说明「没有」，而不是被留空卡住。

**Why this priority**: 这是 EP-04 链条的入口。没有选题卡，简报、快照、运行都无从谈起。

**Independent Test**: 新建一张卡，七项各填一句，保存，刷新后逐项读回。

**Acceptance Scenarios**:

1. **Given** 一个品牌，**When** 新建选题卡并填齐七项，**Then** 保存成功且刷新后逐项读回。
2. **Given** 一个没有时效依据的选题，**When** 在「为什么是现在」写「无时效依据」，**Then** 正常保存——**MUST NOT** 因为该项没写出时效窗口而拒绝。
3. **Given** 另一个品牌，**When** 读这张卡，**Then** 读不到，拒绝经 workspace-core。

---

### User Story 2 - 四个动作，其中两个要留下原因 (Priority: P1)

开始 / 收藏 / 暂缓 / 放弃。暂缓与放弃要问一句为什么，答案记下来但**不改变 IP 定位**。

**Why this priority**: §5.2 的动作集是选题卡存在的意义——卡本身只是载体，决定才是产物。

**Acceptance Scenarios**:

1. **Given** 一张卡，**When** 依次执行四个动作，**Then** 状态各自改变且可读回。
2. **Given** 选择暂缓或放弃，**When** 提交，**Then** 记下快捷原因与自由备注；**Then** 账号的人设、范围偏好、任何 IP 配置**逐字节未变**。
3. **Given** 选择「开始」，**When** 提交，**Then** 产出一个简报版本。

---

### User Story 3 - 简报改一次就多一版，旧版永远还在 (Priority: P1)

简报可以改，但改是**追加**不是覆盖；任何一个旧版本都还能原样读出来。

**Why this priority**: §5.3 的硬要求。一份能被覆盖的简报，等于没有「冻结」可言——而后面整条链路（快照、运行、复现、审核）都建立在「那一版是什么」可以被回答之上。

**Acceptance Scenarios**:

1. **Given** 一份简报，**When** 修改并保存，**Then** 产生**新版本**，版本号递增。
2. **Given** 已有三个版本，**When** 读第一版，**Then** 内容与当初一致——**MUST NOT** 被后续修改改写。
3. **Given** 任一版本，**When** 尝试更新或删除它，**Then** 不存在这样的路径。

---

### Edge Cases

- 七项里有几项确实无话可说：允许如实写明，**不允许**用必填把人逼成瞎填。
- 同一张卡被连点两次「开始」：不得产生两份首版简报。
- 简报的「渠道」是多个：§5.3 明写「简报可以包含多个渠道」，字段要能装下多个。
- 品牌删除：两张表都要进工作区删除清单，否则 `TestWorkspaceDeletionManifestCoversPublicSchema` 会红——这条已经有人踩过（#68）。
- 版本号并发：两个标签页同时保存简报，两边都应各得一个版本号，不得撞号也不得静默丢一次。

## Requirements *(mandatory)*

### 选题卡（US1、US2）

- **FR-001**: 系统 MUST 支持手工创建选题卡，字段覆盖 `docs/01` §5.2 的七项。
- **FR-002**: 七项中任何一项 MUST 允许「如实说明没有」的取值，MUST NOT 用必填逼出编造内容。
- **FR-003**: 选题卡 MUST 支持四个动作：开始 / 收藏 / 暂缓 / 放弃。
- **FR-004**: 暂缓与放弃 MUST 记录快捷原因与自由备注。
- **FR-005**: 这些记录 MUST NOT 改动账号的人设、范围偏好或任何 IP 配置（Q3 = A 已裁决：存在卡自己这一行上，不聚合）。
- **FR-006**: 选题卡 MUST 属于一个品牌；账号引用 MUST 可空（Q2 = A 已裁决）。

### 冻结简报（US3）

- **FR-007**: 选择「开始」MUST 产出一个 `ContentBriefRevision`，字段覆盖 §5.3 的十一项。
- **FR-008**: 「渠道」MUST 能容纳多个值。
- **FR-009**: 修改简报 MUST 追加新版本，MUST NOT 更新既有版本。
- **FR-010**: 版本表 MUST 是 append-only：没有更新路径，没有删除路径（品牌删除除外）。
- **FR-011**: 版本 id MUST 是稳定且可被外部引用的键；版本号 MUST 只作人读与排序之用，MUST NOT 作跨表键。
- **FR-012**: 同一张卡重复「开始」MUST NOT 产生第二份首版。

### 存储与边界

- **FR-013**: 新表 MUST NOT 有外键或级联；索引 MUST 用 `CREATE INDEX CONCURRENTLY`，每个索引**单独一个迁移文件、单条语句**。
- **FR-014**: 两张表 MUST 登记进工作区删除清单，并在删除事务里随品牌一起清除。
- **FR-015**: 代码 MUST 落在 `server/internal/content/topic-planning/`，MUST 满足接入合同的服务端条目（审计、技术日志、trace、脱敏至少四面），并使 `pnpm check:diagnostics-contract` 通过。
- **FR-016**: 授权 MUST 经 `workspace-core` 的 `Authorize`，越权 MUST 是 404（经 `RefusalStatus` / `RefusalBody`）。MUST NOT 自行判定成员关系。
- **FR-017**: 带路径参数的端点 MUST 有一条**穿过真实中间件、且路径参数 ≠ 上下文值**的用例（workflow 第 12 步）。

### 范围与诚实

- **FR-018**: §5.3 的「既有运行继续引用它开始时的版本」按**结构保证**验收：简报版本 MUST 只插不改，且 MUST 提供一个运行可以钉住的稳定键 `brief_revision_id`。**行为断言（某次运行确实继续看到它钉的那一版）留 agent-workflow 落地时补**，交付说明 MUST 写明这一半尚未断言。
- **FR-019**: 本卡 MUST NOT 调用模型，MUST NOT 读取本地文件、素材包或知识卡。
- **FR-020**: 本卡 MUST NOT 写输入快照（EP-04b）、MUST NOT 生成候选选题（EP-04c）、MUST NOT 实现必用 / 排除（EP-04d）。
- **FR-021**: MUST NOT 写 UI 单测。界面呈现由手动清单验收。
- **FR-022**: 页面 MUST 只挂既有组件（SOP 阶段 UI 规则），MUST NOT 新增控件、MUST NOT 调样式。
- **FR-023**: 交付 MUST 分两个 PR：**先存储与接口，再页面**。

## Success Criteria *(mandatory)*

- **SC-001**: §5.2 的七项字段全部可填、可存、可读回，缺一不可。
- **SC-002**: 四个动作各自可执行，暂缓 / 放弃的原因与备注可读回。
- **SC-003**: 执行暂缓 / 放弃前后，账号配置的差异为 **0 字节**。
- **SC-004**: §5.3 的十一项字段全部落在简报版本上。
- **SC-005**: 修改简报后版本数 +1，旧版本内容**逐字段不变**。
- **SC-006**: 版本表上不存在 UPDATE 或 DELETE 路径（品牌删除除外）——以代码检索与用例共同证明。
- **SC-007**: 同一张卡连续两次「开始」产生的首版简报数为 **1**。
- **SC-008**: 新迁移中外键与级联数为 **0**；非 CONCURRENTLY 索引数为 **0**；一个迁移文件多于一条语句的数为 **0**。
- **SC-009**: `TestWorkspaceDeletionManifestCoversPublicSchema` 通过；删除品牌后两张表中该品牌的行数为 **0**。
- **SC-010**: `pnpm check:content-boundaries` 与 `pnpm check:diagnostics-contract` 均退出 0，且后者报告的落地模块数由 **3** 变为 **4**。
- **SC-011**: 越权读取返回 **404**，且拒绝在技术日志里留下一条事件。
- **SC-012**: 模型调用次数为 **0**；文件读取次数为 **0**。
- **SC-013**: 新增 UI 单测数为 **0**。

## UI Impact

**页面只挂既有组件。** 按 `docs/development/design/README.md` 的 SOP phase rule 与 2026-09-15 的补充规则：界面整套继承上游 Multica 的设计系统与设计 token，只做「把功能挂到既有组件上」，不新增控件、不调样式、不引入第二套视觉语言。

**页面在第二个 PR 交付**，第一个 PR 只有存储与接口。

## Assumptions

- 品牌 = 工作区，与 LT-009 同一口径。
- `topic-planning` 的依赖声明里有 `knowledge-base`，但本卡不使用它——声明的是允许方向，不是必须使用。
- 输入全部是用户手打的文本与 URL 字符串；URL **不被抓取**，只作为文本存下来。
- `agent-workflow` 未落地，因此「运行」在本卡不存在，FR-018 的限制由此而来。
- **看板、标签、评论、指派这些能力不白得。** Q1 裁决为自建表，所以上游 `issue` 周边的现成能力（`Position` 看板排序、`issue_label`、`issue_reaction`、指派）都不会自动适用于选题卡。将来若要「在看板上看选题」，需要单独做一张卡，而不是以为它已经在那里。这是自建表的代价，记在这里以免日后被当成缺陷。
