# Tasks: 诊断模拟器的数据形态

**Input**: `/specs/012-diag-simulator-shapes/` 的 spec.md、plan.md、research.md、data-model.md、contracts/

**实施前置**：本特性与 `specs/011` 都动 `simulator.go`。**实施必须排在 011 合入之后**，并以合入后的 `simulator.go` 为基线重新核实 spec 的 Current State 第 1 节（T001）。

## Loretide 测试口径（不可协商，覆盖任何通用模板）

- **不写、不跑 UI 单测。** 本地与 CI 同样适用；不改名、不改扩展名、不重新归类来绕过。
- **不用自动化点击做验收。** 涉及界面的部分列成手动 Todo，由用户确认。
- 保留改动范围需要的非 UI 检查：Go 测试、契约测试、模块边界检查、`pnpm typecheck`、构建。
- 需要数据库的 Go 用例设 `LORETIDE_DIAG_TEST_DATABASE_URL` 指向独立库**实跑**，并报 PASS / SKIP 计数。
- **没跑的检查记「未执行」，不记通过**（constitution 原则 X）。

## 格式

`[ID] [P?] [Story] 描述` —— `[P]` 表示可并行（不同文件、无依赖）。

---

## Phase 0：基线核实

- [ ] **T001** 以 011 合入后的 `server/internal/content/diagnostics/simulator.go` 为基线，重新核实 spec.md「Current State」第 1 节（父子成链、时间首尾相接、`len(steps) == 9`）。**若 011 改变了步进循环的结构**，research D1 的分流点要重定，并在实现 PR 正文的「规格修正」一节写明。
- [ ] **T002** 核实 spec.md §1a 的读取一侧链路在 011 之后仍然成立：`GetRun` 的 `Limit: 100`、`Store.Query` 的 100 夹取、`STREAM_EVENT_CAP = 200`、`index.tsx:351` 的传入。任一数值变了，FR-005 的做法跟着改。

---

## Phase 1：对外形状与解析（先做，US3 与所有后续任务都依赖它）

- [ ] **T003** [US3] `server/internal/content/diagnostics/simulator.go`：`Scenario` 增加 `Kind string \`json:"kind"\``，现有 16 项全部标 `"fault"`。**不改 id、不改 `Expected`。**
- [ ] **T004** [US3] 先写失败测试 `server/internal/content/diagnostics/simulator_test.go`（或新建 `scenario_kind_test.go`）：`Kind == "fault"` 的 id 集合**逐项等于**那 16 项（data-model INV-1）；每一项的 `Kind` 非空且 ∈ {`fault`, `shape`}。确认失败后再让 T003 让它变绿。
- [ ] **T005** [P] [US3] `packages/core/content/diagnostics/contract.ts`：`overviewSchema` 的 `scenarios` 增加**可缺省**的 `kind`，缺省落 `"fault"`（contracts/scenario-kind.md C-1）。
- [ ] **T006** [P] [US3] `packages/core/content/diagnostics/contract.test.ts`：三条畸形响应测试 —— 缺 `kind`、`kind` 为非字符串、正常响应（contracts/scenario-kind.md「必须附的测试」）。**先写，确认失败。** 这是 constitution 原则 VI 要求的，不是可选项。

---

## Phase 2：US1 —— 九个形态（P1）

### 2a. 形态骨架

- [ ] **T007** [US1] 新建 `server/internal/content/diagnostics/shapes.go`：shape 查表（`ID` / `Items` / `SkipEvaluate` / `FailSink` / `Status` / `Module` / `EmptyModule`，见 data-model §2），九个 id 各一行，`Scenarios` 追加这九项并标 `Kind: "shape"`。
- [ ] **T008** [US1] `simulator.go`：在参数校验之后、`steps` 循环之前对 shape 场景**早分流**。**业务故障场景的代码路径逐字节不变**（research D1）—— 用 `git diff` 确认循环体无改动。
- [ ] **T009** [US1] 先写失败测试：每个 shape id 都能被 `Simulate` 接受（不返回 `ErrConflict`）；`Kind == "shape"` 的 id 与 `docs/13` §7 的 15 项**交集为 0**（INV-2）；每一项的 `Items` 非空且指到真实条目编号（INV-3 / SC-003）。

### 2b. 瀑布形态

- [ ] **T010** [P] [US1] `shape_concurrent`：两个 span 同父、时间**可见重叠**。测试锁**重叠量**（≥ 二者较短时长的 1/4 且 ≥ 100ms），不只是「重叠」（contracts §1）。→ `006-W-3`
- [ ] **T011** [P] [US1] `shape_orphan`：父 id 用固定 16 位小写十六进制字面量。测试**三条**：通过 `hexID`；不出现在本次运行任何 `Span` 里；**经 `Sanitize` 之后仍然非空**（contracts §2 —— 这是本特性最容易悄悄失败的一处）。→ `006-W-5`
- [ ] **T012** [P] [US1] `shape_single_span`：`len(run.Events) == 1`，`Parent == ""`。→ `006-W-6`
- [ ] **T013** [US1] `shape_deep`：> 200 个事件（250），层级嵌套，含错误码的 span 位于深层且各级父节点齐全。→ `006-W-7` / `W-8` / `008-O-3`
- [ ] **T014** [US1] `server/internal/content/diagnostics/store.go`：`GetRun` 用既有 `Filter.After` 游标**内部翻页**取全该次运行的事件，天花板 **500**。**不动 `Store.Query` 的 100 夹取，不动 `handler.diagnosticFilter` 的 `Limit > 100 → ErrConflict`**（data-model §5）。
- [ ] **T015** [US1] 先写失败测试（需 DB）：一次 `shape_deep` 之后 `GetRun` 返回的事件数 **= 运行实际事件数**，不是 100；且 > 200。
- [ ] **T016** [P] [US1] `packages/core/content/diagnostics/trace-waterfall.test.ts`：250 个事件、失败 span 在深层 → 默认 cap 下 `collapsed == true` 且带错误码的 span 及其各级父节点仍在 `rows` 里；`cap = len` 下 `collapsed == false` 且层级完整。**这是 core 的纯函数测试，不是 UI 测试。**

### 2c. 回归状态

- [ ] **T017** [P] [US1] `shape_not_run`：`SkipEvaluate = true`；`service.go` 的 `Run` 按 shape 表决定是否调 `Evaluate`。测试**必须查库**断言 `content_diagnostic_run` payload 里 `regression == "not_run"` —— 断言在 `Simulate` 返回值上的话，不改任何代码它也是绿的（contracts §5）。→ `006-V-1`
- [ ] **T018** [P] [US1] `shape_regression_failed`：**不覆盖 `Regression`**，让 `Expected != Actual`，由 `Evaluate` 自己写出 `failed`（research D6）。→ `006-V-3`
- [ ] **T019** [P] [US1] `shape_undecidable`：`Regression = "passed"` 且 `Status ∉ {completed, failed}`。**MUST NOT 改动 `describeRegressionVerdict`**（FR-008）。补一条 `packages/core/content/diagnostics/regression.test.ts` 断言该组合判为 `undecidable`。→ `006-V-5`

### 2d. 关联信息

- [ ] **T020** [P] [US1] `shape_no_module`：`EmptyModule = true` → `Run.Module == ""` 落库。测试查库断言 payload 的 `module` 为空字符串。→ `006-L-4`（`module` 半）

### 2e. 计数器

- [ ] **T021** [US1] `store.go`：`Store.Technical` 增加失败开关参数，照 `CommitRun` 的 `failAudit` 既有模式办（research D5）。`service.go` 的 `Run` 是**唯一** `true` 来源，条件是该场景 `FailSink`。
- [ ] **T022** [US1] `shape_sink_failure` 测试（需 DB）：跑完后 `Overview.metrics.sink_errors > 0` 且 `dropped > 0`；**紧接着跑一次 `normal`，两个计数不再增长**（SC-005）；`content_operation_audit` 里**有**该次运行的审计行（证明「审计写失败即整体回滚」未被触碰）。→ `002-V11-1` / `002-V11-2` / `002-V05-15`

---

## Phase 3：US2 —— 隔离与不变量（P1，优先级更硬）

- [ ] **T023** [US2] 新建 `server/internal/content/diagnostics/shapes_isolation_test.go`：对**每一个** shape id 各一条 —— `testEnabled == false` 时 `Simulate` 返回 `ErrDenied`，**且 `content_diagnostic_run` 无新行**。逐个场景一条，**不是整体一条**（research D9）。
- [ ] **T024** [US2] 同上文件：对每一个 shape id 各一条 —— `scope.Allows` 为假时 `ErrDenied` 且无记录。
- [ ] **T025** [US2] `Sanitize` 允许值表成员**逐项一致**的断言（`codes`、`oneOf` 各组、`headerValueAdmitted` / `headerPresenceAdmitted` / `headerDenied` / `headerDeniedSuffix`）。**先在 `contract_test.go` / `log_test.go` 里找既有等价断言**；有就只在对照表里补引用，**不重复写**（010 定下的口径）。
- [ ] **T026** [US2] 确认本特性**一个 `os.Getenv` 都没加**：`git diff` 过一遍，`server/internal/content/diagnostics/` 下 `os.Getenv` 出现次数为 0（FR-016）。
- [ ] **T027** [US2] 确认 `server/internal/daemon/` 与任何上游 Multica 代码改动行数为 **0**（SC-007）：`git diff --stat` 贴进 PR 正文。
- [ ] **T028** [US2] 确认新增 UI 单测数为 **0**（SC-008）：`packages/views/` 与 `apps/` 下无新增 `*.test.tsx`。

---

## Phase 4：文档回写（FR-014 / FR-022，交付的一部分，不是收尾）

- [ ] **T029** [US3] `docs/development/diagnostics-acceptance-mapping.md`：重写 §7 对表说明**三处** —— `DIAG-11` 行、§3 的 `V07` 行、§4.2 的 `D13-V07` 行。口径改为「`Kind == "fault"` 的 16 项全部覆盖 §7 的 15 项；`Kind == "shape"` 的 9 项与 §7 交集为 0」。**MUST NOT 停留在「16 项」这个过时数字上**（US3 场景 3）。
- [ ] **T030** [US1] `docs/development/manual-ui-runbook.md`：把 quickstart.md 的造数步骤写进相关条目的备注，**把「缺少对应数据」全部换掉**。必须写明的三件事：`shape_deep` 那一步**会明显变慢，不要刷新**（research D7）；`002-V11-2` 的 `dropped` 走的是**写失败**路径而非原文括注的满环路径（research D5）；`006-L-4` 本形态补的是 **`module` 那一半**，`build` 半今天已可验（research D8）。
- [ ] **T031** [P] [US1] `specs/006-diag-trace-waterfall-regression/manual-ui-todo.md`：`W-3` / `W-5` / `W-6` / `W-7` / `W-8` / `V-1` / `V-3` / `V-5` / `L-4` 备注改为造数步骤；**`L-5` 改为移出理由与替代验证方式**（保留期 `LORETIDE_DIAG_RETENTION_DAYS=1` 或跨工作区引用，FR-020）。
- [ ] **T032** [P] [US1] `specs/008-diag-linkage-and-invariants/manual-ui-todo.md`：`O-3` 备注改为 `shape_deep` 的造数步骤；**`O-2` 改为「今天已可执行：点『触发页面错误（测试）』」**（FR-021），MUST NOT 保留失效旧备注。
- [ ] **T033** [P] [US1] `specs/002-diag-package-stream-recovery/manual-ui-todo.md`：`V11-1` / `V11-2` / `V05-15` 备注改为 `shape_sink_failure` 的造数步骤，并写明来源路径的区别。
- [ ] **T034** [US1] 记录两条**已知界限**：`Service.Export` 的 `Filter{Limit: 100}` 会把 250 span 运行的导出截在 100 条技术事件（既有行为，本特性不改，原则 VIII）；满环路径的 `dropped` 需连跑 4 次 `shape_deep`。写进 runbook 与 `contracts/simulator-shapes.md` 已有的对应小节。

---

## Phase 5：验证与交付

- [ ] **T035** `cd server && go test ./internal/content/diagnostics/... -count=1`，需 DB 的用例设 `LORETIDE_DIAG_TEST_DATABASE_URL` 指向**独立库**实跑。**报 PASS / SKIP 计数**，贴进 PR 正文。SKIP 逐条给原因。
- [ ] **T036** `pnpm typecheck`；`pnpm test --filter @multica/core`（只跑 core，不跑整套，避免拉进 UI 测试）。
- [ ] **T037** 变异验证：对**每一条新规则各挑一处**做反向改动，确认对应断言变红。至少覆盖 —— 孤儿父 id 改成非法（T011 第三条断言必须红）；`GetRun` 的翻页改回单次 `Limit: 100`（T015 必须红）；`SkipEvaluate` 置假（T017 必须红）；`FailSink` 置假（T022 必须红）。**断言不变红的，说明它没在验它声称验的东西。**
- [ ] **T038** 交付记录：逐条列出「已覆盖 / 已移出 / 仍不可达」三类（FR-001）。已覆盖 13 条、已移出 2 条、合计 15 条；**任务描述所说的 19 条与此差 4 条，在 PR 正文里明确点出这个差额仍未补全**（spec Q1）。
- [ ] **T039** PR 正文的「规格修正」一节：列出实施期间发现的规格与实现不一致处，并在同一个 PR 里改掉规格（既定口径）。
- [ ] **T040** `tasks.md` 回勾：已完成项 `[x]`；未真正执行的项保留 `[ ]` 并注明原因（constitution 原则 X —— 打勾只表示实现任务已交付，不表示矩阵条目通过）。

---

## 依赖关系

```text
T001,T002  →  T003..T006  →  T007,T008,T009
                                  ↓
              ┌───────────────────┼───────────────────┐
           T010..T012          T013→T014→T015→T016   T017..T020   T021→T022
              └───────────────────┴───────────────────┘
                                  ↓
                            T023..T028
                                  ↓
                            T029..T034
                                  ↓
                            T035..T040
```

- **T003 必须最先**：其余任务都要往 `Scenarios` 里加项，字段不在就没法标分类。
- **T014 必须在 T013 之后、T016 之前**：没有取全事件，瀑布测试断言的是一个到不了的状态。
- **T021 必须在 T022 之前**：开关不存在，测试无从写起。
- **T029..T034 不是收尾**：FR-014 / FR-022 是需求，不是文档整理。文档没改，`SC-009` 不成立。

## 并行机会

- T005、T006 与 T003、T004 分属前后端，可并行。
- T010、T011、T012 三个形态互不相干，可并行。
- T017、T018、T019、T020 四个形态互不相干，可并行。
- T031、T032、T033 是三份不同的 todo 文件，可并行。

## 不做的事

- 不碰 `server/internal/daemon` 与任何上游 Multica 代码。
- 不新增表、不新增迁移。
- 不放宽 `Sanitize` 白名单，不改「审计写失败即整体回滚」，不改真实执行器的禁用状态。
- 不改 `Store.Query` 的 100 上限，不改 `handler.diagnosticFilter` 的对外分页边界。
- 不改 `buildTraceWaterfall` 的默认 cap，不改 `describeRegressionVerdict`。
- 不改面板下拉的分组呈现（shape 场景进入既有下拉是列表驱动的，零 UI 改动即可选中）。
- 不写 UI 单测。
