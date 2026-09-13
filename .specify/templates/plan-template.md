# Implementation Plan: [FEATURE]

**Branch**: `[###-feature-name]` | **Date**: [DATE] | **Spec**: [link]

**Input**: Feature specification from `/specs/[###-feature-name]/spec.md`

**Note**: This template is filled in by the `/speckit-plan` command; its definition describes the execution workflow.

## Summary

[Extract from feature spec: primary requirement + technical approach from research]

## Technical Context

<!--
  ACTION REQUIRED: Replace the content in this section with the technical details
  for the project. The structure here is presented in advisory capacity to guide
  the iteration process.
-->

**Language/Version**: [e.g., Python 3.11, Swift 5.9, Rust 1.75 or NEEDS CLARIFICATION]

**Primary Dependencies**: [e.g., FastAPI, UIKit, LLVM or NEEDS CLARIFICATION]

**Storage**: [if applicable, e.g., PostgreSQL, CoreData, files or N/A]

**Testing**: [e.g., pytest, XCTest, cargo test or NEEDS CLARIFICATION]

**Target Platform**: [e.g., Linux server, iOS 15+, WASM or NEEDS CLARIFICATION]

**Project Type**: Existing monorepo (Go backend + Next.js web + Electron desktop + Expo mobile + shared packages). Do not re-derive this.

**Performance Goals**: [domain-specific, e.g., 1000 req/s, 10k lines/sec, 60 fps or NEEDS CLARIFICATION]

**Constraints**: [domain-specific, e.g., <200ms p95, <100MB memory, offline-capable or NEEDS CLARIFICATION]

**Scale/Scope**: [domain-specific, e.g., 10k users, 1M LOC, 50 screens or NEEDS CLARIFICATION]

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

[Gates determined based on constitution file]

## Project Structure

### Documentation (this feature)

```text
specs/[###-feature]/
├── plan.md              # This file (/speckit-plan command output)
├── research.md          # Phase 0 output (/speckit-plan command)
├── data-model.md        # Phase 1 output (/speckit-plan command)
├── quickstart.md        # Phase 1 output (/speckit-plan command)
├── contracts/           # Phase 1 output (/speckit-plan command)
└── tasks.md             # Phase 2 output (/speckit-tasks command - NOT created by /speckit-plan)
```

### Source Code (repository root)
<!--
  ACTION REQUIRED: Loretide is an existing monorepo. Do NOT invent a new layout.
  List only the directories and files this feature actually touches, drawn from
  the real tree below. Delete the rest.
-->

```text
server/                       # Go backend: Chi router, sqlc, gorilla/websocket
├── internal/handler/         # HTTP handlers; resolve path UUIDs through loaders
├── internal/service/         # business services (incl. builtin_skills/)
├── internal/testutil/        # dbfx fixtures, testutil.Call - use these, do not open-code
└── migrations/               # one statement per file; CREATE INDEX CONCURRENTLY

apps/
├── web/                      # Next.js App Router
│   └── platform/             # the ONLY place for Next.js navigation/platform APIs
├── desktop/                  # Electron; renderer platform wiring under src/renderer/src/platform/
├── mobile/                   # Expo / React Native - independent; read apps/mobile/CLAUDE.md first
└── docs/                     # Fumadocs site

packages/
├── core/                     # headless logic, API client, React Query hooks, Zustand stores
├── ui/                       # atomic components only; no @multica/core imports
├── views/                    # shared business pages/views; no next/*, no react-router-dom
└── tsconfig/ eslint-config/

e2e/                          # Playwright end-to-end specs
```

Hard constraints the plan must respect (from `CLAUDE.md`):

- Dependency direction is `views -> core + ui`. `core` and `ui` stay independent.
- `packages/core/` must not use `react-dom`, `localStorage` (use `StorageAdapter`), `process.env`, or UI libraries.
- TanStack Query owns server state; Zustand owns client/view state. Workspace-scoped query keys include `wsId`.
- No database foreign keys or cascades; resolve relationships in application code.
- Parse API JSON with `parseWithFallback` + a zod schema. Never cast network JSON to `T`.
- There is no `src/`, `backend/`, or `frontend/` directory. Never create one.

**Structure Decision**: [Document the selected structure and reference the real
directories captured above]

## Complexity Tracking

> **Fill ONLY if Constitution Check has violations that must be justified**

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| [e.g., 4th project] | [current need] | [why 3 projects insufficient] |
| [e.g., Repository pattern] | [specific problem] | [why direct DB access insufficient] |
