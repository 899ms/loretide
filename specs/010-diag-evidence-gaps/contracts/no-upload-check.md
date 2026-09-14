# Contract: 导出路径无外发请求的静态检查

## 形态

`scripts/check-diagnostics-no-upload.mjs`，与 `scripts/check-diagnostics-contract.mjs` 同形：导出纯函数 `check(files)` → `string[]`，CLI 入口负责读盘、打印、设退出码。测试用合成的「路径 → 源码」映射当夹具。

**为什么是静态检查而不是运行时测试**：这条命题是「代码里没有某样东西」。静态检查能直接表达它；运行时测试只能靠「这一次没观察到请求」来证明，而没观察到不等于不存在。

## 扫描清单（显式，不通配）

| 文件 | 在导出/保存链路里的角色 |
|---|---|
| `apps/web/platform/content-diagnostics.ts` | 保存到本机：`createObjectURL` + `<a>` + `click` + `revokeObjectURL`，全文 7 行 |
| `packages/views/content/diagnostics/index.tsx` | 面板；`download` 由 props 注入，面板自己不发请求 |
| `packages/core/content/diagnostics/queries.ts` | `download` mutation，调用 `api.contentDiagnosticDownload` |
| `apps/web/app/[workspaceSlug]/(dashboard)/diagnostics/page.tsx` | 把面板与平台层接起来 |

### 显式排除

**`packages/core/api/client.ts` 不在清单内。** 下载本身经 `api.contentDiagnosticDownload` → `fetchRaw` 走共享 API 客户端，那是**合法的同源调用**——把它判红等于把正常下载判成违规。

### 为什么不是目录通配

将来该目录加入一个正当需要 `fetch` 的文件时，通配会逼作者去改检查而不是去想清楚；显式清单让「范围变了」这件事在 diff 里看得见。代价是链路拆文件时要手动同步，由 P3 兜住。

## 规则

| ID | 规则 |
|---|---|
| **P1** | 清单内文件不得出现 `fetch` / `XMLHttpRequest` / `sendBeacon` / `new WebSocket` |
| **P2** | 匹配 MUST 按**词边界** |
| **P3** | 清单内每个文件 MUST 存在；任一缺失即**失败** |

### P2 不是理论风险

已实测：`packages/views/content/diagnostics/index.tsx` 有 **4 处 `refetch(`**（React Query 重新取数）。

| 匹配方式 | 命中数 |
|---|---:|
| 朴素子串 `fetch(` | **4（全是误报）** |
| 词边界 `(?<![\w.])fetch\(` | **0** |

`.` 也要排除在前缀里：`api.fetchRaw(` 之类的成员调用不是原生 `fetch`。

### P3 为什么必须失败

一个扫不到文件的检查会**永远绿**。那比没有检查更糟——它看起来在工作。

## 输出与退出码

| 情形 | 退出码 | 输出 |
|---|---:|---|
| 通过 | 0 | 一行摘要：检查了 N 个文件 |
| 命中原语 | 非 0 | 每条一行：`<file>: <primitive> is not allowed on the export path` |
| 清单文件缺失 | 非 0 | 点名缺失的文件 |

## 可测性

| 要验的 | 怎么验 |
|---|---|
| **正例** | 对当前仓库退出 0 |
| **负例（fetch）** | 夹具里塞 `fetch("https://example.test/upload")` → 变红并点名文件与 `fetch` |
| **负例（sendBeacon）** | 夹具里塞 `navigator.sendBeacon(...)` → 变红并点名 `sendBeacon` |
| **不误报** | 夹具里放 4 处 `refetch()` 与一处 `api.fetchRaw()` → **不**变红 |
| **清单过时** | 夹具里去掉一个清单文件 → 变红并点名缺失 |

## 接进 CI

`package.json` 新增 `check:diagnostics-no-upload`，与 `check:diagnostics-contract` 同形（`node --test` 自测 && 仓库扫描）。是否加进 `loretide-content.yml` 由主任务决定；**加入时只新增 run 步骤，`on:` 段一字不动**。
