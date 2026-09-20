# Implementation Plan: §3.3 导入少量历史资产

**Spec**: [spec.md](./spec.md)（#213） · **Contract**: [contracts/historical-import.md](./contracts/historical-import.md) · **Issue**: #210
**裁决**: 主控 2026-09-21，PR #213 评论。六条 clarify 全部采纳推荐值 + 三条附加约束。

## 拆成两个 PR

| PR | 范围 | 不含 |
|---|---|---|
| **PR 1 存储与接口** | work-editor 的 `imported` 动作、`topic_card_id=''`、`historical_import` 列；review-delivery 的 `version_id`、`historical_import` 列；027 反查顺序；按卡聚合读路径的排除与负例；`packages/core/content/` 契约 | 页面 |
| **PR 2 页面** | 导入向导（四步、幂等、失败可从该步重试）；「暂无个人表现数据」两处 | 存储 |

PR 2 依赖 PR 1 全部合入。

## PR 1 的形状

### 不新建的东西，先说清楚

**不新建模块**（裁决 Q2 = A）。`scripts/content-boundaries.json` 的 `modules` 依赖表**逐字节不变**（SC-014）。**不新建表**、**不新建端点**。本 PR 全部是对两个已落地模块的加列、放宽与一处顺序调整。

`review-delivery` **不写** `content_work` / `content_artifact` / `content_artifact_version`。依赖表允许它 import work-editor，不等于允许它替 work-editor 写行 —— 这正是 Q2 = B 被否掉的理由，实施时不能从后门做回去。

### 四条迁移

号从 **532** 顺延（当前最大 531）。**若别的卡先合入占号，rebase 时整体重排**，编号与文件名一起动。

| 号 | 内容 | down |
|---|---|---|
| 532 | `content_work` 加 `historical_import boolean NOT NULL DEFAULT false` | `DROP COLUMN IF EXISTS` |
| 533 | `content_publication_record` 加 `historical_import boolean NOT NULL DEFAULT false` | 同上 |
| 534 | `content_publication_record` 加 `version_id text NOT NULL DEFAULT ''` | 同上 |
| 535 | `content_artifact_version` 的 `action` `CHECK` 改写为四值 | 改回三值 |

**R6 不适用**：本 PR 一个索引都不建，`concurrentIndexCleanups` 一条都不登记。加列用 `NOT NULL DEFAULT`，PostgreSQL 11+ 不重写表。

535 的 down 有一句要写进文件注释：**已经有 `imported` 行时它会失败，这是正确行为**。那条 down 的前提就是没导入过任何东西；让它安静通过等于留下一个数据库拒绝的约束。

### Go 改动，按模块

**work-editor**

1. `contract.go`：`ActionImported`，`var Actions` 加到四个。`Source` 不动。
2. `contract_test.go:24`：清单同步 —— 这条用例的存在意义就是不让人只改一边。
3. `version.go`：`ImportVersion`，与既有三条同构，`(source, action)` 由代码选定。
4. `store.go:173`：创建守卫去掉 `work.TopicCardID == ""`；补契约注释，指向同表 `snapshot_id` 已有的那句「A real state, not a missing value」。
5. `Work` 结构 + `workSelect` + `scanWork` + `INSERT`：`HistoricalImport`。
6. 守卫用例：`UPDATE content_work` 的 SET 段不含 `historical_import`，**且**存在写它的 `INSERT`。

**review-delivery**

7. `PublicationRecord` 与 `RecordRequest`：`VersionID`、`HistoricalImport`；`Record` 的 `INSERT` 与 `scan` 同步。既有调用方不传，得到 `''` / `false`，行为逐字不变。
8. 只插不改，所以这两列不需要额外守卫；但要有一条用例钉住**导入后存新版，`version_id` 不动**。

**handler**

9. `content_metric.go` 的 `feedbackPublications.Resolve`：先读 `version_id`，非空短路；为空走原两跳；两跳失败仍返回 `""` 且不拒绝记录。**顺序不能反**。
10. 创建作品与发布记录的 handler 透传两个新字段。

### 按卡聚合的读路径 —— 本 PR 最容易漏的一项

**先枚举，再改。** 不是照着下面两处改完就算（FR-034）。做法：grep `topic_card_id` 与 `ListWorks` 的全部调用方，逐个判定它是「按卡」还是「按工作区」。

| 读路径 | 判定 | 动作 |
|---|---|---|
| `ListWorks(..., topicCardID != "")` | 按卡 | SQL 天然不匹配 `''`，**用负例证明**，不假设 |
| `worksInProgress`（`packages/core/today/sections.ts`） | 按卡语义（026 第二区块） | `WorkLike` 加 `historicalImport`，派生函数过滤 |
| `ListWorks(..., topicCardID == "")` | 按工作区 | **不排除**（FR-036） |

每处一条负例：先造 `topic_card_id=''` 的作品，再断言它不出现。**只有正例的话，在没导入过历史作品的测试库里永远是绿的** —— 这是这条约束唯一的失效方式，也是它必须配负例的原因。

`worksInProgress` 改的是 `packages/core/today/`，它按既有惯例声明本地输入接口而不 import 内容模块；`WorkLike` 加一个字段，类型漂移由适配页的 typecheck 兜。

### `packages/core/content/` 契约

`work-editor` 与 `review-delivery` 的 zod schema 各加字段，走 `parseWithFallback`，各配一条畸形响应用例。

### PR 1 的验证

- `(cd server && go test ./internal/content/... ./internal/handler/...)`
- 两套 db-suites：`scripts/test-go-db.sh --suite handler` 与 `--suite cmd-server`，**只用容器自带的 PostgreSQL，建独立库/角色，绝不指向容器外地址**
- 四条迁移在隔离 schema 里 up / down 各跑一遍；535 的 down 在有 `imported` 行时**预期失败**，单独验证这一点
- `pnpm typecheck --force`、`pnpm lint`、`pnpm test`、`pnpm check:content-boundaries`

## PR 2 的形状

### 导入向导

四步串既有端点（contract 的端点表）。**不是原子的**，所以三件事都要做出来：

1. **每步幂等**（FR-022）：重试不产生第二个对象。幂等键用「本次导入会话 id + 步号」在前端保持，已成功的步骤记下它产出的 id，重试时跳过而不是重发。
2. **停在哪一步写明**（FR-023）：界面列出四步及各自状态（已完成 / 失败 / 未开始），失败那步给重试入口。**不是一句「导入失败」**，也不是从头再来。
3. **不回滚**（FR-023a）：已建出的对象一个都不删。界面明说「已建出的作品可以照常打开和编辑，也可以稍后补一条发布记录」。删除是运营者的决定。

必填项（FR-006）：正文、渠道、发布时间、平台账号、链接或平台内容 id。缺链接的拒绝由 025 既有校验给出并指名 `page_url_or_content_id`（SC-004）—— 前端**不重复实现**这条规则，只把后端指名的字段高亮。

标题缺省取正文可读前缀（FR-016），**不编概括**。

### 「暂无个人表现数据」

两处（FR-025）：账号页、今日工作台「值得写的选题」区块说明行。条件是**指标行数为 0**，派生、不落存储（FR-027）。工作台那条与既有的「候选自动生成暂不可用（EP-04c 未落地）」**并列**，不取代 —— 两句话说的是两件事。

### PR 2 的规矩

- 只挂既有 Multica 组件，不手搭页面控件与布局（宪法 VII / `docs/development/design/README.md`）
- 四语言
- **无 UI 单测**（宪法 II）；非 UI 逻辑（幂等键、步骤状态机、缺省标题的取法、「暂无个人表现数据」的判定）写 node 测试
- 界面项写 `specs/031-historical-import/manual-ui-todo.md` 并在 PR 正文列手验 Todo
- 新页面登记进 `scripts/content-boundaries.json` 的 **`adapters`**（`modules` 依赖表仍然不动）

## 已知边界

1. **导入不是原子的**。裁决的代价，不是疏漏。FR-022 / FR-023 / FR-023a 把它变成看得见、重试得了、不会被系统擅自删掉的状态。
2. **「按卡聚合」的语义靠每个读路径自己维护**。数据库挡不住：没有外键，也没有 `CHECK` 能表达「这个列表不要空卡的作品」。FR-034 ~ FR-036 是一份要重新枚举的清单加一组负例。
3. **四渠道之外的平台记不下来**（FR-031 不扩 `Channel`）。
4. **导入的每一条都会立刻出现在今日工作台第五项**（FR-028，本卡不改 026 判定）。导二十条会让那个区块变长；解法是每条可辨认，不是悄悄过滤。
