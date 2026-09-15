# Implementation Plan: 计划轮换页必须在窗口内送达

**Branch**: `claude/013-diag-stream-rotation` | **Date**: 2026-09-15 | **Spec**: [spec.md](./spec.md)

## Summary

`ContentDiagnosticStream` 的轮换页被安排在窗口**之外**：`rotate` 只能在 `select` 里被置真，而 `select` 在写完一页之后才执行，所以轮换页永远是窗口到期后新起的一轮——它的写出时刻必然晚于窗口边界，晚多少取决于那一轮的两次数据库查询。

修法是把判定**提到写之前**：当「再等一个 ticker 周期就会越过窗口边界」时，把**正在写的这一页**标为轮换页并结束。窗口之内送达的页不与任何传输层边界赛跑。

这不改变任何对外契约：`Page.Rotate` 的含义、客户端状态机、端点、响应形状全部不动。变的只是**哪一页**被标记、**什么时候**结束。

## Technical Context

**Language/Version**: Go 1.26（`server/`）、TypeScript 5 strict（`packages/core/`）

**Primary Dependencies**: Chi、pgx/v5；前端 zod + `parseWithFallback`

**Storage**: 不新增表、不新增迁移、不改查询

**Testing**: `go test ./internal/handler/`（`DATABASE_URL` 实跑）、`packages/core/*.test.ts`（Vitest）。**无 UI 单测**

**Target Platform**: web（第一阶段），Windows 相关搁置

**Project Type**: Existing monorepo。不再推导。

**Constraints**: 只动 `server/internal/handler` 的流处理与必要时 `stream-state.ts`；不动代理；不碰 daemon 与上游 Multica 其它代码；不写 UI 单测。

**Scale/Scope**: 一处判定位置的移动，一条既有测试的窗口修正，两条新断言，一份共享报文夹具。

## Constitution Check

| 原则 | 判定 | 依据 |
|---|---|---|
| I. `CLAUDE.md` 权威 | 通过 | 无新规则 |
| II. 不写 UI 单测 | 通过 | 新增断言全在 `internal/handler/*_test.go` 与 `packages/core/*.test.ts`；界面验收仍由主任务在浏览器做 |
| III. 模块边界 | 通过 | 后端只动 `internal/handler`；前端只可能动 `packages/core` |
| IV. 服务端/客户端状态分离 | 通过 | 无新 store、无新 query key |
| V. 无外键、无级联 | 不适用 | 不新增迁移 |
| VI. 响应解析不强转 | 通过 | `Page` 的字段集不变，`pageSchema` 不动 |
| VII. UI 复用 Multica | 不适用 | 无 UI 改动 |
| VIII. 范围是所领的任务 | **需注意** | 见下 |
| IX. 真实执行器保持禁用 | 通过 | 不涉及 |
| X. 打勾不等于验收 | 通过 | `SC-001` 只能由主任务在真实浏览器给出 |

### 关于原则 VIII

本 PR 会改动一条**既有测试**（`TestContentDiagnosticStreamClosesPlannedWindowWithRotate`）的窗口长度。这不是顺手修：**那条测试的 50ms 窗口短于一个 ticker 周期，它从来没有走过生产走的那条路**，而它的绿色正是「轮换页有回归测试」这个错误结论的来源。FR-006 把它列为需求。

`server/internal/handler/content_diagnostics.go` 是 Loretide 新建的 handler，因此 `go-modern-guidelines` 在我**实际改动的那几行**上适用；但正确性与既有写法优先，风格不是改动理由。

## Project Structure

### Documentation (this feature)

```text
specs/013-diag-stream-rotation/
├── plan.md            # 本文件
├── research.md        # Phase 0
├── contracts/
│   └── stream-rotation.md
├── checklists/requirements.md
└── tasks.md
```

### Source Code (repository root)

```text
server/internal/handler/
├── content_diagnostics.go        # 轮换判定提到写之前
└── content_diagnostics_test.go   # 回归测试改为真实窗口 + 新断言

packages/core/content/diagnostics/
└── stream-state.test.ts          # 基于真实末页报文的断言

specs/013-diag-stream-rotation/testdata/   （或 server 侧 testdata）
└── rotation-last-page.json       # 两侧共用的同一份真实报文
```

**Structure Decision**: 服务端改动集中在 `ContentDiagnosticStream` 的循环，**不抽新函数**——这个循环只有十来行，把判定挪个位置比引入一层间接更容易被下一个人读懂。前端不改产品代码，只加断言。

### 端到端断言怎么落地（FR-007）

Go 测不到 TypeScript，TypeScript 测不到 Go。要让「同一份真实报文」把两侧连起来，用一份**提交进仓库的夹具**：

1. Go 测试跑完一个真实窗口，把**最后一页的原始 JSON 字节**与夹具逐字节比对；
2. TS 测试**读同一个夹具**，喂给 `nextStreamState`，断言状态不是 `disconnected` 且要求立即续连。

这样服务端报文形状一变，Go 那侧先红；而 TS 那侧断言的是真实字节而不是手搓对象。比两边各写一个 mock 强，比跑真浏览器可行。

## Complexity Tracking

| Violation | Why Needed | Simpler Alternative Rejected Because |
|---|---|---|
| 改动既有测试的窗口长度 | 50ms < 1s ticker，该测试从未走过生产路径；不改它，修复之后它照样绿，证明不了任何事 | ① 新加一条真实窗口测试、保留旧的：旧的会继续给出「已覆盖」的假信号；② 不动测试：FR-006 不成立 |
| 提交一份报文夹具 | FR-007 要求两侧基于同一份真实报文，而 Go 与 TS 无法互相调用 | ① 两侧各写 mock：这正是让 50ms 测试骗过所有人的同一类错误；② 只测服务端：漏掉「客户端不进入断线态」这一半，而那是用户真正看见的 |
