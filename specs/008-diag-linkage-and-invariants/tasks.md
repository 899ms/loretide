---
description: "Task list for 008 diagnostics linkage and invariants"
---

# Tasks: 诊断关联跳转与不变量断言

**Input**: Design documents from `/specs/008-diag-linkage-and-invariants/`

**Prerequisites**: plan.md、spec.md（必读）；research.md、data-model.md、contracts/、quickstart.md

**改动文件必须在 plan.md → Source Code 清单内。** 清单外的文件一律不改。

**测试先写**：标注「先写」的任务必须在对应实现之前完成，并**确认其失败**后再写实现。只写了测试没确认过失败的，不算完成。

**四项 clarify 已裁决（全部 A）**，无待决问题。

---

## Phase 1: Setup

- [ ] T001 阅读 `docs/development/design/README.md`（原则 VII 强制：改任何 Web 页面前必读），确认 G1/G2 的界面改动只复用既有 `Button` 禁用态与既有排版，不自造控件；把结论一句话记在实施 PR 正文
- [ ] T002 确认本地 PostgreSQL 可用并导出 `LORETIDE_DIAG_TEST_DATABASE_URL` 与 `MULTICA_TEST_DATABASE_URL`（见 `quickstart.md` 前置）；未设变量时相关用例会跳过，**跳过不得记为通过**
- [ ] T003 记录基线：在**未改动**的代码上跑 `quickstart.md` 的五条命令并保存输出到工作区外的临时文件。**`baseline.txt` 不提交**，内容进实施 PR 正文

---

## Phase 2: Foundational

无阻塞性前置。六项彼此独立，G1/G2 共用一个新文件，G3～G6 各自独立。

---

## Phase 3: User Story 1 — 从审计事件落到技术追踪（P1，G1）

**Goal**：把跳转推导抽成可断言的纯函数，并修掉「追踪编号为空时静默跳到全量日志」的缺陷。

**Independent Test**：`pnpm --filter @multica/core exec vitest run content/diagnostics/linkage.test.ts` 通过，且空 `traceId` 一条能在未修复的实现上变红。

- [ ] T004 [US1] **先写** `packages/core/content/diagnostics/linkage.test.ts` 中 `describeTraceJump` 的 6 行边界表用例（见 `contracts/trace-jump.md`），文件首行必须是 `// @vitest-environment node`（无 DOM 需求）。关键用例：`traceId` 为空时断言**返回的不是 filter 分支**，仅断言 `reason` 不够。确认这些用例在函数尚不存在时失败
- [ ] T005 [US1] 在 `packages/core/content/diagnostics/linkage.ts` 实现 `describeTraceJump(event): TraceJump`，产出 `{kind:"filter"|"unavailable"}` 互斥联合；filter 含 `kind:"technical"`、`traceId`、`runId`（可为空串，空则不参与筛选）、`after:0`；`component`/`severity`/`errorCode`/`from`/`until` 一律不带。`traceId` 为空或非 32 位十六进制时返回 `{kind:"unavailable", reason:"noTrace"}`
- [ ] T006 [US1] 在 `packages/core/content/diagnostics/linkage.ts` 补文件头注释，写明「因原则 II 禁止 UI 单测，凡需测试证明的逻辑必须放在测试够得着的地方」，并指向 `contracts/trace-jump.md`（与 `trace-waterfall.ts`、`regression.ts` 的既有写法一致）
- [ ] T007 [US1] 修改 `packages/views/content/diagnostics/index.tsx` 的 `onTrace`：改为调用 `describeTraceJump(event)`，`filter` 分支按结果设置既有 state 并切到技术日志页；`unavailable` 分支**不切页**。删除原先那串内联 setter，不保留双路径（原则 VIII：不留兼容层）
- [ ] T008 [US1] 修改 `packages/views/content/diagnostics/index.tsx`：「查看追踪」按钮在 `unavailable` 时置为 `disabled` 并显示说明文案，复用既有禁用态样式（T001 的结论），不新建控件
- [ ] T009 [P] [US1] 在 `packages/views/locales/{en,zh-Hans,ja,ko}/common.json` 的 `diagnostics` 段**四语言同时**新增「无关联追踪」说明文案键，键名接续现有 `text114` 之后；参照 `apps/docs/content/docs/developers/conventions.zh.mdx` 的中文产品口吻
- [ ] T010 [US1] 核对 `specs/008-diag-linkage-and-invariants/manual-ui-todo.md`（**已随 spec PR 建好**）的 J-1 ～ J-4 与实际交付的界面行为一致，全部保持「待用户验证」。**不写 UI 单测**

---

## Phase 4: User Story 2 — 下一动作的有效性判定（P1，G3）

**Goal**：断言映射对全部场景错误码穷尽、补齐唯一未断言的分支、成对断言动作与可重试。

**Independent Test**：`cd server && go test ./internal/content/diagnostics -run 'TestNextAction' -count=1 -v`，A1～A5 用例名逐条 PASS。

**本阶段不改任何生产代码**——映射已存在于 `log.go:99-100`。

- [ ] T011 [US2] **先写** `server/internal/content/diagnostics/next_action_test.go`：声明封闭动作集合（`inspect_trace` `retry_simulation` `check_authorization` `check_registered_file` `check_local_client`）于**测试内**（不导出到生产代码，见 research.md R5），实现断言 A1——遍历 `Scenarios` 全部 16 项的 `Expected`（含 `normal`/`slow` 的空码），每个经 `Sanitize` 后 `Next` 非空且属于该集合
- [ ] T012 [US2] 在同文件补断言 A2：`FILE_MISSING` 与 `FILE_CHANGED` 均得 `check_registered_file`——这是当前**唯一未被断言**的分支
- [ ] T013 [US2] 在同文件补断言 A3 与 A5：未知码（如 `NOT_A_REAL_CODE`）与空码均得默认 `inspect_trace` 而非空值
- [ ] T014 [US2] 在同文件补断言 A4：逐分支断言 `Next` 与 `Retryable` 成对——仅四个可重试码为 `true`，其余全部 `false`
- [ ] T015 [US2] 执行 `contracts/next-action.md` 的**四处变异验证**（删 `check_registered_file` 分支 / 加映射不到的场景码 / 改某可重试码的 `Retryable` / 把默认 `Next` 改空），每处确认至少一条用例变红，**改完即还原**。把四次结果记进实施 PR 正文。未做变异验证的不得在对照表写「已闭合」

---

## Phase 5: User Story 3 — 快照不含媒体载荷（P2，G4）

**Goal**：用字段清单断言把「快照无二进制载荷」从结构巧合变成被断言的性质。

**Independent Test**：`cd server && go test ./internal/content/diagnostics -run 'TestSnapshotHasNoMedia' -count=1 -v`。

- [ ] T016 [P] [US3] **先写** `server/internal/content/diagnostics/snapshot_media_test.go`：用反射枚举 `Snapshot` 全部字段名与类型，与测试内的预期清单逐一比对。清单为 data-model.md 列出的 16 个字段：`ConfigVersion` `PersonaRef` `SOPVersion` `SkillVersion` `RuleVersion` `Executor` `ExecutorVersion` `Scope` `Preference` `Required` `Excluded` `Grants` `Hashes` `Temperature` `Budget` `Timeout`。任一处不匹配（新增/删除/改名/改类型）即失败
- [ ] T017 [US3] 变异验证：给 `Snapshot` 临时加 `Blob []byte` → 确认变红；再加一个合法的 `string` 字段 → **也必须变红**（字段清单相对「只查类型」的全部价值在此）。两次改完即还原，结果记进 PR 正文

---

## Phase 6: User Story 4 — 复现不回写偏好（P2，G5）

**Goal**：把埋在确定性循环里的顺带断言，提升为用非夹具值的具名性质断言。

**Independent Test**：`cd server && go test ./internal/content/diagnostics -run 'TestReproduce' -count=1 -v`。

- [ ] T018 [P] [US4] **先写** `server/internal/content/diagnostics/reproduce_preference_test.go`：落一条原运行，其快照 `Preference` 设为**非夹具值**（不得是 `"all"`，否则分不清「真的没改」与「碰巧相等」——FR-015）；以它为 `original` 发起复现；复现后**重读原运行**，断言其 `Snapshot.Preference` 与其它快照输入字段均未变
- [ ] T019 [US4] 在同文件补断言：复现产出的新运行携带自己的快照，且 `Original` 指向原运行；原运行本身未被任何写入路径修改（`Service.Run` 对 `original` 只 `GetRun` 读取，见 research.md R4）
- [ ] T020 [US4] 变异验证：让复现路径写回原运行的偏好 → 确认变红，改完即还原

---

## Phase 7: User Story 5 — 跨层链条端到端（P3，G6）

**Goal**：从一次真实运行出发逐跳走完「审计事件 → 追踪 → 运行 → 原故障运行」，断言标识符对得上。

**Independent Test**：`cd server && go test ./internal/handler -run 'TestContentDiagnosticLinkage' -count=1 -v`，**必须带 `-v` 逐条核对用例名**。

- [ ] T021 [US5] **先写** `server/internal/handler/content_diagnostics_linkage_test.go`：用 `internal/testutil` 的 `dbfx` 建数据、用 `testutil.Call(h, req).Want(status).JSON(&out)` 驱动 handler（**不得**开写 `INSERT ... RETURNING id` 或 `httptest.NewRecorder()` 四件套，见 `CLAUDE.md` → Testing）。第一跳：真实发起一次复现后，从审计事件取 `trace_id` 查技术事件，断言至少一条且其 `run_id` 等于该次运行
- [ ] T022 [US5] 在同文件补第二、三跳：技术事件 `run_id` → 运行；运行 `original_run_id` → 原运行，断言查到的正是那次历史运行。逐跳断言**标识符对得上**，不是各跳分别非空（FR-017）
- [ ] T023 [US5] 在同文件补 FR-018 负例：把链条中某一跳的标识符置空后查询，断言返回「查不到」而**不是**一堆无关记录。理由：`store.go:163` 的 SQL 是 `($4='' OR payload->>'trace_id'=$4)`，**空值即不加条件**，拿空 trace 查会返回全部事件。没有这条负例，「链条走通」可能只是每跳都匹配到了无关记录
- [ ] T024 [US5] 用 `-v` 核对 T021～T023 的用例名**真实执行**。`handler_test.go` 的 `TestMain` 在数据库连不上时 `os.Exit(0)`，整个包会以退出码 0「通过」而一个用例都没跑——**只看退出码会得到假绿**

---

## Phase 8: User Story 6 — 对象与版本的呈现（P2，G2）

> **为什么 P2 排在 P3（Phase 7）之后**：US6 与 US1 共用 `linkage.ts` 和 `index.tsx` 的同一区域，必须串在 US1 之后；而 US5（P3）是独立的 Go 测试，可以先落。这里按**文件依赖**而非纯优先级排序，是刻意的。

**Goal**：在不新增数据读取路径的前提下，让读者能从事件看到对象及其版本。

**Independent Test**：`linkage.test.ts` 中 `describeObjectVersions` 的 6 行边界表通过。

- [ ] T025 [US6] **先写** `packages/core/content/diagnostics/linkage.test.ts` 中 `describeObjectVersions` 的 6 行边界表用例（见 `contracts/trace-jump.md`），含「同 `objectId` 不同 `objectType` 不合并」与「计数守恒」。确认在函数不存在时失败
- [ ] T026 [US6] 在 `packages/core/content/diagnostics/linkage.ts` 实现 `describeObjectVersions(events, target): ObjectVersionGroup`：在**传入数组内**按 `objectType`+`objectId` 归拢 `objectVersion`，去重且按**首次出现顺序**（非字典序），每项带 `firstEventId` 与 `count`；`state` 取 `present`/`unversioned`/`unknownObject`。**不发起任何请求**（FR-004、FR-019）
- [ ] T027 [US6] 修改 `packages/views/content/diagnostics/index.tsx`：展示对象类型/标识/版本；`unversioned` 显示「未记录版本」，**不显示空白、不伪造版本**（FR-007）；`unknownObject` 不显示该区块。数据取自**已取回并经 `eventSchema` 解析**的事件，不新增查询
- [ ] T028 [P] [US6] 在 `packages/views/locales/{en,zh-Hans,ja,ko}/common.json` 四语言同时新增「未记录版本」与对象版本区块所需文案键
- [ ] T029 [US6] 核对 `manual-ui-todo.md` 的 O-1 ～ O-3 齐全，其中 **O-3 必须写明范围说明要求**——只显示当前页内的版本，呈现不得让读者误以为这是该对象的全部版本（Q2-A 的已知代价）

---

## Phase 9: Polish & Cross-Cutting

- [ ] T030 修正 `docs/development/diagnostics-acceptance-mapping.md` **§5 的三处不准确描述**（主任务裁决：不在 spec PR 改，留到本实施 PR 一并修）：**G1** 改为「跳转已存在于 `index.tsx:531` 的内联 setter，缺的是可测性与空追踪编号的处理」；**G3** 改为「映射已存在于 `log.go:99-100` 且 4/5 分支已断言，缺的是穷尽性与 `check_registered_file` 分支」；**G5** 改为「Go 字段名为 `Preference`，`simulator_test.go:8` 与 `snapshot_regression_test.go:48` 已引用，缺的是具名断言与非夹具值」。每处注明原描述错在哪、证据在哪
- [ ] T031 刷新 `docs/development/diagnostics-acceptance-mapping.md` 的 §2 相关行与 §3/§4.2 的 D13-V02/V04/V06/V09/V10 状态。**状态只依据实跑用例**，不因 PR 合并而标通过；手动条目一律「待用户验证」
- [ ] T032 用脚本按 §2 各表「类型」列**重新统计** §4.1 的 13 行计数（不手工累加——PR #26 手写时 13 行错了 4 行），并同步 §4.3 的手动条目总数由 57 改为 **64**（新增本特性 7 条）
- [ ] T033 在 §6 新增本次的证据块，列出实际命令与退出码。vitest **逐文件指定**，不用 `content/diagnostics/` 目录通配（通配会把原则 II 排除的 `queries.test.tsx` 一并跑掉）；`middleware` 包的 17 条 SKIP 需如实写明是 Redis 相关、与本特性无关，**不得写成「无 SKIP」**
- [ ] T034 在 `docs/development/diagnostics-acceptance-mapping.md` 记录**后续条目**（不修，原则 VIII）：复现每次构造固定夹具快照、从不还原原运行快照，因此「复现」并不真的还原原始输入。关系到 D13-V06/V10 的「复现清单」语义，交主任务排期。**从 US4 移到本阶段**——对照表回写必须在全部用例实跑之后，与 T030～T033 同批，避免同一文件跨阶段争用
- [ ] T035 跑 `quickstart.md` 的全特性回归：`pnpm check:content-boundaries`、`pnpm check:diagnostics-contract`、`pnpm typecheck`、Go 与 core 测试，记录退出码。**不跑全量 `pnpm test` / `make test` / Playwright**
- [ ] T036 核对实施 PR 的改动文件**全部落在 plan.md → Source Code 清单内**；清单外若有改动，删掉或在 PR 正文单列说明

---

## Dependencies

```
Setup (T001-T003)
   │
   ├─ US1 G1  (T004→T005→T006→T007→T008, T009 ∥, T010)
   │      └─ US6 G2 (T025→T026→T027, T028 ∥, T029)   # 共用 linkage.ts / index.tsx，串行
   │
   ├─ US2 G3  (T011→T012→T013→T014→T015)      ∥ 独立
   ├─ US3 G4  (T016→T017)                      ∥ 独立
   ├─ US4 G5  (T018→T019→T020)                 ∥ 独立
   └─ US5 G6  (T021→T022→T023→T024)            ∥ 独立
              │
              └─ Polish (T030-T036) 需全部完成
```

**US6 依赖 US1**：两者共用 `linkage.ts` 与 `index.tsx` 同一区域，并行会冲突。

**US2/US3/US4/US5 彼此完全独立**，是纯 Go 测试，互不共享文件，可并行。

## Parallel Opportunities

- T009 与 T028 都改 locales 四语言文件——**不可同时**，但各自与本阶段其它任务并行。
- US2、US3、US4、US5 四个阶段可同时推进（四个新 Go 测试文件互不相干）。
- T016、T018 标 [P]：分属不同文件、无共享前置。

## Implementation Strategy

**MVP = US1（G1）**：它是整张对照表引用最多的排障路径，且顺带修掉一个会误导读者的真实缺陷。单独交付即有价值。

**第二批 = US2 + US3 + US4 + US5**：四项纯测试，无生产改动，风险最低，可并行。

**第三批 = US6**：依赖 US1 的文件，且带一个已知限制（跨页看不全），放最后。

**最后 = Polish**：对照表回写必须在全部用例实跑之后，否则又会出现「因为合并了就标通过」——这正是 §5 三处描述出错的成因。
