---
description: "Task list for 025 review-delivery — manual review requests, delivery tasks and publication records"
---

# Tasks: 人工审核请求、交付任务与发布记录（025）

**Prerequisites**: `spec.md`、`plan.md`、`contracts/review-delivery.md`

**改动文件必须在 plan.md → Project Structure 清单内。**

**五条 clarify 尚未裁决**（Q1 交付快照形态、Q2 任务与请求的关系、Q3 迁移记在哪里、Q4 渠道与交接方式受控集、Q5 声明者与核验方式取值）。本清单按暂定推荐值（Q1=A、Q2=A、Q3=A、Q4=A、Q5 暂定枚举）写。**裁决会改表的数量与迁移数**：Q1=B 多一张交付快照表（+3 个迁移），Q3=B 去掉迁移记录表（−3 个迁移）。T001 是「拿到裁决并回写」，**它没完成之前不要开始 T003 之后的任何一条**。

**暂定四张表、八个索引、十二个迁移。**

**两个 PR**：T001–T029 是存储与接口 PR；T030–T035 是页面 PR；T036–T039 是收尾。

---

## Phase 1: Setup

- [ ] T001 向主控取五条 clarify 的裁决（Q5 需要 SOP §9.2 原文，Q1 需要 §8 原文）。裁决与原文**抄进** `spec.md`（新增「裁决记录」一节）、`plan.md`、`contracts/review-delivery.md` 的开头。**若 §9.2 没有枚举，Q5 的两项就退成自由文本、只保留「必填」**——宁可不发明枚举
- [ ] T002 读 **#141 合入后的 work-editor 实际形状**：`content_artifact_version` 的稳定键列名、`content_artifact.kind` 的取值、`content_work` 的列。本卡只存字符串 id，但**列名与取值要对上实际的树**，不是对上 024 的提案
- [ ] T003 记录基线：`bash scripts/test-go.sh`、两套 db-suites（按 `docs/development/testing-database-suites.md` 配 `LORETIDE_DB_TEST_*`，**指向容器自带 PG 上新建的独立库与最小权限角色**）、三项 check、`pnpm typecheck --force`。**同时记下当时 `server/migrations/` 的最大迁移号**——下面的 `N` 从它顺延，不要照抄本文件里的数字

---

## Phase 2: 存储（十二个迁移，各单条语句）

- [ ] T004 `N_content_review_request.{up,down}.sql`：建表。**无 `REFERENCES`/`CASCADE`（R1/R2）、无 `PRIMARY KEY`/`UNIQUE`（R5）、不建索引（R4）**；`status` 带受控集 `CHECK`；`snapshot jsonb NOT NULL`；`decided_by` / `decision_note` 可为 `''` 并注明**空串是真实状态**（未处置）。`down` 注明已提交的审核请求会丢
- [ ] T005 [P] `N+1_content_review_request_id_unique_idx` `(review_request_id)`；`N+2_content_review_request_workspace_idx` `(workspace_id, artifact_id, created_at DESC)`——**后者前导 `workspace_id`，同时服务「列一个文档的审核历史」与删除链**
- [ ] T006 `N+3_content_review_transition.{up,down}.sql`：建表，含 `subject_kind` 的 `CHECK (subject_kind IN ('review_request','delivery_task'))`。注释写明**为什么两类主体同一张表**（迁移形状完全相同，分表会让「列出这件事的全部动作」要查两次）
- [ ] T007 [P] `N+4_content_review_transition_id_unique_idx` `(transition_id)`；`N+5_content_review_transition_subject_idx` `(workspace_id, subject_kind, subject_id, created_at)`
- [ ] T008 `N+6_content_delivery_task.{up,down}.sql`：建表，`status` 六值 `CHECK`、`handoff_method` 受控集 `CHECK`；`review_request_id` / `scheduled_at` / `handoff_method` / `hold_reason` 均可空（**条件必填由应用代码在状态迁移时校验，不是列约束**，注释写明理由：同一列在不同状态下必填与否不同，列约束表达不了）
- [ ] T009 [P] `N+7_content_delivery_task_id_unique_idx` `(delivery_task_id)`；`N+8_content_delivery_task_workspace_idx` `(workspace_id, status, created_at DESC)`
- [ ] T010 `N+9_content_publication_record.{up,down}.sql`：建表，`status` 五值 `CHECK`；`evidence` 非空由应用校验（**要点名字段**，`CHECK` 只能给出 23514）
- [ ] T011 [P] `N+10_content_publication_record_id_unique_idx` `(publication_record_id)`；`N+11_content_publication_record_artifact_idx` `(workspace_id, artifact_id, created_at DESC)`——**当前发布状态 = 最新一条**（FR-019）就是走这个索引读出来的
- [ ] T012 四张表进 `workspace_delete_manifest_test.go`（标 `workspaceDelete`）；`workspace_delete.sql` 的同一 CTE 链里加四条按 `workspace_id` 的 `DELETE`；跑 `make sqlc` 并核对产物（**单独提交、不手改**）
- [ ] T013 迁移规则核对，**四条都要跑到**：`go test ./internal/migrations -run 'TestContentMigrationConstraints|TestContentConcurrentIndexRegistration'`、`go test ./cmd/migrate -run 'TestEveryConcurrentUpBuildHasCleanup|TestConcurrentIndexCleanupsMatchTheirMigrations'`。**R5 专门确认四个建表迁移都不含 PK/UNIQUE**；**R6 确认八条登记、四个建表迁移未登记**
- [ ] T013a 核对 **FR-020 的索引面**：四张表各有一个以 `workspace_id` 打头的索引（`N+2` / `N+5` / `N+8` / `N+11`）。不以它打头的话，删除链会在持锁期间扫全表

---

## Phase 3: 模块（先写测试）

- [ ] T014 [P] **先写** `states_test.go` 的状态机矩阵（**纯函数，不连数据库**）：审核请求 1+4 条合法迁移、交付任务的全部合法迁移（`draft→ready|cancelled`、`ready→scheduled|handed_off|held|cancelled`、`scheduled→handed_off|held|cancelled`、`held→ready|cancelled`）各一条；**非法**迁移各一条——终态再推进、跳过 `pending` 直接写终态、`cancelled` 复活、`handed_off` 之后再动。确认失败（SC-003）
- [ ] T015 [P] **先写** 受控集越界矩阵：`channel` / `status`（三类各一套）/ `handoff_method` / `claimed_by` / `verification` 各一条越界 → `ErrInvalid`，**且错误点名字段**。确认失败
- [ ] T016 [P] **先写** 「发布记录没有状态机」的正例：`verified_published` 之后再录一条 `removed` **必须被接受**。作品被平台删掉是常态，拦住它等于让人无法如实记录。确认失败（合同 §2）
- [ ] T017 [P] **先写** 条件必填矩阵：`scheduled` 缺 `scheduled_at`、`handed_off` 缺 `handoff_method`、`held` 缺 `hold_reason` 各一条 → 400 且**各自点名**缺的那一项。确认失败
- [ ] T018 [P] **先写** 发布记录三项必填的**三条独立用例**（SC-004，**不合并成一条**）：各缺一次，三次都被拒且各自点名。确认失败
- [ ] T019 [P] **先写** 只插不改的守卫用例（照 022 `TestBriefStoreHasNoUpdateOrDeletePath`）：扫 `content_review_delivery.sql` 与模块源码，出现 `UPDATE CONTENT_PUBLICATION_RECORD` / `UPDATE CONTENT_REVIEW_TRANSITION`，或删除链之外的 `DELETE FROM` 这两张表即红；**且必须有 `INSERT INTO`**（否则守卫在空模块上也绿）。确认失败（SC-005）
- [ ] T020 [P] **先写** 不外发守卫（SC-006，照 `check-diagnostics-no-upload.mjs` 的形状）：模块源码里不存在 `net/http` 客户端调用、不存在 `http.Get` / `http.Post` / `(*http.Client).Do`。确认失败
- [ ] T020a [P] **先写** 不调度的检索用例（SC-010）：全仓检索读取 `scheduled_at` 的代码路径，除本模块的读写与序列化外**不得有第二处**。确认失败
- [ ] T021 新建 `contract.go`：三类实体、受控集（`channel` / 三套 `status` / `handoff_method` / `claimed_by` / `verification`）、`ErrInvalid` / `ErrNotFound` / `ErrStorage`
- [ ] T022 新建 `states.go`：纯函数的合法迁移表与判定，**Go 枚举为准、库 `CHECK` 兜底**（重复一遍是故意的：`CHECK` 给不出「从 X 不能到 Y」这句话）
- [ ] T023 新建 `store.go` 的写路径：**栅栏是事务第一句**（`LockWorkspaceForContentDiagnosticWrite`，#104）；提交审核 / 处置 / 建交付任务 / 推进状态 / 录发布记录。每条改状态的操作**在同一事务内**写 `AuditTx` 与一条 `content_review_transition`（FR-022）
- [ ] T023a **先写** 交付快照不可变用例（FR-004 / SC-001）：提交审核后对同一文档再存 3 个版本、再改文档标题与渠道偏好，**读回 `snapshot` 列逐字节不变**，且绑定版本的内容逐字节不变。确认失败
- [ ] T023b **先写** 重复提交用例（SC-002）：同一文档连续提交两次审核 → 两条 id 不同的请求，**第一条逐字节不变**，第二次**不得**被当作冲突拒绝。确认失败
- [ ] T024 **先写** 「删除已提交后写入被拒」的真实 DB 用例（#104 口径）：五条写路径各一条。确认失败
- [ ] T025 `store.go` 的读路径（列出 / 取单条 / 取某文档最新一条发布记录，**全部按 `workspace_id` 过滤**）
- [ ] T026 跑 `pnpm check:content-boundaries` 与 `pnpm check:diagnostics-contract`，确认**落地模块数 +1** 且两项都退出 0（SC-009）

---

## Phase 4: HTTP（先写测试）

- [ ] T027 **先写** 决策顺序用例：五条各一例；**1–2 的拒绝体除 `trace_id` 外逐字节相同**（逐字节比对，不是「都是 404」）；3–5 是 400 诊断错误对象且**点名字段**。确认失败
- [ ] T028 新建 `content_review.go` 与 `content_delivery.go`：九条端点；400 / 404 映射沿用既有助手，**不自己造 404**；**没有 DELETE 端点，发布记录与迁移记录没有 PATCH**
- [ ] T028a `scripts/content-boundaries.json`：新 handler 文件加入 `adapters`，跑 `pnpm check:content-boundaries`
- [ ] T029 **两个上游文件，同一个 `upstream:` 提交**，PR 正文单列「上游改动」一节（第 13 步）：
      (a) `server/cmd/server/router.go` 挂九条路由；
      (b) `server/cmd/migrate/main.go` 的 `concurrentIndexCleanups` 加 **八条**索引，索引名**逐字**对上迁移文件。**四个建表迁移不登记。**
      另按**工作流第 12 步**：`{reviewId}` 与 `{deliveryId}` **各一条**穿过真实中间件、参数值 ≠ 工作区 id 的用例；填别的工作区的 id 要拿到与「不存在」同形的拒绝，**两种拒绝体互相逐字节相同**（SC-007）

---

## Phase 5: core 契约与 hooks（PR 1）

- [ ] T029a [P] `packages/core/content/review-delivery/contract.ts` + `contract.test.ts`（**首行 `// @vitest-environment node`**）：zod schema、`parseWithFallback`、畸形响应降级、**未知状态值保留而不是丢弃**（装了旧客户端的桌面端会遇到新状态）、空列表读成 `[]` 而不是报错
- [ ] T029b [P] `states.ts` + `states.test.ts`：状态机的前端副本，**用例读 Go 源文件比对**（照 `packages/core/content/ip-profile/scope.test.ts`），漂了就红
- [ ] T029c [P] `queries.ts`：三类实体的列表 / 详情 query 与提交、处置、建任务、推进状态、录发布的 mutation。**全部非乐观**——每一条都会改变别人看到的东西，回滚不是本地可预测的

---

## Phase 6: 页面 PR

- [ ] T030 `packages/views/content/review-delivery/`：审核与交付页面，**只挂既有组件**，不新增控件、不改样式
- [ ] T031 审核请求区：被审版本（只读、指向那一版）、渠道、交付快照（**只读展示**）、四个处置动作与意见输入；已处置的请求**没有任何可改的入口**
- [ ] T032 交付任务区：状态推进按钮按当前状态启用/禁用（非法迁移**禁用并写明为什么**，不是隐藏）；`scheduled` 的计划时间旁**写明「这是人看的待办时间，系统不会到点自动做任何事」**——不写这句，排了时间的人会以为它会自己发
- [ ] T033 发布记录区：录入表单三项必填、历史列表按时间倒序；**当前状态标成「最新一条记录」而不是一个独立字段**（FR-019 在界面上的对应）
- [ ] T034 **AI 入口**（自动核验、发布建议等）：存在、**禁用**、写明「暂不可用（执行器禁用，EP-08 接入）」；**没有任何伪造的核验结果**
- [ ] T035 四语言文案（en / zh-Hans / ja / ko）并跑 `locales/parity.test.ts`；新建 `manual-ui-todo.md`，界面项全部进去并一律记「未执行」；**不写 UI 单测**（宪法 II），核对 `packages/views` 下没有本卡新增的 `.test.tsx`

---

## Phase 7: Polish

- [ ] T036 跑全部验证：`pnpm typecheck --force`、三项 check、`bash scripts/test-go.sh`、两套 db-suites、core vitest
- [ ] T037 变异验证**六处**，每处确认对应用例变红、**改完即还原**。变异必须**可编译**：
      (M1) 给 `content_publication_record` 加一条 `UPDATE` → T019 变红；
      (M2) 在模块里加一个 `http.Get` → T020 变红；
      (M3) 让处置顺手改写 `snapshot` 列 → T023a 变红；
      (M4) 让 `cancelled` 可以回到 `ready` → T014 变红；
      (M5) 从 `concurrentIndexCleanups` 删掉一条索引登记 → T013 的 R6 变红；
      (M6) 让「缺 `evidence`」返回一句笼统的 400 而不点名 → T018 变红
- [ ] T038 核对改动文件全部落在 plan.md 清单内；清单外的在 PR 正文单列
- [ ] T039 PR 正文：迁移说明（四张表 + **八个** CONCURRENTLY 索引、无外键、R5 为何不用 `PRIMARY KEY`、R6 **八条**登记、**每张表都有 `workspace_id` 打头的索引**及其理由）、**上游改动一节**、UI 影响（复用了哪些既有组件）、「SOP §8–§9.2 对应」逐句写明现在可操作到什么程度。**三件本卡验不了的事如实写**：「审核绑定的版本永不改变」只有结构保证（work-editor 只插不改）；「系统绝不会自己发布」是守卫与检索的结论，不是运行时证明；「EP-08 接上不必改调用方」今天无法证明

---

## Dependencies

```text
T001 裁决 → T002 读 #141 实际形状 → T003 基线
  └─ 存储 (T004→T005 [P] → T006→T007 [P] → T008→T009 [P] → T010→T011 [P] → T012 删除链+sqlc → T013,T013a 规则)
       └─ 模块 (T014–T020a 先写 [P] → T021 → T022 → T023 → T023a,T023b,T024 先写 → T025 → T026)
            └─ HTTP (T027 先写 → T028 → T028a → T029 上游两文件 + 第 12 步)
                 └─ core (T029a,T029b,T029c [P])
                      └─ 页面 PR (T030 → T031 → T032 → T033 → T034 → T035)
                           └─ Polish (T036–T039)
```

**T014 / T015 / T016 / T017 / T018 / T019 / T020 / T020a / T023a / T023b / T024 / T027 必须先写并确认失败。**

## Implementation Strategy

1. **裁决没到不要先建表**（T001）。五条里有两条直接改表的数量：Q1=B 会把交付快照拆成独立表，Q3=B 会让迁移记录表整个消失。先建了再改，改的是迁移文件——而迁移文件一旦推过就只能再加一个迁移来改。
2. **条件必填不要写成列约束**（T008）。`scheduled_at` 在 `scheduled` 时必填、在 `draft` 时必须为空，同一列在不同状态下规则不同，`CHECK` 表达得出来也读不懂；更要紧的是 `CHECK` 给不出「你缺的是计划时间」这句话，而 SC-004 要的就是这句话。
3. **守卫用例必须同时断言「有 INSERT」**（T019）。只断言「没有 UPDATE / DELETE」的守卫，在一个还没写任何 SQL 的空模块上也是绿的——022 的原版带了这半条，照抄，别省。
4. **发布记录不要补一个状态机**（T016）。`verified_published → removed` 看起来像倒退，实际是作品被平台删了。这张表记的是「人观察到了什么」，不是「系统允许发生什么」；给它加迁移约束等于让人无法如实记录。
5. **「当前发布状态」不要存成列**（T033 + FR-019）。存一份可变的当前状态就是第二份真相，它一定会在某次并发录入后与记录流不一致，而且不一致时没有任何东西会报警。
6. **`scheduled_at` 旁边那句话是交付物**（T032）。排了时间的人默认系统会到点执行——这是本卡最可能造成真实损失的误解（以为发了，其实没发）。禁用一个按钮不需要解释，但**显示一个时间而不执行它**必须解释。
7. **状态机在 Go 与 TS 各一份，就必须有比对用例**（T029b）。两份手写副本漂移是必然的，`ip-profile` 的 `scope.test.ts` 已经给出了读 Go 源文件比对的做法，照抄。
