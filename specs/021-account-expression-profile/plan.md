# Implementation Plan: 账号表达配置的完整字段（SOP §3.1）—— 存储与接口

**Branch**: `claude/spec-021-account-expression-profile` | **Date**: 2026-09-15 | **Spec**: `spec.md`
**Base**: `app-main` @ `5d6d47f`

## Summary

给 `content_account_revision` 加一列 `profile jsonb`，让**一条 revision 承载整份表达档案**；每个字段带 `{value, status}`；加一个确认端点、一个只读的「能否开始」判定。**本 PR 不含界面**（第二个 PR）。

## Constitution Check

| 原则 | 本卡如何满足 |
|---|---|
| **II 不写 UI 单测** | 本 PR 无界面 |
| **V 迁移规则** | 一次 `ALTER TABLE ... ADD COLUMN`，**独立迁移文件、单条语句、无外键、无级联**；**不加索引**（jsonb 列上没有查询要走索引） |
| **VI 前端边界防御** | 本 PR 无前端；第二个 PR 负责 |
| **VII UI 政策** | 不涉及 |
| **VIII 范围** | 只做字段、状态、确认、判定；**不调模型**、不做文件上传、不做页面 |
| **IX 真实执行器禁用** | 提炼**不实现**、不伪造结果；真实执行器一栏填「未执行」 |
| **X 勾选不等于验收** | 界面项留给第二个 PR 的 manual-ui-todo |

## Source Code（改动必须限于此清单）

```text
server/migrations/
└── 482_content_account_revision_profile.{up,down}.sql   # 加 profile jsonb 列，单条语句

server/pkg/db/queries/
└── content_account_revision.sql        # 改：insert/select 带上 profile

server/pkg/db/generated/                # sqlc 产物，单独提交、不手改

server/internal/content/ip-profile/
├── profile.go        # 新增：字段类型、ExpressionProfile、校验、Readiness、中性表达
├── profile_test.go   # 新增：状态、不猜测四类、判定矩阵、中性表达
└── revision.go       # 改：Revision 带 Profile；SetProfile；写入时结转另一半

server/internal/handler/
├── content_account_profile.go       # 新增：确认端点 + 读判定
└── content_account_profile_test.go  # 新增：负例与结转

server/cmd/server/
├── router.go                        # 上游：挂两条路由（按第 13 步单列）
├── content_account_routes_test.go   # 改：新路由进存在性清单
└── content_account_id_test.go       # 改：穿过真实中间件的用例（第 12 步）

specs/021-account-expression-profile/  # 规格、计划、合同、任务
```

**明确不动**：`packages/`（无 TS 改动，第二个 PR 才有）、`apps/`、`workspace_delete.sql` 与删除清单（**加列不加表**，清单无需改动，且已有用例守着）、daemon、`ip-profile` 之外的模块。

## Structure Decision

- **一条 revision 一份档案**：`profile jsonb` 挂在既有版本表上，LT-012 的「只插不改不删 / 旧版可读 / 并发有界重试」原封不动继承，运行仍然只钉一个 `revision_id`。
- **写入结转另一半**：只改提示词时把当前版本的 `profile` 带过去，只改档案时把 `persona_prompt` 带过去。否则「确认档案之后改一下提示词」会把刚确认的档案清空——这是加列之后最容易出现、也最安静的一种数据丢失，单独有用例。
- **空值强制 `pending`**：客户端把一个空字段标成 `confirmed` 时，服务端把它降回 `pending`。这只**下调**状态、永不写入内容，与「不自动猜测」同向；反过来 400 会让整份档案因为一个无关的空字段而保存失败。
- **`Readiness` 不落库**：它是从档案算出来的。存一份就是第二份真相，而档案一改它就过期。
- **渠道取值复用 `Platforms`**：不另立一份平台清单（`#71` 的受控集合已经有 core 侧的比对测试守着）。

## Complexity Tracking

| 取舍 | 选了什么 | 放弃了什么 |
|---|---|---|
| 档案存储 | 既有版本表加一列 jsonb（Q1-A） | 数据库层的结构校验——由 Go 受控类型兜底，与 `settings.loretide.scope` 同样的取舍 |
| 状态 | 字段内联 `{value,status}`（Q2-A） | 一个独立的已确认清单——它能表示出自相矛盾的状态 |
| 空值被标确认 | 降级为 `pending` | 400——会让一份档案因为一个无关空字段整体保存失败 |
| 可投入时间为 0 | **不计入**最小可开始条件 | 「0 也是一个明确的值」——确认「每周 0 小时」等于说没有时间开始，把它算作满足条件读起来是荒谬的。这是一处**解释**，PR 正文单列 |
