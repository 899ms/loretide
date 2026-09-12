# Loretide application source baseline

Initialized on 2026-09-13 for LT-001.

- Application checkout: `F:/GJ/内容创作工作台/app`.
- Source: https://github.com/multica-ai/multica
- Starting commit: `3551e72e76d2c276e550b668303646d1280fb1e2`.
- Local branch: `main`.
- Cloned from the existing research checkout using `git clone --no-local --no-checkout`; objects are independently transferred, with no alternates or shared object dependency.
- This is a shallow checkout: the pinned commit is retained, but earlier history is not downloaded. Fetch older history when needed with `git fetch --unshallow upstream`; this does not merge upstream changes.
- `upstream` fetch URL points to Multica; its push URL is `DISABLED`. Local `push.default=nothing`; no tracking branch or application `origin` is configured.
- The documentation repository remains `899ms/loretide`. Its `/app/` ignore rule excludes this separate checkout; it is not a submodule. No changes were sent to the existing `899ms/multica` repository.
- Upstream LICENSE and source files are preserved unchanged. The Multica License contains additional conditions; see the existing source assessment before distribution.

## Development authority

Product scope is defined by the parent repository's `docs/11-首版开发基线与实施清单.md`, module boundaries by document 12, and full diagnostics by document 13. Tasks are tracked in the parent's `tasks/` directory. Read this checkout's AGENTS.md and CLAUDE.md for implementation conventions.

Browser-first testing, mandatory human final approval, manual publishing, and removal of OpenClaw from the delivered application remain requirements. Initialization does not implement these requirements: upstream runtime paths are still present and must not be treated as security-verified.

## Verification and next step

Before this documentation addition, the working tree matched the pinned source commit. `git fsck --full` passed; parent Git ignores the application and contains no application gitlink. No dependencies, database, server, desktop window or model process was started. Continue with LT-002: inspect Windows tools and database options before choosing instance configuration.
