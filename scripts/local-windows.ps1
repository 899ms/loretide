#Requires -Version 7.0
# Project-local Windows instance lifecycle: start, stop, status, build.
#
# One checkout is one instance: postgres + supervisor + api + web. The ports are
# fixed (15332 / 18000 / 13000), so a second checkout on the same machine is not
# supported - its `start` fails at the port check below and names the process
# holding the port rather than silently taking it over. See
# docs/development/native-windows.md -> Limits.
#
# `status` has to answer "is this mine", not just "does the port answer". A
# health probe against a port another checkout is serving returns 200 and proves
# nothing, so every component is matched to this checkout by executable path or
# command line first, and probed only once it is owned. This mirrors what
# `make status` reports on Linux (scripts/dev-env.sh -> component_state).
#
# Windows-only calls (CIM, Get-NetTCPConnection, pg_ctl.exe, HTTP) are isolated
# in the wrappers below so scripts/local-windows.test.ps1 can shadow them with
# stubs and run the logic on any platform.

param(
  [ValidateSet('start', 'stop', 'status', 'build')][string]$Action = 'status',
  [switch]$Json,
  [int]$TimeoutSec = 30
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

# BEGIN DEFINITIONS
# Everything between these markers is pure definitions: no side effects, no
# dispatch. scripts/local-windows.test.ps1 slices this region out and
# dot-sources it, so keep it free of top-level statements.

$script:Ports = @{ postgres = 15332; api = 18000; web = 13000 }
$script:ComponentOrder = @('postgres', 'supervisor', 'api', 'web')

# Paths come from the environment when set so tests can point the whole script
# at a temporary directory. Unset, they are the real Windows locations.
function Initialize-InstanceContext {
  param([string]$Root)
  $script:Root = $Root
  $script:Runtime = if ($env:LORETIDE_WIN_RUNTIME) { $env:LORETIDE_WIN_RUNTIME } else { 'F:\loretide-runtime' }
  $script:State = if ($env:LORETIDE_WIN_STATE) { $env:LORETIDE_WIN_STATE } else { Join-Path $Root 'data/windows' }
  $script:PgBin = Join-Path $script:Runtime 'pgsql/bin'
  $script:PgData = Join-Path $script:Runtime 'pg'
}

function Get-StatePath {
  param([Parameter(Mandatory)][string]$Name)
  Join-Path $script:State $Name
}

# --- Platform wrappers (stubbed in tests) ----------------------------------

# Returns $null when the pid is gone. Only ProcessId/ExecutablePath/CommandLine
# are used, so a stub returns a PSCustomObject with those three properties.
function Get-ProcessInfo {
  param([Parameter(Mandatory)][int]$ProcessId)
  try {
    Get-CimInstance Win32_Process -Filter "ProcessId = $ProcessId" -ErrorAction Stop |
      Select-Object -First 1 ProcessId, ParentProcessId, ExecutablePath, CommandLine, CreationDate
  } catch {
    $null
  }
}

# Listening sockets on the instance ports, as @{ Port; OwningProcess }.
function Get-PortListener {
  param([Parameter(Mandatory)][int[]]$Port)
  try {
    Get-NetTCPConnection -State Listen -LocalPort $Port -ErrorAction Stop |
      ForEach-Object { [pscustomobject]@{ Port = [int]$_.LocalPort; OwningProcess = [int]$_.OwningProcess } }
  } catch {
    @()
  }
}

# Read-only pg_ctl calls (status). Capturing output here is safe because the
# command exits on its own.
function Invoke-PgCtl {
  param([Parameter(Mandatory)][string[]]$PgArgument)
  $exe = Join-Path $script:PgBin 'pg_ctl.exe'
  if (-not (Test-Path $exe)) { return [pscustomobject]@{ ExitCode = 127; Output = "pg_ctl.exe not found at $exe" } }
  $out = & $exe @PgArgument 2>&1
  [pscustomobject]@{ ExitCode = $LASTEXITCODE; Output = ($out | Out-String).Trim() }
}

# Starting PostgreSQL has two ways to hang, and both were hit in turn.
#
# It must NOT run through a captured pipeline: the postgres.exe pg_ctl launches
# inherits the redirected handles and holds them for its whole life, so
# `$out = & pg_ctl ... 2>&1` waits for the database to shut down.
#
# It must also NOT use Start-Process -Wait: on Windows that waits for the whole
# process tree, and postgres.exe is pg_ctl's child, so -Wait means the same
# thing by another route. Both leave start hanging minutes after the server is
# ready to serve.
#
# Waiting on the Process object waits for THAT process only. Server output still
# goes to postgres.log via -l.
function Start-PostgresServer {
  $exe = Join-Path $script:PgBin 'pg_ctl.exe'
  if (-not (Test-Path $exe)) { return [pscustomobject]@{ ExitCode = 127; Output = "pg_ctl.exe not found at $exe" } }
  $logPath = Join-Path $script:Runtime 'postgres.log'
  $proc = Start-Process -FilePath $exe -PassThru -WindowStyle Hidden -ArgumentList @(
    '-D', "`"$($script:PgData)`"", '-l', "`"$logPath`"",
    '-o', "`"-p $($script:Ports.postgres) -h 127.0.0.1`"", '-w', 'start'
  )
  if (-not $proc.WaitForExit(60000)) {
    return [pscustomobject]@{ ExitCode = 124; Output = "pg_ctl start did not return within 60s; server log: $logPath" }
  }
  [pscustomobject]@{ ExitCode = $proc.ExitCode; Output = "pg_ctl start exited $($proc.ExitCode); server log: $logPath" }
}

# pg_ctl returning 0 is not the same as the server accepting connections, and
# after the two hangs above the launcher should prove readiness rather than
# assume it. `status` is safe to capture: it spawns nothing.
function Wait-PostgresReady {
  param([int]$TimeoutSec = 30)
  $deadline = (Get-Date).AddSeconds($TimeoutSec)
  do {
    if ((Invoke-PgCtl -PgArgument @('-D', $script:PgData, 'status')).ExitCode -eq 0) { return $true }
    Start-Sleep -Seconds 1
  } while ((Get-Date) -lt $deadline)
  $false
}

# Status code only; the body is never read, so a slow page cannot hold up status.
function Invoke-HealthProbe {
  param([Parameter(Mandatory)][string]$Url, [int]$TimeoutSec = 5)
  try {
    $r = Invoke-WebRequest $Url -TimeoutSec $TimeoutSec -UseBasicParsing -ErrorAction Stop
    [int]$r.StatusCode
  } catch {
    $code = 0
    if ($_.Exception.PSObject.Properties['Response'] -and $_.Exception.Response) {
      try { $code = [int]$_.Exception.Response.StatusCode } catch { $code = 0 }
    }
    $code
  }
}

function Get-CheckoutCommit {
  param([Parameter(Mandatory)][string]$Root)
  try {
    $c = & git -C $Root rev-parse --short HEAD 2>$null
    if ($LASTEXITCODE -eq 0 -and $c) { return "$c".Trim() }
  } catch {
    # git missing or not a repository: the build marker degrades, status does not fail.
  }
  'unknown'
}

function Start-SupervisorProcess {
  param([Parameter(Mandatory)][string]$Root, [Parameter(Mandatory)][string]$ScriptDir)
  $node = (Get-Command node.exe).Source
  Start-Process -FilePath $node `
    -ArgumentList @("`"$ScriptDir/local-windows-supervisor.mjs`"") `
    -WorkingDirectory $Root -WindowStyle Hidden `
    -RedirectStandardOutput (Get-StatePath 'supervisor.out.log') `
    -RedirectStandardError (Get-StatePath 'supervisor.err.log')
}

# --- Identity --------------------------------------------------------------

function Read-PidFile {
  param([Parameter(Mandatory)][string]$Name)
  $path = Get-StatePath $Name
  if (-not (Test-Path $path)) { return 0 }
  $raw = (Get-Content $path -Raw -ErrorAction SilentlyContinue)
  if (-not $raw) { return 0 }
  $parsed = 0
  if ([int]::TryParse($raw.Trim(), [ref]$parsed)) { return $parsed }
  0
}

# A pid is only ours when the process behind it still looks like the component
# we recorded AND its paths sit inside this checkout. A recycled pid belonging
# to an unrelated program fails the command-line test; a same-named process from
# another checkout fails the prefix test.
function Test-ProcessOwned {
  param($Process, [Parameter(Mandatory)][string]$Component)
  if (-not $Process) { return $false }
  $exe = if ($Process.PSObject.Properties['ExecutablePath']) { [string]$Process.ExecutablePath } else { '' }
  $cmd = if ($Process.PSObject.Properties['CommandLine']) { [string]$Process.CommandLine } else { '' }
  $haystack = "$exe $cmd"
  switch ($Component) {
    'supervisor' {
      return ($haystack -like '*local-windows-supervisor.mjs*') -and (Test-PathInsideRoot $haystack)
    }
    'api' {
      $expected = Join-Path $script:State 'api.exe'
      return ($exe -and (Test-SamePath $exe $expected))
    }
    'web' {
      return ($haystack -like '*next*') -and (Test-PathInsideRoot $haystack)
    }
  }
  $false
}

# Windows paths compare case-insensitively and mix separators; normalise both
# before deciding whether a process belongs to this checkout.
function ConvertTo-ComparablePath {
  param([string]$Path)
  if (-not $Path) { return '' }
  ($Path -replace '/', '\').TrimEnd('\').ToLowerInvariant()
}

function Test-SamePath {
  param([string]$Left, [string]$Right)
  (ConvertTo-ComparablePath $Left) -eq (ConvertTo-ComparablePath $Right)
}

function Test-PathInsideRoot {
  param([string]$Text)
  $root = ConvertTo-ComparablePath $script:Root
  if (-not $root) { return $false }
  (ConvertTo-ComparablePath $Text).Contains($root)
}

# The pid holding a port is often not the pid we recorded. `next dev` forks a
# worker and that child owns 13000, so matching the component pid alone reported
# a healthy instance as a conflict against itself. A listener is ours when it is
# a recorded pid, or runs out of this checkout, or descends from a recorded pid.
function Test-ListenerIsOurs {
  param([Parameter(Mandatory)][int]$ProcessId, [int[]]$OurPids = @())
  if ($ProcessId -le 0) { return $false }
  if ($OurPids -contains $ProcessId) { return $true }

  $proc = Get-ProcessInfo -ProcessId $ProcessId
  if (-not $proc) { return $false }

  $exe = if ($proc.PSObject.Properties['ExecutablePath']) { [string]$proc.ExecutablePath } else { '' }
  $cmd = if ($proc.PSObject.Properties['CommandLine']) { [string]$proc.CommandLine } else { '' }
  if (Test-PathInsideRoot "$exe $cmd") { return $true }

  # Walk the parent chain, bounded so a recycled or looping ppid cannot hang us.
  $current = $proc
  for ($depth = 0; $depth -lt 5; $depth++) {
    if (-not $current.PSObject.Properties['ParentProcessId'] -or -not $current.ParentProcessId) { break }
    $parentId = [int]$current.ParentProcessId
    if ($parentId -le 0 -or $parentId -eq [int]$current.ProcessId) { break }
    if ($OurPids -contains $parentId) { return $true }
    $current = Get-ProcessInfo -ProcessId $parentId
    if (-not $current) { break }
  }
  $false
}

function Test-ProcessAlive {
  param([int]$ProcessId, [string]$Component)
  if ($ProcessId -le 0) { return $false }
  $p = Get-ProcessInfo -ProcessId $ProcessId
  if (-not $p) { return $false }
  if (-not $Component) { return $true }
  Test-ProcessOwned -Process $p -Component $Component
}

# --- Status ----------------------------------------------------------------

function Get-PostgresState {
  $result = Invoke-PgCtl -PgArgument @('-D', $script:PgData, 'status')
  $component = [ordered]@{
    state  = 'stopped'
    pid    = 0
    owned  = $false
    port   = $script:Ports.postgres
    detail = "data dir $($script:PgData)"
  }
  if ($result.ExitCode -eq 127) {
    $component.detail = $result.Output
    return @{ component = $component; fatal = $true }
  }
  if ($result.ExitCode -eq 0) {
    $component.state = 'running'
    $component.owned = $true
    $postmaster = Join-Path $script:PgData 'postmaster.pid'
    if (Test-Path $postmaster) {
      $first = (Get-Content $postmaster -TotalCount 1 -ErrorAction SilentlyContinue)
      $parsed = 0
      if ($first -and [int]::TryParse("$first".Trim(), [ref]$parsed)) { $component.pid = $parsed }
    }
  }
  @{ component = $component; fatal = $false }
}

# `restarting` is the supervisor's 3-second crash loop seen from outside: the
# pid file keeps changing and each process is seconds old. Reporting it as
# `running` would hide exactly the failure this command exists to surface.
function Test-RecentStart {
  param($Process)
  if (-not $Process -or -not $Process.PSObject.Properties['CreationDate'] -or -not $Process.CreationDate) { return $false }
  try { return ((Get-Date) - [datetime]$Process.CreationDate).TotalSeconds -lt 10 } catch { return $false }
}

function Get-ChildComponentState {
  param([Parameter(Mandatory)][string]$Name, [int]$Port, [string]$HealthUrl)
  $component = [ordered]@{ state = 'stopped'; pid = 0; owned = $false }
  if ($Port) { $component.port = $Port }

  $processId = Read-PidFile "$Name.pid"
  if ($processId -le 0) { return $component }

  $process = Get-ProcessInfo -ProcessId $processId
  if (-not $process) {
    # A pid file left behind by a dead process is not "running"; say so by name
    # so `start` knows it may clear it.
    $component.state = if ($Name -eq 'supervisor') { 'stale-pid' } else { 'stopped' }
    $component.pid = $processId
    $component.detail = "pid $processId is not running (stale $Name.pid)"
    return $component
  }

  $owned = Test-ProcessOwned -Process $process -Component $Name
  $component.pid = $processId
  $component.owned = $owned
  if (-not $owned) {
    $component.state = if ($Name -eq 'supervisor') { 'stale-pid' } else { 'stopped' }
    $component.detail = "pid $processId does not belong to this checkout"
    return $component
  }

  $component.state = if ((Test-RecentStart -Process $process) -and $Name -ne 'supervisor') { 'restarting' } else { 'running' }
  if ($HealthUrl) {
    $component.health = [ordered]@{ url = $HealthUrl; status = (Invoke-HealthProbe -Url $HealthUrl) }
  }
  $component
}

function Get-BuildInfo {
  $info = [ordered]@{ commit = 'unknown'; api_exe_mtime = $null }
  $buildFile = Get-StatePath 'build.txt'
  if (Test-Path $buildFile) {
    $c = (Get-Content $buildFile -Raw -ErrorAction SilentlyContinue)
    if ($c) { $info.commit = "$c".Trim() }
  }
  $apiExe = Get-StatePath 'api.exe'
  if (Test-Path $apiExe) {
    $info.api_exe_mtime = (Get-Item $apiExe).LastWriteTime.ToString('o')
  }
  $info
}

function Get-InstanceStatus {
  $components = [ordered]@{}
  $pg = Get-PostgresState
  $components.postgres = $pg.component
  $components.supervisor = Get-ChildComponentState -Name 'supervisor'
  $components.api = Get-ChildComponentState -Name 'api' -Port $script:Ports.api -HealthUrl "http://127.0.0.1:$($script:Ports.api)/health"
  $components.web = Get-ChildComponentState -Name 'web' -Port $script:Ports.web -HealthUrl "http://localhost:$($script:Ports.web)"

  # Any listener on our ports that is not one of ours is someone else's process.
  # Without this, a foreign server answering 200 reads as healthy.
  $ourPids = @($components.Values | ForEach-Object { if ($_.owned) { [int]$_.pid } } | Where-Object { $_ -gt 0 })
  $conflicts = @()
  foreach ($listener in (Get-PortListener -Port @($script:Ports.postgres, $script:Ports.api, $script:Ports.web))) {
    if (Test-ListenerIsOurs -ProcessId $listener.OwningProcess -OurPids $ourPids) { continue }
    $proc = Get-ProcessInfo -ProcessId $listener.OwningProcess
    $name = 'unknown'
    if ($proc -and $proc.ExecutablePath) { $name = Split-Path $proc.ExecutablePath -Leaf }
    $conflicts += [ordered]@{ port = $listener.Port; pid = $listener.OwningProcess; process = $name; owned = $false }
  }

  $allRunning = -not ($components.Values | Where-Object { $_.state -ne 'running' -or -not $_.owned })
  [ordered]@{
    checkout   = $script:Root
    build      = Get-BuildInfo
    components = $components
    conflicts  = $conflicts
    ok         = ($allRunning -and $conflicts.Count -eq 0)
    fatal      = $pg.fatal
  }
}

function Write-ComponentLine {
  param([Parameter(Mandatory)][string]$Name, [Parameter(Mandatory)]$Component)
  $owned = if ($Component.owned) { 'yes' } else { 'no' }
  $detailParts = @()
  if ($Component.Contains('port')) { $detailParts += "port=$($Component.port)" }
  if ($Component.Contains('health')) { $detailParts += "health=$($Component.health.url) $($Component.health.status)" }
  if ($Component.Contains('detail') -and $Component.detail) { $detailParts += $Component.detail }
  '{0,-11} {1,-11} pid={2,-7} owned={3,-4} {4}' -f $Name, $Component.state, $Component.pid, $owned, ($detailParts -join '  ')
}

function Write-InstanceStatus {
  param([Parameter(Mandatory)]$Status, [switch]$AsJson)
  if ($AsJson) {
    $payload = [ordered]@{
      checkout   = $Status.checkout
      build      = $Status.build
      components = $Status.components
      conflicts  = $Status.conflicts
      ok         = $Status.ok
    }
    Write-Output ($payload | ConvertTo-Json -Depth 6 -Compress)
  } else {
    Write-Output "checkout  $($Status.checkout)"
    Write-Output "build     commit=$($Status.build.commit) api.exe=$(if ($Status.build.api_exe_mtime) { $Status.build.api_exe_mtime } else { 'absent' })"
    foreach ($name in $script:ComponentOrder) {
      Write-Output (Write-ComponentLine -Name $name -Component $Status.components[$name])
    }
    foreach ($c in $Status.conflicts) {
      Write-Output "conflict    port=$($c.port) held by pid $($c.pid) ($($c.process)) - not this instance"
    }
    Write-Output "ok=$($Status.ok.ToString().ToLowerInvariant())"
  }
  if ($Status.fatal) { return 2 }
  if (-not $Status.ok) { return 1 }
  0
}

# --- Start -----------------------------------------------------------------

# Failures name the component, the log to read and the next command to run.
# The old script let a missing api.exe or secrets.json become a supervisor crash
# loop that only showed up in a log file nobody was told to open.
function Write-PreconditionFailure {
  param([Parameter(Mandatory)][string]$Component, [Parameter(Mandatory)][string]$Reason, [Parameter(Mandatory)][string]$NextStep)
  $log = Get-StatePath "$Component.log"
  Write-Output "[$Component] $Reason"
  Write-Output "            log: $log"
  Write-Output "            next: $NextStep"
}

function Test-StartPreconditions {
  # Order follows research D4: cheapest and most common misses first.
  $secrets = Get-StatePath 'secrets.json'
  if (-not (Test-Path $secrets)) {
    # Only the path is printed. The file holds the database password and JWT
    # secret and must never reach stdout.
    Write-PreconditionFailure -Component 'secrets' `
      -Reason "private configuration is missing at $secrets" `
      -NextStep 'restore it as described in docs/development/native-windows.md (Paths and runtime)'
    return $false
  }

  $apiExe = Get-StatePath 'api.exe'
  if (-not (Test-Path $apiExe)) {
    Write-PreconditionFailure -Component 'api' `
      -Reason "api.exe is missing at $apiExe" `
      -NextStep './scripts/local-windows.ps1 build'
    return $false
  }

  $pgStatus = Invoke-PgCtl -PgArgument @('-D', $script:PgData, 'status')
  if ($pgStatus.ExitCode -eq 127) {
    Write-PreconditionFailure -Component 'postgres' -Reason $pgStatus.Output `
      -NextStep 'install or restore the PostgreSQL runtime under the path above'
    return $false
  }
  if ($pgStatus.ExitCode -ne 0) {
    $started = Start-PostgresServer
    if ($started.ExitCode -ne 0) {
      Write-PreconditionFailure -Component 'postgres' -Reason $started.Output `
        -NextStep "read $(Join-Path $script:Runtime 'postgres.log')"
      return $false
    }
    if (-not (Wait-PostgresReady)) {
      Write-PreconditionFailure -Component 'postgres' `
        -Reason 'pg_ctl returned but the server is still not accepting status checks after 30s' `
        -NextStep "read $(Join-Path $script:Runtime 'postgres.log')"
      return $false
    }
  }

  # Port check before launching anything: a second checkout must stop here and
  # be told who holds the port, not start a supervisor that fights for it.
  $status = Get-InstanceStatus
  foreach ($conflict in $status.conflicts) {
    $component = switch ($conflict.port) {
      $script:Ports.api { 'api' }
      $script:Ports.web { 'web' }
      default { 'postgres' }
    }
    Write-PreconditionFailure -Component $component `
      -Reason "port $($conflict.port) is held by pid $($conflict.pid) ($($conflict.process)), which is not part of this checkout" `
      -NextStep 'stop that process, or use the other checkout; one machine supports one instance'
    return $false
  }

  # A pid file pointing at a dead or unrelated process would otherwise make
  # `start` report "already running" forever.
  if ($status.components.supervisor.state -eq 'stale-pid') {
    Remove-Item (Get-StatePath 'supervisor.pid') -Force -ErrorAction SilentlyContinue
    Write-Output "[supervisor] cleared stale supervisor.pid (pid $($status.components.supervisor.pid) is not this instance)"
  }
  $true
}

function Invoke-InstanceStart {
  param([Parameter(Mandatory)][string]$ScriptDir)
  if (-not (Test-Path $script:State)) { New-Item -ItemType Directory -Force -Path $script:State | Out-Null }

  $before = Get-InstanceStatus
  if ($before.components.supervisor.state -eq 'running') {
    Write-Output "Supervisor already running (pid $($before.components.supervisor.pid)). Nothing to start."
    return 0
  }

  if (-not (Test-StartPreconditions)) { return 1 }

  Set-Content -Path (Get-StatePath 'build.txt') -Value (Get-CheckoutCommit -Root $script:Root) -NoNewline
  # Clear the previous run's pid so the wait below cannot pass on a stale file.
  Remove-Item (Get-StatePath 'supervisor.pid') -Force -ErrorAction SilentlyContinue
  Start-SupervisorProcess -Root $script:Root -ScriptDir $ScriptDir

  if (-not (Wait-SupervisorAlive)) {
    # Launching the process is not the same as it surviving. The supervisor
    # reads secrets.json and exits immediately if anything in it is wrong, and
    # that error only reaches supervisor.err.log - so reporting success here
    # left a dead instance looking started, and the next start launched another.
    Write-PreconditionFailure -Component 'supervisor' `
      -Reason 'the supervisor exited immediately after launch' `
      -NextStep "read $(Get-StatePath 'supervisor.err.log')"
    return 1
  }

  Write-Output 'Local supervisor started. Browser: http://localhost:13000/loretide-dev-check/diagnostics'
  0
}

# Poll for the supervisor to write its pid and still be running under it.
# Both halves matter: the pid file alone proves only that it got far enough to
# write one.
function Wait-SupervisorAlive {
  param([int]$TimeoutSec = 10)
  $deadline = (Get-Date).AddSeconds($TimeoutSec)
  do {
    $processId = Read-PidFile 'supervisor.pid'
    if ($processId -gt 0 -and (Test-ProcessAlive -ProcessId $processId -Component 'supervisor')) { return $true }
    Start-Sleep -Milliseconds 500
  } while ((Get-Date) -lt $deadline)
  $false
}

# --- Stop ------------------------------------------------------------------

# The old `stop` wrote the signal file and returned, so the next `build` could
# fail on an api.exe the dying process still held. Wait for the processes to go,
# and never kill them: a forced exit is how a half-written database page happens.
function Wait-InstanceStop {
  param([int]$TimeoutSec = 30)
  $names = @('api', 'web', 'supervisor')
  $before = Get-InstanceStatus
  $live = @($names | Where-Object { $before.components[$_].owned })
  if ($live.Count -eq 0) {
    Write-Output 'Nothing to stop: no api, web or supervisor process belongs to this checkout.'
    Write-Output 'PostgreSQL is left untouched.'
    return 0
  }

  New-Item -ItemType File -Force (Get-StatePath 'stop') | Out-Null
  Write-Output 'Graceful application stop requested; database retained and running.'

  $pids = @{}
  foreach ($name in $live) { $pids[$name] = [int]$before.components[$name].pid }

  $deadline = (Get-Date).AddSeconds($TimeoutSec)
  $remaining = [System.Collections.Generic.List[string]]::new()
  do {
    $remaining.Clear()
    foreach ($name in $live) {
      if (Test-ProcessAlive -ProcessId $pids[$name] -Component $name) { $remaining.Add($name) }
    }
    if ($remaining.Count -eq 0) { break }
    Start-Sleep -Seconds 1
  } while ((Get-Date) -lt $deadline)

  foreach ($name in $live) {
    if ($remaining -contains $name) {
      Write-Output "$name still running (pid $($pids[$name]))"
    } else {
      Write-Output "$name exited (pid $($pids[$name]))"
    }
  }
  Write-Output 'PostgreSQL left running; no data was removed.'

  if ($remaining.Count -gt 0) {
    Write-Output "Timed out after ${TimeoutSec}s waiting for: $($remaining -join ', '). Not killing them; re-run stop or end them yourself."
    return 1
  }
  0
}

# --- Build -----------------------------------------------------------------

function Invoke-InstanceBuild {
  if (-not (Test-Path $script:State)) { New-Item -ItemType Directory -Force -Path $script:State | Out-Null }
  Push-Location (Join-Path $script:Root 'server')
  try {
    $env:GOTOOLCHAIN = 'auto'
    go build -o (Get-StatePath 'api.exe') ./cmd/server
    if ($LASTEXITCODE) { throw 'API build failed' }
  } finally {
    Pop-Location
  }
  0
}

function Invoke-InstanceAction {
  param([Parameter(Mandatory)][string]$Action, [switch]$Json, [int]$TimeoutSec = 30, [Parameter(Mandatory)][string]$ScriptDir)
  switch ($Action) {
    'build' { return Invoke-InstanceBuild }
    'start' { return Invoke-InstanceStart -ScriptDir $ScriptDir }
    'stop' { return Wait-InstanceStop -TimeoutSec $TimeoutSec }
    default { return Write-InstanceStatus -Status (Get-InstanceStatus) -AsJson:$Json }
  }
}
# END DEFINITIONS

# Entry point
Initialize-InstanceContext -Root (Split-Path $PSScriptRoot -Parent)
exit (Invoke-InstanceAction -Action $Action -Json:$Json -TimeoutSec $TimeoutSec -ScriptDir $PSScriptRoot)
