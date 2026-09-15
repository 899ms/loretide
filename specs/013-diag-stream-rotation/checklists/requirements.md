# Specification Quality Checklist: 计划轮换页必须在窗口内送达

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

### 「服务端不发 rotate」是错的，缺陷比那更具体

主任务的实测是「26 行、0 条含 rotate」，容易读成「服务端没发轮换页」。本机复现证明**服务端确实会发**（真实 ticker、真实 deadline，三个窗口长度各三轮，九次全中）。

真正的缺陷是**它发得太晚**：轮换页被安排在窗口到期之后新起的一轮里，写它之前还要做两次数据库查询。主任务实测的 `25011ms` 正是那两次查询的耗时（δ ≈ 11ms），而连接在窗口边界就没了。

这个区别不是措辞问题——它决定了修法。若是「不发」，就该去找发的逻辑；实际是「晚发」，就该把判定提前。

### 现有测试为什么没拦住

`TestContentDiagnosticStreamClosesPlannedWindowWithRotate` 用 **50ms** 窗口，而 ticker 是 **1 秒**。截止定时器在 ticker 一次都没跳之前就到期——**这条路径生产环境永远走不到**。它一直是绿的，而 PR #21 据此宣布「计划轮换对用户不可见」已闭合。

**假信号比没有覆盖更贵。** FR-006 因此把「窗口必须是 ticker 周期的整数倍」写成需求，而不是留作建议。

### 只断言「末页 rotate 为真」是不够的

改动前的代码也能让末页 rotate 为真。缺陷恰恰在于它**晚了 11 毫秒**。所以回归测试必须断言**结束时刻早于窗口边界**（FR-006、SC-002），否则等于没测这次修复。

### 三处 Q 按推荐值暂定，不阻塞

| 项 | 问题 | 推荐值 |
|---|---|---|
| Q1 | 轮换提前多少 | 提前一个 ticker 周期（现成的量，且正好是「下一页什么时候来」的答案） |
| Q2 | 现有 50ms 测试怎么办 | 窗口改为 ticker 周期整数倍；另留一条用例覆盖退化形态 |
| Q3 | 是否同时动代理 | 不动。窗口内送达之后代理超时不再参与正确性；要查另开一条，不与本修复捆绑 |

### `002-V05-03` 记录但不解决

界面文案与条目互相矛盾（文案说「不应用上方历史筛选」，条目要求筛选跟随，而**实现是筛选跟随**）。按 FR-012 记录并给建议（改文案不改实现），留给主任务裁决。
