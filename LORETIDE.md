# Loretide application development baseline

This is the application branch of the private [899ms/loretide](https://github.com/899ms/loretide) repository. Use **app-main** for application PRs; **main** contains the product documentation and task ledger. Never merge these unrelated histories or target a code PR at main.

```sh
git clone --branch app-main --single-branch https://github.com/899ms/loretide.git loretide-app
```

Read [collaboration rules](docs/development/ai-collaboration.md), [design rules](docs/development/design/README.md), CLAUDE.md, and the documents linked from the main branch. Each Issue supplies a pinned source commit. Use an independent worktree, database and ports.

The baseline contains diagnostics, module-boundary checks and local environment scripts. It is a development checkpoint, not a production release or complete product acceptance. Real agent execution is unconditionally disabled until a separately reviewed implementation changes that policy. Do not remove this gate to make upstream execution tests pass.

**Scope, phase one (decided 2026-09-15): the web app only.** Windows-specific work is parked and is not scheduled: no new native Windows runtime features (`scripts/local-windows.ps1` stays as it is, used only as the local environment for running the app), and no triage of upstream Multica test failures that are specific to Windows. Upstream daemon failures caused by the execution gate are already isolated by [#42](https://github.com/899ms/loretide/pull/42) — an `-overlay` control run showed none of them is a real regression — and need no further work. In scope: `packages/`, `apps/web/` and `server/internal/content/`, plus their diagnostics acceptance. See `tasks/plan.md` on the main branch for the record.

**UI during the SOP phase (2026-09-15):** no UI/UX polish. Inherit Multica's design system and tokens wholesale and only wire features onto existing compositions — see [design rules](docs/development/design/README.md), "SOP phase rule".

Local operator instructions: [native Windows](docs/development/native-windows.md). The current launcher depends on private, pre-provisioned runtime configuration; a fresh clone is not yet a one-command runnable environment. A separate bootstrap task must close that gap using synthetic data and new local secrets.

Known limits: Redis/external-service tests are skipped locally; the full diagnostics browser matrix and account-domain integration remain incomplete. Some upstream provider code and workflows remain in the fork but are not an approved Loretide runtime. No public deployment, model credentials or existing development database are included.

Upstream copyright, UI attribution and the complete [Multica License](LICENSE) are retained. This file does not replace or relicense the upstream terms.
