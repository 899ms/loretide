---
description: "Task list for 030 topic source refs — SOP §5.2 items 2 and 5"
---

# Tasks: 选题卡引用素材条目（030）

**Prerequisites**: `spec.md`、`plan.md`、`contracts/topic-source-refs.md`

**改动文件必须在 plan.md → Project Structure 清单内。**

**三条 clarify 已裁决**（主控 2026-09-21，PR #207 评论）：

- **Q1 = B（两列，按 §5.2 原文改名与归属）**：`fit_source_ids`（第 2 项「引用哪些素材」）、`evidence_source_ids`（第 5 项「证据…缺口」）。两个加列迁移，`jsonb` 数组，无外键。**第 4 项「与已有作品的关系」指作品不指素材，不加字段**；「过去的经营结论」未落地，**不留字段**。
- **Q2 = A**：只用字符串的端口 + handler 适配器，**栅栏事务内**校验，外来 / 不存在 **404 同形**（照 #133）。`archived` **可被引用**，选择器默认不列。
- **Q3 = B**：`StoredSnapshot` 两个扩展字段，**023 的负例一行不改**。
- **新写入路径**：`POST /api/content-topics/{id}/sources`，照 `SetAccount` 单开端点，不挂四个动作上。
- **反向显示**：不做。

**没有新表、没有新索引、删除链不改、登记表不改、`concurrentIndexCleanups` 不改。**

**两个 PR**：T001–T028 存储与接口；T029–T036 页面；T037–T041 收尾。

---

## Phase 1: Setup

- [x] T001 裁决已拿到并回写四个件：`spec.md`（§5.2 原文替换转述、更正表、字段改名与归属、裁决记录一节、FR-001/001a/001b/006/006a/009a/015/016 改写）、`plan.md`、`contracts/topic-source-refs.md`、本文件
- [ ] T002 记录基线：`bash scripts/test-go.sh`、两套 db-suites（按 `docs/development/testing-database-suites.md` 配 `LORETIDE_DB_TEST_*`，指向容器自带 PG 上的独立库与**非超级用户**角色）、三项 check、`pnpm typecheck --force`、core 与 views 的相关用例。**注意**：`packages/views/onboarding/steps/step-workspace.test.tsx` 的「submits the prefix the user was shown」与 `server/pkg/agent` 的 `TestClaude*` / `TestCodex*` / `TestCodeArts*` 在 `app-main` 上已是失败，与本卡无关，基线里记下
- [ ] T003 读四处既有形状并对着写：`topic-planning/store.go:218-247`（`checkAccount` + `SetAccount` 的两段注释，**它们就是本卡的规则**）、`store.go:57-74`（`normalizeStrings` / `encodeStrings` / `decodeStrings`，**尤其 `normalizeStrings` 少做了哪三件事**）、`snapshot.go:45-57`（两个扩展字段的注释，**它同时是「为什么不借 required_sources」的答案**）、`feedback-learning/store.go` 的 `Observation` 端口（#197，本卡端口的形状来源）

---

## Phase 2: 迁移（PR 1）

- [ ] T004 写 `server/migrations/532_content_topic_card_fit_source_ids.{up,down}.sql`：`ALTER TABLE content_topic_card ADD COLUMN IF NOT EXISTS fit_source_ids jsonb NOT NULL DEFAULT '[]'::jsonb`。注释写明它对应 §5.2 第 2 项「引用哪些素材」，以及**为什么是 jsonb 不是 text[]**（同表 `channels` 就是 jsonb） — **FR-001、FR-003**
- [ ] T005 写 `server/migrations/533_content_topic_card_evidence_source_ids.{up,down}.sql`：同形，对应 §5.2 第 5 项 — **FR-001、FR-003**
- [ ] T006 迁移规则逐行复核并记录：两个文件里 `REFERENCES` / `FOREIGN KEY` / `CASCADE` / `CREATE INDEX` 各 **0 处**；`workspace_delete_manifest_test.go` 与 `concurrentIndexCleanups` **零改动**。**逐条写进 PR 正文，不默认成立** — **FR-003、SC-014**
- [ ] T007 实测 up → down → up，确认可逆且 `'[]'::jsonb` 默认值在既有行上填对 — **SC-014**

---

## Phase 3: 模块（PR 1，先写测试）

- [ ] T008 **先写** `sources_test.go` 的归一化用例并确认失败：去空白（`""` 与 `"  "` 都丢）、去重（保留首次出现的顺序）、上限 50（第 51 条即 `ErrInvalid` 且**指名是哪一栏**）、**三者的执行顺序**（先去空白再去重，否则 `""` 与 `"  "` 会被当成两条不同的 id 活到去重之后） — **FR-008、SC-006**
- [ ] T009 写 `server/internal/content/topic-planning/sources.go`：归一化函数 + `SourceReader` 端口。**端口签名只用字符串**（`Exists(ctx, workspaceID, sourceID) (bool, error)`），注释写明为什么——`topic-planning` 的依赖行里没有 `source-inbox`，签名里一出现 `sourceinbox` 的类型就得改登记表 — **FR-010、FR-011**
- [ ] T010 **不改 `normalizeStrings`**：另写一个函数。`channels` 还在用它，改它就是改 022 的行为。在新函数的注释里写明这一点 — **FR-008**
- [ ] T011 `contract.go`：`TopicCard` 加 `FitSourceIDs` / `EvidenceSourceIDs` 两个 `[]string`，JSON 名 `fit_source_ids` / `evidence_source_ids`。注释各自指回 §5.2 的哪一项 — **FR-001**
- [ ] T012 **先写**契约负例并确认失败：`TopicCard` 上的引用字段**恰好两个**，名字逐字相同；**没有**第三个（既不给第 4 项「与已有作品的关系」，也不给「过去的经营结论」） — **FR-001a、FR-001b、SC-001**
- [ ] T013 **先写** `store_integration_test.go` 的校验用例并确认失败：跨品牌 id → 404；不存在的 id → 404；**两个响应体逐字节相同**；一次提交含一条非法 → **两栏都是 0 条写入** — **FR-012、FR-013、SC-004、SC-005**
- [ ] T014 `store.go` 加 `checkSources`：照 `checkAccount` 的形状，**在栅栏事务内**，外来或不存在一律 `ErrNotFound`。`Store` 加 `Sources SourceReader` 字段；为 nil 时 `ErrStorage`（与 `checkAccount` 同口径，**不是静默通过**） — **FR-007、FR-010、FR-012**
- [ ] T015 **先写** `SetSources` 的用例并确认失败：整列替换；**请求里缺省某一栏 → 那一栏不变**；显式空数组 → 清空；`status` / `account_id` / 七项正文**逐字节未变** — **FR-006、FR-006a、SC-002、SC-003a**
- [ ] T016 `store.go` 加 `SetSources`：栅栏第一条语句、同事务审计（动作名 `link-sources`）、校验在事务内、整列替换。**不碰** `status` / `decision_reason` / `decision_note` / `started_brief_revision_id` / `account_id` / 七项正文 — **FR-006、FR-007、FR-026**
- [ ] T017 **先写**反向用例并确认失败：四个动作各执行一次，卡的引用**零变化** — **FR-005、SC-003**
- [ ] T018 创建路径带上两列：`CreateTopicCard` 的 INSERT 加两列，走**同一套**归一化与校验 — **FR-009a、SC-001**
- [ ] T019 读取路径带上两列：`GetTopicCard` / `ListTopicCards` 的 SELECT 与 Scan 加两列，`decodeStrings` 复用；空是 `[]` 不是 nil — **FR-009**
- [ ] T020 **先写** `archived` 用例并确认失败：引用一条 → 把它改成 `archived` → 卡上的引用数**不变**；已归档 id 直接提交给 `SetSources` → **接受**（校验只问品牌与存在性） — **FR-014、SC-007**

---

## Phase 4: 快照（PR 1）

- [ ] T021 **先写**快照用例并确认失败：两栏**分开**冻；冻结后改卡上的引用，快照**不变**；一条都没引时是**空数组不是 null** — **FR-015、FR-017、SC-009**
- [ ] T022 `snapshot.go`：`StoredSnapshot` 加 `FitSources` / `EvidenceSources` 两个 `[]string` 扩展字段，照既有 `AutoPrecheck` / `UsesNeutralExpression` 的形状与注释口径。**`SnapshotInputs` 相应加两个字段，由调用方读卡时填** — **FR-015**
- [ ] T023 **确认 023 的负例一行未改且绿**：`TestNoSourcelessFieldIsInvented`（`snapshot_test.go:90`）与 `TestEmptyCollectionsMarshalAsEmptyNotNull`；`Required` / `Excluded` 仍写空。**diff 里 `snapshot_test.go` 的既有行零删除** — **FR-016、SC-008**
- [ ] T024 确认 022 的 A6 守卫（`store_integration_test.go:468`）一行未改且绿；本卡对 `content_brief_revision` **零语句改动** — **FR-004、SC-008**

---

## Phase 5: HTTP 与 core（PR 1）

- [ ] T025 `handler/content_topic.go`：`SetContentTopicSources` + `topicSourceReader` 适配器（读 `content_source`，按 `workspace_id` + `source_id`）。`topicPlanningStore()` 注入它。取路径参数**只用既有的 `topicCardIDFromURL`**（`content_topic.go:88`，已经是 `chi.URLParam`），**不新写取参数的助手** — **FR-010、FR-018**
- [ ] T026 `router.go` 挂 `r.Post("/sources", h.SetContentTopicSources)` 到 `/{id}` 子路由内 — **FR-018、SC-011**
- [ ] T027 **第 12 步用例**（`content_topic_sources_test.go`）：穿过**真实 router 与中间件**，**路径参数值与上下文里的工作区 id 不同**，断言 handler 用的是 URL 里的那个；另断言未登录 / 非成员经中间件被拒 — **FR-018、SC-011**
- [ ] T028 core：`contract.ts` 两个字段 + zod（**缺失 / 类型不对都读成 `[]`**，照 #197 给 `due` 的 `.catch` 形状）、`contract.test.ts` 三条畸形响应用例（字段缺失 / 类型不对 / 整条不是对象）、`queries.ts` 的 `useSetContentTopicSources`、`snapshot.ts` + `snapshot.test.ts` 的两个扩展字段 — **FR-019、FR-020、SC-012**

---

## Phase 6: 页面（PR 2）

**宪法 II：不写 UI 单测。** 纯函数放 `form-state.ts` 配 node 用例；页面本身只有手验。

- [ ] T029 先读 `docs/development/design/README.md`；只挂既有组件，不新造控件、不设颜色 — **宪法 VII**
- [ ] T030 `form-state.ts` + `form-state.test.ts`：草稿里的两份选中列表、加 / 去一条、送出前的归一化、「缺省 ≠ 清空」在草稿层的表达 — **FR-006a、FR-008**
- [ ] T031 `packages/views/content/topic-planning/index.tsx`：**「为什么适合这个 IP」与「证据是否充分」两栏各一个选择器**；候选走 `useContentSources(wsId, ...)`，只列本品牌、**默认排除 `archived`** — **FR-021、SC-001**
- [ ] T032 **「与已有作品的关系」那一栏不加选择器**。初稿曾按 Issue 的转述挂错过一次，这一条单独列出来以免实施时手滑 — **FR-001a、SC-001**
- [ ] T033 详情：每条引用显示**标题与整理状态**；标题为空时有说得出是哪条的回落，**不显示空白行**；已归档的标「已归档」 — **FR-022、SC-007**
- [ ] T034 界面写明这些引用是**人手选的**，与 §4 步骤 4「系统做什么」一列的「关联资料」分得开；028 的「0 条系统生成」说明仍成立 — **FR-023、SC-013**
- [ ] T035 四语言（en / zh-Hans / ja / ko），跑 `locales/parity.test.ts`。**插值变量不要叫 `count`**（会触发复数规则，027 踩过） — **SC-015**
- [ ] T036 写 `manual-ui-todo.md`：两栏各自的选择器、第 4 项**没有**选择器、归档条目的显示、跨品牌不可选、长文与溢出、无权限只读、四语言。全部「未执行」 — **FR-030、SC-015**

---

## Phase 7: Polish

- [ ] T037 **变异验证七条**，每条先红后还原（plan.md「变异清单」）：M1 缺省改成清空 / M2 校验挪出栅栏 / M3 去掉去重 / M4 部分写入 / M5 两栏合并冻 / M6 往 `required_sources` 写 / M7 加第三个引用字段。**一处没变红就补用例，不换变异** — **SC-003a、SC-005、SC-006、SC-008、SC-009**
- [ ] T038 跑全部验证：`pnpm typecheck --force`、三项 check、`bash scripts/test-go.sh`、两套 db-suites、core 与 views 相关 vitest、`locales/parity.test.ts` — **SC-010**
- [ ] T039 核对四处「不动」：`scripts/content-boundaries.json`、`concurrentIndexCleanups`、`workspace_delete_manifest_test.go`、`snapshot_test.go` 与 `store_integration_test.go` 的既有行（`git show --numstat` 应为 `N 0`） — **FR-003、FR-011、SC-008、SC-010**
- [ ] T040 核对「缺省 ≠ 清空」三处都在：Go 的 `SetSources`（T016）、core 的草稿层（T030）、契约文档（contract §4） — **FR-006a**
- [ ] T041 两个 PR 正文：PR 1 写清**为什么引用挂第 2 项而不是第 4 项**（初稿挂错过，裁决更正）、端口为什么只用字符串、校验为什么在栅栏内、快照为什么走扩展字段而不是 `required_sources`、七条变异证据；PR 2 写清页面复用了哪些既有组件、手验项清单、**零 UI 单测**

---

## Dependencies

```text
Setup (T001–T003)
  └─ 迁移 (T004–T007)
       └─ 模块 (T008 先写 → T009–T012 → T013 先写 → T014 → T015 先写 → T016 → T017 先写 → T018–T019 → T020 先写)
            └─ 快照 (T021 先写 → T022 → T023–T024 确认既有负例未动)
                 └─ HTTP 与 core (T025–T028)
                      └─ 页面 (T029–T036)
                           └─ Polish (T037–T041)
```

**先写并确认失败的清单**：T008、T012、T013、T015、T017、T020、T021。

---

## Implementation Strategy

1. **对着注释写，不要凭印象。** `checkAccount` / `SetAccount` / `StoredSnapshot` / `normalizeStrings` 四处的既有注释就是本卡的规则，T003 要求先读一遍。本卡几乎没有新形状要发明。
2. **两个既有守卫是绕开的，不是改写的。** 022 的 A6 与 023 的 `TestNoSourcelessFieldIsInvented` 在本卡**一行不改**，T023 / T024 专门确认这件事，M6 专门证明 023 那条仍在工作。
3. **「缺省 ≠ 清空」是本卡的第四次同族问题**（019 布尔、027 指标值、029 天数、#197 三态）。它在 FR-006a、SC-003a、T015、T030、T040 与 M1 里各钉一遍。
4. **「归档 ≠ 删除」是第二处同族问题**。`content_source` 没有删除路径，所以「不进候选」绝不能顺手写成「引用失效」。T020 与 FR-014 盯着。
5. **引用挂第 2 项，不是第 4 项。** 初稿按 Issue 的转述挂错过一次。T012 的契约负例与 T032 的页面任务各自防一次。
6. **端口只用字符串是硬约束，不是风格。** 一旦签名里出现 `sourceinbox` 的类型，登记表就得改，而裁决说它不改。`check:content-boundaries` 是机器判定，会当场红。
