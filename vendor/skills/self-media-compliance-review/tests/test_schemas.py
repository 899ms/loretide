import json
from pathlib import Path


def test_published_json_schemas_are_valid_json_with_versions():
    schema_dir = Path("schemas")
    expected = {
        "compliance-report.schema.json",
        "product-consistency.schema.json",
        "video-evidence-manifest.schema.json",
    }

    assert {path.name for path in schema_dir.glob("*.json")} == expected
    for name in expected:
        schema = json.loads((schema_dir / name).read_text())
        assert schema["$schema"] == "https://json-schema.org/draft/2020-12/schema"
        assert schema["type"] == "object"
        assert "schema_version" in schema["properties"]
        assert "schema_version" in schema["required"]

