param([ValidateSet('start','stop','status','build')][string]$Action='status')
$ErrorActionPreference='Stop'
$root=Split-Path $PSScriptRoot -Parent
$runtime='F:\loretide-runtime'
$state=Join-Path $root 'data/windows'
$pg=Join-Path $runtime 'pgsql/bin'
if($Action -eq 'build') {
  Push-Location (Join-Path $root 'server')
  try { $env:GOTOOLCHAIN='auto'; go build -o (Join-Path $state 'api.exe') ./cmd/server; if($LASTEXITCODE){throw 'API build failed'} } finally { Pop-Location }
  exit
}
if($Action -eq 'start') {
  & "$pg/pg_ctl.exe" -D "$runtime/pg" status *> $null
  if($LASTEXITCODE -ne 0) { & "$pg/pg_ctl.exe" -D "$runtime/pg" -l "$runtime/postgres.log" -o '-p 15332 -h 127.0.0.1' -w start; if($LASTEXITCODE){throw 'Postgres start failed'} }
  $pidPath=Join-Path $state 'supervisor.pid'
  if(Test-Path $pidPath) { $existing=Get-CimInstance Win32_Process -Filter "ProcessId = $(Get-Content $pidPath)"; if($existing -and $existing.CommandLine -like '*local-windows-supervisor.mjs*'){Write-Output 'Supervisor already running'; exit} }
  Start-Process -FilePath (Get-Command node.exe).Source -ArgumentList @("`"$PSScriptRoot/local-windows-supervisor.mjs`"") -WorkingDirectory $root -WindowStyle Hidden -RedirectStandardOutput "$state/supervisor.out.log" -RedirectStandardError "$state/supervisor.err.log"
  Write-Output 'Local supervisor started. Browser: http://localhost:13000/loretide-dev-check/diagnostics'
} elseif($Action -eq 'stop') {
  New-Item -ItemType File -Force "$state/stop" | Out-Null
  Write-Output 'Graceful application stop requested; database retained and running.'
} else {
  & "$pg/pg_ctl.exe" -D "$runtime/pg" status
  foreach($url in @('http://localhost:18000/health','http://localhost:13000')) { try { $r=Invoke-WebRequest $url -TimeoutSec 10; Write-Output "$url $($r.StatusCode)" } catch { Write-Output "$url unavailable: $($_.Exception.Message)" } }
}
