"""Measure one cache-cold Webpack development route, without deleting caches."""
import concurrent.futures
import datetime
import json
import pathlib
import subprocess
import time
import urllib.request

root = pathlib.Path('/home/opsadmin/loretide-dev')
stamp = datetime.datetime.now(datetime.timezone.utc).strftime('%Y%m%d-%H%M%S')
out = root / 'data' / ('cold-compile-' + stamp)
out.mkdir(parents=True)
compose = ['sudo', '-n', 'docker', 'compose', '--env-file', '.env.loretide-dev', '-f', 'compose.loretide-dev.yaml']

def run(args):
    return subprocess.check_output(args, cwd=root, text=True).strip()

def request():
    start = time.monotonic()
    try:
        with urllib.request.urlopen('http://127.0.0.1:13000/loretide-dev-check/issues', timeout=600) as response:
            body = response.read()
            return dict(status=response.status, bytes=len(body), seconds=time.monotonic()-start)
    except Exception as error:
        return dict(error=str(error), seconds=time.monotonic()-start)

run(compose + ['stop', 'web'])
cache = root / 'apps/web/.next'
backup = root / 'apps/web' / ('.next.before-cold-' + stamp)
if cache.exists():
    run(['sudo', '-n', 'mv', '--', str(cache), str(backup)])
start = time.monotonic()
run(compose + ['up', '-d', '--no-deps', '--force-recreate', 'web'])
result = dict(timestamp=stamp, cache_backup=str(backup), output=str(out), scope='Webpack dev route; not production build or all routes', samples=[])
executor = concurrent.futures.ThreadPoolExecutor(max_workers=1)
future = None
deadline = start + 660
while time.monotonic() < deadline:
    state = json.loads(run(['sudo', '-n', 'docker', 'inspect', 'loretide-dev-web-1', '--format', '{{json .State}}']))
    if not state['Running']:
        result['container_failure'] = state
        break
    try:
        values = run(['sudo', '-n', 'docker', 'exec', 'loretide-dev-web-1', 'sh', '-c', 'cat /sys/fs/cgroup/memory.current /sys/fs/cgroup/memory.peak; cat /sys/fs/cgroup/memory.events']).splitlines()
        sample = dict(seconds=round(time.monotonic()-start, 2), current_bytes=int(values[0]), peak_bytes=int(values[1]))
        result['samples'].append(sample)
        result['memory_events'] = values[2:]
        logs = run(['sudo', '-n', 'docker', 'logs', '--tail', '20', 'loretide-dev-web-1'])
        if future is None and 'Ready in' in logs:
            result['ready_seconds'] = time.monotonic()-start
            future = executor.submit(request)
        if future is not None and future.done():
            result['request'] = future.result()
            break
    except subprocess.CalledProcessError:
        pass
    (out / 'progress.json').write_text(json.dumps(result, indent=2))
    time.sleep(2)
result['total_seconds'] = time.monotonic()-start
result['peak_bytes'] = max((s['peak_bytes'] for s in result['samples']), default=0)
result['timed_out'] = time.monotonic() >= deadline
(out / 'result.json').write_text(json.dumps(result, indent=2))
with (out / 'web.log').open('w') as handle:
    subprocess.run(['sudo', '-n', 'docker', 'logs', 'loretide-dev-web-1'], stdout=handle, stderr=subprocess.STDOUT)
print(json.dumps({k:v for k,v in result.items() if k != 'samples'}, indent=2), flush=True)
executor.shutdown(wait=False)
