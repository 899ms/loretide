# Feature Specification: 品牌级自动预检开关（LT-015）

**Feature Branch**: `claude/spec-019-lt015-brand-precheck-switch`

**Created**: 2026-09-15

**Status**: Draft

**Input**: 任务卡 LT-015。范围：品牌配置默认自动预检开启及设置界面，所有账号统一使用；**此处只做配置，预检运行由 EP-06 实现**。

**追踪**：R-038 / R-039、D11-V06。**依赖**：LT-013 已满足。

## SOP 对应

`docs/01-完整工作流.md`（文档仓库 `main` 分支）里这句是本卡要让它可操作的对象：

> **已确认新建品牌空间的自动预检默认开启，提审时触发，关闭后保留手动检查。首版所有账号统一使用品牌级开关，不提供账号级覆盖；最终人工审核不可关闭。**

拆成四件事，本卡只做前三件里属于**配置**的部分：

| SOP 子句 | 谁做 |
|---|---|
| 「自动预检**默认开启**」 | **本卡**：读时填默认 `true` |
| 「首版所有账号**统一使用品牌级开关**，不提供账号级覆盖」 | **本卡**：键挂在工作区；账号侧无字段、无控件，并有负例 |
| 「**关闭后保留手动检查**」 | **本卡**：关闭可保存，界面给固定说明；「最终人工审核不可关闭」由审核内核保证 |
| 「**提审时触发**」 | **EP-06**，不在本卡 |

§3.2「设定运营规则」那句「设定每周内容节奏、渠道模板、**审核规则**、默认时区、反馈观察时点」里的「审核规则」，本卡落地其中的自动预检开关一项（「默认时区」已由 LT-009 落地）。

## Current State（以代码为准）

核实自 `app-main` @ `961ee41`。

### 1. 同类配置已有一套成熟手法，本卡照抄

LT-009（#63）把品牌时区做成了**工作区 settings JSON 里的一个键**，零迁移。四个部件齐全：

| 部件 | 位置 | 做了什么 |
|---|---|---|
| 键名 | `server/internal/handler/workspace.go:212` `const workspaceTimezoneKey = "loretide.timezone"` | `loretide.` 前缀避开上游同名键 |
| 写入校验 | 同文件 `validateTimezoneSetting` | 键缺席放行；类型/取值不合法 → `400` |
| 读时填默认 | 同文件 `timezoneFilled`，在 `workspaceToResponse:121` 调用 | **只作用于响应，不回写存储行** |
| 前端纯函数 | `packages/core/workspace/timezone.ts` + `timezone.test.ts` | 键名、默认值、读取、合并写回 |
| 界面 | `packages/views/settings/components/workspace-tab.tsx:344` | `SettingsRow` + `Select`，`api.updateWorkspace` + 写缓存 |

**本卡用同一套手法**，键为 `loretide.auto_precheck`（布尔，缺省 `true`），**不新增迁移、不新增端点、不新增列**。

### 2. 校验与填充的挂点已经存在，只需各加一个键

`validateTimezoneSetting` 有两个调用方：`CreateWorkspace`（`workspace.go:324`）与 `UpdateWorkspace`（`workspace.go:496`）。`timezoneFilled` 有一个：`workspaceToResponse:121`。

**这三处都在 `server/internal/handler/workspace.go`，是上游 Multica 文件。** #63 已经在同一文件里加过这三处。本卡加第二个键，按 workflow 第 13 步走独立 `upstream:` 提交并在 PR 正文单列「上游改动」。

### 3. 设置界面：同一张卡片里已有时区行

`workspace-tab.tsx` 的 `SettingsCard` 里依次是时区行、Logo 行。开关加在**同一张卡片**，与时区行并列。

`Switch` 组件已在 `packages/ui/components/ui/switch.tsx`，`packages/views/settings/components/issue-tab.tsx:50` 有 `SettingsRow` + `Switch` 的现成写法。**不新增控件、不调样式。**

### 4. 账号侧今天没有任何预检字段——这正是要守住的

`packages/core/content/ip-profile/contract.ts:12` 的 `accountSchema`：

```text
account_id, workspace_id?, platform, display_name, settings?, created_at?, updated_at?
```

**`settings` 是一个 `z.record(z.string(), z.unknown())`**，也就是说账号**有**一个能放任意键的设置容器。「不提供账号级覆盖」因此不是「字段不存在所以做不到」，而是一条**必须被断言守住**的规则：就算有人把 `loretide.auto_precheck` 写进某个账号的 `settings`，它也**不得**改变生效值。

因此负例不能只写成「schema 里没有这个字段」——那只证明今天没人加过。见 FR-009。

## Clarifications

### Session 2026-09-15（**主任务已裁决：Q1 / Q2 / Q3 全部 A**）

- **Q1**（说明文案的出现时机）：任务卡写「**关闭时**旁边一句固定说明『关闭不表示免人审』」。→ **裁决 A**：该句**只在关闭时**出现（照卡片字面）；开关行本身**始终**有一句常规描述说明它做什么。理由：常驻一句「关闭不表示免人审」在开着的时候是噪音，而它要提醒的场景恰恰是关闭那一刻。见 FR-006。
- **Q2**（存量工作区怎么读）：读时填默认意味着**所有已存在的品牌**也读作开启，不只是新建的。→ **裁决 A**：**是，一律读作 `true`**，与时区的做法一致。验收只写了「新品牌 true」，但让存量品牌读作「未设置」会多出一个三态，而 EP-06 尚未实现、开着也不会触发任何运行。见 FR-002。
- **Q3**（新建流程要不要放控件）：LT-009 在 onboarding 的建工作区步骤放了时区选择。→ **裁决 A**：**不放**。「新品牌 true」由读时默认直接满足，多一个控件就多一个能在创建时把它关掉的入口，而卡片要的是「默认开启」。创建接口仍会校验该键（有人手工传非布尔值要拿到 400）。见 FR-003、FR-007。

## User Scenarios & Testing *(mandatory)*

### User Story 1 - 品牌运营者看到并能改这个开关 (Priority: P1)

在品牌设置里看到「自动预检」一行，默认开着；关掉能保存，并且旁边明说关掉不等于不用人审。

**Why this priority**: 这是卡片的交付物本身。SOP 的「默认开启」「关闭后保留手动检查」两句要落到一个人能看见、能点的地方。

**Independent Test**: 打开品牌设置 → 看到开关为开 → 关掉 → 刷新 → 仍为关 → 旁边有那句说明。

**Acceptance Scenarios**:

1. **Given** 一个新建品牌，**When** 打开设置，**Then** 自动预检为**开**。
2. **Given** 开关为开，**When** 关掉，**Then** 保存成功，且出现固定说明「关闭不表示免人审」。
3. **Given** 关掉后刷新页面，**When** 重新读取，**Then** 仍为关——不被默认值覆盖回开。

---

### User Story 2 - 一个品牌一个开关，账号不能各行其是 (Priority: P1)

同一品牌下所有账号用同一个开关；账号页没有任何覆盖控件，也没有能起作用的覆盖字段。

**Why this priority**: 与 US1 同为 P1，且**更硬**。SOP 明写「首版所有账号统一使用品牌级开关，不提供账号级覆盖」。一个能在账号上悄悄关掉预检的字段，比开关本身没做出来危险得多——它会让「这个品牌开着预检」这句话不再为真。

**Independent Test**: 账号设置页从头看到尾，没有任何与预检相关的控件；且把该键写进某个账号的 `settings` 后，生效值不变。

**Acceptance Scenarios**:

1. **Given** 账号设置页，**When** 通读全部控件，**Then** **没有**任何自动预检相关控件。
2. **Given** 某账号的 `settings` 里被写入 `loretide.auto_precheck: false`，**When** 求该账号的生效值，**Then** 仍以**品牌**的值为准，账号那份**不生效**。
3. **Given** 两个品牌分别设为开与关，**When** 切换查看，**Then** 各自独立，互不影响。

---

### Edge Cases

- 非布尔值（字符串 `"false"`、数字 `0`、`null`）写入该键：必须 `400`，不能存进去变成一个谁也不知道怎么读的值。
- 键缺席：合法。不携带该键的更新（改名、改 Logo）不得被强制带上它。
- 读时填默认**不得回写**：读一次工作区不应产生一次写。
- 存量工作区（该功能之前建的）没有这个键，读作开启。
- 关闭状态必须能被读回来：这是最容易错的一处——`false` 是布尔零值，用「有没有值」判断会把关闭当成未设置，再被默认值翻回开。

## Requirements *(mandatory)*

### 配置本体（US1）

- **FR-001**: 品牌的自动预检开关 MUST 存在工作区 settings JSON 的 `loretide.auto_precheck` 键，类型 MUST 为布尔。MUST NOT 新增迁移、列或端点。
- **FR-002**: 键缺席时 MUST 读作 **`true`**，包括该功能之前建的工作区（Q2 = A 已裁决）。
- **FR-003**: 读时填默认 MUST 只作用于响应，MUST NOT 回写存储行。
- **FR-004**: `false` MUST 能被存储并读回。MUST NOT 用「值是否为真」或「键是否有值」来判断是否已设置——那会把关闭当成未设置。
- **FR-005**: 写入非布尔值 MUST 返回 `400`。键缺席的更新 MUST 照常通过。

### 界面（US1）

- **FR-006**: 设置页 MUST 用 `packages/ui` 既有的 `Switch` 与 `SettingsRow`，放在工作区设置既有的那张卡片里。**MUST NOT 新增控件、MUST NOT 调样式。** 关闭时 MUST 显示固定说明「关闭不表示免人审」（Q1 = A 已裁决：仅关闭时）。
- **FR-007**: MUST NOT 在新建品牌流程里加这个控件（Q3 = A 已裁决）。
- **FR-008**: 文案 MUST 四语言齐全，`locales/parity.test.ts` MUST 通过。

### 账号级覆盖（US2）

- **FR-009**: MUST 有负例证明账号级覆盖不生效：core 的纯函数 MUST 只接受**工作区**作为入参；且 MUST 有一条断言——把该键写进账号的 `settings` **不改变**生效值。**仅断言「schema 里没有该字段」不够**，因为账号的 `settings` 是一个可放任意键的容器。
- **FR-010**: 账号设置页 MUST NOT 出现任何自动预检控件；手动清单 MUST 有一条通读账号页的负例。

### 范围

- **FR-011**: 本卡 MUST NOT 实现预检的运行、触发或结果展示——那是 EP-06。
- **FR-012**: MUST NOT 调用模型。
- **FR-013**: MUST NOT 写 UI 单测。默认值、非法值、读不回写这三类逻辑 MUST 落在 core 的纯函数 node 测试里。
- **FR-014**: 越权 MUST 沿用既有工作区中间件，MUST NOT 新增权限判定。

## Success Criteria *(mandatory)*

- **SC-001**: 新建品牌读到的自动预检值为 **`true`**。
- **SC-002**: 设为 `false` 后重新读取仍为 `false`，且存储行未被读操作修改。
- **SC-003**: 写入非布尔值返回 `400` 的用例数 ≥ 3（字符串、数字、null）。
- **SC-004**: 不携带该键的工作区更新成功率 100%。
- **SC-005**: 两个品牌各自设值后互不影响。
- **SC-006**: 账号 `settings` 中写入同名键后，生效值与品牌值**完全一致**（即账号那份 0 影响）。
- **SC-007**: 账号设置页中自动预检相关控件数为 **0**。
- **SC-008**: 新增迁移数为 **0**，新增端点数为 **0**，新增 UI 单测数为 **0**。
- **SC-009**: `locales/parity.test.ts` 通过（160）。
- **SC-010**: 本卡涉及模型调用次数为 **0**。

## UI Impact

**复用既有组件，零样式改动。** 按 `docs/development/design/README.md` 的 SOP phase rule：只用 `packages/ui` 既有 `Switch` 与 `SettingsRow`，挂进 `workspace-tab.tsx` 既有的 `SettingsCard`，与时区行并列。不新增控件、不调样式、不引入第二套视觉语言。界面呈现由手动清单验收（原则 II）。

## Assumptions

- 品牌 = 工作区，与 LT-009 同一口径。
- EP-06 未实现，所以本卡交付后开关**不驱动任何运行**；它是配置，验收也只验配置。
- 「最终人工审核不可关闭」由审核内核保证，不在本卡的断言范围——本卡只保证界面不暗示关闭开关就免了人审。
