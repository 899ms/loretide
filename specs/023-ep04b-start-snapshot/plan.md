# Implementation Plan: 开始界面与输入快照（EP-04b）

**Branch**: `claude/spec-023-ep04b-start-snapshot` | **Date**: 2026-09-20 | **Spec**: [spec.md](./spec.md) | **拆分**: [../022-ep04-topic-brief/ep04-breakdown.md](../022-ep04-topic-brief/ep04-breakdown.md)

> **本计划按三条 clarify 的推荐值起草（Q1=A、Q2=A、Q3=A）。** 主任务裁决与推荐值不同时，受影响的段落在下文各自标注了「若选 B/C 则……」，改动范围有界。

## Summary

EP-04b 不新建模块，也不新建实体：它把**已经分散在四处的配置**，在「开始」那一刻收成一份**不可变的输入快照**。

四处分别是：账号表达配置（021，含就绪判定）、简报版本（022）、账号资料范围偏好（LT-014）、品牌预检开关（LT-015）。本卡**一处都不重新定义**，只负责读、校验、固定。

三处照抄既有做法，不发明：

1. **加列而不是加表** —— 照迁移 482 给 `content_account_revision` 加 `profile jsonb` 的做法，给 `content_brief_revision` 加 `snapshot jsonb`。拆分卡写的「无新表」由此成立，而 append-only 的简报版本表天然给了快照不可变性，不需要第二套保证。
2. **授权** —— 照 `workspace-core` 的 `Authorize` + `RefusalStatus` / `RefusalBody`，不自己判成员、不自己造 404。
3. **就绪判定** —— 直接用 `ipprofile.ProfileReadiness` 与 core 的 `profileReadiness`，两侧已由 parity 矩阵钉住；本卡**不复制第三份**。

一处是这张卡自己的、也是最容易做错的：**「开始」这个词在本仓库已经被占用**。EP-04a 的 `action=start` 是「接受选题并冻结首版简报」。EP-04b 的「开始」是「用某一版简报开一次工」。两者必须在 API 上分得开，否则事后没人说得清一次 `start` 到底做了哪件事。

## Technical Context

**Language/Version**: Go 1.26（`server/`）、TypeScript 5 strict（`packages/core`、`packages/views`）

**Storage**: PostgreSQL。**无新表**；一列 `snapshot jsonb NOT NULL DEFAULT '{}'::jsonb` 加在 `content_brief_revision` 上，**单条语句、无索引**（jsonb 上没有要走索引的查询，和 482 同理）。

**Testing**: `go test ./internal/content/topic-planning/`、`./internal/content/ip-profile/`、`./internal/handler/`（按 `docs/development/testing-database-suites.md` 配 `LORETIDE_DB_TEST_*`）；`packages/core/*.test.ts`（node 环境）。**无 UI 单测。**

**Constraints**: 不调模型、不起执行器（宪法 IX）；不读本地文件；页面只挂既有组件。

## Constitution Check

| 原则 | 判定 | 依据 |
|---|---|---|
| I. `CLAUDE.md` 权威 | 通过 | 无新规则 |
| II. 不写 UI 单测 | 通过 | 判定与装配进 Go / core node 测试；界面进 `manual-ui-todo.md` |
| III. 模块边界 | **需注意** | `topic-planning` 要新增对 `ip-profile` 的依赖。`scripts/content-boundaries.json` 里该依赖**已经声明**（`topic-planning` → `workspace-core` / `ip-profile` / `knowledge-base` / `diagnostics`），所以是启用既有声明，不是加新方向。`check:content-boundaries` 必须仍退出 0 |
| IV. 服务端/客户端状态分离 | 通过 | 快照与就绪判定都是服务端状态，走 TanStack Query；本次开始的草稿（选了哪一版、哪个范围）是客户端状态 |
| V. 无外键、无级联 | **需注意** | 一次加列迁移。无外键、无索引；见下 |
| VI. 响应解析不强转 | **需注意** | 新端点 → 新 zod schema + 畸形响应用例 |
| VII. UI 复用 Multica | **需注意** | 页面 PR 只挂既有组件，不新增控件、不改样式 |
| VIII. 范围是所领的任务 | **需注意** | 快照里七个无来源字段**只留位、不接入**；必用/排除是 EP-04d，预检触发是 EP-06 |
| IX. 真实执行器保持禁用 | 通过 | `executor` / `executor_version` 记的是「禁用」本身 |
| X. 打勾不等于验收 | **需注意** | 「已开始的运行继续看到旧配置」本卡**只有结构保证**；没有运行实体读它（`agent-workflow` 未落地），不得因快照正确就记这条为验收通过 |

### 原则 V 落到文件

- **加列迁移一条语句、无外键、无 `REFERENCES`、无级联**；
- **不加索引**：jsonb 列上没有查询要走索引。因此本卡**不触发** `concurrentIndexCleanups` 的 R6 —— 但 R6 的存在本身要求「**如果**后来加了索引就必须登记」，所以 tasks 里保留一条核对项；
- **不新建表**，因此工作区删除清单**无需改动**；仍要跑 `TestWorkspaceDeletionManifestCoversPublicSchema` 证明它不需要改。

> **若 Q1 选 B（新表）**：以上三条全部反转 —— 新表 + 至少两个 CONCURRENTLY 索引（各自单文件单语句）+ 登记 `concurrentIndexCleanups` + 登记删除清单并有用例 + 自证不可变（无 UPDATE/DELETE 路径 + 守卫用例）。工作量约翻倍，仍是一个 PR 对。

## Project Structure

```text
server/
  migrations/
    49N_content_brief_revision_snapshot.{up,down}.sql   # 加列，单语句，无索引
  pkg/db/queries/
    content_brief_revision.sql                          # insert/select 带上 snapshot
  internal/content/topic-planning/
    snapshot.go        # 快照装配：纯函数，输入是四处配置，输出是 Snapshot
    snapshot_test.go   # 装配矩阵 + 七个无来源字段恒空的负例
    contract.go        # StartRequest / StartResult
    store.go           # 写路径：栅栏 + 只插不改
  internal/handler/
    content_topic_start.go       # POST /api/content-topics/{id}/start
    content_topic_start_test.go
  cmd/server/router.go           # 挂两条路由（upstream: 独立提交）

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

第二条**挂在简报版本下面**，不是挂在卡下面：本卡固定的是「这一版简报的这一次开始」，URL 说出这件事，也天然表达了 Q1=A 的一对一基数。

读回快照：`GET /api/content-topics/{id}/briefs/{revisionId}`（**既有端点**，响应里多一个 `snapshot` 字段）。就绪判定读 `GET /api/content-accounts/{id}/profile` 的 `readiness`（**既有端点，不改**）。

> **两条带路径参数的新端点**（`{id}` 与 `{revisionId}`）各需要一条穿过真实中间件、参数值 ≠ 上下文值的用例（工作流第 12 步）。

### 一处必须做的事：把端点挂上路由

`server/cmd/server/router.go` 是上游文件。#71/#73 的教训是「handler 写完了没挂路由，什么都到不了它」。本卡在 `/api/content-topics/{id}` 既有分组里加一条 `r.Post("/briefs/{revisionId}/start", ...)`，**独立提交、以 `upstream:` 开头、PR 正文单列「上游改动」一节**（工作流第 13 步）。

**静态段在参数段之前**：`briefs/{revisionId}/start` 里 `start` 是末段，不会被读成 id；但要确认它不与既有的 `GET /briefs/{revisionId}` 冲突（方法不同，chi 可区分）。

## 两个 PR 的分界

| PR | 内容 | 验收 |
|---|---|---|
| **PR 1 存储与接口** | 迁移、sqlc、`snapshot.go` 装配、`contract.go`、store 写路径与栅栏、handler、路由（`upstream:` 独立提交）、Go 用例 | 快照能写能读、不可变、非法依赖拒绝与不存在同形、七个字段恒空、第 12 步用例 |
| **PR 2 页面** | `packages/core` schema 与 hooks、`packages/views` 开始界面、四语言、`manual-ui-todo.md` | 缺项逐项列出、「开始」按钮态、项目可选、「暂不可用」半边有原因；**无 UI 单测** |

## Complexity Tracking

| 事项 | 为什么不是更简单的做法 |
|---|---|
| 快照装配写成独立的纯函数而不是塞进 handler | 十六个字段的取值规则是本卡的全部产出。塞进 handler 就只能靠一条带数据库的端点用例去覆盖十六个字段的组合，那是本仓库反复纠正过的「把规则藏进 JSX/handler」的同一种错误 |
| 就绪判定在服务端复核一次 | 前端已经有 `profileReadiness`，但前端判定不是授权。这条复核是 FR-004，不是冗余 |
| 七个无来源字段留位而不省略 | 拆分卡明写「不要再造第二套」。省略会让 EP-04d/W-03 接入时改字段名，届时所有已存快照都要迁移 |
| 端点挂在 `briefs/{revisionId}` 下而不是卡下 | 见上表：URL 要说出「固定的是哪一版的哪一次」 |

## 本卡验不了的一件事（写在这里，不留到实施时才发现）

**「已开始的运行继续看到它开始时的配置」——本卡只有结构保证。**

今天没有内容生产运行实体（`agent-workflow` 未落地），所以能验的是「快照写下后不可变」，不是「运行确实读到了它」。与 EP-04a 对 §5.3 的处理口径一致。**实施时不得把 SC-002 当成这条的验收。**
