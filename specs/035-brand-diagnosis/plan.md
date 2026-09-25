---
description: "Implementation plan for 035 feedback-learning — brand/account operating diagnosis (BO-02 / R-057)"
---

# Implementation Plan: 品牌/账号经营诊断（035）

**Spec**: [spec.md](./spec.md) ｜ **Contract**: [contracts/brand-diagnosis.md](./contracts/brand-diagnosis.md) ｜ **Tasks**: [tasks.md](./tasks.md)

**Status**: 已裁决（主控 2026-09-25，PR #267 评论）：Q1～Q8 采纳推荐值；Q3 补充建卡幂等键；下列五条「主控决定」全部已定。**本模块八张表、十六个索引、二十四个迁移，外加 `topic-planning` 的一列、一个索引、两个迁移（Q3 补充），拆四个 PR。**

## 主控决定（均已裁定，2026-09-25，PR #267 评论）

本规格 PR **不改** `scripts/content-boundaries.json`（D8）。

### 1. 模块落点（D9 / Q1）——放进 `feedback-learning`，主控已裁定 2026-09-25

| | A `feedback-learning`（推荐） | B 新模块 `operating-diagnosis` |
|---|---|---|
| 文档依据 | 文档 14「诊断由复盘领域拥有」；文档 12 把「AI 复盘、待采纳/已采纳经营结论」划给 `feedback-learning` | 文档里没有这个模块名 |
| `modules` 依赖表 | **不改** | 要新增一行：`"operating-diagnosis": ["workspace-core", "feedback-learning", "review-delivery", "diagnostics"]`——用户禁止改这张表 |
| 读指标与摘录 | 同模块，直接读 | 要 `feedback-learning` 新开公开读接口，或经 handler 适配器转一手 |
| 读 034 ROI 摘要 | 同模块，直接调 `ReportSummary` | 依赖 `feedback-learning`（同上一行） |
| 读账号、配置、选题卡、作品、发布记录 | 经 handler 适配器（两者相同） | 同左 |
| 代价 | `feedback-learning` 的包级守卫（spec Current State §3）约束本卡的写法；模块变大（027 + 034 + 035） | 一行 `modules` 改动；多一层公开接口 |

**A，主控已裁定 2026-09-25**。B 唯一的好处是模块小一些，而代价正是用户禁止的那张表。

### 2. `adapters` 追加——已同意，由各实施 PR 自带（主控已裁定 2026-09-25；本规格 PR 不改）

| PR | 条目 | 为什么需要 |
|---|---|---|
| 1 | `server/internal/handler/content_opdiag_reports.go` | 报告与标注的 HTTP 处理函数，以及账号、配置、经营规则、选题卡、作品、发布记录/审核/交付六个只读适配器 |
| 2 | `server/internal/handler/content_opdiag_preview.go` | `/preview` |
| 3 | `server/internal/handler/content_opdiag_decisions.go` | 判断、建议、决定、提议、待办；建卡（`topicplanning.Store.Create` / `Get`）与写配置（`ipprofile.Service.SetProfile`）两个写适配器 |
| 4 | `apps/web/app/[workspaceSlug]/(dashboard)/operating-diagnosis/page.tsx` | 页面适配器：把账号列表、选题卡列表作为属性传给 `feedback-learning` 的页面组件 |

`modules` 依赖表**一个字不改**。

### 3. 迁移编号：不预留（D6）

每个实施 PR 合入前，把自己的迁移改号为紧接当时 `app-main` 最大号之后的连续号（文件名、`concurrentIndexCleanups` 的键一起改）。撰写时 `app-main` 最大号 575，#266 将占 576–578。

### 4. D14-V08：部分自动化验收——已接受

服务端链路用一条 handler 包的真实库用例串起来（SC-014）；浏览器闭环进 `manual-ui-todo.md` U-40，记「未执行」；「真实联网」本版不适用。主控已接受这一部分验收（2026-09-25，与 034 的 D14-V15 同一处理）。

### 5. Q2～Q8——均采纳推荐值，主控已裁定 2026-09-25

见 `spec.md` 文末「裁决记录」。

### 6. Q3 补充：建卡幂等键——主控已裁定 2026-09-25

采纳为选题卡带每条建议一个的幂等键 `opdiag-suggestion:<suggestion_id>`；重试关联已建的卡，不建第二张；「卡已建、效果未记」由重试收敛（spec FR-063a，contract §7.4）。

**这一条要动 `topic-planning`**：「同键至多一张卡」只能由建卡的一方保证。改动是增量的，放在 PR 3：`content_topic_card` 加可空列 `origin_key`（一个迁移）、唯一并发索引 `content_topic_card_origin_key_idx`（一个迁移）、公开函数 `Store.CreateOnce`。既有 `Create`、022 的响应 schema、`modules` 依赖表都不变。它是 `topic-planning` 的**公开契约新增**，PR 3 正文要按文档 12 §6 写明「公开契约变更：新增 `CreateOnce`，既有消费者不受影响」，并跑 `topic-planning` 的既有测试。主控 2026-09-25 已接受这三处新增。

**接线方式（主控 2026-09-25 追加条件：`feedback-learning` 不得 import `topic-planning`）**：模块在 `opdiag_decisions.go` 里定义小接口 `TopicCardCreator`，只用本模块的类型与字符串：

```go
// TopicCardDraft is what an adopted suggestion asks for. Plain strings only:
// this module does not import topic-planning.
type TopicCardDraft struct {
	AccountID string // "" = no account
	IPFit     string // the suggestion body
}

type TopicCardCreator interface {
	// CreateOnce returns the card created under key, creating it the first time.
	CreateOnce(ctx context.Context, workspaceID, actor, key string, draft TopicCardDraft) (topicCardID string, created bool, err error)
	// Exists answers the link mode: the card is here and its account matches.
	Exists(ctx context.Context, workspaceID, actor, topicCardID, accountID string) error
}
```

`server/internal/handler/content_opdiag_decisions.go`（已登记为 `adapters` 的 handler 文件）里的 `opdiagTopicCards{store *topicplanning.Store}` 实现它，调 `topicplanning.Store.CreateOnce` / `Get`，由 `h.opdiagStore()` 注入，`topicplanning.Store` 取自既有的 `h.topicPlanningStore()`（`handler/content_topic.go:82`）。这与 034 完全同形：`handler/content_roi_records.go` 定义 `roiAccounts{service *ipprofile.Service}`、`roiWorks{store *workeditor.Store}`，在 `h.roiStore()` 里注入 `feedbacklearning.ROIStore`（`:88-95`），`feedback-learning` 自己不 import `ip-profile` 或 `work-editor`。`ip-profile` 的写接口 `DiagProfileWriter` 同样照此接线。`modules` 依赖表不变。

一条守卫用例（tasks T075a）扫 `feedback-learning` 的全部 Go 源文件（含测试），确认没有 import `server/internal/content/topic-planning`；`pnpm check:content-boundaries` 也会挡，但守卫让它在 `go test` 里就红。

### 7. 图标——PR 4 核实

PR 4 核实项目所用 lucide 版本里有 `Stethoscope`；没有就换一个项目里已有、且与开发诊断的 `Activity` 不同的图标，并在 PR 正文写明（tasks T085）。

### 8. 顺带发现的既有问题——维持后续项

027 `PendingRegistrations` 直接读 `review-delivery` 的表、`feedback-learning` 包注释过时：不在本批（见文末）。

## 照抄什么，不发明什么

六处照抄：

1. **写事务第一句取删除栅栏、同事务写审计**：照 `feedback-learning/store.go:109` 的 `begin()`。
2. **越权与「不存在」同形**：照 `handler/content_metric.go` 的 `feedbackScope` 与 034 的 `roiScope`。
3. **不 import 别的模块，模块定义小接口、handler 适配器回答**：照 `feedbackPublications` 与 034 的 `Accounts` / `Works`。本卡新增的读接口：`DiagAccounts`（`AccountExists`、`Account`、`CurrentProfile`）、`DiagRules`（经营规则与时区）、`DiagTopics`（选题卡的账号）、`DiagWorks`（作品）、`DiagDelivery`（发布记录、审核、交付的全工作区列表）；写接口（PR 3）：`TopicCardCreator`（`CreateOnce` 建卡、`Exists` 只读核实卡，见「主控决定」第 6 条）、`DiagProfileWriter`（`SetProfile`）。这些接口只用本模块的类型与字符串，由 handler 里的适配器实现；`feedback-learning` 不 import `topic-planning` 或 `ip-profile`。
4. **受控集 Go 为准 + `CHECK` 兜底 + 「恰好 N 项」用例 + 登记进 027 的第六集守卫**：照 027 与 034。
5. **报告版本只插、`inputs` 存副本、读时派生「输入已有更新」、复算逐字节相同**：照 034 PR 4（#266）的 `roi_report.go`。
6. **读对方源文件对表、不 import**：`profileFieldKeys` 对 `ip-profile/profile.go` 的 JSON 名，照 027 `TestThePlatformSetAgreesWithIPProfiles`。

两处**不照抄**：

- **不照抄 027 `PendingRegistrations` 直接读 `content_publication_record`**（spec Current State §4）。本卡经 `review-delivery.ListPublications` 读。
- **不照抄 033 的「同一事务建卡」**。033 能在一个事务里建卡，是因为节点就在 `topic-planning` 里；本卡在另一个模块，只能「先记决定、再建卡、后记结果」（Q3）。

## Technical Context

**Language/Version**: Go 1.26（`server/`）、TypeScript 5 strict（`packages/core`、`packages/views`）

**Storage**: PostgreSQL。八张新表（contract §1）+ 十六个 `CONCURRENTLY` 索引（§2），每个索引单独一个迁移文件、单条语句。均值与差值用 `math/big.Rat`，JSON 里是字符串。

**Testing**: `go test ./internal/content/feedback-learning/ ./internal/handler/ ./cmd/server/`；迁移两套本机必须带假库地址（见「验证」）；带库的套件按 `docs/development/testing-database-suites.md` 用 `scripts/test-go-db.sh --suite handler` 与 `--suite cmd-server`；`packages/core/content/feedback-learning/opdiag/*.test.ts`（node 环境）。**无 UI 单测。**

**Constraints**: 不调模型、不起执行器、不外发、不用浮点、不打分、不排名、不读素材与知识、不读别的模块的表、不写开发诊断的表。页面只用既有 Multica 组件。

**Scale**: 一个品牌几个到十几个账号，每月几十到几百条发布记录。生成时把全工作区发布记录、审核、交付读进内存后在 Go 里过滤即可；`review-delivery` 的读接口没有时间过滤参数，也不需要为此给它加。

## Constitution Check

| 原则 | 状态 | 说明 |
|---|---|---|
| I. CLAUDE.md 为准 | 通过 | 迁移、状态、API 兼容规则按 CLAUDE.md 与 constitution 指向的文件 |
| II. 不写 UI 单测 | 通过 | 计算、完整性、引用键、显示键选择进 Go / core node 测试；界面进 `manual-ui-todo.md` |
| III. 模块边界 | 通过（Q1=A 时） | Go 代码全在 `feedback-learning`；`modules` 不改；其他模块经 handler 适配器；`adapters` 追加待批准（「主控决定」第 2 条）。views 落 `packages/views/content/feedback-learning/opdiag/`，所需上游 import 已在 `upstreamImports` 里 |
| IV. 服务端/客户端状态分离 | 通过 | 报告、标注、判断、建议、决定、提议、待办走 TanStack Query，键含 `wsId`；参数草稿、表单在组件状态；写后失效查询，**不做乐观更新**（采纳会跨模块，失败不罕见） |
| V. 无外键、并发索引、单语句 | 通过 | 二十四个迁移逐条过 R1–R6；每张表有 `workspace_id` 打头的索引 |
| VI. zod + `parseWithFallback` | 通过 | 每个新端点一份 schema、一条畸形响应用例；数值是字符串，schema 里就是 `z.string()`；`status`、`reason`、`kind` 用 `z.enum` 且有 `default` 分支 |
| VII. 复用 Multica | 通过 | `SettingsSection` / `SettingsCard` / `SettingsRow`、`Button` / `Input` / `Textarea` / `Select`、既有表格样式；不设颜色；「不可计算」用次要文字色 token |
| VIII. 范围纪律 | **需注意** | AI 判断层、经营记忆、今日工作台接入、账号级授权记录明确不做；发现的既有问题（027 读别人的表）记后续项 |
| IX. 执行器禁用 | 通过 | 不调模型；AI 判断状态恒 `pending_data`；`author_kind` 只有 `human`；守卫用例 |
| X. 打勾不是验收 | 通过 | 界面项一律「未执行」；D14-V08 只能部分覆盖，如实写 |

## Project Structure（改动白名单）

```text
server/
├── migrations/
│   └── <N>_content_opdiag_*.{up,down}.sql        # 24 个；不预留编号，合入前接当时 app-main 最大号（contract §1、§2）
├── migrations/<N>_content_topic_card_origin_key*.{up,down}.sql   # 2 个，PR 3（Q3 补充）
├── internal/content/topic-planning/
│   └── store.go + store_integration_test.go   # CreateOnce（PR 3，公开契约新增）
├── internal/content/feedback-learning/
│   ├── opdiag_contract.go        # 受控集、参数与结果类型、FieldError 复用、profileFieldKeys（PR 1）
│   ├── opdiag_inputs.go          # 输入类型、fingerprint、适配器接口、收集顺序（PR 1）
│   ├── opdiag_report.go          # 报告版本读写、inputs_changed、DiagnosisSummary（PR 1）
│   ├── opdiag_marks.go           # 作品标注读写（PR 1）
│   ├── opdiag_calc.go            # CalculateDiagnosis 骨架与 scope（PR 1）；六个维度（PR 2）
│   ├── opdiag_dimensions.go      # 六个维度的纯函数（PR 2）
│   ├── opdiag_gaps.go            # 缺口与引用键（PR 2）
│   ├── opdiag_annotations.go     # 判断、建议（PR 3）
│   ├── opdiag_decisions.go       # 决定、效果、提议、待办（PR 3）
│   └── opdiag_*_test.go          # 各自的测试与守卫；guards_test.go 登记新受控集
├── internal/handler/
│   ├── content_opdiag_reports.go / _preview.go / _decisions.go
│   ├── content_opdiag_*_test.go
│   └── workspace_delete_manifest_test.go          # 加八张表
├── pkg/db/queries/workspace_delete.sql            # 加八条 DELETE；make sqlc 产物单独提交
├── cmd/migrate/main.go                            # upstream：16 条 concurrentIndexCleanups
└── cmd/server/
    ├── router.go                                  # upstream：/api/content-operating-diagnosis 块
    └── content_opdiag_routes_test.go

packages/core/
├── content/feedback-learning/opdiag/
│   ├── contract.ts + contract.test.ts             # zod schema、畸形响应（PR 1–3 逐步加）
│   ├── display.ts + display.test.ts               # 按 status/reason/rule id 选 i18n 键；不做算术（PR 4）
│   └── queries.ts                                 # TanStack Query hooks
├── content/feedback-learning/index.ts             # 导出
├── paths/paths.ts, paths/route-icons.ts           # upstream：operatingDiagnosis（PR 4）
└── diagnostics/diagnostic-context.ts              # upstream：["operating-diagnosis"]（PR 4）

packages/views/
├── content/feedback-learning/opdiag/*.tsx         # 五个区块（PR 4）
├── layout/route-icon-components.tsx               # upstream：图标（PR 4）
└── locales/{en,zh-Hans,ja,ko}/*.json              # 四语言（PR 4）

apps/web/app/[workspaceSlug]/(dashboard)/operating-diagnosis/page.tsx   # PR 4
scripts/content-boundaries.json                    # 只追加 adapters（「主控决定」第 2 条，待批准）；modules 不动
specs/035-brand-diagnosis/manual-ui-todo.md        # PR 4 回写状态
```

清单外的改动在 PR 正文单列并说明理由。

## 四个 PR 的形状

### PR 1 —— 存储、报告版本与接口

两张表（报告版本、作品标注）+ 四个索引 = 六个迁移。`opdiag_contract.go`、`opdiag_inputs.go`、`opdiag_report.go`、`opdiag_marks.go`、`opdiag_calc.go`（骨架：`scope` + 总体缺口里的配置类缺口；维度注册表为空）。端点：contract §7 标 1 的行。

**PR 1 能生成什么**：`dimensions: []` 的报告（「只看范围」，spec US1 场景 4 本来就合法）。请求里选了任何维度 → 400 点名 `dimensions`，原因「该维度在本版本尚未提供」；PR 2 放开。这样 PR 1 生成的每个版本，在 PR 2 之后用 `opdiag-calc/1` 复算仍逐字节相同（FR-011：新增维度不改变既有结果）。

最容易漏的三处：

1. **收集顺序**（FR-081）：`Authorize` → 全部 `AccountExists` → 才读任何账号数据。用记录调用顺序的假适配器钉住；跨品牌账号 id 与非成员各一条。
2. **`inputs` 只存必要字段**（FR-042）：账号配置只存各项 `status` 与 `revision_id`，不存 `value`；摘录不存原文。反射用例扫 `inputs` 的全部字段名。
3. **`inputs_changed` 不落存储**：库里没有这一列；读版本时重新收集。

### PR 2 —— 六个维度、完整性与补录待办

零个迁移。`opdiag_dimensions.go`、`opdiag_gaps.go`、引用键；`/preview`；放开维度校验。ROI 引用（Q6）若 #266 已合入就在本 PR，否则挪到 PR 3。

计算器的测试全部不碰数据库：contract §5.10 的固定样例逐字写成用例，外加打乱顺序的性质用例（固定种子，200 组）。**`nil-vs-zero`、`two-platforms`、`cadence-unset-vs-zero`、`change` 四条是本 PR 的验收门槛**（D14-V04 的三件事：缺数据、零值、不同平台口径，外加不下结论）。

要同时守住 027 的包级守卫（spec Current State §3）：SQL 里不聚合；表现维度不点名 read 与 play（按 `Metrics` 泛型遍历）。

### PR 3 —— 人写判断与建议、采纳与拒绝

六张表 + 十二个索引 = 十八个迁移；另有 `topic-planning` 的加列与唯一索引两个迁移和公开函数 `CreateOnce`（「主控决定」第 6 条），PR 3 合计二十个迁移。`opdiag_annotations.go`、`opdiag_decisions.go`；handler 的两个写适配器。端点：contract §7 标 3 的行。

最容易漏的四处：

1. **建卡的三步与幂等键**（Q3 与补充）：第 ② 步失败必须写效果 `failed`；第 ③ 步失败（卡已建、效果没记）时建议显示「已采纳，结果未记录」，重试凭幂等键拿回同一张卡——用测试钩子在 ② 与 ③ 之间注入失败，钉住「重试后恰好一张卡」；并发两次重试同样恰好一张。
2. **拒绝不写**（FR-061）：拒绝路径不经任何写适配器；用例数调用次数与所有 `content_opdiag_*` 表行数。
3. **提议确认的版本比较**（Q8）：先读后写的顺序与 409；`SetProfile` 组装时其余十项原样沿用（用例比对新版本与旧版本逐项）。
4. **一个修订一个决定**：唯一索引 `content_opdiag_decision_suggestion_idx` 是最后一道防线；并发两次提交用例，恰好一个成功，另一个 409。

规模偏大：若审查不便，可拆成 3a（判断、建议、拒绝、待办、提议的本模块部分，本模块十八个迁移全在 3a）与 3b（建卡与提议确认两条跨模块路径，含 `topic-planning` 的两个迁移与 `CreateOnce`）。拆不拆由主控在派单时定。

### PR 4 —— 页面

`/{workspaceSlug}/operating-diagnosis` 五个区块：生成诊断（参数表单：范围、窗口、维度勾选与各维度参数、可选 ROI 引用）、报告（范围、每节每维度、缺口 / 补录待办、`inputs_changed` 提示、版本列表、AI 判断占位）、作品标注（窗口内作品逐条标支柱与一致性）、建议与决定（判断、替代解释、局限、建议、采纳 / 拒绝、效果与重试）、待办与提议（待办列表与状态、提议的「当前值 → 提议值」对照与确认 / 放弃）。三处上游路由登记。四语言。`manual-ui-todo.md` 回写。

**依赖**：PR 2 依赖 PR 1；PR 3 依赖 PR 2（判断的 `evidence_refs` 要校验结果里的引用键）；PR 4 依赖 PR 1～3。

## 上游改动（每个 PR 单独一个 `upstream:` 提交，PR 正文单列一节）

| 文件 | PR | 改什么 |
|---|---|---|
| `server/cmd/server/router.go` | 1–3 | `/api/content-operating-diagnosis` 块，各 PR 追加自己的路由；新块前空行 + 一行注释 |
| `server/cmd/migrate/main.go` | 1、3 | `concurrentIndexCleanups` 追加本 PR 的索引，自成一块 |
| `packages/core/paths/paths.ts` | 4 | `operatingDiagnosis: () => \`${ws}/operating-diagnosis\`` |
| `packages/core/paths/route-icons.ts` | 4 | `operatingDiagnosis: { segment: "operating-diagnosis", icon: "Stethoscope", navKey: "operating_diagnosis" }`；`RouteIconName` 联合类型加 `"Stethoscope"`（开发诊断用的是 `Activity`，不共用） |
| `packages/views/layout/route-icon-components.tsx` | 4 | 引入并登记 lucide 的 `Stethoscope`（该文件的 `Record<RouteIconName, LucideIcon>` 漏登记会编译失败） |
| `packages/core/diagnostics/diagnostic-context.ts` | 4 | `["operating-diagnosis"]`（这是开发诊断给**页面操作**记上下文，不是把经营诊断接进开发诊断面板，FR-091） |

每个都要 `git diff -w` 只有新增行。

## 验证

每个 PR 本地至少：

- `(cd server && DATABASE_URL='postgres://none:none@127.0.0.1:1/none?sslmode=disable' go test ./cmd/migrate ./internal/migrations)` ——**必须带这个假库地址**：上游 migrate 测试不设时默认连 `localhost:5432/multica`，会连到本机真库。
- `(cd server && go test ./internal/content/feedback-learning/)`
- `(cd server && go test ./internal/handler/ -run 'ContentOpDiag|WorkspaceDelet')`
- `(cd server && go test ./cmd/server/ -run 'ContentOpDiag')`
- `pnpm typecheck --force`、`pnpm check:content-boundaries`、`pnpm check:diagnostics-contract`
- `pnpm --filter @multica/core exec vitest run content/feedback-learning/opdiag`

带库的两套（`scripts/test-go-db.sh --suite handler`、`--suite cmd-server`）与全量检查以**远程验收**为准：主控在构建服务器上跑 `~/loretide-ci/lt-verify.sh <branch> all`。本地没跑的检查在 PR 正文记「未执行」，不记通过；带库用例报 PASS / SKIP 计数，不以退出码为证据（记忆：缓存命中的 typecheck 不是证据，要 `--force`）。

## 已知边界

1. **一致性与覆盖全靠人标注**（Q2=A）。没人标就只有「未检查 / 未标注」；这是执行器禁用时唯一诚实的做法。
2. **建卡与记效果之间有窗口，由重试收敛**（Q3=A 与补充）。窗口里失败时卡已建而效果未记，建议显示「已采纳，结果未记录」；重试凭幂等键拿回同一张卡，不会多建。窗口本身仍在（卡不会自动出现在效果里，要有人点重试）；彻底消除要 `topic-planning` 暴露接受外部事务的函数（Q3=B，不做）。
3. **提议确认有读写竞态**（Q8=A）。读当前 `revision_id` 与 `SetProfile` 之间，别人改了配置，本卡看不到；结果是别人的修改被本次确认覆盖成「当前值 + 提议值」。彻底消除要给 `SetProfile` 加 `base_revision`（后续项）。
4. **账号归属只走选题卡**（FR-026，主控已接受 2026-09-25）。历史导入的作品没有选题卡，全部进「账号未知」，报告里列为具名局限 `scope.historical_import_account_unknown`；需要的话后续卡可以给作品标注加 `kind = account`，但那是人工补录，不是推断。
5. **采样时点不一致**（Q7=A）。老发布记录的「最后一次采样」通常比新的晚，均值之差会带上这个偏差；局限说明固定写出，不做校正。
6. **`inputs` 存副本**会让报告版本行变大（每月几百条输入，约几百 KB 的 jsonb），换来的是复算不依赖原表。
7. **D14-V08 只能部分自动化**（「主控决定」第 4 条）。

## 发现的既有问题（后续项，不在本卡修；主控 2026-09-25 确认不在本批）

- `feedback-learning/metric.go:208` `PendingRegistrations` 直接读 `review-delivery` 的表 `content_publication_record`（违反文档 12 §4 第 1 条）。
- `feedback-learning` 的包注释写着「It aggregates nothing」，034 加入 ROI 计算后已不准确（守卫只管 SQL，注释管的是整个包）；本卡放进来后更不准确。建议改注释，不在本卡改。
