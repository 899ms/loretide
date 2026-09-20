# Tasks: source-inbox 存储与接口（028 实施 PR 1）

**Spec**: [spec.md](./spec.md) · **Plan**: [plan.md](./plan.md) · **Contract**: [contracts/source-inbox.md](./contracts/source-inbox.md) · **Issue**: #174

## Phase 1: 基线

- [x] T001 核对迁移最大号（当时 515 → 从 516 起）；记下若 #166 先合入需整体重排
- [x] T002 核对已落地模块数（实测 6，本卡后 7；派单写的 7→8 与实测不符，PR 正文如实写）

## Phase 2: 存储

- [x] T003 516 `content_source` 建表：不可变列（`kind` / `url` / `captured_at` / `recorded_by` / `historical_import`）+ 可整理列（`title` / `tags` / `annotation` / `personal_judgement` / `status`）；`kind` 与 `status` 带 `CHECK`；**无 PK / UNIQUE / FK / CASCADE**（R1 / R2 / R5）
- [x] T004 [P] 517 / 518 索引，各单文件单语句 CONCURRENTLY；518 是 `workspace_id` 打头
- [x] T005 519 `content_source_snapshot` 建表：`content`、`content_hash`、`captured_at`
- [x] T006 [P] 520 / 521 / 522 索引；521 `(workspace_id, content_hash)` 供去重，522 `(workspace_id, source_id)`
- [x] T007 523 `content_source_revision` 建表：改了什么、谁、何时
- [x] T008 [P] 524 / 525 索引；525 `(workspace_id, source_id, created_at)`
- [x] T009 七条索引登记 `concurrentIndexCleanups`（517 / 518 / 520 / 521 / 522 / 524 / 525），**名字逐字节一致**；三张建表迁移**不登记**（R6）
- [x] T010 删除链：`workspace_delete.sql` 三张表 + 重新生成；删除清单 `workspace_delete_manifest_test.go` 三条
- [x] T011 容器自带 PG 验证 up / 失败重试 / down

## Phase 3: 模块（守卫与契约先写）

- [x] T012 **先写** `guards_test.go` 守卫 1：`content_source_snapshot` 与 `content_source_revision` 无 UPDATE / DELETE 路径，**且各有 INSERT**（少后半条则空模块恒绿）
- [x] T013 **先写** `guards_test.go` 守卫 2：每条 `UPDATE content_source` 的 **SET 段**不含五个不可变列，**且至少存在一条 UPDATE**（否则守卫空转）。SET 段的切法要排除 `WHERE` 与 `RETURNING`
- [x] T014 [P] **先写** 守卫 3 / 4：零出站（`net/http` / `http.Client` / `oauth` / `access_token` / 模型调用字样）、无 `Fetch*/Crawl*/Download*/Scrape*` 方法
- [x] T015 [P] **先写** 守卫 5：受控集**只有** `Kinds` 与 `Statuses` 两个，多一个就红（防「顺手给解析状态补枚举」）
- [x] T016 `contract.go`：`Kind` {`pasted_text`, `url`}、`Status` {`inbox`, `organized`, `archived`}、结构体、`ErrInvalid` / `ErrNotFound` / `ErrStorage`、`FieldError` 逐字段 400、`ContentHash`（sha256 十六进制）
- [x] T017 [P] `contract_test.go`：受控集越界各一条负例；`kind = url` 必填 `url`、`kind = pasted_text` 必须无 `url`；空正文拒绝；`ContentHash` 对同一输入稳定、对不同输入不同
- [x] T018 `store.go`：`Store`、`begin` 取栅栏（第一条语句）、`audit` / `reportFailure`、`appendRevision`（整理记录的**唯一**写入点）
- [x] T019 `source.go`：`CreateSource` —— `pasted_text` 在**同一事务**里写一条快照并算哈希；`url` **不写快照**；`ListSources`（status / tag 筛选）、`GetSource`
- [x] T020 `organize.go`：`Organize`（改可变列 + 追加整理记录）、`BulkTag` / `BulkArchive`（**逐条**各追加一条记录）
- [x] T021 `dedup.go`：`DuplicatesByHash` —— 只查只返回，**不合并不删除**
- [x] T022 `store_integration_test.go`：真实 DB —— 建条目 + 快照、URL **零快照**、整理留痕、批量逐条留痕、归档后仍可查、哈希去重返回候选且**两条都还在**、跨品牌 404、不可变列在整理后逐项未变

## Phase 4: 接口

- [x] T023 **先写** 端点用例：列表筛选、建条目（响应带 `duplicates`）、整理、整理历史、批量逐条结果、去重候选
- [x] T024 `content_source.go` 七个端点；**无 DELETE**；静态段 `duplicates` / `bulk` 注册在 `{id}` 之前
- [x] T025 **第 12 步**：`{id}` 与 `{id}/revisions` 各一条用例穿过真实中间件，**路径参数值 ≠ 上下文值**
- [x] T026 越权经 `workspace-core.Authorize`，拒绝与「不存在」同形
- [x] T027 上游两文件（`router.go` 挂载、`content-boundaries.json` 的 `adapters`）单列 `upstream:` 提交，**既有行零删除**

## Phase 5: core

- [x] T028 [P] `contract.ts`：zod + `parseWithFallback`；**畸形响应用例**（未知 `kind` / `status` 降级、`null` 集合归一）
- [x] T029 [P] `queries.ts`：列表 / 单条 / 整理历史 / 重复候选 query；建条目 / 整理 / 批量 mutation。**整理不做乐观更新**（会导航或需确认的流程不乐观）
- [x] T030 `packages/core/package.json` 加 `./content/source-inbox`

## Phase 6: 验证

- [x] T031 容器自带 PG 跑两套 db-suites（handler / cmd-server）+ `go test ./internal/content/...`
- [x] T032 `pnpm typecheck --force` + 三项 check + core vitest
- [x] T033 变异验证**五处**，每处确认对应用例变红、**改完即还原**，变异必须**可编译**：
      (M1) `UPDATE content_source` 的 SET 段带上 `captured_at` → 守卫 2 变红；
      (M2) 整理路径改写快照而非追加记录 → 守卫 1 变红；
      (M3) 给 `kind = url` 也写一条快照 → URL 零快照用例变红；
      (M4) 去重端点顺手删掉重复那条 → 「只提示不删除」用例变红；
      (M5) 从 `concurrentIndexCleanups` 删掉 521 → `TestEveryConcurrentUpBuildHasCleanup` 变红
- [x] T034 核对改动文件全部落在 plan.md 清单内；清单外的在 PR 正文单列
- [x] T035 PR 正文：迁移说明（三表 + 七个 CONCURRENTLY 索引、无外键、R5 为何不用 `PRIMARY KEY`、R6 七条登记、每表 `workspace_id` 打头索引及理由）、**上游改动一节**、第 12 步、**URL 无去重提示这条已知限制**、落地模块实测数字

## Phase 7: 页面 PR（#179）

- [x] T036 `packages/views/content/source-inbox/`：收件箱页，**只挂既有组件**，不新增控件、不改样式
- [x] T037 非 UI 判定进 `packages/core/content/source-inbox/draft.ts` + node 测试：草稿校验逐字段、链接协议白名单、标签切分、请求体成形、**去重提示恒不阻断**、状态流转选项、批量结果汇总
- [x] T038 列表按状态与标签筛选；空态 / 加载态 / 失败态各自可见
- [x] T039 新建：粘贴文本或链接二选一（选链接时正文框消失并写明「只存不打开」）、标题 / 标签 / 批注 / 为什么收藏、「历史导入」勾选
- [x] T040 **批注与个人判断是两个分开的输入框**（§4 步骤 4「原文、自动摘要和个人判断分开保存」）
- [x] T041 详情：原文**只读**、哈希 / 采集时间 / 录入人只读；链接条目显示「没有存正文」而不是空框
- [x] T042 整理动作与整理记录流；状态按钮只给当前状态以外的，**且已归档能回到待整理**
- [x] T043 批量标签 / 归档，**逐条结果**（N 条成功、M 条没成功并列出 id）
- [x] T044 去重提示：创建**之后**提示，**不阻断创建、无合并入口、无删除入口**
- [x] T045 「解析暂不可用」占位恒定可见并写明原因
- [x] T046 四语言并跑 `locales/parity.test.ts`；`manual-ui-todo.md` 界面项全部进去；**不写 UI 单测**（宪法 II），核对无新增 `.test.tsx`
- [x] T047 `pnpm typecheck --force` + 三项 check + core vitest；**0 抓取**（页面不发任何对 URL 的请求）；不接今日工作台

## Dependencies

```text
T001,T002 基线
  └─ 存储 (T003→T004 [P] → T005→T006 [P] → T007→T008 [P] → T009 → T010 → T011)
       └─ 模块 (T012,T013,T014,T015 先写 [P] → T016 → T017 [P] → T018 → T019 → T020 → T021 → T022)
            └─ HTTP (T023 先写 → T024 → T025 → T026 → T027 上游)
                 └─ core (T028,T029 [P] → T030)
                      └─ 验证 (T031 → T032 → T033 → T034 → T035)
```

**T012–T015、T017、T023 必须先写并确认失败。**

## Implementation Strategy

1. **守卫 2 是本卡最容易写空的一条**（T013）。「SET 段不含不可变列」要真的切出 SET 段——`WHERE source_id = $1` 和 `RETURNING captured_at` 里出现列名都是合法的，照整条语句扫会误报；而如果模块里一条 `UPDATE content_source` 都没有（比如整理改走了别的写法），守卫又会**恒绿**。两头都要断言。
2. **URL 零快照要用「数一数」而不是「看一眼」**（T022）。断言「`kind = url` 的条目的快照数 == 0」，而不是断言某个函数没被调用——后者在实现换个写法后就失效了。
3. **去重用例必须断言两条都还在**（T022 / M4）。只断言「返回了候选」的用例，在一个顺手删掉重复条目的实现上**照样绿**。R-011 原文是「内容相同**不删除**独立的收藏上下文与批注」。
4. **批量操作逐条留痕**（T020）。一次批量打标签三条，就要有三条整理记录；写成一条「批量操作」记录，之后就无法回答「这一条是什么时候被打上这个标签的」。
5. **静态路由段先于参数段注册**（T024）。`duplicates` 和 `bulk` 若排在 `{id}` 之后，会被当成条目 id——023 的 `persona/revisions` 踩过同一个坑，那里的注释留着。
