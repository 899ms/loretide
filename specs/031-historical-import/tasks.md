# Tasks: §3.3 导入少量历史资产

**Spec**: [spec.md](./spec.md) · **Plan**: [plan.md](./plan.md) · **Contract**: [contracts/historical-import.md](./contracts/historical-import.md) · **Issue**: #210

两个 PR。PR 2 依赖 PR 1 全部合入。`[P]` 表示与相邻任务无依赖、可并行。

---

# PR 1 — 存储与接口

## Phase 1: 基线

- [x] T001 核对迁移最大号（规格撰写时 531 → 从 532 起）。**若别的卡先合入占号，rebase 时编号与文件名整体重排**
- [x] T002 核对 `scripts/content-boundaries.json`：记下 `modules` 依赖表的当前内容，PR 结束时 diff 必须为空（SC-014）
- [x] T003 枚举**全部**按选题卡聚合的读路径：grep `topic_card_id` 与 `ListWorks` 的所有调用方，逐个判定「按卡」还是「按工作区」，把结论写进 PR 正文。**这一步是 FR-034 的前提，不是可选的调研**

## Phase 2: 迁移

- [x] T004 532 `content_work` 加 `historical_import boolean NOT NULL DEFAULT false`；down 为 `DROP COLUMN IF EXISTS`
- [x] T005 [P] 533 `content_publication_record` 加 `historical_import boolean NOT NULL DEFAULT false`
- [x] T006 [P] 534 `content_publication_record` 加 `version_id text NOT NULL DEFAULT ''`
- [x] T007 535 `content_artifact_version` 的 `action` `CHECK` 改写为四值（`DROP CONSTRAINT` + `ADD CONSTRAINT`，单语句单文件）
- [x] T008 535 的 down 写回三值，并在文件注释里写明：**已有 `imported` 行时这条 down 会失败，这是正确行为**——让它安静通过等于留下一个数据库拒绝的约束
- [x] T009 **不登记 `concurrentIndexCleanups`**：本 PR 一个索引都不建（R6 的适用前提是建索引）。在 PR 正文写明这是刻意的，不是漏登记
- [x] T010 容器自带 PG 验证四条 up / down（**建独立库与角色，绝不指向容器外地址**）；T008 那条失败单独验证
- [x] T011 确认 `workspace_delete.sql` 与删除清单用例**不需要改**：本 PR 不建表，只加列

## Phase 3: work-editor（守卫先写）

- [x] T012 **先写守卫**：任何 `UPDATE content_work` 的 **SET 段**不含 `historical_import`，**且**存在一条写它的 `INSERT`。少了后半条，空实现也是绿的（照抄 028 `guards_test.go` 的结构）。SET 段的切法要排除 `WHERE` 与 `RETURNING`
- [x] T013 `contract.go`：`ActionImported`，`var Actions` 四值。**`Source` 一个值都不加**（注释自称「Exactly three, per SOP 7.1」）
- [x] T014 `contract_test.go:24` 的清单同步为四值 —— 这条用例存在的意义就是不让人只改一边
- [x] T015 `version.go` 加 `ImportVersion`：与既有三条同构，`source=edited` / `action=imported` 由代码选定，**请求体给不了**
- [x] T016 用例：`ImportVersion` 产出 `edited` / `imported`，`restored_from` 与 `adopted_from` 皆空（FR-010 不虚构版本链）
- [x] T017 用例：导入版本的 `created_at` 落在导入时刻的同一分钟内；**没有任何路径能把它写成过去**（FR-014a）
- [x] T018 `store.go:173` 创建守卫去掉 `work.TopicCardID == ""`；补契约注释，指向同表 `snapshot_id` 已有的「A real state, not a missing value」
- [x] T019 `Work` 结构 + `workSelect` + `scanWork` + `INSERT` 加 `HistoricalImport`
- [x] T020 用例：`CreateWork` 接受 `topic_card_id=''` 并写入 `historical_import=true`

## Phase 4: 按卡聚合的排除（负例先写）

- [x] T021 **先写负例**：造一件 `topic_card_id=''` 的作品，断言它**不出现**在 `ListWorks(..., topicCardID != "")` 的结果里。现有 SQL `($2='' OR topic_card_id=$2)` 看上去天然成立——**用负例证明，不假设**
- [x] T022 **先写负例**：同一件作品**不出现**在今日工作台「正在推进的作品」区块的派生结果里
- [x] T023 **正例**：同一件作品**出现**在全工作区作品列表（`ListWorks(..., "")`）里。排除的是「按卡」的语义，不是作品本身（FR-036）
- [x] T024 `packages/core/today/types.ts` 的 `WorkLike` 加 `historicalImport`；`sections.ts` 的 `worksInProgress` 过滤掉它。node 测试
- [x] T025 T003 枚举出的**其余每一处**按卡读路径，各加过滤与一条负例。**一处都不能只靠正例**

## Phase 5: review-delivery

- [x] T026 `PublicationRecord` 与 `RecordRequest` 加 `VersionID` 与 `HistoricalImport`；`Record` 的 `INSERT` 与 `scan` 同步
- [x] T027 用例：既有调用方不传新字段时，行为与本 PR 之前**逐字相同**（`''` / `false`）
- [x] T028 用例：导入之后在同一份文档上正常存一版新的，发布记录的 `version_id` **不变**（FR-012，发布后快照不随新版本移动）
- [x] T029 用例：`reported_published` 缺 `page_url_or_content_id` 仍被拒绝并指名该字段（025 既有约束，本 PR 没碰它）

## Phase 6: 027 的反查顺序

- [x] T030 `handler/content_metric.go` 的 `feedbackPublications.Resolve`：先读 `version_id`，非空短路；为空走原两跳。**顺序不能反**——一条既有交付任务又有直接指向的记录，权威答案是直接指向
- [x] T031 用例：带 `version_id` 的记录短路返回它
- [x] T032 用例：`version_id` 为空但有交付任务的记录，反查结果与本 PR 之前**逐字相同**
- [x] T033 用例：两跳都失败仍返回 `""` 且**不拒绝记录**（027 既有语义，FR-011b 要求它不被改掉）
- [x] T034 确认 `feedbacklearning` 包**一个字都没改**（FR-029）

## Phase 7: handler 与前端契约

- [x] T035 创建作品与创建发布记录的 handler 透传两个新字段；`historical_import` 只在创建时读，**任何改路径都不读它**
- [x] T036 用例：尝试把已存在对象的 `historical_import` 改掉 → 被拒绝并指名该字段（FR-018 / SC-007）
- [x] T037 `packages/core/content/work-editor` 与 `review-delivery` 的 zod schema 加字段，走 `parseWithFallback`
- [x] T038 [P] 每个改动过的 schema 各一条畸形响应用例
- [x] T039 第 12 步：本 PR 触及的每个带路径参数的端点，各一条穿过真实中间件且**路径参数值与上下文值不同**的用例

## Phase 8: 验证

- [x] T040 `(cd server && go test ./internal/content/... ./internal/handler/...)`
- [x] T041 两套 db-suites：`scripts/test-go-db.sh --suite handler` 与 `--suite cmd-server`
- [x] T042 `pnpm typecheck --force`、`pnpm lint`、`pnpm test`
- [x] T043 `pnpm check:content-boundaries`；并 diff `scripts/content-boundaries.json` 的 `modules`，**必须为空**（SC-014）
- [x] T044 PR 正文：T003 的枚举结论、T009 为何不登记 `concurrentIndexCleanups`、以及**上游改动一节**（若有；本 PR 预期为无）

## PR 1 实施记录（与计划不同的地方）

计划写在实施之前，几处实际情况与它不同。记在这里，而不是把任务描述改成事后正确的样子。

1. **T003 的枚举结果：按卡聚合的读路径只有两处，不是「已知两处 + 未知若干」。** 全仓 grep `topic_card_id` 与 `ListWorks` 的调用方，结果是：`ListWorks(topicCardID != "")`（唯一的服务端按卡读）、`worksInProgress`（026 第二区块）。`ListWorks(topicCardID == "")` 是按工作区，不排除。前端 `useContentWorks(wsId, topicCardId)` 只是把参数透传给前两者。T025 因此没有「其余每一处」要改——**这是枚举的结论，不是跳过**。

2. **`worksInProgress` 按 `historicalImport` 排除，不按空的 `topicCardId`。** 两者在导入产生的作品上恰好同时成立，但它们是两件事：一张选题卡被删掉会留下一件没有卡、却确实还在写的作品。多了一条用例钉住这个区别（「keeps a work that has no topic card but was not imported」）。

3. **`WorkLike.historicalImport` 是可选字段。** 没有 532 迁移的后端不发这个字段，可选让那种调用方仍然能通过类型检查，而 `!== true` 让「没有」读成「不是导入」——如果读成 `true`，那个部署上每一件作品都会从第二区块消失。

4. **`ImportVersion` 之外还加了一个端点**：`POST /api/content-works/{id}/artifacts/{artifactId}/versions/import`。契约里写的是「该文档的版本写入口」，没说是新端点还是旧端点加标志。选了新端点，理由与 027 的两个指标端点分开是同一条：**调用了哪个入口，就是服务端记下的来历**；一个能自己声明来历的调用方不算在报告来历。有一条用例把 `{"source":"generated","action":"adopted"}` 打进请求体，断言它被完全忽略。

5. **两个模块的测试夹具要补迁移。** `work-editor` 与 `review-delivery` 的隔离 schema 夹具各自写死一份迁移清单，不跟着 `cmd/migrate` 走。没补之前**整包全红**（连既有用例一起），不是只有新用例红——这个夹具没有「少一列就少一个断言」的中间状态。

6. **535 的 down 在有 `imported` 行时失败，已实测。** 先建一条 `imported` 版本再跑 down，PostgreSQL 报 `check constraint ... is violated by some row`；删掉那行再跑，四条 down 依次成功，约束回到三值。这是写进 down 文件注释里的行为，不是意外。

7. **`content_work` 的 `RETURNING` 子句也要改。** `RenameWork` 的 `UPDATE ... RETURNING` 列表是手写的，与 `workSelect` 不共享。守卫只管 `SET` 段，所以它不会替这里报错——漏了的话是 `scanWork` 在运行时少一列。

### 变异验证（各一处）

每条守卫与负例都用一次反向改动验证过会红，改动随后原样还原：

| 改动 | 红的用例 |
|---|---|
| `worksInProgress` 去掉过滤 | `excludes a historical import even when its document is being edited` |
| `ListWorks` 的 SQL 加上 `OR topic_card_id=''` | `TestAnImportedWorkIsNotListedUnderAnyTopicCard` |
| 027 反查改成先走两跳 | `TestAStatedVersionBeatsTheTwoHopsWhenBothExist` |
| `RenameWork` 的 SET 段加 `historical_import=false` | `TestNoUpdateAssignsHistoricalImport` |
| `ImportVersion` 改用 `ActionSaved` | `TestSourceAndActionAreNeverReadFromInput` + `TestImportingWritesOneVersionWithNoChainBehindIt` |

---

# PR 2 — 页面

## Phase 9: 导入向导

- [x] T045 [P] node 测试先行：四步状态机（未开始 / 已完成 / 失败），断言失败时**后续步骤不启动**（FR-023）
- [x] T046 [P] node 测试先行：幂等键的取法与「已成功的步骤记下产出 id、重试时跳过」的判定（FR-022）
- [x] T047 [P] node 测试先行：缺省标题取正文可读前缀；**不编概括**（FR-016）
- [x] T048 向导页面：四步串既有端点（contract 的端点表），顺序被 025 的 `ResolveVersion` 锁死，不能并发
- [x] T049 必填项校验只做「非空」这类本地判断；**缺链接的规则不重复实现**，由 025 后端指名 `page_url_or_content_id`，前端只高亮它（SC-004）
- [x] T050 失败界面：列出四步各自状态，失败那步给**从该步重试**的入口；**不是一句「导入失败」，也不是从头再来**
- [x] T051 失败界面明说「已建出的作品可以照常打开和编辑，也可以稍后补一条发布记录」；**没有任何自动删除**（FR-023a）
- [x] T052 正文超 200000 runes 被拒绝时，**没有任何对象被建出来**（SC-008）——即长度校验发生在第一步之前

## Phase 10: 历史导入的可辨认

- [x] T053 作品列表：导入的作品带「历史导入」标识，与正常创建的可区分（FR-017）
- [x] T054 今日工作台第五项「待补录的反馈」：每条能看出是否为历史导入（FR-028）。**不改 026 的判定，也不把它们过滤掉**——它们确实待补录

## Phase 11: 「暂无个人表现数据」

- [x] T055 [P] node 测试先行：判定只看**指标行数是否为 0**，不含任何时间窗口或阈值（FR-024）
- [x] T056 账号页一条恒定可见的说明
- [x] T057 工作台「值得写的选题」区块说明行一条，**与既有的「候选自动生成暂不可用（EP-04c 未落地）」并列**，不取代（SC-011）
- [ ] T058 用例：录入任意一条指标后两处都消失，且**没有出现任何系统生成的表现摘要**（FR-026，宪法 IX）

## Phase 12: 收尾

- [x] T059 四语言
- [x] T060 只挂既有 Multica 组件；不手搭页面控件与布局（宪法 VII / `docs/development/design/README.md`）
- [x] T061 **无 UI 单测**（宪法 II）；非 UI 逻辑的 node 测试已在 T045 ~ T047 / T055
- [x] T062 界面项写 `specs/031-historical-import/manual-ui-todo.md`，并在 PR 正文列手验 Todo
- [x] T063 新页面登记进 `scripts/content-boundaries.json` 的 **`adapters`**；`modules` 依赖表**仍然不动**
- [ ] T064 `pnpm typecheck --force`、`pnpm lint`、`pnpm test`、`pnpm check:content-boundaries`、views package-exports

## PR 2 实施记录

- 实现了导入四步状态机与其 node 测试；失败后保留已完成步骤与稳定幂等键。
- 后续步骤失败时，已完成步骤所使用的字段会锁定；尚未使用的字段保持可编辑，避免重试把已有对象与屏幕上的新输入混在一起。
- 后端目前没有用于四个内容写端点的持久化幂等键语义；前端键不会下传，也不能防御“服务端成功、响应丢失”的重试重复创建。该缺口已记录为审查阻塞，不能以本地跳过已收到响应的步骤当作 FR-022 完成。
- 页面复用既有设置页、输入、选择、按钮与作品列表组件，并登记到内容边界 adapters。
- `pnpm typecheck --force` 与 `pnpm check:content-boundaries` 已通过；仅执行了 workflow、API 传输、路径和个人表现的 node 测试。根据本任务约束，未运行全量 `pnpm lint` / `pnpm test`、任何 UI 测试、数据库套件或 browser/computer-use 验收，因此 T064 与 T058 留待后续受控验证及用户手验。
