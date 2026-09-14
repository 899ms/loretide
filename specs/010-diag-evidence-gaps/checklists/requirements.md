# Specification Quality Checklist: 补齐「代码存在但无测试」的定向证据

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

Current State 引用了具体文件、行号与测试名。这是刻意的，不计为违反：本特性的对象**就是「某一行有没有测试」**，不点名测试就无法判定。Requirements 与 Success Criteria 两节不含符号名或技术选型——那两节才是可测性的检验对象。与 005 / 007 / 008 / 009 同一口径。

### 逐行核实改变了交付物构成

任务描述点名 8 条。逐行核实后：

| | 条数 | 处理 |
|---|---:|---|
| 已有测试，只需改对照表引用 | **2** | DIAG-02 私有正文脱敏、DIAG-12 下载文件名与原始字节 |
| 写测试也关不掉（缺功能） | **1** | DIAG-03 级别配置 → Q1 |
| 真实缺口，新增测试 | **6** | DIAG-04、DIAG-05、DIAG-07 ×2、DIAG-08、DIAG-12 不自动上传 |

**「补 8 条测试」会有 3 条是错的**：2 条重复、1 条测不到。规格把这一点抬到 Current State 第 0 节，而不是留给实施阶段发现。

### 一处自相矛盾，本特性会改掉

对照表 DIAG-12「下载保存服务端原始字节与文件名」行的**类型是「代码存在但无测试」，备注却写着 handler 侧测试「已通过」**。类型与备注互相打架。FR-004 与 SC-002 把它钉成验收条件。

### 三处 Q 按推荐值暂定，不阻塞

Q1（DIAG-03 缺功能怎么记）、Q2（两条未点名的非界面行纳不纳入）、Q3（不自动上传的核对形式）已按推荐值写入 Clarifications 与对应 FR。三者都**不改变任何 FR 的可测性**。**不计为清单未通过项。**

### 下一步

`/speckit-clarify`（三处 Q 待主任务裁决）→ `/speckit-plan` → `/speckit-tasks` → `/speckit-analyze`。
