# Implementation Plan: 诊断模拟器的数据形态

**Branch**: `claude/spec-012-diag-simulator-shapes` | **Date**: 2026-09-15 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/012-diag-simulator-shapes/spec.md`

## Summary

手动验收矩阵里有一批条目跑不了，原因不是没时间，是**模拟器造不出那个数据形态**。本特性给 `server/internal/content/diagnostics` 增加一组**诊断自验专用场景**（shape 场景），它们与现有 16 个业务故障场景并列在 `Scenarios` 里，走同一条 `Simulate` 入口、同一道隔离门禁，产出并发 span、孤儿 span、单 span 运行、>200 span 运行、`not_run` / `failed` / 冲突的回归结果、空 `module`，以及可恢复的 sink 写入失败。

技术路线有三个决定性判断，都来自对代码的核实而不是对规格的复述：

1. **不给注入点另开一道门。** 规格 Q3 暂定「sink 失败与缓冲丢弃的注入点复用 `testEnabled`，由装配处注入」。核实后发现**更简单且更安全的做法**：把 sink 失败本身做成一个 shape 场景。它于是走 `Simulate` 的既有门禁，**不需要新的开关、新的环境变量、新的装配参数**，恢复也不需要动作——失败只作用于该次运行的写入。见 research D5。FR-015 / FR-016 因此被平凡满足。
2. **`>200 span` 光造出来不够用。** 运行详情读取路径是 `GetRun` → `Store.Query(Filter{Run:id, Limit:100})`，而 `Store.Query` 把 `Limit` 夹在 100 以内。瀑布的折叠阈值是 `STREAM_EVENT_CAP = 200`。所以即便运行有 250 个 span，界面拿到的也只有 100 个，**`order.length > cap` 永远不成立，`006-W-7` 的折叠提示永远出不来**。必须同时让运行详情把该次运行的事件取全。见 research D2。
3. **`Scenario` 加一个分类字段是对外形状的改动。** `Scenarios` 经 `Overview` 下发，前端 `overviewSchema` 有对应的 zod 解析。按 API 兼容规则，新字段在前端必须**可缺省**，并附一条畸形响应测试。见 research D3。

## Technical Context

**Language/Version**: Go 1.26（`server/`）、TypeScript 5 strict（`packages/core/`）

**Primary Dependencies**: Chi、pgx/v5、`go.opentelemetry.io/otel/trace`；前端 zod + `parseWithFallback`

**Storage**: PostgreSQL。本特性**不新增表、不新增迁移**——所有形态都写进既有的 `content_diagnostic_run` / `content_technical_log` / `content_operation_audit`

**Testing**: `go test ./internal/content/diagnostics/...`（需 DB 的用例读 `LORETIDE_DIAG_TEST_DATABASE_URL`）、`packages/core/*.test.ts`（Vitest）。**无 UI 单测**

**Target Platform**: 本地开发实例（`APP_ENV=development` 且 `LORETIDE_DIAGNOSTICS_TEST=1`）

**Project Type**: Existing monorepo。不再推导。

**Performance Goals**: 一次 `shape_deep` 运行会触发 250+ 次 `Store.Technical`，每次一个事务并在末尾各跑一次 `PruneTechnical`。这在开发实例上是**秒级**而非毫秒级操作，runbook 必须写明「这一步会慢」，否则跑矩阵的人会以为卡死。见 research D7。

**Constraints**: 不碰 `server/internal/daemon` 与任何上游 Multica 代码；不放宽 `Sanitize` 白名单；不改「审计写失败即整体回滚」；真实执行器保持禁用；不写 UI 单测；不新增前端读取路径（同一端点、同一响应形状不算新增）。

**Scale/Scope**: 9 个新场景 id，1 个 `Scenario` 新字段，1 处运行详情取数改动，1 处 `Store.Technical` 失败开关。

## Constitution Check

*GATE：Phase 0 之前必须通过；Phase 1 设计之后复核。*

| 原则 | 判定 | 依据 |
|---|---|---|
| I. `CLAUDE.md` 权威 | 通过 | 无新规则；命名、测试归属、API 兼容全部按 `CLAUDE.md` |
| II. 不写 UI 单测 | 通过 | 新增测试全在 `server/internal/content/diagnostics/*_test.go` 与 `packages/core/content/diagnostics/*.test.ts`。呈现由手动矩阵验收——**这正是本特性存在的理由** |
| III. 模块边界 | 通过 | 后端改动只在 `internal/content/diagnostics`；前端只改 `packages/core/content/diagnostics/contract.ts`（zod 解析），不碰 `views`/`ui` |
| IV. 服务端/客户端状态分离 | 不适用 | 无新 store、无新 query key |
| V. 无外键、无级联 | 不适用 | **不新增迁移**（见 Technical Context → Storage） |
| VI. 响应解析不强转 | **需动作** | `Scenario` 新增 `kind` 字段 → `overviewSchema` 同步更新，字段**可缺省**，附畸形响应测试。见 contracts/scenario-kind.md |
| VII. UI 复用 Multica | 不适用 | 无 UI 改动：shape 场景进入既有下拉是列表驱动的，不需要新控件 |
| VIII. 范围是所领的任务 | **需注意** | research D2 的运行详情取数改动**不是顺手修的缺陷**，而是「`006-W-7` 可执行」这条需求**不做就不成立**的前置。按原则 VIII 的例外条款（「除非所需行为离了它无法工作」）纳入，并在 data-model 里划出确切边界 |
| IX. 真实执行器保持禁用 | 通过 | 全部形态是虚拟时间下的构造数据，不触发任何执行器；无新增默认测试会解析 agent CLI |
| X. 打勾不等于验收 | 通过 | `tasks.md` 的勾只表示实现任务已交付；矩阵条目的通过与否由手动跑出来 |

### 关于「隔离实例之外注入被拒绝」的出处

任务描述把它记为 **constitution 原则 VII**。核实 `.specify/memory/constitution.md` 后：**原则 VII 是「UI reuses Multica, it does not reinvent it」**，不是这条。这条规则的实际出处是 constitution 的 **Development Workflow** 一节（「一个任务、一个执行者、一个 worktree、自己的数据库与端口」）与它指向的 `docs/development/ai-collaboration.md`，加上**原则 IX**（真实执行器保持禁用）。

**需求本身不变**：每一类新形态都要有负例断言证明隔离实例之外不可达（FR-017 / SC-004）。变的只是引用编号。规格 US2 与 FR-017 的引文照此更正。

## Project Structure

### Documentation (this feature)

```text
specs/012-diag-simulator-shapes/
├── plan.md                        # 本文件
├── research.md                    # Phase 0
├── data-model.md                  # Phase 1
├── quickstart.md                  # Phase 1
├── contracts/
│   ├── simulator-shapes.md        # 9 个 shape 场景逐个的产出契约与不变量
│   └── scenario-kind.md           # Scenario.kind 的对外形状与前端解析契约
├── checklists/requirements.md
└── tasks.md                       # Phase 2（/speckit-tasks 产出，不由 /speckit-plan 创建）
```

### Source Code (repository root)

```text
server/
└── internal/content/diagnostics/
    ├── simulator.go               # Scenarios 加 Kind；shape 场景的构造分支
    ├── shapes.go                  # 新增：shape 场景表与各自的事件构造
    ├── service.go                 # Run 里按场景决定是否 Evaluate / 是否让 sink 失败
    ├── store.go                   # GetRun 取全该次运行的事件；Technical 增加失败开关
    ├── shapes_test.go             # 新增：逐形态的正例
    ├── shapes_isolation_test.go   # 新增：逐形态的负例（隔离之外不可达）
    └── simulator_test.go          # 既有：业务故障类仍等于原 16 项

packages/core/content/diagnostics/
├── contract.ts                    # overviewSchema 的 scenarios 增加可缺省 kind
└── contract.test.ts               # 畸形响应测试（缺 kind / kind 非字符串）

docs/development/
├── manual-ui-runbook.md           # 相关条目备注改为可执行的造数步骤
└── diagnostics-acceptance-mapping.md  # DIAG-11 / §3 V07 / §4.2 的 §7 对表说明重写

specs/002-diag-package-stream-recovery/manual-ui-todo.md
specs/006-diag-trace-waterfall-regression/manual-ui-todo.md
specs/008-diag-linkage-and-invariants/manual-ui-todo.md   # 三份备注回写
```

**Structure Decision**: 后端改动集中在 `server/internal/content/diagnostics`，新形态的构造逻辑独立成 `shapes.go` 而不是继续往 `simulator.go` 的步进循环里塞 `switch` 分支——那个循环的结构（父子成链、时间首尾相接）**正是这些形态造不出来的原因**，在它内部加分支只会把两套语义搅在一起。`Simulate` 对 shape 场景的处理是一次早分流，业务故障场景的代码路径**逐字节不变**。

前端唯一改动是 zod 解析，落在 `packages/core/`；`packages/views/` 与 `packages/ui/` 不动。

### 与 `specs/011` 的次序

两者都动 `simulator.go`。**本 PR 只产出规格**；实施排在 011 合入之后，并以合入后的 `simulator.go` 为基线重新核实 spec 的 Current State 第 1 节。若 011 改变了步进循环的结构，research D1 的分流点需要跟着重定。

## Complexity Tracking

| Violation | Why Needed | Simpler Alternative Rejected Because |
|---|---|---|
| 原则 VIII：改动 `Store.GetRun` 的取数，超出「只加场景」的最小面 | `006-W-7` 的验收对象是**折叠提示**，触发条件是 `order.length > 200`。运行详情最多返回 100 条事件，所以造出 250 个 span 之后界面仍然不折叠——需求不做这一步就**不成立**，不是顺手修的缺陷 | ① 调低 `buildTraceWaterfall` 的默认 cap：那是改产品行为去迁就验收，本末倒置；② 放宽 `Store.Query` 的 100 上限：那会顺带放宽对外端点的分页边界，面更大；③ 记 `006-W-7` 仍不可达：SC-001 要求「仍需标『缺少对应数据』的条目数为 0」，等于不交付 |
| `Scenario` 增加字段，改动对外形状 | 硬约束要求新形态出现在 `Scenarios` 里，同时要求 `docs/13` §7 的对表口径不变糊。两者只能靠**结构里的分类**同时满足 | 只在文档里约定哪几项属于 §7：那正是 Q2 要避免的——对表结论会退化成一句没法被机器核对的话，而 D13-V07 记「部分」的依据之一就是那次对表 |
