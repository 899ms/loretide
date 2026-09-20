# Specification Quality Checklist: 选题卡引用素材条目（030）

**Purpose**: Validate specification completeness and quality before proceeding to planning
**Created**: 2026-09-20
**Updated**: 2026-09-21（裁决回写后重跑）
**Feature**: [spec.md](../spec.md)

## Content Quality

- [x] No implementation details (languages, frameworks, APIs)
- [x] Focused on user value and business needs
- [x] Written for non-technical stakeholders
- [x] All mandatory sections completed

**说明（前两项为什么打勾而不是打叉）**：本仓的规格按 `docs/development/spec-kit-workflow.md` 第 1 步要求「Current State 一节以代码为准，逐条核实并给出文件与行号」。Current State 与裁决记录里的文件名、行号、迁移编号，是**核实记录**，不是实现方案；它们回答的是「今天是什么样」，不是「要怎么写」。实现方案在 `plan.md` 与 `contracts/topic-source-refs.md`。User Scenarios、Requirements 与 Success Criteria 三节按行为写。

## Requirement Completeness

- [x] No [NEEDS CLARIFICATION] markers remain
- [x] Requirements are testable and unambiguous
- [x] Success criteria are measurable
- [ ] Success criteria are technology-agnostic (no implementation details)
- [x] All acceptance scenarios are defined
- [x] Edge cases are identified
- [x] Scope is clearly bounded
- [x] Dependencies and assumptions identified

**第一项**：三条 clarify 已于 2026-09-21 全部裁决（PR #207 评论），已回写为「裁决记录」一节，spec 内**零处「暂定」**（`grep -c 暂定 spec.md` = 0）。

**未打勾的一项**：SC-008、SC-010、SC-011、SC-014 点了具体的测试名、脚本名与文件名（`TestBriefStoreHasNoUpdateOrDeletePath`、`TestNoSourcelessFieldIsInvented`、`check:content-boundaries`、`router.go`、`workspace_delete_manifest_test.go`）。**这是故意的，不是疏漏**：这四条的验收对象就是「那两条既有守卫仍然绿且一行未改」「那几份登记表零改动」，脱离具体名字就无法验证。模板这一条针对的是「用 API 响应时间代替用户可感知的结果」那一类；其余十二条 SC 均为行为口径。

## Feature Readiness

- [x] All functional requirements have clear acceptance criteria
- [x] User scenarios cover primary flows
- [x] Feature meets measurable outcomes defined in Success Criteria
- [x] No implementation details leak into specification

## FR / SC 覆盖对照（analyze 会查的那一项）

`tasks.md` 的 41 条任务逐条标了它覆盖的 FR / SC。**没有任务覆盖的 FR 或 SC 是覆盖缺口**，本轮自查结果：

| 未被任务直接点名的 | 为什么不是缺口 |
|---|---|
| FR-002（无外键无级联） | 由 T006 的迁移逐行复核覆盖（它点的是 FR-003 与 SC-014，同一次复核） |
| FR-024（不调用执行器） | 本卡零执行器代码；由 T034 的「人手选的」说明与 028 既有的 SC-004 覆盖 |
| FR-025 / FR-027（授权与诊断合同） | 沿用既有卡端点的口径，由 T016（同事务审计）与 T038（三项 check）覆盖 |
| FR-028 / FR-029 / FR-031（三条「不做」） | 由「无此端点、无此查询、无此改动」的事实覆盖；contract §7 逐条列了守卫 |

## 本规格自己发现的三处，记在这里

1. **选题卡的七项今天根本改不了**（Current State 第 2 节）。Issue #204 没有提到。它把「给引用开一条写入路径」从可选项变成必需品，US2 因此是 P1 而不是 P3。**裁决已接受**，并把「其余五项仍不可编辑」明确记入 Out of Scope 6。
2. **引用挂错了项**（「原文依据」第二节）。初稿按 Issue 的转述挂在第 4 项「与已有作品的关系」与第 5 项。裁决贴回 §5.2 原文后更正：「引用哪些素材」五个字在**第 2 项「为什么适合这个 IP」**里；第 4 项指的是**作品**，不是素材。字段因此叫 `fit_source_ids` 而不是 `relation_source_ids`。**这一处更正在 `tasks.md` 里有两条任务各防一次**（T012 的契约负例、T032 的页面任务）。
3. **「过去的经营结论」没有地方可引**（§5.2 第 2 项后半句）。§10.3 的 Learning 未落地，027 的 FR-023 明写不为它建表。本卡**不留字段**——一个恒空的字段和 023 的 `required_sources` 是同一种谎，而那件事 023 是拿负例挡住的。**裁决已确认。**

## Notes

- 三条 clarify 裁决已回写，spec 内零处「暂定」（第 2 步）
- `plan.md` 的 Project Structure 是实施阶段的改动白名单（第 7 步）
- `tasks.md` 编号 T001–T041 连续、无重号、无缺号、无 `T008a` 式编号（第 4 步）
- 按宪法 X：`tasks.md` 的勾选只表示该实施任务已交付，不表示已合并，也不表示主任务已验收
