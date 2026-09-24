# Specification Quality Checklist: 成本、线索、成交与 ROI 复盘（034）

**Purpose**: Validate specification completeness and quality before proceeding to implementation
**Created**: 2026-09-25
**Feature**: [spec.md](../spec.md)

## Content Quality

- [x] No implementation details (languages, frameworks, APIs)
      — Current State 一节刻意引用真实文件、行号与规则编号，这是本仓库规格的惯例（「以代码为准」）。FR 里出现的列名、端点与原因码是验收要能直接断言的名字，与 027 / 031 同一写法。
- [x] Focused on user value and business needs
- [x] Written for non-technical stakeholders
      — 部分受限：读者是主控与执行会话，计算规则需要精确到最小货币单位。
- [x] All mandatory sections completed

## Requirement Completeness

- [ ] No [NEEDS CLARIFICATION] markers remain
      — 正文没有该标记，但文末「待裁决」列了七条（Q1～Q7），每条的推荐值已作为暂定值写进 FR。**主控裁决后勾上**。
- [x] Requirements are testable and unambiguous
      — 金额解析、舍入、余数分配、原因码优先级、窗口规则都写死了取值与断言形状；D14-V12 的两个固定样例逐字进了 contract §5.4。
- [x] Success criteria are measurable
- [x] Success criteria are technology-agnostic (no implementation details)
      — SC-012 / SC-015 提到的守卫与迁移规则是本仓库既有的可运行检查。
- [x] All acceptance scenarios are defined
- [x] Edge cases are identified
      — 本卡真正的坑有三个：「不可计算」被显示成 0；多触点让品牌合计重复；未换算币种被静默排除。每个都有 FR、SC 和手验项。
- [x] Scope is clearly bounded
      — AI 解释与采纳（D1）、营销节点关联（Q4）、品牌级配置（Q5）、文件上传（W-03）明确不做。D14-V15 只能部分覆盖，SC-016 如实写。
- [x] Dependencies and assumptions identified
      — 唯一的登记表改动（`feedback-learning` 加 `idempotency`）写在 plan.md「主控决定」，本卡未改 `scripts/content-boundaries.json`。

## Feature Readiness

- [x] All functional requirements have clear acceptance criteria
- [x] User scenarios cover primary flows
- [x] Feature meets measurable outcomes defined in Success Criteria
- [x] No implementation details leak into specification

## D14 验收覆盖

| 编号 | 覆盖 | 在哪 |
|---|---|---|
| D14-V11 | 全部（模型输入一半为结构保证） | FR-016、FR-026～FR-029、FR-058、FR-062、FR-070；SC-007～SC-009；PR 1、PR 3、PR 4 |
| D14-V12 | 全部 | FR-043、FR-044；SC-001；T048；U-22、U-23 |
| D14-V13 | 全部 | FR-006、FR-025、FR-042、FR-046、FR-047、FR-050～FR-052；SC-002；T049 |
| D14-V14 | 全部 | FR-030～FR-035、FR-054；SC-003、SC-004、SC-010；T041～T043、T073 |
| D14-V15 | **部分**：录入 → 复盘 → 来历这一段 | SC-016；manual-ui-todo。采纳段记「未执行（宪法 IX，后续卡）」 |
| D14-V16 | 全部 | FR-017～FR-021、FR-036、FR-037；SC-005、SC-006；T020、T050；U-12、U-30 |

## Notes

- **本卡只出规格，不含实现**（派单口径）。
- 宪法 II：本卡不写 UI 单测；界面项由用户在浏览器验收。
- 宪法 X：本清单勾选只代表「已核对」，不代表验收通过。
