# Implementation Plan: 内容对象的空间授权入口（LT-010）

**Branch**: `claude/spec-014-lt010-workspace-authorization` | **Date**: 2026-09-15 | **Spec**: [spec.md](./spec.md)

## Summary

把 `diagnosticScope` 里已经在跑的授权判定**提取**成 content 层可复用的助手，让第二个内容模块不必抄一遍。

**这是提取，不是新建**。因此风险集中在一处：诊断的行为必须一个字节都不变。计划把它当成首要约束，而不是事后检查。

同时，本特性是**接入合同生效后的第一个新模块目录**，所以 E1/E2/E3 要靠**真实接入**满足——拒绝判定经诊断记一条技术事件——而不是塞一个能骗过检查的调用。

## Technical Context

**Language/Version**: Go 1.26

**Primary Dependencies**: 现有依赖，**不新增**。模块只用标准库 + `content/diagnostics`

**Storage**: 复用上游 `member` 表，**只读**。**无迁移、无 schema 变更、无外键、无索引**

**Testing**: Go `testing`；DB 相关用例走 `internal/testutil` 的 `dbfx` / `testutil.Call`

**Project Type**: Existing monorepo. Do not re-derive this.

**Constraints**:
- **不碰上游 Multica 代码与 `server/internal/daemon/`**；上游 workspace 只读
- **诊断行为逐字节不变**（FR-007）——这是提取型改动的唯一真风险
- **不新增 UI 单测**；本卡**无界面改动**
- 模块目录**不得** import `server/internal/handler`（会形成反向依赖）；成员关系经**接口**注入

**Scale/Scope**: 1 个新模块目录（3 个文件）、1 处 handler 接入改写、1 处边界注册核对

## Constitution Check

| 原则 | 判定 | 说明 |
|---|---|---|
| I. CLAUDE.md 权威 | PASS | 遵循测试分层与包边界 |
| II. 无 UI 单测 | PASS | 本卡无界面改动，不产生 UI 用例与手动条目 |
| III. 模块边界 | PASS | 新模块只 import 标准库与 `content/diagnostics`（`content-boundaries.json` 已声明 `workspace-core → diagnostics`）；**不** import handler；handler 侧的 `content_diagnostics.go` 已在 `adapters` 白名单内 |
| IV. 状态分离 | PASS（不适用） | 纯服务端 |
| V. 数据库 | PASS | 无迁移、无外键、无级联、无索引 |
| VI. 响应解析 | PASS | 不改端点与 schema |
| VII. UI 复用 | PASS（不适用） | 无界面改动。**SOP 阶段界面规则**（2026-09-15）记录于 spec 的 UI Impact，本卡无适用项 |
| VIII. 范围 | PASS | 严格限于 LT-010；`Scope.Accounts` 的真实账号授权留给 LT-011，本卡只保留接口形状 |
| IX. 真实执行器禁用 | PASS | 不涉及 |
| X. 勾选不等于验收 | PASS | 接入合同第 5 节明确「通过检查 ≠ 接入合格」，PR 正文如实写明检查只证明痕迹 |

**无停止条件触发。**

## Project Structure

### Source Code（改动必须限于此清单）

```text
server/internal/content/workspace-core/
├── authz.go            # 新增：Reason / Decision / Membership 接口 / Recorder 接口 / Authorize()
├── http.go             # 新增：Refusal() —— 新模块用的规范映射（无权与不存在同为 404 + 诊断错误对象）
└── authz_test.go       # 新增：五条核心负例 + 允许路径 + 归属变化（E3 引用 diagnostics）

server/internal/handler/
├── content_diagnostics.go              # 改：diagnosticScope 改为调用 workspacecore.Authorize；映射到它现有的 404/403，逐字节不变
└── content_diagnostics_authz_test.go   # 新增：钉住诊断三种拒绝组合的状态码与 body
```

**明确不动**：`server/internal/daemon/`、上游 workspace handler、迁移、CI、`packages/`、`scripts/content-boundaries.json`（`workspace-core` 已注册，无需改）。

**Structure Decision**：

- 模块目录名取 **`workspace-core`**，与 `content-boundaries.json` 的注册键**逐字一致**——检查器用目录名做模块名（`classify()`），改名就等于新增一个未注册模块。Go 包名为 `workspacecore`（标识符不能带连字符，目录名可以）。
- 成员关系经 **`Membership` 接口**注入，参数与返回值都是基本类型，模块因此不依赖 `db.Member` 也不依赖 handler。
- 拒绝时经 **`Recorder` 接口**的 `Technical(ctx, diagnostics.Event)` 记一条技术事件——这同时是 E1（import）与 E2（技术日志调用点）的**真实**来源。

## Complexity Tracking

无违规。

两处需要记录的**取舍**：

1. **诊断保留两种状态码（404/403），新模块统一 404。** 「统一行为」在本特性里指**对新模块**统一；诊断已在生产上按现行形状被 `002-V05-11` 验过，改它是回归而不是统一（Q2 裁决 A）。代价是两套映射并存，故契约里把「谁用哪套」写死。
2. **每次判定都查库，不缓存。** 代价是每次一条查询；换来的是「归属一变，下一次判定立刻拒绝」可以被直接断言，而不需要构造缓存失效（Q3 裁决 A）。
