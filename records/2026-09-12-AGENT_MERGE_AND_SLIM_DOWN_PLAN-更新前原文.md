# 两个内容审核开源库的合并与精简执行说明

> 目标读者：负责实施的编码 Agent（Claude Code、Codex、Cursor、OpenCode 等）  
> 目标：将 `self-media-compliance-review` 与 `yuwen-publish-precheck` 合成为一个轻量、可维护、可扩展的统一内容发布审核项目。  
> 重要：不要直接拼接两个仓库的全部文件；先盘点、再映射、再抽取、再重构、再验证。

---

## 0. 任务目标

现有两个上游仓库：

```text
A = JuneYaooo/self-media-compliance-review
B = yuwen-cool/yuwen-publish-precheck
```

将它们收敛为一个新项目：

```text
content-publishing-gate
```

新项目只保留“发布前审核”必需的能力：

```text
1. 事实主张与来源证据检查
2. 合规、夸大、误导、侵权与平台风险检查
3. 编辑质量、品牌/IP 风格与渠道适配检查
4. 最终版本、人工审批和发布就绪检查
5. 结构化审核报告、规则版本和可回归评估
```

新项目不负责：

```text
- 内容生成
- 自动发布
- 账号登录、爬虫、浏览器自动化
- 视频剪辑、图文排版
- 全量知识库/RAG
- 完整项目管理或数据分析系统
- 将审核结果表述为法律意见或“保证合规”
```

目标是产生一个可被 Web App、CLI、MCP Server 或本地 Agent 调用的“审核内核”，而不是再次做一个超长提示词集合。

---

## 1. 合并总原则

### 1.1 不直接合并仓库历史

不要把 A、B 的 Git 历史硬合并，也不要直接把双方根目录文件复制到同一层。

原因：

- 两套 `README.md`、`SKILL.md`、`LICENSE`、规则表述和输出规范可能冲突。
- 相同风险会被重复检查，导致输出冗余、模型成本升高、结论不一致。
- 上游案例、平台规则、词库、模板和第三方材料可能具有不同许可或再分发条件。
- 未来跟踪上游更新会困难。

正确策略：

```text
建立一个干净的新仓库
→ 将 A 与 B 固定为 upstream reference
→ 盘点全部文件与许可证
→ 仅抽取必要能力
→ 统一规则 ID、数据结构、严重等级和输出协议
→ 通过评估集验证行为
```

### 1.2 合并“能力”，不是合并“文档长度”

保留 A 的核心能力：

```text
- 事实 Claim 提取
- Claim 与 Source/Evidence 的绑定
- 证据充分性、相关性、时效性、适用范围检查
- 高风险主张、电商主张、视频证据和平台风险检查
- 结构化合规报告思路
```

保留 B 的核心能力：

```text
- 发布前 Preflight / Checklist 思路
- 自定义 Profile、表达规则和词库模板
- 模板化输出
- 数据、历史、词库、评估、脚本的分层方式
- 项目维护、版本化和回归评估机制
```

删掉或降级：

```text
- 两边重复出现的通用敏感词、绝对化用语与基础文案建议
- 重复的平台介绍与重复案例描述
- 与运行时无关的 Roadmap、贡献指南、维护说明
- 没有具体触发条件的主观“爆款写作”建议
- 不适用于目标渠道/内容类型的规则
- 不能验证来源、不能解释触发条件的静态断言
```

### 1.3 按需加载，不做全量上下文拼装

任何一次审核都只加载：

```text
core rules
+ 当前目标渠道规则
+ 当前内容领域规则
+ 命中的必要案例/词库
+ 用户自己的品牌 Profile / 自定义规则
```

绝不将所有平台、全部历史案例、所有词表、全部 README/SKILL 内容塞入一次模型上下文。

---

## 2. 先做资产盘点

在创建/修改新项目之前，先产出以下文件：

```text
docs/upstream-inventory.md
docs/license-and-attribution.md
docs/rule-mapping.csv
```

### 2.1 上游文件盘点

必须递归列出 A 与 B 的文件，按以下分类写入 `docs/upstream-inventory.md`：

| 分类 | 判断标准 | 处理候选 |
|---|---|---|
| 运行规则 | Agent 判断时直接使用的规则、Skill、schema | 迁移、改写或引用 |
| 数据与案例 | 词库、案例、历史记录、抓取数据 | 审核许可后按需迁移 |
| 模板 | Profile、表达、输出模板 | 合并为新项目模板 |
| 评估 | tests、evals、样例输入输出 | 迁移并统一断言 |
| 工具 | scripts、tools | 只保留可复用且无高风险依赖的部分 |
| 治理文档 | README、Roadmap、Contributing、Security | 新项目重写，不直接复制 |
| 第三方材料 | 平台规则、截图、案例、引用、抓取内容 | 单独记录来源、许可和更新时间 |

已知结构提示：

```text
A:
- SKILL.md
- agents/openai.yaml
- schemas/compliance-report.schema.json
- schemas/product-consistency.schema.json
- schemas/video-evidence-manifest.schema.json
- references/ 下按平台、电商主张、视频证据、直播证据、近期案例拆分
- tools/、tests/、docs/

B:
- SKILL.md
- data/cases/、data/history/、data/wordlists/
- evals/
- templates/expressions.md
- templates/my-rules.md
- templates/profile.md
- templates/词库示例.md
- scripts/、references/
- CHANGELOG.md、ROADMAP.md、SECURITY.md、THIRD_PARTY_NOTICES.md
```

### 2.2 许可证和来源台账

在迁移任何文件之前：

1. 读取两个仓库当前 `LICENSE`。
2. 读取 B 的 `THIRD_PARTY_NOTICES.md`。
3. 检查 README、SKILL、references、data、cases、wordlists 是否另有许可、署名、非商用或再分发限制。
4. 对每个迁移文件记录：来源仓库、来源路径、上游 commit SHA、原许可证、是否修改、是否需要保留署名、是否可再分发。
5. 不确定的案例、截图、平台材料、抓取数据，默认不迁移到公开发行包；可保留 URL 与元信息，运行时按授权策略获取。

`docs/license-and-attribution.md` 最少包含：

```markdown
| New path | Upstream | Upstream path | Commit SHA | License | Modified | Attribution required | Distribution decision |
|---|---|---|---|---|---:|---:|---|
```

没有完成该台账，不得宣布项目可公开发布。

---

## 3. 新项目目录结构

创建下面的目标结构。除非有明确需求，不要增加额外顶层目录。

```text
content-publishing-gate/
├── README.md
├── SKILL.md
├── LICENSE
├── CHANGELOG.md
├── pyproject.toml                 # 或 package.json；仅选择一个主运行时
├── docs/
│   ├── architecture.md
│   ├── upstream-inventory.md
│   ├── license-and-attribution.md
│   ├── policy-sources.md
│   ├── rule-mapping.csv
│   └── migration-decisions.md
├── schemas/
│   ├── review-request.schema.json
│   ├── claim.schema.json
│   ├── evidence.schema.json
│   ├── finding.schema.json
│   ├── review-report.schema.json
│   └── release-readiness.schema.json
├── rules/
│   ├── core/
│   │   ├── evidence-and-claims.yaml
│   │   ├── compliance-and-risk.yaml
│   │   ├── editorial-basics.yaml
│   │   └── release-readiness.yaml
│   ├── channels/
│   │   ├── xiaohongshu.yaml
│   │   ├── wechat-official-account.yaml
│   │   ├── douyin.yaml
│   │   ├── wechat-channels.yaml
│   │   ├── bilibili.yaml
│   │   └── kuaishou.yaml
│   ├── domains/
│   │   ├── ecommerce-claims.yaml
│   │   ├── technical-performance.yaml
│   │   ├── video-evidence.yaml
│   │   ├── live-commerce.yaml
│   │   └── high-sensitivity.yaml
│   └── user/
│       └── .gitkeep
├── templates/
│   ├── profile.md
│   ├── custom-rules.md
│   ├── expressions.md
│   ├── wordlist.example.yaml
│   └── publish-handoff.md
├── cases/
│   ├── curated/
│   └── private/.gitkeep
├── evals/
│   ├── fixtures/
│   ├── expected/
│   ├── regression/
│   └── README.md
├── scripts/
│   ├── validate_schemas.*
│   ├── validate_rules.*
│   ├── run_evals.*
│   └── sync_upstream_metadata.*
└── src/
    ├── rule_loader.*
    ├── claim_extractor.*
    ├── review_orchestrator.*
    ├── finding_deduper.*
    ├── readiness_gate.*
    └── report_renderer.*
```

说明：

- `rules/` 只存可执行且可定位的规则，禁止放大段散文式说明。
- `cases/` 只存已核准可再分发、对规则判断有用的精选案例；大规模历史抓取数据不进默认安装包。
- `rules/user/` 只用于用户自己的品牌禁区、术语、禁用表达和渠道特例，默认不提交真实客户数据。
- `docs/` 与运行时上下文隔离；不要让 Agent 每次加载治理文档。

---

## 4. 规则合并与去重

### 4.1 固定四层规则架构

将所有可运行规则映射到以下四层；不得创建重叠含义的第五层。

```text
L1 Evidence & Claims
L2 Compliance & Risk
L3 Editorial & Channel
L4 Release Readiness
```

| 层级 | 负责内容 | 上游主要来源 | 默认输出 |
|---|---|---|---|
| L1 | 事实、数据、引用、测试、适用范围、来源时效、视频证据 | A | BLOCK/WARN/PASS |
| L2 | 夸大、绝对化、功效、比较、促销、侵权、敏感与平台风险 | A + B | BLOCK/WARN/INFO/PASS |
| L3 | 标题正文一致、表达、占位符、品牌/IP 风格、渠道规格 | B + A 的渠道材料 | WARN/INFO/PASS |
| L4 | 最终版本、审核、豁免、负责人、素材、排期、交接 | B | BLOCK/PASS |

### 4.2 统一规则格式

所有迁移后的规则必须转换为 YAML 或 JSON，并至少包含以下字段：

```yaml
rule_id: EVIDENCE_SCOPE_001
layer: L1
name: 结论超出证据适用范围
purpose: 防止把单一案例、局部测试或供应商材料扩大成普遍结论
applies_to:
  content_types: [article, post, short_video_script, product_copy]
  channels: [all]
  domains: [technical-performance, ecommerce-claims]
trigger:
  claim_types: [performance, efficacy, comparison, numeric]
  indicators: [显著降低, 更省, 更稳定, 一定有效]
required_evidence:
  - direct_source
  - scope_or_conditions
severity:
  default: warn
  upgrade_to_block_when:
    - no_source_attached
    - high_sensitivity_domain
    - commercial_claim
fix_guidance: 补充来源、测试条件、对照组、样本范围和适用边界；或收窄结论。
source_registry_ids: [SRC-A-TECH-001]
upstream_trace:
  - repo: JuneYaooo/self-media-compliance-review
    path: references/ecommerce-claims.md
    commit: <PINNED_SHA>
```

规则不得只写“注意合规”“避免夸大”这种不能稳定执行的抽象句。

### 4.3 统一规则 ID

采用下列命名空间，不得沿用两套互相冲突的编号体系：

```text
INPUT_REQUIRED_001
EVIDENCE_SOURCE_001
EVIDENCE_RELEVANCE_001
EVIDENCE_SCOPE_001
EVIDENCE_FRESHNESS_001
EVIDENCE_CONTEXT_001
EVIDENCE_FABRICATION_001
RISK_ABSOLUTE_001
RISK_MISLEADING_001
RISK_COMPARISON_001
RISK_PROMOTION_001
RISK_SENSITIVE_001
RIGHTS_ASSET_001
RIGHTS_QUOTE_001
EDITORIAL_CONSISTENCY_001
EDITORIAL_PLACEHOLDER_001
EDITORIAL_STYLE_001
CHANNEL_REQUIRED_001
CHANNEL_FORMAT_001
RELEASE_VERSION_001
RELEASE_APPROVAL_001
RELEASE_OPEN_BLOCK_001
RELEASE_WARN_WAIVER_001
RELEASE_HANDOFF_001
```

### 4.4 必须去重的内容

合并时，以下项目只保留一个主规则：

| 重复主题 | 唯一归属层 | 合并策略 |
|---|---|---|
| 绝对化词语 | L2 | 统一词表 + 上下文升级条件；不要各平台重复写一遍 |
| 无来源数据/结论 | L1 | 统一为 Claim → Evidence 映射检查 |
| 夸大、误导、缺适用范围 | L1 或 L2 | 根因是证据不足归 L1；根因是商业承诺归 L2 |
| 标题与正文不一致 | L3 | 只保留一个一致性规则 |
| 内部备注、TODO、占位符 | L3 | 只保留一个交付清理规则 |
| 标签/封面/字数/视频字段 | L3 | 移入按渠道加载的配置文件 |
| 审核未完成 | L4 | 只由发布门禁判断，不在其他层重复阻断 |
| 未关闭 BLOCK | L4 | 汇总所有层的结果后统一判断 |

### 4.5 降级为 INFO 的内容

下列项目不能默认阻断发布，除非用户通过 `rules/user/` 明确设为硬规则：

```text
- 标题不够吸睛
- 情绪不够强
- 口语化不足
- 没有热点词
- 互动引导偏弱
- 不够短、不够爽、不够爆款
- 个人审美偏好
```

这些只能作为 `INFO`，不得伪装为平台官方规则或合规结论。

---

## 5. 规则加载策略

### 5.1 基础加载

每次审核固定加载：

```text
rules/core/evidence-and-claims.yaml
rules/core/compliance-and-risk.yaml
rules/core/editorial-basics.yaml
rules/core/release-readiness.yaml
```

### 5.2 条件加载

基于 `content_type`、`channels`、`intent` 和抽取到的 Claim 类型，按需加载：

```text
channels/xiaohongshu.yaml
channels/wechat-official-account.yaml
channels/douyin.yaml
channels/wechat-channels.yaml
channels/bilibili.yaml
channels/kuaishou.yaml
domains/ecommerce-claims.yaml
domains/technical-performance.yaml
domains/video-evidence.yaml
domains/live-commerce.yaml
domains/high-sensitivity.yaml
rules/user/*
```

示例：

```text
输入：小红书图文 + 商品推荐 + “节能 30%”性能结论
加载：
- 4 个 core 文件
- channels/xiaohongshu.yaml
- domains/ecommerce-claims.yaml
- domains/technical-performance.yaml
- rules/user/profile 与自定义规则

不加载：
- Bilibili、快手、视频号规则
- 直播规则
- 未命中的历史案例全文
```

### 5.3 案例加载

案例不作为默认上下文。仅在以下情况下检索少量相关案例：

- 某规则触发后，需要提供风险解释或修复示例。
- 某个目标平台与内容类别存在明确的历史判罚/高风险模式。
- 用户请求“为什么这个表述有风险”。

每次最多加载少量高度相关案例，必须附来源、日期和适用限制。

---

## 6. 统一数据模型

### 6.1 审核请求

新建 `schemas/review-request.schema.json`。最小输入：

```json
{
  "work_id": "work_001",
  "version_id": "v1",
  "content_type": "article",
  "channels": ["xiaohongshu"],
  "title": "标题",
  "body": "正文",
  "authoring_context": {
    "brand_or_ip": "品牌或IP名称",
    "audience": "目标受众",
    "intent": "科普"
  }
}
```

建议支持：

```json
{
  "sources": [],
  "claims": [],
  "assets": [],
  "style_guide": {},
  "publish_plan": {},
  "user_rules": [],
  "review_options": {
    "run_layers": ["L1", "L2", "L3", "L4"],
    "strictness": "standard"
  }
}
```

### 6.2 Claim 模型

新建 `schemas/claim.schema.json`。Claim 必须有：

```json
{
  "claim_id": "clm_001",
  "text": "该方案可降低能耗。",
  "claim_type": "performance | numeric | efficacy | comparison | price | promotion | certification | policy | testimonial | other",
  "location": {
    "section": "正文第3段",
    "start_offset": 120,
    "end_offset": 133
  },
  "source_ids": ["src_001"],
  "scope": "适用条件或时间范围",
  "risk_level": "low | medium | high"
}
```

### 6.3 Finding 模型

每条 Finding 必须可执行：

```json
{
  "finding_id": "EVD-001",
  "layer": "L1",
  "rule_id": "EVIDENCE_SCOPE_001",
  "severity": "BLOCK | WARN | INFO | PASS",
  "title": "性能结论缺少适用范围",
  "locations": [
    {
      "section": "正文第3段",
      "quote": "该方案能显著降低室内能耗。",
      "claim_id": "clm_003"
    }
  ],
  "issue": "结论未提供测试条件、对照组或数据来源。",
  "why_it_matters": "读者可能将局部案例理解为普遍性能承诺。",
  "required_action": "补充来源与测试边界，或收窄为特定案例结论。",
  "suggested_rewrite": "在本次测试条件下，该方案相较对照组呈现能耗下降趋势。",
  "source_ids": [],
  "waivable": false,
  "status": "open | resolved | waived"
}
```

### 6.4 总体报告

新建 `schemas/review-report.schema.json`。必须包含：

```text
work_id
version_id
review_id
ruleset_version
reviewed_at
review_scope
limitations
overall_status
scores
claim_summary
findings
gate_decision
publish_handoff
```

### 6.5 发布门禁判断

只允许 `readiness_gate` 计算最终发布状态。其他 Agent 或规则不得自行声称“可发布”。

逻辑：

```text
存在 open BLOCK                    => BLOCKED
不存在 BLOCK，存在 open WARN        => NEEDS_REVISION
不存在 BLOCK/WARN，未人工批准       => PENDING_HUMAN_APPROVAL
WARN 均已具名豁免且人工批准          => APPROVED_WITH_WARNINGS
无 BLOCK/WARN 且人工批准             => APPROVED
已审批 + 最终版本 + 渠道交付完整     => READY_TO_PUBLISH
```

---

## 7. 审核编排逻辑

实现 `review_orchestrator`，严格按以下步骤运行：

```text
1. ValidateInput
2. DetectRiskContext
3. LoadRules
4. ExtractClaims
5. MapClaimsToEvidence
6. RunEvidenceReview (L1)
7. RunComplianceReview (L2)
8. RunEditorialAndChannelReview (L3)
9. DeduplicateFindings
10. RunReleaseReadinessGate (L4)
11. RenderJSONReport
12. RenderHumanSummary
```

### 7.1 不能跳过 L1

如果检测到产品性能、功效、数字、比较、认证、价格、促销、政策或测试结论，则 L1 必须执行。

### 7.2 证据优先

如果一条内容同时触发：

```text
“数据无来源”
“疑似夸大”
“比较缺乏依据”
```

先建立一个 L1 根因 Finding，例如：

```text
EVIDENCE_SOURCE_001 / EVIDENCE_SCOPE_001
```

并在 `why_it_matters` 中说明可能带来的 L2 误导/宣传风险。不要机械地产生 3 条重复 BLOCK。

### 7.3 去重规则

`finding_deduper` 必须：

- 根据同一 `claim_id`、同一文本位置、同一根因合并问题。
- 保留最高严重等级。
- 合并关联规则 ID 和所有命中位置。
- 不丢失每个原始规则的溯源信息。
- 最终报告中每个问题都必须能被用户修改或解释。

### 7.4 改写建议约束

生成改写建议时：

- 不得伪造数据、来源、政策、认证、案例或测试结论。
- 不得为降低风险而删除关键限制信息。
- 优先：补来源 → 增加条件 → 收窄结论 → 加披露 → 删除无法支撑的主张。
- 如果无足够信息，不要写“已验证”“符合规定”“官方认可”。

---

## 8. 精简交付物

### 8.1 运行时最小包

默认安装包应仅包含：

```text
SKILL.md
schemas/
rules/core/
rules/channels/ 中已支持渠道
rules/domains/ 中基础领域
templates/
evals/ 中最小回归集
必要的运行脚本/源代码
LICENSE 与归属声明
```

### 8.2 不进入默认运行时包

以下内容默认不装载给 Agent：

```text
完整 README
CHANGELOG
ROADMAP
CONTRIBUTING
MAINTAINING
全部历史案例
大规模抓取数据
重复的上游说明
原始平台文档副本
无关渠道规则
```

它们可保留在 `docs/` 或外部资料库，仅在维护、溯源或检索命中时使用。

### 8.3 规则规模控制

初始版本建议：

```text
Core 规则：15–25 条
每个平台：5–12 条差异规则
每个领域：5–10 条高风险规则
回归案例：至少 30 条
```

不要一开始导入数百条低质量、相互重复或难以解释的规则。

---

## 9. 初始规则集建议

第一版必须实现以下规则：

### L1 Evidence & Claims

```text
EVIDENCE_SOURCE_001       高风险事实主张无来源
EVIDENCE_RELEVANCE_001    来源不直接支持结论
EVIDENCE_SCOPE_001        结论超出样本/工况/案例范围
EVIDENCE_FRESHNESS_001    时效性数据或规则可能过期
EVIDENCE_CONTEXT_001      图表/截图/引语/视频缺上下文
EVIDENCE_FABRICATION_001  疑似编造来源或不可核验引用
```

### L2 Compliance & Risk

```text
RISK_ABSOLUTE_001         绝对化、保证性表达
RISK_MISLEADING_001       夸大、遗漏限制、误导性表达
RISK_COMPARISON_001       对比竞品无比较对象/标准/依据
RISK_PROMOTION_001        价格、折扣、库存、销量、促销条件不完整
RISK_SENSITIVE_001        医疗、金融、法律、安全等敏感领域缺风险提示
RIGHTS_ASSET_001          图片、视频、商标、肖像使用权不明
RIGHTS_QUOTE_001          引文、评价、截图、案例授权或归属不明
```

### L3 Editorial & Channel

```text
EDITORIAL_CONSISTENCY_001 标题、正文、图文、字幕、口播相互矛盾
EDITORIAL_PLACEHOLDER_001 TODO/占位符/内部备注/测试链接残留
EDITORIAL_STYLE_001       与用户明确的品牌/IP 禁区冲突
CHANNEL_REQUIRED_001      目标渠道必须字段或素材缺失
CHANNEL_FORMAT_001        目标渠道格式要求不满足
```

### L4 Release Readiness

```text
RELEASE_VERSION_001       最终采用版本未确认
RELEASE_APPROVAL_001      人工审批未完成
RELEASE_OPEN_BLOCK_001    存在未关闭阻断项
RELEASE_WARN_WAIVER_001   警告未处理或未具名豁免
RELEASE_HANDOFF_001       负责人、排期或渠道交接包不完整
```

---

## 10. 模板迁移策略

从 B 中迁移并重命名：

```text
templates/profile.md        # IP 定位、受众、语气、禁区、常用术语
templates/my-rules.md       # 用户自定义强制规则
templates/expressions.md    # 推荐/禁用表达；默认只产生 INFO 或 WARN
templates/词库示例.md        # 转为 wordlist.example.yaml
```

要求：

- Profile 只定义“用户明确声明”的风格、禁区、受众、品牌承诺和术语，不让 Agent 根据少量文章擅自固化人格。
- 用户词库只影响命中的内容；不得自动覆盖合规规则。
- 用户自定义规则可以加严，不应无理由降低 Core BLOCK 的门槛。
- 任何自定义规则必须有 `rule_id`、严重级别和触发范围。

从 A 中迁移：

```text
A 的平台规则       → rules/channels/
A 的电商主张规则    → rules/domains/ecommerce-claims.yaml
A 的视频证据规则    → rules/domains/video-evidence.yaml
A 的直播证据规则    → rules/domains/live-commerce.yaml
A 的产品一致性 schema → 合并至 claim/evidence/finding schema
A 的合规报告 schema → 作为 review-report schema 的输入参考
```

---

## 11. 评估与验收

### 11.1 必须建立回归评估集

不要仅凭“Agent 看起来回答得不错”验收。创建至少 30 条 fixture，覆盖：

```text
- 无来源数字
- 有来源但不支持结论
- 过度外推实验数据
- 绝对化功效承诺
- 竞品比较无依据
- 价格/折扣无期限与条件
- 用户评价/截图无授权说明
- 图文与口播数据不一致
- TODO/占位符残留
- 缺最终版本
- 缺人工审批
- 已豁免 WARN
- 多渠道字段差异
- 技术性能内容的工况缺失
- 正常低风险科普内容
```

### 11.2 核心断言

每个 fixture 至少断言：

```text
expected overall_status
required rule_id(s)
minimum severity
forbidden duplicate finding(s)
can_schedule
can_publish
```

### 11.3 验收标准

项目在 MVP 阶段满足以下条件即可：

```text
- 所有 schema 可验证
- 所有规则均有唯一 rule_id
- 每个 BLOCK 有原文定位与明确修复动作
- 每个高风险事实 Finding 提供来源状态
- 不生成无依据的改写建议
- 同一根因不产生重复 BLOCK
- 有 open BLOCK 时永远不可排期/发布
- 有 open WARN 时默认不可排期/发布
- 没有人工批准时永远不是 READY_TO_PUBLISH
- 评估集可重复运行并输出差异
```

---

## 12. 迁移执行顺序

严格按以下顺序提交代码。每一步单独提交，提交信息清晰。

```text
Commit 1: chore: initialize unified publishing gate repository
Commit 2: docs: add upstream inventory and license attribution ledger
Commit 3: feat: add unified request and report schemas
Commit 4: feat: add four-layer core rules and rule id registry
Commit 5: feat: migrate channel and domain rules with upstream traceability
Commit 6: feat: add conditional rule loader and review orchestrator
Commit 7: feat: add finding deduplication and release readiness gate
Commit 8: feat: add user profile and custom rules templates
Commit 9: test: add regression fixtures and evaluation runner
Commit 10: docs: document migration decisions and runtime loading model
```

每个提交后运行：

```text
- schema validation
- rule validation
- evaluation suite
- license/attribution lint（如已实现）
```

不要在一个大提交中混合重构、迁移、格式化和功能修改。

---

## 13. 完成定义

任务完成时，必须交付：

1. 一个独立、可运行的 `content-publishing-gate` 项目。
2. 一个可被 Agent 读取的精简 `SKILL.md`，只包含运行行为和边界，不包含冗长背景材料。
3. 统一的输入、Claim、Evidence、Finding、Report 和 Release Readiness Schema。
4. 四层规则架构与唯一规则 ID 注册表。
5. 按渠道、领域、用户配置的按需加载机制。
6. Findings 去重器和唯一发布门禁器。
7. 至少 30 个回归测试样例及可重复运行的评估脚本。
8. 许可证、上游来源、第三方资料、迁移决策的完整台账。
9. README 中明确说明：该项目是发布前辅助审核工具，不提供法律意见，不保证平台审核结果。

最终汇报必须包括：

```text
- 迁移了哪些模块
- 删除/未迁移了哪些模块及原因
- 合并掉了哪些重复规则
- 当前支持哪些渠道与内容类型
- 当前规则总数和回归样例数
- 已知限制
- 许可证与归属状态
- 后续建议（不超过 5 条）
```

---

## 14. 禁止事项

实施过程中禁止：

```text
- 直接拼接两份 SKILL.md
- 直接复制两份 LICENSE 后声称项目许可明确
- 将未核验的抓取案例或第三方材料打包再分发
- 把全部平台规则塞入默认 Prompt
- 将风格建议升级为 BLOCK
- 让任一子 Agent 自己决定发布
- 在没有来源时替用户生成看似可信的数据/法规/案例
- 将“审核未发现问题”写成“绝对合规”
- 用删除原文全部关键内容的方式规避风险提示
- 跳过评估集就宣布迁移完成
```

---

## 15. 给执行 Agent 的最终指令

请先生成资产盘点、许可证台账和规则映射，不要立即复制文件。之后建立干净的新仓库，以四层审核架构重写运行时规则，保留上游可追溯性，并通过结构化 schema、去重逻辑与回归评估保证输出可用于真实发布工作流。

优先交付“少而稳定、可解释、可验证、可扩展”的审核内核；不要追求一次覆盖所有平台、所有行业和所有写作风格。
