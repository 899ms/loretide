---
description: "Task list for 023 EP-04b — start screen and input snapshot"
---

# Tasks: 开始界面与输入快照（EP-04b）

**Prerequisites**: `spec.md`、`plan.md`、`contracts/start-snapshot.md`

**改动文件必须在 plan.md → Project Structure 清单内。**

**三条 clarify 已裁决**（主控 2026-09-20）：**Q1=B**（新表，append-only，一版简报可多次开始）、Q2=A、Q3=A，附带一问接受；T026 入口 = 选题卡详情页 `start` 之后的区块，列表不加。本清单按裁决写。

**两个 PR**：T001–T023 是存储与接口 PR；T024–T031 是页面 PR；T032–T035 是两个 PR 各自的收尾。

---

## Phase 1: Setup

- [x] T001 记录基线：`bash scripts/test-go.sh`、`bash scripts/test-go-db.sh --suite handler`（按 `docs/development/testing-database-suites.md` 配 `LORETIDE_DB_TEST_*`，**指向容器自带 PG 上新建的独立库与最小权限角色**）、三项 check、`pnpm typecheck --force`

---

## Phase 2: 存储（Q1=B，新表）

- [x] T002 新建 `server/migrations/490_content_start_snapshot.{up,down}.sql`：建表，九列。**无 `REFERENCES`、无 `CASCADE`（R1/R2）、无 `PRIMARY KEY`、无 `UNIQUE`（R5）、不建索引（R4）**。`down` 的 `DROP TABLE` 在注释里写明已开始的快照会丢，与 485 一致
- [x] T003 [P] 新建 `491_content_start_snapshot_id_unique_idx.{up,down}.sql`：`CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS content_start_snapshot_id_unique_idx ON content_start_snapshot (snapshot_id);` **单文件单语句**
- [x] T004 [P] 新建 `492_content_start_snapshot_workspace_idx.{up,down}.sql`：`(workspace_id, brief_revision_id, created_at DESC)`，单文件单语句
- [x] T005 [P] 新建 `493_content_start_snapshot_card_idx.{up,down}.sql`：`(workspace_id, topic_card_id, created_at DESC)`，单文件单语句。**它服务详情页那个区块**（T028 的入口）
- [x] T006 **按模块既有做法调整**：`topic-planning` 不走 sqlc——它的 SQL 是 `store.go` 里的 Go 字符串常量（`briefSelect` 等），`pkg/db/queries/` 下只有 `content_account*.sql`。所以快照的 INSERT 与两条 SELECT 写在 `start.go`，守卫用例（T015）改为扫 `start.go` + `store.go`，与 022 的 `TestBriefStoreHasNoUpdateOrDeletePath` 同形。**唯一进 sqlc 的是工作区删除链那条 `DELETE`**，它本来就在 `workspace_delete.sql` 里
- [x] T007 跑 `make sqlc`（只因删除链改了 `workspace_delete.sql`）并核对生成结果：`models.go` 新增 `ContentStartSnapshot`、`workspace_delete.sql.go` 新增一条 CTE。产物**单独提交、未手改**
- [x] T008 `content_start_snapshot` 进 `server/internal/handler/workspace_delete_manifest_test.go` 的清单，标 `workspaceDelete`；删除事务的同一 CTE 链里加一条按 `workspace_id` 的 `DELETE`；跑 `TestWorkspaceDeletionManifestCoversPublicSchema` 与 022 的删除用例
- [x] T009 迁移规则核对，**四条都要跑到**：`go test ./internal/migrations -run 'TestContentMigrationConstraints|TestContentConcurrentIndexRegistration'`（R1–R6 与登记）、`go test ./cmd/migrate -run 'TestEveryConcurrentUpBuildHasCleanup|TestConcurrentIndexCleanupsMatchTheirMigrations'`。**R5 专门确认 490 不含 PK/UNIQUE**

---

## Phase 3: 快照装配（先写测试）

- [x] T010 [P] **先写** `snapshot_test.go` 的装配矩阵：`source_scope` 与 `saved_preference` **各自独立**（构造两者不同的用例）、`persona_ref` 取开始那一刻的 `revision_id`、预检开关与中性表达被记下。确认失败
- [x] T011 [P] **先写** 九个恒空字段的负例：`config_version` / `sop_version` / `skill_version` / `rule_version` / `executor_version` / `required_sources` / `excluded_sources` / `grants` / `file_hashes` 任一非空即红。确认失败
- [x] T012 [P] **先写** 「三处同名不同物」的用例：简报的 `source_scope` 自由文本**不进**快照的 `source_scope`；给简报写一个不在受控集里的文本仍然合法（022 的既有行为不被本卡收紧）。确认失败
- [x] T013 新建 `server/internal/content/topic-planning/snapshot.go`：**纯函数**，输入是四处配置的值，输出 `diagnostics.Snapshot` + 两个扩展键。不读数据库、不读时钟（时间由调用方传入，理由同 `CanRead` 的 `Now`）
- [x] T014 `contract.go` 加 `StartRequest` / `StartSnapshot`；`store.go` 写路径：栅栏 + **纯 INSERT**；读路径：按版本列出、按 id 取单份，**全部按 `workspace_id` 过滤**
- [x] T015 **先写** 只插不改的守卫用例（照 022 的 A6 形状）：扫 `pkg/db/queries/content_start_snapshot.sql`，出现 `UPDATE` 或删除链之外的 `DELETE` 即红。确认它现在是绿的，并用一次可编译变异（加一条 UPDATE）确认它会红
- [x] T016 跑 `pnpm check:content-boundaries`，确认 `topic-planning → ip-profile` 这条**既有声明**被启用后仍退出 0

---

## Phase 4: HTTP（先写测试）

- [x] T017 **先写** `content_topic_start_test.go`：决策顺序**六条**各一例；**1–4 的拒绝体逐字节相同**（逐字节比对，不是「都是 404」）；第 6 条列出 `missing[]` 且与 1–4 不同形。**没有第 7 条**。确认失败
- [x] T018 **先写** 不可变与多次开始：
      (a) 开始一次 → 改账号配置 / 改账号偏好 / 追加简报版本 / 改品牌开关**各一遍** → 读回快照**十六字段与两个扩展键逐字节不变**（SC-002，四件事各一条用例，不合并）；
      (b) 同一版简报连续开始两次、两次 `source_scope` 不同 → **两个 `snapshot_id` 不同**且**第一份逐字节不变**（SC-002a）。确认失败
- [x] T019 **先写** 服务端就绪复核用例：构造「读判定时可开始、写入前已不可开始」的时序，断言被拒（FR-004）。确认失败
- [x] T020 新建 `server/internal/handler/content_topic_start.go`（写）与快照读取 handler（两条 GET）：400 / 404 映射沿用既有助手，**不自己造 404**；**没有 409 分支**
- [x] T021 `scripts/content-boundaries.json`：新 handler 文件加入 `adapters`，跑 `pnpm check:content-boundaries`

---

## Phase 5: 路由

- [x] T022 **先写**：三条新路由进既有的路由存在性清单，断言未登录/非成员经中间件被拒。确认失败
- [x] T023 **两个上游文件，同一个 `upstream:` 提交**，PR 正文单列「上游改动」一节（第 13 步）：
      (a) `server/cmd/server/router.go` 挂 `POST /briefs/{revisionId}/start`、`GET /briefs/{revisionId}/snapshots`、`GET /snapshots/{snapshotId}`；
      (b) `server/cmd/migrate/main.go` 的 `concurrentIndexCleanups` 加 491/492/493 三条，索引名**逐字**对上迁移文件。**490 不登记。**
      另按**工作流第 12 步**：至少一条穿过真实中间件、`{id}` / `{revisionId}` / `{snapshotId}` 的值 ≠ 上下文值的用例；`{snapshotId}` 必须单独有一条——把别的工作区的 `snapshot_id` 填进来要拿到与「不存在」同形的拒绝

---

> **T024–T025 提前到 PR 1**（主控派单 2026-09-20 指明「core 契约与 hooks」属存储与接口 PR）。页面 PR 只剩 T026–T031。

## Phase 6: 页面 PR

- [x] T024 [P] `packages/core/content/topic-planning/snapshot.ts`：zod schema + 畸形响应降级；`snapshot.test.ts` 用 `// @vitest-environment node`
- [x] T025 [P] `queries.ts`：读既有 profile 端点的 `readiness` 与开始写入。**非乐观**。〔更正：原文「写入会 409，且它会导航」是 Q1=A 时代的残留——裁决后端点**没有 409**（重复开始合法），本卡也**没有导航去处**（无运行实体）。实到：不乐观、不导航，飞行中禁用按钮，成功后把新快照追加进开始记录。〕
- [x] T026 `packages/views/content/topic-planning/start.tsx`：**只挂既有组件**（SettingsSection / SettingsCard / SettingsRow / Select / Input / Button / SettingsSaveState）。缺项**逐项列出**；账号未选时要求先选；项目可选
- [x] T027 「暂不可用」半边（本地文件 / 素材包 / 知识卡）：**显示原因**，不是空白、不是加载中、**没有任何伪造条目**
- [x] T028 **入口（已裁决）**：开始界面挂在**选题卡详情页 EP-04a `start` 动作区块之后**的一个区块里；**选题卡列表不加入口**。该区块同时展示「这张卡一共开始过几次」（493 索引为它而建）。`manual-ui-todo.md` 里**不再**留待裁定项。〔实到：后端今天只提供「某一版的全部开始」，所以展示的是**当前选中版本**的次数；按卡汇总待一条按卡列出的端点，已记入 `manual-ui-todo.md` 的「已知边界」〕
- [x] T029 四语言文案（en / zh-Hans / ja / ko）并跑 `locales/parity.test.ts`
- [x] T030 新建 `manual-ui-todo.md`，界面项全部进去
- [x] T031 **不写 UI 单测**（宪法 II）。核对：`packages/views` 下没有本卡新增的 `.test.tsx`
- [x] T032 跑全部验证：`pnpm typecheck --force`、三项 check、Go 两套（含 `LORETIDE_DB_TEST_*` 的 handler 套件）、core 与 views vitest

---

## Phase 7: Polish

- [x] T033 变异验证四处，每处确认对应用例变红、**改完即还原**。变异必须**可编译**：
      (M1) 让 `saved_preference` 复制 `source_scope`（两个字段合一）→ T010 变红；
      (M2) 让 `grants` 填入一个占位 id → T011 变红；
      (M3) 在 `content_start_snapshot.sql` 里加一条 UPDATE → T015 变红；
      (M4) 从 `concurrentIndexCleanups` 里删掉 492 的登记 → T009 的 R6 变红
- [x] T034 核对改动文件全部落在 plan.md 清单内；清单外的在 PR 正文单列
- [ ] T035 PR 正文：迁移说明（一张新表 + 三个 CONCURRENTLY 索引、无外键、R5 为何不用 PRIMARY KEY、R6 三条登记）、**上游改动一节**（`router.go` 与 `cmd/migrate/main.go`，同一提交）、UI 影响（复用了哪些既有组件）、「SOP 对应」逐句写明现在可操作到什么程度，**「已开始的运行继续看到旧配置」如实写「只有结构保证，行为断言留 agent-workflow」**

---

## Dependencies

```text
T001 基线
  └─ 存储 (T002 建表 → T003,T004,T005 索引 [P] → T006 → T007 sqlc → T008 删除清单 → T009 迁移规则)
       └─ 装配 (T010,T011,T012 先写 [P] → T013 → T014 → T015 守卫 → T016)
            └─ HTTP (T017,T018,T019 先写 → T020 → T021)
                 └─ 路由与登记 (T022 先写 → T023 上游两文件 + 第 12 步)
                      └─ 页面 PR (T024,T025 [P] → T026 → T027 → T028 入口 → T029 四语言 → T030 手验清单 → T031)
                           └─ Polish (T032-T035)
```

**T010/T011/T012/T015/T017/T018/T019/T022 必须先写并确认失败。**

## Implementation Strategy

1. **先钉死「两个字段各自独立」**（T007）。把 `source_scope` 与 `saved_preference` 写成同一个值是本卡最容易犯、也最安静的错误 —— 两者相等在绝大多数用例里都成立，只有构造「改了本次但没改偏好」的那一条才看得出来。
2. **九个恒空字段各一条负例，而不是一条「都为空」**（T008）。笼统一条在有人填上其中一个时仍然会红，但不会说是哪一个；而这九个各有各的接入方（EP-04d / W-03 / EP-08），说清是哪一个才知道该找谁。
3. **不可变用例做四件事而不是一件**（T018 的 (a)）。「改了配置快照不变」听起来是一条，实际是四个不同的来源各有一条路径；只测一个，另外三个坏了不会有人发现。
4. **「一版简报可多次开始」要有自己的用例**（T018 的 (b)）。它是 Q1=B 才换来的行为，也是最容易被「顺手加个幂等保护」悄悄做没的一条——加了就变成 Q1=A 的基数，而且不会有别的用例发现。
5. **三条索引登记各自核对，不是一条「R6 绿」**（T009 + T033 的 M4）。#122 的教训正是：不登记的并发索引，被中断的建索引会在重试时被记成成功，索引永久不可用且运行时没有任何提示。
