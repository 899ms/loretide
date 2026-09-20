# Specification Quality Checklist: 人工审核请求、交付任务与发布记录（025）

**Purpose**: Validate specification completeness and quality before proceeding to planning
**Created**: 2026-09-20
**Feature**: [spec.md](../spec.md)

## Content Quality

- [x] No implementation details (languages, frameworks, APIs)
      — Current State 一节**刻意**引用真实文件、表名与规则编号。这是本仓库的规格惯例（见 `specs/022` / `specs/023` / `specs/024` 同名小节），因为「以代码为准」是派单要求；它描述的是**已经存在的事实**，不是本卡的实现选择。Requirements 与 Success Criteria 两节不含实现细节。
- [x] Focused on user value and business needs
- [x] Written for non-technical stakeholders
      — 部分受限：读者是主任务与执行会话，Current State 需要技术精度。
- [x] All mandatory sections completed

## Requirement Completeness

- [ ] No [NEEDS CLARIFICATION] markers remain
      — **未通过，且是故意的**。五条 clarify 列在 spec 文末「待裁决」一节，已各带一个暂定推荐值，规格按推荐值写成可读的整体。**Q1 与 Q3 的裁决会改变表的数量**（Q1=B 多一张交付快照表、Q3=B 去掉迁移记录表），**Q5 需要 SOP §9.2 原文**才能定枚举——派单口径是「clarify 回主控、推荐值暂定不阻塞」，所以这一条要到裁决回写后（tasks.md T001）才能勾上。
- [x] Requirements are testable and unambiguous
      — 未裁决的四处都**指名道姓**地指向对应的 Q（FR-004→Q1、FR-007→Q3、FR-010/FR-013→Q4、FR-017→Q5），不是含混带过。其余 FR 都写死了可验的取值与断言形状。
- [x] Success criteria are measurable
      — 每条 SC 都给了用例条数与比较方式（「逐字节相同」而不是「一样」，「三条用例，不合并」而不是「有覆盖」）。
- [x] Success criteria are technology-agnostic (no implementation details)
      — SC-005 / SC-006 / SC-009 提到守卫与迁移规则编号：它们是本仓库既有的、可运行的检查，不是本卡的实现选择。
- [x] All acceptance scenarios are defined
- [x] Edge cases are identified
- [x] Scope is clearly bounded
      — 见 Out of Scope：平台 API、自动发布、调度器、聚合与建议（属 `feedback-learning`）、附件与素材回填、模型调用。
- [x] Dependencies and assumptions identified
      — 关键依赖是 work-editor（#141）尚未合入；规格只硬要求「版本不可变且有稳定键」，实际列名在 tasks.md T002 以合入后的树为准。

## Feature Readiness

- [x] All functional requirements have clear acceptance criteria
- [x] User scenarios cover primary flows
- [x] Feature meets measurable outcomes defined in Success Criteria
- [x] No implementation details leak into specification

## Notes

- **本卡只出规格，不含实现**（派单口径）。plan.md 与 contracts/ 按暂定推荐值写，裁决后回写三处：spec 的「裁决记录」、plan 的 Project Structure（迁移数量）、contract 的表定义。
- **两处是我在原文之外补的，已在合同里标明**：(a) `held` 从 `ready` / `scheduled` 进、回到 `ready`——§7.1 原文只说「可 cancelled / held」，没说出入边；(b) 发布记录**没有状态机**，「只前进不删」落在「记录只插不改不删」上，而不是「状态不能回退」。两处若裁决另有口径，改的是合同 §2 的图与对应用例。
- **有一条我特意没有发明**：Q5 的声明者与核验方式，若 §9.2 没有枚举就退成自由文本、只保留「必填」。三项必填（声明者 / 证据 / 核验方式）是 §9.2 明确的，枚举不是。
- 宪法 II：本卡不写 UI 单测；界面项由主控在浏览器验收。
- 宪法 X：本清单勾选只代表「已核对」，不代表验收通过。
