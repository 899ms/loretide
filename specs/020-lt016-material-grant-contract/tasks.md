---
description: "Task list for 020 material and task grant contract (LT-016)"
---

# Tasks: 素材与任务授权范围合同（LT-016）

**Prerequisites**: spec.md、plan.md、contracts/material-grant.md

**改动文件必须在 plan.md → Source Code 清单内。**

**三项 clarify 已裁决（全部 A）**，并追加一条：缓存失效写成硬要求，本卡不实现缓存。

**零迁移、零端点、零界面、零 TS 改动。**

---

## Phase 1: Setup

- [x] T001 确认本机 PostgreSQL 可用并导出 `DATABASE_URL`（`TestMain` 读的是它；设错会 `os.Exit(0)` 打印 `ok` 而一个用例都没跑）
- [x] T002 记录基线：`go test ./internal/content/... ./internal/handler/`、三项 check，以便区分回归与基线

---

## Phase 2: 判定（先写矩阵）

- [x] T003 **先写** `server/internal/content/workspace-core/grant_test.go` 的**权限矩阵夹具**：合同 §7 的 12 行逐行表驱动，期望值写在表里而不是散在断言里。确认失败
- [x] T004 [P] **先写**边界用例：零值授权、引用缺字段、多份授权取其一、品牌级授权覆盖账号级资源、拒绝理由取最接近的那一个。确认失败
- [x] T005 新建 `grant.go`：`Principal` / `ResourceKind` / `ResourceRef` / `Grant` / `GrantReason` / `GrantDecision` / `ReadRequest` / `CanRead`，按合同 §3 的顺序判定
- [x] T006 拒绝时记技术事件，沿用 `authz.go` 的 `record` 路径（同一个 `Recorder`、同一个 `Code`），并断言事件**不含资源 id 与正文**
- [x] T007 改 `authz.go` 的包注释那一句：把「不回答归属」改精确为「不替调用方查表；对调用方交来的事实作判定是本包的事」。**只改这一句**

---

## Phase 3: 提示词不是凭证

- [x] T008 **先写** `server/internal/handler/content_grant_prompt_test.go`：在同一品牌下建两个真实账号，用 LT-012 的端点把**两者的人设提示词设为逐字节相同**，再对 B 的资源以 A 的授权调 `CanRead`，断言**仍然拒绝**。确认失败（或说明为何一开始就是绿的）
- [x] T009 确认该用例不是空的：说明它能抓住什么样的改动

---

## Phase 4: 合同与文档

- [x] T010 `contracts/material-grant.md` 的缓存一节写成**硬要求**，并列出实现缓存的那张卡必须同时交付的三样东西
- [x] T011 跑 `pnpm check:diagnostics-contract`，确认仍为 `checked 3 landed modules`（本卡不新增模块目录，若变成 4 说明建错了地方）

---

## Phase 5: Polish

- [x] T012 变异验证三处（contracts §8 的 M1–M3），每处确认对应矩阵行变红、**改完即还原**。变异必须**可编译**
- [x] T013 跑全部验证：`go test` 报三计数、三项 check；**说明 typecheck 不涉及**（无 TS 改动）；**说明零迁移**
- [x] T014 核对改动文件全部落在 plan.md 清单内；清单外的在 PR 正文单列
- [x] T015 PR 正文按模板：UI 影响写「无」、零迁移说明、接入合同项、「SOP 对应」写明**本 PR 让 W-02 出口 CP-06 还差什么**

---

## Dependencies

```
Setup (T001-T002)
  └─ 判定 (T003,T004 先写 [P] → T005 → T006 → T007 注释)
       └─ 提示词 (T008 先写 → T009)
            └─ 合同 (T010 → T011)
                 └─ Polish (T012-T015)
```

**T003/T004/T008 必须先写并确认失败**：否则无法区分「实现对」与「测试跟着实现走」。

## Implementation Strategy

1. **矩阵先行**：这张卡的产出就是一张判定表，先把表写出来，实现只是让它变绿。
2. **拒绝理由单独对待**：理由不进响应、只进日志，所以它的正确性只能靠用例保证——没有别的地方会暴露它写错了。
3. **提示词那条单独成段**，因为它跨两个模块，混在矩阵里会被写成一个假设而不是一次真实验证。
