# Contract: `describeRegressionVerdict` / `describeRunLinkage`

**位置**：`packages/core/content/diagnostics/regression.ts`

**消费者**：`packages/views/content/diagnostics/index.tsx` 的运行列表。纯函数，无 DOM 依赖，可在 node 环境测试。

## `describeRegressionVerdict`

```text
describeRegressionVerdict(run: DiagnosticRun) -> RegressionVerdict
```

### 四态真值表（FR-005）

| `regression` | `status` 指示未完成 | `verdict` | 说明 |
|---|---|---|---|
| `""`（空串） | 任意 | `not_run` | 从未评估。**这是当前面板渲染成空单元格的那一格** |
| `"passed"` | 否 | `passed` | 唯一返回 `passed` 的分支 |
| `"passed"` | 是 | `undecidable` | 状态与结论冲突，两个原值都带出，不择一相信 |
| `"failed"` | 任意 | `failed` | — |
| 其他任意值 | 任意 | `undecidable` | 后端将来新增枚举时的安全落点，原值经 `rawRegression` 带出 |

### 不变量

1. **不得伪报通过**：`verdict === "passed"` 当且仅当上表第二行成立。`not_run` 与 `undecidable` 在任何调用下都不等于 `passed`（FR-005、D13-V09）。
2. **原值不丢**：`rawRegression` 始终等于 `run.regression` 的原值，包括空串与未知值，使「无法判定」可就地复核。
3. **纯函数**：同一输入恒返回同一结果，不读时钟、不读全局状态。

### 展示约束（由调用方遵守，写入 manual-ui-todo）

- 显示 `passed` 时必须同时显示 `expectedCode` 与 `actualCode`（FR-006）。
- `passed` 的文案必须表达「模拟结果符合该故障预期」，不得表达「该功能可用」（US2 场景 4）。

## `describeRunLinkage`

```text
describeRunLinkage(run: DiagnosticRun, readableRunIds: ReadonlySet<string>) -> RunLinkage
```

- `readableRunIds`：当前已取回、当前用户可读的运行 id 集合。由调用方提供，**本函数不发起任何请求**——它不知道也不需要知道授权规则（FR-012）。

### `originalRun.state` 判定

| 条件 | state |
|---|---|
| `run.originalRunId === ""` | `self` |
| 非空且 ∈ `readableRunIds` | `linkable` |
| 非空、不 ∈ `readableRunIds`、格式合法 | `unreadable` |
| 非空但格式不合法 | `missing` |

### 不变量

1. **`self` 与 `missing` 不得混淆**：首次运行（`self`）不是断链，界面措辞必须区分（FR-007）。
2. **不推断**：`scenario` / `module` / `build` 为空串一律 `missing`（显示「未记录」），不从其他字段推导替代值。
3. **无副作用**：不发请求、不改 `readableRunIds`。

## 保密

两个函数都只读取 `Run` 的既有字段，不拼接、不展开 `snapshot` 的内容，输出不含凭据或正文。
