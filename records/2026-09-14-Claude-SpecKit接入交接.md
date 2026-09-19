# Claude 交接：Loretide 增量接入 Spec Kit

日期：2026-09-14。用户要求：依据全量评估，为 Claude 编写交接文档。

## 任务目标与授权边界

在已有 Loretide 工程中增量接入 Spec Kit，沿用现有需求、架构和协作流程，不重建工程、不重写已有业务代码。先校准规范，再交付接入底座，后续从一个新功能试点开始。

本文件是任务准备与实施交接，不代表已创建 Issue、已领取、已初始化正式工程或已获准合并。本次主任务仅生成此文件，没有向 Claude 自动发消息。Claude 收到后先只读核对现有任务：只领取 ready、无负责人、依赖已关闭且范围明确的对应 Issue；不存在合适 Issue 时，准备任务正文和文件范围交回主任务登记，不自行绕过领取流程开始改动。

## 先读这些资料

1. [完整评估及11项发现](F:/GJ/内容创作工作台/records/2026-09-14-SpecKit全量接入评估.md)。以这份报告取代先前简评中被纠正的判断。
2. [协作规范](F:/GJ/内容创作工作台/CONTRIBUTING.md)、当前 checkout 的 AGENTS.md。
3. [开发基线](F:/GJ/内容创作工作台/docs/11-首版开发基线与实施清单.md)，以及同目录 docs/04、docs/12、docs/13、docs/design/README.md。
4. [应用说明](F:/GJ/内容创作工作台/app/LORETIDE.md)、[Claude 架构指导](F:/GJ/内容创作工作台/app/CLAUDE.md)、[现行测试政策](F:/GJ/内容创作工作台/app/docs/development/README.md)、[应用协作规范](F:/GJ/内容创作工作台/app/docs/development/ai-collaboration.md)。
5. [全局安装记录](F:/GJ/内容创作工作台/records/2026-09-13-SpecKit全局安装.md)。

以上绝对路径供当前机器阅读。交付到仓库的文档须使用可移植路径/固定提交链接；其他机器拿不到本地评估文件时，由主任务提供文件，不要假称已读。

## 仓库与基线

- GitHub：<https://github.com/899ms/loretide>，私有仓库。
- 文档 checkout：`F:/GJ/内容创作工作台`，分支 main；交接时本地 HEAD 为 `56c0264f0f5e5f715c743b9964baa8f0f7a75d38`。
- 应用 checkout：`F:/GJ/内容创作工作台/app`，分支 app-main；交接时本地 HEAD 为 `13a0f50bd75c9ba1817356e46b2f700424de5a40`。
- 两分支是同一远程仓库的独立历史。文档 PR 目标 main，应用配置 PR 目标 app-main；严禁相互合并历史。
- 执行时重新确认远端、SHA、Issue 状态和已有 PR，不能把上述时间点当作永久最新状态。

原目录有用户未提交修改：根目录 docs/11、tasks/plan、派发记录及多份未跟踪记录；app 有营销首页删除/新入口、next-env.d.ts 和 .agents/worktrees/。全部保留，不 reset/clean、不整目录暂存或覆盖，不借接入收走无关文件。

独立 worktree 从已确认的目标分支提交建立。分支名称优先遵守所领取 Issue 的明确要求；未指定时可用 `codex/issue-<编号>-spec-kit`，不得伪造 Issue 编号。两阶段属于不同历史，使用各自的 Issue、worktree 和 PR。

## 不可违反的规则

- 禁止编写或运行 UI 单元测试，**本地和 CI 均适用**；禁止通过测试扩展名/分类变化绕过。
- 禁止 computer use、浏览器自动点击验收。涉及 UI 时提供受影响界面和具体手动 Todo，由用户验证；未验证不能填通过。
- 保留所改范围需要的非 UI 契约、Go、模块边界、类型检查与构建；不要运行可能包含 UI 测试的全量测试命令。
- 保持真实 AI 执行器禁用，不读/复制/上传模型客户端凭据，不启用真实模型、发布、业务库迁移或服务重启。
- 不修改生产模块、依赖锁文件、现有测试源码或 CI workflow 来完成本次配置接入；若确需新增非 UI 配置检查，先在对应 Issue 明确文件范围。
- 一 Issue 一执行者一独立 worktree。领取后重读确认唯一所有者；共享 GitHub 账号的 assignee 不替代执行者/实际会话标识，无法获得真实会话 ID 就如实说明，禁止编造。
- 交付 Draft PR，正文用 `Related to #编号`，置 review-needed；由主任务审查、合并及关闭。不要使用自动关闭关键字。

## 已验证的工具事实

- Specify CLI 已全局安装：`C:/Users/13900/.local/bin/specify.exe`，版本1.0.6，来源 spec-kit v1.0.6，提交 `96c9bd657bfd5de0d651a6165084932b7304ac99`。先运行版本检查，版本不同先说明，不自行升级。
- 临时试验目录：`C:/Users/13900/AppData/Local/Temp/loretide-speckit-assessment-20260914`。其中有试验规格和临时 Git 元数据，不可整目录复制进应用工程。
- 骨架生成41个文件；6个 PowerShell 文件语法解析通过，7个 JSON 解析通过。
- Codex skills 与 Claude skills 双集成成功；integration status 为 OK，managed modified/missing/invalid/unchecked 均0。这不等于真实 Agent 会话已加载成功。
- **1.0.6 核心 create-new-feature.ps1 不创建或切换 Git 分支**。临时实测生成规格后原 task/issue-* 分支未变。不要为不存在的默认分支切换问题改脚本。
- FEATURE_DIRECTORY 环境变量和本地 feature.json 用于定位规格；旧 SPECIFY_FEATURE 值可以与目录不一致。PathsOnly 成功只表示路径解析，不证明文件存在或授权正确。

生成方式供已领取接入 Issue 后参考，须在新建的隔离准备目录或已审查 worktree 使用，禁止直接对共享 app 执行：

```powershell
# 替换为本次独立准备目录；非直接执行到共享 app 的命令。
specify init '<独立准备目录>' --integration codex --integration-options="--skills" --script ps --non-interactive
Set-Location '<独立准备目录>'
specify integration install claude --script ps
specify integration status
```

这是一套已试验选项，不是对已有文件运行 --force 的授权。需要融入现有非空 worktree 时先审查生成差异，只纳入本次批准的文件。

## 阶段 A：校准权威规范（main）

先处理报告 F01～F03：

1. docs/11 的 Tailscale/Linux 日常开发描述与当前 Windows 决策冲突，改为与最新确认一致。
2. docs/04、11、13 对检查器/诊断“尚未实现”的笼统描述需逐项核实。分别记录已实现、已验证、未验收，不能批量宣布完成。
3. 统一协作规则入口，保留根 CONTRIBUTING 的主任务派发前预占与唯一执行者确认要求；明确“浏览器证据”来自用户手验。
4. 保留需求及验收编号，避免产生另一套 PRD。历史记录不改写；仅校准当前有效规范。
5. 根目录未提交的 docs/11 增量不是 main HEAD 的一部分。先只读 diff，取得主任务指定的可复现需求来源后在独立 worktree 局部修订，不将整份脏文件直接搬运或提交。

建议允许范围：CONTRIBUTING.md、上述确需校准的规范文档，以及本 Issue 的 records 文件。实际以领取 Issue 的文件清单为准；不是授权顺手修改整个 docs。

产物：文档 Draft PR、逐项校准依据、仍未验证项。阶段 B 可以准备，但最终来源锁定须等 A 合并后由主任务提供/确认 main SHA。

## 阶段 B：接入底座（app-main）

### 建议文件范围

- `.specify/` 的必要脚本、模板、集成 manifest 和 constitution。
- `.agents/skills/speckit-*/`、`.claude/skills/speckit-*/` 的受控入口。
- `.gitignore` 的局部白名单调整。
- `docs/development/spec-kit.md`、`docs/development/spec-kit-sources.json`（后者是项目自定义清单）。
- AGENTS.md/CLAUDE.md/LORETIDE.md/应用协作入口中必要的局部链接或规则同步，以 Issue 明确范围为准。
- 本次接入 records；用于工具验证的临时规格放临时目录，不伪装成真实业务规格提交。

### 必须落实的适配

1. **来源与原则**：constitution 保存稳定约束和来源关系，不复制全部 PRD。来源清单记录 repo、main 固定 SHA、path、内容哈希；新 app clone 可读取，缺少必要来源时明确停止，不仅写 ../docs。不得提交 token 或私有运行配置。
2. **Claude 文件可分发**：现有 .gitignore 忽略整个 .claude/。仅放行需交付的 skills，继续忽略本地设置；.agents/worktrees 不能被一起提交。用 git check-ignore 和实际暂存清单验证。
3. **规格定位**：每个 worktree 校验真实仓库、分支、Issue、feature ID、目录、基线一致。禁止用户级全局 SPECIFY_FEATURE* 设置；feature.json 保持本地忽略。多个 Issue 共用规格时先发布同一规格基线。
4. **执行限范围**：初期开放规划命令；implement 不能执行整个功能的所有 Issue，必须限定当前已领取任务。若暂不实现这个限制，就明确禁用该执行入口。converge 不得自行扩大范围或声明验收。
5. **Issue 转换暂不开放**：默认 taskstoissues 按 T001 去重，跨功能会碰撞。先沿用主任务手动映射；不必在本次底座中开发完整转换器。以后启用须使用 repo+feature+task 唯一键并补验证。
6. **测试与架构**：模板写入本地/CI UI 禁令、用户手验、现有非 UI 检查及实际 server/apps/web 结构；不保留会诱导重建 src/backend/frontend 的占位方案。真实执行器、模块边界、独立环境规则继续有效。
7. **完成语义**：tasks.md 勾选不能代表合并或用户验收；GitHub 管领取/审查状态，records 存实际证据。不要维护第二套负责人/状态账。
8. **升级**：记录 CLI/生成版本和本地定制清单；升级在临时目录比较后单独 PR，不使用 --force 清掉项目适配。额外 Git/发布/执行扩展本次不启用。

## 验证与交付

仅做与配置相关的非 UI 验证，按完整评估第7节逐项记录：

- PowerShell 语法、JSON 解析、integration status；有意定制导致 modified 时给出清单，不隐瞒。
- 新 clone/独立 worktree 的规范可获取性与固定版本一致性。
- 两种集成的入口文件被跟踪、没有用户本地文件和路径泄漏。
- 正确/错误规格路径、旧环境变量、Issue/task 越界的正反例；不得只用 PathsOnly 成功作为通过证据。
- 双 Agent 入口均保留 UI 禁令和任务边界。未实际在新 Claude/Codex 会话验证加载时明确标未验证。
- 若未开放执行/转换入口，明确记录限制，不为“全功能接入”擅自扩展工作量。
- 本次配置接入通常 UI 影响为无，手动 UI Todo 为无；如果实际改动了 UI，说明超出当前范围并交回主任务处理。

交付格式：

```text
Issue / 唯一执行者 / worktree / 分支：
目标分支及基线 SHA：
Draft PR / 当前 head SHA：
权威文档来源 SHA：
改动文件及用途：
已完成的适配：
实际验证命令、结果及证据：
未验证项、阻塞项和暂未开放入口：
UI 影响：无（或逐项说明）
手动 UI Todo：无（或待用户验证清单）
回滚：只撤销本 Issue 接入提交
待主任务：审查、合并、关闭；执行者不自行操作
```

不要重复索要已有授权，但遇到缺少 ready Issue、依赖规范未合并或没有范围来源时，说明具体缺项。继续可以独立进行的只读准备，不触碰未授权文件。

## 本次会话记录与关联文件

用户在全量评估后要求“写一份交接文档给 claude”。本轮重新确认两处本地 HEAD 和领取流程，创建本文件；没有初始化工程、创建/领取 Issue、派发 Claude 或提交/推送任何文件。

- 本文件：供 Claude 接手时理解目标、约束、阶段、验证与交付格式。
- 完整评估：具体发现、源码证据、临时试验结果和未来验收标准。
- 全局安装记录：工具安装来源；与项目配置接入区分。

本次 UI 影响：无；手动 UI Todo：无。

## 补充：Claude 模型选择建议

用户追问开发使用 Opus 5 还是 Sonnet 5。对本次接入建议使用 Opus 5：工作重点是历史规范冲突、双分支引用、执行授权边界和多 Agent 流程适配，需要综合判断。边界和模板稳定后，明确文件范围的机械修改可使用 Sonnet 5，关键审查仍独立进行。这是结合本项目的选择建议，不是本仓库两模型实测排名，也未替用户切换模型。

已核对官方发布说明：[Opus 5](https://www.anthropic.com/news/claude-opus-5)、[Sonnet 5](https://www.anthropic.com/news/claude-sonnet-5)。不凭型号推断用户订阅额度或实际单任务费用。
