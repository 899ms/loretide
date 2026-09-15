# Tasks: 计划轮换页必须在窗口内送达

**Input**: `/specs/013-diag-stream-rotation/` 的 spec.md、plan.md、research.md、contracts/

## Loretide 测试口径（不可协商）

- **不写、不跑 UI 单测。**
- 需要数据库的 Go 用例设 `DATABASE_URL` **实跑**，报 PASS / SKIP / FAIL 三个计数。
- 前端逐文件指定 vitest，不用目录通配。
- **没跑的检查记「未执行」，不记通过。**

## 格式

`[ID] [P?] [Story] 描述` —— `[P]` 表示可并行。

---

## Phase 1：先让缺陷有一条会红的测试

- [x] T001 [US2] `server/internal/handler/content_diagnostics_test.go`：把 `TestContentDiagnosticStreamClosesPlannedWindowWithRotate` 的窗口由 `50ms` 改为 **3s**（ticker 周期的整数倍，走生产路径），并**新增一条断言：整条流的结束时刻严格早于窗口边界**。确认在改动前的代码上**变红**——红的原因必须是「结束时刻晚于边界」，不是「末页没有 rotate」。
- [x] T002 [P] [US2] 同文件新增退化形态用例：窗口短于一个 ticker 周期时，**首页即轮换页、总页数为 1**（research D6）。改动前应红（当前实现会写两页）。

---

## Phase 2：修复

- [x] T003 [US1] `server/internal/handler/content_diagnostics.go`：把轮换判定提到写之前——用 `end := time.Now().Add(diagnosticStreamWindow)`，每轮以 `!time.Now().Add(tick).Before(end)` 决定这一页是否为轮换页；为真则写出后 `return`。**删掉 `deadline` 定时器**（窗口边界变成一次比较，不再是要竞争的 channel，顺带消掉 deadline 与 ticker 同时就绪时 select 的随机性）。
- [x] T004 [US1] 确认 T001 / T002 转绿，且 `TestContentDiagnosticStreamEndsWithoutRotateWhenMembershipIsRevoked` 与 `TestContentDiagnosticStreamAppliesRequestedFilter` **不改自身仍然通过**（FR-005 / D7）。

---

## Phase 3：端到端断言（FR-007）

- [x] T005 [US1] 生成夹具 `rotation-last-page.json`：跑一个真实窗口，取最后一页的原始 JSON 字节，提交进仓库。
- [x] T006 [US1] Go 侧：新增断言，真实窗口的最后一页原始字节与夹具**逐字节相等**。
- [x] T007 [US1] TS 侧：`packages/core/content/diagnostics/stream-state.test.ts` 读**同一份夹具** → `pageSchema` 解析 → `nextStreamState` → 断言 `status !== "disconnected"` 且 `reconnectInMs === 0`。**不得手搓对象。**

---

## Phase 4：范围自证与文档

- [x] T008 [P] [US2] 自证 `apps/web` 代理配置改动行数为 **0**（SC-007）。
- [x] T009 [P] [US2] 自证 `server/internal/daemon` 与上游 Multica 其它代码改动行数为 **0**（SC-008），新增 UI 单测数为 **0**（SC-009）。
- [x] T010 [US2] 把 `002-V05-03` 的口径冲突写进 `docs/development/manual-ui-runbook.md` 的对应位置：**记录 + 建议，不解决**（FR-012）。

---

## Phase 5：验证

- [x] T011 `DATABASE_URL` 实跑 `go test ./internal/handler/ -count=1`，报三个计数。
- [x] T012 前端逐文件跑 `stream-state.test.ts` 与 `contract.test.ts`。
- [x] T013 `pnpm typecheck --force`（若无 TS 产品代码改动则说明）。
- [x] T014 变异验证：把判定改回「写之后」，T001 的时刻断言必须变红。
- [x] T015 `tasks.md` 回勾；未真正执行的项保留 `[ ]` 并注明原因。**`SC-001` 只能由主任务在真实浏览器给出，本 PR 记「未执行」。**

---

## 依赖

```text
T001,T002  →  T003  →  T004  →  T005  →  T006,T007
                                             |
                                       T008,T009,T010
                                             |
                                       T011..T015
```

## 不做的事

- 不动代理配置（FR-010）。
- 不改 `stream-state.ts` 的产品代码——它今天已经正确。
- 不解决 `002-V05-03`，只记录与建议。
- 不碰 daemon 与上游 Multica 其它代码；不写 UI 单测。
