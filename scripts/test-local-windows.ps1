param([ValidateSet('diagnostics','handler')][string]$Suite='diagnostics')
$ErrorActionPreference='Stop'
$root=Split-Path $PSScriptRoot -Parent
$config=Get-Content (Join-Path $root 'data/windows/test-db.json') -Raw | ConvertFrom-Json
if($config.database -notmatch '^loretide_handler_test_[a-z0-9_]+$' -or $config.role -ne $config.database) { throw 'Refusing non-isolated test configuration' }
$names=@('PGPASSWORD','DATABASE_URL','LORETIDE_DIAG_TEST_DATABASE_URL','GOTOOLCHAIN')
$previous=@{}
foreach($name in $names) { $previous[$name]=[Environment]::GetEnvironmentVariable($name,'Process') }
Push-Location (Join-Path $root 'server')
try {
  $env:PGPASSWORD=$config.password
  $identity=& F:\loretide-runtime\pgsql\bin\psql.exe -h 127.0.0.1 -p 15332 -U $config.role -d $config.database -Atc "SELECT current_database() || '|' || current_user || '|' || rolsuper FROM pg_roles WHERE rolname=current_user"
  if($LASTEXITCODE -ne 0 -or $identity -ne "$($config.database)|$($config.role)|false") { throw 'Test database identity or privilege check failed' }
  $env:DATABASE_URL="postgres://$($config.role):$($config.password)@127.0.0.1:15332/$($config.database)?sslmode=disable"
  $env:LORETIDE_DIAG_TEST_DATABASE_URL=$env:DATABASE_URL
  $env:GOTOOLCHAIN='auto'
  # Serialize this launcher's runs; handler fixtures share a database.
  $lock=[IO.File]::Open((Join-Path $root 'data/windows/test-db.lock'),'OpenOrCreate','ReadWrite','None')
  try {
    $package=if($Suite -eq 'handler'){'./internal/handler'}else{'./internal/content/diagnostics'}
    $log=Join-Path $root "data/windows/$Suite-latest.jsonl"
    go test $package -count=1 -json > $log
    if($LASTEXITCODE -ne 0){throw "Tests failed; see $log"}
    $events=Get-Content $log | ForEach-Object { $_ | ConvertFrom-Json }
    if(-not ($events | Where-Object {$_.Action -eq 'pass' -and $_.Test})) {throw 'No passing test cases; do not accept a skipped suite'}
    Write-Output "$Suite passed; evidence: $log"
  } finally { $lock.Dispose() }
} finally {
  Pop-Location
  foreach($name in $names) { [Environment]::SetEnvironmentVariable($name,$previous[$name],'Process') }
}
