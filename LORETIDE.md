# Loretide application development baseline

This is the application branch of the private [899ms/loretide](https://github.com/899ms/loretide) repository. Use **app-main** for application PRs; **main** contains the product documentation and task ledger. Never merge these unrelated histories or target a code PR at main.

```sh
git clone --branch app-main --single-branch https://github.com/899ms/loretide.git loretide-app
```

Read [collaboration rules](docs/development/ai-collaboration.md), [design rules](docs/development/design/README.md), CLAUDE.md, and the documents linked from the main branch. Each Issue supplies a pinned source commit. Use an independent worktree, database and ports.

The baseline contains diagnostics, module-boundary checks and local environment scripts. It is a development checkpoint, not a production release or complete product acceptance. Real agent execution is unconditionally disabled until a separately reviewed implementation changes that policy. Do not remove this gate to make upstream execution tests pass.

Local operator instructions: [native Windows](docs/development/native-windows.md). The current launcher depends on private, pre-provisioned runtime configuration; a fresh clone is not yet a one-command runnable environment. A separate bootstrap task must close that gap using synthetic data and new local secrets.

Known limits: Redis/external-service tests are skipped locally; the full diagnostics browser matrix and account-domain integration remain incomplete. Some upstream provider code and workflows remain in the fork but are not an approved Loretide runtime. No public deployment, model credentials or existing development database are included.

Upstream copyright, UI attribution and the complete [Multica License](LICENSE) are retained. This file does not replace or relicense the upstream terms.
