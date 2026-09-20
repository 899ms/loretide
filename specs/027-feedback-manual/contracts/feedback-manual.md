# 合同：人工指标与反馈摘录（027，人工切片）

**状态**：**六条 clarify 已裁决**（主控 2026-09-21，全文见 PR #163 评论）。SOP §10.1 / §7.1「AI 复盘」行 / §3.2 与 PRD R-044 R-045 原文抄在 `spec.md` 开头。下面每一个列名、每一个受控集都要能指回其中一句。

---

## 1. 两张表

迁移号**在实施时按当时的最大值顺延**（本规格写成时最大号是 515）。下文用 `N`、`N+1`… 指代。

**为什么是两张不是三张**：AI 复盘本阶段产生不出任何内容，所以它**没有表**——状态是派生的（没有报告就是 `pending_data`）。建一张恒空的表只会让人以为它迟早会被填，而删掉一张已经推过的表要再加一个迁移。见「裁决之外我补的两处」。

### `content_manual_metric` —— 人工指标观测（**只插不改不删**）

§10.1 点名十项，一列一项，一个不多一个不少：

| 列 | 指回原文的哪一段 | 说明 |
|---|---|---|
| `manual_metric_id` | — | 稳定键。唯一性走并发索引，**不是** `PRIMARY KEY`（R5） |
| `workspace_id` | — | 一切查询按它过滤；删除按它删 |
| `publication_record_id` | 「每条观测**关联发布记录**」 | 025 的发布记录。**不存 `version_id`**——版本两跳解出（§3） |
| `platform` | 「保存**平台**」 | 受控集四值，与 025 渠道一致；**不 import `ip-profile`**，用一条读源文件对表的用例 |
| `account_id` | 「**账号**」 | 字符串 |
| `metric` | 「**指标名**」 | 受控集**恰好十一项**（§2），**无「其它」** |
| `value` | 「**数值**」 | **可空**。`NULL` 是「未知」，`0` 是「已确认的零值」——§10.1 原话 |
| `unit` | 「**单位**」 | 自由文本（原文没给枚举） |
| `window` | 「**统计窗口**」 | 自由文本。见「裁决之外我补的两处」第 2 点 |
| `sampled_at` | 「**采样时间**」 | 看这一眼的时刻 |
| `recorded_by` | 「**录入人**」 | 服务端从会话取，**不接受调用方指定** |
| `evidence_note` | 「**证据附件**」 | 自由文本。**附件本身等 W-03**，本阶段没有任何附件存储路径，有负例 |
| `source_type` | 「**数据来源类型**」 | **系统写入**，受控集 `manual` / `csv_import`，**不是请求体字段**（§4 说明怎么写进去的） |
| `created_at` | — | |

**`value` 用 `bigint` 而不是 `text`**：它就是个数，存成文本会让每个读端各自解析一次，解析失败时还得各自决定怎么办。可空正是为了表达「未知」——这是 `NULL` 这个值本来就该干的事。

**没有 UPDATE。** R-044 的「更新保留历史」就是这个意思：抄错了就再录一条，读端取最新。改写第一条等于把「我当时抄错了」这件事抹掉，而那本身是历史的一部分。

### `content_feedback_excerpt` —— 反馈摘录（**只插不改不删**）

| 列 | 指回原文 | 说明 |
|---|---|---|
| `feedback_excerpt_id` | — | 稳定键 |
| `workspace_id` | — | |
| `publication_record_id` | — | 绑一条发布记录 |
| `source_type` | 「**评论、私信和线索**」 | 受控集**恰好三项** `comment` / `private_message` / `lead`，**无「其它」** |
| `redacted_excerpt` | 「支持**脱敏摘录**」 | 别人说的话，**已由人脱敏**。系统不做自动脱敏 |
| `interpretation` | 「**自己的解释**」；R-045「引用摘录与运营者判断**分别保存**」 | 录入者自己的判断 |
| `tags` | 「**分类**」；R-045「标签」 | 自由标签，`text[]` 或逗号分隔——实施时定，规格只要求「多个、自由」 |
| `occurred_at` | — | 这句话是什么时候说的 |
| `recorded_by` | — | 服务端从会话取 |
| `created_at` | — | |

**摘录与解释是两列，不是一段。** 混成一段之后，谁也分不出哪句是证据、哪句是判断——而 §10.2 的复盘正是要拿证据去检验判断。两列都可以各自为空。

### 每张表两个索引

| 索引 | 列 | 服务什么 |
|---|---|---|
| `content_manual_metric_id_unique_idx` | `(manual_metric_id)` | 稳定键唯一性（R5 不写在建表里） |
| `content_manual_metric_record_idx` | `(workspace_id, publication_record_id, sampled_at DESC)` | 「列一条发布记录的指标」「这条有没有指标」（待补录判定）与**删除链** |
| `content_feedback_excerpt_id_unique_idx` | `(feedback_excerpt_id)` | 同上 |
| `content_feedback_excerpt_record_idx` | `(workspace_id, publication_record_id, occurred_at DESC)` | 「列一条发布记录的摘录」与**删除链** |

**两个 `record_idx` 都以 `workspace_id` 打头**：另两个以稳定键打头，没有这两个的话删除链会在持锁期间扫全表。

**共六个迁移文件**（两个建表 + 四个索引），每个单条语句；建表迁移不含 `PRIMARY KEY` / `UNIQUE`（R5）；四个索引登记进 `cmd/migrate` 的 `concurrentIndexCleanups`（R6），**两个建表迁移不登记**；**新条目放进自己的块**（前空行 + 一行注释），否则 gofmt 会重排既有对齐（025 因此返工过一次）。

---

## 2. 受控集（四个）

| 集合 | 取值 | 出处 |
|---|---|---|
| `metric` | `impression` / `read` / `play` / `completion` / `like` / `comment` / `favorite` / `share` / `follow` / `direct_message` / `conversion` —— **恰好十一个** | §10.1「曝光、阅读、播放、完播、点赞、评论、收藏、分享、关注、私信和转化」。**没有「其它」** |
| `platform` | `xiaohongshu` / `wechat_mp` / `douyin` / `shipinhao` —— 四个 | 与 025 的渠道同一份；一条用例读 `ip-profile` 的 Go 源文件对表 |
| `source_type`（指标） | `manual` / `csv_import` —— 两个，**系统写入** | §10.1「数据来源类型」；「数据以运营者手工记录和文件导入进入系统」 |
| `source_type`（摘录） | `comment` / `private_message` / `lead` —— **恰好三个** | §10.1「评论、私信和线索」。**没有「其它」** |

**另有一个受控集不落在表上**：AI 复盘的状态，§7.1 原文七值 `pending_data` / `queued` / `generating` / `generated` / `failed` / `edited` / `superseded`。本卡**只产生 `pending_data`**，其余六个是保留项，有一条扫源码的负例（照 024 对 `generated` 的做法）。

**「阅读」与「播放」是两个值，不是一个。** §10.1 原文：「不同平台的'阅读'和'播放'分别保留，不直接合并排名」。记录层天然分开；另有一条用例禁止任何把两者相加或排名的路径。

**没有第五个受控集。** `unit`、`window`、`evidence_note`、`redacted_excerpt`、`interpretation`、`tags` 全是自由文本——原文点名了字段但没给取值，按 025 Q5 立下的规矩不发明。一条扫源码的负例守住这条。

---

## 3. 版本怎么解（裁决 Q2=A）

`content_publication_record` **没有 `version_id` 列**。版本在**读时**两跳解出：

```text
manual_metric.publication_record_id
  → content_publication_record.delivery_task_id
    → content_delivery_task.review_request_id
      → content_review_request.version_id
```

**任何一跳断了就是 `unknown`**，与 025 `version_match` 的三态口径一致。补记历史的发布记录 `delivery_task_id` 是空串，所以它永远解不出版本——**这不构成拒绝录入的理由**。数据是真的，只是不知道对应哪一版。

**不改 025 的表**：给发布记录加一列 `version_id` 会更干净，但 025 的裁决刻意没给它这一列，而改一张已合入的表要再加一个迁移加一次回填。

---

## 4. 端点与决策顺序

| 方法 | 路径 | 用途 |
|---|---|---|
| `POST` | `/api/content-metrics` | 表单录一条，服务端写 `source_type = manual` |
| `POST` | `/api/content-metrics/import` | 批量录入，服务端写 `source_type = csv_import`；**全有或全无** |
| `GET` | `/api/content-metrics` | 列出，可按 `publication_record_id` / `artifact_id` / `metric` 筛 |
| `POST` | `/api/content-feedback` | 摘一条 |
| `GET` | `/api/content-feedback` | 列出，可按 `publication_record_id` / `artifact_id` 筛 |
| `GET` | `/api/content-feedback/pending` | 待补录（派生） |

**`source_type` 是怎么做到「系统写入」的**：两个端点，服务端各写死一个值。做成请求体字段再校验，等于把「这条数据怎么来的」交给调用方声明——那就不是来源了。

**没有任何路径参数。** 两张表都只插不改，没有单条改动路径，所以第 12 步的「参数 ≠ 上下文」用例在本卡**没有对象**——这件事要在 PR 正文写明，而不是默默跳过（FR-032）。路由存在性用例照常要。

决策顺序，**第一个不通过者即拒绝**：

| 序 | 检查 | 失败时 |
|---|---|---|
| 1 | 工作区成员资格（`workspace-core.Authorize`） | `RefusalStatus` / `RefusalBody`，与「不存在」同形 |
| 2 | 发布记录存在且属本工作区 | 同上 |
| 3 | 受控集校验（`metric` / `platform` / 摘录的 `source_type`） | 400 诊断错误对象，**点名字段**；批量时**点名第几行** |
| 4 | 必填项（摘录的 `occurred_at`；指标的 `metric` / `sampled_at`） | 400，**点名缺的那一项** |
| 5 | 长度上限（按 rune） | 400，点名字段 |

**1–2 的拒绝体逐字节相同**（`trace_id` 除外）。3–5 与 1–2 不同形是故意的：那是调用方自己输入的问题，说清楚不泄露别人的任何东西。

**批量是全有或全无**：任一行不通过，整批不写，拒绝里点名第几行的哪一列。部分写入会让人以为全录上了——而「录上了几条」正是这个功能唯一要回答的问题。

---

## 5. 待补录（派生，不带时间逻辑）

```text
待补录 = 发布记录 WHERE status IN ('reported_published','verified_published')
         AND NOT EXISTS (指标记录 WHERE publication_record_id = 它)
```

**没有时间比较**。裁决 Q5=A：品牌级的「反馈观察时点」是 §3.2 那张卡的活，今天没有那个设置，「过了没过」就无从判断。写一个默认天数等于发明一条 SOP 没说的规则。

**没有「待补录」这个存储状态**——与 025 的「待登记」同一口径。一条检索用例确认存储层里既没有这个列，**也没有任何时间比较**。

`failed` / `removed` / `unknown` 的发布记录**不在**待补录里：没发出去的东西谈不上补录真实结果。

**今日工作台第五项用的就是这条**（FR-025）：core 提供一个派生函数，PR 1 交付。

---

## 6. 不外发、不聚合、不复盘的自证

- **只插不改**：两张表没有 UPDATE、没有删除链之外的 DELETE，守卫扫模块源码（022 `TestBriefStoreHasNoUpdateOrDeletePath` 的形状），**且必须有 `INSERT INTO`**，否则空模块也绿；
- **不外发**：源码里不存在 HTTP 客户端调用、不存在平台凭据的读写、不存在模型调用——三条守卫，一次可编译变异确认会红；
- **不聚合**：源码里不存在 `sum(` / `avg(` 之类的聚合查询（删除清单的按 `workspace_id` 计数除外），也不存在分类或打分路径；
- **不合并阅读与播放**：一条检索用例，外加 `read` 与 `play` 各自读出互不影响的用例（§10.1 原文）；
- **不复盘**：AI 复盘状态集七值，本模块只产生 `pending_data`，一条扫源码的负例确认其余六个没有产生路径；
- **没有附件路径**：`evidence_note` 是自由文本，源码里不存在任何文件上传或附件引用；
- **没有自动脱敏**：源码里不存在任何个人信息识别或替换路径。脱敏是人做的，界面写明。

---

## 7. 对外的两处改动（都不改登记表）

1. **`specs/026` 的 FR-004 / FR-005b**（FR-025）：PR 1 同时更新——把 `feedback-learning` 从「不读未落地模块」的名单里去掉，第五项不再是「暂不可用」。**这是规格文件的改动，不是代码**。
2. **`review-delivery` 页面组件上的一个可选 render 插槽**（FR-026）：页面 PR 用，由 web 适配器注入本卡的区块。与 #161 对 `work-editor` 做的完全一样——`feedback-learning` 的依赖表里没有 review-delivery 的反向边，所以只能这样接。

`cmd/server/router.go` 与 `cmd/migrate/main.go` 是上游文件 → 路由与索引登记放进**同一个 `upstream:` 提交**，PR 正文单列「上游改动」一节（第 13 步）。
