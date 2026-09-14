# Research: 007 诊断接入合同与交付检查

Phase 0。Technical Context 无 NEEDS CLARIFICATION（五处均已由 2026-09-14 clarify 解决）。以下为落地前的技术决策。

## D1. 检查脚本的形状：抄既有检查器的分法

- **Decision**: 导出一个**纯函数** `check(files, config) -> string[]`，`files` 是 `路径 -> 源码` 的映射；CLI 入口负责走目录、读文件、打印错误、设退出码。与 `scripts/check-content-boundaries.mjs` 完全同构。
- **Rationale**: 这不是风格偏好，它直接解决夹具问题。既有检查器的测试用**合成文件映射**当输入（`check({'server/internal/content/work-editor/a.go': 'package editor\nimport "..."'}, config)`），所以负例不需要在仓库里造一个假模块目录——三个「分别只缺一条」的夹具全是内存里的字符串。在仓库里造假模块反而会被既有的边界检查器扫到。
- **Alternatives considered**: 直接在 CLI 里读盘并判定——无法在不建真目录的前提下测负例，否决。

## D2. 「模块已落地」怎么判定

- **Decision**: 模块 `M` 已落地 ⟺ `server/internal/content/M/` 下存在至少一个 `.go` 文件。未落地的模块**完全跳过**，不产出任何输出。
- **Rationale**: FR-005 与 SC-003。12 个模块里 11 个今天没有目录；若默认「声明即受约束」，交付第一天就是 11 条没人能处理的红线，检查会在第一周被关掉。
- **边界情形**：目录存在但只有 `doc.go` 这类骨架——按定义**算已落地**，因此会被要求接入。这是刻意的：一个已经开始写的模块就该从第一天带上诊断，而不是攒到最后补。作者若确有理由，走 FR-012 的豁免登记，而不是靠「还很空」自动豁免。
- **Alternatives considered**: 用「文件数 > N」当门槛——N 取多少都是任意的，且给了「拆小文件绕过」的空间，否决。

## D3. E2「三类调用点」认什么符号

- **Decision**: 在模块目录的非测试 `.go` 源码里，匹配对以下**已导出**公共入口的调用（`diagnostics.` 限定或点导入后的裸名）：
  - **审计**：`Audit`、`CommitRun`
  - **技术日志**：`Technical`、`SlogHandler`、`LogBuffer`
  - **trace**：`Child`、`Pack`、`Unpack`、`DecodeQueuedEnvelope`
  三类满足**任意一类**即通过 E2。
- **Rationale**: 这些是 Current State §3 核实过的真实公共 API。只要一类即可，是因为不同模块的诊断重心本就不同——一个只读模块可能永远不写审计，强制三类齐全会逼出假调用。
- **刻意排除 `NewID` 与 `Sanitize`**：它们是工具函数，用了不代表接入了诊断。E2 的存在价值就是把「import 了但只用工具函数」挡在外面。
- **Alternatives considered**: 解析 AST 判定真实调用——Go 的 AST 在 Node 侧要自己写解析器；既有检查器对 Go 也是走词法扫描（`extractGoImports` 的注释/字符串跳过逻辑）。沿用同一水位，并在合同里写明「静态检查只证明痕迹存在，不证明语义正确」。

## D4. 豁免登记

- **Decision**: 放 `scripts/diagnostics-contract.json`（与 `content-boundaries.json` 同目录），形如 `{"version":1,"exemptions":[{"module":"...","reason":"...","where":"...","expires":"YYYY-MM-DD"}]}`。**四个字段都必填**，缺任一项则配置本身判为无效、检查失败。
- **Rationale**: 不碰 `scripts/content-boundaries.json`（ARCH-01/02 的交付物）。必填 `reason` 与 `where` 是为了让豁免在 PR diff 里**读起来像一个决定**，而不是一行静默的白名单。
- **到期日期为什么必填**（主任务 2026-09-14 补充）：一条没有期限的豁免就是永久豁免，而永久豁免与删掉检查的效果完全相同——豁免出口存在的意义是给合理的例外一条**临时**通道，不是给它一张免死金牌。因此缺 `expires` 不是「用默认值」而是**配置无效**，已过期则停止生效并在失败输出里点明过期，与「缺证据」区分开：两者要补的东西不同。
- **Status**: 主任务 2026-09-14 确认。
- **Alternatives considered**: 命令行开关 / 环境变量——会被塞进 CI 脚本等于静默关闭，否决。

## D5. 合同文本放哪、写成什么

- **Decision**: `docs/development/diagnostics-onboarding-contract.md`，与 `diagnostics-acceptance-mapping.md` 并列。每个诊断面一行三列：**要做什么 / 公共入口 / 怎么算做到了**。第三列必须是可判定的措辞。
- **Rationale**: FR-002。「怎么算做到了」这一列是合同与愿望清单的分界线——写不出这一列的条目，就说明那一项还没想清楚。
- **错误码枚举那一行**：第二列写「**暂无公共入口**，`log.go` 的 `codes` 未导出」，第三列写「向诊断包申请导出，导出前由人工审查」，并注明导出是后续任务（clarify FR-003）。

## D6. 「模拟不代替真实通过」怎么落地

- **Decision**: 三处同时落：
  1. 合同正文一节，把证据分为「模拟可得」与「仅真实执行器可得」两栏，后者在执行器禁用期间一律填「未执行（constitution 原则 IX）」；
  2. PR 模板一条勾选项，措辞不给「全绿」留口子；
  3. mapping 的 D13-V12 行注明第二条子句不可能通过，因此该项**不标整体通过**。
- **Rationale**: FR-013、FR-014、FR-017。三条子句状态不同，合在一行会让「模拟全绿」被读成「V12 通过」——这是三条里唯一**现在就可能被误读**的。
- **静态检查不介入**：没有任何静态手段能判断「这次是真的跑了执行器」。检查脚本对此**不做判定**，合同里写明这一条靠人工与记录，不靠脚本。

## D7. CI 接入

- **Decision**: 在 `.github/workflows/loretide-content.yml` 既有两条边界检查步骤之后，新增一条 run 步骤。**`on:` 段一字不动**（FR-009、SC-005 用逐字节比对钉住）。
- **Status**: 主任务 2026-09-14 确认——在**现有 job 内**新增一条 `run: pnpm check:diagnostics-contract`。
- **Rationale**: 时机论证见 plan.md 的 Complexity Tracking。

## D8. 测试落点

- **Decision**: `scripts/check-diagnostics-contract.test.mjs`，`node:test`。用例：
  - 正例：`diagnostics` 模块（真实读盘那份配置）通过；
  - 负例 ×3：分别只缺 E1 / E2 / E3，各自断言**报出的是对应那一条**，而不只是「失败了」；
  - 沉默：未落地模块不产生输出；
  - 豁免：登记后通过；登记缺字段则配置无效并失败；
  - 只用工具函数（`NewID`）不算 E2。
- **Rationale**: SC-002 要求缺项粒度可验证，所以负例必须断言**错误文本里的那一条**。SC-004 要求变异验证——删掉任一条规则后，对应负例必须变红。
