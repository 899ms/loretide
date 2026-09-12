#!/usr/bin/env python3
"""Compare product detail page information with video claims for compliance review."""

from __future__ import annotations

import argparse
import json
import re
import sys
from dataclasses import dataclass, field
from decimal import Decimal, InvalidOperation
from typing import Any


@dataclass
class ProductInfo:
    """Product information extracted from the detail page."""

    url: str = ""
    title: str = ""
    brand: str = ""
    skus: list[dict] = field(default_factory=list)
    current_price: str = ""
    original_price: str = ""
    gifts: list[str] = field(default_factory=list)
    activity_rules: str = ""
    activity_deadline: str = ""
    category: str = ""
    captured_at: str = ""


@dataclass
class VideoClaims:
    """Claims extracted from the video/script."""

    source: str = ""
    brand_mentioned: str = ""
    style_color_mentioned: str = ""
    specs_mentioned: dict = field(default_factory=dict)
    price_mentioned: str = ""
    original_price_mentioned: str = ""
    gifts_mentioned: list[str] = field(default_factory=list)
    activity_mentioned: str = ""
    deadline_mentioned: str = ""
    efficacy_claims: list[str] = field(default_factory=list)
    data_claims: list[str] = field(default_factory=list)


COMPARISON_CHECKS = [
    {
        "check_id": "商品三一致-品牌",
        "field_video": "brand_mentioned",
        "field_product": "brand",
        "label": "品牌一致性",
        "description": "视频展示/口播品牌 vs 详情页品牌",
    },
    {
        "check_id": "商品三一致-款式颜色",
        "field_video": "style_color_mentioned",
        "field_product": "title",
        "label": "款式颜色一致性",
        "description": "视频展示的款式/颜色 vs 详情页",
    },
    {
        "check_id": "商品三一致-规格",
        "field_video": "specs_mentioned",
        "field_product": "skus",
        "label": "规格一致性",
        "description": "口播规格(数量/重量/体积) vs SKU 规格",
    },
    {
        "check_id": "商品三一致-价格",
        "field_video": "price_mentioned",
        "field_product": "current_price",
        "label": "价格一致性",
        "description": "口播价格 vs 商品链接价格",
    },
    {
        "check_id": "赠品条件完整性",
        "field_video": "gifts_mentioned",
        "field_product": "gifts",
        "label": "赠品一致性",
        "description": "视频中的赠品信息 vs 实际赠品规则",
    },
    {
        "check_id": "活动期限完整性",
        "field_video": "deadline_mentioned",
        "field_product": "activity_deadline",
        "label": "活动期限一致性",
        "description": "视频中的活动期限 vs 实际活动设置",
    },
]

QIANCHUAN_CHECKS = [
    {
        "check_id": "千川低质素材 商品堆砌",
        "description": "多个不相关商品强行堆放在同一画面",
    },
    {
        "check_id": "千川低质素材 卖惨/演戏/炒作",
        "description": "哭诉、下跪、演戏式砍价、虚假紧迫",
    },
    {
        "check_id": "千川低质素材 图片轮播/大字报/高饱和",
        "description": "纯图片轮播、大字报风格、高饱和闪烁",
    },
    {
        "check_id": "千川低质素材 黑边/遮挡/水印/模糊",
        "description": "黑边、遮挡、第三方水印、模糊变形",
    },
    {
        "check_id": "千川低质素材 商品主体不明确",
        "description": "商品占比<30%、配料表替代商品展示",
    },
    {
        "check_id": "千川低质素材 污垢/血腥/危险/恶俗",
        "description": "污垢、血腥、危险动作、恶俗内容",
    },
    {
        "check_id": "千川低质素材 素材授权",
        "description": "视频/BGM/肖像/字体授权状态",
    },
]

REGULATED_CATEGORIES = {
    "食品": {"risk": "high", "requires": "食品经营许可证、食品生产许可证"},
    "保健食品": {"risk": "blocker", "requires": "保健食品批准证书、食品经营许可证"},
    "化妆品": {"risk": "high", "requires": "化妆品生产许可证、备案/注册凭证"},
    "医疗器械": {"risk": "blocker", "requires": "医疗器械经营备案/许可证、产品注册证"},
    "药品": {"risk": "blocker", "requires": "药品经营许可证 (普通达人禁止带货)"},
    "母婴": {"risk": "high", "requires": "对应品类资质"},
    "宠物食品": {"risk": "high", "requires": "饲料生产许可证"},
    "金融": {"risk": "blocker", "requires": "金融业务许可证 (普通达人禁止带货)"},
    "教育培训": {"risk": "high", "requires": "办学许可证"},
    "酒类": {"risk": "high", "requires": "酒类经营许可证"},
}


def _to_str(value: Any) -> str:
    return str(value).strip() if value else ""


def _find_price(skus: list[dict]) -> str:
    if not skus:
        return ""
    prices = {str(sku.get("price", "")) for sku in skus if sku.get("price")}
    if len(prices) == 1:
        return prices.pop()
    return ", ".join(sorted(prices)) if prices else ""


def _price_values(*values: Any) -> set[Decimal]:
    """Extract normalized monetary values without using substring matching."""
    prices: set[Decimal] = set()
    for value in values:
        if value is None:
            continue
        if isinstance(value, (int, float, Decimal)):
            candidates = [str(value)]
        else:
            normalized = str(value).replace(",", "")
            candidates = re.findall(r"(?<!\d)(\d+(?:\.\d{1,2})?)(?!\d)", normalized)
        for candidate in candidates:
            try:
                prices.add(Decimal(candidate).normalize())
            except InvalidOperation:
                continue
    return prices


def _format_prices(values: set[Decimal]) -> str:
    return ", ".join(format(value, "f") for value in sorted(values))


def compare_str(video_val: str, product_val: str) -> dict:
    """Compare two string fields."""
    video_val = _to_str(video_val)
    product_val = _to_str(product_val)
    if not video_val:
        return {"result": "待核验", "reason": "视频中未声明"}
    if not product_val:
        return {"result": "待核验", "reason": "详情页信息缺失"}
    if video_val == product_val:
        return {"result": "一致", "reason": f"均声明为: {video_val}"}
    # Check if video_val is contained within product_val (e.g. "清爽型 50g" in "XX品牌 补水保湿面霜 50g 清爽型")
    if video_val in product_val:
        return {"result": "一致", "reason": f"视频声明包含在详情页中: {video_val}"}
    if product_val in video_val:
        return {"result": "一致", "reason": f"详情页信息包含在视频声明中: {product_val}"}
    # Check if all significant parts of video_val appear in product_val
    video_parts = [p.strip() for p in video_val.replace("、", " ").replace("，", " ").split() if len(p.strip()) >= 2]
    if video_parts and all(part in product_val for part in video_parts):
        return {"result": "一致", "reason": f"视频声明各部分均在详情页中找到: {video_val}"}
    return {"result": "不一致", "reason": f"视频: {video_val}；详情页: {product_val}"}


def compare_specs(video_specs: dict, product_skus: list[dict]) -> dict:
    """Compare spec claims against SKU data."""
    if not video_specs:
        return {"result": "待核验", "reason": "视频中未声明规格"}
    if not product_skus:
        return {"result": "待核验", "reason": "SKU 信息缺失"}

    mismatches = []
    for key, video_val in video_specs.items():
        video_str = str(video_val).strip()
        sku_vals = {
            str(sku.get(key, sku.get("spec", {}).get(key, ""))).strip()
            for sku in product_skus
        }
        sku_vals.discard("")
        if not sku_vals:
            mismatches.append(f"{key}: 视频声明 {video_str}，SKU 中未找到 {key}")
        elif video_str not in sku_vals:
            mismatches.append(f"{key}: 视频声明 {video_str}，SKU 值为 {', '.join(sorted(sku_vals))}")

    if not mismatches:
        return {"result": "一致", "reason": "所有规格声明与 SKU 匹配"}
    return {"result": "不一致", "reason": "; ".join(mismatches)}


def compare_price(
    video_price: str,
    product_skus: list[dict],
    current_price: str = "",
) -> dict:
    """Compare price claims."""
    video_price = _to_str(video_price)
    if not video_price:
        return {"result": "待核验", "reason": "视频中未声明价格"}

    sku_prices = [sku.get("price") for sku in product_skus if sku.get("price") is not None]
    video_values = _price_values(video_price)
    product_values = _price_values(current_price, *sku_prices)
    if not video_values:
        return {"result": "待核验", "reason": f"无法解析视频价格: {video_price}"}
    if not product_values:
        return {"result": "待核验", "reason": "详情页当前价和 SKU 价格均缺失"}

    matched = video_values & product_values
    reason = f"视频: {_format_prices(video_values)}；详情页/SKU: {_format_prices(product_values)}"
    if matched:
        return {"result": "一致", "reason": reason}
    return {"result": "不一致", "reason": reason}


def compare_activity(video_activity: str, product_rules: str) -> dict:
    """Compare a stated promotion with the captured activity rules."""
    video_activity = _to_str(video_activity)
    product_rules = _to_str(product_rules)
    if not video_activity:
        return {"result": "待核验", "reason": "视频中未声明活动规则"}
    if not product_rules:
        return {"result": "待核验", "reason": "详情页活动规则缺失"}

    compact_video = re.sub(r"[\s，,。；;：:]", "", video_activity)
    compact_product = re.sub(r"[\s，,。；;：:]", "", product_rules)
    if compact_video == compact_product or compact_video in compact_product:
        return {"result": "一致", "reason": f"视频活动可在详情页规则中确认: {video_activity}"}
    return {"result": "不一致", "reason": f"视频: {video_activity}；详情页: {product_rules}"}


def compare_gifts(video_gifts: list[str], product_gifts: list[str]) -> dict:
    """Compare gift claims."""
    if not video_gifts:
        return {"result": "待核验", "reason": "视频中未提及赠品"}
    if not product_gifts:
        return {"result": "待核验", "reason": "赠品规则信息缺失"}
    video_set = set(_to_str(g) for g in video_gifts)
    product_set = set(_to_str(g) for g in product_gifts)
    if video_set == product_set:
        return {"result": "一致", "reason": f"赠品: {', '.join(video_set)}"}
    missing_in_video = product_set - video_set
    missing_in_product = video_set - product_set
    parts = []
    if missing_in_product:
        parts.append(f"视频提及但规则中无: {', '.join(missing_in_product)}")
    if missing_in_video:
        parts.append(f"规则中有但视频未说明: {', '.join(missing_in_video)}")
    return {"result": "不一致" if missing_in_product else "不完整", "reason": "; ".join(parts)}


def compare_product_link(
    product_info: dict,
    video_claims: dict,
    include_qianchuan: bool = True,
) -> dict:
    """Run all consistency comparisons and return a structured report."""

    product = ProductInfo(
        url=product_info.get("url", ""),
        title=product_info.get("title", ""),
        brand=product_info.get("brand", ""),
        skus=product_info.get("skus", []),
        current_price=product_info.get("current_price", ""),
        original_price=product_info.get("original_price", ""),
        gifts=product_info.get("gifts", []),
        activity_rules=product_info.get("activity_rules", ""),
        activity_deadline=product_info.get("activity_deadline", ""),
        category=product_info.get("category", ""),
        captured_at=product_info.get("captured_at", ""),
    )
    claims = VideoClaims(
        source=video_claims.get("source", ""),
        brand_mentioned=video_claims.get("brand_mentioned", ""),
        style_color_mentioned=video_claims.get("style_color_mentioned", ""),
        specs_mentioned=video_claims.get("specs_mentioned", {}),
        price_mentioned=video_claims.get("price_mentioned", ""),
        original_price_mentioned=video_claims.get("original_price_mentioned", ""),
        gifts_mentioned=video_claims.get("gifts_mentioned", []),
        activity_mentioned=video_claims.get("activity_mentioned", ""),
        deadline_mentioned=video_claims.get("deadline_mentioned", ""),
        efficacy_claims=video_claims.get("efficacy_claims", []),
        data_claims=video_claims.get("data_claims", []),
    )

    results: list[dict] = []

    # Brand
    results.append({
        "check_id": "商品三一致-品牌",
        "label": "品牌一致性",
        **compare_str(claims.brand_mentioned, product.brand),
    })

    # Style/color
    results.append({
        "check_id": "商品三一致-款式颜色",
        "label": "款式颜色一致性",
        **compare_str(claims.style_color_mentioned, product.title),
    })

    # Specs
    results.append({
        "check_id": "商品三一致-规格",
        "label": "规格一致性",
        **compare_specs(claims.specs_mentioned, product.skus),
    })

    # Price
    results.append({
        "check_id": "商品三一致-价格",
        "label": "价格一致性",
        **compare_price(claims.price_mentioned, product.skus, product.current_price),
    })

    if claims.original_price_mentioned:
        results.append({
            "check_id": "商品三一致-原价",
            "label": "原价/划线价一致性",
            **compare_price(claims.original_price_mentioned, [], product.original_price),
        })

    # Gifts
    results.append({
        "check_id": "赠品条件完整性",
        "label": "赠品一致性",
        **compare_gifts(claims.gifts_mentioned, product.gifts),
    })

    if claims.activity_mentioned or product.activity_rules:
        results.append({
            "check_id": "活动规则完整性",
            "label": "活动规则一致性",
            **compare_activity(claims.activity_mentioned, product.activity_rules),
        })

    # Activity deadline
    results.append({
        "check_id": "活动期限完整性",
        "label": "活动期限一致性",
        **compare_str(claims.deadline_mentioned, product.activity_deadline),
    })

    # Efficacy claims risk
    if claims.efficacy_claims:
        results.append({
            "check_id": "电商功效声明",
            "label": "功效声明风险",
            "result": "待核验",
            "reason": f"视频声明: {'; '.join(claims.efficacy_claims[:5])}；需提供资质证据",
        })

    if claims.data_claims:
        results.append({
            "check_id": "电商数据声明",
            "label": "销量/好评等数据声明",
            "result": "待核验",
            "reason": f"视频声明: {'; '.join(claims.data_claims[:5])}；需核验数据来源、口径和时间范围",
        })

    # Regulated category
    category_risk = None
    for cat_key in sorted(REGULATED_CATEGORIES, key=len, reverse=True):
        cat_info = REGULATED_CATEGORIES[cat_key]
        if cat_key in product.category or cat_key in product.title:
            category_risk = {
                "check_id": f"行业资质-{cat_key}",
                "label": f"{cat_key}行业资质",
                "result": "待核验",
                "reason": f"需要: {cat_info['requires']}",
                "risk": cat_info["risk"],
            }
            break

    report = {
        "schema_version": "1.0",
        "product_url": product.url,
        "captured_at": product.captured_at or "未记录",
        "comparisons": results,
        "category_risk": category_risk,
        "qianchuan_flags": [],
    }

    # Qianchuan checks (manual flags, default to unchecked)
    if include_qianchuan:
        report["qianchuan_checks"] = [
            {"check_id": qc["check_id"], "description": qc["description"], "flagged": None}
            for qc in QIANCHUAN_CHECKS
        ]
        report["qianchuan_note"] = (
            "千川低质素材检查项需人工逐帧审核。详见 references/qianchuan-low-quality.md。"
        )

    return report


def render_json(report: dict) -> str:
    return json.dumps(report, ensure_ascii=False, indent=2)


def render_markdown(report: dict) -> str:
    lines = [
        "## 商品一致性审核",
        "",
        f"- 商品链接: {report.get('product_url', '未提供')}",
        f"- 抓取时间: {report.get('captured_at', '未记录')}",
        "",
        "| 检查项 | 结果 | 说明 |",
        "| --- | --- | --- |",
    ]
    for comp in report.get("comparisons", []):
        lines.append(
            f"| {comp['label']} | **{comp['result']}** | {comp.get('reason', '')} |"
        )

    if report.get("category_risk"):
        cr = report["category_risk"]
        lines.extend([
            "",
            f"### 行业资质风险: {cr['label']}",
            f"- 等级: **{cr.get('risk', '未知')}**",
            f"- 原因: {cr.get('reason', '')}",
        ])

    if report.get("qianchuan_checks"):
        lines.extend([
            "",
            "### 千川低质素材检查",
            "",
            "| 检查项 | 是否触发 | 说明 |",
            "| --- | --- | --- |",
        ])
        for qc in report["qianchuan_checks"]:
            flagged = qc.get("flagged")
            status = "⚠️ 待审核" if flagged is None else ("❌ 触发" if flagged else "✅ 通过")
            lines.append(f"| {qc['description']} | {status} | |")
        lines.extend(["", f"> {report.get('qianchuan_note', '')}"])

    return "\n".join(lines) + "\n"


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(
        description="Compare product detail page info with video claims for e-commerce compliance."
    )
    parser.add_argument("--product", required=True, help="Path to product info JSON file")
    parser.add_argument("--claims", required=True, help="Path to video claims JSON file")
    parser.add_argument("--format", choices=("markdown", "json"), default="markdown")
    parser.add_argument("--no-qianchuan", action="store_true", help="Skip 千川 checks")

    args = parser.parse_args(argv)

    try:
        with open(args.product) as f:
            product_info = json.load(f)
        with open(args.claims) as f:
            video_claims = json.load(f)
    except (FileNotFoundError, json.JSONDecodeError) as exc:
        print(f"Error reading input files: {exc}", file=sys.stderr)
        return 1

    report = compare_product_link(
        product_info,
        video_claims,
        include_qianchuan=not args.no_qianchuan,
    )

    if args.format == "json":
        print(render_json(report))
    else:
        print(render_markdown(report))
    return 0


if __name__ == "__main__":
    raise SystemExit(main(sys.argv[1:]))
