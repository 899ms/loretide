from pathlib import Path

from tools.tikhub.lib import tikhub_client
from tools.tikhub.lib.tikhub_client import _env_files

REPO_ROOT = Path(__file__).resolve().parents[1]


def test_env_files_point_at_repo_root(monkeypatch):
    monkeypatch.delenv("TIKHUB_ENV_FILE", raising=False)
    monkeypatch.delenv("TIKHUB_NO_ENV_FILE", raising=False)

    files = _env_files()

    assert REPO_ROOT / ".env" in files
    assert REPO_ROOT.parent / ".env" not in files


def test_env_files_honor_custom_file_and_no_env_file(monkeypatch):
    custom = Path("/tmp/custom.env")
    monkeypatch.setenv("TIKHUB_ENV_FILE", str(custom))
    monkeypatch.setenv("TIKHUB_NO_ENV_FILE", "1")

    assert _env_files() == [custom]


def test_load_api_key_prefers_process_env(monkeypatch):
    monkeypatch.setenv("TIKHUB_API_KEY", "from-env")
    monkeypatch.setenv("TIKHUB_NO_ENV_FILE", "1")

    assert tikhub_client.load_api_key() == "from-env"


def test_api_base_url_defaults_to_documented_rest_host(monkeypatch):
    monkeypatch.delenv("TIKHUB_API_BASE_URL", raising=False)

    assert tikhub_client.api_base_url() == "https://api.tikhub.io"


def test_api_base_url_can_use_mainland_rest_host(monkeypatch):
    monkeypatch.setenv("TIKHUB_API_BASE_URL", "https://api.tikhub.dev/")

    assert tikhub_client.api_base_url() == "https://api.tikhub.dev"
