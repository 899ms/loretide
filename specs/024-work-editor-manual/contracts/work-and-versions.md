# 合同：作品、文档与版本（024，人工写作切片）

**状态**：**已裁决**（主控 2026-09-21，含 SOP §7.1 原文）。Q1=A（附带 1 不要自动存版、附带 2 要 `workspace_id` 打头的索引）、Q2=A、Q3 严格按原文。

**§7.1「文档编辑」行原文**：

> | 文档编辑 | working / saved；版本有 generated / edited / adopted 来源与动作记录 | 不用一个"完成"覆盖所有行为 |

> 采用当前版本表示"此版是下一步工作基线"。提交审核会冻结具体渠道版本。恢复旧版本会产生一个新版本；已审核、已交接、已发布版本始终保留。

**不变量**：版本只插不改不删；编辑副本可变；两者不同表。

---

## 1. 三个实体

### `content_work` —— 作品容器（可变）

| 列 | 说明 |
|---|---|
| `work_id` | 被引用的稳定键。唯一性走 495 的并发索引，**不是** `PRIMARY KEY`（R5） |
| `workspace_id` | 一切查询按它过滤；删除按它删 |
| `topic_card_id` | 挂在哪张卡上。**字符串，不 import `topic-planning`**（Q2=A） |
| `snapshot_id` | 可为 `''`：**没有经过「开始」也能写**。空串是真实状态，不是缺失值 |
| `title` | 可变 |
| `created_at` / `updated_at` | |

**作品没有状态列。** §7.1「文档编辑」行给的状态只有 `working` / `saved`，那是**编辑副本**的；`drafting` / `in_review` / … / `published` 属 §7.1 其它行（审核与交付的对象），不进本卡。加一列比改一列容易。

**作品不复制快照内容。** `StartSnapshot` 已带 `topic_card_id` / `brief_revision_id` / `account_id` / `project_id`；引用一个 `snapshot_id` 就等于间接拿到了它们，复制会造出第二份可能不一致的真相（FR-003）。

### `content_artifact` —— 文档（可变，含编辑副本）

| 列 | 说明 |
|---|---|
| `artifact_id` | 稳定键，唯一性走 497 |
| `work_id` / `workspace_id` | |
| `kind` | 受控集，`CHECK` 兜底 |
| `title` | |
| `position` | **显式序号**。不按创建时间排序：同一作品下两篇同类型文档是合法的，而创建时间说不出作者想要的先后 |
| `draft_body` | **编辑副本，可变**。自动保存写它 |
| `draft_status` | **`working` / `saved`**（§7.1 原文）。`saved` = 编辑副本与最新版本逐字相同；`working` = 有尚未存为版本的改动 |
| `draft_saved_at` | 编辑副本最后一次落库的时间 |
| `created_at` / `updated_at` | |

**为什么编辑副本是列而不是表**：文档行本来就可变（标题、序号会改），所以一条 `UPDATE content_artifact` 不破坏任何守卫。放进版本表则要给一张 append-only 表开 UPDATE，那正是 023 选新表时拒绝的做法。

### `content_artifact_version` —— 版本（**只插不改不删**）

| 列 | 说明 |
|---|---|
| `version_id` | 稳定键，唯一性走 500 |
| `artifact_id` / `work_id` / `workspace_id` | `work_id` 冗余存放，使「按作品列出全部版本」不必回表 |
| `revision` | **每文档**自增。给人读与排序，**不是**跨表键——第 3 版对每篇文档意思都不同 |
| `source` | 受控集 `generated` / `edited` / `adopted`，见下 |
| `action` | 受控集 `saved` / `restored` / `adopted`，**与 `source` 分开的第二列** |
| `body` | 这一版的全文 |
| `restored_from` | 可为 `''`。`action = restored` 时记它恢复自哪一条 |
| `adopted_from` | 可为 `''`。`action = adopted` 时记它采用了哪一条 |
| `actor_id` | 谁存的 |
| `created_at` | 服务端时间 |

`(artifact_id, revision)` 的唯一性走 501 的并发唯一索引——它同时是并发写入的互斥：两个写入者读到同一个当前版本号 N 并都尝试插 N+1 时，**恰好一个**拿到 23505，另一个成功。

---

## 2. 受控集

### 版本来源 `source` —— **恰好三个**（§7.1 原文）

| 值 | 含义 | 本阶段 |
|---|---|---|
| `generated` | 模型产出的一版 | **留位，产生不出来**。负例扫的是「模块里有没有产生它的路径」（EP-08 接入） |
| `edited` | 内容是人写的 | **今天唯一产生得出来的来源**。一次普通保存、一次恢复，都是它 |
| `adopted` | 采用为下一步工作基线的一版 | 今天产生得出来 |

**「恢复」不在这张表里。** 它不是第四种来源——恢复回来的内容仍然是人写的，所以来源是 `edited`；「这一版是恢复来的」是一次**动作**，记在下一张表。

### 版本动作 `action` —— 与来源**分开的第二列**

| 值 | 什么时候 | 同时记 |
|---|---|---|
| `saved` | 一次普通的「存为版本」 | — |
| `restored` | 恢复旧版产生的新版 | `restored_from = <被恢复版本 id>`，`source = edited` |
| `adopted` | 采用为基线产生的新版 | `adopted_from = <被采用版本 id>`，`source = adopted` |

**页面的历史侧栏两列都显示。** 把两件事压成一列，读者要去看另一列才知道发生了什么，等于把信息藏起来。

> **`saved` 是裁决没有点名的一个值。** 裁决只点名了 `restored` 与 `adopted`；一次普通保存的 `action` 需要一个值，我取 `saved`，与编辑副本状态的用词一致。**若应当是空串，改动是一个枚举值。**

### 文档类型 `kind`（暂定最小集）

| 值 | 含义 |
|---|---|
| `body` | 正文 |
| `channel_draft` | 渠道稿 |

加一种要改 `CHECK`，即一次迁移。**这是刻意的代价**：类型集合的变化应当是一次可审阅的改动，而不是某个人在 Go 里加一行。

### 编辑副本状态 `draft_status` —— **只有两个**（§7.1 原文）

| 值 | 含义 |
|---|---|
| `working` | 编辑副本有**尚未存为版本**的改动 |
| `saved` | 编辑副本与该文档**最新版本逐字相同** |

**不用一个「完成」覆盖所有行为**（§7.1 原话）。所以没有 `done`，没有 `finished`，也**没有** `drafting` / `in_review` / `approved` / `handed_off` / `published`——那些是 §7.1 其它行的对象，由 review-delivery 与交付卡落地。

**为什么存而不是每次读时重算**：`saved` 的定义是「与最新版本逐字相同」，重算要把最新版本的全文取出来比一遍；列文档的端点就得取 N 份全文。所以它被**存下来**，代价是两个真相里可能有一个会错——因此**每一处改动它的写路径都要有一条用例，断言存下来的状态与重算的结果一致**（FR-007b）。改动它的地方恰好三处：自动保存（→ `working`）、存为版本（→ `saved`）、恢复（→ `saved`，因为恢复同时改写编辑副本并产生版本）。

---

## 3. 端点与决策顺序

八条端点见 `plan.md`。每一条的决策顺序都是同一个形状：

| 序 | 检查 | 失败时 |
|---|---|---|
| 1 | 工作区成员资格（`workspace-core.Authorize`） | `RefusalStatus` / `RefusalBody`，与「不存在」同形 |
| 2 | 作品存在且属本工作区 | 同上 |
| 3 | 文档存在、属本工作区、且属这个作品 | 同上 |
| 4 | 版本存在、属本工作区、且属这篇文档 | 同上 |
| 5 | 输入受控集校验（`kind` / `status` / 长度上限） | 400 诊断错误对象 |

**1–4 的拒绝体必须逐字节相同**（除 `trace_id`）：任何差异都能被用来探测某个 id 是否在别的品牌存在。

### 自动保存：`PATCH .../artifacts/{artifactId}`

- 只动 `draft_body` / `draft_status` / `draft_saved_at` / `title` / `position`；
- 改了正文即把 `draft_status` 置为 `working`；
- **不产生版本**（FR-009），也没有「顺手存一版」的兜底——那会让历史里混进人没有决定过的条目；
- 幂等的：同样的 body 再 PATCH 一次，结果相同。

### 存为一版：`POST .../versions`

- 用**编辑副本当前的内容**建一条 `source = edited`、`action = saved` 的版本；
- 同一事务里把 `draft_status` 置为 `saved`；
- 版本号按 022 `AppendBrief` 的**两步写法**：**先锁文档行，再在第二条语句里数号**。READ COMMITTED 下等锁不刷新语句快照，在锁定语句里数号会数到赢家提交之前的状态，选到一个已被用掉的号，把一次合法保存变成 503（Issue #109）；
- 空内容**合法**（FR-014）。

### 恢复：`POST .../versions/{versionId}/restore`

- 产生**新版本**，`source = edited`（内容仍是人写的，只是从旧版取来）、`action = restored`、`restored_from = {versionId}`，内容与被恢复版逐字相同；
- **同时**把编辑副本改成那份内容——否则界面上「恢复了」但编辑框还是旧的——并把 `draft_status` 置为 `saved`；
- 任何既有版本**一个字节都不动**。

### 采用：`POST .../versions/{versionId}/adopt`（Q3=A）

- 产生**新版本**，`source = adopted`、`action = adopted`、`adopted_from = {versionId}`，内容与被采用版**逐字相同**；
- §7.1 原文：「采用当前版本表示『此版是下一步工作基线』」。与恢复同一个模式，所以历史里看得见「在这里采用了第 3 版」。
- **不用可变指针代替**：指针可变，「采用过哪几版」这段历史就没了。

---

## 4. 本阶段做不到的事，怎么表示

| 事 | 本阶段 |
|---|---|
| 选段 AI 改写 | 入口存在、禁用、写明「暂不可用（执行器禁用，EP-08 接入）」 |
| 全文润色 | 同上 |
| 候选版本 | 同上；**没有任何伪造的候选** |
| `source = generated` 的版本 | **恒为 0**，有负例 |
| 审核 / 交接 / 发布 | **状态值也不在本卡**（§7.1 那几行属 review-delivery 与交付卡）；流程更不落地 |
| 「已审核版本永不删除」 | **结构保证**：没有删除路径。不是「一条已发布版本挺过了清理」 |
| 附件引用清单 | **完全不碰**，等 W-03 的素材实体。不预留列——加列比改列容易 |
| 版本差异渲染 | 页面 PR，只用既有组件；没有合适的就并排显示两版全文，**不引入 diff 库** |
| 协同编辑 | 不做 |

---

## 5. 迁移（**十个文件**，每个单条语句）

```sql
-- 494_content_work.up.sql
-- 无 PRIMARY KEY、无 UNIQUE、无 REFERENCES：R1/R2/R5。
-- 没有 status 列：7.1「文档编辑」行的状态只有 working / saved，那是编辑副本的；
-- drafting…published 属 7.1 其它行，由 review-delivery 与交付卡去加。
CREATE TABLE IF NOT EXISTS content_work (
    work_id       text NOT NULL,
    workspace_id  text NOT NULL,
    topic_card_id text NOT NULL,
    -- '' when the work was not started from a snapshot. A real state.
    snapshot_id   text NOT NULL,
    title         text NOT NULL,
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now()
);
```

```sql
-- 497_content_artifact.up.sql
CREATE TABLE IF NOT EXISTS content_artifact (
    artifact_id    text NOT NULL,
    work_id        text NOT NULL,
    workspace_id   text NOT NULL,
    kind           text NOT NULL CHECK (kind IN ('body','channel_draft')),
    title          text NOT NULL,
    position       bigint NOT NULL,
    -- The mutable editing copy. Autosave writes here and produces no version.
    draft_body     text NOT NULL DEFAULT '',
    -- 7.1's two states, and only those two.
    draft_status   text NOT NULL DEFAULT 'saved' CHECK (draft_status IN ('working','saved')),
    draft_saved_at timestamptz NOT NULL DEFAULT now(),
    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now()
);
```

```sql
-- 500_content_artifact_version.up.sql
-- Append-only. Never UPDATEd, never DELETEd except with its workspace.
-- source and action are two columns on purpose (7.1: "来源与动作记录").
-- Restoring is not a fourth source: the content is still what a person wrote,
-- so the source stays 'edited' and 'restored' is recorded as the action.
CREATE TABLE IF NOT EXISTS content_artifact_version (
    version_id    text NOT NULL,
    artifact_id   text NOT NULL,
    work_id       text NOT NULL,
    workspace_id  text NOT NULL,
    revision      bigint NOT NULL,
    source        text NOT NULL CHECK (source IN ('generated','edited','adopted')),
    action        text NOT NULL CHECK (action IN ('saved','restored','adopted')),
    body          text NOT NULL,
    restored_from text NOT NULL DEFAULT '',
    adopted_from  text NOT NULL DEFAULT '',
    actor_id      text NOT NULL,
    created_at    timestamptz NOT NULL DEFAULT now()
);
```

**七个索引**，各自单文件单语句：

| 迁移 | 索引 | 为什么 |
|---|---|---|
| 495 | `content_work_id_unique_idx (work_id)` | 稳定键唯一性（R5 不让它当 PK） |
| **496** | `content_work_card_idx (workspace_id, topic_card_id, created_at DESC)` | 列出一张卡下的作品；**前导 `workspace_id` 同时服务删除链**。原稿漏了它——495 不以 `workspace_id` 打头，删除会扫全表 |
| 498 | `content_artifact_id_unique_idx (artifact_id)` | 稳定键唯一性 |
| 499 | `content_artifact_work_idx (workspace_id, work_id, position)` | 列出一个作品的文档；前导 `workspace_id` 供删除链 |
| 501 | `content_artifact_version_id_unique_idx (version_id)` | 稳定键唯一性 |
| 502 | `content_artifact_version_unique_idx (artifact_id, revision)` | **既是唯一性也是并发互斥**：两个写入者抢同一个号时恰好一个拿到 23505 |
| **503** | `content_artifact_version_workspace_idx (workspace_id, artifact_id, revision DESC)` | 裁决的附带问题 2：**删除链按 `workspace_id` 删版本表不能走全表扫**——删除要在一个事务里完成，扫全表会拖长持锁时间。501/502 都不以 `workspace_id` 打头 |

**每张表都有一个以 `workspace_id` 打头的索引**（496 / 499 / 503），这是 FR-021a。

**登记**：**七条**索引迁移全部进 `concurrentIndexCleanups`，索引名逐字相同；**494 / 497 / 500 三个建表迁移不登记**（建表不建索引，登记一个不存在的索引同样是静默 no-op，R6 的反向断言会红）。

**删除清单**：三张表各进 `workspace_delete_manifest_test.go`（标 `workspaceDelete`），删除链同一 CTE 链里三条 `DELETE`，并有一条用例断言删除后三张表在该工作区的行数都是 0。

**down**：七个 `DROP INDEX CONCURRENTLY IF EXISTS`（各单语句）+ 三个 `DROP TABLE IF EXISTS`。在 `494/497/500.down.sql` 的注释里写明**已写的内容会因此丢失**，与 485 / 490 的处理一致。
