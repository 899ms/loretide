[CmdletBinding()]
param(
  [string]$DatabaseUrl = ""
)

$ErrorActionPreference = "Stop"
$originalLocation = Get-Location
$originalDatabaseUrl = $env:LORETIDE_DIAG_TEST_DATABASE_URL
$originalPgPassword = $env:PGPASSWORD
$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot "..\..")).Path

function Assert-SourcePresence {
  param([string]$Path, [string]$Pattern, [string]$Description)
  if (-not (Test-Path -LiteralPath $Path)) {
    throw "Missing expected diagnostic source: $Path"
  }
  if (-not (Select-String -LiteralPath $Path -Pattern $Pattern -Quiet)) {
    throw "Source-presence check failed: $Description"
  }
  Write-Host "PASS source presence: $Description"
}

function Invoke-SelectedGoTests {
  param([string[]]$ExpectedTests)
  $pattern = ($ExpectedTests -join "|")
  $lines = & go -C server test ./internal/content/diagnostics -run $pattern -count=1 -json 2>&1
  $exitCode = $LASTEXITCODE
  $events = @()
  foreach ($line in $lines) {
    Write-Host $line
    try { $events += ($line | ConvertFrom-Json) } catch { }
  }
  if ($exitCode -ne 0) { throw "Selected Go tests failed with exit code $exitCode." }
  foreach ($name in $ExpectedTests) {
    $runs = @($events | Where-Object { $_.Action -eq "run" -and $_.Test -eq $name })
    if ($runs.Count -ne 1) {
      throw "Expected exactly one Go test run for $name; observed $($runs.Count)."
    }
  }
  Write-Host "PASS Go behavior tests: $($ExpectedTests.Count) named tests ran exactly once"
}

function Invoke-SelectedVitestTests {
  $reportPath = [System.IO.Path]::GetTempFileName()
  try {
    & pnpm exec vitest run apps/web/platform/content-diagnostics.test.ts packages/core/content/diagnostics/queries.test.tsx --reporter=json --outputFile $reportPath
    $exitCode = $LASTEXITCODE
    if ($exitCode -ne 0) { throw "Selected Vitest tests failed with exit code $exitCode." }
    $report = Get-Content -Raw -LiteralPath $reportPath | ConvertFrom-Json
    $expectedFiles = @("apps/web/platform/content-diagnostics.test.ts", "packages/core/content/diagnostics/queries.test.tsx")
    $actualFiles = @($report.testResults | ForEach-Object { $_.name.Replace("\\", "/") })
    if ($report.numTotalTests -ne 2 -or $report.numPassedTests -ne 2 -or $report.numFailedTests -ne 0 -or $report.testResults.Count -ne 2) {
      throw "Vitest JSON report did not show exactly two passing selected tests."
    }
    foreach ($file in $expectedFiles) {
      if (-not ($actualFiles | Where-Object { $_.EndsWith($file) })) { throw "Vitest JSON report omitted $file." }
    }
    Write-Host "PASS Vitest behavior tests: 2 named files and 2 tests passed"
  } finally {
    Remove-Item -LiteralPath $reportPath -Force -ErrorAction SilentlyContinue
  }
}

try {
  Set-Location $repoRoot

  # These checks prove source presence only. Behavioural acceptance comes from
  # the named Go/Vitest tests below, never from a text match.
  Assert-SourcePresence "apps/web/platform/content-diagnostics.ts" "application/json" "download JSON content type is wired"
  Assert-SourcePresence "apps/web/platform/content-diagnostics.ts" "link.click" "download trigger is wired"
  Assert-SourcePresence "packages/core/content/diagnostics/queries.ts" 'after=\$\{cursor.current\}' "stream cursor query is wired"
  Assert-SourcePresence "packages/core/content/diagnostics/queries.ts" "mergeEvents" "stream merge helper is wired"
  Assert-SourcePresence "server/internal/content/diagnostics/service.go" "Real executor replay is unavailable" "real-executor limit is disclosed"

  Invoke-SelectedGoTests @(
    "TestTracePropagationAndUntrustedIdentity",
    "TestSnapshotImmutableAndRevocation",
    "TestDiagnosticLimitsRejectInvalidConfiguration",
    "TestSecretsNeverEnterTechnicalLog",
    "TestWebSocketQueuePropagationDuplicateAndRevocation",
    "TestTransportNonTestDenied",
    "TestEverySimulatedFaultAndDeterministicTime",
    "TestSimulationCannotAuthorizeOrReplayHumanActions"
  )
  Invoke-SelectedVitestTests

  if ($DatabaseUrl) {
    $uri = [Uri]$DatabaseUrl
    $databaseName = $uri.AbsolutePath.TrimStart("/")
    if ($uri.Scheme -notin @("postgres", "postgresql") -or $uri.Host -notin @("localhost", "127.0.0.1", "::1") -or $uri.Port -ne 15402 -or $databaseName -notmatch '^loretide_diag_acceptance_[a-z0-9_]+$' -or -not $uri.UserInfo) {
      throw "DatabaseUrl must name a credentialed localhost:15402 loretide_diag_acceptance_* database."
    }
    $psql = Get-Command psql -ErrorAction SilentlyContinue
    if (-not $psql) { throw "PostgreSQL protocol gate requires psql on PATH; no integration test was started." }
    $userInfo = [Uri]::UnescapeDataString($uri.UserInfo).Split(":", 2)
    $env:PGPASSWORD = if ($userInfo.Count -eq 2) { $userInfo[1] } else { "" }
    $identity = & $psql.Source --no-psqlrc --tuples-only --no-align --quiet --host $uri.Host --port $uri.Port --username $userInfo[0] --dbname $databaseName --command "SELECT current_database(), current_user, (SELECT rolsuper FROM pg_roles WHERE rolname = current_user);" 2>&1
    if ($LASTEXITCODE -ne 0) { throw "PostgreSQL protocol/identity gate failed; integration test was not started." }
    $parts = ($identity | Select-Object -Last 1) -split '\|'
    if ($parts.Count -ne 3 -or $parts[0] -ne $databaseName -or $parts[1] -ne $userInfo[0] -or $parts[2] -ne "f") {
      throw "PostgreSQL identity gate requires the requested database/current user and a non-superuser role."
    }
    $env:LORETIDE_DIAG_TEST_DATABASE_URL = $DatabaseUrl
    Invoke-SelectedGoTests @("TestPostgresAuditRollbackIsolationAndRetention", "TestPostgresFullScenariosExportAndHealth")
  } else {
    Write-Host "SKIP isolated PostgreSQL acceptance: provide a verified localhost:15402 test database URL."
  }
} finally {
  $env:LORETIDE_DIAG_TEST_DATABASE_URL = $originalDatabaseUrl
  $env:PGPASSWORD = $originalPgPassword
  Set-Location $originalLocation
}
