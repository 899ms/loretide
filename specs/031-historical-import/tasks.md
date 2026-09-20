# Tasks: §3.3 导入少量历史资产

**Spec**: [spec.md](./spec.md) · **Plan**: [plan.md](./plan.md) · **Contract**: [contracts/historical-import.md](./contracts/historical-import.md) · **Issue**: #210

两个 PR。PR 2 依赖 PR 1 全部合入。`[P]` 表示与相邻任务无依赖、可并行。

---

# PR 1 — 存储与接口

## Phase 1: 基线

- [ ] T001 核对迁移最大号（规格撰写时 531 → 从 532 起）。**若别的卡先合入占号，rebase 时编号与文件名整体重排**
- [ ] T002 核对 `scripts/content-boundaries.json`：记下 `modules` 依赖表的当前内容，PR 结束时 diff 必须为空（SC-014）
- [ ] T003 枚举**全部**按选题卡聚合的读路径：grep `topic_card_id` 与 `ListWorks` 的所有调用方，逐个判定「按卡」还是「按工作区」，把结论写进 PR 正文。**这一步是 FR-034 的前提，不是可选的调研**

## Phase 2: 迁移

- [ ] T004 532 `content_work` 加 `historical_import boolean NOT NULL DEFAULT false`；down 为 `DROP COLUMN IF EXISTS`
- [ ] T005 [P] 533 `content_publication_record` 加 `historical_import boolean NOT NULL DEFAULT false`
- [ ] T006 [P] 534 `content_publication_record` 加 `version_id text NOT NULL DEFAULT ''`
- [ ] T007 535 `content_artifact_version` 的 `action` `CHECK` 改写为四值（`DROP CONSTRAINT` + `ADD CONSTRAINT`，单语句单文件）
- [ ] T008 535 的 down 写回三值，并在文件注释里写明：**已有 `imported` 行时这条 down 会失败，这是正确行为**——让它安静通过等于留下一个数据库拒绝的约束
- [ ] T009 **不登记 `concurrentIndexCleanups`**：本 PR 一个索引都不建（R6 的适用前提是建索引）。在 PR 正文写明这是刻意的，不是漏登记
- [ ] T010 容器自带 PG 验证四条 up / down（**建独立库与角色，绝不指向容器外地址**）；T008 那条失败单独验证
- [ ] T011 确认 `workspace_delete.sql` 与删除清单用例**不需要改**：本 PR 不建表，只加列

## Phase 3: work-editor（守卫先写）

- [ ] T012 **先写守卫**：任何 `UPDATE content_work` 的 **SET 段**不含 `historical_import`，**且**存在一条写它的 `INSERT`。少了后半条，空实现也是绿的（照抄 028 `guards_test.go` 的结构）。SET 段的切法要排除 `WHERE` 与 `RETURNING`
- [ ] T013 `contract.go`：`ActionImported`，`var Actions` 四值。**`Source` 一个值都不加**（注释自称「Exactly three, per SOP 7.1」）
- [ ] T014 `contract_test.go:24` 的清单同步为四值 —— 这条用例存在的意义就是不让人只改一边
- [ ] T015 `version.go` 加 `ImportVersion`：与既有三条同构，`source=edited` / `action=imported` 由代码选定，**请求体给不了**
- [ ] T016 用例：`ImportVersion` 产出 `edited` / `imported`，`restored_from` 与 `adopted_from` 皆空（FR-010 不虚构版本链）
- [ ] T017 用例：导入版本的 `created_at` 落在导入时刻的同一分钟内；**没有任何路径能把它写成过去**（FR-014a）
- [ ] T018 `store.go:173` 创建守卫去掉 `work.TopicCardID == ""`；补契约注释，指向同表 `snapshot_id` 已有的「A real state, not a missing value」
- [ ] T019 `Work` 结构 + `workSelect` + `scanWork` + `INSERT` 加 `HistoricalImport`
- [ ] T020 用例：`CreateWork` 接受 `topic_card_id=''` 并写入 `historical_import=true`

## Phase 4: 按卡聚合的排除（负例先写）

- [ ] T021 **先写负例**：造一件 `topic_card_id=''` 的作品，断言它**不出现**在 `ListWorks(..., topicCardID != "")` 的结果里。现有 SQL `($2='' OR topic_card_id=$2)` 看上去天然成立——**用负例证明，不假设**
- [ ] T022 **先写负例**：同一件作品**不出现**在今日工作台「正在推进的作品」区块的派生结果里
- [ ] T023 **正例**：同一件作品**出现**在全工作区作品列表（`ListWorks(..., "")`）里。排除的是「按卡」的语义，不是作品本身（FR-036）
- [ ] T024 `packages/core/today/types.ts` 的 `WorkLike` 加 `historicalImport`；`sections.ts` 的 `worksInProgress` 过滤掉它。node 测试
- [ ] T025 T003 枚举出的**其余每一处**按卡读路径，各加过滤与一条负例。**一处都不能只靠正例**

## Phase 5: review-delivery

- [ ] T026 `PublicationRecord` 与 `RecordRequest` 加 `VersionID` 与 `HistoricalImport`；`Record` 的 `INSERT` 与 `scan` 同步
- [ ] T027 用例：既有调用方不传新字段时，行为与本 PR 之前**逐字相同**（`''` / `false`）
- [ ] T028 用例：导入之后在同一份文档上正常存一版新的，发布记录的 `version_id` **不变**（FR-012，发布后快照不随新版本移动）
- [ ] T029 用例：`reported_published` 缺 `page_url_or_content_id` 仍被拒绝并指名该字段（025 既有约束，本 PR 没碰它）

## Phase 6: 027 的反查顺序

- [ ] T030 `handler/content_metric.go` 的 `feedbackPublications.Resolve`：先读 `version_id`，非空短路；为空走原两跳。**顺序不能反**——一条既有交付任务又有直接指向的记录，权威答案是直接指向
- [ ] T031 用例：带 `version_id` 的记录短路返回它
- [ ] T032 用例：`version_id` 为空但有交付任务的记录，反查结果与本 PR 之前**逐字相同**
- [ ] T033 用例：两跳都失败仍返回 `""` 且**不拒绝记录**（027 既有语义，FR-011b 要求它不被改掉）
- [ ] T034 确认 `feedbacklearning` 包**一个字都没改**（FR-029）

## Phase 7: handler 与前端契约

- [ ] T035 创建作品与创建发布记录的 handler 透传两个新字段；`historical_import` 只在创建时读，**任何改路径都不读它**
- [ ] T036 用例：尝试把已存在对象的 `historical_import` 改掉 → 被拒绝并指名该字段（FR-018 / SC-007）
- [ ] T037 `packages/core/content/work-editor` 与 `review-delivery` 的 zod schema 加字段，走 `parseWithFallback`
- [ ] T038 [P] 每个改动过的 schema 各一条畸形响应用例
- [ ] T039 第 12 步：本 PR 触及的每个带路径参数的端点，各一条穿过真实中间件且**路径参数值与上下文值不同**的用例

## Phase 8: 验证

- [ ] T040 `(cd server && go test ./internal/content/... ./internal/handler/...)`
- [ ] T041 两套 db-suites：`scripts/test-go-db.sh --suite handler` 与 `--suite cmd-server`
- [ ] T042 `pnpm typecheck --force`、`pnpm lint`、`pnpm test`
- [ ] T043 `pnpm check:content-boundaries`；并 diff `scripts/content-boundaries.json` 的 `modules`，**必须为空**（SC-014）
- [ ] T044 PR 正文：T003 的枚举结论、T009 为何不登记 `concurrentIndexCleanups`、以及**上游改动一节**（若有；本 PR 预期为无）

---

# PR 2 — 页面

## Phase 9: 导入向导

- [ ] T045 [P] node 测试先行：四步状态机（未开始 / 已完成 / 失败），断言失败时**后续步骤不启动**（FR-023）
- [ ] T046 [P] node 测试先行：幂等键的取法与「已成功的步骤记下产出 id、重试时跳过」的判定（FR-022）
- [ ] T047 [P] node 测试先行：缺省标题取正文可读前缀；**不编概括**（FR-016）
- [ ] T048 向导页面：四步串既有端点（contract 的端点表），顺序被 025 的 `ResolveVersion` 锁死，不能并发
- [ ] T049 必填项校验只做「非空」这类本地判断；**缺链接的规则不重复实现**，由 025 后端指名 `page_url_or_content_id`，前端只高亮它（SC-004）
- [ ] T050 失败界面：列出四步各自状态，失败那步给**从该步重试**的入口；**不是一句「导入失败」，也不是从头再来**
- [ ] T051 失败界面明说「已建出的作品可以照常打开和编辑，也可以稍后补一条发布记录」；**没有任何自动删除**（FR-023a）
- [ ] T052 正文超 200000 runes 被拒绝时，**没有任何对象被建出来**（SC-008）——即长度校验发生在第一步之前

## Phase 10: 历史导入的可辨认

- [ ] T053 作品列表：导入的作品带「历史导入」标识，与正常创建的可区分（FR-017）
- [ ] T054 今日工作台第五项「待补录的反馈」：每条能看出是否为历史导入（FR-028）。**不改 026 的判定，也不把它们过滤掉**——它们确实待补录

## Phase 11: 「暂无个人表现数据」

- [ ] T055 [P] node 测试先行：判定只看**指标行数是否为 0**，不含任何时间窗口或阈值（FR-024）
- [ ] T056 账号页一条恒定可见的说明
- [ ] T057 工作台「值得写的选题」区块说明行一条，**与既有的「候选自动生成暂不可用（EP-04c 未落地）」并列**，不取代（SC-011）
- [ ] T058 用例：录入任意一条指标后两处都消失，且**没有出现任何系统生成的表现摘要**（FR-026，宪法 IX）

## Phase 12: 收尾

- [ ] T059 四语言
- [ ] T060 只挂既有 Multica 组件；不手搭页面控件与布局（宪法 VII / `docs/development/design/README.md`）
- [ ] T061 **无 UI 单测**（宪法 II）；非 UI 逻辑的 node 测试已在 T045 ~ T047 / T055
- [ ] T062 界面项写 `specs/031-historical-import/manual-ui-todo.md`，并在 PR 正文列手验 Todo
- [ ] T063 新页面登记进 `scripts/content-boundaries.json` 的 **`adapters`**；`modules` 依赖表**仍然不动**
- [ ] T064 `pnpm typecheck --force`、`pnpm lint`、`pnpm test`、`pnpm check:content-boundaries`、views package-exports
