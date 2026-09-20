---
description: "Task list for 024 work-editor — manual writing, works, artifacts and versions"
---

# Tasks: 人工写作的作品容器、文档与版本（024）

**Prerequisites**: `spec.md`、`plan.md`、`contracts/work-and-versions.md`

**改动文件必须在 plan.md → Project Structure 清单内。**

**三条 clarify 已裁决**（主控 2026-09-21，含 SOP §7.1 原文）：**Q1=A**（不自动存版；**要** `workspace_id` 打头的索引）、**Q2=A**（不动登记表）、**Q3 严格按原文**：

- 编辑副本状态**只有** `working` / `saved`；`drafting…published` **不进本卡**，作品**没有状态列**；
- 版本来源**恰好三个** `generated` / `edited` / `adopted`；
- 「恢复」**不是来源而是动作**：`source = edited`、`action = restored`、`restored_from`；
- 「采用」：`source = adopted`、`action = adopted`、`adopted_from`；
- **来源与动作是两列**，历史侧栏两列都显示。

**三张表、七个索引、十个迁移（494–503）。**

**两个 PR**：T001–T026 是存储与接口 PR；T027–T033 是页面 PR；T034–T037 是收尾。

---

## Phase 1: Setup

- [x] T001 裁决已拿到并回写 `spec.md`（「裁决记录」一节）、`plan.md`、`contracts/`。SOP §7.1 原文已抄进 spec 与 contract 的开头
- [ ] T002 记录基线：`bash scripts/test-go.sh`、两套 db-suites（按 `docs/development/testing-database-suites.md` 配 `LORETIDE_DB_TEST_*`，**指向容器自带 PG 上新建的独立库与最小权限角色**）、三项 check、`pnpm typecheck --force`

---

## Phase 2: 存储（十个迁移，各单条语句）

- [ ] T003 `494_content_work.{up,down}.sql`：建表。**无 `REFERENCES`/`CASCADE`（R1/R2）、无 `PRIMARY KEY`/`UNIQUE`（R5）、不建索引（R4）**；**没有 `status` 列**（裁决：`drafting…published` 不进本卡），注释写明为什么；`snapshot_id` 可为 `''` 并注明它是真实状态。`down` 注明已写内容会丢
- [ ] T004 [P] `495_content_work_id_unique_idx` `(work_id)`；`496_content_work_card_idx` `(workspace_id, topic_card_id, created_at DESC)`——**后者前导 `workspace_id`，同时服务「列一张卡下的作品」与删除链**
- [ ] T005 `497_content_artifact.{up,down}.sql`：建表，含 `kind` 的 `CHECK`、可变的 `draft_body`，以及 **`draft_status` 的 `CHECK (draft_status IN ('working','saved'))`**——§7.1 只给这两个值
- [ ] T006 [P] `498_content_artifact_id_unique_idx` `(artifact_id)`；`499_content_artifact_work_idx` `(workspace_id, work_id, position)`
- [ ] T007 `500_content_artifact_version.{up,down}.sql`：建表，**`source` 与 `action` 两个 `CHECK`**——`source IN ('generated','edited','adopted')`（恰好三个，`generated` 留位）、`action IN ('saved','restored','adopted')`；`restored_from` / `adopted_from` 两个可空指针
- [ ] T008 [P] `501_content_artifact_version_id_unique_idx` `(version_id)`；`502_content_artifact_version_unique_idx` `(artifact_id, revision)`——**后者既是唯一性也是并发互斥**
- [ ] T009 `503_content_artifact_version_workspace_idx` `(workspace_id, artifact_id, revision DESC)`：裁决的附带问题 2。**501/502 都不以 `workspace_id` 打头，没有它删除链会在持锁期间扫全表**
- [ ] T010 三张表进 `workspace_delete_manifest_test.go`（标 `workspaceDelete`）；`workspace_delete.sql` 的同一 CTE 链里加三条按 `workspace_id` 的 `DELETE`；跑 `make sqlc` 并核对产物（**单独提交、不手改**）
- [ ] T011 迁移规则核对，**四条都要跑到**：`go test ./internal/migrations -run 'TestContentMigrationConstraints|TestContentConcurrentIndexRegistration'`、`go test ./cmd/migrate -run 'TestEveryConcurrentUpBuildHasCleanup|TestConcurrentIndexCleanupsMatchTheirMigrations'`。**R5 专门确认三个建表迁移都不含 PK/UNIQUE**；**R6 确认七条登记、三个建表迁移未登记**
- [ ] T011a 核对 **FR-021a**：三张表各有一个以 `workspace_id` 打头的索引（496 / 499 / 503）

---

## Phase 3: 模块（先写测试）

- [ ] T012 [P] **先写** `contract_test.go` 的受控集矩阵：`kind` / `draft_status` / `source` / `action` **四个**受控集越界各一条 → `ErrInvalid`；长度上限**按 rune 计数**（中文用例）；空内容合法。确认失败
- [ ] T012a [P] **先写** 来源与动作的组合矩阵：一次普通保存 = (`edited`, `saved`)；恢复 = (`edited`, `restored`) 且 `restored_from` 非空；采用 = (`adopted`, `adopted`) 且 `adopted_from` 非空。**「恢复」不产生第四种来源**——若有人把 `source` 写成 `restored` 即红。确认失败
- [ ] T013 [P] **先写** 「`generated` 产生不出来」的负例：模块**没有任何**产生 `source = generated` 的路径。确认失败
- [ ] T014 [P] **先写** 只插不改的守卫用例（照 022 `TestBriefStoreHasNoUpdateOrDeletePath` / 023 同名用例）：扫模块源码，出现 `UPDATE CONTENT_ARTIFACT_VERSION` 或删除链之外的 `DELETE FROM CONTENT_ARTIFACT_VERSION` 即红；**且必须有 `INSERT INTO`**（否则守卫在空模块上也绿）。确认失败
- [ ] T015 新建 `contract.go`：`Work`（**无状态列**）/ `Artifact`（含 `DraftStatus`）/ `ArtifactVersion`（含 `Source` 与 `Action` 两个字段）、**四个**受控集、`ErrInvalid` / `ErrNotFound` / `ErrStorage`
- [ ] T016 新建 `store.go` 的写路径：**栅栏是事务第一句**（`LockForContentDiagnosticWrite`，#104）；建作品 / 建文档 / PATCH 文档（**只动编辑副本、状态与标题序号，不产生版本**；改了正文即置 `working`）
- [ ] T016a **先写** 编辑副本状态一致性用例（**FR-007b**）：在**三处**会改它的写路径上（自动保存 → `working`、存为版本 → `saved`、恢复 → `saved`），断言**存下来的状态与重算的结果一致**（重算 = 编辑副本与最新版本是否逐字相同）。状态是存的不是算的，两个真相里有一个会错，这条用例是唯一能让它出声的东西。确认失败
- [ ] T017 **先写** 版本号并发用例：N 个并发「存为一版」→ **N 条版本号互不相同**，0 条失败到调用方（冲突方重试后成功）。确认失败
- [ ] T018 `store.go` 的存版本路径：写 (`source = edited`, `action = saved`)，同一事务里把 `draft_status` 置为 `saved`；版本号**照抄 022 `AppendBrief` 的两步写法**——先锁文档行，**再在第二条语句里数号**。理由（Issue #109）写进注释：READ COMMITTED 下等锁不刷新语句快照，在锁定语句里数号会选到已被用掉的号
- [ ] T019 **先写** 恢复与采用：各产生**新版本**；被引用的旧版**逐字节不变**；恢复写 (`edited`, `restored`, `restored_from`) 并**同时**把编辑副本改成那份内容、状态置 `saved`；采用写 (`adopted`, `adopted`, `adopted_from`)，内容与被采用版逐字相同。确认失败
- [ ] T020 `store.go` 的恢复与采用路径 + 读路径（列出 / 取单条，**全部按 `workspace_id` 过滤**）
- [ ] T021 跑 `pnpm check:content-boundaries` 与 `pnpm check:diagnostics-contract`，确认**落地模块数由 4 变 5**且两项都退出 0

---

## Phase 4: HTTP（先写测试）

- [ ] T022 **先写** 决策顺序用例：五条各一例；**1–4 的拒绝体除 `trace_id` 外逐字节相同**（逐字节比对，不是「都是 404」）；第 5 条是 400 诊断错误对象。确认失败
- [ ] T023 **先写** 自动保存不产生版本：连续 PATCH N 次 → 版本数仍为 0 **且状态为 `working`**；再 POST 一次 → 版本数为 1 **且状态为 `saved`**。**版本数**与**状态**是两条断言，任一半坏了都不能被另一半掩盖
- [ ] T024 新建 `content_work.go` 与 `content_work_version.go`：八条端点；400 / 404 映射沿用既有助手，**不自己造 404**；**没有 DELETE，没有版本的 PATCH**
- [ ] T025 `scripts/content-boundaries.json`：新 handler 文件加入 `adapters`，跑 `pnpm check:content-boundaries`
- [ ] T026 **两个上游文件，同一个 `upstream:` 提交**，PR 正文单列「上游改动」一节（第 13 步）：
      (a) `server/cmd/server/router.go` 挂八条路由；
      (b) `server/cmd/migrate/main.go` 的 `concurrentIndexCleanups` 加 **495 / 496 / 498 / 499 / 501 / 502 / 503 七条**，索引名**逐字**对上迁移文件。**494 / 497 / 500 三个建表迁移不登记。**
      另按**工作流第 12 步**：至少一条穿过真实中间件、`{id}` / `{artifactId}` / `{versionId}` 三个值互不相等且都 ≠ 工作区 id 的用例；**`{versionId}` 必须单独有一条**——填别的工作区的 `version_id` 要拿到与「不存在」同形的拒绝

---

## Phase 5: core 契约与 hooks（PR 1）

- [ ] T027 [P] `packages/core/content/work-editor/contract.ts` + `contract.test.ts`（node 环境）：zod schema、畸形响应降级、空历史读成 `[]` 而不是报错
- [ ] T028 [P] `queries.ts`：列作品 / 作品详情 / 文档 / 版本历史 / 单条版本的 query；建作品、建文档、自动保存、存版本、恢复、采用的 mutation。**自动保存是频繁写，其余非乐观**

---

## Phase 6: 页面 PR

- [ ] T029 `packages/views/content/work-editor/`：编辑器页，**只挂既有组件**，不新增控件、不改样式
- [ ] T030 编辑副本状态 `working` / `saved` 在界面上看得出来；版本历史侧栏**来源与动作两列都显示**（§7.1「来源与动作记录」）——压成一列会逼读者去看另一列才知道发生了什么
- [ ] T031 **三个 AI 入口**（选段改写 / 全文润色 / 候选版本）：存在、**禁用**、写明「暂不可用（执行器禁用，EP-08 接入）」；**没有任何伪造的候选**
- [ ] T032 版本差异：**只用既有组件**；没有合适的就**并排显示两版全文**，**不引入 diff 库**（plan 已写死这条，免得页面 PR 临时决定引入依赖）
- [ ] T033 四语言文案（en / zh-Hans / ja / ko）并跑 `locales/parity.test.ts`；新建 `manual-ui-todo.md`，界面项全部进去；**不写 UI 单测**（宪法 II），核对 `packages/views` 下没有本卡新增的 `.test.tsx`

---

## Phase 7: Polish

- [ ] T034 跑全部验证：`pnpm typecheck --force`、三项 check、`bash scripts/test-go.sh`、两套 db-suites、core 与 views vitest
- [ ] T035 变异验证**七处**，每处确认对应用例变红、**改完即还原**。变异必须**可编译**：
      (M1) 让自动保存顺手插一条版本 → T023 变红；
      (M2) 让恢复改写被恢复的那一版而不是插新版 → T019 变红；
      (M3) 把数号并进锁定语句（还原 Issue #109 的写法）→ T017 变红；
      (M4) 让某处产生一条 `source = generated` → T013 变红；
      (M5) 从 `concurrentIndexCleanups` 删掉 502 的登记 → T011 的 R6 变红；
      (M6) 让恢复写 `source = restored`（把动作当成第四种来源）→ T012a 变红；
      (M7) 让存版本忘记把 `draft_status` 置为 `saved` → T016a 与 T023 变红
- [ ] T036 核对改动文件全部落在 plan.md 清单内；清单外的在 PR 正文单列
- [ ] T037 PR 正文：迁移说明（三张表 + **七个** CONCURRENTLY 索引、无外键、R5 为何不用 `PRIMARY KEY`、R6 **七条**登记、**每张表都有 `workspace_id` 打头的索引**及其理由）、**上游改动一节**、UI 影响（复用了哪些既有组件）、「SOP 对应」逐句写明现在可操作到什么程度。**两件本卡验不了的事如实写**：「已审核/交接/发布版本永不删除」只有结构保证；「EP-08 接上不必改调用方」今天无法证明

---

## Dependencies

```text
T001 裁决 → T002 基线
  └─ 存储 (T003→T004 [P] → T005→T006 [P] → T007→T008 [P] → T009 → T010 删除链+sqlc → T011,T011a 规则)
       └─ 模块 (T012,T012a,T013,T014 先写 [P] → T015 → T016 → T016a 先写 → T017 先写 → T018 → T019 先写 → T020 → T021)
            └─ HTTP (T022,T023 先写 → T024 → T025 → T026 上游两文件 + 第 12 步)
                 └─ core (T027,T028 [P])
                      └─ 页面 PR (T029 → T030 → T031 → T032 → T033)
                           └─ Polish (T034-T037)
```

**T012 / T012a / T013 / T014 / T016a / T017 / T019 / T022 / T023 必须先写并确认失败。**

## Implementation Strategy

1. **版本号的两步写法不要重新发明**（T018）。022 的 `AppendBrief` 已经踩过一次（Issue #109），症状是「一次合法保存变成 503」，而且只在并发下出现——单线程测试永远绿。M3 就是把它还原，确认 T017 真的看得见。
2. **守卫用例必须同时断言「有 INSERT」**（T014）。只断言「没有 UPDATE / DELETE」的守卫，在一个还没写任何 SQL 的空模块上也是绿的——022 的原版就带了这半条，照抄它，别省。
3. **自动保存与存版本分两条用例**（T023），而不是一条「版本数对」。前者证明频繁写不污染历史，后者证明显式存得下来；合并成一条，任一半坏了都可能被另一半掩盖。
4. **`generated` 的负例扫的是「有没有产生它的路径」**（T013），不是「当前数据里有几条」。后者在一个空库上恒绿。
5. **三个 AI 入口是禁用不是隐藏**（T031）。隐藏等于这个产品没打算做；空白会被读成没做完；转圈会被读成马上就好。**写明原因**是这条唯一的交付物。
6. **来源与动作分两列，别在实施时「简化」回一列**（T012a + M6）。§7.1 原文就是「来源与动作记录」，而「恢复」回来的内容仍是人写的——来源理应还是 `edited`。把 `restored` 塞进 `source` 看起来更省一列，实际是把「这一版是谁写的」和「它是怎么来的」两个问题混成一个。
7. **编辑副本状态是存的，所以它会错**（T016a + M7）。三处写路径各要一条一致性断言；少一处，那一处就是它开始撒谎的地方。
