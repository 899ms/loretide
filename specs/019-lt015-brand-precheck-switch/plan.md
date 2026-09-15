# Implementation Plan: 品牌级自动预检开关（LT-015）

**Branch**: `claude/spec-019-lt015-brand-precheck-switch` | **Date**: 2026-09-15 | **Spec**: [spec.md](./spec.md)

## Summary

照 LT-009（#63）的手法，把自动预检开关做成工作区 settings JSON 里的 `loretide.auto_precheck` 键（布尔，缺省 `true`）：服务端加校验与读时填默认，core 加一个纯函数模块，设置页加一个 `Switch` 行。**零迁移、零新端点、零新控件。**

一处与时区不同、必须当心：**`false` 是布尔的零值**。时区那套用「有没有字符串值」判断是否已设置，照搬到布尔上会把「关闭」当成「未设置」，再被默认值翻回开。读取与「是否已显式设置」必须是两个独立判断。

另一处：账号的 `settings` 是一个可放任意键的容器（`z.record(z.string(), z.unknown())`），所以「不提供账号级覆盖」不能靠「字段不存在」成立，必须有断言守住。

## Technical Context

**Language/Version**: Go 1.26（`server/`）、TypeScript 5 strict（`packages/core`、`packages/views`）

**Storage**: 既有 `workspace.settings` JSONB。**不新增迁移、不新增列。**

**Testing**: `go test ./internal/handler/`（`DATABASE_URL` 实跑）、`packages/core/*.test.ts`（node 环境）、`locales/parity.test.ts`。**无 UI 单测。**

**Constraints**: 不调用模型；不实现预检运行（EP-06）；不新增控件不调样式；越权沿用既有中间件。

## Constitution Check

| 原则 | 判定 | 依据 |
|---|---|---|
| I. `CLAUDE.md` 权威 | 通过 | 无新规则 |
| II. 不写 UI 单测 | 通过 | 逻辑进 core node 测试；界面进手动清单 |
| III. 模块边界 | 通过 | `packages/core/workspace/` 新模块；`views` 只消费它，不放逻辑 |
| IV. 服务端/客户端状态分离 | 通过 | 沿用 `api.updateWorkspace` + `qc.setQueryData`，与同页其它字段一致；无新 store |
| V. 无外键、无级联 | 不适用 | **不新增迁移** |
| VI. 响应解析不强转 | **需注意** | 工作区响应经既有路径；core 纯函数对 `settings` 的每种畸形形态（`null`、数组、非对象、非布尔）都要有默认，不得让调用方逐个防 |
| VII. UI 复用 Multica | **需注意** | 只用既有 `Switch` / `SettingsRow` / `SettingsCard`；见 `issue-tab.tsx:50` 的现成写法 |
| VIII. 范围是所领的任务 | **需注意** | 见下 |
| IX. 真实执行器保持禁用 | 通过 | 不调用模型，不触发任何执行 |
| X. 打勾不等于验收 | 通过 | 界面五条由主任务在浏览器确认 |

### 关于原则 VIII 与上游改动

校验与读时填充的三个挂点都在 `server/internal/handler/workspace.go`——**上游 Multica 文件**。不改它这张卡不成立：没有校验，非布尔值会被存进去；没有读时填充，「默认开启」无处实现。

按 workflow 第 13 步：**独立 `upstream:` 提交**，PR 正文单列「上游改动」，说明改了什么、为何 SOP 没它不成立、不碰上游的替代及为何不选、回滚方式，并附「其余行为逐字节不变」的自证。

`go-modern-guidelines` 在这个上游文件里**不适用**——那里按上游既有写法走（与 `validateTimezoneSetting` / `timezoneFilled` 对称）。

## Project Structure

```text
server/internal/handler/
├── workspace.go                       # upstream: 加键名常量、校验、读时填充（三个挂点各一处）
└── workspace_auto_precheck_test.go    # 新增：默认/存取/非法值/不回写/跨工作区隔离

packages/core/workspace/
├── auto-precheck.ts                   # 新增：键名、默认值、读取、是否已显式设置、合并写回
└── auto-precheck.test.ts              # 新增：node 环境；含账号级覆盖不生效的负例

packages/views/
├── settings/components/workspace-tab.tsx   # 既有 SettingsCard 内加一个 Switch 行
└── locales/{en,ja,ko,zh-Hans}/settings.json # 四语言文案

specs/019-lt015-brand-precheck-switch/
├── contracts/auto-precheck.md
├── manual-ui-todo.md
└── checklists/requirements.md
```

**Structure Decision**: core 模块与 `timezone.ts` 并列，**不合并进同一文件**——两者的默认值语义不同（字符串 vs 布尔零值），放一起会让「怎么判断已设置」这条差异被读者忽略。

### 关于「带路径参数的端点」（workflow 第 12 步）

本卡**不新增端点**，沿用既有的 `PATCH /api/workspaces/{id}`。该端点的路径参数 ≠ 上下文值的用例属于 #84 已覆盖的范围，本卡不重复；这一点在 tasks 里作为一条显式核对项，而不是默认略过。

## Complexity Tracking

| Violation | Why Needed | Simpler Alternative Rejected Because |
|---|---|---|
| 改上游 `workspace.go` | 校验与读时填充没有别的挂点；没有它们「默认开启」与「非布尔 400」都不成立 | ① 在 core 侧填默认、服务端不管：那样服务端会存下任何垃圾值，而两端对「什么是合法值」的判断会各说各话——正是 #63 的注释里专门避免的那种分裂；② 新开一个 Loretide 自己的端点：多一条读写路径，且与同一 blob 的其它键各走各的 |
| 多一个 core 模块而不是并入 `timezone.ts` | 布尔零值使「是否已设置」的判断与字符串不同 | 并入会让两套语义共处一文件，下一个人照着上面那半写下面那半，就会把关闭当成未设置 |
