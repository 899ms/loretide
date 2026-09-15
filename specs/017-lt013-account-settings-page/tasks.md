---
description: "Task list for 017 account settings page (LT-013)"
---

# Tasks: 账号设置页面可用（LT-013）

**Prerequisites**: spec.md、plan.md、contracts/account-settings-ui.md

**改动文件必须在 plan.md → Source Code 清单内。**

**三项 clarify 已裁决（全部 A）**，并追加三条要求（路由用例覆盖八条 + 断中间件拒绝、不抄插件门控、本 PR 不加侧栏入口）。

**constitution 原则 II**：`packages/views/content/ip-profile/` 下**不写任何 `.test.tsx`**；UI 行为进 `manual-ui-todo.md`。

---

## Phase 1: 路由挂载（补 #71/#73 的盲点）

- [x] T001 **先写** `server/internal/handler/content_account_routes_test.go`：逐条断言八个端点在路由表上存在（不是 404-no-route），并断言**未登录**与**非成员**经 `RequireWorkspaceMember` 被拒。确认失败——今天它必须红，因为端点根本没挂
- [x] T002 改 `server/cmd/server/router.go`：在既有 `RequireWorkspaceMember` 分组内加 `r.Route("/api/content-accounts", ...)`，挂八个端点。**纯接线**：不改 handler、不加字段、不改语义
- [x] T003 确认 T001 转绿，且 `go test ./internal/handler/` 其余用例不受影响

---

## Phase 2: core 纯逻辑（先写测试）

- [x] T004 [P] **先写** `packages/core/content/ip-profile/platforms.test.ts`（`// @vitest-environment node`）：读 `server/internal/content/ip-profile/account.go`，解析 `Platforms` 的值，与常量逐值比对。确认失败
- [x] T005 新建 `platforms.ts`：8 个平台常量 + 类型。注释写明单一事实来源在 Go，改动需同时改迁移
- [x] T006 [P] **先写** `form-state.test.ts`（node）：**A1 切账号不沿用草稿**（桶里只有 A 时 `selectDraft(state,B)` 返回 B 的服务端值）、A2 保存成功后清桶、编辑只影响一个桶。确认失败
- [x] T007 新建 `form-state.ts`：`selectDraft` / `editDraft` / `discardDraft`，纯函数，无 React
- [x] T008 [P] **先写** `contract.test.ts`（node）：**A3 409 判定为 `conflict`**、**A4 非 409 为 `failed` 且带 `next_action`**、**A5 错误体不合 schema 时降级**、账号/版本响应经 `parseWithFallback` 解析且脏数据不抛。确认失败
- [x] T009 新建 `contract.ts`：zod schema（account / accountList / revision）、`parseAccount*`、`describeAccountError`、`saveOutcome`（判别联合 `saved|conflict|failed`）
- [x] T010 新建 `queries.ts`：TanStack Query hooks（列表、当前人设、创建、更新、保存人设）。**服务端状态全在 Query**，workspace 作用域的 key 带 `wsId`
- [x] T011 新建 `index.ts` 出口；`packages/core/package.json` 加 `./content/ip-profile`
- [x] T012 `packages/core/api/client.ts` 加账号与人设方法，沿用 `contentDiagnosticRequest` 的写法

---

## Phase 3: views 与路由页（不写 UI 单测）

- [x] T013 新建 `packages/views/content/ip-profile/index.tsx`：账号列表 + 创建 + 平台/显示名 + 人设提示词 + 保存态。**只用 `packages/ui` 既有组件与 `@multica/views/settings/layout` 的组合**；对照 `workspace-tab.tsx`。**不读任何插件开关**
- [x] T014 `packages/views/package.json` 加 `./content/ip-profile` 出口
- [x] T015 四语言文案：`packages/views/locales/{en,zh-Hans,ja,ko}/common.json` 加 `contentAccounts` 段；跑 `packages/views/locales/parity.test.ts`
- [x] T016 新建 `apps/web/app/[workspaceSlug]/(dashboard)/accounts/page.tsx`，照诊断页五行写法
- [x] T017 `scripts/content-boundaries.json`：新页面加入 `adapters`；跑 `pnpm check:content-boundaries`
- [x] T018 确认 `packages/views/content/ip-profile/` 下**没有** `.test.tsx`（A9）

---

## Phase 4: 手动清单

- [x] T019 新建 `specs/017-lt013-account-settings-page/manual-ui-todo.md`，至少六条：创建 A/B 账号、切换不沿用输入、刷新保持、保存三态、双账号提示词互不影响、插件关闭仍可用。**默认全部未勾**
- [x] T020 手动清单加一条**待裁定项**：「侧栏是否需要入口」——本 PR 不加，留给主任务定

---

## Phase 5: Polish

- [x] T021 变异验证三处（contracts M1–M3），每处确认对应用例变红、**改完即还原**。变异必须**可编译**
- [x] T022 跑全部验证：core 纯函数 vitest 逐文件、locale parity、`go test ./internal/handler/` 三计数、`pnpm typecheck --force` 报非缓存任务数、三项 check
- [x] T023 核对改动文件全部落在 plan.md 清单内；清单外的在 PR 正文单列
- [x] T024 PR 正文按模板：**单列「此前端点未挂载」**、UI 影响只列复用组件清单、加「SOP 对应」一节写明 docs/01 §3.1 哪些句子已可操作、哪些仍缺

---

## Dependencies

```
路由 (T001 先写→T002→T003)
  └─ core (T004,T006,T008 先写 [P] → T005,T007,T009 → T010 → T011,T012)
       └─ views (T013→T014→T015→T016→T017→T018)
            └─ 手动清单 (T019,T020)
                 └─ Polish (T021-T024)
```

**T001 必须先红**：它是这张卡存在的证据——`#71` / `#73` 合并时没有任何用例发现端点没挂载。
**T004/T006/T008 必须先写并确认失败**：否则无法区分「实现对」与「测试跟着实现走」。

## Implementation Strategy

1. **路由先挂**，否则后面每一步都在为一个调不通的接口写代码。
2. **纯逻辑先于渲染**：三条变异全部落在 core，渲染层不承担可测性责任。
3. **手动清单单独成段**，因为它是 UI 验收的**唯一**载体，混在 Polish 里会被当成收尾杂项。
