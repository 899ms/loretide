# Specification Quality Checklist: source-inbox 首切片（粘贴文本与 URL 的素材收件箱）

**Purpose**: Validate specification completeness and quality before proceeding to planning
**Created**: 2026-09-20
**Updated**: 2026-09-21（按主控裁决回写）
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

**三个 clarify 已全部裁决并回写**（PR #172 评论，2026-09-21）：Q1 提供了 §3.3 / §4 / §7.1 / §11 与 PRD R-009 ~ R-011 原文并定下字段清单，Q2 = A，Q3 = A 修正版。

初版有两项未打勾，现均已满足：

- **「No [NEEDS CLARIFICATION] markers remain」**：三处标记已按裁决替换为具体条款。
- **「All functional requirements have clear acceptance criteria」**：初版未打勾的具体原因是 FR-009（历史导入）与 FR-013（解析状态列）没有可验收的判据。现在——**历史导入**由 §3.3 原文定为「系统保留的标识」，落为布尔、创建时定、不可改（FR-007），可验收；**解析状态列**按 Q3 = A 修正版**不建**，改为 Out of Scope 第 1 条并写明它随解析器卡出现（FR-023 / FR-024），也就不再有悬空的判据。

### 原文改写了初版的两处推测

1. **字段清单不再是猜的**。§4 步骤 1 / 4 与 R-009 / R-010 把 `annotation`（个人批注）与 `personal_judgement`（个人判断）定为**两个分开的字段**（FR-006）——初版只有一个笼统的「来源说明」。「自动摘要」那一列本卡不建（FR-001）。
2. **`SourceSnapshot` 本卡就要建**。初版把原文放在条目主行上；R-010 明确「原文件与正文快照分别保存」，且快照带内容哈希（FR-010 ~ FR-014）。去重也因此改为**按 `content_hash`**（FR-021），而不是初版设想的「不做去重」。

### 三条必须守住的底线

- **重复素材只提示，不合并不删除**（FR-022）。R-011 原文是「内容相同**不删除**独立的收藏上下文与批注」，§4 是「**先提示**合并关联」——合并是人的决定。实施时把「顺手合并一下」当成优化，会直接违反这两句。
- **解析状态列不建**（FR-023）。裁决明确**不采纳**「粘贴文本直接记 `ready`」——没有解析器却写「已就绪」正是宪法 X 要防的那类。
- **粘贴超限明确拒绝，不截断**（Assumptions 2）。截断会让哈希对应到一段谁也没打算保存的内容，而哈希是去重的唯一依据。

### 一条由裁决直接推出、需要主任务知晓的限制

**URL 条目没有去重提示。** 去重按 `content_hash`，哈希在快照上，而 `kind = url` 本卡不生成快照（0 抓取）——两次收同一个链接不会被提示重复。这是「URL 0 抓取」与「按内容哈希去重」两条决定相乘的结果，**不是遗漏**，已写进 Out of Scope 的「已知限制」。要覆盖它需要对 URL 规范化后哈希（会把 `?utm_source=` 之类算成不同）或等抓取能力，本卡不自行选其一。
