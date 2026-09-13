# Loretide Constitution

This file is a **gate checklist**, not a second copy of the project rules. The authority for
architecture, naming, state, testing, and product voice is `CLAUDE.md` and the documents it
points to. Every principle below names where the full rule lives; read that source before
deciding a plan satisfies it.

## Core Principles

### I. CLAUDE.md is authoritative

`CLAUDE.md` at the repo root, plus the files it references (`apps/mobile/CLAUDE.md`,
`apps/docs/content/docs/developers/conventions.mdx`, `docs/development/design/README.md`,
`docs/development/ai-collaboration.md`), are the source of truth. This constitution adds no
new rules; it only lists the ones a plan must be checked against. On conflict, `CLAUDE.md`
wins and this file is wrong and must be corrected.

### II. No UI unit tests, no automated UI acceptance (NON-NEGOTIABLE)

Never write or run UI unit tests, locally or in CI. Never reclassify one to evade this.
Never use computer use or automated browser clicking for acceptance. UI verification is a
manual Todo listing screens, steps, and expected results, confirmed by the user. Keep the
non-UI checks the change needs: contract tests, Go tests, module-boundary and permission
checks, `pnpm typecheck`, build. A check not run is recorded as not run, never as passed.

### III. Module boundaries hold

Dependency direction is `views -> core + ui`; `core` and `ui` stay independent.
`packages/core/` uses no `react-dom`, `localStorage` (use `StorageAdapter`), `process.env`,
or UI libraries. `packages/ui/` imports no `@multica/core` and holds no business logic.
`packages/views/` uses no `next/*` and no `react-router-dom`. Platform APIs live only in
`apps/web/platform/` and the desktop renderer's `platform/`. A plan that needs a boundary
relaxed is rejected, not the boundary. See `CLAUDE.md` -> Package Boundaries.

### IV. Server state and client state stay separate

TanStack Query owns anything fetched from the API; Zustand owns filters, drafts, modals,
layout, and navigation history. Workspace-scoped query keys include `wsId`. WebSocket events
invalidate or patch the Query cache and never mirror server payloads into Zustand.
Optimistic updates only when the outcome is locally predictable, the user stays on the
screen, failure is rare, and rollback is trivial. See `CLAUDE.md` -> State Rules.

### V. The database has no foreign keys and no cascades

Resolve relationships, validation, and dependent cleanup in application code, in a
transaction when parent and cleanup must commit together. Every migration index uses
`CREATE INDEX CONCURRENTLY`, one statement per file. Conditionally skipped migrations still
record in `schema_migrations`, so later migrations touching those objects use idempotent
DDL. See `CLAUDE.md` -> Database and Migration Rules.

### VI. API responses are parsed, never cast

Parse API JSON with `parseWithFallback` and a zod schema. Downstream UI optional-chains and
defaults defensively; server booleans are checked with `=== true`; server-driven enum
switches have a `default` branch. New or changed endpoints ship a schema update and a
malformed-response test. Installed desktop clients talk to newer backends - this is why.
See `CLAUDE.md` -> API Compatibility.

### VII. UI reuses Multica, it does not reinvent it

Read `docs/development/design/README.md` before changing any Web page, diagnostics and test
pages included. Reuse Multica components, layouts, typography, and design tokens. Font sizes
come from the role-named `--text-*` scale, not Tailwind's default ramp. Prefer shadcn/Base UI
over custom implementations. No hardcoded colors. See `CLAUDE.md` -> UI Rules.

### VIII. Scope is the claimed task, nothing adjacent

Implement exactly the tasks the current assignment covers. A bug, performance issue, or
cleanup found along the way is reported as a follow-up, not fixed in passing, unless the
required behavior cannot work without it. No compatibility layers, fallback paths, dual
writes, or shims in internal code unless explicitly requested. No broad refactors.
See `CLAUDE.md` -> Coding Rules.

### IX. Real AI executors stay disabled

Keep real agent executors disabled. Do not read, copy, or upload model client credentials.
Default tests must never resolve or execute user-installed agent CLIs; real-agent smoke tests
live behind the `agentintegration` build tag and check `MULTICA_RUN_REAL_AGENT_SMOKE=1`.
See `CLAUDE.md` -> Testing.

### X. A checked box is not acceptance

A ticked task in `tasks.md` means that implementation task was delivered. It does not mean
merged, and it does not mean the user accepted it. GitHub holds claim and review state;
`records/` holds evidence. Do not maintain a second ownership or status ledger inside
`specs/`. See `docs/development/ai-collaboration.md`.

## Stop Conditions

Stop and report rather than proceed when any of these holds:

- This constitution is missing, or any principle above is still an unfilled placeholder.
- The authoritative source a principle names cannot be read.
- The task's allowed file range is unknown, or the work would touch files outside it.
- A plan can only pass a gate by weakening the rule the gate checks.

A soft reminder in a template is not a gate. These stops are mandatory.

## Development Workflow

Specs, plans, and task lists produced by `/speckit-*` are inputs to the existing workflow,
not a replacement for it. Claiming, isolation (one task, one executor, one worktree, its own
database and ports), Draft PR delivery, and merge arbitration follow
`docs/development/ai-collaboration.md`.

Currently open: `/speckit-specify`, `/speckit-clarify`, `/speckit-plan`, `/speckit-tasks`,
`/speckit-analyze`, `/speckit-checklist`.

Currently closed, do not use:

- `/speckit-taskstoissues` - it de-duplicates on local task numbers (`T001`), which collide
  across features. Issue creation stays manual.
- `/speckit-implement` unrestricted - it executes the whole `tasks.md`. Run it only against
  the task IDs and file range of the currently claimed assignment.
- `/speckit-converge` must not widen scope or declare acceptance on its own.

## Governance

This constitution supersedes convenience, not `CLAUDE.md`. Amending it requires the same
review as a code change: a Draft PR, the reason recorded, and the affected templates updated
in the same PR. Every plan fills Constitution Check before Phase 0 research and re-checks it
after Phase 1 design; a violation is recorded in Complexity Tracking with the simpler
alternative that was rejected and why, or the plan changes.

**Version**: 1.0.0 | **Ratified**: 2026-09-14 | **Last Amended**: 2026-09-14
