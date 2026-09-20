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

- [x] T027 `packages/views/content/workspace-core/index.tsx`：四个区块，**只挂既有组件**，不新增控件、不改样式。可用的上游 import 里已包含 `@multica/core/workspace` 与 `@multica/views/settings/layout`（`upstreamImports`），**登记表不改**
- [x] T027a `packages/views/package.json` 加 `./content/workspace-core` 导出，**与 `index.tsx` 同一个提交**。#177 曾只加导出不加目录，把 `rich-content/package-exports.test.ts` 弄红了一轮
- [x] T028 节奏区：八个平台各一个数字输入；**留空与填 0 在界面上看得出来是两件事**（留空显示「未设」，0 显示「这周不发」）
- [x] T029 渠道模板区：每渠道一段自由文本；**四个能交付、四个不能**，不能交付的**写明这一点**（FR-012a），但**照样可以配**
- [x] T029a 账号标识：主页链接输入，写明**只存不访问**；账号名沿用账号页既有字段，**不在本区块重复提供编辑入口**
- [x] T030 审核规则区：显示「自己审核」；**团队复核写明是后续能力**，**不提供任何成员选择控件——包括禁用的**（FR-019）
- [x] T030a 既有的**自动预检开关**（019）与审核规则**在同一处可见**，**不改它的键名、默认值或语义**（FR-020）
- [x] T031 观察时点区：全局一个数 + 每渠道可覆盖；界面说得出**「这个渠道自己设的」还是「用的全局值」**（`source` 三态）；**写明「观察时点尚未接入待补录判定」**（FR-026a，PR 3 合入前）
- [x] T032 插槽与注入：`workspace-tab.tsx` 加 `renderOperatingRules?`、`settings-page.tsx` 透传、`apps/web/.../settings/page.tsx` 注入。**两个上游文件各约三行，全是新增**；`git diff -w` 既有行零删除，**单列 `upstream:` 提交**。不要用 `extraDeviceTabs`——它的 prop 名与注释都写着是桌面端设备设置，而且它加的是一个新 tab，不是工作区分区里的一节
- [x] T033 四语言文案（en / zh-Hans / ja / ko）并跑 `locales/parity.test.ts`
- [x] T034 新建 `manual-ui-todo.md`，界面项全部进去并一律记「未执行」；**不写 UI 单测**（宪法 II），核对 `packages/views` 下没有本卡新增的 `.test.tsx`；跑 `rich-content/package-exports.test.ts`

---

## Phase 6: 接进 027（PR 3）

- [x] T035 **先写** `NeedsRegistration` 的新用例：`due = passed` 按原两条件；`due = not_yet` **不在**待补录里；**`due = unknown` 按原两条件**（SC-015）。确认失败。**`unknown` 走 `passed` 的分支，不是 `not_yet` 的**——这是整个 PR 3 最容易做反的一处：把「不知道到没到期」当成「还没到」，会让每一条没有发布时间的记录从工作台静悄悄消失，而它们恰恰最需要有人去看一眼
- [x] T036 改 `server/internal/content/feedback-learning/states.go` 的 `NeedsRegistration`，加第三个参数。`feedback-learning` 的依赖表里**有 `workspace-core`**，所以**直接 import**，不读源文件对表，**登记表不改**
- [x] T036a 改 `NeedsRegistration` 那段注释：它现在写的是「§3.2 的观察时点还不存在」，那句话已经过期。新注释要写清**为什么 `unknown` 走 `passed` 分支**
- [x] T037 改守卫 `TestThePendingDerivationHasNoTimeLogic`：从「禁止时间比较」改成「**天数必须读自参数或设置，不得是字面量**」。**不是删掉它**
- [x] T038 变异验证：把天数写成字面量 `14` → T037 变红（SC-014）。**这是唯一能证明守卫改写之后没变空的东西**
- [x] T039 读端接上：列待补录的查询要带上观察时点。确认 `GET /api/content-feedback/pending` 的既有用例仍绿，必要时补一条

---

## Phase 7: Polish

- [x] T040 跑全部验证：`pnpm typecheck --force`、三项 check、`bash scripts/test-go.sh`、两套 db-suites、core vitest、`locales/parity.test.ts`、**views 的 `rich-content/package-exports.test.ts`**
- [x] T041 核对三个 PR 的上游提交都是既有行零删除（`git show --numstat` 应为 `N 0`）
- [x] T042 核对「未设 ≠ 零值」三处都在：Go 读写（T005）、core 解析（T021）、界面展示（T028）
- [x] T043 PR 3 正文：**为什么它单独一个 PR**（改的是另一个模块里一条刻意写下来的守卫）、守卫从什么改成什么、变异证据、`unknown` 为什么走 `passed` 分支

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

---

## 实施记录（PR 2 页面，2026-09-20）

分支 `claude/impl-029-operating-rules-page`，base `app-main` @ `60a9d2a`。T027–T034 全部落地。

**三处与清单写的不一样，都在 PR 正文单列：**

1. **`adapters` 加了一行，加的是一个上游文件。** 清单 T027 写的是「登记表不改」——那句话对 `modules` 成立，对 `adapters` 不成立。边界检查器 `scripts/check-content-boundaries.mjs:141` 规定：**任何不在内容根目录下的文件，只要 import 了内容模块，就必须在 `adapters` 里**。按主控指定的「独立分节组件 + 只加一行引用」，那一行引用就让 `packages/views/settings/components/workspace-tab.tsx` 成了适配器。先例是 `server/cmd/server/router.go`——它本来就是上游文件且早在 `adapters` 里。

2. **core 的三个新文件从 `packages/core/workspace/index.ts` 再导出。** `upstreamImports` 给 `packages/views/content` 的允许项是 `@multica/core/workspace` **精确匹配**，子路径不算（只有 `@multica/ui` 和 `github.`/`go.` 前缀享受前缀匹配）。`timezone` 与 `auto-precheck` 早就是这么从桶里导出的，本卡照做，**没有去放宽 `upstreamImports`**。

3. **多了一个 `rules-form.ts`（+22 条 node 用例）。** 清单 T028/T031 只说「留空与填 0 看得出来是两件事」「说得出是渠道的还是全局的」，没说判定放哪。和 027 的 `form.ts`、028 的 `draft.ts` 同一个理由：按钮可不可点、一个值该怎么读，是规则不是排版；桌面端将来复用的是这些纯函数，不是这个页面。

**一处顺手修掉的**：`SettingsSaveState` 的错误文案我原本写成 `contentAccounts.saveFailed`，那个键不存在——`contentAccounts` 里已有的是 `failed`。typecheck 当场报出来，改用既有键，没有新增一个同义的。

**页面 PR 没有变异验证这一环**（宪法 II 不写 UI 单测），所以 22 条 node 用例是这一段唯一的自动化证据。其中「清空的框不能变成 0」「存进去的 0 不能显示成未设置」两组是照着已知会出错的地方写的，不是照着实现反推的。

---

## 实施记录（PR 3 接进 027，2026-09-20）

分支 `claude/impl-029-feedback-observation`，base `app-main` @ `7c67e40`。T035–T043 全部落地。

**为什么它单独一个 PR**：它改的不是本卡的模块，而是 027 里**一条刻意写下来的守卫**——`TestThePendingDerivationHasNoTimeLogic` 当初是为了挡住「发明一个默认天数」才写的。和设置页放同一个 PR，审的人就得同时判断「这个新设置对不对」和「那条守卫该不该动」，而后者才是真正要看清楚的地方。

**守卫从什么改成什么**：`TestThePendingDerivationHasNoTimeLogic` → `TestThePendingDerivationReadsItsWindowFromSettings`。**没有删**，禁的东西一条没少，只是从「禁止时间比较」收紧成「禁止**写死的**时间比较」，现在查三件事：

1. 派生用的那条 SQL 里没有时间比较（原样保留）；
2. 整个模块的 Go 代码里没有写死的窗口——四条正则分别打 `N * 24 * time.Hour`、`N * time.Hour`、`time.Duration(N)`、`AddDate(0, 0, N)`；
3. `NeedsRegistration` 的签名里仍然有 `workspacecore.Due`，且模块里仍然提到 `workspacecore.DueUnknown`。

第 2 条先 `stripGoComments` 再扫。这是 PR 1 的教训（凭据守卫当初匹配到了我自己写的注释）：**一条会因为文件解释了自己而报警的守卫，方向是反的。**

**变异证据（四次，全部先红后复原）：**

| | 变异 | 结果 |
|---|---|---|
| M1 | 把天数写成字面量 `14*24*time.Hour` | 守卫红：`a hard-coded observation window appears in this module: "14*24*time.Hour"` |
| M2 | 把 `unknown` 当 `not_yet` 处理 | `TestNeedsRegistrationTreatsAnUnknownWindowAsWorthLookingAt` 红：`an unknown window behaves like 'not yet'; those records would vanish from the workbench` |
| M3 | 适配器不读设置，一律答 `unknown` | 四条真实 DB 用例红（列/不列/渠道覆盖/改设置生效） |
| M4 | 去掉每请求一次的设置缓存 | `TestTheObservationAdapterReadsTheSettingsOncePerRequest` 红：`the settings were read 5 times for 5 rows, want 1` |

**`unknown` 为什么走 `passed` 的分支**：不知道到没到期，不是「还没到」。没设过观察天数的品牌、以及 025 允许存在的「没有发布时间」的发布记录，恰恰是最需要有人去看一眼的那些；把它们归到「还没到」，它们会从工作台上**无声地消失**，而且没有任何地方会报警。这是本卡第三次遇到同一个坑（019 的布尔、027 的指标值、029 的天数），所以它在契约 §8、plan 风险表、T035、策略 5、函数注释、两条用例和 M2 里各钉了一遍。

**三处与清单写的不一样：**

1. **窗口在 Go 里过滤，不在 SQL 里。** T039 写的是「列待补录的查询要带上观察时点」。真放进 SQL，那个天数要么被拼进语句、要么在 SQL 里多一份同样的规则；而守卫第 1 条要查的恰恰是「SQL 里没有时间比较」。改成扫描后在 Go 里问一次 `NeedsRegistration`：**SQL 一个字没改**，第三个条件只在一个地方出现。

2. **多了一个 `Observation` 端口，没有直接调 `workspacecore.Store`。** 照既有的 `Publications` 先例：这个模块只读自己的表，别人的表由适配器回答。`Due` 类型本身还是从 `workspace-core` 来的——依赖表里本来就有它，在这边重抄一个三态枚举就是等着走样的第二份定义。`Observation` 为 nil 时一律答 `DueUnknown`，所以没接线的测试夹具的行为和 029 之前完全一样。

3. **适配器加了每请求一次的设置缓存（清单没写）。** 待补录是一行问一次窗口，照原样写下去，一个有两百条已发布记录的品牌就是两百次一模一样的设置查询。顺带把 `now` 也冻在请求开始——分散在扫描过程中的不同时刻去比，同一页里的两行可能对「这个窗口过没过」给出不同答案，而一份自相矛盾的列表比一份晚了一秒的更糟。缓存是**每请求**的：`feedbackStore()` 每次调用都新建一个，所以改了设置下一次请求就生效（`TestChangingTheWindowChangesTheList` 盯着这一点）。

**前端顺手修掉一处会连累整张表的地方**：`due` 在 zod schema 里是 `z.string().catch("").optional()`，而它的同级字段都不是。理由写在注释里——**这个字段是本卡新加的，一张在它出现之前就能正常工作的列表，不该因为某一行的 `due` 回来的形状不对就整张塌掉**。行本身仍然指着一条真实的、等着补数的发布记录，那才是工作台要的东西。缺失或不可用都读成「没答案」，**绝不读成「已过」**。

**文案更新**：027 页面「待补录」区块与今日工作台第五项都改了口径，并各自多说了一句天数从哪来；029 设置页的观察时点说明也补上了它现在的后果（改这个数字会改工作台列谁）。四语言同步，`locales/parity.test.ts` 160 条绿。旧口径的两条 spec 断言（026 FR-005b 的「没有到期概念」、027 FR-020 的「MUST NOT 带任何时间逻辑」）用 026 既有的「更新」块格式就地补注，**原文一行未删**——它们记的是当初为什么不发明一个默认天数，那条理由今天依然成立。

**本 PR 没有 `upstream:` 提交**：改到的共享文件只有四个 `locales/*/common.json`，动的三个键（`contentFeedback.*`、`contentToday.feedback.derivation`、`operatingRules.observationHint`）全是 Loretide 在 027/029 里自己加的，没有触到上游 Multica 的任何一行。
