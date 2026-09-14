# Specification Quality Checklist: DIAG-04 持久 outbox

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

### 关于「No implementation details」这一项

Current State 一节**引用了具体符号名、文件路径与行号**（`dispatch.go` 的 `Outbox`、`LogBuffer.Dropped`、`service.go:17`、迁移 `468`/`472`/`473`）。这是刻意的，不计为违反：

- 本特性的对象**就是**一段既有代码，规格必须说清它今天是什么样，否则「换成落库实现」没有起点；
- 这些引用集中在 Current State 与硬约束里，**Requirements 与 Success Criteria 两节不含任何符号名或技术选型**——那两节才是可测性与技术无关性的检验对象；
- 前几个特性（005 / 007 / 008）用的是同一口径。

### 三处 Q 已由主任务裁决（2026-09-14）

| 项 | 裁决 | 连带改动 |
|---|---|---|
| Q1 调用点 | **A**：新增 `Store` 入口承载「同事务业务写入 + 登记」，`Store.Audit` 一字不动。**补充**：该入口只能承载 diagnostics 模块自己的业务写入，不得触碰其他模块表 | FR-005 定稿；新增 **FR-005a**、**SC-011** |
| Q2 排水器 | **A**：提交后就地排水 + 进程内周期扫描。**补充**：认领用行级租约（`claimed_until` 之类）、派发幂等、扫描间隔可配置有默认值且测试可缩短 | FR-008/FR-009 定稿；新增 **FR-009a**、**FR-009b**、**SC-010**、**SC-012** |
| Q3 拿不到真事务 | **A**：失败关闭，`Register` 返回错误 | FR-004 定稿 |

spec.md 内已无「暂定」字样。FR 28 条、SC 12 条。

### §2 是本规格最重要的一条

Current State §2 记的是「**今天没有任何生产调用方**」。这不是背景，是约束：需求原话「与业务写入同事务」在今天没有真实事务可挂靠。规格把它抬成 Q1 而不是留给实施阶段自己决定，因为两种答案会产出完全不同的交付物（一个动 `Store`，一个不动）。

### 一条刻意不做的简化

「把 `TestOutboxDoesNotSurviveTheProcess` 反转」听上去是改一条断言，实际不能就地改：它断言的主语是 `MemoryOutbox`，而 `MemoryOutbox` 按硬约束要保留，它确实不跨进程。FR-014 因此要求**两条断言并存**，互为反面。就地反转会悄悄删掉对保留下来的那个实现的限制记录。

### 下一步

`/speckit-clarify`（三处 Q 待主任务裁决）→ `/speckit-plan` → `/speckit-tasks` → `/speckit-analyze`。
