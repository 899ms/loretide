$ErrorActionPreference = 'Stop'
$diagRoot = 'C:\loretide-diag-acceptance-15402'
$pgBin = 'F:\loretide-runtime\pgsql\bin'
if (Test-Path -LiteralPath $diagRoot) { throw 'Target already exists; inspect before retry.' }
if (Get-NetTCPConnection -State Listen -LocalPort 15402 -ErrorAction SilentlyContinue) { throw '15402 occupied' }
New-Item -ItemType Directory -Path $diagRoot | Out-Null
$identity = [Security.Principal.WindowsIdentity]::GetCurrent().Name
& icacls $diagRoot /inheritance:r /grant:r "${identity}:(OI)(CI)F" 'SYSTEM:(OI)(CI)F' | Out-Null
if ($LASTEXITCODE) { throw 'ACL failed' }
$adminSecret = [Convert]::ToHexString([Security.Cryptography.RandomNumberGenerator]::GetBytes(32))
$appSecret = [Convert]::ToHexString([Security.Cryptography.RandomNumberGenerator]::GetBytes(32))
Set-Content -LiteralPath "$diagRoot\admin-password" -Value $adminSecret -NoNewline
& "$pgBin\initdb.exe" -D "$diagRoot\data" -U diag_cluster_admin --pwfile="$diagRoot\admin-password" --auth=scram-sha-256 --encoding=UTF8 --locale=C *> "$diagRoot\initdb.log"
if ($LASTEXITCODE) { throw 'initdb failed; inspect local log' }
Add-Content -LiteralPath "$diagRoot\data\postgresql.conf" -Value "`nlisten_addresses = '127.0.0.1'`nport = 15402`n"
& "$pgBin\pg_ctl.exe" -D "$diagRoot\data" -l "$diagRoot\postgres.log" -w start
if ($LASTEXITCODE) { throw 'pg startup failed' }
$priorPassword = $env:PGPASSWORD
try {
 $env:PGPASSWORD = $adminSecret
 $sql = "CREATE ROLE loretide_diag_acceptance_user LOGIN PASSWORD '$appSecret' NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS;`nCREATE DATABASE loretide_diag_acceptance_local OWNER loretide_diag_acceptance_user;"
 $sql | & "$pgBin\psql.exe" -X -v ON_ERROR_STOP=1 -h 127.0.0.1 -p 15402 -U diag_cluster_admin -d postgres
 if ($LASTEXITCODE) { throw 'Provision failed' }
 $url = "postgresql://loretide_diag_acceptance_user:${appSecret}@127.0.0.1:15402/loretide_diag_acceptance_local?sslmode=disable"
 Set-Content -LiteralPath "$diagRoot\test-database-url" -Value $url -NoNewline
 $env:PGPASSWORD = $appSecret
 & "$pgBin\psql.exe" -X -v ON_ERROR_STOP=1 -h 127.0.0.1 -p 15402 -U loretide_diag_acceptance_user -d loretide_diag_acceptance_local -Atc 'SELECT current_database(), current_user, rolsuper, rolcreatedb, rolcreaterole, rolreplication, rolbypassrls FROM pg_roles WHERE rolname=current_user;'
 if ($LASTEXITCODE) { throw 'Identity verification failed' }
} finally { $env:PGPASSWORD = $priorPassword }
