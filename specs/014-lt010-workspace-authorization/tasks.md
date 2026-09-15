---
description: "Task list for 014 workspace authorization helper (LT-010)"
---

# Tasks: 内容对象的空间授权入口（LT-010）

**Prerequisites**: spec.md、plan.md、contracts/workspace-authorization.md

**改动文件必须在 plan.md → Source Code 清单内。**

**测试先写**：标注「先写」的必须在实现前完成并**确认失败**（编译不过也算失败，但要说明）。

**三项 clarify 已裁决（全部 A）**，无待决问题。

---

## Phase 1: Setup

- [x] T001 确认本地 PostgreSQL 可用并导出 `MULTICA_TEST_DATABASE_URL` / `LORETIDE_DIAG_TEST_DATABASE_URL`；跳过不得记为通过
- [x] T002 记录基线：未改动时跑 `check:diagnostics-contract`（应报 `checked 1 landed module`）、`check:content-boundaries`、诊断授权相关 Go 用例；保存到工作区外，**不提交**

---

## Phase 2: User Story 1 —— 可复用的授权助手（P1）

- [x] T003 [US1] **先写** `server/internal/content/workspace-core/authz_test.go`：A1 非成员→`ReasonNotMember`、A2 角色不足→`ReasonRole`、A3 主体为空→`ReasonNoActor`、A4 空间标识不可用→`ReasonNoWorkspace`、允许路径带回 actor 与 workspace。用**假成员读取器**，不连库。确认失败（模块不存在，编译不过）
- [x] T003a [US1] **先写**任务卡点名的五条负例，**各自一个用例、名字直呼其事**（SC-005）：①**同一用户在 A 是成员、对 B 不是** → 拒绝；②**另一身份**对同一空间 → 拒绝；③**传入他人的 `workspace_id`** → 拒绝；④**关联空间根本不存在** → 与 ③ **同一理由**（`ReasonNotMember`，不泄漏存在性）；⑤并发归属变化见 T007。前四条与 T003 的通用断言**不重复**：T003 断的是理由枚举，这四条断的是任务卡逐条点名的场景
- [x] T004 [US1] 新建 `server/internal/content/workspace-core/authz.go`（`package workspacecore`）：`Reason` 枚举、`Decision`、`Membership` 接口（参数与返回值只用基本类型，**不依赖 `db.Member`、不 import handler**）、`Authorize(ctx, members, actor, workspace, allowedRoles...)`
- [x] T005 [US1] 在 `authz.go` 补文件头注释：说明它只答「主体能否进空间」、**不答对象归属**，并指向 `contracts/workspace-authorization.md`

---

## Phase 3: User Story 4 —— 归属变化立刻生效（P2，与 US1 同文件）

- [x] T006 [US4] **先写** A5：假读取器计数，断言两次判定**读了两次**成员关系（不缓存，Q3-A）
- [x] T007 [US4] **先写** A6：先允许，再让假读取器返回「已移除」/「降级」，断言下一次判定拒绝

---

## Phase 4: 接入合同与拒绝记录（P1）

- [x] T008 **先写** A7：断言**每次拒绝**都经 `Recorder.Technical` 记了一条技术事件，允许路径不记。确认失败
- [x] T009 在 `authz.go` 增加 `Recorder` 接口并在拒绝路径调用 `Technical(ctx, diagnostics.Event{...})`。**这是真实接入**：事件字段要填得有意义（component、code、outcome、next_action），**不得**为过检查塞无意义调用（FR-013a）
- [x] T010 核对 E1/E2/E3：E1 非测试文件 import `content/diagnostics`；E2 出现 `Technical` 调用点；E3 `_test.go` 引用 `diagnostics`。跑 `pnpm check:diagnostics-contract`，**确认报 `checked 2 landed modules`**
- [x] T011 跑 `pnpm check:content-boundaries`：确认新模块未越界（只 import 标准库与 `content/diagnostics`），且 handler 侧 import 走 `adapters` 白名单

---

## Phase 5: User Story 3 —— 拒绝不泄漏（P1）

- [x] T012 [US3] 新建 `server/internal/content/workspace-core/http.go`：`Refusal()` 规范映射——非成员与角色不足**同为 404**，body 用诊断错误对象形状，**不含对象字段**（FR-004、FR-005）
- [x] T013 [US3] **先写** A9：`server/internal/handler/content_diagnostics_authz_test.go` 中，对「他人空间的真实 id」与「不存在的 id」两次请求断言响应**逐字节相同**

---

## Phase 6: User Story 2 —— 诊断逐字节不变（P1）

- [x] T014 [US2] **先写** A8：钉住诊断三种拒绝组合——非成员 **404**、角色不足 **403**、账号不匹配 **403**，错误码均 `AUTHORIZATION_DENIED`，body 为经 `Sanitize` 的诊断错误对象。**在改 handler 之前先让它们通过**（钉住现状）
- [x] T015 [US2] 改写 `diagnosticScope`：判定改为调用 `workspacecore.Authorize`，映射到它**现有**的 404/403。A8 必须继续通过——若变红即为回归，不得改测试迁就实现
- [x] T016 [US2] 跑诊断既有的全部授权用例，确认无回归

---

## Phase 7: Polish

- [x] T017 变异验证 5 处（见 contracts M1–M5），每处确认对应用例变红，**改完即还原**
- [x] T018 跑全部验证命令并记录退出码；Go 报 PASS/SKIP/FAIL 三个计数；`pnpm typecheck --force` 报非缓存任务数
- [x] T019 核对改动文件全部落在 plan.md → Source Code 清单内；清单外的在 PR 正文单列
- [x] T020 PR 正文：改动与用途、命令与退出码、**UI 影响写「无」**、SOP 对应一节（本 PR 让 §3 哪一步更近、LT-011 还差什么）、变异验证结果、接入合同「通过检查 ≠ 接入合格」的说明

---

## Dependencies

```
Setup (T001-T002)
   └─ US1 助手 (T003→T003a→T004→T005)
        ├─ US4 归属变化 (T006→T007)          # 同文件，串行
        ├─ 接入合同 (T008→T009→T010→T011)    # 依赖 T004 的结构
        ├─ US3 不泄漏 (T012, T013)
        └─ US2 诊断不变 (T014 先钉现状 → T015 → T016)
             └─ Polish (T017-T020)
```

**T014 必须在 T015 之前并先通过**：先钉住现状，再改实现——顺序反了就无法区分「行为没变」和「测试跟着实现一起变了」。

## Implementation Strategy

1. **MVP = US1 + 接入合同**：助手可用且模块合法接入，第二个模块就能开始用。
2. **US2 是风险所在**：提取型改动唯一可能出错的地方就是诊断行为漂移，因此 T014 先行。
3. US3、US4 可与接入合同并行（不同断言、同一文件需串行编辑）。

---

## 执行记录（2026-09-15）

T001–T020 全部执行并回勾。与计划不同或需主任务知道的：

- **接入合同真的生效了**：`pnpm check:diagnostics-contract` 由基线的 `checked 1 landed module` 变为 **`checked 2 landed modules`**。E2 由**真实接入**满足——每次拒绝经 `Recorder.Technical` 记一条技术事件（字段填了 workspace / actor / code / outcome / reason），不是为过检查塞的调用。
- **T014 先于 T015 且先通过**：四条诊断拒绝用例在改 handler **之前**就绿，改完仍绿。这是区分「行为没变」与「测试跟着实现一起改了」的唯一办法。
- **变异验证首两次无效（已修）**：M1 删角色分支、M2 删 `!found` 条件，都会让变量变成未使用而**编译失败**——编译错误不能证明用例抓得住行为。改成 `roleAllowed` 恒真、以及让缺成员行不再判为 `not_member`（两者都可编译、只改行为）后才真正变红。
- **`content_diagnostics.go` 在基线上就不是 gofmt 干净的**：误跑一次 `gofmt -w` 会把该文件炸成 **371 行插入**（真实改动约 37 行）。已回退重做，保持周围密集风格；该文件基线即不干净，非本次引入。
- **基线已有一条失败（非本次引入）**：`TestWorkspaceDeletionManifestCoversPublicSchema` 报 `unclassified=[content_dispatch_outbox]`，与 LT-009 时同一条。本特性**不新增任何表**，与它无关。
- **无界面改动**，故 SOP 阶段界面规则无适用项；未新增 UI 单测、未产生手动条目。
