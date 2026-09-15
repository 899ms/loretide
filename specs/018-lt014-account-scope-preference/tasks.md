---
description: "Task list for 018 account scope preference (LT-014)"
---

# Tasks: 账号资料范围偏好（LT-014）

**Prerequisites**: spec.md、plan.md、contracts/account-scope.md

**改动文件必须在 plan.md → Source Code 清单内。**

**三项 clarify 按推荐值暂定**（settings 键 / 专用端点 + 服务端合并 / 账号页不加控件），主任务可随时改判。

**零迁移**：偏好是 `settings` jsonb 的一个键。

---

## Phase 1: Setup

- [x] T001 确认本机 PostgreSQL 可用并导出 `DATABASE_URL`（注意：`TestMain` 读的是 `DATABASE_URL`，不是任务卡写的 `MULTICA_TEST_DATABASE_URL`；设错会 `os.Exit(0)` 打印 `ok` 而一个用例都没跑）
- [x] T002 记录基线：`go test ./internal/handler/ ./cmd/server/`、三项 check、`pnpm lint` 的既有 error 数，以便区分回归与基线

---

## Phase 2: 模块层（先写测试）

- [x] T003 [P] **先写** `server/internal/content/ip-profile/scope_test.go`：**A1 默认 `all`**、A12 存着非法值也读作 `all`、A5 非法值被拒（含空串、大小写、数字）、**A4 合并保留其他键**、A2 读不改入参。确认失败
- [x] T004 新建 `scope.go`：`Scope` 类型、`Scopes`（`local`/`web`/`all`）、`ScopeSettingsKey = "loretide.scope"`、`DefaultScope = "all"`、`ValidateScope`、`ScopeOf(settings)`（读时补默认）、`withScope(settings, scope)`（合并，返回新 map）
- [x] T005 改 `service.go`：加 `SetScope`（校验 → 读账号 → 合并 → Update → 审计）；`Get` / `List` 读时补默认；`Create` / `Update` 校验 `settings` 里的 scope（FR-007）
- [x] T006 跑 `pnpm check:diagnostics-contract`，确认仍为 `checked 3 landed modules`（本卡不新增模块目录）

---

## Phase 3: HTTP 与负例

- [x] T007 **先写** `server/internal/handler/content_account_scope_test.go`：A1 新建账号读到 `all` **且库里无该键**、A3 设置后读到设置值、**A7 改 A 不影响 B**、A5 非法值 400 诊断错误对象且存储不变、**A6 经 `PATCH settings` 的非法值同样 400**、A8 越权 404 与不存在逐字节相同。确认失败
- [x] T008 新建 `server/internal/handler/content_account_scope.go`：`SetAccountScope` 端点；400/404 映射沿用 `accountInputError` / `writeAccountRefusal`
- [x] T009 `scripts/content-boundaries.json`：新 handler 文件加入 `adapters`；跑 `pnpm check:content-boundaries`

---

## Phase 4: 路由（上游，按第 13 步单列）

- [x] T010 **先写**：把 `PUT /api/content-accounts/{id}/scope` 加入 `server/cmd/server/content_account_routes_test.go` 的存在性清单（第 12 步）。确认失败
- [x] T011 改 `server/cmd/server/router.go`：在既有 `/api/content-accounts/{id}` 子路由内加一行。**独立提交，`upstream:` 开头**
- [x] T012 确认 T010 转绿，且 `TestMountingContentAccountsLeftNeighbouringRoutesAlone` 仍绿

---

## Phase 5: 「不回写偏好」的账号侧断言

- [x] T013 **先写** A10：账号偏好设成一个**夹具永不产生的值**，跑一次诊断运行与一次复现，断言账号 `settings` 里的值**逐字节不变**。手法与 `diagnostics/reproduce_preference_test.go` 一致——用夹具会产生的值（如 `all`）断言等于没断言
- [x] T014 确认「运行快照可表达固定值」：断言 `Snapshot.Scope` 可以被钉成与账号偏好**不同**的值且互不干扰。**不新增快照字段**（FR-012）

---

## Phase 6: core 类型

- [x] T015 [P] **先写** `packages/core/content/ip-profile/scope.test.ts`（`// @vitest-environment node`）：**A11 读 `scope.go` 比对受控集合**、默认 `all`、`null`/非对象/数组/非法值全部降级、`withAccountScope` 保留其他键。确认失败
- [x] T016 新建 `scope.ts`；改 `index.ts` 导出
- [x] T017 `contract.ts`：账号 schema 已有 `settings`，确认 scope 读取路径经得住脏数据（不新增 schema 字段）

---

## Phase 7: Polish

- [x] T018 变异验证三处（contracts M1–M3），每处确认对应用例变红、**改完即还原**。变异必须**可编译**
- [x] T019 跑全部验证：`go test` 三计数、core vitest 逐文件、`pnpm typecheck --force` 报非缓存任务数、三项 check、`pnpm lint` 对比基线
- [x] T020 核对改动文件全部落在 plan.md 清单内；清单外的在 PR 正文单列
- [x] T021 PR 正文按模板：**「上游改动」一节**（router.go 一行接线）、**存储所有权**（`content_account` 归 `ip-profile`）、**零迁移说明**、「SOP 对应」一节

---

## Dependencies

```
Setup (T001-T002)
  └─ 模块 (T003 先写→T004→T005→T006)
       └─ HTTP (T007 先写→T008→T009)
            └─ 路由 (T010 先写→T011 上游→T012)
                 └─ 不回写断言 (T013 先写→T014)
                      └─ core (T015 先写→T016→T017)
                           └─ Polish (T018-T021)
```

**T003 / T007 / T010 / T013 / T015 必须先写并确认失败**：否则无法区分「实现对」与「测试跟着实现走」。

## Implementation Strategy

1. **先把「读不回写」钉死**，它是本卡最容易悄悄破坏的一条——补默认的代码离「顺手存一下」只有一行之遥。
2. **两个校验入口一套规则**：专用端点写完立刻补 `PATCH` 路径，否则中间存在一个可绕过的窗口。
3. **Phase 5 单独成段**，因为它跨模块（账号 + 诊断），混在别的段里会被当成附带项。
