---
description: "Implementation plan for 030 topic source refs — SOP §5.2 items 2 and 5"
---

# Implementation Plan: 选题卡引用素材条目（030）

**Prerequisites**: `spec.md`、`contracts/topic-source-refs.md`

**裁决**：主控 2026-09-21，PR #207 评论。Q1 = B（两列，按原文改名归属）、Q2 = A、Q3 = B、新端点 `POST /{id}/sources`、反向显示不做。

---

## 照抄什么，不发明什么

这张卡几乎没有新形状要想。每一处都有一个先例，逐条列出来，是为了在实施时能对着看，而不是凭印象做。

1. **端口 + 适配器** 照 #197 的 `Observation`（`feedback-learning/store.go`）。签名只用字符串是关键差别：`feedback-learning` 的依赖行里有 `workspace-core`，所以它能用 `workspacecore.Due`；`topic-planning` 的依赖行里**没有** `source-inbox`，所以端口退到 `(bool, error)`。
2. **栅栏事务内校验 + 同形 404** 照 #133 的 `checkAccount` / `SetAccount`（`topic-planning/store.go:218-247`）。那两段的注释就是本卡要遵守的规则，一字不用改写。
3. **独立端点而不是动作上的字段** 照 `SetAccount`。`store.go:239-243` 的注释已经把理由写好了。
4. **jsonb 数组的读写** 照同表 `channels`：`encodeStrings` / `decodeStrings`（`store.go:57-74`）。**但归一化另写**——`normalizeStrings` 只把 nil 变 `[]`，去空白 / 去重 / 上限三件事一件都不做，而 `channels` 还在用它，改它就是改 022 的行为。
5. **快照扩展字段** 照 `StoredSnapshot` 既有的 `AutoPrecheck` / `UsesNeutralExpression`（`snapshot.go:45-57`）。那段注释同时是「为什么不借 `required_sources`」的答案。
6. **前端新字段的容错** 照 #197 给 `due` 加的 `z.string().catch("")`：一张在新字段出现之前就能显示的卡，不该因为新字段形状不对就整张读不出来。
7. **加列迁移** 照 `407_issue_source_context` 一类的既有加列文件；本卡两个文件各一列，各有 `.down.sql`。

**发明的只有两处**：每栏 50 条的上限，和「请求里缺省某一栏 = 不改这一栏」的语义。两处都在 spec 的「我补的地方」里单列。

---

## Technical Context

**语言 / 运行时**：Go 1.26（后端）、TypeScript strict（前端）、Next.js App Router（web）。

**存储**：PostgreSQL 17。两个加列迁移 **532 / 533**，`jsonb NOT NULL DEFAULT '[]'::jsonb`。无新表、无新索引、无外键、无级联。

**既有约束**：R1–R6（本卡只有 R1 / R2 有对象可遵守，其余三条无新增对象——见 contract §1.1）、工作区删除栅栏（#104）、诊断接入合同、宪法 II（不写 UI 单测）、宪法 VI（zod + `parseWithFallback`）、宪法 VIII（不顺手扩范围）、宪法 IX（不碰执行器）。

**测试**：Go 单测 + 真实 DB 用例（`internal/handler` 与 `internal/content/topic-planning`）、core 的 node 用例、**零 UI 单测**、手验清单。

**不需要的**：新模块、登记表改动、新索引、删除链改动、上游 Multica 改动。

---

## Constitution Check

| 原则 | 判定 | 依据 |
|---|---|---|
| I CLAUDE.md 权威 | 通过 | 本计划不新增规则 |
| II 不写 UI 单测 | 通过 | 页面部分零单测，验收走 `manual-ui-todo.md` |
| III 模块边界 | 通过 | 端口只用字符串 → `topic-planning` 对 `source-inbox` 零 import；`check:content-boundaries` 是机器判定 |
| IV 服务端 / 客户端状态分离 | 通过 | 引用是服务端状态，走 TanStack Query；选择器里「还没保存的选中项」是客户端状态，放组件本地 |
| V 无外键无级联、并发索引 | 通过 | 两个加列迁移，无 FK / CASCADE / 新索引。**逐行复核写进 SC-014**，不默认成立 |
| VI 响应必解析 | 通过 | zod + `parseWithFallback` + 畸形响应用例（FR-019 / FR-020） |
| VII UI 复用 Multica | 通过 | 选择器用既有组件，不新造控件；先读 `docs/development/design/README.md` |
| VIII 范围 | 通过 | §5.2 其余五项的可编辑性是发现的缺口，**记 Out of Scope 6，本卡不做** |
| IX 执行器禁用 | 通过 | 不调用任何执行器；引用全部人手选（FR-024） |
| X 勾选不等于验收 | 通过 | `tasks.md` 回勾只表示实施任务已交付 |

**Stop Conditions 逐条**：constitution 可读；权威来源可读；文件范围见下方清单；**没有任何一条门是靠削弱它要检查的规则通过的**——尤其 022 的 A6 与 023 的负例，本卡是绕开它们而不是改写它们（contract §5、§7）。

---

## Project Structure（实施阶段的改动白名单）

清单外的文件需要动时，**照做但在 PR 正文单列并说明为什么另一条路更差**（工作流第 7 步）。

### 迁移

```
server/migrations/532_content_topic_card_fit_source_ids.up.sql
server/migrations/532_content_topic_card_fit_source_ids.down.sql
server/migrations/533_content_topic_card_evidence_source_ids.up.sql
server/migrations/533_content_topic_card_evidence_source_ids.down.sql
```

### 后端

```
server/internal/content/topic-planning/contract.go        # TopicCard 加两个字段
server/internal/content/topic-planning/sources.go         # 新：归一化 + SourceReader 端口
server/internal/content/topic-planning/sources_test.go    # 新：归一化的 node 级用例（纯函数）
server/internal/content/topic-planning/store.go           # SetSources + checkSources + 创建/读取带上两列
server/internal/content/topic-planning/snapshot.go        # StoredSnapshot 两个扩展字段
server/internal/content/topic-planning/snapshot_test.go   # 新增用例；023 既有负例一行不改
server/internal/content/topic-planning/store_integration_test.go  # 真实 DB 用例
server/internal/handler/content_topic.go                  # SetContentTopicSources + 适配器
server/internal/handler/content_topic_sources_test.go     # 新：真实 DB + 第 12 步路由用例
server/cmd/server/router.go                               # 挂 POST /{id}/sources
```

### 前端

```
packages/core/content/topic-planning/contract.ts          # 两个字段 + zod + 容错
packages/core/content/topic-planning/contract.test.ts     # 畸形响应用例
packages/core/content/topic-planning/form-state.ts        # 草稿里的两份选中列表
packages/core/content/topic-planning/form-state.test.ts   # 归一化与「缺省 ≠ 清空」的 node 用例
packages/core/content/topic-planning/queries.ts           # useSetContentTopicSources
packages/core/content/topic-planning/snapshot.ts          # 快照两个扩展字段 + zod
packages/core/content/topic-planning/snapshot.test.ts     # 同上
packages/views/content/topic-planning/index.tsx           # 两栏各一个选择器 + 详情显示
packages/views/locales/{en,zh-Hans,ja,ko}/common.json      # 四语言
```

### 文档

```
specs/030-topic-source-refs/spec.md
specs/030-topic-source-refs/plan.md
specs/030-topic-source-refs/tasks.md
specs/030-topic-source-refs/contracts/topic-source-refs.md
specs/030-topic-source-refs/checklists/requirements.md
specs/030-topic-source-refs/manual-ui-todo.md             # 实施 PR 2 时产出
```

### 明确不动

- `scripts/content-boundaries.json`（`modules` 与 `adapters` 都不动——`content_topic.go` 已在册）
- `server/cmd/migrate/main.go` 的 `concurrentIndexCleanups`（无新增索引）
- `server/internal/handler/workspace_delete_manifest_test.go`（`content_topic_card` 已在册）
- `server/pkg/db/queries/workspace_delete.sql`（加列不改删除链）
- `server/internal/content/topic-planning/store.go` 里 `content_brief_revision` 的任何语句（022 A6）
- `snapshot.go` 的 `Required` / `Excluded` 两行（023 负例）
- `normalizeStrings`（`channels` 还在用）
- 任何上游 Multica 文件

---

## 两个 PR 的分界

照 SOP 阶段规则：**先出存储与接口 PR，再出页面 PR。**

| | 内容 | 不含 |
|---|---|---|
| **PR 1 存储与接口** | 两个迁移、契约两个字段、归一化、`SourceReader` 端口、`SetSources`、创建路径带上两列、适配器、路由、快照两个扩展字段、core 的解析与 hooks | 页面、四语言、手验清单 |
| **PR 2 页面** | 两栏各自的选择器、详情显示标题与状态、「人手选的」说明、四语言、`manual-ui-todo.md` | 无新端点、无契约改动 |

**为什么 PR 1 就把快照做掉**：快照扩展字段是后端的事，而且它和 `SetSources` 共用同一份「卡上当时引了什么」的读取。拆到 PR 2 会让两个 PR 都碰 `snapshot.go`。

---

## 风险与对策

| 风险 | 为什么真实 | 对策 |
|---|---|---|
| **把引用挂回第 4 项** | 初稿就挂错过一次（按 Issue 的转述）。字段名 `existing_content_relation` 和「与已有作品的关系」念起来很像「和素材的关系」 | 契约用例断言 `TopicCard` 上**只有两个**引用字段且名字逐字相同（contract §7）；spec 的更正表钉在最前面 |
| **`unknown` / 缺省被读成「清空」** | 本仓第四次遇到同一族问题（019 布尔、027 指标值、029 天数、#197 三态）。这次是「请求里没给这一栏」被读成「设成空的」 | FR-006a + SC-003a 两个方向各一条用例；**一次变异**：把「缺省」改成「清空」，确认用例变红 |
| **归档被当成删除** | `content_source` 没有删除路径，所以「不在候选里」很容易被顺手写成「引用失效」 | FR-014 + SC-007；用例：引用 → 归档 → 引用数不变；已归档 id 直接提交 → 接受 |
| **校验被挪出栅栏** | 写起来更顺手，而且测不出来——单机顺序执行永远绿 | 照 #133 的形状，栅栏是事务第一条语句；**一次变异**：把校验挪到 `begin` 之前，确认栅栏用例变红 |
| **端口签名混进 `sourceinbox` 类型** | 写 `Exists` 时很想直接返回 `sourceinbox.Source` 好拿标题 | `check:content-boundaries` 是机器判定，会当场红。标题由前端用 `useContentSources` 自己查，不走这个端口 |
| **部分写入** | 两栏两次写，很容易写成「第一栏成功、第二栏失败」 | 同一事务；FR-013 + SC-005：一条非法 → 两栏都是 0 条 |
| **023 的负例被顺手改掉** | 加快照字段时最容易顺手动那张表 | SC-008 断言它一行未改；**diff 里 `snapshot_test.go` 的既有行零删除** |
| **N+1 校验** | 50 条引用 = 50 次 `Exists` | 端口按需可以是单条；若实测慢，改成一次批量查询——**但这是实施期的度量决定，不预先优化**，记在这里以免日后被当成缺陷 |

---

## 变异清单（实施时逐条做，先红后还原）

| | 变异 | 预期变红 |
|---|---|---|
| M1 | 把「请求里缺省某一栏」改成「清空这一栏」 | `缺省 ≠ 清空` 的用例 |
| M2 | 校验挪到栅栏之前 | 栅栏用例 |
| M3 | 归一化去掉「去重」 | 同一条提交两次存两条的用例 |
| M4 | 一条非法时先写合法的那几条 | 全有或全无的用例 |
| M5 | 把两栏合并成一个列表冻进快照 | 快照分栏用例 |
| M6 | 往 `required_sources` 里写引用 | **023 的既有负例**（证明它仍在工作） |
| M7 | 给 `TopicCard` 加第三个引用字段（挂第 4 项） | 契约用例（只有两个引用字段） |

**一处变异没变红，说明那条规则没有被任何用例保护**，正确反应是补用例，不是换一个更容易红的变异（工作流「变异验证怎么做」）。
