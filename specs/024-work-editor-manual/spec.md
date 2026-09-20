# Feature Specification: 人工写作的作品容器、文档与版本（work-editor 首切片）

**Feature Branch**: `claude/spec-024-work-editor-manual`

**Created**: 2026-09-20

**Status**: Draft（三条 clarify 待裁决，其中 Q3 需要 SOP 原文，见文末）

**Input**: Issue #137。SOP §6.2「没有模型时仍可人工写作、编辑、审核和记录反馈」；§7 作品容器 / 文档 / 版本历史；§7.1「文档编辑」行。

**模块**：`work-editor`。`docs/12` §2 已把「**正文、候选修改、内容版本与附件引用清单**」列为该模块独占维护的数据。

**边界**：web-only、**不调模型**、执行器保持禁用（宪法 IX）。

---

## Current State（以代码为准）

全部核实自 `app-main` @ `6085983`。

### 1. 模块已登记，目录还是空的

`scripts/content-boundaries.json` 里 **`work-editor` 已是注册模块**，依赖声明为 `workspace-core` 与 `diagnostics`——**只有这两个**（`topic-planning` 不在里面，见下）。

`server/internal/content/` 下目前只有四个目录：`diagnostics`、`ip-profile`、`topic-planning`、`workspace-core`；`pnpm check:diagnostics-contract` 报 `checked 4 landed modules; skipped 8 not yet landed`。

**后果**：本卡新建 `server/internal/content/work-editor/` 的那一刻，该目录**立刻受接入合同约束**，落地模块数由 4 变 5。合同三条不是交付后再补的东西，是这张卡的一部分。

**一处必须先处理的事实**：作品挂在选题卡上，而 `work-editor` 的依赖声明里**没有 `topic-planning`**。两条路：要么在 `scripts/content-boundaries.json` 里给 `work-editor` 加这条依赖方向，要么让作品只存 `topic_card_id` 这个**字符串**而不 import 对方的包。见 **Q2**。

### 2. 可以挂靠的三个既有实体

| 实体 | 表 | 稳定键 | 本卡怎么用 |
|---|---|---|---|
| 选题卡 | `content_topic_card`（483） | `topic_card_id` | 作品挂在它上面；它自己的 `account_id` 可空（#133） |
| 简报版本 | `content_brief_revision`（485） | `brief_revision_id` | 本卡**不直接引用**——作品经开始快照间接指向它 |
| 开始快照 | `content_start_snapshot`（490） | `snapshot_id` | 作品**可**引用，**可空**：没有经过「开始」也能写 |

`StartSnapshot` 已带 `topic_card_id` / `brief_revision_id` / `account_id` / `project_id`，所以作品引用一个 `snapshot_id` 就等于间接拿到了这四项；**本卡不把它们复制到作品行上**，那会造出第二份可能不一致的真相。

### 3. append-only 版本表的现成模板

`content_account_revision`（479）与 `content_brief_revision`（485）是同一个形状，本卡照抄：

```text
<entity>_id   text NOT NULL   -- 被别处引用的稳定键；唯一性走单独的 CONCURRENTLY 索引
parent_id     text NOT NULL   -- 它属于谁
workspace_id  text NOT NULL   -- 一切查询按它过滤，删除按它删
revision      bigint NOT NULL -- 每父自增，给人读与排序，不是跨表键
created_at    timestamptz
```

479 的注释已经写明为什么 **append-only**：「可被 UPDATE 的历史，旧版本可读与『钉了某一版的运行继续看到它钉的那一版』两样都给不了」。**这段话对内容版本同样成立**，而且 Issue 明写「已审核/已交接/已发布版本永不删除」。

022 的 `TestBriefStoreHasNoUpdateOrDeletePath` 与 023 的 `TestStartSnapshotStoreHasNoUpdateOrDeletePath` 是这条的守卫形状：扫模块源码，出现 `UPDATE <表>` 或（删除链之外的）`DELETE` 即红。

**但本卡与它们有一处关键不同**：编辑副本是**可变**的（自动保存），版本是**不可变**的。两者不能放同一张表——023 的裁决理由正是「给 append-only 表开一条受限 UPDATE 会破掉守卫」。见 **Q1**。

### 4. 受控集的既有做法

`content_account`（477）的 `platform`：**Go 受控枚举为准产出 400 诊断错误对象，库加 `CHECK` 兜底**（specs/015 裁决）。`content_topic_card`（483）的 `status` 同形：`CHECK (status IN ('draft','started','saved','deferred','dropped'))`。

本卡的文档类型与状态照此办理。

### 5. 迁移规则（#122 之后全部生效）

- R1/R2 无外键、无级联；
- R3/R4 每个索引 `CREATE INDEX CONCURRENTLY`，**单文件单语句**；
- **R5**（从 483 起）建表迁移**不得**含 `PRIMARY KEY` 或 `UNIQUE`——479 有 `PRIMARY KEY` 是因为它早于这条规则，**本卡不能照抄那一行**；
- **R6** 每个建索引的 up 迁移必须登记进 `cmd/migrate` 的 `concurrentIndexCleanups`，索引名逐字相同，**建表迁移不登记**；
- 新表进 `workspace_delete_manifest_test.go` 并在删除链同一 CTE 里有一条按 `workspace_id` 的 `DELETE`。

下一个迁移号是 **494**。

### 6. 写路径与授权的既有约束

- 写事务第一句取 `LockForContentDiagnosticWrite`（#104 的删除栅栏），不靠「顺便审计了」间接获得；
- 越权一律经 `workspace-core` 的 `RefusalStatus` / `RefusalBody`，与「不存在」同形；
- 带路径参数的端点按工作流第 12 步：**必须有一条穿过真实中间件、参数值 ≠ 上下文值**的用例；本卡的路径参数会有三到四个，`{versionId}` 是新的一类 id。

### 7. 执行器禁用

`generated` 来源、选段 AI 改写、全文润色、候选版本——**本阶段一个都不做**。界面上留位并写明「暂不可用（执行器禁用，EP-08 接入）」，接口形状定下来使 EP-08 接上不必改调用方。

---

## User Scenarios & Testing *(mandatory)*

### User Story 1 - 没有模型也能把一篇写完 (Priority: P1)

小张从一张选题卡开一个作品，里面新建一篇正文文档，直接开始打字。中途关掉浏览器，回来内容还在。写到一个段落他觉得可以了，显式存一版，历史里出现「第 1 版 · 人工」。

**Why this priority**：这是 §6.2 那句话的全部内容——没有模型时仍可人工写作。没有它，其余两条都没有对象。

**Independent Test**：只实现这一条即可验收：建作品 → 建文档 → 打字 → 刷新仍在 → 存版本 → 历史里有一条。

**Acceptance Scenarios**：

1. **Given** 一张选题卡，**When** 在它上面建作品，**Then** 作品被创建；**不经过「开始」也能建**（快照引用可空）。
2. **Given** 一个作品，**When** 新建一篇文档，**Then** 文档类型来自受控集，作品下可以有多篇文档。
3. **Given** 一篇文档，**When** 打字并等待自动保存，**Then** 刷新后内容还在，**且历史里没有新增版本**——自动保存动的是编辑副本。
4. **Given** 编辑副本有内容，**When** 显式存一版，**Then** 历史里出现一条 `source = edited` 的版本，编辑副本内容不变。
5. **Given** 从未存过版本的新文档，**When** 打开历史，**Then** 显示「还没有版本」而不是报错——空历史是正常状态。

---

### User Story 2 - 历史只增不减，恢复也是往前走 (Priority: P2)

小张写了三版，觉得第 1 版更好，恢复它。历史里**不是**回到第 1 版，而是多出第 4 版，内容与第 1 版相同，来源写明「恢复自第 1 版」。他把第 4 版「采用」为下一步基线。

**Why this priority**：这是 §7 版本历史的实质。它建立在 Story 1 之上，但没有它，历史只是一串备份。

**Independent Test**：存三版 → 恢复第 1 版 → 断言版本数为 4 且第 1 版逐字节不变 → 采用 → 断言基线指向对的那一版。

**Acceptance Scenarios**：

1. **Given** 已有三版，**When** 恢复第 1 版，**Then** 产生**第 4 版**；第 1、2、3 版**逐字节不变**。
2. **Given** 已有版本，**When** 任何人通过任何端点尝试修改或删除一条版本，**Then** 不存在这样的端点——版本**只插不改不删**（工作区删除除外）。
3. **Given** 一条版本，**When** 采用它为下一步基线，**Then** 这件事被记下来且可读回（形态见 **Q3**）。
4. **Given** 版本已被审核 / 交接 / 发布，**When** 任何清理路径运行，**Then** 它仍在——本卡**只做结构保证**（没有删除路径），「审核/交接/发布确实发生过」由 review-delivery 与交付卡落地。
5. **Given** 两个人几乎同时存版本，**When** 两次写入并发，**Then** 恰好一方拿到下一个版本号，另一方重试后拿到再下一个；**不出现两条同号版本**。

---

### User Story 3 - AI 的那半边明确地不可用 (Priority: P3)

小张看到「选段改写」「全文润色」「候选版本」三个入口都在，都是禁用的，旁边写着「暂不可用（执行器禁用，EP-08 接入）」。他继续手写，没有任何东西挡他。

**Why this priority**：它不产生新价值，但它是**诚实**的那一条。空白会被读成「还没做完」，转圈会被读成「马上就好」，伪造的候选会被当成真的。

**Independent Test**：打开文档 → 三个入口存在、禁用、有原因；手写路径完全不受影响。

**Acceptance Scenarios**：

1. **Given** 打开一篇文档，**When** 看 AI 相关入口，**Then** 它们**存在且禁用**，并写明原因。
2. **Given** AI 入口禁用，**When** 走人工写作与存版本，**Then** 全程可用——禁用的是 AI，不是编辑器。
3. **Given** 任何路径，**When** 检查已存版本，**Then** **没有任何一条** `source = generated` 的版本——本阶段产生不出来，也不许伪造。

---

### Edge Cases

- **编辑副本与最新版本不一致**：这是常态（写了但还没存版本），界面要能说出「有未存为版本的改动」，**不是**错误。
- **作品所属的选题卡被删**：无外键，不级联。本卡的口径见 **Q2**。
- **引用的开始快照所属账号被改配置**：快照本身不可变（023），作品引用的是 `snapshot_id`，因此不受影响；**作品不复制快照内容**。
- **同一作品下两篇同类型文档**：允许（正文与另一版正文都是正文）；排序由显式序号决定，不由创建时间。
- **超长正文**：需要一个上限。按 `ip-profile` 的做法**按 rune 计数**而不是字节，否则中文只拿到英文三分之一的额度。
- **空版本**：存一版空白是合法的（§3.1 的「待补充不是错误」同理），不是校验失败。
- **工作区在写作途中被删**：写路径持删除栅栏，整个保存失败，不留半条记录。
- **时钟**：版本时间由服务端生成，不接受客户端传入。

---

## Requirements *(mandatory)*

### Functional Requirements

**作品容器**

- **FR-001**：系统 MUST 允许在一张选题卡上创建作品；一张卡 MUST 可以有多个作品。
- **FR-002**：作品 MUST 可以引用一个开始快照（`snapshot_id`），且该引用 MUST 可空——没有经过「开始」也能写。
- **FR-003**：作品 MUST NOT 复制快照里的账号、简报版本或资料范围；引用一个 id 即可，复制会造出第二份可能不一致的真相。
- **FR-004**：作品 MUST 有一个来自受控集的状态，取值与语义见 **Q3**。

**文档**

- **FR-005**：一个作品 MUST 可以有多篇文档，且 MUST 有一个显式的排序，MUST NOT 依赖创建时间排序。
- **FR-006**：文档类型 MUST 来自受控集；越界 MUST 产出 400 诊断错误对象，MUST NOT 落成存储错误。受控集按既有做法：**Go 枚举为准，库加 `CHECK` 兜底**。
- **FR-007**：文档 MUST 有一份**可变的编辑副本**，自动保存写它。

**版本历史**

- **FR-008**：版本 MUST 只插不改不删：没有更新端点、没有删除端点、模块源码里没有 `UPDATE`/`DELETE`（工作区删除链那一条除外），并有守卫用例。
- **FR-009**：自动保存 MUST NOT 产生版本。版本的产生时机见 **Q1**。
- **FR-010**：版本 MUST 记录来源，受控集至少含 `edited`；`generated` MUST 在受控集里**留位但本阶段产生不出来**，并有一条负例钉住「不存在 `generated` 版本」。
- **FR-011**：恢复一条旧版本 MUST 产生一条**新版本**，MUST NOT 改写或删除任何既有版本；新版本 MUST 记下它恢复自哪一条。
- **FR-012**：版本号 MUST 每文档自增；并发写入 MUST 恰好一方成功拿号，MUST NOT 出现两条同号版本。
- **FR-013**：版本的时间戳 MUST 由服务端生成。
- **FR-014**：空内容的版本 MUST 合法。
- **FR-015**：正文长度上限 MUST 按 rune 计数。

**采用为基线**

- **FR-016**：系统 MUST 能把某一版标为「下一步的基线」，且这件事 MUST 可读回。形态见 **Q3**。

**AI 的那半边**

- **FR-017**：选段改写、全文润色、候选版本三个入口 MUST 存在、MUST 禁用、MUST 写明原因，MUST NOT 是空白、加载中或伪造内容。
- **FR-018**：本卡 MUST NOT 调用模型、MUST NOT 起真实执行器（宪法 IX）。
- **FR-019**：AI 入口禁用 MUST NOT 影响任何人工写作路径。

**授权与边界**

- **FR-020**：所有读写 MUST 按 `workspace_id` 过滤；越权 MUST 经 `workspace-core` 产出与「不存在」同形的拒绝。
- **FR-021**：写入 MUST 与工作区删除栅栏在同一事务内。

**工程约束**

- **FR-022**：新表的迁移 MUST 无外键、无级联（R1/R2）；建表迁移 MUST NOT 含 `PRIMARY KEY` 或 `UNIQUE`（R5）；每个索引 MUST `CONCURRENTLY` 且单文件单语句（R3/R4）；每个建索引的 up 迁移 MUST 登记进 `concurrentIndexCleanups`，索引名逐字相同，建表迁移 MUST NOT 登记（R6）。
- **FR-023**：每张新表 MUST 进工作区删除清单并在删除链同一 CTE 里有一条按 `workspace_id` 的 `DELETE`，且 MUST 有一条用例断言删除后该表在该工作区的行数为 0。
- **FR-024**：接入合同三条 MUST 通过；落地模块数 MUST 由 4 变 5。
- **FR-025**：交付 MUST 拆成「存储与接口 PR」与「页面 PR」两次。
- **FR-026**：界面 MUST 只复用既有 Multica 组件，MUST NOT 新增控件或改样式；四语言 MUST 齐。
- **FR-027**：MUST NOT 写 UI 单测；界面项 MUST 进 `manual-ui-todo.md`。
- **FR-028**：每个带路径参数的新端点 MUST 有一条穿过真实中间件、参数值 ≠ 上下文值的用例；`{versionId}` 是新的一类 id，MUST 单独有一条。

### Key Entities

- **作品（Work）**：一次产出的容器。挂在一张选题卡上，可引用一个开始快照（可空）。有标题与状态。**可变**（标题、状态会改）。
- **文档（Artifact）**：作品下的一篇。有受控类型、标题、显式序号，以及**一份可变的编辑副本**。
- **版本（ArtifactVersion）**：一篇文档的一次快照。**只插不改不删**。有稳定的 `version_id`、每文档自增的 `revision` 计数器、来源（`edited` / `generated` 留位 / 见 Q3）、可选的「恢复自」指针。

---

## Success Criteria *(mandatory)*

- **SC-001**：不经过「开始」也能建作品并写完一篇文档，全流程 0 次模型调用。
- **SC-002**：连续自动保存 N 次后，版本数仍为 0；显式存版本后为 1。**自动保存不产生版本**有独立用例。
- **SC-003**：恢复第 1 版后版本数 +1，且第 1 版的内容、来源、时间戳**逐字节不变**。
- **SC-004**：模块源码里出现 `UPDATE <版本表>` 或删除链之外的 `DELETE <版本表>` 即有用例变红（守卫照 022/023 的形状）。
- **SC-005**：并发存版本 N 次，得到 N 条**版本号互不相同**的版本，0 条失败到用户面前（冲突方重试后成功）。
- **SC-006**：所有响应里 `source = generated` 的版本数**恒为 0**，有一条用例在它非 0 时变红。
- **SC-007**：三个 AI 入口在界面上**永远显示原因**，且不存在任何伪造的候选或润色结果。
- **SC-008**：迁移规则 **R1–R6 逐条**全绿；R5 专门确认建表迁移不含 `PRIMARY KEY` / `UNIQUE`；R6 确认每条索引登记名逐字相同且建表迁移未被登记。
- **SC-009**：删一个工作区后，本卡每张新表在该工作区的行数为 0，邻居工作区不受影响。
- **SC-010**：跨品牌的作品 / 文档 / 版本 id 三种非法引用各返回与「不存在」同形的拒绝，三者响应体**除 trace_id 外逐字节相同**。

---

## Assumptions

1. **文档类型受控集先给最小几种**：正文、渠道稿。按 specs/015 的做法（Go 枚举为准 + 库 `CHECK` 兜底）。加一种需要一次迁移改 `CHECK`，这是刻意的代价：类型集合变化应当是可审阅的一次改动。
2. **「作品」与「文档」是两层，不是一层**。Issue 明写作品下有多篇文档（正文、渠道稿等）。把两者合一会让「同一次产出的多个渠道稿」无处安放。
3. **编辑副本不是版本**。自动保存必须便宜且频繁；若每次都产生版本，历史会被淹没到无法使用，「恢复到某一版」也就失去意义。
4. **本卡不落地审核与交接**。`已审核 / 已交接 / 已发布`的状态值可能出现在受控集里，但**使它们发生**的流程属 review-delivery（EP-06/EP-07）与交付卡。本卡只保证「到了那些状态的版本没有删除路径」——**结构保证**，与 EP-04a 对 §5.3 的处理口径一致。
5. **作品不复制快照内容**，只引用 `snapshot_id`（FR-003）。
6. **附件引用清单**（`docs/12` §2 归给 work-editor 的第四项）**不在本卡**：它需要素材实体，那是 W-03。本卡不预留列，加列比改列容易。

---

## Out of Scope

- 任何模型调用：选段改写、全文润色、候选版本生成（EP-08）。
- 审核流程与交接、发布（review-delivery / 交付卡）。
- 附件引用清单（等 W-03 的素材实体）。
- 版本差异（diff）视图与来源侧栏的**渲染**——留页面 PR；本卡只保证数据够画。
- 协同编辑 / 实时多人光标。
- 导出为渠道格式。

---

## 待裁决（clarify）

三条都给了推荐值，**不阻塞** plan；主任务裁决后回写本节与相应 FR。**Q3 需要 SOP 原文。**

### Q1：编辑副本放哪里，版本什么时候产生？

**Context**：编辑副本必须**可变**（自动保存），版本必须**不可变**。023 刚刚因为「给 append-only 表开一条受限 UPDATE 会破掉 022 的守卫」而选了新表；同样的取舍在这里再次出现。

| 选项 | 做法 | 含义 |
|---|---|---|
| **A（推荐）** | 编辑副本是 `content_artifact` 行上的**可变列**（`draft_body` + `draft_saved_at`）；版本在**独立的 append-only 表**里，由显式「存为版本」产生 | 文档行本来就是可变的（标题、序号会改），所以可变列不破坏任何守卫；版本表的守卫可以是干净的「无 UPDATE 无 DELETE」。自动保存是一次 `UPDATE content_artifact`，便宜 |
| B | 编辑副本单独一张表 `content_artifact_draft`，一文档一行 | 多一张表、多一组索引、多一条删除链；换来的只是「文档行不被频繁 UPDATE」，而文档行本来就可变 |
| C | 不要编辑副本，每次自动保存产生一条版本 | 历史被淹没，「恢复到某一版」失去意义；也让 `edited` 这个来源不再表示「人决定存下来的一版」 |

**附带问题 1**：显式存版本之外，是否还要「离开页面时自动存一版」之类的兜底？**推荐不要**——那会让版本历史里混进人没有决定过的条目，而编辑副本已经保证了不丢内容。

**附带问题 2（写 contract 时发现的）**：版本表需要**第六个索引**吗？`version_id` 唯一索引与 `(artifact_id, revision)` 唯一索引都不以 `workspace_id` 打头，所以**工作区删除按 `workspace_id` 删版本表会走全表扫**——而删除要在一个事务里完成，扫全表会拖长持锁时间。**推荐加** `(workspace_id, artifact_id, revision DESC)`。代价是一个迁移、一条 R6 登记。

### Q2：`work-editor` 怎么引用选题卡？

**Context**：`scripts/content-boundaries.json` 里 `work-editor` 的依赖只有 `workspace-core` 与 `diagnostics`，**没有 `topic-planning`**。而作品要挂在选题卡上。

| 选项 | 做法 | 含义 |
|---|---|---|
| **A（推荐）** | 只存 `topic_card_id` / `snapshot_id` **字符串**，**不 import** `topic-planning` 包；校验「这张卡在这个工作区存在」由 handler 层（adapter）做 | 登记表不动，模块依赖图不变宽；work-editor 保持只依赖 workspace-core 与 diagnostics。代价：存在性校验在 adapter 而不在模块里 |
| B | 在登记表里给 `work-editor` 加 `topic-planning` 依赖，模块内直接调它 | 依赖方向变宽一条，而本卡只需要「这个 id 存在吗」这一个问题；一旦 import 进来，以后什么都容易顺手调 |
| C | 不引用选题卡，作品独立存在 | 与 Issue「`Work` 挂在选题卡」相悖 |

**连带**：选题卡被删时作品怎么办？本卡建议**不处理**——选题卡今天**没有删除路径**（022 只有状态 `dropped`，不删行），所以这是一个尚不存在的问题；写进 Out of Scope 比现在设计一个用不上的清理更诚实。

### Q3：§7.1「文档编辑」行的状态集合、以及「采用」的形态——**需要 SOP 原文**

**Context**：Issue 说「状态按 §7.1『文档编辑』行；不用一个『完成』覆盖所有行为」，但没有给出那一行的原文；本检出没有文档仓库。同时 Issue 把 `adopted` 与 `edited` / `generated` 并列写在「版本来源」里，而「采用当前版本为下一步基线」读起来更像一个指针。

**需要原文的部分**：`docs/01` §7.1 表格里「文档编辑」那一行的**完整原文**（该行列出的每一种行为与状态）。

**暂定值**（据 Issue 与既有卡推断，**仅供不阻塞**）：

- 作品/文档状态受控集暂定 `drafting`（在写）/ `in_review`（送审中）/ `revising`（按意见改）/ `approved`（已审核）/ `handed_off`（已交接）/ `published`（已发布）。本卡**只落地能由人工操作达成的转换**，其余留位。
- 「采用」的形态：

| 选项 | 做法 | 含义 |
|---|---|---|
| **A（推荐）** | 采用产生一条**新版本**，`source = adopted`，内容与被采用版逐字相同，并记 `adopted_from` | 与「恢复产生新版本」同一个模式，历史里看得见「在这里采用了第 3 版」；代价是内容重复存储 |
| B | 文档行上一个可变指针列 `baseline_version_id`，不产生版本 | 不重复存储；代价是指针可变，「采用过哪几版」这段历史就没了 |
| C | 版本上的一个布尔标记 | 需要把旧标记改回 false，等于给 append-only 表开 UPDATE——**与 FR-008 直接冲突，不推荐** |
