# Specification Quality Checklist: §3.3 导入少量历史资产（已发表作品的粘贴导入）

**Purpose**: Validate specification completeness and quality before proceeding to planning
**Created**: 2026-09-20
**Feature**: [spec.md](../spec.md)

## Content Quality

- [x] No implementation details (languages, frameworks, APIs)
- [x] Focused on user value and business needs
- [x] Written for non-technical stakeholders
- [x] All mandatory sections completed

## Requirement Completeness

- [ ] No [NEEDS CLARIFICATION] markers remain
- [x] Requirements are testable and unambiguous
- [x] Success criteria are measurable
- [x] Success criteria are technology-agnostic (no implementation details)
- [x] All acceptance scenarios are defined
- [x] Edge cases are identified
- [x] Scope is clearly bounded
- [x] Dependencies and assumptions identified

## Feature Readiness

- [ ] All functional requirements have clear acceptance criteria
- [x] User scenarios cover primary flows
- [x] Feature meets measurable outcomes defined in Success Criteria
- [x] No implementation details leak into specification

## Notes

### 两项未打勾，原因具体

**「No [NEEDS CLARIFICATION] markers remain」**：本规格没有用 `[NEEDS CLARIFICATION]` 标记，而是把六个待裁决项集中在 **Clarifications** 一节，每项给出选项、推荐值与代价，并把推荐值写进了对应的 FR（标为「暂定」）。这样做的理由是主控要求「推荐值暂定不阻塞」——散落的标记会让规格读起来像半成品，集中一节则能一次裁决完。**但它们确实还没裁决**，所以这一项不打勾。

**「All functional requirements have clear acceptance criteria」**：不打勾的具体原因是**三条 FR 的判据依赖裁决结果**：

- **FR-011 / FR-012 / FR-013（发布后快照）**：判据本身是清楚的（SC-005：存新版后快照不动），但「快照是什么」在 Q4 裁决前有三种形态。若 Q4 = C（读时推断），SC-005 会直接红——这正是不推荐 C 的原因。
- **FR-014（版本动作）**：Q1 = B 时，「导入」这一动作在版本行上不留痕，US1 场景 2 仍然绿，但 US2 场景 1 的可区分性要完全落到作品层。
- **FR-022 / FR-023（失败语义）**：只在 Q2 = A 下成立。Q2 = B 时它们要换成「整体回滚」，SC-013 随之改写。

规格已经把每种裁决下判据怎么变写在了条款里，所以裁决之后回写是改文字、不是重做结构。

### 三处刻意的「不做」，各有记录

1. **不为四渠道之外的平台加 `other`**（Out of Scope 2）：与 027 指标集拒绝 `other` 同一理由——给了逃生舱，第一个被塞进去的就是本该在清单里的名字。
2. **不做跨作品去重**（Edge Cases）：028 的去重按素材内容哈希，作品侧没有对应机制；发明一个会在「同一篇发过两个渠道」上立刻出错。
3. **不把历史导入的发布记录排除在今日工作台第五项之外**（FR-028）：它们确实待补录。代价（一次导入二十条会淹没该区块）写进了 Edge Cases，解法是让每条可辨认，而不是悄悄过滤。

### 一条已经验证过的事实，写在这里防止后续卡重做

**§3.3 的「常用参考资料」「典型用户问题」两路今天就能做**：028 的 `historical_import` 从 Go 契约、迁移 `CHECK`、`packages/core/content/source-inbox/draft.ts` 一直到 `packages/views/content/source-inbox/index.tsx:386` 的勾选框全部已落地。US5 的场景 1 **不需要本卡的任何实现即可验收**。本卡把它写进规格是为了说明这一路已经通了，不是为了新做。
