# Implementation Plan: 开始界面与输入快照（EP-04b）

**Branch**: `claude/spec-023-ep04b-start-snapshot` | **Date**: 2026-09-20 | **Spec**: [spec.md](./spec.md) | **拆分**: [../022-ep04-topic-brief/ep04-breakdown.md](../022-ep04-topic-brief/ep04-breakdown.md)

> **三条 clarify 已裁决（主控 2026-09-20）：Q1=B、Q2=A、Q3=A，附带一问接受。** 本计划已按裁决改写。裁决改变了什么，见 [spec.md → 裁决记录](./spec.md#裁决记录主控-2026-09-20)。

## Summary

EP-04b 不新建模块，也不新建实体：它把**已经分散在四处的配置**，在「开始」那一刻收成一份**不可变的输入快照**。

四处分别是：账号表达配置（021，含就绪判定）、简报版本（022）、账号资料范围偏好（LT-014）、品牌预检开关（LT-015）。本卡**一处都不重新定义**，只负责读、校验、固定。

三处照抄既有做法，不发明：

1. **新表，照 append-only 版本表的既有形状** —— `content_start_snapshot`，`snapshot_id` 是被引用的稳定键、唯一性由单独的 CONCURRENTLY 索引保证（不是 `PRIMARY KEY`，R5），建表与每个索引各一个单语句迁移，照 `content_topic_card`（483/488）与 `content_brief_revision`（485/486/487/489）的做法。**与拆分卡的「无新表」一句冲突**——裁决理由是给简报加列需要在那张表上开一条受限 `UPDATE`，而 022 的 A6 守卫说的是「只插不改不删」，那条守卫不容破例。
2. **授权** —— 照 `workspace-core` 的 `Authorize` + `RefusalStatus` / `RefusalBody`，不自己判成员、不自己造 404。
3. **就绪判定** —— 直接用 `ipprofile.ProfileReadiness` 与 core 的 `profileReadiness`，两侧已由 parity 矩阵钉住；本卡**不复制第三份**。

一处是这张卡自己的、也是最容易做错的：**「开始」这个词在本仓库已经被占用**。EP-04a 的 `action=start` 是「接受选题并冻结首版简报」。EP-04b 的「开始」是「用某一版简报开一次工」。两者必须在 API 上分得开，否则事后没人说得清一次 `start` 到底做了哪件事。

## Technical Context

**Language/Version**: Go 1.26（`server/`）、TypeScript 5 strict（`packages/core`、`packages/views`）

**Storage**: PostgreSQL。**一张新表 + 三个 CONCURRENTLY 索引**，四个迁移文件，每个单条语句；建表迁移不含 `PRIMARY KEY` / `UNIQUE`（R5）。

**Testing**: `go test ./internal/content/topic-planning/`、`./internal/content/ip-profile/`、`./internal/handler/`（按 `docs/development/testing-database-suites.md` 配 `LORETIDE_DB_TEST_*`）；`packages/core/*.test.ts`（node 环境）。**无 UI 单测。**

**Constraints**: 不调模型、不起执行器（宪法 IX）；不读本地文件；页面只挂既有组件。

## Constitution Check

| 原则 | 判定 | 依据 |
|---|---|---|
| I. `CLAUDE.md` 权威 | 通过 | 无新规则 |
| II. 不写 UI 单测 | 通过 | 判定与装配进 Go / core node 测试；界面进 `manual-ui-todo.md` |
| III. 模块边界 | **需注意** | `topic-planning` 要新增对 `ip-profile` 的依赖。`scripts/content-boundaries.json` 里该依赖**已经声明**（`topic-planning` → `workspace-core` / `ip-profile` / `knowledge-base` / `diagnostics`），所以是启用既有声明，不是加新方向。`check:content-boundaries` 必须仍退出 0 |
| IV. 服务端/客户端状态分离 | 通过 | 快照与就绪判定都是服务端状态，走 TanStack Query；本次开始的草稿（选了哪一版、哪个范围）是客户端状态 |
| V. 无外键、无级联 | **需注意** | 一张新表 + 三个索引，是这张卡最容易违规的地方。R1/R2/R3/R4/R5/R6 六条全部生效；见下 |
| VI. 响应解析不强转 | **需注意** | 新端点 → 新 zod schema + 畸形响应用例 |
| VII. UI 复用 Multica | **需注意** | 页面 PR 只挂既有组件，不新增控件、不改样式 |
| VIII. 范围是所领的任务 | **需注意** | 快照里七个无来源字段**只留位、不接入**；必用/排除是 EP-04d，预检触发是 EP-06 |
| IX. 真实执行器保持禁用 | 通过 | `executor` / `executor_version` 记的是「禁用」本身 |
| X. 打勾不等于验收 | **需注意** | 「已开始的运行继续看到旧配置」本卡**只有结构保证**；没有运行实体读它（`agent-workflow` 未落地），不得因快照正确就记这条为验收通过 |

### 原则 V 落到文件（六条规则逐条）

| 规则 | 要求 | 落到 |
|---|---|---|
| **R1/R2** | 无 `REFERENCES` / `FOREIGN KEY`、无 `CASCADE` | 快照与简报版本、卡、账号、工作区的关系全部在应用代码里解；删除在删除事务里显式做 |
| **R3** | 每个索引 `CREATE INDEX CONCURRENTLY` | 491 / 492 / 493 |
| **R4** | 建并发索引的文件**只能有这一条语句** | 491 / 492 / 493 各自成文件 |
| **R5** | 建表迁移**不得**有 `PRIMARY KEY` 或 `UNIQUE`（会隐式建非并发索引） | 490 只有列；`snapshot_id` 的唯一性走 491 |
| **R6**（#122） | 每个**建索引的** up 迁移必须登记进 `cmd/migrate` 的 `concurrentIndexCleanups`，索引名逐字相同 | 491 / 492 / 493 三条登记；**490 不登记**（登记一个不存在的索引同样是静默 no-op，R6 的反向断言会红） |
| **删除清单** | 新表进 `workspace_delete_manifest_test.go`，标 `workspaceDelete` | 删除事务的同一 CTE 链里加一条按 `workspace_id` 的 `DELETE` |

**不可变靠「没有写它的第二条路径」**：查询文件里这张表不出现 `UPDATE`，不出现（删除链之外的）`DELETE`；守卫用例照 022 的 A6 形状扫查询文件。

## Project Structure

```text
server/
  migrations/
    490_content_start_snapshot.{up,down}.sql             # 建表，无 PK/UNIQUE（R5）
    491_content_start_snapshot_id_unique_idx.{up,down}.sql
    492_content_start_snapshot_workspace_idx.{up,down}.sql
    493_content_start_snapshot_card_idx.{up,down}.sql
  pkg/db/queries/
    content_start_snapshot.sql                           # 只有 INSERT 与 SELECT
  internal/content/topic-planning/
    snapshot.go        # 快照装配：纯函数，输入是四处配置，输出是 Snapshot
    snapshot_test.go   # 装配矩阵 + 七个无来源字段恒空的负例
    contract.go        # StartRequest / StartSnapshot
    store.go           # 写路径：栅栏 + 只插；读路径：按版本列出、按 id 取单份
  internal/handler/
    content_topic_start.go       # POST /api/content-topics/{id}/start
    content_topic_start_test.go
  cmd/server/router.go           # 挂三条路由 ┐ 同一个 upstream: 提交
  cmd/migrate/main.go            # 三条索引登记 ┘

packages/core/content/topic-planning/
  snapshot.ts        # 快照的 zod schema 与只读投影
  snapshot.test.ts   # 解析、畸形降级（node 环境）
  queries.ts         # useStartReadiness / useStartRun

packages/views/content/topic-planning/
  start.tsx          # 开始界面，只挂既有组件

specs/023-ep04b-start-snapshot/
  contracts/start-snapshot.md
  manual-ui-todo.md  # 页面 PR 时新建
```

### 命名：把两个「start」分开

| 谁 | 动作 | 含义 |
|---|---|---|
| EP-04a（既有） | `POST /api/content-topics/{id}/actions` body `{action:"start"}` | 接受选题，冻结首版简报，卡状态 → `started` |
| EP-04b（本卡） | `POST /api/content-topics/{id}/briefs/{revisionId}/start` | 用这一版简报开一次工，固定输入快照 |

第二条**挂在简报版本下面**，不是挂在卡下面：本卡固定的是「这一版简报的这一次开始」，URL 说出这件事。

读回是**两条新端点**（Q1=B 下快照是独立实体，不再搭既有的简报读取端点）：

- `GET /api/content-topics/{id}/briefs/{revisionId}/snapshots` —— 该版简报的全部快照；
- `GET /api/content-topics/{id}/snapshots/{snapshotId}` —— 单份快照。

就绪判定读 `GET /api/content-accounts/{id}/profile` 的 `readiness`（**既有端点，不改**）。

> **三条带路径参数的新端点**都要按工作流第 12 步办：至少一条穿过真实中间件、且 `{id}` / `{revisionId}` / `{snapshotId}` 的值 ≠ 上下文值的用例。`{snapshotId}` 尤其要有——它是新的一类 id，把别的工作区的 `snapshot_id` 填进来必须拿到与「不存在」同形的拒绝。

### 一处必须做的事：把端点挂上路由

**本卡要改两个上游文件**，放进**同一个** `upstream:` 提交，PR 正文单列「上游改动」一节（工作流第 13 步）：

1. `server/cmd/server/router.go` —— 三条路由。#71/#73 的教训是「handler 写完了没挂路由，什么都到不了它」。
2. `server/cmd/migrate/main.go` —— 491/492/493 三条索引登记。#122 刚刚证明过：不登记的并发索引，被中断的建索引会在重试时被记成成功，索引永久不可用且运行时没有任何提示。**R6 会让它在 CI 上变红**，所以这不是可选项。

**路由的静态段与参数段**：`briefs/{revisionId}/start` 里 `start` 是末段，不会被读成 id；`GET /briefs/{revisionId}/snapshots` 同理。`GET /snapshots/{snapshotId}` 是卡下面的新分支，与 `briefs` 平级，不冲突。

## 两个 PR 的分界

| PR | 内容 | 验收 |
|---|---|---|
| **PR 1 存储与接口** | 四个迁移、sqlc、`snapshot.go` 装配、`contract.go`、store 写读路径与栅栏、三个 handler、路由与索引登记（同一个 `upstream:` 提交）、删除清单、Go 用例 | 快照能写能读、只插不改、一版简报可多次开始、非法依赖拒绝与不存在同形、九个字段恒空、R5/R6 与删除清单全绿、第 12 步用例 |
| **PR 2 页面** | `packages/core` schema 与 hooks、`packages/views` 开始界面、四语言、`manual-ui-todo.md` | 缺项逐项列出、「开始」按钮态、项目可选、「暂不可用」半边有原因；**无 UI 单测** |

## Complexity Tracking

| 事项 | 为什么不是更简单的做法 |
|---|---|
| 快照装配写成独立的纯函数而不是塞进 handler | 十六个字段的取值规则是本卡的全部产出。塞进 handler 就只能靠一条带数据库的端点用例去覆盖十六个字段的组合，那是本仓库反复纠正过的「把规则藏进 JSX/handler」的同一种错误 |
| 就绪判定在服务端复核一次 | 前端已经有 `profileReadiness`，但前端判定不是授权。这条复核是 FR-004，不是冗余 |
| 七个无来源字段留位而不省略 | 拆分卡明写「不要再造第二套」。省略会让 EP-04d/W-03 接入时改字段名，届时所有已存快照都要迁移 |
| 端点挂在 `briefs/{revisionId}` 下而不是卡下 | 见上表：URL 要说出「固定的是哪一版的哪一次」 |
| 三个索引而不是一个 | 491 是 `snapshot_id` 的唯一性（R5 不让它当 PK）；492 服务工作区删除与「这一版简报开始过几次」；493 服务详情页那个区块（T026 的入口）的「这张卡开始过几次」。492 的前导列是 `workspace_id`，删除走得到它，所以不需要第四个 |
| 快照没有 `revision` 计数器 | 简报需要它是因为 §5.3 要给人看「第几版」。快照没有这个需求，加一个只会让人以为它是跨表键 |

## 本卡验不了的一件事（写在这里，不留到实施时才发现）

**「已开始的运行继续看到它开始时的配置」——本卡只有结构保证。**

今天没有内容生产运行实体（`agent-workflow` 未落地），所以能验的是「快照写下后不可变」，不是「运行确实读到了它」。与 EP-04a 对 §5.3 的处理口径一致。**实施时不得把 SC-002 当成这条的验收。**
