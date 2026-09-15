# Specification Quality Checklist: 品牌级自动预检开关（LT-015）

**Created**: 2026-09-15 · **Feature**: [spec.md](../spec.md)

## Content Quality

- [x] No implementation details (languages, frameworks, APIs)
- [x] Focused on user value and business needs
- [x] Written for non-technical stakeholders
- [x] All mandatory sections completed

## Requirement Completeness

- [x] No [NEEDS CLARIFICATION] markers remain
- [x] Requirements are testable and unambiguous
- [x] Success criteria are measurable
- [x] Success criteria are technology-agnostic
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

### 「不提供账号级覆盖」不能靠「字段不存在」成立

核实 `packages/core/content/ip-profile/contract.ts:12` 后改了负例的写法：账号的 `settings` 是 `z.record(z.string(), z.unknown())`，**能放任意键**。所以「schema 里没有这个字段」只证明**今天没人加过**，不证明加了也没用。

必须断言的是：把 `loretide.auto_precheck` 写进某个账号的 `settings`，**生效值不变**。加上纯函数只接受工作区、类型上就传不进账号，这条 SOP 规则才真的被守住。见 FR-009。

### `false` 是布尔零值——这是本卡唯一不能照抄时区的地方

时区那套用「有没有字符串值」判断是否已设置。照搬到布尔上，**关闭会被当成未设置，再被默认值翻回开**。用户关掉预检、第二天发现它自己开了，而且没有任何记录说明发生过什么。

因此「读到什么值」与「是否已显式设置」是两个独立判断，两端各有断言（core 的 `hasStoredAutoPrecheck`、Go 的 `StoredFalseSurvivesAReadAsFalse`），变异验证也各打了一枪（M2、M6、M8）。

### 本卡只做配置

SOP 那句话有四个子句，「提审时触发」属于 EP-06。所以验收里没有任何一条是「预检跑了没有」——开关交付后**不驱动任何运行**。把这一点写进 spec 的 Assumptions，免得下一轮验收时有人按「预检应该跑起来」去验一个还没实现的东西。

### 三处 Q 已裁决

| 项 | 裁决 |
|---|---|
| Q1 | A —— 说明句只在关闭时出现，开关行常驻一句功能描述 |
| Q2 | A —— 存量品牌一律读作 `true`，不引入三态 |
| Q3 | A —— onboarding 不放控件，创建接口仍校验非布尔 400 |
