# Feature Specification: 品牌账号与平台配置（LT-011）

**Feature Branch**: `claude/spec-015-lt011-account-platform-config`

**Created**: 2026-09-15

**Status**: Draft

**Input**: `tasks/todo.md` LT-011——「实现品牌下 Account 与配置读写，保存平台、显示名及表达/内容形式设置；Account/ChannelAccount 是同一实体」。

**Traces to**: R-005 ～ R-008；D11-V02。

**SOP**: §3.1「账号表达配置」的**存储与接口层**；LT-012 / LT-013 的前置。

## Current State（以代码为准，2026-09-15 于 `app-main` `7bf0a62` 核实）

### 上游没有可复用的「品牌内容账号」

**全仓没有任何名为 `account` 的表。** 上游 Multica 有一组 `channel_*` 表，但那是**另一件事**：

| 上游 | 是什么 | 与本卡的关系 |
|---|---|---|
| `channel_installation` | 飞书 / Slack **机器人应用**的安装凭据（`app_id`、`app_secret_encrypted`、`bot_open_id`），挂在 `agent_id` 上 | **无关**。它是 IM 集成，不是品牌发内容的账号 |
| `channel_user_binding` | IM 用户 ↔ Multica 用户的绑定 | 无关 |
| `user` / `member` | **登录**身份与空间成员关系 | 无关。本卡的 Account 不是登录账号 |

**结论**：LT-011 的 Account 是**新实体**。规格里必须把这个区分写死，否则「channel account」这个词会被读成 `channel_installation`。

### 诊断层已经为它留好了位置

`content_diagnostic_run` / `content_operation_audit` / `content_technical_log` 三张表**都已经有 `account_id text NOT NULL DEFAULT ''` 列**（迁移 `468`）。LT-010 的 `diagnosticScope` 注释也写着「真实账号权限待账号域交付」，`Scope.Accounts []string` 目前恒为空。

**也就是说这个实体的形状早就被预留了**：账号 id 是 **text**，与空间 id 同样是 text（`workspace_id text`）。

### 授权入口已就绪（#69）

`content/workspace-core` 的 `Authorize(ctx, members, recorder, actor, workspace, roles...)` 已可用，返回类型化判定；`RefusalStatus` / `RefusalBody` 提供**新模块用的规范映射**——无权与不存在**同为 404**，响应体是诊断错误对象，不含对象正文。**本卡每个账号操作都必须经它判定。**

### 工作区删除清单是一张必须同步的清单

`server/internal/handler/workspace_delete_manifest_test.go` 有一张全表清单，新表未登记即 `unclassified` 失败；真正的删除在 `server/pkg/db/queries/workspace_delete.sql`。**#45 曾漏掉这件事**（`content_dispatch_outbox` 至今 `unclassified`，两次交付都撞到）。本卡**两处都要改**：登记 + 真删。

### 其它事实

- `content-boundaries.json` 注册 12 个模块，**没有 `account`**；与本域最接近的登记名是 **`ip-profile`**（依赖 `workspace-core` + `diagnostics`，正是本卡所需）。
- 最新迁移为 `476`，本卡从 **477** 起。
- 迁移文件风格：`CREATE TABLE IF NOT EXISTS`，列用 `text` / `jsonb` / `timestamptz`，**无外键**。

## Clarifications

### Session 2026-09-15

三项**均已裁决为 A**，与推荐值一致，正文无需回改。

- **Q1 落哪个登记模块？** → **A（已裁决）**：`ip-profile`。PR 正文引用**表所有权**口径；`docs/12` §2 在文档仓库、本检出没有，故按 `content-boundaries.json` 的登记与 `diagnostics-onboarding-contract` 的模块表口径写，并勾选 PR 模板的存储所有权项。
- **Q2 列类型？** → **A（已裁决）**：`text`，与 `content_diagnostic_run` 等同层表一致。
- **Q3 平台枚举在哪校验？** → **A（已裁决）**：Go 受控枚举**为准**（产出 400 诊断错误对象），库加 `CHECK` 兜底。**`CHECK` 不是外键/级联，允许**。Assumptions 写明「新增平台需要一次迁移」并列出首版枚举值。
- **补充**：新表 MUST 登记进工作区删除清单**并有用例**（FR-011、US4）。

## User Scenarios & Testing *(mandatory)*

### User Story 1 —— 一个品牌下可以有多个互不影响的账号（P1）

用户在品牌 A 下建两个账号：一个小红书、一个抖音，各有自己的显示名与表达设置。改其中一个，另一个不受影响。

**Why this priority**：这是任务卡验收第一条「同品牌多个账号独立」。账号是后续所有内容的归属单位，独立性错了后面全错。

**Independent Test**：建两个账号，改其中一个的显示名与表达设置，读另一个，断言逐字段未变。

**Acceptance Scenarios**：

1. **Given** 品牌 A，**When** 建两个不同平台的账号，**Then** 都创建成功，各有独立 id。
2. **Given** 两个账号，**When** 更新其一，**Then** 另一个的每个字段都未变。
3. **Given** 品牌 A 有账号，**When** 列出 A 的账号，**Then** 只返回 A 的，按稳定顺序。

---

### User Story 2 —— 跨品牌读写一律拒绝，且不告诉你它存在（P1）

品牌 B 的成员拿着品牌 A 的账号 id 来读或改，被拒绝；拒绝的样子与「这个账号根本不存在」**完全一样**。

**Why this priority**：任务卡验收「不能凭传入 workspace_id 越权；拒绝结果不泄漏对象正文」。这是 LT-010 helper 的第一个真实消费者，也是它是否真的有用的检验。

**Independent Test**：同一账号 id，分别用 A 成员与 B 成员请求，断言前者成功、后者 404；再用一个不存在的 id 请求，断言与后者响应逐字节相同。

**Acceptance Scenarios**：

1. **Given** 账号属于 A，**When** B 的成员读它，**Then** 404，body 不含显示名、平台或任何对象字段。
2. **Given** 同上，**When** B 的成员改它，**Then** 404 且 A 的数据未变。
3. **Given** 一个不存在的账号 id，**When** 请求，**Then** 与上面两条**同一响应**。
4. **Given** 请求带了别人的 `workspace_id`，**When** 处理，**Then** 按授权助手判定拒绝，不因为传了什么而放行。

---

### User Story 3 —— 非法平台被拒（P1）

平台取值是受控的。传一个不在集合内的值，创建或更新被拒，返回 400 诊断错误对象，且什么都没写进去。

**Why this priority**：任务卡验收「非法平台拒绝」。平台是后续投放与统计的分组键，脏值会一路传下去。

**Acceptance Scenarios**：

1. **Given** 平台传 `"myspace"`，**When** 创建，**Then** 400 诊断错误对象，账号未创建。
2. **Given** 已有账号，**When** 更新平台为非法值，**Then** 400，原平台不变。
3. **Given** 集合内的每一个平台，**When** 创建，**Then** 都成功。

---

### User Story 4 —— 删除工作区，账号随之消失（P2）

品牌被删除时，它下面的账号一并删除，不留孤儿行。

**Why this priority**：`#45` 漏过一次（`content_dispatch_outbox` 至今 `unclassified`），两次交付都撞到。这条不是新规则，是一条**已经被忘记过的规则**。

**Acceptance Scenarios**：

1. **Given** 品牌下有账号，**When** 删除该品牌，**Then** 账号行被删除。
2. **Given** 新表，**When** 跑删除清单漂移用例，**Then** 该表**已登记**，不出现在 `unclassified`。

---

### Edge Cases

- 显示名为空或全空白：拒绝，400。显示名是人识别账号的唯一凭据。
- 显示名重复：**允许**。同一品牌下两个同名不同平台的账号是常见的，唯一性由 id 承担。
- 表达/内容形式设置为空对象：允许——本卡只做最小切片，完整字段在 LT-012 之后。
- 账号 id 格式非法：与「无权 / 不存在」同一响应，不得回 400 暴露「格式对了但没权限」的差别。
- 同一品牌同一平台多个账号：**允许**（一个品牌可以有两个小红书号）。

## Requirements *(mandatory)*

### Functional Requirements

**存储**

- **FR-001**: MUST 新增一张账号表，含：id、所属空间、平台、显示名、表达/内容形式设置（JSON）、创建与更新时间。
- **FR-002**: MUST NOT 新增外键或级联；空间隔离靠**应用层 `workspace_id` 过滤 + 授权助手**。
- **FR-003**: 索引 MUST 用独立文件的 `CREATE INDEX CONCURRENTLY`，一个文件一条语句。
- **FR-004**: MUST NOT 新建独立人设表，MUST NOT 新建绑定关系表（W-02 边界）；表达配置挂在账号上。

**接口**

- **FR-005**: MUST 提供创建、读取（单个与列表）、更新三类操作。
- **FR-006**: 每个操作 MUST 经 `workspace-core` 的授权助手判定；拒绝 MUST 用它的规范映射——**无权与不存在同为 404**，body 为诊断错误对象，**不含对象正文**。
- **FR-007**: 列表 MUST 只返回当前空间的账号，且顺序稳定。
- **FR-008**: 更新 MUST 只影响目标账号，同空间其它账号逐字段不变。

**平台**

- **FR-009**: 平台 MUST 取自受控集合；非法值 MUST 返回 **400 诊断错误对象**且不写入。
- **FR-010**: 显示名 MUST 非空（去空白后）；空则 400。

**生命周期**

- **FR-011**: 删除工作区 MUST 一并删除其账号，且新表 MUST 登记进工作区删除清单（`workspace_delete_manifest_test.go`）与真正的删除语句（`workspace_delete.sql`）——**两处都要**。

**工程约束**

- **FR-012**: 新模块目录 MUST 满足接入合同 E1/E2/E3；`pnpm check:diagnostics-contract` MUST 报 **`checked 3 landed modules`**。
- **FR-013**: sqlc 生成文件 MUST 单独提交并核对（`make sqlc` 的产物不手改）。
- **FR-014**: MUST NOT 修改上游 Multica 代码与 `server/internal/daemon/`。
- **FR-015**: 本卡**无界面改动**；MUST NOT 新增 UI 单测。

### Key Entities

- **账号（Account / ChannelAccount，同一实体）**：品牌下的**内容渠道账号**——在某个平台上发内容的那个号。**不是**登录账号（`user`），**不是** IM 机器人安装（`channel_installation`）。
- **平台**：受控枚举。账号所在的内容平台。
- **表达/内容形式设置**：JSON。本卡只做最小切片；SOP §3.1 的完整字段清单留给 LT-012 之后。

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: 同品牌两个账号，改其一后另一个**逐字段不变**。
- **SC-002**: 跨品牌读、跨品牌写、不存在的 id 三者响应**逐字节相同**（均 404，body 无对象字段）。
- **SC-003**: 受控集合内每个平台都能创建；集合外的值 **100%** 被拒为 400 且未写入。
- **SC-004**: 删除工作区后账号行数为 **0**；删除清单漂移用例通过（新表不在 `unclassified`）。
- **SC-005**: `pnpm check:diagnostics-contract` 报 **`checked 3 landed modules`**。
- **SC-006**: 四条核心负例各有一个用例且**先写并确认失败**；每条不变量各一处变异验证。
- **SC-007**: 新增 UI 单测 **0**；上游与 daemon 改动 **0**；外键与级联 **0**。

## UI Impact

**无。** 本卡是存储与接口层，页面在 LT-013。未新增 UI 单测、未产生手动条目。

> SOP 阶段界面规则（2026-09-15）：界面整套继承上游 Multica 设计系统与设计 token，只把功能挂到既有组件上，不新增控件、不调样式。**本卡无适用项**；若实施中发现确需最小页面，按此规则执行并在 PR 正文只说明复用了哪些既有组件。

## Assumptions

- **本卡只做最小切片**：平台、显示名、表达/内容形式设置。SOP §3.1 的完整字段清单（受众、常见问题、经验与可信依据、定位主张、内容支柱、表达风格、禁用表达、主要渠道、每周可投入时间、内容目标、风格样本）与「Agent 提炼候选档案」**留给 LT-012 之后**。`docs/01` §3.1 在文档仓库，本检出没有，字段清单以任务卡转述为准。
- 表达设置存为 JSON，使完整字段落地时**不需要再加列**。
- 账号 id 与空间 id 都用 text，与诊断三表已有的 `account_id text` / `workspace_id text` 对齐——那些列本就是为这个实体预留的。
- `Scope.Accounts` 的真实账号级授权**不在本卡**：本卡提供账号实体，让 LT-012 之后可以填充它。
- 「Account/ChannelAccount 是同一实体」按任务卡执行：**只有一个实体、一张表**，不做两层。
- **平台枚举首版取值**（Q3 裁决 A）：`xiaohongshu`、`douyin`、`wechat_mp`、`bilibili`、`zhihu`、`weibo`、`kuaishou`、`shipinhao`，共 **8** 个。Go 侧受控枚举与库 `CHECK` 两处**必须一致**，有用例逐值比对两边。
- **新增一个平台需要一次迁移**（改 `CHECK` 约束），这是 Q3-A 纵深防御的代价，在此写明以免加平台时才发现。`CHECK` 是列约束，**不是外键也不是级联**，不违反数据库硬约束。
- 本卡**不提供删除账号接口**：任务卡只要求创建 / 读取 / 更新。账号的停用与删除语义（历史内容如何归属）留给后续卡，**不在本卡自行决定**。

## 待澄清问题（已按推荐值暂定，不阻塞）

### Q1 新模块落哪个登记名？

`content-boundaries.json` 没有 `account`；任务卡要求「用登记名，不新造名字」。

| 选项 | 做法 | 含义 |
|---|---|---|
| **A（推荐，已暂定）** | `server/internal/content/ip-profile/` | 已登记且**已声明依赖 `workspace-core` + `diagnostics`**，正是本卡需要的两条。SOP §3.1「账号表达配置」属该域。**用这个名字不等于建人设表**——W-02 的「无独立人设模型」说的是数据模型（表达配置挂在账号上、不另起 persona 表），模块名只是目录归属 |
| B | 新增 `account` 模块 | 与「用登记名，不新造名字」冲突，且会让 `docs/12` §2 的模块表与代码分叉 |
| C | 放进 `workspace-core` | 那是授权与空间基础设施；塞入账号业务会让它长成杂物间，下一个模块无处可去 |

### Q2 `workspace_id` 与账号 id 用 text 还是 uuid？

| 选项 | 做法 | 含义 |
|---|---|---|
| **A（推荐，已暂定）** | `text` | 与同层三张 content 表一致；`workspace_delete.sql` 的 `$1::text` 模式可直接沿用；诊断的 `account_id text` 本就是为它预留的，类型一致才能直接填 |
| B | `uuid` | 类型更严格，但与同层三表不一致，删除语句要两种写法，且写进诊断的 `account_id text` 时要转换 |

### Q3 平台枚举在哪校验？

| 选项 | 做法 | 含义 |
|---|---|---|
| **A（推荐，已暂定）** | Go 侧受控枚举**为准**（产出 400 诊断错误对象）+ 库 `CHECK` 作最后防线 | 纵深防御：绕过 API 的写入也进不来。**代价**：新增一个平台要一次迁移——这一点必须在规格里写明，不能等到加平台时才发现 |
| B | 只在应用层校验 | 加平台不需要迁移，但任何绕过 API 的写入都能落脏值，而这张表会被后续模块当分组键读 |
| C | 只加库 `CHECK` | 违反 FR-009——错误会是数据库约束错误，不是 400 诊断错误对象 |
