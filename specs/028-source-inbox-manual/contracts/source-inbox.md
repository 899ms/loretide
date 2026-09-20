# Contract: source-inbox（028 实施 PR 1）

面向 `server/internal/content/source-inbox/` 与 `/api/content-sources`。页面不在本 PR。

## 受控集

| 集合 | 值 | 越界 |
|---|---|---|
| `kind` | `pasted_text`、`url` | 400，`{"field":"kind"}` |
| `status` | `inbox`、`organized`、`archived` | 400，`{"field":"status"}` |

Go 枚举是权威；数据库 `CHECK` 是兜底。**没有第三个受控集**——解析状态列本卡不建（spec Out of Scope 1），守卫测试钉住这一点。

## 表

### `content_source`

| 列 | 可变 | 说明 |
|---|---|---|
| `source_id` | 否 | |
| `workspace_id` | 否 | |
| `kind` | **否** | 受控 |
| `url` | **否** | `kind = url` 必填；`pasted_text` 必须为空 |
| `captured_at` | **否** | |
| `recorded_by` | **否** | |
| `historical_import` | **否** | 布尔，创建时定（§3.3） |
| `title` | 是 | |
| `tags` | 是 | `text[]` |
| `annotation` | 是 | 个人批注（§4 步骤 1） |
| `personal_judgement` | 是 | 个人判断（§4 步骤 4），与原文分列 |
| `status` | 是 | 受控 |
| `updated_at` | 是 | |

五个「否」由守卫 2 钉住：任何 `UPDATE content_source` 的 SET 段都不得出现它们。

### `content_source_snapshot`（只插不改）

`snapshot_id`、`workspace_id`、`source_id`、`content`、`content_hash`（sha256 十六进制）、`captured_at`。

`kind = pasted_text` 恰好一条；**`kind = url` 零条**（首版 0 抓取）。

### `content_source_revision`（只插不改）

`revision_id`、`workspace_id`、`source_id`、`changed_fields`（`text[]`）、`actor_id`、`created_at`。

记的是**哪些字段变了**，不是整条快照——不可变部分本来就不会变，抄进每条记录只会让历史变胖。

## 端点

所有端点经 `workspace-core.Authorize`；越权拒绝与「不存在」**同形**。**没有 DELETE。**

### `GET /api/content-sources`

`?status=` `?tag=`。→ `{"sources":[...]}`，按 `captured_at` 倒序。

### `POST /api/content-sources`

```json
{"kind":"pasted_text","title":"…","content":"…","tags":["a"],
 "annotation":"…","personal_judgement":"…","historical_import":false}
```

- `kind = pasted_text`：`content` 必填非空；同事务写一条快照并算哈希。
- `kind = url`：`url` 必填且格式合法；`content` 必须缺省；**不写快照、不发任何出站请求**。

→ `201`，`{"source":{…},"duplicates":["<source_id>",…]}`。

`duplicates` 是**提示**：哈希相同的已有条目。**它不阻止创建，不触发任何合并或删除**（R-011「内容相同不删除独立的收藏上下文与批注」）。`kind = url` 恒为空数组——没有快照就没有哈希（见「已知限制」）。

### `GET /api/content-sources/{id}` → `{"source":{…},"snapshot":{…}|null}`

### `PATCH /api/content-sources/{id}`

可改 `title` / `tags` / `annotation` / `personal_judgement` / `status`。传入任何不可变列 → 400 并点名该字段。→ `{"source":{…}}`，并追加一条整理记录。

### `GET /api/content-sources/{id}/revisions` → `{"revisions":[…]}`，时间正序。

### `POST /api/content-sources/bulk`

```json
{"source_ids":["…"],"add_tags":["a"],"status":"archived"}
```

→ `{"results":[{"source_id":"…","ok":true},{"source_id":"…","ok":false,"reason":"not_found"}]}`

**逐条结果**：部分失败时成功的保留、失败的点名，不声称整批成功。每条成功各追加**一条**整理记录。

### `GET /api/content-sources/duplicates?content_hash=…`

→ `{"sources":[…]}`。**只返回候选。不合并、不删除、不改动任何行。**

## 不变量（每条都有用例）

1. 五个不可变列在任意次整理后逐字节未变。
2. `content_source_snapshot` 与 `content_source_revision` 无 UPDATE / DELETE 路径，且各有 INSERT。
3. `kind = url` 的条目快照数恒为 0；模块源码零出站字样。
4. 重复的两条**都还在**，各自批注互不影响。
5. `archived` 的条目仍可查到；全模块无 DELETE 端点。
6. 批量 M 条产生 M 条整理记录。
7. 带路径参数的端点，路径值 ≠ 上下文值时按 404 同形拒绝（第 12 步）。

## 已知限制

**URL 条目没有去重提示。** 去重按 `content_hash`，哈希在快照上，而 `kind = url` 本卡不生成快照——两次收同一个链接不会被提示重复。这是「URL 0 抓取」与「按内容哈希去重」两条裁决相乘的结果，**不是遗漏**（spec Out of Scope 已记）。
