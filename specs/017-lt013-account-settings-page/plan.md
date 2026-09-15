# Implementation Plan: 账号设置页面可用（LT-013）

**Branch**: `claude/spec-017-lt013-account-settings-page` | **Date**: 2026-09-15 | **Spec**: `spec.md`
**Base**: `app-main` @ `1e8bccd`

## Summary

把 LT-011 的账号接口与 LT-012 的人设版本接口接到浏览器里：挂路由、加前端两根（`packages/core/content/ip-profile` 与 `packages/views/content/ip-profile`）、加一个工作区路由页。UI 一律复用既有设置组合，不新增控件。UI 行为验收进 `manual-ui-todo.md`，纯逻辑（表单分桶、409 判定、错误对象解析）下沉 core 写 node 环境纯函数测试。

## Constitution Check

| 原则 | 本卡如何满足 |
|---|---|
| **II 不写 UI 单测、不做自动点击** | `packages/views/content/ip-profile/` 下**不放任何 `.test.tsx`**；六条浏览器验收进 `manual-ui-todo.md` |
| **V 迁移规则** | 本卡**零迁移**，不涉及 |
| **VI 前端边界防御** | 所有响应过 `parseWithFallback` + zod；错误对象按接入合同 §3 呈现 `next_action` |
| **VII UI 政策 / SOP 阶段规则** | 只复用 `packages/ui` 与设置组合；PR 的 UI 影响一节只列复用清单 |
| **VIII 范围** | 只做 LT-013；不做版本历史界面、不做实时同步、不做人设库/绑定 |
| **IX 真实执行器禁用** | 本卡不涉及执行器；真实执行器一栏填「未执行」 |
| **X 勾选不等于验收** | 手动条目默认**未勾**，由人在浏览器里跑过才勾 |

**唯一需要说明的偏离**：任务卡写「后端预期零改动」，但核实下来路由未挂载（spec Q1）。暂定值 A 会改 `server/cmd/server/router.go` 一处。这是**接线**不是新能力：不加 handler、不改语义、不碰 `server/internal/content/`。

## Source Code（改动必须限于此清单）

```text
server/cmd/server/router.go               # 改：挂载 /api/content-accounts 八个端点（Q1-A，纯接线）
server/internal/handler/content_account_routes_test.go  # 新增：路由存在性与授权分组用例

packages/core/content/ip-profile/
├── index.ts               # 新增：出口
├── platforms.ts           # 新增：平台常量（与 Go 同源）
├── platforms.test.ts      # 新增：node 环境，读 account.go 比对（Q2-A）
├── contract.ts            # 新增：zod schema、parseAccount、describeAccountError、saveOutcome
├── contract.test.ts       # 新增：node 环境，错误对象解析 / 409 判定
├── form-state.ts          # 新增：按账号分桶的表单状态机（纯函数）
├── form-state.test.ts     # 新增：node 环境，脏输入隔离
└── queries.ts             # 新增：TanStack Query hooks（列表 / 当前人设 / 创建 / 更新 / 保存人设）

packages/views/content/ip-profile/
└── index.tsx              # 新增：页面组件，复用设置组合。**无 .test.tsx**

packages/core/package.json                # 改：加 ./content/ip-profile 出口
packages/views/package.json               # 改：加 ./content/ip-profile 出口
packages/core/api/client.ts               # 改：加账号与人设的客户端方法
packages/views/locales/{en,zh-Hans,ja,ko}/common.json  # 改：新增 contentAccounts 文案段
apps/web/app/[workspaceSlug]/(dashboard)/accounts/page.tsx  # 新增：五行路由页
scripts/content-boundaries.json           # 改：新页面加入 adapters 白名单
specs/017-lt013-account-settings-page/    # 规格、计划、合同、任务、手动清单
```

**明确不动**：`server/internal/content/`（后端逻辑零改动）、`server/migrations/`（零迁移）、`packages/views/settings/`（不改上游共享组件）、`apps/desktop/`、`apps/mobile/`、daemon、上游任何文件。

## Structure Decision

- **两根分工**：`core` 拿 Query 与纯逻辑，`views` 只拿渲染。边界配置本来就只给 `packages/views/content` 批准了 `react` / `@multica/ui` / i18n / navigation / `@multica/views/settings/layout`——**没有 `@tanstack/react-query`**，所以「hooks 在 core」不是风格选择，是边界强制。
- **表单状态是纯函数 + 一个 `useState`**，不进 Zustand：它不跨页面、不需持久化，放进全局 store 只会多一份要清理的东西。按账号分桶的 reducer 在 `form-state.ts`，node 测试锁住「切账号丢草稿」。
- **平台常量在 core**，测试读 Go 源文件比对。`ip-profile` 模块允许依赖 `diagnostics`（`content-boundaries.json` 的 `modules` 段写着 `["workspace-core","diagnostics"]`），所以错误对象的呈现可以复用诊断模块已有的形状而不是另起一套。
- **保存态复用 `SettingsSaveState`**，409 走 `error` 态但换 `errorLabel`（spec 已论证）。

## Complexity Tracking

| 取舍 | 选了什么 | 放弃了什么 |
|---|---|---|
| 路由挂载 | 本 PR 挂（Q1-A） | 「后端零改动」的字面承诺——换来的是这张卡的验收真能在浏览器里跑 |
| 平台枚举 | 常量 + 比对测试（Q2-A） | 运行时接口的「绝对同源」——换来的是零后端接口新增；测试保证两边不会悄悄分叉 |
| 未保存输入 | 切账号即丢弃 | 草稿保留——任务卡明写「不沿用」，保留要额外回答存哪/多久/冲突算谁的 |
| 版本历史 | 只用「读当前 / 写新版」 | 历史界面——不在交付里，做了就是超范围（列出端点仍会挂载，供后续卡用） |
