# Specification Quality Checklist: source-inbox 首切片（粘贴文本与 URL 的素材收件箱）

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

**三个 [NEEDS CLARIFICATION] 是有意保留的**，已按派单回主控。其中 **Q1 是硬阻塞**，另两条带推荐值不阻塞。

「All functional requirements have clear acceptance criteria」**未打勾**，原因具体：**FR-009（历史导入标识）与 FR-013（解析状态列）今天没有可验收的判据**——前者的语义只存在于 §3.3 原文里，后者取决于 Q3 的裁决。这两条在拿到答案前**不能进实施**，其余 25 条可以。

「No implementation details」打勾需要说明：Current State 一节刻意写了字段名、文件路径与迁移号。这是派单要求「Current State 以代码为准」的结果——把事实基础写死，规格其余部分才不会建立在转述之上。FR 与 SC 本身保持技术无关。

### Current State 查出的三处缺口，plan 阶段不得静默跳过

- **A** `topic-planning` 的依赖表里**没有** `source-inbox`（是经 `knowledge-base` 的二跳），而且 `existing_content_relation` / `evidence_gaps_and_investment` 都是**自由文本**——**今天没有任何结构化的引用位**。把 id 塞进自由文本不是引用（→ Q2）。
- **B** 今日工作台今天是**六个区块**且顺序被 026 的 FR-005 / FR-005a 钉死，「待整理收件箱」既不在 §2 五项内也不是第六项 → 本卡不接入（Assumptions 4），接入作为 026 的后续修订。
- **C** 「历史导入」在代码与既有规格里**完全不存在**（全仓 grep 无命中），其语义只能来自 §3.3 原文 → Q1。

### 一条必须守住的底线

**Q3 的选项 C（粘贴文本直接记 `ready`）不会被自行采纳。** 在没有解析器的情况下把状态写成「已就绪」，是宪法 X「打勾不等于验收」要防的那一类。
