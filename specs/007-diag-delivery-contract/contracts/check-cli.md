# Contract: `check-diagnostics-contract` 检查脚本

## 形态

与 `scripts/check-content-boundaries.mjs` 同构：

```js
export function check(files, config) -> string[]   // 纯函数，返回错误行；空数组表示通过
```

`files` 为 `路径 -> 源码` 映射，`config` 为解析后的模块图与豁免配置。CLI 入口走目录、读文件、打印、设退出码。

**纯函数是可测性的前提**：测试用合成的文件映射当夹具，负例不需要在仓库里造假模块目录——造了反而会被既有边界检查器扫到。

## 输入

| 来源 | 用途 |
|---|---|
| `scripts/content-boundaries.json` | **只读**模块名列表。本功能不写它、不改它的规则 |
| `scripts/diagnostics-contract.json` | 豁免登记 |
| `server/internal/content/**/*.go` | 判定 E1 / E2 / E3 |

## 判定

对每个模块：未落地 → 跳过；已豁免 → 通过；否则要求 E1 && E2 && E3（定义见 data-model.md）。

## 输出与退出码

| 情形 | 退出码 | 输出 |
|---|---|---|
| 通过 | 0 | 摘要：检查 N 个已落地模块，跳过 M 个未落地 |
| 缺项 | 非 0 | 每缺项一行：`<module>: missing <E1\|E2\|E3> — <该条的具体说明>` |
| 配置无效 | 非 0 | 指出无效之处 |

**缺项必须点名到条**（FR-011a）。只说「该模块无证据」不满足合同——作者需要知道补哪一条。

## 不做的事

- **不判定「是否真的跑了真实执行器」**。没有任何静态手段能判断这件事；该子句靠合同文本与 PR 检查表，不靠脚本（research D6）。
- **不判定语义正确性**。静态检查只证明痕迹存在。合同正文必须写明这一点，避免「检查通过」被读成「接入正确」。
- **不约束 `packages/core/content/` 与 `packages/views/content/`**（clarify FR-016）。
- **不要求错误码枚举存在**（clarify FR-003）——它今天未导出。

## CI

在 `loretide-content.yml` 既有两条边界检查步骤之后新增一条 run 步骤。**`on:` 段一字不动**，由 SC-005 的逐字节比对钉住。
