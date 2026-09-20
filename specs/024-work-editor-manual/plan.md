# Implementation Plan: 人工写作的作品容器、文档与版本（024）

**Branch**: `claude/spec-024-work-editor-manual` | **Date**: 2026-09-20 | **Spec**: [spec.md](./spec.md) | **Issue**: #137

> **本计划按三条 clarify 的推荐值起草（Q1=A、Q2=A、Q3=A + 暂定状态集）。** 裁决与推荐值不同时，受影响的段落在下文各自标注了「若选 B/C 则……」，改动范围有界。**Q3 若拿到 SOP 原文且状态集合不同，改的是一次 `CHECK` 迁移与一个 Go 枚举，不影响表结构。**

## Summary

`work-editor` 模块的第一次落地：**三张表**（作品、文档、版本）、三组端点、一页界面，分两个 PR 交付。

四处照抄既有做法，不发明：

1. **append-only 版本表** 照 `content_account_revision`（479）与 `content_brief_revision`（485）的形状——但**不照抄 479 的 `PRIMARY KEY`**：R5 从 483 起生效，唯一性必须走单独的 CONCURRENTLY 索引（488/489/491 都是这么做的）。
2. **可变行 + 不可变历史** 照 023 的分法。023 的裁决理由是「给 append-only 表开一条受限 UPDATE 会破掉 022 的 A6 守卫」；这里同一个取舍，同一个答案：编辑副本放**本来就可变**的文档行上，版本单独一张只插的表。
3. **受控集** 照 `content_account.platform`（477）与 `content_topic_card.status`（483）：**Go 枚举为准产出 400 诊断错误对象，库加 `CHECK` 兜底**。
4. **授权** 照 `workspace-core` 的 `Authorize` + `RefusalStatus` / `RefusalBody`——不自己判成员、不自己造 404。

两处是这张卡自己的：

- **新建 `server/internal/content/work-editor/` 会让落地模块数由 4 变 5**，该目录从创建那一刻起受接入合同约束。合同不是交付后再补，是这张卡的一部分。
- **版本号的并发**。022 的 `AppendBrief` 已经踩过一次（Issue #109）：先锁父行、再在**第二条语句**里数号，因为 READ COMMITTED 下等锁不会刷新语句快照，在锁定语句里数号会数到赢家提交之前的状态。本卡的版本号是同一个形状，**照抄那个两步写法**，不要重新发明。

## Technical Context

**Language/Version**: Go 1.26（`server/`）、TypeScript 5 strict（`packages/core`、`packages/views`）

**Storage**: PostgreSQL。**三张新表 + 五个 CONCURRENTLY 索引**，八个迁移文件（494–501），每个单条语句；建表迁移不含 `PRIMARY KEY` / `UNIQUE`。

**Testing**: `go test ./internal/content/work-editor/`（自带 schema 的集成夹具，照 `topic-planning` 的 `newTopicFixture`）、`./internal/handler/` 与 `./cmd/server/`（按 `docs/development/testing-database-suites.md` 配 `LORETIDE_DB_TEST_*`）、`packages/core/*.test.ts`（node）。**无 UI 单测。**

**Constraints**: 不调模型、不起执行器；页面只挂既有组件。

## Constitution Check

| 原则 | 判定 | 依据 |
|---|---|---|
| I. `CLAUDE.md` 权威 | 通过 | 无新规则 |
| II. 不写 UI 单测 | 通过 | 逻辑进 Go 与 core node 测试；界面进 `manual-ui-todo.md` |
| III. 模块边界 | **需注意** | 新模块目录，落地数 4→5。依赖只用 `workspace-core` 与 `diagnostics`（Q2=A 不动登记表）。`check:content-boundaries` 与 `check:diagnostics-contract` 都必须仍退出 0 |
| IV. 服务端/客户端状态分离 | **需注意** | 自动保存是服务端写入，走 mutation；**编辑中的文本是客户端状态**，不进 Query 缓存 |
| V. 无外键、无级联 | **需注意** | 三张新表 + 五个索引，是这张卡最容易违规的地方。R1–R6 全生效；见下 |
| VI. 响应解析不强转 | **需注意** | 新端点 → 新 zod schema + 畸形响应用例 |
| VII. UI 复用 Multica | **需注意** | 页面 PR 只挂既有组件 |
| VIII. 范围是所领的任务 | **需注意** | AI 三个入口只留位；审核/交接/发布只留状态值不落地流程；附件清单完全不碰 |
| IX. 真实执行器保持禁用 | 通过 | 不调模型；`generated` 来源有负例钉住「产生不出来」 |
| X. 打勾不等于验收 | **需注意** | 「已审核/已交接/已发布版本永不删除」本卡**只有结构保证**（没有删除路径）；不得因为没有删除端点就把这条记成验收通过 |

### 原则 V 落到文件（六条规则逐条）

| 规则 | 要求 | 落到 |
|---|---|---|
| R1/R2 | 无 `REFERENCES` / `FOREIGN KEY`、无 `CASCADE` | 作品↔卡、作品↔快照、文档↔作品、版本↔文档的关系全部在应用代码里解；删除在删除事务里显式做 |
| R3 | 每个索引 `CREATE INDEX CONCURRENTLY` | 495 / 497 / 498 / 500 / 501 |
| R4 | 建并发索引的文件只能有这一条语句 | 五个索引各自成文件 |
| **R5** | 建表迁移**不得**有 `PRIMARY KEY` / `UNIQUE` | 494 / 496 / 499 只有列与 `CHECK`；三个 id 的唯一性走 495 / 497 / 500 |
| **R6** | 建索引的 up 迁移逐条登记，索引名逐字相同 | 五条登记；**494 / 496 / 499 不登记**（登记一个不存在的索引同样是静默 no-op，R6 的反向断言会红） |
| 删除清单 | 三张表各进 `workspace_delete_manifest_test.go` | 删除链同一 CTE 里三条按 `workspace_id` 的 `DELETE`，并有断言删除后行数为 0 的用例 |

**`CHECK` 不受 R1/R2 约束**（它不是外键），是受控集的兜底，与 477/483 同一做法。

**不可变靠「没有写它的第二条路径」**：查询里版本表不出现 `UPDATE`，不出现（删除链之外的）`DELETE`；守卫用例照 022 的 `TestBriefStoreHasNoUpdateOrDeletePath` 与 023 的同名用例。

> **若 Q1 选 B**（编辑副本单独一张表）：四张表、七个索引、十个迁移、删除链四条、删除清单四项。表结构之外的东西都不变。

## Project Structure

```text
server/
  migrations/
    494_content_work.{up,down}.sql                     # 建表，无 PK/UNIQUE（R5）
    495_content_work_id_unique_idx.{up,down}.sql
    496_content_artifact.{up,down}.sql                 # 含 draft 可变列（Q1=A）
    497_content_artifact_id_unique_idx.{up,down}.sql
    498_content_artifact_work_idx.{up,down}.sql
    499_content_artifact_version.{up,down}.sql
    500_content_artifact_version_id_unique_idx.{up,down}.sql
    501_content_artifact_version_unique_idx.{up,down}.sql   # (artifact_id, revision)
  pkg/db/queries/
    workspace_delete.sql                               # 三条 DELETE
  internal/content/work-editor/
    contract.go            # Work / Artifact / ArtifactVersion / 受控集 / 错误
    contract_test.go       # 受控集与校验矩阵（无库）
    store.go              # 写路径：栅栏 + 版本号两步写法；读路径全部按 workspace_id
    store_test.go
    store_integration_test.go   # 自带 schema 的夹具，照 newTopicFixture
  internal/handler/
    content_work.go        # 作品与文档
    content_work_version.go# 版本：存一版 / 列出 / 取单条 / 恢复
    *_test.go
  cmd/server/router.go     # 挂路由 ┐ 同一个 upstream: 提交
  cmd/migrate/main.go      # 五条索引登记 ┘

packages/core/content/work-editor/
  contract.ts / contract.test.ts    # zod schema + 畸形响应降级
  queries.ts                        # hooks

packages/views/content/work-editor/  # 页面 PR
specs/024-work-editor-manual/
  contracts/work-and-versions.md
  manual-ui-todo.md                  # 页面 PR 时新建
```

### 端点

| 方法 | 路径 | 作用 |
|---|---|---|
| `GET` / `POST` | `/api/content-works` | 列出（按卡筛选）/ 新建作品 |
| `GET` / `PATCH` | `/api/content-works/{id}` | 读 / 改标题与状态 |
| `GET` / `POST` | `/api/content-works/{id}/artifacts` | 列出 / 新建文档 |
| `PATCH` | `/api/content-works/{id}/artifacts/{artifactId}` | 改标题、序号、**编辑副本**（自动保存走它） |
| `GET` / `POST` | `/api/content-works/{id}/artifacts/{artifactId}/versions` | 版本历史 / **存为一版** |
| `GET` | `/api/content-works/{id}/artifacts/{artifactId}/versions/{versionId}` | 读单条版本 |
| `POST` | `/api/content-works/{id}/artifacts/{artifactId}/versions/{versionId}/restore` | 恢复 → **产生新版本** |
| `POST` | `/api/content-works/{id}/artifacts/{artifactId}/versions/{versionId}/adopt` | 采用为基线 → **产生新版本**（Q3=A） |

**四个路径参数**（`{id}` / `{artifactId}` / `{versionId}` 与上下文工作区），按工作流第 12 步：至少一条穿过真实中间件、三个值互不相等且都 ≠ 工作区 id 的用例；**`{versionId}` 单独一条**——把别的工作区的 `version_id` 填进来必须拿到与「不存在」同形的拒绝。

**没有 `DELETE`，没有版本的 `PATCH`。** 这不是遗漏，是 FR-008。

### 一处必须做的事：把端点挂上路由

`server/cmd/server/router.go` 是上游文件。#71/#73 的教训是「handler 写完了没挂路由，什么都到不了它」。同一个 `upstream:` 提交里还有 `server/cmd/migrate/main.go` 的**五条索引登记**——#122 刚证明过，不登记的并发索引，被中断的建索引会在重试时被记成成功，索引永久不可用且运行时没有任何提示，而 **R6 会让它在 CI 上变红**。PR 正文单列「上游改动」一节（工作流第 13 步）。

## 两个 PR 的分界

| PR | 内容 | 验收 |
|---|---|---|
| **PR 1 存储与接口** | 八个迁移、删除链与清单、`contract.go` / `store.go`、八个 handler、路由与索引登记（同一 `upstream:` 提交）、core 契约与 hooks、Go 用例 | 建作品/文档/存版本能跑通；自动保存不产生版本；恢复产生新版本且旧版逐字节不变；版本只插不改不删（守卫）；并发不撞号；越权与不存在同形；`generated` 恒为 0；R1–R6 与删除清单全绿；第 12 步用例 |
| **PR 2 页面** | 编辑器页、版本历史侧栏、三个禁用的 AI 入口、四语言、`manual-ui-todo.md` | 自动保存可见；「有未存为版本的改动」能说出来；历史可读、可恢复；AI 入口禁用且有原因；**无 UI 单测** |

**版本差异（diff）渲染在 PR 2**，且只用既有组件；本卡不引入 diff 库。若没有合适的既有组件，就**并排显示两版全文**而不是新造一个差异控件——这条写在这里，免得页面 PR 临时决定引入依赖。

## Complexity Tracking

| 事项 | 为什么不是更简单的做法 |
|---|---|
| 三张表而不是两张 | 作品与文档合一会让「同一次产出的多个渠道稿」无处安放（Issue 明写文档有多篇）。文档与版本合一会让可变的编辑副本与不可变的历史挤在一张表里——023 刚为此选了分表 |
| 版本号照抄 022 的两步写法 | 直接在锁定语句里数号会在并发下数到赢家提交之前的状态，把一次合法保存变成 503（Issue #109 的原文）。这不是可以重新发明的地方 |
| `generated` 在受控集里但产生不出来 | 省略它会让 EP-08 接入时改受控集 → 一次 `CHECK` 迁移 + 所有已存行重新校验。留位并用负例钉住「本阶段恒为 0」代价更小 |
| 采用产生新版本而不是一个指针（Q3=A） | 指针可变，「采用过哪几版」这段历史就没了；而布尔标记要把旧标记改回 false，那是给 append-only 表开 UPDATE，与 FR-008 直接冲突 |
| 编辑副本是列而不是表 | 文档行本来就可变（标题、序号会改），可变列不破坏任何守卫；单独一张表只是多一张表、一组索引和一条删除链 |

## 本卡验不了的两件事（写在这里，不留到实施时才发现）

1. **「已审核 / 已交接 / 已发布的版本永不删除」只有结构保证。** 本卡不落地审核与交接，所以能验的是「没有任何删除版本的路径」，不是「一条已发布的版本真的挺过了某次清理」。后者要等 review-delivery 与交付卡。
2. **「EP-08 接上不必改调用方」今天无法证明。** 能做的是把 `generated` 留在受控集里、把三个入口的形状定下来；真正的证明是 EP-08 落地时没有改本卡的接口——那时候才知道。

**实施时不得把 SC-006（`generated` 恒为 0）当成第 2 条的验收。**
