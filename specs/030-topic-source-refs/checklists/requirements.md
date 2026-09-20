# Specification Quality Checklist: 选题卡引用素材条目（022 契约扩展）

**Purpose**: Validate specification completeness and quality before proceeding to planning
**Created**: 2026-09-20
**Feature**: [spec.md](../spec.md)

## Content Quality

- [x] No implementation details (languages, frameworks, APIs)
- [x] Focused on user value and business needs
- [x] Written for non-technical stakeholders
- [x] All mandatory sections completed

**说明（前两项为什么打勾而不是打叉）**：本仓的规格按 `docs/development/spec-kit-workflow.md` 第 1 步要求「Current State 一节以代码为准，逐条核实并给出文件与行号」。Current State 与 Clarifications 里的文件名、行号、迁移编号，是**核实记录**，不是实现方案；它们回答的是「今天是什么样」，不是「要怎么写」。User Scenarios、Requirements 与 Success Criteria 三节仍然按行为写，不含技术选型——唯一的例外是 FR-001 / FR-010 / FR-015 三条里的**暂定值**，它们按第 2 步的规定「推荐值即暂定值，写进对应 FR」，裁决后收敛。

## Requirement Completeness

- [x] No [NEEDS CLARIFICATION] markers remain
- [x] Requirements are testable and unambiguous
- [x] Success criteria are measurable
- [ ] Success criteria are technology-agnostic (no implementation details)
- [x] All acceptance scenarios are defined
- [x] Edge cases are identified
- [x] Scope is clearly bounded
- [x] Dependencies and assumptions identified

**第一项**：三条待裁决项没有用 `[NEEDS CLARIFICATION]` 标记，而是写成 Clarifications 一节的 Q1 / Q2 / Q3，每条带选项表与推荐值，推荐值同时写进对应 FR 作为暂定值。这是本仓第 2 步的做法（029 同形），不是把问题藏起来。

**未打勾的一项**：SC-008、SC-010、SC-011、SC-014 点了具体的测试名、脚本名与文件名（`TestBriefStoreHasNoUpdateOrDeletePath`、`check:content-boundaries`、`router.go`、`workspace_delete_manifest_test.go`）。**这是故意的，不是疏漏**：这四条的验收对象就是「那条既有守卫仍然绿」「那份登记表零改动」，脱离具体名字就无法验证，而模板的这一条针对的是「用 API 响应时间代替用户可感知的结果」那一类。其余十一条 SC 均为行为口径。

## Feature Readiness

- [x] All functional requirements have clear acceptance criteria
- [x] User scenarios cover primary flows
- [x] Feature meets measurable outcomes defined in Success Criteria
- [x] No implementation details leak into specification

## 本规格自己发现的两处，记在这里

1. **选题卡的七项今天根本改不了**（Current State 第 2 节）。Issue #204 没有提到这一点。它把「给引用开一条写入路径」从可选项变成必需品，US2 因此是 P1 而不是 P3。
2. **本仓读不到 SOP 原文**（「原文依据」第一节）。§5.2 七项只能按 `specs/022` 的转述写。裁决时需要补原句——本卡要决定引用挂在哪一项下面，而那两项的原句才说得清它们各自在问什么。

## Notes

- Items marked incomplete require spec updates before `/speckit-clarify` or `/speckit-plan`
- 三条 clarify 裁决回写后，spec 内不应再有「暂定」字样（第 2 步）
