# Implementation Plan: 品牌账号与平台配置（LT-011）

**Branch**: `claude/spec-015-lt011-account-platform-config` | **Date**: 2026-09-15 | **Spec**: [spec.md](./spec.md)

## Summary

给品牌装上**账号**这个实体：一张表、三类操作（创建 / 读取 / 更新）、受控平台枚举，每个操作都经 LT-010 的授权助手判定。

**这是 LT-010 助手的第一个真实消费者**——它到底好不好用，本卡会给出答案而不是断言。

三件容易漏的事，计划把它们当成一等任务而不是收尾检查：

1. **工作区删除要改两处**（清单 + 真删语句）。`#45` 只漏了这个，代价是 LT-009 与 LT-010 两次交付的基线都是红的。
2. **sqlc 生成物单独提交**，不手改。
3. **接入合同**：`ip-profile` 是第三个落地模块，E1/E2/E3 由真实接入满足。

## Technical Context

**Language/Version**: Go 1.26

**Primary Dependencies**: 现有依赖，不新增。模块只用标准库 + `content/diagnostics` + `content/workspace-core`

**Storage**: PostgreSQL。**1 张新表 + 1 个并发索引**（独立文件）。**无外键、无级联**

**Testing**: Go `testing`；`internal/testutil` 的 `dbfx` / `testutil.Call`

**Project Type**: Existing monorepo. Do not re-derive this.

**Constraints**:
- 列类型 `text`，与 `content_diagnostic_run` 等同层表一致（Q2-A）
- 平台：Go 枚举为准 + 库 `CHECK` 兜底（Q3-A）；`CHECK` 不是外键/级联
- **不碰上游 Multica 代码与 `server/internal/daemon/`**
- **无界面改动**；不写 UI 单测
- 手写文件若超 5 个，按任务卡拆「存储」与「API」两个子任务——**本计划为 5 个**（见下），不拆

**Scale/Scope**: 3 个迁移文件（表 / 索引 / down）、1 个查询文件、1 个新模块目录（3 个手写文件）、2 处删除路径改动、sqlc 生成物

## Constitution Check

| 原则 | 判定 | 说明 |
|---|---|---|
| I. CLAUDE.md 权威 | PASS | 遵循迁移规则、测试分层、包边界 |
| II. 无 UI 单测 | PASS | 本卡无界面改动 |
| III. 模块边界 | PASS | `ip-profile` 已登记且已声明依赖 `workspace-core` + `diagnostics`——正是本卡所需，无需改 `content-boundaries.json` |
| IV. 状态分离 | PASS（不适用） | 纯服务端 |
| V. 数据库 | PASS | **无外键、无级联**；索引用独立文件 `CREATE INDEX CONCURRENTLY`，一文件一语句；`CHECK` 是列约束，不在禁止之列 |
| VI. 响应解析 | PASS | 新增端点，响应形状写入契约；无前端消费者（页面在 LT-013） |
| VII. UI 复用 | PASS（不适用） | 无界面改动。SOP 阶段界面规则记于 spec 的 UI Impact |
| VIII. 范围 | PASS | 只做最小切片；§3.1 完整字段与「Agent 提炼候选档案」留给 LT-012 之后，Assumptions 写明 |
| IX. 真实执行器禁用 | PASS | 不涉及 |
| X. 勾选不等于验收 | PASS | 接入合同「通过检查 ≠ 接入合格」在 PR 正文如实写明 |

**无停止条件触发。**

## Project Structure

### Source Code（改动必须限于此清单）

```text
server/migrations/
├── 477_content_account.up.sql            # 新增：账号表（无外键、无级联；平台 CHECK）
├── 477_content_account.down.sql
└── 478_content_account_workspace_idx.up.sql   # 新增：CREATE INDEX CONCURRENTLY，单语句单文件
    478_content_account_workspace_idx.down.sql

server/pkg/db/queries/
├── content_account.sql                   # 新增：Create / Get / List / Update
└── workspace_delete.sql                  # 改：删除工作区时一并删账号（真删）

server/pkg/db/generated/                  # sqlc 产物，`make sqlc` 生成，单独提交、不手改

server/internal/content/ip-profile/
├── account.go                            # 新增：Platform 枚举、校验、Account 形状
├── service.go                            # 新增：Create / Get / List / Update，每个操作经 workspace-core 判定
└── account_test.go                       # 新增：枚举、校验、隔离（E3 引用 diagnostics）

server/internal/handler/
├── content_account.go                    # 新增：HTTP 接入（adapters 白名单需登记）
└── content_account_test.go               # 新增：四条核心负例 + 正例

server/internal/handler/
└── workspace_delete_manifest_test.go     # 改：新表登记进清单

scripts/content-boundaries.json           # 改：handler 的新文件加入 adapters 白名单
```

**明确不动**：上游 workspace / user / member、`server/internal/daemon/`、`packages/`、CI、`content-boundaries.json` 的 `modules` 段（`ip-profile` 已登记）。

**手写文件计数**：`account.go`、`service.go`、`account_test.go`、`content_account.go`、`content_account_test.go` = **5 个**（迁移与 SQL 不算手写代码文件，sqlc 产物为生成物）。**恰好不超阈值，不拆子任务**；若实施中超出，按任务卡拆「存储」与「API」。

**Structure Decision**：

- 模块目录 **`ip-profile`**，与 `content-boundaries.json` 的注册键逐字一致——检查器用目录名做模块名，改名等于新增未登记模块。Go 包名 `ipprofile`。
- 授权**不在模块内做**：模块接收已判定的空间与主体，HTTP 层负责调用 `workspacecore.Authorize` 并按 `RefusalStatus` / `RefusalBody` 作答。这样模块不依赖 HTTP，也不重复实现判定。
- `content_account.go` 需加入 `content-boundaries.json` 的 `adapters` 白名单——handler 不在 content 根下，import content 模块必须经该白名单（`content_diagnostics.go` 同理）。

## Complexity Tracking

无违规。

两处取舍：

1. **平台用库 `CHECK` 兜底，代价是加平台要迁移。** 换来的是绕过 API 的写入也落不进脏值，而这张表会被后续模块当分组键读（Q3 裁决 A）。
2. **本卡不提供删除账号接口。** 任务卡只要求创建 / 读取 / 更新；账号停用与删除的语义（历史内容如何归属）是产品决定，不在本卡自行决定。
