# Specification Quality Checklist: 品牌/账号经营诊断（035）

**Purpose**: Validate specification completeness and quality before proceeding to implementation
**Created**: 2026-09-25
**Feature**: [spec.md](../spec.md)

## Content Quality

- [x] No implementation details (languages, frameworks, APIs)
      — Current State 一节刻意引用真实文件、行号与规则编号，这是本仓库规格的惯例（「以代码为准」）。FR 里出现的列名、端点与原因码是验收要能直接断言的名字，与 027 / 034 同一写法。
- [x] Focused on user value and business needs
- [x] Written for non-technical stakeholders
      — 部分受限：读者是主控与执行会话，计算口径需要精确到取哪一次采样、哪一周。
- [x] All mandatory sections completed

## Requirement Completeness

- [x] No [NEEDS CLARIFICATION] markers remain
      — **裁决后勾上**（主控 2026-09-25，PR #267 评论）：Q1～Q8 采纳推荐值，Q3 补充建卡幂等键；spec 文末改为「裁决记录」。
- [x] Requirements are testable and unambiguous
      — 窗口边界、账号归属、采样取法、周的定义、缺口键、引用键都写死了取值与断言形状；固定样例逐字进了 contract §5.10。
- [x] Success criteria are measurable
- [x] Success criteria are technology-agnostic (no implementation details)
      — SC-012 / SC-013 提到的守卫与迁移规则是本仓库既有的可运行检查。
- [x] All acceptance scenarios are defined
- [x] Edge cases are identified
      — 本卡真正的坑有四个：未知被显示成 0；两个平台的数字被放进同一排名；差值被读成原因；拒绝时顺手改了配置。每个都有 FR、SC 和手验项（U-10、U-12、U-14、U-24）。
- [x] Scope is clearly bounded
      — AI 判断层（只留挂接）、经营记忆、今日工作台接入、账号级授权记录、营销节点关联明确不做。D14-V08 只能部分自动化，SC-014 如实写。
- [x] Dependencies and assumptions identified
      — `modules` 依赖表不改；`adapters` 追加待批准（plan.md「主控决定」第 2 条），由各实施 PR 自己加；本规格 PR 未改 `scripts/content-boundaries.json`。ROI 引用依赖 #266（Q6）。

## Feature Readiness

- [x] All functional requirements have clear acceptance criteria
- [x] User scenarios cover primary flows
- [x] Feature meets measurable outcomes defined in Success Criteria
- [x] No implementation details leak into specification

## D14 验收覆盖

| 编号 | 覆盖 | 在哪 |
|---|---|---|
| D14-V01 | 全部（本版可越权读的「账号专属材料」不存在，结构上保证诊断不读素材与知识） | FR-080～FR-082；SC-009；T026、T015；U-35 |
| D14-V04 | 全部 | FR-012～FR-015、FR-020～FR-029、FR-033；SC-001～SC-003；T038～T047、T087；U-09～U-14 |
| D14-V05 | 全部 | FR-040～FR-045、FR-060～FR-069（含 FR-063a 幂等重试）；SC-004～SC-008；T017～T019、T065～T074（含 T068a、T070a）；U-19、U-24～U-33 |
| D14-V08 | **部分**（主控 2026-09-25 已接受）：服务端链路自动化；浏览器闭环手验；真实联网本版不适用 | SC-014；T079；U-40；spec「D14-V08 覆盖说明」 |

## Notes

- **本卡只出规格，不含实现**（派单口径）。
- 宪法 II：本卡不写 UI 单测；界面项由用户在浏览器验收。T087 读的是 JSON 文案，不是 UI 单测。
- 宪法 X：本清单勾选只代表「已核对」，不代表验收通过。
