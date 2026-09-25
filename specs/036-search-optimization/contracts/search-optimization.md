# 合同：平台搜索优化（036）

**状态**：已裁决（主控已裁定 2026-09-25，PR #272 评论）：Q1～Q8 采纳推荐值；实施五个 PR（原 PR 2 拆成 PR 2 / PR 3）。每个列名、受控集都要能指回 `spec.md` 开头的 R-060 原文或主控 D1–D8，指不回去的在 `spec.md`「本规格补的设计」里单列。

| 部分 | 模块 | Go 文件前缀 | handler 文件 |
|---|---|---|---|
| 搜索主题；建议、决定、效果、差异 | `topic-planning` | `search_` | `content_search_themes.go`、`content_search_suggestions.go` |
| `ApplyBody` | `work-editor` | `apply.go` | （由 `content_search_suggestions.go` 调用） |
| 搜索指标、排名观察 | `feedback-learning` | `search_` | `content_search_observations.go` |

路由前缀 `/api/content-search`。

---

## 1. 五张表与一处 CHECK 变更

**迁移编号不预留**（D6）：每个实施 PR 合入前把自己的迁移改号为紧接当时 `app-main` 最大号之后的连续号。规格撰写时最大号 584，仅供参考。每张表：

- 建表迁移不含 `PRIMARY KEY` / `UNIQUE` / `REFERENCES` / `CASCADE`（R1、R2、R5）；
- 受控集用 `CHECK (... IN (...))` 兜底，Go 为准；
- 只插不改：没有 `UPDATE`，没有 `ON CONFLICT DO UPDATE`，没有删除链之外的 `DELETE`；
- 进 `workspace_delete_manifest_test.go` 与 `workspace_delete.sql` 的同一 CTE 链。

**修订制的共同列**（§1.1、§1.2、§1.6 省略不写）：`workspace_id text NOT NULL`、`revision integer NOT NULL`（从 1 起）、`voided boolean NOT NULL DEFAULT false`、`recorded_by text NOT NULL`（服务端从会话取）、`created_at timestamptz NOT NULL DEFAULT now()`。当前状态 = 同一 id 的最大 `revision`。

### 1.1 `content_search_theme_revision` —— 搜索主题（`topic-planning`，PR 1）

| 列 | 类型 | 指回 | 说明 |
|---|---|---|---|
| `theme_id` | text NOT NULL | — | 稳定键 |
| `name` | text NOT NULL | R-060「关键词/主题组」 | 1～100 rune |
| `platform` | text NOT NULL CHECK 八项 | R-060「所选中国平台」 | `ipprofile.Platforms` |
| `account_id` | text NOT NULL DEFAULT '' | R-060「账号」 | '' = 品牌级 |
| `business_goal` | text NOT NULL DEFAULT '' | R-060「经营目标」 | ≤2000 rune |
| `questions` | jsonb NOT NULL DEFAULT '[]' | R-060「用户搜索问题」 | 字符串数组，0～50 条，每条 ≤500 rune |
| `keywords` | jsonb NOT NULL DEFAULT '[]' | R-060「关键词/主题组」 | 字符串数组，0～100 个，每个 ≤100 rune |
| `intent` | text NOT NULL CHECK §3 | R-060「搜索意图」 | Q7 |
| `origin` | text NOT NULL CHECK §3 | R-060「人工输入关键词、已有客户提问及授权素材」 | |
| `origin_note` | text NOT NULL DEFAULT '' | D14-V09「来源可追溯」 | ≤2000 rune；`customer_question` 必填 |
| `source_ids` | jsonb NOT NULL DEFAULT '[]' | R-060「关联资料」 | ≤50；`authorized_material` 至少一个 |
| `topic_card_ids` | jsonb NOT NULL DEFAULT '[]' | R-060「候选选题」 | ≤50 |
| `brief_revision_ids` | jsonb NOT NULL DEFAULT '[]' | R-060「内容简报」 | ≤50 |
| `note` | text NOT NULL DEFAULT '' | — | ≤2000 rune |

**没有** `search_volume`、`competition`、`rank` 列（FR-014，Q5）。

### 1.2 `content_search_suggestion_revision` —— 优化建议（`topic-planning`，PR 2）

| 列 | 类型 | 指回 | 说明 |
|---|---|---|---|
| `suggestion_id` | text NOT NULL | — | 稳定键 |
| `work_id`, `artifact_id`, `base_version_id` | text NOT NULL | R-060「修改差异」；文档 14「关联作品候选版本」 | 三项在修订之间不变（FR-030） |
| `theme_id` | text NOT NULL | R-060「目标问题」 | 同工作区的主题 |
| `theme_revision` | integer NOT NULL | D14-V09 可追溯 | 服务端写：建这一修订时主题的当前修订 |
| `target_question` | text NOT NULL | R-060「准确回答目标问题」 | 1～500 rune |
| `aspects` | text[] NOT NULL | R-060「标题、正文、话题及平台允许配置的描述」 | §3 `SuggestionAspects` 的非空子集，已排序 |
| `rationale` | text NOT NULL | R-060「依据」 | 1～5000 rune |
| `evidence_source_ids` | jsonb NOT NULL DEFAULT '[]' | R-060「依据」 | ≤50 |
| `proposed_body` | text NOT NULL | R-060「可编辑建议」 | ≤200000 rune，≠ 基础版本正文 |
| `author_kind` | text NOT NULL CHECK `human` | D1 | 本版恰好一项 |

### 1.3 `content_search_suggestion_decision` —— 决定（`topic-planning`，PR 2，只插）

| 列 | 说明 |
|---|---|
| `workspace_id`, `decision_id` | |
| `suggestion_id`, `suggestion_revision` | 决定时的当前修订 |
| `decision` CHECK `adopt`/`abandon` | FR-050 |
| `note` text NOT NULL DEFAULT '' | ≤2000 rune |
| `decided_by`, `created_at` | |

同一 `suggestion_id` 只能有一行：唯一索引兜底（§2），Go 先查，冲突映射为 409 `suggestion_id`。

### 1.4 `content_search_suggestion_effect` —— 效果（`topic-planning`，PR 2 建表、PR 3 写入，只插）

| 列 | 说明 |
|---|---|
| `workspace_id`, `effect_id` | |
| `decision_id` | |
| `outcome` CHECK `done`/`failed` | |
| `version_id` text NOT NULL DEFAULT '' | `done` 时为 `ApplyBody` 返回的新版本；`failed` 时为 '' |
| `failure_code` text NOT NULL DEFAULT '' CHECK `''` 或 §3 `EffectFailures` | |
| `created_at` | |

一个决定可有多条效果（失败后重试）；**最多一条 `done`**：Go 在写入前查（同一事务里先对该决定行 `SELECT ... FOR UPDATE`，并发的两次重试因此排队，后到者看得见先到者的 `done`），另有一条用例。不加部分唯一索引（与 035 §1.6 同一理由）。

### 1.5 `content_search_metric` —— 搜索指标（`feedback-learning`，PR 4，只插）

与 027 `content_manual_metric` 同形（Q2=A）：

| 列 | 类型 | 说明 |
|---|---|---|
| `workspace_id`, `search_metric_id` | text NOT NULL | |
| `publication_record_id` | text NOT NULL | 经 `feedbackPublications.Resolve` 核实 |
| `platform` | text NOT NULL CHECK 四项 | `feedbacklearning.Platforms` |
| `account_id` | text NOT NULL DEFAULT '' | 非空时经 `Accounts.AccountExists` 核实 |
| `metric` | text NOT NULL CHECK §3 | 恰好两项 |
| `value` | bigint NULL | NULL = 未知，0 = 确认为零；≥ 0 |
| `unit` | text NOT NULL DEFAULT '' | ≤500 rune |
| `stat_window` | text NOT NULL | **必填**，1～500 rune |
| `sampled_at` | timestamptz NOT NULL | |
| `evidence_note` | text NOT NULL DEFAULT '' | ≤20000 rune |
| `source_type` | text NOT NULL CHECK `manual` | 服务端写 |
| `recorded_by`, `created_at` | | |

### 1.6 `content_search_rank_observation_revision` —— 排名观察（`feedback-learning`，PR 4）

| 列 | 类型 | 说明 |
|---|---|---|
| `observation_id` | text NOT NULL | 稳定键 |
| `platform` | text NOT NULL CHECK 四项 | Q8=A |
| `account_id` | text NOT NULL DEFAULT '' | 观察用的是哪个品牌账号（可空）；经适配器核实 |
| `query` | text NOT NULL | 1～200 rune，NFC + 去首尾空白 |
| `theme_id` | text NOT NULL DEFAULT '' | 可空；经适配器向 `topic-planning` 核实 |
| `publication_record_id` | text NOT NULL DEFAULT '' | 可空；经 `feedbackPublications.Resolve` 核实 |
| `observed_at` | timestamptz NOT NULL | 不晚于服务器时间 + 10 分钟 |
| `conditions` | text NOT NULL | 1～1000 rune：登录状态、地区、设备、排序方式等 |
| `result_kind` | text NOT NULL CHECK §3 | |
| `position` | integer NULL | `position` 时必填且 ≥ 1，否则 NULL |
| `scanned_depth` | integer NULL | `not_found` 时必填且 ≥ 1，否则 NULL |
| `evidence_note` | text NOT NULL | 1～2000 rune |

`CHECK ((result_kind = 'position' AND position >= 1 AND scanned_depth IS NULL) OR (result_kind = 'not_found' AND scanned_depth >= 1 AND position IS NULL))` 兜底。

### 1.7 `content_artifact_version` 的 `action` CHECK（`work-editor`，PR 3，Q3=A）

照迁移 535，一条 `ALTER TABLE`：

```sql
ALTER TABLE content_artifact_version
    DROP CONSTRAINT IF EXISTS content_artifact_version_action_check,
    ADD CONSTRAINT content_artifact_version_action_check
        CHECK (action IN ('saved', 'restored', 'adopted', 'imported', 'suggestion_applied'));
```

down 迁移还原为四项（注释写明：已有 `suggestion_applied` 行时 down 会失败，要先人工处理这些行）。`source` 集合**不变**（仍是 SOP 7.1 的三项）。

---

## 2. 索引（每个一个迁移文件、单条语句、`CONCURRENTLY`，登记进 `concurrentIndexCleanups`）

| 索引 | 表 | 列 | 唯一 | 用途 |
|---|---|---|---|---|
| `content_search_theme_revision_key_idx` | 1.1 | `(workspace_id, theme_id, revision)` | 是 | 修订号并发兜底 |
| `content_search_theme_revision_time_idx` | 1.1 | `(workspace_id, created_at)` | 否 | 列表 |
| `content_search_suggestion_revision_key_idx` | 1.2 | `(workspace_id, suggestion_id, revision)` | 是 | 修订号并发兜底 |
| `content_search_suggestion_revision_doc_idx` | 1.2 | `(workspace_id, artifact_id, created_at)` | 否 | 按文档列建议 |
| `content_search_suggestion_decision_key_idx` | 1.3 | `(workspace_id, suggestion_id)` | 是 | 一条建议一个决定 |
| `content_search_suggestion_effect_decision_idx` | 1.4 | `(workspace_id, decision_id, created_at)` | 否 | 读效果 |
| `content_search_metric_id_idx` | 1.5 | `(workspace_id, search_metric_id)` | 是 | 同 027 迁移 527 |
| `content_search_metric_record_idx` | 1.5 | `(workspace_id, publication_record_id, sampled_at)` | 否 | 按发布记录读 |
| `content_search_rank_observation_key_idx` | 1.6 | `(workspace_id, observation_id, revision)` | 是 | 修订号并发兜底 |
| `content_search_rank_observation_theme_idx` | 1.6 | `(workspace_id, theme_id, observed_at)` | 否 | 按主题读 |
| `content_search_rank_observation_record_idx` | 1.6 | `(workspace_id, publication_record_id, observed_at)` | 否 | 按发布记录读 |

合计：PR 1 两个、PR 2 四个、PR 4 五个。

---

## 3. 受控集（Go 为准，`CHECK` 兜底，各一条「恰好 N 项」用例）

`topic-planning`（登记进 `search_guards_test.go` 的受控集清单用例）：

| 集合 | 恰好 | 取值 | 指回 |
|---|---|---|---|
| `SearchIntents` | 6 | `learn`、`solve`、`compare`、`buy`、`find`、`unclassified` | R-060「搜索意图」；Q7 |
| `ThemeOrigins` | 3 | `manual_keyword`、`customer_question`、`authorized_material` | R-060 第 2 条 |
| `SuggestionAspects` | 4 | `title`、`body`、`topics`、`description` | R-060 第 3 条 |
| `AuthorKinds` | 1 | `human` | D1 |
| `SuggestionDecisions` | 2 | `adopt`、`abandon` | R-060 第 4 条；D14-V10 |
| `EffectOutcomes` | 2 | `done`、`failed` | |
| `EffectFailures` | 5 | `base_moved`、`draft_unsaved`、`no_change`、`target_not_found`、`storage` | FR-053 |
| `SuggestionStates`（读时派生，不入库） | 5 | `open`、`adopted`、`adopt_failed`、`adopt_unrecorded`、`abandoned` | FR-038 |
| `UnknownReasons`（读时派生） | 1 | `no_data_source` | D2 |
| `DataOrigins` | 1 | `manual_only` | D1 |

`feedback-learning`（登记进 027 `guards_test.go` 的 `TestThereIsNoSixthControlledSet`）：

| 集合 | 恰好 | 取值 |
|---|---|---|
| `SearchMetrics` | 2 | `search_impression`、`search_visit` |
| `RankResultKinds` | 2 | `position`、`not_found` |
| `SearchMetricSources` | 1 | `manual` |

规则 id（前端翻译，服务端只输出 id）：`rank.single_observation`、`volume.no_data_source`、`competition.no_data_source`、`optimization.answer_the_question`、`optimization.no_ranking_promise`、`search.local_only`。守卫：这些 id 与 Go 字符串字面量不含排名承诺词（FR-081），只放行 `optimization.no_ranking_promise` 这一个否定句的 id。

---

## 4. 请求与响应

所有请求体严格解码（`DisallowUnknownFields`），未知字段 400 点名该字段。服务端写的字段（`recorded_by`、`theme_revision`、`author_kind`、`source_type`、`data_origin`）请求体里出现时**接受并丢弃**（同 034 `roiServerWritten`：回显记录的客户端不能因此失败）。

### 4.1 主题

请求（创建）：`name, platform, account_id, business_goal, questions, keywords, intent, origin, origin_note, source_ids, topic_card_ids, brief_revision_ids, note`。修订另带 `base_revision`（必填）与 `voided`。

响应：

```json
{
  "theme_id": "…", "revision": 2, "voided": false,
  "name": "羊绒大衣怎么洗", "platform": "xiaohongshu", "account_id": "…",
  "business_goal": "…", "questions": ["…"], "keywords": ["…"],
  "intent": "solve", "origin": "customer_question", "origin_note": "…",
  "source_ids": [], "topic_card_ids": [], "brief_revision_ids": [], "note": "",
  "recorded_by": "…", "created_at": "…",
  "search_volume": {"status": "unknown", "reason": "no_data_source"},
  "competition":   {"status": "unknown", "reason": "no_data_source"},
  "data_origin": "manual_only"
}
```

「排名」**不在**主题响应里：它来自 `feedback-learning` 的观察列表（§4.4），页面组合（FR-076）。`GET /themes/{themeId}/revisions` 返回全部修订，最新在前。

### 4.2 建议

请求（创建）：`work_id, artifact_id, base_version_id, theme_id, target_question, aspects, rationale, evidence_source_ids, proposed_body`。修订另带 `base_revision`；修订里出现 `work_id`/`artifact_id`/`base_version_id`/`theme_id` 与首修订不同 → 400 点名该字段。

响应：

```json
{
  "suggestion_id": "…", "revision": 1,
  "work_id": "…", "artifact_id": "…", "base_version_id": "…",
  "theme_id": "…", "theme_revision": 2, "target_question": "…",
  "aspects": ["body", "title", "topics"], "rationale": "…",
  "evidence_source_ids": [], "proposed_body": "…", "author_kind": "human",
  "recorded_by": "…", "created_at": "…",
  "state": "open", "base_is_current": true, "theme_changed": false,
  "diff": {"ops": [{"op": "equal", "text": "…"}, {"op": "delete", "text": "…"}, {"op": "insert", "text": "…"}],
           "inserted_lines": 3, "deleted_lines": 1},
  "decision": null, "effects": []
}
```

`decision` 与 `effects` 在有记录时给出；`diff` 只在单条读取与比较里给，列表不给（省流量）。

### 4.3 搜索指标

请求：`publication_record_id, platform, account_id, metric, value, unit, stat_window, sampled_at, evidence_note`。`value` 可为 `null`。响应在请求字段之外加 `search_metric_id, recorded_by, source_type, created_at, data_origin`。

### 4.4 排名观察

请求：`platform, account_id, query, theme_id, publication_record_id, observed_at, conditions, result_kind, position, scanned_depth, evidence_note`。修订另带 `base_revision` 与 `voided`。响应另加 `observation_id, revision, voided, recorded_by, created_at, data_origin`，以及固定的 `rule: "rank.single_observation"`。**没有**任何跨观察字段。

---

## 5. 建议状态（读时派生）

| 条件 | `state` |
|---|---|
| 没有决定 | `open` |
| 决定 `abandon` | `abandoned` |
| 决定 `adopt`，有一条 `done` 效果 | `adopted` |
| 决定 `adopt`，没有效果 | `adopt_unrecorded` |
| 决定 `adopt`，最新效果 `failed` | `adopt_failed`（另给 `failure_code`） |

`base_is_current` = 适配器读到的文档最新版本 `version_id` == `base_version_id`。`theme_changed` = 主题当前修订 > `theme_revision` 或当前修订 `voided`。都不落存储。

---

## 6. 差异 `DiffLines(base, proposed string) Diff`

- 纯函数：不读库、不读时钟；放在 `topic-planning/search_diff.go`。
- 两边都按 `\n` 切行（保留空行；末尾换行差异算一行差异）；不做 NFC、不去空白：差异描述的是真实字节。
- 最短编辑脚本（Myers，O((N+M)·D)）；相邻同类操作合并；多个等长解时优先「先删后插」，保证确定性。
- 输出 `ops`（`equal` / `delete` / `insert`，每项带原文行，合并后的多行用 `\n` 连接）与 `inserted_lines`、`deleted_lines`。
- 固定样例（逐字进 `search_diff_test.go`）：`identical`（全 `equal`，两个计数为 0——供守卫用，建议本身不允许无变化）、`append-line`、`replace-first-line`、`delete-middle`、`trailing-newline`、`empty-base`、`chinese-paragraphs`（三段中文，只改第二段）、`crlf-kept`（`\r` 原样留在行内）。

---

## 7. 端点（前缀 `/api/content-search`，全部在 `RequireWorkspaceMember` 组之外，handler 内 `Authorize`，`h.DiagnosticTrace`）

| 方法 | 路径 | PR | 说明 |
|---|---|---|---|
| GET | `/themes` | 1 | `platform`、`account_id`、`topic_card_id`、`include_archived` 过滤 |
| POST | `/themes` | 1 | 建主题（修订 1） |
| GET | `/themes/{themeId}` | 1 | 当前修订 |
| GET | `/themes/{themeId}/revisions` | 1 | 全部修订 |
| POST | `/themes/{themeId}/revisions` | 1 | 新修订 / 归档 |
| GET | `/suggestions` | 2 | `work_id`、`artifact_id`、`theme_id`、`state` 过滤 |
| POST | `/suggestions` | 2 | 建建议 |
| GET | `/suggestions/compare` | 2 | `ids=a,b[,c,d]`；只读 |
| GET | `/suggestions/{suggestionId}` | 2 | 当前修订 + 差异 + 状态 |
| POST | `/suggestions/{suggestionId}/revisions` | 2 | 新修订 |
| POST | `/suggestions/{suggestionId}/decisions` | 2 | `{decision, revision, note}`；PR 2 只接受 `abandon`，PR 3 放开 `adopt` |
| POST | `/decisions/{decisionId}/retry` | 3 | 重试采用 |
| GET | `/metrics` | 4 | `publication_record_id` 必填 |
| POST | `/metrics` | 4 | 登记一条 |
| GET | `/rank-observations` | 4 | `theme_id` / `publication_record_id` / `query` 至少一个；`include_voided` |
| POST | `/rank-observations` | 4 | 登记一次 |
| POST | `/rank-observations/{observationId}/revisions` | 4 | 更正 / 作废 |

`/suggestions/compare` 是静态段，必须先于 `/suggestions/{suggestionId}` 匹配；路由用例各一条。

### 7.1 决策顺序（第一个不通过者即拒绝）

1. `Authorize`（非成员 → 与不存在同形）；
2. 路径参数 → 本模块读（不存在 / 别的品牌 → 404 同形）；
3. 严格解码与字段校验（400 点名字段）；
4. 引用核实（素材、选题卡、简报、账号、主题、发布记录、作品版本；→ 400 点名字段，或与不存在同形）；
5. 修订号 / 状态（409 点名 `base_revision` / `revision` / `suggestion_id` / `decision_id`）；
6. 采用的预检查（409 点名 `base_version_id` / `draft_status`）；
7. 写（栅栏 → 业务行 → 审计）。

### 7.2 采用流程

```text
POST /suggestions/{id}/decisions {decision: adopt, revision: r}
  ① 读建议（本模块）→ 当前修订 == r？否 → 409 revision
  ② 已有决定？是 → 409 suggestion_id
  ③ 适配器 Document(work_id, artifact_id) → latest_version_id == base_version_id？否 → 409 base_version_id
                                        → draft_status == saved？否 → 409 draft_status
  ④ 事务 A（topic-planning）：栅栏 → 再查一次「已有决定？」→ 插决定 → 审计 → 提交
  ⑤ 适配器 Apply(key = "search-suggestion:" + suggestion_id, base_version_id, proposed_body)
  ⑥ 事务 B（topic-planning）：栅栏 → 查「已有 done？」→ 插效果（done + version_id / failed + code）→ 审计 → 提交
  响应：建议（含 decision 与 effects）
```

⑤ 失败 → ⑥ 写 `failed`；⑥ 失败 → 状态 `adopt_unrecorded`，重试即可。重试（`POST /decisions/{decisionId}/retry`）= 从 ⑤ 开始，**不**再做 ③（幂等重放不看当前状态）。

`abandon`：①②，然后事务 A 写决定（`abandon`）与审计。**不**做 ③⑤⑥。

### 7.3 模块接口（只用本模块类型与字符串）与适配器

`topic-planning/search_ports.go`：

```go
// SearchDocument is what adoption needs to know about the target document.
type SearchDocument struct {
	WorkID, ArtifactID string
	LatestVersionID    string // "" when the document has no version
	DraftSaved         bool
}

type SearchApply struct {
	Key, WorkID, ArtifactID, BaseVersionID, Body string
}

// SearchWorks is implemented by handler/content_search_suggestions.go over
// work-editor's public Store. topic-planning does not import work-editor.
type SearchWorks interface {
	Document(ctx context.Context, workspaceID, actor, workID, artifactID string) (SearchDocument, error)
	VersionBody(ctx context.Context, workspaceID, actor, workID, artifactID, versionID string) (string, error)
	Apply(ctx context.Context, workspaceID, actor string, apply SearchApply) (versionID string, err error)
}
```

错误：`ErrNotFound`、`ErrStorage`，以及新增的 `ErrBaseMoved`、`ErrDraftUnsaved`、`ErrNoChange`（`topic-planning` 自己的哨兵，`FieldError` 形式点名字段 → 409 / 400）。

`feedback-learning/search_ports.go`：

```go
// SearchThemes answers "is this search theme here". Implemented by
// handler/content_search_observations.go over topic-planning's Store.
type SearchThemes interface {
	ThemeExists(ctx context.Context, workspaceID, actor, themeID string) error
}
```

账号核实复用 034 的 `Accounts`（`roiAccounts`），发布记录核实复用 027 的 `feedbackPublications`。

**每个适配器方法的错误映射**（FR-104，照 `opdiagReadError`）：

| 对方返回 | 映射为 |
|---|---|
| `nil` | `nil` |
| `workspacecore.ErrNotFound`、对方 `ErrNotFound`、对方 `ErrInvalid`（例如 id 格式不对） | 调用方模块的 `ErrNotFound` |
| `workeditor.ErrBaseMoved` / `ErrDraftUnsaved` / `ErrNoChange` | `topicplanning.ErrBaseMoved` / `ErrDraftUnsaved` / `ErrNoChange` |
| 其余 | 调用方模块的 `ErrStorage` |

适配器里的 nil store 返回调用方的 `ErrStorage`。每个方法一条「工作区删除已提交后」用例，断言端点 404。

### 7.4 `work-editor.ApplyBody`（公开契约新增，PR 3）

`work-editor/apply.go`：

```go
// BodyApplication is a whole new body for one document, written only if the
// document still stands where the caller last saw it.
type BodyApplication struct {
	BaseVersionID string
	Body          string
}

var (
	ErrBaseMoved    = errors.New("document has a newer version than the base")
	ErrDraftUnsaved = errors.New("document has unsaved edits")
	ErrNoChange     = errors.New("body equals the base version")
)

// ApplyBody appends one version with (source, action) = (edited,
// suggestion_applied). Replays by idempotency key return the first version
// without re-checking the base.
func (s *Store) ApplyBody(ctx context.Context, workspaceID, actor, workID, artifactID string,
	application BodyApplication, requests ...idempotency.Request) (ArtifactVersion, error)
```

- 实现为 `appendVersion` 的一个新意图（加 `baseVersionID`、`body` 两个字段到 `versionIntent`），不另开一条写版本的路径；`TestTheVersionTableHasNoUpdateOrDeletePath` 与 `TestSourceAndActionAreNeverReadFromInput` 继续成立。
- 顺序：栅栏 → 幂等 `Claim`（重放即返回）→ 审计 → 锁文档 → 最新版本 == `BaseVersionID`（文档没有版本时也算不等）→ `draft_status == saved` → `Body != 基础版本正文` → 取号插入 → 编辑副本 = `Body`、`saved` → 幂等 `Complete` → 提交。
- 幂等请求由适配器用 `idempotency.NewRequest("apply-search-suggestion", artifactID, key, {base_version_id, sha256(body)})` 构造；同键不同输入 → `idempotency.ErrConflict` → 适配器映射为 `ErrStorage`（不应发生：同一建议的基础版本与正文不变）。
- 这是 `work-editor` 的**公开契约新增**：既有四个入口、响应形状、024/025/031 的消费者都不变。PR 3 正文按文档 12 §6 写明。

---

## 8. 给 EP-06 的约定（预检报告过期）

本卡不建预检表（Current State §1）。主控已裁定 2026-09-25：接受「采用必产生新 `version_id`」的结构性保证；报告的实际过期待 EP-06 落地后按下面三条补测（后续项）。EP-06 落地时 MUST：

1. 预检报告以 `(artifact_id, version_id)` 为键；
2. 「报告相对当前稿过期」= 报告的 `version_id` ≠ 该文档最新版本的 `version_id`，读时派生；
3. 终审（`review-delivery` 的人工批准）只能引用与被审版本 `version_id` 相同的报告。

本卡产生的版本（`action = suggestion_applied`）与其他版本一视同仁，不需要 EP-06 特殊处理。

---

## 9. AI 与联网的挂接（只写合同，不写代码）

- AI 建议以后写进同一张 `content_search_suggestion_revision`，`author_kind = ai`（届时一个迁移改 `CHECK`），**同样**只能经人采用才产生新版本。
- 联网研究的结果以后作为主题的新 `origin` 取值（届时一个迁移），并带运行记录的引用；在那之前，请求里的 `scope` / `budget` 一律 400。
- AI 分析搜索表现以后只读 §4.3、§4.4 的公开读接口。

---

## 10. 不做的自证（守卫用例，每条配一次可编译变异确认会红）

| 守卫 | 位置 | 变异 |
|---|---|---|
| 五张表只插不改，每张表有 `INSERT INTO` | `topic-planning/search_guards_test.go`、`feedback-learning/search_guards_test.go` | 加一句 `UPDATE content_search_theme_revision` |
| 本卡源文件无出站请求、无执行器 import | 同上 + `handler/content_search_guards_test.go` | 在 `search_theme.go` 里 import `net/http` 并调 `http.Get` |
| `topic-planning` 不 import `work-editor` / `feedback-learning`；`feedback-learning` 不 import `topic-planning` | 同上 | 在 `search_suggestion.go` 里 import `work-editor` |
| 放弃路径不调 `SearchWorks.Apply` | `topic-planning/search_guards_test.go`（按函数体扫） | 在 `abandon` 里调一次 `Apply` |
| 本卡源文件不写 `content_review_*` / `content_delivery_*` / `content_publication_*` | `handler/content_search_guards_test.go` | 在适配器里加一句 `INSERT INTO content_review_request` |
| 响应类型无 `score` / `rank` / `density` / `keyword_count` / `seo` / `avg` / `best` / `current_rank` / `search_volume` 存储字段 | 两模块的反射用例 | 给主题类型加 `SearchVolume *int64` |
| Go 字符串字面量与本页 i18n 无排名承诺词 | 两模块 + core node 测试 | 加一个 `"guarantee top rank"` 字面量 |
| `feedback-learning` SQL 不聚合 | 027 既有守卫 | 在 `search_metric.go` 里写 `SUM(value)` |
| 受控集「恰好 N 项」 | 各自的矩阵用例 | 给 `SearchMetrics` 加 `search_rank` |
