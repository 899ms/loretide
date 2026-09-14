# Data Model: 007 诊断接入合同与交付检查

**无数据库变更、无迁移、无 API 形状变更。** 本功能的「数据」是两份配置与一份判定结果。

## 豁免配置（`scripts/diagnostics-contract.json`，新增）

```text
{
  "version": 1,
  "exemptions": [
    { "module": "<模块名>", "reason": "<为什么这个模块的接入不在模块目录内>", "where": "<实际接入位置>", "expires": "<YYYY-MM-DD>" }
  ]
}
```

| 字段 | 规则 |
|---|---|
| `version` | 固定 `1`；不认识的版本号使配置无效 |
| `module` | MUST 是 `content-boundaries.json` 里声明的模块之一；未知模块名使配置无效 |
| `reason` | MUST 非空。存在的意义是让豁免在 diff 里读起来像一个决定，而不是一行白名单 |
| `where` | MUST 非空，指出实际接入位置（如 `server/internal/handler/xxx.go`） |
| `expires` | **MUST 存在且为 `YYYY-MM-DD`**。缺失即整份配置无效。一条没有期限的豁免会变成永久豁免，而永久豁免与删掉检查的效果相同 |

**到期后的行为**：`expires` 已过的豁免**停止生效**，该模块重新按 E1/E2/E3 判定；失败输出 MUST 指明原因是**豁免已过期**，而不是笼统地报缺证据——两者要补的东西不同（前者是续期或真接入，后者只是接入）。

**配置无效一律失败**，不降级为告警——一个读不懂自己配置的检查比没有检查更危险。

## 模块判定结果（进程内）

每个**已落地**模块产出一条判定：

```text
{ module, landed: true, e1Import: bool, e2CallSite: bool, e3Test: bool, exempt: bool }
```

| 条件 | 含义 | 判定方式 |
|---|---|---|
| **landed** | `server/internal/content/<module>/` 下有至少一个 `.go` 文件 | 未落地则完全跳过，不产出任何输出 |
| **E1** | 至少一个**非测试** `.go` import 了 `content/diagnostics` | 词法扫描，跳过注释与字符串 |
| **E2** | 同目录内至少出现一次审计 / 技术日志 / trace 三类调用点**之一** | 见下表 |
| **E3** | 至少一个 `_test.go` 引用 `diagnostics` | 词法扫描 |
| **exempt** | 该模块在豁免配置中登记**且未过期** | 登记且未过期时三条均不再要求；已过期视同未登记 |

**通过条件**：`exempt || (E1 && E2 && E3)`。

## E2 认可的调用点

| 类别 | 公共入口 |
|---|---|
| 审计 | `Audit`、`CommitRun` |
| 技术日志 | `Technical`、`SlogHandler`、`LogBuffer` |
| trace | `Child`、`Pack`、`Unpack`、`DecodeQueuedEnvelope` |

**刻意不认**：`NewID`、`Sanitize` 等工具函数。用了它们不代表接入了诊断——E2 存在的全部价值就是把「import 了但只用工具函数」挡在外面。

## 输出契约

| 情形 | 输出 | 退出码 |
|---|---|---|
| 全部已落地模块通过 | 一行摘要（检查了几个模块、跳过几个） | 0 |
| 某模块缺 E1 / E2 / E3 | 每个缺项一行，**点名模块与具体哪一条** | 非 0 |
| 配置无效（含缺 `expires`） | 说明哪里无效 | 非 0 |
| 豁免已过期 | 指明该模块的豁免过期日期，并按 E1/E2/E3 重新判定 | 非 0 |
| 无已落地模块 | 摘要说明「0 个模块已落地」 | 0 |

## 合同文本的行结构（`docs/development/diagnostics-onboarding-contract.md`）

每个诊断面一行：

| 列 | 要求 |
|---|---|
| 要做什么 | 一句话 |
| 公共入口 | 真实存在的导出符号；**没有就直说没有**（错误码枚举即此情形） |
| 怎么算做到了 | MUST 可判定；写不出这一列的条目说明该项还没想清楚 |
