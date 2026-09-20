# Specification Quality Checklist: 开始界面与输入快照（EP-04b）

**Purpose**: Validate specification completeness and quality before proceeding to planning
**Created**: 2026-09-20
**Feature**: [spec.md](../spec.md)

## Content Quality

- [x] No implementation details (languages, frameworks, APIs)
      — 说明：Current State 一节**刻意**引用真实文件、字段与端点名。这是本仓库的规格惯例（见 `specs/022` 同名小节），因为「以代码为准」是派单要求；它描述的是**已经存在的事实**，不是本卡的实现选择。Requirements 与 Success Criteria 两节不含实现细节。
- [x] Focused on user value and business needs
- [x] Written for non-technical stakeholders
      — 部分受限：本卡的读者是主任务与执行会话，Current State 需要技术精度。
- [x] All mandatory sections completed

## Requirement Completeness

- [x] No [NEEDS CLARIFICATION] markers remain
      — 三条 clarify **已由主控裁决**（2026-09-20，Q1=B / Q2=A / Q3=A，附带一问接受，T026 入口已定），文末「裁决记录」一节记了结论与它改变了什么。
- [x] Requirements are testable and unambiguous
- [x] Success criteria are measurable
- [x] Success criteria are technology-agnostic (no implementation details)
      — SC-002 提到「十六个字段」是对既有契约的计数，不是实现选择。
- [x] All acceptance scenarios are defined
- [x] Edge cases are identified
- [x] Scope is clearly bounded
      — 见 Out of Scope：EP-04c / EP-04d / EP-06 / agent-workflow / Grant 存储。
- [x] Dependencies and assumptions identified

## Feature Readiness

- [x] All functional requirements have clear acceptance criteria
- [x] User scenarios cover primary flows
- [x] Feature meets measurable outcomes defined in Success Criteria
- [x] No implementation details leak into specification

## Notes

- **Q1=B 已回写**：FR-013（独立实体 + 稳定键）、FR-014（字面只插不改）、**新增 FR-014a**（可多次开始、端点不幂等）、FR-023（R1–R6 全生效）、**新增 FR-023a**（删除清单）、Key Entities（新表九列、无 `revision` 计数器）。决策顺序去掉第 7 条 409，读回改为两条新 GET，上游改动从一个文件变两个。
- **Q2=A 已回写**：FR-022 的九个字段各有一条负例。
- **Q3=A 已回写**：FR-009～FR-012。
- **T026 入口已定**：选题卡详情页 `start` 之后的区块，列表不加。它反过来加了第三个索引 493（按卡列出），见 `analysis.md` §3 第 4 点。
- 宪法 II：本卡不写 UI 单测、不做自动浏览器验收（FR-027）。界面项在实施时进 `manual-ui-todo.md`。
- 宪法 X：本清单勾选只代表「已核对」，不代表验收通过。
