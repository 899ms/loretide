# Report Format and Schemas

Read this file for serious reviews, machine-to-machine handoff, or when the user
asks for JSON. Quick reviews may collapse the table to bullets but must keep the
same evidence and uncertainty fields.

## Markdown Report

```markdown
# 合规风险审核

- 平台: <platforms>
- 内容范围: <video/script/cover/copy/link/etc.>
- 证据覆盖: <reviewed and unreviewed surfaces>
- 结论: Pass | Low | Medium | High | Blocker
- 最高风险: <one sentence>

## 风险明细

| 等级 | 平台/条款 | 证据位置 | 风险说明 | 修改建议 |
| --- | --- | --- | --- | --- |
| High | 视频号 4.11.1 | 口播 00:12 | 无法核验的销售数据 | 删除数字或补可验证来源 |

## 待核验

- <unreviewed audio/visual/authorization/qualification/etc.>

## 发布前复审清单

- [ ] 风险音频已消音或替换
- [ ] 风险字幕、封面和标题已修改
- [ ] 敏感画面已删除或充分遮挡
- [ ] 商品价格、赠品、规格和链接一致
- [ ] 所需资质、版权、肖像和素材授权已核验
```

For commerce content, append the paired-comparison results and the Qianchuan
manual checklist described in `ecommerce-workflow.md`.

## JSON Reports

Published JSON Schemas:

- `../schemas/compliance-report.schema.json`
- `../schemas/product-consistency.schema.json`
- `../schemas/video-evidence-manifest.schema.json`

Use `schema_version: "1.0"`. A compliance report includes `platforms`,
`content_scope`, `coverage`, `risk_level`, `highest_risk`, `findings`,
`pending_verification`, and `reviewed_at`. Each finding includes severity,
rule identifier, evidence pointer, reason, and concrete fix.

`Pass` is a report-level result, never a finding severity. Do not emit `Pass`
when required audio, visuals, links, qualifications, or authorization remain
unreviewed.
