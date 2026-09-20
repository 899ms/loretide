# Specification Quality Checklist: 今日工作台（SOP §2 入口页）

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

**三个 clarify 已全部裁决并回写**（PR #158 评论，2026-09-21）：Q1 = B（+ 派生口径进 core）、Q2 = A、Q3 = C。规格「裁决记录」一节逐条列出落到了哪些 FR / Assumption。

「No implementation details」一项打勾需要说明：Current State 一节刻意写了端点、字段名与文件路径。这不是需求泄漏实现，而是派单要求「Current State 以代码为准」——把事实基础写死，规格其余部分才不会建立在转述之上。FR 与 SC 本身保持技术无关。

### 裁决纠正的两处，务必不要在 plan 阶段退回原样

1. **§2 原文第五项是「待补录的反馈」，不是「待补充的经营底座」**（Issue #155 转述有误）。区块按原文五项编排；「账号配置缺项」是**第六个附加区块**，标题按本义写，**不冒充第五项**（FR-005 / FR-005a）。
2. **§11 要求选题区块显示「选题理由、预计投入」**。初版规格漏了「预计投入」，已补 FR-010a / FR-010b。

### Current State 查出的五处缺口，plan 阶段不得静默跳过

- **A** Issue 假设的 `proposed` / `shortlisted` **在代码里不存在**（→ Q2 = A，只列 `draft`）；`TopicCard` 无来源字段，「只展示人工创建」今天恒真但无从施加（→ FR-011a）。
- **B** 作品的 `working` 状态在**文档**上而非作品上 → 1+N。
- **C** 账号 `readiness` 只在 `/profile` 上而非账号列表上 → 1+N。
- **D** §11 的「预计投入」只能取账号的每周可投入时间，且**该字段可能是「待补充」状态** → 此时必须显示「未确认」（FR-010b）。与 C 同源，应复用同一次读取（Assumptions 2）。
- **E** `feedback-learning` 模块**不存在**，§2 第五项今天没有数据源 → 区块存在但显示暂不可用（FR-005b）。

### 本卡不适用项

**工作流第 12 步（路径参数穿过真实中间件）在本卡没有适用对象**，因为 FR-021 禁止新增端点。实施 PR 应当**明确记为「不适用」并说明原因**，而不是默默跳过（FR-019）。

### 与 025 的顺序依赖

审核与交付两个区块的条目在 `review-delivery` 页面落地前**不可点**并写明原因（FR-022a、Assumptions 6）。这是真实功能缺口，不是遗漏。
