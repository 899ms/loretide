---
description: "Implementation plan for 029 operating rules — SOP §3.2 brand-level settings"
---

# Implementation Plan: 运营规则——品牌级设置（029）

**Spec**: [spec.md](./spec.md) ｜ **Contract**: [contracts/operating-rules.md](./contracts/operating-rules.md)

**Status**: **三条 clarify 已裁决**（主控 2026-09-20，PR #192 评论）。**没有新表、没有迁移**，代码落 `workspace-core`，登记表不改。交付拆**三个 PR**。

## 照抄什么，不发明什么

五处照抄既有做法：

1. **settings JSONB 键 + 写时校验 + 读时填默认且不回写** 照 `loretide.timezone`（LT-009）与 `loretide.auto_precheck`（019）。
2. **「存过没有」与「存的是什么」分成两个函数** 照 019 的 `hasStoredAutoPrecheck` / `isAutoPrecheckEnabled`。本卡每个可缺省的数值字段都要有这一对。
3. **自己的合并端点，服务端合并** 照 LT-014 的 `PUT /api/content-accounts/{id}/scope`——**不照** LT-009 / 019 的前端合并（理由见合同第 4 节）。
4. **三态而不是布尔** 照 025 的 `version_match{matched,differs,unknown}` 与 027 的「空 ≠ 0」。观察时点是第三次。
5. **可选 render 插槽 + 适配器注入** 照 #161 给 `work-editor` 加 `renderArtifactExtras`、#184 给 `review-delivery` 加 `renderPublicationExtras`。

一处**不照抄**：**不在前端合并 settings**。LT-009 与 019 都是 `withAutoPrecheck(workspace.settings, value)` 再整体 PUT 回去，中间隔一次网络往返，两个人同时改会互相抹掉。本卡四项设置是一张表单里填的，冲突面比一个开关大得多。

## Technical Context

**Language/Version**: Go 1.26（`server/`）、TypeScript 5 strict（`packages/core`、`packages/views`）

**Storage**: **没有新表、没有迁移。** 工作区 `settings` 的 `loretide.operating_rules` 一个键；账号 `settings` 的 `loretide.homepage` 一个键。

**Testing**: `go test ./internal/content/workspace-core/`、`./internal/handler/`（两套 db-suites 按 `docs/development/testing-database-suites.md`）；`packages/core/*.test.ts`（node 环境）。**无 UI 单测。**

**Constraints**: 不存凭据、不访问链接、不调度、不提醒、不发明默认天数、不提供成员选择（包括禁用的）、不调模型、不起执行器。页面只挂既有组件。

**Scale**: 一个品牌八个渠道、四项设置。一个 JSONB 键放得下，且远没到需要索引的量级。

## Constitution Check

| 原则 | 状态 | 说明 |
|---|---|---|
| II. 不写 UI 单测 | 通过 | 判定与派生进 Go / core node 测试；界面进 `manual-ui-todo.md` |
| III. 模块边界 | 通过 | `workspace-core` 已在册，依赖只有 `diagnostics`。views 落 `packages/views/content/workspace-core/`，可用的上游 import 里**已经包含** `@multica/core/workspace` 与 `@multica/views/settings/layout`（见 `scripts/content-boundaries.json` 的 `upstreamImports`）——登记表一个字不改 |
| V. 无外键、CONCURRENTLY 索引、单语句迁移 | **不适用** | 没有迁移。**PR 正文要写明为什么不适用**——「没改」和「忘了改」在 diff 上长得一样 |
| VI. zod + `parseWithFallback` | **需注意** | `cadence` 与 `observation` 的值要能表达「未设」。core 侧用 `number \| undefined` 加一个 `stored` 判定，**不能**用 `?? 0` |
| VIII. 范围纪律 | **需注意** | PR 3 改的是 `feedback-learning`——这是裁决给的范围，不是我扩的。它单独一个 PR，正是为了让这次越界看得见 |
| IX. 真实执行器保持禁用 | 通过 | 本卡不调模型；渠道说明只是被读出来，怎么用是 EP-04 / EP-08 |
| X. 打勾不是验收 | 通过 | 界面项一律记「未执行」 |

## Project Structure

```text
server/
├── internal/content/workspace-core/
│   ├── operating_rules.go        # 键名、受控集、读写、三个「存过没有」函数
│   ├── observation.go            # ObservationDue 三态派生
│   ├── operating_rules_test.go   # 矩阵与「未设 ≠ 零值」（无数据库）
│   ├── observation_test.go       # 三态逐条
│   └── guards_test.go            # 不存凭据 / 不外发 / 不调度 / 无写死天数 / 无成员选择
├── internal/handler/
│   ├── content_operating_rules.go      # GET/PUT /api/operating-rules（无路径参数）
│   ├── content_account_homepage.go     # PUT /api/content-accounts/{id}/homepage（有路径参数）
│   └── *_test.go
└── cmd/server/router.go                # 三条路由 ┐ upstream: 单列提交，既有行 0 删除

packages/core/workspace/
├── operating-rules.ts         # 镜像键名与受控集；number | undefined，不 ?? 0
├── operating-rules.test.ts    # node 环境；受控集读 Go 源文件对表
└── homepage.ts                # loretide.homepage 的读写与 URL 判定

packages/views/content/workspace-core/
└── index.tsx                  # 四个区块（页面 PR），只挂既有组件

packages/views/settings/components/
├── workspace-tab.tsx          # 加一个可选 render 插槽 ┐ upstream: 同一个提交
└── settings-page.tsx          # 透传该插槽              ┘ 两个文件各 ~3 行，全是新增

apps/web/app/[workspaceSlug]/(dashboard)/settings/page.tsx   # 适配器注入（Loretide 文件）

specs/029-operating-rules/
├── contracts/operating-rules.md
└── manual-ui-todo.md          # 页面 PR 时新建
```

## 页面落点：为什么是插槽，为什么要动两个上游文件

裁决要求「沿用 `workspace-tab.tsx` 那条路径」且「**能注入优先**」。这两条合起来的唯一做法：

```
apps/web/.../settings/page.tsx        ← Loretide 文件，注入
   └─ SettingsPage renderWorkspaceExtras={...}      ← 上游，加 1 个 prop + 透传
        └─ WorkspaceTab renderOperatingRules={...}  ← 上游，加 1 个 prop + 1 次调用
             └─ OperatingRulesSections              ← Loretide 文件，区块正文全在这里
```

**代价要说清：这样会动两个上游文件，而把区块直接写进 `workspace-tab.tsx` 只动一个。** 选两个，是因为那一个的代价是两百行 Loretide UI 长在上游文件里——019 加一个开关是二十行，尚可；四个区块不是。两个 prop 加起来约六行，全是新增，`git diff -w` 上既有行零删除。

**已经存在的 `extraDeviceTabs` 不能用**：它的 prop 名与注释都写着 "Device settings supplied by the desktop platform"，而且它加的是一个**新 tab**，不是工作区分区里的一节。用它就等于把运营规则从「和时区并排」挪成「另起一页」，与裁决相反。

## 三个 PR 的分界

| PR | 内容 | 验收口径 |
|---|---|---|
| **PR 1 存储与接口** | `workspace-core` 的两个新文件、三条端点、core 的 schema 与 URL 判定、路由（`upstream:` 单列） | 「未设 ≠ 零值」各一条、三态逐条、守卫六条、越权同形、无迁移（正文写明）、`/homepage` 的参数 ≠ 上下文用例 |
| **PR 2 页面** | 四个区块、插槽与适配器注入（`upstream:` 单列）、四语言、`manual-ui-todo.md` | 只挂既有组件、无 UI 单测、界面写明「配模板 ≠ 可交付」与「观察时点尚未接入待补录」 |
| **PR 3 接进 027** | `NeedsRegistration` 加第三个条件、守卫改写、变异验证 | `unknown` 走 `passed` 分支的两条用例、守卫改写后的变异（写死 14 变红） |

**PR 3 不与 PR 1 合并。** 它改的是另一个模块里一条刻意写下来的守卫，值得自己的一次审查——这也是裁决把它单列出来的原因。

## 风险与对策

| 风险 | 对策 |
|---|---|
| **「未设」被某一层折成 0** | 与 027 的「空 ≠ 0」同一件事，第三次出现。三处各一条用例：Go 读写、core 解析、界面展示。`ReadCadence` 返回 `(value, stored)` 两个值而不是一个 `*int`，让「忘了看第二个返回值」在 Go 里至少是显眼的 |
| **PR 3 把 `unknown` 当成 `not_yet`** | 整个 PR 3 最容易做反的一处：没有发布时间的记录会从工作台静悄悄消失，而它们恰恰最需要有人看一眼。合同第 8 节写死 `unknown` 走 `passed` 分支，SC-015 两条用例 |
| **守卫改写之后变空** | 从「禁止时间比较」改成「禁止写死的时间比较」，判断变复杂了就可能写漏。SC-014 的变异（把天数写成 `14`）是唯一能证明它没变空的东西 |
| 上游文件被 gofmt / prettier 重排 | 025 因此返工过一次。两个 prop 各自放在参数列表末尾，`git diff -w` 必须只有新增行 |
| 八个平台里四个不能交付 | 界面写明（FR-012a），并有一条用例断言模板能建在不能交付的渠道上——这是**正常**，不是要拦的事 |
| 前端合并 settings 导致互相抹掉 | 不做前端合并。服务端合并端点（合同第 4 节），照 LT-014 |
| 「一个 JSONB 键塞结构化数据」长大 | 裁决已知此代价（Q1=A 的选项表里写了）。若将来 `templates` 要结构化字段，那就是把这一个键拆成一张表，届时是一张新卡，不是本卡的返工 |
