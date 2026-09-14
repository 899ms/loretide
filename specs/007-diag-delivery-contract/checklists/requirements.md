# Specification Quality Checklist: 诊断接入合同与交付检查

**Purpose**: Validate specification completeness and quality before proceeding to planning
**Created**: 2026-09-14
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

### 全部通过（2026-09-14 clarify 回写后复验）

| 项 | 决定 | 对 spec 的影响 |
|---|---|---|
| FR-003 错误码枚举 | 清单只写「申请导出」，导出列后续任务；**本功能不改生产代码**，检查不得要求该枚举存在 | FR-003 重写，去掉「是否顺带导出」的分支 |
| FR-011 最小证据 | E1 import + E2 三类调用点之一 + E3 测试引用，**三条同时满足**；缺哪条报哪条 | FR-011 拆为三条具名条件，新增 FR-011a（缺项粒度）；FR-006 补引用；SC-002 改为三个分别只缺一条的夹具 |
| FR-016 覆盖范围 | 静态检查只管服务端；前端两根写进合同文本、不做检查 | FR-016 收窄，新增 FR-016a（前端要求入文本 + 注明无静态检查） |

### 两项按推荐值暂定，不阻塞

CI 接入与豁免登记位置主任务未答，已按推荐项写入 Clarifications 与 Assumptions 并标注「暂定（待主任务确认）」。两者都不改变任何 FR 的可测性：前者是一个 CI 步骤的有无，后者是一个配置文件的位置，改动量各一行。**不计为清单未通过项。**

### 关于 §2 的记录

Current State 专门记了「12 个模块中 11 个尚无目录」。这不是背景，是**约束**：它决定了检查脚本必须以「模块落地才生效」为默认，否则本功能交付的第一天就会产生 11 条无法处理的红线。FR-005 与 SC-003 把这一点钉成了验收条件。

### 一条不会被本功能闭合的子句

D13-V12 的第二条（真实 Codex 及远程阶段实测）在执行器禁用期间**不可能通过**。FR-017 明写交付不得声称 D13-V12 整体通过；FR-014 进一步要求不存在「真实执行器未跑却全绿」的状态。这是本规格刻意保留的未闭合项，不是遗漏。

### 下一步

进入 `/speckit-plan`。
