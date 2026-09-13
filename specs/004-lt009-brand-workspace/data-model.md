# Data Model: 004 品牌空间的时区属性与切换隔离

无 schema 变更。以下为 `settings` JSON 的键约定与响应形状。

## workspace（既有表，不改）

| 列 | 类型 | 说明 |
|---|---|---|
| id | UUID | |
| name | TEXT | |
| slug | TEXT UNIQUE | |
| description | TEXT | |
| settings | JSONB NOT NULL DEFAULT '{}' | **本功能使用的键见下** |
| context / repos / issue_prefix / avatar_url | … | 既有，不改 |

## settings 键约定

| 键 | 类型 | 必填 | 默认 | 校验 |
|---|---|---|---|---|
| `loretide.timezone` | string（IANA 时区名） | 否 | 缺失时读作 `Asia/Shanghai`（响应侧补，不回写） | 服务端 `time.LoadLocation`；前端 `Intl.DateTimeFormat` try/catch |

其余键：原样透传，本功能不读不写。

## WorkspaceResponse（线上，不改字段集合）

```json
{ "id": "...", "name": "...", "slug": "...", "settings": { "loretide.timezone": "Asia/Shanghai", "...": "..." }, "...": "..." }
```

- `settings.loretide.timezone` 在响应中**始终存在**（缺省补 `Asia/Shanghai`）。
- 旧客户端忽略该键；新客户端用 `getWorkspaceTimezone()` 读取并兜底。

## CreateWorkspaceRequest（线上，新增可选字段）

```json
{ "name": "...", "slug": "...", "description": "...", "context": "...", "issue_prefix": "...", "settings": { "loretide.timezone": "Asia/Shanghai" } }
```

`settings` 可选；含 `loretide.timezone` 时校验。

## UpdateWorkspaceRequest（线上，字段集合不变）

`settings` 已存在；含 `loretide.timezone` 时校验，非法 → 400 不写入；不含时不动该键。

## 客户端读取器（core 纯函数）

```text
getWorkspaceTimezone(ws: { settings?: unknown }): string
  settings 非对象 → DEFAULT
  key 缺失 / 非字符串 / isValidTimezone=false → DEFAULT
  否则 → 该值
```

## 隔离（既有，不改）

- 所有空间范围查询键含 `wsId`；所有查询按 `workspace_id` 过滤；`X-Workspace-ID` 选择空间。
- 本功能只验收，不新增实体或关系。
