# Contract: `./scripts/local-windows.ps1 status -Json`

## Output（stdout，单个 JSON 对象）

```json
{
  "checkout": "F:\\GJ\\内容创作工作台\\app",
  "build": { "commit": "13a0f50", "api_exe_mtime": "2026-09-14T01:20:00+08:00" },
  "components": {
    "postgres":   { "state": "running|stopped", "pid": 1234, "owned": true, "port": 15332, "detail": "data dir F:\\loretide-runtime\\pg" },
    "supervisor": { "state": "running|stopped|stale-pid", "pid": 2345, "owned": true },
    "api":        { "state": "running|stopped|restarting", "pid": 3456, "owned": true, "port": 18000, "health": { "url": "http://127.0.0.1:18000/health", "status": 200 } },
    "web":        { "state": "running|stopped|restarting", "pid": 4567, "owned": true, "port": 13000, "health": { "url": "http://localhost:13000", "status": 200 } }
  },
  "conflicts": [ { "port": 13000, "pid": 9999, "process": "node.exe", "owned": false } ],
  "ok": true
}
```

- `owned`：进程可执行路径 / 命令行前缀与 `checkout` 一致。
- `health.status`：HTTP 状态码；组件未运行时省略 `health`，不发请求。
- `conflicts`：监听目标端口但 `owned=false` 的进程；非空时 `ok=false`。
- `ok`：四组件均 running 且 owned 且无 conflicts。

## Exit codes

| 码 | 含义 |
|---|---|
| 0 | `ok=true` |
| 1 | 至少一个组件未运行，或存在 conflicts |
| 2 | 脚本前置错误（如 pg_ctl 不存在） |

## 人类可读输出（无 `-Json`）

每组件一行：`<name>  <state>  pid=<n>  owned=<yes|no>  <detail>`；conflicts 单独列出；末行 `ok=<true|false>`。字段与 JSON 一一对应。

## 保密

任何输出不包含 `secrets.json` 内容、数据库密码、JWT。
