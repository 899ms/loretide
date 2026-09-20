# Specification Quality Checklist: 运营规则——品牌级设置（029）

**Purpose**: Validate specification completeness and quality before proceeding to planning
**Created**: 2026-09-20
**Feature**: [spec.md](../spec.md)

## Content Quality

- [x] No implementation details (languages, frameworks, APIs)
      — 有一处**故意的例外**：Current State 一节逐行引用了文件路径、行号与既有键名。这不是实现方案，是「今天到底是什么样」的证据；`docs/development/spec-kit-workflow.md` 要求 Current State 以代码为准，025 / 027 都是这么写的。要求（FR-*）本身不指定语言、框架或接口形状。
- [x] Focused on user value and business needs
- [x] Written for non-technical stakeholders
      — User Story 1–4 用的是小张的语言；技术证据集中在 Current State 与 Q1–Q3，主控读的就是这两处。
- [x] All mandatory sections completed

## Requirement Completeness

- [x] No [NEEDS CLARIFICATION] markers remain
      — **改成了「三条待裁决」一节**（Q1 存储形态与模块归属、Q2 渠道集与主页链接落点、Q3 027 的待补录判定改不改），每条带选项表、代价与暂定推荐值。这是本仓 025 / 027 已采用并被主控接受的形式；标记式的 `[NEEDS CLARIFICATION]` 会散落在正文里，而裁决需要的是一处能一次读完的比较表。
- [x] Requirements are testable and unambiguous
      — 三条待裁决影响的 FR 已逐条标出（FR-012 → Q2、FR-021 → Q2 同类、FR-026 → Q3、FR-027 → Q1、FR-031 → Q1）。其余 FR 与裁决无关，现在就能测。
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

**三条待裁决已带暂定推荐值，不阻塞 `/speckit-plan`。** 按主控的常规指令（「推荐值暂定不阻塞」），plan 与 tasks 可以按 Q1=A / Q2=A / Q3=A 起草；裁决回来后回写四个件并重跑 `/speckit-analyze`，与 025 / 027 的流程一致。

**Q3 是这三条里我最没把握的一条。** 它要动的是 027 里一条**刻意写下来的守卫**（`TestThePendingDerivationHasNoTimeLogic`），而那条守卫的注释正是为了防住「顺手写一个默认天数」。观察时点落地之后，守卫的前提确实过期了——但让守卫认识新的合法写法，和把守卫拆掉，是两件不一样的事。规格里已经写明：若裁为 B，守卫改成「天数必须读自设置，不得是字面量」，而不是允许时间比较。

**一处本规格没有回答、也不该由它回答的事**：§3.2 归哪个模块，权威表在文档仓库 `docs/12 §2`，本会话读不到。Q1 因此只给了三个选项与各自代价，没有替主控选。
