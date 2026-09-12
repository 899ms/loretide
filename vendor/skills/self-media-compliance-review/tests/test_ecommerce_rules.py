import json
from pathlib import Path

from tools.compare_product_link import (
    compare_product_link,
    compare_str,
    compare_specs,
    compare_price,
    compare_activity,
    compare_gifts,
)
from tools.extract_claims import extract_claims, summarize_claims


FIXTURES = Path(__file__).parent / "fixtures"


def load_fixture(name: str) -> dict:
    return json.loads((FIXTURES / name).read_text())


# --- compare_product_link ---

def test_compare_product_link_all_consistent():
    data = load_fixture("product_mismatch.json")
    report = compare_product_link(
        data["product"], data["video_claims_consistent"], include_qianchuan=False
    )

    assert report["schema_version"] == "1.0"
    for comp in report["comparisons"]:
        assert comp["result"] == "一致", f"{comp['label']} should be consistent, got: {comp}"


def test_compare_product_link_detects_mismatches():
    data = load_fixture("product_mismatch.json")
    report = compare_product_link(
        data["product"], data["video_claims_inconsistent"], include_qianchuan=False
    )

    inconsistent = [c for c in report["comparisons"] if c["result"] == "不一致"]
    assert len(inconsistent) >= 4, f"Expected 4+ mismatches, got {len(inconsistent)}: {inconsistent}"

    # Brand should be mismatched
    brand_check = next(c for c in report["comparisons"] if c["check_id"] == "商品三一致-品牌")
    assert brand_check["result"] == "不一致"

    # Price should be mismatched
    price_check = next(c for c in report["comparisons"] if c["check_id"] == "商品三一致-价格")
    assert price_check["result"] == "不一致"


def test_compare_product_link_includes_qianchuan_by_default():
    data = load_fixture("product_mismatch.json")
    report = compare_product_link(data["product"], data["video_claims_consistent"])

    assert "qianchuan_checks" in report
    assert len(report["qianchuan_checks"]) == 7


def test_compare_product_link_skips_qianchuan_when_disabled():
    data = load_fixture("product_mismatch.json")
    report = compare_product_link(
        data["product"], data["video_claims_consistent"], include_qianchuan=False
    )

    assert report.get("qianchuan_checks", []) == []


# --- compare_str ---

def test_compare_str_match():
    assert compare_str("XX品牌", "XX品牌") == {
        "result": "一致",
        "reason": "均声明为: XX品牌",
    }


def test_compare_str_mismatch():
    assert compare_str("YY品牌", "XX品牌") == {
        "result": "不一致",
        "reason": "视频: YY品牌；详情页: XX品牌",
    }


def test_compare_str_missing_video():
    assert compare_str("", "XX品牌") == {
        "result": "待核验",
        "reason": "视频中未声明",
    }


def test_compare_str_missing_product():
    assert compare_str("XX品牌", "") == {
        "result": "待核验",
        "reason": "详情页信息缺失",
    }


# --- compare_specs ---

def test_compare_specs_match():
    result = compare_specs(
        {"净含量": "50g", "类型": "清爽型"},
        [{"spec": {"净含量": "50g", "类型": "清爽型"}}],
    )
    assert result["result"] == "一致"


def test_compare_specs_mismatch():
    result = compare_specs(
        {"净含量": "100g"},
        [{"spec": {"净含量": "50g"}}],
    )
    assert result["result"] == "不一致"


def test_compare_specs_missing():
    result = compare_specs({}, [])
    assert result["result"] == "待核验"


# --- compare_price ---

def test_compare_price_match():
    result = compare_price("129", [{"price": "129.00"}])
    assert result["result"] == "一致"


def test_compare_price_mismatch():
    result = compare_price("99", [{"price": "129.00"}])
    assert result["result"] == "不一致"


def test_compare_price_missing():
    result = compare_price("", [])
    assert result["result"] == "待核验"


def test_compare_price_does_not_use_substring_matching():
    result = compare_price("9.9", [{"price": "19.9"}])
    assert result["result"] == "不一致"


def test_compare_price_uses_current_price_without_skus():
    result = compare_price("￥29.90元", [], "29.9")
    assert result["result"] == "一致"


def test_compare_price_accepts_one_value_from_multiple_skus():
    result = compare_price("券后价 89 元", [{"price": "99"}, {"price": "89.00"}])
    assert result["result"] == "一致"


def test_compare_activity_requires_matching_promotion():
    assert compare_activity("满2件减20", "满2件减20元，活动至8月31日")["result"] == "一致"
    assert compare_activity("满2件减30", "满2件减20元，活动至8月31日")["result"] == "不一致"


# --- compare_gifts ---

def test_compare_gifts_match():
    result = compare_gifts(["试用装5g×2"], ["试用装5g×2"])
    assert result["result"] == "一致"


def test_compare_gifts_extra_in_video():
    result = compare_gifts(["送面膜5片", "免费加赠精华液"], ["试用装5g×2"])
    assert result["result"] == "不一致"


def test_compare_gifts_missing():
    result = compare_gifts([], [])
    assert result["result"] == "待核验"


# --- extract_claims ---

def test_extract_claims_finds_absolutes():
    text = "这是最好的产品，全网最低价，100%有效果"
    claims = extract_claims(text)
    absolutes = [c["text"] for c in claims["absolutes"]]
    assert "最好" in absolutes
    assert "全网最低" in absolutes
    assert "100%" in absolutes


def test_extract_claims_finds_efficacy():
    text = "用了7天就能白，保证能瘦10斤，根治敏感肌，医生推荐的药妆级别产品"
    claims = extract_claims(text)
    efficacy_texts = [c["text"] for c in claims["efficacy"]]
    assert any("7天" in t and "白" in t for t in efficacy_texts)
    assert any("保证" in t and "瘦" in t for t in efficacy_texts)
    assert any("根治" in t for t in efficacy_texts)


def test_extract_claims_finds_price():
    text = "到手价只要99元，原价299，限时特价，买一送一，赠品免费送"
    claims = extract_claims(text)
    assert len(claims["price"]) >= 3


def test_extract_claims_finds_comparisons():
    text = "比大牌好用10倍，别人的产品根本做不到这个效果"
    claims = extract_claims(text)
    assert len(claims["comparisons"]) >= 1


def test_extract_claims_finds_regulated_topics():
    text = "治疗失眠症状，7天快速美白祛斑，保证通过率100%上岸，稳赚不赔"
    claims = extract_claims(text)
    assert len(claims["regulated_topics"]) >= 2


def test_extract_claims_empty_text():
    claims = extract_claims("这是一段普通的商品介绍，没有违规内容。")
    total = sum(len(v) for v in claims.values())
    assert total == 0


def test_extract_claims_skips_negated_and_instructional_examples():
    claims = extract_claims("本工具不保证一定通过，文案里不能写全网最低，发布前必须检查授权。")
    matched = {item["text"] for values in claims.values() for item in values}
    assert "保证" not in matched
    assert "全网最低" not in matched
    assert "必须" not in matched


def test_regulated_category_prefers_specific_match():
    report = compare_product_link(
        {"title": "保健食品软糖", "category": "保健食品", "current_price": "39.9"},
        {"price_mentioned": "39.9"},
        include_qianchuan=False,
    )
    assert report["category_risk"]["check_id"] == "行业资质-保健食品"
    assert report["category_risk"]["risk"] == "blocker"


def test_product_report_checks_activity_and_data_claims():
    report = compare_product_link(
        {
            "title": "普通商品",
            "current_price": "29.9",
            "activity_rules": "满2件减20元",
        },
        {
            "price_mentioned": "29.9",
            "activity_mentioned": "满2件减20",
            "data_claims": ["已售100万件"],
        },
        include_qianchuan=False,
    )
    by_id = {item["check_id"]: item for item in report["comparisons"]}
    assert by_id["活动规则完整性"]["result"] == "一致"
    assert by_id["电商数据声明"]["result"] == "待核验"


# --- summarize_claims ---

def test_summarize_claims_categorizes_correctly():
    claims = extract_claims("这是最好的产品，保证有效，7天美白，比大牌好用10倍，治疗失眠")
    summary = summarize_claims(claims)
    assert len(summary["forbidden_expressions"]) >= 2
    assert summary["total_claims"] >= 4


# --- ecommerce references exist ---

def test_ecommerce_references_exist():
    refs_dir = Path("references")
    assert (refs_dir / "douyin-ecommerce.md").is_file()
    assert (refs_dir / "qianchuan-low-quality.md").is_file()
    assert (refs_dir / "ecommerce-claims.md").is_file()
    assert (refs_dir / "cases" / "ecommerce-cases.md").is_file()


# --- ecommerce tools exist ---

def test_ecommerce_tools_exist():
    tools_dir = Path("tools")
    assert (tools_dir / "compare_product_link.py").is_file()
    assert (tools_dir / "extract_claims.py").is_file()
