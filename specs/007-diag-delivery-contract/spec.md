# Feature Specification: 诊断接入合同与交付检查

**Feature Branch**: `007-diag-delivery-contract`

**Created**: 2026-09-14

**Status**: Draft

**Input**: User description: "诊断接入合同与交付检查——闭合 docs/development/diagnostics-acceptance-mapping.md 里 D13-V12「每个后续功能交付时有操作、错误、trace、故障和回归证据；真实 Codex 及远程阶段分别追加实测，模拟不代替真实通过」三条子句的「无覆盖」，以及 DIAG-13 卡片的「公共接入合同」。目标是一份可执行的合同：(a) 新增 content 模块必须接入的诊断面以清单形式固定；(b) PR 层的交付检查（PR 模板项 + 一个静态检查脚本）；(c)「模拟不代替真实通过」在文档与检查表里的落地方式。"

**Traces to**: `docs/development/diagnostics-acceptance-mapping.md` §3 的 D13-V12 三行、§4.2 的 `D13-V12 未满足`、§4.3 第 2 条；DIAG-13 卡片。

## Current State（以代码为准）

核实自 `app-main` @ `04a6250f7`。

### 1. D13-V12 的三条子句，今天各自的位置

| 子句 | 现状 | 出处 |
|---|---|---|
| 每个后续功能交付时有操作、错误、trace、故障和回归证据 | **无覆盖**。这是对**后续**交付的流程要求，既没有写下来的接入合同，也没有任何检查 | mapping §3 V12 第 1 行 |
| 真实 Codex 及远程阶段分别追加实测 | **无覆盖**。真实执行器按 constitution 原则 IX 保持禁用（`executionpolicy.Check()` 恒返回 `ErrDisabled`），因此这一条今天在技术上不可能通过 | mapping §3 V12 第 2 行 |
| 模拟不代替真实通过 | **口径层已有**：`Overview` 的 `unverified` 组件状态、面板 `text026` 与 `text007`（「未执行不表示已通过」）。**流程级无检查** | mapping §3 V12 第 3 行 |

### 2. 模块现状：合同要约束的对象，11/12 还不存在

`scripts/content-boundaries.json` 声明 **12 个模块**：`agent-gateway`、`agent-workflow`、`diagnostics`、`feedback-learning`、`ip-profile`、`knowledge-base`、`project-collab`、`review-delivery`、`source-inbox`、`topic-planning`、`work-editor`、`workspace-core`。

`server/internal/content/` 下**只有 `diagnostics/` 一个目录**。其余 11 个模块只在依赖清单里存在，尚无任何代码。

**这决定了检查脚本的形状**：一个「每个模块都必须有诊断证据」的检查，今天要么只检查 `diagnostics` 自己（几乎无意义），要么对 11 个不存在的目录报错（噪音）。合同必须在**模块落地的那一刻**才对它生效，而不是提前把 11 条红线摆在那里。

### 3. 诊断包今天提供的接入面

这是「接入合同」能引用的真实公共 API（`server/internal/content/diagnostics/`）：

| 诊断面 | 公共入口 | 状态 |
|---|---|---|
| 审计写入点 | `Store.Audit(ctx, scope, Event)`（事务内，失败即整体回滚） | 已导出 |
| 技术日志 | `Store.Technical(ctx, Event)`、`SlogHandler`、`LogBuffer` | 已导出 |
| trace 传播 | `Child`、`Pack`、`Unpack`、`DecodeQueuedEnvelope`；HTTP 边界由 `middleware.Trace` 承担 | 已导出 |
| 事务后派发 | `Outbox` 接口、`MemoryOutbox`（进程内，不跨重启） | 已导出 |
| 模拟场景 | `Scenarios`（16 个，`Scenario{ID, Expected}`）、`Simulate`、`Evaluate` | 已导出 |
| 脱敏 | `Sanitize`、`RequestIdentity` | 已导出 |
| **错误码枚举** | `log.go` 的 `var codes` | **未导出** |

**错误码枚举是个真缺口**：合同要求新模块「使用统一错误码」，但 `codes` 是包级私有的，模块既无法枚举也无法校验成员。今天唯一的间接途径是 `Sanitize` 会把不认识的 `Code` 改写为 `INTERNAL`——那是**事后纠正**，不是可供模块引用的枚举。

### 4. 检查与 PR 层现状

- `scripts/check-content-boundaries.mjs` 是同类检查的既有范式：纯 Node、无依赖、有自己的 `--test` 套件（13 用例），在 `.github/workflows/loretide-content.yml` 第 20～21 行运行，触发条件为 push / PR 到 `app-main`。
- `.github/PULL_REQUEST_TEMPLATE.md` 的 Checklist 已有一条模块相关项（第 41 行，存储所有权），本功能的交付检查项与它并列。

### 不在本功能范围

不改 `content-boundaries.json` 的模块图与依赖规则；不改既有边界检查器的任何规则；不改 CI 触发条件；不新增迁移；不实现任何业务模块；不改诊断包的既有行为。

## User Scenarios & Testing *(mandatory)*

### User Story 1 - 新模块的作者知道「接入诊断」到底要做哪几件事 (Priority: P1)

一个准备写 `source-inbox` 的开发者，能从一份清单读到：这个模块必须在哪些点写审计、技术日志要带哪些字段、trace 怎么接、错误码从哪里取、要提供哪些模拟场景与回归夹具。清单指向真实的公共入口，照着做完就满足合同。

**Why this priority**: D13-V12 第一条子句的全部内容。没有这份清单，「有操作、错误、trace、故障和回归证据」对每个新模块都要重新讨论一次，结果必然因人而异。

**Independent Test**: 拿清单对照已接入的 `diagnostics` 模块逐条核对，每条都能指到真实代码；再拿一个尚不存在的模块（如 `source-inbox`）走一遍，每条都能落到具体动作而不是口号。

**Acceptance Scenarios**:

1. **Given** 一份接入清单，**When** 开发者逐条阅读，**Then** 每条都指向一个真实存在的公共入口或一个可执行的动作，不出现「视情况而定」这类无法判定的措辞。
2. **Given** 清单要求使用统一错误码，**When** 开发者去找这个枚举，**Then** 他能拿到一个可引用、可校验成员的公共清单；若今天不存在，合同 MUST 说明改用什么方式，不能要求开发者引用私有符号。
3. **Given** 一个模块声称已接入，**When** 按清单逐条核对，**Then** 每条的满足与否都有客观依据，不依赖审阅者的印象。

---

### User Story 2 - PR 层能自动挡住「新模块没接诊断」 (Priority: P1)

一个新增 content 模块的 PR，如果没有接入诊断，检查会失败并指出缺哪一项；接入之后恢复通过。PR 模板里有对应的一条勾选项，让作者在提交前就知道这条存在。

**Why this priority**: 与 US1 同为 P1 且互补——清单靠人读，检查靠机器。DIAG-13 的「公共接入合同」缺的正是后者；没有检查，清单就只是又一份没人回头看的文档。

**Independent Test**: 造一个有目录但无诊断接入的模块夹具，检查失败并点名缺项；补上最小接入后恢复通过；对尚无目录的模块，检查既不失败也不报噪音。

**Acceptance Scenarios**:

1. **Given** 一个模块目录存在但没有任何诊断接入，**When** 运行检查，**Then** 失败，退出码非 0，并点名是哪个模块缺哪一项。
2. **Given** 同一模块补上最小接入证据，**When** 再次运行，**Then** 通过。
3. **Given** 一个在模块图中声明但尚无目录的模块，**When** 运行检查，**Then** 既不失败也不产生噪音——合同在模块落地时才生效。
4. **Given** 检查脚本自身，**When** 运行它的测试套件，**Then** 正例与负例都覆盖，且负例会因为规则被删掉而变红。
5. **Given** 一个 PR 新增 content 模块，**When** 作者打开 PR 模板，**Then** 有一条明确的交付检查项，措辞可判定。

---

### User Story 3 - 「模拟通过」不会被读成「真的通过」 (Priority: P1)

任何人看到诊断的回归结果，都能分清哪些是模拟场景跑出来的、哪些是真实执行器跑出来的。真实执行器目前禁用，因此相关证据一律记为「未执行」，而不是缺省当作通过。

**Why this priority**: D13-V12 第三条子句，且是三条里唯一**现在就可能被误读**的——前两条是「还没有」，这一条是「已有一半，容易被当成全部」。

**Independent Test**: 在交付记录与检查表里找到真实执行器一栏，确认它显式写着未执行及原因；确认没有任何地方把模拟结果计入真实通过。

**Acceptance Scenarios**:

1. **Given** 一份模块交付记录，**When** 查看证据栏，**Then** 模拟证据与真实执行器证据分列，后者标为「未执行」并注明原因（执行器按 constitution 原则 IX 禁用）。
2. **Given** 全部模拟场景通过，**When** 判定 D13-V12，**Then** 仍不记为通过——模拟通过只满足其中一条子句。
3. **Given** 交付检查表，**When** 勾选项被填写，**Then** 不存在一个「全绿」状态可以在真实执行器未跑的情况下达成。

---

### Edge Cases

- 模块目录存在但只有骨架（例如只有一个 `doc.go`）：检查如何判定，不能因为「有目录」就要求完整接入，也不能因为「还很空」就永远豁免。
- 一个模块的诊断接入写在别处（例如统一在 handler 层接入，模块目录内不出现诊断调用）：检查会误报，合同需要给出登记豁免的方式。
- `content-boundaries.json` 新增一个模块但代码尚未落地：检查不得因此变红。
- 模块被删除或改名：检查不得留下指向不存在模块的红线。
- 三个 content 根（server / core / views）中只有 server 有该模块：合同对另外两个根的要求是什么。
- 检查脚本自身出错（读不到配置、目录不可读）：必须失败并说明，不得静默通过。

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: 交付 MUST 包含一份写下来的诊断接入清单，逐项覆盖：审计写入点、技术日志字段、trace 传播、错误码枚举、模拟场景与回归夹具。
- **FR-002**: 清单的每一项 MUST 指向一个真实存在的公共入口或一个可执行的动作，MUST NOT 使用无法判定的措辞。
- **FR-003**: 清单 MUST NOT 要求模块引用当前未导出的符号。错误码枚举目前私有（Current State §3），合同 MUST 明确该项的可执行做法 [NEEDS CLARIFICATION: 本功能只在清单里描述「向诊断包申请导出」并把导出列为后续任务（不改生产代码），还是把导出 `codes` 作为本功能的一部分交付（一处生产代码改动）？]。
- **FR-004**: 交付 MUST 包含一个静态检查脚本，核对每个**已落地**的 content 模块具备诊断接入的最小证据。
- **FR-005**: 检查 MUST 对「在模块图中声明但目录尚不存在」的模块保持沉默——既不失败也不告警。
- **FR-006**: 检查失败时 MUST 指出是哪个模块缺哪一项，MUST NOT 只给一个总的失败。
- **FR-007**: 检查脚本 MUST 有自己的测试套件，含正例与负例；负例 MUST 在对应规则被移除时变红。
- **FR-008**: 检查脚本 MUST 与 `scripts/check-content-boundaries.mjs` 同类：纯 Node、无新增依赖、可独立运行、退出码表达结果。
- **FR-009**: 检查 MUST NOT 修改 `.github/workflows/loretide-content.yml` 的触发条件；接入 CI 时只新增运行步骤。
- **FR-010**: `.github/PULL_REQUEST_TEMPLATE.md` MUST 新增一条交付检查项，措辞可判定，与既有的存储所有权项并列。
- **FR-011**: 「最小证据」的判定标准 MUST 明确且可静态核对 [NEEDS CLARIFICATION: 判定为「模块目录内出现对 diagnostics 包的 import」，还是「出现审计 / 技术日志 / trace 三类调用点」，还是「前者 + 模块内存在引用 diagnostics 的测试文件」？三者的误报率与实现复杂度依次上升]。
- **FR-012**: 合同 MUST 说明模块的诊断接入写在模块目录之外时如何登记豁免，MUST NOT 只能靠关闭检查绕过。
- **FR-013**: 交付 MUST 在文档与检查表两处落地「模拟不代替真实通过」：模拟证据与真实执行器证据分列，后者在执行器禁用期间一律记为「未执行」。
- **FR-014**: MUST NOT 存在一个可在真实执行器未跑的情况下达成的「全绿」状态。
- **FR-015**: 本功能 MUST NOT 改动 `content-boundaries.json` 的模块图与依赖规则、既有边界检查器的规则、CI 触发条件；MUST NOT 新增迁移；MUST NOT 实现任何业务模块。
- **FR-016**: 合同覆盖的 content 根范围 MUST 明确 [NEEDS CLARIFICATION: 只约束 `server/internal/content/<module>/`，还是同时约束 `packages/core/content/<module>/` 与 `packages/views/content/<module>/`？后两者今天同样只有 `diagnostics` 落地，且前端侧的「审计写入点」语义与服务端不同]。
- **FR-017**: 交付 MUST 更新 `docs/development/diagnostics-acceptance-mapping.md` 中 D13-V12 与 DIAG-13 的对应行，MUST NOT 声称 D13-V12 整体通过——第二条子句（真实 Codex 实测）在执行器禁用期间不可能通过。

### Key Entities

- **接入清单（Contract）**：新模块必须接入的诊断面逐项说明；每项含「要做什么 / 公共入口 / 怎么算做到了」。
- **最小证据（Evidence）**：一个已落地模块满足接入合同的可静态核对的痕迹。
- **豁免登记（Exemption）**：模块接入写在别处时的显式记录，含模块名、原因、实际接入位置。
- **交付检查表（Delivery checklist）**：PR 层的勾选项，把模拟证据与真实执行器证据分列。

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: 拿接入清单对照 `diagnostics` 模块逐条核对，每条都能指到真实代码，无法指到的条目数为 0。
- **SC-002**: 造一个有目录、无诊断接入的模块夹具，检查退出码非 0 且输出点名该模块与缺项；补上最小接入后退出码为 0。
- **SC-003**: 在当前代码上运行检查，退出码为 0，且不对 11 个尚无目录的模块产生任何输出。
- **SC-004**: 检查脚本的测试套件在对应规则被移除时至少一条用例变红（变异验证，非「写完看绿」）。
- **SC-005**: `.github/workflows/loretide-content.yml` 的 `on:` 段在本功能前后逐字节一致。
- **SC-006**: 交付记录与检查表中，真实执行器一栏显式为「未执行」并注明原因；mapping 中 D13-V12 不被标为整体通过。
- **SC-007**: 本功能不新增任何迁移文件，`server/migrations/` 的文件数在前后一致。

## UI Impact

**无。** 本功能交付的是文档、一个 Node 检查脚本、PR 模板的一条勾选项，不触及任何页面。

## Assumptions

- 真实执行器在本功能期间保持禁用（constitution 原则 IX），因此 D13-V12 的第二条子句不可能通过；合同的职责是让这件事**可见**，不是让它通过。
- 检查脚本沿用 `check-content-boundaries.mjs` 的形态与运行方式；是否加入 CI 由主任务决定，加入时只新增步骤、不动触发条件。
- `diagnostics` 模块自身作为「已接入」的参照样本，不需要为满足本合同而改动。
- 模块图以 `scripts/content-boundaries.json` 为唯一来源，本功能只读不写。
- 本功能不产出任何手动 UI 验收项——没有页面改动。
