# Specification Quality Checklist: 营销节点联动选题（033）

**Purpose**: Validate specification completeness and quality before proceeding to planning
**Created**: 2026-09-25
**Feature**: [spec.md](../spec.md)

## Content Quality

- [x] No implementation details (languages, frameworks, APIs)
- [x] Focused on user value and business needs
- [x] Written for non-technical stakeholders
- [x] All mandatory sections completed

**说明**：本仓规格按 `docs/development/spec-kit-workflow.md` 第 1 步要求「Current State 以代码为准，给出文件与行号」。Current State、主控前置决定与待裁决项里的文件名、行号、迁移编号是**核实记录**，回答「今天是什么样」；实现方案在 `plan.md` 与 `contracts/marketing-nodes.md`。User Scenarios、Requirements、Success Criteria 按行为写。

## Requirement Completeness

- [ ] No [NEEDS CLARIFICATION] markers remain
- [x] Requirements are testable and unambiguous
- [x] Success criteria are measurable
- [ ] Success criteria are technology-agnostic (no implementation details)
- [x] All acceptance scenarios are defined
- [x] Edge cases are identified
- [x] Scope is clearly bounded
- [x] Dependencies and assumptions identified

**第一项未勾**：Q1（排期缺位）、Q2（日期粒度）、Q3（导入范围）三条待主控裁决。每条带推荐值，推荐值已作为暂定值写进对应 FR（FR-008～FR-017、FR-025、FR-038）。裁决与推荐不同时，改动范围限于那几条 FR 与 tasks.md 顶部注明的任务。另有 plan.md「主控决定」D1–D4 四条，不属于规格的不确定项，属于登记表与小的设计选择。

**第四项未勾**：SC-011、SC-012、SC-013 点了具体的文件与检查名（`content_constraints_test.go`、`router.go`、`check:content-boundaries`）。这是故意的：这三条的验收对象就是「既有的机器检查仍然绿」，脱离名字无法验证。其余 SC 为行为口径。

## Feature Readiness

- [x] All functional requirements have clear acceptance criteria
- [x] User scenarios cover primary flows
- [x] Feature meets measurable outcomes defined in Success Criteria
- [x] No implementation details leak into specification

## FR / SC 覆盖对照

`tasks.md` 的 60 条任务逐条标了覆盖的 FR / SC。未被任务直接点名的：

| 未被直接点名的 | 为什么不是缺口 |
|---|---|
| FR-029（不调用执行器、不生成文字） | 由 T035 的「关联理由全部原样读出」用例与 contract §7 的「不 import 执行器」守卫覆盖；T053 覆盖页面侧 |
| FR-043（Authorize 同口径） | 由 T024（实现）与 T026 / T027（未登录、非成员被拒）覆盖 |
| FR-052（`modules` 不动） | 由 T059 的 diff 核对与每个 PR 的 `check:content-boundaries` 覆盖 |
| FR-058（不写 UI 单测） | 由 PR 3 卡片「测试」一栏与 T058 的 PR 正文声明覆盖 |

## 本规格自己发现的，记在这里

1. **「日历领域」不存在**（Current State 第 3 节）。BO-01 原文把它列为依赖。文档 12 把「日历关联」划给未落地的 `project-collab`；仓库里唯一的计划时间是作品级交付待办的 `scheduled_at`。本卡没有悄悄新建日历，列为 Q1。
2. **「账号专属材料」今天不存在**（Current State 第 6 节）。素材没有账号列，授权记录没有存下来。D14-V01 后半句在本版无对象可测，改为一条负例守住将来（FR-045、T033）。
3. **`Create` 的账号校验在栅栏外**（Current State 第 2 节，`store.go:167`），与 `SetAccount` 注释的规则不一致。本卡不修，记 Out of Scope 10。
4. **包级测试没有时区库**：`time/tzdata` 只在 `server/cmd/server/main.go:19` import，节点文件要自己 import 一次（plan 风险表）。
5. **「简报」在本仓的含义**：简报版本只在「开始」时冻结，而「开始」就是启动创作。所以 R-056 的「采用后创建或关联简报」在本仓只能落为「创建或关联选题卡」（Assumptions）。

## Notes

- `tasks.md` 编号 T001–T060 连续、无重号、无缺号、无 `T008a` 式编号
- 按宪法 X：`tasks.md` 的勾选只表示该实施任务已交付，不表示已合并或已验收
- 本规格 PR 未运行任何构建或测试；只跑了 `pnpm check:content-boundaries` 确认文档未影响检查
