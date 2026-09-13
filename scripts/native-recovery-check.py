"""Deliberately crash each supervised Loretide service and verify recovery."""
import json
import os
from pathlib import Path
import signal
import subprocess
import time
import urllib.request

root = Path(__file__).resolve().parent.parent
control = ["supervisorctl", "-c", str(root / "scripts/native-supervisor.conf")]

def pid(name):
    return int(subprocess.check_output(control + ["pid", name], text=True).strip())

results = []
for name in ("postgres", "api", "web"):
    old = pid(name)
    if old <= 1 or os.getpgid(old) != old:
        raise RuntimeError(f"Refusing unexpected process group for {name}")
    # Each Supervisor program owns its process group; no unrelated PID is killed.
    os.killpg(old, signal.SIGKILL)
    started = time.monotonic()
    while time.monotonic() - started < 90:
        time.sleep(1)
        new = pid(name)
        state = subprocess.run(control + ["status", name], capture_output=True, text=True).stdout
        if new > 1 and new != old and "RUNNING" in state:
            break
    else:
        raise RuntimeError(f"{name} did not recover")
    result = dict(service=name, old_pid=old, new_pid=new, running_seconds=round(time.monotonic()-started, 2))
    url = "http://100.109.104.61:13000/login" if name == "web" else "http://127.0.0.1:18000/health"
    with urllib.request.urlopen(url, timeout=180) as response:
        response.read()
        result["http_status"] = response.status
    result["ready_seconds"] = round(time.monotonic()-started, 2)
    results.append(result)
    print(json.dumps(result), flush=True)
(root / "data/native/recovery-check.json").write_text(json.dumps(results, indent=2))
