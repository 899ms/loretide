# Specification Quality Checklist: 诊断 HTTP 追踪贯通与请求脱敏

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

初稿两项未通过，原因是 FR-015～FR-017 三个 [NEEDS CLARIFICATION]。主任务已于 2026-09-14 逐条回答并写回 `## Clarifications`：

| 项 | 决定 | 对 spec 的影响 |
|---|---|---|
| FR-015 挂载范围 | 传播挂全部 API 路由的公共中间件层；记录范围不变，仍只有 `/api/content-diagnostics` 写技术事件 | FR-015 重写为「传播全局、记录模块内」两个关注点；US1 新增验收场景 7（业务路由只传播不记录）；Edge Cases 新增「不得改变非诊断路由的响应形状与错误语义」 |
| FR-016 入站信任 | 边界一律新建本实例 trace；用户凭据路径的入站值仅作关联属性（不作查询键、不跨 workspace 关联、不用于授权）；只有 daemon 的 machine credential 路径采信为父级 | FR-001 改为「一律本实例新建」；FR-004 拆为 FR-004（按来源区分）与 FR-004a（非法值既不采信也不留存）；FR-016 补「判定基于已完成的认证结果，不依赖消息自称身份」；US1 描述与验收场景 1～4 重写 |
| FR-017 outbox 形态 | 只定接口：事务内登记 + 提交后派发，不落库、不新增迁移；登记项含 id / 类型 / 载荷 / 幂等键，签名可被持久实现替换 | FR-017 重写；FR-010 拆出 FR-010a 明确「进程退出会丢失未派发项」；US3 描述、Independent Test、验收场景 2/4 调整，新增场景 6 把该限制写成验收项；Edge Cases 对应条改为「明确接受的限制」 |

复验结果：两项现均通过。**FR-010a 与 US3 场景 6 是刻意把一条已知限制写成验收项**——按 constitution 原则 X 与仓库既有口径，未保证的事不能靠沉默蒙混过去，要能被读出来。

### 一项待确认，不阻塞

技术事件的请求身份形状（Clarifications 第 4 条）暂定「路由模板 + HTTP 方法」，主任务未回答。这是四个候选里最保守的一个，可证明不含用户取值；若后续改为更细的形状属放宽而非纠错，已写在 Assumptions。**不计为清单未通过项**：FR-006 的验收判定在暂定形状下是明确且可测的。

### 关于 Current State 的写法

本 spec 的 Current State 逐条标注了「合同已存在 / 某一段未接线」，并给出文件与行号级的核实结果（`app-main` @ `0bd37da87`）。这是为了避免把「已有实现」误读成「没有代码」——`docs/development/diagnostics-acceptance-mapping.md` 与 `tasks/diagnostics.md` 都有同样的提醒。

### 下一步

进入 `/speckit-plan`。
