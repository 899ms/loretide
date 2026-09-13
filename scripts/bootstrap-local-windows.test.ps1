#!/usr/bin/env pwsh
Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$scriptPath = Join-Path (Split-Path $PSScriptRoot -Parent) 'scripts/bootstrap-local-windows.ps1'
$errors = $null
[Management.Automation.Language.Parser]::ParseFile($scriptPath,[ref]$null,[ref]$errors) | Out-Null
if ($errors) { throw "bootstrap-local-windows.ps1 parse error: $($errors[0].Message)" }
$source = Get-Content -Raw $scriptPath
foreach ($required in @('RuntimePath and OutputPath must be ASCII paths','LORETIDE_EXECUTION_POLICY=''disabled''','NOSUPERUSER','MULTICA_DEV_VERIFICATION_CODE=''888888''','Port $port is already listening','Remove-Item $passwordFile -Force','Partial PostgreSQL data directory exists','pnpm install --frozen-lockfile','apps\web\node_modules\next\dist\bin\next','''--webpack''','Set-Content -Path $apiPid','Stopped this instance API/Web processes; PostgreSQL and data were retained.')) { if ($source -notlike "*$required*") { throw "missing required bootstrap safeguard: $required" } }
foreach ($forbidden in @("Join-Path `$root 'data/windows'",'test-db.json','docker compose','Stop-Computer','Restart-Computer','New-NetFirewallRule')) { if ($source -match [regex]::Escape($forbidden)) { throw "forbidden dependency or host mutation in bootstrap script: $forbidden" } }
Write-Output 'bootstrap-local-windows contract checks passed'
