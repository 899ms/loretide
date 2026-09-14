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

