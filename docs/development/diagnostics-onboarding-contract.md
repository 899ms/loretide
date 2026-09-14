# 诊断接入合同

## 1. 这份文件是什么

**一份可判定的接入清单。** 新增一个 content 模块时，它规定该模块必须接入哪些诊断面、每一面的公共入口是什么、怎样才算做到了。

它的存在是为了让「有操作、错误、trace、故障和回归证据」这句话对每个模块只讨论一次，而不是每次交付重新讨论一遍。对照关系见 `docs/development/diagnostics-acceptance-mapping.md` 的 D13-V12 与 DIAG-13。

**配套的静态检查**是 `scripts/check-diagnostics-contract.mjs`（`pnpm check:diagnostics-contract`）。检查与本文件是两半：本文件靠人读，检查靠机器。**两者都不等于接入合格**，原因见第 5 节。

**约束范围**：服务端条目（第 2 节）有静态检查；前端两根（第 3 节）**当前没有静态检查**，由人工审查把关。

## 2. 服务端条目（`server/internal/content/<module>/`）

每行三列：**要做什么 / 公共入口 / 怎么算做到了**。第三列是本合同与愿望清单的分界线——写不出第三列的条目，说明那一项还没想清楚。

公共入口全部核实自 `server/internal/content/diagnostics/`（分支 `app-main`）。

| 诊断面 | 公共入口 | 怎么算做到了 |
|---|---|---|
| **审计写入点** | `Store.Audit(ctx, Scope, Event)`、`Store.CommitRun(ctx, Scope, Run, failAudit)` | 模块的每个改变状态的操作在**同一个事务内**调用 `Store.Audit`；审计写失败即整体回滚，不得吞掉错误继续提交 |
| **技术日志** | `Store.Technical(ctx, Event)`、`SlogHandler`、`LogBuffer`（`NewLogBuffer`） | 模块的失败路径产出 `Event`，至少填 `Component` / `Severity` / `Action` / `Outcome` / `Code`；技术日志写失败**只计数**（`LogBuffer.Errors`），不得拖垮业务调用 |
| **trace 传播** | `Child(ctx)`、`Pack(ctx, operation, attempt, sequence)`、`Unpack(ctx, Envelope)`、`DecodeQueuedEnvelope(ctx, wire)`；HTTP 边界由 `server/internal/middleware/trace.go` 承担 | 模块内跨步骤的调用用 `Child` 取子 span；跨进程/队列的消息用 `Pack` 打包、接收端用 `Unpack` 或 `DecodeQueuedEnvelope` 还原。模块**不自己造 trace id**，也不自己解析 `traceparent` |
| **事务后派发** | `Outbox` 接口、`MemoryOutbox`（`NewMemoryOutbox`） | 事务内 `Register`，提交后 `Settle(ctx, tx, true)`，回滚则 `Settle(ctx, tx, false)`。**`MemoryOutbox` 是进程内实现，不跨重启**——模块若要求「重启后仍会派发」，必须自带持久实现，不得假设默认实现能给 |
| **模拟场景与回归夹具** | `Scenarios`（16 个 `Scenario{ID, Expected}`）、`Simulate`、`Evaluate` | 模块的失败模式能映射到 `Scenarios` 里的某个场景 ID；映射不上的失败模式在交付说明里列出，并说明为什么现有 16 个场景不覆盖它 |
| **脱敏** | `Sanitize(Event)`、`RequestIdentity(method, pattern, status, header)` | 进入日志或审计的 `Event` 一律过 `Sanitize`；HTTP 边界只用 `RequestIdentity` 描述请求（路由模板 + 方法 + 状态码 + 存在性档头名），**不得**把原始路径、查询串或请求头值塞进 `Event` |
| **错误码枚举** | **暂无公共入口**（`log.go` 的 `codes` 未导出） | **向诊断包申请导出错误码枚举**；在导出之前，模块使用的 `Code` 由**人工审查**对照 `log.go` 的 `codes` 核对。导出是**后续任务**，不在本合同的交付范围内 |

### 2.1 关于错误码枚举这一行

`codes` 是包级私有的 map，模块既无法枚举它也无法校验成员。今天唯一的间接途径是 `Sanitize` 会把不认识的 `Code` 改写为 `INTERNAL`——那是**事后纠正**，不是可供模块引用的枚举。

因此本合同**不要求模块引用私有符号**，静态检查也**不把该枚举的存在作为判定条件**。这一行的第二列诚实地写「暂无公共入口」，而不是指向一个模块拿不到的东西。

## 3. 前端两根（`packages/core/content/<module>/`、`packages/views/content/<module>/`）

> **本节当前没有静态检查。** 以下要求由**人工审查**把关；前端侧的静态检查列为后续任务。把要求写下来不等于已经强制执行——不要把本节读成「机器会挡住」。

- 诊断错误对象经 `parseWithFallback` 与 zod schema 解析，**不得**直接 cast 网络 JSON；
- 失败态呈现 `next_action`，不得只显示一个泛化错误；
- 不得把私有正文、凭据带进任何前端日志或上报路径；
- 服务端布尔字段用 `=== true` 判断，服务端驱动的枚举分支带 `default`。

依据见 `CLAUDE.md` → API Compatibility 与 constitution 原则 VI。

## 4. 模拟不代替真实通过

证据分两栏。**填的时候分开填，读的时候分开读。**

| 栏 | 内容 | 执行器禁用期间 |
|---|---|---|
| **模拟可得** | 16 个 `Scenarios` 场景、回归夹具、契约测试与 handler 测试 | 照常填写实际结果 |
| **仅真实执行器可得** | 真实 Codex 实测、远程阶段实测 | **一律填「未执行（constitution 原则 IX：真实执行器保持禁用）」** |

三条规则：

1. 第二栏填「未执行」时，该模块的诊断验收**不得**被记为整体通过——模拟全绿只满足其中一部分。
2. **不存在**一个「全绿」状态可以在第二栏未填的情况下达成。任何汇总口径若能在真实执行器没跑的情况下显示全绿，那个口径是错的，应当修正口径而不是修正这条规则。
3. 未执行就写未执行。不写「N/A」、不留空、不用「暂不适用」代替——这三种写法都会在汇总时被当成没有问题。

## 5. 静态检查的边界

`pnpm check:diagnostics-contract` 判定三条**痕迹**：

- **E1**：模块目录内至少一个**非测试** `.go` 文件 import 了 `content/diagnostics`；
- **E2**：模块目录内至少出现一次审计 / 技术日志 / trace 三类调用点**之一**；
- **E3**：至少一个 `_test.go` 引用 `diagnostics`。

**这三条只证明痕迹存在。** 它们不证明：

- 接入的语义正确（审计写在了该写的地方、字段填对了、事务边界对）；
- 覆盖完整（每个改变状态的操作都写了审计）；
- 真实执行器跑过（**没有任何静态手段能判断这件事**，见第 4 节）。

**通过检查 ≠ 接入合格。** 检查是下限，本文件第 2、3 节是要求，人工审查是判定。一个模块可以在检查全绿的情况下完全没有正确接入——检查存在的价值只是让「一点都没接」这件事无法悄悄通过。

### 5.1 诊断包自身怎么判定

`diagnostics` 模块**提供**这套接口，它无法 import 自己，因此 E1/E2/E3 三条对它不适用。检查对它走一条单独的**提供方规则**：

| 条件 | 判定 |
|---|---|
| **P1** | 模块目录内至少一个非测试 `.go` 文件声明 `package diagnostics` |
| **P2** | 本合同第 2 节第二列点名的 **23 个公共入口**在模块内均有导出声明（`func` / 方法 / `type` / `var`） |
| **P3** | 模块带有 `_test.go` |

这条规则比消费方规则更强——**它把第 2 节第二列钉成了机器核对的对象**：改名或删掉其中一个公共入口，检查会红，而不是让本文件悄悄指向一个已经不存在的符号。

错误码枚举**不在 P2 的名单里**，因为 `log.go` 的 `codes` 未导出——检查不要求一个模块拿不到的东西存在。

## 6. 接入写在模块目录之外时：登记豁免

模块的诊断接入有时正当地写在别处（例如审计统一在 handler 层完成）。这种情况**登记豁免**，不要靠关掉检查绕过。

登记在 `scripts/diagnostics-contract.json`：

```json
{
  "version": 1,
  "exemptions": [
    { "module": "source-inbox", "reason": "审计统一在 handler 层写入", "where": "server/internal/handler/source_inbox.go", "expires": "2026-12-31" }
  ]
}
```

四个字段**全部必填**：

| 字段 | 规则 |
|---|---|
| `module` | 必须是 `scripts/content-boundaries.json` 里声明的模块之一 |
| `reason` | 非空。让豁免在 PR diff 里读起来像一个决定，而不是一行静默的白名单 |
| `where` | 非空，指出实际接入位置 |
| `expires` | `YYYY-MM-DD`。**缺这一项即判为无效配置，检查失败** |

**到期日期为什么必填**：一条没有期限的豁免就是永久豁免，而永久豁免与删掉这条检查的效果完全相同。豁免出口存在的意义是给合理的例外一条**临时**通道。

**到期之后**：豁免停止生效，该模块重新按 E1/E2/E3 判定；失败输出会指明原因是**豁免已过期**，而不是笼统地报缺证据——两者要补的东西不同（前者是续期或真接入，后者只是接入）。

## 7. 合同什么时候对一个模块生效

**模块落地的那一刻**，即 `server/internal/content/<module>/` 下出现第一个 `.go` 文件。

在模块图（`scripts/content-boundaries.json`）里已声明但尚无目录的模块，检查**完全沉默**——既不失败也不告警。今天 12 个模块里 11 个属于这种情况。

目录存在但只有骨架（例如只有一个 `doc.go`）**算已落地**，因此立刻受约束。这是刻意的：一个已经开始写的模块就该从第一天带上诊断，而不是攒到最后补。确有理由的例外走第 6 节的豁免登记，而不是靠「还很空」自动豁免。
