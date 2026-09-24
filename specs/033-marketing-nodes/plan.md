---
description: "Implementation plan for 033 marketing nodes — BO-01 / R-056"
---

# Implementation Plan: 营销节点联动选题（033）

**Prerequisites**: `spec.md`、`contracts/marketing-nodes.md`

**主控前置决定**：2026-09-25 派单 8 条，已编码进 spec 的 FR。**裁决**：Q1/Q2/Q3 均为 A；下方「主控决定」D1–D4 均已批准（主控已裁定 2026-09-25，PR #256 评论）。

---

## 主控决定（均已裁定，2026-09-25）

### D1 `adapters` 加两行（`modules` 不动）——已批准

按主控要求本卡**不改** `scripts/content-boundaries.json`。实施 PR 需要的改动是在 `adapters` 数组末尾追加这两行，**不多不少**：

```json
"server/internal/handler/content_marketing_node.go",
"apps/web/app/[workspaceSlug]/(dashboard)/marketing-nodes/page.tsx"
```

- 第一行：节点的 HTTP 处理函数与两个端口适配器（账号、素材状态）放在新文件里，不塞进已有 349 行的 `content_topic.go`。先例：`content_topic_start.go` 从 `content_topic.go` 分出来时也单独登记了。
- 第二行：新页面是一个适配器，它把 `ip-profile` 的账号列表与 `source-inbox` 的素材列表作为属性传给 `topic-planning` 的页面组件，与 `topics/page.tsx` 同一做法。
- `modules` 依赖表**一个字不改**：节点放在 `topic-planning` 里（D2），它需要的账号与素材都经已有的端口或新的只用字符串的端口拿到。

**谁来改（已定）**：两行由**实施 PR** 各自加上：PR 1 加第一行，PR 3 加第二行，主控审查时对照本节。**本规格 PR 不改 `scripts/content-boundaries.json`**，一个字都不改。

### D2 节点放在 `topic-planning`，不新建模块——已同意

文档 14 写「节点由选题/日历领域拥有」。日历领域不存在（spec Current State 第 3 节），选题领域是 `topic-planning`。放在它里面：

- 建卡、读卡在同一个模块内完成，采用可以在一个事务里建卡并标记候选（FR-032）；
- 账号经已有的 `AccountReader`，素材经 030 的 `SourceReader` 加一个同形的 `SourceStatusReader`，都只用字符串。

**被拒绝的方案**：新登记一个 `marketing-calendar` 模块。它要依赖 `topic-planning`（建卡）、`ip-profile`（账号）、`workspace-core`、`diagnostics`，要改 `modules`；而且采用时建卡会变成跨模块调用，没法与标记候选放进同一个事务，除非 `topic-planning` 暴露一个接受外部事务的函数——那比把节点放进来更侵入。

### D3 开始快照不加「节点版本」扩展字段——已同意

文档 14 说「运行快照引用确定版本」。今天没有「运行」（`agent-workflow` 未落地），只有选题卡「开始」时冻结的 `StoredSnapshot`（`topic-planning/snapshot.go:45-57`）。

- **已定：本卡不加**。可追溯性由「卡 → 候选 → `adopted_revision` → 只插的版本表」提供，改 `snapshot.go` 会碰 023 的守卫文件，且每次开始都要多一次按卡反查候选。
- 未采用的备选：照 030 的做法给 `StoredSnapshot` 加第五个扩展字段 `marketing_node_revisions`（数组：`{node_id, revision}`），放进 PR 2。023 的 `TestNoSourcelessFieldIsInvented` 一行不改。

### D4 预填到 `timing` 的那段文字用哪种语言——已同意中文固定模板

采用时 `timing` 预填一段由字段拼成的事实（contract §4.3）。服务端只存一种写法。**中文固定模板**：节点名、账号都是中文，卡正文也都是中文；做成四语言需要服务端知道用户语言，这在 content 模块里还没有先例。

### 另：远程验收

每个实施 PR 在交付前跑一次**远程验收**：`ssh ... ~/loretide-ci/lt-verify.sh <branch> all`（主控使用的远程编译测试机，带一次性 PG17 测试库与工具链容器）。PR 正文给出它的逐项结果与计数。它不替代本机的 `-v` 带计数的 Go 测试，也不替代手验。

---

## 照抄什么，不发明什么

| 要做的事 | 先例 | 位置 |
|---|---|---|
| 写事务第一条语句取栅栏 | `begin` | `topic-planning/store.go:83` |
| 校验在栅栏内、外来与不存在 404 同形 | `SetAccount` / `checkSources` | `store.go:275-356` |
| 只用字符串的端口 + handler 适配器 | `SourceReader` | `topic-planning/sources.go`，适配器 `handler/content_topic.go:93` |
| 只插的版本表 + 文本扫描守卫 | 022 A6 `TestBriefStoreHasNoUpdateOrDeletePath` | `topic-planning/store_integration_test.go` |
| 缺省与显式空值分开的请求体 | `PatchString` / `TopicBodyPatch` | `topic-planning/contract.go:96-150` |
| 唯一并发索引 + `ON CONFLICT` 幂等 | 538/539 导入幂等 | `server/migrations/539_*`、`content/idempotency/request.go` |
| 「未设 ≠ 零」 | 029 运营规则 | `workspace-core/operating_rules.go:23-31` |
| 时区校验拒绝 `""` 与 `Local` | LT-009 | `handler/workspace.go:224-249` |
| 真实中间件 + 路径参数 ≠ 上下文 id 的路由用例 | 030 | `server/cmd/server/content_topic_routes_test.go` |
| 页面适配器把他模块数据作属性传入 | 选题页 | `apps/web/app/[workspaceSlug]/(dashboard)/topics/page.tsx` |
| 新路由登记 | 031 历史导入（路径与图标）、026/028（侧栏） | `paths.ts:82`、`route-icons.ts:134`、`app-sidebar.tsx:176-182` 的 `contentNav` |

**发明的**：阶段与准备期的算法（contract §3.2）、撞期与重复风险的确定规则（contract §5）、影响清单的「按版本号记决定」（FR-039）。三处都在 spec「我补的地方」里单列。

---

## Technical Context

**语言 / 运行时**：Go 1.26、TypeScript strict、Next.js App Router（web）。

**存储**：PostgreSQL 17。3 张新表、6 个并发索引，共 9 个迁移。**不预占编号**：每个实施 PR 在合并前把迁移改号为紧接当时 `app-main` 最大号之后的连续编号，033 与 034 谁先合并谁先取号；下文用 `N`…`N+8` 表示相对顺序（contract §1.2）。建表迁移在注释外不含 `UNIQUE` / `PRIMARY KEY` / `REFERENCES` / `FOREIGN KEY` / `CASCADE`；候选幂等键 `(workspace_id, node_id, account_id)` 的唯一索引是**单独一个并发索引迁移**（N+8），已与 `content_constraints_test.go` 的 R1–R6 实现逐条核对，无冲突（contract §1.2）。

**既有约束**：R1–R6、工作区删除栅栏（#104）、删除链清单、诊断接入合同、宪法 II / VI / VII / VIII / IX。

**测试**：Go 纯函数单测（日期、校验、判定）、真实 DB 用例（`topic-planning` 与 `handler`）、`cmd/server` 真实路由用例、core 的 node 用例（zod、CSV、表单状态）、**零 UI 单测**、手验清单。

**不需要的**：新模块、`modules` 改动、上游 Multica 业务逻辑改动、执行器、联网。

---

## Constitution Check

| 原则 | 判定 | 依据 |
|---|---|---|
| I CLAUDE.md 权威 | 通过 | 不新增规则 |
| II 不写 UI 单测 | 通过 | 页面零单测；纯函数（CSV、表单状态、解析）放 core 配 node 用例；UI 验收走 `manual-ui-todo.md` |
| III 模块边界 | 通过（待 D1） | 节点在 `topic-planning` 内；新端口只用字符串；`modules` 不动；`adapters` 两行见 D1 |
| IV 服务端 / 客户端状态分离 | 通过 | 节点、候选、影响清单走 TanStack Query，key 含 `wsId`；表单草稿与导入文本放组件本地 |
| V 无外键无级联、并发索引 | 通过 | contract §1.3 逐条；6 个索引各一个文件并登记 R6 |
| VI 响应必解析 | 通过 | 五种响应形状各有 zod 与畸形用例；未知枚举读成「未知」保留原值 |
| VII UI 复用 Multica | 通过 | 设置页布局与既有组件；先读 `docs/development/design/README.md` |
| VIII 范围 | 通过 | 日历、项目、AI 候选、联网、今日工作台、卡上反向显示、`Create` 栅栏外校验，全部记 Out of Scope |
| IX 执行器禁用 | 通过 | 候选全由确定规则与人工输入产生；守卫断言不 import 执行器 |
| X 勾选不等于验收 | 通过 | `tasks.md` 回勾只表示交付 |

**Stop Conditions**：constitution 可读；权威来源可读；文件范围见下方清单；**唯一越出既有登记的是 `adapters` 两行，主控已批准（D1），由实施 PR 添加；没有靠削弱任何规则通过**。

---

## Project Structure（实施阶段的改动白名单）

清单外的文件需要动时，照做但在 PR 正文单列并说明为什么另一条路更差（工作流第 7 步）。

### PR 1 存储、领域服务与节点 API

```
# 编号在合并前按当时 app-main 最大号改定；N = 最大号 + 1
server/migrations/<N>_content_marketing_node.{up,down}.sql                        # 建表，无 UNIQUE/PK/REFERENCES/CASCADE
server/migrations/<N+1>_content_marketing_node_id_unique_idx.{up,down}.sql
server/migrations/<N+2>_content_marketing_node_workspace_idx.{up,down}.sql
server/migrations/<N+3>_content_marketing_node_revision.{up,down}.sql               # 建表
server/migrations/<N+4>_content_marketing_node_revision_id_unique_idx.{up,down}.sql
server/migrations/<N+5>_content_marketing_node_revision_node_revision_idx.{up,down}.sql
server/migrations/<N+6>_content_marketing_node_candidate.{up,down}.sql              # 建表
server/migrations/<N+7>_content_marketing_node_candidate_id_unique_idx.{up,down}.sql
server/migrations/<N+8>_content_marketing_node_candidate_key_idx.{up,down}.sql      # 候选幂等键，单独一个并发唯一索引
server/cmd/migrate/main.go                                   # concurrentIndexCleanups 六行
server/pkg/db/queries/workspace_delete.sql                   # 三张表进删除链
server/pkg/db/generated/workspace_delete.sql.go              # sqlc 重新生成
server/internal/handler/workspace_delete_manifest_test.go    # 三行 workspaceDelete

server/internal/content/topic-planning/marketing_node.go             # 类型、校验、请求解码、tzdata import
server/internal/content/topic-planning/marketing_node_dates.go       # Today / PreparationStartsOn / Phase
server/internal/content/topic-planning/marketing_node_dates_test.go
server/internal/content/topic-planning/marketing_node_test.go        # 校验与变更类型判定
server/internal/content/topic-planning/marketing_node_store.go       # 新建、导入、修改、确认、取消、读、历史
server/internal/content/topic-planning/marketing_node_store_integration_test.go
server/internal/content/topic-planning/marketing_node_guards_test.go # 只插守卫、不碰下游表守卫
server/internal/content/topic-planning/store.go                      # Store 加 Now 字段（仅此一处）

server/internal/handler/content_marketing_node.go            # 新（D1）：处理函数 + 适配器
server/internal/handler/content_marketing_node_test.go       # 真实 DB handler 用例
server/cmd/server/router.go                                  # 挂 /api/content-marketing-nodes
server/cmd/server/content_marketing_node_routes_test.go      # 第 12 步用例

packages/core/api/client.ts                                  # 节点端点方法
packages/core/content/topic-planning/marketing-nodes.ts      # zod + 解析 + 容错
packages/core/content/topic-planning/marketing-nodes.test.ts # 畸形响应用例
packages/core/content/topic-planning/marketing-node-import.ts       # CSV 纯函数
packages/core/content/topic-planning/marketing-node-import.test.ts
packages/core/content/topic-planning/queries.ts              # 节点 hooks
packages/core/content/topic-planning/index.ts                # 导出
```

### PR 2 候选、采用、改期取消影响

```
server/internal/content/topic-planning/marketing_node_candidate.go            # 整理、改、采用、影响、读时计算
server/internal/content/topic-planning/marketing_node_candidate_test.go       # 撞期、缺口、重复风险的纯函数用例
server/internal/content/topic-planning/marketing_node_candidate_integration_test.go
server/internal/content/topic-planning/marketing_node_guards_test.go          # 追加下游表守卫的覆盖
server/internal/content/topic-planning/store.go                               # Create 的 INSERT 抽成事务内函数；Create 行为不变
server/internal/content/topic-planning/marketing_node.go                      # SourceStatusReader 端口声明
server/internal/handler/content_marketing_node.go                             # 候选端点 + SourceStatusReader 适配器
server/internal/handler/content_marketing_node_test.go
server/cmd/server/router.go
server/cmd/server/content_marketing_node_routes_test.go

packages/core/api/client.ts
packages/core/content/topic-planning/marketing-nodes.ts
packages/core/content/topic-planning/marketing-nodes.test.ts
packages/core/content/topic-planning/queries.ts
```

### PR 3 页面

```
apps/web/app/[workspaceSlug]/(dashboard)/marketing-nodes/page.tsx   # 新（D1）：适配器
packages/views/content/topic-planning/marketing-nodes.tsx           # 页面组件
packages/views/content/topic-planning/index.ts                      # 导出
packages/core/content/topic-planning/marketing-node-form.ts         # 表单状态纯函数
packages/core/content/topic-planning/marketing-node-form.test.ts
packages/core/paths/paths.ts                                        # marketingNodes()
packages/core/paths/route-icons.ts                                  # 图标条目
packages/views/layout/app-sidebar.tsx                               # contentNav 在 topics 之后加一项
packages/views/locales/{en,zh-Hans,ja,ko}/common.json               # 四语言：页面文案
packages/views/locales/{en,zh-Hans,ja,ko}/layout.json               # 四语言：侧栏标签（nav 标签从 useT("layout") 读）
specs/033-marketing-nodes/manual-ui-todo.md                         # 如实更新组件与入口
```

### 文档（本规格 PR）

```
specs/033-marketing-nodes/spec.md
specs/033-marketing-nodes/plan.md
specs/033-marketing-nodes/tasks.md
specs/033-marketing-nodes/contracts/marketing-nodes.md
specs/033-marketing-nodes/checklists/requirements.md
specs/033-marketing-nodes/manual-ui-todo.md
```

### 明确不动

- `scripts/content-boundaries.json`：本规格 PR 不改；实施 PR 1 / PR 3 各加 D1 批准的那一行，其余不动
- `content_topic_card` 表结构、`TopicCard` 契约（不加列、不加字段）
- `content_brief_revision`、`content_start_snapshot` 的任何语句；`snapshot.go`（D3 已定不加）
- `review-delivery`、`work-editor`、`feedback-learning`、`source-inbox` 的任何文件
- `normalizeStrings`、`NormalizeSourceIDs`（只调用，不改）
- `Create` 的对外行为（PR 2 只把 INSERT 抽出，账号校验位置不在本卡修，记 Out of Scope 10）
- 今日工作台 `today/page.tsx`

---

## 三个 PR 的分界

| | 内容 | 不含 | 验收 |
|---|---|---|---|
| **PR 1** | 9 个迁移（合并前改号）、删除链、节点的新建 / 导入 / 修改 / 确认 / 取消 / 读 / 历史、日期与阶段、core 解析与 CSV | 候选、采用、影响、页面 | D14-V01（节点部分）、D14-V02（日期部分）、D14-V03（版本只插） |
| **PR 2** | 候选整理与读时计算、改候选、采用（建卡 / 挂卡）、影响清单与决定、下游表守卫 | 页面 | D14-V01（候选与采用部分）、D14-V02（候选与不启动创作）、D14-V03（幂等、影响、下游不变） |
| **PR 3** | 页面、路由、侧栏、四语言、表单纯函数、手验清单 | 新端点 | D14-V08 前半段（手验）；D14-V01/02/03 的页面表现（手验） |

**为什么候选表在 PR 1 建而不是 PR 2**：三张表的迁移号连续、删除链一次改完，比 PR 2 再改一次删除链与 sqlc 生成少一轮审查。PR 1 不写任何访问候选表的代码。

---

## 风险与对策

| 风险 | 为什么真实 | 对策 |
|---|---|---|
| **迁移号与并行分支撞号** | 033 与 034 同时在做，都要新迁移 | 不预占编号；每个实施 PR 合并前按当时 `app-main` 最大号改号，谁先合并谁先取号；改号后重跑迁移规则检查与 `cmd/migrate` 登记用例 |
| **抽出 `Create` 的 INSERT 时改了 `Create` 的行为** | 抽函数最容易顺手把栅栏外的账号校验也挪进来——那是「顺手修」（宪法 VIII） | PR 2 只移动 INSERT 与素材校验到一个接收 `pgx.Tx` 的函数；`Create` 的既有用例全部一行不改且绿；账号校验位置（`store.go:167`，在栅栏外）是**后续项**，主控已确认不在本卡范围（spec Out of Scope 10） |
| **按 24 小时算天数** | 最自然的写法 `start.Add(-time.Duration(n)*24*time.Hour)` 在夏令时切换日差一小时，跨午夜时会差一天 | contract §3.2 规定日期三元组；SC-003 的纽约用例专门抓这个；变异 M2 |
| **「今天」用服务器时区或 UTC** | 服务器在 UTC，上海 00:30 时 UTC 还是前一天 | FR-014 + SC-003 上海 / 洛杉矶同一时刻两个用例；变异 M1 |
| **包级测试读不到时区库** | `time/tzdata` 只在 `main` 包 import，Windows 上没有系统时区库 | 节点文件自己 `import _ "time/tzdata"`（标准库，边界检查允许） |
| **提前量 0 与未设置混同** | 第五次同族问题 | 可空列 + core `{set:false}`；SC-002；变异 M3 |
| **重复整理产生重复候选** | 两个请求并发时应用层的「先查再插」会各插一条 | N+8 单独的并发唯一索引 + `ON CONFLICT DO NOTHING`；并发用例；变异 M4 |
| **重复采用建两张卡** | 双击、重试、并发 | 候选行 `FOR UPDATE` + 已采用直接返回；并发用例；变异 M5 |
| **改期悄悄改了卡** | 「顺便把卡的 timing 更新一下」看起来很贴心 | FR-040 + 下游表守卫 + 逐字节比较用例；变异 M6 |
| **品牌级候选用 NULL 账号** | NULL 在唯一索引里互不相等，品牌级候选会重复 | `account_id text NOT NULL DEFAULT ''`；用例：无适用账号节点整理 3 次仍 1 条 |
| **导入把 `import` 当节点 id** | chi 路由顺序 | 静态段先注册 + 用例（FR-050） |
| **校验挪出栅栏** | 单机顺序执行永远绿 | 照 030 的栅栏用例形状；变异 M7 |
| **撞期查询变慢** | 每条候选都扫同品牌节点 | 品牌内节点数量小；先不优化，实测慢再改，记在这里以免被当成缺陷 |

---

## 变异清单（实施时逐条做，先红后还原）

| | 变异 | 预期变红 |
|---|---|---|
| M1 | 「今天」改用 UTC 而不是节点时区 | 上海 / 洛杉矶同一时刻用例 |
| M2 | 准备期开始日改成 `Add(-n*24h)` | 纽约夏令时用例 |
| M3 | 提前量未设置读成 0 | 未设置 ≠ 0 的 Go 用例与 core 用例 |
| M4 | 整理候选去掉 `ON CONFLICT`，改成先查再插 | 并发整理用例 |
| M5 | 采用时不检查候选已采用 | 重复采用用例（卡数只 +1） |
| M6 | 改期时顺带 UPDATE 卡的 `timing` | 下游表守卫 + 逐字节比较用例 |
| M7 | 账号 / 素材校验挪到 `begin` 之前 | 栅栏用例 |
| M8 | 版本修改改为 UPDATE 旧版本 | 只插守卫 + 历史用例 |
| M9 | 影响清单忽略 `impact_decision_revision` | 「决定后再改期重新出现」用例 |
| M10 | 导入判重跨品牌 | 两品牌同名导入用例 |

**一处变异没变红，说明那条规则没有被任何用例保护**，正确反应是补用例，不是换一个更容易红的变异。
