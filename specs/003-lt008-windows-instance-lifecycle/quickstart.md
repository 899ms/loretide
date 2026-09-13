# Quickstart: 003 验证指引

前置：PowerShell 7+；`F:\loretide-runtime\pgsql` 存在；仓库根目录。

## 自动检查

```powershell
pwsh -File scripts/local-windows.test.ps1
```

预期：前置缺失（api.exe / secrets.json / 端口占用）各自退出非 0 且输出组件名；陈旧 pid 清理；`-Json` 可被 `ConvertFrom-Json` 解析。

## 手动全流程（SC-001，用户执行并记录）

```powershell
./scripts/local-windows.ps1 start            # 期望：四组件启动，输出浏览器地址
netstat -ano | findstr "15332 18000 13000"    # 记录监听 pid
./scripts/local-windows.ps1 start            # 期望：报告已运行，退出 0
netstat -ano | findstr "15332 18000 13000"    # 期望：与上一次相同，无新增
./scripts/local-windows.ps1 status           # 期望：四行 running、owned=yes、ok=true
./scripts/local-windows.ps1 status -Json | ConvertFrom-Json | Select -Expand components
./scripts/local-windows.ps1 stop             # 期望：等待并报告三组件已退出；postgres 仍 running
& F:\loretide-runtime\pgsql\bin\pg_ctl.exe -D F:\loretide-runtime\pg status   # 期望：running
./scripts/local-windows.ps1 build            # 期望：成功（api.exe 已释放）
./scripts/local-windows.ps1 start
```

## 前置缺失（SC-002）

```powershell
Rename-Item data/windows/api.exe api.exe.bak; ./scripts/local-windows.ps1 start; echo "exit=$LASTEXITCODE"; Rename-Item data/windows/api.exe.bak api.exe
Rename-Item data/windows/secrets.json secrets.json.bak; ./scripts/local-windows.ps1 start; echo "exit=$LASTEXITCODE"; Rename-Item data/windows/secrets.json.bak secrets.json
```

预期：两次均 `exit≠0`，输出分别指出 `api` 与 `secrets`，且不含任何密钥内容。

## 数据保留（SC-003）

停止前后各执行一次 `psql -p 15332 -c "select count(*) from workspace"`，行数一致。

## 手动 UI Todo（用户确认）

1. `start` 后浏览器打开 `http://localhost:13000/loretide-dev-check/diagnostics`，页面可达，诊断概览显示 web 组件健康。
