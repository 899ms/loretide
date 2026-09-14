# Research: 004 品牌空间的时区属性与切换隔离

Phase 0。Technical Context 无 NEEDS CLARIFICATION；以下为技术决策。

## D1. 存储与键名

- **Decision**: `workspace.settings["loretide.timezone"]`，字符串，IANA 名。
- **Rationale**: clarify 2026-09-14 定为 settings JSON；前缀避免与上游未来键冲突；`client.updateWorkspace` 已接受 `settings`，`UpdateWorkspace` handler 已透传。
- **Alternatives considered**: 新增列——已否决。

## D2. 服务端校验与默认

- **Decision**: `CreateWorkspace` / `UpdateWorkspace` 若请求含 `settings` 且其中有 `loretide.timezone`，用 `time.LoadLocation(name)` 校验，失败 → 400 `invalid timezone`，不写入；其余键不动。`workspaceToResponse` 在 `settings` 缺该键或值非字符串时填入 `Asia/Shanghai`（只在响应中补，不回写数据库）。
- **Rationale**: FR-001、FR-002、FR-007。`time.LoadLocation` 依赖系统 tzdata；Windows 上 Go 使用内置 `time/tzdata`——需在 server 入口 `import _ "time/tzdata"`（若尚未导入），否则 Windows 开发机校验会失败。这是实施时必须核对的一点。
- **Alternatives considered**: 只在前端校验——不满足「服务端 MUST 校验」；回写默认值到数据库——改变历史数据，否决。

## D3. 前端读取器

- **Decision**: `packages/core/workspace/timezone.ts` 导出 `TIMEZONE_SETTINGS_KEY = "loretide.timezone"`、`DEFAULT_TIMEZONE = "Asia/Shanghai"`、`isValidTimezone(name)`（用 `Intl.DateTimeFormat(undefined, {timeZone: name})` try/catch，Node 与浏览器均可用）、`getWorkspaceTimezone(ws: {settings?: unknown}): string`（缺失 / 非字符串 / 非法 → 默认）。
- **Rationale**: FR-004 防御式解析；纯函数可在 node 环境测；core 不依赖 DOM。
- **Alternatives considered**: 为整个 Workspace 建 zod schema——上游目前无此 schema，范围过大；留作后续任务。

## D4. 时区候选列表

- **Decision**: 在 views 组件内用 `Intl.supportedValuesOf("timeZone")` 生成；不可用时回退到少量常用值 + 手输。
- **Rationale**: 无新依赖；core 不得触碰浏览器 API（constitution III 精神）。

## D5. `useUpdateWorkspace`

- **Decision**: 实施第一步确认 `packages/core/workspace/mutations.ts` 无同名 hook（Phase 0 grep 未见）；新增 `useUpdateWorkspace()`：`mutationFn: (vars: {id, data}) => api.updateWorkspace(id, data)`，`onSettled` invalidate `["workspaces"]` 与 `["workspace", id]`（键名以现有 `queries.ts` 为准）。不做乐观更新——设置页保存后停留原页，但字段来自 settings JSON，回滚需重取，不满足 `CLAUDE.md` 乐观更新四条件中的「回滚 trivial」。
- **已核实**：`UpdateWorkspace` 把请求的 `settings` 整体 marshal 写入（`workspace.go:403-405`），是替换不是合并；`timezone.ts` 增加纯函数 `withWorkspaceTimezone(settings: unknown, tz: string): Record<string, unknown>`（非对象视为 `{}`，保留其他键），设置页保存时用它合并后整体发送。
- **Rationale**: FR-003；`CLAUDE.md` State Rules。

## D6. 创建表单

- **Decision**: `step-workspace.tsx` 提交体加 `settings: {[TIMEZONE_SETTINGS_KEY]: tz}`；默认预选 `Asia/Shanghai`（clarify：可选，缺省 Asia/Shanghai）；若 `client.createWorkspace` 请求体类型不含 `settings`，扩展类型并核对 Go `CreateWorkspaceRequest` 是否接收 `settings`（Phase 0 见其字段为 name/slug/description/context/issue_prefix——**需要加 `Settings any`** 并按 D2 校验）。
- **Rationale**: US1；这是后端唯一需要新增字段的地方。

## D7. 切换隔离验收

- **Decision**: 不新建机制；验收用 quickstart 第 3 节的抽样：切换后网络面板核对 5 个空间范围接口响应的 `workspace_id`，诊断流事件的 `workspace_id`。
- **Rationale**: US2 / FR-005；机制已是硬规则。

## D8. 非成员拒绝

- **Decision**: 复用既有 `getWorkspaceMember` 校验；新增 Go 用例：非成员 GET / PATCH 时区 → 403/404 且 body 无 `name` / `settings`。
- **Rationale**: US3 / FR-006。

## D9. 术语与 locale

- **Decision**: 文案键：`onboarding.json → step_workspace.timezone_label / timezone_hint`，`settings.json → workspace.timezone_label / timezone_default_badge / timezone_invalid`；四个语言目录同时加（`parity.test.ts` 会检查键一致）；zh-Hans 用「工作区」「时区」。
- **Rationale**: FR-008；clarify 定「工作区」。
