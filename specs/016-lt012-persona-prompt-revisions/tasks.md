---
description: "Task list for 016 persona prompt configuration revisions (LT-012)"
---

# Tasks: 账号人设提示词的配置版本（LT-012）

**Prerequisites**: spec.md、plan.md、contracts/persona-revision.md

**改动文件必须在 plan.md → Source Code 清单内。**

**测试先写**：标注「先写」的必须在实现前完成并**确认失败**（编译不过也算失败，但要说明）。

**三项 clarify 已裁决（全部 A）**，无待决问题。

---

## Phase 1: Setup

- [x] T001 确认本地 PostgreSQL 可用并导出 `MULTICA_TEST_DATABASE_URL`
- [x] T002 记录基线：`check:diagnostics-contract`（应为 `checked 3 landed modules`）、`check:content-boundaries`、`go test ./internal/handler -run Workspace`。**基线已知 `content_dispatch_outbox` 一条 `unclassified` 失败**（`#45` 遗留，非本卡），记下以便区分

---

## Phase 2: 存储层

- [x] T003 新建 `server/migrations/479_content_account_revision.{up,down}.sql`：表 `content_account_revision`，列 `revision_id`(PK) / `account_id` / `workspace_id` / `revision` / `persona_prompt` / `created_at`。**无外键、无级联**；**唯一约束不写在建表语句里**（Q3）
- [x] T004 新建 `server/migrations/480_content_account_revision_unique_idx.{up,down}.sql`：`CREATE UNIQUE INDEX CONCURRENTLY` on `(account_id, revision)`，**一文件一语句**。这条索引既是并发互斥依据，也是求当前版本的索引
- [x] T005 新建 `server/migrations/481_content_account_revision_workspace_idx.{up,down}.sql`：`CREATE INDEX CONCURRENTLY` on `(workspace_id)`，供工作区删除
- [x] T006 新建 `server/pkg/db/queries/content_account_revision.sql`：Insert / GetByID / GetCurrent / ListByAccount / NextRevision / Count。**每条按 `account_id` + `workspace_id` 过滤**；**不得有任何 UPDATE 或 DELETE**（只插不改不删）
- [x] T007 跑 `make sqlc` 并**核对生成结果**（列类型、参数、无外键推断）；产物**单独提交、不手改**

---

## Phase 3: 模块层（`ip-profile`，不新增模块目录）

- [x] T008 [P] **先写** `server/internal/content/ip-profile/revision_test.go`：**A2 空提示词有效**、**A3 纯空白与空串同等对待**、A10 超长被拒、**A8 重试耗尽回 409**（注入恒冲突的假 store，断言不死循环且返回 409）。确认失败
- [x] T009 新建 `server/internal/content/ip-profile/revision.go`：`Revision` 形状、`ValidatePersonaPrompt`（**空与纯空白都有效**，仅超长拒绝）、`SetPersonaPrompt`（读当前最大 → 插 +1；冲突**有界重试 ≤3**，仍冲突返回 `ErrRevisionConflict`）
- [x] T010 扩 `Store` 接口加版本相关方法；写入版本时记**审计事件**（E2 的真实来源：谁、哪个账号、第几版）
- [x] T010a **D11-V02 与 FR-014 的锁**：`#71` 的 `TestThisFeatureAddsNoPersonaOrBindingTable` 只扫了迁移 `477`/`478`，**覆盖不到本卡的 `479`～`481`**。新增用例断言：①本卡的迁移只建 `content_account_revision` 一张表，②表名与列名**不含** `persona_table` / `persona_library` / `_binding` 之类（persona_prompt 只是一个**字段**的版本历史，不是人设实体）；③`Revision` 形状与设置请求**不含任何任务级 persona 覆盖字段**（FR-014）——否则「账号配置是唯一事实来源」当场失效。**这两条是验收项，此前没有任何断言守着本卡的迁移**
- [x] T011 跑 `pnpm check:diagnostics-contract`，**确认仍为 `checked 3 landed modules`**（本卡不新增模块目录，若变成 4 说明建错了地方）

---

## Phase 4: HTTP 与核心负例

- [x] T012 **先写** `server/internal/handler/content_account_revision_test.go` 的核心负例：**A9 跨品牌读/写 → 404 且与「不存在」逐字节相同**、A4 改 A 三次后 B 不变、**A5 改动后旧 `revision_id` 仍读到旧内容**、A1 连续两次 → 版本 1/2 且版本 1 不变。确认失败
- [x] T013 **先写 A7 真并发**：起**两个 goroutine 同时**对同一账号写入，断言产生版本 1 与 2、无重复、无丢写。**必须是真并发**——顺序写两次不算，那测不到唯一索引的互斥
- [x] T014 新建 `server/internal/handler/content_account_revision.go`：四个端点（设置 / 读当前 / 读指定 / 列出），每个先经 `workspacecore.Authorize`；冲突耗尽回 409 诊断错误对象；超长回 400。**不提供修改或删除版本的端点**（与只插不改不删矛盾）
- [x] T015 把新 handler 文件加入 `scripts/content-boundaries.json` 的 `adapters` 白名单，跑 `pnpm check:content-boundaries`
- [x] T016 **A6 的守卫**：新增用例断言查询文件里**没有任何 `UPDATE` 或 `DELETE`** 针对版本表（工作区删除语句除外），使「只插不改不删」被机器守住而不只是约定

---

## Phase 5: 工作区删除（两处都改）

- [x] T017 **先写** A11：删除工作区后版本行数为 **0**。确认失败
- [x] T018 改 `server/pkg/db/queries/workspace_delete.sql`：加 `DELETE FROM content_account_revision WHERE workspace_id = $1::text`（**真删**），重跑 `make sqlc`
- [x] T019 改 `server/internal/handler/workspace_delete_manifest_test.go`：登记 `content_account_revision`（**登记**）
- [x] T020 确认 T018 与 T019 **两处都改了**。只改一处：只登记不真删 = 数据留存但测试绿；只真删不登记 = 清单漂移失败

---

## Phase 6: Polish

- [x] T021 变异验证 6 处（contracts M1–M6），每处确认对应用例变红、**改完即还原**。变异必须**可编译**——编译错误不能证明用例抓得住行为
- [x] T022 跑全部验证：`go test` 报 PASS/SKIP/FAIL 三计数、`pnpm typecheck --force` 报非缓存任务数、三项 check
- [x] T023 核对改动文件全部落在 plan.md 清单内；清单外的在 PR 正文单列
- [x] T024 PR 正文按模板：勾选**存储所有权**（`content_account_revision` 归 `ip-profile`）与接入合同项；**UI 影响写「无」**；加「SOP 对应」一节写明 **LT-013 页面还差什么**

---

## Dependencies

```
Setup (T001-T002)
   └─ 存储 (T003→T004→T005→T006→T007 sqlc)
        └─ 模块 (T008 先写→T009→T010→T010a D11-V02/FR-014→T011 合同)
             └─ HTTP (T012,T013 先写→T014→T015→T016)
                  └─ 删除 (T017 先写→T018 真删→T019 登记→T020 两处核对)
                       └─ Polish (T021-T024)
```

**T007 必须在 T009 之前**：模块要用生成的类型。
**T012/T013 必须在 T014 之前并确认失败**：先写负例，否则无法区分「实现对」与「测试跟着实现走」。

## Implementation Strategy

1. **MVP = 存储 + 模块 + 设置/读当前**：有了版本，LT-013 页面就能开始。
2. **A7 真并发与 A8 重试耗尽是本卡最容易糊弄过去的两条**，所以单独列任务、单独变异（M3、M4）。
3. **Phase 5 单独成段**——`#45` 已经漏过一次，混在 Polish 里就会再漏。
