# E-Commerce Workflow

Read this file when content involves 带货、挂车、商品链接、详情页、SKU、佣金、
成交、选品、千川、抖店、精选联盟、商品三一致、功效承诺、赠品、优惠券、
到手价、小黄车、直播带货 or other commercial promotion.

Also read the applicable rule files:

- `douyin-ecommerce.md` for Douyin commerce rules;
- `qianchuan-low-quality.md` for paid-material quality checks;
- `ecommerce-claims.md` for efficacy and claim rules;
- `cases/ecommerce-cases.md` when cases are relevant.

## Required Inputs

Collect the detail-page URL or full screenshot and capture time, all SKU values,
current and original prices, gifts, coupons and activity conditions, deadlines,
business and category qualifications, brand authorization, and source licenses.
Also ask for relevant penalty or rejected-ad history. Missing inputs remain
`待核验`.

## Choose an Entry Point

### 发布前合规

Review the complete video package, universal risks, product consistency, and
applicable platform and commerce references. An unresolved `Blocker` or
unaccepted `High` means the package is not ready to publish.

### 选品前风险

Before promotion, identify regulated categories, missing qualifications, and
high-risk detail-page claims. Return a condensed risk level and blocker list.

### 文案生成前风险

Extract detail-page claims before writing. Separate claims that must be removed,
claims that should be rewritten, and claims requiring evidence. Do not turn a
detail-page statement into a verified fact merely because the merchant wrote it.

## 商品一致性审核

Compare all public surfaces, including video, voiceover, subtitles, product card,
and detail page:

- brand, style, color, pattern, and shape;
- quantity, weight, volume, size, and other SKU specifications;
- current price, original or line-through price, and applicable SKU;
- gift, coupon, red-packet, activity conditions, and deadline;
- efficacy or performance claims and supporting qualifications;
- video, BGM, image, font, portrait, trademark, and brand authorization.

Specific regulated categories take precedence over broad matches. For example,
`保健食品` must not be reduced to the broader `食品` category.

## Automated Prechecks

Run paired structured comparison when product and claim JSON are available:

```bash
python3 tools/compare_product_link.py \
  --product product_info.json \
  --claims video_claims.json \
  --format markdown|json
```

Scan copy before or during review:

```bash
python3 tools/extract_claims.py --file script.txt --format markdown|json
```

The comparison report conforms to
`../schemas/product-consistency.schema.json`. Exact numeric price comparison
supports SKU prices and `current_price`; substring overlap is not a match.
Activity, original-price, data, efficacy, and regulated-category results may
require human evidence.

These tools do not open a product page, verify a qualification, understand every
visual, or assign the final compliance verdict. Negated or instructional examples
are filtered where possible, but every hit still needs contextual review.

## Qianchuan Manual Review

Inspect the actual frames for product pile-up, theatrical hardship or clearance
stories, picture carousels or big-character posters, high-saturation layouts,
black borders, obstruction, watermarks, blur, unclear product focus, disturbing
material, specification mismatch, and missing authorization. The generated
checklist uses `flagged: null` until a reviewer inspects the material.
