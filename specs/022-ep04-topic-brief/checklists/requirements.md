# Specification Quality Checklist: 手工选题卡与冻结简报（EP-04a）

**Created**: 2026-09-15 · **Feature**: [spec.md](../spec.md) · **拆分**: [ep04-breakdown.md](../ep04-breakdown.md)

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

### Q1 的答案在 `docs/12` §2 里已经写了

任务卡要求在 clarify 里给出「复用上游 `issue` vs 新建 content 表」的选项与各自代价。核实后发现这不完全是一个开放选择：**`docs/12` §2 的数据所有权表已经把「选题、理由、创作简报」判给 `topic-planning`**，而 `issue` 不归任何 content 模块所有。

所以复用 `issue` 的代价里，最贵的一项不是技术耦合，而是**要先改 §2**——那是文档仓库的决定，不是这张卡能顺手做的。技术耦合（看板 `Position`、`issue_dependency` / `issue_label` / `issue_reaction`、`agent_task_queue` 的认领、删除事务的 `FOR UPDATE` 栅栏）是第二层代价，已逐项列在 spec 的对照表里。

**推荐 B，但代价诚实写出来**：看板、标签、评论、指派这些能力不会白得，将来若要「在看板上看选题」需要另做。

### 一条本卡验不了的要求，单独立了 FR

§5.3 的「既有运行继续引用它开始时的版本」——**今天没有运行**（`agent-workflow` 未落地）。结构上的 append-only 能保证「旧版本还在」，但保证不了「某次运行看到的还是它钉的那一版」，因为没有东西在钉。

写成 FR-018 而不是在实施时才发现，是因为它决定验收怎么写。**把结构正确当成这条已验收，等 agent-workflow 落地时不会有人回头补这条断言。**

### 拆分方案里两处对骨架的调整

1. **EP-04b 的前置缺口**：任务卡把「最小可开始条件」记作 `specs/021`，但本仓库 `specs/` 只到 `020`。派 EP-04b 之前要确认那份规格的去向——不补齐，「最小」包含哪几项没有依据。
2. **EP-04a 的未验证项**：即上一节。

其余四张卡的边界、名称、交付与依赖**照骨架采纳**。

### 四处 Q 按推荐值暂定，不阻塞

| 项 | 问题 | 推荐值 |
|---|---|---|
| Q1 | 复用上游 `issue` vs 新建 content 表 | **B 新建**；§2 归属表已判过，复用要先改 §2 |
| Q2 | 选题卡挂品牌还是品牌 + 账号 | **A 挂品牌，账号可空**；定渠道前可能还没选账号 |
| Q3 | 暂缓 / 放弃的偏好信号存哪里 | **A 存在卡自己这一行上**，不建信号表、不聚合——聚合属 `feedback-learning` |
| Q4 | EP-04a 要不要再拆两张卡 | **A 不拆，一张卡两个 PR**；「开始」同时改状态并产出首版，拆开会让一个动作横跨两卡 |

---

## `/speckit-analyze` 复核（2026-09-15）

跨产物一致性检查：`ep04-breakdown.md` / `spec.md` / `plan.md` / `contracts/` / `tasks.md`。

### 覆盖

- **FR-001 ～ FR-023 全部有对应任务**（`tasks.md` 每条带 FR/SC 映射），无孤儿需求。
- 任务编号 **T001 ～ T041**，无重复、无断号、单调递增，全部匹配 `^- \[ \] T[0-9]{3}( \[P\])?( \[US[0-9]\])?`。
- 引用的 FR / SC 编号全部存在。

### 复核中补上的两处

| # | 问题 | 整改 |
|---|---|---|
| 1 | **FR-019 / FR-020 / SC-012 没有任何任务覆盖**——「不调模型、不读文件、不写快照、不生成候选、不做必用排除」全是否定式要求，而否定式要求最容易在交付时变成一句「我没写」而不是一条可复核的证据 | 新增 **T032 范围自证**，要求逐项给出可复核的检索或计数 |
| 2 | 任务清单原先不带 FR 映射 | 每条补上 `— **FR-xxx**`，与 `specs/010` 的写法一致。这不是形式：实施时按任务走，没有映射就无法回答「这条 FR 谁在管」 |

### 四处 Q 已裁决

| 项 | 裁决 |
|---|---|
| Q1 | **B** —— `topic-planning` 自建表；`docs/12` §2 归属已判，不动上游 `issue`。**看板 / 标签 / 评论 / 指派能力不白得**，已写进 spec 的 Assumptions |
| Q2 | **A** —— 挂品牌，`account_id` 可空 |
| Q3 | **A** —— 暂缓 / 放弃的原因与备注存卡上，不建信号表、不聚合；聚合归 `feedback-learning` |
| Q4 | **A** —— 一张卡两个 PR：先存储与接口，后页面 |

### 拆分方案两处调整已被接受

1. **EP-04b 的依赖写成 `specs/021`（实施中）**——另一会话正在 `claude/spec-021-account-expression-profile` 上做，EP-04b 等它合入后再派。
2. **§5.3 那条改为结构保证口径**，行为断言留 agent-workflow。

