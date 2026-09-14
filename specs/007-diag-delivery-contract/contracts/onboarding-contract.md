# Contract: 接入合同文本要写成什么样

产物：`docs/development/diagnostics-onboarding-contract.md`。

## 服务端条目（静态检查覆盖）

每行三列：**要做什么 / 公共入口 / 怎么算做到了**。第三列可判定，否则该行不合格。

| 诊断面 | 公共入口（Current State 核实） | 备注 |
|---|---|---|
| 审计写入点 | `Store.Audit`、`Store.CommitRun` | 事务内，失败即整体回滚 |
| 技术日志 | `Store.Technical`、`SlogHandler`、`LogBuffer` | 失败只计数，不拖垮业务 |
| trace 传播 | `Child`、`Pack`、`Unpack`、`DecodeQueuedEnvelope`；HTTP 边界由 `middleware.Trace` 承担 | — |
| 事务后派发 | `Outbox`、`MemoryOutbox` | 进程内，不跨重启——合同须写明该限制 |
| 模拟场景与回归夹具 | `Scenarios`（16 个）、`Simulate`、`Evaluate` | — |
| 脱敏 | `Sanitize`、`RequestIdentity` | — |
| **错误码枚举** | **暂无公共入口**（`log.go` 的 `codes` 未导出） | 第三列写「向诊断包申请导出，导出前由人工审查」；导出为**后续任务**（clarify FR-003） |

## 前端两根条目（**无静态检查**）

`packages/core/content/<module>/` 与 `packages/views/content/<module>/` 的要求写进合同文本，并 **MUST 显著注明当前无静态检查、由人工审查把关**，前端侧检查列为后续任务（clarify FR-016a）：

- 诊断错误对象经 `parseWithFallback` 与 zod schema 解析，不得直接 cast；
- 失败态呈现 `next_action`，不得只显示一个泛化错误；
- 不得把私有正文、凭据带进任何前端日志或上报路径。

## 「模拟不代替真实通过」一节

证据分两栏：

| 栏 | 内容 | 执行器禁用期间 |
|---|---|---|
| 模拟可得 | 16 个场景、回归夹具、契约与 handler 测试 | 照常填写 |
| 仅真实执行器可得 | 真实 Codex 实测、远程阶段实测 | **一律填「未执行（constitution 原则 IX）」** |

MUST NOT 存在一个「全绿」状态可在第二栏未填的情况下达成（FR-014）。

## 静态检查的边界声明

合同 MUST 写明：静态检查只证明**痕迹存在**，不证明接入语义正确，也不证明真实执行器跑过。通过检查 ≠ 接入合格。
