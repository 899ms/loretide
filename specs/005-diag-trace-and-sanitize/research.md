# Research: 005 诊断 HTTP 追踪贯通与请求脱敏

Phase 0。Technical Context 无 NEEDS CLARIFICATION（三处已由 2026-09-14 clarify 解决）；以下为落地前必须定下的技术决策。

## D1. 父级采信的判定放在哪一层

- **Decision**: 拆成两步。**第一步**（全局，`r.Use` 在 `router.go:1290` 一带的公共栈）：无条件为请求建立本实例 trace，并把**校验通过**的入站 `traceparent` 存入 context 作为「候选父级」，此时不作任何信任判断。**第二步**（`middleware.DaemonAuth` 之后，仅 daemon 路由组）：把候选父级提升为真正的父级。其余路由组不做提升，候选值只作为关联属性。
- **Rationale**: 公共中间件栈跑在认证之前（`DaemonAuth` 在 1440、`Auth` 在 1512/1522），所以**全局中间件根本拿不到 `X-Actor-Source`**。FR-016 要求「判定基于已完成的认证结果而非消息内容」，因此提升动作必须在认证之后。`X-Actor-Source` 由认证中间件写入且客户端提供的值被剥离（`actor_guards.go:111-116` 注释明载），是可信信号。
- **Alternatives considered**:
  - 单个全局中间件直接读请求头判断来源 —— 让客户端自称身份决定信任，正是 FR-016 禁止的，否决。
  - 把整个传播中间件下移到认证之后 —— 未认证路由（健康检查、登录）就失去 trace，且认证失败路径的诊断价值恰好最高，否决。

## D2. 边界建 trace 与既有 `Child()` 的关系

- **Decision**: 中间件在 ctx 中放入有效 span context 后，`handler.DiagnosticTrace` 保留 `Child(ctx)` 调用不变——此时 `Child` 看到有效父级，会**沿用其 TraceID 并新开 span**，正是想要的行为。不改 `Child` 的实现。
- **Rationale**: `Child()` 已经写成「父级有效则沿用 TraceID，否则随机新开」。今天它每次都走后一条分支，唯一原因是 ctx 里没有 span context。补上边界这一步，其余全部代码路径（`Pack` 从 ctx 注入 carrier、`DecodeQueuedEnvelope`、`ServeSimulationTransport`）**一行不改**即连通。这是本功能改动面小的根本原因。
- **Alternatives considered**: 在 handler 内直接从请求头提取 —— 每个 handler 重复一遍，且与「传播全局」的决定冲突，否决。

## D3. 请求身份的安全形状

- **Decision**: 记录**路由模板 + HTTP 方法 + 响应状态码**三元组，由 `Event.route`（`<METHOD> <模式>`）与 `Event.status`（状态码）两个字段承载。路由模板取自 chi 的 `RouteContext().RoutePattern()`，它返回的是注册时的模式（含 `{id}` 占位符），**不是**实际路径，因此天然不含取值。查询串的取值与**键名**都不记，原始路径与路径哈希都不记。
- **Rationale**: `RoutePattern()` 给出的是编译期就确定的有限集合，可证明不含用户输入；不需要任何正则清洗，也就不存在「清洗漏一种写法」的风险。
- **Status**: 主任务 2026-09-14 补答确认，并明确排除查询串键名与路径哈希。状态码为补答新增的一项——它使「哪个请求失败了」可以直接从事件读出，而不必回到日志里对时间。
- **Alternatives considered**: 对 `r.URL.Path` 做正则替换 —— 需要枚举所有 id 形态（UUID、slug、数字、hex），漏一种就泄漏，且无法自证完备，否决。

## D4. 请求头准入规则的形状

- **Decision**: 三档，逐名列举见 `contracts/request-sanitization.md`——**记值 / 只记存在 / 一律不记**，未列入者默认不记。名称比对大小写不敏感（`textproto.CanonicalMIMEHeaderKey`）。第三档除逐名项外还有四条后缀模式（`*-token` / `*-secret` / `*-key` / `*-password`），且优先于前两档。同名多值不拼接，按整体丢弃处理。
- **第二档为什么只记「存在」**：`user-agent` 与 `accept` 对诊断有用（区分客户端、内容协商），但取值是自由文本、可被调用方塞入任意内容。记「存在」保住了诊断价值，又不给自由文本留入口。**长度也不记**——长度是取值的一种泄漏。
- **Rationale**: `log.go` 现有全部规则都是 allowlist（`oneOf`、`token` 正则、`hexID`）。denylist 的失败模式是「新增一个敏感头就漏」，与仓库既有姿态不一致。
- **Alternatives considered**: 记录头名称但屏蔽取值 —— 头名称本身可能暴露内部结构，且 FR-005 明确「名称、取值、片段、长度」都不得进入，否决。

## D5. 模块边界检查器要不要把中间件登记为 adapter

- **Decision**: 实施时先跑 `pnpm check:content-boundaries`（或等价的两条 node 命令）确认。`server/internal/middleware/trace.go` 若 import `server/internal/content/diagnostics`，按检查器规则它属于 content 根之外的文件，需要在 `scripts/content-boundaries.json` 的 `adapters` 列表里，且只能 import 模块的 `index`。**首选做法是不 import**：中间件只用 `go.opentelemetry.io/otel` 与标准库，通过 `context.Context` 与 diagnostics 包交换，从而不产生跨根依赖、不需要改 `adapters`。
- **Rationale**: 少改一处共享配置就少一次跨功能耦合。002 的经验是平台层为避免跨界 import 而就地写结构体，同样适用。
- **Alternatives considered**: 直接把中间件加进 `adapters` —— 可行，但改的是全仓共享配置，且会给后续「中间件可以随便 import content 模块」开口子，列为次选。

## D6. 出站 `traceparent` 注入点

- **Decision**: `server/internal/daemon/client.go` 的四处 `http.NewRequestWithContext`（713、1179、1215、1248）统一经一个小helper注入 `traceparent`。队列与 WS 两段**不动**——它们走 `Envelope.Carrier`，`Pack` 已经在注入。
- **Rationale**: 只有真实 HTTP 出站需要 W3C 头；既有 Envelope 路径改动一行都不需要。
- **Alternatives considered**: 给 `http.Client` 套 `RoundTripper` —— 更自动，但会给该 client 的**全部**请求加头，超出本功能范围且难以针对性测试，列为后续可选。

## D7. 派发接口的签名形状

- **Decision**: 登记项含 `id`、`kind`、`payload`、`idempotency key` 四项（clarify FR-017 明确）。接口分两个动作：`Register(tx, item)` 在事务内登记，`Dispatch()` 在提交后触发。进程内实现用有界队列；失败计入既有 `LogBuffer.Errors` / `Dropped` 口径，不新增指标名。
- **Rationale**: 四元组是持久实现所需的最小集合（id 去重、kind 路由、payload 载荷、幂等键防重放），因此进程内实现换成落库实现时调用方不必改。
- **Alternatives considered**: 只传 payload —— 持久实现必然要补 id 与幂等键，届时调用方全改，违背 clarify「签名可被替换」的要求，否决。

## D8. 测试落点

- **Decision**:
  - **中间件**（`middleware/trace_test.go`）：表驱动，纯 `httptest`，不需要数据库。覆盖建 trace、用户凭据不采信、daemon 采信、非法值既不采信也不留存、不改响应形状。
  - **handler**（`content_diagnostics_test.go`）：用既有 `testutil.Call` + `dbfx`，覆盖三段连续性、记录范围不变、请求身份形状。
  - **规则**（`log_regression_test.go`）：负例为主——凭据头、路径参数、查询串、同名多值、大小写变体。
  - **派发**（`dispatch_test.go`）：回滚不派发、提交派发、失败可见、幂等去重、有界丢弃。
  - **core 契约**（`contract.test.ts`）：新字段缺省值与畸形响应兜底，`// @vitest-environment node`。
- **Rationale**: `CLAUDE.md` Testing「每个行为一个规范层」。中间件行为不需要 DOM 也不需要数据库，放在最窄的层。
