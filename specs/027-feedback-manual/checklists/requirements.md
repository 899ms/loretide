# Specification Quality Checklist: 人工登记真实结果与反馈摘录（027）

**Purpose**: Validate specification completeness and quality before proceeding to planning
**Created**: 2026-09-20
**Feature**: [spec.md](../spec.md)

## Content Quality

- [x] No implementation details (languages, frameworks, APIs)
      — Current State 一节**刻意**引用真实文件、表名、列名与规则编号。这是本仓库的规格惯例（见 `specs/022`–`026` 同名小节），因为「以代码为准」是派单要求；它描述的是**已经存在的事实**，不是本卡的实现选择。Requirements 与 Success Criteria 两节不含实现细节。
- [x] Focused on user value and business needs
- [x] Written for non-technical stakeholders
      — 部分受限：读者是主任务与执行会话，Current State 需要技术精度。
- [x] All mandatory sections completed

## Requirement Completeness

- [x] No [NEEDS CLARIFICATION] markers remain
      — **裁决后勾上**（主控 2026-09-21，PR #163 评论）。六条全部裁决，SOP §10.1 / §7.1「AI 复盘」行 / §3.2 与 PRD R-044 R-045 原文已抄进 spec 的「SOP 与 PRD 原文」一节，结论与「它改变了什么」记在「裁决记录」一节。文末不再有「待裁决」。
- [x] Requirements are testable and unambiguous
      — 裁决后无例外：FR-002（十列）、FR-003（`metric` 恰好十一项）、FR-004（空≠0）、FR-008（`source_type` 系统写）、FR-014（摘录来源恰好三项、摘录与解释分两列）、FR-020（待补录不带时间逻辑）、FR-021（AI 状态七值）都写死了取值与断言形状。
- [x] Success criteria are measurable
      — 每条 SC 都给了用例条数与比较方式（「逐字节不变」而不是「一样」，「留空与 0 读回来不同」而不是「字段可空」）。
- [x] Success criteria are technology-agnostic (no implementation details)
      — SC-002 / SC-004 / SC-009 提到守卫与迁移规则编号：它们是本仓库既有的、可运行的检查，不是本卡的实现选择。
- [x] All acceptance scenarios are defined
- [x] Edge cases are identified
      — 其中两条是本卡真正的坑：`published_at` 为空算不出到期；**留空被记成 0** 会在任何后续聚合里变成真实坏数据。
- [x] Scope is clearly bounded
      — 见 Out of Scope：§10.2 复盘、§10.3 经营结论、任何聚合与比较、平台 API、品牌级观察时点设置页（属 §3.2 另卡）、CSV 文件上传与证据附件（等 W-03）、**自动**脱敏、把「阅读」与「播放」合并或排名（§10.1 明确禁止）。
      **唯一一处跨出本卡的是 FR-025**（PR 1 同时更新 026 的两条 FR 并提供第五项派生函数）——那是裁决 Q6=A 给的范围，不是我扩的。
- [x] Dependencies and assumptions identified
      — 关键依赖是 025 已合入的 `content_publication_record`，其**没有 `version_id` 列**这一点产生了 Q2（裁决：两跳解出，解不出是 `unknown` 且不拒绝录入）。页面 PR 还依赖 #161（025 页面）合入——本卡的区块挂在它之后的插槽里。

## Feature Readiness

- [x] All functional requirements have clear acceptance criteria
- [x] User scenarios cover primary flows
- [x] Feature meets measurable outcomes defined in Success Criteria
- [x] No implementation details leak into specification

## Notes

- **本卡只出规格，不含实现**（派单口径）。裁决后已补齐 `plan.md` / `tasks.md` / `contracts/feedback-manual.md`——**Q1 定了列，才谈得上建几张表**。
- **我原先暂定的两处被原文推翻了，这是好事**：摘录来源我暂定「自由文本」，原文点名了 `comment` / `private_message` / `lead` 三项；观察时点我暂定「记在记录上 + 一个默认天数常量」，裁决砍掉了那个常量——没有 §3.2 的设置，「过了没过」本来就无从判断，写个默认天数等于发明一条 SOP 没说的规则。**首版的待补录因此不带任何时间逻辑，比我提的更简单也更诚实。**
- **裁决之外我补了两处，都单列在 spec 的「裁决记录」里**：(1) **没有 AI 复盘报告表**——本阶段产生不出内容，恒空的表会被读成「它迟早会被填」，所以状态是派生的；(2) **`window` 取自由文本**——裁决写的是「自由文本或起止时间二选一，规格定」，我选自由文本，因为「发布后 14 天累计」「上线首日」这类说法用起止时间表达不了，强拆成两个时间戳会逼人把不知道的边界编出来。两处各一处改动即可翻转。
- **一处我特意没做**：没有把「阅读」和「播放」并成一个指标名。§10.1 原文是「分别保留，不直接合并排名」，所以它们是受控集里的两个值，另加 SC-009 禁止任何合并或排名的路径。
- **本卡最容易悄悄出错的地方是 `value` 的空被折成 0**，因此它在三处各钉了一次（Go 纯函数、真实 DB、core 解析），不是一次——不一致时没有任何东西会报警。
- 宪法 II：本卡不写 UI 单测；界面项由主控在浏览器验收。
- 宪法 X：本清单勾选只代表「已核对」，不代表验收通过。
