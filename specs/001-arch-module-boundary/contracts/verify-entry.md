# Contract: `pnpm check:content-boundaries`

命令行契约。消费者：开发者、执行 agent、`scripts/check.sh`、CI。

## Invocation

```bash
pnpm check:content-boundaries
```

无参数。工作目录须为仓库根。

## Behavior

1. 运行 `node --test scripts/check-content-boundaries.test.mjs`。任一用例失败 → 立即退出，退出码非 0。
2. 运行 `node scripts/check-content-boundaries.mjs`。发现任一违规 → 退出码非 0，每条违规一行：`<file>: <category>`。
3. 两步均通过 → 退出码 0。

## Exit codes

| 码 | 含义 |
|---|---|
| 0 | 自测与扫描均通过 |
| 非 0 | 自测失败，或扫描发现违规；输出指出是哪一步 |

## Output

- 自测：`node --test` 的 TAP 输出（用例名与 pass/fail）。
- 扫描：违规列表；无违规时输出扫描摘要（文件数 / 模块数）。
- 不输出任何密钥或环境变量。

## Stability

与 CI 工作流 `loretide-content.yml` 的两步命令逐字相同；任何一方改动须同步另一方。
