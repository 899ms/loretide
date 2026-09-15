# Contract: 品牌级自动预检开关

## 键

```text
workspace.settings["loretide.auto_precheck"] : boolean
```

`loretide.` 前缀避开上游同名键，与 `loretide.timezone` 同一口径。**无迁移、无列、无端点。**

## 服务端（`server/internal/handler/workspace.go`，上游文件）

| 规则 | 内容 |
|---|---|
| S-1 | 键缺席 → 放行。不携带该键的更新（改名、改 Logo）不得被强制带上它 |
| S-2 | 键存在且非布尔（字符串、数字、`null`、对象、数组）→ **`400`** |
| S-3 | 读时若键缺席或非布尔 → 响应里填 **`true`** |
| S-4 | 填默认**只作用于响应**。读一次工作区 MUST NOT 产生一次写 |
| S-5 | 存下的 `false` 读回来仍是 `false`。**MUST NOT** 用「值为真」或「有值」判断是否已设置 |
| S-6 | 校验挂在 `CreateWorkspace` 与 `UpdateWorkspace` 两处，与 `validateTimezoneSetting` 对称 |

## 客户端（`packages/core/workspace/auto-precheck.ts`）

| 导出 | 契约 |
|---|---|
| `AUTO_PRECHECK_SETTINGS_KEY` | 与服务端常量逐字相同 |
| `DEFAULT_AUTO_PRECHECK` | `true` |
| `isAutoPrecheckEnabled(workspace)` | **只接受工作区**。`settings` 为 `null` / 数组 / 非对象 / 键非布尔 → 返回默认值 |
| `hasStoredAutoPrecheck(workspace)` | 与上一条**分开**：`false` 是已设置，不是未设置 |
| `withAutoPrecheck(settings, enabled)` | 合并进既有 settings 后返回新对象——端点整体替换 settings，只发一个键会把其余键抹掉 |

**MUST NOT** 提供任何接受账号的重载或变体。

## 账号级覆盖：不生效（FR-009）

账号的 `settings` 是 `z.record(z.string(), z.unknown())`，**能放任意键**。所以「不提供覆盖」必须被断言守住，而不是靠字段不存在：

- 纯函数只接受工作区，账号对象**在类型上就传不进去**；
- 一条断言：把 `loretide.auto_precheck: false` 写进账号的 `settings`，生效值仍等于品牌的值。

**只断言「schema 里没有该字段」不够**——那只证明今天没人加过。

## 最容易悄悄失败的地方

- **把 `false` 当成未设置。** 时区那套用「有没有字符串值」判断，照搬到布尔上，关闭会被默认值翻回开。读取与「是否已显式设置」必须是两个判断。
- **读时填充回写存储行。** 那会让一次读变成一次写，也会让「从未设置」这个状态在第一次被读之后永远消失。
- **只发一个键。** 端点整体替换 settings，`{auto_precheck: x}` 会抹掉时区。
