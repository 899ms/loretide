# Contract: 下一动作的有效性（G3）

被断言的生产代码：`server/internal/content/diagnostics/log.go:99-100`（`Sanitize` 内）。本特性**不改这段代码**，只补断言。

## 现状映射（已存在于生产代码）

| 错误码 | Next | Retryable |
|---|---|---|
| （默认，含空码与未知码） | `inspect_trace` | `false` |
| `NETWORK_UNAVAILABLE` `SEARCH_FAILED` `TIMEOUT` `DATABASE_UNAVAILABLE` | `retry_simulation` | `true` |
| `AUTHORIZATION_DENIED` | `check_authorization` | `false` |
| `FILE_MISSING` `FILE_CHANGED` | `check_registered_file` | `false` |
| `MODEL_AUTH` `MODEL_QUOTA` | `check_local_client` | `false` |

**已知动作集合（封闭，5 个）**：`inspect_trace` `retry_simulation` `check_authorization` `check_registered_file` `check_local_client`

该集合声明在测试内，不导出到生产代码——见 research.md R5。

## 「有效」的判定（Q3 裁决 A）

一个下一动作**有效**，当且仅当三条同时成立：

1. **非空**
2. **属于上述封闭集合**
3. 对**全部模拟场景声明的预期错误码**穷尽——不存在某个场景码落到集合外

## 断言清单

| # | 断言 | FR |
|---|---|---|
| A1 | 遍历 `Scenarios` 全部 16 项的 `Expected`（含 `normal` / `slow` 的空码），每个经 `Sanitize` 后 `Next` 非空且属于封闭集合 | FR-008、FR-010 |
| A2 | `FILE_MISSING` 与 `FILE_CHANGED` 均得 `check_registered_file`——**当前唯一未被断言的分支** | FR-009 |
| A3 | 一个未知码（如 `NOT_A_REAL_CODE`）得默认 `inspect_trace` 而非空值 | FR-008 |
| A4 | 逐分支断言 `Next` 与 `Retryable` 成对：仅四个可重试码 `Retryable:true`，其余全部 `false` | FR-011 |
| A5 | 空错误码得默认动作，非空值 | FR-008 |

## 覆盖状况对照

| 分支 | 本特性之前 | 之后 |
|---|---|---|
| 默认 `inspect_trace` | 已断言（`log_regression_test.go:61`） | 保持 + 穷尽性 |
| `retry_simulation` | 已断言（:194） | 保持 + 成对断言 |
| `check_authorization` | 已断言（:205） | 保持 + 成对断言 |
| `check_local_client` | 已断言（:296） | 保持 + 成对断言 |
| **`check_registered_file`** | **未断言** | **新增（A2）** |
| **16 场景码穷尽性** | **未断言** | **新增（A1）** |
| **Next/Retryable 一致性** | **未成对断言** | **新增（A4）** |

## 变异验证（SC-004）

改完即还原，每处须至少让一条用例变红：

1. 把 `check_registered_file` 分支删掉 → A2 变红
2. 给 `Scenarios` 加一个映射不到的新错误码 → A1 变红
3. 把某个可重试码的 `Retryable` 改成 `false` → A4 变红
4. 把默认分支的 `Next` 改成空串 → A3、A5 变红

「写完看绿」不算验证。
