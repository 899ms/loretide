# #232 Web SOP 实现缺口核查与下一批实施卡

## 结论

- **核查基线**：`01618357fb665fbf99412b696248b85bd846a229`（`feat(content): 031 PR 2 — 历史导入页面 (#228)`）。这是 Issue #232 指定的 `origin/app-main` 基线；本地的 `app-main` ref 当时落后 4 个提交，因此本记录始终以该 SHA 而非本地 ref 名称判断。
- **本轮真实 Web 实现缺口**：1 项，见「下一批实施卡」。它是已有选题卡正文的编辑入口，规模 **S**，不依赖真实执行器。
- **不创建卡**：自动候选/运行/AI 改写或复盘、平台直连、解析和文件能力均已被规格明确搁置；不能把它们当作本轮缺陷。既有手动清单尚未完成的项目属于“已实现，待人工验证”，不等同于代码缺失。
- **本轮执行边界**：只读取文档、规格、路由、API 客户端与页面/模块符号；未连接本地数据库、未执行迁移/服务/真实执行器、未运行 UI 测试或 computer use。

## 依据与方法

权威需求为文档仓库的 `docs/01-完整工作流.md`、`docs/11-首版开发基线与实施清单.md`、`docs/12-模块边界与变更回归约束.md`、`docs/13-完整开发诊断与操作日志需求.md` 和 `docs/design/README.md`。本次仅覆盖 `docs/01` 的手工 SOP 及 `docs/11` 已解锁 Web 范围，并逐项查看 `specs/021` 至 `specs/031`、当前路由与客户端/页面符号。

“实现”只表示当前基线存在可达的页面/客户端/API 路径和对应合入 PR；不表示浏览器手验、真实数据或 Agent 路径已经验收。已有 `manual-ui-todo.md` 的未执行项仍由用户手验，特别是 Issue #211/#220 的既有待验范围，本卡不重开也不替代。

## SOP 矩阵

| SOP 要求与规格 | 分类 | 当前基线证据（代码路径 / 符号） | 已合入证据 | 结论 |
| --- | --- | --- | --- | --- |
| §3.1 账号表达配置；`021-account-expression-profile` | 已实现，待人工验证 | `apps/web/app/[workspaceSlug]/(dashboard)/accounts/page.tsx` → `AccountSettingsPage`；`packages/views/content/ip-profile/index.tsx` 的 `AccountSettingsPage` / `expression-profile.tsx` 的 `ExpressionProfileSections`；`/api/content-accounts` 路由及 `api.createContentAccount` | #100（存储/API）、#120（页面） | 手工录入、确认状态与最小可开始条件已有路径；“Agent 提炼候选档案”不在本期实现。 |
| §3.2 运营规则；`029-operating-rules` | 已实现，待人工验证 | `packages/views/content/workspace-core/index.tsx` 的 `OperatingRulesSections`；`packages/core/api/client.ts` 的 `setContentOperatingRules`；`GET/PUT /api/operating-rules` | #195（存储/API）、#199 与 #216（页面及修复） | 规则设置是品牌级手工路径；调度、团队复核与自动发布不属于本卡。 |
| §3.3 少量历史资产；`031-historical-import` | 已实现，待人工验证 | `apps/web/app/[workspaceSlug]/(dashboard)/historical-import/page.tsx` 的 `HistoricalImportPage` / `runHistoricalImport`；`POST /api/content-works/{id}/artifacts/{artifactId}/versions/import` 与发布记录入口 | #217（存储/API）、#228（页面）、#231（重放修复） | 仅单条粘贴导入；文件、批量、跨作品去重与聚合明确未做。 |
| §4 手工素材输入；`028-source-inbox-manual` | 已实现，待人工验证 | `apps/web/app/[workspaceSlug]/(dashboard)/sources/page.tsx` → `SourceInboxPage`；`packages/views/content/source-inbox/index.tsx`；`/api/content-sources` 的 list/create/organize/revisions | #176（存储/API）、#182（页面） | 粘贴文本与 URL 的手工收件箱已落地；解析、抓取、文件/目录、素材包和来源互联均为明确后续范围。 |
| §5.2 手工选题卡与 §5.3 冻结简报；`022-ep04-topic-brief`、`023-ep04b-start-snapshot`、`030-topic-source-refs` | 已实现，待人工验证；另有 1 个正文编辑缺口 | `apps/web/app/[workspaceSlug]/(dashboard)/topics/page.tsx` → `TopicPlanningPage` / `StartRunSection`；`/api/content-topics`、`/briefs`、`/start`、`/sources`；`packages/core/content/topic-planning` 查询与契约 | #107/#119（022）、#135/#140（023）、#227/#229（030） | 创建、动作、简报、开始快照和两类素材引用可达。已建卡的其余五个正文项无法更新，见下方实施卡。 |
| §7 人工作品、文档和版本；`024-work-editor-manual` | 已实现，待人工验证 | `packages/views/content/work-editor/index.tsx` 的 `WorkSections`；`packages/core/content/work-editor/queries.ts`；`/api/content-works` 及 artifacts/versions 路由 | #145（存储/API）、#152（页面） | 人工写作、存版本、采用/恢复具备入口；AI 改写、diff 渲染、来源侧栏和媒体文件不是本期代码缺口。 |
| §8–§9 人工审核、交接、发布记录；`025-review-delivery-manual` | 已实现，待人工验证 | `packages/views/content/review-delivery/index.tsx` 的 `ReviewDeliverySections`；`/api/content-reviews`、`/api/content-deliveries`、`/api/content-publications` | #150（存储/API）、#161（页面） | 手动审批/交接/发布登记路径存在；平台 API、自动发布、调度、媒体附件和模型审核被明确排除。 |
| §10.1 人工指标和反馈；`027-feedback-manual` | 已实现，待人工验证 | `packages/views/content/feedback-learning/index.tsx` 的 `FeedbackLearningSections`；`packages/core/content/feedback-learning`；`/api/content-metrics`、`/api/content-feedback` | #177（存储/API）、#184（页面） | 人工指标、CSV 文本导入与反馈摘录存在；AI 复盘和 Learning 没有实现且属于执行器后续工作。 |
| §2 / §11 日常工作台；`026-today-dashboard` | 已实现，待人工验证 | `apps/web/app/[workspaceSlug]/(dashboard)/today/page.tsx` 的 `TodayPage` 与七个 `*Section`；`packages/core/today` 派生函数 | #165（页面）、#206（收件箱/入口补充） | 已接入选题、作品、审核、交接、待补录反馈、账号缺口与待整理素材。自动候选推荐未落地，但页面明确说明。 |

## 明确搁置或需决策的范围（不是本轮缺陷卡）

| 范围 | 分类 | 规格依据与当前证据 | 为什么不建本轮实施卡 |
| --- | --- | --- | --- |
| 自动候选选题、每日建议（EP-04c） | 明确搁置；依赖执行器与 W-03 | `specs/023.../spec.md` Out of Scope；`specs/031.../spec.md` Out of Scope；`today/page.tsx` 明示 EP-04c 未落地 | 需要 Agent/资料能力和后续范围裁决，不能用手工卡替代。 |
| 内容生产运行、重试/取消及“运行实际引用快照” | 明确搁置；依赖 `agent-workflow` | `specs/022` FR-018、`specs/023` Out of Scope；当前 `server/internal/content/` 无 `agent-workflow` 模块 | 目前只具备 append-only 的结构保证；行为验收须随运行实体落地。 |
| AI 提炼、改写、审核、复盘和 Learning | 明确搁置；真实执行器禁用 | `specs/021` Assumptions、`024`/`025`/`027` Out of Scope | 该能力会改变权限、执行器与安全验证范围，需单独决策和专项卡。 |
| 网页抓取/解析、文件/目录、素材包、媒体附件 | 明确搁置；属 W-03/解析器 | `specs/028`、`025`、`031` Out of Scope | 当前手工文本/URL/缺失说明是诚实降级，不可视作实现漏项。 |
| 平台登录、自动发布、自动排期/调度、平台数据回流 | 明确禁止或后续 | `docs/11` 首版边界；`specs/025`、`027`、`029` Out of Scope | 不应以“补齐 SOP”名义越过人工发布边界。 |
| 素材反向引用视图、来源间关联、选题引用过去 Learning | 明确搁置 / 依赖未落地实体 | `specs/030` Out of Scope 1/2/7 | 前两项已有明确不做裁决；Learning 尚无实体，需先解决上游依赖。 |

## 下一批实施卡（最多 3 项）

### 卡 1 — S：已建选题卡的正文五项编辑

- **触发与缺口证明**：SOP §5.2 的选题卡需要可维护七项内容。当前 `server/internal/content/topic-planning/store.go` 只有 `SetAccount`、`SetSources` 与四个动作两类 UPDATE；路由只提供 `POST /{id}/account`、`POST /{id}/sources`、`POST /{id}/actions`，没有正文 `PATCH`/`PUT`。`specs/030-topic-source-refs/spec.md` Current State §2 和 Out of Scope 6 将“其余五项的编辑能力”明确记为真实缺口。当前前端 `TopicPlanningPage` 仅有新建草稿编辑，已建卡没有相应 mutation。
- **目标**：新增一个只编辑 `audience_problem_judgment`、`ip_fit`、`timing`、`existing_content_relation`、`evidence_gaps_and_investment` 的受控写入口；不改变账号关联、两类来源引用、状态、动作理由或已冻结的简报/快照。`channels` 和 `recommended_action` 是否与这五项同卡处理属于**需要产品决策**：它们也在创建形状中，但 #030 的“其余五项”措辞未给出更新语义，实施前必须在卡内写明裁决，不能自行扩大 PATCH。
- **预计文件**：`server/internal/content/topic-planning/{contract.go,store.go,*_test.go}`、`server/internal/handler/content_topic.go` 及其测试、`server/cmd/server/router.go` 与路由用例、`packages/core/api/client.ts`、`packages/core/content/topic-planning/{contract.ts,queries.ts,*test.ts}`、`packages/views/content/topic-planning/index.tsx`、`apps/web/app/[workspaceSlug]/(dashboard)/topics/page.tsx`（若组合接口需要），并更新对应规格/手动清单；不改诊断、其他内容模块或 UI token。
- **非 UI 验证**：先写契约/handler 负例，确认跨工作区返回既有 404 形状；确认只允许裁决字段、未知字段拒绝、来源/账号/状态/简报与快照不变；真实路由穿越用例的路径参数与上下文不同；core 的 zod 畸形响应用例、类型检查、边界/诊断合同检查。数据库型验证只在 CI 的隔离合成数据库执行，本机不连库。
- **手动 Todo**：创建一张卡后更新每个允许字段；刷新/重新打开确认保存；再检查来源引用、账号、状态与已经开始的快照未被改写；检查无权限为只读/404；检查四语言、长文本、窄窗口、键盘焦点。零 UI 单测、零 computer-use 验收。
- **依赖与并行**：依赖 #229 已在基线；与诊断任务可并行（但实现必须保留既有 audit/trace/授权合同）；不依赖执行器、平台或 W-03。**需要上述字段范围裁决后才可开工。**
- **禁止项**：不得把正文编辑夹带进四个状态动作；不得改写 append-only 的简报、开始快照或来源引用；不得新增模型调用、真实执行器、平台接口、迁移或独立 UI 组件；不得运行本机数据库、服务、UI 测试或 computer use 作为验收。
- **回滚**：独立实施 PR 可整体 revert；本审计 PR 只需 revert 本记录。

## 未被提升为实现卡的验证缺口

`specs/031-historical-import/tasks.md` 的 T058/T064 与若干旧任务清单仍记录测试/验证未完成；它们没有证明当前功能不存在。#232 的规则禁止本轮运行本地数据库、服务、迁移和 UI 测试，因此本记录把它们保留为“待人工/受控验证”，而不把测试待跑包装成产品缺口。

## 交付与状态

- 本 PR 只新增本记录，没有生产代码、迁移、数据库操作或 UI 测试。
- 建议验证：`git diff --check`，再检查记录中的基线 SHA、路径、路由和 PR 号与本分支一致；不运行受限制的命令。
- Issue #232 完成后按 `CONTRIBUTING.md` 保持 Draft PR：关联 `Related to #232`，移除 `in-progress` 并添加 `review-needed`；不关闭 Issue，也不合并 PR。
