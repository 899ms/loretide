<#
.SYNOPSIS
Creates and operates an isolated native Windows Loretide development instance.

.DESCRIPTION
Generated state stays below OutputPath. Existing data/windows configuration,
model-client credentials, Docker, global firewall settings and system restart
are outside this script's scope. Real agent execution is always disabled.
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
  [ValidatePattern('^[a-z][a-z0-9_]{2,50}$')][string]$Role = 'loretide_bootstrap_app',
  [Parameter(DontShow)][hashtable]$TestHooks = @{}
)
Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

function New-BootstrapSecret([int]$Bytes = 32) {
  $buffer = New-Object byte[] $Bytes
  $rng = [Security.Cryptography.RandomNumberGenerator]::Create()
  try { $rng.GetBytes($buffer) } finally { $rng.Dispose() }
  -join ($buffer | ForEach-Object { $_.ToString('x2') })
}

function New-VerificationCode {
  $buffer = New-Object byte[] 4
  $rng = [Security.Cryptography.RandomNumberGenerator]::Create()
  try { $rng.GetBytes($buffer) } finally { $rng.Dispose() }
  (100000 + ([BitConverter]::ToUInt32($buffer,0) % 900000)).ToString()
}

function Invoke-WithEnvironment([hashtable]$Values,[scriptblock]$Operation) {
  $previous = @{}
  foreach ($name in $Values.Keys) {
    $previous[$name] = [Environment]::GetEnvironmentVariable($name,'Process')
    [Environment]::SetEnvironmentVariable($name,[string]$Values[$name],'Process')
  }
  try { & $Operation }
  finally { foreach ($name in $Values.Keys) { [Environment]::SetEnvironmentVariable($name,$previous[$name],'Process') } }
}

function Invoke-Hook([string]$Name,[object[]]$Arguments,[scriptblock]$Default) {
  if ($script:Hooks.ContainsKey($Name)) { return & $script:Hooks[$Name] @Arguments }
  & $Default @Arguments
}

function Test-SamePath([string]$Left,[string]$Right) {
  [string]::Equals([IO.Path]::GetFullPath($Left).TrimEnd('\'),[IO.Path]::GetFullPath($Right).TrimEnd('\'),[StringComparison]::OrdinalIgnoreCase)
}

function Convert-ProcessStartToUtc($Value) {
  if ($Value -is [DateTime]) { return $Value.ToUniversalTime().ToString('o') }
  ([Management.ManagementDateTimeConverter]::ToDateTime([string]$Value)).ToUniversalTime().ToString('o')
}

function Get-ActualProcessIdentity([int]$ProcessId) {
  Invoke-Hook 'GetProcess' @($ProcessId) {
    param($ProcessId)
    $cim = Get-CimInstance Win32_Process -Filter "ProcessId = $ProcessId" -ErrorAction SilentlyContinue
    if (-not $cim) { return $null }
    [pscustomobject]@{pid=[int]$cim.ProcessId;parentPid=[int]$cim.ParentProcessId;startedAtUtc=(Convert-ProcessStartToUtc $cim.CreationDate);executablePath=[IO.Path]::GetFullPath([string]$cim.ExecutablePath);commandLine=[string]$cim.CommandLine}
  }
}

function Get-ActualProcessChildren([int]$ProcessId) {
  Invoke-Hook 'GetChildren' @($ProcessId) {
    param($ParentId)
    @(Get-CimInstance Win32_Process -Filter "ParentProcessId = $ParentId" -ErrorAction SilentlyContinue | ForEach-Object {
      [pscustomobject]@{pid=[int]$_.ProcessId;parentPid=[int]$_.ParentProcessId;startedAtUtc=(Convert-ProcessStartToUtc $_.CreationDate);executablePath=[IO.Path]::GetFullPath([string]$_.ExecutablePath);commandLine=[string]$_.CommandLine}
    })
  }
}

function Assert-TrackedIdentity($Expected,$Actual,[string]$Name) {
  if (-not $Actual) { return $false }
  $mismatches = @()
  if ([int]$Expected.pid -ne [int]$Actual.pid) { $mismatches += 'pid' }
  $expectedStart = if ($Expected.startedAtUtc -is [DateTime]) { $Expected.startedAtUtc.ToUniversalTime() } else { [DateTimeOffset]::Parse([string]$Expected.startedAtUtc).UtcDateTime }
  $actualStart = if ($Actual.startedAtUtc -is [DateTime]) { $Actual.startedAtUtc.ToUniversalTime() } else { [DateTimeOffset]::Parse([string]$Actual.startedAtUtc).UtcDateTime }
  if ($expectedStart.Ticks -ne $actualStart.Ticks) { $mismatches += 'start-time' }
  if (-not (Test-SamePath $Expected.executablePath $Actual.executablePath)) { $mismatches += 'executable' }
  if ([string]$Expected.commandLine -ne [string]$Actual.commandLine) { $mismatches += 'command-line' }
  if ($mismatches.Count) { throw "Refusing $Name operation: PID identity does not match this instance ($($mismatches -join ', ')); expected start $($expectedStart.ToString('o')), actual $($actualStart.ToString('o'))." }
  $true
}

function Get-ProcessRecordPath([string]$Name) { Join-Path $script:State "$Name-process.json" }
function Read-ProcessRecord([string]$Name) { $path=Get-ProcessRecordPath $Name; if(Test-Path $path){Get-Content -Raw $path|ConvertFrom-Json}else{$null} }
function Remove-StaleProcessRecord([string]$Name) { $path=Get-ProcessRecordPath $Name; if(Test-Path $path){Remove-Item -LiteralPath $path -Force} }

function Write-ProcessRecord([string]$Name,$Identity,[string]$WorkingDirectory) {
  $record=[ordered]@{schemaVersion=1;instanceId=$script:Config.instanceId;name=$Name;pid=[int]$Identity.pid;parentPid=[int]$Identity.parentPid;startedAtUtc=[string]$Identity.startedAtUtc;executablePath=[string]$Identity.executablePath;commandLine=[string]$Identity.commandLine;workingDirectory=[IO.Path]::GetFullPath($WorkingDirectory)}
  $record|ConvertTo-Json|Set-Content -Path (Get-ProcessRecordPath $Name) -Encoding utf8NoBOM
  [pscustomobject]$record
}

function Save-InstanceConfig { $script:Config | ConvertTo-Json | Set-Content -Path $script:ConfigPath -Encoding utf8NoBOM }

function Get-VerifiedTrackedProcess([string]$Name) {
  $record=Read-ProcessRecord $Name
  if(-not $record){return $null}
  if($record.instanceId -ne $script:Config.instanceId -or $record.name -ne $Name){throw "Refusing $Name operation: process record belongs to another instance."}
  $actual=Get-ActualProcessIdentity ([int]$record.pid)
  if(-not $actual){return $null}
  [void](Assert-TrackedIdentity $record $actual $Name)
  $record
}

function Assert-WebChildIdentity($Root,$Child) {
  $nodeName=[IO.Path]::GetFileName([string]$Child.executablePath)
  $after=[DateTime]::Parse($Child.startedAtUtc).ToUniversalTime() -ge [DateTime]::Parse($Root.startedAtUtc).ToUniversalTime()
  $belongs=$Child.commandLine -like "*$($script:RepoRoot)\apps\web*" -or $Child.commandLine -like '*next-server*'
  if($nodeName -notin @('node.exe','node') -or -not $after -or -not $belongs){throw 'Refusing web stop: child process identity is not attributable to this instance.'}
}

function Get-VerifiedDescendants($Root,[string]$Name) {
  $result = [Collections.Generic.List[object]]::new()
  $pending = [Collections.Generic.Queue[object]]::new()
  foreach ($child in @(Get-ActualProcessChildren ([int]$Root.pid))) { $pending.Enqueue($child) }
  while ($pending.Count) {
    $child = $pending.Dequeue()
    if ($Name -eq 'api') { throw 'Refusing api stop: unexpected child processes are attached.' }
    Assert-WebChildIdentity $Root $child
    $result.Add($child)
    foreach ($grandchild in @(Get-ActualProcessChildren ([int]$child.pid))) { $pending.Enqueue($grandchild) }
  }
  @($result)
}

function Get-VerifiedStopPlan([string]$Name) {
  $rootRecord=Get-VerifiedTrackedProcess $Name
  if(-not $rootRecord){return [pscustomobject]@{name=$Name;root=$null;children=@()}}
  $children=@(Get-VerifiedDescendants $rootRecord $Name)
  [pscustomobject]@{name=$Name;root=$rootRecord;children=$children}
}

function Invoke-VerifiedStopPlan($Plan) {
  if(-not $Plan.root){Remove-StaleProcessRecord $Plan.name;return $false}
  foreach($child in ($Plan.children|Sort-Object startedAtUtc -Descending)){Invoke-Hook 'StopProcess' @([int]$child.pid) {param($ProcessId);Stop-Process -Id $ProcessId -ErrorAction Stop}}
  Invoke-Hook 'StopProcess' @([int]$Plan.root.pid) {param($ProcessId);Stop-Process -Id $ProcessId -ErrorAction Stop}
  Remove-StaleProcessRecord $Plan.name
  $true
}

function Quote-WindowsArgument([string]$Value) {
  if($Value -notmatch '[\s"]'){return $Value}
  '"'+($Value -replace '(\\*)"','$1$1\"' -replace '(\\+)$','$1$1')+'"'
}

function Start-BoundaryProcess([string]$Name,[string]$FilePath,[string[]]$Arguments,[string]$WorkingDirectory,[hashtable]$Environment) {
  $spec = [pscustomobject]@{name=$Name;filePath=$FilePath;arguments=$Arguments;workingDirectory=$WorkingDirectory}
  $process=Invoke-WithEnvironment $Environment {
    Invoke-Hook 'StartProcess' @($spec) {
      param($ProcessSpec)
      $options=@{FilePath=$ProcessSpec.filePath;WorkingDirectory=$ProcessSpec.workingDirectory;WindowStyle='Hidden';PassThru=$true;RedirectStandardOutput=(Join-Path $script:Logs "$($ProcessSpec.name).log");RedirectStandardError=(Join-Path $script:Logs "$($ProcessSpec.name).err.log")}
      if($ProcessSpec.arguments.Count){$options.ArgumentList=$ProcessSpec.arguments}
      Start-Process @options
    }
  }
  if(-not $process -or -not $process.Id){throw "$Name process did not return a PID."}
  $identity=Get-ActualProcessIdentity ([int]$process.Id)
  if(-not $identity){throw "$Name process exited before its identity could be recorded."}
  Write-ProcessRecord $Name $identity $WorkingDirectory
}

function Test-Endpoint([string]$Url) { [bool](Invoke-Hook 'TestHealth' @($Url) {param($Target);try{(Invoke-WebRequest $Target -TimeoutSec 5 -UseBasicParsing).StatusCode -eq 200}catch{$false}}) }
function Wait-Endpoint([string]$Url,[int]$Seconds=45) { if($script:Hooks.ContainsKey('TestHealth')){return Test-Endpoint $Url};$deadline=[DateTime]::UtcNow.AddSeconds($Seconds);do{if(Test-Endpoint $Url){return $true};Start-Sleep -Milliseconds 500}while([DateTime]::UtcNow -lt $deadline);$false }

function Invoke-CheckedCommand([string]$FilePath,[string[]]$Arguments,[string]$WorkingDirectory,[string]$LogPath,[hashtable]$Environment=@{}) {
  Invoke-WithEnvironment $Environment {
    if($script:Hooks.ContainsKey('RunCommand')){$result=& $script:Hooks.RunCommand $FilePath $Arguments $WorkingDirectory $LogPath;if($null -ne $result -and [int]$result -ne 0){throw "Command failed with exit code $result; inspect $LogPath."};return}
    Push-Location $WorkingDirectory
    try{& $FilePath @Arguments *>&1|Tee-Object -FilePath $LogPath;if($LASTEXITCODE){throw "Command failed with exit code $LASTEXITCODE; inspect $LogPath."}}
    finally{Pop-Location}
  }
}

function Get-ToolVersions {
  if($script:Hooks.ContainsKey('ToolVersions')){return & $script:Hooks.ToolVersions}
  $goText=Invoke-WithEnvironment @{GOTOOLCHAIN='auto'} {Push-Location (Join-Path $script:RepoRoot 'server');try{(& go version)-join''}finally{Pop-Location}}
  [pscustomobject]@{node=((& node --version)-join'').TrimStart('v');pnpm=((& pnpm --version)-join'').Trim();go=([regex]::Match($goText,'go([0-9]+\.[0-9]+(?:\.[0-9]+)?)')).Groups[1].Value;postgres=([regex]::Match(((& (Join-Path $script:PgBin 'postgres.exe') --version)-join''),'([0-9]+\.[0-9]+)')).Groups[1].Value;next=(Get-Content -Raw (Join-Path $script:RepoRoot 'apps\web\node_modules\next\package.json')|ConvertFrom-Json).version}
}

function Ensure-ToolchainDependencies {
  if ($script:Hooks.ContainsKey('ToolVersions')) { return }
  foreach ($command in @('node.exe','go.exe','pnpm.cmd')) {
    if (-not (Get-Command $command -ErrorAction SilentlyContinue)) { throw "Missing dependency: $command. Install it and retry." }
  }
  foreach ($command in @('initdb.exe','pg_ctl.exe','psql.exe','postgres.exe')) {
    $path = Join-Path $script:PgBin $command
    if (-not (Test-Path $path)) { throw "Missing PostgreSQL dependency: $path. Install a native PostgreSQL runtime at RuntimePath; Docker is not used." }
  }
  $nextPackage = Join-Path $script:RepoRoot 'apps\web\node_modules\next\package.json'
  if (-not (Test-Path $nextPackage)) {
    Invoke-CheckedCommand 'pnpm' @('install','--frozen-lockfile') $script:RepoRoot (Join-Path $script:Logs 'pnpm-install.log')
  }
  if (-not (Test-Path $nextPackage)) { throw 'Dependency installation completed without the apps/web Next.js package.' }
}

function Assert-VersionAtLeast([string]$Name,[string]$Actual,[string]$Minimum) {$a=$null;$m=$null;if(-not [Version]::TryParse($Actual,[ref]$a) -or -not [Version]::TryParse($Minimum,[ref]$m) -or $a -lt $m){throw "$Name $Minimum or newer is required; found '$Actual'."}}
function Assert-Toolchain {$v=Get-ToolVersions;Assert-VersionAtLeast Node $v.node '22.0';Assert-VersionAtLeast Go $v.go '1.26.6';Assert-VersionAtLeast PostgreSQL $v.postgres '17.0';Assert-VersionAtLeast Next.js $v.next '16.3.4';if($v.pnpm -ne '10.28.2'){throw "pnpm 10.28.2 is required; found '$($v.pnpm)'."};$v|ConvertTo-Json|Set-Content -Path (Join-Path $script:Logs 'toolchain.json') -Encoding utf8NoBOM}
function Assert-PortSet([int]$Web,[int]$Api,[int]$Postgres){foreach($port in @($Web,$Api,$Postgres)){if($port -lt 1024 -or $port -gt 65535){throw "Invalid instance port: $port."}};if((@($Web,$Api,$Postgres)|Select-Object -Unique).Count -ne 3){throw 'Web, API and PostgreSQL ports must be distinct.'}}
function Assert-Name([string]$Value,[string]$Label){if($Value -notmatch '^[a-z][a-z0-9_]{2,50}$'){throw "Invalid $Label in instance configuration."}}
function Test-PortListening([int]$Port) { Invoke-Hook PortInUse @($Port) { param($Value); [bool](Get-NetTCPConnection -State Listen -LocalPort $Value -ErrorAction SilentlyContinue) } }

function Read-ValidatedConfig([hashtable]$Bound) {
  if(-not(Test-Path $script:ConfigPath)){throw "No bootstrap instance at $script:OutputRoot. Run bootstrap first."}
  $config=Get-Content -Raw $script:ConfigPath|ConvertFrom-Json
  if($config.schemaVersion -ne 2 -or $config.executionPolicy -ne 'disabled'){throw 'Instance configuration schema or execution policy is invalid.'}
  $instanceId = [guid]::Empty
  if (-not [guid]::TryParse([string]$config.instanceId,[ref]$instanceId) -or $instanceId -eq [guid]::Empty) { throw 'Instance configuration has an invalid instanceId.' }
  foreach ($secretName in @('databasePassword','jwtSecret','bootstrapPassword')) { if ([string]::IsNullOrWhiteSpace([string]$config.$secretName)) { throw "Instance configuration is missing $secretName." } }
  if ([string]$config.verificationCode -notmatch '^\d{6}$') { throw 'Instance configuration has an invalid private verification code.' }
  $propertyNames=@($config.PSObject.Properties.Name)
  if ($propertyNames -notcontains 'postgresInitialized' -or $propertyNames -notcontains 'databaseInitialized' -or $config.postgresInitialized -isnot [bool] -or $config.databaseInitialized -isnot [bool]) { throw 'Instance configuration initialization state is invalid.' }
  Assert-PortSet ([int]$config.webPort) ([int]$config.apiPort) ([int]$config.postgresPort);Assert-Name $config.database database;Assert-Name $config.role role
  if(-not (Test-SamePath $config.outputPath $script:OutputRoot) -or -not (Test-SamePath $config.postgresDataPath $script:PgData) -or -not (Test-SamePath $config.repoRoot $script:RepoRoot)){throw 'Instance configuration path binding does not match this checkout/output directory.'}
  foreach($pair in @(@('RuntimePath','runtimePath'),@('WebPort','webPort'),@('ApiPort','apiPort'),@('PostgresPort','postgresPort'),@('Database','database'),@('Role','role'))){if($Bound.ContainsKey($pair[0])){$expected=$Bound[$pair[0]];$actual=$config.($pair[1]);$matches=if($pair[0] -eq 'RuntimePath'){Test-SamePath $expected $actual}else{[string]$expected -eq [string]$actual};if(-not $matches){throw "Explicit $($pair[0]) does not match the selected instance configuration."}}}
  $config
}

function Invoke-Psql([string[]]$Arguments,[string]$Password,[switch]$Capture) {
  if($script:Hooks.ContainsKey('Psql')){return & $script:Hooks.Psql $Arguments $Capture}
  Invoke-WithEnvironment @{PGPASSWORD=$Password} {if($Capture){$value=& (Join-Path $script:PgBin 'psql.exe') @Arguments;if($LASTEXITCODE){throw "psql failed with exit code $LASTEXITCODE."};return($value-join"`n").Trim()};& (Join-Path $script:PgBin 'psql.exe') @Arguments|Out-Null;if($LASTEXITCODE){throw "psql failed with exit code $LASTEXITCODE."}}
}

function Assert-PostgresIdentity([string]$Identity) {
  $parts=$Identity -split '\|',2
  if($parts.Count -ne 2 -or -not (Test-SamePath $parts[0] $script:PgData) -or $parts[1] -ne [string]$script:Config.postgresPort){throw 'Refusing PostgreSQL operation: listener data directory/port does not match this instance.'}
}

function Ensure-Postgres {
  if($script:Hooks.ContainsKey('EnsurePostgres')){& $script:Hooks.EnsurePostgres $script:Config;return}
  & (Join-Path $script:PgBin 'pg_ctl.exe') -D $script:PgData status *> $null
  if($LASTEXITCODE -ne 0){& (Join-Path $script:PgBin 'pg_ctl.exe') -D $script:PgData -l (Join-Path $script:Logs 'postgres.log') -o "-p $($script:Config.postgresPort) -h 127.0.0.1" -w start;if($LASTEXITCODE){throw 'PostgreSQL failed to start; inspect postgres.log.'}}
  $identity=Invoke-Psql @('-h','127.0.0.1','-p',[string]$script:Config.postgresPort,'-U','loretide_bootstrap_admin','-d','postgres','-Atc',"SELECT current_setting('data_directory') || '|' || current_setting('port')") $script:Config.bootstrapPassword -Capture
  Assert-PostgresIdentity $identity
}

function Get-PostgresStatus {
  if($script:Hooks.ContainsKey('GetPostgresStatus')){return (& $script:Hooks.GetPostgresStatus $script:Config)}
  & (Join-Path $script:PgBin 'pg_ctl.exe') -D $script:PgData status *> $null
  if($LASTEXITCODE -ne 0){return 'not running'}
  $identity=Invoke-Psql @('-h','127.0.0.1','-p',[string]$script:Config.postgresPort,'-U','loretide_bootstrap_admin','-d','postgres','-Atc',"SELECT current_setting('data_directory') || '|' || current_setting('port')") $script:Config.bootstrapPassword -Capture
  Assert-PostgresIdentity $identity
  'owned and running'
}

function New-InstanceConfig {[pscustomobject][ordered]@{schemaVersion=2;instanceId=[guid]::NewGuid().ToString();repoRoot=$script:RepoRoot;outputPath=$script:OutputRoot;runtimePath=$script:RuntimeRoot;postgresDataPath=$script:PgData;webPort=$script:RequestedWebPort;apiPort=$script:RequestedApiPort;postgresPort=$script:RequestedPostgresPort;database=$script:RequestedDatabase;role=$script:RequestedRole;databasePassword=(New-BootstrapSecret);jwtSecret=(New-BootstrapSecret);bootstrapPassword=(New-BootstrapSecret);verificationCode=(New-VerificationCode);executionPolicy='disabled';postgresInitialized=$false;databaseInitialized=$false}}

function Initialize-Database {
  if($script:Hooks.ContainsKey('InitializeDatabase')){& $script:Hooks.InitializeDatabase $script:Config;return}
  if (-not $script:Config.postgresInitialized) {
    if (Test-Path $script:PgData) { throw "Partial PostgreSQL data directory exists at $script:PgData. Preserve it for diagnosis and retry with a new OutputPath." }
    $passwordFile=Join-Path $script:State 'initdb-password.tmp';Set-Content -Path $passwordFile -Value $script:Config.bootstrapPassword -NoNewline
    try{Invoke-CheckedCommand (Join-Path $script:PgBin 'initdb.exe') @('-D',$script:PgData,'-U','loretide_bootstrap_admin','--auth-local=trust','--auth-host=scram-sha-256',"--pwfile=$passwordFile") $script:RepoRoot (Join-Path $script:Logs 'initdb.log')}
    finally{Remove-Item -LiteralPath $passwordFile -Force -ErrorAction SilentlyContinue}
    $script:Config.postgresInitialized=$true;Save-InstanceConfig
  }
  Ensure-Postgres
  $roleSql=Join-Path $script:State 'role-setup.sql.tmp';Set-Content -Path $roleSql -NoNewline -Value "DO `$`$ BEGIN IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname='$($script:Config.role)') THEN CREATE ROLE $($script:Config.role) LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT PASSWORD '$($script:Config.databasePassword)'; END IF; END `$`$;"
  try{Invoke-Psql @('-h','127.0.0.1','-p',[string]$script:Config.postgresPort,'-U','loretide_bootstrap_admin','-d','postgres','-v','ON_ERROR_STOP=1','-f',$roleSql) $script:Config.bootstrapPassword}
  finally{Remove-Item -LiteralPath $roleSql -Force -ErrorAction SilentlyContinue}
  $exists=Invoke-Psql @('-h','127.0.0.1','-p',[string]$script:Config.postgresPort,'-U','loretide_bootstrap_admin','-d','postgres','-Atc',"SELECT 1 FROM pg_database WHERE datname='$($script:Config.database)'") $script:Config.bootstrapPassword -Capture
  if($exists -ne '1'){Invoke-Psql @('-h','127.0.0.1','-p',[string]$script:Config.postgresPort,'-U','loretide_bootstrap_admin','-d','postgres','-v','ON_ERROR_STOP=1','-c',"CREATE DATABASE $($script:Config.database) OWNER $($script:Config.role)") $script:Config.bootstrapPassword}
  $script:Config.databaseInitialized=$true;Save-InstanceConfig
}

function Get-InstanceEnvironment {@{APP_ENV='development';PORT=[string]$script:Config.apiPort;FRONTEND_PORT=[string]$script:Config.webPort;DATABASE_URL="postgres://$($script:Config.role):$($script:Config.databasePassword)@127.0.0.1:$($script:Config.postgresPort)/$($script:Config.database)?sslmode=disable";JWT_SECRET=$script:Config.jwtSecret;FRONTEND_ORIGIN="http://127.0.0.1:$($script:Config.webPort)";CORS_ALLOWED_ORIGINS="http://127.0.0.1:$($script:Config.webPort)";MULTICA_APP_URL="http://127.0.0.1:$($script:Config.webPort)";REMOTE_API_URL="http://127.0.0.1:$($script:Config.apiPort)";NEXT_PUBLIC_API_URL='';NEXT_PUBLIC_WS_URL='';LORETIDE_EXECUTION_POLICY='disabled';LORETIDE_DIAGNOSTICS_TEST='1';MULTICA_DEV_VERIFICATION_CODE=$script:Config.verificationCode;NEXT_TELEMETRY_DISABLED='1';GOTOOLCHAIN='auto'}}

function Start-Instance {
  Ensure-Postgres;$environment=Get-InstanceEnvironment;$api=Get-VerifiedTrackedProcess api;$web=Get-VerifiedTrackedProcess web
  if(-not $api){Remove-StaleProcessRecord api};if(-not $web){Remove-StaleProcessRecord web}
  $apiUrl="http://127.0.0.1:$($script:Config.apiPort)/health";$webUrl="http://127.0.0.1:$($script:Config.webPort)"
  if($api -and -not (Test-Endpoint $apiUrl)){throw 'Owned API process is running but unhealthy; refusing duplicate start or in-use executable rebuild.'}
  if($web -and -not (Test-Endpoint $webUrl)){throw 'Owned Web process is running but unhealthy; refusing duplicate start.'}
  if($api -and $web){Write-Output "Instance already running and healthy. Browser: $webUrl  API: $apiUrl";return}
  if(-not $api -and (Test-PortListening ([int]$script:Config.apiPort))){throw "Refusing API start: port $($script:Config.apiPort) is already owned by an untracked listener."}
  if(-not $web -and (Test-PortListening ([int]$script:Config.webPort))){throw "Refusing Web start: port $($script:Config.webPort) is already owned by an untracked listener."}
  if(-not $api){Invoke-CheckedCommand go @('run','./cmd/migrate','up') (Join-Path $script:RepoRoot server) (Join-Path $script:Logs 'migrate.log') $environment;Invoke-CheckedCommand go @('build','-o',(Join-Path $script:State 'api.exe'),'./cmd/server') (Join-Path $script:RepoRoot server) (Join-Path $script:Logs 'build-api.log') $environment;Start-BoundaryProcess api (Join-Path $script:State 'api.exe') @() (Join-Path $script:RepoRoot server) $environment|Out-Null;if(-not(Wait-Endpoint $apiUrl)){throw 'Partial start: API identity was retained but health did not become ready; inspect API logs.'}}
  if(-not $web){$nextPath=Join-Path $script:RepoRoot 'apps\web\node_modules\next\dist\bin\next';$arguments=@((Quote-WindowsArgument $nextPath),'dev','--webpack','--hostname','127.0.0.1','--port',[string]$script:Config.webPort);try{Start-BoundaryProcess web (Get-Command node.exe).Source $arguments (Join-Path $script:RepoRoot 'apps\web') $environment|Out-Null}catch{throw "Partial start: API state was retained and Web failed to launch. $($_.Exception.Message)"};if(-not(Wait-Endpoint $webUrl)){throw 'Partial start: API/Web identities were retained but Web health did not become ready; inspect Web logs.'}}
  Write-Output "Started isolated instance. Browser: $webUrl  API: $apiUrl"
}

function Invoke-BootstrapMain([string]$SelectedAction,[string]$SelectedRuntime,[string]$SelectedOutput,[int]$SelectedWeb,[int]$SelectedApi,[int]$SelectedPostgres,[string]$SelectedDatabase,[string]$SelectedRole,[hashtable]$Hooks,[hashtable]$Bound) {
  $script:Hooks=$Hooks;$script:RepoRoot=(Resolve-Path (Join-Path $PSScriptRoot '..')).Path;$script:OutputRoot=[IO.Path]::GetFullPath($SelectedOutput)
  if($script:OutputRoot -notmatch '^[\x20-\x7E]+$'){throw 'OutputPath must be an ASCII path.'}
  $script:State=Join-Path $script:OutputRoot state;$script:Logs=Join-Path $script:OutputRoot logs;$script:PgData=Join-Path $script:OutputRoot postgres;$script:ConfigPath=Join-Path $script:State instance.json
  $script:RequestedWebPort=$SelectedWeb;$script:RequestedApiPort=$SelectedApi;$script:RequestedPostgresPort=$SelectedPostgres;$script:RequestedDatabase=$SelectedDatabase;$script:RequestedRole=$SelectedRole
  New-Item -ItemType Directory -Force -Path $script:State,$script:Logs|Out-Null
  if(Test-Path $script:ConfigPath){$script:Config=Read-ValidatedConfig $Bound;$script:RuntimeRoot=[IO.Path]::GetFullPath([string]$script:Config.runtimePath);$script:PgBin=Join-Path $script:RuntimeRoot 'pgsql\bin'}else{if($SelectedAction -ne 'bootstrap'){throw "No bootstrap instance at $script:OutputRoot. Run bootstrap first."};$script:RuntimeRoot=[IO.Path]::GetFullPath($SelectedRuntime);if($script:RuntimeRoot -notmatch '^[\x20-\x7E]+$'){throw 'RuntimePath must be an ASCII path.'};$script:PgBin=Join-Path $script:RuntimeRoot 'pgsql\bin';Assert-PortSet $SelectedWeb $SelectedApi $SelectedPostgres;Assert-Name $SelectedDatabase database;Assert-Name $SelectedRole role;$script:Config=New-InstanceConfig}
  if($SelectedAction -in @('bootstrap','start')){Ensure-ToolchainDependencies;Assert-Toolchain;if(-not (Test-Path $script:ConfigPath)){foreach($port in @($script:Config.webPort,$script:Config.apiPort,$script:Config.postgresPort)){if(Test-PortListening ([int]$port)){throw "Port $port is already listening; choose an isolated port."}};Save-InstanceConfig};if($SelectedAction -eq 'bootstrap' -and -not $script:Config.databaseInitialized){Initialize-Database};if(-not $script:Config.databaseInitialized){throw 'Bootstrap is incomplete; run -Action bootstrap before start.'};Start-Instance;return}
  if($SelectedAction -eq 'stop'){$webPlan=Get-VerifiedStopPlan web;$apiPlan=Get-VerifiedStopPlan api;$webStopped=Invoke-VerifiedStopPlan $webPlan;$apiStopped=Invoke-VerifiedStopPlan $apiPlan;Write-Output "Stopped verified instance processes (web=$webStopped api=$apiStopped); PostgreSQL and data were retained.";return}
  $api=Get-VerifiedTrackedProcess api;$web=Get-VerifiedTrackedProcess web;Write-Output "api: $(if($api){'owned pid '+$api.pid}else{'not running'})";Write-Output "web: $(if($web){'owned pid '+$web.pid}else{'not running'})";Write-Output "postgres: $(Get-PostgresStatus)"
}

if($MyInvocation.InvocationName -ne '.'){Invoke-BootstrapMain $Action $RuntimePath $OutputPath $WebPort $ApiPort $PostgresPort $Database $Role $TestHooks $PSBoundParameters}
