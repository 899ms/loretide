import importlib.util
import json
from pathlib import Path

import pytest


REPO_ROOT = Path(__file__).resolve().parents[1]


def load_client():
    path = REPO_ROOT / "tools" / "tikhub" / "lib" / "tikhub_client.py"
    spec = importlib.util.spec_from_file_location("tikhub_rest_client_under_test", path)
    module = importlib.util.module_from_spec(spec)
    assert spec.loader is not None
    spec.loader.exec_module(module)
    return module


def load_refresh(client_module):
    path = REPO_ROOT / "tools" / "tikhub" / "scripts" / "refresh_tools.py"
    previous = __import__("sys").modules.get("tikhub_client")
    __import__("sys").modules["tikhub_client"] = client_module
    try:
        spec = importlib.util.spec_from_file_location("tikhub_refresh_under_test", path)
        module = importlib.util.module_from_spec(spec)
        assert spec.loader is not None
        spec.loader.exec_module(module)
        return module
    finally:
        if previous is None:
            __import__("sys").modules.pop("tikhub_client", None)
        else:
            __import__("sys").modules["tikhub_client"] = previous


class FakeResponse:
    def __init__(self, payload):
        self.payload = json.dumps(payload).encode()

    def __enter__(self):
        return self

    def __exit__(self, *_args):
        return False

    def read(self):
        return self.payload


def test_get_endpoint_uses_query_and_bearer_auth(monkeypatch):
    module = load_client()
    catalog = [{
        "name": "douyin_web_fetch_one_video",
        "method": "GET",
        "path": "/api/v1/douyin/web/fetch_one_video",
        "queryParameters": ["aweme_id", "need_anchor_info"],
        "pathParameters": [],
    }]
    captured = {}
    monkeypatch.setattr(module, "_load_catalog", lambda _platform: catalog)

    def fake_urlopen(request, timeout):
        captured["request"] = request
        captured["timeout"] = timeout
        return FakeResponse({"code": 200, "data": {"ok": True}})

    monkeypatch.setattr(module.urllib.request, "urlopen", fake_urlopen)
    result = module.TikhubClient("douyin", api_key="secret").call(
        "douyin_web_fetch_one_video",
        {"aweme_id": "123", "need_anchor_info": True},
    )

    assert result["code"] == 200
    request = captured["request"]
    assert request.method == "GET"
    assert "aweme_id=123" in request.full_url
    assert "need_anchor_info=true" in request.full_url
    assert request.get_header("Authorization") == "Bearer secret"
    assert request.data is None


def test_post_endpoint_splits_path_query_and_json_body(monkeypatch):
    module = load_client()
    monkeypatch.setattr(module, "_load_catalog", lambda _platform: [{
        "name": "example_post",
        "method": "POST",
        "path": "/api/v1/douyin/example/{item_id}",
        "queryParameters": ["cursor"],
        "pathParameters": ["item_id"],
    }])
    captured = {}

    def fake_urlopen(request, timeout):
        captured["request"] = request
        return FakeResponse({"code": 200})

    monkeypatch.setattr(module.urllib.request, "urlopen", fake_urlopen)
    module.TikhubClient("douyin", api_key="secret").call(
        "example_post",
        {"item_id": "a/b", "cursor": "2", "keyword": "蛋糕"},
    )

    request = captured["request"]
    assert request.method == "POST"
    assert "/a%2Fb?cursor=2" in request.full_url
    assert json.loads(request.data) == {"keyword": "蛋糕"}


def test_client_rejects_paths_outside_documented_rest_api(monkeypatch):
    module = load_client()
    monkeypatch.setattr(module, "_load_catalog", lambda _platform: [{
        "name": "unsafe",
        "method": "POST",
        "path": "/other/protocol",
        "queryParameters": [],
        "pathParameters": [],
    }])

    with pytest.raises(module.TikhubError, match="refusing non-REST endpoint"):
        module.TikhubClient("douyin", api_key="secret").call("unsafe")


def test_openapi_catalog_preserves_rest_method_and_locations():
    client = load_client()
    refresh = load_refresh(client)
    document = {"paths": {
        "/api/v1/douyin/web/fetch_item/{item_id}": {"post": {
            "summary": "Fetch item",
            "parameters": [
                {"name": "item_id", "in": "path", "required": True, "schema": {"type": "string"}},
                {"name": "cursor", "in": "query", "schema": {"type": "string"}},
            ],
            "requestBody": {"content": {"application/json": {"schema": {
                "type": "object",
                "properties": {"keyword": {"type": "string"}},
                "required": ["keyword"],
            }}}},
        }},
        "/internal/not-supported": {"get": {"summary": "Ignore"}},
    }}

    catalog = refresh.build_catalog(document, "douyin")

    assert len(catalog) == 1
    endpoint = catalog[0]
    assert endpoint["name"] == "douyin_web_fetch_item_item_id"
    assert endpoint["method"] == "POST"
    assert endpoint["path"] == "/api/v1/douyin/web/fetch_item/{item_id}"
    assert endpoint["queryParameters"] == ["cursor"]
    assert endpoint["pathParameters"] == ["item_id"]
    assert endpoint["inputSchema"]["required"] == ["item_id", "keyword"]


def test_cached_catalogs_are_rest_only_and_include_review_endpoints():
    douyin = json.loads((REPO_ROOT / "tools/tikhub/references/tools-douyin.json").read_text())
    xhs = json.loads((REPO_ROOT / "tools/tikhub/references/tools-xiaohongshu.json").read_text())
    for endpoint in douyin + xhs:
        assert endpoint["path"].startswith("/api/v1/")
        assert endpoint["method"] in {"GET", "POST", "PUT", "PATCH", "DELETE"}

    douyin_names = {endpoint["name"] for endpoint in douyin}
    xhs_names = {endpoint["name"] for endpoint in xhs}
    assert "douyin_app_v3_fetch_one_video_by_share_url" in douyin_names
    assert "douyin_app_v3_fetch_video_high_quality_play_url" in douyin_names
    assert "xiaohongshu_app_v2_search_notes" in xhs_names
    assert "xiaohongshu_app_v2_get_note_comments" in xhs_names


def test_tikhub_transport_contains_no_mcp_protocol_or_host():
    client_source = (REPO_ROOT / "tools/tikhub/lib/tikhub_client.py").read_text().lower()
    cli_source = (REPO_ROOT / "tools/tikhub/bin/tikhub").read_text().lower()
    for source in (client_source, cli_source):
        assert "mcp.tikhub.io" not in source
        assert "jsonrpc" not in source
        assert "mcp-session-id" not in source
