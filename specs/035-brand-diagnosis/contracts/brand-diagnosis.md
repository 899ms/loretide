# 合同：品牌/账号经营诊断（035）

**状态**：已裁决（主控 2026-09-25，PR #267 评论）：Q1～Q8 采纳推荐值；Q3 补充的建卡幂等键见 §7.4；历史导入的具名局限见 §5.9。每个列名、受控集都要能指回 `spec.md` 开头的 R-057 原文或主控 D1–D9；指不回去的在 `spec.md`「本规格补的设计」里单列。

模块：`feedback-learning`（Q1=A，主控已裁定 2026-09-25）。Go 代码在 `server/internal/content/feedback-learning/` 下以 `opdiag_` 为文件名前缀；handler 文件以 `content_opdiag_` 为前缀。路由前缀 `/api/content-operating-diagnosis`。

---

## 1. 八张表

**迁移编号不预留**（D6）：每个实施 PR 在合入前把自己的迁移改号为紧接当时 `app-main` 最大号之后的连续号。规格撰写时最大号 575（#266 将占 576–578），仅供参考。每张表：

- 建表迁移不含 `PRIMARY KEY` / `UNIQUE` / `REFERENCES` / `CASCADE`（R1、R2、R5）；
- 受控集用 `CHECK (... IN (...))` 兜底，Go 为准；
- 只插不改：没有 `UPDATE`，没有 `ON CONFLICT DO UPDATE`，没有删除链之外的 `DELETE`；
- 进 `workspace_delete_manifest_test.go` 与 `workspace_delete.sql` 的同一 CTE 链。

**修订制的共同列**（§1.3、1.4、1.7、1.8 省略不写）：`workspace_id text NOT NULL`、`revision integer NOT NULL`（从 1 起）、`voided boolean NOT NULL DEFAULT false`、`recorded_by text NOT NULL`（服务端从会话取）、`created_at timestamptz NOT NULL DEFAULT now()`。当前状态 = 同一 id 的最大 `revision`。

### 1.1 `content_opdiag_report_version` —— 报告版本（PR 1，只插）

| 列 | 类型 | 指回 | 说明 |
|---|---|---|---|
| `workspace_id` | text NOT NULL | D4 | |
| `report_id` | text NOT NULL | — | 稳定键 |
| `version_no` | integer NOT NULL | D14-V05 | 从 1 起 |
| `scope_kind` | text CHECK `account`/`brand` | R-057「账号报告及授权范围内的品牌汇总」 | |
| `account_ids` | text[] NOT NULL | 同上 | 已排序；`account` 恰好一个 |
| `title` | text NOT NULL DEFAULT '' | — | ≤200 rune |
| `params` | jsonb NOT NULL | R-057「分析范围、时间窗口」 | §4 |
| `inputs` | jsonb NOT NULL | R-057「原始依据」；D14-V05「输入版本」 | §6：每项输入的必要字段副本与 `fingerprint` |
| `calc_version` | text NOT NULL | — | `opdiag-calc/1` |
| `result` | jsonb NOT NULL | R-057「数据完整性……局限」 | §5 |
| `created_by` | text NOT NULL | | 服务端写 |
| `created_at` | timestamptz NOT NULL DEFAULT now() | | |

**没有 AI 判断列或表**（FR-071）。

### 1.2 `content_opdiag_work_mark` —— 作品标注（PR 1，只插）

一行 = 人对一条作品的一次标注。当前状态 = 同一 `(work_id, kind, item)` 按 `(created_at, mark_id)` 最新的一行。

| 列 | 类型 | 说明 |
|---|---|---|
| `workspace_id`, `mark_id` | text NOT NULL | |
| `work_id` | text NOT NULL | 经适配器核实存在 |
| `kind` | text CHECK `pillar`/`consistency` | Q2=A |
| `item` | text NOT NULL | `pillar`：支柱名（自由文本，≤100 rune，NFC + 去首尾空白）；`consistency`：§3 的账号配置项键之一 |
| `verdict` | text CHECK `tagged`/`untagged`/`consistent`/`inconsistent`/`unsure` | `pillar` 只许前两个，`consistency` 只许后三个（Go 校验；`CHECK` 兜底这一组合） |
| `account_id` | text NOT NULL DEFAULT '' | `consistency` 必填（对照哪个账号的配置）；`pillar` 可空 |
| `profile_revision_id` | text NOT NULL DEFAULT '' | `consistency` 必填：标注时对照的账号配置版本（服务端经适配器读当前版本写入，请求体不能指定） |
| `note` | text NOT NULL DEFAULT '' | ≤2000 rune |
| `recorded_by`, `created_at` | | |

### 1.3 `content_opdiag_judgement_revision` —— 判断、替代解释、局限（PR 3）

| 列 | 说明 |
|---|---|
| `judgement_id` | 稳定键 |
| `report_id`, `version_no` | 挂在哪个报告版本 |
| `kind` CHECK `judgement`/`alternative_explanation`/`limitation` | FR-050 |
| `basis` CHECK `evidence`/`qualitative` | FR-051 |
| `evidence_refs` text[] NOT NULL DEFAULT '{}' | §5.8 的引用键；`qualitative` 时必须为空 |
| `about_judgement_id` text NOT NULL DEFAULT '' | 替代解释指向的判断 |
| `body` text NOT NULL | ≤5000 rune，非空 |
| `author_kind` CHECK `human` | FR-053；本版恰好一项 |

### 1.4 `content_opdiag_suggestion_revision` —— 建议（PR 3）

| 列 | 说明 |
|---|---|
| `suggestion_id` | 稳定键 |
| `report_id`, `version_no` | |
| `body` text NOT NULL | ≤5000 rune |
| `target_kind` CHECK `topic_card`/`todo`/`profile_proposal` | FR-052；**恰好三项**，没有「经营记忆」 |
| `target` jsonb NOT NULL | §7.3 |
| `judgement_ids` text[] NOT NULL DEFAULT '{}' | 必须是同一报告版本上的判断 |
| `evidence_refs` text[] NOT NULL DEFAULT '{}' | |
| `author_kind` CHECK `human` | |

### 1.5 `content_opdiag_decision` —— 决定（PR 3，只插）

| 列 | 说明 |
|---|---|
| `workspace_id`, `decision_id` | |
| `suggestion_id`, `suggestion_revision` | 决定的是哪一修订 |
| `decision` CHECK `adopt`/`reject` | |
| `mode` text NOT NULL DEFAULT '' CHECK `''`/`create`/`link` | 只对 `topic_card` 有意义 |
| `link_target_id` text NOT NULL DEFAULT '' | `link` 时的选题卡 id |
| `note` text NOT NULL DEFAULT '' | ≤2000 rune |
| `decided_by`, `created_at` | |

同一 `(suggestion_id, suggestion_revision)` 只能有一行：唯一索引兜底（§2），Go 先查，冲突映射为 409 `suggestion_id`。

### 1.6 `content_opdiag_effect` —— 效果（PR 3，只插）

| 列 | 说明 |
|---|---|
| `workspace_id`, `effect_id` | |
| `decision_id` | |
| `outcome` CHECK `done`/`failed` | |
| `target_kind` | 同建议 |
| `target_id` text NOT NULL DEFAULT '' | `topic_card_id` / `todo_id` / `proposal_id`；`failed` 时为 '' |
| `failure_code` text NOT NULL DEFAULT '' CHECK `''`/`target_refused`/`target_not_found`/`storage` | |
| `created_at` | |

一个决定可有多条效果（失败后重试）；**最多一条 `done`**：Go 在写入前查，另有一条守卫用例；不加部分唯一索引（R5 之外的唯一性都来自单独的并发索引迁移，部分索引会让删除链之外多一个特殊形状，不值得）。

### 1.7 `content_opdiag_profile_proposal_revision` —— 账号配置修改提议（PR 3）

| 列 | 说明 |
|---|---|
| `proposal_id` | 稳定键 |
| `account_id` | |
| `decision_id` | 由哪个采纳决定产生 |
| `base_revision_id` | 产生提议时账号配置的当前 `revision_id` |
| `patches` jsonb NOT NULL | `[{field, value}]`，`field` 取 §3 的八个文本项之一，不重复，1～8 项；`value` ≤20000 rune（与 `ip-profile.MaxProfileFieldRunes` 相同） |
| `state` CHECK `proposed`/`confirmed`/`dismissed` | |
| `applied_revision_id` text NOT NULL DEFAULT '' | `confirmed` 时 `ip-profile` 新版本的 `revision_id` |

### 1.8 `content_opdiag_todo_revision` —— 待办（PR 3）

| 列 | 说明 |
|---|---|
| `todo_id` | 稳定键 |
| `title` text NOT NULL | ≤200 rune，非空 |
| `note` text NOT NULL DEFAULT '' | ≤2000 rune |
| `account_id` text NOT NULL DEFAULT '' | |
| `origin_kind` CHECK `suggestion`/`data_gap` | |
| `origin_decision_id` text NOT NULL DEFAULT '' | `suggestion` 时必填 |
| `origin_report_id`, `origin_version_no`, `origin_gap_key` | `data_gap` 时必填；`gap_key` 必须存在于该版本的缺口清单 |
| `state` CHECK `open`/`done`/`dropped` | |

同一 `(origin_report_id, origin_version_no, origin_gap_key)` 已有未作废的待办 → 409 `origin_gap_key`（重复点「加入待办」不产生第二条）。

---

## 2. 索引（每个一个迁移文件、单条语句、`CONCURRENTLY`，登记进 `concurrentIndexCleanups`）

| 表 | 索引名 | 列 | 唯一 | 服务什么 |
|---|---|---|---|---|
| report_version | `content_opdiag_report_version_key_idx` | `(workspace_id, report_id, version_no)` | 是 | 版本号不重复；删除链 |
| | `content_opdiag_report_version_time_idx` | `(workspace_id, created_at DESC)` | | 列表 |
| work_mark | `content_opdiag_work_mark_key_idx` | `(workspace_id, mark_id)` | 是 | |
| | `content_opdiag_work_mark_work_idx` | `(workspace_id, work_id, kind, item, created_at)` | | 取最新标注 |
| judgement_revision | `content_opdiag_judgement_revision_key_idx` | `(workspace_id, judgement_id, revision)` | 是 | |
| | `content_opdiag_judgement_revision_report_idx` | `(workspace_id, report_id, version_no)` | | 读一个版本的判断 |
| suggestion_revision | `content_opdiag_suggestion_revision_key_idx` | `(workspace_id, suggestion_id, revision)` | 是 | |
| | `content_opdiag_suggestion_revision_report_idx` | `(workspace_id, report_id, version_no)` | | |
| decision | `content_opdiag_decision_key_idx` | `(workspace_id, decision_id)` | 是 | |
| | `content_opdiag_decision_suggestion_idx` | `(workspace_id, suggestion_id, suggestion_revision)` | 是 | 一个修订只有一个决定 |
| effect | `content_opdiag_effect_key_idx` | `(workspace_id, effect_id)` | 是 | |
| | `content_opdiag_effect_decision_idx` | `(workspace_id, decision_id, created_at)` | | |
| profile_proposal_revision | `content_opdiag_profile_proposal_revision_key_idx` | `(workspace_id, proposal_id, revision)` | 是 | |
| | `content_opdiag_profile_proposal_revision_account_idx` | `(workspace_id, account_id)` | | |
| todo_revision | `content_opdiag_todo_revision_key_idx` | `(workspace_id, todo_id, revision)` | 是 | |
| | `content_opdiag_todo_revision_time_idx` | `(workspace_id, created_at DESC)` | | |

**八张表、十六个索引、二十四个迁移**：PR 1 六个（report_version、work_mark 各 1 + 2），PR 2 零个，PR 3 十八个。另有 §7.4 在 `topic-planning` 的两个迁移（加列、唯一索引），也在 PR 3，PR 3 合计二十个。

---

## 3. 受控集（Go 为准，`CHECK` 兜底，各一条「恰好 N 项」用例，登记进 027 `guards_test.go`）

| Go 名 | 取值 | 出处 |
|---|---|---|
| `DiagnosisDimensions` | `consistency` `coverage` `cadence` `performance` `audience_feedback` `execution_flow` | R-057 第 2 条，恰好六项 |
| `DiagnosisScopes` | `account` `brand` | R-057 第 1 条 |
| `MarkKinds` | `pillar` `consistency` | Q2=A；R-057「内容支柱」「定位与表达一致性」 |
| `MarkVerdicts` | `tagged` `untagged` `consistent` `inconsistent` `unsure` | 同上 |
| `DimensionReasons`（有序） | `no_data` `missing_config` `missing_comparison_window` `missing_observation_window` `no_delivery_channel` | FR-012 |
| `GapKinds` | `profile_field_pending` `work_unchecked` `work_untagged` `cadence_unset` `observation_unset` `published_at_missing` `metric_missing` `account_unresolved` `work_missing` `publication_status_unknown` `excerpt_untagged` | R-057「补录待办」 |
| `DiagnosisRuleIDs` | §5.9 | FR-015 |
| `DataOrigins` | `manual_only` | FR-073，本版恰好一项 |
| `JudgementKinds` | `judgement` `alternative_explanation` `limitation` | R-057「AI 判断、替代解释、局限」（本版由人写） |
| `JudgementBases` | `evidence` `qualitative` | R-057「允许定性分析」 |
| `AuthorKinds` | `human` | FR-053，本版恰好一项 |
| `SuggestionTargets` | `topic_card` `todo` `profile_proposal` | R-057「账号配置修改、选题或待办」 |
| `DecisionKinds` | `adopt` `reject` | D14-V05 |
| `AdoptModes` | `create` `link` | Q3=A |
| `EffectOutcomes` | `done` `failed` | Q3=A |
| `EffectFailures` | `target_refused` `target_not_found` `storage` | |
| `ProposalStates` | `proposed` `confirmed` `dismissed` | D3「仍需显式确认」 |
| `TodoStates` | `open` `done` `dropped` | |
| `TodoOrigins` | `suggestion` `data_gap` | FR-032、FR-064 |

**照抄而不是新定的列表**（`[]string`，不算受控集，各有一条读对方源文件对表的用例，不 import）：

- `profileFieldKeys`：`ip-profile/profile.go` 的十一个 JSON 名 `audience` `common_questions` `experience` `positioning` `content_pillars` `expression_style` `forbidden_expressions` `content_goals` `primary_channels` `weekly_hours` `style_samples`。一致性检查项取它的子集。
- `profileTextKeys`：其中前八个（`TextField` 类型的项）。提议只能改这八个（FR-067）。
- 平台与指标：沿用本模块已有的 `Platforms`（四个）与 `Metrics`（十一个）。

**自由文本**（不发明取值）：支柱名、标签、判断与建议正文、待办标题与说明、各 `note`。

**不存在的东西**（守卫用例扫参数与结果类型的字段名）：`role`、`industry`、`score`、`grade`、`rating`、`rank`、`level`。

---

## 4. 参数 `params`

```json
{
  "scope": {"kind": "brand", "account_ids": ["a1", "a2"]},
  "window": {"start": "2026-09-01", "end": "2026-09-30", "timezone": "Asia/Shanghai"},
  "comparison_window": {"start": "2026-08-01", "end": "2026-08-31"},
  "dimensions": [
    {"key": "consistency", "items": ["positioning", "expression_style", "forbidden_expressions"]},
    {"key": "coverage", "pillars": ["面料知识", "穿搭", "门店活动"]},
    {"key": "cadence"},
    {"key": "performance", "metrics": ["impression", "favorite", "direct_message"], "platforms": []},
    {"key": "audience_feedback", "sources": []},
    {"key": "execution_flow"}
  ],
  "roi_report_ref": {"report_id": "r1", "version_no": 3},
  "generated_at": "2026-10-02T09:00:00+08:00"
}
```

- 严格解码：未知字段 → 400 点名该字段（FR-004）。
- `timezone` 与 `generated_at` 由服务端写入，请求体里带了也被覆盖（同 034 `generated_at`）。
- `dimensions` 里同一 `key` 出现两次 → 400 `dimensions`。`items` 空 → 该维度 `missing_config`；`pillars` 空 → 同上；`metrics` 空 → 同上；`platforms` / `sources` 空 = 全部。
- `comparison_window` 只在选了 `performance` 时必填；没有 → 该维度 `not_computable/missing_comparison_window`（不是 400：用户可以先看其他维度）。
- `pillars` ≤ 50 项，每项 ≤100 rune，NFC + 去首尾空白后去重，保持用户给的顺序。
- `roi_report_ref` 可空（Q6）。

---

## 5. 计算器

纯函数 `CalculateDiagnosis(params Params, inputs Inputs) (Result, error)`，文件 `opdiag_calc.go`，**不读库、不读时钟、不用浮点**；`error` 只在参数不可用时返回，并点名字段。

### 5.1 共同规则

- **窗口归属**：发布记录按 `published_at` 落入窗口；摘录按 `occurred_at`；审核的「窗口内被拒」按 `decided_at`。时间一律先换到参数时区再与 `[start 00:00, end+1 00:00)` 比较。
- **账号归属**（FR-026）：发布记录 → `work_id` → 作品的 `topic_card_id` → 选题卡的 `account_id`；任一环为空或不存在 → 「账号未知」。指标用自身 `account_id`；审核用自身 `account_id`；交付任务经它的 `review_request_id` 找审核的 `account_id`，找不到 → 「账号未知」。
- **只算已发布**：节奏与表现只看状态为 `reported_published` / `verified_published` 的发布记录（本模块已有 `PublishedStatuses`）。
- **合计与均值**：`math/big.Int` / `big.Rat`；JSON 里是字符串；`display` 两位小数、四舍五入远离零，负号 U+2212。
- **排序**：所有数组按固定键排序（账号节按 `display_name`、`account_id`；分组按平台、指标在受控集里的顺序；周按 ISO 周；清单按 id），**从不按数值排序**。

### 5.2 结果形状

```json
{
  "calc_version": "opdiag-calc/1",
  "data_origin": "manual_only",
  "scope": {
    "kind": "brand",
    "accounts": [{"account_id": "a1", "platform": "xiaohongshu", "display_name": "品牌主号", "profile_revision_id": "pr9"}],
    "window": {...}, "comparison_window": {...},
    "input_counts": {"publications": 12, "works": 11, "metrics": 40, "excerpts": 9, "work_marks": 20, "reviews": 6, "delivery_tasks": 5}
  },
  "sections": [
    {"section": "account", "account_id": "a1", "dimensions": {"cadence": {...}, "performance": {...}}},
    {"section": "unknown_account", "dimensions": {...}},
    {"section": "brand", "dimensions": {"cadence": {...}, "execution_flow": {...}}}
  ],
  "gaps": [{"gap_key": "performance/metric_missing/publication/p7", "kind": "metric_missing", "dimension": "performance",
            "ref": {"kind": "publication", "id": "p7"}, "account_id": "a1", "fix_route": "feedback"}],
  "roi_reference": null,
  "rules": ["common.unknown_is_not_zero", "common.no_cross_platform_ranking"]
}
```

每个维度：

```json
{"status": "ok", "facts": {...}, "completeness": {"expected": 20, "present": 12, "gap_keys": ["..."]},
 "limits": ["performance.difference_is_not_cause"], "records": [{"kind": "publication", "id": "p1"}]}
```

或 `{"status": "not_computable", "reason": "missing_config", "completeness": {...}, "limits": [...], "records": []}`。

**节的规则**：`account` 范围只有一节 `account`。`brand` 范围有每个账号一节、一节 `unknown_account`（只在确有归不到账号的记录时出现）、一节 `brand`（只含 `cadence` 与 `execution_flow` 两个维度的品牌层面计数，其余维度不在品牌节里重复出现，避免被读成「品牌合计」）。

### 5.3 一致性（`consistency`）

对节内账号：

- `profile_items`：参数 `items` 里每一项的账号配置 `status`（`confirmed` / `pending`）；`pending` → 缺口 `profile_field_pending`。
- `works`：节内、窗口内有已发布记录的作品；每一项检查项计 `consistent` / `inconsistent` / `unsure` / `unchecked`（该作品该项没有标注，或最新标注的 `account_id` 不是本账号）。`unchecked` → 缺口 `work_unchecked`。
- `marks_on_older_revision`：计入的标注里 `profile_revision_id` ≠ 当前配置版本的条数；> 0 时 `limits` 加 `consistency.marks_on_older_profile`。
- `unknown_account` 节不算一致性（没有配置可对照），维度 `not_computable/no_data`。

### 5.4 表现变化（`performance`）

- 分组键 `(platform, metric)`，`platform` 取指标自身的 `platform`；参数 `platforms` 非空时只取其中的。
- 一条发布记录属于窗口 W，当且仅当它的 `published_at` 落在 W 内。每个窗口、每组：
  - `publications`：W 内、至少有一条该组指标采样（任意值，含 nil）的发布记录数；
  - 对每条这样的发布记录，取 `sampled_at ≤ generated_at` 的**最后一次**采样（相同时按 `created_at`、再按 `manual_metric_id`）；
  - `with_value`：最后一次采样值非 nil 的条数；`unknown`：为 nil 的条数；
  - `sum`：`with_value` 那些值之和；`mean`：`sum / with_value`（`with_value = 0` → `mean` 为 `not_computable/no_data`）。
- `change`：两窗口 `mean` 都可算 → `current.mean − baseline.mean`（有理数串 + `display`）；否则 `not_computable`，原因取不可算的那一侧（对比窗口缺 → `missing_comparison_window` 或 `no_data`）。
- `stat_windows`：该组计入的最后一次采样的 `stat_window` 去重集合（NFC + 去首尾空白后比较），按字典序；多于一种 → `stat_window_mixed = true`，`limits` 加 `performance.stat_window_mixed`。
- 窗口内已发布、但所选指标一条采样都没有的发布记录 → 缺口 `metric_missing`（每条发布记录一个，不按指标重复）。
- `limits` 恒含 `performance.difference_is_not_cause`、`performance.samples_taken_at_different_ages`。
- **没有**跨组合计、跨组比较、排序字段。`facts.groups` 按平台在 `Platforms` 里的顺序、再按指标在 `Metrics` 里的顺序排列。

### 5.5 受众反馈（`audience_feedback`）

- 窗口内（`occurred_at`）、节内（按摘录的发布记录归属账号）的摘录；参数 `sources` 非空时只取其中的。
- `by_source`：`(platform, source_type)` → 条数；平台 = 摘录所属发布记录的 `channel`。
- `by_tag`：标签规范化 = NFC + 去首尾空白，**不做大小写折叠、不合并近义**；一条摘录多个标签各计一次；`limits` 加 `audience_feedback.tags_not_additive`。
- `untagged`：没有标签的条数 → 每条一个缺口 `excerpt_untagged`。
- 一条摘录都没有 → `not_computable/no_data`（不是 0 条的「ok」）。

### 5.6 节奏（`cadence`）

- 渠道 = 发布记录的 `channel`（四个交付渠道）。账号节只看该账号平台对应的渠道；账号平台不在四个渠道里 → `not_computable/no_delivery_channel`。
- 周 = 参数时区的 ISO 周（周一 00:00 起）。窗口首尾不完整的周 `complete = false`，不对比。
- 每周：`published`（窗口内、该周、该渠道、已发布的条数）；`target`：经营规则 `cadence[channel]`，`{set: false}` 或 `{set: true, per_week: n}`；`compared`：`complete && target.set`；`met`：只在 `compared` 时出现，`published >= per_week`。
- `target.set = false` → 缺口 `cadence_unset`（每渠道一个）。
- `published_at` 为空的已发布记录 → 缺口 `published_at_missing`，不落入任何周。
- `limits` 恒含 `cadence.target_is_brand_channel_level`（目标是品牌在该渠道的周篇数，不是某个账号的）。

### 5.7 执行流程（`execution_flow`）

以 `generated_at` 为准；节内按 §5.1 的账号归属：

- `reviews_pending`、`reviews_changes_requested`：清单 `[{review_request_id, account_id, waiting_days}]`，`waiting_days` = 参数时区下 `requested_at` 到 `generated_at` 的整日数。
- `reviews_rejected_in_window`：`decided_at` 落入窗口、状态 `rejected` 的条数与清单。
- `deliveries_held`：状态 `held` 的交付任务清单。
- `deliveries_due`：`review-delivery` 读时派生 `due = true` 的交付任务清单（本卡不重算）。
- `publications_failed` / `publications_unknown` / `publications_removed`：按最新状态的清单；`unknown` 另进缺口 `publication_status_unknown`。
- `publications_missing_metrics_after_window`：已发布、本模块没有任何该发布记录的指标、且 `workspacecore.ObservationDueFor(rules, channel, published_at, generated_at) == passed` 的清单；该渠道没有观察时点 → 这一项 `not_computable/missing_observation_window`，并出缺口 `observation_unset`。
- **没有任何天数阈值**；`limits` 恒含 `execution_flow.no_threshold`。

### 5.8 引用键（判断与建议的 `evidence_refs`）

结果里每个可引用的事实都有一个确定的引用键，由计算器同时产出清单 `result.refs`（不单独存，读版本时即得）：

| 键的形状 | 例子 |
|---|---|
| `scope` | `scope` |
| `<section>/<dimension>` | `account:a1/cadence` |
| `<section>/consistency/<item>` | `account:a1/consistency/positioning` |
| `<section>/coverage/<pillar>` | `account:a1/coverage/面料知识` |
| `<section>/cadence/<channel>/<iso_week>` | `brand/cadence/xiaohongshu/2026-W37` |
| `<section>/performance/<platform>/<metric>` | `account:a1/performance/xiaohongshu/favorite` |
| `<section>/audience_feedback/tag/<tag>` | `account:a1/audience_feedback/tag/价格` |
| `<section>/execution_flow/<fact>` | `brand/execution_flow/reviews_pending` |
| `gap/<gap_key>` | `gap/performance/metric_missing/publication/p7` |
| `roi_reference` | 仅当有 ROI 引用 |

判断或建议提交时，每个 `evidence_refs` 元素必须在该版本的 `result.refs` 里，否则 400 点名 `evidence_refs`。

### 5.9 规则 id（`DiagnosisRuleIDs`，前端翻译）

`common.unknown_is_not_zero`、`common.no_cross_platform_ranking`、`common.no_score`、`common.window_in_brand_timezone`、`consistency.marks_on_older_profile`、`coverage.multi_pillar_not_additive`、`cadence.target_is_brand_channel_level`、`cadence.incomplete_week_not_compared`、`performance.difference_is_not_cause`、`performance.samples_taken_at_different_ages`、`performance.stat_window_mixed`、`audience_feedback.tags_not_additive`、`execution_flow.no_threshold`、`roi_reference.shown_as_is`、`scope.historical_import_account_unknown`。

`scope.historical_import_account_unknown` 是具名局限（主控 2026-09-25 接受「历史导入归账号未知」）：只要范围内有历史导入作品（`work.historical_import = true`）的发布记录，它就进 `result.rules`，并进 `unknown_account` 节每个维度的 `limits`；`scope` 另给 `historical_import_publications` 计数。账号范围的报告里这些记录不属于该账号，同样在 `rules` 里列出这条局限，前端据此写「有 N 条历史导入发布记录因无法关联账号而未计入」。

四语言文案里这些键的译文**不许**含「增长 / 提升 / 下降 / 导致 / 带来 / 因为」及 `growth` / `caused` / `because`（除了明确的否定句，例如 `performance.difference_is_not_cause` 的译文「差值只描述两个窗口的数字，不说明原因」——守卫按键白名单放行这一条）。

### 5.10 固定样例（逐字进 `opdiag_calc_test.go`）

| 名称 | 输入 | 期望 |
|---|---|---|
| `empty-scope` | 一个账号，窗口内零条记录，选全部六维 | 一致性、覆盖、节奏、表现、受众反馈五个维度 `not_computable/no_data`（窗口内没有已发布记录时，「这周 0 篇」与「这周发了没登记」分不开，所以节奏也是未知）；缺口里仍有 `profile_field_pending` 等配置类缺口；`execution_flow` 为 `ok` 且各清单为空（审核与交付是工作台自己的状态，没有就是没有） |
| `nil-vs-zero` | 两条发布记录的 `favorite`：一条 nil、一条 0 | `with_value = 1`、`unknown = 1`、`sum = "0"`、`mean.display = "0.00"` |
| `latest-sample` | 同一发布记录 `favorite` 采样 10（9-02）、25（9-09）、生成于 9-05 | 取 10 |
| `two-platforms` | 小红书 `impression` 100、抖音 `play` 300 | 两组，分别给数；结果类型没有跨组合计字段 |
| `stat-window-mixed` | 同组两条，`stat_window` 为「发布后 7 天」「发布后 14 天」 | `stat_window_mixed = true`，`stat_windows` 两项 |
| `change` | 对比窗口均值 20、本窗口均值 15 | `change.display = "−5.00"`，`limits` 含 `performance.difference_is_not_cause` |
| `cadence-unset-vs-zero` | 渠道 A 无目标、渠道 B 目标 0，两者当周都 0 篇 | A：`target.set = false`、无 `met`、缺口 `cadence_unset`；B：`compared = true`、`met = true` |
| `incomplete-week` | 窗口 9-03（周三）至 9-30 | 第一周 `complete = false`、无 `met` |
| `pillar-untagged` | 三条作品，一条标「穿搭」，一条先标「穿搭」后 `untagged` 再标「面料知识」，一条无标注 | 穿搭 1、面料知识 1、未标注 1 |
| `pillars-empty` | 选覆盖维度，`pillars = []` | `not_computable/missing_config` |
| `observation-unset` | 经营规则无观察时点 | `publications_missing_metrics_after_window` 为 `not_computable/missing_observation_window` |
| `unknown-account` | 历史导入作品（无选题卡）的一条发布记录 | 品牌范围出现 `unknown_account` 节；缺口 `account_unresolved`；`rules` 与该节 `limits` 含 `scope.historical_import_account_unknown`；`scope.historical_import_publications = 1` |
| `order-independent` | 任一样例打乱输入数组顺序 | `result` JSON 逐字节相同 |

---

## 6. 输入快照 `inputs`

每项输入是 `{kind, id, fingerprint, fields}`，`fingerprint` = `fields` 的规范化 JSON 的 SHA-256。**只存计算要的字段**（FR-042）：

| `kind` | `fields` | 从哪来 |
|---|---|---|
| `account` | `platform`、`display_name` | `ipprofile.Service.Get` |
| `profile` | `revision_id`、十一项各自的 `status`（**不含 `value`**） | `Service.CurrentPersonaRevision` |
| `operating_rules` | `cadence`、`observation`（内容副本；它没有版本号） | `workspacecore.RulesFrom` |
| `publication` | `work_id`、`channel`、`status`、`published_at` | `review-delivery.ListPublications` |
| `work` | `topic_card_id`、`historical_import`、`title` | `work-editor.Store.GetWork` |
| `topic_card` | `account_id` | `topic-planning.Store.Get` |
| `metric` | `publication_record_id`、`platform`、`account_id`、`metric`、`value`、`stat_window`、`sampled_at`、`created_at` | 本模块 |
| `excerpt` | `publication_record_id`、`source_type`、`tags`、`occurred_at`（**不含 `redacted_excerpt`、`interpretation`**） | 本模块 |
| `review` | `account_id`、`status`、`requested_at`、`decided_at` | `review-delivery.ListReviews` |
| `delivery_task` | `review_request_id`、`status`、`due` | `review-delivery.ListTasks` |
| `work_mark` | `work_id`、`kind`、`item`、`verdict`、`account_id`、`profile_revision_id`、`created_at` | 本模块 |
| `roi_summary` | `ReportSummary` 整份（它自己的允许清单已排除客户字段） | 本模块 `Store.ReportSummary`（#266） |

**收集顺序**（FR-081，一条用例钉住）：`Authorize` → 逐个 `AccountExists`（任一失败即拒绝，此前没有任何账号数据被读）→ 账号与配置 → 经营规则 → 发布记录（全工作区读出后按窗口与归属在 Go 里过滤）→ 作品 → 选题卡 → 审核、交付 → 本模块的指标、摘录、标注 → ROI 摘要。

**收集范围**：两个窗口里（选了 `performance` 才有对比窗口）的已发布记录与它们的作品、选题卡；这些发布记录的全部指标与摘录；这些作品的全部标注；全部未结的审核与交付任务（执行流程要「当前」）。不在范围内的不进快照。

`inputs_changed`（FR-043）：用同一参数（`generated_at` 取当前时间）重新收集，按 `(kind, id)` 对齐比较 `fingerprint`，返回 `{changed: bool, added: [...], modified: [...], removed: [...]}`。不落存储。

---

## 7. 端点

全部挂在 `/api/content-operating-diagnosis` 下，照 `/api/content-roi` 的挂法（`RequireWorkspaceMember` 组之外，`h.DiagnosticTrace`，handler 内 `workspacecore.Authorize`）。

| PR | 方法 | 路径 | 用途 |
|---|---|---|---|
| 1 | GET | `/reports` | 每份报告的最新版本头（不含 `inputs`） |
| 1 | POST | `/reports` | 新报告 + 版本 1 |
| 1 | GET | `/reports/{reportId}/versions` | 版本列表 |
| 1 | POST | `/reports/{reportId}/versions` | 生成新版本（参数缺省沿用上一版） |
| 1 | GET | `/reports/{reportId}/versions/{versionNo}` | 一版，含派生的 `inputs_changed` 与 `ai_judgement_state` |
| 1 | GET | `/work-marks` | 按 `work_id` 或 `account_id` 列当前标注 |
| 1 | POST | `/work-marks` | 追加一条标注 |
| 2 | POST | `/preview` | 用给定参数算一次，**不保存**；返回 §5.2 |
| 3 | GET | `/reports/{reportId}/versions/{versionNo}/annotations` | 这一版上的判断、建议、决定、效果 |
| 3 | POST | `/reports/{reportId}/versions/{versionNo}/judgements` | 新判断 |
| 3 | POST | `/judgements/{judgementId}/revisions` | 判断新修订 |
| 3 | POST | `/reports/{reportId}/versions/{versionNo}/suggestions` | 新建议 |
| 3 | POST | `/suggestions/{suggestionId}/revisions` | 建议新修订 |
| 3 | POST | `/suggestions/{suggestionId}/decisions` | 采纳或拒绝（带 `suggestion_revision`） |
| 3 | POST | `/decisions/{decisionId}/retry` | 效果失败后重试（可改 `mode`） |
| 3 | GET | `/profile-proposals` | 按 `account_id` / `state` 列提议，含「当前值 → 提议值」对照 |
| 3 | POST | `/profile-proposals/{proposalId}/confirm` | 显式确认（带 `base_revision_id`） |
| 3 | POST | `/profile-proposals/{proposalId}/dismiss` | 放弃 |
| 3 | GET | `/todos` | 当前待办 |
| 3 | POST | `/todos` | 从缺口加入待办（`origin_kind = data_gap`） |
| 3 | POST | `/todos/{todoId}/revisions` | 改状态、标题、说明 |

### 7.1 决策顺序（第一个不通过者即拒绝）

| 序 | 检查 | 失败时 |
|---|---|---|
| 1 | 工作区成员资格（`workspace-core.Authorize`） | `RefusalStatus` / `RefusalBody`，与「不存在」同形 |
| 2 | 路径里的 id 在本工作区存在；参数里每个账号、作品、选题卡、ROI 报告版本存在 | 同上，逐字节相同 |
| 3 | 严格解码：未知字段 | 400 点名字段 |
| 4 | 受控集 | 400 点名字段 |
| 5 | 必填项与组合（窗口、`verdict` 与 `kind`、`basis` 与 `evidence_refs`、`target` 与 `target_kind`） | 400 点名字段 |
| 6 | 长度上限（按 rune） | 400 点名字段 |
| 7 | 引用在该版本存在（`evidence_refs`、`judgement_ids`、`gap_key`） | 400 点名字段 |
| 8 | `base_revision` / `base_revision_id` 与当前不符 | 409 点名字段 |
| 9 | 已有决定 / 已有 `done` 效果 / 缺口已有待办 | 409 点名 `suggestion_id` / `decision_id` / `origin_gap_key` |

### 7.2 采纳流程

| 目标 | 流程 | 事务 |
|---|---|---|
| `todo` | 栅栏 → 核验 → 决定 → 待办修订 1（`open`）→ 效果 `done` → 审计 | 一个（本模块） |
| `profile_proposal` | 栅栏 → 核验（账号存在；读当前配置 `revision_id`，经适配器）→ 决定 → 提议修订 1（`proposed`）→ 效果 `done` → 审计 | 一个（本模块）；**不写 `ip-profile`** |
| `topic_card` + `link` | 适配器只读核实选题卡存在且账号相符 → 栅栏 → 决定 → 效果 `done`（`target_id` = 该卡）→ 审计 | 一个（本模块） |
| `topic_card` + `create` | ① 栅栏 → 决定 → 审计 → 提交；② 适配器调 `topicplanning.Store.CreateOnce`（幂等键 `opdiag-suggestion:<suggestion_id>`，`draft`，`account_id` 取 `target.account_id`，`ip_fit` 预填建议正文）；③ 栅栏 → 效果 `done` 或 `failed` → 审计 → 提交 | 三步，Q3=A；② 幂等，见 §7.4 |

重试（`/decisions/{decisionId}/retry`）：对最新效果为 `failed`、或**还没有任何效果记录**的 `topic_card` 采纳决定可用；走上表 `create` 的 ②③（同一个幂等键）或 `link` 的核实与③。已有 `done` 效果 → 409 `decision_id`。

**提议确认**：适配器读账号当前配置 → `revision_id ≠ base_revision_id` → 409；相等 → 组装新 `ExpressionProfile`（当前各项原样，`patches` 里的项替换 `value` 并置 `status = confirmed`）→ `ipprofile.Service.SetProfile` → 栅栏 → 提议修订 `confirmed` 带 `applied_revision_id` → 审计 → 提交。读与 `SetProfile` 之间的竞态见 plan.md「已知边界」（Q8）。

**拒绝**：栅栏 → 决定（`reject`）→ 审计 → 提交。**不经过任何适配器的写方法**；一条用例用记录调用的假适配器断言零次写调用。

### 7.3 建议的 `target`

| `target_kind` | `target` | 校验 |
|---|---|---|
| `topic_card` | `{"account_id": "a1"}`（可为 ''） | 账号存在 |
| `todo` | `{"title": "...", "account_id": ""}` | 标题非空 ≤200 rune |
| `profile_proposal` | `{"account_id": "a1", "patches": [{"field": "content_pillars", "value": "..."}]}` | 账号存在；`field` ∈ 八个文本项，不重复；1～8 项 |

### 7.4 建卡幂等键（Q3 补充，主控已裁定 2026-09-25）

**键**：`opdiag-suggestion:<suggestion_id>`，每条建议一个，与修订号无关——同一建议的新修订再采纳，仍指向同一张卡。

**由谁保证**：只有建卡的一方能保证「同键至多一张」，所以放在 `topic-planning`。**增量**改动：既有 `Create` 行为不变，`modules` 依赖表不变（键存在 `topic-planning` 自己的表上，不依赖 `idempotency` 模块）。

| 改动 | 内容 |
|---|---|
| 迁移 1 | `ALTER TABLE content_topic_card ADD COLUMN IF NOT EXISTS origin_key text`（可空，既有行为 NULL；照 536 / 537 给该表加列的先例） |
| 迁移 2 | `CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS content_topic_card_origin_key_idx ON content_topic_card (workspace_id, origin_key)`（NULL 互不冲突，既有卡不受影响；登记进 `concurrentIndexCleanups`） |
| 公开函数 | `func (s *Store) CreateOnce(ctx context.Context, actor string, card TopicCard, originKey string) (TopicCard, bool, error)`：栅栏事务内先按 `(workspace_id, origin_key)` 读，有则返回那张卡与 `created=false`；没有则按 `Create` 的同一套校验与 `insertCardTx` 插入并写 `origin_key`，返回 `created=true`。并发的第二个插入撞唯一索引时，在同一调用里回读并返回已建的卡，不向调用方报错。`originKey` 为空 → `ErrInvalid` |
| 对外字段 | `TopicCard` 不新增对外字段（`origin_key` 只供 `CreateOnce` 查重），022 的响应 schema 不变 |

**收敛**：「卡已建、效果未记」时，建议显示「已采纳，结果未记录」；重试再调 `CreateOnce` 得到同一张卡（`created=false`），写效果 `done` 指向它。任意次数、任意并发的重试，一条建议最多对应一张选题卡。

**用例**（PR 3，tasks T070、T070a）：钩子在 ② 成功后、③ 之前注入失败 → 重试 → 选题卡总数恰好 +1，效果 `done` 指向那张卡；两个重试并发 → 同样恰好 +1。`topic-planning` 侧另有 `CreateOnce` 自己的用例（同键两次、并发两次、空键拒绝、既有 `Create` 不写 `origin_key`）。

---

## 8. 公开读接口与 AI 挂接

```go
// DiagnosisSummary is what a later reader - the AI judgement layer, or the
// today dashboard - may see of one diagnosis report version.
func (s *Store) DiagnosisSummary(ctx context.Context, workspaceID, reportID string, versionNo int) (DiagnosisSummary, error)
```

**允许清单**（`DiagnosisSummary` 只含这些）：`report_id`、`version_no`、`calc_version`、`data_origin`、`scope`（账号 id、平台、显示名、配置 `revision_id`、两个窗口、各类输入条数）、`sections[].dimensions[]` 的 `status` / `reason` / `facts` / `completeness` / `limits`、`gaps[]`（`gap_key`、`kind`、`dimension`、`ref`、`account_id`）、`rules`、`roi_reference`（它自己的摘要）、`created_at`。

**不含**：`inputs` 原文、任何人写的正文（判断、建议、待办、`note`）、`created_by` / `recorded_by`、摘录原文与解读、账号配置各项的 `value`、`customer_ref`、`order_ref`。一条反射用例扫全部字段名（含嵌套）确认。

**AI 挂接（本卡只写合同，不写代码）**：

- 挂接键：`(report_id, version_no)`。
- 读：只经 `DiagnosisSummary`。扩充允许清单要主控批准。
- 写：AI 产生的判断与建议进 §1.3 / §1.4 两张表，`author_kind = ai`（届时一个迁移改 `CHECK`，并改 `AuthorKinds` 与「恰好一项」用例）；它们**同样**只能经 §7.2 的人工决定产生效果。
- 状态：`ai_judgement_state` 读时派生，复用 027 的 `ReviewState`，本卡恒为 `pending_data`。
- AI 永远不能写经营记忆：经营记忆的采纳是另一张卡，届时也只能由人决定。

---

## 9. 不做的自证（守卫用例，每条配一次可编译变异确认会红）

- **只插不改**：本卡八张表在 `opdiag_*.go` 里没有 `UPDATE`、`ON CONFLICT DO UPDATE`、删除链之外的 `DELETE`；**且**每张表有 `INSERT INTO`。
- **不用浮点**：`opdiag_calc.go` 及其辅助文件不含 `float32` / `float64`。
- **不外发、不调模型**：沿用 027 的包级守卫。
- **不聚合在 SQL 里**：沿用 027 的包级守卫；本卡的 SQL 里没有 `SUM(` / `AVG(` / `GROUP BY` / `count(`。
- **不合并阅读与播放**：沿用 027 的包级守卫；表现维度按指标 id 泛型遍历。
- **不读别人的表**：`opdiag_*.go` 的 SQL 只出现 `content_opdiag_`、`content_manual_metric`、`content_feedback_excerpt`，以及 034 读接口所在文件里的 `content_roi_`。
- **不读素材与知识**：`opdiag_*.go` 与 `server/internal/handler/content_opdiag_*.go` 不引用 `source-inbox` / `sourceinbox` / `knowledge-base` / `knowledgebase`。
- **不写开发诊断**：`opdiag_*.go` 不写 `diagnostics` 模块的表；开发诊断模块的源文件不出现 `content_opdiag_`。
- **没有分数与角色**：参数、结果、`DiagnosisSummary` 的类型里没有 `score` / `grade` / `rating` / `rank` / `level` / `role` / `industry` 字段（反射）。
- **没有 AI 与记忆**：迁移里没有 AI 判断表或经营记忆表；`AuthorKinds` 恰好 `human`；模块里没有写经营记忆的路径。
- **拒绝不写**：`reject` 路径的函数体里不调用任何适配器的写方法（按函数名白名单扫）。
