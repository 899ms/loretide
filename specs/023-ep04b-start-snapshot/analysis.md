# 跨件一致性核对（analyze）

**日期**：2026-09-20 ｜ **件**：`spec.md` / `plan.md` / `tasks.md` / `contracts/start-snapshot.md` / `checklists/requirements.md`

**口径**：只核对四份规格件之间是否自洽、以及它们与 `app-main` @ `24ee6ca` 的代码事实是否一致。**不做实现。**

---

## 1. 覆盖矩阵：每条 FR 至少落到一个任务与一条 SC

| FR | plan / contract | tasks | SC |
|---|---|---|---|
| FR-001～005 就绪判定 | contract §3 第 6 条 | T013 T015 T023 | SC-001 SC-007 |
| FR-006～008 选定版本 | contract §1 §3 第 3 条 | T013 T023 | SC-003 |
| FR-009～012 资料范围 | contract §4「三处同名不同物」 | T007 T009 T023 | SC-002 |
| FR-013 十六字段对齐 | contract §4 | T007 T010 | SC-002 |
| FR-014 不可改写 | contract §6 | T014 T017 T029(M3) | SC-002 |
| FR-015 / 015a 记什么 | contract §4「两项记录但不参与判定」 | T007 T010 | SC-002 |
| FR-016～017 事后不变 / 服务端时间 | contract §2 §6 | T014 | SC-002 |
| FR-018 项目可选 | contract §2 | T013 T023 | SC-004 |
| FR-019 越权同形 | contract §3 第 1–4 条 | T013 T019 T020 | SC-003 |
| FR-020 不调模型 | plan「宪法 IX」 | T010 | — |
| FR-021 暂不可用半边 | — | T024 | SC-006 |
| FR-022 九个恒空 | contract §4 | T008 T029(M2) | SC-005 |
| FR-023 迁移硬规则 | plan「原则 V 落到文件」+ contract §7 | T003 T006 | SC-008 |
| FR-024 删除栅栏 | contract §6 | T011 | — |
| FR-025 两个 PR | plan「两个 PR 的分界」 | Phase 分界 | — |
| FR-026 只挂既有组件 | plan | T023 | — |
| FR-027 不写 UI 单测 | plan | T027 | — |
| FR-028 第 12 步 | plan「命名」小节 | T020 | — |

**两处空缺，均为故意**：

- **FR-020 / FR-024 没有对应 SC。** 它们是「不做某事」与「做法内部约束」，可验证但不适合写成用户可见的成功指标。FR-020 由 T010 的纯函数不接触执行器保证；FR-024 由 T011 与 #104 的既有栅栏用例保证。
- **FR-025～028 没有对应 SC**：它们是工程纪律，不是产品结果。

---

## 2. 与代码事实的一致性（逐条回核）

| 规格里的断言 | 代码依据 | 结论 |
|---|---|---|
| 最小条件是四项且只计 `confirmed` | `ip-profile/profile.go:242` `ProfileReadiness` | ✅ 一致 |
| `readiness` 由服务端算好返回 | `handler/content_account_profile.go` `ProfileReadResponse` | ✅ |
| `TopicCard.AccountID` 可空 | `topic-planning/contract.go:53` `*string` | ✅ |
| `BriefRevision.SourceScope` 是自由文本 | `topic-planning/store.go` 无受控集校验 | ✅ |
| `action=start` 已被 EP-04a 占用 | `topic-planning/contract.go:28` `ActionStart` | ✅ |
| 偏好在 `settings["loretide.scope"]`，默认 `all` | `ip-profile/scope.go` `ScopeSettingsKey` / `DefaultScope` | ✅ |
| 预检开关在工作区 `settings["loretide.auto_precheck"]` | `handler/workspace.go:280` | ✅ |
| Grant 无任何存储 | `workspace-core/grant.go` 注释 + 仓库无 grant 表/端点 | ✅ |
| `Snapshot` 十六字段、`Scope` 与 `Preference` 独立 | `diagnostics/contract.go:51` | ✅ |
| `agent-workflow` 未落地 | `pnpm check:diagnostics-contract` 报 `checked 4 landed modules; skipped 8` | ✅ |
| `topic-planning → ip-profile` 依赖已声明 | `scripts/content-boundaries.json` | ✅ 启用既有声明，非新方向 |
| 下一个迁移号是 490 | `server/migrations/` 最大为 489 | ✅ |

---

## 3. 核对中发现并已修正的三处不自洽

1. **FR-014 与 contract §6 冲突。** FR-014 原文是「只插不改：没有更新端点」，而 Q1=A 的做法在 SQL 层开了一条受限 `UPDATE`。已把 FR-014 改写成精确口径（没有更新端点 + 写入只能把空变非空 + 不得改写已存快照），并注明 Q1=B 下它退化为字面意义的「只插」。
2. **SC-002 只数了十六个字段**，但 FR-015 要求记预检开关与中性表达，这两项在 `Snapshot` 里没有对应字段。已新增 **FR-015a** 把它们显式定义为扩展键（并禁止挤进既有字段），SC-002 改为「十六个对齐字段与两个扩展键」。
3. **FR-022 只列了三个字段**，而 contract §4 要求九个恒空。已把 FR-022 改为九个并要求**各有一条**负例，理由写进条文本身。

---

## 4. 未解决但已显式登记的风险

| 风险 | 登记在 | 为什么不在本阶段消解 |
|---|---|---|
| Q1=A 要在 `content_brief_revision` 上开一条受限 UPDATE，与 022 的「只插不改不删」守卫用例正面相关 | contract §6 的引用块、tasks T017 | 这是 Q1 的**真实代价**，必须由主任务权衡「无新表」与「只插不改不容破例」，不该由执行会话单方面决定 |
| 「已开始的运行继续看到旧配置」本卡只有结构保证 | spec Assumption 3、plan 末节、tasks T031 | `agent-workflow` 未落地，没有运行实体读它。与 EP-04a 对 §5.3 的处理口径一致 |
| `budget` / `timeout_ms` 填 0，而简报的 `cost_limit` / `time_limit` 是自由文本 | contract §4 表格 | 把自由文本映射成整数需要一个受控集，那是改 022 的契约，超出本卡范围 |
| 开始界面从哪里进入 | tasks T026 | 留给主任务定，本 PR 不加入口（与 specs/017 的 L-09 同一处理） |

---

## 5. 宪法逐条

| 原则 | 判定 | 依据 |
|---|---|---|
| II 不写 UI 单测 | ✅ | FR-027、T027；界面项进 `manual-ui-todo.md`（T026） |
| III 模块边界 | ✅ | 启用既有声明的依赖方向；T012 / T018 各跑一次 `check:content-boundaries` |
| V 无外键、索引规则 | ✅ | 加列不加表、单语句、无索引；T006 显式证明「不需要改删除清单」而不是默认它不需要 |
| VIII 范围 | ✅ | 九个字段留位不接入；EP-04c/d、EP-06、agent-workflow 全在 Out of Scope |
| IX 执行器禁用 | ✅ | `executor` 记「禁用」本身，不起进程 |
| X 打勾不等于验收 | ✅ | 「本卡验不了的一件事」在 plan 与 tasks T031 各写了一遍 |
