$ErrorActionPreference = "Stop"
. "$PSScriptRoot\verify.ps1" -Library

function Assert-Throws {
  param([scriptblock]$Action, [string]$Name, [string]$MustNotContain = "")
  try { & $Action; throw "$Name did not reject" } catch {
    if ($_.Exception.Message -eq "$Name did not reject") { throw }
    if ($MustNotContain -and $_.Exception.Message.Contains($MustNotContain)) { throw "$Name leaked a secret" }
  }
}
function New-GoEvents {
  param([string[]]$Names, [string]$Terminal = "pass")
  $events = foreach ($name in $Names) { [pscustomobject]@{Action="run";Test=$name}; [pscustomobject]@{Action=$Terminal;Test=$name} }
  return @($events)
}
function New-VitestReport {
  param([bool]$IncludeBoth = $true, [bool]$Pass = $true)
  $files = @([pscustomobject]@{name="C:/repo/apps/web/platform/content-diagnostics.test.ts"})
  if ($IncludeBoth) { $files += [pscustomobject]@{name="C:/repo/packages/core/content/diagnostics/queries.test.tsx"} }
  return [pscustomobject]@{numTotalTests=2;numPassedTests=if($Pass){2}else{1};numFailedTests=if($Pass){0}else{1};testResults=$files}
}

$goNames = @("one", "two")
Assert-GoEvidence (New-GoEvents $goNames) $goNames
Assert-Throws { Assert-GoEvidence @([pscustomobject]@{Action="pass";Test="one"}) @("one") } "missing Go run"
Assert-Throws { Assert-GoEvidence (New-GoEvents @("one") "skip") @("one") } "skipped Go test"
Assert-Throws { Assert-GoEvidence (New-GoEvents @("one") "fail") @("one") } "failed Go test"
Assert-VitestEvidence (New-VitestReport)
Assert-Throws { Assert-VitestEvidence (New-VitestReport $false) } "missing Vitest file"
Assert-Throws { Assert-VitestEvidence (New-VitestReport $true $false) } "failed Vitest test"
foreach ($url in @("postgres://u:p@localhost:15403/loretide_diag_acceptance_x", "postgres://u:p@localhost:15402/not_a_test_db", "postgres://u:p@localhost:15402/loretide_diag_acceptance_x?host=other")) { Assert-Throws { Assert-DatabaseUri $url } "invalid database URI" }
$target = Assert-DatabaseUri "postgres://u:p@localhost:15402/loretide_diag_acceptance_x?sslmode=disable"
Assert-DatabaseIdentity @("loretide_diag_acceptance_x|u|f") $target
Assert-Throws { Assert-DatabaseIdentity @("wrong|u|f") $target } "mismatched database identity"
Assert-Throws { Assert-DatabaseIdentity @("loretide_diag_acceptance_x|u|t") $target } "superuser identity"

$saved = Get-Location; $oldDb = $env:LORETIDE_DIAG_TEST_DATABASE_URL; $oldPassword = $env:PGPASSWORD
$fakeSecret = "not-for-output"
$runner = { param($name,$context) if($name -eq "go"){ return New-GoEvents @("TestTracePropagationAndUntrustedIdentity", "TestSnapshotImmutableAndRevocation", "TestDiagnosticLimitsRejectInvalidConfiguration", "TestSecretsNeverEnterTechnicalLog", "TestWebSocketQueuePropagationDuplicateAndRevocation", "TestTransportNonTestDenied", "TestEverySimulatedFaultAndDeterministicTime", "TestSimulationCannotAuthorizeOrReplayHumanActions") }; if($name -eq "vitest"){ return New-VitestReport }; if($name -eq "psql"){ throw "driver failure $fakeSecret" } }
Invoke-DiagnosticsAcceptance -CommandRunner $runner
if ((Get-Location).Path -ne $saved.Path -or $env:LORETIDE_DIAG_TEST_DATABASE_URL -ne $oldDb -or $env:PGPASSWORD -ne $oldPassword) { throw "success path did not restore process state" }
Assert-Throws { Invoke-DiagnosticsAcceptance -DatabaseUrl "postgres://u:p@localhost:15402/loretide_diag_acceptance_x" -CommandRunner $runner } "runner failure" $fakeSecret
if ((Get-Location).Path -ne $saved.Path -or $env:LORETIDE_DIAG_TEST_DATABASE_URL -ne $oldDb -or $env:PGPASSWORD -ne $oldPassword) { throw "failure path did not restore process state" }
Write-Host "PASS verify.ps1 simulated behavior tests"
