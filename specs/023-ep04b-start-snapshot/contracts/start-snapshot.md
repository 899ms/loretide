# 合同：开始与输入快照（EP-04b）

**状态**：草案，随 `spec.md` 的三条待裁决而定。本文件按推荐值 Q1=A / Q2=A / Q3=A 写。

**对齐对象**：`server/internal/content/diagnostics/contract.go:51` 的 `Snapshot`。**字段名逐名沿用，不新造同义字段。**

---

## 1. 两个「start」

| 谁 | 端点 | 含义 | 状态 |
|---|---|---|---|
| EP-04a | `POST /api/content-topics/{id}/actions` body `{"action":"start"}` | 接受选题，冻结首版简报，卡 → `started` | **既有，本卡不改** |
| EP-04b | `POST /api/content-topics/{id}/briefs/{revisionId}/start` | 用这一版简报开一次工，固定输入快照 | 本卡新增 |

读回：`GET /api/content-topics/{id}/briefs/{revisionId}` —— **既有端点**，响应体多一个 `snapshot` 字段。

**没有更新端点，没有删除端点。** 快照的不可变由「没有写它的第二条路径」保证，不由约束保证。

---

## 2. 请求

```json
{
  "account_id": "…",
  "source_scope": "all",
  "project_id": ""
}
```

- `account_id` 必填：选题卡的 `account_id` 可空，而就绪判定是按账号算的。
- `source_scope` 必填，受控集 `local|web|all`。
- `project_id` 可选，留空合法（FR-018）。
- 未知字段一律拒绝（`DisallowUnknownFields`），与账号档案写入同一读法。
- 请求体**不接受**任何时间字段：快照时间由服务端生成（FR-017）。
- 请求体**不接受** `snapshot`：调用方不能自带一份快照。

## 3. 决策顺序

按此顺序，**第一个不通过者即拒绝**，不继续：

| 序 | 检查 | 失败时 |
|---|---|---|
| 1 | 工作区成员资格（`workspace-core.Authorize`） | `RefusalStatus` / `RefusalBody`，与「不存在」同形 |
| 2 | 选题卡存在且属本工作区 | 同上 |
| 3 | 简报版本存在、属本工作区、且属这张卡 | 同上 |
| 4 | 账号存在且属本工作区 | 同上 |
| 5 | `source_scope` 在受控集内 | 400 诊断错误对象（`ErrScope` 的既有读法） |
| 6 | 账号就绪判定 `can_start == true` | 400 诊断错误对象，**列出 `missing[]`** |
| 7 | 该简报版本尚未被开始过（Q1=A 的一对一） | 409 诊断错误对象，`retryable: false` |

**1–4 的拒绝体必须逐字节相同**：任何差异都能被用来探测某个 id 是否在别的品牌存在。

**6 与 1–4 不同形**是故意的：缺项是**调用方自己账号的**配置问题，告诉他缺什么不泄露任何别人的东西。

## 4. 十六个字段怎么填

| 快照字段 | 来源 | 本阶段 |
|---|---|---|
| `config_version` | 无来源 | **恒空** `""`（Q2） |
| `persona_ref` | 账号当前 `content_account_revision.revision_id` | 开始那一刻读到的值 |
| `sop_version` | 无来源 | **恒空** |
| `skill_version` | 无来源 | **恒空** |
| `rule_version` | 无来源 | **恒空** |
| `executor` | 宪法 IX | 固定字面量，表示「禁用」 |
| `executor_version` | 同上 | **恒空** |
| `source_scope` | **请求里本次的选择** | `local` / `web` / `all` |
| `saved_preference` | 开始那一刻读到的 `settings["loretide.scope"]`（读不到填 `all`） | 与上一行**各自独立**，可以不同 |
| `required_sources` | EP-04d / W-03 | **恒空** `[]` |
| `excluded_sources` | EP-04d / W-03 | **恒空** `[]` |
| `grants` | LT-016 只有判定纯函数，无存储 | **恒空** `[]` |
| `file_hashes` | 无本地文件输入 | **恒空** `{}` |
| `temperature` | 不调模型 | `0` |
| `budget` | 简报的 `cost_limit` 是自由文本，不可映射为整数 | `0`，原因记在此 |
| `timeout_ms` | 简报的 `time_limit` 同上 | `0`，同上 |

**恒空的九项各有一条负例**：它们非空时用例变红。这不是把 TODO 写成断言——它是在说「今天这些没有真实来源，谁要填上必须先改这条规则」。

### 三处同名不同物，不得混为一谈

| 名字 | 是什么 | 受控集？ | 在快照里 |
|---|---|---|---|
| `BriefRevision.source_scope`（022） | §5.3 十一项里的「资料范围」**散文** | 否，自由文本 | **不参与**。它随简报版本一起被钉住 |
| `settings["loretide.scope"]`（LT-014） | 账号「上次选了什么」 | 是，`local/web/all` | `saved_preference` |
| 本次开始的选择 | 这一次实际用哪个范围 | 是，同上 | `source_scope` |

**本卡不得给 `BriefRevision.source_scope` 加受控集校验** —— 022 已存的自由文本行会因此变成非法。

### 两项记录但不参与判定

| 事项 | 为什么记 |
|---|---|
| 品牌自动预检开关（LT-015） | 事后要解释「这一篇为什么没走预检」。**触发是 EP-06，本卡不据此拒绝** |
| 中性表达（021 的 `uses_neutral_expression`） | 事后要解释成稿为什么是中性口吻。021 已定它是标记不是门槛，**不阻止开始** |

> **这两项在 `diagnostics.Snapshot` 里没有对应字段。** 不能塞进既有字段——十六个都各有含义。推荐在快照 JSON 里加两个 `topic-planning` 自己的键，并在合同与代码注释里显式标注为**扩展**，不属于对齐的十六项。这样「与 Snapshot 对齐」这句话仍然精确。**此点请随 Q1 一并裁决。**

## 5. 成功响应

`201 Created`，响应体是被更新后的简报版本（含 `snapshot`）。**不是**一个新实体的 id —— Q1=A 下快照没有自己的 id，它就是这一版简报的一部分。

## 6. 不可变的自证

- 写路径只有一条（`start`），且 `snapshot` 只在仍为 `'{}'::jsonb` 时可写（写入语句带 `WHERE snapshot = '{}'::jsonb`，影响行数为 0 即 409）；
- `content_brief_revision` 已有「只插不改不删」的守卫用例（022 的 A6）。本卡**不得**为了写 snapshot 而放宽它：这条 UPDATE 要在守卫用例里显式登记为**唯一允许的一条**，并有用例钉住「它只能把 `{}` 变成非 `{}`，不能改已有值」；
- 写入与工作区删除栅栏在同一事务（#104）。

> **这是 Q1=A 的真实代价**，写在这里而不是留到实施时发现：为了不加表，简报版本表上被开了一条受限 UPDATE。若主任务认为 022 的「只插不改」不容破例，**请选 Q1=B**（新表），代价是与拆分卡的「无新表」冲突。

## 7. 迁移

```sql
-- 490_content_brief_revision_snapshot.up.sql（单语句、无外键、无索引）
ALTER TABLE content_brief_revision
  ADD COLUMN snapshot jsonb NOT NULL DEFAULT '{}'::jsonb;
```

- 加列不加表 → **工作区删除清单无需改动**，但仍跑 `TestWorkspaceDeletionManifestCoversPublicSchema` 证明；
- 不加索引 → **不触发 #122 的 R6**，但 tasks 保留一条核对项；
- down 直接 `DROP COLUMN`。**已开始的快照会因此丢失**，在迁移注释里写明，与 482 的处理一致。
