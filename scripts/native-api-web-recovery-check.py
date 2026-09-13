"""Crash only supervised API/Web process groups and verify recovery. Leaves PostgreSQL alone."""
import json
import os
import signal
import subprocess
import time
import urllib.request
from pathlib import Path

root = Path(__file__).resolve().parent.parent
control = ["supervisorctl", "-c", str(root / "scripts/native-supervisor.conf")]


def pid(name: str) -> int:
    return int(subprocess.check_output(control + ["pid", name], text=True).strip())


def status(name: str) -> str:
    return subprocess.run(control + ["status", name], capture_output=True, text=True).stdout


results = []
for name in ("api", "web"):
    old = pid(name)
    if old <= 1 or os.getpgid(old) != old:
        raise RuntimeError(f"Refusing unexpected process group for {name}: {old}")
    os.killpg(old, signal.SIGKILL)
    started = time.monotonic()
    while time.monotonic() - started < 120:
        time.sleep(1)
        try:
            new = pid(name)
        except Exception:
            continue
        state = status(name)
        if new > 1 and new != old and "RUNNING" in state:
            break
    else:
        raise RuntimeError(f"{name} did not recover: {status(name)}")
    result = {
        "service": name,
        "old_pid": old,
        "new_pid": new,
        "running_seconds": round(time.monotonic() - started, 2),
    }
    if name == "web":
        url = "http://100.109.104.61:13000/loretide-dev-check/diagnostics"
    else:
        url = "http://127.0.0.1:18000/health"
    # wait until HTTP succeeds
    http_started = time.monotonic()
    while True:
        try:
            with urllib.request.urlopen(url, timeout=30) as response:
                body = response.read(200)
                result["http_status"] = response.status
                result["ready_seconds"] = round(time.monotonic() - started, 2)
                result["body_prefix"] = body[:80].decode("utf-8", "replace")
                break
        except Exception as exc:
            if time.monotonic() - http_started > 180:
                raise RuntimeError(f"{name} HTTP not ready: {exc}") from exc
            time.sleep(2)
    results.append(result)
    print(json.dumps({k: v for k, v in result.items() if k != "body_prefix"}), flush=True)

out = root / "data/native/api-web-recovery-check.json"
out.write_text(json.dumps(results, indent=2) + "\n")
print("wrote", out)
