# Specification Quality Checklist: 平台搜索优化（036）

**Purpose**: Validate specification completeness and quality before proceeding to implementation
**Created**: 2026-09-25
**Feature**: [spec.md](../spec.md)

## Content Quality

- [x] No implementation details (languages, frameworks, APIs)
      — Current State 一节刻意引用真实文件、行号与规则编号，这是本仓库规格的惯例（「以代码为准」）。FR 里出现的列名、端点与原因码是验收要能直接断言的名字，与 034 / 035 同一写法。
- [x] Focused on user value and business needs
- [x] Written for non-technical stakeholders
      — 部分受限：读者是主控与执行会话，采用的三步顺序、幂等键与错误映射需要写到可断言的程度。
- [x] All mandatory sections completed

## Requirement Completeness

- [ ] No [NEEDS CLARIFICATION] markers remain
      — **未勾**：Q1～Q8 待主控裁定（spec 文末）。每题的推荐值已写进 FR，裁定不同时相应 FR 要改。
- [x] Requirements are testable and unambiguous
      — 差异算法、状态派生、预检查与采用的顺序、幂等键、未知的表示方式都写死了取值与断言形状；差异的固定样例进了 contract §6。
- [x] Success criteria are measurable
- [x] Success criteria are technology-agnostic (no implementation details)
      — SC-007 / SC-012 / SC-013 提到的守卫与迁移规则是本仓库既有的可运行检查。
- [x] All acceptance scenarios are defined
- [x] Edge cases are identified
      — 本卡真正的坑有四个：未知被显示成 0 或估算数；单次观察被显示成排名；采用后旧批准被带到新版本；放弃时顺手改了东西。每个都有 FR、SC 和手验项（U-05、U-24、U-13、U-16）。另有两个实现上的坑：幂等 `Claim` 必须在基础版本核对之前（否则重试失败），以及每个适配器都要把已删除的工作区映射成 404（035 PR 1 的教训）。
- [x] Scope is clearly bounded
      — AI 建议、联网研究、搜索量接口、CSV 导入、诊断接入、预检运行明确不做。
- [x] Dependencies and assumptions identified
      — `modules` 依赖表不改；`adapters` 追加待批准（plan.md「主控决定」第 2 条），由各实施 PR 自己加；本规格 PR 未改 `scripts/content-boundaries.json`。`work-editor` 有一处公开契约新增（`ApplyBody` 与一个动作值）。

## Feature Readiness

- [x] All functional requirements have clear acceptance criteria
- [x] User scenarios cover primary flows
- [x] Feature meets measurable outcomes defined in Success Criteria
- [x] No implementation details leak into specification

## D14 验收覆盖

| 编号 | 覆盖 | 在哪 |
|---|---|---|
| D14-V09 | 全部（「本地模式不联网」以守卫证明：本版只有本地人工模式） | FR-001～FR-005、FR-010～FR-019、FR-080、FR-084；SC-001、SC-002、SC-013；T008～T020；U-02～U-06、U-28 |
| D14-V10 | 比较、采用、放弃、不沿用旧终审、观察保留窗口与条件：全部。**旧预检报告过期：只能结构性证明**——EP-06 未实现，代码里没有预检报告；本卡保证新 `version_id` 与正文变化，并把「报告按 `version_id` 绑定」写进合同 §8 | FR-030～FR-060、FR-070～FR-075；SC-003～SC-007、SC-009、SC-010；T037～T062、T075～T077；U-08～U-25 |
| D14-V15 | **部分**（待主控确认接受）：关键词 → 候选 → 采用 → 审核 → 发布记录 → 搜索观测的服务端链路自动化；放弃不改任何东西自动化；浏览器全程手验；成本 / 线索 / 成交 / 复盘 / 采纳由 034、035 负责；AI 分析不适用 | SC-008、SC-014；T050、T082；U-16、U-30；spec「D14-V15 覆盖说明」 |

## Notes

- **本卡只出规格，不含实现**（派单口径）。
- 宪法 II：本卡不写 UI 单测；界面项由用户在浏览器验收。T089、T090 读的是纯函数与 JSON 文案，不是 UI 单测。
- 宪法 X：本清单勾选只代表「已核对」，不代表验收通过。
