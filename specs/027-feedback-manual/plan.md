---
description: "Implementation plan for 027 feedback-learning — manual metrics and feedback excerpts"
---

# Implementation Plan: 人工登记真实结果与反馈摘录（027）

**Spec**: [spec.md](./spec.md) ｜ **Contract**: [contracts/feedback-manual.md](./contracts/feedback-manual.md)

**Status**: **六条 clarify 已裁决**（主控 2026-09-21，PR #163 评论）。**两张表、四个索引、六个迁移**定稿。SOP §10.1 / §7.1 / §3.2 与 PRD R-044 R-045 原文抄在 `spec.md`。

## 照抄什么，不发明什么

四处照抄既有做法：

1. **append-only 表 + 守卫用例** 照 `content_brief_revision`（022）、`content_publication_record`（025）——两张表都用它。
2. **派生显示不进存储** 照 025 的「待登记」与「到期待办」——「待补录」用它。
3. **受控集：Go 枚举为准 + 库 `CHECK` 兜底** 照 `content_topic_card.status`（022）——四个受控集都用它。
4. **跨模块不 import，读源文件对表** 照 025 的渠道对表用例——`platform` 用它。

一处**不照抄**：**没有 AI 复盘报告表**。023 / 025 遇到「这一步还没接上」时都留了列或表；本卡留的是**一个派生状态**，因为这一阶段连一行内容都产生不出来，恒空的表会被读成「它迟早会被填」。

**一处裁决明确要求不发明**：Q1 之外的自由文本字段（`unit` / `window` / `evidence_note` / `redacted_excerpt` / `interpretation` / `tags`）——§10.1 点名了字段但没给取值，按 025 Q5 的规矩不发明枚举。整卡只有**四个**受控集落在表上，外加 AI 复盘状态集那一个不落表的。

## Technical Context

**Language/Version**: Go 1.26（`server/`）、TypeScript 5 strict（`packages/core`、`packages/views`）

**Storage**: PostgreSQL。**两张新表 + 每表两个 CONCURRENTLY 索引**，每个索引单独一个迁移文件、单条语句；建表迁移不含 `PRIMARY KEY` / `UNIQUE`。`value` 用可空 `bigint`——`NULL` 就是「未知」这个值本来该干的事。

**Testing**: `go test ./internal/content/feedback-learning/`、`./internal/handler/`（按 `docs/development/testing-database-suites.md` 配 `LORETIDE_DB_TEST_*`）；`packages/core/*.test.ts`（node 环境）。**无 UI 单测。**

**Constraints**: 不调模型、不起执行器、**不发起任何外发请求**、**不存平台凭据**、**不做任何聚合**、**不自动脱敏**；页面只挂既有组件。

**Scale**: 一个品牌下每篇作品每个窗口十一个指标，量级是每天几十到几百行；摘录同量级。索引按 `workspace_id` 打头即可，不需要分区。

## Constitution Check

| 原则 | 状态 | 说明 |
|---|---|---|
| II. 不写 UI 单测 | 通过 | 判定与派生进 Go / core node 测试；界面进 `manual-ui-todo.md` |
| III. 模块边界 | **需注意** | `feedback-learning` 的依赖表是 `workspace-core` / `review-delivery` / `diagnostics`——**没有 `ip-profile`**（`platform` 自己定义 + 读源文件对表）、**没有 `work-editor`**（版本存字符串、两跳解出） |
| V. 无外键、CONCURRENTLY 索引、单语句迁移 | 通过 | R1–R6 逐条 |
| VI. zod + `parseWithFallback` | **需注意** | `value` 可空且**空≠0**，core 侧要用 `number \| null` 而不是 `number`，畸形降级也不能落成 0 |
| VIII. 范围纪律 | **需注意** | 只存不聚合；§10.2 / §10.3 只留占位；**唯一的例外是 FR-025**——裁决要求 PR 1 同时更新 026 的两条 FR 并提供第五项派生函数，这是裁决给的范围，不是我扩的 |
| IX. 真实执行器保持禁用 | 通过 | AI 复盘只产生 `pending_data`，有负例扫源码 |
| X. 打勾不是验收 | 通过 | 界面项一律记「未执行」，由主控在浏览器验收 |

## Project Structure

```text
server/
├── migrations/
│   ├── N_content_manual_metric.{up,down}.sql            # 建表，单语句，无 PK/UNIQUE
│   ├── N+1_content_manual_metric_id_unique_idx.*        # 各自单文件单语句 CONCURRENTLY
│   ├── N+2_content_manual_metric_record_idx.*
│   ├── N+3_content_feedback_excerpt.{up,down}.sql
│   ├── N+4_content_feedback_excerpt_id_unique_idx.*
│   └── N+5_content_feedback_excerpt_record_idx.*
├── internal/content/feedback-learning/
│   ├── contract.go          # 两类实体、四个受控集 + AI 复盘状态集、十列/六列的类型
│   ├── states.go            # 纯函数：待补录判定、批量校验、版本解析的形状
│   ├── states_test.go       # 矩阵（无数据库）
│   ├── metric.go            # 录一条 / 批量 / 列出
│   ├── excerpt.go           # 摘一条 / 列出
│   ├── store.go             # 栅栏 + 审计 + 公共读写
│   ├── guards_test.go       # 只插不改 / 不外发 / 不聚合 / 不合并 read+play / 无附件 / 无自动脱敏 / 无第五个受控集 / 对表
│   └── store_integration_test.go
├── internal/handler/
│   ├── content_metric.go          # 三条指标端点 + 适配器（发布记录存在性、版本两跳）
│   ├── content_feedback.go        # 三条摘录与待补录端点
│   └── *_test.go
├── cmd/server/router.go            # 挂六条路由 ┐ 同一个 upstream: 提交
└── cmd/migrate/main.go             # 四条索引登记 ┘（放进自己的块，既有行 0 删除）

packages/core/content/feedback-learning/
├── contract.ts       # zod schema 与受控集；value 是 number | null
├── contract.test.ts  # 畸形降级、未知状态保留、**空不能落成 0**（node 环境）
├── csv.ts            # 粘贴的 CSV 文本 → 行；全有或全无的校验
├── csv.test.ts
├── pending.ts        # 今日工作台第五项的派生函数（FR-025）
├── pending.test.ts
└── queries.ts        # 读写 hooks

packages/views/content/feedback-learning/
└── index.tsx         # 指标与摘录区块，只挂既有组件（页面 PR）

specs/026-today-dashboard/spec.md   # PR 1 同时更新 FR-004 / FR-005b（FR-025）
specs/027-feedback-manual/
├── contracts/feedback-manual.md
└── manual-ui-todo.md  # 页面 PR 时新建
```

## 两个 PR 的分界

| PR | 内容 | 验收口径 |
|---|---|---|
| **PR 1 存储与接口** | 两张表与六个迁移、模块、六条端点、路由与索引登记（同一个 `upstream:` 提交）、core 的 schema / CSV 解析 / 第五项派生函数、**026 两条 FR 的更新** | 受控集矩阵逐条、**空≠0**、批量全有或全无、守卫七条、删除清单两张表各一条、版本两跳解出与 `unknown` |
| **PR 2 页面** | 指标与摘录区块、粘贴 CSV、AI 复盘占位、工作台第五项接上、四语言、`manual-ui-todo.md` | 只挂既有组件、无 UI 单测；界面项全部记「未执行」 |

**PR 2 要等 #161（025 页面）合入**——本卡的区块挂在 025 发布记录那一段之后，用它的组件上的插槽。

## 风险与对策

| 风险 | 对策 |
|---|---|
| **`value` 的空被某一层折成 0** | 这是本卡最容易悄悄发生的数据损坏，而且不一致时没有任何东西会报警。三处各一条用例：Go 存取（SC-002）、core 解析（`number \| null`，畸形降级也不落 0）、读端展示。**不是一条，是三条** |
| 批量部分写入 | 全有或全无（FR-011a / SC-007）；拒绝点名第几行的哪一列。「录上了几条」是这个功能唯一要回答的问题 |
| 受控集在 Go / TS 两侧各写一份会漂移 | 照 025：前端用例**读 Go 源文件**比对（`metric` 十一项、两个 `source_type`、AI 状态七项） |
| 「阅读」「播放」被后人合并成一个指标 | SC-009 两条：检索用例禁止合并/排名路径，外加两者互不影响的读用例 |
| AI 复盘的其余六个状态被谁顺手产生 | 扫源码的负例（照 024 对 `generated`）；状态集是受控集保留项，不是能力 |
| **FR-025 的跨卡改动被漏掉** | 裁决把它写成 PR 1 的交付要求（SC-022），不是一句提醒；PR 正文要单列「对 026 的改动」 |
| 版本解不出被误当成错误 | `unknown` 与 025 `version_match` 同口径；SC-015 明确断言**不拒绝录入** |
