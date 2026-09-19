---
description: "Task list for 021 account expression profile — storage and interfaces"
---

# Tasks: 账号表达配置的完整字段（存储与接口）

**Prerequisites**: spec.md、plan.md、contracts/expression-profile.md

**改动文件必须在 plan.md → Source Code 清单内。**

**三项 clarify 已裁决（全部 A）**，并追加一条：最小可开始条件只计 `status=confirmed`。

**本 PR 无界面、无 TS 改动。** 页面是第二个 PR。

---

## Phase 1: Setup

- [ ] T001 确认本机 PostgreSQL 可用并导出 `DATABASE_URL`（`TestMain` 读的是它；设错会 `os.Exit(0)` 打印 `ok` 而一个用例都没跑）
- [ ] T002 记录基线：`go test ./internal/content/... ./internal/handler/ ./cmd/server/`、三项 check

---

## Phase 2: 存储

- [x] T003 新建 `server/migrations/482_content_account_revision_profile.{up,down}.sql`：`ALTER TABLE content_account_revision ADD COLUMN profile jsonb NOT NULL DEFAULT '{}'::jsonb`。**单条语句、无外键、无索引**（jsonb 上没有要走索引的查询）
- [x] T004 改 `content_account_revision.sql`：insert 与四条 select 带上 `profile`。**仍不得有任何 UPDATE / DELETE**（A6 的守卫用例已存在，改完要确认它仍绿）
- [x] T005 跑 `make sqlc` 并核对生成结果；产物**单独提交、不手改**（Windows 无 `make`，已执行 Makefile 内固定的 sqlc v1.31.1 等价命令并确认无漂移）
- [ ] T006 确认**工作区删除清单无需改动**（加列不加表），并跑 `TestWorkspaceDeletionManifestCoversPublicSchema` 证明

---

## Phase 3: 模块层（先写测试）

- [x] T007 [P] **先写** `profile_test.go`：**A3 空值默认 pending 且永不被填内容**（身份/经历/成绩/商业承诺四类各一条）、空值被标 confirmed 时降级、A8 渠道取值受控、A9 非法状态值被拒。确认失败
- [x] T008 [P] **先写** `Readiness` 的判定矩阵：四项齐备为真；缺任一项为假且报出缺哪几项；**A5 pending 有值也不算数**；`weekly_hours = 0` 不算满足。确认失败
- [x] T009 [P] **先写** A7 中性表达：无已确认样本即为真；有样本即为假；它不参与 `Readiness`。确认失败
- [x] T010 新建 `profile.go`：`FieldStatus` / `TextField` / `ListField` / `HoursField` / `ExpressionProfile` / `NormalizeProfile` / `ValidateProfile` / `ProfileReadiness` / `UsesNeutralExpression`
- [x] T011 改 `revision.go`：`Revision` 带 `Profile`；`SetProfile`；**写入时结转另一半**（A4）
- [x] T012 跑 `pnpm check:diagnostics-contract`，确认仍为 `checked 3 landed modules`

---

## Phase 4: HTTP

- [x] T013 **先写** `content_account_profile_test.go`：**A4 只改提示词不清空档案 / 只改档案不清空提示词**（两个方向）、A1 一次确认一版、A2 旧版不变、A9 非法结构 400 且存储不变、A10 越权 404 与不存在同形、**A11 旧版本读出来档案为空且不报错**。确认失败（数据库用例已写并编译；受任务禁用业务数据库约束，本轮未执行）
- [x] T014 新建 `content_account_profile.go`：确认端点 + 读判定端点；400/404 映射沿用既有助手
- [x] T015 `scripts/content-boundaries.json`：新 handler 文件加入 `adapters`，跑 `pnpm check:content-boundaries`

---

## Phase 5: 路由（上游，按第 13 步单列）

- [ ] T016 **先写**：两条新路由进 `content_account_routes_test.go` 的存在性清单。确认失败（清单已写且测试包已编译；数据库型 `TestMain` 本轮不启动，红灯执行待专用合成数据库）
- [x] T017 改 `server/cmd/server/router.go` 挂路由。**独立提交，`upstream:` 开头**
- [ ] T018 **按第 12 步**：在 `content_account_id_test.go` 加一条**穿过真实中间件、账号 id ≠ 工作区 id** 的用例（已写并编译；执行和 URL-helper 变异待专用合成数据库）

---

## Phase 6: Polish

- [x] T019 变异验证三处（contracts §8 的 M1–M3），每处确认对应用例变红、**改完即还原**。变异必须**可编译**
- [ ] T020 跑全部验证：`go test` 报三计数、三项 check、`pnpm typecheck --force`（**说明本 PR 无 TS 改动，跑它只为证明没碰坏**）
- [ ] T021 核对改动文件全部落在 plan.md 清单内；清单外的在 PR 正文单列
- [ ] T022 PR 正文按模板：迁移说明（加列、无外键、无索引及理由）、**上游改动一节**、UI 影响写「无」、「SOP 对应」逐句写明 §3.1 每一句现在可操作到什么程度，**提炼那一句如实写「未开始」**；并写明**第二个 PR 的范围**

---

## Dependencies

```
Setup (T001-T002)
  └─ 存储 (T003→T004→T005 sqlc→T006 清单)
       └─ 模块 (T007,T008,T009 先写 [P] → T010 → T011 → T012)
            └─ HTTP (T013 先写 → T014 → T015)
                 └─ 路由 (T016 先写 → T017 上游 → T018 第 12 步)
                      └─ Polish (T019-T022)
```

**T007/T008/T009/T013/T016 必须先写并确认失败。**

## Implementation Strategy

1. **结转那条先钉死**：加列之后最容易出现的数据丢失就是「改一半清空另一半」，而且它很安静。
2. **「不自动猜测」拆成四条**，因为它是 §3.1 唯一一句明确的禁止，笼统一条测不出是哪一类漏了。
3. **判定矩阵与中性表达分开写**：前者是门槛，后者是标记，混在一起会让「没有样本也能开始」这件事变得不明显。
