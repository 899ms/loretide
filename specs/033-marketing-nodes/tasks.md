---
description: "Task list for 033 marketing nodes — BO-01 / R-056"
---

# Tasks: 营销节点联动选题（033）

**Prerequisites**: `spec.md`、`plan.md`、`contracts/marketing-nodes.md`

**改动文件必须在 plan.md → Project Structure 清单内。**

**裁决**：Q1/Q2/Q3 = A，D1–D4 均批准（主控已裁定 2026-09-25，PR #256 评论）。D1 的两行 `adapters` 由 PR 1、PR 3 各自加上，本规格 PR 不改 `scripts/content-boundaries.json`。

**迁移号**：不预占。下文用 `N`…`N+8` 表示相对顺序；每个实施 PR 合并前把迁移改号为紧接当时 `app-main` 最大号之后的连续编号，033 与 034 谁先合并谁先取号。

**三个 PR 按顺序做，前一个合并后下一个才从 `app-main` 新起分支**（工作流第 7 步）。每个 PR 各自是一个可审查的整体。

---

## 共用：验证命令

每个 PR 交付前全部跑一遍，PR 正文按工作流第 10 节给出命令、退出码、PASS / SKIP / FAIL 三个计数：

```bash
# 纯逻辑与边界
bash scripts/test-go.sh
pnpm typecheck --force                      # 不接受缓存命中（本机 memory：cached typecheck is not evidence）
pnpm check:content-boundaries
pnpm check:diagnostics-contract
pnpm check:diagnostics-no-upload
pnpm --filter @multica/core exec vitest run content/topic-planning

# 迁移规则（R1–R6，文本检查，不连库）
(cd server && go test -v ./internal/migrations/ ./cmd/migrate/ -run 'Content|Concurrent|Cleanup')

# 带库的三个套件，只经 wrapper（docs/development/testing-database-suites.md）
bash scripts/test-go-db.sh --suite topic-planning
bash scripts/test-go-db.sh --suite handler
bash scripts/test-go-db.sh --suite cmd-server

# 远程验收（主控的远程编译测试机，一次性 PG17 测试库）
ssh ... ~/loretide-ci/lt-verify.sh <branch> all
```

本机没有隔离测试库时，带库套件记「环境未配置，未执行」，以**远程验收**的计数为准；**不记为 PASS**（工作流铁律 1）。任何时候都不连接非测试数据库。

---

## Phase 1: Setup（三个 PR 共用，PR 1 开始时做）

- [x] T001 裁决已回写进 spec、plan、contract 与本文件（Q1/Q2/Q3 = A，D1–D4 批准，主控 2026-09-25）；spec 内不再有「暂定」字样
- [ ] T002 记录基线：上面「共用：验证命令」在 `app-main` 上各跑一次，把已知失败（若有）写进 PR 正文，与本卡无关的不修
- [ ] T003 对着读既有形状：`topic-planning/store.go:83`（`begin`）、`:275-356`（`checkSources` / `SetAccount` 的注释）、`sources.go`（只用字符串的端口）、`contract.go:95-150`（`PatchString` / `DisallowUnknownFields`）、`store_integration_test.go:483`（A6 守卫的写法）、`handler/workspace.go:224-249`（时区校验）、`server/cmd/server/content_topic_routes_test.go`（第 12 步用例）

---

## PR 1 卡片：存储、迁移、节点领域服务与 API

| 项 | 内容 |
|---|---|
| **分支** | `claude/033-pr1-marketing-node-storage`，从 `app-main` 新起 |
| **文件** | plan.md「PR 1」清单；`adapters` 加 D1 第一行 `server/internal/handler/content_marketing_node.go` |
| **测试** | 日期纯函数、校验与变更类型判定、store 真实库用例、handler 真实库用例、`cmd/server` 真实路由用例、core zod 畸形用例、CSV 纯函数用例、迁移规则文本检查 |
| **验收** | D14-V01（节点：两品牌同名互不可见、同形 404、导入判重不跨品牌）；D14-V02（时区换日、跨年、夏令时、提前量未设 ≠ 0）；D14-V03（版本只插、并发修改 409） |
| **不含** | 候选、采用、影响、页面 |
| **验证** | 「共用：验证命令」全部 + 远程验收 `lt-verify.sh <branch> all` |

### 迁移（PR 1）

- [ ] T004 写 `server/migrations/<N>_content_marketing_node.{up,down}.sql`，列照 contract §1.1；注释以外不出现 `UNIQUE` / `PRIMARY KEY` / `REFERENCES` / `FOREIGN KEY` / `CASCADE`；文件头注释写明「唯一性来自 N+1 的并发索引」— **FR-003、FR-004、FR-046**
- [ ] T005 [P] 写 N+1 / N+2 两个并发索引迁移，各一条语句 — **FR-046**
- [ ] T006 写 `<N+3>_content_marketing_node_revision.{up,down}.sql`（建表，同 T004 的五个词禁用）；`lead_days integer` **可空**，注释写明 NULL = 未设置、0 = 不需要准备 — **FR-001、FR-005、FR-013**
- [ ] T007 [P] 写 N+4 / N+5 两个并发唯一索引迁移；N+5 的注释写明它是并发修改的第二道防线 — **FR-005、FR-007**
- [ ] T008 写 `<N+6>_content_marketing_node_candidate.{up,down}.sql`（建表，同 T004 的五个词禁用，**不在表上写唯一约束**）；`account_id text NOT NULL DEFAULT ''`，注释写明为什么不用 NULL — **FR-019**
- [ ] T009 [P] 写 N+7 / N+8；N+8 是候选幂等键 `(workspace_id, node_id, account_id)`，**单独一个文件、一条 `CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS` 语句** — **FR-019**
- [ ] T010 `server/cmd/migrate/main.go` 的 `concurrentIndexCleanups` 加六行（N+1/N+2/N+4/N+5/N+7/N+8）；跑 `go test ./internal/migrations/ ./cmd/migrate/`，确认 R1–R6 全绿 — **FR-046、SC-011**
- [ ] T011 删除链：`workspace_delete.sql` 加三段（候选 → 版本 → 节点），重新生成 sqlc；`workspace_delete_manifest_test.go` 加三行 `workspaceDelete` — **FR-047**

### 领域：类型、校验、日期（PR 1，先写测试）

- [ ] T012 **先写** `marketing_node_test.go` 并确认失败：`kind` 受控集；名称 1–200；结束早于开始 400 指名 `ends_on`；跨度 > 366 天 400；时区 `""`、`Local`、`Mars/Olympus` 各 400 指名 `timezone`；`lead_days` −1 / 366 / 1.5 400；未知键 400；适用账号 > 20 条 400；`accounts` 按 `account_id` 去重保留首次；素材走 `NormalizeSourceIDs`（字段名 `material_source_ids`）；导入 101 行 400 — **FR-001、FR-002、FR-008、FR-011～FR-013**
- [ ] T013 **先写** `marketing_node_dates_test.go` 并确认失败：同一注入时刻 `2026-11-10T16:30:00Z`，Asia/Shanghai 的「今天」是 11-11、America/Los_Angeles 是 11-10；跨年节点 2026-12-31～2027-01-02 提前 10 天准备期 2026-12-21，2027-01-01 为 `live`；纽约 11-05 提前 7 天准备期 10-29；提前量未设置时开始前为 `before_start_unknown_lead`；提前量 0 时开始日当天为 `live`、前一天为 `before_preparation` — **FR-014～FR-016、SC-003**
- [ ] T014 写 `marketing_node_dates.go`：日期三元组、按日历日加减、`Today` / `PreparationStartsOn` / `Phase`；**不用** `24*time.Hour` — **FR-014～FR-016**
- [ ] T015 写 `marketing_node.go`：类型、请求解码（`DisallowUnknownFields`、`lead_days` 的「缺失 / null / 数字」三分）、形状校验、变更类型判定（contract §3.3）；文件里 `import _ "time/tzdata"` 并在注释写明原因 — **FR-001～FR-006、FR-011～FR-013**
- [ ] T016 **先写**变更类型判定用例并确认失败：只改 `goal` → `edit`；改 `lead_days` 从未设置到 0 → `reschedule`；内容完全相同 → 400 — **FR-006、SC-008**

### 领域：store（PR 1，先写测试）

- [ ] T017 **先写** `marketing_node_store_integration_test.go` 并确认失败：新建 → 读回逐字段一致、版本 1、`active`、`manual`；修改 → 版本 2，版本 1 逐字节未变；同一 `base_revision` 并发两次修改恰好一次 409；取消后再修改 / 确认 400；导入 3 行（1 行与已有同名同开始日）→ 2 建 1 重复，再导入一次全重复；导入节点为 `unconfirmed` + `import`；确认只改状态、追加 `confirm` 版本、不改 `date_certainty` — **FR-003～FR-010、FR-037、SC-001**
- [ ] T018 **先写**品牌隔离用例并确认失败：两个工作区各建「双十一」，各自列表各 1 个；用对方节点号读 / 改 / 确认 / 取消 / 读历史全部 404 且与随机 id 的 404 相同；B 导入 A 已有的行不算重复；适用账号用别的品牌的账号 → 404、素材用别的品牌的素材 → 404，两者响应体相同，且节点未被部分写入 — **FR-042、FR-044、SC-009**
- [ ] T019 **先写**提前量用例并确认失败：`lead_days` 省略、`null`、`0` 分别存读，前两者读回「未设置」，后者读回 0 — **FR-013、SC-002**
- [ ] T020 写 `marketing_node_store.go`：新建、导入、修改、确认、取消、读、列、历史。每条写路径经 `begin`（栅栏为第一条语句）、同事务审计（`create-marketing-node` / `import-marketing-nodes` / `revise-marketing-node` / `confirm-marketing-node` / `cancel-marketing-node`）、账号与素材校验**在事务内**、节点行 `FOR UPDATE` 后比较 `base_revision` — **FR-005、FR-007、FR-044、FR-048**
- [ ] T021 `store.go` 的 `Store` 加 `Now func() time.Time` 字段（未设置用 `time.Now`），**只加这一个字段** — **FR-014**
- [ ] T022 **先写**栅栏用例并确认失败：工作区删除已提交时写节点 → 404，无孤儿行（照 `content_topic_fence_test.go` 的形状） — **FR-048、US6 场景 5**
- [ ] T023 **先写** `marketing_node_guards_test.go` 的只插守卫并确认失败（先让它在空实现上因为「找不到 INSERT」而红）：`marketing_node*.go` 不得出现 `UPDATE CONTENT_MARKETING_NODE_REVISION` / `DELETE FROM CONTENT_MARKETING_NODE_REVISION`，必须出现 `INSERT INTO CONTENT_MARKETING_NODE_REVISION` — **FR-005、FR-041**

### HTTP 与 core（PR 1）

- [ ] T024 写 `server/internal/handler/content_marketing_node.go`：PR 1 的八个端点；`workspace-core.Authorize` 与 `/api/content-topics` 同口径；路径参数只用 `chi.URLParam`；错误映射照 contract §2.2 — **FR-042、FR-043、FR-049**
- [ ] T025 `router.go` 挂 `/api/content-marketing-nodes`，`r.Use(h.DiagnosticTrace)`，`/import` 注册在 `/{nodeId}` 之前 — **FR-049、FR-050、SC-012**
- [ ] T026 **先写** `server/cmd/server/content_marketing_node_routes_test.go` 并确认失败：每个带 `{nodeId}` 的端点穿过真实 router 与中间件、路径参数值 ≠ 上下文工作区 id、断言 handler 用的是 URL 里的那个；未登录与非成员被拒；`POST /import` 没被当成节点 id — **FR-049、FR-050、SC-012**
- [ ] T027 **先写** `content_marketing_node_test.go` 的 handler 真实库用例：400 指名字段、404 同形、409 — **FR-042、FR-043**
- [ ] T028 core：`packages/core/api/client.ts` 八个方法；`marketing-nodes.ts` 的节点、版本、导入结果三种 zod；`lead_days` 缺失 / `null` → `{set:false}`；未知 `phase` / `change_kind` 读成 `{kind:"unknown", raw}`；`marketing-nodes.test.ts` 三种形状各三类畸形；`marketing-node-import.ts` + 用例（引号内逗号、空行、BOM、列数不对、`lead_days` 空与 0）；`queries.ts` hooks（key 含 `wsId`） — **FR-051、SC-002、SC-015**
- [ ] T029 PR 1 变异 M1、M2、M3（Go 与 core 各一次）、M7、M8、M10，每条先红后还原，PR 正文逐条列出变红的用例名 — **SC-001～SC-003、SC-009**
- [ ] T030 PR 1 验证：合并前把 9 个迁移改号为紧接当时 `app-main` 最大号的连续编号（033 与 034 谁先合并谁先取号），同步 `concurrentIndexCleanups` 的键，改号后重跑迁移规则检查；然后「共用：验证命令」全部 + 远程验收；PR 正文列 R1–R6 逐条复核表（contract §1.3）与迁移 up / down 实测结果 — **SC-011、SC-013**

---

## PR 2 卡片：候选整理、采用、改期 / 取消影响

| 项 | 内容 |
|---|---|
| **分支** | `claude/033-pr2-marketing-node-candidates`，PR 1 合并后从 `app-main` 新起 |
| **文件** | plan.md「PR 2」清单 |
| **测试** | 读时计算纯函数（撞期、缺口、重复风险、关联理由）、候选 store 真实库用例（幂等、并发、采用、挂卡、影响）、下游表守卫、handler 与真实路由用例、core 畸形用例 |
| **验收** | D14-V01（候选与采用：用对方卡挂 404、候选不部分写入）；D14-V02（候选显示关联理由、缺口与来源；采用前不启动创作——6 张下游表行数零变化）；D14-V03（重复整理不重复、重复采用不重复建卡、改期 / 取消列出受影响卡、历史快照与已发布记录逐字节不变） |
| **不含** | 页面；交付待办参与撞期（Q1=A）；开始快照扩展字段（D3 已定不加）；`store.go:167` 栅栏外账号校验的修正（后续项；已由 #261 修复，2026-09-25） |
| **验证** | 「共用：验证命令」全部 + 远程验收 `lt-verify.sh <branch> all` |

### 读时计算（PR 2，先写测试）

- [ ] T031 **先写** `marketing_node_candidate_test.go` 的纯函数用例并确认失败：撞期——同账号区间相交算、不同账号不算、品牌级节点与任何账号算、已取消与待确认节点不算、提前量未设置按开始日起算、区间端点相接（一方结束日 = 另一方准备期开始日）算相交；重复风险——名称少于 2 字不匹配、`%` `_` 被转义、`dropped` 卡不算、品牌级候选查全品牌；`lead_short` 在提前量未设置时为 `null` — **FR-024、FR-025、FR-027、SC-003**
- [ ] T032 `marketing_node.go` 声明 `SourceStatusReader`（只用字符串：`Status(ctx, workspaceID, sourceID) (status string, found bool, err error)`），注释写明为什么不带账号参数、将来加账号归属时必须接 `CanRead` — **FR-026、FR-045**
- [ ] T033 **先写**负例并确认它守住了：`SourceReader` 与 `SourceStatusReader` 的方法签名里没有账号参数（反射读方法签名），失败信息写明「加了账号参数就必须在同一 PR 接入 workspace-core.CanRead」 — **FR-045**

### 候选 store（PR 2，先写测试）

- [ ] T034 **先写** `marketing_node_candidate_integration_test.go` 的整理用例并确认失败：两个适用账号 → 2 条；再整理 → 2 条且已写角度不变；两个请求并发整理 → 2 条；无适用账号 → 1 条品牌级，整理 3 次仍 1 条；移出一个账号后候选仍在、`in_scope=false`；`unconfirmed` / `cancelled` 节点整理 400 — **FR-018～FR-022、SC-004**
- [ ] T035 **先写**读时计算的真实库用例并确认失败：受众未填的账号 → `relation.account.audience.status` 为 `pending`；素材一条归档一条不存在 → 两种缺口；同账号有一张正文含节点名的卡 → `name_match`；另一个同期节点 → 出现在 `collisions`；来源、`date_certainty`、`date_basis` 原样带出 — **FR-023～FR-028、SC-010**
- [ ] T036 **先写**采用用例并确认失败：`mode=create` → 卡数 +1、`draft`、账号与候选一致、`timing` 为 contract §4.3 模板、`ip_fit` = 角度、`fit_source_ids` = 节点素材；候选 `adopted` 并记 `adopted_revision`；同一候选采用 3 次（含 2 次并发）卡数仍只 +1 且返回同一张卡；简报版本、开始记录、作品、审核请求、交付待办、发布记录 6 张表行数零变化 — **FR-030～FR-033、FR-035、SC-005**
- [ ] T037 **先写**挂卡用例并确认失败：挂本品牌卡 → 那张卡全部列逐字节未变；挂别的品牌的卡 → 404 且候选未被部分写入；卡账号 ≠ 候选账号 → 400 指名 `topic_card_id` — **FR-034、SC-006、SC-009**
- [ ] T038 **先写**改候选用例并确认失败：角度缺省不改、显式空串清空；`open` ↔ `dismissed` 可来回；已采用 → `dismissed` 400 — **FR-020、FR-036**
- [ ] T039 `store.go`：把 `Create` 里的 INSERT 与素材校验抽成一个接收 `pgx.Tx` 的函数，`Create` 改为调用它。**`Create` 的对外行为、既有用例一行不改且全绿**；`Create` 里位于栅栏外的账号校验（`store.go:167`）**不动**（Out of Scope 10；已由 #261 修复，2026-09-25） — **FR-032**
- [ ] T040 写 `marketing_node_candidate.go`：整理（`ON CONFLICT DO NOTHING`）、改候选、采用（候选行 `FOR UPDATE`、已采用直接返回、账号与素材校验在事务内、调用 T039 的函数）、读时计算；审计 `sync-marketing-candidates` / `edit-marketing-candidate` / `adopt-marketing-candidate` — **FR-018～FR-036、FR-048**

### 改期、取消与影响（PR 2，先写测试）

- [ ] T041 **先写**影响用例并确认失败：采用 → 改期 → 影响清单 1 条，带改期前后四个字段；采用 → 取消 → 1 条且 `cancelled=true`；只改 `goal` → 0 条；决定 `kept` → 0 条；再改期 → 重新出现；采用发生在改期**之后**的候选 → 不列 — **FR-038、FR-039、SC-007、SC-008**
- [ ] T042 **先写**「下游不变」用例并确认失败：在一个带选题卡、简报版本、开始记录、作品、作品版本、发布记录的夹具上执行改期、取消、影响决定各一次，六张表的全部行逐字节比较不变；候选的 `topic_card_id` 与 `adopted_revision` 不变 — **FR-040、SC-007**
- [ ] T043 `marketing_node_guards_test.go` 追加下游表守卫：`marketing_node*.go` 不得出现对 contract §7 所列表的 `UPDATE` / `DELETE`；对 `content_topic_card` 的写只允许经 T039 的函数 — **FR-041**
- [ ] T044 实现影响清单与影响决定（审计 `decide-marketing-impact`）；影响决定**只写候选行** — **FR-038～FR-040**

### HTTP 与 core（PR 2）

- [ ] T045 handler 加六个端点与 `SourceStatusReader` 适配器（读 `content_source` 的 `status`，按 `workspace_id` + `source_id`）；`router.go` 挂上 — **FR-049**
- [ ] T046 **先写**真实路由用例：`{nodeId}` 与 `{candidateId}` 两个参数都与上下文工作区 id 不同，且两者互不相同；断言 handler 分别用了 URL 里的那两个值 — **FR-049、SC-012**
- [ ] T047 core：六个 client 方法；候选与影响条目两种 zod；未知 `material_gaps[].kind` / `duplicate_risks[].reason` 读成 `unknown` 保留原值；`lead_short` 的 `null` 读成 `"unknown"`；两种形状各三类畸形用例；hooks — **FR-051、SC-015**
- [ ] T048 PR 2 变异 M4、M5、M6、M9，每条先红后还原 — **SC-004、SC-005、SC-007、SC-008**
- [ ] T049 PR 2 验证：「共用：验证命令」全部 + 远程验收；PR 正文单列「采用前不启动创作」的 6 张表行数对比 — **SC-005、SC-013**

---

## PR 3 卡片：页面

| 项 | 内容 |
|---|---|
| **分支** | `claude/033-pr3-marketing-node-page`，PR 2 合并后从 `app-main` 新起 |
| **文件** | plan.md「PR 3」清单；`adapters` 加 D1 第二行 `apps/web/app/[workspaceSlug]/(dashboard)/marketing-nodes/page.tsx` |
| **测试** | **零 UI 单测**（宪法 II）。只有 core 的表单状态纯函数用例、`route-icons.test.ts`（既有覆盖用例，不改）、`locales/parity.test.ts`（既有） |
| **验收** | `manual-ui-todo.md` 全部条目，由用户在浏览器逐条验收；D14-V08 **前半段**（节点 → 候选 → 选题卡 → 既有作品与手工反馈流程）；D14-V08「→ 诊断」一段**未执行（依赖 BO-02）**；「模拟与联网分别标识」**本卡无此对象** |
| **验证** | `pnpm typecheck --force`、`pnpm check:content-boundaries`、core vitest、`locales/parity.test.ts` + 远程验收 `lt-verify.sh <branch> all`。**不用 computer use，不写 UI 单测** |

- [ ] T050 先读 `docs/development/design/README.md`；列出将复用的组件（设置页布局 `SettingsContent` / `SettingsTab` / `SettingsSection` / `SettingsCard` / `SettingsRow`、`Button`、`Input`、`Textarea`、既有下拉与复选组件），不新造控件、不设颜色 — **FR-054**
- [ ] T051 **先写** `marketing-node-form.test.ts` 并确认失败：从当前版本初始化表单时所有字段带上（尤其 `lead_days` 未设置保持未设置、0 保持 0）；整版提交时没动的字段原值不丢；时区默认取品牌时区 — **FR-012、FR-013、SC-002**
- [ ] T052 写 `marketing-node-form.ts`；`packages/views/content/topic-planning/marketing-nodes.tsx`：节点列表（按开始日、显示阶段与「按哪个时区的今天」）、新建 / 编辑表单、导入框（逐行结果）、节点详情（版本历史、候选、影响清单） — **FR-053、FR-017**
- [ ] T053 候选区恒定说明文字（FR-055）；采用按钮旁说明文字（FR-056）；未填写的账号字段显示「未填写」；未知枚举显示原值 — **FR-023、FR-051、FR-055、FR-056、SC-010**
- [ ] T054 页面适配器 `apps/web/app/[workspaceSlug]/(dashboard)/marketing-nodes/page.tsx`：把账号列表与素材列表作为属性传入，与 `topics/page.tsx` 同一做法；采用成功后提供「去选题页」的链接（用 `paths.workspace(slug).topics()`，**用 slug 不用 id**，Issue #191） — **FR-053**
- [ ] T055 [P] 路由登记：`paths.ts` 加 `marketingNodes()`、`route-icons.ts` 加条目、`app-sidebar.tsx` 的 `contentNav` 在 `topics` 之后加一项 — **FR-053**
- [ ] T056 [P] 四语言：`common.json`（页面文案）与 `layout.json`（侧栏标签）× en / zh-Hans / ja / ko；跑 `locales/parity.test.ts`；插值变量不叫 `count` — **FR-057**
- [ ] T057 按实际实现更新 `manual-ui-todo.md` 的入口与组件说明，条目状态全部保持「未执行」 — **FR-058、SC-016**
- [ ] T058 PR 3 验证并在 PR 正文写：UI 影响、手动 UI Todo、零 UI 单测、D14-V08 的三段状态（前半段待手验 / 诊断段未执行（依赖 BO-02）/ 模拟与联网本卡无此对象） — **SC-014、SC-016**

---

## Phase Final: 收尾（每个 PR 各自做）

- [ ] T059 核对「明确不动」清单（plan.md）：`git diff --stat app-main...HEAD` 里不出现 `content-boundaries.json`（除 D1 批准、由本 PR 负责的那一行）、`snapshot.go`、`review-delivery/`、`work-editor/`、`feedback-learning/`、`source-inbox/`、`today/page.tsx` — **FR-052、SC-013**
- [ ] T060 PR 正文按工作流第 10 节写全七节（改动与用途、命令与退出码、变异验证、未验证项、UI 影响、手动 UI Todo、回滚）；回滚写明本 PR 最终迁移号的 `.down.sql` 需逆序另跑，并说明是否实测过可逆 — **SC-011**

---

## Dependencies

```text
Setup (T001–T003)
  └─ PR 1: 迁移 (T004–T011) → 类型 / 日期 (T012 先写 → T013 先写 → T014–T016) → store (T017–T019 先写 → T020–T021 → T022–T023 先写) → HTTP / core (T024–T028) → 变异与验证 (T029–T030)
       └─ PR 2（PR 1 合并后）: 读时计算 (T031 先写 → T032 → T033) → 候选 store (T034–T038 先写 → T039 → T040) → 影响 (T041–T042 先写 → T043 → T044) → HTTP / core (T045–T047) → 变异与验证 (T048–T049)
            └─ PR 3（PR 2 合并后）: T050 → T051 先写 → T052–T056 → T057–T058
每个 PR 末尾: T059–T060
```

**先写并确认失败的清单**：T012、T013、T016、T017、T018、T019、T022、T023、T026、T027、T031、T033、T034、T035、T036、T037、T038、T041、T042、T046、T051。

---

## 验收编号对照

| 验收 | PR 1 | PR 2 | PR 3（手验） | 本卡不覆盖 |
|---|---|---|---|---|
| D14-V01 | T018（节点、导入） | T037（挂卡）、T046 | U-14、U-15 | 账号专属材料：今天不存在（spec Current State 第 6 节），由 T033 负例守住将来 |
| D14-V02 | T013、T019 | T035、T036 | U-03～U-06、U-09～U-11 | — |
| D14-V03 | T017、T023 | T034、T036、T041、T042 | U-12、U-13 | — |
| D14-V08 | — | — | U-16（前半段） | 「→ 诊断」一段（BO-02）；模拟与联网分别标识（本卡没有模拟结果、没有联网） |
