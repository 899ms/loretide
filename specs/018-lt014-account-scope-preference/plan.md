# Implementation Plan: 账号资料范围偏好（LT-014）

**Branch**: `claude/spec-018-lt014-account-scope-preference` | **Date**: 2026-09-15 | **Spec**: `spec.md`
**Base**: `app-main` @ `77fd852`

## Summary

在账号 `settings` 的 `loretide.scope` 键下存 `local` / `web` / `all`，默认 `all`，读时补默认而不回写，写时服务端合并以保住其余键。加一条专用端点、一份 core 常量与助手，以及一条断言：诊断运行与复现不动账号存着的偏好。**零迁移、零界面。**

## Constitution Check

| 原则 | 本卡如何满足 |
|---|---|
| **II 不写 UI 单测** | 本卡无界面，无 `.test.tsx` |
| **V 迁移规则** | **零迁移**——偏好是 `settings` jsonb 的一个键（Q1-A） |
| **VI 前端边界防御** | core 的读取助手对 `null` / 非对象 / 非法值全部降级到默认，不抛 |
| **VII UI 政策** | 不涉及 |
| **VIII 范围** | 只做接口与 core 类型；开始界面属 EP-04；账号页不加控件（Q3-A） |
| **IX 真实执行器禁用** | 不涉及；真实执行器一栏填「未执行」 |
| **X 勾选不等于验收** | 无手动条目；所有验收都有自动用例 |

**上游改动**：预期只有 `server/cmd/server/router.go` 一处接线（新增一条子路由）。按工作流第 13 步单独提交、PR 正文单列。

## Source Code（改动必须限于此清单）

```text
server/internal/content/ip-profile/
├── scope.go               # 新增：Scope 类型、Scopes、ValidateScope、ScopeSettingsKey、ScopeOf、withScope
├── scope_test.go          # 新增：默认值、非法值、读不改、合并保留其他键
└── service.go             # 改：SetScope；Get/List 读时补默认；Create/Update 校验 settings 里的 scope

server/internal/handler/
├── content_account_scope.go       # 新增：PUT /{id}/scope 端点 + 400/404 映射
└── content_account_scope_test.go  # 新增：默认、设置、隔离、非法值、越权、PATCH 绕过、运行不回写

server/cmd/server/
├── router.go                          # 改（上游）：挂 /{id}/scope
└── content_account_routes_test.go     # 改：新路由加入存在性清单（第 12 步）

packages/core/content/ip-profile/
├── scope.ts               # 新增：CONTENT_SCOPES、DEFAULT_SCOPE、isContentScope、getAccountScope、withAccountScope
├── scope.test.ts          # 新增：node 环境；读 scope.go 比对受控集合；默认与降级
├── contract.ts            # 改：账号 schema 的 settings 已是 record，加 scope 读取的导出
└── index.ts               # 改：导出 scope

specs/018-lt014-account-scope-preference/   # 规格、计划、合同、任务
```

**明确不动**：`server/migrations/`（零迁移）、`server/internal/content/diagnostics/`（快照字段已存在，只加断言不改形状）、`packages/views/`（无界面）、`apps/`、daemon。

## Structure Decision

- **形状逐条照抄 `#63` 的工作区时区**：`validateTimezoneSetting`（写时校验）+ `timezoneFilled`（读时补默认、只作用于响应）。两个功能在产品上无关，但「settings 里的一个受控键」这个问题是同一个，另发明一套只会让下一个人要读两遍。
- **合并在服务端**：`SetScope` 读账号 → `maps.Copy` 出一份新 blob → 带完整 blob 调 `UpdateAccount`。因为 `Patch.Settings` 是整体替换，合并交给客户端就等于交给「记得合并」。
- **校验有两个入口，一套规则**：专用端点与 `PATCH /{id}` 的 settings 都过 `ValidateScope`，否则前者形同虚设（FR-007）。
- **不新增快照字段**：`Snapshot.Scope` / `Preference` 已存在；本卡加的是账号侧的「不回写」断言，与 `reproduce_preference_test.go` 对齐而不是并列。

## Complexity Tracking

| 取舍 | 选了什么 | 放弃了什么 |
|---|---|---|
| 存储位置 | `settings` 的一个键 | 专用列的可索引性——今天没有任何查询按范围过滤 |
| 写入接口 | 专用端点 + 服务端合并 | 「零新增路由」——换来的是客户端不可能忘记合并，且并发改不同键不会互相覆盖 |
| 并发 | 后写的赢 | 版本号与 409——丢一次单选框的选择，代价与丢一次创作确认不是一个量级 |
| 历史 | 不记 | 偏好变更史——运行快照的 `saved_preference` 已经逐次记着了 |
