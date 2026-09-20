---
description: "Task list for 023 EP-04b — start screen and input snapshot"
---

# Tasks: 开始界面与输入快照（EP-04b）

**Prerequisites**: `spec.md`、`plan.md`、`contracts/start-snapshot.md`

**改动文件必须在 plan.md → Project Structure 清单内。**

**三条 clarify 未裁决**：本清单按推荐值 Q1=A / Q2=A / Q3=A 写。**T001 是「拿到裁决」**，它没回来之前 T003 起的存储任务不要动 —— Q1 选 B 会让 T003–T006 变成另一组任务（新表 + 两个索引 + 删除清单 + R6 登记）。

**两个 PR**：T002–T020 是存储与接口 PR；T021–T028 是页面 PR。

---

## Phase 1: Setup

- [ ] T001 **拿到三条 clarify 的裁决**并回写 `spec.md` 的「待裁决」一节与 `contracts/start-snapshot.md` 的状态行。Q1 若非 A，先改 plan 的「原则 V 落到文件」与 Project Structure，再继续
- [ ] T002 记录基线：`bash scripts/test-go.sh`、`bash scripts/test-go-db.sh --suite handler`（按 `docs/development/testing-database-suites.md` 配 `LORETIDE_DB_TEST_*`，指向本机独立库）、三项 check、`pnpm typecheck --force`

---

## Phase 2: 存储（Q1=A）

- [ ] T003 新建 `server/migrations/490_content_brief_revision_snapshot.{up,down}.sql`：`ALTER TABLE content_brief_revision ADD COLUMN snapshot jsonb NOT NULL DEFAULT '{}'::jsonb`。**单条语句、无外键、无索引**；down 的 `DROP COLUMN` 在注释里写明已开始的快照会丢
- [ ] T004 改 `server/pkg/db/queries/` 的简报查询：insert 与全部 select 带上 `snapshot`；新增**唯一一条受限 UPDATE**（`SET snapshot = $N WHERE … AND snapshot = '{}'::jsonb`）
- [ ] T005 跑 `make sqlc` 并核对生成结果；产物**单独提交、不手改**
- [ ] T006 核对：加列不加表 → 工作区删除清单**无需改动**，跑 `TestWorkspaceDeletionManifestCoversPublicSchema` 证明；不加索引 → 不触发 #122 的 R6，跑 `go test ./internal/migrations -run TestContentConcurrentIndexRegistration` 与 `./cmd/migrate -run TestEveryConcurrentUpBuildHasCleanup` 证明仍绿

---

## Phase 3: 快照装配（先写测试）

- [ ] T007 [P] **先写** `snapshot_test.go` 的装配矩阵：`source_scope` 与 `saved_preference` **各自独立**（构造两者不同的用例）、`persona_ref` 取开始那一刻的 `revision_id`、预检开关与中性表达被记下。确认失败
- [ ] T008 [P] **先写** 九个恒空字段的负例：`config_version` / `sop_version` / `skill_version` / `rule_version` / `executor_version` / `required_sources` / `excluded_sources` / `grants` / `file_hashes` 任一非空即红。确认失败
- [ ] T009 [P] **先写** 「三处同名不同物」的用例：简报的 `source_scope` 自由文本**不进**快照的 `source_scope`；给简报写一个不在受控集里的文本仍然合法（022 的既有行为不被本卡收紧）。确认失败
- [ ] T010 新建 `server/internal/content/topic-planning/snapshot.go`：**纯函数**，输入是四处配置的值，输出 `diagnostics.Snapshot` + 两个扩展键。不读数据库、不读时钟（时间由调用方传入，理由同 `CanRead` 的 `Now`）
- [ ] T011 `contract.go` 加 `StartRequest` / `StartResult`；`store.go` 写路径：栅栏 + 受限 UPDATE，影响行数为 0 即 `ErrConflict`
- [ ] T012 跑 `pnpm check:content-boundaries`，确认 `topic-planning → ip-profile` 这条**既有声明**被启用后仍退出 0

---

## Phase 4: HTTP（先写测试）

- [ ] T013 **先写** `content_topic_start_test.go`：决策顺序七条各一例；**1–4 的拒绝体逐字节相同**；第 6 条列出 `missing[]` 且与 1–4 不同形；第 7 条 409。确认失败
- [ ] T014 **先写** 不可变用例：开始一次 → 改账号配置 / 改账号偏好 / 追加简报版本 / 改品牌开关**各一遍** → 读回快照**十六字段逐字节不变**（SC-002）。确认失败
- [ ] T015 **先写** 服务端就绪复核用例：构造「读判定时可开始、写入前已不可开始」的时序，断言被拒（FR-004）。确认失败
- [ ] T016 新建 `server/internal/handler/content_topic_start.go`：400 / 404 / 409 映射沿用既有助手，**不自己造 404**
- [ ] T017 核对 022 的「只插不改不删」守卫用例：把这条受限 UPDATE **显式登记为唯一允许的一条**，并加用例钉住「只能把 `{}` 变成非 `{}`」
- [ ] T018 `scripts/content-boundaries.json`：新 handler 文件加入 `adapters`，跑 `pnpm check:content-boundaries`

---

## Phase 5: 路由

- [ ] T019 **先写**：新路由进既有的路由存在性清单，断言未登录/非成员经中间件被拒。确认失败
- [ ] T020 改 `server/cmd/server/router.go` 挂 `POST /briefs/{revisionId}/start`。**独立提交、`upstream:` 开头、PR 正文单列「上游改动」一节**。另按**工作流第 12 步**加一条穿过真实中间件、`{id}` 与 `{revisionId}` 的值 ≠ 上下文值的用例

---

## Phase 6: 页面 PR

- [ ] T021 [P] `packages/core/content/topic-planning/snapshot.ts`：zod schema + 畸形响应降级；`snapshot.test.ts` 用 `// @vitest-environment node`
- [ ] T022 [P] `queries.ts`：`useStartReadiness`（读既有 profile 端点的 `readiness`）与 `useStartRun`。**非乐观**——写入会 409，且它会导航
- [ ] T023 `packages/views/content/topic-planning/start.tsx`：**只挂既有组件**（SettingsSection / SettingsCard / SettingsRow / Select / Input / Button / SettingsSaveState）。缺项**逐项列出**；账号未选时要求先选；项目可选
- [ ] T024 「暂不可用」半边（本地文件 / 素材包 / 知识卡）：**显示原因**，不是空白、不是加载中、**没有任何伪造条目**
- [ ] T025 四语言文案（en / zh-Hans / ja / ko）并跑 `locales/parity.test.ts`
- [ ] T026 新建 `manual-ui-todo.md`，界面项全部进去；并在里面留一条给主任务的待裁定项：**开始界面从哪里进入**（选题卡详情？列表行内？）本 PR 不加入口
- [ ] T027 **不写 UI 单测**（宪法 II）。核对：`packages/views` 下没有本卡新增的 `.test.tsx`
- [ ] T028 跑全部验证：`pnpm typecheck --force`、三项 check、Go 两套（含 `LORETIDE_DB_TEST_*` 的 handler 套件）、core 与 views vitest

---

## Phase 7: Polish

- [ ] T029 变异验证三处，每处确认对应用例变红、**改完即还原**。变异必须**可编译**：
      (M1) 让 `saved_preference` 复制 `source_scope`（两个字段合一）→ T007 变红；
      (M2) 让 `grants` 填入一个占位 id → T008 变红；
      (M3) 去掉写入语句的 `WHERE snapshot = '{}'::jsonb` → T014 或 T017 变红
- [ ] T030 核对改动文件全部落在 plan.md 清单内；清单外的在 PR 正文单列
- [ ] T031 PR 正文：迁移说明（加列、无外键、无索引及理由）、**上游改动一节**（router.go）、UI 影响（复用了哪些既有组件）、「SOP 对应」逐句写明现在可操作到什么程度，**「已开始的运行继续看到旧配置」如实写「只有结构保证，行为断言留 agent-workflow」**

---

## Dependencies

```text
T001 裁决
  └─ T002 基线
       └─ 存储 (T003→T004→T005 sqlc→T006 核对)
            └─ 装配 (T007,T008,T009 先写 [P] → T010 → T011 → T012)
                 └─ HTTP (T013,T014,T015 先写 → T016 → T017 → T018)
                      └─ 路由 (T019 先写 → T020 上游 + 第 12 步)
                           └─ 页面 PR (T021,T022 [P] → T023 → T024 → T025 → T026 → T027)
                                └─ Polish (T028-T031)
```

**T007/T008/T009/T013/T014/T015/T019 必须先写并确认失败。**

## Implementation Strategy

1. **先钉死「两个字段各自独立」**（T007）。把 `source_scope` 与 `saved_preference` 写成同一个值是本卡最容易犯、也最安静的错误 —— 两者相等在绝大多数用例里都成立，只有构造「改了本次但没改偏好」的那一条才看得出来。
2. **九个恒空字段各一条负例，而不是一条「都为空」**（T008）。笼统一条在有人填上其中一个时仍然会红，但不会说是哪一个；而这九个各有各的接入方（EP-04d / W-03 / EP-08），说清是哪一个才知道该找谁。
3. **不可变用例做四件事而不是一件**（T014）。「改了配置快照不变」听起来是一条，实际是四个不同的来源各有一条路径；只测一个，另外三个坏了不会有人发现。
