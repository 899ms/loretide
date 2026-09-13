<#
.SYNOPSIS
Creates and operates an isolated native Windows Loretide development instance.

.DESCRIPTION
This script never reads data/windows, .env, or model-client configuration. It writes
all generated state below OutputPath and keeps real agent execution disabled.
#>
[CmdletBinding()]
param(
  [ValidateSet('bootstrap','start','stop','status')][string]$Action = 'bootstrap',
  [string]$RuntimePath = 'C:\loretide-runtime',
  [string]$OutputPath = (Join-Path $PSScriptRoot '..\data\bootstrap-windows'),
  [ValidateRange(1024,65535)][int]$WebPort = 13101,
  [ValidateRange(1024,65535)][int]$ApiPort = 18101,
  [ValidateRange(1024,65535)][int]$PostgresPort = 15401,
  [ValidatePattern('^[a-z][a-z0-9_]{2,50}$')][string]$Database = 'loretide_bootstrap_dev',
  [ValidatePattern('^[a-z][a-z0-9_]{2,50}$')][string]$Role = 'loretide_bootstrap_app'
)
Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$root = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
$OutputPath = [IO.Path]::GetFullPath($OutputPath)
$ascii = '^[\x20-\x7E]+$'
if ($RuntimePath -notmatch $ascii -or $OutputPath -notmatch $ascii) { throw 'RuntimePath and OutputPath must be ASCII paths.' }
if ((@($WebPort,$ApiPort,$PostgresPort) | Select-Object -Unique | Measure-Object | Select-Object -ExpandProperty Count) -ne 3) { throw 'WebPort, ApiPort, and PostgresPort must be distinct.' }
if ($Action -eq 'bootstrap' -and -not (Test-Path (Join-Path $OutputPath 'state\instance.json'))) {
  foreach ($port in @($WebPort,$ApiPort,$PostgresPort)) {
    if (Get-NetTCPConnection -State Listen -LocalPort $port -ErrorAction SilentlyContinue) { throw "Port $port is already listening; choose an isolated port." }
  }
}

$pgBin = Join-Path $RuntimePath 'pgsql\bin'
$commands = @('initdb.exe','pg_ctl.exe','psql.exe') | ForEach-Object { Join-Path $pgBin $_ }
foreach ($command in $commands) { if (-not (Test-Path $command)) { throw "Missing PostgreSQL dependency: $command. Install a native PostgreSQL runtime at RuntimePath; Docker is not used." } }
foreach ($command in @('node.exe','go.exe','pnpm.cmd')) { if (-not (Get-Command $command -ErrorAction SilentlyContinue)) { throw "Missing dependency: $command. Install it and retry; no files were created." } }

$state = Join-Path $OutputPath 'state'
$logs = Join-Path $OutputPath 'logs'
$pgData = Join-Path $OutputPath 'postgres'
$configPath = Join-Path $state 'instance.json'
New-Item -ItemType Directory -Force -Path $state,$logs | Out-Null

function New-Secret([int]$Bytes = 32) {
  $buffer = New-Object byte[] $Bytes
  $rng = [Security.Cryptography.RandomNumberGenerator]::Create()
  try { $rng.GetBytes($buffer) } finally { $rng.Dispose() }
  return -join ($buffer | ForEach-Object { $_.ToString('x2') })
}
function Read-Config {
  if (-not (Test-Path $configPath)) { throw "No bootstrap instance at $OutputPath. Run with -Action bootstrap first." }
  return Get-Content -Raw $configPath | ConvertFrom-Json
}
function Write-Config($config) { $config | ConvertTo-Json -Depth 4 | Set-Content -Encoding utf8NoBOM $configPath }
function Invoke-Pg([string[]]$Arguments, [string]$Password) {
  $before = $env:PGPASSWORD; $env:PGPASSWORD = $Password
  try { & (Join-Path $pgBin 'psql.exe') @Arguments; if ($LASTEXITCODE) { throw "psql failed with exit code $LASTEXITCODE" } }
  finally { $env:PGPASSWORD = $before }
}
function Assert-Policy($config) {
  if ($config.executionPolicy -ne 'disabled') { throw 'Refusing to start: real executor policy must remain disabled.' }
}
function Start-Instance($config) {
  Assert-Policy $config
  $apiPid = Join-Path $state 'api.pid'; $webPid = Join-Path $state 'web.pid'
  foreach ($pidFile in @($apiPid,$webPid)) {
    if (Test-Path $pidFile) { $id = [int](Get-Content $pidFile); if (Get-Process -Id $id -ErrorAction SilentlyContinue) { throw "Instance process $id is already running." }; Remove-Item $pidFile -Force }
  }
  & (Join-Path $pgBin 'pg_ctl.exe') -D $pgData status *> $null
  if ($LASTEXITCODE -ne 0) {
    & (Join-Path $pgBin 'pg_ctl.exe') -D $pgData -l (Join-Path $logs 'postgres.log') -o "-p $($config.postgresPort) -h 127.0.0.1" -w start
    if ($LASTEXITCODE) { throw 'PostgreSQL failed to start; inspect the redacted postgres log.' }
  }
  $envBlock = @{ APP_ENV='development'; PORT="$($config.apiPort)"; FRONTEND_PORT="$($config.webPort)"; DATABASE_URL="postgres://$($config.role):$($config.databasePassword)@127.0.0.1:$($config.postgresPort)/$($config.database)?sslmode=disable"; JWT_SECRET=$config.jwtSecret; FRONTEND_ORIGIN="http://127.0.0.1:$($config.webPort)"; CORS_ALLOWED_ORIGINS="http://127.0.0.1:$($config.webPort)"; MULTICA_APP_URL="http://127.0.0.1:$($config.webPort)"; NEXT_PUBLIC_API_URL="http://127.0.0.1:$($config.apiPort)"; NEXT_PUBLIC_WS_URL="ws://127.0.0.1:$($config.apiPort)/ws"; LORETIDE_EXECUTION_POLICY='disabled'; LORETIDE_DIAGNOSTICS_TEST='1'; MULTICA_DEV_VERIFICATION_CODE='888888'; NEXT_TELEMETRY_DISABLED='1'; GOTOOLCHAIN='auto' }
  Push-Location (Join-Path $root 'server')
  try {
    $old = @{}; foreach ($key in $envBlock.Keys) { $old[$key] = [Environment]::GetEnvironmentVariable($key,'Process'); [Environment]::SetEnvironmentVariable($key,$envBlock[$key],'Process') }
    try { & go run ./cmd/migrate up *>&1 | Tee-Object -FilePath (Join-Path $logs 'migrate.log'); if ($LASTEXITCODE) { throw 'Migration failed; inspect migrate.log.' }; & go build -o (Join-Path $state 'api.exe') ./cmd/server *>&1 | Tee-Object -FilePath (Join-Path $logs 'build-api.log'); if ($LASTEXITCODE) { throw 'API build failed; inspect build-api.log.' } }
    finally { foreach ($key in $envBlock.Keys) { [Environment]::SetEnvironmentVariable($key,$old[$key],'Process') } }
  } finally { Pop-Location }
  $saved = $envBlock; foreach ($key in $saved.Keys) { [Environment]::SetEnvironmentVariable($key,$saved[$key],'Process') }
  try {
    $api = Start-Process -FilePath (Join-Path $state 'api.exe') -WorkingDirectory (Join-Path $root 'server') -RedirectStandardOutput (Join-Path $logs 'api.log') -RedirectStandardError (Join-Path $logs 'api.err.log') -WindowStyle Hidden -PassThru
    $web = Start-Process -FilePath (Get-Command node.exe).Source -ArgumentList @((Join-Path $root 'apps\web\node_modules\next\dist\bin\next'),'dev','--webpack','--hostname','127.0.0.1','--port',"$($config.webPort)") -WorkingDirectory (Join-Path $root 'apps\web') -RedirectStandardOutput (Join-Path $logs 'web.log') -RedirectStandardError (Join-Path $logs 'web.err.log') -WindowStyle Hidden -PassThru
    Set-Content -Path $apiPid -Value $api.Id -NoNewline; Set-Content -Path $webPid -Value $web.Id -NoNewline
  } finally { foreach ($key in $saved.Keys) { [Environment]::SetEnvironmentVariable($key,$null,'Process') } }
  Write-Output "Started isolated instance. Browser: http://127.0.0.1:$($config.webPort)  API: http://127.0.0.1:$($config.apiPort)/health"
}

if ($Action -eq 'bootstrap') {
  @("node: $(& node --version)","go: $(& go version)","pnpm: $(& pnpm --version)") | Set-Content -Encoding utf8NoBOM (Join-Path $logs 'toolchain.log')
  if (-not (Test-Path (Join-Path $root 'node_modules\.bin\next.cmd'))) {
    Push-Location $root
    try { & pnpm install --frozen-lockfile *>&1 | Tee-Object -FilePath (Join-Path $logs 'pnpm-install.log'); if ($LASTEXITCODE) { throw 'pnpm install failed; inspect pnpm-install.log.' } }
    finally { Pop-Location }
  }
  if (-not (Test-Path $configPath)) {
    $bootstrapPassword = New-Secret; $config = [ordered]@{ webPort=$WebPort; apiPort=$ApiPort; postgresPort=$PostgresPort; database=$Database; role=$Role; databasePassword=(New-Secret); jwtSecret=(New-Secret); bootstrapPassword=$bootstrapPassword; executionPolicy='disabled' }
    if (Test-Path $pgData) { throw "Partial PostgreSQL data directory exists at $pgData without instance.json. Preserve it for diagnosis; retry with a new OutputPath." }
    if (-not (Test-Path $pgData)) {
      $passwordFile = Join-Path $state 'initdb-password.tmp'; Set-Content -Path $passwordFile -Value $bootstrapPassword -NoNewline
      try { & (Join-Path $pgBin 'initdb.exe') -D $pgData -U loretide_bootstrap_admin --auth-local=trust --auth-host=scram-sha-256 --pwfile=$passwordFile *>&1 | Tee-Object -FilePath (Join-Path $logs 'initdb.log'); if ($LASTEXITCODE) { throw 'initdb failed; inspect initdb.log.' } }
      finally { Remove-Item $passwordFile -Force -ErrorAction SilentlyContinue }
    }
    Write-Config ([pscustomobject]$config)
  }
  $config = Read-Config
  & (Join-Path $pgBin 'pg_ctl.exe') -D $pgData status *> $null
  if ($LASTEXITCODE -ne 0) { & (Join-Path $pgBin 'pg_ctl.exe') -D $pgData -l (Join-Path $logs 'postgres.log') -o "-p $($config.postgresPort) -h 127.0.0.1" -w start; if ($LASTEXITCODE) { throw 'PostgreSQL failed to start; inspect postgres.log.' } }
  $admin = @('-h','127.0.0.1','-p',"$($config.postgresPort)",'-U','loretide_bootstrap_admin','-d','postgres','-v','ON_ERROR_STOP=1')
  Invoke-Pg ($admin + @('-c',"DO `$`$ BEGIN IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname='$($config.role)') THEN CREATE ROLE $($config.role) LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT PASSWORD '$($config.databasePassword)'; END IF; END `$`$;")) $config.bootstrapPassword
  Invoke-Pg ($admin + @('-tc',"SELECT 1 FROM pg_database WHERE datname='$($config.database)'")) $config.bootstrapPassword | Out-Null
  if ($LASTEXITCODE -ne 0) { throw 'Database probe failed.' }
  $exists = Invoke-Pg ($admin + @('-Atc',"SELECT 1 FROM pg_database WHERE datname='$($config.database)'")) $config.bootstrapPassword
  if ($exists -ne '1') { Invoke-Pg ($admin + @('-c',"CREATE DATABASE $($config.database) OWNER $($config.role);")) $config.bootstrapPassword }
  Start-Instance $config
  exit
}
if ($Action -eq 'start') { Start-Instance (Read-Config); exit }
if ($Action -eq 'stop') { foreach ($file in @('api.pid','web.pid')) { $path = Join-Path $state $file; if (Test-Path $path) { $id=[int](Get-Content $path); Stop-Process -Id $id -ErrorAction SilentlyContinue; Remove-Item $path -Force } }; Write-Output 'Stopped this instance API/Web processes; PostgreSQL and data were retained.'; exit }
$config = Read-Config; Assert-Policy $config; foreach ($name in @('api','web')) { $path=Join-Path $state "$name.pid"; $id=if(Test-Path $path){Get-Content $path}else{'missing'}; Write-Output "$name pid: $id" }; & (Join-Path $pgBin 'pg_ctl.exe') -D $pgData status
