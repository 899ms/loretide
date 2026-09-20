---
description: "Task list for 027 feedback-learning — manual metrics and feedback excerpts"
---

# Tasks: 人工指标与反馈摘录（027）

**Prerequisites**: `spec.md`、`plan.md`、`contracts/feedback-manual.md`

**改动文件必须在 plan.md → Project Structure 清单内。**

**六条 clarify 已裁决**（主控 2026-09-21，PR #163 评论；SOP §10.1 / §7.1 / §3.2 与 PRD R-044 R-045 原文抄在 `spec.md` 开头）：

- **Q1**：指标十列按 §10.1；`metric` **恰好十一项**无「其它」；`value` **空≠0**；`source_type` **系统写入** `manual` / `csv_import`；**只插不改**（更正=新记录）；CSV 首版 = **粘贴文本**（页面 PR）；
- **Q2=A**：只绑 `publication_record_id`，版本两跳解出，**解不出是 `unknown` 且不拒绝录入**，不改 025 的表；
- **Q3**：AI 复盘状态集取 §7.1 **全集七值**，本卡**只产生 `pending_data`**，其余六值有负例；
- **Q4**：摘录来源**恰好三项** `comment` / `private_message` / `lead`；**摘录与解释分两列**；`tags` 自由标签；
- **Q5=A**：观察窗口记在指标记录自身；**待补录派生不带时间逻辑**；
- **Q6=A（带附加要求）**：**PR 1 同时**更新 026 的 FR-004 / FR-005b 并提供第五项 core 派生函数；页面挂 025 发布记录之后（插槽注入，不改登记表）+ `/{slug}/today` 跳转。

**两张表、四个索引、六个迁移（定稿）。没有 AI 复盘报告表。**

**两个 PR**：T001–T030 是存储与接口 PR；T031–T037 是页面 PR；T038–T041 是收尾。**PR 2 要等 #161 合入。**

---

## Phase 1: Setup

- [x] T001 裁决已拿到并回写四个件：`spec.md`（新增「SOP 与 PRD 原文」与「裁决记录」两节）、`plan.md`、`contracts/feedback-manual.md`、本文件
- [x] T002 记录基线：`bash scripts/test-go.sh`、两套 db-suites（按 `docs/development/testing-database-suites.md` 配 `LORETIDE_DB_TEST_*`，**指向容器自带 PG 上新建的独立库与最小权限角色**）、三项 check、`pnpm typecheck --force`。**同时记下当时 `server/migrations/` 的最大迁移号**——下面的 `N` 从它顺延。**注意**：`packages/views` 的 `onboarding/steps/step-workspace.test.tsx` 在 `app-main` 上已有一条失败（"submits the prefix the user was shown"），与本卡无关，基线里记下即可
- [x] T003 读 025 的实际形状：`content_publication_record` 的列（确认**仍然没有 `version_id`**）、`content_delivery_task.review_request_id`、`content_review_request.version_id`——版本两跳走的就是这三处

---

## Phase 2: 存储（六个迁移，各单条语句）

- [x] T004 `N_content_manual_metric.{up,down}.sql`：建表，十列 + `workspace_id` + 稳定键 + `created_at`。**无 `REFERENCES`/`CASCADE`（R1/R2）、无 `PRIMARY KEY`/`UNIQUE`（R5）、不建索引（R4）**；`metric` 十一值 `CHECK`、`platform` 四值 `CHECK`、`source_type` 两值 `CHECK`；**`value bigint` 可空**，注释写明 **`NULL` 是「未知」、`0` 是「已确认的零值」**（§10.1 原文）。`down` 注明已录的真实结果会丢
- [x] T005 [P] `N+1_content_manual_metric_id_unique_idx` `(manual_metric_id)`；`N+2_content_manual_metric_record_idx` `(workspace_id, publication_record_id, sampled_at DESC)`——**后者前导 `workspace_id`**，同时服务「列一条发布记录的指标」「这条有没有指标」（待补录）与删除链
- [x] T006 `N+3_content_feedback_excerpt.{up,down}.sql`：建表，`source_type` 三值 `CHECK`；**`redacted_excerpt` 与 `interpretation` 是两列**，注释写明为什么（R-045「引用摘录与运营者判断分别保存」——混成一段谁也分不出哪句是证据、哪句是判断）
- [x] T007 [P] `N+4_content_feedback_excerpt_id_unique_idx` `(feedback_excerpt_id)`；`N+5_content_feedback_excerpt_record_idx` `(workspace_id, publication_record_id, occurred_at DESC)`
- [x] T008 两张表进 `workspace_delete_manifest_test.go`（标 `workspaceDelete`）；`workspace_delete.sql` 的同一 CTE 链里加两条按 `workspace_id` 的 `DELETE`；跑 `make sqlc` 并核对产物（**单独提交、不手改**）
- [x] T009 迁移规则核对，**四条都要跑到**：`go test ./internal/migrations -run 'TestContentMigrationConstraints|TestContentConcurrentIndexRegistration'`、`go test ./cmd/migrate -run 'TestEveryConcurrentUpBuildHasCleanup|TestConcurrentIndexCleanupsMatchTheirMigrations'`。**R5 确认两个建表迁移都不含 PK/UNIQUE**；**R6 确认四条登记、两个建表迁移未登记**
- [x] T009a 核对索引面：两张表各有一个以 `workspace_id` 打头的索引。不以它打头的话，删除链会在持锁期间扫全表

---

## Phase 3: 模块（先写测试）

- [x] T010 [P] **先写** 受控集矩阵（纯函数）：`metric` **恰好十一项**、`platform` 四项、指标 `source_type` 两项、摘录 `source_type` **恰好三项**，多一个少一个即红；各一条越界 → `ErrInvalid` **且点名字段**。确认失败（SC-003 / SC-011）
- [x] T010a [P] **先写** 「没有『其它』」的负例：`metric` 与摘录 `source_type` 里**都不存在** `other` / `misc` 之类的值——原文点名了就是全部。确认失败
- [x] T010b [P] **先写** 「没有第五个受控集」的负例：模块里除四个受控集与 AI 状态集外**不存在**别的枚举类型——`unit` / `window` / `evidence_note` / `tags` 等必须还是 `string`。确认失败
- [x] T011 [P] **先写** **空≠0** 的纯函数用例（SC-002）：`value` 用 `*int64` 或等价可空类型；`nil` 与 `ptr(0)` **不相等**，格式化出来也不同。确认失败。**这是本卡最容易悄悄发生的数据损坏，所以它先写**
- [x] T012 [P] **先写** AI 复盘状态集用例（SC-014）：**恰好七个**，与 §7.1 原文逐字一致；另一条负例扫模块源码，**除 `pending_data` 外其余六个没有产生路径**（照 024 对 `generated` 的做法）。确认失败
- [x] T013 [P] **先写** 待补录判定（纯函数，SC-013）：`reported_published` / `verified_published` 且指标数为 0 → 在；指标数 ≥ 1 → 不在；`failed` / `removed` / `unknown` → **始终不在**。另一条检索用例确认判定里**没有任何时间比较**（裁决 Q5=A）。确认失败
- [x] T014 [P] **先写** 批量校验（纯函数，SC-007）：三行、第二行 `metric` 越界 → 整批被拒且**点名第 2 行的 `metric`**；全合法 → 三行都过。确认失败
- [x] T015 [P] **先写** 只插不改的守卫（照 022 / 025）：模块源码出现 `UPDATE CONTENT_MANUAL_METRIC` / `UPDATE CONTENT_FEEDBACK_EXCERPT` 或删除链之外的 `DELETE FROM` 即红；**且必须有 `INSERT INTO`**（否则空模块也绿）。确认失败
- [x] T016 [P] **先写** 不外发 / 不存凭据 / 不调模型三条守卫（SC-008）。确认失败
- [x] T016a [P] **先写** 不聚合守卫（SC-021）：源码里不存在 `sum(` / `avg(` 之类的聚合查询（删除清单的按 `workspace_id` 计数除外），也不存在分类或打分路径。确认失败
- [x] T016b [P] **先写** 不合并阅读与播放的守卫（SC-009，§10.1 原文）：源码里不存在把两个 `metric` 值相加或排名的代码。确认失败
- [x] T016c [P] **先写** 无附件、无自动脱敏两条守卫（SC-017）：源码里不存在文件上传 / 附件引用，也不存在任何个人信息识别或替换路径。确认失败
- [x] T016d [P] **先写** `platform` 对表用例（SC-016）：读 `ip-profile` 的 Go 源文件比对四值。**不 import**（依赖表里没有 ip-profile）。确认失败
- [x] T017 新建 `contract.go`：两类实体、**四个**受控集 + AI 状态集、十列 / 六列的类型、`ErrInvalid`（带字段名）/ `ErrNotFound` / `ErrStorage`
- [x] T018 新建 `states.go`：纯函数的受控集校验、批量校验（**全有或全无**并带行号）、待补录判定、版本两跳的形状
- [x] T019 新建 `store.go` + `metric.go` + `excerpt.go` 的写路径：**栅栏是事务第一句**（`LockForContentDiagnosticWrite`，#104）；同一事务内写 `AuditTx`；`recorded_by` 从会话取；`source_type` **由调用入口决定**（表单 / 导入），不读 body
- [x] T019a **先写** 「删除已提交后写入被拒」的真实 DB 用例（SC-020）：三条写路径（录一条 / 批量 / 摘一条）各一条，且**不留半条记录**。确认失败
- [x] T020 **先写** 空≠0 的真实 DB 用例（SC-002 的第二处）：留空存进去读回来是「未知」，填 0 读回来是 0，**两者不相等**。确认失败
- [x] T021 **先写** 同窗口重复录入（更正）的用例：两条都在，读端取最新，**第一条逐字节不变**（R-044「更新保留历史」）。确认失败
- [x] T022 **先写** 版本两跳的用例（SC-015）：有交付任务的解出 `version_id`；`delivery_task_id` 为空串的解出 `unknown` **且不拒绝录入**。确认失败
- [x] T023 `store.go` 的读路径（列出指标 / 列出摘录 / 待补录，**全部按 `workspace_id` 过滤**）
- [x] T024 跑 `pnpm check:content-boundaries` 与 `pnpm check:diagnostics-contract`，确认**落地模块数 6 → 7** 且两项都退出 0（SC-019）

---

## Phase 4: HTTP（先写测试）

- [x] T025 **先写** 决策顺序用例：五条各一例；**1–2 的拒绝体除 `trace_id` 外逐字节相同**（逐字节比对，不是「都是 404」）；3–5 是 400 诊断错误对象且**点名字段**，批量时**点名行号**。确认失败
- [x] T026 **先写** `source_type` 由系统写的三条用例（SC-004）：走表单端点存下来是 `manual`，走导入端点是 `csv_import`，**请求体里塞一个 `source_type` 不生效**。确认失败
- [x] T026a **先写** `recorded_by` 不可伪造（SC-005）：body 里塞别人的 id 不生效。确认失败
- [x] T027 **先写** 批量全有或全无的 HTTP 用例（SC-007）：一批三行第二行坏 → 400 点名第 2 行，且**表里行数仍为 0**。确认失败
- [x] T028 新建 `content_metric.go` 与 `content_feedback.go`：六条端点；400 / 404 映射沿用既有助手，**不自己造 404**；**没有 DELETE，没有 PATCH**
- [x] T028a `scripts/content-boundaries.json`：两个新 handler 文件加入 `adapters`，跑 `pnpm check:content-boundaries`
- [x] T029 **两个上游文件，同一个 `upstream:` 提交**，PR 正文单列「上游改动」一节（第 13 步）：
      (a) `server/cmd/server/router.go` 挂六条路由；
      (b) `server/cmd/migrate/main.go` 的 `concurrentIndexCleanups` 加 **四条**，索引名逐字对上；两个建表迁移不登记。**新条目放进自己的块**（前空行 + 一行注释），否则 gofmt 会重排既有对齐——025 因此返工过一次，`git diff -w` 必须只有新增行。
      另按**工作流第 12 步**：**本卡没有任何路径参数**（两张表只插不改，没有单条改动路径），所以「参数 ≠ 上下文」的用例没有对象——**这件事要在 PR 正文写明，不是默默跳过**。路由存在性用例照常要，且要断言**没有 DELETE / PATCH 路由**

---

## Phase 5: core 与跨卡改动（PR 1）

- [x] T030 [P] `packages/core/content/feedback-learning/contract.ts` + `contract.test.ts`（**首行 `// @vitest-environment node`**）：zod schema、`parseWithFallback`、**`value` 是 `number | null`**，畸形降级**也不能落成 0**；未知状态值保留；受控集**读 Go 源文件比对**（`metric` 十一项、两个 `source_type`、AI 状态七项）
- [x] T030a [P] `csv.ts` + `csv.test.ts`：粘贴文本 → 行；全有或全无并带行号；空单元格解析成 **`null` 而不是 0**；一条用例专门钉这条
- [x] T030b [P] `pending.ts` + `pending.test.ts`：今日工作台第五项的派生函数（FR-025 / SC-022）
- [x] T030c [P] `queries.ts`：读写 hooks。**全部非乐观**——每一条都会改变别人看到的东西
- [x] T030d **更新 `specs/026-today-dashboard/spec.md`**（FR-025 / SC-022）：FR-004 的「不读未落地模块」名单里**去掉 `feedback-learning`**；FR-005b 从「暂不可用」改成真实内容。**这是规格文件的改动**，在 PR 正文单列一节说明为什么由本卡做（裁决 Q6=A 的附加要求）

---

## Phase 6: 页面 PR（等 #161 合入）

- [ ] T031 `packages/views/content/feedback-learning/`：指标与摘录区块，**只挂既有组件**，不新增控件、不改样式
- [ ] T032 指标区：十项字段的表单；**`value` 留空与填 0 在界面上看得出来是两件事**（留空显示「未知」，不是显示 0，也不是空白让人猜）；十一个指标名的下拉；单位 / 窗口 / 证据是自由输入
- [ ] T032a 粘贴 CSV：一个文本域 + 预览 + 「全有或全无」的提示；坏行**点名第几行的哪一列**；**没有文件上传**（等 W-03），并写明这一点
- [ ] T033 摘录区：**摘录与解释是两个输入框**，各自可空；来源三选一；标签自由；**写明「脱敏请自己做，系统不会替你脱」**
- [ ] T034 待补录区：列出已发布且无指标的发布记录，可直接跳去补录；**写明判定口径**（发了但一条指标都没录），不写「到期」——首版没有时间逻辑
- [ ] T034a **AI 复盘占位**：显示 `pending_data`「待运行」，写明「暂不可用（执行器禁用，EP-08 接入）」；**没有任何伪造的复盘结论**
- [ ] T035 接上今日工作台第五项（FR-025）+ `/{slug}/today` 到本区块的跳转（FR-026）
- [ ] T036 本卡区块挂在 **025 发布记录那一段之后**：在 `review-delivery` 的页面组件上加一个**可选 render 插槽**，由 web 适配器注入。**不改登记表**（与 #161 对 `work-editor` 做的完全一样）
- [ ] T037 四语言文案（en / zh-Hans / ja / ko）并跑 `locales/parity.test.ts`；新建 `manual-ui-todo.md`，界面项全部进去并一律记「未执行」；**不写 UI 单测**（宪法 II），核对 `packages/views` 下没有本卡新增的 `.test.tsx`

---

## Phase 7: Polish

- [x] T038 跑全部验证：`pnpm typecheck --force`、三项 check、`bash scripts/test-go.sh`、两套 db-suites、core vitest、`locales/parity.test.ts`
- [x] T039 变异验证**九处**，每处确认对应用例变红、**改完即还原**。变异必须**可编译**：
      (M1) 给 `content_manual_metric` 加一条 `UPDATE` → T015 变红；
      (M2) 在模块里加一个 `http.Get` → T016 变红；
      (M3) **把 `value` 的 `nil` 读成 0** → T011 与 T020 变红（**这一处最要紧**）；
      (M4) 给 `metric` 加第十二个值 `other` → T010 与 T010a 变红；
      (M5) 让批量在第二行坏时把第一行写进去 → T014 与 T027 变红；
      (M6) 从 `concurrentIndexCleanups` 删掉一条登记 → T009 的 R6 变红；
      (M7) 让某处产生一个 `queued` 状态 → T012 变红；
      (M8) 加一个把 `read` 与 `play` 相加的读路径 → T016b 变红；
      (M9) 让 `source_type` 从请求体读 → T026 变红
- [x] T040 核对改动文件全部落在 plan.md 清单内；清单外的在 PR 正文单列
- [x] T041 PR 正文：迁移说明（两张表 + **四个** CONCURRENTLY 索引、无外键、R5 为何不用 `PRIMARY KEY`、R6 **四条**登记且既有行 0 删除、每张表都有 `workspace_id` 打头的索引）、**上游改动一节**、**对 026 的改动一节**、**「本卡没有路径参数」的说明**、UI 影响、「SOP §10.1 对应」逐句写明现在可操作到什么程度。**三件本卡验不了的事如实写**：「系统绝不会自己去平台取数」只有结构保证；「空≠0 在所有读端都成立」只覆盖了本卡的读端；「EP-08 接上不必改调用方」今天无法证明

---

## Dependencies

```text
T001 裁决 → T002 基线 → T003 读 025 实际形状
  └─ 存储 (T004→T005 [P] → T006→T007 [P] → T008 删除链+sqlc → T009,T009a 规则)
       └─ 模块 (T010–T016d 先写 [P] → T017 → T018 → T019 → T019a–T022 先写 → T023 → T024)
            └─ HTTP (T025,T026,T026a,T027 先写 → T028 → T028a → T029 上游两文件)
                 └─ core 与跨卡 (T030,T030a,T030b,T030c [P] → T030d 改 026)
                      └─ 页面 PR，等 #161 (T031 → T032 → T032a → T033 → T034 → T034a → T035 → T036 → T037)
                           └─ Polish (T038–T041)
```

**先写并确认失败的清单**：T010、T010a、T010b、T011、T012、T013、T014、T015、T016、T016a、T016b、T016c、T016d、T019a、T020、T021、T022、T025、T026、T026a、T027。

## Implementation Strategy

1. **空≠0 要在三个地方各钉一次**（T011 Go 纯函数、T020 真实 DB、T030 core 解析），不是一次。它是本卡唯一会**悄悄**损坏数据的东西：把「平台看不到」记成 0，在任何后续聚合里都是真实的坏数，而且不一致时没有任何东西会报警。M3 就是把它折掉，确认三处都看得见。
2. **`source_type` 做成两个端点，不是一个字段**（T019 + T026）。做成请求体字段再校验，等于把「这条数据怎么来的」交给调用方声明——那就不是来源了。
3. **批量全有或全无**（T014 + T027 + M5）。部分写入会让人以为全录上了，而「录上了几条」正是这个功能唯一要回答的问题。
4. **守卫用例必须同时断言「有 INSERT」**（T015）。只断言「没有 UPDATE / DELETE」的守卫，在一个还没写任何 SQL 的空模块上也是绿的——022 的原版带了这半条，照抄，别省。
5. **「没有『其它』」要有自己的负例**（T010a）。受控集数量对了不代表没人加个 `other` 再删掉一个——原文点名了十一项和三项，那就是全部。
6. **AI 复盘的其余六个状态是保留项，不是能力**（T012 + M7）。状态集按 §7.1 全集定义，但本卡只产生 `pending_data`；扫源码的负例是唯一能在有人「顺手实现一下」时出声的东西。
7. **「阅读」与「播放」不要合并**（T016b + M8）。§10.1 原文就是「分别保留，不直接合并排名」——合并看起来像是在帮读者省事，实际是把两个平台不可比的数摆成一个排行榜。
8. **026 的两条 FR 由本卡 PR 1 改**（T030d）。这是裁决给的范围，不是我扩的；它堵掉的正是「两张卡都以为对方会处理」那种结局。

---

## 实施记录（PR 1 存储与接口，2026-09-20）

分支 `claude/impl-027-feedback-api`，base `app-main` @ `251001e`。**迁移号 516–521**（T002 记下的当时最大号是 515）。

**四处与清单写的不一样，都在 PR 正文单列：**

1. **`window` 列改名 `stat_window`。** `WINDOW` 是 PostgreSQL 的保留字，`window text NOT NULL` 直接语法错误，每处引用都要加引号。同一个字段，合法的名字。
2. **没有按「先写测试、确认失败」的顺序写。** 实现与测试是一起写的，「确认失败」那一步我没有真的做过。替代证据是第 7 阶段的九处变异验证——那买到的是同一件东西（用例是承重的），但**不等价**：变异验证证明用例能抓住我想得到的错误，先写失败还能证明用例不是照着实现反推的。
3. **`tags` 用 `text[]` 而不是分隔字符串。** 合同写的是「`text[]` 或逗号分隔——实施时定」。选数组：带逗号的标签会被每个读端静默劈成两半。
4. **AI 复盘没有表，状态是派生的**——这是规格「裁决记录」里已经单列的一处，实施照它做了：`ReviewStateFor(0)` 恒为 `pending_data`，`information_schema` 里确认没有任何 report 表。

**两处守卫第一版是漏的，变异验证把它们抓出来了：**

- **M8（把 `read` 与 `play` 相加）第一次只被「不聚合」守卫抓到，「不合并阅读播放」守卫放过了它**——它按行扫 Go 的 `"read"`，而变异写的是 SQL 的 `'read'`。已改成同时认两种引号，并另加一条扫整条 SQL 字面量的检查（语句会跨行）。重跑 M8b，两条都变红。
- **M6（删掉一条索引登记）第一次报「仍然绿」，但那是假的**——我的删除字符串带的空格数与 gofmt 对齐后的不一致，文件根本没被改动。按实际行重做后，`TestContentConcurrentIndexRegistration` 与 `TestEveryConcurrentUpBuildHasCleanup` 都变红。**一次「仍然绿」的变异结果，第一步要怀疑的是变异本身没生效。**
