# 合同：审核请求、交付任务与发布记录（025，人工切片）

**状态**：**待裁决**（五条 clarify 见 `spec.md` 文末）。本合同按暂定推荐值写：Q1=A、Q2=A、Q3=A、Q4=A、Q5 暂定枚举。**裁决后回写本文件。**

**§7.1 三行原文**（主控摘录，Issue #142）：

> 审核请求 `pending → changes_requested / approved / rejected / cancelled`（审核绑定不可变版本和交付快照）
>
> 交付任务 `draft → ready → scheduled / handed_off；可 cancelled / held`（scheduled 为人工待办计划时间，handed_off 表示已导出或交给运营者）
>
> 发布记录 `reported_published / verified_published / failed / removed / unknown`（每条带声明者、证据和核验方式）

---

## 1. 四张表

迁移号**在实施时按当时的最大值顺延**（`specs/024` 的提案占用 494–503，尚未合入）。下文用 `N`、`N+1`… 指代。

### `content_review_request` —— 审核请求（`status` 可变，Q3=A）

| 列 | 说明 |
|---|---|
| `review_request_id` | 被引用的稳定键。唯一性走并发索引，**不是** `PRIMARY KEY`（R5） |
| `workspace_id` | 一切查询按它过滤；删除按它删 |
| `work_id` / `artifact_id` | 字符串，**不 import `work-editor` 的包**（登记表里有这条依赖，但存 id 更省；与 024 对 `topic_card_id` 的处理同口径） |
| `version_id` | **被冻结的那一版**。work-editor 的版本只插不改，引用它即等于冻结 |
| `channel` | 受控集（Q4=A） |
| `snapshot` | `jsonb NOT NULL`，**不可变的交付快照**（Q1=A，字段见 §3） |
| `status` | 受控集五值，初始 `pending` |
| `requested_by` / `requested_at` | 谁提交的、何时 |
| `decided_by` / `decided_at` | 处置人与时间；未处置时为 `''` / 零值 |
| `decision_note` | 审核意见，自由文本 |
| `created_at` / `updated_at` | |

**没有「当前版本」指针。** 绑定的是 `version_id`，作者之后存多少版都够不到它。

### `content_review_transition` —— 状态迁移记录（**只插不改不删**，Q3=A）

| 列 | 说明 |
|---|---|
| `transition_id` | 稳定键 |
| `workspace_id` | |
| `subject_kind` | 受控集 `review_request` / `delivery_task`。**一张表记两类主体**，因为两者的迁移形状完全相同，分两张表只会让「列出这件事的全部动作」要查两次 |
| `subject_id` | 审核请求或交付任务的 id |
| `from_status` / `to_status` | 迁移前后。`from_status` 为 `''` 表示创建 |
| `reason` | `held` 的原因、处置意见的摘要等；自由文本，可空串 |
| `actor_id` / `created_at` | |

**为什么迁移记录不靠审计事件**：审计是诊断读模型——它会过 `Sanitize`、会按保留期清理、component 白名单里还没有内容模块名（#108 之后才登记）。产品历史（「谁在什么时候把它 held 了，为什么」）要长期、逐字保留，两件事不该共用一条管道。

### `content_delivery_task` —— 交付任务（`status` 可变）

| 列 | 说明 |
|---|---|
| `delivery_task_id` | 稳定键 |
| `workspace_id` / `work_id` / `artifact_id` | |
| `review_request_id` | **可空串**（Q2=A）：`draft` 阶段允许还没有审核请求 |
| `channel` | 受控集，与审核请求同一份 |
| `status` | 受控集六值，初始 `draft` |
| `scheduled_at` | `scheduled` 时必填；**人看的待办时间**，没有任何代码读它去执行 |
| `handoff_method` | `handed_off` 时必填，受控集 `export` / `handed_to_operator` |
| `hold_reason` | `held` 时必填 |
| `created_at` / `updated_at` | |

### `content_publication_record` —— 发布记录（**只插不改不删**）

| 列 | 说明 |
|---|---|
| `publication_record_id` | 稳定键 |
| `workspace_id` / `work_id` / `artifact_id` | |
| `delivery_task_id` | **可空串**：补记历史时可能没有任务 |
| `channel` | 受控集 |
| `status` | 受控集五值 |
| `claimed_by` | **声明者**，受控集（Q5 暂定） |
| `evidence` | **证据**，自由文本，**必填非空** |
| `verification` | **核验方式**，受控集（Q5 暂定） |
| `actor_id` / `created_at` | 谁录的、何时 |

**没有「当前发布状态」列。** 当前状态是**最新一条记录**（FR-019）。存一份可变的当前状态就是第二份真相，而它一定会在某次并发录入后与记录流不一致。

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
draft     → ready | cancelled
ready     → scheduled | handed_off | held | cancelled
scheduled → handed_off | held | cancelled
held      → ready | cancelled
handed_off → (无出边)
cancelled  → (无出边)
```

`held` 从哪里来、回到哪里去：§7.1 原文只说「可 cancelled / held」，没说它从哪个状态进。上面这张图是**本合同的补全**：从 `ready` 与 `scheduled` 进，回到 `ready`。**若裁决另有口径，改的是这张图与对应用例。**

### 发布记录

**没有状态机**——每条记录是一次独立的声明，五个状态都可以是任意一条记录的值。「只前进不删」（Issue 原话）在这里的落地是：**记录只插不改不删**，而不是「状态不能回退」。一条 `verified_published` 之后再来一条 `removed` 是正常的（作品被平台删了），拦住它反而会让人无法如实记录。

---

## 3. 交付快照（Q1=A，字段待 §8 原文确认）

提交审核时冻结，写进 `content_review_request.snapshot`：

| 键 | 记什么 |
|---|---|
| `channel` | 提交那一刻选的渠道 |
| `version_id` | 被审版本（与列冗余，使快照单独拿出来也自洽） |
| `artifact_kind` | 被审文档的类型（`body` / `channel_draft`） |
| `work_id` / `artifact_id` | |
| `start_snapshot_id` | 若该作品挂在一次「开始」上则记它，否则 `''`；**空串是真实状态** |
| `submitted_at` | 服务端时间 |

**这份 jsonb 不可变**：它所在的行虽然 `status` 可变，但**没有任何写路径会改 `snapshot` 列**，且有一条用例断言处置前后它逐字节相同。

---

## 4. 端点与决策顺序

| 方法 | 路径 | 用途 |
|---|---|---|
| `POST` | `/api/content-reviews` | 提交审核（body 带 `artifact_id` / `version_id` / `channel`） |
| `GET` | `/api/content-reviews` | 列出，可按 `artifact_id` / `status` 筛 |
| `GET` | `/api/content-reviews/{reviewId}` | 单条 |
| `POST` | `/api/content-reviews/{reviewId}/decision` | 处置（`approved` / `changes_requested` / `rejected` / `cancelled` + 意见） |
| `POST` | `/api/content-deliveries` | 建交付任务 |
| `GET` | `/api/content-deliveries` | 列出 |
| `POST` | `/api/content-deliveries/{deliveryId}/status` | 推进状态（带 `scheduled_at` / `handoff_method` / `hold_reason`） |
| `POST` | `/api/content-publications` | 录一条发布记录 |
| `GET` | `/api/content-publications` | 列出，可按 `artifact_id` 筛 |

决策顺序，**第一个不通过者即拒绝**：

| 序 | 检查 | 失败时 |
|---|---|---|
| 1 | 工作区成员资格（`workspace-core.Authorize`） | `RefusalStatus` / `RefusalBody`，与「不存在」同形 |
| 2 | 主体存在且属本工作区（作品 / 文档 / 版本 / 请求 / 任务） | 同上 |
| 3 | 受控集校验（状态、渠道、交接方式、声明者、核验方式） | 400 诊断错误对象，**点名字段** |
| 4 | 必填项（`scheduled_at` / `handoff_method` / `hold_reason` / 三项发布必填） | 400，**点名缺的那一项** |
| 5 | 状态迁移合法性 | 400，说明「从 X 不能到 Y」 |

**1–2 的拒绝体逐字节相同**（`trace_id` 除外）。3–5 与 1–2 不同形是故意的：那是调用方自己输入的问题，说清楚不泄露别人的任何东西。

---

## 5. 不可变与不外发的自证

- 发布记录与迁移记录：**没有 UPDATE，没有 DELETE**（工作区删除链除外），守卫扫查询文件（022 `TestBriefStoreHasNoUpdateOrDeletePath` 的形状）；
- 交付快照：有一条用例，处置前后 `snapshot` 列逐字节相同；
- **不外发**：模块源码里不存在 HTTP 客户端调用，守卫扫源码（`check-diagnostics-no-upload.mjs` 的形状）；一次可编译变异确认会红；
- **不调度**：代码里不存在读取 `scheduled_at` 去执行的路径，一条检索用例（SC-010）。

---

## 6. 迁移

四张表 + 每表的稳定 id 唯一索引 + 每表的工作区查询索引 ≈ **十二个文件**，每个单条语句。建表迁移**不得**含 `PRIMARY KEY` / `UNIQUE`（R5）；每个建索引的 up 迁移登记进 `cmd/migrate` 的 `concurrentIndexCleanups`（R6），**建表迁移不登记**；四张表都进 `workspace_delete_manifest_test.go` 并在删除链同一 CTE 里各有一条 `DELETE`。

`cmd/migrate/main.go` 与 `cmd/server/router.go` 都是上游文件 → 索引登记与路由挂载放进**同一个 `upstream:` 提交**，PR 正文单列「上游改动」一节（第 13 步）。
