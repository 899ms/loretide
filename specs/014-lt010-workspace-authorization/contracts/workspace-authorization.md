# Contract: 空间授权助手（`content/workspace-core`）

## 助手回答什么，不回答什么

> **回答**：这个**主体**能不能进这个**空间**。
> **不回答**：某个**对象**是不是属于这个空间。

对象归属由调用方查（它才知道自己的表）。把这条写在最前面，是因为一个以为助手替它查了归属的调用方，会写出「拿到 allow 就直接返回对象」的代码——而那个对象可能属于别的空间。

## 判定

```text
Authorize(ctx, members, actor, workspace, allowedRoles...) -> Decision

Decision = { Allowed: bool, Reason: Reason, Actor, Workspace string }
```

| `Reason` | 何时 | 调用方通常怎么处理 |
|---|---|---|
| `ReasonNone` | 允许 | 继续 |
| `ReasonNoActor` | 主体为空（未认证） | **401**，不并入 404 |
| `ReasonNoWorkspace` | 空间标识为空或不可解析 | 与「无权」同一响应 |
| `ReasonNotMember` | 查不到成员行 | 与「不存在」同一响应 |
| `ReasonRole` | 是成员但角色不在允许集合 | 与「无权」同一响应 |

**理由供排障与记录用，不直接决定响应。** 响应由调用方按下面两套映射之一决定。

## 两套映射，写死谁用哪套

| 调用方 | 非成员 | 角色不足 | 账号不匹配 | 依据 |
|---|---|---|---|---|
| **诊断（既有）** | **404** | **403** | **403** | 已在生产上被 `002-V05-11` 验过；本特性**逐字节不变** |
| **新内容模块** | **404** | **404** | —— | 无权与不存在不可区分（FR-004） |

三种情况错误码都是 `AUTHORIZATION_DENIED`。

**为什么不统一成一套**：把诊断改成 404 会让已验收的 403 失效——那是回归，不是统一。「统一」在本特性里指对**后来者**统一（Q2 裁决 A）。

## 拒绝响应的形状

只含以下字段，经 `diagnostics.Sanitize` 产出：

```json
{"error": "...", "code": "AUTHORIZATION_DENIED", "trace_id": "...",
 "component": "...", "retryable": false, "next_action": "check_authorization"}
```

**不含任何对象字段**——没有名称、没有设置、没有 id 回显。

### 存在性也不泄漏

「他人空间的真实 id」与「根本不存在的 id」**必须产出逐字节相同的响应**。这是 `ReasonNotMember` 覆盖两者的原因：查不到成员行，无论是因为空间不存在还是因为你不在里面，对调用者都是同一句话。

## 不变量

| # | 规则 | 断言位置 |
|---|---|---|
| A1 | 非成员拒绝，理由 `ReasonNotMember` | `authz_test.go` |
| A2 | 角色不足拒绝，理由 `ReasonRole`——**与 A1 是不同理由**，但默认映射到同一响应 | `authz_test.go` |
| A3 | 主体为空 → `ReasonNoActor`，**不**与 404 合并 | `authz_test.go` |
| A4 | 空间标识为空/不可解析 → `ReasonNoWorkspace` | `authz_test.go` |
| A5 | **每次判定都读成员关系**，不缓存 | `authz_test.go`（计数假实现） |
| A6 | 成员被移除或降级后，下一次判定拒绝 | `authz_test.go` |
| A7 | 每次**拒绝**都经 `Recorder.Technical` 记一条技术事件；允许不记 | `authz_test.go` |
| A8 | 诊断的三种拒绝组合状态码与错误码不变 | `content_diagnostics_authz_test.go` |
| A9 | 「他人空间真实 id」与「不存在 id」两次响应逐字节相同 | `content_diagnostics_authz_test.go` |

## 接入合同（E1/E2/E3）

本模块是合同生效后的**第一个**新模块目录，三条都由**真实接入**满足：

| 条 | 怎么满足 | 不是靠什么 |
|---|---|---|
| **E1** | `authz.go` import `content/diagnostics`（用 `diagnostics.Event`） | —— |
| **E2** | 拒绝时调用 `Recorder.Technical(ctx, diagnostics.Event{...})` 记一条技术事件 | **不是**为了过检查塞一个 `NewID()`——检查本就把工具函数排除在 E2 之外 |
| **E3** | `authz_test.go` 引用 `diagnostics` | —— |

`pnpm check:diagnostics-contract` 应报 **`checked 2 landed modules`**（diagnostics + workspace-core）。

> 合同第 5 节：**通过检查 ≠ 接入合格。** 这三条只证明痕迹存在，不证明语义正确、覆盖完整。本模块的语义由上面 A1–A9 九条断言承担。

## 变异验证

| # | 改动 | 应变红 |
|---|---|---|
| M1 | 角色检查恒真 | A2 |
| M2 | 非成员当作允许 | A1、A9 |
| M3 | 判定结果缓存一次 | A5、A6 |
| M4 | 拒绝时不记技术事件 | A7 |
| M5 | 诊断的非成员分支改成 403 | A8 |
