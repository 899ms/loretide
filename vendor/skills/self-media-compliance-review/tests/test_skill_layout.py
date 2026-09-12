from pathlib import Path


def test_single_skill_repository_uses_root_level_layout():
    assert Path("SKILL.md").is_file()
    assert Path("agents/openai.yaml").is_file()
    assert Path("references/xiaohongshu.md").is_file()
    assert Path("references/cases/xiaohongshu.md").is_file()
    assert Path("references/douyin-ecommerce.md").is_file()
    assert Path("references/qianchuan-low-quality.md").is_file()
    assert Path("references/ecommerce-claims.md").is_file()
    assert Path("references/cases/ecommerce-cases.md").is_file()
    assert Path("references/video-evidence-integrity.md").is_file()
    assert Path("references/video-review-workflow.md").is_file()
    assert Path("references/ecommerce-workflow.md").is_file()
    assert Path("references/live-evidence-workflow.md").is_file()
    assert Path("references/report-schema.md").is_file()
    assert Path("schemas/compliance-report.schema.json").is_file()
    assert Path("schemas/product-consistency.schema.json").is_file()
    assert Path("schemas/video-evidence-manifest.schema.json").is_file()
    assert Path("tools/search_local_evidence.py").is_file()
    assert Path("tools/analyze_video.py").is_file()
    assert Path("tools/tikhub/lib/tikhub_client.py").is_file()
    assert Path("tools/tikhub/references/tools-douyin.json").is_file()
    assert not Path("skills/self-media-compliance-review/SKILL.md").exists()


def test_docs_link_to_root_level_skill_layout():
    readme = Path("README.md").read_text()
    sources = Path("docs/sources.md").read_text()

    assert "./SKILL.md" in readme
    assert "./references/xiaohongshu.md" in readme
    assert "./references/cases/xiaohongshu.md" in readme
    assert "./references/recent-cases-2025-2026.md" in readme
    assert "references/cases/xiaohongshu.md" in sources
    assert "skills/self-media-compliance-review" not in readme
    assert "skills/self-media-compliance-review" not in sources


def test_ecommerce_references_include_qianchuan_checklist():
    qianchuan = Path("references/qianchuan-low-quality.md").read_text()
    assert "商品堆砌式累加" in qianchuan
    assert "品牌、款式、颜色、图案、形状不一致" in qianchuan
    assert "卖惨、演戏炒作、清仓话术" in qianchuan
    assert "图片轮播、大字报、高饱和度配色" in qianchuan
    assert "黑边、遮挡、水印、模糊变形" in qianchuan
    assert "商品主体不明确" in qianchuan


def test_ecommerce_references_include_consistency_categories():
    douyin_ec = Path("references/douyin-ecommerce.md").read_text()
    assert "商品三一致" in douyin_ec
    assert "商品展示与详情页一致性" in douyin_ec
    assert "功效与效果宣传" in douyin_ec
    assert "带货话术禁区" in douyin_ec


def test_skill_md_includes_ecommerce_routing():
    skill = Path("SKILL.md").read_text()
    assert "douyin-ecommerce.md" in skill
    assert "qianchuan-low-quality.md" in skill
    assert "ecommerce-claims.md" in skill
    assert "ecommerce-cases.md" in skill
    assert "tools/search_local_evidence.py" in skill
    assert "商品一致性审核" in skill
    assert "machine-readable" in skill.lower() or "json" in skill.lower()
    assert "发布前合规" in skill
    assert "选品前风险" in skill
    assert "文案生成前风险" in skill
    assert "video-review-workflow.md" in skill
    assert "live-evidence-workflow.md" in skill
    assert "report-schema.md" in skill
    assert "Do not return `Pass`" in skill
