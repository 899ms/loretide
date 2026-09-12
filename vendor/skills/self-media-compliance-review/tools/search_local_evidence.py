#!/usr/bin/env python3
"""Search the repository's published compliance references without network access."""

from __future__ import annotations

import argparse
import json
import re
import sys
from dataclasses import dataclass
from pathlib import Path
from typing import Iterable


REPO_ROOT = Path(__file__).resolve().parents[1]
DEFAULT_PATHS = (REPO_ROOT / "references", REPO_ROOT / "docs" / "sources.md")

PLATFORM_ALIASES = {
    "xiaohongshu": ("小红书", "xiaohongshu", "red"),
    "douyin": ("抖音", "douyin"),
    "kuaishou": ("快手", "kuaishou"),
    "bilibili": ("b站", "哔哩哔哩", "bilibili"),
    "wechat-channels": ("视频号", "wechat channels"),
    "wechat-official-account": ("公众号", "微信公众平台", "wechat official"),
    "douyin-ecommerce": ("抖音电商", "抖店", "千川", "douyin e-commerce"),
}

QUERY_EXPANSIONS = {
    "导流": ("引流", "站外", "联系方式", "微信", "私信", "二维码", "交易导流"),
    "引流": ("导流", "站外", "联系方式", "微信", "私信", "二维码", "交易导流"),
    "限流": ("低浏览", "不收录", "小眼睛", "减少推荐", "0播放", "不推流"),
    "申诉": ("复核", "解封", "恢复", "整改", "处罚通知"),
    "封号": ("封禁", "账号限制", "解封"),
    "搬运": ("原创", "转载", "盗图", "抄袭", "版权"),
    "带货": ("商品", "挂车", "小黄车", "营销", "抖店", "千川"),
    "功效": ("疗效", "效果承诺", "医疗", "减肥", "保健", "前后对比"),
    "未成年": ("儿童", "学生", "未成年人"),
    "ai": ("aigc", "人工智能", "合成内容", "生成式人工智能"),
}

QUERY_KEYWORDS = {
    "笔记",
    "视频",
    "直播",
    "评论",
    "账号",
    "违规",
    "处罚",
    "申诉",
    "封号",
    "封禁",
    "限流",
    "导流",
    "引流",
    "搬运",
    "版权",
    "营销",
    "带货",
    "商品",
    "功效",
    "承诺",
    "资质",
    "价格",
    "未成年",
    "医疗",
    "金融",
    "ai",
    "aigc",
}

SOURCE_TYPES = {
    "references/cases/": "案例库",
    "references/recent-cases": "案例索引",
    "references/": "规则参考",
    "docs/sources.md": "来源清单",
}


@dataclass(frozen=True)
class Section:
    path: Path
    line: int
    heading: str
    body: str


def _relative(path: Path) -> str:
    return path.resolve().relative_to(REPO_ROOT).as_posix()


def source_type(path: Path) -> str:
    relative = _relative(path)
    for prefix, label in SOURCE_TYPES.items():
        if relative.startswith(prefix):
            return label
    return "本地参考"


def iter_markdown_files(paths: Iterable[Path] = DEFAULT_PATHS) -> Iterable[Path]:
    for path in paths:
        if path.is_file() and path.suffix.lower() == ".md":
            yield path
        elif path.is_dir():
            yield from sorted(path.rglob("*.md"))


def parse_sections(path: Path) -> list[Section]:
    lines = path.read_text(encoding="utf-8").splitlines()
    sections: list[Section] = []
    heading = path.stem
    start_line = 1
    body: list[str] = []

    def flush() -> None:
        text = "\n".join(body).strip()
        if text:
            sections.append(Section(path=path, line=start_line, heading=heading, body=text))

    for line_number, line in enumerate(lines, 1):
        match = re.match(r"^(#{1,4})\s+(.+?)\s*$", line)
        if match:
            flush()
            heading = match.group(2)
            start_line = line_number
            body = []
        else:
            body.append(line)
    flush()
    return sections


def _query_terms(query: str) -> tuple[list[str], set[str]]:
    normalized = query.casefold().strip()
    raw_tokens = re.findall(r"[a-z0-9][a-z0-9_-]*|[\u4e00-\u9fff]{2,}", normalized)
    explicit: list[str] = []
    for token in raw_tokens:
        # Long Chinese runs are usually sentences without spaces. Known intent
        # words below provide more useful matches than the whole run.
        if not re.fullmatch(r"[\u4e00-\u9fff]+", token) or len(token) <= 4:
            explicit.append(token)
    vocabulary = QUERY_KEYWORDS | set(QUERY_EXPANSIONS)
    for keyword in sorted(vocabulary, key=lambda value: (-len(value), value)):
        if keyword in normalized and keyword not in explicit:
            explicit.append(keyword)
    terms: list[str] = []
    for token in explicit:
        if token not in terms:
            terms.append(token)
        for needle, expansions in QUERY_EXPANSIONS.items():
            if needle in token:
                for expansion in expansions:
                    if expansion not in terms:
                        terms.append(expansion)
    return terms, set(explicit)


def _platform_terms(platform: str | None) -> tuple[str, ...]:
    if not platform:
        return ()
    key = platform.casefold().strip()
    if key in PLATFORM_ALIASES:
        return PLATFORM_ALIASES[key]
    for aliases in PLATFORM_ALIASES.values():
        if key in (alias.casefold() for alias in aliases):
            return aliases
    return (platform,)


def _score(section: Section, terms: list[str], explicit: set[str], platform_terms: tuple[str, ...]) -> tuple[int, list[str]]:
    heading = section.heading.casefold()
    body = section.body.casefold()
    relative = _relative(section.path).casefold()
    matched: list[str] = []
    score = 0
    for term in terms:
        folded = term.casefold()
        heading_hits = heading.count(folded)
        body_hits = body.count(folded)
        path_hits = relative.count(folded)
        if heading_hits or body_hits or path_hits:
            matched.append(term)
            weight = 6 if term in explicit else 2
            score += heading_hits * weight * 3
            score += min(body_hits, 5) * weight
            score += path_hits * weight * 2
    if not matched:
        return 0, []
    if platform_terms:
        # Platform-specific files and headings are authoritative routing
        # signals. Only the cross-platform index needs body-level routing.
        platform_scope = f"{relative} {heading}"
        if relative == "references/recent-cases-2025-2026.md":
            platform_scope += f" {body}"
        platform_match = any(term.casefold() in platform_scope for term in platform_terms)
        if not platform_match:
            return 0, []
        score += 8
    explicit_matches = sum(1 for term in explicit if term in matched)
    score += explicit_matches * explicit_matches * 4
    return score, matched


def _excerpt(section: Section, matched_terms: list[str], limit: int = 320) -> str:
    text = re.sub(r"\s+", " ", section.body).strip()
    positions = [text.casefold().find(term.casefold()) for term in matched_terms]
    positions = [position for position in positions if position >= 0]
    start = max(0, (min(positions) if positions else 0) - 80)
    excerpt = text[start : start + limit]
    if start:
        excerpt = "..." + excerpt
    if start + limit < len(text):
        excerpt += "..."
    return excerpt


def search_local(query: str, platform: str | None = None, limit: int = 8) -> dict:
    terms, explicit = _query_terms(query)
    platform_terms = _platform_terms(platform)
    ranked = []
    for path in iter_markdown_files():
        for section in parse_sections(path):
            score, matched = _score(section, terms, explicit, platform_terms)
            if score:
                ranked.append((score, section, matched))
    ranked.sort(key=lambda item: (-item[0], _relative(item[1].path), item[1].line))

    results = []
    for score, section, matched in ranked[: max(0, limit)]:
        results.append(
            {
                "source_type": source_type(section.path),
                "path": _relative(section.path),
                "line": section.line,
                "heading": section.heading,
                "score": score,
                "matched_terms": matched,
                "excerpt": _excerpt(section, matched),
            }
        )
    return {
        "search_mode": "local",
        "network_used": False,
        "query": query,
        "platform": platform or "",
        "searched_paths": ["references/**/*.md", "docs/sources.md"],
        "result_count": len(results),
        "results": results,
        "limitations": "仅检索仓库随 Skill 发布的静态语料；不是实时平台数据。",
    }


def render_markdown(report: dict) -> str:
    lines = [
        "## 本地证据检索",
        "",
        f"- 查询: {report['query']}",
        f"- 平台: {report['platform'] or '不限'}",
        "- 网络访问: 否",
        f"- 限制: {report['limitations']}",
        "",
    ]
    for item in report["results"]:
        lines.extend(
            [
                f"### {item['heading']}",
                "",
                f"- 来源: `{item['path']}:{item['line']}` ({item['source_type']})",
                f"- 命中词: {' / '.join(item['matched_terms'])}",
                f"- 摘要: {item['excerpt']}",
                "",
            ]
        )
    if not report["results"]:
        lines.append("未在本地静态语料中找到匹配内容。")
    return "\n".join(lines).rstrip() + "\n"


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description="Search published local compliance references without network access.")
    parser.add_argument("--query", required=True)
    parser.add_argument("--platform")
    parser.add_argument("--limit", type=int, default=8)
    parser.add_argument("--format", choices=("markdown", "json"), default="markdown")
    args = parser.parse_args(argv)

    report = search_local(args.query, platform=args.platform, limit=args.limit)
    if args.format == "json":
        print(json.dumps(report, ensure_ascii=False, indent=2))
    else:
        print(render_markdown(report), end="")
    return 0


if __name__ == "__main__":
    raise SystemExit(main(sys.argv[1:]))
