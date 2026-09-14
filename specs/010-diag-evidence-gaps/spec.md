# Feature Specification: 补齐「代码存在但无测试」的定向证据

**Feature Branch**: `claude/spec-010-diag-evidence-gaps`

**Created**: 2026-09-15

**Status**: Draft

**Input**: User description: "闭合 `docs/development/diagnostics-acceptance-mapping.md` §2 中类型为『代码存在但无测试』且不属于浏览器界面的行，为每行补一条定向自动测试：DIAG-02 私有正文脱敏、DIAG-03 级别配置、DIAG-04 迁移约束、DIAG-05 结果回写携带 operation_id/trace_id、DIAG-07 队列统计与样本不足、DIAG-08 查询路径只读、DIAG-12 下载响应与导出路径无外发请求。DIAG-06/09/13 与 DIAG-12 预览等界面行不在范围，保持手动。"

**Traces to**: `docs/development/diagnostics-acceptance-mapping.md` §2 各卡片表内类型为「代码存在但无测试」的行、§4.1 的 24 项计数、§4.3 第 4 条。

## Current State（以代码为准）

核实自 `app-main` @ `af02b6e`。

### 0. 先说结论：点名的 8 条里，**有 2 条已经有测试，1 条写测试关不掉**

任务描述要求「逐行核实哪些已有测试只是对照表没引用（若有，则只改对照表引用，不重复写）」。逐行核实的结果如下，**这改变了本特性的交付物构成**：

| 点名的行 | 核实结果 | 本特性怎么处理 |
|---|---|---|
| DIAG-02 交付私有正文脱敏 | **已有测试**，对照表未引用 | **只改对照表引用，不写新测试** |
| DIAG-03 交付级别配置 | **写测试关不掉**——按级别过滤写入的配置路径在生产代码里不存在 | **Q1**，见 Clarifications |
| DIAG-04 遵守无 FK / 并发索引迁移约束 | 无测试，属人工核对 | 新增 Go 测试 |
| DIAG-05 交付结果回写 trace | 现有测试只覆盖内存态，**落库回写链路无测试** | 新增 DB 背书测试 |
| DIAG-07 交付队列统计 | 无测试 | 新增测试 |
| DIAG-07 验证样本不足 | 无测试 | 新增测试 |
| DIAG-08 查询不能直接修改业务状态 | 无测试 | 新增 DB 背书测试 |
| DIAG-12 下载保存服务端原始字节与文件名 | **已有测试**（对照表自己的备注就写着「已通过」，但该行类型仍记「代码存在但无测试」） | **只改对照表引用与类型，不写新测试** |
| DIAG-12 不自动上传诊断包 | 无自动核对 | **Q3**，见 Clarifications |

### 1. DIAG-02「交付私有正文脱敏」：已有测试，且比该行要求的更强

该行备注写「『不承载正文』由结构保证，未见针对**正文被塞入其他字段**的负例」。这个负例存在：

- `log_regression_test.go` 的 `TestModelOutputSamplesNeverSurviveSanitize`（feature 005/#32 交付）把模型输出样本**逐一塞进 13 个字段**（`Step`、`Build`、`Version`、`Message`、`Next`、`Code`、`Component`、`Action`、`Outcome`、`Severity`、`ActorKind`、`Route`、`Upstream`），逐字段断言样本不残留。测试里还写明了为什么**故意排除** `Workspace` / `Account` / `Actor` / `ObjectID` 四个业务身份字段。
- `TestLogRegressionSanitizeRules` 另外三处断言 `Message` 被错误码的固定文案覆盖（`"Internal error"`、`"Connection interrupted"`、`"Model quota exhausted"`），其中一处**注入的正是一段恶意自由文本**。

「`safe_message` 只能是固定枚举」这句话因此已被钉住。**再写一条是重复。**

### 2. DIAG-03「交付级别配置」：缺的是功能，不是测试

`limits.go` 全文只有一个 `ConfigureLimits(maxLogs, days string) error`，可配置项**只有两个**：`MaxLogs`（100..100000）与 `Retention`（1..90 天），非法值一律返回错误。

`Severity` 在生产代码里只有两种出现方式：`Sanitize` 的枚举白名单 `oneOf(e.Severity,"debug","info","warn","error")`（`log.go:88`），以及各处赋值。**全仓没有任何「按级别过滤写入」的路径**——没有一个配置项、一个判断分支能让 `debug` 级别的事件不被写入。

该行备注要求的正是「按级别过滤写入的配置路径与测试」。**在不改生产代码的前提下，这一行关不掉**：测试只能固定现状，而现状里没有这个功能。已有测试覆盖的是另外两件事（`TestLogRegressionSanitizeRules` 覆盖枚举收敛，`TestDiagnosticLimitsRejectInvalidConfiguration` 覆盖两项配置的边界）。→ **Q1**

### 3. DIAG-04「迁移约束」：确实无测试，且刚刚被确认为人工核对

`af02b6e`（#48）把 §4.3 的这项对表标为已完成，并**明写**：「仓库迁移测试仍不检查这两项，**属人工核对**」。

仓库现有两条迁移 lint 是 `TestMigrationNumericPrefixesAreUnique`（编号唯一）与 `TestMigrationFilesHaveMatchingDirections`（`.up`/`.down` 成对），**都不检查 FK 与并发索引**。

本特性要做的正是把这次人工核对变成每次 CI 都跑的断言。核对范围现在是 `468`～`476`（feature 009 新增了 `474`～`476`）。

### 4. DIAG-05「交付结果回写 trace」：现有测试覆盖的是内存态，不是回写

`TestSimulatorRegressionDeterministicTimeAndIdentifiers`（`simulator_regression_test.go:376-431`）断言同一次运行内全部事件共享同一个 `Operation` 与 `Trace`，且跨运行互不相同。**但它断言的是 `Simulate()` 的内存返回值。**

落库这一段是另一回事：`Store.CommitRun` 在写 `content_diagnostic_run` 之前执行 `saved.Events = nil`——**运行行本身不存事件**。事件走的是同一事务内的 `appendAudit`，而那条审计事件的 `Trace` / `Operation` 是从 `run.Events[0]` 复制过去的。

所以「结果回写携带 operation_id/trace_id」是一条**跨越内存与数据库的链路**，现有测试没有走完它。

### 5. DIAG-07：两行都无测试

- **队列统计**：`service.go:16` 里 `if e.Component=="queue"{o.Metrics.QueueWait+=e.Duration}`——`QueueWait` 是**只累加 component 为 `queue` 的事件时长**。全仓测试文件无一提及 `QueueWait`。
- **样本不足**：`service.go:17` 里 `if len(durations)>=20{...o.Metrics.P95=&v}`——**少于 20 个样本时 `P95` 保持 nil**。全仓测试文件无一提及 `P95`。

### 6. DIAG-08「查询不能直接修改业务状态」：读路径确实只读，但无断言

`Query` / `Runs` / `GetRun` 三个方法全部走 `s.pool.Query` / `s.pool.QueryRow`，SQL 全是 `SELECT`（`store.go:219`、`224`、`268`、`292`），**没有一处 `Exec`**。写入集中在 `Audit` / `Technical` / `CommitRun` / `PruneTechnical`。

结构上成立，但没有任何断言固定它——加一行 `Exec` 不会有任何测试变红。

### 7. DIAG-12：一行已覆盖，一行可静态核对

- **下载保存原始字节与文件名**：`TestContentDiagnosticExportDownloadNamesTheFileItWantsSaved`（`internal/handler/content_diagnostics_test.go:195`）已断言 `Content-Disposition` 以 `attachment;` 开头且含 `filename=`、响应体是服务端原样的 bundle、`redacted` 为 true、保留服务端的 wire key 名。对照表该行的备注自己就写着这条**「已通过」**，但行的类型仍记「代码存在但无测试」——**类型与备注自相矛盾**。缺的只是前端保存路径的测试，而那是 UI 单测，按 constitution 原则 II 不写不跑。
- **不自动上传诊断包**：前端保存路径 `apps/web/platform/content-diagnostics.ts` **全文 7 行**，只有 `URL.createObjectURL` + 创建 `<a>` + `click()` + `revokeObjectURL`，**没有任何网络调用**。这一点可静态核对。→ **Q3**

### 8. 符合条件但未被点名的两行

任务描述说的是「§2 中类型为『代码存在但无测试』且不属于浏览器界面的行」。按这个定义，还有两行符合但未在点名清单里：

| 行 | 情况 |
|---|---|
| DIAG-02 验证嵌套字段 | 备注写「`Event` 为**扁平结构**，嵌套场景不适用但卡片明确要求」。扁平结构下造不出真实的嵌套负例 |
| DIAG-10 不复制媒体 | **已被 `specs/008-diag-linkage-and-invariants` 的 FR-012 / FR-013 认领**（G4：以字段清单形式断言快照不含任何二进制或媒体载荷）。008 尚未实施（`tasks.md` 勾选数为 0） |

→ **Q2**

### 不在本功能范围

界面行一律保持手动，不补 UI 单测：DIAG-06 三行、DIAG-09 六行、DIAG-13 两行、DIAG-12 预览行、DIAG-10「交付模型参数快照」（面板文案行）。不改任何生产代码。不改 CI 触发条件。不新增迁移。不碰执行闸门。

## Clarifications

### Session 2026-09-15（主任务已裁决）

- **Q1**（DIAG-03 级别配置）→ **A**：**只改对照表引用与措辞，不发明配置路径**。该行重新界定为它今天真正交付的东西——`Severity` 受限于四值枚举、`limits.go` 两项可配置且拒绝非法值——两者均已有测试。「按级别过滤写入」作为**缺失功能**单列后续任务，不在本特性内实现。见 FR-003、FR-014。
- **Q2**（两条未点名的非界面行）→ **A**：DIAG-02 验证嵌套字段与 DIAG-10 不复制媒体**都不纳入**，并在交付记录里**显式列为不在范围及原因**（后者已被 `specs/008` 的 FR-012/013 认领，前者在 `Event` 扁平结构下造不出真实嵌套负例）。见 FR-015。
- **Q3**（DIAG-12 不自动上传的核对形式）→ **A**：写成 **`node:test` 静态检查**，与 `scripts/check-diagnostics-contract.mjs` 同形。**附三条约束**：
  1. **扫描范围必须显式列文件**，不用目录通配——清单见 FR-011a；
  2. **排除 `packages/core/api/client.ts`**。下载本身经 `api.contentDiagnosticDownload` → `fetchRaw` 走共享客户端，那是**合法路径**，不得被判红；
  3. 检查**必须同时有正例与负例**：当前仓库退出 0；夹具里塞一个 `fetch("https://…")` 或 `sendBeacon` 必须变红。
  见 FR-011、FR-011a、FR-011b、FR-012。

## User Scenarios & Testing *(mandatory)*

### User Story 1 - 读对照表的人不会把「已有测试」误读成缺口 (Priority: P1)

有人拿对照表判断诊断能力的成熟度。凡是已经有测试固定住的行为，对照表必须指到那条测试；凡是写测试也关不掉的行，必须写明关不掉的原因是缺功能而不是缺测试。

**Why this priority**: 这是**本特性最先要做的事**。逐行核实发现点名的 8 条里有 2 条已有测试、1 条写测试关不掉——先不澄清这个，后面补的测试就会有一部分是重复劳动，而对照表继续误导读者。

**Independent Test**: 拿对照表里被改动的每一行，按它引用的测试名去仓库里找，能找到且该测试确实断言了这一行说的事。

**Acceptance Scenarios**:

1. **Given** 一行已有测试覆盖，**When** 读对照表该行，**Then** 它引用得到具体的测试名，且类型不再是「代码存在但无测试」。
2. **Given** 一行写测试也关不掉，**When** 读对照表该行，**Then** 它写明缺的是**功能**，并指向单列的后续任务，MUST NOT 只说「无测试」。
3. **Given** 某一行的类型与备注互相矛盾（备注说已通过、类型说无测试），**When** 本特性交付后，**Then** 该矛盾不再存在。

---

### User Story 2 - 六条真实缺口各有一条定向测试 (Priority: P1)

迁移约束、回写 trace、队列统计、样本不足、查询只读、不自动上传——每一条都有一条测试盯着，删掉对应的实现会让它变红。

**Why this priority**: 与 US1 同为 P1 且互补。US1 让对照表说实话，US2 让实话里剩下的缺口被真正堵上。

**Independent Test**: 对每条新测试做变异验证——删掉或反转它盯着的那条实现规则，该测试必须变红。

**Acceptance Scenarios**:

1. **Given** 一条新增测试，**When** 删掉它盯着的实现规则，**Then** 该测试变红；还原后变绿。
2. **Given** 需要数据库的测试，**When** 数据库未配置，**Then** 它**跳过**，且跳过在交付记录中记为「未执行」，MUST NOT 记为通过。
3. **Given** 迁移约束测试，**When** 新增一个含 `REFERENCES` 或非并发索引的 `content_` 迁移，**Then** 它变红并点名是哪个文件、违反哪一条。

---

### User Story 3 - 界面行保持手动，不被悄悄「补测试」 (Priority: P2)

DIAG-06 / 09 / 13 与 DIAG-12 预览这些界面行，本特性一条测试都不补，且交付记录写明它们为什么留着。

**Why this priority**: constitution 原则 II 是不可协商的。把界面行「顺手补上」是这类补证据任务最容易犯的错——改个文件名、换个断言方式，UI 单测就混进来了。

**Independent Test**: 交付后统计新增的 UI 单测数，应为 0；`packages/views/content/diagnostics/` 下测试文件数量不变。

**Acceptance Scenarios**:

1. **Given** 本特性的全部改动，**When** 统计新增的 UI 单测，**Then** 数量为 0。
2. **Given** 界面行，**When** 读交付记录，**Then** 每条都写明保持手动及依据。

---

### Edge Cases

- 一条已有测试只覆盖了该行的**一部分**：只改引用会高估覆盖度，必须写清楚覆盖到哪、没覆盖哪。
- 迁移约束测试的范围边界：`468` 之前的迁移不属 diagnostics 模块，扫进来会把别的模块的历史包袱变成本模块的红线。
- 迁移文件里 `REFERENCES` 出现在注释或字符串里：字面量匹配会误报。
- 静态检查扫前端导出路径：该路径若将来被拆成多个文件，检查的文件清单会过时而静默失效。
- 回写 trace 的测试需要真实事务；没有数据库时它只能跳过，而跳过不是通过。
- `P95` 的边界恰好在 20 个样本：19 与 20 两侧都要验，只验一侧等于没验边界。

## Requirements *(mandatory)*

### 核实与更正（US1）

- **FR-001**: 交付 MUST 逐行核实点名的每一行在仓库里是否已有测试，并 MUST 把核实结果（已有／无／关不掉）写进交付记录。
- **FR-002**: 已有测试覆盖的行，交付 MUST 只更新对照表的引用与类型，MUST NOT 新写一条重复的测试。
- **FR-003**: 写测试也关不掉的行，交付 MUST 在对照表写明缺的是**功能**而非测试，并 MUST 指向单列的后续任务（Q1 = A）。
- **FR-004**: 对照表中**类型与备注自相矛盾**的行（DIAG-12 下载行）MUST 被改正。
- **FR-005**: 对照表 §4.1 的计数 MUST 按第 2 节各行重新统计，MUST NOT 只改总数。

### 新增定向测试（US2）

- **FR-006**: MUST 有一条测试断言 `server/migrations/` 中 **`468` 及之后**、文件名含 `content_` 的迁移**不含** `REFERENCES` / `FOREIGN KEY` / `CASCADE`，且其中每一条 `CREATE INDEX` MUST 为 `CONCURRENTLY` 并**单独成一个单语句文件**。失败时 MUST 点名文件与违反的条目。
- **FR-007**: MUST 有一条测试走完「模拟 → 提交落库 → 读回」的链路，断言回写的记录携带与运行一致的 `operation_id` 与 `trace_id`，MUST NOT 只断言两者非空。
- **FR-008**: MUST 有一条测试断言 `QueueWait` 只累计 `component` 为队列的事件时长——非队列事件的时长 MUST NOT 计入。
- **FR-009**: MUST 有一条测试断言样本数不足阈值时 `P95` 为 null、达到阈值时为非 null，且 MUST 覆盖阈值**两侧**。
- **FR-010**: MUST 有一条测试断言 `Query` / `Runs` / `GetRun` 三条读路径**不产生任何写入**——调用前后 diagnostics 模块各表的行数 MUST 一致。
- **FR-011**: MUST 有一条 `node:test` 静态检查证明导出/保存路径**不发起任何外发请求**：被检查的文件中 MUST NOT 出现 `fetch` / `XMLHttpRequest` / `sendBeacon` / `new WebSocket` 这类原生网络原语（Q3 = A）。
- **FR-011a**: 检查的扫描范围 MUST 以**显式文件清单**给出，MUST NOT 用目录通配。清单为导出/保存链路上的四个文件：
  - `apps/web/platform/content-diagnostics.ts`（保存到本机，全文 7 行，只有 `createObjectURL` + `<a>` + `click`）
  - `packages/views/content/diagnostics/index.tsx`（面板，`download` 由 props 注入）
  - `packages/core/content/diagnostics/queries.ts`（`download` mutation，调用 `api.contentDiagnosticDownload`）
  - `apps/web/app/[workspaceSlug]/(dashboard)/diagnostics/page.tsx`（把前两者接起来）

  `packages/core/api/client.ts` MUST 被**排除**：下载本身经 `api.contentDiagnosticDownload` → `fetchRaw` 走共享客户端，那是**合法路径**，把它判红等于把正常下载判成违规。
- **FR-011b**: 检查 MUST 同时有**正例与负例**：对当前仓库退出 0；对一个塞了 `fetch("https://…")` 或 `sendBeacon` 的合成夹具 MUST 变红并点名文件与命中的原语。
- **FR-011c**: 原语匹配 MUST 按**词边界**判定。`fetch(` 的朴素子串匹配会把 React Query 的 `refetch()` 当成网络调用——已核实 `index.tsx` 里有 **4 处 `refetch(`**，朴素匹配得 4 个误报，词边界匹配得 0 个。
- **FR-012**: FR-011a 的文件清单 MUST 在其中任一文件不存在时**失败而不是静默通过**——一个扫不到文件的检查会永远绿。
- **FR-013**: 每条新增测试 MUST 在对应实现规则被删除或反转时变红（变异验证）。

### 范围与边界（US3）

- **FR-014**: 「按级别过滤写入」MUST 作为**后续任务**单列，MUST NOT 在本特性内实现——本特性 MUST NOT 改动任何生产代码。
- **FR-015**: DIAG-02「验证嵌套字段」与 DIAG-10「不复制媒体」MUST 显式列为不在本特性范围，并写明原因（后者已被 `specs/008` 认领）（Q2 = A）。
- **FR-016**: 本特性 MUST NOT 新增任何 UI 单测，MUST NOT 为界面行补自动测试；界面行 MUST 保持手动验收。
- **FR-017**: 本特性 MUST NOT 改动 CI 触发条件、MUST NOT 新增迁移、MUST NOT 改动执行闸门。
- **FR-018**: 需要数据库的测试在数据库未配置时 MUST 跳过而非失败，且交付记录 MUST 把跳过记为「未执行」，MUST NOT 记为通过。
- **FR-019**: 若实施中发现某条行为**必须改生产代码才能测**，MUST 停下来报告，MUST NOT 自行改动生产代码。

### Key Entities

- **对照表行（Mapping row）**：一条卡片条目与它的证据、类型、备注。本特性改变其中若干行的**类型**与**引用**，不改变条目本身。
- **定向测试（Targeted test）**：一条只盯住一行对照表条目的测试，失败信息能让读者直接定位到那一行。
- **静态核对（Static check）**：对「代码里没有某样东西」这类命题的检查，与运行时测试互补。

## Success Criteria *(mandatory)*

- **SC-001**: 对照表中被本特性改动的每一行，其引用的测试名都能在仓库中找到，且该测试确实断言该行说的事；找不到或对不上的行数为 **0**。
- **SC-002**: 对照表中**类型与备注自相矛盾**的行数为 **0**。
- **SC-003**: 六条新增测试各自做过变异验证，变红率 **6/6**。
- **SC-003a**: 静态检查对当前仓库退出码为 **0**；对塞入 `fetch("https://…")` 或 `sendBeacon` 的夹具退出码**非 0** 且输出点名文件与命中的原语。
- **SC-003b**: 静态检查对 `index.tsx` 现有的 4 处 `refetch(` 产生的误报数为 **0**。
- **SC-004**: 迁移约束测试在被注入一个含 `REFERENCES` 的 `content_` 迁移夹具时变红，并在失败信息里点名该文件。
- **SC-005**: `P95` 的阈值两侧各有一条断言（不足 → null，达到 → 非 null）。
- **SC-006**: 只读断言在任一读路径被改为写入时变红。
- **SC-007**: 本特性新增的 UI 单测数为 **0**；`packages/views/content/diagnostics/` 下测试文件数前后一致。
- **SC-008**: 本特性对 `server/` 下**生产代码**（非 `_test.go`）的改动行数为 **0**。
- **SC-009**: 对照表 §4.1 的三列计数与第 2 节逐行统计一致，且等于 **122 / 15 / 8**（合计 145，条目总数不变）——本特性把 9 行从「代码存在但无测试」移入「自动测试已通过」。
- **SC-010**: 带数据库的测试在配置了数据库时**无一跳过**（`-v` 输出的 SKIP 计数为 0）。

## UI Impact

**无。** 本特性交付测试与对照表更新，不触及任何页面，也不为界面行补自动测试。

## Assumptions

- 对照表第 2 节是「哪些行属于本特性」的唯一来源；判断某行是否界面行以其证据列指向 `packages/views/` 或面板文案为准。
- 已有测试是否「覆盖该行」由测试断言的内容判定，不看测试名。
- `specs/008` 在本特性实施期间可能仍未实施；DIAG-10 那一行无论 008 何时落地都不由本特性处理。
- 迁移约束测试的范围以 `468` 为下界、以文件名含 `content_` 为条件；这两条是为了不把别的模块的历史包袱扫进来。
- Q1/Q2/Q3 已由主任务裁决为 A；Q3 的三条附加约束（显式文件清单、排除 `client.ts`、正负例齐备）写入 FR-011a / FR-011b / FR-012 与 SC-003a。
