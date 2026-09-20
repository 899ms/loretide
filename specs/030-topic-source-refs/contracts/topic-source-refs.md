# Contract: 选题卡引用素材条目（030）

**裁决**：主控 2026-09-21，PR #207 评论。Q1 = B（两列，按 §5.2 原文改名与归属）、Q2 = A、Q3 = B。

**原文锚点**：§5.2 第 2 项「为什么适合这个 IP，**引用哪些素材**和过去的经营结论」；第 5 项「**证据是否充分，存在什么缺口**，需要多少研究或制作投入」。**第 4 项「与已有作品的关系」指作品，不是素材，本卡不碰。**

---

## 1. 存储：选题卡上的两列，不是一张表

两个加列迁移，各一列，都在既有的 `content_topic_card` 上：

| 迁移 | 列 | 类型 | 对应 §5.2 |
|---|---|---|---|
| **532** | `fit_source_ids` | `jsonb NOT NULL DEFAULT '[]'::jsonb` | 第 2 项「引用哪些素材」 |
| **533** | `evidence_source_ids` | `jsonb NOT NULL DEFAULT '[]'::jsonb` | 第 5 项「证据…缺口」 |

`jsonb` 而不是 `text[]`：同一张表的 `channels` 就是 `jsonb`（迁移 483），`store.go` 的 `encodeStrings` / `decodeStrings` 已经是 `json.Marshal` / `json.Unmarshal` 一对。跨表去学 `content_source.tags` 的 `text[]` 会让同一张表里两种集合两种形状。

**两列而不是一列**：第 2 项与第 5 项问的是两个问题。第 2 项的素材是**立论依据**（这个选题为什么适合这个 IP），第 5 项的素材是**证据**（够不够、缺什么）。同一条素材可以只属于其中一个。压进一个列表，「这条为什么在这里」就只能靠人猜——这是本仓反复踩的同一类（019 的布尔、027 的指标值、029 的天数、#197 的三态）。

**两列而不是一张关联表**：反向视图不做（Out of Scope 1），而关联表买到的正是反向查询。它的账单是 4 个迁移 + 删除清单 + 删除链 CTE + 一套新 store。等真要反向视图时，加一个 GIN 并发索引即可，**不需要数据迁移**。

**没有第三列。** §5.2 第 2 项后半句的「过去的经营结论」（§10.3 Learning）未落地——027 的 FR-023 明写不为它建表。一个恒空的列是一条没人做过的声明，023 的 `required_sources` 就是拿负例挡住这种东西的，不是拿空字段圆过去的。

### 1.1 R1–R6 在本卡的适用范围

| 规则 | 本卡 |
|---|---|
| R1 无 `REFERENCES` / `FOREIGN KEY` | 适用，两个迁移都不含 |
| R2 无 `CASCADE` | 适用，两个迁移都不含 |
| R3 索引必须 `CONCURRENTLY` | **无新增索引**，无对象可违反 |
| R4 含并发索引的文件单语句 | 同上 |
| R5 建表不内联 `PRIMARY KEY` / `UNIQUE` | **无新建表** |
| R6 索引登记进 `concurrentIndexCleanups` | **无新增索引，`server/cmd/migrate/main.go` 零改动** |
| 工作区删除清单 | `content_topic_card` **已在册**（`workspace_delete_manifest_test.go:66`），加列不改它 |

**这张表必须在交付时逐行复核，不是默认成立**（SC-014）。

---

## 2. 归一化：三件事，`normalizeStrings` 一件都不做

`store.go:57` 的 `normalizeStrings` 只把 nil 变成 `[]`。引用列表需要它之外的三件事，所以另写一个函数，**不改 `normalizeStrings`**（`channels` 还在用它，改它就是改 022 的行为）。

每一栏，按顺序：

1. **去空白**：`strings.TrimSpace` 后为空的丢掉。空字符串 id 不是「没引用」，是一条坏数据。
2. **去重**：保留**首次出现**的顺序。同一条素材被选两次，存一条。
3. **上限**：每栏 **50 条**。超过即 `ErrInvalid` → 400，错误里**指名是哪一栏**（`fit_source_ids` 还是 `evidence_source_ids`），照 `source-inbox` 的 `FieldError` 形状。

**顺序是有意义的**：先去空白再去重，否则 `""` 和 `"  "` 会被当成两条不同的 id 留到去重之后。上限在最后，量的是归一化之后的条数——提交 60 条里有 15 条重复，结果是 45 条，通过。

**两栏各自独立归一化。** 同一条素材出现在两栏里是合法的（它既是立论依据也是证据），不是重复。

---

## 3. 校验：端口只用字符串，栅栏事务内，两种拒绝同形

### 3.1 端口

```go
// SourceReader answers whether a source id belongs to this brand.
//
// Strings only, no source-inbox types: topic-planning's dependency row does
// not list source-inbox (the path through knowledge-base is two hops), and a
// port that carried sourceinbox.ErrNotFound would make this module import it.
// The adapter in handler/content_topic.go does the mapping instead.
type SourceReader interface {
    Exists(ctx context.Context, workspaceID, sourceID string) (bool, error)
}
```

**为什么只用字符串**：`AccountReader`（`store.go:26`）可以直接用 `ipprofile.Account`，因为 `ip-profile` 在 `topic-planning` 的依赖行里。`source-inbox` 不在。签名里一出现 `sourceinbox` 的任何类型，`topic-planning` 就得 import 它，登记表 `modules` 就得改。**本卡不改登记表**，所以端口退到字符串。先例是 #197 的 `Observation`。

`adapters` 也不改：`handler/content_topic.go` 已在册。

### 3.2 校验在哪里

照 `checkAccount`（`store.go:218-237`）与 `SetAccount` 的注释（`store.go:245-247`）：

- 校验在**工作区删除栅栏事务内**。栅栏外答出来的校验，可能在工作区删除之前答完、在删除之后才把写入应用上去。
- 栅栏是事务的**第一条语句**（#104）。
- **外来品牌的 id 与不存在的 id 得到同一个 `ErrNotFound`**（404），响应体逐字节相同——否则一次拒绝就能用来探测哪些 id 存在。

### 3.3 全有或全无

一次请求里只要有一条 id 不合法，整次拒绝，**两栏都不写**。不存在「合法的那几条先存下来」。

### 3.4 `archived` 可以被引用

校验只问两件事：**是不是本品牌的**、**在不在**。状态不参与。

理由在 Current State 第 5 节：`content_source` 全仓只有品牌删除链那一条 `DELETE`，所以归档不是删除。已建立的引用不因为对方被归档而失效——把归档当失效，卡上的引用会**无声地少掉一条**，而没有任何地方会说少了什么。

**选择器是另一回事**：界面默认不把已归档的列进候选（FR-021）。「不推荐新引用」与「已引用的失效」是两件事。

---

## 4. 写入端点：自己的端点，不是动作上的字段

```
POST /api/content-topics/{id}/sources
```

请求体：

```json
{
  "fit_source_ids": ["src-1", "src-2"],
  "evidence_source_ids": ["src-3"]
}
```

| 规则 | 说明 |
|---|---|
| **整列替换** | 给了哪一栏，那一栏就被这个列表整体替换。与 `SetAccount` 同形 |
| **缺省 ≠ 清空** | 请求体里**没有**某个键 → 那一栏**不变**。清空必须用显式的 `[]` |
| **不碰状态** | 不改 `status` / `decision_reason` / `decision_note` / `started_brief_revision_id` |
| **不碰账号** | 不改 `account_id` |
| **不碰正文** | 七项自由文本一个字不动 |
| **审计** | 同事务内写一条，动作名 `link-sources`，照 `SetAccount` 的 `link-account` |

**为什么不做成四个动作上的一个字段**：`SetAccount` 的注释已经写过一次同样的理由——那四个动作改的是卡的去向，这个改的是它的依据；合在一起会让「收藏」能悄悄改掉这张卡引了哪些素材。

**为什么不是 PATCH 整张卡**：今天没有这样的端点（Current State 第 2 节），而开一个能改七项正文的 PATCH 是另一张卡的范围（Out of Scope 6）。本卡只开引用这一条缝。

### 4.1 创建也接受这两个键

`POST /api/content-topics` 同样接受 `fit_source_ids` / `evidence_source_ids`，走**同一套**归一化与校验（US1 是建卡时就挂上）。

`decodeTopicBody`（`content_topic.go:82`）用的是 `DisallowUnknownFields`，所以这两个键**必须加进 `TopicCard` 结构体**才能被接受——这也意味着旧客户端发来的请求不受影响（少两个字段是合法的）。

### 4.2 拒绝体

照既有口径，不新造：

| 情况 | 状态 | 说明 |
|---|---|---|
| 不是成员 / 卡不是本品牌的 | 404 | `workspace-core` 的既有拒绝 |
| 引用的 id 外来或不存在 | **404** | 与上一条**同形**（3.2） |
| 超过 50 条 / 字段类型不对 | 400 | 诊断式错误，**指名是哪一栏** |
| 存储故障 | 500 | 不带数据库原文 |

---

## 5. 冻结：两个新的扩展字段，`required_sources` 一个字不碰

```go
type StoredSnapshot struct {
    diagnostics.Snapshot
    AutoPrecheck          bool     `json:"auto_precheck"`
    UsesNeutralExpression bool     `json:"uses_neutral_expression"`
    // 030: the card's source references as they stood at this start.
    FitSources            []string `json:"fit_sources"`
    EvidenceSources       []string `json:"evidence_sources"`
}
```

**为什么是扩展字段而不是 `required_sources`**：`snapshot.go:45-57` 的注释已经写好了——

> 十六项里每一项都另有含义，借用其中一项会让「与 `diagnostics.Snapshot` 对齐」变成半句真话。

具体到这里：`required_sources` 是 **EP-04d 的「必用资料」**，意思是「这次运行必须用这些」。卡上的引用是「这些素材是我这么判断的依据 / 我的证据」。**不是一回事。** 填进去等于往快照里放一条没人做过的声明——而 023 的 `TestNoSourcelessFieldIsInvented` 存在的全部意义就是拦住这个。

**023 的负例一行不改，继续全绿。** 一条为了拦住「安静的默认值」而写的守卫，不该因为来了个新需求就被改写。029 的 Q3 用同样的理由保住了 027 的守卫（改成「禁止**写死的**天数」而不是删掉）；这里更省事，连改都不用改。

**两栏分开冻。** 合并成一个列表，快照就答不出「哪条是立论依据、哪条是证据」——而快照存在的理由就是把当时的状态如实钉住。

**空是空数组，不是 null。** 照 `snapshot.go:86-89` 的既有口径：这两个字段会被 marshal 进快照那一行，nil 读回来是 JSON `null`，空切片读回来是 `[]`，消费方不该需要知道自己拿到的是哪一个。

---

## 6. 前端读：缺失读成空数组，绝不读成「解析失败」

| wire | 读成 |
|---|---|
| 字段存在且是字符串数组 | 原样 |
| 字段**缺失**（旧后端） | `[]` |
| 字段**类型不对** | `[]`，**且这张卡的其余字段照常可读** |
| 整条响应不是对象 | 整列表退化为 `[]`，照 `parseWithFallback` 的既有口径 |

**为什么类型不对不能让整张卡消失**：这两个字段是本卡新加的。一张在它们出现之前就能正常显示的卡，不该因为其中一个的形状不对就整张读不出来。先例是 #197 给 `due` 加的 `.catch("")`——同样的理由，同样的写法。

---

## 7. 不做什么（每条都有守卫或负例）

| 不做 | 守卫 |
|---|---|
| 不给 §5.2 第 4 项「与已有作品的关系」加引用字段 | 契约用例：`TopicCard` 上**只有两个**引用字段，名字逐字相同 |
| 不为「过去的经营结论」留字段 | 同上一条的负例：第三个引用字段不存在 |
| 不给 `content_brief_revision` 开 UPDATE | 022 的 A6（`store_integration_test.go:468`）**一行不改** |
| 不填 `required_sources` / `excluded_sources` | 023 的 `TestNoSourcelessFieldIsInvented` **一行不改** |
| 不新增索引、不改删除清单、不改 `concurrentIndexCleanups` | 迁移复核（SC-014）+ `workspace_delete_manifest_test.go` 零改动 |
| `topic-planning` 不 import `source-inbox` | 端口签名只用字符串 + `pnpm check:content-boundaries` |
| 不改登记表 | `scripts/content-boundaries.json` 零改动（`modules` 与 `adapters` 都是） |
| 不做反向视图 | 无此端点、无此查询 |
| 不调用任何执行器、不产生系统生成的关联建议 | 照 028 FR-001 的负例形状：本模块没有任何路径能产生它 |
| 引用入口不碰状态 / 账号 / 正文 | 两个方向各一条用例（SC-003） |

---

## 8. 第 12 步：路径参数用例

`POST /api/content-topics/{id}/sources` 带路径参数，所以同一个 PR 必须有一条：

- 穿过**真实 router 与中间件**（不是 `httptest` 直调 handler）；
- 路径参数值**与上下文里的工作区 id 不同**；
- 取参数只用 `chi.URLParam`，不借用会读上下文的助手。

`topicCardIDFromURL`（`content_topic.go:88`）今天就是 `chi.URLParam(r, "id")`，本卡沿用，**不新写取参数的助手**。

LT-011 / LT-012 / LT-013 的三次教训写在 `docs/development/spec-kit-workflow.md` 第 12 步。
