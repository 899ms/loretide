---
description: "Implementation plan for 034 feedback-learning — cost, lead, deal and ROI review (BO-06 / R-061)"
---

# Implementation Plan: 成本、线索、成交与 ROI 复盘（034）

**Spec**: [spec.md](./spec.md) ｜ **Contract**: [contracts/roi-review.md](./contracts/roi-review.md) ｜ **Tasks**: [tasks.md](./tasks.md)

**Status**: 已裁决（主控 2026-09-25，PR #255 评论）。D1–D9 已写入；Q1～Q7 采纳推荐值；`modules` 依赖表不改，导入幂等在模块内自建。**十张表、二十个索引、三十个迁移，拆五个 PR。**

## 照抄什么，不发明什么

六处照抄既有做法：

1. **写事务第一句取删除栅栏、同事务写审计** 照 `feedback-learning/store.go:109` 的 `begin()`。
2. **越权与「不存在」同形** 照 `handler/content_metric.go` 的 `feedbackScope`。
3. **不 import 别的模块，模块定义小接口、适配器回答** 照 `feedbackPublications`。本卡新增三个：`Accounts`（调 `ipprofile.Service.Get`）、`Works`（调 `workeditor.Store.GetWork`）、沿用 `Publications`。
4. **受控集 Go 为准 + `CHECK` 兜底 + 「恰好 N 项」用例** 照 027 `contract.go`。
5. **粘贴 CSV 在前端解析、服务端全有或全无、点名第几行** 照 027 `csv.ts` 与 `ImportContentMetrics`。
6. **`Idempotency-Key` 占位、指纹、同键同输入重放、同键异输入 409** 照 031 PR 1（#231）的 `idempotency` 模块与 `content_import_idempotency.go`——**照它的形状在本模块里重写，不 import 它**（主控否决了改 `modules` 依赖表）。占位表 `content_roi_import_claim` 只插不改，与 031 的一处差别见 contract §1.9。

两处**不照抄**：

- **不是只插不改的「更正即再录一条」，而是修订制**。027 的指标可以「读端取最新」，是因为一条指标没有身份；这里的成本、线索、成交有稳定 id，报告要引用「这笔成本的第几版」，所以每行带 `(id, revision)`，更正是同 id 的下一个修订号。仍然只插不改。
- **计算结果存下来**。027 刻意没有报告表（那时产生不出内容）；本卡的报告内容全部由确定性计算产生，而原文要求「计算结果……分开保存」「后续调整产生新的报告输入版本」，所以有报告版本表。AI 解释仍然没有表（FR-061）。

## Technical Context

**Language/Version**: Go 1.26（`server/`）、TypeScript 5 strict（`packages/core`、`packages/views`）

**Storage**: PostgreSQL。十张新表（contract §1）+ 二十个 `CONCURRENTLY` 索引（§2），每个索引单独一个迁移文件、单条语句。金额 `bigint` 最小单位，汇率与比率在 Go 里用 `math/big.Rat`，JSON 里是字符串。

**Testing**: `go test ./internal/content/feedback-learning/ ./internal/handler/ ./internal/migrations/ ./cmd/migrate/ ./cmd/server/`；带库的套件按 `docs/development/testing-database-suites.md` 用 `scripts/test-go-db.sh --suite handler` 与 `--suite cmd-server`；`packages/core/content/feedback-learning/*.test.ts`（node 环境）。**无 UI 单测。**

**Constraints**: 不调模型、不起执行器、不发外部请求、不用浮点、不收集客户个人信息、不自动补来源、不自动换汇。页面只用既有 Multica 组件。

**Scale**: 一个品牌每月几十到几百条成本/线索/成交，报告每月几份。计算器一次读窗口内全部记录进内存即可，不需要分页计算或物化视图。

## Constitution Check

| 原则 | 状态 | 说明 |
|---|---|---|
| I. CLAUDE.md 为准 | 通过 | 迁移、状态、API 兼容规则按 CLAUDE.md 与 constitution 指向的文件 |
| II. 不写 UI 单测 | 通过 | 计算、分摊、去重、CSV 解析、显示字符串选择进 Go / core node 测试；界面进 `manual-ui-todo.md` |
| III. 模块边界 | 通过 | Go 代码全在 `feedback-learning`，`modules` 依赖表**不改**：账号、作品、发布记录经适配器接口，导入幂等在模块内自建（不 import `idempotency`）。`adapters` 追加已获批准（见「主控决定」）。views 落 `packages/views/content/feedback-learning/roi/`，所需上游 import 已在 `upstreamImports` 里 |
| IV. 服务端/客户端状态分离 | 通过 | 记录与报告走 TanStack Query，键含 `wsId`；表单草稿、报告参数草稿在组件状态；写后失效查询，**不做乐观更新**（金额类写入失败不罕见，回滚也不简单） |
| V. 无外键、并发索引、单语句 | 通过 | 三十个迁移逐条过 R1–R6；每张表有 `workspace_id` 打头的索引 |
| VI. zod + `parseWithFallback` | 通过 | 每个新端点一份 schema、一条畸形响应用例；金额与比率是字符串，schema 里就是 `z.string()`，前端不转数字 |
| VII. 复用 Multica | 通过 | `SettingsSection` / `SettingsCard` / `SettingsRow`、`Button` / `Input` / `Textarea` / `Select`、既有表格样式；不设颜色，「不可计算」用次要文字色 token |
| VIII. 范围纪律 | **需注意** | AI 解释与采纳明确不做（D1）；营销节点关联不做（Q4）；品牌级类别/阶段配置不做（Q5）。发现的既有问题记后续项 |
| IX. 执行器禁用 | 通过 | 不调模型；AI 解释状态恒 `pending_data`；守卫用例 |
| X. 打勾不是验收 | 通过 | 界面项一律「未执行」；D14-V15 只能部分覆盖，如实写 |

## 主控决定（2026-09-25，PR #255 评论）

本规格 PR **不改** `scripts/content-boundaries.json`（D9）。

### 1. `modules` 依赖表：不改（否决）

原提议给 `feedback-learning` 加 `idempotency` 依赖，**已被否决**：用户禁止改 `modules` 依赖表。因此：

- PR 3 的导入幂等**完全在 `feedback-learning` 内**实现：自建占位表 `content_roi_import_claim`（contract §1.9）+ 一个唯一并发索引，指纹计算、占位、重放写在 `roi_import.go` 里，照 031 的形状重写。
- 模块源码 MUST NOT import `server/internal/content/idempotency`；一条守卫用例扫 import（contract §8）。
- 与 031 的差别只有一处：本模块所有表只插不改，所以占位行插入时就带上 `import_batch_id`，重放从导入批次行重建响应，不需要 031 那一步 `UPDATE ... SET completed=true`。
- 代价：幂等机制在仓库里有两份（031 的通用模块、本模块的专用表）。以后若允许改依赖表，可以把本表换成通用模块，接口（`Idempotency-Key` 请求头、409 语义）不变。

### 2. `adapters` 追加：已批准

每个实施 PR 在自己的改动里追加自己的那一条，**本规格 PR 不改该文件**：

| PR | 条目 |
|---|---|
| 1 | `server/internal/handler/content_roi_records.go` |
| 2 | `server/internal/handler/content_roi_preview.go` |
| 3 | `server/internal/handler/content_roi_import.go` |
| 4 | `server/internal/handler/content_roi_report.go` |
| 5 | `apps/web/app/[workspaceSlug]/(dashboard)/roi-review/page.tsx` |

### 3. 迁移编号：不预留

每个实施 PR 在合入前，把自己的迁移改号为紧接当时 `app-main` 最大号之后的连续号（文件名、`concurrentIndexCleanups` 的键一起改）。033 与 034 的实施谁先合入谁先占号。

### 4. D14-V15：接受部分验收

本卡覆盖「录入 → 复盘 → 查看来历」；AI 建议的采纳/拒绝作为后续卡。

## Project Structure（改动白名单）

```text
server/
├── migrations/
│   └── <N>_content_roi_*.{up,down}.sql          # 30 个；不预留编号，合入前接当时 app-main 最大号（contract §1、§2）
├── internal/content/feedback-learning/
│   ├── roi_contract.go         # 受控集、币种表、FieldError 复用、实体结构（PR 1）
│   ├── roi_money.go            # 金额字符串解析、舍入、汇率换算（PR 1；换算 PR 2 用）
│   ├── roi_dedupe.go           # 去重键（PR 1）
│   ├── roi_records.go          # 成本/线索/触点/成交/调整的读写（PR 1）
│   ├── roi_allocate.go         # 最大余数法，分摊与多触点共用（PR 2）
│   ├── roi_attribution.go      # 归因判断读写（PR 2）
│   ├── roi_calc.go             # 纯函数计算器（PR 2）
│   ├── roi_import.go           # 导入、模块内幂等占位与重放（PR 3）
│   ├── roi_report.go           # 报告版本、ReportSummary（PR 4）
│   └── roi_*_test.go           # 各自的测试与守卫
├── internal/handler/
│   ├── content_roi_records.go / _preview.go / _import.go / _report.go
│   ├── content_roi_*_test.go
│   ├── workspace_delete_manifest_test.go        # 加十张表
├── pkg/db/queries/workspace_delete.sql          # 加十条 DELETE；make sqlc 产物单独提交
├── cmd/migrate/main.go                          # upstream：19 条 concurrentIndexCleanups
└── cmd/server/
    ├── router.go                                # upstream：/api/content-roi 块
    └── content_roi_routes_test.go

packages/core/
├── content/feedback-learning/roi/
│   ├── contract.ts + contract.test.ts           # zod schema、畸形响应（PR 1–4 逐步加）
│   ├── csv.ts + csv.test.ts                     # 导入粘贴解析（PR 3）
│   ├── display.ts + display.test.ts             # 选哪个 i18n 键显示；不做算术（PR 5）
│   └── queries.ts                               # TanStack Query hooks
├── content/feedback-learning/index.ts           # 导出
├── package.json                                 # 如需新增子路径导出
├── paths/paths.ts, paths/route-icons.ts         # upstream：roiReview（PR 5）
└── diagnostics/diagnostic-context.ts            # upstream：["roi-review"]（PR 5）

packages/views/
├── content/feedback-learning/roi/*.tsx          # 五个区块（PR 5）
├── layout/route-icon-components.tsx             # upstream：Receipt 图标（PR 5）
└── locales/{en,zh-Hans,ja,ko}/*.json            # 四语言（PR 5）

apps/web/app/[workspaceSlug]/(dashboard)/roi-review/page.tsx   # PR 5
scripts/content-boundaries.json                  # 只追加 adapters（「主控决定」第 2 条）；modules 不动
specs/034-roi-review/manual-ui-todo.md           # PR 5 回写状态
```

清单外的改动在 PR 正文单列并说明理由。

## 五个 PR 的形状

### PR 1 —— 记录存储与接口

五张表（成本、线索、触点、成交、调整）+ 十三个索引 = 十八个迁移。金额解析、币种表、去重键、修订制写路径、`base_revision` 冲突、疑似重复 409、FR-019 字段组合校验、线索合并。端点见 contract §6 标 1 的行。core 的 zod schema 与畸形响应用例同 PR 交付。**不含**分摊（请求体带 `allocations` 返回 400）、归因判断、计算。

最容易漏的两处：

1. **修订号并发**：两个人同时对同一成本提交 `base_revision=1`。先到者写 2；后到者在同一事务里读到最新是 2 → 409。万一两个事务都读到 1，唯一索引 `(workspace_id, cost_id, revision)` 让后提交者失败，同样映射为 409。**要有一条真实 DB 的并发用例**，只测第一层会漏掉第二层。
2. **合并后的读取**：`GET /leads/{leadId}` 对被合并线索返回它自己的修订并标明 `merged_into`；对目标线索返回自己的触点 **加上** 合并进来的触点，按 `touch_id` 去重。

### PR 2 —— 分摊、归因与确定性计算

两张表（分摊、归因判断）+ 两个索引 = 四个迁移。`roi_allocate.go`（最大余数法）、`roi_calc.go`（纯函数）、`roi_money.go` 的换算。`POST /preview`：读窗口内记录，组装 `ReportInput`，调计算器，**不保存**。

计算器的测试全部不碰数据库：contract §5.4 的九个固定样例逐字写成用例，外加分摊与多触点的性质用例（固定随机种子，1000 组）。**D14-V12 的两个样例是本 PR 的验收门槛**。

### PR 3 —— 导入

两张表（导入批次、导入占位）+ 三个索引 = 五个迁移。不改 `modules` 依赖表，不 import `idempotency`。

流程：前端解析粘贴文本 → `POST /imports`（`record_kind` + 行 + 每行可选的 `not_duplicate_of`；请求头可带 `Idempotency-Key`）→ 事务内：栅栏 → 生成 `import_batch_id` → 有键时写占位（contract §1.9：`INSERT ... ON CONFLICT DO NOTHING`；已被占用且指纹相同 → 从那个批次行重建原响应并返回，不再写；指纹不同 → 409）→ 逐行校验（任一失败整体回滚，占位随之回滚，点名行列）→ 逐行算去重键并查重 → 写入未命中和已确认的行 → 写导入批次 → 审计 → 提交。

响应由一个「从导入批次行构造响应」的函数产生，首次返回与重放共用它，所以两次逐字节相同。

预览是前端做的（它已有行）；「哪些行疑似重复」需要服务端判断，所以提供 `dry_run: true`：同一流程走到查重为止、不写、不占位，返回每行的判定。

### PR 4 —— 报告版本

一张表 + 两个索引 = 三个迁移。生成版本 = 与 `/preview` 同一段组装代码 + 把 `inputs`（修订内容的完整副本）、`params`、`calc_version`、`result` 写进一行。`inputs_changed` 读时派生。`ReportSummary` 公开读接口与它的字段扫描用例。复算用例：每个固定样例存成版本 → 从库读回 → 用 `inputs` 复算 → 与 `result` 逐字节相同。

### PR 5 —— 页面

`/{workspaceSlug}/roi-review` 五个区块；导入的粘贴、预览、确认重复；报告的参数表单、指标卡、来历展开、版本列表、「输入已有更新」提示、AI 解释占位。三处上游路由登记。四语言。`manual-ui-todo.md` 回写。

**依赖**：PR 2 依赖 PR 1；PR 3 依赖 PR 1；PR 4 依赖 PR 2；PR 5 依赖 PR 1–4。PR 3 与 PR 2 可以并行。

## 上游改动（每个 PR 单独一个 `upstream:` 提交，PR 正文单列一节）

| 文件 | PR | 改什么 |
|---|---|---|
| `server/cmd/server/router.go` | 1–4 | `/api/content-roi` 块，各 PR 追加自己的路由；新块前空行 + 一行注释 |
| `server/cmd/migrate/main.go` | 1–4 | `concurrentIndexCleanups` 追加本 PR 的索引，自成一块 |
| `packages/core/paths/paths.ts` | 5 | `roiReview: () => \`${ws}/roi-review\`` |
| `packages/core/paths/route-icons.ts` | 5 | `roiReview: { segment: "roi-review", icon: "Receipt", navKey: "roi_review" }`；`RouteIconName` 联合类型加 `"Receipt"` |
| `packages/views/layout/route-icon-components.tsx` | 5 | 引入并登记 lucide 的 `Receipt`（该文件的 `Record<RouteIconName, LucideIcon>` 会在漏登记时编译失败） |
| `packages/core/diagnostics/diagnostic-context.ts` | 5 | `["roi-review"]` |

每个都要 `git diff -w` 只有新增行。

## 验证

每个 PR 本地至少：

- `(cd server && go test ./internal/content/feedback-learning/ ./internal/migrations/ ./cmd/migrate/)`
- `(cd server && go test ./internal/handler/ -run 'ContentROI|WorkspaceDelet' ./cmd/server/ -run 'ContentROI')`
- `pnpm typecheck --force`、`pnpm check:content-boundaries`、`pnpm check:diagnostics-contract`
- `pnpm --filter @multica/core exec vitest run content/feedback-learning`

带库的两套（`scripts/test-go-db.sh --suite handler`、`--suite cmd-server`）与全量检查以**远程验收**为准：主控在构建服务器上跑 `~/loretide-ci/lt-verify.sh <branch> all`。本地没跑的检查在 PR 正文记「未执行」，不记通过；带库用例报 PASS / SKIP 计数，不以退出码为证据。

## 已知边界

1. **阶段是自由文本**（Q5=A，已裁定），转化率只能按「是否到达过某阶段」判断，没有阶段顺序。这条局限写进报告的「计算规则与局限」（FR-048a），不只写在这里。要有顺序，需要品牌级阶段配置（另卡）。
2. **跨期成本靠人分摊到期间**（Q6=A）。忘了分摊的季度费用会整笔落在发生月。界面在成本表单上写明这一点。
3. **汇率只在报告里**。同一笔 USD 成本在两份报告里可能按不同汇率换算——这是真实情况（不同时点的汇率），每份报告显示自己用的汇率。
4. **去重键会漏也会误报**：两笔真实不同但类别、日期、金额都相同的成本会被标疑似重复（用户确认即可）；订单号抄错一位的重复成交不会被发现。重复识别是提示，不是保证。
5. **D14-V15 只能部分验收**（主控已接受）：采纳与拒绝建议依赖 AI 解释层，本卡不建，作为后续卡。
6. **`inputs` 存修订内容副本**会让报告版本行变大（每月几百条记录，约几百 KB 的 jsonb）。换来的是复算不依赖原表；若嫌大，可改成只存 `(kind, id, revision)` 并依赖原表只插不改，代价是删除工作区之外的任何数据清理都会破坏复算。
