# 合同：成本、线索、成交与 ROI 复盘（034）

**状态**：已裁决（主控 2026-09-25，PR #255 评论）：Q1～Q7 采纳推荐值；导入幂等在模块内自建（§1.9），不 import `idempotency`。每个列名、受控集都要能指回 `spec.md` 开头的 R-061 原文或「主控已定」D1–D9；指不回去的在 `spec.md`「本规格补的设计」里单列。

模块：`feedback-learning`。所有 Go 代码在 `server/internal/content/feedback-learning/` 下以 `roi_` 为文件名前缀。

---

## 1. 十张表

**迁移编号不预留**（主控 2026-09-25）：每个实施 PR 在合入前把自己的迁移改号为紧接当时 `app-main` 最大号之后的连续号，文件名一起改；033 与 034 的实施谁先合入谁先占号。规格撰写时最大号 539，仅供参考。每张表：

- 建表迁移不含 `PRIMARY KEY` / `UNIQUE` / `REFERENCES` / `CASCADE`（R1、R2、R5）；
- 受控集用 `CHECK (... IN (...))` 兜底，Go 枚举为准；
- 只插不改：没有 `UPDATE`，没有删除链之外的 `DELETE`；
- 进 `workspace_delete_manifest_test.go` 与 `workspace_delete.sql` 的同一 CTE 链。

**修订制的共同列**（下文各表省略不写）：`workspace_id text NOT NULL`、`revision integer NOT NULL`（从 1 起）、`voided boolean NOT NULL DEFAULT false`、`note text NOT NULL DEFAULT ''`、`recorded_by text NOT NULL`（服务端从会话取）、`created_at timestamptz NOT NULL DEFAULT now()`。当前状态 = 同一 id 的最大 `revision`。

### 1.1 `content_roi_cost_revision` —— 成本（PR 1）

| 列 | 类型 | 指回 | 说明 |
|---|---|---|---|
| `cost_id` | text | — | 稳定键 |
| `category` | text | 「策划、人工、拍摄、剪辑、投放等类别」 | 自由文本（Q5=A），≤500 rune |
| `pricing` | text CHECK `amount`/`labor_time` | 「人工时间须有用户设定单价才能折算金额」 | |
| `amount_minor` | bigint NULL | 「金额」 | `pricing=labor_time` 且缺单价时为 NULL，表示「不可计算」 |
| `currency` | text NOT NULL | 「币种」 | 三位大写字母；Go 侧查币种表 |
| `labor_minutes` | integer NULL | | 仅 `labor_time` |
| `labor_rate_minor` | bigint NULL | 「用户设定单价」 | 每小时，最小单位；NULL = 未设 |
| `incurred_at` | timestamptz NOT NULL | 「时间」 | |
| `ad_spend` | boolean NOT NULL DEFAULT false | R-061「广告 ROAS（归因广告收入÷广告支出）」 | |
| `account_id` | text NOT NULL DEFAULT '' | 「可关联账号」 | '' = 未关联 |
| `work_id` | text NOT NULL DEFAULT '' | 「作品」 | |
| `campaign_label` | text NOT NULL DEFAULT '' | 「活动/项目」 | Q4=A 自由文本 |
| `evidence_note` | text NOT NULL DEFAULT '' | 「来源证据」 | 附件等 W-03 |
| `dedupe_key` | text NOT NULL DEFAULT '' | D14-V11 | §4 |
| `not_duplicate_of` | text[] NOT NULL DEFAULT '{}' | D14-V11「留痕」 | 用户确认「不是重复」时对照的记录 id |
| `source_type` | text CHECK `manual`/`import` | | 系统写，由调用的端点决定（027 做法） |
| `import_batch_id` | text NOT NULL DEFAULT '' | | 导入写入时填 |

`amount_minor` 与 `labor_*` 的组合用一条表级 `CHECK` 兜底：`pricing='amount'` 时 `amount_minor IS NOT NULL AND labor_minutes IS NULL`。

**没有**「计入销售成本」一类的列（FR-012）。

### 1.2 `content_roi_cost_allocation` —— 分摊（PR 2，只插）

一行 = 某成本修订的一份。随成本修订版本化：改分摊 = 给成本写新修订并带新分摊。

| 列 | 说明 |
|---|---|
| `workspace_id`, `cost_id`, `cost_revision` | 属于哪一修订 |
| `target_kind` CHECK `work`/`account`/`campaign_label`/`period` | FR-031 |
| `target_id` | 作品 id / 账号 id / 标签文本 / `YYYY-MM` |
| `method` CHECK `weights`/`amounts` | |
| `weight` integer NULL | `weights` 方式的正整数权重 |
| `allocated_minor` bigint NOT NULL | 服务端按 FR-032 算出（`amounts` 方式即用户填的数） |
| `created_at` | |

同一 `(cost_id, cost_revision)` 的 `allocated_minor` 之和 = 该修订的 `amount_minor`，由写事务内的 Go 代码保证并有用例；数据库不表达这条约束。

### 1.3 `content_roi_lead_revision` —— 线索（PR 1）

| 列 | 说明 |
|---|---|
| `lead_id` | 稳定键 |
| `customer_ref` | 化名标识，≤100 rune；可空（不知道是谁也能登记） |
| `stage` | 自由文本（Q5=A），≤100 rune |
| `qualified` boolean NOT NULL DEFAULT false | 「有效线索」 |
| `first_seen_at` timestamptz NOT NULL | 首次登记时间，决定落入哪个窗口 |
| `merged_into` text NOT NULL DEFAULT '' | 非空 = 已合并进该线索 |
| `dedupe_key`, `not_duplicate_of`, `source_type`, `import_batch_id` | 同 1.1 |

**没有**姓名、手机、微信、邮箱、证件、地址列（FR-016，守卫用例按列名扫）。

### 1.4 `content_roi_touch_revision` —— 触点 = 证据（PR 1）

| 列 | 说明 |
|---|---|
| `touch_id` | 稳定键 |
| `lead_id` | 挂在哪条线索 |
| `evidence_type` CHECK 六值 | `platform_linked_content` / `content_comment` / `customer_statement` / `dedicated_channel` / `account_only` / `unknown` |
| `platform` text NOT NULL DEFAULT '' | '' 或 `ip-profile` 八平台之一（`CHECK` 含 ''） |
| `account_id`, `work_id`, `publication_record_id` | 均可为 '' |
| `role` CHECK `first_touch`/`pre_booking`/`other` | |
| `paid` boolean NOT NULL DEFAULT false | 投放触点，广告 ROAS 分子用 |
| `occurred_at` timestamptz NOT NULL | |
| `evidence_note` | |

字段组合校验表见 spec FR-019；在 Go 里做，`CHECK` 只兜底 `unknown` 行必须三者皆空这一条。

### 1.5 `content_roi_deal_revision` —— 成交（PR 1）

| 列 | 说明 |
|---|---|
| `deal_id` | 稳定键 |
| `lead_id` text NOT NULL DEFAULT '' | 关联线索 |
| `order_ref` text NOT NULL DEFAULT '' | 订单标识，≤200 rune |
| `amount_minor` bigint NOT NULL, `currency` | 确认金额 |
| `closed_at` timestamptz NOT NULL | |
| `gross_basis` CHECK `none`/`stated_gross_profit`/`cogs` | |
| `gross_profit_minor` bigint NULL | `stated_gross_profit` 时必填 |
| `cogs_minor` bigint NULL | `cogs` 时必填 |
| `dedupe_key`, `not_duplicate_of`, `source_type`, `import_batch_id` | 同 1.1 |

### 1.6 `content_roi_adjustment_revision` —— 退款/调整（PR 1）

| 列 | 说明 |
|---|---|
| `adjustment_id`, `deal_id` | |
| `kind` CHECK `refund`/`adjustment` | |
| `revenue_delta_minor` bigint NOT NULL | 退款为正数，表示减少；调整可正可负 |
| `gross_delta_minor` bigint NULL | NULL = 未给出（FR-025） |
| `currency` | 必须与成交相同（Go 校验） |
| `occurred_at` | |

### 1.7 `content_roi_attribution_revision` —— 归因判断（PR 2）

稳定键就是 `deal_id`：一笔成交一份判断，修订制。

| 列 | 说明 |
|---|---|
| `deal_id` | |
| `judgement` CHECK `confirmed`/`operator_judgement`/`multi_touch`/`unknown` | |
| `touch_ids` text[] NOT NULL DEFAULT '{}' | 采信的触点；`unknown` 时必须为空 |
| `weights` integer[] NOT NULL DEFAULT '{}' | 与 `touch_ids` 等长或为空（无人工权重） |

### 1.8 `content_roi_import_batch` —— 导入批次（PR 3，只插）

| 列 | 说明 |
|---|---|
| `import_batch_id`, `workspace_id` | |
| `record_kind` CHECK `cost`/`lead`/`deal` | |
| `row_count`, `written_count`, `skipped_count` integer | |
| `rows` jsonb NOT NULL | 每行 `{row, outcome: written|duplicate|confirmed_not_duplicate, record_id, duplicate_of[]}` |
| `idempotency_key` text NOT NULL DEFAULT '' | 便于排查，不是唯一性来源（唯一性在 §1.9 的占位表） |
| `recorded_by`, `created_at` | |

### 1.9 `content_roi_import_claim` —— 导入占位（PR 3，只插）

导入的 `Idempotency-Key` 幂等，**完全在本模块内**，不 import `idempotency` 模块（主控否决了改 `modules` 依赖表）。形状照 031 的 `content_import_idempotency`（迁移 538 / 539），差别只有一处：031 先插占位、再 `UPDATE` 写入 `completed` 与 `response_body`；本表只插不改，占位行插入时就带上本次的 `import_batch_id`，重放时从导入批次行重建响应。

| 列 | 说明 |
|---|---|
| `workspace_id` text NOT NULL | |
| `record_kind` text CHECK `cost`/`lead`/`deal` | 相当于 031 的 `resource_scope`；同一个键可用于不同记录类型 |
| `idempotency_key` text NOT NULL | 请求头 `Idempotency-Key`，≤255 字节，空则不走幂等 |
| `request_fingerprint` text NOT NULL | 请求体（`record_kind` + 行 + 每行 `not_duplicate_of`）规范化 JSON 的 SHA-256 十六进制；在 `roi_import.go` 里计算 |
| `import_batch_id` text NOT NULL | 本次导入写出的批次；在占位前生成 |
| `created_at` timestamptz NOT NULL DEFAULT now() | |

**流程**（全部在导入的同一写事务里，栅栏之后）：

1. `INSERT ... ON CONFLICT (workspace_id, record_kind, idempotency_key) DO NOTHING RETURNING import_batch_id`。有返回 → 本请求是第一个，继续导入。
2. 没有返回 → 这个键已被一个**已提交**的请求占用：若另一个事务正持有同一个键，PostgreSQL 会让第 1 步等它结束——它回滚，第 1 步照常插入成功；它提交，第 1 步不返回行。此时 `SELECT` 读出那一行。
3. 读到的 `request_fingerprint` 与本请求不同 → 409，点名 `Idempotency-Key`。
4. 相同 → 读 `import_batch_id` 对应的导入批次行，重建响应返回；**不再写任何行**。重建函数与首次返回用同一个函数，所以两次响应逐字节相同（有用例）。

`dry_run: true` 不占位。

### 1.10 `content_roi_report_version` —— 报告输入版本（PR 4，只插）

| 列 | 说明 |
|---|---|
| `report_id`, `version_no` integer | 稳定键对 |
| `title` | ≤200 rune |
| `params` jsonb NOT NULL | §5.1 |
| `inputs` jsonb NOT NULL | 参与记录 `[{kind, id, revision}]`，以及计算时读到的各修订内容的完整副本（复算不依赖原表是否还在） |
| `calc_version` text NOT NULL | 如 `roi-calc/1` |
| `result` jsonb NOT NULL | §5.2 |
| `created_by`, `created_at` | |

**没有 AI 解释表**（FR-061）。

---

## 2. 索引（每个一个迁移文件、单条语句、`CONCURRENTLY`，登记进 `concurrentIndexCleanups`）

| 表 | 索引名 | 列 | 唯一 | 服务什么 |
|---|---|---|---|---|
| cost_revision | `content_roi_cost_revision_key_idx` | `(workspace_id, cost_id, revision)` | 是 | 修订号不重复；按 id 读；删除链 |
| | `content_roi_cost_revision_time_idx` | `(workspace_id, incurred_at)` | | 窗口查询 |
| | `content_roi_cost_revision_dedupe_idx` | `(workspace_id, dedupe_key)` | | 重复识别 |
| cost_allocation | `content_roi_cost_allocation_cost_idx` | `(workspace_id, cost_id, cost_revision)` | | 读分摊；删除链 |
| lead_revision | `content_roi_lead_revision_key_idx` | `(workspace_id, lead_id, revision)` | 是 | |
| | `content_roi_lead_revision_time_idx` | `(workspace_id, first_seen_at)` | | |
| | `content_roi_lead_revision_dedupe_idx` | `(workspace_id, dedupe_key)` | | |
| touch_revision | `content_roi_touch_revision_key_idx` | `(workspace_id, touch_id, revision)` | 是 | |
| | `content_roi_touch_revision_lead_idx` | `(workspace_id, lead_id)` | | 读一条线索的触点 |
| deal_revision | `content_roi_deal_revision_key_idx` | `(workspace_id, deal_id, revision)` | 是 | |
| | `content_roi_deal_revision_time_idx` | `(workspace_id, closed_at)` | | |
| | `content_roi_deal_revision_dedupe_idx` | `(workspace_id, dedupe_key)` | | |
| adjustment_revision | `content_roi_adjustment_revision_key_idx` | `(workspace_id, adjustment_id, revision)` | 是 | |
| | `content_roi_adjustment_revision_deal_idx` | `(workspace_id, deal_id)` | | |
| attribution_revision | `content_roi_attribution_revision_key_idx` | `(workspace_id, deal_id, revision)` | 是 | |
| import_batch | `content_roi_import_batch_key_idx` | `(workspace_id, import_batch_id)` | 是 | |
| | `content_roi_import_batch_time_idx` | `(workspace_id, created_at DESC)` | | 列表 |
| import_claim | `content_roi_import_claim_key_idx` | `(workspace_id, record_kind, idempotency_key)` | 是 | 同键只占一次；`ON CONFLICT` 的目标；删除链 |
| report_version | `content_roi_report_version_key_idx` | `(workspace_id, report_id, version_no)` | 是 | |
| | `content_roi_report_version_time_idx` | `(workspace_id, created_at DESC)` | | 列表 |

**十张表、二十个索引、三十个迁移。** 每张表的第一个索引以 `workspace_id` 打头，删除链不会扫全表。唯一索引承担并发写同一修订号时的最后一道防线：第二个写入者得到唯一冲突，映射为 409 `base_revision`。

---

## 3. 受控集

| 集合 | 取值 | 出处 |
|---|---|---|
| 币种 → 小数位 | `CNY 2` `HKD 2` `TWD 2` `USD 2` `EUR 2` `GBP 2` `SGD 2` `JPY 0` `KRW 0` | D2；Q1=A |
| `pricing` | `amount` / `labor_time` | R-061「人工时间须有用户设定单价」 |
| `evidence_type` | 六值，见 1.4，**无「其它」** | R-061 第 5、6 条；D4 |
| `role` | `first_touch` / `pre_booking` / `other` | R-061「首次了解、预约前触点等角色」；Q5=A |
| `platform` | `ip-profile` 的八个平台 + ''；读 Go 源文件对表 | 029 做法 |
| `gross_basis` | `none` / `stated_gross_profit` / `cogs` | R-061「可用毛利依据」「毛利须扣除对应销售成本」 |
| `adjustment.kind` | `refund` / `adjustment` | R-061「退款/调整记录」 |
| `judgement` | `confirmed` / `operator_judgement` / `multi_touch` / `unknown` | R-061「可确认来源、用户判断、多触点及来源不明」 |
| `target_kind` | `work` / `account` / `campaign_label` / `period` | FR-031 |
| `allocation.method` | `weights` / `amounts` | FR-031 |
| `attribution_method`（报告参数） | `first_touch` / `last_touch` / `even_split` / `judgement_weights` | R-061「归因方法由人选择」 |
| `source_type` | `manual` / `import`，系统写 | 027 做法 |
| `reason`（不可计算原因码，有序） | `no_data`、`currency_unconverted`、`labor_rate_missing`、`missing_gross_profit`、`refund_without_gross_delta`、`missing_denominator`、`zero_denominator` | FR-042；多个原因同时成立时取表中最前的 |
| 指标 id | `spend_total`、`ad_spend_total`、`qualified_leads`、`bookings`、`deals`、`net_revenue`、`attributed_net_revenue`、`attributed_gross_profit`、`attribution_coverage_count`、`attribution_coverage_amount`、`conversion_rate`、`cost_per_qualified_lead`、`cost_per_deal`、`business_roi`、`revenue_to_spend`、`ad_roas` | FR-043～FR-049 |

**自由文本**（不发明取值）：`category`、`stage`、`campaign_label`、`customer_ref`、`order_ref`、`note`、`evidence_note`。

---

## 4. 去重键

服务端计算，SHA-256 十六进制。规范化：Unicode NFC、去首尾空白、连续空白压成一个；**不做大小写折叠**（与 `oneOf` 同理）。日期按品牌时区取 `YYYY-MM-DD`。

| 记录 | 键的组成 | 为空时 |
|---|---|---|
| 成本 | `category` + `incurred_at` 的日期 + `currency` + (`amount_minor` 或 `labor_minutes`/`labor_rate_minor`) + `ad_spend` | 不会为空 |
| 线索 | `customer_ref` + `first_seen_at` 的日期 | `customer_ref` 为空 → 键为 ''，不参与重复识别 |
| 成交 | `order_ref` 非空时只用 `order_ref`；否则 `lead_id` + `closed_at` 的日期 + `currency` + `amount_minor` | — |

只与**当前有效**（最新修订未作废、线索未合并）的记录比。命中时写入被挡下，响应 `409 {code: "possible_duplicate", matches: [...]}`；请求带上 `not_duplicate_of: [命中的 id]` 后写入，这些 id 存进该行并写审计。改自己的修订时不与自己比。

---

## 5. 计算器

纯函数 `roiCalculate(input ReportInput) Result`，文件 `roi_calc.go`，**不读库、不读时钟、不用浮点**。

### 5.1 输入 `params`

```json
{
  "window": {"start": "2026-09-01", "end": "2026-09-30", "timezone": "Asia/Shanghai"},
  "report_currency": "CNY",
  "rates": [{"from": "USD", "to": "CNY", "rate": "7.1234", "note": "9 月 30 日中行牌价", "entered_by": "u1", "entered_at": "..."}],
  "attribution_method": "even_split",
  "conversion": {"from_stage": "咨询", "to_stage": "预约"},
  "booking_stage": "预约",
  "scope": {"account_ids": [], "work_ids": [], "campaign_labels": []},
  "generated_at": "..."
}
```

`generated_at` 由服务端在生成时写入，是 FR-047「晚到退款」的截止点；计算器只读它，不读时钟。

### 5.2 输出 `result`

```json
{
  "calc_version": "roi-calc/1",
  "window": {...}, "report_currency": "CNY",
  "metrics": {
    "business_roi": {"status": "ok", "value": "2", "display": "200.00%",
      "numerator": "200000", "denominator": "100000",
      "formula": "business_roi/1", "records": [{"kind":"cost","id":"c1","revision":1,"amount_minor":"100000","currency":"CNY","converted_minor":"100000"}]},
    "revenue_to_spend": {"status": "ok", "value": "10", "display": "10.00 倍", ...},
    "ad_roas": {"status": "not_computable", "reason": "zero_denominator", "records": []}
  },
  "breakdown": {
    "by_work": [...], "by_account": [...],
    "account_level_unknown_work": {...}, "brand_level_unknown_account": {...},
    "unattributed": {...}
  },
  "rules": ["退款与调整跟随所属成交计入，截止到报告生成时间", "..."]
}
```

- 所有金额与比率在 JSON 里是**字符串**：整数金额是十进制整数串；比率 `value` 是约分后的有理数串（`"2"`、`"-1/2"`），`display` 是按 FR-005 舍入后的显示串。前端只显示 `display`，不解析 `value` 做算术。
- 百分比显示：`value × 100`，两位小数，负号用 U+2212 `−`。倍数显示：两位小数 + 「倍」（i18n 键，不硬编码）。
- 公式 id 与计算版本是常量；改任何一个公式或舍入规则 → `roi-calc/2`，旧版本的报告保留 `roi-calc/1` 的结果。

### 5.3 公式（`roi-calc/1`）

| 指标 | 公式 |
|---|---|
| `spend_total` | 窗口内有效成本（按范围过滤后取分摊到范围内的份额；无范围过滤时取原额）换算后之和 |
| `ad_spend_total` | 其中 `ad_spend=true` 的部分 |
| `net_revenue` | 窗口内成交的 `amount_minor − Σ revenue_delta`（退款按 FR-047） |
| `attributed_net_revenue` | 按归因方法分给已归因触点的净额之和（不含来源不明/未判断） |
| `attributed_gross_profit` | 同上，毛利口径 |
| `business_roi` | `(attributed_gross_profit − spend_total) / spend_total` |
| `revenue_to_spend` | `attributed_net_revenue / spend_total` |
| `ad_roas` | 分给 `paid=true` 触点的净额 / `ad_spend_total` |
| `attribution_coverage_count` | 已归因成交数 / 窗口内成交数 |
| `attribution_coverage_amount` | 已归因成交净额 / 窗口内成交净额 |
| `conversion_rate` | 到达 `to_stage` 的线索数 / 到达 `from_stage` 的线索数（同一批：窗口内首次登记） |
| `cost_per_qualified_lead` | `spend_total / qualified_leads` |
| `cost_per_deal` | `spend_total / deals` |

「到达某阶段」首版按「任一修订的 `stage` 等于该值」判断（阶段是自由文本，没有顺序可比较，Q5=A 的代价）。这条代价 MUST 作为一条固定文字进 `result.rules`（FR-048a）：「阶段是自由文本，没有先后顺序；转化率只按线索是否到达过某阶段计算，不反映阶段之间的先后。」

### 5.4 固定样例（必须逐字出现在用例里）

| 名称 | 输入 | 期望 |
|---|---|---|
| `D14-V12/roi` | 一笔成本 `100000 CNY`；一笔成交 `stated_gross_profit=300000`，归因判断 `confirmed`、采信一个触点 | `business_roi.display = "200.00%"` |
| `D14-V12/revenue` | 同一笔成本；一笔成交 `amount_minor=1000000`，`gross_basis=none`，同样归因 | `revenue_to_spend.display = "10.00 倍"`；`business_roi.reason = "missing_gross_profit"` |
| `negative` | 成本 `100000`；毛利 `50000` | `"−50.00%"` |
| `zero-spend` | 无成本；有成交 | 三个以投入为分母的指标 `zero_denominator`（窗口内有成本记录但合计为 0）或 `no_data`（窗口内一条成本都没有） |
| `unconverted` | 成本 `USD 10000`，报告币种 CNY，无汇率 | `spend_total.reason = "currency_unconverted"`，`records` 含该成本 |
| `converted` | 同上加 `USD→CNY 7.1234` | `spend_total.value = "71234"` |
| `split-10001` | 成交 `10001`，两个采信触点，`even_split` | 两份 `5001`、`5000` |
| `alloc-10000` | 成本 `10000`，三个作品权重 1:1:1 | `3334`、`3333`、`3333`（目标键字典序最小的拿余数） |
| `one-booking-two-works` | 一条线索阶段「预约」、两个作品触点、一笔成交 | `bookings = 1`、`deals = 1` |

---

## 6. 端点

全部挂在 `/api/content-roi` 下，照 `/api/content-metrics` 的挂法（`RequireWorkspaceMember` 组之外，`h.DiagnosticTrace`，handler 内 `workspacecore.Authorize`）。

| PR | 方法 | 路径 | 用途 |
|---|---|---|---|
| 1 | GET | `/costs` | 列当前有效成本（可按窗口、账号、作品筛） |
| 1 | POST | `/costs` | 新建，`source_type=manual` |
| 1 | GET | `/costs/{costId}` | 全部修订 |
| 1 | POST | `/costs/{costId}/revisions` | 新修订（带 `base_revision`；作废也走这里） |
| 1 | GET / POST | `/leads` | 同上 |
| 1 | GET | `/leads/{leadId}` | 全部修订 + 触点（含合并进来的） |
| 1 | POST | `/leads/{leadId}/revisions` | 新修订 |
| 1 | POST | `/leads/{leadId}/merge` | 把本线索合并进 `target_lead_id`（写本线索的新修订） |
| 1 | POST | `/leads/{leadId}/touches` | 加触点 |
| 1 | POST | `/leads/{leadId}/touches/{touchId}/revisions` | 触点新修订 |
| 1 | GET / POST | `/deals` | 同上 |
| 1 | GET | `/deals/{dealId}` | 全部修订 + 调整 + 归因判断 |
| 1 | POST | `/deals/{dealId}/revisions` | 新修订 |
| 1 | POST | `/deals/{dealId}/adjustments` | 退款/调整 |
| 1 | POST | `/deals/{dealId}/adjustments/{adjustmentId}/revisions` | 调整新修订 |
| 2 | POST | `/deals/{dealId}/attribution` | 归因判断新修订（带 `base_revision`，首次为 0） |
| 2 | POST | `/preview` | 用给定参数算一次，**不保存**；返回 §5.2 |
| 3 | POST | `/imports` | 导入，`source_type=import`；支持 `Idempotency-Key`（本模块占位表，§1.9） |
| 3 | GET | `/imports` | 批次列表 |
| 3 | GET | `/imports/{batchId}` | 批次详情（逐行结果） |
| 4 | POST | `/reports` | 新报告 + 版本 1 |
| 4 | GET | `/reports` | 每份报告的最新版本 |
| 4 | GET | `/reports/{reportId}/versions` | 版本列表 |
| 4 | GET | `/reports/{reportId}/versions/{versionNo}` | 一版，含派生的 `inputs_changed` |
| 4 | POST | `/reports/{reportId}/versions` | 生成新版本 |

成本的分摊（PR 2）是 `POST /costs` 与 `/costs/{costId}/revisions` 请求体里的可选 `allocations` 字段；PR 1 的这两个端点收到非空 `allocations` 时返回 400 点名 `allocations`（PR 2 放开）。

### 决策顺序（第一个不通过者即拒绝）

| 序 | 检查 | 失败时 |
|---|---|---|
| 1 | 工作区成员资格（`workspace-core.Authorize`） | `RefusalStatus` / `RefusalBody`，与「不存在」同形 |
| 2 | 路径里的 id 在本工作区存在；引用的账号、作品、发布记录、线索、成交存在 | 同上，逐字节相同 |
| 3 | 受控集 | 400 诊断错误对象，点名字段；批量点名第几行 |
| 4 | 必填项、字段组合（FR-019）、金额格式（FR-003）、币种一致 | 400，点名字段 |
| 5 | 长度上限（按 rune） | 400，点名字段 |
| 6 | `base_revision` 与当前最新不符 | 409，点名 `base_revision` |
| 7 | 疑似重复且未确认 | 409 `possible_duplicate` |
| 8 | 幂等键冲突（仅 `/imports`） | 409，点名 `Idempotency-Key` |

### 带路径参数的端点

上表中带 `{...}` 的每一个，各一条穿过真实 router 与中间件、路径参数值 ≠ 上下文值的用例（工作流第 12 步），只用 `chi.URLParam` 取参数。

---

## 7. 给后续消费者的公开读接口

```go
// ReportSummary is what a later reader - BO-02's diagnosis, or an AI
// explanation layer - may see of one report version.
func (s *Store) ReportSummary(ctx context.Context, workspaceID, reportID string, versionNo int) (ReportSummary, error)
```

`ReportSummary` **只含**：`report_id`、`version_no`、`title`、`calc_version`、`params`（去掉 `rates[].note` 与 `entered_by`）、`metrics`（每项的 `status`、`display`、`reason`、`formula`，以及 `records` 里的 `kind`/`id`/`revision`/金额）、`breakdown`（按作品/账号的金额与计数）、`rules`、`created_at`。

**不含**：`customer_ref`、`order_ref`、`note`、`evidence_note`、`recorded_by`、任何线索或成交的其他文本列。一条反射用例扫 `ReportSummary` 的全部字段名（含嵌套）确认。这是 FR-062 与 D14-V11「模型输入不包含未授权客户字段」的结构保证；真正的模型输入组装在 AI 解释卡里再验一次。

---

## 8. 不做的自证（守卫用例，每条配一次可编译变异确认会红）

- **只插不改**：模块里没有针对十张表的 `UPDATE`，没有删除链之外的 `DELETE`；**且**存在 `INSERT INTO`。
- **不用浮点**：`roi_calc.go`、`roi_money.go`、`roi_allocate.go` 不含 `float32` / `float64`。
- **不外发、不调模型**：模块里没有 HTTP 客户端调用、没有模型调用。
- **不 import `idempotency`**：模块源码里没有对 `server/internal/content/idempotency` 的 import（`pnpm check:content-boundaries` 也会挡，但守卫用例让它在 `go test` 里就红）。
- **不自动补来源**：模块里没有在触点或归因判断上写 `work_id` / `evidence_type` / `judgement` 的路径，除了处理请求体的那一处（按函数名白名单扫）。
- **不收集个人信息**：迁移与 Go 结构里没有 `name` / `phone` / `mobile` / `wechat` / `email` / `id_card` / `address` 类的客户字段。
- **没有 AI 解释表、没有采纳路径**：迁移里没有这样的表；模块里没有写选题、待办、经营记忆的路径。
