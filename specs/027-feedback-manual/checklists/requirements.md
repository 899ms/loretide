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

- [ ] No [NEEDS CLARIFICATION] markers remain
      — **未通过，且是故意的**。六条 clarify 列在文末，其中 **Q1 / Q3 / Q4 需要 SOP §10.1 与 §7.1 的原文**，我手上没有。派单口径是「clarify 回主控、推荐值暂定不阻塞」，所以这一条要到裁决回写后才能勾上。
- [x] Requirements are testable and unambiguous
      — 未定的三处都**指名道姓**地指向对应的 Q（FR-005→Q1、FR-017→Q3、FR-009→Q4），不是含混带过。其余 FR 都写死了可验的断言形状。
- [x] Success criteria are measurable
      — 每条 SC 都给了用例条数与比较方式（「逐字节不变」而不是「一样」，「留空与 0 读回来不同」而不是「字段可空」）。
- [x] Success criteria are technology-agnostic (no implementation details)
      — SC-002 / SC-004 / SC-009 提到守卫与迁移规则编号：它们是本仓库既有的、可运行的检查，不是本卡的实现选择。
- [x] All acceptance scenarios are defined
- [x] Edge cases are identified
      — 其中两条是本卡真正的坑：`published_at` 为空算不出到期；**留空被记成 0** 会在任何后续聚合里变成真实坏数据。
- [x] Scope is clearly bounded
      — 见 Out of Scope：§10.2 复盘、§10.3 经营结论、任何聚合与比较、平台 API、品牌级观察时点设置页、脱敏。
- [x] Dependencies and assumptions identified
      — 关键依赖是 025 已合入的 `content_publication_record`，其**没有 `version_id` 列**这一点直接产生了 Q2。

## Feature Readiness

- [x] All functional requirements have clear acceptance criteria
- [x] User scenarios cover primary flows
- [x] Feature meets measurable outcomes defined in Success Criteria
- [x] No implementation details leak into specification

## Notes

- **本卡只出规格，不含实现**（派单口径）。`plan.md` / `tasks.md` / `contracts/` 待裁决后再出——**Q1 会决定有几张表、有哪些列**，现在写计划等于先建表再问要建什么。
- **三处我特意没有发明**：指标字段名（Q1）、复盘占位的状态取值（Q3）、摘录来源是否受控集（Q4）。025 的 Q5 已经立过这个规矩——SOP 没给枚举就退成自由文本、只保留必填。拿到原文前写下任何一个列名，都是替 SOP 做决定。
- **一处我补的**：`content_publication_record` 没有 `version_id` 列是我从 025 的迁移文件读出来的，Issue 里「绑定发布记录与版本」这句因此有缺口。Q2 把三种补法和各自的代价都写了，没有替主控选。
- **一处跨卡的账**：026 的 FR-004 / FR-005b 在本卡落地后就过期。Q6 把它摆出来，因为**这正是那种两张卡都以为对方会处理的事**。
- 宪法 II：本卡不写 UI 单测；界面项由主控在浏览器验收。
- 宪法 X：本清单勾选只代表「已核对」，不代表验收通过。
