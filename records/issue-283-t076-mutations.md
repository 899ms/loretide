# Issue #283 · T076 mutation-runner handoff

This adds a mutation runner for the four T076 regressions at the current 036 PR3 head. It is prepared, not executed. Do not run on a developer workstation or against a local/business database. The parent controller should copy or fetch this exact head into its existing isolated test container and run the script there, serialized with the runner's existing lock.

## Script

[`issue-283-t076-mutations.mjs`](issue-283-t076-mutations.mjs) runs each targeted test green before mutation, validates exactly one textual anchor per patch, applies one mutant, requires the named test to run and fail at a test assertion with its expected diagnostic (compile errors, skips, and unrelated failures do not count), restores the exact original file bytes in `finally`, then requires the same targeted test to pass again.

Run from the clean checkout root in the isolated container:

```sh
node records/issue-283-t076-mutations.mjs
```

Required environment is the same explicit isolated DB contract as `scripts/test-go-db.sh`: `LORETIDE_DB_TESTS=1`, `LORETIDE_DB_TEST_DATABASE_URL`, `LORETIDE_DB_TEST_DATABASE`, `LORETIDE_DB_TEST_ROLE`, and `LORETIDE_DB_TEST_RUN_ID`. The direct T060 work-editor fixture receives `LORETIDE_WORK_TEST_DATABASE_URL` from that already-provisioned database URL. The script has not been run in this task.

## Mutation-to-test map

| Mutation | Targeted regression tests | Expected red signal |
| --- | --- | --- |
| Move `idempotency.Claim` after `ApplyBody` base/document checks in `work-editor/version.go` | `TestApplyBodyAppendsOneSuggestionVersionAndReplays` and `TestApplyBodySameKeyConcurrentWritesOneVersion` (T060); `TestContentSearchSuggestionRetryReplaysTheRealWorkVersion` (T067) | Replay no longer returns the original version; concurrent ApplyBody or retry fails to recover the committed version. |
| Commit the adoption decision before checking document base and saved-draft preconditions in `topic-planning/search_suggestion.go` | `TestContentSearchSuggestionAdoptionPreflightAndRepeat` (T064/T065) | The draft-status conflict still occurs, but an irreversible decision row was already committed. |
| Map `workspacecore.ErrNotFound` to `ErrStorage` in `content_search_suggestions.go` | `TestContentSearchSuggestionAdaptersAndEndpointsAfterWorkspaceDeletion` (T071) | Deleted-workspace adapter answer becomes storage/503 instead of not-found/404. |
| Copy an approved old `content_review_request` onto the newly applied version in `content_search_suggestions.go` | `TestContentSearchAdoptionKeepsOldReviewAndDeliveryOnV3` (T072); `TestSearchHandlersDoNotWriteDeliveryOrCallNetwork` (T049) | v4 gains an inherited approved review and the static downstream-write guard reports the SQL write. |

## Current verification state

- Prepared against code head `3ad3234ebe2ed25967bcb852f89019cdd7f7b13c`; no mutation or DB test has been executed locally.
- The normal isolated GitHub Actions run `36254189731` passed all four jobs on that code head. This is not evidence for T076 mutation resistance; the parent controller owns the serial mutation run and should report each mutation's red assertion plus restored-green result.
