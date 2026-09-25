---
description: "Implementation plan for 036 topic-planning + work-editor + feedback-learning — platform search optimization (BO-05 / R-060)"
---

# Implementation Plan: 平台搜索优化（036）

**Spec**: [spec.md](./spec.md) ｜ **Contract**: [contracts/search-optimization.md](./contracts/search-optimization.md) ｜ **Tasks**: [tasks.md](./tasks.md)

**Status**: 待裁决（Q1～Q8，见 spec 文末）。下文按推荐值写：**三个模块五张表、十一个索引、一处 CHECK 变更，共十八个迁移，拆四个 PR。**

| PR | 模块 | 表 | 索引 | 其他迁移 | 迁移合计 |
|---|---|---|---|---|---|
| 1 搜索主题 | `topic-planning` | 1 | 2 | — | 3 |
| 2 建议与采用 | `topic-planning` + `work-editor` | 3 | 4 | 1（`action` CHECK） | 8 |
| 3 搜索指标与排名观察 | `feedback-learning` | 2 | 5 | — | 7 |
| 4 页面 | — | — | — | — | 0 |

撰写时 `app-main` 最大迁移号 584（`8bd4d55`）；**不预留**，合入前改号（D6）。

## 主控决定（待裁定）

本规格 PR **不改** `scripts/content-boundaries.json`。

### 1. 模块落点（D5 / Q1）

| | A 建议放 `topic-planning`（推荐） | B 建议放 `work-editor` |
|---|---|---|
| 文档依据 | 文档 14「搜索主题由选题领域维护」；D3「经作品编辑器公开接口、由 handler 适配器」 | 文档 12 §2：`work-editor` 独占「候选修改」「提交候选/采用新版本」 |
| 采用的事务 | 三步：决定（`topic-planning`）→ `ApplyBody`（`work-editor`，幂等键）→ 效果；中途失败由重试收敛 | 一个事务：决定 + 版本 + 效果 |
| 主题引用 | 同模块，直接读 | `work-editor` 要一个新适配器反向读 `topic-planning` 的主题 |
| `work-editor` 的变化 | 一个公开函数 `ApplyBody`、一个动作值 | 三张表、主题/问题/依据这些搜索概念、比较与差异 |
| `modules` 依赖表 | 不改 | 不改 |

**推荐 A**：它就是 D3 描述的形状；`appendVersion` 已有幂等键机制，跨模块窗口不用新发明；`work-editor` 只长一个函数。B 唯一的优势是原子性，而 A 的窗口已经有 035 验证过的收敛做法。

指标与观察放 `feedback-learning`、主题放 `topic-planning`，两处没有备选：文档 14 原话指定。

### 2. `adapters` 追加——由各实施 PR 自带（待批准；本规格 PR 不改）

| PR | 条目 | 为什么需要 |
|---|---|---|
| 1 | `server/internal/handler/content_search_themes.go` | 主题的 HTTP 处理函数；复用既有 `topicSourceReader` 与账号读 |
| 2 | `server/internal/handler/content_search_suggestions.go` | 建议、比较、决定、重试；`searchWorks` 适配器实现 `topicplanning.SearchWorks`，调 `workeditor.Store` 的 `GetArtifact`、`ListVersions`、`GetVersion`、`ApplyBody` |
| 3 | `server/internal/handler/content_search_observations.go` | 指标与观察；`searchThemes` 适配器实现 `feedbacklearning.SearchThemes`，调 `topicplanning.Store` 的主题读 |
| 4 | `apps/web/app/[workspaceSlug]/(dashboard)/search-optimization/page.tsx` | 页面适配器：组合 `topic-planning` 与 `feedback-learning` 两个模块的视图，并把作品/文档/版本列表作为属性传入 |

`modules` 依赖表**一个字不改**。

### 3. `work-editor` 的公开契约新增（PR 2）

`Store.ApplyBody` + 三个错误哨兵 + 动作 `suggestion_applied`（contract §7.4、§1.7）。PR 2 正文按文档 12 §6 写：「所属模块 `work-editor`；公开契约变更：新增 `ApplyBody` 与动作值 `suggestion_applied`；消费者：`topic-planning`（经 handler 适配器）、前端版本列表；既有 `SaveVersion` / `RestoreVersion` / `AdoptVersion` / `ImportVersion` 不变」，并跑 `work-editor` 全部既有测试与前端 `work-editor` 契约测试。

`idempotency` 包目前只服务历史导入（包注释如此写）；本卡让 `work-editor` 在第二个场景用它。它已在 `work-editor` 的依赖里，不是新依赖；本卡不改该包的代码与注释（注释更新记为后续项）。

### 4. D14-V15：部分自动化验收

服务端链路一条 handler 包真实库用例（SC-014）；浏览器闭环进 `manual-ui-todo.md` 036-U-30，记「未执行」；AI 分析与联网本版不适用。请主控确认接受这一部分验收（与 034 / 035 同一处理）。

### 5. Q2～Q8

见 `spec.md` 文末「待裁决问题」。

## 照抄什么，不发明什么

七处照抄：

1. **写事务第一句取删除栅栏、同事务写审计**：`topic-planning/store.go:89` 的 `begin()`、`feedback-learning/store.go:109` 的 `begin()`、`work-editor` 的 `appendVersion`。
2. **越权与「不存在」同形**：`handler/content_topic.go` 的 `topicScope`、034 的 `roiScope`。
3. **模块定义小接口、handler 适配器回答**：035 的 `opdiagTopics` / `opdiagWorks` 与 `h.opdiagStore()` 注入。
4. **已删除工作区映射**：035 的 `opdiagReadError`（`handler/content_opdiag_reports.go:97-110`），三个新适配器文件各写一个同形函数，不共享（各自映射到自己调用方模块的错误）。
5. **修订制 + `base_revision` 409**：033 营销节点、034 记录、035 判断与建议。
6. **跨模块采用的「决定 → 动作 → 效果」与幂等收敛**：035 PR 3 的建卡（spec Q3 补充）；本卡的幂等机制用 `work-editor` 已有的 `idempotency.Request`，不给 `work-editor` 加列。
7. **CHECK 集合扩一项**：迁移 535（`content_artifact_version_action_imported`）。

两处**不照抄**：

- **不照抄 027 把搜索指标塞进 `Metrics`**（Q2）：新表同形照抄。
- **不照抄 035「一个修订一个决定」**：一条建议一个决定（spec「本规格补的设计」第 2 条）。

## Technical Context

**Language/Version**: Go 1.26（`server/`）、TypeScript 5 strict（`packages/core`、`packages/views`）

**Storage**: PostgreSQL。五张新表（contract §1）+ 十一个 `CONCURRENTLY` 索引（§2）+ 一处 CHECK 变更（§1.7）；每个索引单独一个迁移文件、单条语句。主题的数组列是 jsonb，列表在 Go 里过滤。

**Testing**: `go test ./internal/content/topic-planning/ ./internal/content/work-editor/ ./internal/content/feedback-learning/ ./internal/handler/ ./cmd/server/`；迁移两套本机必须带假库地址（见「验证」）；带库的套件按 `docs/development/testing-database-suites.md` 用 `scripts/test-go-db.sh --suite handler` 与 `--suite cmd-server`；`packages/core/content/{topic-planning,feedback-learning}/search/*.test.ts`（node 环境）。**无 UI 单测。**

**Constraints**: 不联网、不调模型、不起执行器、不打分、不排名、不预测、不计关键词次数、不读别的模块的表、不写 `review-delivery`。页面只用既有 Multica 组件。

**Scale**: 一个品牌几十到几百个主题、每个文档几条开着的建议、每月几十到几百条观测。差异按行算，200000 rune 的正文在 Myers 下是毫秒级。

## Constitution Check

| 原则 | 状态 | 说明 |
|---|---|---|
| I. CLAUDE.md 为准 | 通过 | 迁移、状态、API 兼容规则按 CLAUDE.md 与 constitution 指向的文件 |
| II. 不写 UI 单测 | 通过 | 差异、状态派生、显示键选择进 Go / core node 测试；界面进 `manual-ui-todo.md` |
| III. 模块边界 | 通过（Q1=A 时） | 三个模块各管各的表；互不 import；`modules` 不改；跨模块经三个 handler 适配器；`adapters` 追加待批准 |
| IV. 服务端/客户端状态分离 | 通过 | 主题、建议、观测走 TanStack Query，键含 `wsId`；表单与比较勾选在组件状态；写后失效查询，**不做乐观更新**（采用跨模块） |
| V. 无外键、并发索引、单语句 | 通过 | 十八个迁移逐条过 R1–R6；每张表有 `workspace_id` 打头的索引 |
| VI. zod + `parseWithFallback` | 通过 | 每个新端点一份 schema、一条畸形响应用例；`state`、`intent`、`origin`、`result_kind`、`metric` 用 `z.enum` 且有兜底；`work-editor` 的 `VERSION_ACTIONS` 加一项 |
| VII. 复用 Multica | 通过 | `SettingsSection` / `SettingsCard` / `SettingsRow`、`Button` / `Input` / `Textarea` / `Select`；不设颜色；「未知」用次要文字色 |
| VIII. 范围纪律 | **需注意** | AI 建议、联网研究、搜索量接口、CSV 导入、诊断接入明确不做；`HoldForChangedTarget` 无调用方、`feedbackPublications` 错误映射两处既有问题记后续项 |
| IX. 执行器禁用 | 通过 | 不调模型；`author_kind` 只有 `human`；`data_origin` 只有 `manual_only`；守卫 |
| X. 打勾不是验收 | 通过 | 界面项一律「未执行」；预检报告过期只能结构性证明（EP-06 未实现），D14-V15 只能部分覆盖，都如实写 |

## Project Structure（改动白名单）

```text
server/
├── migrations/
│   ├── <N>_content_search_theme_revision*.{up,down}.sql            # PR 1：3 个
│   ├── <N>_content_search_suggestion_*.{up,down}.sql               # PR 2：7 个
│   ├── <N>_content_artifact_version_action_suggestion_applied.{up,down}.sql   # PR 2：1 个
│   └── <N>_content_search_{metric,rank_observation}*.{up,down}.sql # PR 3：7 个
├── internal/content/topic-planning/
│   ├── search_contract.go        # 受控集、主题与建议类型、错误哨兵（PR 1、2）
│   ├── search_theme.go           # 主题读写（PR 1）
│   ├── search_diff.go            # DiffLines 纯函数（PR 2）
│   ├── search_ports.go           # SearchWorks 接口（PR 2）
│   ├── search_suggestion.go      # 建议、比较、决定、效果、重试（PR 2）
│   └── search_*_test.go          # 含 search_guards_test.go
├── internal/content/work-editor/
│   ├── apply.go + apply_integration_test.go   # ApplyBody（PR 2，公开契约新增）
│   ├── contract.go               # Actions 加 suggestion_applied（PR 2）
│   └── guards_test.go            # 既有守卫覆盖 apply.go（PR 2 只核对，不放宽）
├── internal/content/feedback-learning/
│   ├── search_contract.go        # 受控集与类型（PR 3）
│   ├── search_ports.go           # SearchThemes 接口（PR 3）
│   ├── search_metric.go / search_observation.go   # PR 3
│   ├── search_*_test.go
│   └── guards_test.go            # 登记三个新受控集（PR 3）
├── internal/handler/
│   ├── content_search_themes.go / _suggestions.go / _observations.go
│   ├── content_search_*_test.go  # 含每个适配器方法的「删除后 404」用例
│   └── workspace_delete_manifest_test.go          # 加五张表
├── pkg/db/queries/workspace_delete.sql            # 加五条 DELETE；make sqlc 产物单独提交
├── cmd/migrate/main.go                            # upstream：11 条 concurrentIndexCleanups
└── cmd/server/
    ├── router.go                                  # upstream：/api/content-search 块
    └── content_search_routes_test.go

packages/core/
├── content/topic-planning/search/{contract,contract.test,queries,display,display.test}.ts
├── content/feedback-learning/search/{contract,contract.test,queries,display,display.test}.ts
├── content/work-editor/contract.ts + contract.test.ts   # VERSION_ACTIONS 加一项（PR 2）
├── content/{topic-planning,feedback-learning}/index.ts  # 导出
├── paths/paths.ts, paths/route-icons.ts                 # upstream：searchOptimization（PR 4）
└── diagnostics/diagnostic-context.ts                    # upstream：["search-optimization"]（PR 4）

packages/views/
├── content/topic-planning/search/*.tsx            # 主题、建议、比较（PR 4）
├── content/feedback-learning/search/*.tsx         # 搜索表现（PR 4）
├── content/work-editor/index.tsx                  # 动作文案一行（PR 2）+ 「搜索优化」链接（PR 4）
├── layout/route-icon-components.tsx               # upstream：图标（PR 4）
└── locales/{en,zh-Hans,ja,ko}/*.json              # 四语言（PR 2 动作文案、PR 4 页面）

apps/web/app/[workspaceSlug]/(dashboard)/search-optimization/page.tsx   # PR 4
scripts/content-boundaries.json                    # 只追加 adapters（「主控决定」第 2 条，待批准）；modules 不动
specs/036-search-optimization/manual-ui-todo.md    # PR 4 回写状态
```

清单外的改动在 PR 正文单列并说明理由。

## 四个 PR 的形状

### PR 1 —— 搜索主题存储与接口（`topic-planning`）

一张表 + 两个索引 = 三个迁移。`search_contract.go`（主题部分）、`search_theme.go`。端点：contract §7 标 1 的五行。

最容易漏的三处：

1. **严格解码拒绝 `search_volume` / `competition` / `rank` / `scope` / `budget`**（FR-002、FR-014）：请求类型里没有这些字段，`DisallowUnknownFields` 自然点名；用例逐个写，防止以后有人「顺手」加一个可选字段。
2. **账号与平台一致**：`account_id` 非空时，读账号的平台，与 `platform` 不等 → 400 `platform`。
3. **引用核实不泄露存在性**：别的品牌的素材、选题卡、简报与不存在的，拒绝体逐字节相同。

### PR 2 —— 建议、比较、采用 / 放弃（`topic-planning` + `work-editor`）

三张表 + 四个索引 + 一处 CHECK = 八个迁移。`search_diff.go`、`search_ports.go`、`search_suggestion.go`；`work-editor/apply.go`；handler `content_search_suggestions.go`；前端 `VERSION_ACTIONS` 与动作文案。

最容易漏的五处：

1. **预检查在决定之前**（FR-052）：基础版本已变或有未保存草稿时，决定表行数不变。
2. **幂等 `Claim` 在 `ApplyBody` 的核对之前**（FR-054）：否则「版本已写、效果未记」后的重试会因为「最新版本 ≠ 基础版本」而失败，而不是拿回同一个版本。用测试钩子在 ⑤ 与 ⑥ 之间注入失败钉住。
3. **放弃不调任何写**（FR-051）：假 `SearchWorks` 记调用次数。
4. **不写 `review-delivery`**（FR-057）：SC-006 的真实库用例 + 守卫。
5. **`ApplyBody` 不开第二条写版本的路径**：作为 `appendVersion` 的意图实现；`work-editor` 既有守卫一条都不放宽。

规模偏大：若审查不便，可拆成 2a（建议、比较、放弃，含三张表与四个索引共七个迁移）与 2b（`ApplyBody`、动作迁移、采用与重试、前端动作文案）。拆不拆由主控在派单时定。

### PR 3 —— 搜索指标与排名观察（`feedback-learning`）

两张表 + 五个索引 = 七个迁移。`search_contract.go`、`search_ports.go`、`search_metric.go`、`search_observation.go`；handler `content_search_observations.go`。

最容易漏的三处：

1. **027 包级守卫**：SQL 不聚合、`count(` 不新增；列表按时间排序在 SQL 里做（`ORDER BY` 不是聚合）。
2. **nil 与 0**：`value` 用 `*int64`，JSON `null` 与 `0` 各一条往返用例。
3. **`theme_id` 核实经适配器**：`feedback-learning` 不 import `topic-planning`，import 守卫覆盖 `_test.go`。

### PR 4 —— 页面

`/{workspaceSlug}/search-optimization` 三个区块：「搜索主题」（列表、筛选、表单、修订历史、未知显示）、「优化建议」（按文档列建议、写建议、差异、比较、采用 / 放弃、重试、固定提示）、「搜索表现」（按发布记录登记与查看搜索指标；按主题 / 发布记录登记与查看排名观察）。页面适配器组合两个模块的视图。作品编辑器页加一个「搜索优化」链接（带 `work_id`、`artifact_id`）。四语言。`manual-ui-todo.md` 回写。

**依赖**：PR 2 依赖 PR 1（建议引用主题）；PR 3 依赖 PR 1（观察引用主题），与 PR 2 无依赖，可并行；PR 4 依赖 PR 1～3。

## 上游改动（每个 PR 单独一个 `upstream:` 提交，PR 正文单列一节）

| 文件 | PR | 改什么 |
|---|---|---|
| `server/cmd/server/router.go` | 1–3 | `/api/content-search` 块，各 PR 追加自己的路由；新块前空行 + 一行注释 |
| `server/cmd/migrate/main.go` | 1–3 | `concurrentIndexCleanups` 追加本 PR 的索引，自成一块 |
| `packages/core/paths/paths.ts` | 4 | `searchOptimization: () => \`${ws}/search-optimization\`` |
| `packages/core/paths/route-icons.ts` | 4 | `searchOptimization: { segment: "search-optimization", icon: "SearchCheck", navKey: "search_optimization" }`（图标 PR 4 核实，见 tasks） |
| `packages/views/layout/route-icon-components.tsx` | 4 | 登记所选图标 |
| `packages/core/diagnostics/diagnostic-context.ts` | 4 | `["search-optimization"]` |

每个都要 `git diff -w` 只有新增行。

## 验证

每个 PR 本地至少：

- `(cd server && DATABASE_URL='postgres://none:none@127.0.0.1:1/none?sslmode=disable' go test ./cmd/migrate ./internal/migrations)` ——**必须带这个假库地址**：上游 migrate 测试不设时默认连 `localhost:5432/multica`，会连到本机真库。
- 本 PR 涉及的模块：`(cd server && go test ./internal/content/topic-planning/)`、`./internal/content/work-editor/`、`./internal/content/feedback-learning/`
- `(cd server && go test ./internal/handler/ -run 'ContentSearch|WorkspaceDelet')`
- `(cd server && go test ./cmd/server/ -run 'ContentSearch')`
- `pnpm typecheck --force`、`pnpm check:content-boundaries`、`pnpm check:diagnostics-contract`
- `pnpm --filter @multica/core exec vitest run content/topic-planning/search content/feedback-learning/search`（PR 2 另加 `content/work-editor`）

带库的两套（`scripts/test-go-db.sh --suite handler`、`--suite cmd-server`）与全量检查以**远程验收**为准：主控在构建服务器上跑 `~/loretide-ci/lt-verify.sh <branch> all`。本地没跑的检查在 PR 正文记「未执行」，不记通过；带库用例报 PASS / SKIP 计数，不以退出码为证据（缓存命中的 typecheck 不是证据，要 `--force`）。

## 已知边界

1. **预检报告过期只能结构性证明**（Current State §1）。EP-06 未实现；本卡保证新 `version_id` 与正文变化，并把「按 `version_id` 绑定报告」写进合同 §8。
2. **采用有跨模块窗口，由重试收敛**（Q1=A、Q6=A）。版本已写而效果未记时，建议显示「已采用，结果未记录」，要有人点重试；彻底消除要 Q1=B。
3. **决定之后的竞态会留下一个失败的采用**（US5 场景 5）。预检查挡住了常见情况；剩下的竞态结果是 `failed/base_moved`，终局，人需要基于新版本重写。
4. **标题、话题、描述不结构化**（Q4=A）。`aspects` 只是标签；系统不能核对「这条建议真的只改了标题」。
5. **差异按行**。中文长段落里改一个字，整段显示为一删一插；这是确定性与实现成本的取舍，不做字级差异。
6. **主题平台可以是八个，观测只能在四个交付渠道上**（Q8=A）。
7. **D14-V15 只能部分自动化**（「主控决定」第 4 条）。

## 发现的既有问题（后续项，不在本卡修）

- `review-delivery/delivery.go:249` 的 `HoldForChangedTarget` 在服务端没有调用方：内容出新版本后，交付任务不会自动转 `held`，SOP 9.3 的提醒不存在。
- `handler/content_metric.go:83-93` 的 `feedbackPublications.Resolve` 把任何查询错误（含数据库故障）映射为 `ErrNotFound`：存储故障会以 404 出现。本卡复用它，但不改。
- `idempotency` 包注释只说历史导入；本卡之后有第二个使用场景，注释应更新（不在本卡改）。
