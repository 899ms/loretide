# Implementation Plan: 手工选题卡与冻结简报（EP-04a）

**Branch**: `claude/spec-022-ep04-topic-brief` | **Date**: 2026-09-15 | **Spec**: [spec.md](./spec.md) | **拆分**: [ep04-breakdown.md](./ep04-breakdown.md)

## Summary

`topic-planning` 模块的第一次落地：两张表（选题卡、简报版本）、两组端点、一页界面，分两个 PR 交付（先存储与接口，再页面）。

三处照抄既有做法，不发明：

1. **版本表** 照 `content_account_revision` 的 append-only 形状，但按当前迁移硬规则把 `revision_id` 唯一性也拆成单独的 CONCURRENTLY 索引——`revision_id` 是被引用的键，`revision` 只给人读。
2. **授权** 照 `workspace-core` 的 `Authorize` + `RefusalStatus` / `RefusalBody` —— 不自己判成员、不自己造 404。
3. **工作区删除** 照四张 `content_` 表 —— 删除清单登记 + 同一 CTE 链里一条 `DELETE`，无外键无级联。

一处是这张卡自己的：**新建 `server/internal/content/topic-planning/` 会让落地模块数由 3 变 4**，该目录从创建那一刻起受接入合同约束。合同不是交付后再补，是这张卡的一部分。

## Technical Context

**Language/Version**: Go 1.26（`server/`）、TypeScript 5 strict（`packages/core`、`packages/views`）

**Storage**: PostgreSQL。**两张新表 + 五个 CONCURRENTLY 索引**，每个索引单独一个迁移文件、单条语句；建表迁移不使用会隐式建索引的 `PRIMARY KEY`。

**Testing**: `go test ./internal/content/topic-planning/` 与 `./internal/handler/`（`DATABASE_URL` 实跑）、`packages/core/*.test.ts`（node）。**无 UI 单测。**

**Constraints**: 不调模型；不读文件；不写快照；页面只挂既有组件。

## Constitution Check

| 原则 | 判定 | 依据 |
|---|---|---|
| I. `CLAUDE.md` 权威 | 通过 | 无新规则 |
| II. 不写 UI 单测 | 通过 | 逻辑进 core node 测试与 Go 测试；界面进手动清单 |
| III. 模块边界 | **需注意** | 新模块目录；依赖只用 `workspace-core` 与 `diagnostics`，不用 `knowledge-base`。`check:content-boundaries` 必须仍退出 0 |
| IV. 服务端/客户端状态分离 | 通过 | 服务端状态走 TanStack Query；无新 Zustand store |
| V. 无外键、无级联 | **需注意** | 两张新表 + 索引，是这张卡最容易违规的地方。见下 |
| VI. 响应解析不强转 | **需注意** | 新端点 → 新 zod schema + 畸形响应测试 |
| VII. UI 复用 Multica | **需注意** | 页面 PR 只挂既有组件 |
| VIII. 范围是所领的任务 | 通过 | 快照、候选生成、必用/排除三项各自划给 b/c/d |
| IX. 真实执行器保持禁用 | 通过 | 不调模型、不触发执行 |
| X. 打勾不等于验收 | **需注意** | FR-018：「既有运行继续引用旧版」本卡未验证，不得因结构正确就记通过 |

### 原则 V 的三条硬要求，逐条落到文件

- **无外键、无 `REFERENCES`、无级联**：选题卡与简报版本之间、与工作区之间的关系全部在应用代码里解，删除在删除事务里显式做。
- **每个索引 `CREATE INDEX CONCURRENTLY`，单独一个迁移文件、单条语句**：PostgreSQL 拒绝在事务或多语句串里建并发索引。`content_account_revision` 的唯一索引（480）就是因此单独成文件的。
- **建表迁移里不建索引**：同上。建表一个文件，每个索引各一个文件。

## Project Structure

```text
server/
├── migrations/
│   ├── 4xx_content_topic_card.up.sql            # 建表，单语句
│   ├── 4xx_content_topic_card_id_unique_idx.up.sql
│   ├── 4xx_content_topic_card_workspace_idx.up.sql
│   ├── 4xx_content_brief_revision.up.sql
│   ├── 4xx_content_brief_revision_id_unique_idx.up.sql
│   ├── 4xx_content_brief_revision_unique_idx.up.sql   # UNIQUE (topic_card_id, revision)
│   └── 4xx_content_brief_revision_workspace_idx.up.sql
│       （各配 .down.sql；编号在实施时按当时的最大值顺延）
├── internal/content/topic-planning/              # 新模块目录
│   ├── contract.go        # TopicCard / BriefRevision 形状与动作枚举
│   ├── store.go           # 读写，审计在同一事务内
│   └── *_test.go
├── internal/handler/
│   ├── content_topic.go   # 端点，授权经 workspace-core
│   └── content_topic_test.go
├── cmd/server/router.go   # 挂载新端点 ← 见下「一处必须做的事」
├── pkg/db/queries/workspace_delete.sql           # 两条 DELETE
└── internal/handler/workspace_delete_manifest_test.go  # 两行登记

packages/core/content/topic-planning/
├── contract.ts + contract.test.ts   # zod schema 与畸形响应测试
└── queries.ts

packages/views/content/topic-planning/            # 第二个 PR
```

**Structure Decision**: 服务端逻辑放模块目录、HTTP 放 `internal/handler`，与 `ip-profile` 的现状一致（`content_diagnostics.go` 在 handler、领域在 `content/diagnostics`）。

### 一处必须做的事：把端点挂上路由

`specs/017` 的实施里发现过：#71 与 #73 交付了八个账号端点，**两次都没挂进 `server/cmd/server/router.go`**，在运行中的服务里全都不可达，而处理器测试直接调函数、挂没挂都一样绿。

**本卡的第一个 PR MUST 把新端点挂进 `router.go` 并有一条穿过真实路由的用例**，不能重演同一件事。

## 两个 PR 的分界

| PR | 内容 | 验收 |
|---|---|---|
| **PR 1 存储与接口** | 迁移、模块目录、端点、路由挂载、删除清单、core 的 schema 与解析 | Go 测试、`check:*` 三项、越权 404、第 12 步用例 |
| **PR 2 页面** | `packages/views/content/topic-planning/`，只挂既有组件 | 手动清单；无 UI 单测 |

SOP 阶段规则要求先出存储与接口 PR 再出页面 PR，本卡照此拆。

## Complexity Tracking

| Violation | Why Needed | Simpler Alternative Rejected Because |
|---|---|---|
| 新建一个 content 模块目录（落地模块 3 → 4） | `docs/12` §2 把「选题、理由、创作简报」判给 `topic-planning`；写在别处就是跨模块写表 | 复用上游 `issue`：见 spec Q1 的对照表——要改 §2、要走第 13 步、且会继承看板位置、依赖、标签、`agent_task_queue` 认领与删除栅栏 |
| 两张表而不是一张 | 简报版本必须 append-only，选题卡必须可改状态。混在一张表里意味着改状态就改了历史 | 用一张表加 `is_current` 标记：状态更新会 UPDATE 到历史行上，§5.3 的「旧版本保持可读」立刻不成立 |
