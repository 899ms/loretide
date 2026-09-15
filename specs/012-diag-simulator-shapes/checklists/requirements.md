# Specification Quality Checklist: 诊断模拟器的数据形态

**Purpose**: Validate specification completeness and quality before proceeding to planning
**Created**: 2026-09-15
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

### 关于「No implementation details」

Current State 大量引用文件、行号与符号名（`simulator.go` 的 `Child(ctx)` 链、`service.go:25` 的无条件 `Evaluate`、`index.tsx:574` 的 `limit: "25"`）。这是刻意的，不计为违反：本特性要回答的问题就是「**今天为什么造不出**」，不指到具体那一行就只能说「大概是设计如此」。Requirements 与 Success Criteria 两节不含符号名。与 005 / 007 / 008 / 009 / 010 同一口径。

### 逐条核实的结果改变了范围

任务描述列了 (a)～(h)，逐条核实后**两条要移出**：

| 条目 | 核实结果 | 处理 |
|---|---|---|
| **008-O-2** 追踪内事件都没有版本 | **今天已经可以做**。面板「触发页面错误（测试）」按钮走 `Service.ClientError`，该事件不设 `Version` 且自带全新 `Trace` | 移出（FR-021），只改 runbook 备注 |
| **006-L-5** 原故障运行不可读 | 要的是「被引用的运行已被保留期裁剪或跨工作区不可读」——那是**保留期与授权**行为。让模拟器造一个「不可读的运行」等于让它写一条本不该存在的记录 | 移出（FR-020），改由保留期配置或跨工作区引用验证 |

硬约束原文要求「若某一项确实不该由模拟器承担，MUST 在规格里显式排除并说明理由，不要硬凑」——这两条正是。

### 一处数目对不上，已抬到 Current State 第 0 节

描述说 **19 条**，(a)～(h) 实际点名 **15 条**。差的 4 条是哪几条未知。规格覆盖点名的 15 条并把差异写进 FR-001 与交付记录要求，而不是假装 15 就是 19。**不补全的风险不是漏做，而是交付后仍有 4 条卡在同一个原因上，且没人知道是哪 4 条。**

### 三处 Q 按推荐值暂定，不阻塞

| 项 | 问题 | 推荐值 |
|---|---|---|
| Q1 | 19 与 15 的差额是哪 4 条 | 覆盖点名的 15 条，差异写进交付记录 |
| Q2 | 新形态在 `Scenarios` 里怎么标注才不让 §7 对表口径变糊 | 给 `Scenario` 加分类字段，让归属成为**结构里的事实**而非文档约定 |
| Q3 | 第 5 类注入点的门禁 + 两条移出 | 复用既有 `testEnabled` 判定，装配处注入；两条移出如上 |

三者都**不改变任何 FR 的可测性**。**不计为清单未通过项。**

### US2 的优先级说明

US2（隔离门禁）与 US1 同为 P1，但在冲突时**US2 优先**。本特性做的事情本质上是「让系统能产出反常数据」——一个能在生产伪造 `passed` 的开关，比 19 条跑不了的验收项危险得多。FR-017 因此要求**每一类新形态各有一条负例断言**，而不是一条笼统的「门禁有效」。

### 下一步

`/speckit-clarify`（三处 Q 待主任务裁决）→ `/speckit-plan` → `/speckit-tasks` → `/speckit-analyze`。**实施排在 `specs/011` 合入之后**，两者都动 `simulator.go`。
