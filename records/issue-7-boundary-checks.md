# Issue #7 — ARCH-TEST-01：模块边界检查器回归加固

- Issue: https://github.com/899ms/loretide/issues/7
- 执行者：Claude (Opus 4.8) — Claude Code 会话 `26a4a9e9-f381-4563-8524-33475f9214dd`
- 分支：`task/issue-7-boundary-checks`（基线 app-main `49ef4eb231378b15954f10b1def4778c5be78392`）
- 工作目录：独立 git worktree `loretide-development-04017b`
- PR 目标：`app-main`（Draft）

## 修改的文件（严格限定在 Issue 文件边界内）

1. `scripts/check-content-boundaries.mjs` — 加固检查器准确性：
   - 新增 `extractGoImports()`：小型词法分析器，跳过注释与字符串/原始字符串/rune 字面量，
     仅在代码上下文识别 `import` 关键字，替代原来的正则抽取。
   - 新增 `hasComputedModuleLoad()`：基于 TS AST 判定动态 `import()`/`require()`
     的实参是否为纯字符串字面量；非字面量（标识符、字符串拼接、模板串、缺参）即拒绝。
   - `check()` 入口对输入文件路径做反斜杠→`/` 标准化（Windows 路径）。
2. `scripts/check-content-boundaries.test.mjs` — 新增 9 个可复现回归用例（保留原 2 个）。
3. `docs/development/content-boundary-checks.md` — 新增：检查器职责、加固点、静态分析局限。
4. `records/issue-7-boundary-checks.md` — 本交付记录。

未修改：`scripts/content-boundaries.json`（未放宽白名单）、生产业务代码、CI workflow、
环境脚本、其它测试。未与 #1/#2 重叠。

说明：Issue 提到的“docs/12”在仓库中未找到对应文件；以 `content-boundaries.json`
与检查器实现为权威规格进行加固。

## 修复的真实缺陷（老检查器行为，本地实测 old `check()`）

| 场景 | 老检查器 | 期望/新检查器 |
| --- | --- | --- |
| Go `import(...)` 组内注释含 `)`，其后有越界 import | 非贪婪正则在 `)` 处截断，**漏报** forbidden dep（`errors=[]`）| 正确报 forbidden dependency |
| Go 注释/原始字符串中的 `import "..."` | **误报** forbidden dependency | 忽略，`errors=[]` |
| `import()`/`require()` 文本出现在注释/字符串中 | **误报** computed import forbidden | AST 判定，忽略 |
| `import("a"+"b")`（拼接） | 仅因 preProcessFile 抽出字面量 `a` 而报“unapproved upstream import a”（理由错误）| 明确报 computed import forbidden |
| Windows 反斜杠路径的越界 import | 分类失败→误判为 rogue adapter，文件名还被显示成 `packagescontent...` | 标准化后正确报 forbidden dependency |
| Windows 反斜杠路径的合法 import | **误报**（当作外部 adapter）| `errors=[]` |

另新增回归锁定（老实现本已正确，防止回退）：TS re-export / type-only 仍参与分层与依赖校验、
相对路径私有导入、三模块循环依赖。

## 本地验证证据

TypeScript 5.9.3（与仓库 catalog 一致），独立 scratchpad 环境：

- 修复后单元测试：`node --test scripts/check-content-boundaries.test.mjs`
  → `tests 11 / pass 11 / fail 0`（原 2 + 新 9）。
- 修复前：上述 6 个场景以 old `check()` 直测，4 类明确为 BUG（漏报/误报），
  1 类理由错误（见上表）。
- 全仓真实扫描：`node scripts/check-content-boundaries.mjs`
  → `Content boundaries passed (3519 files; 12 registered modules)`，退出码 0，
  证明新逻辑对真实代码无新增误报。

（本地环境仅用于加速；验收以 PR head SHA 对应的真实 GitHub Actions 为准。）

## 真实 CI 运行证据

- PR：https://github.com/899ms/loretide/pull/8 （Draft，base `app-main`）
- head SHA：`4f9d5fe91b96e673157c943c0f5a7cb5508d272d`
- Actions run（成功）：https://github.com/899ms/loretide/actions/runs/34759796386
  workflow `Loretide content contracts`，conclusion `success`。
- 关键步骤（真实日志）：
  - `node --test scripts/check-content-boundaries.test.mjs` → `# tests 11 / # pass 11 / # fail 0`。
  - `node scripts/check-content-boundaries.mjs` → `Content boundaries passed (3520 files; 12 registered modules)`。

（后续仅追加本记录的提交会再触发一次同 workflow 的运行，内容等价、同样通过。）

## 覆盖 / 仍不支持

- 已覆盖：Go 分组/别名/blank/dot 导入、注释与字符串中的 import、
  TS re-export/type-only/dynamic import/require、相对路径私有导入、
  Windows 路径标准化、（含三模块）循环依赖、无法静态计算的模块加载被明确拒绝。
- 仍不支持（属静态分析固有局限，未宣称保证运行时行为）：不求解传入
  `import()`/`require()` 的运行时值（直接拒绝）、不跟踪构建期 codegen 或
  超出 `check()` 前缀改写的路径别名、不建模条件/平台相关打包。

## 回滚方法

- 仅 revert 本任务对检查器/测试/说明的改动：`git revert <本任务提交>`，
  或关闭 PR 后丢弃分支 `task/issue-7-boundary-checks`。
- 无数据/生产/CI-workflow 变更需回滚；`content-boundaries.json` 未改动。
