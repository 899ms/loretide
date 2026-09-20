# 合同：审核请求、交付任务与发布记录（025，人工切片）

**状态**：**五条 clarify 已裁决**（主控 2026-09-21，全文见 PR #144 评论）。本合同按裁决写：Q1=A（八键快照）、Q2=A（可选引用 + §9.3 的 held 规则）、Q3=A（可变 `status` + append-only 迁移记录）、Q4=A（渠道四值、交接三值、只有渠道稿可审）、**Q5 不发明枚举**。

**§7.1 三行原文**：

> 审核请求 `pending → changes_requested / approved / rejected / cancelled`（审核绑定不可变版本和交付快照）
>
> 交付任务 `draft → ready → scheduled / handed_off；可 cancelled / held`（scheduled 为人工待办计划时间，handed_off 表示已导出或交给运营者）
>
> 发布记录 `reported_published / verified_published / failed / removed / unknown`（每条带声明者、证据和核验方式）

**§8 / §9.1 / §9.2 / §9.3 原文**抄在 `spec.md` 的「SOP 原文」一节。下面每一个受控集、每一个必填项都要能指回其中一句。

---

## 1. 四张表

迁移号**在实施时按当时的最大值顺延**（`specs/024` 的提案占用 494–503，尚未合入）。下文用 `N`、`N+1`… 指代。

### `content_review_request` —— 审核请求（`status` 可变，Q3=A）

| 列 | 说明 |
|---|---|
| `review_request_id` | 被引用的稳定键。唯一性走并发索引，**不是** `PRIMARY KEY`（R5） |
| `workspace_id` | 一切查询按它过滤；删除按它删 |
| `work_id` / `artifact_id` | 字符串，**不 import `work-editor` 的包**（与 024 对 `topic_card_id` 的处理同口径） |
| `version_id` | **被冻结的那一版**。work-editor 的版本只插不改，引用它即等于冻结 |
| `account_id` | **目标账号**。§8 把它算进快照，§9.3 又把「更改账号」列为触发重审的三件事之一，所以它必须是一等列而不只是快照里的一个键 |
| `channel` | 受控集四值（§8 首版） |
| `snapshot` | `jsonb NOT NULL`，**不可变的交付快照**，键恰好八个（§3） |
| `status` | 受控集五值，初始 `pending` |
| `requested_by` / `requested_at` | 谁提交的、何时 |
| `decided_by` / `decided_at` | 处置人与时间；未处置时为 `''` / 零值 |
| `decision_note` | 审核意见，自由文本 |
| `created_at` / `updated_at` | |

**没有「当前版本」指针。** 绑定的是 `version_id`，作者之后存多少版都够不到它。这同时就是 §8「系统不得把'已通过'自动套在新版本上」的结构保证：`approved` 写在绑着具体 `version_id` 的那一行上，没有第二处可以读它（FR-004c / SC-017）。

**只有渠道稿可以进来**（Q4 附带一问＝是）：写路径先校验被提交文档的 `kind == channel_draft`，不是就拒并点名 `kind`。校验在**栅栏之内**做——和 022 的同品牌校验同一个理由，否则读到的是事务外的旧值。

### `content_review_transition` —— 状态迁移记录（**只插不改不删**，Q3=A）

| 列 | 说明 |
|---|---|
| `transition_id` | 稳定键 |
| `workspace_id` | |
| `subject_kind` | 受控集 `review_request` / `delivery_task`。**一张表记两类主体**，因为两者的迁移形状完全相同，分两张表只会让「列出这件事的全部动作」要查两次 |
| `subject_id` | 审核请求或交付任务的 id |
| `from_status` / `to_status` | 迁移前后。`from_status` 为 `''` 表示创建 |
| `reason` | **`held` / `cancelled` / 失败 / 延后必填**（§9.2「注明原因」，裁决 Q3）；「明确选择继续交付旧快照」也写在这里（§9.3）。其余迁移可空串 |
| `actor_id` / `created_at` | |

**为什么迁移记录不靠审计事件**：审计是诊断读模型——它会过 `Sanitize`、会按保留期清理、component 白名单里还没有内容模块名（#108 / #126 之后才登记）。产品历史（「谁在什么时候把它 held 了，为什么」）要长期、逐字保留，两件事不该共用一条管道。

### `content_delivery_task` —— 交付任务（`status` 可变）

| 列 | 说明 |
|---|---|
| `delivery_task_id` | 稳定键 |
| `workspace_id` / `work_id` / `artifact_id` | |
| `review_request_id` | **可空串**（Q2=A）：`draft` 阶段允许还没有审核请求；`ready` 及之后要求被引用的请求是 `approved` |
| `channel` | 受控集四值，与审核请求同一份 |
| `status` | 受控集六值，初始 `draft` |
| `scheduled_at` | `scheduled` 时必填；**人看的待办时间**，没有任何代码读它去执行。§9.1 的「到期后产生站内待办」是**读时比较**，不是后台任务（FR-009a） |
| `handoff_method` | `handed_off` 时必填，受控集 `export` / `copy` / `handed_to_operator`（§9.1 三种动作）。**三者都不等于发布成功**（FR-010a / SC-012） |
| `created_at` / `updated_at` | |

`held` 的原因、`cancelled` 的原因**不在本表**，在迁移记录的 `reason` 里——原因属于「那一次动作」，不属于「这个任务现在的样子」。同一个任务可以被 held 两次，理由不同。

### `content_publication_record` —— 发布记录（**只插不改不删**）

Q5 的裁决是**不发明枚举**，所以除 `status` 与 `version_match` 外全是自由文本或布尔。

| 列 | 说明 |
|---|---|
| `publication_record_id` | 稳定键 |
| `workspace_id` / `work_id` / `artifact_id` | |
| `delivery_task_id` | **可空串**：补记历史时可能没有任务 |
| `channel` | 受控集四值 |
| `status` | 受控集五值 |
| `actor_id` | **系统记录的声明者**，服务端从会话取，**不接受调用方指定** |
| `declared_by` | 可选自由文本：「运营同事小王」。actor 是「谁录的」，它是「谁说的」，两者常常不是同一个人 |
| `page_url_or_content_id` | §9.1 的「页面链接或内容 ID」。`reported_published` / `verified_published` **必填** |
| `receipt_note` | 自由文本，§9.1 的「截图/回执」（截图上传等 W-03）。**`failed` / `removed` 时必填非空**，那是它们的原因落点（FR-017b） |
| `verification_note` | 自由文本的核验说明。`verified_published` **必填** |
| `published_at` | §9.1 的「实际发布时间」 |
| `platform_account` | 自由文本，§9.1 的「平台账号」 |
| `platform_edited` | 布尔，§9.1 的「是否在平台上修改」 |
| `edit_note` | 平台改了什么的差异说明 |
| `version_match` | **恰好三态** `matched` / `differs` / `unknown`（§9.1 点名了 `unknown`） |
| `created_at` | |

**没有「当前发布状态」列。** 当前状态是**最新一条记录**（FR-019）。存一份可变的当前状态就是第二份真相，而它一定会在某次并发录入后与记录流不一致。

**也没有「待登记」这个值。** 它是派生的（有 `handed_off` 任务且一条记录都没有），因为 §9.2 的原文是「继续显示'待登记'，**不推断平台状态**」——把它存成一个状态，就等于系统声称自己知道平台上发生了什么。

---

## 2. 状态机（每条迁移都要有用例，SC-003）

### 审核请求

```text
(创建) → pending
pending → changes_requested | approved | rejected | cancelled
四个终态 → (无出边)
```

### 交付任务

```text
(创建) → draft
draft      → ready | cancelled
ready      → scheduled | handed_off | held | cancelled
scheduled  → handed_off | held | cancelled
held       → ready | cancelled
handed_off → (无出边)
cancelled  → (无出边)
```

`held` 的出入边（从 `ready` / `scheduled` 进、回到 `ready`）是本合同的补全，**已被裁决接受**。

**§9.3 的自动 held**：当任务引用的审核快照与当前选定的交付目标不一致时——新稿被选为交付目标，或附件、账号、实质性渠道配置被更改——任务**自动**从 `ready` / `scheduled` 进入 `held`，reason 记明是哪一项变了。回到原状态有两条路：新快照审核通过，或用户**明确选择继续交付旧的已审核快照**（这个选择本身写进 reason，界面同时展示该快照的版本号与预览）。

### 发布记录

**没有状态机**——每条记录是一次独立的声明，五个状态都可以是任意一条记录的值。「只前进不删」（Issue 原话）在这里的落地是：**记录只插不改不删**，而不是「状态不能回退」。一条 `verified_published` 之后再来一条 `removed` 是正常的（作品被平台删了），拦住它反而会让人无法如实记录。

---

## 3. 交付快照（Q1=A，键恰好八个）

提交审核时冻结，写进 `content_review_request.snapshot`。§8 的原文是「**具体渠道、具体文档版本及附件、具体交付配置**的快照」，八个键就是这句话的逐项落点：

| 键 | 记什么 | 指回原文的哪一段 |
|---|---|---|
| `channel` | 提交那一刻选的渠道 | 「具体渠道」 |
| `work_id` | 作品 | 「具体文档版本」 |
| `artifact_id` | 文档 | 同上 |
| `version_id` | 被审版本（与列冗余，使快照单独拿出来也自洽） | 同上 |
| `account_id` | 目标账号 | §8「目标账号发生变化…需要重新审核」 |
| `start_snapshot_id` | 若该作品挂在一次「开始」上则记它，否则 `''`；**空串是真实状态** | 上下文追溯 |
| `attachments` | **数组，W-03 之前恒为 `[]`** | 「及附件」 |
| `delivery_config` | 对象，**扩展键**，首版只放渠道模板标识 | 「具体交付配置」 |

**这份 jsonb 不可变**：它所在的行虽然 `status` 可变，但**没有任何写路径会改 `snapshot` 列**，且有一条用例断言处置前后它逐字节相同。

**`attachments` 恒空要有负例**（SC-011）：本阶段没有附件存储，所以不存在任何往里写东西的路径。写一条「键恰好八个」的用例而不写这条负例，等于只挡住了少键，没挡住有人顺手往里塞一个 id。

**`delivery_config` 现在就留着，不等要用时再加迁移**：它是 jsonb 里的一个键，现在放进去的代价是零；等到需要「具体交付配置」时再加，代价是一次迁移加一次回填。

---

## 4. 端点与决策顺序

| 方法 | 路径 | 用途 |
|---|---|---|
| `POST` | `/api/content-reviews` | 提交审核（body 带 `artifact_id` / `version_id` / `channel` / `account_id`；**只接受渠道稿**） |
| `GET` | `/api/content-reviews` | 列出，可按 `artifact_id` / `status` 筛 |
| `GET` | `/api/content-reviews/{reviewId}` | 单条（含快照与迁移记录） |
| `POST` | `/api/content-reviews/{reviewId}/decision` | 处置（`approved` / `changes_requested` / `rejected` / `cancelled` + 意见） |
| `POST` | `/api/content-deliveries` | 建交付任务 |
| `GET` | `/api/content-deliveries` | 列出（含派生的「到期待办」与「待登记」标记） |
| `POST` | `/api/content-deliveries/{deliveryId}/status` | 推进状态（带 `scheduled_at` / `handoff_method` / `reason`） |
| `POST` | `/api/content-publications` | 录一条发布记录 |
| `GET` | `/api/content-publications` | 列出，可按 `artifact_id` 筛 |

**没有 `{publicationId}` 路径参数**：发布记录只插不改不删，没有单条改动路径，所以第 12 步的用例只有 `{reviewId}` 与 `{deliveryId}` 两类。

决策顺序，**第一个不通过者即拒绝**：

| 序 | 检查 | 失败时 |
|---|---|---|
| 1 | 工作区成员资格（`workspace-core.Authorize`） | `RefusalStatus` / `RefusalBody`，与「不存在」同形 |
| 2 | 主体存在且属本工作区（作品 / 文档 / 版本 / 请求 / 任务） | 同上 |
| 3 | 受控集校验（`status` 三套、`channel`、`handoff_method`、`version_match`、`subject_kind`） | 400 诊断错误对象，**点名字段** |
| 4 | 类型与前置条件（`kind == channel_draft`；`ready` 及之后要求引用的请求 `approved`） | 400，**点名**是哪一条 |
| 5 | 必填项（`scheduled_at` / `handoff_method` / `reason` / 三项条件必填的发布字段） | 400，**点名缺的那一项** |
| 6 | 状态迁移合法性 | 400，说明「从 X 不能到 Y」 |

**1–2 的拒绝体逐字节相同**（`trace_id` 除外）。3–6 与 1–2 不同形是故意的：那是调用方自己输入的问题，说清楚不泄露别人的任何东西。

**条件必填不写成列约束**：`scheduled_at` 在 `scheduled` 时必填、在 `draft` 时应为空，同一列在不同状态下规则不同；更要紧的是 `CHECK` 给不出「你缺的是计划时间」这句话，而 SC-004 要的就是这句话。

---

## 5. 不可变与不外发的自证

- 发布记录与迁移记录：**没有 UPDATE，没有 DELETE**（工作区删除链除外），守卫扫查询文件（022 `TestBriefStoreHasNoUpdateOrDeletePath` 的形状），**且必须有 `INSERT INTO`**，否则守卫在空模块上也绿；
- 交付快照：一条用例，处置前后 `snapshot` 列逐字节相同；
- **不外发**：模块源码里不存在 HTTP 客户端调用，守卫扫源码（`check-diagnostics-no-upload.mjs` 的形状）；一次可编译变异确认会红；
- **不存密钥、不提供发布接口**（§9.2 原文）：两条检索守卫；
- **不调度**：代码里不存在读取 `scheduled_at` 去执行的路径，一条检索用例（SC-010）；「到期」只出现在读路径的比较里；
- **交接 ≠ 发布**：三种交接方式各推一次，发布记录数仍为 0、读路径不显示「已发布」（SC-012）。

---

## 6. 迁移

四张表 + 每表的稳定 id 唯一索引 + 每表的工作区查询索引 = **十二个文件**，每个单条语句。建表迁移**不得**含 `PRIMARY KEY` / `UNIQUE`（R5）；每个建索引的 up 迁移登记进 `cmd/migrate` 的 `concurrentIndexCleanups`（R6），**四个建表迁移不登记**；四张表都进 `workspace_delete_manifest_test.go` 并在删除链同一 CTE 里各有一条 `DELETE`。

`cmd/migrate/main.go` 与 `cmd/server/router.go` 都是上游文件 → 索引登记与路由挂载放进**同一个 `upstream:` 提交**，PR 正文单列「上游改动」一节（第 13 步）。
