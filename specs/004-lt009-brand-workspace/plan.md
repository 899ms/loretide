# Implementation Plan: 品牌空间的时区属性与切换隔离

**Branch**: `004-lt009-brand-workspace` | **Date**: 2026-09-14 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `/specs/004-lt009-brand-workspace/spec.md`

**Blocked by**: LT-008、ARCH-02、DG-01（实施等待；规格与计划已可用）

## Summary

在既有 Workspace 上加一个时区属性：存 `settings["loretide.timezone"]`（零迁移），服务端在创建 / 更新路径校验 IANA 名，未设置时读作 `Asia/Shanghai`；前端加一个纯函数读取器 + 创建表单与设置页的时区选择；切换隔离不新建机制，只把既有 `wsId` 查询键与 `workspace_id` 过滤变成可复核的验收；中文术语沿用「工作区」。不新建实体，不碰 project。

## Technical Context

**Language/Version**: Go 1.26（handler）；TypeScript strict（core / views / web）

**Primary Dependencies**: 既有 chi handler、sqlc `db.Workspace`（`settings` JSONB）；TanStack Query、zod、`parseWithFallback`；`Intl.supportedValuesOf("timeZone")`（浏览器时区列表，无新依赖）；Go 侧 `time.LoadLocation` 校验 IANA 名

**Storage**: `workspace.settings` JSONB 的 `loretide.timezone` 键。无迁移、无新列、无索引

**Testing**: Go：`testutil.Call` 对 Create / Update / Get 的时区校验与默认；TS：`packages/core/workspace/timezone.test.ts`（`// @vitest-environment node`）测读取 / 默认 / 非法值；畸形响应测试。不写 UI 单测

**Target Platform**: Web 与桌面端共享 `packages/views`（创建在 onboarding 步骤，设置在 settings 工作区 tab）

**Project Type**: Existing monorepo (Go backend + Next.js web + Electron desktop + Expo mobile + shared packages). Do not re-derive this.

**Performance Goals**: 无额外查询；时区读取为纯函数

**Constraints**: 不改 `db.Workspace` 结构；其余 `settings` 键原样透传；不引入独立人设 / IP 模型；不碰 project；术语「工作区」

**Scale/Scope**: 后端 1 个文件 + 测试；core 3 个文件（新增 `timezone.ts`、`mutations.ts` 加 `useUpdateWorkspace`、`queries.ts` 不改）；views 2 个文件；locales 4 × 2 个 JSON；测试 2～3 个文件

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| 原则 | 适用 | 判定 |
|---|---|---|
| I. CLAUDE.md 权威 | 是 | 通过 |
| II. 无 UI 单测 / 无自动 UI 验收 | 是 | 通过：时区逻辑抽为纯函数在 node 环境测；表单与设置页进手动 UI Todo |
| III. 模块边界 | 是 | 通过：`views` 用 `@multica/core/workspace` 公开导出；不引入 `next/*`；时区列表用浏览器 `Intl`，不进 core（core 不得依赖 DOM API → 列表获取放 views 层，core 只做校验与默认） |
| IV. 状态分离 | 是 | 通过：时区随 workspace 服务器状态走 Query；表单草稿留在组件本地 state |
| V. 数据库 | 是 | 通过：零迁移，无外键，无索引（clarify 已定 settings JSON） |
| VI. API 解析 | 是 | 通过：`getWorkspaceTimezone(ws)` 防御式读取 `settings`，非法或缺失回退默认；新增畸形响应测试（`settings` 为 null / 字符串 / 非法时区名）；上游无工作区级 zod schema，本功能只在读取器层防御，整表 schema 另立任务 |
| VII. UI 复用 | 是 | 通过：时区选择用 `packages/ui` 既有 Select / Combobox；无则 `pnpm ui:add`；不手写控件 |
| VIII. 范围 | 是 | 通过：不碰 project、不建人设模型、不改术语表 |
| IX. 执行器禁用 | 是 | 通过：不触碰 |
| X. 勾选 ≠ 验收 | 是 | 通过：手动 UI Todo 由用户确认 |

**Stop Conditions**：无触发。→ 进入 Phase 0。

**Post-design re-check**：通过。注意点：`Intl.supportedValuesOf` 在 core 不可用（core 禁 DOM/浏览器 API 的精神），已放到 views。

## Project Structure

### Documentation (this feature)

```text
specs/004-lt009-brand-workspace/
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/workspace-timezone.md
├── checklists/requirements.md
└── tasks.md
```

### Source Code (repository root)

```text
server/internal/handler/workspace.go            # CreateWorkspace / UpdateWorkspace：校验 settings["loretide.timezone"]；workspaceToResponse：缺省填 Asia/Shanghai
server/internal/handler/workspace_test.go       # + 合法 / 非法 / 缺省 / 其余 settings 键透传 用例

packages/core/workspace/timezone.ts             # 新增：TIMEZONE_SETTINGS_KEY、DEFAULT_TIMEZONE、getWorkspaceTimezone(ws)、isValidTimezone(name)（纯函数）
packages/core/workspace/timezone.test.ts        # 新增：// @vitest-environment node
packages/core/workspace/mutations.ts            # + useUpdateWorkspace()（若确认不存在则新增；包装 api.updateWorkspace，settle 后 invalidate workspace 查询）
packages/core/workspace/index.ts                # 导出 timezone.ts
packages/core/api/client.ts                     # createWorkspace 请求体允许 settings（若当前不允许）

packages/views/onboarding/steps/step-workspace.tsx   # + 时区选择（默认预选 Asia/Shanghai，可改）
packages/views/settings/components/workspace-tab.tsx # + 时区显示与修改（标注「默认」）
packages/views/locales/{en,ja,ko,zh-Hans}/onboarding.json   # + step_workspace.timezone_*
packages/views/locales/{en,ja,ko,zh-Hans}/settings.json     # + workspace.timezone_*
```

**Structure Decision**: 时区校验与默认在 core 纯函数 + Go handler 两处各一份（前后端各自防御）；时区候选列表只在 views 层通过 `Intl.supportedValuesOf("timeZone")` 获取。不新建目录。

## Complexity Tracking

无违反项。
