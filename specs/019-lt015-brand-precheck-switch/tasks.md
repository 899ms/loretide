# Tasks: 品牌级自动预检开关（LT-015）

**Input**: `/specs/019-lt015-brand-precheck-switch/` 的 spec.md、plan.md、contracts/

## 测试口径（不可协商）

- **不写、不跑 UI 单测。** 默认值、非法值、读不回写三类逻辑进 core 的 **node** 纯函数测试。
- Go 用例设 `DATABASE_URL` **实跑**，报 PASS / SKIP / FAIL 三个计数。
- 前端逐文件指定 vitest。
- **没跑的检查记「未执行」，不记通过。**

---

## Phase 1：core 纯函数（先写测试）

- [x] T001 [US1] `packages/core/workspace/auto-precheck.test.ts`（`// @vitest-environment node`）：先写失败测试——键名、默认 `true`、`settings` 为 `null`/数组/非对象/键非布尔时的默认、**`false` 能被读回**、`hasStoredAutoPrecheck` 与读取分开、`withAutoPrecheck` 合并不抹掉其它键。确认红。
- [x] T002 [US2] 同文件补负例：把 `loretide.auto_precheck: false` 写进一个**账号**形状的对象的 `settings`，断言生效值仍取品牌的值（FR-009）。确认红。
- [x] T003 [US1] 新建 `packages/core/workspace/auto-precheck.ts` 让 T001 / T002 转绿。导出见 `contracts/auto-precheck.md`。**只接受工作区**，不提供账号重载。
- [x] T004 [P] [US1] `packages/core/workspace/index.ts` 导出新模块，与 `timezone` 并列。

---

## Phase 2：上游改动（独立 `upstream:` 提交）

- [x] T005 [US1] 先写失败测试 `server/internal/handler/workspace_auto_precheck_test.go`：新建工作区读作 `true`；存 `false` 读回 `false`；非布尔值（字符串 / 数字 / `null`）各一条 `400`；不带该键的更新成功；**读操作不修改存储行**；两个工作区各自独立。确认红。
- [x] T006 [US1] `server/internal/handler/workspace.go`：加 `workspaceAutoPrecheckKey` 常量、`defaultWorkspaceAutoPrecheck`、`validateAutoPrecheckSetting`、`autoPrecheckFilled`；挂到 `CreateWorkspace`、`UpdateWorkspace`、`workspaceToResponse` 三处，与时区那三处对称。**按上游既有写法，不套用 go-modern-guidelines。**
- [x] T007 [US1] 自证「其余行为逐字节不变」：时区相关的既有用例不改自身仍通过；`git diff` 只增不改既有行。
- [x] T008 [US1] 这两步（T006 / T007）单独成一个 `upstream:` 开头的提交。

---

## Phase 3：界面与文案

- [x] T009 [US1] `packages/views/settings/components/workspace-tab.tsx`：在既有 `SettingsCard` 里、时区行之后加一个 `SettingsRow` + `Switch`，保存走 `api.updateWorkspace` + `qc.setQueryData`（与时区行同一写法）。**只用既有组件，不调样式。**
- [x] T010 [US1] 关闭时显示固定说明「关闭不表示免人审」（Q1 = A 已裁决：仅关闭时）。
- [x] T011 [P] [US1] `packages/views/locales/{en,ja,ko,zh-Hans}/settings.json` 四语言文案：标签、描述、关闭说明。
- [x] T012 [US1] `pnpm --filter @multica/views exec vitest run locales/parity.test.ts` 通过（160）。

---

## Phase 4：手动清单与范围自证

- [x] T013 [US1] `specs/019-lt015-brand-precheck-switch/manual-ui-todo.md`，**至少五条**：新品牌默认开、关闭可保存并显示说明、切换品牌各自独立、刷新保持、**账号页无覆盖控件**。
- [x] T014 [P] [US2] 自证账号设置页自动预检相关控件数为 **0**（SC-007）。
- [x] T015 [P] [US1] 自证新增迁移 **0**、新增端点 **0**、新增 UI 单测 **0**（SC-008）。
- [x] T016 [US1] 核对 workflow 第 12 步：本卡**不新增端点**，沿用既有 `PATCH /api/workspaces/{id}`；其「路径参数 ≠ 上下文值」的用例由 #84 覆盖。显式核对并记录，不默认略过。

---

## Phase 5：验证

- [x] T017 `DATABASE_URL` 实跑 `go test ./internal/handler/ -count=1`，报三个计数。
- [x] T018 前端逐文件跑 `auto-precheck.test.ts`、`timezone.test.ts`、`locales/parity.test.ts`。
- [x] T019 `pnpm typecheck --force`，报非缓存任务数。
- [x] T020 三项 check：`pnpm check:content-boundaries`、`check:diagnostics-contract`、`check:diagnostics-no-upload`。
- [x] T021 变异验证：每条新不变量各一处——把默认值改成 `false`、把「已设置」判断改成看值真假（关闭会被翻回开）、去掉非布尔校验、让读时填充回写。
- [x] T022 `tasks.md` 回勾；界面五条记「未执行」（只能由主任务在浏览器给出）。

---

## 依赖

```text
T001,T002 → T003 → T004
                    |
T005 → T006 → T007 → T008
                    |
              T009,T010,T011 → T012
                    |
              T013..T016
                    |
              T017..T022
```

## 不做的事

- 不实现预检的运行、触发或结果展示（EP-06）。
- 不调用模型。
- 不新增迁移、端点、控件、UI 单测。
- 不在新建品牌流程里加控件（FR-007）。
- 不改上游 `workspace.go` 里与本键无关的任何一行。
