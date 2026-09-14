#!/usr/bin/env pwsh
# Behaviour tests for scripts/local-windows.ps1 - the PowerShell counterpart of
# scripts/dev-env.test.sh.
#
# Nothing real is started. The script under test isolates every Windows-only
# call (Get-CimInstance, Get-NetTCPConnection, pg_ctl.exe, Invoke-WebRequest)
# behind a wrapper function, so this file dot-sources the definition region and
# shadows those wrappers with stubs. That keeps the identity, precondition and
# wait logic runnable on a Linux agent, where Win32_Process does not exist; the
# real end-to-end run stays a manual Windows step (quickstart.md).
#
# The pattern - slice the definitions out at a marker, then stub - is the one
# scripts/install.ps1.test.ps1 already uses for the installer.

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$RepoRoot = Split-Path (Split-Path $PSCommandPath -Parent) -Parent
$ScriptPath = Join-Path $RepoRoot 'scripts/local-windows.ps1'

$script:Failures = 0
$script:Checks = 0

function Fail-Test {
  param([string]$Message)
  Write-Host "FAIL: $Message" -ForegroundColor Red
  $script:Failures++
}

function Assert-Equal {
  param($Expected, $Actual, [string]$What)
  $script:Checks++
  if ($Expected -ne $Actual) { Fail-Test "$What : expected '$Expected', got '$Actual'" }
}

function Assert-True {
  param([bool]$Condition, [string]$What)
  $script:Checks++
  if (-not $Condition) { Fail-Test $What }
}

function Assert-Contains {
  param([string]$Haystack, [string]$Needle, [string]$What)
  $script:Checks++
  if (-not $Haystack.Contains($Needle)) {
    Fail-Test "$What : expected output to contain '$Needle'`n--- observed ---`n$Haystack`n----------------"
  }
}

function Assert-NotContains {
  param([string]$Haystack, [string]$Needle, [string]$What)
  $script:Checks++
  if ($Haystack.Contains($Needle)) {
    Fail-Test "$What : output must NOT contain '$Needle'`n--- observed ---`n$Haystack`n----------------"
  }
}

# ---------------------------------------------------------------------------
# 0. The script must parse, and must keep the pieces the spec pins down
# ---------------------------------------------------------------------------
$parseErrors = $null
[System.Management.Automation.Language.Parser]::ParseFile($ScriptPath, [ref]$null, [ref]$parseErrors) | Out-Null
if ($parseErrors) {
  $parseErrors | ForEach-Object { Write-Host "  $($_.Message) (line $($_.Extent.StartLineNumber))" }
  Fail-Test 'local-windows.ps1 has parse errors'
  exit 1
}

$source = Get-Content -Raw -Path $ScriptPath
Assert-Contains $source '#Requires -Version 7.0' 'script declares its minimum PowerShell version'

# FR-009: the supervisor's execution policy is not this feature's to relax.
$supervisor = Get-Content -Raw -Path (Join-Path $RepoRoot 'scripts/local-windows-supervisor.mjs')
Assert-Contains $supervisor "LORETIDE_EXECUTION_POLICY:'disabled'" 'supervisor keeps execution policy disabled'

# ---------------------------------------------------------------------------
# Load the definition region only: no param block, no dispatch, no side effects.
# ---------------------------------------------------------------------------
$begin = $source.IndexOf('# BEGIN DEFINITIONS')
$end = $source.IndexOf('# END DEFINITIONS')
if ($begin -lt 0 -or $end -lt 0) { Fail-Test 'definition markers missing from local-windows.ps1'; exit 1 }
. ([ScriptBlock]::Create($source.Substring($begin, $end - $begin)))

# ---------------------------------------------------------------------------
# Fixture: a temporary checkout root and state directory
# ---------------------------------------------------------------------------
$Tmp = Join-Path ([System.IO.Path]::GetTempPath()) ("lw-test-" + [guid]::NewGuid().ToString('n'))
$FakeRoot = Join-Path $Tmp 'checkout'
$FakeState = Join-Path $FakeRoot 'data/windows'
$FakeRuntime = Join-Path $Tmp 'runtime'
New-Item -ItemType Directory -Force -Path $FakeState, (Join-Path $FakeRuntime 'pgsql/bin'), (Join-Path $FakeRuntime 'pg') | Out-Null

$env:LORETIDE_WIN_STATE = $FakeState
$env:LORETIDE_WIN_RUNTIME = $FakeRuntime
Initialize-InstanceContext -Root $FakeRoot

# The placeholder stands in for the database password and JWT. No assertion
# below may ever find it in output: FR-007 forbids printing secrets.
$SecretMarker = 'PLACEHOLDER_SECRET'
function New-Secrets {
  Set-Content -Path (Join-Path $FakeState 'secrets.json') -Value "{`"password`":`"$SecretMarker`",`"jwt`":`"$SecretMarker`"}"
}
function New-ApiExe { Set-Content -Path (Join-Path $FakeState 'api.exe') -Value 'stub' }

# --- Stubs -----------------------------------------------------------------
# Shadow the script's platform wrappers. $script:Procs maps pid -> process info;
# $script:Listeners is what Get-NetTCPConnection would report.
$script:Procs = @{}
$script:Listeners = @()
$script:PgExit = 0
$script:HealthCalls = [System.Collections.Generic.List[string]]::new()

function Get-ProcessInfo {
  param([Parameter(Mandatory)][int]$ProcessId)
  if ($script:Procs.ContainsKey($ProcessId)) { return $script:Procs[$ProcessId] }
  $null
}
function Get-PortListener {
  param([Parameter(Mandatory)][int[]]$Port)
  @($script:Listeners | Where-Object { $Port -contains $_.Port })
}
function Invoke-PgCtl {
  param([Parameter(Mandatory)][string[]]$PgArgument)
  [pscustomobject]@{ ExitCode = $script:PgExit; Output = "stub pg_ctl exit $($script:PgExit)" }
}
function Invoke-HealthProbe {
  param([Parameter(Mandatory)][string]$Url, [int]$TimeoutSec = 5)
  $script:HealthCalls.Add($Url)
  200
}
function Get-CheckoutCommit { param([string]$Root) 'abc1234' }
function Start-SupervisorProcess {
  param([string]$Root, [string]$ScriptDir)
  $script:SupervisorStarted = $true
}

function New-Proc {
  param([int]$ProcessId, [string]$ExecutablePath = '', [string]$CommandLine = '', $CreationDate = $null)
  [pscustomobject]@{
    ProcessId = $ProcessId; ExecutablePath = $ExecutablePath
    CommandLine = $CommandLine; CreationDate = $CreationDate
  }
}

function Reset-Fixture {
  $script:Procs = @{}
  $script:Listeners = @()
  $script:PgExit = 0
  $script:SupervisorStarted = $false
  $script:HealthCalls.Clear()
  Get-ChildItem $FakeState -File -ErrorAction SilentlyContinue | Remove-Item -Force
}

# A fully running, fully owned instance.
function Set-RunningInstance {
  $api = Join-Path $FakeState 'api.exe'
  $script:Procs[201] = New-Proc -ProcessId 201 -ExecutablePath 'node.exe' -CommandLine "node $FakeRoot/scripts/local-windows-supervisor.mjs"
  $script:Procs[202] = New-Proc -ProcessId 202 -ExecutablePath $api
  $script:Procs[203] = New-Proc -ProcessId 203 -ExecutablePath 'node.exe' -CommandLine "node $FakeRoot/apps/web/node_modules/next/dist/bin/next dev"
  Set-Content -Path (Join-Path $FakeState 'supervisor.pid') -Value '201'
  Set-Content -Path (Join-Path $FakeState 'api.pid') -Value '202'
  Set-Content -Path (Join-Path $FakeState 'web.pid') -Value '203'
  Set-Content -Path (Join-Path $FakeState 'postmaster.pid') -Value '100'
  Set-Content -Path (Join-Path $FakeRuntime 'pg/postmaster.pid') -Value '100'
  # `start` records the commit it launched; status reads it back.
  Set-Content -Path (Join-Path $FakeState 'build.txt') -Value 'abc1234' -NoNewline
}

# ===========================================================================
# US1 - status proves which process is which, and whose
# ===========================================================================

# T006: everything stopped -> exit 1, no HTTP probe, and fast.
Reset-Fixture
$sw = [System.Diagnostics.Stopwatch]::StartNew()
$script:PgExit = 3
$stopped = Get-InstanceStatus
$out = (Write-InstanceStatus -Status $stopped | Out-String)
$sw.Stop()
Assert-Equal 'stopped' $stopped.components.postgres.state 'postgres reported stopped'
Assert-Equal 'stopped' $stopped.components.api.state 'api reported stopped'
Assert-Equal 'stopped' $stopped.components.web.state 'web reported stopped'
Assert-Equal 'stopped' $stopped.components.supervisor.state 'supervisor reported stopped'
Assert-Equal $false $stopped.ok 'ok=false when nothing runs'
Assert-Equal 0 $script:HealthCalls.Count 'no HTTP request is made for a stopped component'
Assert-True ($sw.Elapsed.TotalSeconds -lt 3) "status returns in under 3s when stopped (took $($sw.Elapsed.TotalSeconds)s)"
Assert-Contains $out 'ok=false' 'human output ends with ok=false'

# Exit code contract: 1 when a component is down, 2 when pg_ctl is absent.
Reset-Fixture
$script:PgExit = 3
$code = Write-InstanceStatus -Status (Get-InstanceStatus) 6>$null | Select-Object -Last 1
Assert-Equal 1 $code 'exit code 1 when a component is not running'

# T006: -Json parses and carries all four components.
Reset-Fixture
New-Secrets
New-ApiExe
Set-RunningInstance
$json = (Write-InstanceStatus -Status (Get-InstanceStatus) -AsJson | Select-Object -First 1)
$parsed = $json | ConvertFrom-Json
foreach ($name in @('postgres', 'supervisor', 'api', 'web')) {
  Assert-True ($null -ne $parsed.components.$name) "JSON carries the $name component"
}
Assert-Equal 202 $parsed.components.api.pid 'JSON reports the api pid from api.pid'
Assert-Equal $true $parsed.components.api.owned 'api owned when its executable is this checkout''s api.exe'
Assert-Equal 200 $parsed.components.api.health.status 'api health status present when running'
Assert-Equal 'abc1234' $parsed.build.commit 'build commit comes from build.txt or git'
Assert-Equal $true $parsed.ok 'ok=true when all four run and are owned'
Assert-NotContains $json $SecretMarker 'JSON output never contains the secret'

# T006: a foreign listener on 13000 is a conflict, not a healthy web.
Reset-Fixture
New-Secrets
New-ApiExe
Set-RunningInstance
$script:Procs[9999] = New-Proc -ProcessId 9999 -ExecutablePath 'C:\other\node.exe' -CommandLine 'node somewhere-else'
$script:Listeners = @([pscustomobject]@{ Port = 13000; OwningProcess = 9999 })
$conflicted = Get-InstanceStatus
$out = (Write-InstanceStatus -Status $conflicted | Out-String)
Assert-Equal 1 $conflicted.conflicts.Count 'a foreign listener is reported as a conflict'
Assert-Equal 9999 $conflicted.conflicts[0].pid 'the conflict names the holding pid'
Assert-Equal $false $conflicted.ok 'ok=false while a conflict exists'
Assert-Contains $out 'not this instance' 'human output flags the foreign listener'
Assert-NotContains $out $SecretMarker 'status output never contains the secret'

# Our own listeners are not conflicts.
Reset-Fixture
New-Secrets
New-ApiExe
Set-RunningInstance
$script:Listeners = @(
  [pscustomobject]@{ Port = 18000; OwningProcess = 202 },
  [pscustomobject]@{ Port = 13000; OwningProcess = 203 }
)
$owned = Get-InstanceStatus
Assert-Equal 0 $owned.conflicts.Count 'our own listeners are not conflicts'
Assert-Equal $true $owned.ok 'ok=true with our own listeners on our ports'

# A pid belonging to an unrelated program is not ours, even though it is alive.
Reset-Fixture
New-Secrets
New-ApiExe
Set-RunningInstance
$script:Procs[202] = New-Proc -ProcessId 202 -ExecutablePath 'C:\Windows\notepad.exe'
$recycled = Get-InstanceStatus
Assert-Equal $false $recycled.components.api.owned 'a recycled pid is not reported as our api'
Assert-Equal $false $recycled.ok 'ok=false when a component pid is not ours'

# An api process seconds old is a crash loop, not a healthy instance.
Reset-Fixture
New-Secrets
New-ApiExe
Set-RunningInstance
$script:Procs[202] = New-Proc -ProcessId 202 -ExecutablePath (Join-Path $FakeState 'api.exe') -CreationDate (Get-Date).AddSeconds(-2)
$restarting = Get-InstanceStatus
Assert-Equal 'restarting' $restarting.components.api.state 'a just-started api reports restarting'
Assert-Equal $false $restarting.ok 'ok=false while a component is restarting'

# ===========================================================================
# US2 - start refuses, by component, when a precondition is missing
# ===========================================================================

# T008: secrets.json missing.
Reset-Fixture
New-ApiExe
$out = (Test-StartPreconditions | Out-String)
Assert-Contains $out '[secrets]' 'missing secrets.json names the secrets component'
Assert-Contains $out 'next:' 'the failure says what to do next'
Assert-NotContains $out $SecretMarker 'the secrets failure does not print the secret'

# T008: api.exe missing.
Reset-Fixture
New-Secrets
$out = (Test-StartPreconditions | Out-String)
Assert-Contains $out '[api]' 'missing api.exe names the api component'
Assert-Contains $out 'local-windows.ps1 build' 'the api failure points at build'
Assert-NotContains $out $SecretMarker 'the api failure does not print the secret'

# T008: a foreign process holding the web port.
Reset-Fixture
New-Secrets
New-ApiExe
$script:Procs[9999] = New-Proc -ProcessId 9999 -ExecutablePath 'C:\other\node.exe'
$script:Listeners = @([pscustomobject]@{ Port = 13000; OwningProcess = 9999 })
$out = (Test-StartPreconditions | Out-String)
Assert-Contains $out '[web]' 'an occupied web port names the web component'
Assert-Contains $out '9999' 'the port failure names the holding pid'
Assert-Contains $out 'one machine supports one instance' 'the port failure states the single-instance limit'

# T008: a stale supervisor.pid is cleared rather than believed.
Reset-Fixture
New-Secrets
New-ApiExe
Set-Content -Path (Join-Path $FakeState 'supervisor.pid') -Value '4242'   # no such process
$out = (Test-StartPreconditions | Out-String)
Assert-True (-not (Test-Path (Join-Path $FakeState 'supervisor.pid'))) 'a stale supervisor.pid is deleted'
Assert-Contains $out 'cleared stale supervisor.pid' 'clearing the stale pid is reported'

# A successful start records the commit it launched, so status can report it.
Reset-Fixture
New-Secrets
New-ApiExe
$code = Invoke-InstanceStart -ScriptDir (Join-Path $FakeRoot 'scripts') | Select-Object -Last 1
Assert-Equal 0 $code 'a clean start exits 0'
Assert-True $script:SupervisorStarted 'a clean start launches the supervisor'
Assert-Equal 'abc1234' (Get-Content (Join-Path $FakeState 'build.txt') -Raw).Trim() 'start writes the build commit'

# T008: preconditions pass when everything is in place.
Reset-Fixture
New-Secrets
New-ApiExe
$ok = Test-StartPreconditions | Select-Object -Last 1
Assert-Equal $true $ok 'preconditions pass with secrets, api.exe, postgres and free ports'

# T008: starting an already-running instance is a no-op that exits 0.
Reset-Fixture
New-Secrets
New-ApiExe
Set-RunningInstance
$out = (Invoke-InstanceStart -ScriptDir (Join-Path $FakeRoot 'scripts') | Out-String)
$code = Invoke-InstanceStart -ScriptDir (Join-Path $FakeRoot 'scripts') | Select-Object -Last 1
Assert-Contains $out 'already running' 'a second start reports the instance is already running'
Assert-Equal 0 $code 'a second start exits 0'
Assert-True (-not $script:SupervisorStarted) 'a second start does not launch another supervisor'

# ===========================================================================
# US3 - stop waits, reports, and never kills
# ===========================================================================

# T010: nothing running -> exit 0, idempotent, no stop file needed.
Reset-Fixture
$out = (Wait-InstanceStop -TimeoutSec 2 | Out-String)
$code = Wait-InstanceStop -TimeoutSec 2 | Select-Object -Last 1
Assert-Equal 0 $code 'stop exits 0 when nothing is running'
Assert-Contains $out 'Nothing to stop' 'stop says there was nothing to stop'
Assert-Contains $out 'PostgreSQL' 'stop states PostgreSQL is untouched'

# T010: processes that never exit -> timeout path, exit 1, no kill.
Reset-Fixture
New-Secrets
New-ApiExe
Set-RunningInstance
$sw = [System.Diagnostics.Stopwatch]::StartNew()
$out = (Wait-InstanceStop -TimeoutSec 2 | Out-String)
$sw.Stop()
Assert-Contains $out 'still running' 'the timeout path lists the components that did not exit'
Assert-Contains $out 'Timed out' 'the timeout is reported as such'
Assert-Contains $out 'Not killing them' 'stop states it did not kill anything'
Assert-Contains $out '202' 'the timeout output names the surviving pid'
Assert-True (Test-Path (Join-Path $FakeState 'stop')) 'stop wrote the graceful-stop signal file'
Assert-True ($sw.Elapsed.TotalSeconds -lt 10) 'the wait honours its timeout instead of hanging'
$code = Wait-InstanceStop -TimeoutSec 2 | Select-Object -Last 1
Assert-Equal 1 $code 'stop exits 1 when a component outlives the timeout'

# T010: processes that do exit -> exit 0 and each component reported.
Reset-Fixture
New-Secrets
New-ApiExe
Set-RunningInstance
$script:Procs = @{}   # every process has gone
$out = (Wait-InstanceStop -TimeoutSec 5 | Out-String)
$code = Wait-InstanceStop -TimeoutSec 5 | Select-Object -Last 1
Assert-Equal 0 $code 'stop exits 0 once the processes are gone'

# ---------------------------------------------------------------------------
Remove-Item -Recurse -Force $Tmp -ErrorAction SilentlyContinue
if ($script:Failures -gt 0) {
  Write-Host "`n$($script:Failures) failed / $($script:Checks) checks" -ForegroundColor Red
  exit 1
}
Write-Host "`nAll $($script:Checks) checks passed." -ForegroundColor Green
exit 0
