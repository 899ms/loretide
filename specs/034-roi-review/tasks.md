---
description: "Task list for 034 feedback-learning — cost, lead, deal and ROI review (BO-06 / R-061)"
---

# Tasks: 成本、线索、成交与 ROI 复盘（034）

**Prerequisites**: `spec.md`、`plan.md`、`contracts/roi-review.md`

**改动文件必须在 plan.md → Project Structure 清单内。**

**五个 PR，按顺序**。每个 PR 是一张可以单独审的卡：列出文件、测试、覆盖的验收编号、本地验证命令，以及**远程验收**（主控在构建服务器上跑 `~/loretide-ci/lt-verify.sh <branch> all`）。PR 2 与 PR 3 可以并行；其余按序。

`[P]` 表示与相邻任务无依赖、可并行。「先写」表示该测试先写并确认失败。

实施前先确认 spec 文末七条待裁决的结论；若有与推荐值不同的，按 spec 里每条标出的「影响」改对应 FR 与本文件的对应任务，不扩散。

---

# PR 1 —— 记录存储与接口（成本、线索、触点、成交、退款/调整）

**分支**：`claude/034-pr1-roi-records`，从 `app-main` 起。
**覆盖**：FR-001～FR-004、FR-010～FR-024、FR-026（手工登记部分）、FR-070～FR-076；SC-005、SC-008、SC-012～SC-015；D14-V11（越权、重复识别的手工部分）、D14-V16（证据分开、作品可空）。
**不含**：分摊（请求体带非空 `allocations` → 400）、归因判断、计算、导入、报告、页面。

## Phase 1: 基线

- [ ] T001 记下 `server/migrations/` 当前最大号（规格撰写时 539），本 PR 从下一个号起连续取 18 个；若 033 或别的卡先合入占号，rebase 时编号与文件名整体重排
- [ ] T002 记下 `scripts/content-boundaries.json` 的当前内容；本 PR 对它只允许追加 `adapters` 一条（`server/internal/handler/content_roi_records.go`），且须主控在 PR 上确认（plan.md「主控决定」第 2 条）
- [ ] T003 读三处既有形状并在 PR 正文写一句结论：`feedback-learning/store.go` 的 `begin()`、`handler/content_metric.go` 的 `feedbackScope` 与 `feedbackPublications`、`handler/workspace_delete_manifest_test.go` 的清单形状

## Phase 2: 迁移（18 个，各单条语句）

- [ ] T004 五个建表迁移：`content_roi_cost_revision`、`content_roi_lead_revision`、`content_roi_touch_revision`、`content_roi_deal_revision`、`content_roi_adjustment_revision`，列按 contract §1.1、1.3～1.6；无 `PRIMARY KEY` / `UNIQUE` / `REFERENCES` / `CASCADE`；受控集 `CHECK`；down 注释写明「会丢失已登记的真实经营数据」
- [ ] T005 [P] 十三个索引迁移（contract §2 表中这五张表的行），每个 `CREATE [UNIQUE] INDEX CONCURRENTLY IF NOT EXISTS`，单文件单语句
- [ ] T006 `server/cmd/migrate/main.go` 的 `concurrentIndexCleanups` 追加 13 条，自成一块（前空行 + 一行注释）；五个建表迁移**不登记**
- [ ] T007 五张表进 `workspace_delete_manifest_test.go`（`workspaceDelete`）；`workspace_delete.sql` 同一 CTE 链加五条按 `workspace_id` 的 `DELETE`；`make sqlc`，产物单独提交、不手改
- [ ] T008 跑 `go test ./internal/migrations -run 'TestContentMigrationConstraints|TestContentConcurrentIndexRegistration'` 与 `go test ./cmd/migrate -run 'TestEveryConcurrentUpBuildHasCleanup|TestConcurrentIndexCleanupsMatchTheirMigrations'`，四条都要跑到并报用例名

## Phase 3: 模块纯函数（先写测试）

- [ ] T009 [P] 先写：受控集矩阵——`evidence_type` **恰好六项**无「其它」、`pricing` 两项、`role` 三项、`gross_basis` 三项、`adjustment.kind` 两项、`source_type` 两项；各一条越界 → `FieldError` 点名字段（SC-014）
- [ ] T010 [P] 先写：`platform` 取值与 `ip-profile/account.go` 的八个平台逐字相同——读 Go 源文件对表，**不 import**
- [ ] T011 [P] 先写：币种表九项与小数位；`"3000.00" CNY → 300000`；`"3000.005" CNY` 拒绝点名 `amount`；`"100" JPY → 100`；`"1.5" JPY` 拒绝；`"-0.01"` 接受为 `-1`；超 `10^15` 拒绝；前导 `+`、空格、千分位逗号、科学计数法各一条拒绝（FR-001～FR-004）
- [ ] T012 [P] 先写：工时折算 `360 分钟 × 15000/小时 → 90000`；`1 分钟 × 1/小时` 按远离零四舍五入得 `0`；`30 分钟 × 1/小时` 得 `1`（0.5 远离零）；缺单价 → `amount_minor` 为 nil 且状态 `labor_rate_missing`（FR-011）
- [ ] T013 [P] 先写：FR-019 字段组合表逐格一条用例（六种依据类型 × 三个字段），不合法的点名那一个字段
- [ ] T014 [P] 先写：去重键——同输入同键；只差首尾空白同键；大小写不同**不同键**；线索 `customer_ref` 为空键为 ''；成交有 `order_ref` 时只看它（contract §4）
- [ ] T015 新建 `roi_contract.go`、`roi_money.go`、`roi_dedupe.go`，让 T009～T014 变绿

## Phase 4: 存储（先写测试，真实 DB）

- [ ] T016 先写守卫（`roi_guards_test.go`）：模块源码里没有针对五张表的 `UPDATE`、没有删除链之外的 `DELETE`，**且**每张表都有 `INSERT INTO`；计算相关文件无 `float32` / `float64`；无 HTTP 客户端与模型调用；迁移与 Go 结构无 `name`/`phone`/`mobile`/`wechat`/`email`/`id_card`/`address` 类字段（SC-012、SC-013、FR-073）
- [ ] T017 先写：新建成本 → 修订 1；带 `base_revision=1` 改金额 → 修订 2，修订 1 逐字节不变；`base_revision=1` 再提交 → 409 点名 `base_revision`；作废是修订 3 且 `voided=true`
- [ ] T018 先写：**修订号并发**——两个事务同时以 `base_revision=1` 提交，恰好一个成功，另一个得到 409（唯一索引那一层也要被触发一次：用测试钩子让两者都越过读检查）
- [ ] T019 先写：线索合并——B 合并进 A 写 B 的新修订并审计；读 A 得到 A 与 B 的触点、按 `touch_id` 去重；A→B→A 成环拒绝点名 `merged_into`；取消合并恢复
- [ ] T020 先写：五种来源各一条（平台关联、客户自述、仅知账号、完全未知、多触点），作品可空的读回仍为 ''，依据类型不变（SC-005 前半）
- [ ] T021 先写：成交 `cogs` 与 `stated_gross_profit` 两种依据写入读回；退款使净额 < 0 → 400 点名 `amount`；退款币种与成交不同 → 400 点名 `currency`
- [ ] T022 先写：手工登记疑似重复 → 409 `possible_duplicate` 带命中 id；带 `not_duplicate_of` 后写入、该列有值、审计有一条；作废或已合并的记录不参与比对（FR-026）
- [ ] T023 先写：删除栅栏——工作区删除已提交后，每条写路径被拒绝且不留半条记录（SC-015）
- [ ] T024 新建 `roi_records.go`：`begin()` 复用；每条写路径 栅栏 → 业务行 → `AuditTx`；`Accounts` / `Works` 接口定义在模块里
- [ ] T025 删除工作区后五张表在该工作区行数各为 0（进既有的删除链用例）

## Phase 5: HTTP（先写测试）

- [ ] T026 先写：决策顺序（contract §6 表）逐序一条；越权与不存在逐字节相同（除 `trace_id`）；每个端点一条越权用例（SC-008）
- [ ] T027 先写：请求体 `amount` 为 JSON 数字 → 400 点名 `amount`；`recorded_by` / `source_type` 在请求体里 → 被忽略（服务端写）
- [ ] T028 先写：PR 1 的 `POST /costs` 与 `/costs/{costId}/revisions` 带非空 `allocations` → 400 点名 `allocations`
- [ ] T029 先写（工作流第 12 步）：本 PR 每个带路径参数的端点（`/costs/{costId}`、`/costs/{costId}/revisions`、`/leads/{leadId}`、`/leads/{leadId}/revisions`、`/leads/{leadId}/merge`、`/leads/{leadId}/touches`、`/leads/{leadId}/touches/{touchId}/revisions`、`/deals/{dealId}`、`/deals/{dealId}/revisions`、`/deals/{dealId}/adjustments`、`/deals/{dealId}/adjustments/{adjustmentId}/revisions`）各一条穿过真实 router 与中间件、参数 ≠ 上下文的用例，只用 `chi.URLParam`
- [ ] T030 新建 `handler/content_roi_records.go`：`roiScope`（同 `feedbackScope`）、`roiAccounts`（调 `ipprofile.Service.Get`）、`roiWorks`（调 `workeditor.Store.GetWork`）、复用 `feedbackPublications`
- [ ] T031 upstream 提交：`router.go` 挂 `/api/content-roi` 块（本 PR 的路由），自成一块；`git diff -w` 只有新增行
- [ ] T032 新建 `cmd/server/content_roi_routes_test.go`：本 PR 每个端点的路由存在性用例（FR-074）

## Phase 6: core 契约

- [ ] T033 [P] `packages/core/content/feedback-learning/roi/contract.ts`：五类记录的 zod schema，金额字段 `z.string()`，走 `parseWithFallback`；`contract.test.ts` 每个 schema 一条畸形响应用例（首行 `// @vitest-environment node`）
- [ ] T034 [P] `queries.ts`：列表与详情查询、写入 mutation（非乐观，成功后失效），query key 含 `wsId`

## Phase 7: 验证与 PR

- [ ] T035 本地：`(cd server && go test ./internal/content/feedback-learning/ ./internal/migrations/ ./cmd/migrate/)`；`(cd server && go test ./internal/handler/ -run 'ContentROI|WorkspaceDelet')`；`(cd server && go test ./cmd/server/ -run 'ContentROI')`；`pnpm typecheck --force`；`pnpm check:content-boundaries`；`pnpm check:diagnostics-contract`；`pnpm --filter @multica/core exec vitest run content/feedback-learning/roi`
- [ ] T036 变异验证各一处，确认对应用例变红后原样还原：去掉 `base_revision` 检查 → T017；在模块里加一条 `UPDATE content_roi_cost_revision` → T016；`account_only` 放行非空作品 → T013；金额解析改用 `strconv.ParseFloat` → T011 与 T016
- [ ] T037 Draft PR 正文：所属模块、公开契约变更、迁移号范围、上游改动一节、每个带路径参数端点的第 12 步用例名、本地已跑与未跑的检查（未跑记「未执行」）；**远程验收**：请主控跑 `~/loretide-ci/lt-verify.sh claude/034-pr1-roi-records all`，带库套件以其 PASS / SKIP 计数为准

---

# PR 2 —— 分摊、归因判断与确定性计算

**分支**：`claude/034-pr2-roi-calc`，依赖 PR 1 合入。
**覆盖**：FR-005、FR-006、FR-020、FR-021、FR-025、FR-030～FR-052；SC-001～SC-004、SC-006；D14-V12、D14-V13、D14-V14（分摊与多触点）、D14-V16（品牌不重复计数）。

## Phase 8: 迁移（4 个）

- [ ] T038 两个建表：`content_roi_cost_allocation`、`content_roi_attribution_revision`（contract §1.2、§1.7）
- [ ] T039 [P] 两个索引：`content_roi_cost_allocation_cost_idx`、`content_roi_attribution_revision_key_idx`；`concurrentIndexCleanups` 追加 2 条；两表进删除清单与删除链、`make sqlc`
- [ ] T040 跑 T008 的四条迁移规则用例

## Phase 9: 分摊与多触点（纯函数，先写）

- [ ] T041 [P] 先写：`10000` 按 1:1:1 → `3334/3333/3333`，余数给目标键字典序最小者；`100` 按七个 1 → 和为 `100`；`-10000` 按 1:1:1 → `-3334/-3333/-3333`；权重全 0 → 拒绝；`amounts` 和不等于原额 → 拒绝点名 `allocations`（SC-003）
- [ ] T042 [P] 先写：性质用例——固定种子 1000 组（原额、2～20 份、权重 1～1000），各份和恒等于原额，同一输入两次结果逐字节相同
- [ ] T043 [P] 先写：多触点 `10001` 两触点 `even_split` → `5001/5000`，并列时按（发生时间，触点 id）；`first_touch` / `last_touch`（优先 `pre_booking`）/ `judgement_weights`（无权重的笔退回 `even_split` 并标记）各一条；性质用例：各份之和恒等于成交分配基数（SC-004）
- [ ] T044 新建 `roi_allocate.go`；分摊写进 PR 1 的成本写路径（放开 `allocations`，T028 改为期望成功）

## Phase 10: 归因判断

- [ ] T045 先写：写归因判断后，该成交所有触点的修订行逐字节不变（SC-005 后半）；`unknown` 带非空 `touch_ids` → 400；`touch_ids` 引用别的线索的触点 → 与不存在同形拒绝；`weights` 与 `touch_ids` 不等长 → 400
- [ ] T046 先写：守卫——模块里除请求处理函数外，没有写 `work_id` / `evidence_type` / `judgement` 的路径（按函数名白名单扫，FR-021）
- [ ] T047 新建 `roi_attribution.go` 与 `POST /deals/{dealId}/attribution`；第 12 步用例；路由存在性用例

## Phase 11: 计算器（纯函数，先写）

- [ ] T048 [P] 先写：**D14-V12 固定样例两条**，名称与输入逐字照 contract §5.4：`D14-V12/roi` → `business_roi.display == "200.00%"`；`D14-V12/revenue` → `revenue_to_spend.display == "10.00 倍"`，且 `business_roi.reason == "missing_gross_profit"`；另断言 `revenue_to_spend` 的 id 与 i18n 键不含 `roi` / `profit` / 「利润」（SC-001）
- [ ] T049 [P] 先写：D14-V13 各一条——缺毛利、零投入（`zero_denominator`）与无成本记录（`no_data`）分开、退款缺毛利变动（`refund_without_gross_delta`）、负回报 `"−50.00%"`、未换算币种（`currency_unconverted` 且 `records` 含那笔）、录汇率后 `"71234"`、窗口外成交不计入、窗口内成交的晚到退款在 `generated_at` 之前计入之后不计入（SC-002）
- [ ] T050 [P] 先写：D14-V16——一条线索一次预约两个作品触点 → `bookings=1`、`deals=1`；作品层「触达成交数」各 1；来源不明成交分给作品与账号 0、仍在品牌总额与覆盖率分母里；仅知账号的份额进 `account_level_unknown_work`（SC-006）
- [ ] T051 [P] 先写：同一输入调用两次，`result` 的 JSON 序列化逐字节相同；打乱输入记录顺序，结果仍相同（FR-040）
- [ ] T052 [P] 先写：原因码优先级——同时缺单价与未换算时取表中靠前的 `currency_unconverted`，`records` 两笔都在（FR-051）
- [ ] T053 新建 `roi_calc.go`：`ReportInput` / `Result` 类型、`calcVersion = "roi-calc/1"`、公式表（contract §5.3）；`roi_money.go` 加汇率换算（`big.Rat`，远离零四舍五入）

## Phase 12: 预览端点与收尾

- [ ] T054 先写：`POST /preview` 返回与直接调计算器相同的结果；不写任何表（调用前后九张表行数不变）；越权同形拒绝
- [ ] T055 新建 `handler/content_roi_preview.go`：窗口按 `loretide.timezone` 转成时间点，读窗口内有效记录组装 `ReportInput`；upstream 提交挂路由；路由存在性用例
- [ ] T056 [P] core：计算结果 schema（指标值全是字符串，`status` 用 `z.enum` 并有 `default` 分支）与畸形响应用例
- [ ] T057 变异验证：余数改成全给第一份 → T041；`business_roi` 分子去掉减投入 → T048；未换算时静默排除 → T049；品牌成交数改为按触点计 → T050
- [ ] T058 本地验证同 T035；Draft PR 正文列出 contract §5.4 每个固定样例对应的用例名；**远程验收**：`~/loretide-ci/lt-verify.sh claude/034-pr2-roi-calc all`

---

# PR 3 —— 导入（幂等、重复识别、审计）

**分支**：`claude/034-pr3-roi-import`，依赖 PR 1 合入，以及 plan.md「主控决定」第 1 条（`feedback-learning` 加 `idempotency` 依赖）已获确认。
**覆盖**：FR-026～FR-029；SC-007；D14-V11（导入部分）。

- [ ] T059 按主控确认，在 `scripts/content-boundaries.json` 的 `feedback-learning` 依赖里加 `idempotency`，`adapters` 加 `server/internal/handler/content_roi_import.go`；跑 `pnpm check:content-boundaries`
- [ ] T060 迁移 3 个：`content_roi_import_batch` 建表 + 两个索引；`concurrentIndexCleanups`、删除清单与删除链、`make sqlc`；跑迁移规则四条
- [ ] T061 先写：一批三行第二行币种 `RMB` → 一行都不写，拒绝点名第 2 行 `currency`；九张表行数不变
- [ ] T062 先写：一批里两行与已有记录去重键相同 → 这两行不写，批次记录逐行 `duplicate` 带 `duplicate_of`；其余行写入且 `source_type=import`、`import_batch_id` 有值
- [ ] T063 先写：同一行带 `not_duplicate_of` → 写入、结果 `confirmed_not_duplicate`、审计一条
- [ ] T064 先写：同 `Idempotency-Key` 同输入重发 → 返回逐字节相同的原响应、行数不变；同键异输入 → 409 点名 `Idempotency-Key`；异键同行 → 全部识别为疑似重复（SC-007）
- [ ] T065 先写：`dry_run: true` → 返回逐行判定，不写任何表、不占幂等键
- [ ] T066 先写：批内自身重复（两行去重键相同）→ 第二行标 `duplicate`，`duplicate_of` 指向同批第一行生成的记录
- [ ] T067 新建 `roi_import.go`：事务内 栅栏 → `Claim` → 全批校验 → 查重 → 写入 → 批次行 → `Complete` → `AuditTx`（`operation=roi_import`，`resource_scope=record_kind`）
- [ ] T068 新建 `handler/content_roi_import.go`：`POST /imports`、`GET /imports`、`GET /imports/{batchId}`；第 12 步用例（`{batchId}`）；upstream 提交挂路由；路由存在性用例
- [ ] T069 [P] core：`roi/csv.ts` 三种记录的列定义与解析（金额保持字符串、点名第几行）+ node 测试；导入响应与批次 schema + 畸形响应用例
- [ ] T070 变异验证：`Claim` 移到事务外 → T064；查重只比作废记录 → T062；校验失败时已写的行不回滚 → T061
- [ ] T071 本地验证同 T035；**远程验收**：`~/loretide-ci/lt-verify.sh claude/034-pr3-roi-import all`

---

# PR 4 —— 报告版本

**分支**：`claude/034-pr4-roi-report`，依赖 PR 2 合入（若 PR 3 已合入，也要覆盖导入写入的记录）。
**覆盖**：FR-041、FR-053～FR-058、FR-060～FR-063；SC-009～SC-011；D14-V13（可追溯）、D14-V14（更新不覆盖旧报告）。

- [ ] T072 迁移 3 个：`content_roi_report_version` 建表 + 两个索引；`concurrentIndexCleanups`、删除清单与删除链、`make sqlc`；跑迁移规则四条；`adapters` 加 `content_roi_report.go`（按「主控决定」第 2 条）
- [ ] T073 先写：生成版本 1 → 修改其引用的一条成本 → 版本 1 的 `params` / `inputs` / `result` 逐字节不变；读版本 1 时派生 `inputs_changed=true` 并列出那条；库里没有存这个标记的列（SC-010）
- [ ] T074 先写：生成版本 2 → 版本号 +1，参数默认沿用版本 1，两版并存
- [ ] T075 先写：**复算**——contract §5.4 的每个固定样例存成报告版本，从库读回，用 `inputs` 与 `calc_version` 复算，与 `result` 逐字节相同（SC-011）
- [ ] T076 先写：`calc_version` 与每个指标的 `formula` 都在结果里；把计算器常量改成 `roi-calc/2` 后，旧版本读出来仍是 `roi-calc/1` 与原结果（FR-041）
- [ ] T077 先写：`ReportSummary` 反射扫全部字段名（含嵌套），不含 `customer_ref`、`order_ref`、`note`、`evidence_note`、`recorded_by`（SC-009）
- [ ] T078 先写：AI 解释状态恒为 `pending_data`；迁移里没有解释表；模块里没有写选题、待办、经营记忆的路径（FR-060～FR-063）
- [ ] T079 新建 `roi_report.go`：与 `/preview` 共用组装代码；`inputs` 存修订内容完整副本；`ReportSummary`
- [ ] T080 新建 `handler/content_roi_report.go`：contract §6 标 4 的五个端点；第 12 步用例（`{reportId}`、`{versionNo}`，`versionNo` 非数字 → 与不存在同形）；upstream 提交挂路由；路由存在性用例
- [ ] T081 [P] core：报告版本与列表 schema + 畸形响应用例
- [ ] T082 变异验证：生成版本时引用当前最新修订而不是快照 → T073；`ReportSummary` 加 `CustomerRef` 字段 → T077
- [ ] T083 本地验证同 T035；**远程验收**：`~/loretide-ci/lt-verify.sh claude/034-pr4-roi-report all`

---

# PR 5 —— 页面

**分支**：`claude/034-pr5-roi-pages`，依赖 PR 1～4 合入。
**覆盖**：FR-080～FR-082，以及前四个 PR 各项在界面上的呈现；SC-016（D14-V15 部分）。**不写 UI 单测，不用 computer use。**

- [ ] T084 upstream 提交：`paths.ts` 加 `roiReview`；`route-icons.ts` 加 `roiReview`（`RouteIconName` 联合类型加 `Receipt`）；`packages/views/layout/route-icon-components.tsx` 加 `Receipt`；`diagnostic-context.ts` 加 `["roi-review"]`；跑两处既有覆盖用例（`route-icons.test.ts`、`diagnostic-context.test.ts`）
- [ ] T085 [P] node 测试先行：`roi/display.ts`——按 `status` 与 `reason` 选 i18n 键；`display` 字符串原样透传，不解析数字；未知 `reason` 走 `default` 分支显示通用「不可计算」
- [ ] T086 `apps/web/app/[workspaceSlug]/(dashboard)/roi-review/page.tsx` 适配器；`adapters` 追加该路径（按「主控决定」第 2 条）
- [ ] T087 `packages/views/content/feedback-learning/roi/`：成本区块（直接金额/工时切换、广告投放勾选、分摊编辑、合计与原额并列显示）
- [ ] T088 线索区块（化名提示、触点列表、依据类型六选一且按 FR-019 禁用不适用字段、合并对话框）
- [ ] T089 成交区块（毛利依据三选一、退款/调整、归因判断：判断类型、从触点里勾选、可选权重）
- [ ] T090 导入区块（粘贴、`dry_run` 预览、逐行错误与疑似重复、逐行「不是重复」确认、批次列表与详情）
- [ ] T091 复盘报告区块（参数表单、汇率录入、生成/预览、指标卡带公式与来历展开、「不可计算」原因、分作品/账号/来源不明表、版本列表与「输入已有更新」、AI 解释占位）
- [ ] T092 四语言（en / zh-Hans / ja / ko），`locales/parity.test.ts` 通过；「销售额/投入比」「广告 ROAS」各语言标签不含 ROI / 利润类词（ROAS 本身除外）
- [ ] T093 只用既有 Multica 组件与 `--text-*` 字号；不设颜色；「不可计算」、负回报、「来源不明」用次要文字色 token（FR-082）
- [ ] T094 `specs/034-roi-review/manual-ui-todo.md` 核对并在 PR 正文列出；全部状态「未执行」
- [ ] T095 本地：`pnpm typecheck --force`、`pnpm check:content-boundaries`、`pnpm check:diagnostics-contract`、`pnpm --filter @multica/core exec vitest run content/feedback-learning/roi paths diagnostics`、`packages/views` 的 `rich-content/package-exports.test.ts` 与 `locales/parity.test.ts`；**远程验收**：`~/loretide-ci/lt-verify.sh claude/034-pr5-roi-pages all`；界面由用户按 `manual-ui-todo.md` 手验

---

# 收尾

- [ ] T096 每个 PR 合入后，主控在文档仓库回写 BO-06 的进度与 D14-V11～V16 的覆盖情况；D14-V15 记「部分：采纳段未执行（宪法 IX，后续卡）」
- [ ] T097 登记后续卡：AI 解释与建议、采纳进选题/预算待办/经营记忆（D1）；营销节点关联（Q4）；品牌级类别/阶段配置（Q5，若裁 A）
