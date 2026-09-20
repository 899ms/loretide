---
description: "Task list for 029 operating rules — SOP §3.2 brand-level settings"
---

# Tasks: 运营规则——品牌级设置（029）

**Prerequisites**: `spec.md`、`plan.md`、`contracts/operating-rules.md`

**改动文件必须在 plan.md → Project Structure 清单内。**

**三条 clarify 已裁决**（主控 2026-09-20，PR #192 评论）：

- **Q1 = A**：四项设置全部作为**工作区 `settings` 的 `loretide.operating_rules` 键**，代码落 **`workspace-core`**，**登记表不改，没有迁移**；
- **Q2 = A**：渠道模板按 **`ip-profile` 的八个平台**；主页链接进**账号 `settings` 的 `loretide.homepage`**，不改 `content_account` 表；**界面写明「配了模板不等于能交付」**；
- **Q3 = 改良 B，拆成三个 PR**：PR 1 设置 + 派生函数、PR 2 页面、**PR 3 改 027 的 `NeedsRegistration`**；守卫**不拆**，改成「**天数必须读自设置，不得是字面量**；缺设置或 `published_at` 为空答『无法判断』」。
- **我补的三条全部接受**：节奏按渠道、观察时点按渠道可缺省到全局、字段约束说明是自由文本。
- **页面**沿用 `workspace-tab.tsx` 路径；上游改动 `upstream:` 单列、**既有行零删除**；**能注入优先**。

**没有新表、没有迁移、没有索引、删除链不改。**

**三个 PR**：T001–T026 存储与接口；T027–T034 页面；T035–T039 接进 027；T040–T043 收尾。

---

## Phase 1: Setup

- [x] T001 裁决已拿到并回写四个件：`spec.md`（新增「裁决记录」一节、FR-012a / FR-026 / FR-026a / FR-027 / FR-031 / FR-033 改写、SC-014 ～ SC-016 新增）、`plan.md`、`contracts/operating-rules.md`、本文件
- [x] T002 记录基线：`bash scripts/test-go.sh`、两套 db-suites（按 `docs/development/testing-database-suites.md` 配 `LORETIDE_DB_TEST_*`，**指向容器自带 PG 上新建的独立库与最小权限角色**）、三项 check、`pnpm typecheck --force`、`packages/views` 的 `rich-content/package-exports.test.ts`。**注意**：`packages/views/onboarding/steps/step-workspace.test.tsx` 的「submits the prefix the user was shown」在 `app-main` 上已有一条失败，与本卡无关，基线里记下即可
- [x] T003 读三处既有形状：`server/internal/handler/workspace.go` 的 `validateTimezoneSetting` / `timezoneFilled` / `validateAutoPrecheckSetting` / `autoPrecheckFilled`（写时校验 + 读时填默认且不回写）、`packages/core/workspace/auto-precheck.ts` 的四个函数（**尤其 `hasStoredAutoPrecheck` 为什么单独存在**）、`server/cmd/server/router.go` 的 `PUT /{id}/scope` 那条注释（LT-014 为什么不用 PATCH 上的字段）

---

## Phase 2: 模块（PR 1，先写测试）

- [x] T004 [P] **先写** 受控集矩阵（纯函数）：`platform` **恰好八项**且与 `ip-profile` 的 Go 源文件**逐字相同**（读源文件对表，**不 import**——`workspace-core` 的依赖表里没有 `ip-profile`）；`review_rule` **恰好一项** `self`。多一个少一个即红。确认失败
- [x] T004a [P] **先写** 「没有第三个受控集」的负例：模块里除这两个之外不存在别的枚举——`note` / `homepage` 等必须还是 `string`。确认失败
- [x] T005 [P] **先写** **「未设 ≠ 零值」**的用例（SC-002）：`ReadCadence` 对「键缺席」与「值为 0」返回**不同的 `stored`**；`ReadObservation` 同理。确认失败。**这是本卡最容易悄悄发生的数据损坏，所以它先写**——019 已经在布尔上踩过一次
- [x] T005a [P] **先写** `ReadObservation` 的 `source` 三态用例：渠道自己设了 → `"channel"`；渠道没设走全局 → `"global"`；都没设 → `"none"`。**不是布尔**：界面要能说出是哪一种
- [x] T006 [P] **先写** 观察时点派生的**三态**用例（SC-009）：没设 → `unknown`；`published_at` 为空 → `unknown`；已过 → `passed`；未到 → `not_yet`。另一条负例扫源码确认**不存在任何写死的天数常量**。确认失败
- [x] T007 [P] **先写** 校验用例：负的目标条数 / 负的观察天数 / 越界 `platform` / 非 `self` 的 `review_rule`，各一条，**各自点名字段**（合同第 5 节的表）。确认失败
- [x] T008 [P] **先写** 合并语义用例：写 `loretide.operating_rules` 之后，同一个工作区的 `loretide.timezone` 与 `loretide.auto_precheck` **逐字节不变**（SC-003）。确认失败
- [x] T009 [P] **先写** 不存凭据守卫（SC-004）：扫模块源码与接口定义，无 password / token / cookie / secret 类字段；**且 MUST 断言模块确实有写入路径**，否则空模块也绿（照 022 的原版，别省这半条）。确认失败
- [x] T009a [P] **先写** 不外发守卫（SC-005）：模块里不存在 HTTP 客户端调用。确认失败
- [x] T009b [P] **先写** 不调度守卫（SC-007）：不存在定时器、cron、通知发送路径。确认失败
- [x] T009c [P] **先写** 无成员选择守卫（SC-008）：不存在任何成员查询或选择路径——**包括禁用的控件**。确认失败
- [x] T010 新建 `server/internal/content/workspace-core/operating_rules.go`：键名常量、两个受控集、`ReadCadence` / `ReadObservation`（**各返回两个值**）、`TemplateNoteFor`、校验与合并
- [x] T011 新建 `server/internal/content/workspace-core/observation.go`：`ObservationDue` 三态
- [x] T012 新建 `guards_test.go`，把 T009 ～ T009c 落进去

---

## Phase 3: HTTP（PR 1，先写测试）

- [x] T013 **先写** 决策顺序用例：越权与不存在**逐字节相同**（除 `trace_id`）；400 是诊断错误对象且点名字段。确认失败
- [x] T014 **先写** **服务端合并**的用例：`PUT /api/operating-rules` 只带 `cadence` 时，`templates` / `review_rule` / `observation` 与**其余 settings 键**都不变。确认失败。**这是本卡不照抄 LT-009 / 019 的那一处**——它们在前端合并，中间隔一次网络往返
- [x] T015 **先写** 读时填默认且**不回写**的用例：`GET` 之后工作区那一行的 `updated_at` 与 `settings` 逐字节不变。确认失败
- [x] T016 **先写** `PUT /api/content-accounts/{id}/homepage` 的用例：只放行 `http` / `https`，`javascript:` / `file:` / `data:` 各一条负例；写入之后账号的 `loretide.scope` **不变**。确认失败
- [x] T016a **先写**（**工作流第 12 步**）`/homepage` 的**参数 ≠ 上下文**用例：穿过**真实 router 与中间件**，路径里的账号 id 与上下文里的不是同一个 → 与「不存在」同形的 404。只用 `chi.URLParam`，不自己解析路径（LT-011/012/013 的教训）。确认失败
- [x] T017 新建 `server/internal/handler/content_operating_rules.go`：`GET` / `PUT /api/operating-rules`。**没有路径参数**（品牌由 `X-Workspace-ID` 决定）——第 12 步对这两条没有对象，**这件事要在 PR 正文写明，不是默默跳过**
- [x] T018 新建 `server/internal/handler/content_account_homepage.go`：`PUT /api/content-accounts/{id}/homepage`
- [x] T019 `scripts/content-boundaries.json`：两个新 handler 文件加入 `adapters`，跑 `pnpm check:content-boundaries`。**`modules` 依赖表一个字不改**
- [x] T020 **一个上游文件，一个 `upstream:` 提交**，PR 正文单列「上游改动」一节（第 13 步）：`server/cmd/server/router.go` 挂三条路由。**新增条目放进自己的块**（前空行 + 一行注释）；`git diff -w` 必须只有新增行——025 因 gofmt 重排返工过一次

---

## Phase 4: core（PR 1）

- [x] T021 [P] `packages/core/workspace/operating-rules.ts` + `operating-rules.test.ts`（**首行 `// @vitest-environment node`**）：zod schema、`parseWithFallback`、**`number | undefined` 加一个 `stored` 判定，绝不 `?? 0`**；受控集**读 Go 源文件比对**（八个平台、一个 `review_rule`）
- [x] T021a [P] `packages/core/workspace/homepage.ts` + 测试：`loretide.homepage` 的读写与 URL 判定（只放行 `http` / `https`）
- [x] T021b [P] `packages/core/package.json` 加 `./workspace/operating-rules` 与 `./workspace/homepage` 两条导出，与既有的 `./workspace/auto-precheck` 并列
- [x] T022 [P] 读写 hooks（放在既有 `packages/core/workspace/` 下，与 `mutations.ts` 并列）。**全部非乐观**——设置会改变别人看到的东西
- [x] T023 跑 `pnpm check:content-boundaries` 与 `pnpm check:diagnostics-contract`，确认**落地模块数仍是 8**（本卡不新建 Go 模块目录，只给 `workspace-core` 加文件）且两项都退出 0
- [x] T024 变异验证**五处**，每处确认对应用例变红、**改完即还原**。变异必须**可编译**：
      (M1) 把 `ReadCadence` 的「键缺席」读成 `0` → T005 变红（**这一处最要紧**）；
      (M2) 让 `ObservationDue` 在没设天数时返回 `not_yet` → T006 变红；
      (M3) 在模块里加一个 `platform_password` 字段 → T009 变红；
      (M4) 在模块里加一个 `http.Get` → T009a 变红；
      (M5) 让 `PUT` 整体替换而不是合并 → T014 变红
- [x] T025 核对改动文件全部落在 plan.md 清单内；清单外的在 PR 正文单列
- [x] T026 PR 1 正文：**「为什么没有迁移」一节**（裁决 Q1=A；R1–R6 / 删除清单 / 删除链 / 索引各条为何不适用——「没改」和「忘了改」在 diff 上长得一样）、**上游改动一节**、**「`/api/operating-rules` 没有路径参数」的说明**、「§3.2 对应」逐句写明现在可操作到什么程度。**两件本卡验不了的事如实写**：「系统永远不需要平台凭据」只有结构保证；「渠道说明进模型上下文之后不会带上别的」只覆盖了本卡的组装函数

---

## Phase 5: 页面（PR 2）

- [ ] T027 `packages/views/content/workspace-core/index.tsx`：四个区块，**只挂既有组件**，不新增控件、不改样式。可用的上游 import 里已包含 `@multica/core/workspace` 与 `@multica/views/settings/layout`（`upstreamImports`），**登记表不改**
- [ ] T027a `packages/views/package.json` 加 `./content/workspace-core` 导出，**与 `index.tsx` 同一个提交**。#177 曾只加导出不加目录，把 `rich-content/package-exports.test.ts` 弄红了一轮
- [ ] T028 节奏区：八个平台各一个数字输入；**留空与填 0 在界面上看得出来是两件事**（留空显示「未设」，0 显示「这周不发」）
- [ ] T029 渠道模板区：每渠道一段自由文本；**四个能交付、四个不能**，不能交付的**写明这一点**（FR-012a），但**照样可以配**
- [ ] T029a 账号标识：主页链接输入，写明**只存不访问**；账号名沿用账号页既有字段，**不在本区块重复提供编辑入口**
- [ ] T030 审核规则区：显示「自己审核」；**团队复核写明是后续能力**，**不提供任何成员选择控件——包括禁用的**（FR-019）
- [ ] T030a 既有的**自动预检开关**（019）与审核规则**在同一处可见**，**不改它的键名、默认值或语义**（FR-020）
- [ ] T031 观察时点区：全局一个数 + 每渠道可覆盖；界面说得出**「这个渠道自己设的」还是「用的全局值」**（`source` 三态）；**写明「观察时点尚未接入待补录判定」**（FR-026a，PR 3 合入前）
- [ ] T032 插槽与注入：`workspace-tab.tsx` 加 `renderOperatingRules?`、`settings-page.tsx` 透传、`apps/web/.../settings/page.tsx` 注入。**两个上游文件各约三行，全是新增**；`git diff -w` 既有行零删除，**单列 `upstream:` 提交**。不要用 `extraDeviceTabs`——它的 prop 名与注释都写着是桌面端设备设置，而且它加的是一个新 tab，不是工作区分区里的一节
- [ ] T033 四语言文案（en / zh-Hans / ja / ko）并跑 `locales/parity.test.ts`
- [ ] T034 新建 `manual-ui-todo.md`，界面项全部进去并一律记「未执行」；**不写 UI 单测**（宪法 II），核对 `packages/views` 下没有本卡新增的 `.test.tsx`；跑 `rich-content/package-exports.test.ts`

---

## Phase 6: 接进 027（PR 3）

- [ ] T035 **先写** `NeedsRegistration` 的新用例：`due = passed` 按原两条件；`due = not_yet` **不在**待补录里；**`due = unknown` 按原两条件**（SC-015）。确认失败。**`unknown` 走 `passed` 的分支，不是 `not_yet` 的**——这是整个 PR 3 最容易做反的一处：把「不知道到没到期」当成「还没到」，会让每一条没有发布时间的记录从工作台静悄悄消失，而它们恰恰最需要有人去看一眼
- [ ] T036 改 `server/internal/content/feedback-learning/states.go` 的 `NeedsRegistration`，加第三个参数。`feedback-learning` 的依赖表里**有 `workspace-core`**，所以**直接 import**，不读源文件对表，**登记表不改**
- [ ] T036a 改 `NeedsRegistration` 那段注释：它现在写的是「§3.2 的观察时点还不存在」，那句话已经过期。新注释要写清**为什么 `unknown` 走 `passed` 分支**
- [ ] T037 改守卫 `TestThePendingDerivationHasNoTimeLogic`：从「禁止时间比较」改成「**天数必须读自参数或设置，不得是字面量**」。**不是删掉它**
- [ ] T038 变异验证：把天数写成字面量 `14` → T037 变红（SC-014）。**这是唯一能证明守卫改写之后没变空的东西**
- [ ] T039 读端接上：列待补录的查询要带上观察时点。确认 `GET /api/content-feedback/pending` 的既有用例仍绿，必要时补一条

---

## Phase 7: Polish

- [ ] T040 跑全部验证：`pnpm typecheck --force`、三项 check、`bash scripts/test-go.sh`、两套 db-suites、core vitest、`locales/parity.test.ts`、**views 的 `rich-content/package-exports.test.ts`**
- [ ] T041 核对三个 PR 的上游提交都是既有行零删除（`git show --numstat` 应为 `N 0`）
- [ ] T042 核对「未设 ≠ 零值」三处都在：Go 读写（T005）、core 解析（T021）、界面展示（T028）
- [ ] T043 PR 3 正文：**为什么它单独一个 PR**（改的是另一个模块里一条刻意写下来的守卫）、守卫从什么改成什么、变异证据、`unknown` 为什么走 `passed` 分支

---

## Dependencies

```text
T001 裁决回写 → T002 基线 → T003 读三处既有形状
  └─ PR 1 模块 (T004–T009c 先写 [P] → T010 → T011 → T012)
       └─ PR 1 HTTP (T013–T016a 先写 → T017 → T018 → T019 → T020 上游路由)
            └─ PR 1 core (T021,T021a,T021b,T022 [P] → T023 → T024 变异 → T025 → T026)
                 └─ PR 2 页面 (T027 → T027a → T028 → T029 → T029a → T030 → T030a → T031 → T032 插槽 → T033 → T034)
                      └─ PR 3 接进 027 (T035 先写 → T036 → T036a → T037 → T038 变异 → T039)
                           └─ Polish (T040–T043)
```

**先写并确认失败的清单**：T004、T004a、T005、T005a、T006、T007、T008、T009、T009a、T009b、T009c、T013、T014、T015、T016、T016a、T035。

## Implementation Strategy

1. **「未设 ≠ 零值」要在三个地方各钉一次**（T005 Go、T021 core、T028 界面），不是一次。它是本卡唯一会**悄悄**损坏数据的东西：把「还没想好」记成「这周不发 0 条」，在任何后续统计里都是真实的坏数，而且不一致时没有任何东西会报警。M1 就是把它折掉，确认三处都看得见。**019 已经在布尔上踩过一次，那次的教训是 `hasStoredAutoPrecheck` 必须是一个独立函数。**
2. **`ReadCadence` 返回两个值，不返回 `*int`**（T010）。指针能表达「没有」，但忘了判空就是一次 panic 或一次静默的零值；两个返回值在 Go 里至少是显眼的。
3. **服务端合并，不是前端合并**（T014 + M5）。LT-009 与 019 都是前端读一遍再整体写回，中间隔一次网络往返。四项设置是一张表单里填的，冲突面比一个开关大得多——LT-014 已经换过做法，router.go 里那行注释写着原因。
4. **守卫改写要有自己的变异**（T037 + T038）。从「禁止时间比较」变成「禁止写死的时间比较」，判断变复杂了就可能写漏，而一条写漏的守卫和一条不存在的守卫在 CI 上都是绿的。
5. **`unknown` 走 `passed` 的分支**（T035 + T036a）。判断不出到期，不等于这条不用补录。反过来做会让没有发布时间的记录从工作台消失——它们恰恰是最需要有人看一眼的那些。
6. **PR 3 单列**。它改的是另一个模块里一条刻意写下来的守卫。合进 PR 1 会让这次越界混在四十个文件里看不见，而裁决把它单列出来正是为了让它看得见。
7. **能注入优先，代价是两个上游文件而不是一个**（T032）。把四个区块直接写进 `workspace-tab.tsx` 只动一个文件，但那意味着两百行 Loretide UI 长在上游文件里。两个 prop 加起来约六行，全是新增。

---

## 实施记录（PR 1 存储与接口，2026-09-20）

分支 `claude/impl-029-operating-rules-api`，base `app-main` @ `76ce3d0`。T001–T026 全部落地。**零迁移、零新表、登记表 `modules` 一个字未动**，只有 `adapters` 多了两行。

**四处与清单写的不一样，都在 PR 正文单列：**

1. **`workspace-core` 多了一个 `store.go`。** analyze 时我报过这条：FR-028/029 要求每条写入取栅栏并同事务审计，而 plan 给这个包的只有两个纯函数文件，它历史上也没有任何存储。我当时提了 (a)/(b)/(c) 三条出路、倾向 (a)，主控派活时没有改口，所以按 (a) 做了，并把包注释补准——它原本那句「不替调用方查对象」说的是不读**别人的**表，而运营规则就在工作区那一行上。**这是我自己拍的板，请复核时看一眼。**

2. **端点按 LT-014 做成服务端合并，不是 LT-009/019 的前端合并。** 合同第 4 节写了理由：前端合并要先读完整 settings 再整体写回，中间隔一次网络往返，两个人同时改会互相抹掉。`PUT /api/operating-rules` 与 `PUT /api/content-accounts/{id}/homepage` 都在服务端用 `jsonb_set` 只写一个键。

3. **`workspace.id` 是 uuid，不是 text。** 每张内容表的 `workspace_id` 都是 text，我照着写了 `$1::text`，三条用例当场红在 `operator does not exist: uuid = text`。改成 `$1::uuid`，并在读路径先 `uuid.Parse` 一次——不是一个 uuid 就答 404，与写路径的栅栏给出的答案一致，而不是让类型转换失败变成 500。

4. **hooks 放 `packages/core/workspace/operating-rules-queries.ts`。** 清单说「与 `mutations.ts` 并列」，但塞进既有 `mutations.ts` 会让一个上游文件长出 Loretide 的内容，单开一个文件更干净。

**一处守卫第一版是漏的，变异把它抓出来了：**

**M5（把 `PUT` 改成整体替换）第一次只被真实 DB 用例抓到，静态守卫放过了它。** 守卫问的是「包里有没有出现过 `jsonb_set(`」，而变异只改了两条语句中的一条，另一条还在，包级检索就满足了。已改成**逐条检查每一处 `SET settings =` 的右侧表达式**，并断言至少有两处（规则一处、主页一处）。重跑 M5b，静态守卫也变红。**这是 027 的 M6/M8 同一个教训第三次出现：一条放过了变异的守卫，就是一条没在工作的守卫。**
