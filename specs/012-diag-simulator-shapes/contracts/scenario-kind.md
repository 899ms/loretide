# Contract: `Scenario.kind` 的对外形状

`Scenarios` 经 `Service.Overview` 下发给面板，是**对外形状**。本文件定的是它变更之后服务端与客户端各自必须守住的东西。依据：`CLAUDE.md` → API Compatibility，constitution 原则 VI。

## 服务端

```json
{
  "scenarios": [
    {"id": "normal",          "expected_code": "",        "kind": "fault"},
    {"id": "timeout",         "expected_code": "TIMEOUT", "kind": "fault"},
    {"id": "shape_concurrent","expected_code": "",        "kind": "shape"}
  ]
}
```

| 规则 | 内容 |
|---|---|
| S-1 | 每一项的 `kind` **必须**是 `"fault"` 或 `"shape"`，不得为空、不得省略 |
| S-2 | `kind == "fault"` 的 id 集合**恰好**是本特性之前的 16 项（data-model INV-1） |
| S-3 | `kind == "shape"` 与 `docs/13` §7 的 15 项交集为空（INV-2） |
| S-4 | 判断分类 **MUST** 读 `Kind` 字段，**MUST NOT** 解析 id 前缀（INV-4） |

## 客户端（`packages/core/content/diagnostics/contract.ts`）

| 规则 | 内容 | 为什么 |
|---|---|---|
| C-1 | `kind` 在 zod 里**可缺省**，缺省时落默认值 `"fault"` | 装好的桌面端会连上更旧的后端。那时响应里没有 `kind`；若必填，`parseWithFallback` 整体失败，概览会**连组件状态和指标一起丢**。默认 `"fault"` 语义也对——旧后端只有业务故障场景 |
| C-2 | `kind` 为非字符串时**不得**让 `overviewSchema` 整体失败 | 同上。单个字段的畸形不该带走整张概览 |
| C-3 | 不得把网络 JSON 强转为 `T`，解析仍走 `parseWithFallback` + schema | `CLAUDE.md` → API Compatibility |
| C-4 | 若前端出现按 `kind` 分支的逻辑，**必须**有 `default` 分支 | 原则 VI：服务端驱动的枚举 switch 需要 default。今天没有这样的分支；这条是给下一个人的 |

## 必须附的测试（FR-013 / SC-010）

落在 `packages/core/content/diagnostics/contract.test.ts`：

1. **缺 `kind`**：`scenarios` 每项只有 `id` 与 `expected_code` → 解析成功，`kind` 为 `"fault"`，概览其余字段完整。
2. **`kind` 非字符串**：某项 `kind: 42` → 解析不整体失败，概览其余字段完整。
3. **正常响应**：`kind` 为 `"fault"` / `"shape"` → 原样透出。

这三条是 constitution 原则 VI 要求的「新增或变更字段随附畸形响应测试」，不是可选项。

## 不做的事

- **不改 `transform` 已有的输出键名。** `expectedCode` 等现有键名任何改动都会波及面板，而面板不在本特性范围。
- **不在面板里按 `kind` 分组下拉。** shape 场景进入既有下拉是列表驱动的，零 UI 改动即可选中；分组是呈现优化，属原则 VIII 的「顺带」。
