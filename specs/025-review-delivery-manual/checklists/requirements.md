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

- [x] No [NEEDS CLARIFICATION] markers remain
      — **裁决后勾上**（主控 2026-09-21，PR #144 评论）。五条全部裁决，SOP §8 / §9.1 / §9.2 / §9.3 原文已抄进 spec 的「SOP 原文」一节，结论与「它改变了什么」记在「裁决记录」一节。文末不再有「待裁决」。
- [x] Requirements are testable and unambiguous
      — 裁决后无例外：FR-004（快照键恰好八个）、FR-010（交接三值）、FR-013（渠道四值）、FR-017（Q5 的字段清单，**没有枚举**）都写死了取值；FR-009a / FR-019a 把两处「显示」明确成读时派生而不是存储状态。
- [x] Success criteria are measurable
      — 每条 SC 都给了用例条数与比较方式（「逐字节相同」而不是「一样」，「三条用例，不合并」而不是「有覆盖」）。
- [x] Success criteria are technology-agnostic (no implementation details)
      — SC-005 / SC-006 / SC-009 提到守卫与迁移规则编号：它们是本仓库既有的、可运行的检查，不是本卡的实现选择。
- [x] All acceptance scenarios are defined
- [x] Edge cases are identified
- [x] Scope is clearly bounded
      — 见 Out of Scope：平台 API、自动发布、调度器、聚合与建议（属 `feedback-learning`）、附件与媒体文件本身（W-03）、截图上传、模型调用。§9.2 原文的「系统不保存平台发布密钥，也不提供发布执行接口」已写进 FR-023。
- [x] Dependencies and assumptions identified
      — 关键依赖是 work-editor（#141）尚未合入；规格只硬要求「版本不可变且有稳定键」，实际列名在 tasks.md T002 以合入后的树为准。

## Feature Readiness

- [x] All functional requirements have clear acceptance criteria
- [x] User scenarios cover primary flows
- [x] Feature meets measurable outcomes defined in Success Criteria
- [x] No implementation details leak into specification

## Notes

- **本卡只出规格，不含实现**（派单口径）。五条裁决已回写四个文件：spec（「SOP 原文」+「裁决记录」两节）、plan（表数与风险）、contract（表定义、状态机、快照八键、六步决策顺序）、tasks（T001 已勾，新增九条先写用例与三处变异）。
- **两处原文之外的补充已被裁决接受**：(a) `held` 从 `ready` / `scheduled` 进、回到 `ready`；(b) 发布记录**没有状态机**，「只前进不删」落在「记录只插不改不删」上。
- **裁决之外我另补了两处，都单列在 spec 的「裁决记录」里**：`failed` / `removed` 的原因借用 `receipt_note`（Q5 的字段清单里没有 `reason` 列）；`DeliveryPackage` 是派生视图而不是第五张表。两处各一处改动即可翻转。
- **一处我特意没做**：「延后」没有变成第七个状态——它是改 `scheduled_at` 或进 `held`，两者都写迁移记录与原因，而 §7.1 的六个状态里没有它。
- **Q5 的裁决删掉了两个我本来要发明的枚举**。T015a 是守住它的那条负例：半年后看到两个自由文本字段的人，会觉得「补全一下」是在帮忙。
- 宪法 II：本卡不写 UI 单测；界面项由主控在浏览器验收。
- 宪法 X：本清单勾选只代表「已核对」，不代表验收通过。
