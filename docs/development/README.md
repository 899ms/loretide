# Loretide local development

Read [AI collaboration](ai-collaboration.md) before claiming tasks and [Multica design requirements](design/README.md) before UI changes.

The active environment is native Windows on the current computer. API, Web, restored PostgreSQL, development login and the diagnostics overview have been verified locally. See [Windows startup, paths and limitations](native-windows.md). Full business acceptance, real executor acceptance and Windows reboot auto-start remain incomplete. Historical native-linux and reliability reports describe the previous VPS; they do not establish local readiness. The paused Sol task must not be resumed automatically.

Use one GitHub Issue per executor and branch/worktree, separate test databases and ports, and Draft PR delivery to the main reviewer. Application source currently exists in this independent checkout, not in the parent documentation repository on GitHub. A code Issue cannot be ready without an accessible application baseline.

## Testing policy: do not write or run UI unit tests (local or CI); manual UI acceptance

UI unit tests are **not written and not run — neither locally nor in CI** — and
are **not** acceptance evidence. UI behavior is also **not** verified with
computer-use / automated clicking. UI changes are accepted by the **user**,
manually, against the affected UI surfaces and a manual Todo (below), following
the [Multica design requirements](design/README.md).

### Classify a test by behavior, not by file extension

A `.test.ts` / `.test.tsx` name does not decide anything; what the test *does*
decides:

- **UI test — do not write or run it (local or CI).** Renders a component,
  mounts the DOM, or drives a React hook (`@testing-library/react`, `renderHook`,
  `document`, `window`, `jsdom` environment, DOM APIs such as
  `URL.createObjectURL` / `HTMLElement.click`).
- **Non-UI test — run in CI.** Pure functions, schema/contract parsing, state
  transitions, locale parity, backend Go tests. These have no DOM and typically
  run under `// @vitest-environment node`.

The Loretide content-contracts workflow
(`.github/workflows/loretide-content.yml`) runs only the non-UI checks. The two
UI suites it used to run — `content/diagnostics/queries.test.tsx` (a React hook)
and `platform/content-diagnostics.test.ts` (DOM download) — have been removed
from CI execution. **Their source files are kept** (not deleted), but they are
not run — not in CI and not locally as part of verification. Their absence from a
CI run is this policy in effect, **not a pass**: no automated coverage runs for
those behaviors anywhere. When adding tests, keep pure logic in a node-env
`.test.ts` so it can run in CI; do not write UI unit tests or automated
click-through acceptance.

### Manual UI verification Todo (template)

Any change that affects the UI must list the affected surfaces and a manual Todo,
and stays **pending acceptance until the user confirms**. Reuse this template in
the PR / delivery record:

```markdown
### UI impact
- Affected surfaces: <route / page / component, e.g. /:slug/diagnostics — overview panel>
- Nature of change: <visual / interaction / copy / none>

### Manual UI Todo (pending user confirmation)
- [ ] Reuse Multica components, layout, typography and design tokens (no hand-built controls)
- [ ] Verify in a real browser at the affected route(s): <what to look at>
- [ ] Light and dark theme both correct
- [ ] Loading / empty / error states render
- [ ] Long text / overflow / narrow width handled
- [ ] Keyboard focus and active/hover states remain distinguishable
- Status: PENDING — awaiting user acceptance (do not mark done from CI or self-review)
```

A change with no UI effect states `UI impact: none` and `Manual UI Todo: none`.
This document and the CI change themselves have no UI impact.
