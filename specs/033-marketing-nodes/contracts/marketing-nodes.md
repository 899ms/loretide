# Contract: 营销节点联动选题（033）

**依据**：`spec.md`（R-056、D14-V01/02/03/08、主控前置决定 1–8）。Q1/Q2/Q3 按推荐值 A 写成；主控裁决不同时，受影响的段落在各节末尾标出。

**模块**：`topic-planning`。本合同里的所有 Go 类型都在 `server/internal/content/topic-planning/`，所有 TS 类型都在 `packages/core/content/topic-planning/`。

---

## 1. 存储

### 1.1 三张表

**`content_marketing_node`**——节点的当前指针。可 UPDATE（只改 `status`、`current_revision`、`updated_at`）。

| 列 | 类型 | 说明 |
|---|---|---|
| `node_id` | `text NOT NULL` | 服务端生成 |
| `workspace_id` | `text NOT NULL` | 品牌 |
| `status` | `text NOT NULL CHECK (status IN ('unconfirmed','active','cancelled'))` | |
| `origin` | `text NOT NULL CHECK (origin IN ('manual','import'))` | 服务端写 |
| `current_revision` | `bigint NOT NULL` | 与版本表里最大的 `revision` 相等 |
| `created_at` / `updated_at` | `timestamptz NOT NULL DEFAULT now()` | |

**`content_marketing_node_revision`**——只插。**不得有 UPDATE / DELETE 语句**（品牌删除链除外），由文本扫描守卫钉住（§7）。

| 列 | 类型 | 说明 |
|---|---|---|
| `revision_id` | `text NOT NULL` | |
| `node_id` / `workspace_id` | `text NOT NULL` | |
| `revision` | `bigint NOT NULL` | 从 1 起 |
| `change_kind` | `text NOT NULL CHECK (change_kind IN ('create','edit','reschedule','confirm','cancel'))` | 服务端按 §3.3 判定 |
| `status_after` | `text NOT NULL` | 这一版之后节点的状态，历史里能看出何时确认、何时取消 |
| `name` | `text NOT NULL` | 1–200 字 |
| `kind` | `text NOT NULL CHECK (kind IN ('holiday','industry','brand_campaign','marketing'))` | |
| `starts_on` / `ends_on` | `date NOT NULL` | 节点时区里的日历日期，含首尾（Q2=A） |
| `timezone` | `text NOT NULL` | IANA，非空、非 `Local` |
| `lead_days` | `integer` **可空** | NULL = 未设置；0 = 不需要准备（§3.2） |
| `accounts` | `jsonb NOT NULL DEFAULT '[]'::jsonb` | `[{ "account_id": "...", "role": "..." }]`，≤ 20 条，按 `account_id` 去重保留首次 |
| `goal` | `text NOT NULL DEFAULT ''` | 运营目标，≤ 2000 字 |
| `material_source_ids` | `jsonb NOT NULL DEFAULT '[]'::jsonb` | 复用 030 的 `NormalizeSourceIDs`（去空白、去重、≤ 50） |
| `date_certainty` | `text NOT NULL CHECK (date_certainty IN ('confirmed','tentative'))` | |
| `date_basis` | `text NOT NULL DEFAULT ''` | 日期依据，≤ 1000 字 |
| `note` | `text NOT NULL DEFAULT ''` | 这次修改的备注 |
| `actor` | `text NOT NULL` | |
| `created_at` | `timestamptz NOT NULL DEFAULT now()` | |

每一版存**完整内容**，不存差异。读历史不需要回放，改期前后的对比就是两行相减。

**`content_marketing_node_candidate`**——候选。可 UPDATE，但只能改人写的列与采用 / 决定记录（§4）。

| 列 | 类型 | 说明 |
|---|---|---|
| `candidate_id` | `text NOT NULL` | |
| `workspace_id` / `node_id` | `text NOT NULL` | |
| `account_id` | `text NOT NULL DEFAULT ''` | `''` = 品牌级候选。**用空串不用 NULL**：唯一索引里 NULL 互不相等，会让品牌级候选可以重复 |
| `angle` | `text NOT NULL DEFAULT ''` | 人写的切入角度，≤ 2000 字 |
| `status` | `text NOT NULL DEFAULT 'open' CHECK (status IN ('open','adopted','dismissed'))` | |
| `dismiss_reason` | `text NOT NULL DEFAULT ''` | |
| `topic_card_id` | `text NOT NULL DEFAULT ''` | 采用后非空 |
| `adopted_revision` | `bigint` 可空 | 采用时节点的 `current_revision` |
| `impact_decision` | `text NOT NULL DEFAULT '' CHECK (impact_decision IN ('','kept','handled'))` | |
| `impact_decision_note` | `text NOT NULL DEFAULT ''` | |
| `impact_decision_revision` | `bigint` 可空 | 做决定时节点的版本号 |
| `impact_decided_by` | `text NOT NULL DEFAULT ''` | |
| `created_at` / `updated_at` | `timestamptz NOT NULL DEFAULT now()` | |

准备时间、撞期、资料缺口、重复风险、关联理由**不存**，每次读取时算（§5）。存下来就会过期：「今天」每天都在变。

### 1.2 迁移清单（从 540 起；实施时若被占用则整体顺延）

| 号 | 文件 | 内容 |
|---|---|---|
| 540 | `540_content_marketing_node` | 建表 |
| 541 | `541_content_marketing_node_id_unique_idx` | `CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS ... ON content_marketing_node (node_id)` |
| 542 | `542_content_marketing_node_workspace_idx` | `(workspace_id, created_at DESC)` |
| 543 | `543_content_marketing_node_revision` | 建表 |
| 544 | `544_content_marketing_node_revision_id_unique_idx` | `UNIQUE (revision_id)` |
| 545 | `545_content_marketing_node_revision_node_revision_idx` | `UNIQUE (node_id, revision)`——并发修改的第二道防线（§3.4） |
| 546 | `546_content_marketing_node_candidate` | 建表 |
| 547 | `547_content_marketing_node_candidate_id_unique_idx` | `UNIQUE (candidate_id)` |
| 548 | `548_content_marketing_node_candidate_key_idx` | `UNIQUE (workspace_id, node_id, account_id)`——**候选的幂等键**（FR-019） |

每个文件都有 `.down.sql`。541、542、544、545、547、548 六个登记进 `server/cmd/migrate/main.go` 的 `concurrentIndexCleanups`。

### 1.3 R1–R6 逐条

| 规则 | 本卡 |
|---|---|
| R1 无 `REFERENCES` / `FOREIGN KEY` | 适用。候选指向节点与卡、版本指向节点，全部由应用在栅栏事务里校验 |
| R2 无 `CASCADE` | 适用 |
| R3 索引必须 `CONCURRENTLY` | 适用，六个 |
| R4 含并发索引的文件只一条语句 | 适用，六个文件各一条 |
| R5 建表不内联 `PRIMARY KEY` / `UNIQUE` | 适用，三张表都没有；唯一性全部来自 541/544/545/547/548 |
| R6 登记 `concurrentIndexCleanups` | 适用，六个 |
| 工作区删除链 | 三张表加进 `workspace_delete.sql`（删除顺序：候选 → 版本 → 节点，放在 `content_topic_card` 一段附近）与 `workspace_delete_manifest_test.go`（`workspaceDelete`）；重新生成 sqlc |

**这张表在交付时要逐行复核，不是默认成立**（SC-011）。

---

## 2. HTTP 端点

全部在 `/api/content-marketing-nodes` 下，挂在通用成员中间件**之外**、带 `h.DiagnosticTrace`，与 `/api/content-topics`（`router.go:1896`）同形：每个处理函数自己走 `workspace-core.Authorize`。品牌由 `X-Workspace-ID` 决定；路径参数只用 `chi.URLParam` 取。

| 方法 | 路径 | 用途 | PR |
|---|---|---|---|
| GET | `/` | 列节点（`?status=` 可选），返回当前版本 + 阶段 | 1 |
| POST | `/` | 手工新建（`active`，版本 1，`create`） | 1 |
| POST | `/import` | 批量导入（`unconfirmed`，逐行结果）。**注册在 `/{nodeId}` 之前** | 1 |
| GET | `/{nodeId}` | 读节点当前版本 + 阶段 | 1 |
| GET | `/{nodeId}/revisions` | 版本历史，按版本号升序 | 1 |
| POST | `/{nodeId}/revisions` | 修改 / 改期（带 `base_revision`） | 1 |
| POST | `/{nodeId}/confirm` | 确认（带 `base_revision`） | 1 |
| POST | `/{nodeId}/cancel` | 取消（带 `base_revision`、`note`） | 1 |
| GET | `/{nodeId}/candidates` | 列候选（含 §5 的读时计算） | 2 |
| POST | `/{nodeId}/candidates/sync` | 整理候选（幂等） | 2 |
| PATCH | `/{nodeId}/candidates/{candidateId}` | 改切入角度 / 放弃 / 改回待定 | 2 |
| POST | `/{nodeId}/candidates/{candidateId}/adopt` | 采用：新建卡或挂已有卡 | 2 |
| GET | `/{nodeId}/impact` | 影响清单 | 2 |
| POST | `/{nodeId}/candidates/{candidateId}/impact-decision` | 对影响记录决定 | 2 |

### 2.1 请求体

**新建 / 修改**（修改多一个 `base_revision`）：

```json
{
  "name": "双十一",
  "kind": "marketing",
  "starts_on": "2026-11-11",
  "ends_on": "2026-11-11",
  "timezone": "Asia/Shanghai",
  "lead_days": 14,
  "accounts": [{ "account_id": "a1", "role": "主推" }],
  "goal": "清库存 + 拉新",
  "material_source_ids": ["s1", "s2"],
  "date_certainty": "confirmed",
  "date_basis": "",
  "note": ""
}
```

- `lead_days` 省略或 `null` = 未设置；`0` = 不需要准备。修改是**整版提交**（每一版存完整内容），所以修改请求里省略 `lead_days` 的意思是「这一版未设置」，页面必须把原值带上。这与 030 的「缺省 = 不改」不同，原因是版本表存完整内容；页面层由表单状态保证不丢值（§6 的 `marketing-node-form.ts` 用例）。
- 未知键、多余 JSON 值 → 400（照 `TopicBodyPatch` 的 `DisallowUnknownFields`，`contract.go:128-141`）。

**导入**：`{ "rows": [ <与新建同形，无 note>, ... ] }`，≤ 100 行。CSV 解析在前端纯函数里做，服务端只收 JSON。

**确认**：`{ "base_revision": 1, "note": "" }`。**取消**：`{ "base_revision": 3, "note": "活动取消" }`。

**改候选**：`{ "angle"?: string, "status"?: "open" | "dismissed", "dismiss_reason"?: string }`，各键可缺省，缺省 = 不改。

**采用**：`{ "mode": "create" }` 或 `{ "mode": "link", "topic_card_id": "..." }`。

**影响决定**：`{ "decision": "kept" | "handled", "note": "" }`。

### 2.2 错误口径

| 情况 | 响应 |
|---|---|
| 字段不合法（日期顺序、跨度、时区、`kind`、长度、上限） | 400，`{"field": "...", "reason": "..."}`（`FieldError`，`contract.go:20`） |
| 节点 / 候选 / 卡 / 账号 / 素材不属于本品牌或不存在 | 404，**同一个响应体**（FR-042） |
| `base_revision` 不是当前版本 | 409 |
| 对 `unconfirmed` / `cancelled` 节点整理候选；对 `cancelled` 节点修改 / 确认；对已 `active` 节点确认 | 400，`field` 为 `status` |
| 挂的卡账号与候选账号不同 | 400，`field` 为 `topic_card_id` |
| 已采用的候选改为 `dismissed` | 400，`field` 为 `status` |

---

## 3. 节点规则

### 3.1 校验顺序

在栅栏事务**之外**只做纯形状校验（字段、长度、日期、时区可解析）；**账号与素材的归属校验在栅栏事务之内**（FR-044）。这一点与 `Create` 今天的写法不同（`store.go:167` 在栅栏外查账号），本卡照 `SetAccount` 注释（`store.go:308-310`）的要求做。

### 3.2 日期与阶段（纯函数，放 `marketing_node_dates.go`）

```go
// civil date in the node's zone; never time.Duration arithmetic on days.
func Today(now time.Time, loc *time.Location) Date
func PreparationStartsOn(startsOn Date, leadDays *int) (Date, bool) // false when lead unset
func Phase(today, startsOn, endsOn Date, leadDays *int) Phase
```

- `Date` 是年月日三元组，加减天数用 `time.Date(y, m, d+n, 12, 0, 0, 0, time.UTC)` 再取年月日，**不**用 `24*time.Hour` 相乘——夏令时切换那天不是 24 小时。
- 阶段：`today > ends_on` → `ended`；`starts_on ≤ today ≤ ends_on` → `live`；否则若 `lead_days` 未设置 → `before_start_unknown_lead`；否则 `today ≥ 准备期开始日` → `preparing`，再否则 `before_preparation`。
- 响应里的每个节点都带 `today`（`YYYY-MM-DD`）与 `timezone`，让页面写明「按 Asia/Shanghai 的 2026-11-11 计算」。
- 当前时刻由 `Store.Now func() time.Time` 注入，未设置时用 `time.Now`。
- 节点文件 `import _ "time/tzdata"`：`main.go:19` 的那一个只在 `main` 包里，包级单测在没有系统时区库的 Windows 上会读不到 `America/Los_Angeles`。

### 3.3 变更类型判定

与上一版相比，`starts_on`、`ends_on`、`timezone`、`lead_days`（含「未设置 ↔ 某值」）任一不同 → `reschedule`；否则 → `edit`。确认 → `confirm`；取消 → `cancel`。内容与上一版完全相同的修改请求 → 400（`field: "revision"`），不产生空版本。

### 3.4 并发修改

事务内：栅栏 → `SELECT ... FROM content_marketing_node WHERE workspace_id=$1 AND node_id=$2 FOR UPDATE` → 比较 `current_revision` 与 `base_revision`，不同则 409 → 插入 `revision = current + 1` → 更新指针。`(node_id, revision)` 唯一索引是第二道防线：即使有人绕过行锁，同一个版本号也插不进两次。

### 3.5 导入判重

事务内逐行：同品牌、`status <> 'cancelled'` 的节点，当前版本 `btrim(name)` 与本行相同且 `starts_on` 相同 → 本行记为 `duplicate`（带已有节点号）。同一批里后出现的相同行也记为 `duplicate`。每行结果：`{"row": 3, "outcome": "created" | "duplicate" | "invalid", "node_id"?: "...", "field"?: "..."}`。

---

## 4. 候选规则

### 4.1 整理（sync）

事务内：栅栏 → 锁节点行 → 节点必须 `active` → 对当前版本的每个 `account_id`（没有则 `''`）执行

```sql
INSERT INTO content_marketing_node_candidate (candidate_id, workspace_id, node_id, account_id)
VALUES ($1, $2, $3, $4)
ON CONFLICT (workspace_id, node_id, account_id) DO NOTHING
```

`ON CONFLICT` 依赖 548 的唯一索引。**不更新任何已有行**：人写的角度、状态、采用记录都不碰（FR-020）。不删除任何行：账号被移出适用范围后，它的候选在读取时标 `in_scope: false`（FR-021）。

注意：节点在「有适用账号」与「没有适用账号」之间切换后，品牌级候选与账号候选可能同时存在，都保留，读取时各自标 `in_scope`。

### 4.2 改候选（PATCH）

只改 `angle`、`status`（`open` ↔ `dismissed`）、`dismiss_reason`。`status` 为 `adopted` 时拒绝改为 `dismissed`（FR-036）；`angle` 在采用后仍可改（卡已经建好，改角度不影响卡）。

### 4.3 采用

事务内：栅栏 → `SELECT ... FROM content_marketing_node_candidate ... FOR UPDATE` →

1. 候选已 `adopted` → 直接返回已有的卡（200，同一响应体），**不再建**（FR-033）。
2. `mode=create` → 在同一事务里校验候选账号仍属于本品牌（`AccountReader`）、节点引用的素材仍属于本品牌（`SourceReader`）→ 调用从 `Create` 抽出的事务内插入函数建 `draft` 卡 → 更新候选 `status='adopted'`、`topic_card_id`、`adopted_revision = 节点 current_revision` → 审计 `adopt-marketing-candidate` → 提交。
3. `mode=link` → 读卡（`workspace_id` 过滤，不存在即 404）→ 卡有账号且 ≠ 候选账号（品牌级候选不限）→ 400 → 更新候选 → 审计 → 提交。**卡的行不被写**。

预填（FR-031）：

| 卡字段 | 值 |
|---|---|
| `account_id` | 候选的 `account_id`；`''` → NULL |
| `timing` | 固定模板，四语言由服务端**不**翻译：`"{name}｜{starts_on}–{ends_on}（{timezone}）｜准备期自 {prep_starts_on}"`；提前量未设置时最后一段写 `准备期未设置`。**这是一段拼出来的事实，不是生成的文字** |
| `ip_fit` | 候选的 `angle`（可为空） |
| `fit_source_ids` | 节点当前版本的 `material_source_ids` |
| 其余正文、`channels`、`recommended_action`、`evidence_source_ids` | 空 |
| `status` | `draft` |

`timing` 模板语言是一个待主控确认的小点（plan.md「主控决定」第 4 条）：服务端只存一种写法，推荐中文，因为本产品的使用者与节点名本身都是中文。

### 4.4 影响清单

读取时计算：本节点所有 `status='adopted'` 的候选中，满足

- 版本表里存在 `revision > adopted_revision` 且 `change_kind IN ('reschedule','cancel')` 的版本；并且
- `impact_decision_revision IS NULL` 或 `impact_decision_revision < current_revision`

的那些。每条返回：`candidate_id`、`topic_card_id`、`account_id`、卡的当前 `status`（同模块直接读 `content_topic_card`，卡不存在时写 `missing`）、`adopted_revision`、`current_revision`、`before`（采用时版本的四个日期字段）、`after`（当前版本的四个字段）、`cancelled`（布尔）。

影响决定：写 `impact_decision`、`impact_decision_note`、`impact_decision_revision = current_revision`、`impact_decided_by`，审计 `decide-marketing-impact`。**不写卡、不写任何其他表**（FR-040）。

---

## 5. 候选的读时计算

`GET /{nodeId}/candidates` 每条候选附带：

```json
{
  "candidate_id": "c1", "node_id": "n1", "account_id": "a1",
  "angle": "", "status": "open", "dismiss_reason": "",
  "topic_card_id": "", "adopted_revision": null,
  "impact_decision": "", "impact_decision_revision": null,
  "in_scope": true,
  "timing": {
    "today": "2026-11-01", "timezone": "Asia/Shanghai",
    "phase": "preparing",
    "preparation_starts_on": "2026-10-28",
    "days_until_start": 10, "lead_days": 14, "lead_short": true
  },
  "relation": {
    "goal": "清库存 + 拉新", "role": "主推",
    "account": {
      "audience": { "value": "…", "status": "confirmed" },
      "content_pillars": { "value": "", "status": "pending" },
      "content_goals": { "value": "…", "status": "confirmed" }
    }
  },
  "collisions": [{ "node_id": "n2", "name": "…", "starts_on": "…", "ends_on": "…" }],
  "material_gaps": [{ "kind": "archived", "source_id": "s2" }],
  "duplicate_risks": [{ "topic_card_id": "t9", "reason": "name_match" }],
  "origin": "manual", "date_certainty": "confirmed", "date_basis": ""
}
```

| 字段 | 规则 |
|---|---|
| `timing` | §3.2。`lead_short` = 未开始且 `days_until_start < lead_days`；提前量未设置时为 `null`，**不是 `false`** |
| `relation.account` | 经 `AccountReader.CurrentPersonaRevision` 读该账号当前配置的三项，原样带出 `value` 与 `status`；品牌级候选或账号从未配置时为 `null`。页面对 `status` 非 `confirmed` 或 `value` 为空的一项显示「未填写」 |
| `collisions` | 同品牌、`status='active'`、非本节点的节点中，区间 `[准备期开始日或开始日, 结束日]` 与本节点同样区间相交，且（任一方适用账号为空，或双方共享本候选的账号）。**区间比较用各自时区里的日期直接比**——两个节点时区不同时，日期按字面比较，这是 Q2=A 的已知近似，写进页面说明 |
| `material_gaps` | 节点无引用 → `[{kind:"none"}]`；每条引用经新增的**只用字符串**的端口 `SourceStatusReader.Status(ctx, workspaceID, sourceID) (status string, found bool, err error)` 查，`archived` → `archived`，找不到 → `missing` |
| `duplicate_risks` | FR-027。`name_match`：同品牌、同账号（品牌级候选为全品牌）、`status <> 'dropped'`、五项正文任一项 `ILIKE '%' || 节点名 || '%'`（节点名去首尾空白，少于 2 个字时跳过；`%` `_` 转义）；`adopted_from_node`：本节点其他候选采用过的卡 |

**关联理由里没有一句话是系统写的。** 每个字段都是从某张表里某一列原样读出来的，或是对日期的算术。

---

## 6. 前端（core）

`packages/core/content/topic-planning/marketing-nodes.ts`：

- zod schema：节点、版本、候选、影响条目、导入结果。
- 未知的 `phase` / `change_kind` / `material_gaps[].kind` / `duplicate_risks[].reason` → `{ kind: "unknown", raw }`，**不丢整条**。
- `lead_days`：缺失或 `null` → `{ set: false }`；数字 → `{ set: true, value }`。**绝不把缺失读成 0**。
- `lead_short`：`null` → `"unknown"`，`true` / `false` 照读；`=== true` 判断（宪法 VI）。

`marketing-node-import.ts`：CSV 文本 → 行数组。纯函数，node 用例覆盖：引号内逗号、空行、BOM、列数不对、`lead_days` 空与 `0`。**不做**日期合法性判断以外的任何推断（比如不从名称猜类型）。

`marketing-node-form.ts`：编辑表单状态。整版提交时从当前版本带上所有字段，保证「没动的字段」不会因为省略而变成未设置（§2.1）。

---

## 7. 守卫与变异

| 守卫 | 形状 |
|---|---|
| 版本表只插 | 扫 `marketing_node*.go`（不含 `_test.go`），不得出现 `UPDATE CONTENT_MARKETING_NODE_REVISION` / `DELETE FROM CONTENT_MARKETING_NODE_REVISION`；**必须**出现 `INSERT INTO CONTENT_MARKETING_NODE_REVISION`（否则守卫空转） |
| 不碰下游表 | 同一批文件不得出现对 `content_topic_card`（除经抽出的插入函数）、`content_brief_revision`、`content_start_snapshot`、`content_work`、`content_artifact_version`、`content_review_*`、`content_delivery_task`、`content_publication_record` 的 `UPDATE` / `DELETE` |
| 不生成文字 | `marketing_node*.go` 不 import 任何 agent / executor 包；`check:content-boundaries` 已把 `topic-planning` 的依赖限死，本条再加一条文本断言以便失败时说清原因 |
| 素材只看品牌与存在 | 负例：`SourceStatusReader` 与 `SourceReader` 的签名里没有账号参数；若将来加了，这条用例要求同一 PR 接入 `CanRead` |

变异清单见 `plan.md`。
