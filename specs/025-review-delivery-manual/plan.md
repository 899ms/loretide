---
description: "Implementation plan for 025 review-delivery — manual review, delivery and publication"
---

# Implementation Plan: 人工审核请求、交付任务与发布记录（025）

**Spec**: [spec.md](./spec.md) ｜ **Contract**: [contracts/review-delivery.md](./contracts/review-delivery.md)

**Status**: **五条 clarify 已裁决**（主控 2026-09-21，PR #144 评论）。Q1=A 与 Q3=A 都不改表的数量，**四张表、十二个迁移定稿**。SOP §8 / §9.1 / §9.2 / §9.3 原文抄在 `spec.md`。

## 照抄什么，不发明什么

三处照抄既有做法：

1. **append-only 表 + 守卫用例** 照 `content_brief_revision`（022）与 `content_start_snapshot`（023）——发布记录与迁移记录用它。
2. **不可变 jsonb 快照** 照 `content_start_snapshot.snapshot`（023）——交付快照用它。
3. **可变状态列 + 受控集** 照 `content_topic_card.status`（022）——审核请求与交付任务的 `status` 用它，Go 枚举为准、库 `CHECK` 兜底。

第四处照抄：**派生显示不进存储**——「待登记」与「到期待办」照 022 列表筛选的做法在读路径算，不新增状态值。

一处**不照抄**：**不用审计事件当产品历史**。审计是诊断读模型（脱敏、保留期、component 白名单），产品历史要长期逐字保留，所以迁移记录是自己的表。

**一处裁决明确要求不发明**：Q5——SOP §9.2 没有给声明者与核验方式的枚举，所以本卡**不定义**这两个受控集，它们是自由文本。整个规格里只剩下五个受控集：`channel`（4）、三套 `status`（5 / 6 / 5）、`handoff_method`（3）、`version_match`（3）、`subject_kind`（2）。

## Technical Context

**Language/Version**: Go 1.26（`server/`）、TypeScript 5 strict（`packages/core`、`packages/views`）

**Storage**: PostgreSQL。**四张新表 + 每表两个 CONCURRENTLY 索引**，每个索引单独一个迁移文件、单条语句；建表迁移不含 `PRIMARY KEY` / `UNIQUE`。

**Testing**: `go test ./internal/content/review-delivery/`、`./internal/handler/`（按 `docs/development/testing-database-suites.md` 配 `LORETIDE_DB_TEST_*`）；`packages/core/*.test.ts`（node 环境）。**无 UI 单测。**

**Constraints**: 不调模型、不起执行器、**不发起任何外发请求**、**不引入调度器**；页面只挂既有组件。

**Scale**: 一个品牌下的审核请求与交付任务是人工产物，数量级是每天个位数到几十条；发布记录同量级。索引按 `workspace_id` 打头即可，不需要分区。

## Constitution Check

| 原则 | 状态 | 说明 |
|---|---|---|
| II. 不写 UI 单测 | 通过 | 判定与状态机进 Go / core node 测试；界面进 `manual-ui-todo.md` |
| III. 模块边界 | **需注意** | `review-delivery` 的依赖表里**没有** `ip-profile` / `topic-planning`；渠道受控集按 Q4=A 在本模块自己定义**四个值**，并用一条用例读 `ip-profile` 的 Go 源文件比对（不 import） |
| V. 无外键、CONCURRENTLY 索引、单语句迁移 | 通过 | R1–R6 逐条 |
| VI. zod + `parseWithFallback` | 通过 | core 侧三个响应各一份 schema 与畸形降级 |
| VIII. 范围纪律 | **需注意** | 本卡只存记录，不聚合（那是 `feedback-learning`）；不碰平台 API |
| IX. 真实执行器保持禁用 | 通过 | 本卡更强：不外发、**不存平台密钥、不提供发布执行接口**（§9.2 原文），三条守卫。另 §8「AI 不能执行人工通过动作」= FR-006a |
| X. 打勾不是验收 | 通过 | 界面项一律记「未执行」，由主控在浏览器验收 |

## Project Structure

```text
server/
├── migrations/
│   ├── N_content_review_request.{up,down}.sql          # 建表，单语句，无 PK/UNIQUE
│   ├── N+1_content_review_request_id_unique_idx.*      # 各自单文件单语句 CONCURRENTLY
│   ├── N+2_content_review_request_workspace_idx.*
│   ├── N+3_content_review_transition.{up,down}.sql
│   ├── N+4_content_review_transition_id_unique_idx.*
│   ├── N+5_content_review_transition_subject_idx.*
│   ├── N+6_content_delivery_task.{up,down}.sql
│   ├── N+7_content_delivery_task_id_unique_idx.*
│   ├── N+8_content_delivery_task_workspace_idx.*
│   ├── N+9_content_publication_record.{up,down}.sql
│   ├── N+10_content_publication_record_id_unique_idx.*
│   └── N+11_content_publication_record_artifact_idx.*
├── internal/content/review-delivery/
│   ├── contract.go          # 三类实体、五个受控集、八键快照、状态机与错误
│   ├── states.go            # 纯函数：合法迁移表、条件必填判定、派生显示（待登记 / 到期待办）
│   ├── states_test.go       # 状态机矩阵与派生显示矩阵（无数据库）
│   ├── store.go             # 栅栏 + 审计 + 写入；读路径按 workspace_id
│   └── store_integration_test.go
├── internal/handler/
│   ├── content_review.go          # 审核请求端点
│   ├── content_delivery.go        # 交付任务与发布记录端点
│   └── *_test.go
├── pkg/db/queries/content_review_delivery.sql          # INSERT / SELECT；发布与迁移无 UPDATE/DELETE
├── cmd/server/router.go            # 挂九条路由 ┐ 同一个 upstream: 提交
└── cmd/migrate/main.go             # 八条索引登记 ┘

packages/core/content/review-delivery/
├── contract.ts       # zod schema 与受控集
├── contract.test.ts  # 畸形降级、未知状态保留（node 环境）
├── states.ts         # 状态机的前端副本 + 与 Go 源文件比对的用例
├── states.test.ts
└── queries.ts        # 读写 hooks

packages/views/content/review-delivery/
└── index.tsx         # 审核与交付页面，只挂既有组件

specs/025-review-delivery-manual/
├── contracts/review-delivery.md
└── manual-ui-todo.md  # 页面 PR 时新建
```

## 两个 PR 的分界

| PR | 内容 | 验收口径 |
|---|---|---|
| **PR 1 存储与接口** | 四张表与十二个迁移、模块、九条端点、路由与索引登记（同一个 `upstream:` 提交）、core 的 schema 与状态机副本 | 状态机矩阵逐条、守卫（不改不删 / 不外发 / 不存密钥 / 不提供发布接口 / 不调度）、第 12 步**两类** id 各一条、删除清单四张表各一条、SC-012「交接 ≠ 发布」三条 |
| **PR 2 页面** | 审核与交付页面、四语言、`manual-ui-todo.md` | 只挂既有组件、无 UI 单测；界面项全部记「未执行」 |

## 风险与对策

| 风险 | 对策 |
|---|---|
| work-editor（#141）尚未合入，`version_id` 的实际形状可能与 024 contract 有出入 | PR 1 的第一步是**读 #141 合入后的实际形状**；本规格只硬要求「版本不可变且有稳定键」 |
| 「一张表记两类主体」（迁移记录）可能被读成偷懒 | 合同里写明理由：两者迁移形状完全相同，分表会让「列出这件事的全部动作」要查两次 |
| 受控集在 Go 与 TS 两侧各写一份会漂移 | 照 `packages/core/content/ip-profile/scope.test.ts`：前端的用例**读 Go 源文件**比对，漂了就红 |
| `scheduled_at` 被后人接上调度器而无人察觉 | SC-010 的检索用例钉住「没有任何代码读它去执行」；§9.1 的「到期待办」是读时比较（FR-009a），不是事件 |
| §9.3 的自动 `held` 触发点分散（换稿 / 换附件 / 换账号 / 改渠道配置）会被实现成四段各自的代码 | 判定收敛成 `states.go` 的**一个纯函数**：比较任务引用的快照与当前交付目标；四个触发点是同一次比较的四个输入 |
| Q5 不发明枚举后，自由文本字段容易退化成「什么都能填也什么都不填」 | 条件必填是唯一的约束面，SC-004 的三条用例逐字段点名；**不补充长度以外的格式校验** |
| `failed` / `removed` 的原因借用了 `receipt_note`（裁决字段清单里没有 `reason` 列） | 已在 spec 的「裁决记录」单列，一处改动即可换成独立列 |
