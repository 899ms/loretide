# 合同：账号设置页面

## 1. 端点（服务端已存在，本卡只挂载）

| 方法 | 路径 | handler | 成功 | 失败 |
|---|---|---|---|---|
| GET | `/api/content-accounts` | `ListContentAccounts` | 200 `{accounts:[...]}` | 503 |
| POST | `/api/content-accounts` | `CreateContentAccount` | 201 Account | 400 输入 / 404 授权 |
| GET | `/api/content-accounts/{id}` | `GetContentAccount` | 200 Account | 404 |
| PATCH | `/api/content-accounts/{id}` | `UpdateContentAccount` | 200 Account | 400 / 404 |
| GET | `/api/content-accounts/{id}/persona` | `GetAccountPersonaPrompt` | 200 Revision | 404 |
| POST | `/api/content-accounts/{id}/persona` | `SetAccountPersonaPrompt` | **201** Revision | 400 超长 / **409 冲突可重试** / 404 |
| GET | `/api/content-accounts/{id}/persona/{revisionId}` | `GetAccountPersonaRevision` | 200 Revision | 404 |
| GET | `/api/content-accounts/{id}/persona/revisions` | `ListAccountPersonaRevisions` | 200 `{revisions:[...]}` | 404 |

全部挂在 `RequireWorkspaceMember` 分组下，与 `/api/content-diagnostics` 同级。**本卡页面只用前六个**；后两个一并挂载是因为它们属于同一资源，分两次挂载只会让下一张卡再改一次 `router.go`。

## 2. 三种保存结果

| 服务端 | 前端判定 | 用户读到 |
|---|---|---|
| 201 | `saved` | 「已保存」 |
| **409** | `conflict` | 「有人同时改了这个账号，再保存一次即可」 |
| 400 / 404 / 网络 | `failed` | 「保存失败」+ 错误对象的 `next_action` |

`conflict` 与 `failed` 在**领域层是两个值**（`saveOutcome` 返回的判别联合），只是渲染时都落到 `SettingsSaveState` 的 `error` 外观、传不同的 `errorLabel`。**判定必须在 core 的纯函数里**，否则 409 的语义就只存在于一个 JSX 分支里，测不到。

## 3. 表单分桶

```
FormState = { [accountId]: { platform, displayName, personaPrompt } }
```

- `selectDraft(state, account)`：桶里没有就用**服务端值**初始化，不用上一个账号的值；
- `editDraft(state, accountId, patch)`：只改那个桶；
- `discardDraft(state, accountId)`：保存成功或离开账号时清掉该桶——**清掉而不是写回**，下次进来重新从服务端取。

「切账号不沿用」= `selectDraft(state, B)` 在 `state` 里只有 A 的桶时，返回的是 **B 的服务端值**。这是一条可以用 `expect` 写死的断言，不需要浏览器。

## 4. 不变量

| # | 不变量 |
|---|---|
| A1 | 切到另一个账号，草稿不跨桶 |
| A2 | 保存成功后该账号的草稿被清空，界面转为服务端值 |
| A3 | 409 判定为 `conflict`，文案不含「失败」 |
| A4 | 非 409 的失败判定为 `failed`，并带上 `next_action` |
| A5 | 错误体不合 schema 时降级为通用文案，不抛、不显示 `undefined` |
| A6 | 平台常量与 Go `Platforms` 逐值相等 |
| A7 | 账号列表与人设内容来自服务端查询，非本地持久化 |
| A8 | 页面不读任何插件开关 |
| A9 | `packages/views/content/ip-profile/` 下没有 `.test.tsx` |
| A10 | 四语言键齐全 |

## 5. 变异验证（三处，任务卡指定）

| # | 改动 | 应变红 |
|---|---|---|
| M1 | `selectDraft` 在桶里没有时**回落到上一个账号的草稿** | A1 |
| M2 | 409 判定并入 `failed` | A3 |
| M3 | 错误对象不过 schema，直接读 `body.next_action` | A5 |

## 6. UI 复用清单（PR 的「UI 影响」一节照此填写）

`SettingsContent` / `SettingsTab` / `SettingsSection` / `SettingsCard` / `SettingsRow` / `SettingsSaveState`（`@multica/views/settings/layout`）、`Select` 系列、`Input`、`Textarea`、`Button`（`@multica/ui/components/ui/*`）。**零新增控件、零样式改动。**

**一条观察，不是缺口**：`SettingsSaveStatus` 没有可重试语义（只有 `idle|saving|saved|error`）。本卡用 `errorLabel` 承载，不动上游。若以后多个页面都要区分「可重试」与「失败」的外观，那时才值得动共享组件——记在这里以便那时有据。
