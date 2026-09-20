# 跨件一致性核对（analyze）

**日期**：2026-09-20 ｜ **件**：`spec.md` / `plan.md` / `tasks.md` / `contracts/work-and-versions.md` / `checklists/requirements.md`

**口径**：只核对四份规格件之间是否自洽、以及它们与 `app-main` @ `6085983` 的代码事实是否一致。**不做实现。**

---

## 1. 覆盖矩阵：每条 FR 至少落到一个任务与一条 SC

| FR | plan / contract | tasks | SC |
|---|---|---|---|
| FR-001～004 作品容器 | contract §1 `content_work` | T003 T016 T022 | SC-001 SC-010 |
| FR-005～006 文档与受控类型 | contract §1 §2 | T005 T012 T015 T024 | SC-001 |
| FR-007 编辑副本 | contract §1「为什么是列不是表」 | T005 T016 | SC-002 |
| **FR-008 只插不改不删** | contract 不变量 + §3 | **T014 守卫** T024 T035(M2) | **SC-004** |
| FR-009 自动保存不产生版本 | contract §3「自动保存」 | **T023** T035(M1) | **SC-002** |
| FR-010 `generated` 留位不产生 | contract §2 §4 | **T013** T035(M4) | **SC-006** |
| FR-011 恢复产生新版本 | contract §3「恢复」 | T019 T020 T035(M2) | SC-003 |
| **FR-012 版本号并发** | plan Complexity + contract §5（501 索引） | **T017 T018** T035(M3) | **SC-005** |
| FR-013 服务端时间 | contract §1 | T018 | — |
| FR-014 空版本合法 | contract §3 | T012 | — |
| FR-015 rune 计数 | — | T012 | — |
| FR-016 采用为基线 | contract §3「采用」 | T019 T020 | SC-003 |
| FR-017～019 AI 三入口 | contract §4 | T031 | SC-007 |
| FR-020 越权同形 | contract §3 决策顺序 | T022 T026 | SC-010 |
| FR-021 删除栅栏 | plan「原则 V」 | T016 | — |
| **FR-022 R1–R6** | plan 六行表 + contract §5 | **T003 T004 T005 T006 T008 T009 T011 T026(b) T035(M5)** | **SC-008** |
| FR-023 删除清单 | contract §5 | **T010** | **SC-009** |
| FR-024 合同三条 / 4→5 | plan「模块边界」 | T021 T025 | — |
| FR-025 两个 PR | plan「两个 PR 的分界」 | Phase 分界 | — |
| FR-026 只挂既有组件 | plan | T029 T032 | — |
| FR-027 不写 UI 单测 | plan | T033 | — |
| FR-028 第 12 步 | plan「端点」 | T026 | — |

**五处空缺，均为故意**：FR-013 / FR-014 / FR-015 是输入规则，有任务无 SC——它们是「不会出错」而不是用户可见的结果；FR-021 是做法内部约束；FR-025～028 是工程纪律。与 022/023 两卡的处理一致。

---

## 2. 与代码事实的一致性（逐条回核）

| 规格里的断言 | 代码依据 | 结论 |
|---|---|---|
| `work-editor` 已登记，依赖**只有** `workspace-core` / `diagnostics` | `scripts/content-boundaries.json` | ✅ 这正是 Q2 存在的原因 |
| `server/internal/content/` 只有四个目录 | 目录列表 | ✅ 落地数 4→5 |
| `check:diagnostics-contract` 现报 `checked 4 landed modules; skipped 8` | 实跑 | ✅ |
| `StartSnapshot` 带 `topic_card_id` / `brief_revision_id` / `account_id` / `project_id` | `topic-planning/start.go:55` | ✅ 所以作品只引用 `snapshot_id` 就够 |
| `TopicCard.AccountID` 可空 | `topic-planning/contract.go:60` `*string` | ✅ |
| append-only 模板与它的理由 | `479_content_account_revision.up.sql` 注释 | ✅ |
| **479 有 `PRIMARY KEY`，但 R5 从 483 起生效** | `internal/migrations/content_constraints_test.go` `implicitIndexRuleFloor = 483` | ✅ **新表不能照抄那一行**，plan 与 contract 都写明了 |
| 受控集做法 = Go 枚举 + 库 `CHECK` | `477_content_account.up.sql:22`、`483_content_topic_card.up.sql:16` | ✅ |
| 版本号两步写法与它的理由 | `topic-planning/store.go:365-402`（Issue #109 的注释原文） | ✅ |
| append-only 守卫的形状 | `TestBriefStoreHasNoUpdateOrDeletePath`（022）、`TestStartSnapshotStoreHasNoUpdateOrDeletePath`（023） | ✅ 含「必须有 INSERT」那半条 |
| 下一个迁移号是 494 | `server/migrations/` 最大为 493 | ✅ 本卡占 494–501（+1 若加第六个索引） |
| 删除链与清单的位置 | `pkg/db/queries/workspace_delete.sql`、`handler/workspace_delete_manifest_test.go` | ✅ |
| 写路径持 `LockForContentDiagnosticWrite` | `topic-planning/store.go:77` `begin()` | ✅ |

---

## 3. 核对中发现的三处，已处理

1. **`work-editor` 的依赖里没有 `topic-planning`。** 这不是规格写漏，是登记表的事实，而作品必须挂在选题卡上。已升格为 **Q2**，并给出「只存字符串不 import」的推荐值——它不动登记表，也不把依赖图变宽。
2. **版本表缺一个以 `workspace_id` 打头的索引。** 写 contract §5 时才发现：`version_id` 唯一索引与 `(artifact_id, revision)` 唯一索引都不以 `workspace_id` 打头，**工作区删除按 `workspace_id` 删版本表会走全表扫**，而删除要在一个事务里完成。已作为 **Q1 的附带问题 2** 提出（推荐加第六个索引），并在 tasks 里单列为 T007 —— 不裁决就没有这个任务，而不是默默加上。
3. **`restored` / `adopted` 是否该并入 `edited`。** 已在 contract §2 里给出理由并定下不并：来源是历史侧栏第一眼的信息，压成一个值再让读者去看另一列才知道发生了什么，是把信息藏起来。

---

## 4. 未解决但已显式登记的风险

| 风险 | 登记在 | 为什么不在本阶段消解 |
|---|---|---|
| **Q3 需要 SOP §7.1 原文** | spec Q3、tasks T001、本节 | 本检出没有文档仓库。暂定状态集已写进 spec 与 contract 以便不阻塞；**改状态集只是一次 `CHECK` 迁移 + 一个 Go 枚举，不动表结构**，所以晚一点裁决的代价有界 |
| 「已审核/交接/发布版本永不删除」只有结构保证 | spec Assumption 4、plan 末节、tasks T037 | 本卡不落地审核与交接。能验的是「没有删除路径」，不是「一条已发布版本挺过了某次清理」 |
| 「EP-08 接上不必改调用方」今天无法证明 | plan 末节、tasks T037 | 真正的证明是 EP-08 落地时没有改本卡的接口——那时候才知道。**不得拿 SC-006 当它的验收** |
| 文档类型加一种要改 `CHECK` | spec Assumption 1、contract §2 | 刻意：类型集合的变化应当是一次可审阅的改动 |
| 附件引用清单完全不碰、不预留列 | spec Out of Scope、contract §4 | 它需要素材实体（W-03）。加列比改列容易，预留一个形状未知的列更糟 |

---

## 5. 宪法逐条

| 原则 | 判定 | 依据 |
|---|---|---|
| II 不写 UI 单测 | ✅ | FR-027、T033；界面项进 `manual-ui-todo.md` |
| III 模块边界 | ✅ | Q2=A 不动登记表；T021 / T025 各跑一次两项 check；落地数 4→5 是显式验收项 |
| IV 服务端/客户端状态分离 | ✅ | 自动保存走 mutation；**编辑中的文本是客户端状态**，不进 Query 缓存（plan Constitution Check 已写明） |
| V 无外键、索引规则 | ✅ | R1–R6 逐条落到 plan 六行表与 T003–T011；T035 的 M5 用一次**可编译**变异证明 R6 真的会红 |
| VIII 范围 | ✅ | AI 三入口只留位；审核/交接/发布只留状态值；附件清单完全不碰 |
| IX 执行器禁用 | ✅ | `generated` 有负例钉住「没有产生它的路径」，扫的是路径不是当前数据 |
| X 打勾不等于验收 | ✅ | 两件验不了的事在 plan 与 tasks T037 各写了一遍 |
