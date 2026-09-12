# Douyin E-Commerce Compliance Reference

Use this reference when the target platform is 抖音/抖音电商 and the content involves 带货、挂车、商品链接、抖店、精选联盟 or any commercial promotion.

Source audit date: 2026-08-11.

> **Related**: 千川低质素材 → `references/qianchuan-low-quality.md` | 功效声明规则 → `references/ecommerce-claims.md` | 电商违规案例 → `references/cases/ecommerce-cases.md`

Primary sources:

- 抖店后台规则中心: https://fxg.jinritemai.com/
- 抖音电商学习中心: https://school.jinritemai.com/doudian/web/home
- 抖音规则中心: https://www.douyin.com/rule/policy
- 抖音用户服务协议: https://www.douyin.com/agreements/?id=6773906068725565448
- 千川低质素材治理规则 (see `references/qianchuan-low-quality.md`)

Source note: the 抖店 rule center and 电商学习中心 are JavaScript-rendered. Recheck official pages in a browser for high-stakes decisions. When official page text is unavailable, prefer conservative (restrictive) review decisions.

## E-Commerce Severity Additions

Beyond the base Douyin severity hints in `references/douyin.md`:

- `Blocker`: false product information, unqualified special-industry goods (medical devices, drugs, health food, infant formula, special medical formula food), fraudulent price/gift/activity claims, off-platform transaction funnel, unauthorized use of others' product qualifications, obvious 商品三不一致 (video vs detail page vs script), minor involvement in commercial content without guardian consent, 自制食品/化妆品无资质.
- `High`: likely 千川低质素材 rejection, exaggerated efficacy claims, missing commercial disclosure, inconsistent price/gift/specs between video and link, 功效承诺 without qualification, unmarked paid promotion, 卖惨/演戏炒作式带货, missing AI/synthetic label on product demonstration.
- `Medium`: ambiguous before/after claims, borderline product demonstration that may confuse, missing activity deadline/scope, vague qualification reference.
- `Low`: minor wording polish, slightly misleading thumbnail that doesn't affect purchase decision.

## E-Commerce Specific Checks

### 1. 商品展示与详情页一致性 (Product-Consistency)

Flag when any of the following do not match between the video/live and the actual product detail page:

- **品牌/商标**: the brand shown in the video versus the brand on the detail page.
- **款式/颜色/图案/形状**: the visual appearance of the product in the video versus the SKU images and descriptions.
- **规格/数量/重量**: oral claims about quantity, weight, volume, size versus the SKU specifications.
- **价格**: any price mentioned in voiceover, subtitles, or on-screen text versus the linked product price.
- **赠品/红包/优惠**: any gift, red packet, coupon, or discount conditions stated versus the actual activity rules.
- **活动期限**: start/end dates or quantity limits mentioned versus the actual activity settings.
- **发货/物流/售后**: delivery time, shipping method, return/refund policy claims versus the store settings.

Evidence to request: product detail page screenshot, SKU list, price history, activity rule page, store qualification page.

### 2. 行业资质与特殊类目 (Regulated Categories)

Flag when the product belongs to a regulated category and no qualification evidence is provided:

| Category | Required Qualifications | Common Violations |
|---|---|---|
| 食品/保健食品 | 食品经营许可证、食品生产许可证 | 普通食品宣称保健/治疗功效 |
| 化妆品 | 化妆品生产许可证、备案/注册凭证 | 宣称医疗功效、伪造备案号 |
| 医疗器械 | 医疗器械经营备案/许可证、产品注册证 | 无证销售、夸大适用范围 |
| 药品 | 药品经营许可证 | 禁止普通达人带货 |
| 母婴/婴幼儿食品 | 食品经营许可证 + 对应品类资质 | 普通食品冒充婴幼儿食品 |
| 宠物食品/用品 | 饲料生产许可证等 | 无证自制宠物食品 |
| 金融/保险/贷款 | 金融业务许可证 | 禁止普通达人推广 |
| 教育培训 | 办学许可证等 | 保证效果、虚假师资 |
| 酒类 | 酒类经营许可证 | 未成年人饮酒暗示 |

Mark regulated-category products as `待核验: 行业资质` until qualification evidence is provided.

### 3. 功效与效果宣传 (Efficacy Claims)

Flag:

- 使用绝对化用语: `最好`, `第一`, `唯一`, `全网最低`, `100%`, `绝对`, `保证`, `必`.
- 效果承诺: `必瘦`, `必白`, `包过`, `稳赚`, `根治`, `治愈`, `永不`.
- 无法核验的对比: `比XX品牌好10倍`, `市面唯一`, `别人家没有`.
- 前后对比: before/after photos or videos without clear labeling of conditions (lighting, angle, time, no editing).
- 用户评价作假: fabricated testimonials, staff posing as buyers, scripted "真实反馈".
- 数据造假: 无出处的销量、好评率、复购率数据.
- AI合成商品效果: AI-generated product demonstration without clear labeling.
- 跨类目功效: 化妆品宣称医疗功效, 食品宣称治疗功效, 普通用品宣称保健功效.

Required fix: remove absolute/guaranteed language; replace with verifiable product facts or personal experience with context; add qualification proof; label AI-generated demonstrations.

### 4. 价格与促销表达 (Price and Promotion)

Flag:

- 价格不一致: video says ¥99, link shows ¥129.
- 赠品条件不完整: "买一送一" 但未说明送的规格/型号/条件.
- 活动期限不明确: "限时特价" 未标明起止时间.
- 虚假限量: "仅剩10件" 但库存充足.
- 虚假原价: 虚构原价后进行折扣对比.
- 红包/优惠券使用条件隐藏: 满减门槛、使用范围、有效期未说明.
- 诱导点击: "点下方链接领取优惠" 但链接无优惠.

Required fix: align all price/gift/activity claims with the actual link; add activity scope and deadlines; remove fake urgency.

### 5. 带货话术禁区 (Forbidden Sales Scripts)

Flag these high-risk patterns:

- **卖惨/演戏**: 哭诉家庭困难、编造悲惨故事、演戏式砍价/争吵.
- **清仓话术**: "工厂倒闭"、"亏本清仓"、"最后一天"（非真实场景）.
- **伪公益**: 宣称部分收入捐赠但无真实公益项目.
- **身份冒充**: 冒充品牌方、厂家、专家、医生、律师.
- **站外引流**: 引导加微信、QQ、群聊、其他平台交易.
- **私域转化**: "私信拿链接"、"评论区扣1"、"点头像看".
- **刷单诱导**: 引导虚假交易、刷好评、刷销量.
- **贬低竞品**: 无事实依据地贬低其他品牌或商品.
- **恐慌营销**: "不用这个产品就会XX".

### 6. 商品链接与挂车规范 (Product Link and Cart)

Flag:

- 挂车商品与视频展示商品不一致.
- 商品链接失效、下架、或价格变更后视频未更新.
- 达人无权限带货该类目商品.
- 精选联盟商品未确认佣金/库存/发货.
- 商品标题/主图包含违禁词或夸大宣传.
- 商品详情页存在虚假销量/评价.

### 7. 行业专属规则 (Industry-Specific)

#### 食品

- 禁止宣传医疗/保健功效.
- 禁止使用患者形象或医生推荐.
- 禁止与药品/保健食品对比.
- 自制食品必须有食品生产许可证或小作坊登记证.
- 婴幼儿配方食品、特殊医学用途配方食品禁止普通达人带货.

#### 化妆品

- 禁止宣传医疗功效（治疗、修复、消炎等）.
- 禁止使用医生/药师推荐.
- 禁止夸大功效（7天美白、3天祛痘等）.
- 必须公示化妆品备案/注册号.
- 特殊用途化妆品（美白、防晒、染发等）须有特妆批号.

#### 服饰/鞋包

- 材质成分必须与吊牌/水洗标一致.
- 尺码建议不可承诺"一定合身".
- 原单/尾单/代工厂货须有品牌授权，否则为假货.

## E-Commerce Report Labels

Use platform labels:

- `抖音电商 商品三一致`
- `抖音电商 行业资质`
- `抖音电商 功效宣传`
- `抖音电商 价格促销`
- `抖音电商 带货话术`
- `抖音电商 商品链接规范`
- `抖音电商 <食品/化妆品/母婴/宠物/服饰>行业规则`
- `千川低质素材治理 详见 qianchuan-low-quality.md`
