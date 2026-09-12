#!/usr/bin/env python3
"""Extract claims from video scripts, subtitles, or marketing copy for compliance review."""

from __future__ import annotations

import argparse
import json
import re
import sys

# Patterns that flag risky claims
ABSOLUTE_PATTERNS = [
    r"最好",
    r"第一[名品牌个]",
    r"唯一",
    r"全网最低",
    r"100%",
    r"绝对",
    r"保证",
    r"肯定",
]

EFFICACY_PATTERNS = [
    r"(必|一定|肯定|绝对)(能|会|可以)?(瘦|白|美|高|好|有效|有用|成功)",
    r"(根治|治愈|永不复发|彻底消除|完全去除)",
    r"(\d+天)(内|就|之|以)?(内|后|能)?(瘦|白|美|祛痘|去皱|淡斑|美白|变白|见效|好|有效)",
    r"(包过|稳赚|必赚|稳赢|必涨)",
    r"(不.*就|否则)(会|能|可以).{0,10}(瘦|白|胖|丑|老|病)",
    r"(医生|专家|院士|教授)(推荐|认可|背书|代言)",
    r"药(妆|品|用).{0,5}(级别|标准|成分|效果)",
    r"医学(护肤|级别|认证|研究)",
    r"(干细胞|基因|修复|再生|抗衰|逆龄).{0,5}(技术|成分|效果|因子)",
    r"保证.{0,5}(能|会|可以).{0,5}(瘦|白|美|好|有效)",
]

PRICE_PATTERNS = [
    r"(到手价|券后价|折后价|最终价|只需|只要)[^\d\n]*(\d+\.?\d*)",
    r"(原价|市场价|专柜价|吊牌价)[^\d\n]*(\d+\.?\d*)",
    r"(限时|限量|最后|仅剩|即将涨价|马上涨价)",
    r"(买\d+送\d+|买一送一|第二[件半价])",
    r"(赠品|赠送|白送|免费送|加赠)",
    r"(全网最低|史上最低|历史最低|破底价)",
]

DATA_PATTERNS = [
    r"(销量|已售|卖出)[^\d\n]*(\d+[万多亿]?)",
    r"(好评率|好评|口碑)[^\d\n]*(\d+\.?\d*%?)",
    r"(复购率|回头率)[^\d\n]*(\d+\.?\d*%?)",
    r"(排名|排行)[^\d\n]*([第前]\d+)",
    r"(\d+[万多亿]?\+?\s*(人|位|个|名).{0,5}(在用|选择|购买|推荐|使用|好评))",
]

COMPARISON_PATTERNS = [
    r"(比|和).{0,10}(好|强|牛|厉害|便宜|有效)\d+倍",
    r"(别人|其他|别家|别的地方).{0,5}(没有|做不到|不行|差远了)",
    r"(市面|全网|所有|任何).{0,5}(唯一|独有|仅有|找不到)",
    r"(传统|普通|一般|以前|别的).{0,5}(产品|方法|品牌).{0,5}(不行|没用|伤害|危害|缺点)",
]

REGULATED_TOPIC_PATTERNS = [
    (r"(治疗|治愈|主治|医治).{0,10}(疾病|病症|症状|病人)", "医疗功效"),
    (r"(减肥|瘦身|减重|减脂|瘦脸|瘦腿|瘦肚子)", "减肥功效"),
    (r"(美白|祛斑|淡斑|去皱|抗皱|紧致|提拉|抗衰)", "化妆品功效"),
    (r"(降血压|降血糖|降血脂|排毒|通便|养胃|护肝|补肾)", "保健功效"),
    (r"(通过率|录取率|上岸|提分|涨分|高分|满分)", "教育效果"),
    (r"(收益率|回报率|年化|稳赚|保本|无风险|翻倍)", "金融承诺"),
]

NEGATING_CONTEXT = re.compile(r"(?:不|并不|不能|不会|不要|别|禁止|避免|删除|去掉|慎用|不能写|不能说)$")


def _is_negated_or_instructional(text: str, start: int) -> bool:
    prefix = re.sub(r"\s+", "", text[max(0, start - 10) : start])
    return bool(NEGATING_CONTEXT.search(prefix))


def extract_claims(text: str) -> dict:
    """Extract all claim types from text and classify risk levels."""

    results: dict[str, list[dict]] = {
        "absolutes": [],
        "efficacy": [],
        "price": [],
        "data": [],
        "comparisons": [],
        "regulated_topics": [],
    }

    for pattern in ABSOLUTE_PATTERNS:
        for match in re.finditer(pattern, text):
            if _is_negated_or_instructional(text, match.start()):
                continue
            results["absolutes"].append({
                "text": match.group(),
                "position": match.start(),
                "risk": "high",
                "reason": "绝对化用语",
                "fix": f"删除「{match.group()}」或改为「受欢迎的」「热销的」等非绝对化表达",
            })

    for pattern in EFFICACY_PATTERNS:
        for match in re.finditer(pattern, text):
            if _is_negated_or_instructional(text, match.start()):
                continue
            results["efficacy"].append({
                "text": match.group(),
                "position": match.start(),
                "risk": "high",
                "reason": "功效承诺",
                "fix": f"删除「{match.group()}」或改为个人体验描述并标注「个人使用感受，因人而异」",
            })

    for pattern in PRICE_PATTERNS:
        for match in re.finditer(pattern, text):
            if _is_negated_or_instructional(text, match.start()):
                continue
            results["price"].append({
                "text": match.group(),
                "position": match.start(),
                "risk": "medium",
                "reason": "价格/促销声明",
                "fix": "确保价格声明与链接一致；标注活动期限和条件",
            })

    for pattern in DATA_PATTERNS:
        for match in re.finditer(pattern, text):
            if _is_negated_or_instructional(text, match.start()):
                continue
            results["data"].append({
                "text": match.group(),
                "position": match.start(),
                "risk": "medium",
                "reason": "数据声明",
                "fix": "补充数据来源、统计口径和时间范围，或删除无出处数据",
            })

    for pattern in COMPARISON_PATTERNS:
        for match in re.finditer(pattern, text):
            if _is_negated_or_instructional(text, match.start()):
                continue
            results["comparisons"].append({
                "text": match.group(),
                "position": match.start(),
                "risk": "high",
                "reason": "贬低竞品或无法验证的比较",
                "fix": "删除对比表达，改为客观描述本产品特点",
            })

    for pattern, topic_label in REGULATED_TOPIC_PATTERNS:
        for match in re.finditer(pattern, text):
            if _is_negated_or_instructional(text, match.start()):
                continue
            results["regulated_topics"].append({
                "text": match.group(),
                "position": match.start(),
                "risk": "blocker" if any(kw in match.group() for kw in ["治疗", "治愈", "根治"]) else "high",
                "reason": f"涉及「{topic_label}」监管领域",
                "fix": f"删除「{match.group()}」；{topic_label}类声明须提供资质证据或全部移除",
            })

    return results


def summarize_claims(claims: dict) -> dict:
    """Summarize extracted claims into forbidden / risky / needs-evidence lists."""

    forbidden: list[str] = []
    risky: list[str] = []
    needs_evidence: list[str] = []

    for claim in claims.get("absolutes", []) + claims.get("comparisons", []):
        if claim["risk"] in ("high", "blocker"):
            forbidden.append(claim["text"])

    for claim in claims.get("efficacy", []):
        if claim["risk"] == "blocker":
            forbidden.append(claim["text"])
        else:
            risky.append(claim["text"])

    for claim in claims.get("regulated_topics", []):
        if claim["risk"] == "blocker":
            forbidden.append(claim["text"])
        else:
            needs_evidence.append(claim["text"])

    for claim in claims.get("price", []) + claims.get("data", []):
        needs_evidence.append(claim["text"])

    return {
        "forbidden_expressions": sorted(set(forbidden)),
        "risky_expressions": sorted(set(risky)),
        "needs_evidence": sorted(set(needs_evidence)),
        "total_claims": sum(len(v) for v in claims.values()),
        "summary": (
            f"提取 {sum(len(v) for v in claims.values())} 个声明: "
            f"{len(set(forbidden))} 个禁用、{len(set(risky))} 个高风险、{len(set(needs_evidence))} 个须补证据"
        ),
    }


def render_json(claims: dict, summary: dict) -> str:
    return json.dumps(
        {"claims": claims, "summary": summary},
        ensure_ascii=False,
        indent=2,
    )


def render_markdown(claims: dict, summary: dict) -> str:
    lines = [
        "## 声明提取与风险评估",
        "",
        f"- {summary['summary']}",
        "",
    ]

    if summary["forbidden_expressions"]:
        lines.extend([
            "### 🚫 禁用表达（必须删除）",
            "",
        ])
        for expr in summary["forbidden_expressions"]:
            lines.append(f"- ~~{expr}~~")
        lines.append("")

    if summary["risky_expressions"]:
        lines.extend([
            "### ⚠️ 高风险表达（建议删除或改写）",
            "",
        ])
        for expr in summary["risky_expressions"]:
            lines.append(f"- {expr}")
        lines.append("")

    if summary["needs_evidence"]:
        lines.extend([
            "### 📋 须补证据（保留须提供证据）",
            "",
        ])
        for expr in summary["needs_evidence"]:
            lines.append(f"- {expr} → 待核验")
        lines.append("")

    if claims:
        lines.extend([
            "### 详细声明对照",
            "",
            "| 类型 | 文本 | 风险 | 说明 | 修改建议 |",
            "| --- | --- | --- | --- | --- |",
        ])
        for category, items in claims.items():
            for item in items:
                lines.append(
                    f"| {category} | {item['text']} | **{item['risk']}** | "
                    f"{item['reason']} | {item['fix']} |"
                )
        lines.append("")

    return "\n".join(lines)


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(
        description="Extract claims from scripts/copy for e-commerce compliance review."
    )
    parser.add_argument("--text", help="Text to analyze")
    parser.add_argument("--file", help="Path to file containing text to analyze")
    parser.add_argument("--format", choices=("markdown", "json"), default="markdown")

    args = parser.parse_args(argv)

    if args.file:
        try:
            with open(args.file) as f:
                text = f.read()
        except (FileNotFoundError, OSError) as exc:
            print(f"Error reading file: {exc}", file=sys.stderr)
            return 1
    elif args.text:
        text = args.text
    else:
        print("Either --text or --file is required.", file=sys.stderr)
        return 1

    claims = extract_claims(text)
    summary = summarize_claims(claims)

    if args.format == "json":
        print(render_json(claims, summary))
    else:
        print(render_markdown(claims, summary))
    return 0


if __name__ == "__main__":
    raise SystemExit(main(sys.argv[1:]))
