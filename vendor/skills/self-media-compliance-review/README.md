<div align="center">

# self-media-compliance-review

**自媒体视频发布前的违规风险审核 Skill。**

适用于 Claude Code / Codex / OpenClaw / Hermes 等支持 Skills 的 agent。把它装进 agent 后，可以在短视频、切片、封面、标题、字幕、口播、商品链接和发布文案交付前，按本仓库整理的参考规则做一遍结构化风险审核。

它不是“敏感词表”，而是一套面向发布前质检的审核流程：会检查画面、声音、文字、封面、评论引导、带货信息、资质、授权、引流和平台常见风险，并给出具体修改建议。

也欢迎大家在 [Issues](https://github.com/JuneYaooo/self-media-compliance-review/issues) 里提供各平台规则资料、审核经验、踩坑案例和整改思路。后续会不定期整理进这个 skill。

[![GitHub stars](https://img.shields.io/github/stars/JuneYaooo/self-media-compliance-review?style=flat)](https://github.com/JuneYaooo/self-media-compliance-review/stargazers)
[![License](https://img.shields.io/badge/license-MIT-blue.svg)](./LICENSE)
[![Codex Skill](https://img.shields.io/badge/Codex-Skill-black.svg)](./SKILL.md)
[![Platforms](https://img.shields.io/badge/platforms-6%20platforms-orange.svg)](#-已覆盖平台与电商专项)

</div>

---

## 能做什么

- **按平台读取参考文件**：根据目标平台读取视频号、微信公众号、抖音、快手、B站、小红书的规则参考。
- **电商合规审核**：商品三一致检查（视频 vs 详情页 vs 口播）、千川低质素材避坑、功效声明核验、行业资质排查、价格/赠品/活动一致性。
- **覆盖常见发布面**：检查脚本、封面、标题、字幕、口播、画面、BGM、评论、商品链接和账号资料。
- **本地视频证据分析**：自动提取首 0–5 秒、场景切换、周期帧、结尾帧、音轨、字幕和技术信号，生成可追溯的视频审核证据包。
- **风险分级**：输出 `Pass` / `Low` / `Medium` / `High` / `Blocker`。
- **证据定位**：要求标注视频时间点、字幕行、封面文案、商品链接或待核验证据。
- **修改建议**：给出消音、打码、删改、补资质、补授权、改写话术、移除链接等具体动作。
- **案例排查参考**：对限流、封号、申诉、原创争议、评论灰产等场景给出排查方向和申诉材料清单。
- **本地证据检索**：默认可用，只搜索随 Skill 发布的规则和案例文件，不联网、不消耗 API 额度，并返回可追溯的文件和行号。
- **实时平台证据（可选）**：仅在用户明确要求实时搜索时，通过当前环境已有的平台浏览插件/Skill、浏览器工具或 TikHub 直连 REST API 检索近期案例和讨论；TikHub 不走 MCP。
- **视频证据防幻觉**：分开记录分享文案、平台元数据、实际媒体、OCR/ASR 和多模态模型输出；来源不一致时只报告异常与待核验原因，不武断归因。
- **隐形规则补充**：除官方规则外，整理了各平台创作者实际发布和评论区讨论中的经验样本，用来补充官方没写明的隐形审核尺度（作为症状和争议线索，不当作平台规则）。
- **机器可读输出**：支持 JSON 格式输出，可接入胶囊影院、选品助手和飞书审批流程。
- **稳定的数据契约**：视频证据、商品一致性和最终审核报告均提供带版本号的 JSON Schema。
- **可扩展规则文件**：新增平台时，可以按相同结构补充参考文件。

## 适合哪些场景

| 场景 | 适合程度 | 说明 |
| --- | --- | --- |
| 短视频最终交付前审核 | 适合 | 检查标题、封面、字幕、口播、画面和平台文案。 |
| 视频切片 / B站切片 / 抖音切片 | 适合 | 可接在剪辑和包装之后，作为发布前检查。 |
| 带货视频风险检查 | 适合 | 重点看商品链接、价格赠品一致性、虚假营销、引流、资质、千川低质素材和商品三一致。 |
| 千川投放素材审核 | 适合 | 对照千川低质素材 10 类标准逐项检查，降低素材被拒风险。 |
| 选品前风险排查 | 适合 | 检查商品类目是否需要特殊资质，详情页是否存在高风险声明。 |
| 文案生成前合规 | 适合 | 扫一遍产品卖点，标出禁用词、高风险表达和需要补证据的声明。 |
| 健康、财经、法律等高风险内容 | 适合 | 会标出资质、功效承诺、专业建议和误导风险。 |
| 版权、肖像、隐私复查 | 适合 | 会把授权、来源、肖像和隐私作为待核验项。 |
| 替代人工法务审核 | 不适合 | 本项目是风险控制辅助，不提供法律结论。 |

## 已覆盖平台与电商专项

| 平台 | 规则文件 | 案例/申诉场景 |
| --- | --- | --- |
| 微信视频号 / WeChat Channels | [`wechat-channels.md`](./references/wechat-channels.md) | [`cases/wechat-channels.md`](./references/cases/wechat-channels.md) |
| 微信公众号 / WeChat Official Accounts | [`wechat-official-account.md`](./references/wechat-official-account.md) | [`cases/wechat-official-account.md`](./references/cases/wechat-official-account.md) |
| 抖音 / Douyin | [`douyin.md`](./references/douyin.md) | [`cases/douyin.md`](./references/cases/douyin.md) |
| 快手 / Kuaishou | [`kuaishou.md`](./references/kuaishou.md) | [`cases/kuaishou.md`](./references/cases/kuaishou.md) |
| B站 / Bilibili | [`bilibili.md`](./references/bilibili.md) | [`cases/bilibili.md`](./references/cases/bilibili.md) |
| 小红书 / Xiaohongshu | [`xiaohongshu.md`](./references/xiaohongshu.md) | [`cases/xiaohongshu.md`](./references/cases/xiaohongshu.md) |
| 抖音电商 / Douyin E-Commerce | [`douyin-ecommerce.md`](./references/douyin-ecommerce.md) | [`cases/ecommerce-cases.md`](./references/cases/ecommerce-cases.md) |
| 千川低质素材 | [`qianchuan-low-quality.md`](./references/qianchuan-low-quality.md) | — |
| 电商功效声明 | [`ecommerce-claims.md`](./references/ecommerce-claims.md) | — |

规则来源汇总见：[`docs/sources.md`](./docs/sources.md)。跨平台案例索引见：[`recent-cases-2025-2026.md`](./references/recent-cases-2025-2026.md)。

## 安装

把下面这段话发给你的 AI 助手即可：

> 帮我安装 self-media-compliance-review：
> https://github.com/JuneYaooo/self-media-compliance-review

安装方式取决于你正在使用的 agent。安装完成后，让它告诉你如何调用，以及是否需要重启或刷新当前 session。

更新仓库后，应重新执行同一种安装流程来刷新已安装副本。很多 agent 会在 session 启动时缓存 Skill 指令，所以更新完成后通常需要新开一次 session。

## 怎么用

直接用自然语言调用：

> 使用 self-media-compliance-review，审核这个视频的标题、封面、字幕、口播、商品链接和发布文案，目标平台是抖音和小红书。

分析本地视频文件：

> 使用 self-media-compliance-review，分析这个本地视频是否可能违规，目标平台是抖音。先提取视频帧、音轨和字幕证据，再给出带时间点的风险结论；如果声音或画面没有审完，不要判定 Pass。

或在视频交付前：

> 交付前跑一遍 self-media-compliance-review，平台是视频号、B站。

排查账号状态或申诉问题：

> 使用 self-media-compliance-review，帮我排查小红书笔记小眼睛为 0 的可能原因，并按案例库给出申诉前要准备的证据。

参考既往案例和创作者讨论：

> 使用 self-media-compliance-review，结合案例库里抖音"带货话术被判违规"的既往经验和评论讨论，帮我看看这条口播会不会踩隐形规则。

电商场景的几种入口：

> 使用 self-media-compliance-review，审核这个带货视频的商品一致性、千川低质素材和功效声明，目标平台是抖音电商。

> 使用 self-media-compliance-review，帮我看看这个品能不能带，有哪些资质要求和风险点。

> 使用 self-media-compliance-review，写文案前帮我扫一遍这个商品的详情页，标出禁用词和需要补证据的卖点。

## 本地视频分析

仓库提供 [tools/analyze_video.py](./tools/analyze_video.py)，用于把本地视频整理成 agent 可以逐项审核的证据包。它依赖 `ffmpeg` 和 `ffprobe`，默认完全在本机运行，不调用 TikHub，也不会搜索平台内容。

证据包包含目标平台、视频元数据和 SHA-256、首 0–5 秒帧、场景切换帧、周期帧、结尾帧、contact sheet、音轨、嵌入/同名字幕、用户单独提供的口播稿或字幕、黑屏和静音区间，以及文本中的功效、绝对化、价格、数据、竞品比较和监管话题预筛信号。所有帧都带时间点，方便在最终报告中引用。

Whisper 转写是可选能力，不是默认依赖。只有明确启用转写并且本机安装了 Whisper 时才会运行；模型可能需要下载，因此 agent 应先说明。没有转写、口播稿或音频审核能力时，报告必须把声音标为“待核验”，不能因为抽样画面正常就判断视频通过。

脚本不会自动给视频定性。它输出的黑屏、静音和文本命中只是预筛；agent 仍需检查 contact sheet 和疑似时间点原帧，结合目标平台规则判断 `Pass/Low/Medium/High/Blocker`。抽帧可能漏掉极短画面，高风险、快切、长视频以及医疗、金融、未成年人等内容应提高采样密度或检查完整时间线。

商品对比和文案扫描也是预筛：它们可以精确比较结构化价格、SKU、赠品和活动字段，或定位可疑声明，但不会自动打开详情页、验证资质、理解全部画面，也不会代替人工给最终结论。所有自动命中都要回到原始证据复核。

抽帧证据不能被描述成“人工逐一看完每一帧”。需要 Gemini 或其他视频多模态模型时，必须提交经过 `ffprobe` 和哈希验证的实际视频文件，保存成功的原始结构化响应，再用原帧、可见字幕和可确认音频交叉核验时间码。模型调用失败时返回的默认结果不构成证据。完整规则见 [`video-evidence-integrity.md`](./references/video-evidence-integrity.md)。

## 默认：本地证据检索

普通审核可以自动搜索仓库随 Skill 发布的 `references/**/*.md` 和 `docs/sources.md`，不需要 TikHub、浏览器、登录状态或额外授权，也不会产生网络请求。适合从多个平台规则和案例文件中查找与“导流、限流、封号、搬运、功效、申诉”等症状相关的段落。

本地检索结果会保留文件路径、行号、章节标题和来源类型，便于复核。它只代表当前仓库中的静态知识，不能冒充实时平台信息；没有命中也不代表内容合规或平台上不存在相似案例。开发机上的 `local/` 历史采集目录不会随 Skill 发布，因此不属于正式检索语料。

## 可选：用户明确要求时实时检索

实时检索默认关闭。即使当前环境已经配置 TikHub key，或者安装了小红书浏览插件、相关 Skill、`agent-browser` 等浏览器工具，也不会因此自动联网搜索。普通审核只使用静态规则、案例库和用户提供的材料。

只有当用户明确提出“实时搜一下”“查近期案例”“看看平台当前讨论”等要求时，agent 才检查当前环境是否存在可用通道：

- 用户指定 TikHub、某个插件或某个 Skill 时，只使用用户指定的通道；不可用时说明情况，不静默换源。
- 用户没有指定通道时，可以优先使用目标平台专用的浏览插件或 Skill；也可以在已经配置并可调用时使用 TikHub，或通过 `agent-browser` 一类真实浏览工具访问公开结果。
- 浏览器通道应遵守登录、访问权限、验证码、频率限制和平台规则，不绕过访问控制。
- 所有实时通道失败时，静态合规审核继续进行；不能把“搜索失败”写成“没有相似案例”。

仓库自带的 [tools/xhs_dynamic_evidence.py](./tools/xhs_dynamic_evidence.py) 是 TikHub 的一个可选小红书适配器，[tools/tikhub/bin/tikhub](./tools/tikhub/bin/tikhub) 则可调用已缓存目录中的抖音和小红书 REST 端点。它们只请求 TikHub 官方 `https://api.tikhub.io/api/v1/...`（中国大陆可显式配置 `.dev` 基础域名），使用 Bearer API key，禁止请求 `mcp.tikhub.io` 或任何 TikHub MCP 工具。调用可能产生 TikHub 费用或消耗账号额度，因此不会作为默认审核步骤。

调用前在进程环境或已被 git 忽略的 `.env` 中设置 `TIKHUB_API_KEY`；中国大陆网络可按需设置 `TIKHUB_API_BASE_URL=https://api.tikhub.dev`。不要把任何密钥写入命令输出、报告、测试夹具或提交记录。

TikHub 返回的原始响应、媒体、签名 URL、日志和报告必须放在 `.gitignore` 已覆盖的本地目录。仓库只提交通用代码、OpenAPI 端点目录、测试、Skill 指令和静态知识，不提交 API key、Cookie、认证令牌、账号安全标识或付费查询结果。

示例：

> 使用 self-media-compliance-review 审核这条笔记，并实时搜索一下近期相似的导流处罚案例。当前有什么可用搜索通道就用什么；如果都不可用，继续静态审核并说明未搜索。

> 使用 self-media-compliance-review 审核这条笔记，只通过 TikHub 搜近期相似案例；TikHub 不可用时不要换成其他来源。

实时帖子和评论必须单独放在“实时平台证据（可选）”中，注明数据通道、搜索词、检索时间、内容 ID 或 URL、发布时间和证据限制。普通创作者帖子和评论区讨论不是平台规则，只能作为症状和排查线索。

建议提供：

- 最终视频路径或可访问链接
- 口播稿、字幕、封面文案、标题、简介、评论区引导
- 商品链接、价格、赠品、活动规则
- 账号身份、资质证明、素材授权说明
- 目标平台和发布场景
- （电商场景）商品详情页截图、SKU 列表、品牌授权、行业资质

## 输出格式

审核报告会尽量按这个结构输出：

```markdown
# 合规风险审核

- 平台: 抖音 / 小红书
- 内容范围: 视频、封面、标题、字幕、商品链接、发布文案
- 结论: High
- 最高风险: 标题和口播存在无法核验的功效承诺

## 风险明细

| 等级 | 平台/条款 | 证据位置 | 风险说明 | 修改建议 |
| --- | --- | --- | --- | --- |
| High | 小红书 商业推广/虚假营销 | 口播 00:12 | 暗示产品有确定功效 | 删除确定性承诺，改成体验描述并补证 |

## 待核验

- 产品资质
- 素材授权
- 商品链接价格和赠品是否一致
```

机器接入请直接使用仓库内的三个版本化 Schema：

- [`compliance-report.schema.json`](./schemas/compliance-report.schema.json)：最终审核报告。
- [`product-consistency.schema.json`](./schemas/product-consistency.schema.json)：商品一致性对比。
- [`video-evidence-manifest.schema.json`](./schemas/video-evidence-manifest.schema.json)：本地视频证据清单。

## 本地开发与校验

仓库不依赖运行时 Python 三方包；测试与静态检查需要 `pytest`、`ruff`，视频测试还需要 `ffmpeg` 和 `ffprobe`：

依次运行 `python3 -m pytest -q`、`ruff check .` 和
`python3 /path/to/skill-creator/scripts/quick_validate.py .`。

CI 会执行同一组 lint、测试和基础 Skill 结构校验。修改 `SKILL.md`、工具或 Schema 后，应先让这些检查全部通过，再刷新 agent 中的安装副本。

## 添加新平台

欢迎直接提 [Issue](https://github.com/JuneYaooo/self-media-compliance-review/issues) 或 PR。适合提供的信息包括：平台官方规则链接、实际违规提示截图、申诉经验、容易误判的内容类型、有效的整改方式。

## 致谢

- [LINUX DO](https://linux.do/)：中文开发者社区，感谢社区里关于 AI agent、Skills、自媒体工作流和内容生产实践的讨论。

## 免责声明

本项目是发布前风险控制辅助工具，不是法律意见，也不能保证平台审核一定通过。平台规则、执法尺度和账号状态会变化，高风险内容发布前应重新核对官方规则，并由负责人做最终判断。

## License

MIT，详见 [LICENSE](./LICENSE)。
