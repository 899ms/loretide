# Content module boundary checker

`scripts/check-content-boundaries.mjs` enforces the content module architecture
declared in `scripts/content-boundaries.json`. It is a **static** import
checker: it reads source text, extracts import specifiers, and validates them
against the configured module graph. It runs in the `Loretide content
contracts` workflow via:

```bash
node --test scripts/check-content-boundaries.test.mjs   # the checker's own tests
node scripts/check-content-boundaries.mjs               # scan the repository
```

## Running it locally

```bash
pnpm check:content-boundaries
```

This is the repository entry point; run it from the repository root, with no
arguments. It runs the two commands above in that order, joined by `&&`, so a
failing self-test exits before the scan and a passing scan can never mask it.
Exit code 0 means both steps passed; any non-zero exit names the step that
failed. The full command-line contract is in
`specs/001-arch-module-boundary/contracts/verify-entry.md`.

The script and the configuration are the same ones CI runs — the two commands
are byte-identical to the `Loretide content contracts` workflow steps, so
Windows PowerShell, a Linux shell and CI all exercise one implementation. If
either side changes, change the other in the same PR. Windows path separators
are normalised inside the checker (see *Windows path normalization* below), so
results do not differ by platform.

Node 22 or newer is required (`package.json` `engines.node` is `>=22`); CI
pins Node 22. `scripts/check.sh` runs this entry after `pnpm typecheck`, so
`make check` covers it too.

## What it enforces

Given the roots and module graph in `content-boundaries.json`:

- **Layering.** `packages/views/content` may depend on `packages/core/content`,
  never the reverse; the three roots (`server`, `core`, `views`) do not import
  across each other except that one allowed `views -> core` direction.
- **Declared dependencies.** A module may import another module only if the
  target is listed in that module's dependency array.
- **Public interface.** Cross-module imports must target a module's `index`
  entry; importing a private (non-index) path is rejected.
- **Adapters.** Only files listed in `adapters` may import content from outside
  the content roots, and only a module's public `index`.
- **Upstream allow-list.** Non-content imports must be Go standard library or an
  entry in `upstreamImports` for that root.
- **Platform isolation.** `packages/core/content` may not touch `localStorage`
  or `process.env`.
- **No cycles.** The cross-module import graph must be acyclic.
- **No uncomputable module loads.** A dynamic `import()`/`require()` whose target
  cannot be resolved statically is rejected rather than ignored.

## Accuracy hardening (Issue #7)

The checker previously used regular expressions over raw source. That produced
both false negatives (missed violations) and false positives (flagged
non-imports). The following are now handled precisely, each with a regression
test in `scripts/check-content-boundaries.test.mjs`:

### Go imports — lexer instead of regex

`extractGoImports()` is a small lexer that skips comments and string, raw-string
and rune literals, then reads the `import` keyword only in code context. This
fixes:

- **Comment/string false positives.** `import "..."` inside a `//` or `/* */`
  comment, an interpreted string, or a raw (backtick) string is no longer read
  as an import.
- **Group truncation.** A `)` inside a comment within an `import ( ... )` group
  no longer ends the group early and hides the imports after it.

Grouped, aliased (`m "path"`), blank (`_ "path"`) and dot (`. "path"`) imports
are all extracted; standard-library entries are ignored by the module rules.
Import aliases may be Unicode identifiers (e.g. `别名 "path"`), and are not
skipped. Import-path string literals are decoded according to Go string escape
rules (`\x`, `\u`, `\U`, octal, and the simple `\n`/`\t`/… escapes), so an
obfuscated path such as `"github\x2ecom/..."` resolves to its real value
(`github.com/...`) and is classified correctly instead of being mistaken for a
dotless standard-library path.

### TypeScript / JavaScript

- **Import extraction** uses the TypeScript scanner (`ts.preProcessFile`), which
  ignores comments and strings and covers static `import`, `import type`,
  re-exports (`export ... from`), and string-literal `import()`/`require()`.
  Type-only and re-export edges are checked with the same layering and
  dependency rules as value imports — they are architectural dependencies.
- **Uncomputable module loads** are detected on the AST (`hasComputedModuleLoad`)
  rather than by a raw-source regex. A dynamic `import()`/`require()` whose
  argument is not a plain string literal — an identifier, a concatenation such as
  `import("a" + "b")`, a template literal, or a missing argument — is rejected.
  Because detection is AST-based, `import(...)`/`require(...)` text inside
  comments or strings no longer triggers a false positive.

### Windows path normalization

`check()` normalizes backslash separators in input file paths before
classifying them. Previously a Windows-style path (`packages\core\content\...`)
failed root matching, so the file was treated as external: real violations were
misreported as rogue-adapter errors and legitimate imports raised false
positives. Import specifiers themselves always use `/` and are unaffected.

### Cycles

Cross-module import cycles are reported even when the configuration would permit
each individual edge, including cycles spanning three or more modules.

## Limits

This is static analysis of import specifiers. It does **not** guarantee runtime
behavior: it does not resolve values passed to a computed `import()`/`require()`
(those are rejected, not evaluated), does not follow build-time codegen or
path aliases beyond the prefix rewrites in `check()`, and does not model
conditional or platform-specific bundling. It validates the declared module
graph and the shape of import statements, nothing more.

## Acceptance mapping (ARCH-01/02)

Evidence for `tasks/architecture.md` ARCH-01 and ARCH-02, produced by
`specs/001-arch-module-boundary`. The card text itself lives in the
documentation repository (branch `main`) and is not duplicated here; the rows
below follow the deliverable / acceptance / verification triad as restated in
`specs/001-arch-module-boundary/spec.md` → *Current State*. Status writeback on
the cards is the main task's, not this repository's — a row here is evidence,
not acceptance.

Commands were run on Linux, Node v22.22.2, pnpm 10.28.2, at repository commit
`d7211755a` plus this change. Each is independently re-runnable.

| Card item | Met | Evidence (command + exit code + file or test case) | Notes |
| --- | --- | --- | --- |
| ARCH-01 deliverable — an executable module dependency manifest | Yes | `scripts/content-boundaries.json` (`version: 1`) declares 3 content roots, 12 modules with their allowed dependency arrays, per-root `upstreamImports`, and 5 `adapters` files. Loaded and asserted against by every case in `scripts/check-content-boundaries.test.mjs`. | Ownership of database tables is **not** part of this manifest; see the storage-ownership row below. |
| ARCH-01 acceptance — the module set matches docs/12 §2 | Yes | `node -e` over `scripts/content-boundaries.json` → 12 modules: `agent-gateway`, `agent-workflow`, `diagnostics`, `feedback-learning`, `ip-profile`, `knowledge-base`, `project-collab`, `review-delivery`, `source-inbox`, `topic-planning`, `work-editor`, `workspace-core`. That is 11 product modules + `diagnostics`, exit 0. | docs/12 is in the documentation repository and not readable from this checkout, so the comparison made here is against `spec.md` → *Current State*, which restates docs/12 §2 as "11 modules + `diagnostics`". Difference count against that restatement: **0**. Name-by-name verified by the main task against docs/12 §2 on 2026-09-14 (11 + `diagnostics`, difference 0): `workspace-core`, `ip-profile`, `source-inbox`, `knowledge-base`, `topic-planning`, `work-editor`, `agent-workflow`, `review-delivery`, `feedback-learning`, `project-collab` and `agent-gateway` all appear in `content-boundaries.json`, which adds only `diagnostics`. |
| ARCH-01 verification — the manifest is machine-checked, not prose | Yes | `pnpm check:content-boundaries` → exit 0, `Content boundaries passed (3523 files; 12 registered modules)`. Mutating the manifest breaks the build: cases `cycles fail even if configuration permits both edges` and `three-module import cycle is reported` add an edge to a cloned config and assert a `cycle:` error. | — |
| ARCH-02 deliverable — a static boundary checker with negative coverage | Yes | `scripts/check-content-boundaries.mjs`; `node --test scripts/check-content-boundaries.test.mjs` → 13 pass, 0 fail, exit 0. Negative cases cover cross-module private paths, reverse layering, `electron`, `next/navigation`, unregistered modules, 2- and 3-module cycles, computed `import()`, and Windows-style paths (case `Windows backslash input paths are normalised before classification`). | Go private entries are covered: `check()` returns `private import …` for a cross-module non-`index` Go path in single, grouped, aliased, blank (`_`) and dot (`.`) import forms (verified by running `check()` directly; the single-line form is asserted in case `public dependencies pass; private, reverse, platform and unregistered imports fail`). The grouped forms have no dedicated assertion of their own — behaviour is correct, the assertion is implied. Optional follow-up, not a gap in enforcement. |
| ARCH-02 acceptance — the checker runs in CI | Yes, for `app-main` | `.github/workflows/loretide-content.yml` lines 20–21 run the two commands on `push` and `pull_request` to `app-main`. | The workflow does **not** run for this feature's PR, which targets `chore/spec-kit-baseline`. FR-007 forbids changing the trigger, so there is no CI run link for this change: recorded as **not run**, not as passed. Byte-equality of the local entry with the two workflow steps was checked instead (`node -e` comparison → `identical: true`). |
| ARCH-02 verification — the checker is wired into local development verification | Yes | `package.json` → `check:content-boundaries`; `CLAUDE.md` → Verification → Useful checks; `scripts/check.sh` line 89 (so `make check` covers it). Timed run: `pnpm check:content-boundaries` → exit 0 in ~2.2 s, well inside the 60 s budget. Failure paths proved: appending `import "electron";` to `packages/core/content/diagnostics/index.ts` → exit 1 with `packages/core/content/diagnostics/index.ts: unapproved upstream import electron`, exit 0 again after reverting; a deliberately failing self-test case → exit 1 with the scan summary never printed, so a passing scan cannot mask a failing self-test. | — |

### What this evidence does not cover

The checker is **static analysis of import specifiers**. Reading these rows as
broader assurance than that would be wrong:

- **No runtime guarantee.** It validates the declared module graph and the shape
  of import statements. It does not execute code, resolve computed
  `import()`/`require()` arguments (those are rejected, not evaluated), follow
  build-time codegen or path aliases beyond the prefix rewrites in `check()`,
  or model conditional or platform-specific bundling.
- **No SQL-layer ownership.** Nothing here checks that a migration or sqlc query
  touches only tables owned by the module changing them; the checker cannot see
  SQL. That rule is carried by the storage-ownership item in
  `.github/PULL_REQUEST_TEMPLATE.md` and by main-task review. A table → module
  map is not defined in this repository, and a static SQL ownership scan was
  deliberately not added (clarification 2026-09-14: false-positive rate too
  high).
- **No coverage outside the content roots.** Only `server/internal/content`,
  `packages/core/content` and `packages/views/content` are scanned. The rest of
  `packages/core`, `packages/ui` and `packages/views` is out of scope for this
  checker; the repository-wide package boundaries in `CLAUDE.md` → *Package
  Boundaries* are enforced by other means.
- **A tick is not acceptance.** These rows record what was run and what it
  returned. Card status in the documentation repository is written back by the
  main task.
