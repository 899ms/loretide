---
description: "Task list for 015 brand account and platform configuration (LT-011)"
---

# Tasks: 品牌账号与平台配置（LT-011）

**Prerequisites**: spec.md、plan.md、contracts/account-api.md

**改动文件必须在 plan.md → Source Code 清单内。**

**测试先写**：标注「先写」的必须在实现前完成并**确认失败**（编译不过也算失败，但要说明）。

**三项 clarify 已裁决（全部 A）**，无待决问题。

---

## Phase 1: Setup

- [x] T001 确认本地 PostgreSQL 可用并导出 `MULTICA_TEST_DATABASE_URL`；跳过不得记为通过
- [x] T002 记录基线：`check:diagnostics-contract`（应报 `checked 2 landed modules`）、`check:content-boundaries`、`go test ./internal/handler -run Workspace`；保存到工作区外，**不提交**。基线已知有一条 `content_dispatch_outbox` 的 `unclassified` 失败（非本卡引入），记下来以便区分

---

## Phase 2: 存储层

- [x] T003 新建 `server/migrations/477_content_account.up.sql` / `.down.sql`：表 `content_account`，列全 `text` / `jsonb` / `timestamptz`（Q2-A）；平台列带 `CHECK` 含 8 个枚举值；**无外键、无级联**
- [x] T004 新建 `server/migrations/478_content_account_workspace_idx.up.sql` / `.down.sql`：`CREATE INDEX CONCURRENTLY` on `(workspace_id)`，**一文件一语句**（PostgreSQL 不允许并发索引在事务或多语句串里）
- [x] T005 新建 `server/pkg/db/queries/content_account.sql`：Create / Get / List / Update。**每条查询都带 `workspace_id` 条件**——隔离是查询的一部分，不是调用方的自觉
- [x] T006 跑 `make sqlc` 生成 `server/pkg/db/generated/`，**核对生成结果**（列类型、参数顺序、是否有意外的外键推断）；生成物**单独提交、不手改**（FR-013）

---

## Phase 3: 模块层（`ip-profile`）

- [x] T007 **先写** `server/internal/content/ip-profile/account_test.go`：A5 非法平台被拒、A6 枚举内每个平台都通过、**A7 Go 枚举与库 `CHECK` 逐值一致**（读迁移文件比对，不靠记忆）、A8 显示名去空白为空被拒。确认失败（模块不存在）
- [x] T008 新建 `server/internal/content/ip-profile/account.go`（`package ipprofile`）：`Platform` 受控枚举（8 值）、`ValidatePlatform`、`ValidateDisplayName`、`Account` 形状
- [x] T009 新建 `server/internal/content/ip-profile/service.go`：Create / Get / List / Update。**每条都按 `workspace_id` 过滤**；创建与更新写**审计事件**（`Audit`），这是 E2 的真实来源——不是为过检查塞的调用
- [x] T009a **D11-V02 的锁**：新增用例断言本卡**没有**建独立人设表、也**没有**建绑定关系表——扫 `server/migrations/` 中本卡新增的文件，断言只创建了 `content_account` 一张表，且没有任何 `persona` / `profile_binding` / `account_binding` 之类的表或关联表。**这条是验收项（W-02 边界），此前没有任何断言守着它**；没有它，下一个人加一张 persona 表不会有任何东西变红
- [x] T010 核对 E1/E2/E3 并跑 `pnpm check:diagnostics-contract`，**确认报 `checked 3 landed modules`**

---

## Phase 4: HTTP 接入与负例

- [x] T011 **先写** `server/internal/handler/content_account_test.go` 的四条核心负例：**A2 跨品牌读 → 404 且 body 无对象字段**、**A3 跨品牌写 → 404 且目标未变**、**A4 不存在的 id → 与 A2 逐字节相同**、A5 非法平台 → 400 诊断错误对象。确认失败
- [x] T012 **先写** A1 与 A9：同品牌两账号互不影响（改其一，另一个**逐字段**不变）、列表只返回当前空间且顺序稳定
- [x] T013 新建 `server/internal/handler/content_account.go`：四个端点；每个操作先 `workspacecore.Authorize`，拒绝用 `RefusalStatus` / `RefusalBody`；平台与显示名非法返回 400 诊断错误对象
- [x] T014 把 `server/internal/handler/content_account.go` 加入 `scripts/content-boundaries.json` 的 `adapters` 白名单（handler 不在 content 根下，import content 模块必须经白名单），跑 `pnpm check:content-boundaries` 确认通过

---

## Phase 5: 工作区删除（`#45` 漏过的那件事）

- [x] T015 **先写** A10：删除工作区后账号行数为 **0**。确认失败
- [x] T016 改 `server/pkg/db/queries/workspace_delete.sql`：加一段 `DELETE FROM content_account WHERE workspace_id = $1::text`（**真删**），重跑 `make sqlc`
- [x] T017 改 `server/internal/handler/workspace_delete_manifest_test.go`：把 `content_account` 登记为 `workspaceDelete`（**登记**）。A11 应通过——新表不出现在 `unclassified`
- [x] T018 确认 T016 与 T017 **两处都改了**。只改一处的后果：只登记不真删 = 数据留存但测试绿；只真删不登记 = 清单漂移失败。`#45` 正是漏了这一对

---

## Phase 6: Polish

- [x] T019 变异验证 5 处（见 contracts M1–M5），每处确认对应用例变红，**改完即还原**。变异必须**可编译**——编译错误不能证明用例抓得住行为
- [x] T020 跑全部验证：`go test` 报 PASS/SKIP/FAIL 三个计数、`pnpm typecheck --force` 报非缓存任务数、`check:content-boundaries`、`check:diagnostics-contract`、`check:diagnostics-no-upload`
- [x] T021 核对改动文件全部落在 plan.md → Source Code 清单内；清单外的在 PR 正文单列
- [x] T022 PR 正文按模板：**勾选存储所有权项**（模块 `ip-profile`，口径按 `content-boundaries.json` 登记与 `diagnostics-onboarding-contract` 模块表——`docs/12` §2 在文档仓库、本检出没有）、勾选接入合同项；**UI 影响写「无」**；加「SOP 对应」一节

---

## Dependencies

```
Setup (T001-T002)
   └─ 存储 (T003→T004→T005→T006 sqlc)
        └─ 模块 (T007 先写→T008→T009→T009a D11-V02→T010 合同)
             └─ HTTP (T011,T012 先写→T013→T014 白名单)
                  └─ 删除 (T015 先写→T016 真删→T017 登记→T018 两处核对)
                       └─ Polish (T019-T022)
```

**T006 必须在 T009 之前**：模块要用 sqlc 生成的类型。
**T011/T012 必须在 T013 之前并确认失败**：先写负例，否则无法区分「实现对」与「测试跟着实现走」。

## Implementation Strategy

1. **MVP = 存储 + 模块 + 创建/读取**：有了实体，LT-012 就能开始。
2. **负例是本卡的核心**（任务卡原话），所以 Phase 4 的先写测试不是形式——四条负例先红再绿。
3. **Phase 5 单独成段**，因为它是 `#45` 已经漏过一次的地方，混在 Polish 里就会再漏一次。

---

## 执行记录（2026-09-15）

T001–T022 全部执行并回勾。与计划不同或需主任务知道的：

- **变异验证暴露了我自己测试里的一个真缺口。** M2「更新不校验账号归属」**没有变红**——因为原有的跨品牌用例里，用户**不是**对方品牌的成员，授权助手在查询之前就拒了，查询自身的 `workspace_id` 条件**根本没被走到**。而这正是任务卡点名的「同用户跨品牌」：一个**同时属于两个品牌**的人，通过 B 的授权后递上 A 的账号 id。补了 `TestAMemberOfTwoBrandsCannotReachOneBrandsAccountWhileActingInTheOther`，M2 才真正变红。**没做变异验证就会把这条漏掉。**
- **M2b（读路径不再按空间过滤）第一次无法表达为纯行为变异**：改 SQL 谓词形状会让 sqlc 重命名/合并参数，生成结构体变了 → 编译失败，而编译错误不能证明用例抓得住行为。改用显式 `sqlc.arg('workspace_id')` 命名保持结构体不变后才变红。
- **工作区删除两处都改了**（T016 真删 + T017 登记）。清单漂移用例现在**只**因 `content_dispatch_outbox` 失败——那是 `#45` 遗留、非本卡引入；`content_account` 已不在 `unclassified` 列表里。
- **`ip-profile` 模块不能 import 生成的 `db` 包**：内容边界检查只批准 pgx / uuid / websocket / otel 作为上游 import。模块因此自定义 `Store` 接口，由 handler 适配 sqlc——与 `workspace-core` 的 `Membership` 同一手法。这个约束反而是对的设计：模块不连库也能测。
- **接入合同 `checked 3 landed modules`**，E2 由**真实接入**满足：创建与更新写审计事件，拒绝写技术事件。
- **无界面改动**，SOP 阶段界面规则无适用项；未新增 UI 单测。
- **本卡不提供删除账号接口**（任务卡只要求创建/读取/更新）；账号停用与删除的语义留给后续卡，未自行决定。
