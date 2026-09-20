# 跨件一致性核对（analyze）

**第二轮**，2026-09-20，**裁决后重跑**（Q1=B / Q2=A / Q3=A，附带一问接受，T026 入口已定）。

**件**：`spec.md` / `plan.md` / `tasks.md` / `contracts/start-snapshot.md` / `checklists/requirements.md`

**口径**：只核对四份规格件之间是否自洽、以及它们与 `app-main` @ `24ee6ca` 的代码事实是否一致。**不做实现。**

---

## 0. 第一轮之后变了什么

第一轮是按推荐值 Q1=A 写的。裁决取 **B**，以下六处随之反转，**已全部改完**：

| 处 | Q1=A | Q1=B（现行） |
|---|---|---|
| 存储 | 给 `content_brief_revision` 加 `snapshot jsonb`，一个加列迁移、无索引 | **新表 `content_start_snapshot` + 三个 CONCURRENTLY 索引**，四个迁移文件 |
| FR-014 | 「写入只能把空变非空」 | **字面意义的只插不改**；新增 **FR-014a**：一版简报可多次开始，端点**不幂等** |
| 决策顺序 | 七条，第 7 条「已开始过」→ 409 | **六条**，**没有 409 分支** |
| 迁移规则 | 不触发 R6，删除清单无需改动 | **R5 / R6 / 删除清单全部生效**（FR-023 / FR-023a） |
| 上游改动 | 只有 `router.go` | `router.go` **与** `cmd/migrate/main.go`，**同一个 `upstream:` 提交** |
| 读回端点 | 搭既有的简报读取端点 | **两条新 GET**（快照成了独立实体） |

第一轮记录的那条风险「Q1=A 要在简报表上开一条受限 UPDATE，与 022 的 A6 守卫正面相关」——**裁决正是以此为由选的 B，该风险已消解**。

---

## 1. 覆盖矩阵：每条 FR 至少落到一个任务与一条 SC

| FR | plan / contract | tasks | SC |
|---|---|---|---|
| FR-001～005 就绪判定 | contract §3 第 6 条 | T017 T019 T026 | SC-001 SC-007 |
| FR-006～008 选定版本 | contract §1 §3 第 3 条 | T017 T026 | SC-003 |
| FR-009～012 资料范围 | contract §4「三处同名不同物」 | T010 T012 T026 | SC-002 |
| FR-013 十六字段对齐 + 稳定键 | contract §4 §5 | T010 T013 T014 | SC-002 |
| **FR-014 只插不改** | contract §6 | **T015 守卫** T018(a) T033(M3) | SC-002 |
| **FR-014a 可多次开始、不幂等** | contract §3「没有第 7 条」 | **T018(b)** | **SC-002a** |
| FR-015 / 015a 记什么 + 扩展键 | contract §4「两个扩展键」 | T010 T013 | SC-002 |
| FR-016～017 事后不变 / 服务端时间 | contract §2 §6 | T018(a) | SC-002 |
| FR-018 项目可选 | contract §2 §5（空串是真实状态） | T017 T026 | SC-004 |
| FR-019 越权同形 | contract §3 第 1–4 条 | T017 T022 T023 | SC-003 |
| FR-020 不调模型 | plan「宪法 IX」 | T013 | — |
| FR-021 暂不可用半边 | — | T027 | SC-006 |
| FR-022 九个恒空 | contract §4 | T011 T033(M2) | SC-005 |
| **FR-023 R1–R6** | plan「原则 V 落到文件」六行表 + contract §7 | **T002 T003 T004 T005 T009 T023(b) T033(M4)** | SC-008 |
| **FR-023a 删除清单** | contract §7 | **T008** | SC-008 |
| FR-024 删除栅栏 | contract §6 | T014 | — |
| FR-025 两个 PR | plan「两个 PR 的分界」 | Phase 分界 | — |
| FR-026 只挂既有组件 | plan | T026 | — |
| FR-027 不写 UI 单测 | plan | T031 | — |
| FR-028 第 12 步 | plan「命名」小节 | T023 | — |

**四处空缺，均为故意**（与第一轮同）：FR-020 / FR-024 是「不做某事」与做法内部约束，可验证但不适合写成用户可见的成功指标；FR-025～028 是工程纪律，不是产品结果。

---

## 2. 与代码事实的一致性（逐条回核）

| 规格里的断言 | 代码依据 | 结论 |
|---|---|---|
| 最小条件是四项且只计 `confirmed` | `ip-profile/profile.go:242` `ProfileReadiness` | ✅ |
| `readiness` 由服务端算好返回 | `handler/content_account_profile.go` `ProfileReadResponse` | ✅ |
| `TopicCard.AccountID` 可空 | `topic-planning/contract.go:53` `*string` | ✅ |
| `BriefRevision.SourceScope` 是自由文本 | `topic-planning/store.go` 无受控集校验 | ✅ |
| `action=start` 已被 EP-04a 占用 | `topic-planning/contract.go:28` `ActionStart` | ✅ |
| 偏好在 `settings["loretide.scope"]`，默认 `all` | `ip-profile/scope.go` | ✅ |
| 预检开关在工作区 `settings["loretide.auto_precheck"]`，默认 `true` | `handler/workspace.go:280` | ✅ |
| Grant 无任何存储 | `workspace-core/grant.go` 注释 + 仓库无 grant 表/端点 | ✅ |
| `Snapshot` 十六字段、`Scope` 与 `Preference` 独立 | `diagnostics/contract.go:51` | ✅ |
| `agent-workflow` 未落地 | `check:diagnostics-contract` 报 `checked 4 landed modules; skipped 8` | ✅ |
| `topic-planning → ip-profile` 依赖已声明 | `scripts/content-boundaries.json` | ✅ 启用既有声明，非新方向 |
| **下一个迁移号是 490，本卡占 490–493** | `server/migrations/` 最大为 489 | ✅ |
| **R5 从迁移 483 起生效，490 在范围内** | `internal/migrations/content_constraints_test.go:38` `implicitIndexRuleFloor = 483` | ✅ 建表迁移不得含 PK/UNIQUE |
| **R6 要求建索引的 up 迁移逐条登记，名字逐字相同** | `cmd/migrate/migrate_mul5999_index_retry_test.go` + `internal/migrations` 的 R6 | ✅ 491/492/493 登记，**490 不登记** |
| **新表必须进删除清单** | `handler/workspace_delete_manifest_test.go`（`content_brief_revision` 等六张 `content_` 表都在） | ✅ 标 `workspaceDelete` |
| **建表不用 PK 是有先例的** | 488 / 489 分别给 `content_topic_card` / `content_brief_revision` 单独建 id 唯一索引 | ✅ 照抄 |

---

## 3. 本轮核对中发现并已修正的四处不自洽

1. **决策顺序还留着第 7 条 409。** Q1=B 下重复开始是合法行为，用「已存在快照」拒绝会把正常产品行为变成错误。已删去第 7 条，并在 contract §3 显式写「**没有第 7 条**」和「本端点不是幂等的」，免得实施时有人「顺手加个幂等保护」把 Q1=B 换来的行为做没。对应新增 FR-014a 与 T018(b)。
2. **读回端点仍搭在既有的简报读取端点上。** Q1=A 下快照是简报的一个字段，那样可以；Q1=B 下它是独立实体，必须有自己的读取路径。已改为两条新 GET，并因此把第 12 步的用例从两个参数扩到**三个**——`{snapshotId}` 是新的一类 id，必须单独有一条「填别的工作区的 id 拿到与不存在同形的拒绝」。
3. **上游改动只写了 `router.go`。** R6 要求三条索引登记进 `cmd/migrate/main.go`，那也是上游文件。已在 plan、contract §7、tasks T023 三处写明**两个文件、同一个 `upstream:` 提交**。
4. **T026 的待裁定项已被裁决取代。** 入口 = 选题卡详情页 `start` 之后的区块，列表不加。已从 `manual-ui-todo` 的任务里删去该待裁定项（T028 写明入口），并据此**加了第三个索引 493**（`workspace_id, topic_card_id, created_at DESC`）——那个区块要显示「这张卡一共开始过几次」，没有这条索引就是按卡全表扫。

> 第 4 点是裁决反过来影响了存储设计的一个例子：入口定在详情页，就多出一个「按卡列出」的查询，索引数从两个变三个。写在这里，免得后来的人以为 493 是多余的。

---

## 4. 未解决但已显式登记的风险

| 风险 | 登记在 | 为什么不在本阶段消解 |
|---|---|---|
| 与拆分卡的「**无新表**」一句冲突 | contract §1、plan Summary、spec 裁决记录 | 裁决已权衡：给简报加列要破 022 的 A6 守卫，那条不容破例。**拆分卡不追改**，冲突记在合同里 |
| 「已开始的运行继续看到旧配置」本卡只有结构保证 | spec Assumption 3、plan 末节、tasks T035 | `agent-workflow` 未落地，没有运行实体读它。与 EP-04a 对 §5.3 的口径一致 |
| `budget` / `timeout_ms` 填 0，而简报的 `cost_limit` / `time_limit` 是自由文本 | contract §4 表格 | 把自由文本映射成整数需要一个受控集，那是改 022 的契约，超出本卡范围 |
| 端点不幂等 | contract §3、FR-014a | 这是 Q1=B 的**设计结果**，不是缺陷。页面在飞行中禁用按钮；服务端不得用「已存在」去拒绝 |

---

## 5. 宪法逐条

| 原则 | 判定 | 依据 |
|---|---|---|
| II 不写 UI 单测 | ✅ | FR-027、T031；界面项进 `manual-ui-todo.md`（T030） |
| III 模块边界 | ✅ | 启用既有声明的依赖方向；T016 / T021 各跑一次 `check:content-boundaries` |
| V 无外键、索引规则 | ✅ | R1–R6 逐条落到 plan 的六行表与 T002–T005、T009；T033 的 M4 用一次**可编译**变异证明 R6 真的会红 |
| VIII 范围 | ✅ | 九个字段留位不接入；EP-04c/d、EP-06、agent-workflow 全在 Out of Scope |
| IX 执行器禁用 | ✅ | `executor` 记「禁用」本身，不起进程 |
| X 打勾不等于验收 | ✅ | 「本卡验不了的一件事」在 plan 与 tasks T035 各写了一遍 |
