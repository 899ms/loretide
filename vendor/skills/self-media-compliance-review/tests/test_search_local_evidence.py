from pathlib import Path

from tools.search_local_evidence import Section, _platform_terms, _query_terms, _score
from tools.search_local_evidence import parse_sections, search_local


def test_parse_sections_splits_heading_line_and_body(tmp_path):
    doc = tmp_path / "sample.md"
    doc.write_text("# Title\n\n## First\n\nsome body text\n\n## Second\n\nother text\n")

    sections = parse_sections(doc)

    assert [(s.heading, s.line) for s in sections] == [("First", 3), ("Second", 7)]
    assert "some body text" in sections[0].body
    assert "other text" in sections[1].body


def test_query_terms_keep_short_runs_and_expand_known_keywords():
    terms, explicit = _query_terms("导流 申诉")

    assert {"导流", "申诉"} <= explicit
    assert "引流" in terms
    assert "站外" in terms
    assert "复核" in terms


def test_platform_terms_resolve_aliases():
    assert _platform_terms("小红书")[0] == "小红书"
    assert _platform_terms("xiaohongshu")[0] == "小红书"
    assert _platform_terms("未知平台") == ("未知平台",)


def test_score_platform_filter_rejects_other_platform_sections():
    terms = ["导流", "限流"]
    platform_terms = _platform_terms("小红书")
    douyin_case = Section(
        path=Path("references/cases/douyin.md"), line=1, heading="限流", body="导流 限流 申诉"
    )
    xhs_case = Section(
        path=Path("references/cases/xiaohongshu.md"), line=1, heading="限流", body="导流 限流 申诉"
    )

    assert _score(douyin_case, terms, set(terms), platform_terms)[0] == 0
    assert _score(xhs_case, terms, set(terms), platform_terms)[0] > 0


def test_search_local_finds_case_corpus_and_is_deterministic():
    first = search_local("导流", platform="小红书", limit=5)
    second = search_local("导流", platform="小红书", limit=5)

    assert first["network_used"] is False
    assert first["search_mode"] == "local"
    assert first["result_count"] > 0
    assert first == second
    assert any(r["source_type"] == "案例库" for r in first["results"])
    assert all(
        r["path"].startswith("references/") or r["path"] == "docs/sources.md"
        for r in first["results"]
    )
