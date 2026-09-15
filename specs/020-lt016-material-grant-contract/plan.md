# Implementation Plan: 素材与任务授权范围合同（LT-016）

**Branch**: `claude/spec-020-lt016-material-grant-contract` | **Date**: 2026-09-15 | **Spec**: `spec.md`
**Base**: `app-main` @ `5ca6793`

## Summary

在 `workspace-core` 里加一层**纯判定**：给定主体、已通过品牌授权的工作区、一份资源引用和一组授权，回答「能不能读」。零迁移、零端点、零界面。权限矩阵用表驱动夹具跑满。

## Constitution Check

| 原则 | 本卡如何满足 |
|---|---|
| **II 不写 UI 单测** | 本卡无界面 |
| **V 迁移规则** | **零迁移**，不涉及 |
| **VI 前端边界防御** | 不涉及前端 |
| **VII UI 政策** | 不涉及 |
| **VIII 范围** | 只做合同与判定；资源实体、Grant 存储、任务工作流接入都不在本卡 |
| **IX 真实执行器禁用** | 不涉及；真实执行器一栏填「未执行」 |
| **X 勾选不等于验收** | 无手动条目；每条验收都有自动用例 |

**上游改动**：无。`workspace-core` 是 Loretide 自有模块（`#69` 建立），不是上游 Multica 代码；包注释的改动属于本卡自己的文件。

## Source Code（改动必须限于此清单）

```text
server/internal/content/workspace-core/
├── authz.go        # 改：只改包注释那一句，把边界说精确
├── grant.go        # 新增：Principal / ResourceRef / Grant / GrantDecision / CanRead
└── grant_test.go   # 新增：权限矩阵夹具 + 六种拒绝 + 边界情形

server/internal/handler/
└── content_grant_prompt_test.go  # 新增：两账号提示词逐字节相同仍拒绝
                                   # （放这里是因为 workspace-core 不能 import ip-profile，
                                   #   而这条用例要同时用到两个模块）

specs/020-lt016-material-grant-contract/  # 规格、计划、合同、任务
```

**明确不动**：`server/migrations/`（零迁移）、`scripts/content-boundaries.json` 的 `modules`（仍 12 个）、`server/cmd/server/router.go`（无端点）、`packages/`（无 TS 改动）、`apps/`、daemon、上游任何文件。

## Structure Decision

- **判定是纯函数**：不查库、不读系统时间（`Now` 由调用方传）、不缓存。这样权限矩阵不需要一张表就能跑满，而资源切片落地时接上去的是**数据来源**，不是判定逻辑。
- **`Grant` 与 `Authorize` 同族**：类型化 `Decision` + `Reason`，拒绝记技术事件，响应映射复用 `RefusalStatus` / `RefusalBody`。不另造一套词汇。
- **品牌级资源不需要授权**：`ResourceRef.Account` 为空即品牌共享，同品牌主体直接可读。没有这一层，「专属」就不是一个有意义的状态。
- **提示词的那条用例放 handler 包**：`workspace-core` 依赖只有 `diagnostics`，import 不到 `ip-profile`；把用例放在两个模块都可见的地方，才能用**真实的账号与真实的提示词**来钉，而不是用一个假设。

## Complexity Tracking

| 取舍 | 选了什么 | 放弃了什么 |
|---|---|---|
| 判定放哪 | `workspace-core`（Q1-A） | 一个独立的第 13 模块——换来的是模块图不动、无新依赖边、接入合同仍 3 个 |
| 存储 | 不落库（Q2-A） | LT-017 少一步——换来的是不给一张还没有消费者的表定死形状 |
| 主体 | 任务或人（Q3-A） | 以账号为主体的简单——换来的是撤销能到单次任务 |
| 缓存 | 不实现，只写硬要求 | 判定的开销——`Authorize` 也不缓存，理由相同：撤销要立刻生效 |
