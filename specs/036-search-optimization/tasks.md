---
description: "Task list for 036 topic-planning + work-editor + feedback-learning — platform search optimization (BO-05 / R-060)"
---

# Tasks: 平台搜索优化（036）

**Prerequisites**: `spec.md`、`plan.md`、`contracts/search-optimization.md`。**待裁决**：Q1～Q8 见 spec 文末；下列任务按推荐值写，裁定不同时由主控在派单里指出要改的任务。

**改动文件必须在 plan.md → Project Structure 清单内。**

**四个 PR，按顺序**（PR 3 只依赖 PR 1，可与 PR 2 并行）。每个 PR 是一张可以单独审的卡：列出文件、测试、覆盖的验收编号、本地验证命令，以及**远程验收**（主控在构建服务器上跑 `~/loretide-ci/lt-verify.sh <branch> all`）。

`[P]` 表示与相邻任务无依赖、可并行。「先写」表示该测试先写并确认失败。

**迁移编号**：每个 PR 在合入前把自己的迁移改号为紧接当时 `app-main` 最大号之后的连续号（文件名与 `concurrentIndexCleanups` 的键一起改）；撰写时最大号 584。

**本地迁移测试的固定写法**（每张卡都用这一条，**不许省掉前缀**：上游 migrate 测试不设 `DATABASE_URL` 时默认连 `localhost:5432/multica`）：

```bash
(cd server && DATABASE_URL='postgres://none:none@127.0.0.1:1/none?sslmode=disable' go test ./cmd/migrate ./internal/migrations)
```

**本地不连任何数据库**。带库的用例（真实 DB）在本地会 SKIP，以远程验收的 PASS / SKIP 计数为准。

---

# PR 1 —— 搜索主题存储与接口（`topic-planning`）

**分支**：`claude/036-pr1-search-themes`，从 `app-main` 起。
**覆盖**：FR-001～FR-005、FR-010～FR-019、FR-080、FR-084、FR-100～FR-108（主题部分）；SC-001、SC-002、SC-012（主题部分）、SC-013（主题部分）；D14-V09 全部。
**不含**：建议、采用、指标、观察、页面。

**文件**：`server/migrations/<N>_content_search_theme_revision*.{up,down}.sql`（3 个）；`server/internal/content/topic-planning/search_{contract,theme}.go` 与 `search_{contract,theme,guards}_test.go`、`search_theme_integration_test.go`；`server/internal/handler/content_search_themes.go` 与测试；`workspace_delete_manifest_test.go`、`workspace_delete.sql` + sqlc 产物；`server/cmd/migrate/main.go`、`server/cmd/server/router.go`（upstream）；`server/cmd/server/content_search_routes_test.go`；`packages/core/content/topic-planning/search/{contract,contract.test,queries}.ts`；`scripts/content-boundaries.json`（只追加 `adapters` 一条，待批准）。

## Phase 1: 基线

- [ ] T001 本 PR 共 3 个迁移。开发时从当时 `app-main` 最大号之后取号；合入前 rebase 到最新 `app-main` 再改号
- [ ] T002 记下 `scripts/content-boundaries.json` 当前内容；本 PR 只追加 `adapters` 一条 `server/internal/handler/content_search_themes.go`；`modules` **一个字不改**
- [ ] T003 读并在 PR 正文写一句结论：`topic-planning/store.go` 的 `begin()` 与 `Guard`、`sources.go` 的 `SourceReader`、`handler/content_topic.go` 的 `topicScope` 与 `topicSourceReader`、033 `marketing_node_store.go` 的修订写法

## Phase 2: 迁移（3 个，各单条语句）

- [ ] T004 建表 `content_search_theme_revision`（contract §1.1）；无 `PRIMARY KEY` / `UNIQUE` / `REFERENCES` / `CASCADE`；`platform`、`intent`、`origin` 的 `CHECK`；**没有** `search_volume` / `competition` / `rank` 列；down 注释写明「会丢失全部搜索主题」
- [ ] T005 [P] 两个索引迁移（contract §2 前两行），`CREATE [UNIQUE] INDEX CONCURRENTLY IF NOT EXISTS`，单文件单语句；`concurrentIndexCleanups` 追加 2 条，自成一块
- [ ] T006 表进 `workspace_delete_manifest_test.go` 与 `workspace_delete.sql` 同一 CTE 链；`make sqlc`，产物单独提交、不手改
- [ ] T007 跑迁移测试（文首固定写法），报出 `TestContentMigrationConstraints`、`TestContentConcurrentIndexRegistration`、`TestEveryConcurrentUpBuildHasCleanup`、`TestConcurrentIndexCleanupsMatchTheirMigrations` 的用例名与结果

## Phase 3: 模块（先写测试）

- [ ] T008 [P] 先写：受控集矩阵——`SearchIntents` 恰好 6、`ThemeOrigins` 恰好 3、`DataOrigins` 恰好 1、`UnknownReasons` 恰好 1；越界 400 点名字段；平台集与 `ipprofile.Platforms` 相同（直接引用，`topic-planning` 可以 import `ip-profile`）
- [ ] T009 [P] 先写：严格解码——`search_volume`、`competition`、`rank`、`scope`、`budget`、`online` 与任意未知字段各一条 400 点名该字段；服务端写的字段（`recorded_by`、`data_origin`）接受并丢弃（FR-002、FR-014、SC-002）
- [ ] T010 [P] 先写：字段规则——问题与关键词都空 → `questions`；`customer_question` 缺 `origin_note`；`authorized_material` 缺 `source_ids`；长度上限各一条；关键词 NFC + 去空白 + 去重保序（「羊绒 」与「羊绒」合一，「Cashmere」与「cashmere」不合）（FR-012、FR-017）
- [ ] T011 [P] 先写：读时派生——任一主题 `search_volume`、`competition` 为 `{unknown, no_data_source}`，`data_origin = manual_only`；反射扫主题存储类型无 `SearchVolume`、`Competition`、`Rank`、`Score` 字段
- [ ] T012 先写守卫（`search_guards_test.go`）：主题表只插不改且有 `INSERT INTO`、无 `DO UPDATE`；`search_*.go` 不 import `net/http` / `agent-workflow` / `agent-gateway` / `work-editor` / `feedback-learning`；Go 字符串字面量无排名承诺词（FR-081）
- [ ] T013 新建 `search_contract.go`（主题部分）让 T008～T012 变绿

## Phase 4: 存储（先写测试，真实 DB）

- [ ] T014 先写：建主题 → 修订 1；改关键词（带 `base_revision = 1`）→ 修订 2，修订 1 逐字节不变；`base_revision` 不符 → 409；归档 → `voided` 修订 3；列表默认不含已归档，`include_archived=true` 含（FR-010、FR-016、SC-001）
- [ ] T015 先写：引用核实——素材经假 `SourceReader`；选题卡与简报在本模块读；别的工作区的素材 / 选题卡 / 简报与不存在的，拒绝体逐字节相同（FR-013）
- [ ] T016 先写：账号——不存在 → 400 `account_id`；账号平台 ≠ `platform` → 400 `platform`
- [ ] T017 先写：修订号并发——两个事务同时写同一主题的新修订，恰好一个成功，另一个 409（唯一索引那一层也要被触发一次）
- [ ] T018 先写：列表过滤——`platform`、`account_id`、`topic_card_id`；排序 `name`、`theme_id`
- [ ] T019 先写：删除栅栏——工作区删除已提交后，建主题与写修订都被拒绝（404，不是 503）且不留半条记录（SC-012）
- [ ] T020 新建 `search_theme.go`：`begin()` 复用；写路径 栅栏 → 业务行 → `AuditTx`

## Phase 5: HTTP（先写测试）

- [ ] T021 先写：决策顺序（contract §7.1）逐序一条；每个端点一条越权用例（非成员 → 与不存在逐字节相同，除 `trace_id`）
- [ ] T022 先写：`/themes/{themeId}`、`/themes/{themeId}/revisions`（GET、POST）各一条穿过真实 router 与中间件、路径参数 ≠ 上下文的用例，只用 `chi.URLParam`
- [ ] T023 先写：handler 里用到的账号读与素材读，各一条「工作区删除后 → 404」用例（FR-104）
- [ ] T024 新建 `handler/content_search_themes.go`：`searchScope`（同 `topicScope`）；`searchThemesReadError` 映射（照 `opdiagReadError`）
- [ ] T025 upstream 提交：`router.go` 挂 `/api/content-search` 块（本 PR 五条路由），自成一块；`git diff -w` 只有新增行
- [ ] T026 新建 `cmd/server/content_search_routes_test.go`：本 PR 每个端点的路由存在性用例（FR-107）

## Phase 6: core 契约

- [ ] T027 [P] `packages/core/content/topic-planning/search/contract.ts`：主题与修订的 zod schema，`intent`、`origin`、`status` 用 `z.enum` 且有兜底；`search_volume` / `competition` 是 `{status, reason}` 对象；`contract.test.ts` 每个 schema 一条畸形响应用例（首行 `// @vitest-environment node`）
- [ ] T028 [P] `queries.ts`：列表、详情、修订历史、创建、修订；query key 含 `wsId`；写后失效，不乐观

## Phase 7: 验证与 PR

- [ ] T029 本地：
  - `(cd server && DATABASE_URL='postgres://none:none@127.0.0.1:1/none?sslmode=disable' go test ./cmd/migrate ./internal/migrations)`
  - `(cd server && go test ./internal/content/topic-planning/)`
  - `(cd server && go test ./internal/handler/ -run 'ContentSearch|WorkspaceDelet')`
  - `(cd server && go test ./cmd/server/ -run 'ContentSearch')`
  - `pnpm typecheck --force`、`pnpm check:content-boundaries`、`pnpm check:diagnostics-contract`
  - `pnpm --filter @multica/core exec vitest run content/topic-planning/search`
- [ ] T030 变异验证各一处，确认对应用例变红后原样还原：给请求类型加 `SearchVolume *int64` → T009、T011；写修订时 `UPDATE` 旧行 → T012、T014；别的工作区素材返回不同错误体 → T015；删除后映射成 `ErrStorage` → T019、T023
- [ ] T031 Draft PR 正文：所属模块、公开契约变更（无；新增端点）、迁移号范围、上游改动一节、每个带路径参数端点的真实中间件用例名、本地已跑与未跑的检查（未跑记「未执行」）
- [ ] T032 **远程验收**：请主控跑 `~/loretide-ci/lt-verify.sh claude/036-pr1-search-themes all`；带库套件以其 PASS / SKIP 计数为准

---

# PR 2 —— 建议、比较、采用 / 放弃（`topic-planning` + `work-editor`）

**分支**：`claude/036-pr2-search-suggestions`，依赖 PR 1 合入。
**覆盖**：FR-030～FR-041、FR-050～FR-060、FR-100～FR-108（建议部分）；SC-003～SC-008、SC-011、SC-012（建议部分）；D14-V10 的比较、采用、放弃、不沿用旧终审（旧报告过期为结构性证明）；D14-V15 的「放弃不改任何东西」。
**可选拆分**：2a（建议、比较、放弃；三张表四个索引共 7 个迁移）/ 2b（`ApplyBody`、动作迁移、采用与重试、前端动作文案），见 plan.md。

**文件**：三张表与四个索引的迁移（7 个）；`work-editor` 动作迁移 `<N>_content_artifact_version_action_suggestion_applied.{up,down}.sql`（1 个）；`topic-planning/search_{diff,ports,suggestion}.go`、`search_contract.go`（建议部分）与测试；`work-editor/apply.go`、`contract.go`（`Actions`）、`apply_integration_test.go`；`server/internal/handler/content_search_suggestions.go` 与测试；删除清单与删除链、sqlc；`main.go`、`router.go`（upstream）；路由存在性用例；`packages/core/content/topic-planning/search/{contract,queries}.ts`；`packages/core/content/work-editor/contract.ts` + `contract.test.ts`；`packages/views/content/work-editor/index.tsx`（动作文案一个 `case`）；`packages/views/locales/{en,zh-Hans,ja,ko}/*.json`（动作文案）；`scripts/content-boundaries.json`（`adapters` 追加 `content_search_suggestions.go`）。

## Phase 8: 迁移（8 个）

- [ ] T033 三个建表：`content_search_suggestion_revision`、`_decision`、`_effect`（contract §1.2～§1.4）
- [ ] T034 [P] 四个索引迁移（contract §2 第 3～6 行）；`concurrentIndexCleanups` 追加 4 条；三表进删除清单与删除链、`make sqlc`
- [ ] T035 `work-editor` 动作迁移（contract §1.7）：一条 `ALTER TABLE` 同时 DROP 与 ADD；down 还原四项并在注释写明已有新动作行时 down 会失败
- [ ] T036 跑迁移测试（文首固定写法），报出四条迁移规则用例的名字与结果

## Phase 9: 差异与建议（先写）

- [ ] T037 [P] 先写：`DiffLines` 固定样例（contract §6 八条，名称逐字照抄）；性质用例——固定种子 200 组随机文本，`apply(base, diff) == proposed`、同一输入逐字节相同
- [ ] T038 [P] 先写：受控集矩阵——`SuggestionAspects` 4、`AuthorKinds` 恰好 `human`、`SuggestionDecisions` 2、`EffectOutcomes` 2、`EffectFailures` 5、`SuggestionStates` 5
- [ ] T039 [P] 先写：字段规则——`proposed_body` 与基础版本相同 → 400；`aspects` 空 / 越界 / 重复；`rationale` 空；`target_question` 空或超长；主题已归档 → 400 `theme_id`；修订里改 `work_id` / `artifact_id` / `base_version_id` / `theme_id` → 400 点名该字段（FR-030～FR-034）
- [ ] T040 [P] 先写：状态派生——contract §5 的五种状态各一条；`base_is_current`、`theme_changed` 各真假一条；库里没有存这些的列
- [ ] T041 [P] 先写：比较——2～4 条正常；1 条、5 条 → 400 `ids`；`same_base` 真假各一条；排序按 `created_at`、`suggestion_id`；比较前后所有表行数不变（SC-003）
- [ ] T042 先写守卫（`search_guards_test.go` 追加）：三张新表只插不改且有 `INSERT INTO`；放弃路径的函数体不调用 `SearchWorks.Apply`（按函数名扫）；`topic-planning` 全部 `.go`（含 `_test.go`）不 import `work-editor`；响应类型无 `score` / `rank` / `density` / `keyword_count` / `seo`（SC-011）
- [ ] T043 新建 `search_diff.go`、`search_ports.go`、`search_suggestion.go`（建议与比较部分）

## Phase 10: `work-editor.ApplyBody`（先写，真实 DB）

- [ ] T044 先写：正常路径——新版本 `source = edited`、`action = suggestion_applied`、`revision` +1、正文 = 给定正文、编辑副本 = 新正文且 `saved`；审计一条
- [ ] T045 先写：最新版本 ≠ 基础版本 → `ErrBaseMoved`；`draft_status = working` → `ErrDraftUnsaved`；正文 = 基础版本 → `ErrNoChange`；三者都不写任何行
- [ ] T046 先写：幂等——同键第二次调用（此时最新版本已是第一次写出的那个，≠ 基础版本）→ 返回同一个版本、不再写；同键不同输入 → `idempotency.ErrConflict`；两个事务并发同键 → 恰好一个新版本
- [ ] T047 先写：删除栅栏——工作区删除后 `ApplyBody` 返回 `ErrNotFound`，不留半条
- [ ] T048 核对 `work-editor` 既有守卫 `TestTheVersionTableHasNoUpdateOrDeletePath`、`TestSourceAndActionAreNeverReadFromInput`、`TestNoPathWritesTheGeneratedSource` 覆盖 `apply.go` 且仍为绿；**不放宽任何一条**
- [ ] T049 新建 `work-editor/apply.go`：作为 `appendVersion` 的新意图实现（`versionIntent` 加 `baseVersionID`、`body`），`Claim` 在核对之前；`contract.go` 的 `Actions` 加 `ActionSuggestionApplied`；跑 `work-editor` 全部既有测试

## Phase 11: 采用与放弃（先写，真实 DB）

- [ ] T050 先写：**放弃**——只多一条决定与一条审计；本卡其余表行数不变；作品版本数、编辑副本、选题卡数、简报修订数、主题修订数不变；假 `SearchWorks` 零次 `Apply`（SC-008）
- [ ] T051 先写：**预检查**——基础版本不是最新 → 409 `base_version_id`，决定表行数不变；编辑副本未保存 → 409 `draft_status`，不写；`revision` 不是当前修订 → 409 `revision`（SC-005 前两条）
- [ ] T052 先写：**采用**——顺序：决定提交 → `Apply` → 效果 `done` 带 `version_id`；响应含决定与效果（SC-004 第一条）
- [ ] T053 先写：`Apply` 返回存储错误 → 效果 `failed/storage`；重试成功 → 第二条效果 `done`；已 `done` 再重试 → 409 `decision_id`
- [ ] T054 先写（真实 DB，本卡最重要的一条）：测试钩子在 `Apply` 成功后、写效果前注入失败 → 版本已写、没有效果，状态 `adopt_unrecorded`；**重试 → 凭幂等键拿回同一个版本，文档版本数恰好比采用前多 1**，效果 `done` 指向它；再重试 → 409（SC-004）
- [ ] T055 先写（真实 DB）：同一「版本已写、效果未记」状态下两个重试并发 → 版本数仍恰好多 1，最多一条 `done`（PR 正文写明另一个得到 409 还是同一版本的 `done`）
- [ ] T056 先写：决定之后被抢先保存（假 `SearchWorks.Apply` 返回 `ErrBaseMoved`；真实 DB 版在决定与 `Apply` 之间插一次 `SaveVersion`）→ 效果 `failed/base_moved`（SC-005 第三条）
- [ ] T057 先写：一条建议一个决定——并发两次提交，恰好一个成功，另一个 409 `suggestion_id`（唯一索引那一层也要被触发一次：用测试钩子让两者都越过读检查）
- [ ] T058 先写：已有决定的建议写修订 → 409 `suggestion_id`

## Phase 12: HTTP、链路与前端

- [ ] T059 先写：`/suggestions/{suggestionId}`、`/suggestions/{suggestionId}/revisions`、`/suggestions/{suggestionId}/decisions`、`/decisions/{decisionId}/retry` 各一条穿过真实中间件、参数 ≠ 上下文的用例；`/suggestions/compare` 不被 `{suggestionId}` 吃掉的路由用例
- [ ] T060 先写：每个端点一条越权用例；决策顺序（contract §7.1）第 4～6 序各一条
- [ ] T061 先写：`searchWorks` 的 `Document`、`VersionBody`、`Apply` 三个方法各一条「工作区删除已提交后 → 端点 404，不是 503」（FR-104；035 PR 1 的坑）
- [ ] T062 先写（真实库，handler 包）：**不沿用旧终审**——建作品、存第 3 版、提交审核并批准、建交付任务 → 以第 3 版写建议并采用得第 4 版 → `LatestReviewStatusFor(第 4 版)` 为「没有」；第 3 版的审核请求、交付任务逐字节不变；提交第 4 版得到新的 `pending`（SC-006）
- [ ] T063 先写守卫（`handler/content_search_guards_test.go`）：`content_search_*.go` 不写 `content_review_*` / `content_delivery_*` / `content_publication_*`，不含出站请求（SC-007、SC-013）
- [ ] T064 新建 `handler/content_search_suggestions.go`：`searchWorks{store *workeditor.Store}` 实现 `topicplanning.SearchWorks`（`Document` 用 `GetArtifact` + `ListVersions` 取最新；`VersionBody` 用 `GetVersion`；`Apply` 构造 `idempotency.NewRequest("apply-search-suggestion", artifactID, key, …)` 调 `ApplyBody`），在 `h.topicPlanningSearchStore()` 里注入；upstream 提交挂路由；路由存在性用例
- [ ] T065 [P] core：建议、比较、决定、效果的 schema 与畸形响应用例；`queries.ts` 的 mutation（非乐观，成功后失效建议列表、建议详情、作品版本列表）
- [ ] T066 [P] `work-editor` 前端：`VERSION_ACTIONS` 加 `suggestion_applied`、`contract.test.ts` 加一条；`index.tsx` 动作文案加一个 `case`；四语言各一条文案；`locales/parity.test.ts` 通过
- [ ] T067 变异验证：放弃路径里调一次 `Apply` → T050、T042；`Claim` 挪到核对之后 → T046、T054；预检查挪到写决定之后 → T051；在 `topic-planning` 里 import `work-editor` → T042；删除后映射成 `ErrStorage` → T061；采用时复制旧审核请求 → T062、T063
- [ ] T068 本地验证同 T029，另加 `(cd server && go test ./internal/content/work-editor/)` 与 `pnpm --filter @multica/core exec vitest run content/work-editor`
- [ ] T069 Draft PR 正文另写「公开契约变更：`work-editor` 新增 `ApplyBody` 与动作 `suggestion_applied`；消费者与兼容性」一节（文档 12 §6）
- [ ] T070 **远程验收**：`~/loretide-ci/lt-verify.sh claude/036-pr2-search-suggestions all`；T054、T055、T057、T062 等带库用例以其 PASS / SKIP 计数为准

---

# PR 3 —— 搜索指标与排名观察（`feedback-learning`）

**分支**：`claude/036-pr3-search-observations`，依赖 PR 1 合入（与 PR 2 无依赖）。
**覆盖**：FR-070～FR-078、FR-080～FR-083、FR-100～FR-108（观测部分）；SC-009、SC-010、SC-012（观测部分）、SC-014（服务端链路，需要 PR 2 已合入；若 PR 2 未合入，T082 挪到 PR 4 前的一张小卡）；D14-V10「搜索观察保留窗口与条件」；D2。

**文件**：两张表与五个索引的迁移（7 个）；`feedback-learning/search_{contract,ports,metric,observation}.go` 与测试；`guards_test.go`（登记 `SearchMetrics`、`RankResultKinds`、`SearchMetricSources`）；`server/internal/handler/content_search_observations.go` 与测试；删除清单与删除链、sqlc；`main.go`、`router.go`（upstream）；路由存在性用例；`packages/core/content/feedback-learning/search/{contract,contract.test,queries}.ts`；`scripts/content-boundaries.json`（`adapters` 追加 `content_search_observations.go`）。

## Phase 13: 迁移（7 个）

- [ ] T071 两个建表：`content_search_metric`、`content_search_rank_observation_revision`（contract §1.5、§1.6），含 `result_kind` 与两个整数列的组合 `CHECK`
- [ ] T072 [P] 五个索引迁移（contract §2 后五行）；`concurrentIndexCleanups` 追加 5 条；两表进删除清单与删除链、`make sqlc`
- [ ] T073 跑迁移测试（文首固定写法），报出四条迁移规则用例的名字与结果

## Phase 14: 模块（先写）

- [ ] T074 [P] 先写：受控集矩阵——`SearchMetrics` 恰好 2（不含 `search_rank`、`search_volume`、`other`）、`RankResultKinds` 2、`SearchMetricSources` 1；登记进 `TestThereIsNoSixthControlledSet`
- [ ] T075 [P] 先写：搜索指标——`stat_window` 空 → 400；`value` 为 `null` 与 `0` 各一条往返，读回不同；负数 → 400；发布记录不存在 / 别的品牌 → 与不存在同形；`platform` 越界（如 `zhihu`）→ 400（SC-009、SC-010）
- [ ] T076 [P] 先写：排名观察——`position` / `scanned_depth` 的四种组合（只给对的一个、都不给、都给、给错的一个）；`query` / `conditions` / `evidence_note` 空各一条；`observed_at` 晚于服务器时间 10 分钟以上 → 400；作废是新修订，列表默认不含（FR-073、FR-074）
- [ ] T077 [P] 先写：列表——按 `observed_at` 倒序、`observation_id`；三条观察读回三条；反射扫响应类型无 `avg` / `best` / `current_rank` / `median` / `rank` 汇总字段；每条带 `rule = rank.single_observation`（FR-075、SC-010）
- [ ] T078 先写守卫（`search_guards_test.go`）：两表只插不改且有 `INSERT INTO`；`search_*.go` SQL 无 `SUM(` / `AVG(` / `GROUP BY`、不新增 `count(`（027 守卫同样会挡）；`feedback-learning` 全部 `.go`（含 `_test.go`）不 import `topic-planning`；无出站请求
- [ ] T079 新建 `search_contract.go`、`search_ports.go`、`search_metric.go`、`search_observation.go`：`begin()` 复用；账号核实复用 `Accounts`；`theme_id` 经 `SearchThemes`

## Phase 15: HTTP 与链路

- [ ] T080 先写：`/rank-observations/{observationId}/revisions` 穿过真实中间件、参数 ≠ 上下文的用例；每个端点一条越权用例；`GET /rank-observations` 三个过滤参数都缺 → 400
- [ ] T081 先写：`searchThemes.ThemeExists`、复用的账号核实与发布记录核实，各一条「工作区删除已提交后 → 端点 404」（FR-104）
- [ ] T082 先写（真实库，handler 包；需要 PR 2 已合入）：**D14-V15 服务端链路**——建主题 → 建选题卡与作品、存第 1 版 → 写建议并采用得第 2 版 → 提交第 2 版审核并批准 → 建交付任务、登记发布记录 → 登记一条搜索曝光与一条排名观察（关联主题与发布记录）→ 按主题读观察、按发布记录读指标，发布记录解析到第 2 版（SC-014）
- [ ] T083 新建 `handler/content_search_observations.go`：`searchThemes{store *topicplanning.Store}` 实现 `feedbacklearning.SearchThemes`；复用 `feedbackPublications`、`roiAccounts`；在 `h.feedbackSearchStore()` 里注入；upstream 提交挂路由；路由存在性用例
- [ ] T084 [P] core：指标与观察的 schema（`value` 可为 `null`）与畸形响应用例；`queries.ts`
- [ ] T085 变异验证：nil 当 0 存 → T075；列表加一个 `best_position` 字段 → T077；在 `search_observation.go` 里 import `topic-planning` → T078；删除后映射成 `ErrStorage` → T081；`SearchMetrics` 加 `search_rank` → T074
- [ ] T086 本地验证同 T029（模块换成 `./internal/content/feedback-learning/`，vitest 换成 `content/feedback-learning/search`）
- [ ] T087 **远程验收**：`~/loretide-ci/lt-verify.sh claude/036-pr3-search-observations all`

---

# PR 4 —— 页面

**分支**：`claude/036-pr4-search-pages`，依赖 PR 1～3 合入。
**覆盖**：FR-037、FR-076、FR-082、FR-110～FR-115，以及前三个 PR 各项在界面上的呈现；SC-011 的文案部分；SC-014 的浏览器部分（记「未执行」）。**不写 UI 单测，不用 computer use。**

**文件**：`packages/core/paths/{paths,route-icons}.ts`、`packages/views/layout/route-icon-components.tsx`、`packages/core/diagnostics/diagnostic-context.ts`（upstream）；`packages/core/content/{topic-planning,feedback-learning}/search/{display,display.test}.ts`；`packages/views/content/topic-planning/search/*.tsx`、`packages/views/content/feedback-learning/search/*.tsx`；`packages/views/content/work-editor/index.tsx`（一个链接）；`packages/views/locales/{en,zh-Hans,ja,ko}/*.json`；`apps/web/app/[workspaceSlug]/(dashboard)/search-optimization/page.tsx`；`scripts/content-boundaries.json`（`adapters` 追加页面）；`specs/036-search-optimization/manual-ui-todo.md`。

- [ ] T088 先核实图标：在 `packages/views` 依赖的 lucide 版本里确认 `SearchCheck` 导出存在；不存在就换一个项目里已引入、且不与既有导航重复的图标，在 PR 正文写明。然后 upstream 提交：`paths.ts` 加 `searchOptimization`；`route-icons.ts` 加 `searchOptimization`；`route-icon-components.tsx` 登记图标；`diagnostic-context.ts` 加 `["search-optimization"]`；跑 `route-icons.test.ts`、`diagnostic-context.test.ts`
- [ ] T089 [P] node 测试先行：两个 `display.ts`——按 `state`、`reason`、`intent`、`origin`、`result_kind`、规则 id 选 i18n 键；未知值走兜底显示通用「未知」；搜索指标「未登记 = 未知」、nil =「未知」、0 = `0` 三种各一条
- [ ] T090 [P] node 测试先行（读 JSON，不是 UI 单测）：四语言 `search_optimization` 命名空间无排名承诺词（「保证排名」「上首页」「提升排名」「排名第一」、`guarantee`、`boost ranking`、`top rank`），只放行 `optimization.no_ranking_promise` 这一条否定句（SC-011 文案部分）
- [ ] T091 `apps/web/.../search-optimization/page.tsx` 适配器：组合两个模块的视图，传入作品 / 文档 / 版本列表；`adapters` 追加该路径
- [ ] T092 「搜索主题」区块：列表与筛选、表单（问题与关键词多行输入、意图、来源与说明、素材 / 选题卡 / 简报选择）、修订历史、「搜索量 / 竞争度：未知（本版没有搜索数据接口）」、排名一栏（观察列表或「未知（还没有观察记录）」）
- [ ] T093 「优化建议」区块：按作品与文档筛选；写建议（选主题与问题、勾涉及项、写依据、编辑整篇正文，旁边显示基础版本）；差异显示（FR-113）；勾选 2～4 条比较；采用 / 放弃（带说明）；「已采用，写版本失败 / 结果未记录」与重试；「基础版本已不是最新」时采用不可用；固定提示三句（FR-037）
- [ ] T094 「搜索表现」区块：按发布记录登记与查看搜索指标（未登记显示「未知」）；登记与作废排名观察；每条观察旁「单次观察，不代表稳定排名或全平台排名」；主题平台不在四个渠道时显示「该平台没有可登记的发布渠道」
- [ ] T095 作品编辑器页加「搜索优化」链接（带 `work_id`、`artifact_id`）；页面上没有 AI / 联网 / 一键优化入口，只有一行说明（FR-115）
- [ ] T096 四语言（en / zh-Hans / ja / ko），`locales/parity.test.ts` 通过；只用既有 Multica 组件与 `--text-*` 字号；「未知」用次要文字色，不用红黄绿（FR-112）
- [ ] T097 `specs/036-search-optimization/manual-ui-todo.md` 核对并在 PR 正文列出；全部状态「未执行」
- [ ] T098 本地：`pnpm typecheck --force`、`pnpm check:content-boundaries`、`pnpm check:diagnostics-contract`、`pnpm --filter @multica/core exec vitest run content/topic-planning/search content/feedback-learning/search paths diagnostics`、`packages/views` 的 `rich-content/package-exports.test.ts` 与 `locales/parity.test.ts`；后端没改动时迁移测试可不跑，改了就按文首固定写法跑
- [ ] T099 **远程验收**：`~/loretide-ci/lt-verify.sh claude/036-pr4-search-pages all`；界面由用户按 `manual-ui-todo.md` 手验

---

# 收尾

- [ ] T100 每个 PR 合入后，主控在文档仓库回写 BO-05 的进度与 D14-V09 / V10 / V15 的覆盖情况；D14-V10 记「旧预检报告过期：结构性证明（新 `version_id`、正文变化、合同 §8 约定），EP-06 未实现，报告本身不可测」；D14-V15 记「部分：服务端链路已自动化（T082）；浏览器闭环未执行；AI 分析与联网不适用」
- [ ] T101 登记后续卡：AI 生成建议；联网研究（范围、来源、预算）；搜索量 / 竞争度数据接口；搜索指标 CSV 导入；搜索指标接入 035 诊断；`HoldForChangedTarget` 没有调用方；`feedbackPublications` 把存储错误映射成 404；`idempotency` 包注释更新
