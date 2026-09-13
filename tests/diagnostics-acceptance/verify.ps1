[CmdletBinding()]
param(
  [string]$DatabaseUrl = ""
)

$ErrorActionPreference = "Stop"
$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot "..\..")).Path
Set-Location $repoRoot

function Assert-FileContains {
  param([string]$Path, [string]$Pattern, [string]$Description)
  if (-not (Test-Path -LiteralPath $Path)) {
    throw "Missing expected diagnostic source: $Path"
  }
  if (-not (Select-String -LiteralPath $Path -Pattern $Pattern -Quiet)) {
    throw "Static acceptance failed: $Description"
  }
  Write-Host "PASS static: $Description"
}

# These checks deliberately prove only source-level wiring. They are not a
# substitute for a browser or database acceptance result.
Assert-FileContains "apps/web/platform/content-diagnostics.ts" "application/json" "download creates a JSON blob"
Assert-FileContains "apps/web/platform/content-diagnostics.ts" "link.click" "download starts only after the blob is created"
Assert-FileContains "packages/core/content/diagnostics/queries.ts" 'after=\$\{cursor.current\}' "stream resumes from the saved cursor"
Assert-FileContains "packages/core/content/diagnostics/queries.ts" "mergeEvents" "stream de-duplicates resumed events"
Assert-FileContains "server/internal/content/diagnostics/service.go" "Real executor replay is unavailable" "exports disclose the real-executor boundary"
Assert-FileContains "server/internal/content/diagnostics/store_integration_test.go" "cross account" "database suite covers account isolation"
Assert-FileContains "server/internal/content/diagnostics/store_integration_test.go" "expired cursor not visible" "database suite covers retention gaps"

& go -C server test ./internal/content/diagnostics -run 'TestTracePropagationAndUntrustedIdentity|TestSnapshotImmutableAndRevocation|TestDiagnosticLimitsRejectInvalidConfiguration|TestSecretsNeverEnterTechnicalLog|TestWebSocketQueuePropagationDuplicateAndRevocation|TestTransportNonTestDenied|TestEverySimulatedFaultAndDeterministicTime|TestSimulationCannotAuthorizeOrReplayHumanActions' -count=1
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
Write-Host "PASS isolated Go unit/transport acceptance subset"

if ($DatabaseUrl) {
  $uri = [Uri]$DatabaseUrl
  if ($uri.Host -notin @("localhost", "127.0.0.1", "::1") -or $uri.Port -ne 15402) {
    throw "DatabaseUrl must target the reserved isolated PostgreSQL listener on localhost:15402."
  }
  $env:LORETIDE_DIAG_TEST_DATABASE_URL = $DatabaseUrl
  & go -C server test ./internal/content/diagnostics -run 'TestPostgresAuditRollbackIsolationAndRetention|TestPostgresFullScenariosExportAndHealth' -count=1
  if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
  Write-Host "PASS isolated PostgreSQL acceptance subset"
} else {
  Write-Host "SKIP isolated PostgreSQL acceptance: provide -DatabaseUrl for localhost:15402."
}
