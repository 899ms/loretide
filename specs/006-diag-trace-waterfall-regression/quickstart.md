# Quickstart: 006 验证指引（自动部分）

前置：仓库根目录；`pnpm install` 已完成；Node ≥ 22。本页只覆盖**自动可验证**的部分；页面行为见 [manual-ui-todo.md](manual-ui-todo.md)。

## 1. 纯函数测试通过（SC-002 / SC-003 / SC-004 / SC-006）

```bash
pnpm --filter @multica/core exec vitest run content/diagnostics/trace-waterfall.test.ts content/diagnostics/regression.test.ts
```

预期：全部通过，退出 0。两个文件首行均为 `// @vitest-environment node`。

## 2. 确认没有引入 UI 单测（SC-005）

```bash
grep -L "@vitest-environment node" packages/core/content/diagnostics/trace-waterfall.test.ts packages/core/content/diagnostics/regression.test.ts
git diff --stat origin/app-main..HEAD -- '*.test.tsx'
```

预期：第一条无输出（两个文件都声明了 node 环境）；第二条无输出（未新增或修改任何 `.test.tsx`）。

## 3. 模块边界（SC-006）

```bash
pnpm check:content-boundaries
```

预期：退出 0。新增文件位于 `packages/core/content/diagnostics/`，依赖方向为 `views -> core`。

## 4. 类型检查

```bash
pnpm typecheck
```

预期：退出 0。

## 5. i18n 齐全（research D7）

```bash
pnpm --filter @multica/views exec vitest run locales/parity.test.ts
```

预期：退出 0。四语言文案键齐全，无漏译。

## 6. 后端确实未被触碰（FR-010）

```bash
git diff --stat origin/app-main..HEAD -- server/
```

预期：**无输出**。本功能不改任何 Go 代码，现有 Go 用例一条都不需要重跑即可保持有效。

## UI

有页面改动，但按 constitution 原则 II **不做自动 UI 验收**。页面行为逐条见 `manual-ui-todo.md`，由用户在浏览器确认。
