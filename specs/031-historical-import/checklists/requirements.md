# Specification Quality Checklist: §3.3 导入少量历史资产（已发表作品的粘贴导入）

**Purpose**: Validate specification completeness and quality before proceeding to planning
**Created**: 2026-09-20
**Updated**: 2026-09-21（按主控裁决回写，PR #213 评论）
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

**六个 clarify 已全部裁决并回写**（PR #213 评论，2026-09-21）：Q1 ~ Q6 全部采纳推荐值，并带回三条附加约束。

初版有两项未打勾，现均已满足：

- **「No [NEEDS CLARIFICATION] markers remain」**：六项集中在 **Clarifications** 一节，现已改写为裁决结果。被否的选项**保留在原处**——「为什么不选它」是日后有人想改回去时唯一的依据。
- **「All functional requirements have clear acceptance criteria」**：初版未打勾的具体原因是三组 FR 的判据依赖裁决结果。现在——
  - **FR-011 / FR-012 / FR-013（发布后快照）**：Q4 = A 定为发布记录上的 `version_id` 列，判据是 SC-005（存新版后快照不动）与 SC-019（027 反查优先读它）。不推荐的 C（读时推断）会让 SC-005 直接红，这正是它被否的理由。
  - **FR-014（版本动作）**：Q1 = A 定为 `source=edited` / `action=imported`，判据是 SC-001 与新增的 SC-018（`created_at` 是导入时刻）。
  - **FR-022 / FR-023 / FR-023a（失败语义）**：Q2 = A 定，三条附加约束各自可验（SC-013 / SC-013a / SC-013b）。

### 三条附加约束，各自落到了 FR 与 SC

| 附加约束 | FR | SC |
|---|---|---|
| 每步幂等 / 停在哪一步可重试 / 不回滚 | FR-022、FR-023、FR-023a | SC-013、SC-013a、SC-013b |
| 按卡聚合的读路径排除空卡作品，各加负例 | FR-034、FR-035、FR-036 | SC-016、SC-017 |
| 导入版本 `source=edited` / `action=imported`，`created_at` 是导入时刻 | FR-014、FR-014a | SC-018 |

### 三处刻意的「不做」，各有记录

1. **不为四渠道之外的平台加 `other`**（Out of Scope 2）：与 027 指标集拒绝 `other` 同一理由——给了逃生舱，第一个被塞进去的就是本该在清单里的名字。
2. **不做跨作品去重**（Edge Cases）：028 的去重按素材内容哈希，作品侧没有对应机制；发明一个会在「同一篇发过两个渠道」上立刻出错。
3. **不把历史导入的发布记录排除在今日工作台第五项之外**（FR-028）：它们确实待补录。代价（一次导入二十条会淹没该区块）写进了 Edge Cases，解法是让每条可辨认，而不是悄悄过滤。

### 一条已经验证过的事实，写在这里防止后续卡重做

**§3.3 的「常用参考资料」「典型用户问题」两路今天就能做**：028 的 `historical_import` 从 Go 契约、迁移 `CHECK`、`packages/core/content/source-inbox/draft.ts` 一直到 `packages/views/content/source-inbox/index.tsx:386` 的勾选框全部已落地。US5 的场景 1 **不需要本卡的任何实现即可验收**。

### 实施时最容易漏的一处，写在最后

**FR-034 的枚举**。放宽 `topic_card_id` 之后没有任何数据库机制能挡住「历史作品冒充在写的作品」——没有外键，也没有 `CHECK` 能表达「这个列表不要空卡的作品」。而它的失效方式极其安静：**在一个从没导入过历史作品的测试库里，只有正例的测试永远是绿的。** tasks.md 把「先枚举全部按卡读路径」定成 T003（Phase 1，先于任何代码），把负例定成 T021 / T022 先写，就是为了这一点。
