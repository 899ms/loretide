---
description: "Task list for 011 diagnostics panel time semantics and jump reachability"
---

# Tasks: 瀑布时间语义、真实时钟偏差与可达的追踪跳转

**Prerequisites**: spec.md、plan.md、contracts/span-timing.md

**改动文件必须在 plan.md → Source Code 清单内。**

**测试先写**：标注「先写」的必须在实现前完成并**确认失败**。

---

## Phase 1: Setup

- [x] T001 确认本地 PostgreSQL 可用并导出 `LORETIDE_DIAG_TEST_DATABASE_URL`；跳过不得记为通过
- [x] T002 记录基线：未改动时跑验证命令并保存到工作区外。**不提交**，内容进 PR 正文

---

## Phase 2: User Story 1 — 时间语义（P1，缺陷一）

- [x] T003 [US1] **先写** `server/internal/content/diagnostics/span_timing_test.go`：断言 T1 —— 遍历除 `clock_skew` 外的全部场景，相邻事件 `start[i+1] == start[i] + duration[i]`，**0 容差**；单步运行空过。注释写明为何排除 `clock_skew`（contracts/span-timing.md T3）。确认在未修复时失败
- [x] T004 [US1] 修改 `simulator.go`：把 `now = now.Add(duration)` 移到 `Occurred` 盖章**之后**，使 `occurred_at` 为该步**开始**时刻（FR-002）
- [x] T005 [US1] **先写** `packages/core/content/diagnostics/trace-waterfall.test.ts` 新增用例：用**真实形状**（run `93edf840` 的连续链）断言 `buildTraceWaterfall` 相邻行零间隙（FR-004）。**夹具本身不改**——它按「occurred 即开始」构造，与新语义一致
- [x] T006 [US1] 在 `specs/006-diag-trace-waterfall-regression/contracts/trace-waterfall.md` 钉死 `occurred_at` 语义并指向本特性合同（FR-001）
- [x] T007 [US1] 变异验证：把 `now.Add` 改回步进前 → 确认 T1 与零间隙用例变红，改完即还原

---

## Phase 3: User Story 2 — 真实时钟偏差（P2，缺陷二）

- [x] T008 [US2] **先写** `span_timing_test.go` 中的 C2/C4 断言：`clock_skew` 场景经 `Simulate` 后，指定步骤 `Occurred` 早于其父 span；其余 15 个场景不得乱序。确认在未修复时失败
- [x] T009 [US2] 修改 `simulator.go` 的 `clock_skew` 分支：让该步的 `Occurred` 真的往前挪，早于其父 span（FR-006）
- [x] T010 [US2] 在 `trace-waterfall.test.ts` 新增用例：`clock_skew` 形状的输入产出至少一行 `anomaly === "clockSkew"`，且该行偏移**未被夹取**进父区间（FR-007、FR-008）
- [x] T011 [US2] 变异验证：撤掉偏移 → 确认 clockSkew 用例变红，改完即还原

---

## Phase 4: User Story 3 — 可达的跳转落点（P1，缺陷三）

- [x] T012 [US3] 修改 `packages/views/content/diagnostics/index.tsx:1039` 的渲染条件：由 `tab === 3 && !runId && trace` 放宽为有追踪编号即呈现，使瀑布、对象与版本卡片、事件表同屏（FR-010、FR-011、FR-012）。**不新增查询**——事件查询已携带 `trace_id`
- [x] T013 [US3] 改 `specs/008-diag-linkage-and-invariants/manual-ui-todo.md` 的 J-1 / J-4 措辞：技术日志 → 单次追踪，注明「2026-09-15 实施期修正，原措辞与实现不符」（FR-013）
- [x] T014 [US3] 同样改 `docs/development/manual-ui-runbook.md` 的 J-1 / J-4，**两份都改**（FR-013）
- [x] T015 [US3] 在两份手动清单中补记：`008-O-1/O-2/O-3` 此前在 J-1 路径上**不可达**，本次修复后方可验收

---

## Phase 5: Polish

- [x] T016 跑全部验证命令并记录退出码（见 spec 的验证清单），Go 侧报 PASS/SKIP 计数
- [x] T017 核对改动文件全部落在 plan.md → Source Code 清单内；清单外的在 PR 正文单列
- [x] T018 PR 正文写入修复前后的时间轴对照表（用 run `93edf840` 的真实数据）与需用户复验的手动条目

---

## Dependencies

```
Setup (T001-T002)
   └─ US1 时间语义 (T003→T004→T005→T006→T007)
        └─ US2 时钟偏差 (T008→T009→T010→T011)   # 语义上依赖 US1 先修
   └─ US3 跳转落点 (T012→T013→T014→T015)        ∥ 与 US1/US2 独立
        └─ Polish (T016-T018)
```

**US2 依赖 US1**：「早于父 span」的判断必须建立在正确的时间基准上。
**US3 完全独立**：只改渲染条件与文档，不碰时间。

---

## 执行记录（2026-09-15）

T001–T018 全部执行并回勾。与计划不同或需主任务知道的：

- **T003/T008 先写测试确实见了红**：`TestSimulatedStepsAreContiguousInTime`、`TestFirstSimulatedStepStartsAtTheRunBaseline`、`TestClockSkewScenarioProducesOutOfOrderTime` 三条在未修复代码上失败，修复后转绿。
- **T005/T010 的 core 用例修复前后都绿，这是预期的**：缺陷不在瀑布算法里，夹具一直按「occurred 即开始」构造。它们是**回归护栏**（钉住算法不引入间隙、钉住 clockSkew 不被夹取），不是先红后绿的用例。真正抓到缺陷的是 Go 侧。**不把它们算作「变异验证过的覆盖」。**
- **T011 首版变异无效（已修）**：直接删掉 skew 偏移那行会让 `prevStart` 与 `skewed` 变成未使用变量，**编译失败**而非用例变红——编译错误不能证明用例抓得住行为。改成把偏移方向翻正（可编译、行为变化），`TestClockSkewScenarioProducesOutOfOrderTime` 才真正变红。
- **`simulator.go` 在基线上就不是 gofmt 干净的**（`gofmt -l` 在未改动时即列出它）。跑 `gofmt -w` 会把 55 行密集风格炸成 236 行，属无关重构（原则 VIII），**故未跑**；新增行按周围密集风格书写。这是既有状况，不是本次引入。
- **缺陷三无变异验证，且不可能有**：它改的是界面渲染条件，按原则 II 没有自动测试可被变红。验收只能靠用户复验 `008-J-1` 与 `008-O-1`，已在 spec FR-017 与 PR 正文写明。
- **验证命令的一处偏差**：主任务给的 `vitest run content/diagnostics`（目录形式）会跑 **6 个文件 81 条**，其中包含原则 II 明确排除的 `queries.test.tsx`。按文件逐个指定是 **5 文件 80 条**。两者都通过，两个数字都在 PR 正文报出。
