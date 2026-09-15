---
name: go-modern-guidelines
description: Modern Go idioms (JetBrains Modern Go Guidelines, vendored) for Loretide-owned Go code only — server/internal/content/** and new handler files. Never used to refactor upstream Multica code.
---

# Modern Go Guidelines (vendored, scoped)

Source of truth: `FEATURES.md` in this directory (JetBrains, Apache-2.0, pinned to upstream commit `155dc7c`, v1.1.1). Read the relevant section there; there is no CLI and nothing to download.

## Scope — read this before applying anything

1. **Applies only to Loretide-owned Go**: files under `server/internal/content/**`, and handler/route files a Loretide PR creates. It does **not** apply to upstream Multica code (`server/internal/daemon/**`, `server/pkg/**`, existing handlers, anything not created by a Loretide spec). Constitution principle VIII: scope is the claimed task.
2. **Only the lines you are writing or changing.** Do not "modernize" neighbouring code, and never open a file just to apply a guideline. The upstream SKILL's advice to follow a guideline "even when nearby code uses an older pattern" is **overridden** here: in an upstream file, match the upstream pattern.
3. **Target version comes from `server/go.mod`** (currently `go 1.26.x`). Ignore guidelines marked for a newer Go.
4. **Permanently excluded regardless of Go version**, because they conflict with upstream technology choices and would create a second stack:
   - `json_v2` — upstream is `encoding/json` v1 everywhere; do not introduce `encoding/json/v2`.
   - `stdlib_uuid` — upstream standardizes on `github.com/google/uuid`; keep using it.
5. Skip any guideline that would not compile, would change behaviour, or is not a fit for the edited code. Correctness and the repo's existing rules (CLAUDE.md, `gofmt`, `go vet`, checked errors, no FKs/cascades, `CREATE INDEX CONCURRENTLY`) win over style.

## How to use

Before writing new Go in scope: skim the `## Guidelines` index in `FEATURES.md`, then read only the sections whose ID matches what you are about to write (e.g. `errors_as_type`, `sync_waitgroup_go`, `slices_contains`, `maps_keys_values_iter`, `range_over_int`, `min_max`, `cmp_or`, `http_servemux_patterns` — noting the router here is chi, so the last one is informational).

In the PR body, do not list guideline IDs as evidence of anything; they are style, not verification.
