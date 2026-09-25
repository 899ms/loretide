---
description: "Task list for 035 feedback-learning — brand/account operating diagnosis (BO-02 / R-057)"
---

# Tasks: 品牌/账号经营诊断（035）

**Prerequisites**: `spec.md`、`plan.md`、`contracts/brand-diagnosis.md`；**Q1～Q8 须先经主控裁定**（按推荐值写；改判的在开工前改本文件）。

**改动文件必须在 plan.md → Project Structure 清单内。**

**四个 PR，按顺序**。每个 PR 是一张可以单独审的卡：列出文件、测试、覆盖的验收编号、本地验证命令，以及**远程验收**（主控在构建服务器上跑 `~/loretide-ci/lt-verify.sh <branch> all`）。

`[P]` 表示与相邻任务无依赖、可并行。「先写」表示该测试先写并确认失败。

**迁移编号**：每个 PR 在合入前把自己的迁移改号为紧接当时 `app-main` 最大号之后的连续号（文件名与 `concurrentIndexCleanups` 的键一起改）；撰写时最大号 575，#266 将占 576–578。

**本地迁移测试的固定写法**（每张卡都用这一条，**不许省掉前缀**：上游 migrate 测试不设 `DATABASE_URL` 时默认连 `localhost:5432/multica`）：

```bash
(cd server && DATABASE_URL='postgres://none:none@127.0.0.1:1/none?sslmode=disable' go test ./cmd/migrate ./internal/migrations)
```

---

# PR 1 —— 存储、报告版本与接口

**分支**：`claude/035-pr1-opdiag-reports`，从 `app-main` 起。
**覆盖**：FR-001～FR-005、FR-010～FR-011、FR-040～FR-046、FR-070～FR-073、FR-080～FR-092；SC-004、SC-005（只有 `scope` 的样例）、SC-009～SC-013；D14-V01、D14-V05（可追溯部分）。
**不含**：任何维度（请求选了维度 → 400 点名 `dimensions`）、判断与建议、采纳、页面。

**文件**：`server/migrations/<N>_content_opdiag_{report_version,work_mark}*.{up,down}.sql`（6 个）；`server/internal/content/feedback-learning/opdiag_{contract,inputs,report,marks,calc}.go` 与测试；`guards_test.go`（登记受控集）；`server/internal/handler/content_opdiag_reports.go` 与测试；`workspace_delete_manifest_test.go`、`workspace_delete.sql` + sqlc 产物；`server/cmd/migrate/main.go`、`server/cmd/server/router.go`（upstream）；`server/cmd/server/content_opdiag_routes_test.go`；`packages/core/content/feedback-learning/opdiag/{contract,contract.test,queries}.ts`；`scripts/content-boundaries.json`（只追加 `adapters` 一条，待批准）。

## Phase 1: 基线

- [ ] T001 本 PR 共 6 个迁移。开发时从当时 `app-main` 最大号之后取号；合入前 rebase 到最新 `app-main` 再改号
- [ ] T002 记下 `scripts/content-boundaries.json` 当前内容；本 PR 只追加 `adapters` 一条 `server/internal/handler/content_opdiag_reports.go`（plan.md「主控决定」第 2 条）；`modules` **一个字不改**
- [ ] T003 读并在 PR 正文写一句结论：`feedback-learning/store.go` 的 `begin()`、`handler/content_roi_records.go` 的 `roiScope` 与适配器、`guards_test.go` 的五条包级守卫（spec Current State §3）、#266 的 `roi_report.go`（报告版本与复算的形状）

## Phase 2: 迁移（6 个，各单条语句）

- [ ] T004 两个建表：`content_opdiag_report_version`、`content_opdiag_work_mark`（contract §1.1、§1.2）；无 `PRIMARY KEY` / `UNIQUE` / `REFERENCES` / `CASCADE`；受控集 `CHECK`（含 `kind`–`verdict` 组合）；down 注释写明「会丢失已生成的诊断报告与人工标注」
- [ ] T005 [P] 四个索引迁移（contract §2 这两张表的行），`CREATE [UNIQUE] INDEX CONCURRENTLY IF NOT EXISTS`，单文件单语句
- [ ] T006 `concurrentIndexCleanups` 追加 4 条，自成一块；建表迁移不登记
- [ ] T007 两表进 `workspace_delete_manifest_test.go` 与 `workspace_delete.sql` 同一 CTE 链；`make sqlc`，产物单独提交、不手改
- [ ] T008 跑迁移测试（见文首固定写法），报出 `TestContentMigrationConstraints`、`TestContentConcurrentIndexRegistration`、`TestEveryConcurrentUpBuildHasCleanup`、`TestConcurrentIndexCleanupsMatchTheirMigrations` 四条的用例名与结果

## Phase 3: 模块（先写测试）

- [ ] T009 [P] 先写：受控集矩阵——`DiagnosisDimensions` 恰好六项、`DiagnosisScopes` 两项、`MarkKinds` 两项、`MarkVerdicts` 五项、`DataOrigins` 恰好一项；越界点名字段；登记进 `guards_test.go` 的 `TestThereIsNoSixthControlledSet`
- [ ] T010 [P] 先写：`profileFieldKeys` 十一项与 `ip-profile/profile.go` 的 JSON 名逐字相同、`profileTextKeys` 是其中八个 `TextField`——读源文件对表，不 import
- [ ] T011 [P] 先写：参数严格解码——`role`、`industry`、任何未知字段 → 400 点名该字段；`brand` 且 `account_ids = []` → 点名 `scope.account_ids`；`account` 且不是恰好一个；窗口结束早于开始；同一维度两次；PR 1 选任何维度 → 400 点名 `dimensions`（FR-004、FR-005、SC-011）
- [ ] T012 [P] 先写：窗口边界——品牌时区 `2026-09-30T23:30+08:00` 在内、`2026-10-01T00:10+08:00` 在外；`published_at` 为空不落窗口
- [ ] T013 [P] 先写：`fingerprint` 对字段顺序无关、对任一字段敏感；`inputs` 反射扫字段名，不含 `value`（配置项）、`redacted_excerpt`、`interpretation`、`evidence_note`、`customer_ref`、`order_ref`、`recorded_by`（SC-010 前半）
- [ ] T014 [P] 先写：`CalculateDiagnosis` 只含 `scope` 时：输入打乱顺序结果逐字节相同；`data_origin = manual_only`；`calc_version = opdiag-calc/1`
- [ ] T015 先写守卫（`opdiag_guards_test.go`）：两张表只插不改且有 `INSERT INTO`、无 `DO UPDATE`；`opdiag_*.go` 不含 `float32` / `float64`；SQL 只出现 `content_opdiag_`、`content_manual_metric`、`content_feedback_excerpt`；`opdiag_*.go` 与 `handler/content_opdiag_*.go` 不引用 `sourceinbox` / `knowledgebase`；参数、结果、`DiagnosisSummary` 类型没有 `score`/`grade`/`rating`/`rank`/`level`/`role`/`industry` 字段；开发诊断模块源文件不出现 `content_opdiag_`（SC-011、SC-013）
- [ ] T016 新建 `opdiag_contract.go`、`opdiag_inputs.go`、`opdiag_calc.go`（骨架），让 T009～T015 变绿

## Phase 4: 存储（先写测试，真实 DB）

- [ ] T017 先写：生成报告 → 版本 1，`params`/`inputs`/`result` 读回逐字节相同；再生成 → 版本 2，参数缺省沿用版本 1；两版并存
- [ ] T018 先写：版本 1 → 追加一条作品标注 → 版本 1 逐字节不变，读时 `inputs_changed.changed = true` 并列出那条；库里没有存这个标记的列（SC-004）
- [ ] T019 先写：复算——每个只含 `scope` 的样例存成版本、读回、用 `inputs` 与 `calc_version` 复算，逐字节相同（SC-005 的 PR 1 部分）
- [ ] T020 先写：版本号并发——两个事务同时生成同一报告的新版本，恰好得到 2 与 3，或唯一索引冲突映射为重试后成功（照 034 PR 4 的做法，PR 正文写明选哪种）
- [ ] T021 先写：作品标注——`pillar` 只许 `tagged`/`untagged`，`consistency` 只许三值；`consistency` 缺 `account_id` → 400；`profile_revision_id` 由服务端写，请求体带了被忽略；最新标注按 `(created_at, mark_id)` 取
- [ ] T022 先写：删除栅栏——工作区删除已提交后，生成报告与写标注都被拒绝且不留半条记录（SC-012）
- [ ] T023 先写：`DiagnosisSummary` 反射扫字段名，只含 contract §8 的允许清单（SC-010 后半）
- [ ] T024 新建 `opdiag_report.go`、`opdiag_marks.go`：`begin()` 复用；写路径 栅栏 → 业务行 → `AuditTx`；`inputs_changed` 读时派生；`DiagnosisSummary`
- [ ] T025 删除工作区后两张表在该工作区行数为 0（进既有删除链用例）

## Phase 5: HTTP（先写测试）

- [ ] T026 先写：**收集顺序**——假适配器记录调用序列：非成员 → 零次任何读取；参数里有别的品牌的账号 → `AccountExists` 之后零次账号数据读取，拒绝体与「不存在」逐字节相同（除 `trace_id`）（FR-081、SC-009、D14-V01）
- [ ] T027 先写：决策顺序（contract §7.1）逐序一条；每个端点一条越权用例
- [ ] T028 先写（工作流第 12 步）：`/reports/{reportId}/versions`（GET、POST）、`/reports/{reportId}/versions/{versionNo}` 各一条穿过真实 router 与中间件、参数 ≠ 上下文的用例，只用 `chi.URLParam`；`versionNo` 非数字 → 与不存在同形
- [ ] T029 新建 `handler/content_opdiag_reports.go`：`opdiagScope`（同 `roiScope`）；六个只读适配器（账号与配置经 `ipprofile.Service`，经营规则与时区经工作区设置，选题卡经 `topicplanning.Store.Get`，作品经 `workeditor.Store.GetWork`，发布记录/审核/交付经 `reviewdelivery.Store` 的三个 List）
- [ ] T030 upstream 提交：`router.go` 挂 `/api/content-operating-diagnosis` 块（本 PR 的七条路由），自成一块；`git diff -w` 只有新增行
- [ ] T031 新建 `cmd/server/content_opdiag_routes_test.go`：本 PR 每个端点的路由存在性用例（FR-088）

## Phase 6: core 契约

- [ ] T032 [P] `packages/core/content/feedback-learning/opdiag/contract.ts`：报告版本头、版本详情（`result` 只到 `scope` 与缺口）、标注的 zod schema，数值 `z.string()`，`status`/`kind` 用 `z.enum` 且有兜底；`contract.test.ts` 每个 schema 一条畸形响应用例（首行 `// @vitest-environment node`）
- [ ] T033 [P] `queries.ts`：列表、详情、生成、标注；query key 含 `wsId`；写后失效，不乐观

## Phase 7: 验证与 PR

- [ ] T034 本地：
  - `(cd server && DATABASE_URL='postgres://none:none@127.0.0.1:1/none?sslmode=disable' go test ./cmd/migrate ./internal/migrations)`
  - `(cd server && go test ./internal/content/feedback-learning/)`
  - `(cd server && go test ./internal/handler/ -run 'ContentOpDiag|WorkspaceDelet')`
  - `(cd server && go test ./cmd/server/ -run 'ContentOpDiag')`
  - `pnpm typecheck --force`、`pnpm check:content-boundaries`、`pnpm check:diagnostics-contract`
  - `pnpm --filter @multica/core exec vitest run content/feedback-learning/opdiag`
- [ ] T035 变异验证各一处，确认对应用例变红后原样还原：收集顺序把 `AccountExists` 挪到读配置之后 → T026；`inputs` 里加进配置项 `value` → T013；生成版本时覆盖旧版本（`DO UPDATE`）→ T015、T018；`inputs_changed` 落一列 → T018
- [ ] T036 Draft PR 正文：所属模块、公开契约变更（新增 `DiagnosisSummary`）、迁移号范围、上游改动一节、每个带路径参数端点的第 12 步用例名、本地已跑与未跑的检查（未跑记「未执行」）
- [ ] T037 **远程验收**：请主控跑 `~/loretide-ci/lt-verify.sh claude/035-pr1-opdiag-reports all`；带库套件以其 PASS / SKIP 计数为准

---

# PR 2 —— 六个维度、完整性与补录待办

**分支**：`claude/035-pr2-opdiag-dimensions`，依赖 PR 1 合入。
**覆盖**：FR-012～FR-015、FR-020～FR-033；SC-001～SC-003、SC-005（全部样例）；D14-V04 全部。Q6（ROI 引用）若 #266 已合入也在本 PR。
**迁移**：零个。

**文件**：`opdiag_dimensions.go`、`opdiag_gaps.go`、`opdiag_calc.go`（接入维度）与测试；`guards_test.go`（登记 `DimensionReasons`、`GapKinds`、`DiagnosisRuleIDs`）；`server/internal/handler/content_opdiag_preview.go` 与测试；`router.go`（upstream）；路由存在性用例；core `contract.ts`（维度结果 schema）；`scripts/content-boundaries.json`（`adapters` 追加 `content_opdiag_preview.go`）。

## Phase 8: 计算器（纯函数，先写）

- [ ] T038 [P] 先写：contract §5.10 的全部固定样例，名称与输入逐字照抄；**`nil-vs-zero`、`two-platforms`、`cadence-unset-vs-zero`、`change` 四条是验收门槛**（SC-001、SC-002）
- [ ] T039 [P] 先写：性质用例——固定种子 200 组随机输入，打乱数组顺序后 `result` JSON 逐字节相同（FR-010）
- [ ] T040 [P] 先写：`stat_window` 规范化（NFC、去首尾空白）后比较；两种写法 → `stat_window_mixed` 与按字典序的写法清单
- [ ] T041 [P] 先写：节奏——ISO 周按参数时区；不完整周 `complete = false` 且无 `met`；账号平台不在四个渠道 → `no_delivery_channel`；`limits` 恒含 `cadence.target_is_brand_channel_level`
- [ ] T042 [P] 先写：执行流程——`waiting_days` 按参数时区整日；`due` 只取适配器给的值（假适配器给 `true` 而计划时间在未来，也计入，证明本卡不重算）；观察时点缺省 → `missing_observation_window`；源文件里没有天数字面量（按 027 `TestThePendingDerivationReadsItsWindowFromSettings` 的写法）
- [ ] T043 [P] 先写：一致性与覆盖——`marks_on_older_revision` 计数与局限；最新标注是 `untagged` 时不计入该支柱；标注的 `account_id` 不是本节账号 → `unchecked`
- [ ] T044 [P] 先写：受众反馈——标签只做 NFC 与去首尾空白（「价格」与「 价格 」合一，「Price」与「price」不合）；无摘录 → `no_data`
- [ ] T045 [P] 先写：缺口——`gap_key` 确定性；同一缺口在两个维度出现只进总体清单一次；每种 `GapKinds` 至少一条样例
- [ ] T046 [P] 先写：引用键——`result.refs` 覆盖 contract §5.8 的每种形状；与结果里的事实一一对应
- [ ] T047 [P] 先写：不下结论——`DiagnosisRuleIDs` 的键名与 Go 源文件里的字符串字面量不含 `growth`/`caused`/`because`/增长/提升/下降/导致/带来/因为；表现维度 `limits` 恒含 `performance.difference_is_not_cause`（SC-003 服务端部分）
- [ ] T048 [P] 先写：品牌汇总的节——账号节按 `display_name`、`account_id` 排序；`unknown_account` 只在有归不到账号的记录时出现；`brand` 节只含节奏与执行流程
- [ ] T049 新建 `opdiag_dimensions.go`、`opdiag_gaps.go`，接入 `opdiag_calc.go`；同时守住 027 的包级守卫：SQL 不聚合，表现维度不点名 read 与 play
- [ ] T050 （Q6=A 且 #266 已合入时）先写：带 `roi_report_ref` 生成 → `inputs` 里有 `roi_summary` 整份副本，`result.roi_reference` 原样呈现；不存在的报告版本 → 与不存在同形；再实现

## Phase 9: 预览与收尾

- [ ] T051 先写：`POST /preview` 返回与直接调计算器相同的结果；调用前后所有 `content_opdiag_` 表行数不变；越权同形拒绝；T026 的收集顺序同样成立
- [ ] T052 新建 `handler/content_opdiag_preview.go`；放开 PR 1 的「选了维度 → 400」；upstream 提交挂路由；路由存在性用例
- [ ] T053 [P] 复算：T038 的每个固定样例存成报告版本、从库读回、复算逐字节相同（SC-005）；PR 1 里生成的只含 `scope` 的版本在本 PR 代码下复算仍相同（FR-011）
- [ ] T054 [P] core：维度结果 schema（数值全是字符串，`status`/`reason`/`kind` 用 `z.enum` 有兜底）与畸形响应用例
- [ ] T055 变异验证：nil 当 0 → T038 `nil-vs-zero`；两平台合并成一组 → T038 `two-platforms`；目标缺省当 0 → T038 `cadence-unset-vs-zero`；取最早一次采样 → T038 `latest-sample`；按数值排序账号节 → T048
- [ ] T056 本地验证同 T034；Draft PR 正文列出 contract §5.10 每个固定样例对应的用例名
- [ ] T057 **远程验收**：`~/loretide-ci/lt-verify.sh claude/035-pr2-opdiag-dimensions all`

---

# PR 3 —— 人写判断与建议、采纳与拒绝

**分支**：`claude/035-pr3-opdiag-decisions`，依赖 PR 2 合入。
**覆盖**：FR-050～FR-069、FR-032（加入待办）；SC-006～SC-008、SC-014（服务端链路）；D14-V05 全部、D14-V08 服务端部分。Q6 若 PR 2 未做，在本 PR 做（T050）。
**可选拆分**：3a（本模块部分，含全部 18 个迁移）/ 3b（建卡与提议确认两条跨模块路径，零迁移），见 plan.md。

**文件**：六张表的迁移（18 个）；`opdiag_annotations.go`、`opdiag_decisions.go` 与测试；`guards_test.go`（登记 `JudgementKinds`、`JudgementBases`、`AuthorKinds`、`SuggestionTargets`、`DecisionKinds`、`AdoptModes`、`EffectOutcomes`、`EffectFailures`、`ProposalStates`、`TodoStates`、`TodoOrigins`）；`server/internal/handler/content_opdiag_decisions.go` 与测试；删除清单与删除链、sqlc；`main.go`、`router.go`（upstream）；路由存在性用例；core `contract.ts`、`queries.ts`；`scripts/content-boundaries.json`（`adapters` 追加 `content_opdiag_decisions.go`）。

## Phase 10: 迁移（18 个）

- [ ] T058 六个建表：`content_opdiag_judgement_revision`、`_suggestion_revision`、`_decision`、`_effect`、`_profile_proposal_revision`、`_todo_revision`（contract §1.3～§1.8）
- [ ] T059 [P] 十二个索引迁移；`concurrentIndexCleanups` 追加 12 条；六表进删除清单与删除链、`make sqlc`
- [ ] T060 跑迁移测试（文首固定写法），报出四条迁移规则用例的名字与结果

## Phase 11: 判断与建议（先写）

- [ ] T061 [P] 先写：受控集矩阵（本 PR 十一个集合，`AuthorKinds` 恰好 `human`、`SuggestionTargets` 恰好三项且没有 memory 类取值）
- [ ] T062 [P] 先写：`evidence` 且 `evidence_refs` 空 → 400；`qualitative` 且非空 → 400；引用不在该版本 `result.refs` → 400；`judgement_ids` 引用别的版本的判断 → 400；`about_judgement_id` 只许 `alternative_explanation` 使用
- [ ] T063 [P] 先写：修订制——`base_revision` 不符 409；作废是新修订；旧修订逐字节不变
- [ ] T064 [P] 先写：`target` 校验——`profile_proposal` 的 `field` 只许八个文本项，`persona_prompt`/`primary_channels`/`weekly_hours`/`style_samples` → 400 点名字段；重复字段 → 400（FR-067）

## Phase 12: 决定与效果（先写，真实 DB）

- [ ] T065 先写：**拒绝**——只多一条决定与一条审计；其余五张 PR 3 表与两张 PR 1 表行数不变；假写适配器零次调用（SC-006）
- [ ] T066 先写：**采纳为待办**——一个事务三行（决定、待办修订 1、效果 `done`）；审计失败整体回滚、三行都不在
- [ ] T067 先写：**采纳为提议**——一个事务三行；`base_revision_id` = 适配器读到的当前版本；`ip-profile` 版本数不变（SC-007）
- [ ] T068 先写：**采纳为选题卡 `create`**——顺序：决定提交 → 建卡 → 效果 `done` 带卡 id；卡是 `draft`，`ip_fit` = 建议正文，账号 = `target.account_id`；没有调用任何启动、审核或发布的接口
- [ ] T069 先写：建卡失败（假适配器返回错误）→ 决定在、效果 `failed` 带 `failure_code`；重试成功 → 第二条效果 `done`；已 `done` 再重试 → 409 `decision_id`（SC-008）
- [ ] T070 先写：测试钩子在建卡成功后、记效果前注入失败 → 选题卡存在、没有 `done` 效果；读 annotations 时该决定显示「已采纳，结果未记录」；用 `link` 重试指向那张卡 → 效果 `done`（Q3 已知边界）
- [ ] T071 先写：**采纳为选题卡 `link`**——卡不存在或账号不符 → 与不存在同形；成功时决定与效果同一事务
- [ ] T072 先写：一个修订一个决定——并发两次提交，恰好一个成功，另一个 409 `suggestion_id`（唯一索引那一层也要被触发一次：用测试钩子让两者都越过读检查）
- [ ] T073 先写：**提议确认**——当前版本 = 基础版本 → `ip-profile` 版本 +1，新版本十一项中未修改的十项与旧版本逐项相同、修改项 `status = confirmed`；提议 `confirmed` 带 `applied_revision_id`；当前版本 ≠ 基础版本 → 409 `base_revision_id` 且 `ip-profile` 版本数不变；放弃 → `dismissed`，不写 `ip-profile`
- [ ] T074 先写：**加入待办**——`data_gap` 待办的 `gap_key` 必须在该版本缺口清单里；重复加入 → 409 `origin_gap_key`；待办状态修订
- [ ] T075 先写守卫：六张表只插不改且有 `INSERT INTO`；拒绝路径的函数体不调用 `DiagTopicWriter` / `DiagProfileWriter` 的任何方法（按函数名扫）；迁移里没有经营记忆或 AI 判断表；模块里没有写经营记忆的路径（FR-068）
- [ ] T076 新建 `opdiag_annotations.go`、`opdiag_decisions.go`：写接口 `DiagTopicWriter`、`DiagProfileWriter` 定义在模块里

## Phase 13: HTTP 与链路

- [ ] T077 先写（工作流第 12 步）：`/reports/{reportId}/versions/{versionNo}/annotations|judgements|suggestions`、`/judgements/{judgementId}/revisions`、`/suggestions/{suggestionId}/revisions|decisions`、`/decisions/{decisionId}/retry`、`/profile-proposals/{proposalId}/confirm|dismiss`、`/todos/{todoId}/revisions` 各一条穿过真实中间件、参数 ≠ 上下文的用例
- [ ] T078 先写：每个端点一条越权用例；决策顺序（contract §7.1）第 7～9 序各一条
- [ ] T079 先写（真实库，handler 包）：**D14-V08 服务端链路**——建选题卡 → 建作品 → 登记发布记录 → 录一条指标与一条摘录 → 生成诊断（选表现与受众反馈）→ `scope` 与表现维度包含这条发布记录 → 在版本上写一条建议（`topic_card`）→ 采纳 `create` → 选题卡列表多一张 `draft`（SC-014）
- [ ] T080 新建 `handler/content_opdiag_decisions.go`：`topicWriter`（`topicplanning.Store.Create` / `Get`）、`profileWriter`（`ipprofile.Service.CurrentPersonaRevision` + `SetProfile`）；upstream 提交挂路由；路由存在性用例
- [ ] T081 [P] core：判断、建议、决定、效果、提议、待办的 schema 与畸形响应用例；`queries.ts` 的 mutation（非乐观，成功后失效报告版本、annotations、选题卡列表）
- [ ] T082 变异验证：拒绝路径里调一次 `topicWriter.Create` → T065、T075；提议采纳时直接 `SetProfile` → T067；建卡失败不写效果 → T069；去掉 `base_revision_id` 比较 → T073；待办去掉 `gap_key` 重复检查 → T074
- [ ] T083 本地验证同 T034
- [ ] T084 **远程验收**：`~/loretide-ci/lt-verify.sh claude/035-pr3-opdiag-decisions all`；T072、T079 等带库用例以其 PASS / SKIP 计数为准

---

# PR 4 —— 页面

**分支**：`claude/035-pr4-opdiag-pages`，依赖 PR 1～3 合入。
**覆盖**：FR-092～FR-096，以及前三个 PR 各项在界面上的呈现；SC-003 的文案部分；SC-014 的浏览器部分（记「未执行」）。**不写 UI 单测，不用 computer use。**

**文件**：`packages/core/paths/{paths,route-icons}.ts`、`packages/views/layout/route-icon-components.tsx`、`packages/core/diagnostics/diagnostic-context.ts`（upstream）；`packages/core/content/feedback-learning/opdiag/{display,display.test}.ts`；`packages/views/content/feedback-learning/opdiag/*.tsx`；`packages/views/locales/{en,zh-Hans,ja,ko}/*.json`；`apps/web/app/[workspaceSlug]/(dashboard)/operating-diagnosis/page.tsx`；`scripts/content-boundaries.json`（`adapters` 追加页面）；`specs/035-brand-diagnosis/manual-ui-todo.md`。

- [ ] T085 upstream 提交：`paths.ts` 加 `operatingDiagnosis`；`route-icons.ts` 加 `operatingDiagnosis`（`RouteIconName` 加 `Stethoscope`）；`route-icon-components.tsx` 登记 `Stethoscope`；`diagnostic-context.ts` 加 `["operating-diagnosis"]`；跑 `route-icons.test.ts`、`diagnostic-context.test.ts`
- [ ] T086 [P] node 测试先行：`opdiag/display.ts`——按 `status`、`reason`、`kind`、规则 id 选 i18n 键；`display` 字符串原样透传，不解析数字；未知值走 `default` 分支显示通用「不可计算」/「未知」
- [ ] T087 [P] node 测试先行（不是 UI 单测，读 JSON）：四语言里 `operating_diagnosis` 命名空间的译文不含「增长 / 提升 / 下降 / 导致 / 带来 / 因为」与 `growth` / `caused` / `because`，只放行 `performance.difference_is_not_cause` 这一条否定句（SC-003 文案部分）；导航名与开发诊断的不同
- [ ] T088 `apps/web/.../operating-diagnosis/page.tsx` 适配器；`adapters` 追加该路径
- [ ] T089 「生成诊断」区块：范围（账号 / 品牌汇总，品牌汇总逐个勾账号）、窗口与对比窗口（旁边显示品牌时区）、六个维度勾选与各自参数、可选 ROI 引用、预览与生成
- [ ] T090 「报告」区块：范围、各节各维度（表现变化按平台分块，无跨平台合计与排名）、完整性计数与补录待办（每条带去哪里补的链接与「加入待办」）、版本列表、「输入已有更新」提示、AI 判断占位
- [ ] T091 「作品标注」区块：窗口内作品列表，逐条标支柱（多选）与一致性检查项（三值）
- [ ] T092 「建议与决定」区块：判断、替代解释、局限（「定性判断：没有数据支持」标识；引用从报告里点选）；建议（三种目标）；采纳（选题卡可选「新建」/「关联已有」）、拒绝；效果与重试；「已采纳，结果未记录」的补登
- [ ] T093 「待办与提议」区块：待办列表与状态；提议的「当前值 → 提议值」逐项对照、确认、放弃；409 时提示「账号配置已被修改，请刷新后再核对」
- [ ] T094 四语言（en / zh-Hans / ja / ko），`locales/parity.test.ts` 通过
- [ ] T095 只用既有 Multica 组件与 `--text-*` 字号；不设颜色；「不可计算」「未设定」「未知」「账号未知」用次要文字色 token；不用红黄绿表示好坏（FR-095）
- [ ] T096 `specs/035-brand-diagnosis/manual-ui-todo.md` 核对并在 PR 正文列出；全部状态「未执行」
- [ ] T097 本地：`pnpm typecheck --force`、`pnpm check:content-boundaries`、`pnpm check:diagnostics-contract`、`pnpm --filter @multica/core exec vitest run content/feedback-learning/opdiag paths diagnostics`、`packages/views` 的 `rich-content/package-exports.test.ts` 与 `locales/parity.test.ts`；后端没改动时迁移测试可不跑，改了就按文首固定写法跑
- [ ] T098 **远程验收**：`~/loretide-ci/lt-verify.sh claude/035-pr4-opdiag-pages all`；界面由用户按 `manual-ui-todo.md` 手验

---

# 收尾

- [ ] T099 每个 PR 合入后，主控在文档仓库回写 BO-02 的进度与 D14-V01/04/05/08 的覆盖情况；D14-V08 记「部分：服务端链路已自动化（T079）；浏览器闭环未执行；真实联网不适用（本版无联网）」
- [ ] T100 登记后续卡：AI 判断层（D1，挂接键与允许清单见 contract §8）；经营记忆的人工采纳（D3）；待办接入今日工作台；账号级授权记录（Q5）；`SetProfile` 加 `base_revision`（Q8）；027 `PendingRegistrations` 改走 `review-delivery` 公开接口；`feedback-learning` 包注释更新
