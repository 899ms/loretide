# Specification Quality Checklist: 运营规则——品牌级设置（029）

**Purpose**: Validate specification completeness and quality before proceeding to planning
**Created**: 2026-09-20 ｜ **Updated**: 2026-09-20（裁决回写）
**Feature**: [spec.md](../spec.md)

## Content Quality

- [x] No implementation details (languages, frameworks, APIs)
      — 有一处**故意的例外**：Current State 一节逐行引用了文件路径、行号与既有键名。这不是实现方案，是「今天到底是什么样」的证据；`docs/development/spec-kit-workflow.md` 要求 Current State 以代码为准，025 / 027 都是这么写的。要求（FR-*）本身不指定语言、框架或接口形状。
- [x] Focused on user value and business needs
- [x] Written for non-technical stakeholders
      — User Story 1–4 用的是小张的语言；技术证据集中在 Current State 与「裁决记录」两节。
- [x] All mandatory sections completed

## Requirement Completeness

- [x] No [NEEDS CLARIFICATION] markers remain
      — **三条已于 2026-09-20 裁决**（PR #192 评论），「三条待裁决」一节已改写为「裁决记录」。受影响的 FR 全部回写：FR-012 / FR-012a（Q2）、FR-021（观察时点粒度）、FR-026 / FR-026a（Q3）、FR-027（页面与插槽）、FR-031（无迁移）、FR-033（三个 PR）；SC-014 ～ SC-016 新增。
- [x] Requirements are testable and unambiguous
      — 裁决之后没有悬而未决的 FR。形状写死在 `contracts/operating-rules.md`：键名、三态、端点、拒绝体点名的字段，逐条可测。
- [x] Success criteria are measurable
- [x] Success criteria are technology-agnostic (no implementation details)
      — SC-004 / SC-005 / SC-007 / SC-009 点名了要检索的字段名与符号（`app_secret`、`http.Get`、写死的天数常量）。保留：这几条的全部价值就在于「检索得到什么」，抽象成「系统不保存凭据」会让它变成一句无法执行的话。
- [x] All acceptance scenarios are defined
- [x] Edge cases are identified
      — 八条，其中「目标条数是 0」「观察天数是 0」是 019 已经在布尔上踩过的同一个坑。
- [x] Scope is clearly bounded
      — Out of Scope 七条，每条写明归谁。
- [x] Dependencies and assumptions identified
      — Assumptions 七条；另有「我补的地方」三条，写明原文没给、我定了什么、推翻的代价。

## Feature Readiness

- [x] All functional requirements have clear acceptance criteria
- [x] User scenarios cover primary flows
      — §3.2 五项对应四个 Story：节奏（P1）、渠道模板 + 账号标识（P2）、审核规则（P2）、观察时点（P3）。**默认时区没有自己的 Story**，因为它已经落地（LT-009），本卡只是把它显示在同一处（FR-020 同款处理）。
- [x] Feature meets measurable outcomes defined in Success Criteria
- [x] No implementation details leak into specification
      — 同第一项的例外。

## Notes

**四个件已按裁决回写**（2026-09-20）：`spec.md`（裁决记录 + 受影响 FR/SC）、`plan.md`、`contracts/operating-rules.md`、`tasks.md`（43 条，分三个 PR）。

**Q3 的裁决把我最担心的事挡住了。** 我提的改良是「守卫不拆，改成『天数必须读自设置，不得是字面量』」，主控接受了这一条，**并且把它单列成第三个 PR**——后者是我没想到的，而它更好：改的是另一个模块里一条刻意写下来的守卫，合进 PR 1 会让这次越界混在四十个文件里看不见。

**实施时最容易做反的一处，已经写进三个件**：`ObservationDue` 的 `unknown` 必须走 `passed` 的分支，不是 `not_yet` 的。把「不知道到没到期」当成「还没到」，会让每一条没有发布时间的发布记录从今日工作台静悄悄消失——而它们恰恰是最需要有人去看一眼的那些。合同第 8 节、plan 的风险表、tasks 的 T035 与策略 5 各写了一遍。

**一处本规格没有回答、也不该由它回答的事**：§3.2 归哪个模块，权威表在文档仓库 `docs/12 §2`，本会话读不到。Q1 当时只给了三个选项与各自代价，由主控裁为 A（`workspace-core`，登记表不改）。
