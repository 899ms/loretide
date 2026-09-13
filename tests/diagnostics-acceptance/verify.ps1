[CmdletBinding()]
param(
  [string]$DatabaseUrl = "",
  [switch]$Library
)

function Assert-SourcePresence {
  param([string]$Path, [string]$Pattern, [string]$Description)
  if (-not (Test-Path -LiteralPath $Path)) { throw "Missing expected diagnostic source: $Path" }
  if (-not (Select-String -LiteralPath $Path -Pattern $Pattern -Quiet)) { throw "Source-presence check failed: $Description" }
  Write-Host "PASS source presence: $Description"
}

function Assert-GoEvidence {
  param([object[]]$Events, [string[]]$ExpectedTests)
  foreach ($name in $ExpectedTests) {
    $runs = @($Events | Where-Object { $_.Action -eq "run" -and $_.Test -eq $name })
    $passes = @($Events | Where-Object { $_.Action -eq "pass" -and $_.Test -eq $name })
    $nonPasses = @($Events | Where-Object { $_.Test -eq $name -and $_.Action -in @("skip", "fail") })
    if ($runs.Count -ne 1 -or $passes.Count -ne 1 -or $nonPasses.Count -ne 0) {
      throw "Expected exactly one non-skipped pass for $name; observed run=$($runs.Count), pass=$($passes.Count), skip-or-fail=$($nonPasses.Count)."
    }
  }
}

function Assert-VitestEvidence {
  param([object]$Report)
  $expectedFiles = @("apps/web/platform/content-diagnostics.test.ts", "packages/core/content/diagnostics/queries.test.tsx")
  $actualFiles = @($Report.testResults | ForEach-Object { $_.name.Replace("\\", "/") })
  if ($Report.numTotalTests -ne 2 -or $Report.numPassedTests -ne 2 -or $Report.numFailedTests -ne 0 -or $Report.testResults.Count -ne 2) {
    throw "Vitest JSON report did not show exactly two passing selected tests."
  }
  foreach ($file in $expectedFiles) {
    if (-not ($actualFiles | Where-Object { $_.EndsWith($file) })) { throw "Vitest JSON report omitted $file." }
  }
}

function Assert-DatabaseUri {
  param([string]$Value)
  try { $uri = [Uri]$Value } catch { throw "DatabaseUrl has invalid format." }
  $databaseName = $uri.AbsolutePath.TrimStart("/")
  $queryKeys = @($uri.Query.TrimStart("?").Split("&", [System.StringSplitOptions]::RemoveEmptyEntries) | ForEach-Object { [Uri]::UnescapeDataString(($_ -split "=", 2)[0]) })
  $rawCredentials = $uri.UserInfo.Split(":", 2)
  if ($uri.Scheme -notin @("postgres", "postgresql") -or $uri.Host -notin @("localhost", "127.0.0.1", "::1") -or $uri.Port -ne 15402 -or $databaseName -notmatch '^loretide_diag_acceptance_[a-z0-9_]+$' -or $rawCredentials.Count -ne 2 -or -not $rawCredentials[0] -or -not $rawCredentials[1] -or @($queryKeys | Where-Object { $_ -ne "sslmode" }).Count -ne 0) {
    throw "DatabaseUrl must name a credentialed localhost:15402 loretide_diag_acceptance_* database."
  }
  return [pscustomobject]@{ Uri = $uri; DatabaseName = $databaseName; User = [Uri]::UnescapeDataString($rawCredentials[0]); Password = [Uri]::UnescapeDataString($rawCredentials[1]) }
}

function Assert-DatabaseIdentity {
  param([string[]]$Identity, [object]$Target)
  $parts = ($Identity | Select-Object -Last 1) -split '\|'
  if ($parts.Count -ne 3 -or $parts[0] -ne $Target.DatabaseName -or $parts[1] -ne $Target.User -or $parts[2] -ne "f") {
    throw "PostgreSQL identity gate requires the requested database/current user and a non-superuser role."
  }
}

function Invoke-DefaultCommand {
  param([string]$Name, [object]$Context)
  switch ($Name) {
    "go" {
      $lines = & go -C server test ./internal/content/diagnostics -run $Context.Pattern -count=1 -json 2>&1
      if ($LASTEXITCODE -ne 0) { throw "Selected Go tests failed; inspect local test output without sharing connection details." }
      $events = @(); foreach ($line in $lines) { try { $events += ($line | ConvertFrom-Json) } catch { } }
      return $events
    }
    "vitest" {
      $reportPath = [System.IO.Path]::GetTempFileName()
      try {
        $nativeOutput = & pnpm exec vitest run apps/web/platform/content-diagnostics.test.ts packages/core/content/diagnostics/queries.test.tsx --reporter=json --outputFile $reportPath 2>&1
        if ($LASTEXITCODE -ne 0) { throw "Selected Vitest tests failed; inspect local test output without sharing connection details." }
        return Get-Content -Raw -LiteralPath $reportPath | ConvertFrom-Json
      } finally { Remove-Item -LiteralPath $reportPath -Force -ErrorAction SilentlyContinue }
    }
    "psql" {
      $psql = Get-Command psql -ErrorAction SilentlyContinue
      if (-not $psql) { throw "PostgreSQL protocol gate requires psql on PATH; no integration test was started." }
      $identity = & $psql.Source --no-psqlrc --tuples-only --no-align --quiet --host $Context.Target.Uri.Host --port $Context.Target.Uri.Port --username $Context.Target.User --dbname $Context.Target.DatabaseName --command "SELECT current_database(), current_user, (SELECT rolsuper FROM pg_roles WHERE rolname = current_user);" 2>&1
      if ($LASTEXITCODE -ne 0) { throw "PostgreSQL protocol/identity gate failed; integration test was not started." }
      return @($identity)
    }
    default { throw "Unsupported acceptance command: $Name" }
  }
}

function Invoke-RunnerSafely {
  param([scriptblock]$Runner, [string]$Name, [object]$Context)
  try { return & $Runner $Name $Context } catch {
    throw "Acceptance $Name command failed; inspect local output without sharing connection details."
  }
}

function Invoke-DiagnosticsAcceptance {
  param([string]$DatabaseUrl = "", [scriptblock]$CommandRunner = ${function:Invoke-DefaultCommand})
  $originalLocation = Get-Location
  $originalDatabaseUrl = $env:LORETIDE_DIAG_TEST_DATABASE_URL
  $originalPgPassword = $env:PGPASSWORD
  $repoRoot = (Resolve-Path (Join-Path $PSScriptRoot "..\..")).Path
  $tests = @("TestTracePropagationAndUntrustedIdentity", "TestSnapshotImmutableAndRevocation", "TestDiagnosticLimitsRejectInvalidConfiguration", "TestSecretsNeverEnterTechnicalLog", "TestWebSocketQueuePropagationDuplicateAndRevocation", "TestTransportNonTestDenied", "TestEverySimulatedFaultAndDeterministicTime", "TestSimulationCannotAuthorizeOrReplayHumanActions")
  try {
    Set-Location $repoRoot
    Assert-SourcePresence "apps/web/platform/content-diagnostics.ts" "application/json" "download JSON content type is wired"
    Assert-SourcePresence "apps/web/platform/content-diagnostics.ts" "link.click" "download trigger is wired"
    Assert-SourcePresence "packages/core/content/diagnostics/queries.ts" 'after=\$\{cursor.current\}' "stream cursor query is wired"
    Assert-SourcePresence "packages/core/content/diagnostics/queries.ts" "mergeEvents" "stream merge helper is wired"
    Assert-SourcePresence "server/internal/content/diagnostics/service.go" "Real executor replay is unavailable" "real-executor limit is disclosed"
    $events = Invoke-RunnerSafely $CommandRunner "go" ([pscustomobject]@{ Pattern = ($tests -join "|") })
    Assert-GoEvidence $events $tests
    Write-Host "PASS Go behavior tests: $($tests.Count) named tests ran and passed exactly once"
    $report = Invoke-RunnerSafely $CommandRunner "vitest" $null
    Assert-VitestEvidence $report
    Write-Host "PASS Vitest behavior tests: 2 named files and 2 tests passed"
    if ($DatabaseUrl) {
      $target = Assert-DatabaseUri $DatabaseUrl
      $env:LORETIDE_DIAG_TEST_DATABASE_URL = $DatabaseUrl
      $env:PGPASSWORD = $target.Password
      $identity = Invoke-RunnerSafely $CommandRunner "psql" ([pscustomobject]@{ Target = $target })
      Assert-DatabaseIdentity $identity $target
      $dbEvents = Invoke-RunnerSafely $CommandRunner "go" ([pscustomobject]@{ Pattern = "TestPostgresAuditRollbackIsolationAndRetention|TestPostgresFullScenariosExportAndHealth" })
      Assert-GoEvidence $dbEvents @("TestPostgresAuditRollbackIsolationAndRetention", "TestPostgresFullScenariosExportAndHealth")
      Write-Host "PASS isolated PostgreSQL acceptance subset"
    } else { Write-Host "SKIP isolated PostgreSQL acceptance: provide a verified localhost:15402 test database URL." }
  } finally {
    $env:LORETIDE_DIAG_TEST_DATABASE_URL = $originalDatabaseUrl
    $env:PGPASSWORD = $originalPgPassword
    Set-Location $originalLocation
  }
}

if (-not $Library) { Invoke-DiagnosticsAcceptance -DatabaseUrl $DatabaseUrl }
