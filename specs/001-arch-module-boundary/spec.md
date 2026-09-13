# Feature Specification: 模块边界检查的验收闭合与本地入口

**Feature Branch**: `001-arch-module-boundary`

**Created**: 2026-09-14

**Status**: Draft

**Input**: User description: "ARCH-01/02：定义可执行的模块依赖清单，并将边界检查接入开发验证。以当前代码状态为准闭合验收缺口，不重建已有检查器。"

**Traces to**: `tasks/architecture.md` ARCH-01、ARCH-02；`docs/12-模块边界与变更回归约束.md` §2–§5；NFR-03；D10-V01。

## Current State（以代码为准，不以任务卡为准）

任务卡写「全部 TODO」，代码不是。规格基于以下已核实事实：

- **ARCH-01 已交付**：`scripts/content-boundaries.json` 声明三个 content 根目录、12 个模块（docs/12 §2 的 11 个 + `diagnostics`）及各自允许依赖、上游白名单、适配器文件清单。
- **ARCH-02 已在 CI 交付**：`scripts/check-content-boundaries.mjs` 是静态导入检查器；`scripts/check-content-boundaries.test.mjs` 有 13 个用例，负例已覆盖跨模块私有路径、反向依赖、`electron`、`next/navigation`、未登记模块、两模块与三模块循环、计算型 `import()`；工作流 `.github/workflows/loretide-content.yml` 在 push / PR 到 `app-main` 时运行自测与扫描。Issue #7 已做精确性加固并关闭。
- **仍未满足的验收项**：
  1. 本地没有仓库级入口。开发者与 agent 只能手敲 `node scripts/...`；`package.json` 无对应脚本，`make check` 不包含它。卡片要求「接入验证命令与应用 CI」，目前只有 CI 一半。
  2. 存储所有权审查规则未进入 PR 检查表。`.github/PULL_REQUEST_TEMPLATE.md` 的 Checklist 没有「迁移/sqlc 变更只触及本模块拥有的表」一项。
  3. 状态未回写。`tasks/architecture.md` 与 `tasks/todo.md` 仍标 TODO；主任务需要一份逐条对应验收项的证据记录才能回写。这两份文件在文档仓库 `main`，不在本功能改动范围内。

本功能只做上述三项闭合，不扩展检查器覆盖范围到 content 根目录之外。

## Clarifications

### Session 2026-09-14

- Q: 存储所有权（迁移/sqlc 只能碰本模块的表）除 PR 检查表外是否再加 CI 检查？ → A: 只加 PR 检查表。SQL 层所有权静态扫描误报高，先靠作者确认与主任务审查；表→模块映射不在本功能内定义。

## User Scenarios & Testing *(mandatory)*

### User Story 1 - 本地一条命令跑出与 CI 相同的边界检查 (Priority: P1)

开发者或执行 agent 在任一 worktree 里运行一条仓库命令，得到与 CI 完全相同的自测 + 扫描结果和退出码，不需要知道脚本路径。

**Why this priority**: 没有本地入口，边界检查只在 PR 后才被发现，违反 docs/12 §5「每次新增模块/导入或修改公开出口」即触发的要求；也是 constitution 原则 III 的验证手段。

**Independent Test**: 新 clone 后运行该命令，退出码 0；人为在 `packages/core/content/diagnostics/` 加一行 `import "electron"`，再运行退出码非 0 且指出文件；撤销后恢复 0。

**Acceptance Scenarios**:

1. **Given** 干净 checkout，**When** 运行仓库入口命令，**Then** 依次执行检查器自测与仓库扫描，全部通过时退出码 0，输出与 CI 日志同形。
2. **Given** 任一负例被引入，**When** 运行入口命令，**Then** 退出码非 0，输出包含违规文件路径与违规类别。
3. **Given** Windows PowerShell 与 Linux shell，**When** 各自运行入口命令，**Then** 结果一致（路径分隔符差异已由检查器归一化，测试用例 13 覆盖）。

---

### User Story 2 - PR 作者确认存储所有权 (Priority: P2)

提交含数据库迁移或 sqlc 查询变更的 PR 时，作者在检查表中确认所触及的表属于本 PR 修改的模块；若跨模块，在 PR 正文写明理由与协调方式。

**Why this priority**: docs/12 §4 第 2 条禁止模块直接更新其他模块拥有的表，但静态导入检查器看不见 SQL；这一条只能靠检查表把责任放到作者身上。

**Independent Test**: PR 模板渲染后包含该检查项；主任务审查一个含迁移的 PR 时能据此提问。

**Acceptance Scenarios**:

1. **Given** 打开新 PR，**When** 模板加载，**Then** Checklist 含存储所有权确认项，措辞指向 docs/12 §2 的所有权表。
2. **Given** PR 含跨模块表变更且未勾选/未说明，**When** 主任务审查，**Then** 有明确依据要求补充说明。

---

### User Story 3 - 主任务凭证据回写 ARCH-01/02 状态 (Priority: P3)

主任务拿到一份逐条对应 `tasks/architecture.md` 验收项的证据清单（命令、退出码、文件、测试用例编号），据此在文档仓库回写状态，不靠「看起来做了」。

**Why this priority**: 评估 F01 的教训——任务卡与代码脱节会诱导重建。回写需要证据而不是声明。

**Independent Test**: 证据文件存在于 `docs/development/content-boundary-checks.md` 的新增「验收对照」节，每一行可独立复核。

**Acceptance Scenarios**:

1. **Given** 本功能交付完成，**When** 主任务打开证据节，**Then** ARCH-01 与 ARCH-02 的每条验收/验证项各有一行：满足与否、证据位置、未满足项的说明。

---

### Edge Cases

- `content-boundaries.json` 新增模块但 docs/12 §2 未更新（或反之）：由证据节列出差异，不自动同步。
- 新增 content 子目录但未登记：检查器已按「未登记模块」失败（用例 1 覆盖）；入口命令必须透传该失败。
- 本地 Node 版本与 CI（Node 22）不同导致 `node --test` 行为差异：入口命令输出 Node 版本，文档写明要求。
- 检查器自身测试失败但扫描通过：入口命令必须以自测失败为准退出非 0，不允许「扫描通过就算过」。

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: 仓库 MUST 提供一条与现有验证命令并列的入口，顺序执行检查器自测与仓库扫描；任一步失败即退出非 0。
- **FR-002**: 该入口 MUST 在 Windows PowerShell 与 Linux/CI 下调用同一脚本与同一配置，不维护两份逻辑。
- **FR-003**: 入口 MUST 被 `CLAUDE.md` → Verification 的「Useful checks」与 `docs/development/content-boundary-checks.md` 引用，使用者不需要知道脚本路径。
- **FR-004**: PR 模板 Checklist MUST 新增一项：本 PR 的迁移 / sqlc / 存储层变更只触及本 PR 所改模块拥有的表（对照 docs/12 §2）；跨模块时已在正文说明理由与协调方式。
- **FR-005**: 交付 MUST 包含验收对照节，逐条映射 `tasks/architecture.md` ARCH-01、ARCH-02 的交付 / 验收 / 验证项到证据（命令、退出码、文件路径、测试用例名）。
- **FR-006**: 对照节 MUST 明确写出检查器是静态导入检查，不证明运行时行为、不覆盖 SQL 层所有权、不覆盖 content 根目录以外的上游代码。
- **FR-007**: 本功能 MUST NOT 修改检查器规则、`content-boundaries.json` 的模块集合、CI 工作流触发条件，除非验收对照发现与 docs/12 §2 的差异且主任务确认。

### Key Entities

- **模块依赖清单**：`scripts/content-boundaries.json`，字段 roots / modules / upstreamImports / adapters；所有权来源 docs/12 §2。
- **验收对照**：ARCH 卡片每一条验收项 → 证据行；存放在应用工程 `docs/development/content-boundary-checks.md`。

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: 新 clone 运行入口命令，本机 60 秒内完成并退出 0（实测记录时间）。
- **SC-002**: 现有 13 个检查器用例在入口命令下全部执行且通过；引入任一已知负例后入口退出非 0。
- **SC-003**: PR 模板含存储所有权项；模板 diff 可见，且措辞引用 docs/12。
- **SC-004**: 验收对照节覆盖 ARCH-01 的 3 条与 ARCH-02 的 3 条验收 / 验证项，无「待补」行；未满足项写明原因。
- **SC-005**: `content-boundaries.json` 的模块集合与 docs/12 §2 的 11 个模块 + `diagnostics` 逐一对应，差异为 0 或已在对照节列出。

## UI Impact

无。手动 UI Todo：无。

## Assumptions

- 存储所有权不做 CI 静态检查（clarify 2026-09-14）；表→模块映射不在本功能内定义。
- 检查器覆盖范围维持在三个 content 根目录；扩展到 `packages/core` / `packages/ui` / `packages/views` 整体不在本功能内（若需要，另立任务）。
- Go 侧「私有入口」按检查器现有规则处理（导入目标须为模块公开入口）；是否需要 Go 专属规则由 plan 阶段核实后决定，不在本规格预设。
- `tasks/architecture.md` 与 `tasks/todo.md` 的状态回写由主任务在文档仓库 `main` 完成；本功能只提供证据。
- 不新增测试文件类型；如需负例夹具补充，放在现有 `scripts/check-content-boundaries.test.mjs` 内。
