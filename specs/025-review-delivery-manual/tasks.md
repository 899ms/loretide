---
description: "Task list for 025 review-delivery — manual review requests, delivery tasks and publication records"
---

# Tasks: 人工审核请求、交付任务与发布记录（025）

**Prerequisites**: `spec.md`、`plan.md`、`contracts/review-delivery.md`

**改动文件必须在 plan.md → Project Structure 清单内。**

**五条 clarify 已裁决**（主控 2026-09-21，全文见 PR #144 评论；SOP §8 / §9.1 / §9.2 / §9.3 原文抄在 `spec.md` 的「SOP 原文」一节）：

- **Q1=A**：审核请求行上 `snapshot jsonb`，**键恰好八个** `channel` / `work_id` / `artifact_id` / `version_id` / `account_id` / `start_snapshot_id` / `attachments`（**恒空**）/ `delivery_config`（扩展键）；**「已通过」不得自动套在新版本上**；
- **Q2=A**：可选引用；`ready` 及之后要求被引用请求 `approved`。**另按 §9.3 加自动 `held` 规则**；
- **Q3=A**：可变 `status` + 独立 append-only 迁移记录表；**`held` / `cancelled` / 失败 / 延后必带 reason**；
- **Q4=A**：渠道**恰好四个** `xiaohongshu` / `wechat_mp` / `douyin` / `shipinhao`；交接方式**三个** `export` / `copy` / `handed_to_operator`，**三者都不等于发布成功**；**只有渠道稿可以提交审核**；
- **Q5：不发明枚举**。声明者 = 系统 actor + 可选自由文本 `declared_by`；证据 = `page_url_or_content_id` + `receipt_note`；核验 = 自由文本 `verification_note`；另加 `published_at` / `platform_account` / `platform_edited` + `edit_note` / `version_match` 三态。**「待登记」是派生显示，不是存储状态。**

**四张表、八个索引、十二个迁移（表数定稿，裁决不再改动它）。**

**两个 PR**：T001–T029 是存储与接口 PR；T030–T035 是页面 PR；T036–T039 是收尾。

---

## Phase 1: Setup

- [x] T001 裁决已拿到并回写四个文件：`spec.md` 新增「SOP 原文」与「裁决记录」两节、`plan.md`、`contracts/review-delivery.md`、本文件。**Q5 的两个枚举按裁决删除，退成自由文本**——SOP 没给枚举，本卡不发明
- [ ] T002 读 **#141 合入后的 work-editor 实际形状**：`content_artifact_version` 的稳定键列名、`content_artifact.kind` 的取值、`content_work` 的列。本卡只存字符串 id，但**列名与取值要对上实际的树**，不是对上 024 的提案
- [ ] T003 记录基线：`bash scripts/test-go.sh`、两套 db-suites（按 `docs/development/testing-database-suites.md` 配 `LORETIDE_DB_TEST_*`，**指向容器自带 PG 上新建的独立库与最小权限角色**）、三项 check、`pnpm typecheck --force`。**同时记下当时 `server/migrations/` 的最大迁移号**——下面的 `N` 从它顺延，不要照抄本文件里的数字

---

## Phase 2: 存储（十二个迁移，各单条语句）

- [ ] T004 `N_content_review_request.{up,down}.sql`：建表。**无 `REFERENCES`/`CASCADE`（R1/R2）、无 `PRIMARY KEY`/`UNIQUE`（R5）、不建索引（R4）**；`status` 与 `channel` 各带受控集 `CHECK`（渠道四值）；`snapshot jsonb NOT NULL`；**`account_id` 是一等列**（§8 算进快照、§9.3 换账号要重审）；`decided_by` / `decision_note` 可为 `''` 并注明**空串是真实状态**（未处置）。`down` 注明已提交的审核请求会丢
- [ ] T005 [P] `N+1_content_review_request_id_unique_idx` `(review_request_id)`；`N+2_content_review_request_workspace_idx` `(workspace_id, artifact_id, created_at DESC)`——**后者前导 `workspace_id`，同时服务「列一个文档的审核历史」与删除链**
- [ ] T006 `N+3_content_review_transition.{up,down}.sql`：建表，含 `subject_kind` 的 `CHECK (subject_kind IN ('review_request','delivery_task'))`；`reason` 可空串但**条件必填由应用校验**（§9.2「注明原因」）。注释写明**为什么两类主体同一张表**（迁移形状完全相同，分表会让「列出这件事的全部动作」要查两次）
- [ ] T007 [P] `N+4_content_review_transition_id_unique_idx` `(transition_id)`；`N+5_content_review_transition_subject_idx` `(workspace_id, subject_kind, subject_id, created_at)`
- [ ] T008 `N+6_content_delivery_task.{up,down}.sql`：建表，`status` 六值 `CHECK`、`handoff_method` **三值** `CHECK`、`channel` 四值 `CHECK`；`review_request_id` / `scheduled_at` / `handoff_method` 均可空（**条件必填由应用代码在状态迁移时校验，不是列约束**，注释写明理由：同一列在不同状态下必填与否不同，列约束表达不了）。**没有 `hold_reason` 列**——原因属于「那一次动作」，写在迁移记录的 `reason` 里（同一个任务可以被 held 两次，理由不同）
- [ ] T009 [P] `N+7_content_delivery_task_id_unique_idx` `(delivery_task_id)`；`N+8_content_delivery_task_workspace_idx` `(workspace_id, status, created_at DESC)`
- [ ] T010 `N+9_content_publication_record.{up,down}.sql`：建表，`status` 五值与 `version_match` **三值** `CHECK`；`actor_id` 非空；`declared_by` / `page_url_or_content_id` / `receipt_note` / `verification_note` / `platform_account` / `edit_note` **都是自由文本且可空串**（裁决 Q5：不发明枚举），条件必填由应用校验（**要点名字段**，`CHECK` 只能给出 23514）
- [ ] T011 [P] `N+10_content_publication_record_id_unique_idx` `(publication_record_id)`；`N+11_content_publication_record_artifact_idx` `(workspace_id, artifact_id, created_at DESC)`——**当前发布状态 = 最新一条**（FR-019）就是走这个索引读出来的
- [ ] T012 四张表进 `workspace_delete_manifest_test.go`（标 `workspaceDelete`）；`workspace_delete.sql` 的同一 CTE 链里加四条按 `workspace_id` 的 `DELETE`；跑 `make sqlc` 并核对产物（**单独提交、不手改**）
- [ ] T013 迁移规则核对，**四条都要跑到**：`go test ./internal/migrations -run 'TestContentMigrationConstraints|TestContentConcurrentIndexRegistration'`、`go test ./cmd/migrate -run 'TestEveryConcurrentUpBuildHasCleanup|TestConcurrentIndexCleanupsMatchTheirMigrations'`。**R5 专门确认四个建表迁移都不含 PK/UNIQUE**；**R6 确认八条登记、四个建表迁移未登记**
- [ ] T013a 核对 **FR-020 的索引面**：四张表各有一个以 `workspace_id` 打头的索引（`N+2` / `N+5` / `N+8` / `N+11`）。不以它打头的话，删除链会在持锁期间扫全表

---

## Phase 3: 模块（先写测试）

- [ ] T014 [P] **先写** `states_test.go` 的状态机矩阵（**纯函数，不连数据库**）：审核请求 1+4 条合法迁移、交付任务的全部合法迁移（`draft→ready|cancelled`、`ready→scheduled|handed_off|held|cancelled`、`scheduled→handed_off|held|cancelled`、`held→ready|cancelled`）各一条；**非法**迁移各一条——终态再推进、跳过 `pending` 直接写终态、`cancelled` 复活、`handed_off` 之后再动。确认失败（SC-003）
- [ ] T015 [P] **先写** 受控集越界矩阵，**恰好五个受控集**（裁决 Q5 之后不多不少）：`channel`（4 值）/ `status`（5 / 6 / 5 三套）/ `handoff_method`（3 值）/ `version_match`（3 值）/ `subject_kind`（2 值）各一条越界 → `ErrInvalid`，**且错误点名字段**。确认失败
- [ ] T015a [P] **先写** 「受控集恰好五个、没有第六个」的负例（SC-018）：模块里**不存在**声明者或核验方式的枚举类型（裁决 Q5：SOP 没给，本卡不发明）。一条检索用例——否则后人会「顺手补全」一个 SOP 没说过的受控集。确认失败
- [ ] T015b [P] **先写** 渠道对表用例（Q4=A / SC-018）：读 `ip-profile` 的 Go 源文件，比对四个渠道值与它的平台清单一致。**不 import**（依赖表里没有 ip-profile）。确认失败
- [ ] T016 [P] **先写** 「发布记录没有状态机」的正例：`verified_published` 之后再录一条 `removed` **必须被接受**。作品被平台删掉是常态，拦住它等于让人无法如实记录。确认失败（合同 §2）
- [ ] T017 [P] **先写** 条件必填矩阵：`scheduled` 缺 `scheduled_at`、`handed_off` 缺 `handoff_method` 各一条；**外加 SC-020 的四条缺原因用例**（交付任务 `held` / `cancelled`、发布记录 `failed` / `removed`）→ 全部 400 且**各自点名**缺的那一项（§9.2「注明原因」）。确认失败
- [ ] T018 [P] **先写** 发布记录条件必填的**三条独立用例**（SC-004，**不合并成一条**）：`reported_published` 缺 `page_url_or_content_id`、`verified_published` 缺 `page_url_or_content_id`、`verified_published` 缺 `verification_note`；三次都被拒且各自点名。确认失败
- [ ] T018a [P] **先写** `version_match` 三态（SC-004a）：`unknown` **可以录入、不被当作失败、不阻断后续录入**（§9.1「无法拿到完整正文时标为 unknown」）；第四个取值被拒并点名。确认失败
- [ ] T018b [P] **先写** `failed` / `removed` 必带原因（FR-017b）：`receipt_note` 为空时被拒并点名。**这一处是裁决之外我就近选的落点**，若改成独立 `reason` 列，改的是本用例与建表迁移各一处
- [ ] T019 [P] **先写** 只插不改的守卫用例（照 022 `TestBriefStoreHasNoUpdateOrDeletePath`）：扫 `content_review_delivery.sql` 与模块源码，出现 `UPDATE CONTENT_PUBLICATION_RECORD` / `UPDATE CONTENT_REVIEW_TRANSITION`，或删除链之外的 `DELETE FROM` 这两张表即红；**且必须有 `INSERT INTO`**（否则守卫在空模块上也绿）。确认失败（SC-005）
- [ ] T020 [P] **先写** 不外发守卫（SC-006，照 `check-diagnostics-no-upload.mjs` 的形状）：模块源码里不存在 `net/http` 客户端调用、不存在 `http.Get` / `http.Post` / `(*http.Client).Do`。确认失败
- [ ] T020a [P] **先写** 不调度的检索用例（SC-010）：全仓检索读取 `scheduled_at` 的代码路径，除本模块的读写、序列化与**读时的到期比较**外**不得有第二处**。确认失败
- [ ] T020b [P] **先写** **三条**守卫（SC-006）：模块源码里不存在平台密钥的读写、不存在任何「执行发布」的端点或方法（§9.2 原文），**也不存在任何模型调用**（§8「AI 的自检报告…不能执行人工通过动作」，FR-006a）。确认失败
- [ ] T020c [P] **先写** 派生显示的矩阵（SC-013 / SC-013a，纯函数）：**「待登记」**＝有 `handed_off` 任务且该文档发布记录数为 0；**「到期待办」**＝`scheduled_at` 已过且尚未 `handed_off`。各两条（出现 / 消失）。另一条检索用例确认**存储层没有任何叫「待登记」或「到期」的状态值或列**。确认失败
- [ ] T020d [P] **先写** §9.3 的自动 `held` 判定（SC-014，纯函数）：任务引用的快照与当前交付目标不一致时判定为「应当 held」；四个触发点（换稿 / 换附件 / 换账号 / 改渠道配置）是**同一次比较的四个输入**，不是四段代码。确认失败
- [ ] T021 新建 `contract.go`：三类实体、**五个**受控集（`channel` / 三套 `status` / `handoff_method` / `version_match` / `subject_kind`）、**八键快照的类型**、`ErrInvalid` / `ErrNotFound` / `ErrStorage`。**不定义声明者与核验方式的枚举**（裁决 Q5）
- [ ] T022 新建 `states.go`：纯函数的合法迁移表与判定、条件必填判定、**派生显示**（待登记 / 到期待办）、**§9.3 的应当-held 判定**。**Go 枚举为准、库 `CHECK` 兜底**（重复一遍是故意的：`CHECK` 给不出「从 X 不能到 Y」这句话）
- [ ] T023 新建 `store.go` 的写路径：**栅栏是事务第一句**（`LockWorkspaceForContentDiagnosticWrite`，#104）；提交审核 / 处置 / 建交付任务 / 推进状态 / 录发布记录。每条改状态的操作**在同一事务内**写 `AuditTx` 与一条 `content_review_transition`（FR-022）。**提交审核时在栅栏内校验 `kind == channel_draft`**（FR-007a）——在事务外校验读到的是旧值
- [ ] T023a **先写** 交付快照不可变用例（FR-004 / SC-001）：提交审核后对同一文档再存 3 个版本、再改文档标题与渠道偏好，**读回 `snapshot` 列逐字节不变**，且绑定版本的内容逐字节不变。确认失败
- [ ] T023a1 **先写** 快照键**恰好八个**的用例（SC-011）：多一个或少一个即红；外加 `attachments` **恒为空数组**的负例——不存在任何往里写东西的路径。**只写「键恰好八个」不写这条负例，等于只挡住了少键**。确认失败
- [ ] T023a2 **先写** 「`approved` 不套新版本」的用例（FR-004c / SC-017）：请求通过后对同一文档存新版本，读新版本的审核状态得到「未提交」而不是「已通过」（§8 原文）。确认失败
- [ ] T023a3 **先写** 非渠道稿被拒的用例（FR-007a / SC-015）：`kind ≠ channel_draft` 提交审核 → 400 且点名 `kind`。确认失败
- [ ] T023b **先写** 重复提交用例（SC-002）：同一文档连续提交两次审核 → 两条 id 不同的请求，**第一条逐字节不变**，第二次**不得**被当作冲突拒绝。确认失败
- [ ] T023c **先写** 「交接 ≠ 发布」的三条用例（FR-010a / SC-012）：三种交接方式**各推一次**，之后该文档的发布记录数仍为 **0**、读路径**不**把它显示为「已发布」。**这是 §9.1 那句「都不自动等于发布成功」的唯一可执行形式。**确认失败
- [ ] T023d **先写** §9.3 的两条真实写入用例（SC-014）：引用第 3 版已审快照的 `scheduled` 任务，在第 5 版被选为交付目标后**自动变 `held`**；明确选择「继续交付第 3 版」后回到原状态，且该选择与原因都在迁移记录里。换附件、换账号各再一条。确认失败
- [ ] T024 **先写** 「删除已提交后写入被拒」的真实 DB 用例（FR-021 / SC-022，#104 口径）：五条写路径各一条，且**不留半条记录**。确认失败
- [ ] T025 `store.go` 的读路径（列出 / 取单条 / 取某文档最新一条发布记录、**派生的「待登记」与「到期待办」标记**，全部按 `workspace_id` 过滤）
- [ ] T026 跑 `pnpm check:content-boundaries` 与 `pnpm check:diagnostics-contract`，确认**落地模块数 +1** 且两项都退出 0（SC-009）

---

## Phase 4: HTTP（先写测试）

- [ ] T027 **先写** 决策顺序用例：**六条**各一例（成员资格 / 主体存在 / 受控集 / 类型与前置条件 / 条件必填 / 迁移合法性）；**1–2 的拒绝体除 `trace_id` 外逐字节相同**（逐字节比对，不是「都是 404」）；3–6 是 400 诊断错误对象且**点名字段**。确认失败
- [ ] T027a **先写** §8 个人模式的合并操作用例（FR-031 / SC-016）：一次「审核通过并安排交接」产生**两条独立记录**与**各自的迁移记录**，不合并成一条。确认失败
- [ ] T027b **先写** 前置条件用例（SC-019）：`draft` 阶段引用 `pending` 请求建任务成功 → 推 `ready` 被拒并说明前置条件 → 请求变 `approved` 后同一次推进成功；三条。确认失败
- [ ] T027c **先写** 声明者不可伪造用例（SC-021）：body 里塞别人的 `actor_id` 不生效，存下来的是调用者。确认失败
- [ ] T028 新建 `content_review.go` 与 `content_delivery.go`：九条端点；400 / 404 映射沿用既有助手，**不自己造 404**；**没有 DELETE 端点，发布记录与迁移记录没有 PATCH**；`actor_id` **一律从会话取，不读 body**（FR-016）
- [ ] T028a `scripts/content-boundaries.json`：新 handler 文件加入 `adapters`，跑 `pnpm check:content-boundaries`
- [ ] T029 **两个上游文件，同一个 `upstream:` 提交**，PR 正文单列「上游改动」一节（第 13 步）：
      (a) `server/cmd/server/router.go` 挂九条路由；
      (b) `server/cmd/migrate/main.go` 的 `concurrentIndexCleanups` 加 **八条**索引，索引名**逐字**对上迁移文件。**四个建表迁移不登记。**
      另按**工作流第 12 步**：`{reviewId}` 与 `{deliveryId}` **各一条**穿过真实中间件、参数值 ≠ 工作区 id 的用例；填别的工作区的 id 要拿到与「不存在」同形的拒绝，**两种拒绝体互相逐字节相同**（SC-007）。**发布记录没有路径参数**，所以没有第三条

---

## Phase 5: core 契约与 hooks（PR 1）

- [ ] T029a [P] `packages/core/content/review-delivery/contract.ts` + `contract.test.ts`（**首行 `// @vitest-environment node`**）：zod schema、`parseWithFallback`、畸形响应降级、**未知状态值保留而不是丢弃**（装了旧客户端的桌面端会遇到新状态）、空列表读成 `[]` 而不是报错
- [ ] T029b [P] `states.ts` + `states.test.ts`：状态机的前端副本，**用例读 Go 源文件比对**（照 `packages/core/content/ip-profile/scope.test.ts`），漂了就红
- [ ] T029c [P] `queries.ts`：三类实体的列表 / 详情 query 与提交、处置、建任务、推进状态、录发布的 mutation。**全部非乐观**——每一条都会改变别人看到的东西，回滚不是本地可预测的

---

## Phase 6: 页面 PR

- [ ] T030 `packages/views/content/review-delivery/`：审核与交付页面，**只挂既有组件**，不新增控件、不改样式
- [ ] T031 审核请求区：被审版本（只读、指向那一版）、渠道、目标账号、交付快照（**只读展示，八个键都看得见**）、四个处置动作与意见输入；已处置的请求**没有任何可改的入口**。提交入口**只对渠道稿出现**，非渠道稿要写明为什么不能提交（§8）
- [ ] T032 交付任务区：状态推进按钮按当前状态启用/禁用（非法迁移**禁用并写明为什么**，不是隐藏）；`scheduled` 的计划时间旁**写明「这是人看的待办时间，系统不会到点自动做任何事」**——不写这句，排了时间的人会以为它会自己发；三种交接方式旁写明**交接完成不等于已发布**（§9.1）
- [ ] T032a §9.3 的界面：任务因快照过期自动 `held` 时，**展示被交付快照的版本号与预览**，并给出两条出路（等新快照审核通过 / 明确选择继续交付旧快照）。「继续交付旧快照」是一个需要确认的动作，不是一个默认值
- [ ] T033 发布记录区：录入表单（条件必填**在提交前就标出来**）、历史列表按时间倒序；**当前状态标成「最新一条记录」而不是一个独立字段**（FR-019）；交接了但没录时显示**「待登记」并写明系统不会去平台查**（§9.2）；`version_match = unknown` 要有中性表达，不能画成错误
- [ ] T034 **AI 入口**（自动核验、发布建议等）：存在、**禁用**、写明「暂不可用（执行器禁用，EP-08 接入）」；**没有任何伪造的核验结果**
- [ ] T035 四语言文案（en / zh-Hans / ja / ko）并跑 `locales/parity.test.ts`；新建 `manual-ui-todo.md`，界面项全部进去并一律记「未执行」；**不写 UI 单测**（宪法 II），核对 `packages/views` 下没有本卡新增的 `.test.tsx`

---

## Phase 7: Polish

- [ ] T036 跑全部验证：`pnpm typecheck --force`、三项 check、`bash scripts/test-go.sh`、两套 db-suites、core vitest
- [ ] T037 变异验证**九处**，每处确认对应用例变红、**改完即还原**。变异必须**可编译**：
      (M1) 给 `content_publication_record` 加一条 `UPDATE` → T019 变红；
      (M2) 在模块里加一个 `http.Get` → T020 变红；
      (M3) 让处置顺手改写 `snapshot` 列 → T023a 变红；
      (M4) 让 `cancelled` 可以回到 `ready` → T014 变红；
      (M5) 从 `concurrentIndexCleanups` 删掉一条索引登记 → T013 的 R6 变红；
      (M6) 让「缺 `page_url_or_content_id`」返回一句笼统的 400 而不点名 → T018 变红；
      (M7) 让 `handed_off` 顺手插一条 `reported_published` → T023c 变红；
      (M8) 让快照多带一个键（或往 `attachments` 里塞一个 id）→ T023a1 变红；
      (M9) 给声明者加一个 Go 枚举类型 → T015a 变红
- [ ] T038 核对改动文件全部落在 plan.md 清单内；清单外的在 PR 正文单列
- [ ] T039 PR 正文：迁移说明（四张表 + **八个** CONCURRENTLY 索引、无外键、R5 为何不用 `PRIMARY KEY`、R6 **八条**登记、**每张表都有 `workspace_id` 打头的索引**及其理由）、**上游改动一节**、UI 影响（复用了哪些既有组件）、「SOP §8–§9.2 对应」逐句写明现在可操作到什么程度。**三件本卡验不了的事如实写**：「审核绑定的版本永不改变」只有结构保证（work-editor 只插不改）；「系统绝不会自己发布」是守卫与检索的结论，不是运行时证明；「EP-08 接上不必改调用方」今天无法证明

---

## Dependencies

```text
T001 裁决 → T002 读 #141 实际形状 → T003 基线
  └─ 存储 (T004→T005 [P] → T006→T007 [P] → T008→T009 [P] → T010→T011 [P] → T012 删除链+sqlc → T013,T013a 规则)
       └─ 模块 (T014–T020d 先写 [P] → T021 → T022 → T023 → T023a–T023d,T024 先写 → T025 → T026)
            └─ HTTP (T027,T027a,T027b,T027c 先写 → T028 → T028a → T029 上游两文件 + 第 12 步)
                 └─ core (T029a,T029b,T029c [P])
                      └─ 页面 PR (T030 → T031 → T032 → T032a → T033 → T034 → T035)
                           └─ Polish (T036–T039)
```

**先写并确认失败的清单**：T014、T015、T015a、T015b、T016、T017、T018、T018a、T018b、T019、T020、T020a、T020b、T020c、T020d、T023a、T023a1、T023a2、T023a3、T023b、T023c、T023d、T024、T027、T027a、T027b、T027c。

## Implementation Strategy

1. **裁决已到，表的数量定死在四张**（T001）。Q1=A 与 Q3=A 都不加表。实施时唯一还会动的是**迁移号**（T003 从当时最大值顺延）——迁移文件一旦推过就只能再加一个迁移来改。
2. **条件必填不要写成列约束**（T008）。`scheduled_at` 在 `scheduled` 时必填、在 `draft` 时必须为空，同一列在不同状态下规则不同，`CHECK` 表达得出来也读不懂；更要紧的是 `CHECK` 给不出「你缺的是计划时间」这句话，而 SC-004 要的就是这句话。
3. **守卫用例必须同时断言「有 INSERT」**（T019）。只断言「没有 UPDATE / DELETE」的守卫，在一个还没写任何 SQL 的空模块上也是绿的——022 的原版带了这半条，照抄，别省。
4. **发布记录不要补一个状态机**（T016）。`verified_published → removed` 看起来像倒退，实际是作品被平台删了。这张表记的是「人观察到了什么」，不是「系统允许发生什么」；给它加迁移约束等于让人无法如实记录。
5. **「当前发布状态」不要存成列**（T033 + FR-019）。存一份可变的当前状态就是第二份真相，它一定会在某次并发录入后与记录流不一致，而且不一致时没有任何东西会报警。
6. **`scheduled_at` 旁边那句话是交付物**（T032）。排了时间的人默认系统会到点执行——这是本卡最可能造成真实损失的误解（以为发了，其实没发）。禁用一个按钮不需要解释，但**显示一个时间而不执行它**必须解释。
7. **状态机在 Go 与 TS 各一份，就必须有比对用例**（T029b）。两份手写副本漂移是必然的，`ip-profile` 的 `scope.test.ts` 已经给出了读 Go 源文件比对的做法，照抄。渠道四值同理（T015b）。
8. **「不发明枚举」要有一条负例才守得住**（T015a）。裁决说 SOP 没给声明者与核验方式的枚举所以不定义——但半年后看到两个自由文本字段的人，会觉得「补全一下」是在帮忙。M9 就是那一刻，T015a 是那时唯一会出声的东西。
9. **「交接 ≠ 发布」不能只写在文档里**（T023c + M7）。它是本卡最可能造成真实损失的误解：以为发了，其实没发。三种交接方式各推一次、断言发布记录数仍为 0，是 §9.1 那句原文唯一可执行的形式。
10. **§9.3 的四个触发点是一次比较，不是四段代码**（T020d）。换稿、换附件、换账号、改渠道配置——实现成四处各自的 if，必然有一处漏掉。判定收敛成 `states.go` 的一个纯函数，四个触发点只是它的四个输入。
11. **派生显示不要「顺手」存成列**（T020c + FR-019a）。「待登记」与「到期待办」都是一次比较的结果；一旦落成列，它就会在某次并发后与事实不一致，而 §9.2 的原话恰恰是「不推断平台状态」。
