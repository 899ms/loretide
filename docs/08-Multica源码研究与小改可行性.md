# Multica 源码研究与小改可行性

日期：2026-09-12  
状态：源码已下载并定点审阅；未安装依赖、启动服务或运行模型。本文是研究结论，不是应用验收报告。  
后续决定：用户已确认按 Multica 二次开发路线更新文档，当前 v0.4.0 / D-10 见 [09 开发决策](09-Multica二次开发决策与模块映射.md)；本报告保留固定提交的源码证据与限制。  
研究提交：`3551e72e76d2c276e550b668303646d1280fb1e2`  
源码目录：[research/multica](../research/multica)  
上游：[multica-ai/multica](https://github.com/multica-ai/multica/tree/3551e72e76d2c276e550b668303646d1280fb1e2)

## 1. 结论

**Multica 的任务、工作区、Agent 调度和协作基础值得复用。当前已选择复用底座、直接新增内容业务模块、Skill 驱动 AI、插件按需使用。个人原型可先用配置、模板与 Skill 验证；完整 PRD 需要正式内容对象和原生页面。**

当前工程路线为 Go + Next.js；03 已改写技术组合、目录、工作包和命令合同，06 的 Easel/Hermes 工程映射标为历史。内容需求与通用执行原则保留，本轮完成的是文档路线切换，实施代码尚未改造。

当前许可证包含附加条件，直接派生、改品牌或对外服务不是同一件事。详见第 7 节；本轮研究未决定公开部署或商业发行。

## 2. 下载与验证范围

- 使用 `git clone --depth 1` 下载 main 的当时快照，共 5,631 个受版本控制的文件；浅克隆只包含当前提交所需历史，不是完整历史归档。
- 核对 origin、HEAD、干净工作树及 Git 对象连接性；原始仓库不做修改，作为可回看的研究基准。
- 读取根目录 AGENTS.md、CLAUDE.md，检查依赖文件、任务接口、部分 daemon 执行路径、五个重点 Agent 后端、工作区中间件、插件 SDK/存储、Issue 更新和许可证。
- 未进行全仓源码或安全审计，未借用其他目录的旧 Multica 修改、已安装程序或历史测试结果。
- 本机 PATH 找到 Node、pnpm、Go；未找到 Docker、make。这里只检查命令入口，未确认版本满足工程要求；缺少 PATH 入口不等于机器所有位置都未安装。

## 3. 实际技术栈与复用位置

| 层 | 当前源码证据 | 对本项目的意义 |
|---|---|---|
| 后端与 daemon | [server/go.mod](../research/multica/server/go.mod:1)：Go 1.26.6，Chi、pgx、WebSocket 等 | 沿用现有 Go 服务与任务执行，不继续按 FastAPI 模块路径实施 |
| Web | [apps/web/package.json](../research/multica/apps/web/package.json:25)：Next.js 声明 ^16.3.4；[共享依赖](../research/multica/pnpm-workspace.yaml:29)：React 19.2.3 | 沿用 Web 与共享视图，不把 Vite 当 Web 主框架 |
| 工程 | [package.json](../research/multica/package.json)：Node ≥22、pnpm 10.28.2、Turborepo | 已有单仓包边界、测试与构建命令；需实际环境验证 |
| UI 与业务 | [CLAUDE.md](../research/multica/CLAUDE.md)：core / ui / views 分层，Web/Desktop 共享 | 内容视图可放入 packages/views，数据访问放入 packages/core |
| 插件 | [plugin-sdk README](../research/multica/packages/plugin-sdk/README.md)、[协议](../research/multica/packages/plugin-sdk/protocol.ts:21) | 可先增加素材/IP/审核辅助面板；协议 v2，有受限宿主桥接 |

依赖声明与锁定文件属于已下载证据；本轮没有安装依赖，不宣称本机已运行上述版本。

## 4. 任务执行链已经有什么

源码能追到以下链路：

```mermaid
flowchart LR
  A[工作区内的任务 / 会话] --> B[TaskService 与持久化队列]
  B --> C[daemon 领取运行]
  C --> D[ResolveBackend 选择 Agent]
  D --> E[Backend.Execute]
  E --> F[Session.Messages / Result]
  F --> G[完成状态 / 评论 / 聊天结果 / 附件关联]
  G --> H[本项目新增：校验并保存正式作品版本]
```

| 能力 | 读取到的实现 | 结论 |
|---|---|---|
| 统一 Agent 接口 | [agent.go:18](../research/multica/server/pkg/agent/agent.go:18)：Backend.Execute；[Result](../research/multica/server/pkg/agent/agent.go:235)包含输出、会话、状态、用量 | 可复用执行抽象，不需重新规定每个客户端如何输出 |
| 运行时选择 | [SupportedTypes](../research/multica/server/pkg/agent/agent.go:346)、[工厂](../research/multica/server/pkg/agent/agent.go:414)、[ResolveBackend](../research/multica/server/pkg/agent/builtin_runtimes.go) | Pi、OpenCode、Codex、Claude、Hermes 均有后端；可配置扩展基于已支持协议家族，不能把任意命令填进去就保证兼容 |
| 任务领取 | [ClaimTasksByRuntime](../research/multica/server/internal/handler/daemon.go:1694)、[TaskService](../research/multica/server/internal/service/task.go:3815) | 已验证代码存在设备身份和任务领取检查；还需实测完整权限矩阵 |
| 执行与结果 | [daemon.go:8474](../research/multica/server/internal/daemon/daemon.go:8474)、[Execute](../research/multica/server/internal/daemon/daemon.go:9181)、[结果等待](../research/multica/server/internal/daemon/daemon.go:9533) | 有选择后端、执行、事件处理、失败/取消等基础 |
| 回写成果 | [CompleteTask](../research/multica/server/internal/service/task.go:4243)、[评论回写](../research/multica/server/internal/service/task.go:4364) | 能完成任务、回写评论或聊天结果；这不是本项目正式作品 Revision/ReviewSnapshot 的直接替代 |
| 工作区权限 | [workspace.go](../research/multica/server/internal/middleware/workspace.go:162) | 有成员/角色检查和任务 token 的空间绑定；值得保留，不等于文件系统沙箱 |

已看到的具体协议：Codex 使用本机 app-server stdio（[codex.go:338](../research/multica/server/pkg/agent/codex.go:338)）；Claude 使用 stream-json（[claude.go:714](../research/multica/server/pkg/agent/claude.go:714)）；OpenCode 使用 run JSON（[opencode.go:69](../research/multica/server/pkg/agent/opencode.go:69)）；Pi 后端入口见 [pi.go:190](../research/multica/server/pkg/agent/pi.go:190)；Hermes 后端见 [hermes.go:267](../research/multica/server/pkg/agent/hermes.go:267)。这些是静态源码证据，未做本机登录与调用验证。

## 5. 小改、扩展和必须重做的边界

| 内容工作台要求 | 可复用部分 | 仍需完成 | 改动性质 |
|---|---|---|---|
| 隔离空间与项目 | Workspace、成员、项目、任务、接口空间校验 | 内容空间入口、IP 归属与检索/事件验收 | 配置与业务适配 |
| 选题、研究、创作任务 | Issue、分配 Agent、状态、标签/属性、日程字段 | 选题模板、阶段输入输出、来源与作品关联 | 适量扩展 |
| IP 定位与风格 | Skill、项目材料、插件面板 | 版本化 IP 档案、可采用结论、历史引用 | 新业务模块 |
| 网页/文件/速记知识库 | 附件、文本、项目材料 | 来源快照、解析去重、中文检索、知识卡、来源级引用 | 新业务模块 |
| 正式作品与多平台版本 | 任务正文、评论、附件、编辑组件 | 不可变正文版本、WorkingCopy、差异/采用、渠道稿、审核快照 | 新核心业务模块 |
| 人审与人工发布交接 | 任务状态、提醒、成员操作、插件 | 服务端人类主体校验、版本绑定审批、下载包及人工登记 | 新业务规则与界面 |
| 人工指标与 AI 复盘 | 任务运行、表单、评论、统计基础 | 独立人工数据表、统计口径、复盘证据、采纳长期记忆 | 新业务模块 |
| 多 Agent 与模型切换 | 五类目标 Agent 后端、模型配置、会话引用 | 对本项目成果 schema、切换交接、迟到结果的验证 | 适配与回归 |
| Gemini CLI | 当前 SupportedTypes 中没有 gemini；[历史移除迁移](../research/multica/server/migrations/126_runtime_profile_drop_gemini.up.sql) | 独立 CLI 适配及端到端验证；其他 Agent 调用 Gemini 模型不等于 Gemini CLI | 新适配器 |
| 直连模型 API 完成任务 | [llm/client.go](../research/multica/server/pkg/llm/client.go:1)已有 API 客户端封装 | 它当前是标题/追问辅助层，需新增队列执行、受控工具循环、空间连接权限与输出契约 | 不是只填 Key 的改动 |
| 强文件/进程隔离 | 独立执行目录、部分状态目录、任务 token | 符合本项目要求的执行边界与全部目标客户端验证 | 必须专门实现 |
| 移除 OpenClaw | 运行时清单和工厂可定位 | 去除其后端、配置字段、发现/安装路径、UI 和相关契约；保留历史来源证据 | 代码清理与回归 |

Issue 更新中的 [revision 递增与 expected_revision 条件](../research/multica/server/pkg/db/queries/issue.sql:245)用于并发更新校验。读取到的路径不足以支撑“不变作品历史版本、批准快照和引用追溯已完成”的结论；不要只把 Issue 更名为作品就宣称满足 R-031～R-036。

## 6. 插件是受开关控制的候选扩展，不能默认已开放

2026-09-12 补充核验：此前仅根据 SDK 文档提出“插件优先”，没有核对用户入口的发布条件，表述过满。在本报告固定提交中，[settings-page.tsx:69](../research/multica/packages/views/settings/components/settings-page.tsx:69)读取 `plugins_v1`，回退值为 false；[206 行](../research/multica/packages/views/settings/components/settings-page.tsx:206)只在开关开启时展示 Settings → Plugins。[后端 keys.go:21](../research/multica/server/internal/featureflags/keys.go:21)注明插件目录和生命周期管理 API 仍受内部试用开关控制，[57 行](../research/multica/server/internal/featureflags/keys.go:57)同样默认关闭。源码已有实现，不等于用户所用版本、部署或账号已经开放。

开关开启后，[plugins-tab.tsx:671](../research/multica/packages/views/settings/components/plugins-tab.tsx:671)限制 owner/admin 管理插件，其他成员只读；角色限制不等同于入口开关。没有检查用户当前运行实例或更改开关。官方当前 [Skills 文档](https://multica.ai/docs/skills)明确展示的是可复用的 SKILL.md、脚本、模板与参考材料，其导航未列出 Plugins；仅凭这一点也不能推断所有部署都没有插件。

修正建议：先用已开放的任务、Skill 和模板验证内容流程；平台插件作为需要启用并实测的开发选项。Skill 用于 Agent 的工作方法，平台插件用于扩展工作台面板、宿主操作等，两者不能互相替代。本文其他表格提到的插件复用能力均受此条件约束。

可选插件适合素材侧栏、IP 档案摘要和人工审核辅助 UI。研究/创作 Skill 独立管理，不需要包在平台插件中；今日页、作品编辑、版本、审核和真实记录采用原生内容模块，不能依赖插件开放。

限制来自具体源码：插件表面运行在受限 iframe 中，调用宿主 API 受已授予 scope 和当前用户权限共同限制；不能把插件面板权限当作 Agent 权限。插件发布版本不可变，是插件包版本，不是作品版本。见 [插件 SDK](../research/multica/packages/plugin-sdk/README.md)。

[plugin_storage.go:27](../research/multica/server/internal/service/plugin_storage.go:27)定义单值 100 KiB、键数 1,000、总量 5 MiB 的存储限额。它适合设置、小型业务状态或原型，不宜把整个素材库、媒体与完整作品历史都塞入键值存储。

正式作品、来源、审核、人工观测与经营记忆由 Multica Go 后端新增内容模块管理，前端新增原生页面；当前不另建独立内容服务。可选插件调用相同领域接口，不能复制另一套业务规则。具体设计沿用当前仓库迁移规则：新表不加数据库外键、关系与清理在应用事务校验，索引按仓库要求并发创建。

## 7. 许可证是实际采用条件

当前 [LICENSE](../research/multica/LICENSE:1)是 **Multica License：Apache-2.0 文本加 Part I 附加条件**，不能只引用其中 Apache 部分。已同步核对 [指定提交的官方原文](https://raw.githubusercontent.com/multica-ai/multica/3551e72e76d2c276e550b668303646d1280fb1e2/LICENSE)。

按文本明确区分：单一组织内部使用无需商业许可；向第三方提供托管服务（含免费服务）或把代码嵌入商业分发产品，受商业许可要求约束。使用派生 UI 时，改动 Multica 名称、LOGO 和界面版权信息需要书面品牌豁免；只用后端/daemon/CLI 也有保留声明与用户文档归属要求。商业许可和品牌豁免是不同授权。

因此，“先下载研究、做内部流程验证”和“换自己品牌对外提供产品”是不同阶段。小改研究可以推进，但不能默认把该源码当作可随意去品牌、对外托管的宽松许可底座。本轮未移除任何声明、联系权利人、发布或对外提供服务。

## 8. 认证与隔离：不能原样套用之前的安全结论

本项目现有约束包括不复制客户端登录凭据及限制 Agent 文件访问。Multica 的实际实现有差异：

- [codex_home.go:17](../research/multica/server/internal/daemon/execenv/codex_home.go:17)把 auth.json 列为共享链接文件；[准备流程](../research/multica/server/internal/daemon/execenv/codex_home.go:216)建立每任务配置区中的链接；[Windows createFileLink](../research/multica/server/internal/daemon/execenv/codex_home_link_windows.go:25)在符号链接失败时回退到复制。
- [claude.go:720](../research/multica/server/pkg/agent/claude.go:720)使用 bypassPermissions；[OpenCode 启动](../research/multica/server/pkg/agent/opencode.go:69)也带自主执行参数。源码和[安全文档](../research/multica/apps/docs/content/docs/security-model.mdx:12)不能支持“默认全部运行时都是强沙箱”的说法。

这些是读取仓库代码得到的事实，本轮没有读取、链接或复制用户真实凭据。采用 Multica 时须改造上述行为并实测，或明确重新定义产品约束；不能把“调用官方 CLI”描述成“宿主绝不接触认证文件”。之前仅依据架构文档作出的泛化安全表述，以本节源码证据修正。

## 9. 建议的最小改造顺序

1. 保持当前研究 checkout 原样，记录许可证、提交与依赖；在独立实施目录建立派生工程后再改代码。先验证 Windows 启动及数据库/附件恢复，避免接触旧项目的 Multica 实例。
2. 复用 Workspace / Project / Issue / AgentRun；添加一条“选题 → 指派 Agent → 候选内容 → 人工查看”的内容模板和轻面板。可用替身执行器先验证契约。
3. 增加最小 Work / Revision / ReviewSnapshot：接到输出即生成候选版本，人审绑定快照，导出后人工发布。此步完成才能称为作品闭环。
4. 增加人工 PublicationRecord / Observation，AI 只读真实记录并生成 Retrospective；采纳结论进入独立经营记忆。
5. 补来源知识库、IP 版本、API 执行器、Gemini CLI 及其他交付路径；贯穿落实隔离、认证边界与 OpenClaw 移除。可替换架构与完整支持清单分开验收，不能以首个执行器通过代替全部交付。

第一阶段可以是轻改原型；完整目标包含新增领域数据、权限和运行策略。用户随后已确认 Multica 二次开发蓝图，旧工程路线已在 v0.4.0 文档中替换；实际工程和验收仍待实施，不能混用两套目录与技术栈。

## 10. 后续验收证据

最小闭环需留下：实际提交、依赖版本、空间 A/B 越权证据、真实 Agent 调用与归一化输出、作品版本及审核绑定、人工发布和指标登记、AI 复盘引用、失败/取消/重复提交验证。完整交付还需逐执行器和多供应商 API 的切换与隔离测试。

本轮已完成的是下载、Git 校验和定点源码研究；以上应用验收全部待执行。
