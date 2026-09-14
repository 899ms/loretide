# Quickstart: 001 验证指引

前置：仓库根目录；`pnpm install` 已完成；Node ≥ 22。

## 1. 入口命令通过（SC-001、SC-002）

```bash
node --version
time pnpm check:content-boundaries ; echo "exit=$?"
```

预期：13 个用例 pass；扫描无违规；`exit=0`；耗时 < 60s。记录 Node 版本与耗时。

## 2. 负例使入口失败（SC-002）

```bash
echo 'import "electron";' >> packages/core/content/diagnostics/index.ts
pnpm check:content-boundaries ; echo "exit=$?"
git checkout -- packages/core/content/diagnostics/index.ts
pnpm check:content-boundaries ; echo "exit=$?"
```

预期：第一次 `exit≠0` 且输出含 `packages/core/content/diagnostics/index.ts`；恢复后 `exit=0`。

## 3. `make check` 覆盖（Linux / CI 侧）

```bash
grep -n "check:content-boundaries" scripts/check.sh
```

预期：命中一行。（Windows 本机无 make，此项在 CI 日志中核对。）

## 4. PR 模板（SC-003）

```bash
grep -n "owned by the module" .github/PULL_REQUEST_TEMPLATE.md
```

预期：命中一行。

## 5. 验收对照节（SC-004、SC-005）

```bash
grep -n "Acceptance mapping" docs/development/content-boundary-checks.md
python -c "import json;m=json.load(open('scripts/content-boundaries.json'))['modules'];print(len(m),sorted(m))"
```

预期：对照节存在；模块数 12，与 docs/12 §2 的 11 个 + `diagnostics` 一致。

## UI

无 UI 影响；手动 UI Todo：无。
