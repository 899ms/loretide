# Specification Quality Checklist: 人工写作的作品容器、文档与版本（024）

**Purpose**: Validate specification completeness and quality before proceeding to planning
**Created**: 2026-09-20
**Feature**: [spec.md](../spec.md)

## Content Quality

- [x] No implementation details (languages, frameworks, APIs)
      — Current State 一节**刻意**引用真实文件、表名与规则编号。这是本仓库的规格惯例（见 `specs/022` / `specs/023` 同名小节），因为「以代码为准」是派单要求；它描述的是**已经存在的事实**，不是本卡的实现选择。Requirements 与 Success Criteria 两节不含实现细节。
- [x] Focused on user value and business needs
- [x] Written for non-technical stakeholders
      — 部分受限：读者是主任务与执行会话，Current State 需要技术精度。
- [x] All mandatory sections completed

## Requirement Completeness

- [x] No [NEEDS CLARIFICATION] markers remain
      — 三条 clarify 与两个附带问题**已由主控裁决**（2026-09-21），**并给出了 SOP §7.1「文档编辑」行的原文**（抄在 spec 与 contract 的开头）。文末「裁决记录」记了结论与它改变了什么。
- [x] Requirements are testable and unambiguous
      — 裁决后无例外：FR-004（作品无状态列）、FR-007a（`working`/`saved`）、FR-010（来源恰好三个）、FR-010a（动作第二列）、FR-011 / FR-016（恢复与采用的具体取值）都按 §7.1 原文写死了。
- [x] Success criteria are measurable
- [x] Success criteria are technology-agnostic (no implementation details)
      — SC-004 / SC-008 提到守卫与迁移规则编号：它们是本仓库既有的、可运行的检查，不是本卡的实现选择。
- [x] All acceptance scenarios are defined
- [x] Edge cases are identified
- [x] Scope is clearly bounded
      — 见 Out of Scope：模型调用、审核与交接、附件引用清单、diff 渲染、协同编辑、渠道导出。
- [x] Dependencies and assumptions identified

## Feature Readiness

- [x] All functional requirements have clear acceptance criteria
- [x] User scenarios cover primary flows
- [x] Feature meets measurable outcomes defined in Success Criteria
- [x] No implementation details leak into specification

## Notes

- **Q1=A 已回写**：编辑副本是文档行可变列，版本独立 append-only 表，**显式**存版本；不自动存版；**要** `workspace_id` 打头的索引。
- **Q2=A 已回写**：只存字符串 id，不 import `topic-planning`，登记表不动。
- **Q3 已按 SOP 原文回写**：编辑副本状态只有 `working` / `saved`；作品**无状态列**；版本来源恰好 `generated` / `edited` / `adopted`；**来源与动作两列**，恢复是动作不是来源。
- **本轮我补的两处**：`content_work` 也需要一个 `workspace_id` 打头的索引（同一论证，且「列一张卡下的作品」本就需要它）；普通保存的 `action = saved`（裁决未点名，已标明）。
- 宪法 II：本卡不写 UI 单测、不做浏览器验收（FR-027）。
- 宪法 X：本清单勾选只代表「已核对」，不代表验收通过。
