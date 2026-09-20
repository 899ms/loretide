# 跨件一致性核对（analyze）

**第二轮**，2026-09-21，**裁决后重跑**（Q1=A + 两个附带问题、Q2=A、Q3 严格按 SOP §7.1 原文）。

**件**：`spec.md` / `plan.md` / `tasks.md` / `contracts/work-and-versions.md` / `checklists/requirements.md`

**口径**：只核对四份规格件之间是否自洽、以及它们与 `app-main` @ `6085983` 的代码事实是否一致。**不做实现。**

---

## 0. 第一轮之后变了什么

第一轮的状态集与来源集是我按 Issue 推断的**暂定值**。拿到 §7.1 原文后，以下七处反转，**已全部改完**：

| 处 | 暂定 | 按原文（现行） |
|---|---|---|
| 状态集 | `drafting` / `in_review` / `revising` / `approved` / `handed_off` / `published` | **只有 `working` / `saved`**，而且它们属**编辑副本**，不属作品 |
| 作品状态列 | 有 | **没有**（FR-004）。`drafting…published` 是 §7.1 其它行的对象 |
| 版本来源 | 四个（含 `restored`） | **恰好三个** `generated` / `edited` / `adopted` |
| 「恢复」 | 第四种来源 | **动作**：`source = edited`、`action = restored`、`restored_from` |
| 「采用」 | 只有来源 | 来源与动作都是 `adopted`，加 `adopted_from` |
| 列数 | 一列 `source` | **两列** `source` + `action`（§7.1「来源与动作记录」） |
| 索引 / 迁移 | 五个 / 八个 | **七个 / 十个**（494–503） |

第一轮登记的风险「版本表缺一个以 `workspace_id` 打头的索引」**已由裁决消解**（附带问题 2 = 要）。

---

## 1. 我在本轮补的两处，和为什么不算扩大范围

1. **`content_work` 也缺一个以 `workspace_id` 打头的索引。** 裁决点名的是**版本表**，但 `content_work_id_unique_idx (work_id)` 同样不以 `workspace_id` 打头，删除链会扫全表——**同一个论证**。而且「列出一张卡下的作品」本来就需要 `(workspace_id, topic_card_id, created_at DESC)`，一个索引服务两件事。这是补上一个**本来就漏了的读路径索引**，不是新需求。已升为 **FR-021a**（每张新表都要有一个 `workspace_id` 打头的索引）与 **SC-008a**。
2. **一次普通保存的 `action` 值。** 裁决点名了 `restored` 与 `adopted` 两个动作；普通保存需要第三个值，我取 **`saved`**（与编辑副本状态的用词一致）。**已在 spec 的裁决记录与 contract §2 两处标明这是我补的**，若应为空串，改动是一个枚举值。

---

## 2. 覆盖矩阵：每条 FR 至少落到一个任务与一条 SC

| FR | plan / contract | tasks | SC |
|---|---|---|---|
| FR-001～003 作品容器 | contract §1 `content_work` | T003 T016 T022 | SC-001 SC-010 |
| **FR-004 作品无状态列** | contract §1「作品没有状态列」 | **T003**（注释写明为什么） | — |
| FR-005～006 文档与受控类型 | contract §1 §2 | T005 T012 T015 T024 | SC-001 |
| FR-007 编辑副本 | contract §1 | T005 T016 | SC-002 |
| **FR-007a `working` / `saved`** | contract §2「编辑副本状态」 | T005 T012 T023 | **SC-002** |
| **FR-007b 状态存储且与重算一致** | contract §2「为什么存而不是重算」 | **T016a** T035(M7) | **SC-006a** |
| FR-008 只插不改不删 | contract 不变量 + §3 | T014 T024 T035(M2) | SC-004 |
| FR-009 自动保存不产生版本 | contract §3 | T023 T035(M1) | SC-002 |
| **FR-010 来源恰好三个** | contract §2 | T012 T012a T013 T035(M4) | SC-006 |
| **FR-010a 动作是第二列** | contract §2「版本动作」 | **T012a** T030 T035(M6) | SC-003 SC-003a |
| **FR-011 恢复 = (edited, restored)** | contract §3「恢复」 | T019 T020 T035(M6) | **SC-003** |
| FR-012 版本号并发 | plan Complexity + contract §5（502 索引） | T017 T018 T035(M3) | SC-005 |
| FR-013 服务端时间 | contract §1 | T018 | — |
| FR-014 空版本合法 | contract §3 | T012 | — |
| FR-015 rune 计数 | — | T012 | — |
| **FR-016 采用 = (adopted, adopted)** | contract §3「采用」 | T019 T020 | **SC-003a** |
| FR-017～019 AI 三入口 | contract §4 | T031 | SC-007 |
| FR-020 越权同形 | contract §3 决策顺序 | T022 T026 | SC-010 |
| FR-021 删除栅栏 | plan「原则 V」 | T016 | — |
| **FR-021a 每表一个 `workspace_id` 打头索引** | contract §5 表格 | **T004 T006 T009 T011a** | **SC-008a** |
| FR-022 R1–R6 | plan 六行表 + contract §5 | T003–T009 T011 T026(b) T035(M5) | SC-008 |
| FR-023 删除清单 | contract §5 | T010 | SC-009 |
| FR-024 合同三条 / 4→5 | plan「模块边界」 | T021 T025 | — |
| FR-025 两个 PR | plan「两个 PR 的分界」 | Phase 分界 | — |
| FR-026 只挂既有组件 | plan | T029 T032 | — |
| FR-027 不写 UI 单测 | plan | T033 | — |
| FR-028 第 12 步 | plan「端点」 | T026 | — |

**六处空缺，均为故意**（与第一轮同口径）：FR-004 / FR-013 / FR-014 / FR-015 是「不会出错」而非用户可见结果；FR-021 是做法内部约束；FR-025～028 是工程纪律。

---

## 3. 与代码事实的一致性（逐条回核）

| 规格里的断言 | 代码依据 | 结论 |
|---|---|---|
| `work-editor` 依赖**只有** `workspace-core` / `diagnostics` | `scripts/content-boundaries.json` | ✅ Q2=A 不动它 |
| `server/internal/content/` 只有四个目录；`checked 4 landed modules` | 实跑 | ✅ 落地数 4→5 |
| `StartSnapshot` 带 `topic_card_id` / `brief_revision_id` / `account_id` / `project_id` | `topic-planning/start.go:55` | ✅ 作品只引用 `snapshot_id` |
| `TopicCard.AccountID` 可空 | `topic-planning/contract.go:60` | ✅ |
| **479 有内联 `PRIMARY KEY`，R5 从 483 起生效** | `content_constraints_test.go` `implicitIndexRuleFloor = 483` | ✅ 新表不抄那一行 |
| 受控集做法 = Go 枚举 + 库 `CHECK` | `477:22`、`483:16` | ✅ 四个受控集都照此 |
| 版本号两步写法与理由 | `topic-planning/store.go:365-402`（Issue #109 注释） | ✅ |
| append-only 守卫形状（含「必须有 INSERT」） | 022 / 023 的同名用例 | ✅ |
| **下一个迁移号是 494** | `server/migrations/` 最大 493 | ✅ 本卡占 **494–503** |
| 删除链与清单的位置 | `workspace_delete.sql`、`workspace_delete_manifest_test.go` | ✅ |
| 写路径持 `LockForContentDiagnosticWrite` | `topic-planning/store.go:77` | ✅ |

---

## 4. 本轮核对中发现并已修正的四处不自洽

1. **`plan.md` 的索引清单还写着五个、`cmd/migrate` 还写五条登记**，而裁决加了索引。已改为七个 / 七条，并同步了 Project Structure 的迁移文件名与编号（494–503）。
2. **`contracts` 的 `CHECK (source IN (...))` 还含 `restored`。** 已改为恰好三个，并新增 `action` 的 `CHECK`。
3. **`tasks` 的 T007 还是「视裁决决定是否存在」。** 裁决已定要加，T007 改为实在的建表任务、T009 是那条索引，编号整体顺移。
4. **`spec` 的 Edge Case「编辑副本与最新版本不一致」只说了它不是错误**，没说它**就是** `working`。已补——那句话现在指向一个受控值而不是一种感觉。

---

## 5. 未解决但已显式登记的风险

| 风险 | 登记在 | 为什么不在本阶段消解 |
|---|---|---|
| 普通保存的 `action = saved` 是我补的值 | spec 裁决记录、contract §2、本节 | 裁决没点名；标明比默默选一个好，改动是一个枚举值 |
| 编辑副本状态是**存**的，两个真相可能不一致 | spec FR-007b、contract §2、tasks T016a、M7 | 重算要取最新版本全文（列文档时 N 份）。代价用三处写路径的一致性用例兜住 |
| 「已审核、已交接、已发布版本始终保留」只有结构保证 | spec Assumption 4、plan 末节、tasks T037 | 本卡连那几个状态值都不落地。能验的是「没有删除路径」 |
| 「EP-08 接上不必改调用方」今天无法证明 | plan 末节、tasks T037 | 真正的证明是 EP-08 落地时没改本卡接口。**不得拿 SC-006 当它的验收** |
| 文档类型加一种要改 `CHECK` | spec Assumption 1、contract §2 | 刻意：类型集合的变化应当是一次可审阅的改动 |
| 附件引用清单不碰、不预留列 | spec Out of Scope、contract §4 | 需要素材实体（W-03）。预留一个形状未知的列更糟 |

---

## 6. 宪法逐条

| 原则 | 判定 | 依据 |
|---|---|---|
| II 不写 UI 单测 | ✅ | FR-027、T033；界面项进 `manual-ui-todo.md` |
| III 模块边界 | ✅ | Q2=A 不动登记表；T021 / T025 各跑一次两项 check；落地数 4→5 是显式验收项 |
| IV 服务端/客户端状态分离 | ✅ | 自动保存走 mutation；**编辑中的文本是客户端状态**，不进 Query 缓存 |
| V 无外键、索引规则 | ✅ | R1–R6 逐条落到 plan 六行表与 T003–T011a；M5 用一次**可编译**变异证明 R6 真的会红 |
| VIII 范围 | ✅ | AI 三入口只留位；审核/交接/发布**连状态值都不落地**；附件清单完全不碰 |
| IX 执行器禁用 | ✅ | `generated` 的负例扫的是**路径**不是当前数据 |
| X 打勾不等于验收 | ✅ | 两件验不了的事在 plan 与 tasks T037 各写一遍 |
