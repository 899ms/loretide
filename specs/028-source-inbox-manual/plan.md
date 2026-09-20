# Implementation Plan: source-inbox 存储与接口（028 实施 PR 1）

**Spec**: [spec.md](./spec.md)（#172，`1fee64a`） · **Issue**: #174
**Branch**: `claude/impl-028-source-inbox-api`，base `app-main`
**不含页面。**

## 三张表

迁移号从 **516** 顺延（当前最大 515）。**若 #166 先合入占号，rebase 时整体重排**——编号、文件名与 `concurrentIndexCleanups` 的 key 必须一起动，三者逐字节对应。

| 号 | 文件 | 内容 |
|---|---|---|
| 516 | `content_source` | 主表。不可变列 + 可整理列 + 受控状态 |
| 517 | `content_source_id_unique_idx` | `(source_id)` UNIQUE |
| 518 | `content_source_workspace_idx` | `(workspace_id, status, captured_at DESC)` |
| 519 | `content_source_snapshot` | 原文与哈希，只插不改 |
| 520 | `content_source_snapshot_id_unique_idx` | `(snapshot_id)` UNIQUE |
| 521 | `content_source_snapshot_workspace_hash_idx` | `(workspace_id, content_hash)` — 去重查询 + 工作区删除 |
| 522 | `content_source_snapshot_source_idx` | `(workspace_id, source_id)` — 取某条的快照 |
| 523 | `content_source_revision` | 整理记录，只插不改 |
| 524 | `content_source_revision_id_unique_idx` | `(revision_id)` UNIQUE |
| 525 | `content_source_revision_source_idx` | `(workspace_id, source_id, created_at)` |

**每表都有 `workspace_id` 打头的索引**（518 / 521 / 522 / 525）。R5：建表不内联 `PRIMARY KEY` / `UNIQUE`。R3 / R4：每个索引单文件单语句 `CREATE INDEX CONCURRENTLY`。**R6：七个建索引的 up 全部登记 `concurrentIndexCleanups`**（517 / 518 / 520 / 521 / 522 / 524 / 525），建表的三个（516 / 519 / 523）不登记。

### 一处有意的取舍：标签筛选没有 GIN 索引

`tags` 存 `text[]`，按标签筛选用 `$n = ANY(tags)`，走 518 的工作区索引后在品牌内扫描。首版数据量下够用；GIN 索引是后续优化，**不在本卡加**——加它就是再加一个迁移 + 一条登记，而现在没有证据说需要。

## 模块落地：`server/internal/content/source-inbox/`

| 文件 | 内容 |
|---|---|
| `contract.go` | 受控集、结构体、错误、字段校验、`ContentHash`（sha256） |
| `store.go` | `Store`、栅栏、审计与技术日志、整理记录的唯一写入点 |
| `source.go` | 建条目（含粘贴快照）、列表与筛选、单条读取 |
| `organize.go` | 整理（改可变列 + 追加整理记录）、批量打标签 / 归档 |
| `dedup.go` | 按 `content_hash` 找重复候选 |
| `guards_test.go` | 五条守卫（见下） |
| `contract_test.go` | 受控集与字段校验 |
| `store_integration_test.go` | 真实 DB 用例 |

**接入合同三条**（`check:diagnostics-contract`）：`store.go` import diagnostics，并调用 `AuditTx`（审计）、`Technical`（技术日志）、`Child`（追踪）。落地模块数从 **6 → 7**。

> **派单写的是「落地模块 7→8」，与实测不符**：`pnpm check:diagnostics-contract` 现在报「checked 6 landed modules; skipped 6 not yet landed」，已落地的是 diagnostics / workspace-core / ip-profile / topic-planning / work-editor / review-delivery 六个。本卡之后是 7。若 #166 先合入再加一个，则是 8——这大概是派单口径的来源。**以实测数字为准，PR 正文会写实际输出。**

## 五条守卫（`guards_test.go`）

抄 `review-delivery/guards_test.go` 的形状，每条都**扫源码而非扫数据**——「库里没有抓取记录」在空库上恒真，「代码里没有抓取路径」才是本卡的主张。

1. **两张只插不改的表**：源码里没有 `UPDATE content_source_snapshot` / `DELETE FROM content_source_snapshot`，`content_source_revision` 同理；**并断言两张表各有 `INSERT`**——少了后半条，一个没写任何 SQL 的模块也是绿的。
2. **不可变列**：扫每一条 `UPDATE content_source` 的 **SET 段**（不含 `WHERE` 与 `RETURNING`，那里出现列名是合法的），断言不出现 `kind` / `url` / `captured_at` / `recorded_by` / `historical_import`；**并断言至少有一条 `UPDATE content_source`**，否则守卫空转。
3. **零出站**：源码里没有 `net/http`、`http.Get`、`http.Client`、`oauth`、`access_token`、模型调用等字样。这是 FR-002「URL 0 抓取」的代码级证明。
4. **没有抓取接口**：没有 `func (...) Fetch*/Crawl*/Download*/Scrape*(` 形状的方法。
5. **只有两个受控集**：`Kinds` 与 `Statuses`，多一个就红。防的是「顺手给解析状态补个枚举」——Q3 裁决明确不建那一列。

## 端点（`server/internal/handler/content_source.go`）

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/content-sources` | 列表，`status` / `tag` 筛选 |
| POST | `/api/content-sources` | 建条目；`pasted_text` 同事务写快照；响应带 `duplicates` |
| GET | `/api/content-sources/{id}` | 单条 |
| PATCH | `/api/content-sources/{id}` | 整理：`title` / `tags` / `annotation` / `personal_judgement` / `status` |
| GET | `/api/content-sources/{id}/revisions` | 整理历史 |
| POST | `/api/content-sources/bulk` | 批量打标签 / 归档，逐条结果 |
| GET | `/api/content-sources/duplicates` | 按 `content_hash` 返回候选，**只返回，不合并不删除** |

**没有 DELETE。** 静态段 `duplicates` / `bulk` 注册在 `{id}` 之前，否则会被当成 id。

**第 12 步**：`{id}` 与 `{id}/revisions` 各有一条用例穿过真实中间件，且**路径参数值与上下文值不同**。

## core：`packages/core/content/source-inbox/`

`contract.ts`（zod + `parseWithFallback` + 畸形响应用例）、`queries.ts`（列表 / 单条 / 整理历史 / 重复候选的 query，建条目 / 整理 / 批量的 mutation）、`index.ts`。`packages/core/package.json` 加 `./content/source-inbox`。

## 上游改动（单列 `upstream:` 提交，**既有行零删除**）

| 文件 | 改动 |
|---|---|
| `server/cmd/server/router.go` | 挂 `/api/content-sources` |
| `server/cmd/migrate/main.go` | 七条索引清理登记 |
| `server/pkg/db/queries/workspace_delete.sql` + 生成物 | 删除链三张表 |
| `server/internal/handler/workspace_delete_manifest_test.go` | 删除清单三条 |
| `scripts/content-boundaries.json` | `adapters` 加 `content_source.go`；**`modules` 依赖表不动** |

## 变异验证（每处可编译，改完即还原）

| 变异 | 应让谁变红 |
|---|---|
| M1 让 `UPDATE content_source` 的 SET 段带上 `captured_at` | 守卫 2 |
| M2 让整理路径改写快照而不是追加整理记录 | 守卫 1 |
| M3 给 `kind = url` 也写一条快照 | URL 零快照用例 |
| M4 去重端点顺手删掉重复的那条 | 「只提示不删除」用例 |
| M5 从 `concurrentIndexCleanups` 删掉 521 的登记 | `TestEveryConcurrentUpBuildHasCleanup` |

## 不做的事

不含页面、不写 UI 单测、不碰执行器、不解析、不抓取、不做合并、不做选题卡引用、不接今日工作台、不建解析状态列。
