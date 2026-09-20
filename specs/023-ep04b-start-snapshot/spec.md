# Feature Specification: 开始界面与输入快照（EP-04b）

**Feature Branch**: `claude/spec-023-ep04b-start-snapshot`

**Created**: 2026-09-20

**Status**: Draft（三个 clarify 待裁决，见文末）

**Input**: EP-04 拆分方案的第二切片，见 [../022-ep04-topic-brief/ep04-breakdown.md](../022-ep04-topic-brief/ep04-breakdown.md) 的 EP-04b 卡。Issue #127。

**SOP 对应**：`docs/01-完整工作流.md` §5.3「冻结创作简报」之后、正式产出之前的那一步——把**这一次**要用的配置固定下来，使后续修改不影响已经开始的工作。

**模块**：`topic-planning`（快照装配点与端点），消费 `ip-profile` 与 `workspace-core`。**不新建模块。**

---

## Current State（以代码为准）

全部核实自 `app-main` @ `24ee6ca`。

### 1. 就绪判定已经落地，而且是四项（specs/021）

`server/internal/content/ip-profile/profile.go:242` 的 `ProfileReadiness` 与 `packages/core/content/ip-profile/profile.ts` 的 `profileReadiness` 给出同一个判定，由 `specs/021-account-expression-profile/contracts/readiness-parity.json` 的 16 条矩阵两侧同时断言。

**「最小可开始条件」就是这四项，且只计 `status = confirmed`**：

| 缺失键 | 含义 |
|---|---|
| `audience` | 目标受众 |
| `content_pillars` | 内容方向 |
| `primary_channels` | 至少一个渠道 |
| `weekly_hours` | 每周投入 > 0 |

读取入口是 `GET /api/content-accounts/{id}/profile`，响应里 `readiness` 是**服务端算好的**（`{can_start, missing[]}`），`uses_neutral_expression` 是另一个独立判定。

**本卡不重新定义「最小」，也不新增第五项。** 拆分卡当时说「『最小』包含哪几项由那份规格定」——现在定完了。

### 2. 选题卡与简报版本已经落地（specs/022）

- `content_topic_card`（迁移 483）、`content_brief_revision`（485），两者的 CONCURRENTLY 唯一索引各自独立迁移。
- 端点：`GET/POST /api/content-topics`、`GET /api/content-topics/{id}`、`POST /api/content-topics/{id}/actions`、`GET/POST /api/content-topics/{id}/briefs`、`GET /api/content-topics/{id}/briefs/{revisionId}`。
- **`action = start` 已经被占用**：它是 EP-04a 的「接受这个选题并冻结首版简报」，把卡的状态改成 `started` 并写 `started_brief_revision_id`。
- 简报**只插不改**；`brief_revision_id` 是被引用的稳定键，`revision` 只是给人读的计数器。

**两个必须注意的既有事实**：

1. **`TopicCard.AccountID` 是 `*string`，可以为空。** 就绪判定是**按账号**算的，所以一张没有账号的选题卡无法判定就绪——开始界面必须让人选一个账号，或拒绝开始。
2. **`BriefRevision.SourceScope` 已经存在，而且是自由文本**（`store.go` 没有对它做受控集校验）。它是 §5.3 十一项里的「资料范围」散文，**不是** LT-014 那个 `local/web/all` 的受控偏好。两者同名不同物，本卡不得把它们混为一谈。

### 3. 资料范围偏好已经落地（LT-014 / specs/018）

`server/internal/content/ip-profile/scope.go`：

- 受控集 `local` / `web` / `all`，**精确匹配**，不 trim 不折大小写；
- 存在账号 `settings["loretide.scope"]`，**不是列**；
- 读不到时填默认 `all`，且**读不写库**；
- 写入端点是独立的 `PUT /api/content-accounts/{id}/scope`（因为 `PATCH` 会整体替换 settings）。

注释里已经写明了它与快照的关系：**「这是『上次选了什么』，不是配置版本；每次运行都已经把它看到的偏好记在自己的快照里（`diagnostics.Snapshot.Preference`）」**。

### 4. 品牌预检开关已经落地（LT-015 / specs/019）

工作区 `settings["loretide.auto_precheck"]`（`server/internal/handler/workspace.go:280`），读时默认 `true`，**品牌级统一、无账号级覆盖**。它是配置；**触发预检是 EP-06**，不在本卡。

### 5. Grant 合同已经落地，但**没有任何地方存 Grant**（LT-016 / specs/020）

`workspace-core/grant.go` 的 `CanRead(ctx, recorder, ReadRequest)` 是**纯函数**：`Grants []Grant` 由调用方传入。仓库里没有 grant 表、没有 grant 端点，注释也写明「adding a third kind needs no migration, because grants are not stored yet」。

**后果**：`Snapshot.Grants`（`[]string`，授权 id 列表）今天**没有真实来源**。

### 6. 快照形状已经定死，但它属于自检运行

`diagnostics/contract.go:51` 的 `Snapshot` 十六个字段：

```text
config_version  persona_ref     sop_version   skill_version  rule_version
executor        executor_version
source_scope    saved_preference
required_sources  excluded_sources  grants
file_hashes     temperature     budget        timeout_ms
```

`Scope`(`source_scope`) 与 `Preference`(`saved_preference`) 是**两个独立字段**（specs/018 已确认），本卡不得再造第二套。

但承载它的 `Run`（`contract.go:59`）带着 `scenario` / `seed` / `expected_code` / `regression` / `is_test`——**那是诊断自检的模拟运行**，不是内容生产运行。`content_diagnostic_run` 表同理。

**并且：内容生产运行本身还没有主人。** `agent-workflow` 模块在 `scripts/content-boundaries.json` 里登记了但目录不存在（`pnpm check:diagnostics-contract` 报 `checked 4 landed modules; skipped 8 not yet landed`）。所以本卡固定下来的快照，今天**没有运行实体来引用它**——这正是 EP-04a 已经记过的那种「结构保证，行为断言留 agent-workflow 落地时补」。

### 7. 执行器禁用（宪法 IX）

`Snapshot.Executor` / `ExecutorVersion` 今天只能记「禁用」这件事本身。本卡不调模型、不起执行器。

### 8. 工作流硬约束（已在别处钉住，本卡照做）

- 迁移：无外键、无级联；索引一律 `CREATE INDEX CONCURRENTLY` 且**单文件单语句**；新迁移必须登记进 `cmd/migrate` 的 `concurrentIndexCleanups`（#122 的 **R6** 会红）；新表必须进工作区删除清单（`workspace_delete_manifest_test.go`）。
- 写路径必须在同一事务里持工作区删除栅栏（#104 / `LockWorkspaceForContentDiagnosticWrite`）。
- 越权一律走 `workspace-core` 的 `RefusalStatus` / `RefusalBody`，与「不存在」同形。
- 带路径参数的端点按工作流第 12 步：**必须有一条穿过真实中间件、参数值 ≠ 上下文值**的用例。
- 接入合同三条（`pnpm check:content-boundaries` + `pnpm check:diagnostics-contract`）。

---

## User Scenarios & Testing *(mandatory)*

### User Story 1 - 看得见「还差什么」才点得动「开始」 (Priority: P1)

小张在某个品牌下建好了选题卡、冻结了简报，现在要真正开始做这一篇。他打开开始界面，界面告诉他这个账号的表达配置还差「内容方向」和「每周投入」两项，并且「开始」是不可点的。他去账号页补齐并确认，回来「开始」就可点了。

**Why this priority**：这是 EP-04b 唯一一条能独立交付价值的功能——没有它，人只能在看不到原因的情况下被拒绝。其余两条都建立在它之上。

**Independent Test**：只实现这一条即可验收：造一个缺项的账号 → 开始不可点且列出缺项；补齐并确认 → 可点。无需快照落库。

**Acceptance Scenarios**：

1. **Given** 账号的四项最小条件一项未确认，**When** 打开开始界面，**Then** 「开始」不可点，并**逐项列出缺哪几项**（用字段名，不是一句笼统的「配置不完整」）。
2. **Given** 某字段已填但状态仍是 `pending`，**When** 打开开始界面，**Then** 它仍算缺失——**填了不等于确认**。
3. **Given** 四项全部 `confirmed`，**When** 打开开始界面，**Then** 「开始」可点。
4. **Given** 选题卡没有关联账号，**When** 打开开始界面，**Then** 界面要求先选一个账号，**不**用任意账号替它作答。
5. **Given** 界面判定可开始、但在点「开始」的瞬间账号已被别人改回 `pending`，**When** 提交，**Then** 服务端拒绝并说明缺项——**前端的判定不是授权**。

---

### User Story 2 - 开始后配置被固定，事后改不动它 (Priority: P2)

小张选定简报版本、确认这次的资料范围、（可选地）挂上项目，点「开始」。系统把这一刻的配置固定成一份输入快照。第二天他改了账号的资料范围偏好、又改了简报，昨天那次开始的快照**一个字都没变**。

**Why this priority**：这是 EP-04b 的核心交付，但没有 Story 1 它无从触发。

**Independent Test**：开始一次 → 读回快照 → 改配置与简报 → 再读回同一份快照，逐字段比对不变。

**Acceptance Scenarios**：

1. **Given** 已可开始，**When** 点「开始」，**Then** 产生一份输入快照，字段与 `diagnostics.Snapshot` 对齐（不新造字段名）。
2. **Given** 已产生快照，**When** 改账号的资料范围偏好，**Then** 快照里的 `saved_preference` 与 `source_scope` 不变。
3. **Given** 已产生快照，**When** 给同一张卡追加新的简报版本，**Then** 快照仍指向它开始时钉的那一版 `brief_revision_id`。
4. **Given** 已产生快照，**When** 改账号的表达配置（产生新的账号版本），**Then** 快照里的 `persona_ref` 不变。
5. **Given** 快照已存在，**When** 任何人通过任何端点尝试修改它，**Then** 不存在这样的端点——快照**只插不改**。

---

### User Story 3 - 非法依赖被拒，可选的东西留空也能开始 (Priority: P3)

小张试着用 A 品牌的简报版本在 B 品牌开始；试着用一个不存在的版本号开始；试着不填项目就开始。前两者被拒，第三者成功。

**Why this priority**：边界正确性。它不产生新价值，但它是「快照可信」的前提——一份引用了别的品牌的快照比没有快照更糟。

**Independent Test**：三组请求，两拒一成。

**Acceptance Scenarios**：

1. **Given** 简报版本属于 A 品牌，**When** 在 B 品牌请求用它开始，**Then** 返回与「不存在」完全同形的拒绝（经 `workspace-core`）。
2. **Given** 账号属于 A 品牌、简报属于 A 品牌但两者分属不同账号，**When** 请求开始，**Then** 拒绝并说明是依赖不匹配。
3. **Given** `brief_revision_id` 不存在，**When** 请求开始，**Then** 拒绝，**且响应不透露该 id 是否在别的品牌存在**。
4. **Given** 项目字段留空，**When** 请求开始，**Then** 成功——项目是可选的。
5. **Given** 简报版本已被后续版本取代，**When** 明确指定用旧版本开始，**Then** 成功——**能开始的是任何一个存在的版本，不只是最新版**。

---

### Edge Cases

- **同一份简报版本被开始两次**会怎样？见 **Q1**——这是本规格未定的基数问题。
- **账号在「开始」与「写快照」之间被删**：写路径持删除栅栏，整个开始操作失败，不留半份快照。
- **工作区在开始过程中被删**：同上，按 #104 的栅栏，拒绝与「工作区不存在」同形。
- **`required_sources` / `excluded_sources` / `grants` 今天无来源**：见 **Q2**。
- **预检开关关闭时**：本卡仍照常开始，只把开关当时的值记进快照；**触发或跳过预检是 EP-06 的事**，本卡不据此拒绝。
- **中性表达**：`uses_neutral_expression` 为真**不**阻止开始（021 已定它是标记不是门槛），但它应当被记下来，否则事后无法解释成稿为什么是中性口吻。
- **时钟**：快照的时间来自服务端，不接受客户端传入的时间。

---

## Requirements *(mandatory)*

### Functional Requirements

**就绪判定（消费 021，不重新定义）**

- **FR-001**：系统 MUST 在开始界面展示所选账号的最小可开始条件判定，缺项 MUST **逐项列出**，用 021 已有的四个字段名。
- **FR-002**：系统 MUST 以 `status = confirmed` 为唯一口径；已填未确认 MUST 算缺失。
- **FR-003**：系统 MUST NOT 在本卡新增、删除或改写最小条件的任何一项。
- **FR-004**：服务端 MUST 在写快照之前**重新判定一次**就绪；前端的判定只决定按钮可点性，不决定授权。
- **FR-005**：选题卡未关联账号时，系统 MUST 要求先选定账号，且 MUST NOT 用任意账号或品牌默认账号代答。

**选定简报版本（消费 022）**

- **FR-006**：系统 MUST 允许选定该选题卡下**任意一个已存在的**简报版本开始，不限于最新版。
- **FR-007**：系统 MUST 用 `brief_revision_id` 钉住版本，MUST NOT 用 `revision` 计数器。
- **FR-008**：系统 MUST 拒绝跨品牌的简报版本，拒绝形态 MUST 与「不存在」完全同形。

**资料范围（消费 LT-014）**

- **FR-009**：开始界面的资料范围 MUST 以账号已保存的偏好为初值（读不到时为 `all`）。
- **FR-010**：本次开始的实际选择与账号已保存的偏好 MUST 各自独立记入快照（`source_scope` 与 `saved_preference`），MUST NOT 只记一个。
- **FR-011**：资料范围的取值 MUST 限于 `local` / `web` / `all`，精确匹配。
- **FR-012**：系统 MUST 把本次选择写回账号偏好（「保存上次选择」），且该写入 MUST 走 LT-014 既有的独立端点语义，MUST NOT 整体替换 settings。

**输入快照**

- **FR-013**：「开始」成功 MUST 产生恰好一份输入快照，字段 MUST 与 `diagnostics.Snapshot` 的十六个字段逐名对齐，MUST NOT 新造同义字段。
- **FR-014**：快照 MUST 不可改写。口径要精确，因为 Q1=A 下它不是「只插」：没有更新端点、没有删除端点，写入路径只有「开始」一条，且该写入 MUST 只能把空快照变成非空快照，MUST NOT 改写一份已存在的快照。（Q1=B 下这条退化为字面意义的「只插不改」。）
- **FR-015**：快照 MUST 记下开始那一刻的：所钉简报版本、账号表达配置版本（`persona_ref`）、资料范围与账号偏好、品牌预检开关值、是否中性表达、执行器状态。
- **FR-015a**：品牌预检开关与中性表达在 `diagnostics.Snapshot` 的十六项里**没有对应字段**。它们 MUST 作为 `topic-planning` 的**扩展键**显式标注，MUST NOT 挤进任何一个既有字段——十六项各有含义，借用其中任何一个都会让「与 Snapshot 对齐」这句话变成半真。
- **FR-016**：事后修改账号配置、账号偏好、简报或品牌开关 MUST NOT 改变任何已存在的快照。
- **FR-017**：快照的时间戳 MUST 由服务端生成。
- **FR-018**：项目 MUST 可选；留空 MUST 能成功开始。

**授权与边界**

- **FR-019**：所有读写 MUST 按 `workspace_id` 过滤；越权 MUST 经 `workspace-core` 产出与「不存在」同形的拒绝。
- **FR-020**：本卡 MUST NOT 调用模型、MUST NOT 起真实执行器（宪法 IX）。
- **FR-021**：本地文件、素材包、知识卡的输入半边 MUST 呈现为明确的「暂不可用」并说明缺什么，MUST NOT 是空白、加载中或伪造内容。
- **FR-022**：**九个今天没有真实来源的字段** MUST 为空，且 MUST **各有一条**负例钉住「不得伪造」——`config_version`、`sop_version`、`skill_version`、`rule_version`、`executor_version`、`required_sources`、`excluded_sources`、`grants`、`file_hashes`。笼统一条「都为空」在有人填上其中一个时虽然也会红，但不会说是哪一个，而这九个各有各的接入方（EP-04d / W-03 / EP-08）。

**工程约束**

- **FR-023**：若本卡引入新表或新列，迁移 MUST 无外键、无级联；索引 MUST `CONCURRENTLY` 且单文件单语句；MUST 登记进 `concurrentIndexCleanups`；新表 MUST 进工作区删除清单并有用例。
- **FR-024**：快照写入 MUST 与工作区删除栅栏在同一事务内。
- **FR-025**：交付 MUST 拆成「存储与接口 PR」与「页面 PR」两次。
- **FR-026**：界面 MUST 只复用既有 Multica 组件，MUST NOT 新增控件或改样式；四语言 MUST 齐。
- **FR-027**：MUST NOT 写 UI 单测；界面项 MUST 进 `manual-ui-todo.md`。
- **FR-028**：每个带路径参数的新端点 MUST 有一条穿过真实中间件、参数值 ≠ 上下文值的用例。

### Key Entities

- **输入快照（Input Snapshot）**：一次「开始」所固定的全部配置。字段与 `diagnostics.Snapshot` 对齐。引用一个 `brief_revision_id`、一个账号、一个工作区，可选地引用一个项目。**只插不改。** 存放位置与基数见 **Q1**。
- **简报版本（既有）**：`content_brief_revision`，append-only，`brief_revision_id` 是被快照引用的稳定键。
- **选题卡（既有）**：`content_topic_card`，`account_id` 可空。
- **账号表达配置（既有）**：随 `content_account_revision` 走，`revision_id` 是 `persona_ref` 的来源。
- **账号资料范围偏好（既有）**：`settings["loretide.scope"]`，非版本化。
- **品牌预检开关（既有）**：工作区 `settings["loretide.auto_precheck"]`，非版本化。

---

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**：四项最小条件的**每一项**单独缺失时，界面都能点名它；四项全齐时「开始」可点。共 5 种组合各有一条用例。
- **SC-002**：开始一次后，改账号配置、改账号偏好、追加简报版本、改品牌开关四件事**各做一遍**，再读回快照，**十六个对齐字段与两个扩展键逐字节不变**。四件事各有一条用例，不合并成一条。
- **SC-003**：跨品牌简报版本、不存在的版本 id、非本账号的简报三种非法依赖，各返回与「不存在」同形的拒绝；三者的响应体**互相逐字节相同**。
- **SC-004**：项目留空的开始成功率 100%；不存在任何把项目当必填的路径。
- **SC-005**：`required_sources` / `excluded_sources` / `grants` 在本阶段的所有响应里**恒为空数组**，有一条用例在它们非空时变红。
- **SC-006**：「暂不可用」的那半边在界面上**永远显示原因**，且不存在任何伪造的素材/知识卡条目。
- **SC-007**：服务端就绪复核有效：构造「前端判定可开始、提交时已不可开始」的时序，服务端拒绝。
- **SC-008**：接入合同三条全绿；`concurrentIndexCleanups` 的 R6 与工作区删除清单用例全绿。

---

## Assumptions

1. **「最小可开始条件」就是 021 的四项**，本卡不扩展。拆分卡当时的悬置已由 `specs/021` 落定。
2. **「开始」在 EP-04b 的语义与 EP-04a 的 `action=start` 不是同一件事**：前者是「用这一版简报开一次工」，后者是「接受这个选题并冻结首版简报」。本卡不改写 EP-04a 的动作集。
3. **今天没有内容生产运行实体**（`agent-workflow` 未落地），所以本卡交付的是「一份能被将来的运行引用的、不可变的快照」，**不是**「运行确实读到了它」。后者的行为断言留给 agent-workflow——与 EP-04a 对 §5.3 的处理口径一致（结构保证，行为断言后补）。
4. **`Snapshot.Executor` / `ExecutorVersion` 记的是「禁用」这一事实本身**，不是某个真实执行器的版本。
5. **预检开关只被记录，不被执行**。触发在 EP-06。
6. **`sop_version` / `skill_version` / `rule_version` / `config_version` 今天没有版本源**，与 `grants` 同类处理：记空并有负例钉住不得伪造，留给后续卡接入。
7. 界面文案与版式沿用账号页与选题页既有的 Settings 版式，不引入第二套视觉语言。

---

## Out of Scope

- 候选选题自动生成、每日建议（EP-04c，等 EP-08 + W-03）。
- 必用 / 排除资料的**选取**（EP-04d，等 LT-023 检索）；本卡只保证快照里有它们的位置且今天为空。
- 预检的触发与执行（EP-06）。
- 内容生产运行的生命周期、重试、取消（agent-workflow）。
- 本地文件上传与哈希（`file_hashes` 今天为空）。
- Grant 的存储与颁发（LT-016 只交付了判定纯函数）。

---

## 待裁决（clarify）

三条都给了推荐值，**不阻塞**后续 plan；主任务裁决后回写本节与相应 FR。

### Q1：输入快照存在哪里，一份简报版本能开始几次？

**Context**：拆分卡写的是「**无新表**。一页界面加一个快照装配点」。但 `diagnostics` 的 `Run`/`content_diagnostic_run` 带着 `scenario` / `seed` / `expected_code` / `regression` / `is_test`，那是自检模拟运行，不是内容生产运行；而内容生产运行的主人 `agent-workflow` 还没落地。

**What we need to know**：快照落在哪个载体上，以及「同一简报版本能否被开始多次」。

| 选项 | 做法 | 含义 |
|---|---|---|
| **A（推荐）** | 给 `content_brief_revision` 加 `snapshot jsonb` 列（照抄迁移 482 给 `content_account_revision` 加 `profile jsonb` 的做法）；**一个简报版本最多开始一次** | 真正「无新表」；append-only 天然免疫事后修改，不需要第二套不可变保证；代价是**一份简报版本只能开一次工**，要再开一次必须先追加一个新版本（这也说得通：配置不同就是不同的一次）|
| B | 新建 `content_start_snapshot` 表，一个简报版本可有多份快照 | 与拆分卡的「无新表」冲突；换来的是「同一份简报可以用不同配置开多次」；要自己保证不可变（无 UPDATE/DELETE 路径 + 守卫用例）|
| C | 复用 `content_diagnostic_run` 加一个 kind | 把生产语义混进自检模块，`is_test` / `scenario` / `regression` 都会变成半真半假的字段；**不推荐** |

**倾向 A 的理由**：它同时满足「无新表」与「不可变」，而且把「再开一次 = 先定一版新简报」这条规则显性化——反过来说，如果产品上确实需要「同一版简报换个资料范围再跑一次」，那就只能选 B，请明示。

### Q2：`grants` / `required_sources` / `excluded_sources` / 四个 version 字段今天怎么填？

**Context**：LT-016 的 `CanRead` 是纯函数，**仓库里没有任何地方存 Grant**；必用/排除等 EP-04d + W-03；`sop_version` / `skill_version` / `rule_version` / `config_version` 今天没有版本源。

| 选项 | 做法 | 含义 |
|---|---|---|
| **A（推荐）** | 七个字段一律写空（`[]` / `""`），并各有一条**负例**钉住「本阶段必须为空」 | 诚实；接入方改动时用例会红，逼人显式改规则而不是悄悄填上 |
| B | 省略这些字段，等有来源时再加 | 与 `diagnostics.Snapshot` 不再对齐，拆分卡明写「不要再造第二套」 |
| C | 填入占位值（如 `"unavailable"`） | 占位值会被下游当成真值读，最差 |

### Q3：开始界面的「资料范围」以谁为准？

**Context**：三处同名不同物——`BriefRevision.SourceScope`（022，§5.3 的自由文本「资料范围」，无受控集校验）、账号偏好 `settings["loretide.scope"]`（LT-014，受控 `local/web/all`）、`Snapshot.Scope` + `Snapshot.Preference`（specs/018 已确认是**两个独立字段**）。

| 选项 | 做法 | 含义 |
|---|---|---|
| **A（推荐）** | 开始界面用**受控偏好**：初值取账号偏好，本次可改；`saved_preference` 记开始那一刻读到的账号偏好，`source_scope` 记本次实际生效的选择；成功后把本次选择写回账号偏好。简报的 `source_scope` 文本**不参与判定**，只作为简报内容随版本一起被钉住 | 与 specs/018「两个独立字段」逐字一致；「保存上次选择」有确定含义；简报文本与受控偏好不打架 |
| B | 以简报的 `source_scope` 文本为准 | 它是自由文本，无法映射到 `local/web/all`，会逼本卡给它补一个受控集——那是改 022 的契约 |
| C | 只用账号偏好，本次不可改 | 最简单，但「开始界面校验并确认配置」就名存实亡了 |

**注意**：无论选哪个，本卡都**不得**给 `BriefRevision.SourceScope` 加受控集校验——那会让 022 已存的自由文本数据变成非法。
