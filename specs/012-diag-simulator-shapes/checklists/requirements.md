# Specification Quality Checklist: 诊断模拟器的数据形态

**Purpose**: Validate specification completeness and quality before proceeding to planning
**Created**: 2026-09-15
**Feature**: [spec.md](../spec.md)

## Content Quality

- [x] No implementation details (languages, frameworks, APIs)
- [x] Focused on user value and business needs
- [x] Written for non-technical stakeholders
- [x] All mandatory sections completed

## Requirement Completeness

- [x] No [NEEDS CLARIFICATION] markers remain
- [x] Requirements are testable and unambiguous
- [x] Success criteria are measurable
- [x] Success criteria are technology-agnostic (no implementation details)
- [x] All acceptance scenarios are defined
- [x] Edge cases are identified
- [x] Scope is clearly bounded
- [x] Dependencies and assumptions identified

## Feature Readiness

- [x] All functional requirements have clear acceptance criteria
- [x] User scenarios cover primary flows
- [x] Feature meets measurable outcomes defined in Success Criteria
- [x] No implementation details leak into specification

## Notes

### 关于「No implementation details」

Current State 大量引用文件、行号与符号名（`simulator.go` 的 `Child(ctx)` 链、`service.go:25` 的无条件 `Evaluate`、`index.tsx:574` 的 `limit: "25"`）。这是刻意的，不计为违反：本特性要回答的问题就是「**今天为什么造不出**」，不指到具体那一行就只能说「大概是设计如此」。Requirements 与 Success Criteria 两节不含符号名。与 005 / 007 / 008 / 009 / 010 同一口径。

### 逐条核实的结果改变了范围

任务描述列了 (a)～(h)，逐条核实后**两条要移出**：

| 条目 | 核实结果 | 处理 |
|---|---|---|
| **008-O-2** 追踪内事件都没有版本 | **今天已经可以做**。面板「触发页面错误（测试）」按钮走 `Service.ClientError`，该事件不设 `Version` 且自带全新 `Trace` | 移出（FR-021），只改 runbook 备注 |
| **006-L-5** 原故障运行不可读 | 要的是「被引用的运行已被保留期裁剪或跨工作区不可读」——那是**保留期与授权**行为。让模拟器造一个「不可读的运行」等于让它写一条本不该存在的记录 | 移出（FR-020），改由保留期配置或跨工作区引用验证 |

硬约束原文要求「若某一项确实不该由模拟器承担，MUST 在规格里显式排除并说明理由，不要硬凑」——这两条正是。

### 一处数目对不上，已抬到 Current State 第 0 节

描述说 **19 条**，(a)～(h) 实际点名 **15 条**。差的 4 条是哪几条未知。规格覆盖点名的 15 条并把差异写进 FR-001 与交付记录要求，而不是假装 15 就是 19。**不补全的风险不是漏做，而是交付后仍有 4 条卡在同一个原因上，且没人知道是哪 4 条。**

### 三处 Q 按推荐值暂定，不阻塞

| 项 | 问题 | 推荐值 |
|---|---|---|
| Q1 | 19 与 15 的差额是哪 4 条 | 覆盖点名的 15 条，差异写进交付记录 |
| Q2 | 新形态在 `Scenarios` 里怎么标注才不让 §7 对表口径变糊 | 给 `Scenario` 加分类字段，让归属成为**结构里的事实**而非文档约定 |
| Q3 | 第 5 类注入点的门禁 + 两条移出 | 复用既有 `testEnabled` 判定，装配处注入；两条移出如上 |

三者都**不改变任何 FR 的可测性**。**不计为清单未通过项。**

### US2 的优先级说明

US2（隔离门禁）与 US1 同为 P1，但在冲突时**US2 优先**。本特性做的事情本质上是「让系统能产出反常数据」——一个能在生产伪造 `passed` 的开关，比 19 条跑不了的验收项危险得多。FR-017 因此要求**每一类新形态各有一条负例断言**，而不是一条笼统的「门禁有效」。

### specify 阶段结束时的下一步（已执行完）

`/speckit-clarify`（三处 Q 待主任务裁决，按推荐值暂定不阻塞）→ `/speckit-plan` → `/speckit-tasks` → `/speckit-analyze`。结果见下方「Phase 1 之后的复核」。

---

## Phase 1 之后的复核（`/speckit-analyze`，2026-09-15）

跨产物一致性检查：`spec.md` / `plan.md` / `research.md` / `data-model.md` / `contracts/` / `quickstart.md` / `tasks.md`。

### 覆盖

- **FR-001 ～ FR-023 全部有对应任务**，无孤儿需求。
- **SC-001 ～ SC-010 全部可被某条任务验证**，无不可测的成功标准。
- 九个 shape id 在 `data-model` / `contracts` / `quickstart` / `tasks` 四份产物里**逐字一致**。
- 引用的 FR / SC 编号全部存在，无指向空号的引用。

### plan 阶段发现并已整改的三处（HIGH）

| # | 问题 | 整改 |
|---|---|---|
| 1 | **FR-005 原本只覆盖一半。** 运行详情读取路径 `GetRun` → `Query(Limit: 100)` 把事件截在 100 条，而瀑布折叠阈值是 200。只产出 250 个 span 的话，`006-W-7` / `W-8` **交付后仍然跑不了**——正是 SC-001 禁止的结果 | spec 新增 Current State §1a 逐环节列出该链路；FR-005 扩为同时覆盖读取一侧；plan 的 Complexity Tracking 记下这处超出「只加场景」最小面的改动及其理由（原则 VIII 的例外条款）；tasks 拆出 T014 / T015 |
| 2 | **Q3 的暂定值比需要的复杂。** 把 sink 失败做成一个 shape 场景后，它走 `Simulate` 的既有门禁，不需要装配注入、不需要新开关、不需要新环境变量，恢复也不需要动作。`Store.CommitRun(…, failAudit bool)` 已经是这个模式 | research D5；spec 的 Q3 条目下补记 plan 阶段修正。**FR-015 / FR-016 的要求不变，只是被平凡满足**。主任务若坚持原暂定值，装配注入同样可行，差异已记录 |
| 3 | **constitution 引用有误。** 任务描述与规格把「隔离实例之外注入被拒绝」记作**原则 VII**；核实 `.specify/memory/constitution.md` 后，原则 VII 是「UI reuses Multica, it does not reinvent it」 | 实际出处是 constitution 的 **Development Workflow** 一节与 `docs/development/ai-collaboration.md`，加上**原则 IX**。**需求不变，引文更正**（spec US2 / FR-017、plan「Constitution Check」、research D10）。不更正的后果：下一个人照 VII 去翻会翻到一条不相干的 UI 规则，然后开始怀疑负例该断言什么 |

### 顺带记录，不在本特性范围

- `Service.Export` 的 `Filter{Limit: 100}` 会把 250 span 运行的导出截在 100 条技术事件。**既有行为**，按原则 VIII 不在本特性修，但 contracts 与 runbook 都写明，免得下一个人以为导出是全量。
- 一次 `shape_deep` 触发 250 个事务 + 251 次 `PruneTechnical`，秒级。不做批量写入优化（同样是既有行为），但 runbook **必须**写明这一步会慢——跑矩阵的人看到界面停住会刷新，然后拿到一个写了一半的运行。

### 条目编号更正

任务描述写作 `V11-01` / `V11-02`；`specs/002-diag-package-stream-recovery/manual-ui-todo.md` 里的实际编号是 **`V11-1` / `V11-2`**。全部产物已按实际编号统一。

### 下一步

Draft PR（base `app-main`）→ 主任务裁决 Q1 / Q2 / Q3 → 合入 → **等 `specs/011` 合入后**再派实施，两者都动 `simulator.go`。实施第一步是 T001 / T002：以 011 合入后的代码重新核实 Current State。
