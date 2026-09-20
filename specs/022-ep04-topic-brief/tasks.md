# Tasks: 手工选题卡与冻结简报（EP-04a）

**Input**: `/specs/022-ep04-topic-brief/` 的 spec.md、plan.md、contracts/、ep04-breakdown.md

**分两个 PR 交付**：PR 1 存储与接口（T001～T034），PR 2 页面（T035～T041）。

## 测试口径（不可协商）

- **不写、不跑 UI 单测。** 逻辑进 core node 测试与 Go 测试。
- Go 用例设 `DATABASE_URL` **实跑**，报 PASS / SKIP / FAIL 三个计数。
- 前端逐文件指定 vitest。
- **没跑的检查记「未执行」，不记通过。**

---

## PR 1 · Phase 1：迁移

- [ ] T001 迁移编号按当时 `server/migrations/` 的最大值顺延，先确认最大值再写文件名。 — **FR-013**
- [ ] T002 `content_topic_card` 建表迁移（**单语句、无外键、无级联、不建索引**）。字段见 `contracts/` T-1～T-5。 — **FR-001、FR-006、FR-013**
- [ ] T003 [P] `content_topic_card` 的稳定 id 唯一索引与 `workspace_id` 索引 —— **各自单独文件、单条语句、`CREATE UNIQUE INDEX CONCURRENTLY` / `CREATE INDEX CONCURRENTLY`**。建表迁移不得用会隐式建索引的 `PRIMARY KEY`。 — **FR-011、FR-013**
- [ ] T004 `content_brief_revision` 建表迁移（同 T002 的约束）。字段见 B-1。 — **FR-007、FR-008、FR-011、FR-013**
- [ ] T005 [P] `brief_revision_id` 的稳定 id 唯一索引与 `UNIQUE (topic_card_id, revision)` —— **各自单独文件、`CREATE UNIQUE INDEX CONCURRENTLY`**。建表迁移不得用会隐式建索引的 `PRIMARY KEY`。 — **FR-011、FR-013**
- [ ] T006 [P] `content_brief_revision` 的 `workspace_id` 索引 —— 单独文件、`CREATE INDEX CONCURRENTLY`。 — **FR-013**
- [ ] T007 每个 `.up.sql` 配一个可逆的 `.down.sql`。 — **FR-013**
- [ ] T008 本机实跑 `go run ./cmd/migrate up`，确认七个文件全部应用成功；再跑一次确认幂等。 — **FR-013**
- [ ] T009 迁移约束自证：新迁移中外键 / 级联数为 0；非 CONCURRENTLY 索引数为 0；一个文件多于一条语句的数为 0（SC-008）。既有的 `server/internal/migrations/content_constraints_test.go` 覆盖 468 及之后所有 `content_` 迁移，确认它对新文件也通过。 — **FR-013、SC-008**

## PR 1 · Phase 2：模块与存储

- [ ] T010 先写失败测试 `server/internal/content/topic-planning/*_test.go`：七项字段可存可读回；每项允许「如实说明没有」；四个动作改状态；暂缓 / 放弃记原因与备注。 — **FR-001 ～ FR-004**
- [ ] T011 新建 `server/internal/content/topic-planning/contract.go`：`TopicCard` / `BriefRevision` 形状与动作枚举。 — **FR-001、FR-006、FR-008、FR-015**
- [ ] T012 `store.go`：读写。**改变状态的操作在同一事务内调 `diagnostics.Store.AuditTx`**，审计写失败即整体回滚。 — **FR-015**
- [ ] T013 简报版本 append-only：**不提供** UPDATE 或 DELETE 方法。先写一条测试断言这样的方法不存在（代码检索 + 行为断言双管）。 — **FR-009、FR-010、FR-018、SC-006**
- [ ] T014 「开始」的幂等：同一张卡连续两次「开始」只产生一份首版（SC-007）。先写失败测试。 — **FR-012、SC-007**
- [ ] T015 暂缓 / 放弃不触碰 IP 配置：操作前后账号配置**逐字节比对**（SC-003）。先写失败测试。 — **FR-005、SC-003**
- [ ] T016 接入合同四面：审计、技术日志（`Component`/`Severity`/`Action`/`Outcome`/`Code`）、trace（`Child(ctx)`，**不自己造 trace id**）、脱敏（一律过 `Sanitize`）。 — **FR-015**
- [ ] T017 `pnpm check:diagnostics-contract` 通过，且报告的落地模块数由 **3** 变 **4**（SC-010）。 — **FR-015、SC-010**

## PR 1 · Phase 3：端点与授权

- [ ] T018 `server/internal/handler/content_topic.go`：端点。**授权经 `workspace-core` 的 `Authorize`**，越权经 `RefusalStatus` / `RefusalBody` 返回 404。先写失败测试。 — **FR-016**
- [ ] T019 **把端点挂进 `server/cmd/server/router.go`**，并有一条**穿过真实路由**的用例。#71 / #73 交付了八个账号端点却都没挂路由、处理器测试照样全绿——不能重演。 — **FR-016**
- [ ] T020 带路径参数的端点：一条**穿过真实中间件、路径参数 ≠ 上下文值**的用例（workflow 第 12 步）。 — **FR-017**
- [ ] T021 越权返回 404 且**在技术日志里留下一条事件**（SC-011）。 — **FR-016、SC-011**

## PR 1 · Phase 4：工作区删除

- [ ] T022 两张表登记进 `server/internal/handler/workspace_delete_manifest_test.go`。 — **FR-014**
- [ ] T023 `server/pkg/db/queries/workspace_delete.sql` 的同一 CTE 链里各加一条 `DELETE`；`make sqlc` 重新生成，确认生成物差异只含这两条。 — **FR-014**
- [ ] T024 先写失败测试：删除品牌后两张表中该品牌的行数为 0；邻居品牌不受影响；删除失败回滚时行**原样留下**（照 `TestDeleteWorkspace_PurgesContentDiagnosticsAtomically` 的三条断言）。 — **FR-014、SC-009**

## PR 1 · Phase 5：前端契约

- [ ] T025 `packages/core/content/topic-planning/contract.ts`：zod schema。**不得 cast 网络 JSON**。 — **FR-015**
- [ ] T026 `contract.test.ts`（`// @vitest-environment node`）：畸形响应测试——缺字段、类型错、未知状态值各一条；服务端驱动的枚举分支带 `default`。 — **FR-015**
- [ ] T027 `packages/core/package.json` 注册子路径导出（`auto-precheck` 当初漏过一次，`tsc` 报 TS2307）。 — **FR-015**

## PR 1 · Phase 6：验证与交付

- [ ] T028 `DATABASE_URL` 实跑 `go test ./internal/content/topic-planning/ ./internal/handler/ -count=1`，报三个计数。 — **SC-001 ～ SC-007**
- [ ] T029 前端逐文件跑 `contract.test.ts`；`pnpm typecheck --force` 报非缓存任务数。 — **SC-013**
- [ ] T030 三项 check：`check:content-boundaries`、`check:diagnostics-contract`、`check:diagnostics-no-upload`。 — **SC-010**
- [ ] T031 变异验证：每条新不变量各一处——去掉 append-only、去掉「开始」幂等、去掉授权、去掉删除清单登记。 — **FR-009 ～ FR-012、FR-016**
- [ ] T032 范围自证：**模型调用次数为 0、文件读取次数为 0**（SC-012）；未写输入快照、未生成候选选题、未实现必用 / 排除（FR-019、FR-020）。逐项给出可复核的检索或计数，不靠「我没写」这句话。 — **FR-019、FR-020、SC-012**
- [ ] T033 交付说明写明 **FR-018** 的验收口径：§5.3 的「既有运行继续引用它开始时的版本」按**结构保证**交付——简报版本只插不改，运行以 `brief_revision_id` 钉住；**行为断言（某次运行确实继续看到它钉的那一版）留 agent-workflow 落地时补**。交付说明 MUST 写明这一半尚未断言，MUST NOT 因结构正确就记为已验收。 — **FR-018**
- [ ] T034 `manual-ui-todo.md`：本 PR 无页面，手动项留到 PR 2；但**越权 404** 与**删除后清空**两条可由自动测试覆盖，在交付说明里注明。 — **FR-021**

---

## PR 2 · 页面

- [ ] T035 `packages/views/content/topic-planning/`：**只挂既有组件**（SOP 阶段 UI 规则），不新增控件、不调样式。 — **FR-022**
- [ ] T036 七项字段的表单、四个动作的按钮、暂缓 / 放弃的原因与备注输入。 — **FR-001 ～ FR-004**
- [ ] T037 简报版本的查看与修改；修改产生新版本，旧版本可读。 — **FR-007 ～ FR-009**
- [ ] T038 文案四语言齐全，`locales/parity.test.ts` 通过。 — **FR-022**
- [ ] T039 `manual-ui-todo.md` 写齐：七项可填、四个动作、原因与备注、改简报产生新版、旧版可读、跨品牌隔离、页面与既有设置页视觉一致。 — **FR-021**
- [ ] T040 `pnpm typecheck --force`；**新增 UI 单测数为 0**（SC-013）。 — **FR-021、SC-013**
- [ ] T041 `tasks.md` 回勾；界面各条记「未执行」（只能由主任务在浏览器给出）。 — **FR-023**

---

## 依赖

```text
T001..T009（迁移）
      ↓
T010..T017（模块与存储）
      ↓
T018..T021（端点与授权）
      ↓
T022..T024（工作区删除）
      ↓
T025..T027（前端契约）
      ↓
T028..T034（PR 1 验证与交付）
      ↓
T035..T041（PR 2 页面）
```

## 不做的事

- 不调模型、不读本地文件 / 素材包 / 知识卡。
- 不写输入快照（EP-04b）、不生成候选选题（EP-04c）、不做必用 / 排除（EP-04d）。
- 不加外键、不加级联；不在建表迁移里建索引。
- 不自己判成员关系、不自己造 404。
- 不写 UI 单测；页面不新增控件、不调样式。
