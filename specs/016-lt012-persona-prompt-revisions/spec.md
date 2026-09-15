# Feature Specification: 账号人设提示词的配置版本（LT-012）

**Feature Branch**: `claude/spec-016-lt012-persona-prompt-revisions`

**Created**: 2026-09-15

**Status**: Draft

**Input**: `tasks/todo.md` LT-012——「账号配置增加 persona_prompt，变更保存配置版本；提供给后续运行读取的确定快照」。

**Traces to**: R-006 / R-007；D11-V02。

**SOP**: §3.1「创作者确认后形成 AccountConfigRevision，未确认项保留为待补充」的**存储层**；LT-013 页面的前置。

## Current State（以代码为准，2026-09-15 于 `app-main` `8c4fe4f` 核实）

本卡的宿主是 **#71 刚落地的 `content_account`**（迁移 `477`/`478`，模块 `ip-profile`）：

| 列 | 类型 |
|---|---|
| `account_id` | `text` PK |
| `workspace_id` | `text` |
| `platform` | `text` + `CHECK`（8 值） |
| `display_name` | `text` |
| `settings` | `jsonb` DEFAULT `'{}'` |
| `created_at` / `updated_at` | `timestamptz` |

**没有 `persona_prompt`，也没有任何版本历史。** `settings` 的注释写着「JSON so the full SOP 3.1 field list can land later without another column」——本卡要决定 persona_prompt 是否走那条路。

模块形状（`ip-profile`）：`Store` 接口四个方法（Create/Get/List/Update）、`Patch` 三个可选字段、`Service` 在创建与更新时写审计事件。授权在 HTTP 层经 `workspacecore.Authorize`，拒绝用 `RefusalStatus`/`RefusalBody`（无权与不存在同为 404）。

**其它既有事实**：

- `isUniqueViolation(err)`（SQLSTATE `23505`）已存在于 `handler.go:802`——并发插入撞唯一约束时可据此重试。
- 工作区删除要改**两处**：`workspace_delete.sql`（真删）+ `workspace_delete_manifest_test.go`（登记）。`content_account` 已登记；**`content_dispatch_outbox` 仍未登记**（`#45` 遗留，`claude/fix-outbox-deletion-manifest` 分支在处理），所以清单漂移用例在基线上仍是红的——**与本卡无关**，但要能区分。
- 迁移最新为 `478`，本卡从 **479** 起。

## 存储形状的选择（research 里展开，此处给结论）

主任务倾向**独立的不可变版本表**。核实后**同意**，理由来自验收条款本身：

| 验收要求 | 独立版本表 | 加列 + `settings` 里存 jsonb 历史 |
|---|---|---|
| 旧版本可读且内容不变 | 追加写，历史**不可能**被改 | 历史是一个 jsonb 数组，一次读改写就能覆盖掉它 |
| 新运行才能采用新配置 | 运行钉住一个 `revision_id`，那一行永远是那个内容 | 要么钉快照副本，要么读到的是当时的数组状态——都更脆 |
| 并发两次更新版本号单调不重复 | `(account_id, revision)` 唯一约束**由数据库保证** | jsonb 的读-改-写会**丢写**，且无法用约束表达 |

**结论：独立版本表。** 不是因为更「规范」，而是因为另一条路在这三条上都要靠自觉。

## Clarifications

### Session 2026-09-15

三项已提交主任务裁决，**尚未回答**；下列 A 为暂定值，正文按暂定值写就，不阻塞后续阶段。

- **Q1 存储形状？** → **A（暂定）**：独立不可变版本表。
- **Q2 账号上要不要存「当前版本指针」？** → **A（暂定）**：**不存**；当前版本 = 该账号 `revision` 最大的那一行。
- **Q3 版本的对外标识用什么？** → **A（暂定）**：全局 `revision_id text` 主键 + `(account_id, revision)` 唯一。

## User Scenarios & Testing *(mandatory)*

### User Story 1 —— 确认一次人设提示词，就留下一个版本（P1）

创作者写好人设提示词并确认，系统存下一个版本。再改一次，再存一个版本，版本号递增。

**Why this priority**：这是本卡的全部内容，也是 SOP §3.1「创作者确认后形成 AccountConfigRevision」的落点。

**Independent Test**：连续两次设置提示词，断言产生两个版本，版本号为 1、2，内容各自正确。

**Acceptance Scenarios**：

1. **Given** 一个账号，**When** 首次设置提示词，**Then** 产生版本 1，内容为所设值。
2. **Given** 已有版本 1，**When** 再次设置，**Then** 产生版本 2，版本 1 **内容不变**。
3. **Given** 任一版本，**When** 读取，**Then** 返回该版本的提示词与创建时间。

---

### User Story 2 —— 空提示词是有效的，因为「待补充」也是一种状态（P1）

创作者确认时还没想好人设提示词，留空。这**不是**错误——SOP §3.1 明说「未确认项保留为待补充」。空提示词同样产生一个版本，表示「这一次确认过了，内容是空」。

**Why this priority**：任务卡验收第一条就是「空提示词有效」。把空当成非法会让创作者无法推进流程。

**Acceptance Scenarios**：

1. **Given** 提示词为空串，**When** 设置，**Then** 成功，产生一个版本，内容为空串。
2. **Given** 空版本已存在，**When** 读取，**Then** 返回空串——**不是** null、不是「未设置」、不是报错。
3. **Given** 空版本之后设置了非空值，**When** 读取旧版本，**Then** 仍为空串。

---

### User Story 3 —— 改 A 的人设不影响 B（P1）

同一品牌下两个账号各有自己的人设提示词与版本序列。改 A 的，B 的提示词与版本号都不动。

**Why this priority**：任务卡验收「改 A 不影响 B」。版本号**按账号独立计数**，不是全局序列——否则 B 的版本号会因为 A 的改动而跳号。

**Acceptance Scenarios**：

1. **Given** A 与 B 各有版本 1，**When** 改 A 三次，**Then** A 到版本 4，**B 仍在版本 1 且内容不变**。
2. **Given** 同上，**When** 列出 B 的版本，**Then** 只有 B 的，不含 A 的任何一行。

---

### User Story 4 —— 旧版本可读，新运行才采用新配置（P1）

一次运行开始时钉住当时的版本。之后创作者改了人设，**那次运行读到的仍是它钉住的版本**；只有新开的运行才用新版本。

**Why this priority**：任务卡验收「旧版本可读，新运行才能采用新配置」。这是「确定快照」的全部含义。

**Independent Test**：取当前版本 id，改人设，再用那个 id 读，断言内容是改之前的。

**Acceptance Scenarios**：

1. **Given** 运行钉住了版本 `r1`，**When** 创作者改成新内容产生 `r2`，**Then** 用 `r1` 读到的仍是旧内容。
2. **Given** 同上，**When** 查当前版本，**Then** 是 `r2`。
3. **Given** 任何已存在的版本，**When** 任何时候读它，**Then** 内容与写入时逐字节相同——**版本行不可更新**。

---

### User Story 5 —— 并发两次确认，版本号不重复也不跳错（P2）

两个人（或两个标签页）几乎同时确认同一个账号的人设。两次都成功，版本号是 1 和 2，**不会两行都叫 2**，也不会有一次被悄悄丢掉。

**Why this priority**：任务卡验证项点名「同一账号并发两次更新版本号单调不重复」。

**Acceptance Scenarios**：

1. **Given** 同一账号，**When** 并发发起两次设置，**Then** 产生两个版本，版本号为 1 与 2。
2. **Given** 同上，**When** 检查，**Then** 不存在两行同账号同版本号。

---

### Edge Cases

- 提示词非常长：设上限并在超限时返回 400，不落库（避免一行把审计与导出撑爆）。
- 提示词为纯空白：与空串**同等对待**（都是「待补充」），不做 trim 后拒绝——US2 的理由同样适用。
- 账号不存在或属于别的品牌：与「无权」同一响应（404），**不泄漏存在性**。
- 版本 id 格式非法：同上，404。
- 账号被工作区删除带走：其全部版本一并删除。

## Requirements *(mandatory)*

### Functional Requirements

**存储**

- **FR-001**: MUST 新增一张**不可变**的账号配置版本表，含：版本 id、账号、所属空间、版本号、人设提示词、创建时间。
- **FR-002**: 版本行 MUST NOT 被更新或删除（工作区/账号删除除外）；变更一律**追加新行**。
- **FR-003**: 版本号 MUST **按账号独立**递增，`(account_id, revision)` MUST 唯一。
- **FR-004**: MUST NOT 新增外键或级联；索引 MUST 用独立文件的 `CREATE INDEX CONCURRENTLY`，一文件一语句。
- **FR-005**: MUST NOT 新建独立人设表、人设库或绑定关系表——persona_prompt **只是账号配置的一个字段的版本历史**（W-02 边界、D11-V02）。

**行为**

- **FR-006**: 空提示词 MUST 有效并产生版本（「待补充」是合法状态）。
- **FR-007**: 读取任一历史版本 MUST 返回写入时的内容，逐字节相同。
- **FR-008**: 一个账号的变更 MUST NOT 影响另一个账号的内容或版本号。
- **FR-009**: 并发写入 MUST NOT 产生重复版本号，也 MUST NOT 静默丢写。
- **FR-010**: 提示词长度 MUST 有上限；超限返回 400 诊断错误对象且不落库。

**接口与授权**

- **FR-011**: MUST 提供：设置提示词（产生新版本）、读当前版本、读指定版本、列出某账号的版本。
- **FR-012**: 每个操作 MUST 经 `workspace-core` 授权助手判定；拒绝 MUST 用其规范映射（无权与不存在**同为 404**，body 无对象正文）。
- **FR-013**: 每次**写入**版本 MUST 经诊断记一条事件（接入合同 E2 的真实来源）。

**边界**

- **FR-014**: MUST NOT 设计任务级 persona 参数——人设挂在账号配置上，运行**引用一个版本**，不携带自己的 persona 覆盖值。
- **FR-015**: 新表 MUST 登记进工作区删除清单**并有用例**，且删除语句 MUST 真删。
- **FR-016**: MUST NOT 修改上游 Multica 代码与 `server/internal/daemon/`。
- **FR-017**: 本卡**无界面改动**；MUST NOT 新增 UI 单测。
- **FR-018**: `pnpm check:diagnostics-contract` MUST 仍报 **`checked 3 landed modules`**（本卡不新增模块目录，沿用 `ip-profile`）。

### Key Entities

- **账号配置版本（AccountConfigRevision）**：某账号在某一刻被确认的配置快照。本卡只含 `persona_prompt` 一个字段；后续字段落地时**加列或加 JSON，不改版本机制**。
- **当前版本**：该账号 `revision` 最大的那一行（Q2-A：不另存指针）。
- **运行引用**：运行钉住一个版本 id。**本卡不实现运行**，只保证版本可被稳定引用。

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: 连续两次设置产生版本 1、2；版本 1 内容**逐字节不变**。
- **SC-002**: 空提示词成功并产生版本；读回为空串而非 null 或错误。
- **SC-003**: 改 A 三次后 B 仍在版本 1 且内容不变。
- **SC-004**: 改动之后用旧版本 id 读到的仍是旧内容。
- **SC-005**: 并发两次设置产生版本号 1 与 2，无重复。
- **SC-006**: 跨品牌读写返回 404，与「不存在」**逐字节相同**。
- **SC-007**: 删除工作区后版本行为 **0**；清单漂移用例中新表不在 `unclassified`。
- **SC-008**: 五条核心负例各有一个用例且**先写并确认失败**；每条不变量各一处**可编译的**变异验证。
- **SC-009**: 新增 UI 单测 **0**；上游与 daemon 改动 **0**；外键与级联 **0**。

## UI Impact

**无。** 本卡是存储层，页面在 LT-013。未新增 UI 单测、未产生手动条目。

> SOP 阶段界面规则（整套继承上游 Multica 设计系统与设计 token，只把功能挂到既有组件上）**本卡无适用项**。

## Assumptions

- 本卡**只做 `persona_prompt` 一个字段**的版本化。SOP §3.1 的其余字段（受众、内容支柱、禁用表达……）落地时**加列或扩 JSON，版本机制不变**。
- 「未确认项保留为待补充」在本卡的体现就是**空提示词有效**：确认动作发生了、内容还空着，这是一个合法版本而不是错误。
- 版本表归 **`ip-profile`** 所有（与 `content_account` 同模块），不新增模块目录。
- **不实现运行**：本卡只保证版本可被稳定引用；哪个运行钉哪个版本由后续卡决定。
- 不设计任务级 persona 参数（FR-014）——否则「账号配置」就不再是唯一事实来源。
- 提示词上限暂定 **20000 字符**；若主任务有既定口径以其为准。

## 待澄清问题（已按推荐值暂定，不阻塞）

### Q1 独立版本表，还是加列 + jsonb 历史？

| 选项 | 做法 | 含义 |
|---|---|---|
| **A（推荐，已暂定）** | 独立不可变版本表 | 「旧版本不可改」「并发不丢写」「运行钉得住」三条**由数据库保证**而非靠自觉。代价：多一张表、多一次查询 |
| B | `content_account` 加 `persona_prompt` 列 + 历史存 `settings` jsonb | 不加表，但历史是一个可被整体覆盖的数组：读-改-写会丢写，唯一性无法用约束表达，「旧版本可读」变成一句承诺 |

### Q2 账号上要不要存「当前版本指针」？

| 选项 | 做法 | 含义 |
|---|---|---|
| **A（推荐，已暂定）** | 不存指针；当前 = 该账号 `revision` 最大的那行 | **不可能与真相不一致**，因为没有第二份真相。代价：读当前版本要一次 `ORDER BY revision DESC LIMIT 1`（有索引） |
| B | `content_account` 加 `current_revision_id` 列 | 读当前版本一次命中。代价：两处要同步；一旦漂移，账号会指向一个不是最新的版本，而这种 bug 很难被发现 |

### Q3 版本的对外标识用什么？

| 选项 | 做法 | 含义 |
|---|---|---|
| **A（推荐，已暂定）** | 全局 `revision_id text` 主键 + `(account_id, revision)` 唯一 | 运行只需钉一个 id，不必同时记账号；与既有 id 风格（`account_id text`）一致 |
| B | 复合键 `(account_id, revision)`，无全局 id | 少一列，但每个引用方都要存两列，且引用写法到处不同 |
