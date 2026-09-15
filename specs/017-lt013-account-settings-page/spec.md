# Feature Specification: 账号设置页面可用（LT-013）

**Feature Branch**: `claude/spec-017-lt013-account-settings-page`
**Created**: 2026-09-15
**Status**: Draft
**Input**: `tasks/todo.md` LT-013——「浏览器在品牌下创建/切换账号，编辑平台要求与人设提示词，不提供人设库或绑定页面」

追踪 R-005～R-008、D11-V02。本卡是链条 LT-009（品牌空间）→ LT-010（授权）→ LT-011（账号与平台）→ LT-012（人设版本）的**收口**：前四张卡把能力放进了服务端，这一张把它放进浏览器。

---

## Current State（以代码为准，2026-09-15 于 `app-main` `1e8bccd` 核实）

| 事实 | 核实方式 | 结论 |
|---|---|---|
| 账号接口存在 | `server/internal/handler/content_account.go` | `CreateContentAccount` / `GetContentAccount` / `ListContentAccounts` / `UpdateContentAccount` 四个 handler 函数齐全 |
| 人设版本接口存在 | `server/internal/handler/content_account_revision.go` | `SetAccountPersonaPrompt`（201）/ `GetAccountPersonaPrompt`（当前）/ `GetAccountPersonaRevision`（按 id）/ `ListAccountPersonaRevisions` |
| **这八个 handler 一个都没挂到路由上** | `grep -n "content-accounts" server/cmd/server/router.go` → **无匹配**；`/api/content-diagnostics` 在 `router.go:1891` 有 `r.Route` | **页面今天无法调用任何账号接口**，见 Q1 |
| 前端没有账号客户端方法 | `grep -n "contentAccount" packages/core/api/client.ts` → **无匹配**；只有 `contentDiagnosticRequest` / `Stream` / `Download` | 需按既有模式新增 |
| 前端没有账号目录 | `ls packages/core/content` → 仅 `diagnostics`；`ls packages/views/content` → 仅 `diagnostics` | 两根都要新建 |
| 平台枚举只在 Go 一侧 | `server/internal/content/ip-profile/account.go` 的 `Platforms`（8 个值）+ 迁移 `477` 的 CHECK；**没有任何接口把它吐给前端** | 见 Q2 |
| 工作区内路由约定 | `apps/web/app/[workspaceSlug]/(dashboard)/diagnostics/page.tsx` 五行：`"use client"` + `useWorkspaceId()` + 渲染 `@multica/views/content/diagnostics` 的页面组件 + 注入平台能力 | 照此落位 |
| 设置类可复用组合 | `packages/views/settings/components/settings-layout.tsx` 导出 `SettingsContent`（注释写明「also used by workspace diagnostics」）、`SettingsTab` / `SettingsSection` / `SettingsCard` / `SettingsRow` / `SettingsSaveState` | 整套照搬，**不新增控件** |
| 保存态组件的状态集 | `SettingsSaveStatus = "idle" \| "saving" \| "saved" \| "error"` | **没有 conflict 态**，见下「409 怎么呈现」 |
| 四语言 | `packages/views/locales/` 下 `en` / `zh-Hans` / `ja` / `ko`，`parity.test.ts` 逐键比对 | 新文案四语言齐全 |

**没有核实到的，就没有写进这张表。** 上面每一行都能用左列的命令在 `1e8bccd` 上重现。

---

## 409 怎么呈现：复用 `error` 态换文案，不扩状态集

`SettingsSaveState` 只有四个态，而验收要求「保存成功 / 失败 / 409 三态文案明确，409 提示『可重试』而非『失败』」。两条路：

| 做法 | 代价 |
|---|---|
| 扩 `SettingsSaveStatus` 加 `"conflict"` | 改的是**上游共享组件**，影响设置页所有标签页。UI 硬规则明令不新增控件、不改既有组合 |
| **复用 `error` 态，传不同的 `errorLabel`** | 零改动上游。409 时传「有人同时改了这个账号，再保存一次即可」，其余失败传「保存失败」 |

**取后者。** 三态文案的差别是**文案**，不是控件——`SettingsSaveState` 的 `errorLabel` 本来就是调用方给的。这不是绕过验收：验收要的是「用户读到的话不同」，不是「组件多一个枚举值」。

按 design README「既有组合覆盖不了就记录缺口」，这里**不是覆盖不了**，所以不记缺口；但把「共享保存态组件没有可重试语义」作为一条观察写进 `contracts/` 的 UI 复用清单，供以后真要扩时有据可依。

---

## Clarifications

### Session 2026-09-15

三个问题已由主任务**裁决为 A**（2026-09-15）。裁决同时追加了三条要求，记在这里以免只活在对话里：

- **Q1 路由挂载** → 暂定 **A**：本 PR 在 `server/cmd/server/router.go` 既有的「工作区成员」分组内挂载这八个端点，**纯接线，不改任何 handler 逻辑**，并加一条路由存在性用例。
- **Q2 平台枚举来源** → 暂定 **A**：在 `packages/core/content/ip-profile/platforms.ts` 放一份常量，并用一条 **node 环境测试读 `account.go` 比对**，值不一致就红。零后端改动，沿用仓库已有的「测试读源文件比对，而不是第二次手抄」手法（`ip-profile` 自己就是这么守着迁移 CHECK 的）。
- **Q3 页面落位** → **A**：独立工作区路由 `apps/web/app/[workspaceSlug]/(dashboard)/accounts/page.tsx`，照诊断页的五行写法，视觉上复用 `SettingsContent` / `SettingsCard` / `SettingsRow` / `SettingsSaveState`，对照 `workspace-tab.tsx`。

**裁决追加的三条**：

1. **路由用例必须覆盖八条端点**，并断言未登录与非成员**经中间件**被拒——`#71` / `#73` 都没有任何用例发现端点没挂载，这一条补的就是那个盲点。PR 正文单列说明「此前端点未挂载」。
2. **不抄 `PLUGINS_V1_FLAG` 门控。** 设置页多个标签页带这个门控，照搬组合时极易连门控一起抄走。
3. **本 PR 不加侧栏入口**；`manual-ui-todo.md` 里留一条「侧栏是否需要入口」由主任务定。

---

## User Scenarios & Testing *(mandatory)*

### User Story 1 —— 在品牌下创建第一个账号（P1）

创作者打开品牌空间的「账号」页，页面是空的，并说明这里是干什么的。他选一个平台、填一个显示名，保存。列表里出现这个账号并被选中，右侧出现它的平台与人设提示词两块。

**为什么是 P1**：没有创建就没有后面的一切。SOP §3.1 的第一句就是这一步。

**验收**
1. 空状态可读，不是一个空白页或一个转圈；
2. 平台选项**来自受控枚举**（8 个），不是页面里另写一份；
3. 保存过程中保存按钮不可重复点；
4. 创建成功后新账号被选中，且**不需要手动刷新**。

### User Story 2 —— 在两个账号之间切换，未保存的输入不跟着走（P1）

创作者在账号 A 的人设框里打了一半字，没保存，切到账号 B。**B 的框里是 B 的内容，不是 A 的半截草稿。** 切回 A 时，A 的框回到**服务端的内容**，而不是那半截草稿。

**为什么是 P1**：这是任务卡逐字写下的验收（「换账号不沿用前账号未保存输入」），也是最容易写错的一条——用一个 `useState` 装表单，切账号时不重置，就会沿用。

**验收**
1. 切换后输入框内容属于当前账号；
2. 切回原账号显示服务端内容，未保存草稿**丢弃**（不是恢复）；
3. 这条由 `packages/core` 的**纯函数测试**锁住（表单状态机按账号分桶），不靠 UI 单测。

> **为什么「丢弃」而不是「保留草稿」**：保留草稿要回答「草稿存哪、活多久、和服务端版本冲突了算谁的」三个问题，而人设提示词的保存成本只有一次点击。任务卡也明确写的是「不沿用」。

### User Story 3 —— 刷新之后还在（P1）

创作者保存了人设提示词，刷新浏览器，内容还在。

**验收**
1. 刷新后内容来自**服务端读取**，不是 localStorage / Zustand 持久化；
2. 关掉浏览器换一台机器登录，看到的是同一份内容。

### User Story 4 —— 保存的三种结果，说的是三句不同的话（P1）

| 结果 | 用户读到 |
|---|---|
| 成功 | 「已保存」 |
| 并发冲突（409） | 「有人同时改了这个账号，再保存一次即可」——**可重试**，不是失败 |
| 其他失败 | 「保存失败」+ 诊断错误对象里的 `next_action` |

**验收**
1. 三句话互不相同且都能在界面上读到；
2. 409 的措辞不含「失败」；
3. 错误对象经 `parseWithFallback` + zod 解析，**不 cast**；`next_action` 缺失时不崩、不显示 `undefined`。

### User Story 5 —— 两个账号的人设互不影响（P1）

改 A 的人设三次，B 的人设与版本号纹丝不动。这是 LT-012 存储层已经保证的事，页面这一层只要不把它搞砸——比如把版本号当全局的、或者把 B 的请求发成 A 的 id。

### User Story 6 —— 插件功能关掉，这页照样能用（P2）

工作区把插件开关关掉，账号页仍可打开、创建、编辑、保存。

**为什么单列**：设置页里好几个标签页是 `useFeatureEnabled(PLUGINS_V1_FLAG)` 门控的，照搬组合时很容易把门控一起抄进来。这一条就是防这个。

### Edge Cases

- **账号列表为空**：空状态而不是空白；
- **人设提示词为空**：是**合法**的，保存后产生一条版本（LT-012 已定），页面不得把空值当成「没填完」拦下来；
- **超长提示词**：服务端回 400，页面按错误对象呈现，不自己再写一份字数上限；
- **账号在别的标签页被删/改**：本卡不做实时同步，重新进入页面即为最新；
- **网络断开**：保存失败态 + 可再次点击，不静默吞掉；
- **同一账号快速连点两次保存**：第二次可能拿到 409，按可重试呈现。

---

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**：页面 MUST 在工作区路由 `/{workspaceSlug}/accounts` 下打开，并按工作区隔离。
- **FR-002**：页面 MUST 列出当前品牌下的账号，并允许选中其中一个。
- **FR-003**：页面 MUST 支持创建账号（平台 + 显示名）。
- **FR-004**：平台选项 MUST 来自与 Go `Platforms` **同源**的常量，且有测试比对两边一致；页面 MUST NOT 自写第二份列表。
- **FR-005**：页面 MUST 支持编辑选中账号的平台与显示名，并保存。
- **FR-006**：页面 MUST 支持编辑并保存人设提示词；**空提示词是合法输入**。
- **FR-007**：切换账号时，未保存的输入 MUST 被丢弃，且 MUST NOT 出现在另一个账号的输入框里。
- **FR-008**：表单状态 MUST 按账号分桶，且该分桶逻辑 MUST 是 `packages/core` 里的纯函数，有 node 环境测试。
- **FR-009**：服务端状态 MUST 由 TanStack Query 管理；Zustand MUST 只持有本地 UI 状态（当前选中哪个账号、哪个区块展开）。
- **FR-010**：刷新后的内容 MUST 来自服务端读取；MUST NOT 依赖本地持久化。
- **FR-011**：保存 MUST 呈现成功 / 409 可重试 / 其他失败三态，且三者文案不同。
- **FR-012**：409 的文案 MUST NOT 表述为「失败」，MUST 表述为可再试一次。
- **FR-013**：所有接口响应 MUST 经 `parseWithFallback` + zod 解析；MUST NOT 直接 cast 网络 JSON。
- **FR-014**：诊断错误对象 MUST 按接入合同 §3 呈现 `next_action`，缺失时降级为通用文案而不是空白或 `undefined`。
- **FR-015**：页面 MUST NOT 被任何插件开关门控。
- **FR-016**：页面 MUST NOT 提供人设库、人设实体或绑定关系的任何入口（D11-V02 / W-02 边界）。
- **FR-017**：页面 MUST NOT 提供任务级 persona 覆盖入口（账号配置是唯一事实来源）。
- **FR-018**：页面 MUST NOT 提供修改或删除历史版本的入口（版本只插不改不删）。
- **FR-019**：新增文案 MUST 在 `en` / `zh-Hans` / `ja` / `ko` 四种语言齐全，`parity.test.ts` 通过。
- **FR-020**：UI MUST 只使用 `packages/ui` 与 `packages/views` 既有组件；MUST NOT 新增控件、调整样式或引入第二套视觉语言。
- **FR-021**（Q1-A）：八个既有 handler MUST 挂载到工作区成员分组下的 `/api/content-accounts`；此挂载 MUST NOT 改动任何 handler 的内部逻辑。
- **FR-021a**：路由用例 MUST 逐条覆盖八个端点存在，并 MUST 断言未登录与非成员**经中间件**被拒。
- **FR-023**：本 PR MUST NOT 新增侧栏入口；是否加入口由主任务在 `manual-ui-todo.md` 的对应条目上裁定。
- **FR-022**：UI 行为验收 MUST 落在 `manual-ui-todo.md`，MUST NOT 写 UI 单测或自动点击（constitution 原则 II）。

### Key Entities

- **Account**（服务端已有）：`account_id` / `workspace_id` / `platform` / `display_name` / `settings` / 时间戳。页面不新增字段。
- **Revision**（服务端已有）：`revision_id` / `account_id` / `revision` / `persona_prompt` / `created_at`。页面读当前版本、写新版本，**不读写历史以外的任何东西**。
- **AccountFormState**（本卡新增，**纯前端**）：按 `account_id` 分桶的草稿 `{platform, displayName, personaPrompt}` 加一个保存态。**不落盘、不进 Zustand 持久化**——它的整个生命周期就是这一次页面停留。

---

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**：在浏览器里能完成「建 A → 建 B → 给 A 写人设 → 切到 B → 切回 A」全程，A 的人设是 A 的、B 的是 B 的。
- **SC-002**：在 A 的输入框里打字不保存、切到 B，B 框里**一个字符都不来自 A**。
- **SC-003**：保存后刷新，内容仍在；清空浏览器存储后重新登录，内容仍在。
- **SC-004**：三种保存结果在界面上是三句不同的话，409 那句不含「失败」。
- **SC-005**：插件开关关闭时页面全部功能可用。
- **SC-006**：`packages/core` 新增纯函数测试逐文件通过；四语言 parity 通过；`typecheck --force` 全绿；三项 check 通过。
- **SC-007**：变异三处（脏输入隔离、409 判定、错误对象解析）各自使对应测试变红。

---

## UI Impact

**复用既有组件，零新增控件。** 具体清单在 `contracts/account-settings-ui.md`，实现后在 PR 正文照列：

| 用途 | 复用自 |
|---|---|
| 页面外壳与内容轨 | `SettingsContent`（`packages/views/settings/components/settings-layout.tsx`，诊断页已在用） |
| 分区与卡片 | `SettingsTab` / `SettingsSection` / `SettingsCard` / `SettingsRow` |
| 保存态 | `SettingsSaveState`（`errorLabel` 承载 409 文案） |
| 平台选择 | `Select` 系列（`@multica/ui/components/ui/select`） |
| 显示名输入 | `Input`（`@multica/ui/components/ui/input`） |
| 人设提示词输入 | `Textarea`（`@multica/ui/components/ui/textarea`） |
| 保存按钮 | `Button`（`@multica/ui/components/ui/button`） |
| 账号列表项的选中态 | 照抄 `settings-page.tsx` 侧栏的 `data-active` 写法（CLAUDE.md：选中态在 hover 下不得降级） |

对照的上游原生页面：`packages/views/settings/components/workspace-tab.tsx`——它同样是「一组字段 + 保存 + 保存态」，`SettingsCard` / `SettingsRow` / `SettingsSaveState` 的组合直接照搬。

---

## Assumptions

1. **本卡不做实时同步。** 别的标签页改了账号，本页不会自己更新。SOP 这一步是单人操作，多端同步不在 LT-013 的验收里。
2. **本卡不做版本历史界面。** LT-012 的「列出版本」端点会挂载，但页面只用「读当前 / 写新版」两个。历史的呈现没有出现在任务卡的交付里，做了就是超范围。
3. **`settings` JSON 不在本页编辑。** 账号的 `settings` 字段目前没有已定义的键；给一个自由 JSON 编辑框既没有验收也没有意义。
4. **桌面端不挂这一页。** 诊断页同样只在 Web 挂载，本卡照此，不动 desktop 路由。
5. **平台新增需要一次迁移**（LT-011 已写明）；因此同源常量变更也伴随迁移，测试会同时红。

---

## 澄清问题与裁决（三项均裁决为 A）

### Q1 八个 handler 没挂路由——在本 PR 挂，还是另开一卡？

任务卡写「后端预期零改动」，但核实下来 `/api/content-accounts` **在 `router.go` 里不存在**，`#71` 与 `#73` 都只交付了 handler 函数。页面没有可调用的 URL，验收里的每一条都无法在浏览器里成立。

- **A（推荐，已暂定）**：本 PR 在既有的 `RequireWorkspaceMember` 分组内加一个 `r.Route("/api/content-accounts", ...)`，把八个函数挂上，并加路由存在性用例。**纯接线**：不改 handler 内部、不加字段、不改语义。理由是这张卡的名字就叫「页面可用」，而不挂路由它不可用。
- **B**：另开一张后端卡先挂路由，本 PR 只交付前端，浏览器验收全部标「阻塞」。代价是 LT-013 的六条手动验收一条也跑不了，链条收不了口。

### Q2 平台枚举怎么到前端？

- **A（推荐，已暂定）**：`packages/core/content/ip-profile/platforms.ts` 放常量，一条 node 测试读 `server/internal/content/ip-profile/account.go` 比对，两边不一致即红。零后端改动，且与 `ip-profile` 守迁移 CHECK 的手法一致。
- **B**：加一个 `GET /api/content-accounts/platforms` 端点，页面运行时取。更「同源」，但这是**新增后端接口**，与「后端零改动」冲突得比 A 更多。
- **C**：页面里硬编码——任务卡明令禁止，列在这里只为说明它被排除了。

### Q3 页面挂在哪？

- **A（推荐，已暂定）**：独立路由 `/{workspaceSlug}/accounts`，照诊断页落位，视觉复用设计系统的设置组合。任务卡点名的就是诊断页的落位方式。
- **B**：作为设置页的一个新标签页。更贴近「设置类页面」的直觉，但要改 `settings-page.tsx` 与 `settings-navigation.ts`（**上游共享文件**），并让账号配置从属于「工作区设置」——而账号是内容域的一等对象，不是工作区的一项偏好。
