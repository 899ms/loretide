#!/usr/bin/env pwsh
Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$repoRoot = Split-Path $PSScriptRoot -Parent
$scriptPath = Join-Path $PSScriptRoot 'bootstrap-local-windows.ps1'
$parseErrors = $null
[Management.Automation.Language.Parser]::ParseFile($scriptPath,[ref]$null,[ref]$parseErrors) | Out-Null
if ($parseErrors) { $parseErrors | ForEach-Object { Write-Error "$($_.Message) at line $($_.Extent.StartLineNumber)" }; exit 1 }
. $scriptPath

$failures = [Collections.Generic.List[string]]::new()
function It([string]$Name,[scriptblock]$Test) { try { & $Test; Write-Output "PASS $Name" } catch { $failures.Add("$Name`: $($_.Exception.Message)"); Write-Output "FAIL $Name" } }
function Assert-True($Value,[string]$Message) { if (-not $Value) { throw $Message } }
function Assert-Equal($Expected,$Actual,[string]$Message) { if ([string]$Expected -ne [string]$Actual) { throw "$Message expected='$Expected' actual='$Actual'" } }
function Assert-Throws([scriptblock]$Operation,[string]$Pattern) { try { & $Operation; throw 'operation did not throw' } catch { if ($_.Exception.Message -notlike "*$Pattern*") { throw "wrong error: $($_.Exception.Message)" } } }
function New-TestRoot([string]$Name) { $path=Join-Path ([IO.Path]::GetTempPath()) "loretide bootstrap tests\$Name-$([guid]::NewGuid().ToString('N'))";New-Item -ItemType Directory -Force $path|Out-Null;$path }
function New-TestConfig([string]$OutputPath) { $state=Join-Path $OutputPath state;New-Item -ItemType Directory -Force $state|Out-Null;$config=[ordered]@{schemaVersion=2;instanceId='11111111-1111-1111-1111-111111111111';repoRoot=$repoRoot;outputPath=$OutputPath;runtimePath='C:\runtime path';postgresDataPath=(Join-Path $OutputPath postgres);webPort=13101;apiPort=18101;postgresPort=15401;database='test_database';role='test_role';databasePassword='synthetic-db-secret';jwtSecret='synthetic-jwt-secret';bootstrapPassword='synthetic-admin-secret';verificationCode='654321';executionPolicy='disabled';postgresInitialized=$true;databaseInitialized=$true};$config|ConvertTo-Json|Set-Content -Path (Join-Path $state instance.json) -Encoding utf8NoBOM;[pscustomobject]$config }
function New-Identity([int]$ProcessId,[string]$Executable,[string]$Command,[string]$Started='2026-09-13T12:00:00.0000000Z',[int]$Parent=1) { [pscustomobject]@{pid=$ProcessId;parentPid=$Parent;startedAtUtc=$Started;executablePath=$Executable;commandLine=$Command} }
function Write-ProcessRecordForTest($Output,$Config,[string]$Name,$Identity,[string]$Cwd) { [ordered]@{schemaVersion=1;instanceId=$Config.instanceId;name=$Name;pid=$Identity.pid;parentPid=$Identity.parentPid;startedAtUtc=$Identity.startedAtUtc;executablePath=$Identity.executablePath;commandLine=$Identity.commandLine;workingDirectory=$Cwd}|ConvertTo-Json|Set-Content -Path (Join-Path $Output "state\$Name-process.json") }
function New-CommonHooks([hashtable]$Processes,[Collections.Generic.List[object]]$Events,[hashtable]$Health) {
  @{
    ToolVersions={ [pscustomobject]@{node='24.14.0';pnpm='10.28.2';go='1.26.6';postgres='17.11';next='16.3.4'} }
    EnsurePostgres={ param($Config);$Events.Add("postgres:$($Config.postgresPort)") }
    GetPostgresStatus={ 'owned and running' }
    GetApplicationDatabaseIdentity={ [pscustomobject]@{role='test_role';database='test_database';superuser=$false;createDatabase=$false;createRole=$false;replication=$false;bypassRls=$false;owner='test_role'} }
    GetProcess={ param($ProcessId);if($Processes.ContainsKey([int]$ProcessId)){$Processes[[int]$ProcessId]}else{$null} }
    GetChildren={ @() }
    PortInUse={ $false }
    TestHealth={ param($Url);[bool]$Health[$Url] }
    StopProcess={ param($ProcessId);$Events.Add("stop:$ProcessId");$Processes.Remove([int]$ProcessId) }
    RunCommand={ param($File,$Arguments,$Cwd,$Log);$Events.Add("run:$File $($Arguments -join ' ')");if($Arguments -contains 'build'){New-Item -ItemType File -Force (Join-Path $script:State api.exe)|Out-Null};0 }
  }
}

It 'refuses PID reuse and does not stop the foreign process' {
  $output=New-TestRoot 'pid-reuse';$config=New-TestConfig $output;$events=[Collections.Generic.List[object]]::new();$processes=@{};$health=@{}
  $record=[ordered]@{schemaVersion=1;instanceId=$config.instanceId;name='api';pid=4100;parentPid=1;startedAtUtc='2026-09-13T12:00:00.0000000Z';executablePath=(Join-Path $output 'state\api.exe');commandLine='owned-api';workingDirectory=(Join-Path $repoRoot server)}
  $record|ConvertTo-Json|Set-Content -Path (Join-Path $output 'state\api-process.json');$processes[4100]=New-Identity 4100 $record.executablePath 'foreign-command' '2026-09-13T12:01:00.0000000Z';$hooks=New-CommonHooks $processes $events $health
  Assert-Throws { Invoke-BootstrapMain stop 'C:\runtime path' $output 13101 18101 15401 test_database test_role $hooks @{OutputPath=$output} } 'PID identity does not match';Assert-Equal 0 $events.Count 'foreign PID must not be stopped'
}

It 'validates every tracked root before stopping either process' {
  $output=New-TestRoot 'all-roots-first';$config=New-TestConfig $output;$events=[Collections.Generic.List[object]]::new();$processes=@{};$health=@{}
  $api=New-Identity 4110 (Join-Path $output 'state\api.exe') foreign-api '2026-09-13T12:01:00.0000000Z';$web=New-Identity 4111 'C:\Program Files\nodejs\node.exe' owned-web;$processes[4110]=$api;$processes[4111]=$web
  Write-ProcessRecordForTest $output $config api (New-Identity 4110 (Join-Path $output 'state\api.exe') owned-api) (Join-Path $repoRoot server);Write-ProcessRecordForTest $output $config web $web (Join-Path $repoRoot 'apps\web');$hooks=New-CommonHooks $processes $events $health
  Assert-Throws { Invoke-BootstrapMain stop 'C:\runtime path' $output 13101 18101 15401 test_database test_role $hooks @{OutputPath=$output} } 'PID identity does not match';Assert-True (-not($events -match '^stop:')) 'no verified process may stop before every tracked root validates'
}

It 'revalidates a stop plan when its PID identity changes after preflight' {
  $output=New-TestRoot 'stop-plan-revalidation';$config=New-TestConfig $output;$events=[Collections.Generic.List[object]]::new();$processes=@{};$health=@{};$web=New-Identity 4121 'C:\Program Files\nodejs\node.exe' owned-web;$api=New-Identity 4122 (Join-Path $output 'state\api.exe') owned-api;$processes[4121]=$web;$processes[4122]=$api
  Write-ProcessRecordForTest $output $config web $web (Join-Path $repoRoot 'apps\web');Write-ProcessRecordForTest $output $config api $api (Join-Path $repoRoot server);$hooks=New-CommonHooks $processes $events $health;$hooks['GetProcess']={param($ProcessId);if($ProcessId -eq 4122){$processes[4121]=New-Identity 4121 'C:\Windows\System32\cmd.exe' foreign-web '2026-09-13T12:02:00.0000000Z'};if($processes.ContainsKey([int]$ProcessId)){$processes[[int]$ProcessId]}else{$null}}.GetNewClosure()
  Assert-Throws { Invoke-BootstrapMain stop 'C:\runtime path' $output 13101 18101 15401 test_database test_role $hooks @{OutputPath=$output} } 'PID identity does not match';Assert-True (-not($events -match '^stop:')) 'a replaced PID must be rejected before StopProcess'
}

It 'keeps status read-only and does not start PostgreSQL' {
  $output=New-TestRoot 'status-read-only';New-TestConfig $output|Out-Null;$events=[Collections.Generic.List[object]]::new();$hooks=New-CommonHooks @{} $events @{}
  $result=Invoke-BootstrapMain status 'C:\runtime path' $output 13101 18101 15401 test_database test_role $hooks @{OutputPath=$output};Assert-True (($result -join ' ') -like '*postgres: owned and running*') 'status must report the PostgreSQL probe';Assert-Equal 0 $events.Count 'status must not invoke the PostgreSQL start boundary'
}

It 'restores every caller environment value after a failing operation' {
  $beforeRemote=[Environment]::GetEnvironmentVariable('REMOTE_API_URL','Process');$beforeNode=[Environment]::GetEnvironmentVariable('NODE_OPTIONS','Process');[Environment]::SetEnvironmentVariable('REMOTE_API_URL','caller-remote','Process');[Environment]::SetEnvironmentVariable('NODE_OPTIONS','caller-node','Process')
  try{Assert-Throws { Invoke-WithEnvironment @{REMOTE_API_URL='instance-remote';NODE_OPTIONS='instance-node'} {throw 'synthetic failure'} } 'synthetic failure';Assert-Equal caller-remote $env:REMOTE_API_URL 'REMOTE_API_URL must be restored';Assert-Equal caller-node $env:NODE_OPTIONS 'NODE_OPTIONS must be restored'}finally{[Environment]::SetEnvironmentVariable('REMOTE_API_URL',$beforeRemote,'Process');[Environment]::SetEnvironmentVariable('NODE_OPTIONS',$beforeNode,'Process')}
}

It 'resolves tool versions from the repository when start is invoked from an external directory' {
  $output=New-TestRoot external-cwd-success;$config=New-TestConfig $output;$events=[Collections.Generic.List[object]]::new();$processes=@{};$health=@{'http://127.0.0.1:18101/health'=$true;'http://127.0.0.1:13101'=$true}
  $api=New-Identity 4191 (Join-Path $output 'state\api.exe') owned-api;$web=New-Identity 4192 'C:\Program Files\nodejs\node.exe' owned-web;$processes[4191]=$api;$processes[4192]=$web;Write-ProcessRecordForTest $output $config api $api (Join-Path $repoRoot server);Write-ProcessRecordForTest $output $config web $web (Join-Path $repoRoot 'apps\web');$hooks=New-CommonHooks $processes $events $health;[void]$hooks.Remove('ToolVersions');$hooks['EnsureToolchainDependencies']={}
  $probeCwds=@{};$hooks['ToolVersionCommand']={param($Name);$probeCwds[$Name]=(Get-Location).Path;switch($Name){node{return 'v24.14.0'};pnpm{return '10.28.2'};go{return 'go version go1.26.6 windows/amd64'};postgres{return 'postgres (PostgreSQL) 17.11'};default{throw "unexpected tool probe: $Name"}}}.GetNewClosure()
  $external=New-TestRoot external-caller;$original=(Get-Location).Path;Set-Location $external
  try{Invoke-BootstrapMain start 'C:\runtime path' $output 13101 18101 15401 test_database test_role $hooks @{OutputPath=$output}|Out-Null;Assert-True (Test-SamePath $external (Get-Location).Path) 'successful start must restore the external caller cwd';Assert-True (Test-SamePath $repoRoot $probeCwds.node) 'Node probe must run from repo root';Assert-True (Test-SamePath $repoRoot $probeCwds.pnpm) 'pnpm probe must run from repo root';Assert-True (Test-SamePath (Join-Path $repoRoot server) $probeCwds.go) 'Go probe must run from server cwd';Assert-True (Test-SamePath $repoRoot $probeCwds.postgres) 'PostgreSQL probe must run from repo root'}finally{Set-Location $original}
}

It 'restores the external caller directory when a repository-scoped tool probe fails' {
  $output=New-TestRoot external-cwd-failure;New-TestConfig $output|Out-Null;$events=[Collections.Generic.List[object]]::new();$hooks=New-CommonHooks @{} $events @{};[void]$hooks.Remove('ToolVersions');$hooks['EnsureToolchainDependencies']={}
  $probeCwds=@{};$hooks['ToolVersionCommand']={param($Name);$probeCwds[$Name]=(Get-Location).Path;if($Name -eq 'pnpm'){throw 'synthetic pnpm probe failure'};if($Name -eq 'node'){return 'v24.14.0'};throw "unexpected tool probe after failure: $Name"}.GetNewClosure()
  $external=New-TestRoot external-failure-caller;$original=(Get-Location).Path;Set-Location $external
  try{Assert-Throws { Invoke-BootstrapMain start 'C:\runtime path' $output 13101 18101 15401 test_database test_role $hooks @{OutputPath=$output} } 'synthetic pnpm probe failure';Assert-True (Test-SamePath $external (Get-Location).Path) 'failed start must restore the external caller cwd';Assert-True (Test-SamePath $repoRoot $probeCwds.node) 'failing probe sequence must start from repo root';Assert-True (Test-SamePath $repoRoot $probeCwds.pnpm) 'failing pnpm probe must run from repo root';Assert-Equal 0 $events.Count 'tool probe failure must precede service boundaries'}finally{Set-Location $original}
}

It 'treats a healthy owned repeated start as idempotent' {
  $output=New-TestRoot idempotent;$config=New-TestConfig $output;$events=[Collections.Generic.List[object]]::new();$processes=@{};$health=@{'http://127.0.0.1:18101/health'=$true;'http://127.0.0.1:13101'=$true}
  $api=New-Identity 4201 (Join-Path $output 'state\api.exe') owned-api;$web=New-Identity 4202 'C:\Program Files\nodejs\node.exe' owned-web;$processes[4201]=$api;$processes[4202]=$web;Write-ProcessRecordForTest $output $config api $api (Join-Path $repoRoot server);Write-ProcessRecordForTest $output $config web $web (Join-Path $repoRoot 'apps\web');$hooks=New-CommonHooks $processes $events $health
  $result=Invoke-BootstrapMain start 'C:\runtime path' $output 13101 18101 15401 test_database test_role $hooks @{OutputPath=$output};Assert-True (($result -join ' ') -like '*already running and healthy*') 'repeat start must report healthy reuse';Assert-True (-not ($events -match '^run:|^stop:')) 'repeat start must not build/start/stop'
}

It 'retains API identity and reports partial failure when Web launch fails' {
  $output=New-TestRoot partial;New-TestConfig $output|Out-Null;$events=[Collections.Generic.List[object]]::new();$processes=@{};$health=@{'http://127.0.0.1:18101/health'=$true};$hooks=New-CommonHooks $processes $events $health
  $script:startCount=0;$hooks['StartProcess']={param($Spec);$script:startCount++;if($script:startCount -eq 2){throw 'synthetic web failure'};$identity=New-Identity 4301 $Spec.filePath owned-api;$processes[4301]=$identity;[pscustomobject]@{Id=4301}}
  Assert-Throws { Invoke-BootstrapMain start 'C:\runtime path' $output 13101 18101 15401 test_database test_role $hooks @{OutputPath=$output} } 'Partial start*synthetic web failure';Assert-True (Test-Path (Join-Path $output 'state\api-process.json')) 'API identity must remain recorded';Assert-True (-not(Test-Path (Join-Path $output 'state\web-process.json'))) 'failed Web must not get a process record'
}

It 'quotes the workspace Next path and supplies the Web rewrite upstream' {
  $output=New-TestRoot space-path;New-TestConfig $output|Out-Null;$events=[Collections.Generic.List[object]]::new();$processes=@{};$health=@{'http://127.0.0.1:18101/health'=$true;'http://127.0.0.1:13101'=$true};$hooks=New-CommonHooks $processes $events $health;$script:captured=$null
  $script:startCount=0;$hooks['StartProcess']={param($Spec);$script:startCount++;if($script:startCount -eq 1){$identity=New-Identity 4401 $Spec.filePath owned-api;$processes[4401]=$identity;return [pscustomobject]@{Id=4401}};$script:captured=[pscustomobject]@{args=$Spec.arguments;cwd=$Spec.workingDirectory;remote=$env:REMOTE_API_URL;public=$env:NEXT_PUBLIC_API_URL};$identity=New-Identity 4402 $Spec.filePath owned-web;$processes[4402]=$identity;[pscustomobject]@{Id=4402}}
  Invoke-BootstrapMain start 'C:\runtime path' $output 13101 18101 15401 test_database test_role $hooks @{OutputPath=$output}|Out-Null
  $spacedNext='C:\repo with spaces\apps\web\node_modules\next\dist\bin\next';Assert-Equal ('"'+$spacedNext+'"') (Quote-WindowsArgument $spacedNext) 'Next path with spaces must be one quoted argument';Assert-True ($script:captured.args[0] -like '*apps\web\node_modules\next\dist\bin\next*') 'workspace Next entry';Assert-True ($script:captured.args -contains '--webpack') 'Web must use webpack';Assert-Equal (Join-Path $repoRoot 'apps\web') $script:captured.cwd 'Web cwd';Assert-Equal 'http://127.0.0.1:18101' $script:captured.remote 'REMOTE_API_URL';Assert-Equal '' $script:captured.public 'NEXT_PUBLIC_API_URL must not replace upstream'
}

It 'rejects a stale configuration that changes its role or path binding' {
  $output=New-TestRoot stale-config;$config=New-TestConfig $output;$config.role='invalid-role';$config|ConvertTo-Json|Set-Content -Path (Join-Path $output 'state\instance.json');$events=[Collections.Generic.List[object]]::new();$hooks=New-CommonHooks @{} $events @{}
  Assert-Throws { Invoke-BootstrapMain status 'C:\runtime path' $output 13101 18101 15401 test_database test_role $hooks @{OutputPath=$output} } 'Invalid role';Assert-Equal 0 $events.Count 'invalid config must fail before external boundaries'
}

It 'rejects a PostgreSQL listener whose data directory or port is foreign' {
  $output=New-TestRoot pg-identity;$script:PgData=Join-Path $output postgres;$script:Config=[pscustomobject]@{postgresPort=15401}
  Assert-PostgresIdentity "$($script:PgData)|15401";Assert-Throws { Assert-PostgresIdentity 'C:\foreign-postgres|15401' } 'listener data directory/port does not match';Assert-Throws { Assert-PostgresIdentity "$($script:PgData)|5432" } 'listener data directory/port does not match'
}

It 'rejects an unattributed Web child before stopping any process' {
  $output=New-TestRoot web-child;$config=New-TestConfig $output;$events=[Collections.Generic.List[object]]::new();$processes=@{};$health=@{};$web=New-Identity 4501 'C:\Program Files\nodejs\node.exe' "node $repoRoot\apps\web\node_modules\next\dist\bin\next";$processes[4501]=$web;Write-ProcessRecordForTest $output $config web $web (Join-Path $repoRoot 'apps\web');$hooks=New-CommonHooks $processes $events $health
  $hooks['GetChildren']={param($ParentId);if($ParentId -eq 4501){,@(New-Identity 4502 'C:\Windows\System32\cmd.exe' 'foreign child' '2026-09-13T12:01:00.0000000Z' 4501)}else{@()}}
  Assert-Throws { Invoke-BootstrapMain stop 'C:\runtime path' $output 13101 18101 15401 test_database test_role $hooks @{OutputPath=$output} } 'child process identity is not attributable';Assert-True (-not($events -match '^stop:')) 'no process may be stopped when a child is foreign'
}

It 'refuses an untracked API listener before migrations or builds' {
  $output=New-TestRoot foreign-port;New-TestConfig $output|Out-Null;$events=[Collections.Generic.List[object]]::new();$hooks=New-CommonHooks @{} $events @{};$hooks['PortInUse']={param($Port);$Port -eq 18101}
  Assert-Throws { Invoke-BootstrapMain start 'C:\runtime path' $output 13101 18101 15401 test_database test_role $hooks @{OutputPath=$output} } 'already owned by an untracked listener';Assert-True (-not ($events -match '^run:|^stop:|^start:')) 'foreign API port must stop startup before commands or process launch'
}

It 'rejects an elevated application role before migration build or launch' {
  $output=New-TestRoot elevated-role;New-TestConfig $output|Out-Null;$events=[Collections.Generic.List[object]]::new();$hooks=New-CommonHooks @{} $events @{};$hooks['GetApplicationDatabaseIdentity']={ [pscustomobject]@{role='test_role';database='test_database';superuser=$true;createDatabase=$false;createRole=$false;replication=$false;bypassRls=$false;owner='test_role'} };$hooks['StartProcess']={param($Spec);$events.Add("start:$($Spec.name)");throw 'unexpected process launch'}
  Assert-Throws { Invoke-BootstrapMain start 'C:\runtime path' $output 13101 18101 15401 test_database test_role $hooks @{OutputPath=$output} } 'application database identity';Assert-True (-not ($events -match '^run:|^start:')) 'elevated application role must fail before migration build or launch'
}

It 'rejects a database owned by another role before migration build or launch' {
  $output=New-TestRoot wrong-owner;New-TestConfig $output|Out-Null;$events=[Collections.Generic.List[object]]::new();$hooks=New-CommonHooks @{} $events @{};$hooks['GetApplicationDatabaseIdentity']={ [pscustomobject]@{role='test_role';database='test_database';superuser=$false;createDatabase=$false;createRole=$false;replication=$false;bypassRls=$false;owner='foreign_owner'} };$hooks['StartProcess']={param($Spec);$events.Add("start:$($Spec.name)");throw 'unexpected process launch'}
  Assert-Throws { Invoke-BootstrapMain start 'C:\runtime path' $output 13101 18101 15401 test_database test_role $hooks @{OutputPath=$output} } 'application database identity';Assert-True (-not ($events -match '^run:|^start:')) 'wrong database owner must fail before migration build or launch'
}

It 'rejects every forbidden application-role capability before side effects' {
  foreach($privilege in @('createDatabase','createRole','replication','bypassRls')){
    $output=New-TestRoot "forbidden-$privilege";New-TestConfig $output|Out-Null;$events=[Collections.Generic.List[object]]::new();$hooks=New-CommonHooks @{} $events @{};$identity=[ordered]@{role='test_role';database='test_database';superuser=$false;createDatabase=$false;createRole=$false;replication=$false;bypassRls=$false;owner='test_role'};$identity[$privilege]=$true;$hooks['GetApplicationDatabaseIdentity']={ [pscustomobject]$identity }.GetNewClosure();$hooks['StartProcess']={param($Spec);$events.Add("start:$($Spec.name)");throw 'unexpected process launch'}.GetNewClosure()
    Assert-Throws { Invoke-BootstrapMain start 'C:\runtime path' $output 13101 18101 15401 test_database test_role $hooks @{OutputPath=$output} } 'application database identity';Assert-True (-not ($events -match '^run:|^start:')) "$privilege must fail before migration build or launch"
  }
}

It 'stops before migration build or launch when the identity query fails' {
  $output=New-TestRoot identity-query-failure;New-TestConfig $output|Out-Null;$events=[Collections.Generic.List[object]]::new();$hooks=New-CommonHooks @{} $events @{};[void]$hooks.Remove('GetApplicationDatabaseIdentity');$hooks['Psql']={throw 'synthetic identity query failure'};$hooks['StartProcess']={param($Spec);$events.Add("start:$($Spec.name)");throw 'unexpected process launch'}
  Assert-Throws { Invoke-BootstrapMain start 'C:\runtime path' $output 13101 18101 15401 test_database test_role $hooks @{OutputPath=$output} } 'synthetic identity query failure';Assert-True (-not ($events -match '^run:|^start:')) 'identity query failure must precede migration build or launch'
}

It 'enforces tool versions instead of only recording them' {
  $output=New-TestRoot versions;New-TestConfig $output|Out-Null;$events=[Collections.Generic.List[object]]::new();$hooks=New-CommonHooks @{} $events @{};$hooks['ToolVersions']={ [pscustomobject]@{node='20.0.0';pnpm='10.28.2';go='1.26.6';postgres='17.11';next='16.3.4'} }
  Assert-Throws { Invoke-BootstrapMain start 'C:\runtime path' $output 13101 18101 15401 test_database test_role $hooks @{OutputPath=$output} } 'Node 22.0 or newer is required'
}

$source=Get-Content -Raw $scriptPath
foreach($forbidden in @('MULTICA_DEV_VERIFICATION_CODE=''888888''',"Join-Path `$root 'data/windows'",'test-db.json','docker compose','Stop-Computer','Restart-Computer','New-NetFirewallRule','ALTER ROLE','ALTER DATABASE')){if($source -match [regex]::Escape($forbidden)){$failures.Add("forbidden source pattern: $forbidden")}}
if($failures.Count){$failures|ForEach-Object{Write-Error $_};exit 1}
Write-Output 'bootstrap-local-windows behavior tests passed'
