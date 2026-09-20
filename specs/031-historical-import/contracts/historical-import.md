# Contract: 历史资产导入（031 实施 PR 1）

面向 `server/internal/content/work-editor/`、`server/internal/content/review-delivery/` 与它们既有的端点。**不新建模块、不新建表、不新建端点**（裁决 Q2 = A）：本 PR 全部是对两个已落地模块的**加列与放宽**，外加 027 反查顺序的一处调整。页面在 PR 2。

## 本 PR 改的东西，一页看完

| # | 改什么 | 在哪 | 性质 |
|---|---|---|---|
| 1 | `Action` 加 `imported` | work-editor `contract.go` + 迁移 `CHECK` | 受控集扩张 |
| 2 | `ImportVersion` 写路径 | work-editor `version.go` | 新方法 |
| 3 | `topic_card_id` 允许 `''` | work-editor `store.go` 创建守卫 | 守卫放宽 |
| 4 | `content_work.historical_import` | 迁移 + 契约 | 新列，不可改 |
| 5 | `content_publication_record.historical_import` | 迁移 + 契约 | 新列，不可改 |
| 6 | `content_publication_record.version_id` | 迁移 + 契约 | 新列，不可改 |
| 7 | 027 反查优先读 `version_id` | `handler/content_metric.go` | 顺序调整 |
| 8 | 按卡聚合读路径排除空卡作品 | work-editor + `packages/core/today/` | 过滤 + 负例 |

## 1. 受控集：`Action` 从三值变四值

| 集合 | 本 PR 后的值 | 越界 |
|---|---|---|
| `source` | `generated`、`edited`、`adopted` | 400 |
| `action` | `saved`、`restored`、`adopted`、**`imported`** | 400 |

**`Source` 一个值都不加。** 它的注释自称「Exactly three, per SOP 7.1」，那是原文。

三处 MUST 逐字一致（spec FR-032）：

1. `server/internal/content/work-editor/contract.go` 的 `var Actions`
2. `server/internal/content/work-editor/contract_test.go:24` 的清单
3. 迁移 500 的 `CHECK (action IN (...))` —— 由新迁移 `ALTER TABLE ... DROP CONSTRAINT ... ; ALTER TABLE ... ADD CONSTRAINT ...` 改写

## 2. 写路径：`ImportVersion`

与既有三条同构 —— **`(source, action)` 由代码选定，请求体给不了**（`version.go` 现有注释：「The source and action are chosen by the code above, not by a caller」）。

```go
// ImportVersion appends the single version of a historical work.
// source stays `edited`: the body is something a person wrote, years ago.
// action is `imported` because it is not a save - nobody wrote this today.
func (s *Store) ImportVersion(ctx, workspaceID, actor, workID, artifactID string) (ArtifactVersion, error) {
    return s.appendVersion(ctx, workspaceID, actor, workID, artifactID, versionIntent{
        step: "import-version", source: SourceEdited, action: ActionImported,
    })
}
```

`restored_from` 与 `adopted_from` **都为空**（spec FR-010：不虚构版本链）。

`created_at` 由 `DEFAULT now()` 给，**是导入时刻**。没有任何参数能把它往回写（spec FR-014a）。历史的发布时间只存在于发布记录的 `published_at`。

## 3. `content_work.topic_card_id` 允许 `''`

`store.go:173` 的创建守卫从

```go
if workspaceID == "" || actor == "" || work.TopicCardID == "" {
```

去掉 `work.TopicCardID == ""` 一项。列仍是 `text NOT NULL`，**空串是值，不是 NULL** —— 语义与同表 `snapshot_id` 完全一致，那一列的注释已经写好了这句话：「A real state, not a missing value」。024 的契约注释同步补一句。

**这次放宽没有数据库能兜底的部分**：没有外键，也没有 `CHECK` 能表达「这个列表不要空卡的作品」。所以第 8 项是本 PR 的必答题，不是加分项。

## 4 / 5. `historical_import` 两列

| 表 | 列 | 类型 | 可变 |
|---|---|---|---|
| `content_work` | `historical_import` | `boolean NOT NULL DEFAULT false` | **否** |
| `content_publication_record` | `historical_import` | `boolean NOT NULL DEFAULT false` | **否** |

口径与 028 逐字一致：**布尔、创建时确定、此后不可改**。

- `content_publication_record` 本就只插不改，这一列随行写死。
- `content_work` 有一条 `UPDATE ... SET title=$3, updated_at=now()`（`RenameWork`，`store.go:301`），所以要一条守卫：**任何 `UPDATE content_work` 的 SET 段都不得出现 `historical_import`**，且同一条守卫要断言存在一个写它的 `INSERT`（少了后半条，空模块也是绿的 —— 028 的守卫是这么写的，照抄这个结构）。

发布记录**自带**这一列而不是靠 `work_id` 回查：日后做表现聚合时，这是唯一能把历史数字与新发布分开的依据（spec FR-015a）。

## 6. `content_publication_record.version_id` —— 「发布后快照」

```
version_id text NOT NULL DEFAULT ''
```

`''` 是「不知道」，与 `delivery_task_id` 的空串同构。只插不改，所以不需要额外守卫。

它就是 §3.3 的「发布后快照」：**这条发布对应的是这份正文，直接指向，中间没有审核请求与交付任务，因此也没有虚构的版本链。**

**它不随新版本移动**（spec FR-012）。导入之后有人在同一份文档上正常存一版，这一列仍指向导入的那一版 —— 因为发布记录只插不改，不需要任何代码去保证，但要有一条用例把它钉住。

`RecordRequest` 增加 `version_id` 字段；既有调用方不传，得到 `''`，行为与本 PR 之前逐字相同。

## 7. 027 的反查顺序

`server/internal/handler/content_metric.go` 的 `feedbackPublications.Resolve`：

```
先读发布记录的 version_id
  非空 → 直接返回它（短路）
  为空 → 走原来的两跳：发布记录 → 交付任务 → 审核请求
两跳失败 → 仍然返回 ""，仍然不拒绝记录
```

**顺序不能反。** 一条既有交付任务又有直接指向的记录，权威答案是直接指向；先走两跳会答出另一个版本。

027 的契约（`feedbacklearning.Publications` 接口、`ManualMetric.VersionID` 的语义、「解析不出来答 `""`、从不拒绝记录」）**一个字都不改**（spec FR-029）。改的是适配器怎么回答它。

## 8. 按卡聚合的读路径

**必须重新枚举一遍**，不是照着下面这两处改完就算（spec FR-034）。已知的两处：

| 读路径 | 在哪 | 怎么排除 |
|---|---|---|
| 选题卡下的作品列表 | `ListWorks(ctx, ws, actor, topicCardID)`，`topicCardID != ""` 时 | 现有 SQL `($2='' OR topic_card_id=$2)` 在 `$2 != ''` 时天然不匹配 `''` —— **要用负例证明它成立，不是假设** |
| 今日工作台「正在推进的作品」 | `packages/core/today/sections.ts` 的 `worksInProgress` | `WorkLike` 加 `historicalImport`，派生函数过滤掉它 |

**不排除**的读路径（spec FR-036）：全工作区的作品列表（`ListWorks` 传 `topicCardID = ""`）。历史作品是这个工作区真实存在的作品。

每一处各配一条**负例**（spec FR-035）：先造一件 `topic_card_id = ''` 的作品，再断言它**不出现**。只有正例不够 —— 在一个从没导入过历史作品的测试库里，正例永远是绿的。

## 端点：一个都不新增

导入由 web 适配器按序调用既有端点（裁决 Q2 = A）：

| 步 | 端点 | 产出 |
|---|---|---|
| 1 | `POST /api/content-works` | 作品（`topic_card_id=''`、`historical_import=true`） |
| 2 | `POST /api/content-works/{id}/artifacts` | 文档（正文进编辑副本） |
| 3 | 该文档的导入版本写入口 | 唯一的版本（`edited` / `imported`） |
| 4 | `POST /api/content-publications` | 发布记录（`reported_published`、`delivery_task_id=''`、`version_match=unknown`、`version_id=` 第 3 步的版本、`historical_import=true`） |

历史反馈另走 027 既有的 `POST /api/content-metrics` 与摘录入口，本 PR 不碰。

**顺序被 025 锁死**：`Record` 用 `ResolveVersion` 反查 `work_id`，artifact 不存在就是 `ErrNotFound`。

### 幂等（spec FR-022）

每一步用同一份输入重试 MUST NOT 产生第二个对象。幂等键与实现落点在 plan.md；契约层面的要求是：**重试后四类对象的条数各自不变**。

### 既有限制，本 PR 不动

- `reported_published` 要求 `page_url_or_content_id` 非空（`states.go:183`）。导入必须给链接或平台内容 id —— **025 已有的，不是本卡加的**。
- `Channel` 仍是四个。不扩（spec FR-031）。
- 正文上限仍是 `MaxBodyRunes = 200000`，超了拒绝、**不截断**。

## 迁移

下一个可用号 **532**。R5：建表不内联 `PRIMARY KEY` / `UNIQUE`（本 PR 不建表）。R6：每个**建索引**的 up 登记进 `server/cmd/migrate/main.go` 的 `concurrentIndexCleanups`，名字逐字节一致 —— **本 PR 全是 `ALTER TABLE ADD COLUMN` 与 `CHECK` 改写，一个索引都不建，所以一条都不登记**。每个文件单语句。

| 号 | 内容 |
|---|---|
| 532 | `content_work` 加 `historical_import` |
| 533 | `content_publication_record` 加 `historical_import` |
| 534 | `content_publication_record` 加 `version_id` |
| 535 | `content_artifact_version` 的 `action` `CHECK` 改写为四值 |

`ADD COLUMN ... NOT NULL DEFAULT` 在 PostgreSQL 11+ 不重写表。`down` 各自是 `DROP COLUMN IF EXISTS` / 把 `CHECK` 改回三值 —— **改回三值的 down 会在已有 `imported` 行时失败**，这是正确行为：那条 down 的前提就是没有导入过任何东西，让它安静通过等于留下一个数据库拒绝的约束。这一点写进 down 文件的注释。

## 用例

- **受控集**：`action` 四值各自可写，第五个值 400 且指名 `action`。
- **写路径**：`ImportVersion` 产出 `edited` / `imported`，`restored_from` 与 `adopted_from` 皆空；请求体无法指定动作。
- **`created_at`**：导入版本的 `created_at` 在导入时刻的同一分钟内（FR-014a）。
- **空卡作品**：`CreateWork` 接受 `topic_card_id=''`；**负例** —— 它不出现在按卡聚合的读路径里（每处一条）；**正例** —— 它出现在全工作区列表里。
- **守卫**：任何 `UPDATE content_work` 的 SET 段不含 `historical_import`，且存在写它的 `INSERT`。
- **快照不动**：导入后在同一文档存一版新的，发布记录的 `version_id` 不变。
- **027 反查**：带 `version_id` 的记录短路返回它；`version_id` 空但有交付任务的记录，结果与本 PR 之前逐字相同；两者都不拒绝记录。
- **第 12 步**：`/api/content-works/{id}/...` 与本 PR 触及的每个带路径参数的端点，各有一条穿过真实中间件且**路径参数值与上下文值不同**的用例。
- **恶意响应**：`packages/core/content/` 的每个改动过的 schema 各有一条畸形响应用例（`parseWithFallback`）。
