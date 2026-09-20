# 合同：开始与输入快照（EP-04b）

**状态**：**已裁决**（主控 2026-09-20）。Q1=**B**（新表，append-only，一版简报可多次开始）、Q2=A、Q3=A，附带一问接受（预检开关与中性表达作 FR-015a 扩展键）。

**对齐对象**：`server/internal/content/diagnostics/contract.go:51` 的 `Snapshot`。**十六个字段逐名沿用，不新造同义字段**；两个扩展键显式标注，不属于那十六项。

---

## 1. 两个「start」

| 谁 | 端点 | 含义 | 状态 |
|---|---|---|---|
| EP-04a | `POST /api/content-topics/{id}/actions` body `{"action":"start"}` | 接受选题，冻结首版简报，卡 → `started` | **既有，本卡不改** |
| EP-04b | `POST /api/content-topics/{id}/briefs/{revisionId}/start` | 用这一版简报开一次工，固定输入快照 | 本卡新增 |

读回：

- `GET /api/content-topics/{id}/briefs/{revisionId}/snapshots` —— 该版简报的全部快照，**新**；
- `GET /api/content-topics/{id}/snapshots/{snapshotId}` —— 单份快照，**新**。

**快照表没有 UPDATE，没有 DELETE**（工作区删除那一条除外）。这是字面意义的「只插不改」，与 `content_brief_revision`、`content_account_revision` 同一读法。

### 为什么是新表而不是给简报加一列

按 Q1=B 裁决：给 `content_brief_revision` 加 `snapshot jsonb` 需要在那张表上开一条受限 `UPDATE`，而 022 的 A6 守卫用例说的是「只插不改不删」。**那条守卫不容破例。** 代价是与拆分卡的「无新表」一句冲突——记在这里，拆分卡不追改。

新表换来的另一件事：**一版简报可以被开始多次**。同一份简报换个资料范围再跑一次是合法的，不必先追加一个只为换配置的新版本。

---

## 2. 请求

```json
{
  "account_id": "…",
  "source_scope": "all",
  "project_id": ""
}
```

- `account_id` 必填：选题卡的 `account_id` 可空，而就绪判定是按账号算的。
- `source_scope` 必填，受控集 `local|web|all`，**精确匹配**（不 trim、不折大小写，同 LT-014）。
- `project_id` 可选，留空合法（FR-018）。
- 未知字段一律拒绝（`DisallowUnknownFields`），与账号档案写入同一读法。
- 请求体**不接受**任何时间字段：快照时间由服务端生成（FR-017）。
- 请求体**不接受** `snapshot` 或 `snapshot_id`：调用方不能自带一份快照，也不能指定它的 id。

## 3. 决策顺序

按此顺序，**第一个不通过者即拒绝**，不继续：

| 序 | 检查 | 失败时 |
|---|---|---|
| 1 | 工作区成员资格（`workspace-core.Authorize`） | `RefusalStatus` / `RefusalBody`，与「不存在」同形 |
| 2 | 选题卡存在且属本工作区 | 同上 |
| 3 | 简报版本存在、属本工作区、且属这张卡 | 同上 |
| 4 | 账号存在且属本工作区 | 同上 |
| 5 | `source_scope` 在受控集内 | 400 诊断错误对象（`ErrScope` 的既有读法） |
| 6 | 账号就绪判定 `can_start == true` | 400 诊断错误对象，**列出 `missing[]`** |

**1–4 的拒绝体必须逐字节相同**：任何差异都能被用来探测某个 id 是否在别的品牌存在。

**6 与 1–4 不同形**是故意的：缺项是**调用方自己账号的**配置问题，告诉他缺什么不泄露任何别人的东西。

**没有第 7 条。** Q1=B 下重复开始是合法的，不是冲突：同一版简报的第二次开始产生第二份快照。**因此本端点不是幂等的**，这一点要写进 API 文档与页面（按钮在飞行中禁用），但**不得**用「已存在快照」去拒绝——那会把一个正常的产品行为变成错误。

---

## 4. 十六个字段怎么填（Q2=A）

| 快照字段 | 来源 | 本阶段 |
|---|---|---|
| `config_version` | 无来源 | **恒空** `""` |
| `persona_ref` | 账号当前 `content_account_revision.revision_id` | 开始那一刻读到的值 |
| `sop_version` | 无来源 | **恒空** |
| `skill_version` | 无来源 | **恒空** |
| `rule_version` | 无来源 | **恒空** |
| `executor` | 宪法 IX | 固定字面量，表示「禁用」 |
| `executor_version` | 无来源 | **恒空** |
| `source_scope` | **请求里本次的选择**（Q3=A） | `local` / `web` / `all` |
| `saved_preference` | 开始那一刻读到的 `settings["loretide.scope"]`（读不到填 `all`） | 与上一行**各自独立**，可以不同 |
| `required_sources` | EP-04d / W-03 | **恒空** `[]` |
| `excluded_sources` | EP-04d / W-03 | **恒空** `[]` |
| `grants` | LT-016 只有判定纯函数，无存储 | **恒空** `[]` |
| `file_hashes` | 无本地文件输入 | **恒空** `{}` |
| `temperature` | 不调模型 | `0` |
| `budget` | 简报的 `cost_limit` 是自由文本，不可映射为整数 | `0`，原因记在此 |
| `timeout_ms` | 简报的 `time_limit` 同上 | `0`，同上 |

**恒空的九项各有一条负例**（`config_version` / `sop_version` / `skill_version` / `rule_version` / `executor_version` / `required_sources` / `excluded_sources` / `grants` / `file_hashes`）：任一非空即红。笼统一条「都为空」也会红，但不会说是哪一个，而这九个各有各的接入方（EP-04d / W-03 / EP-08）。

### 两个扩展键（FR-015a）

| 键 | 记什么 | 为什么不挤进十六项 |
|---|---|---|
| `auto_precheck` | 开始那一刻的工作区 `settings["loretide.auto_precheck"]`（读不到填 `true`） | 事后要解释「这一篇为什么没走预检」。**触发是 EP-06，本卡不据此拒绝** |
| `uses_neutral_expression` | 开始那一刻由 021 判出的值 | 事后要解释成稿为什么是中性口吻。021 已定它是标记不是门槛，**不阻止开始** |

两者在 `diagnostics.Snapshot` 里没有对应字段，且十六项各有含义，借用任何一个都会让「与 Snapshot 对齐」变成半真。它们**必须**在代码注释与本合同里标注为 `topic-planning` 扩展。

### 三处同名不同物，不得混为一谈（Q3=A）

| 名字 | 是什么 | 受控集？ | 在快照里 |
|---|---|---|---|
| `BriefRevision.source_scope`（022） | §5.3 十一项里的「资料范围」**散文** | 否，自由文本 | **不参与**。它随简报版本一起被钉住 |
| `settings["loretide.scope"]`（LT-014） | 账号「上次选了什么」 | 是，`local/web/all` | `saved_preference` |
| 本次开始的选择 | 这一次实际用哪个范围 | 是，同上 | `source_scope` |

开始成功后把本次选择**写回账号偏好**（「保存上次选择」），走 LT-014 既有的独立端点语义，**不整体替换 settings**。

**本卡不得给 `BriefRevision.source_scope` 加受控集校验** —— 022 已存的自由文本行会因此变成非法。

---

## 5. 实体与响应

### `content_start_snapshot`

| 列 | 类型 | 说明 |
|---|---|---|
| `snapshot_id` | `text NOT NULL` | **被引用的稳定键**，将来 agent-workflow 的运行钉它。唯一性由单独的 CONCURRENTLY 索引保证，**不是** `PRIMARY KEY`（R5） |
| `workspace_id` | `text NOT NULL` | 一切查询按它过滤；工作区删除按它删 |
| `topic_card_id` | `text NOT NULL` | 冗余存放，使「按卡列出全部开始」不必回表 |
| `brief_revision_id` | `text NOT NULL` | 钉住的简报版本。**无外键**（原则 V），关系在应用代码里解 |
| `account_id` | `text NOT NULL` | 就绪判定的对象 |
| `project_id` | `text NOT NULL` | 可选，留空为 `''`——**空串是真实状态，不是缺失值**，与 `ResourceRef.Account` 同一读法 |
| `actor_id` | `text NOT NULL` | 谁开始的 |
| `snapshot` | `jsonb NOT NULL` | 十六个对齐字段 + 两个扩展键 |
| `created_at` | `timestamptz NOT NULL DEFAULT now()` | 服务端时间 |

**没有 `revision` 计数器。** `content_brief_revision` 需要它是因为 §5.3 要给人看「第几版」；快照没有这个需求，加一个只会让人以为它是跨表键。

### 成功响应

`201 Created`，响应体是新建的快照实体（含 `snapshot_id` 与 `snapshot`）。

---

## 6. 不可变的自证

- **没有 UPDATE 语句，没有 DELETE 语句**——工作区删除链里的那一条 `DELETE` 除外；
- 守卫用例照抄 022 的 A6 形状：扫 `pkg/db/queries/` 里这张表的全部语句，出现 `UPDATE` 或（删除链之外的）`DELETE` 即红；
- 写入与工作区删除栅栏在同一事务（#104 / `LockWorkspaceForContentDiagnosticWrite`）。

Q1=A 会要求在 `content_brief_revision` 上开一条受限 UPDATE，与 022 的 A6 冲突；**B 没有这个问题**，代价只是多一张表。

---

## 7. 迁移（四个文件，每个单条语句）

```sql
-- 490_content_start_snapshot.up.sql
-- 无 PRIMARY KEY、无 UNIQUE、无 REFERENCES：R1/R2/R5。
-- 唯一性与查询索引各自单独 CONCURRENTLY 建（R3/R4）。
CREATE TABLE IF NOT EXISTS content_start_snapshot (
    snapshot_id       text NOT NULL,
    workspace_id      text NOT NULL,
    topic_card_id     text NOT NULL,
    brief_revision_id text NOT NULL,
    account_id        text NOT NULL,
    project_id        text NOT NULL,
    actor_id          text NOT NULL,
    snapshot          jsonb NOT NULL,
    created_at        timestamptz NOT NULL DEFAULT now()
);
```

```sql
-- 491_content_start_snapshot_id_unique_idx.up.sql
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS content_start_snapshot_id_unique_idx
    ON content_start_snapshot (snapshot_id);
```

```sql
-- 492_content_start_snapshot_workspace_idx.up.sql
-- 一条索引服务两个查询：工作区删除按前导列，列出某版简报的全部开始按全部三列。
CREATE INDEX CONCURRENTLY IF NOT EXISTS content_start_snapshot_workspace_idx
    ON content_start_snapshot (workspace_id, brief_revision_id, created_at DESC);
```

```sql
-- 493_content_start_snapshot_card_idx.up.sql
-- 「这张卡一共开始过几次」是详情页的区块（T026 的入口），按卡查。
CREATE INDEX CONCURRENTLY IF NOT EXISTS content_start_snapshot_card_idx
    ON content_start_snapshot (workspace_id, topic_card_id, created_at DESC);
```

**每个 up 迁移都必须登记**进 `server/cmd/migrate/main.go` 的 `concurrentIndexCleanups`（#122 的 R6 会红）。登记的索引名要与迁移真正建的那个**逐字相同**——名字对不上的钩子是静默 no-op。

| 版本 | 登记的索引名 |
|---|---|
| `491_content_start_snapshot_id_unique_idx` | `content_start_snapshot_id_unique_idx` |
| `492_content_start_snapshot_workspace_idx` | `content_start_snapshot_workspace_idx` |
| `493_content_start_snapshot_card_idx` | `content_start_snapshot_card_idx` |

`490` 是建表，不建索引，**不登记**（登记一个不存在的索引同样是静默 no-op，R6 的反向断言会红）。

`main.go` 是上游文件 → 这三条登记与 `router.go` 的挂载**一起**放进独立的 `upstream:` 提交，PR 正文单列「上游改动」一节（工作流第 13 步）。

### 工作区删除清单

`content_start_snapshot` 必须加进 `server/internal/handler/workspace_delete_manifest_test.go` 的清单，标为 `workspaceDelete`，并在删除事务的同一 CTE 链里加一条按 `workspace_id` 的 `DELETE`。**无外键、无级联**，关系显式解。

### R5

建表迁移**不得**有 `PRIMARY KEY` 或 `UNIQUE`——它们会隐式建非并发索引。`content_constraints_test.go` 的 R5 从迁移 483 起生效，490 在范围内。

### down

四个 down：`493`/`492`/`491` 各 `DROP INDEX CONCURRENTLY IF EXISTS`（单语句），`490` `DROP TABLE IF EXISTS`。在 `490.down.sql` 的注释里写明**已开始的快照会因此丢失**，与 485 的处理一致。
