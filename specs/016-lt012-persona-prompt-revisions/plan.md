# Implementation Plan: 账号人设提示词的配置版本（LT-012）

**Branch**: `claude/spec-016-lt012-persona-prompt-revisions` | **Date**: 2026-09-15 | **Spec**: [spec.md](./spec.md)

## Summary

给 `content_account` 加上**人设提示词的版本历史**：一张只插不改不删的版本表，按账号独立计数，运行以 `revision_id` 钉住一个确定快照。

**本卡不新增模块目录**——版本表归 `ip-profile`（与 `content_account` 同模块），所以接入合同仍是 `checked 3 landed modules`，不是 4。

三件计划当成一等任务、不留到收尾的事：

1. **唯一约束走独立迁移文件的 `CREATE UNIQUE INDEX CONCURRENTLY`**（Q3 裁决），不写在建表语句里。
2. **工作区删除两处都改**（真删 + 登记）——`#45` 的教训，`#71` 已照做一次。
3. **并发用例是真并发**：起两个 goroutine 同时写，而不是顺序写两次假装并发。

## Technical Context

**Language/Version**: Go 1.26

**Primary Dependencies**: 现有依赖，不新增

**Storage**: PostgreSQL。**1 张新表 + 2 个并发索引**（各自独立文件）。**无外键、无级联**

**Testing**: Go `testing`；`internal/testutil` 的 `dbfx` / `testutil.Call`

**Project Type**: Existing monorepo. Do not re-derive this.

**Constraints**:
- 版本表**只插不改不删**（工作区/账号删除除外）
- 列类型 `text` / `timestamptz`，与 `content_account` 及同层 content 表一致
- 并发冲突：**有界重试 ≤3 次，仍冲突回 409 诊断错误对象**
- **不碰上游 Multica 代码与 `server/internal/daemon/`**
- **无界面改动**；不写 UI 单测

**Scale/Scope**: 3 个迁移（表 + 2 索引，各含 down）、1 个查询文件、`ip-profile` 模块内 2 个文件、handler 1 个文件、2 处删除路径、sqlc 产物

## Constitution Check

| 原则 | 判定 | 说明 |
|---|---|---|
| I. CLAUDE.md 权威 | PASS | 遵循迁移规则、测试分层、包边界 |
| II. 无 UI 单测 | PASS | 本卡无界面改动 |
| III. 模块边界 | PASS | 沿用 `ip-profile`，**不新增模块目录**；模块仍不 import 生成的 `db` 包，由 handler 适配 |
| IV. 状态分离 | PASS（不适用） | 纯服务端 |
| V. 数据库 | PASS | **无外键、无级联**；两个索引各自独立文件、各一条 `CONCURRENTLY` 语句；唯一约束**不写在建表里**（Q3） |
| VI. 响应解析 | PASS | 新增端点，形状写入契约；无前端消费者（页面在 LT-013） |
| VII. UI 复用 | PASS（不适用） | 无界面改动 |
| VIII. 范围 | PASS | 只版本化 `persona_prompt` 一个字段；§3.1 其余字段与运行实现留给后续卡 |
| IX. 真实执行器禁用 | PASS | 不涉及 |
| X. 勾选不等于验收 | PASS | 接入合同「通过检查 ≠ 接入合格」在 PR 正文写明 |

**无停止条件触发。**

## Project Structure

### Source Code（改动必须限于此清单）

```text
server/migrations/
├── 479_content_account_revision.{up,down}.sql          # 表；**不含**唯一约束
├── 480_content_account_revision_unique_idx.{up,down}.sql  # CREATE UNIQUE INDEX CONCURRENTLY (account_id, revision)
└── 481_content_account_revision_workspace_idx.{up,down}.sql # CREATE INDEX CONCURRENTLY (workspace_id)，供工作区删除

server/pkg/db/queries/
├── content_account_revision.sql   # 新增：Insert / GetByID / GetCurrent / ListByAccount / NextRevision / Count
└── workspace_delete.sql           # 改：删除工作区时一并删版本

server/pkg/db/generated/           # sqlc 产物，单独提交、不手改

server/internal/content/ip-profile/
├── revision.go                    # 新增：Revision 形状、提示词校验、SetPersonaPrompt（含有界重试）
└── revision_test.go               # 新增：校验、重试边界、不可变性（E3 引用 diagnostics）

server/internal/handler/
├── content_account_revision.go    # 新增：四个端点，先经 workspacecore.Authorize
└── content_account_revision_test.go # 新增：五条核心负例 + 真并发

server/internal/handler/
└── workspace_delete_manifest_test.go  # 改：新表登记

scripts/content-boundaries.json    # 改：handler 新文件加入 adapters 白名单
```

**明确不动**：`content_account` 的表结构（**不加 `persona_prompt` 列、不加 `current_revision_id` 指针**，Q1/Q2 裁决）、上游代码、`server/internal/daemon/`、`packages/`、`content-boundaries.json` 的 `modules` 段。

**Structure Decision**：

- **版本表独立、账号表不动**。当前版本由 `ORDER BY revision DESC LIMIT 1` 求得——没有第二份真相，也就不可能漂移（Q2-A）。
- `(account_id, revision)` 的唯一索引**既是并发互斥的依据，也是求当前版本的索引**，一条索引两用。
- 模块仍不 import `db` 生成包（内容边界只批准 pgx/uuid/websocket/otel），沿用 `#71` 的 `Store` 接口 + handler 适配。

## Complexity Tracking

无违规。

三处取舍：

1. **不存当前版本指针**，代价是读当前要一次排序取首行（有索引覆盖）。换来的是不可能出现「账号指向的不是最新版本」这类难以发现的漂移（Q2-A）。
2. **有界重试而非直接 409**：两次确认都真实发生，历史应当都留下；直接 409 会让一次确认消失，而创作者看到的只是一个失败。上限 3 次，之后如实回 409 而不是无限循环。
3. **版本号与 `revision_id` 两个标识并存**：`revision` 供人读与排序，`revision_id` 供跨表引用。看似冗余，但让「第几版」和「钉哪一版」各有其字段——用版本号做引用会在账号之间撞车。
