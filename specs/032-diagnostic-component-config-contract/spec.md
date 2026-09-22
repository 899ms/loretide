# Feature Specification: 诊断组件配置事实与状态合同

**Feature Branch**: `codex/diagnostic-config-contract`
**Created**: 2026-09-22
**Status**: Draft design — implementation requires a separately approved card
**Related issue**: #239

## Problem and decision

The diagnostic overview currently derives several component rows from the absence or presence of a
heartbeat. That collapses three different questions: whether a component is configured, whether its
host has ever supplied a verifiable liveness signal, and whether it is currently usable. This contract
introduces a minimal configuration-fact input and a deterministic reduction. It deliberately does not
make the diagnostics service read ambient configuration itself.

**Decision:** a host that owns configuration publishes a redacted `ComponentConfigFact`; the
diagnostics service separately receives liveness observations; a pure reducer produces the response.
An absent fact is `unknown`, never `unconfigured`.

## Scope and non-goals

- Covers overview components `api`, `database`, `web`, `files`, `daemon`, `executor`, and `search`.
- Defines source ownership, state precedence, compatibility, simulator isolation, concurrency, and
  non-UI verification cases.
- Does not implement the contract, change runtime policy, enable execution, read environment values,
  probe a service, persist secrets, or add a UI test.
- Does not reinterpret existing daemon runtime liveness as diagnostics configuration.

## Contract

### 1. Inputs are separate and redacted

The future server-side package owns these closed enums (wire strings are additive and lower case):

```text
Component = api | database | web | files | daemon | executor | search
ConfigState = configured | unconfigured | unknown
HealthState = healthy | unverified | stale | unavailable | unknown
ExecutionState = enabled | disabled | not_applicable | unknown
ConfigSource = server_boot | router_storage | web_capability | runtime_registry |
               execution_policy | search_host
```

`ComponentConfigFact` contains only `component`, `config_state`, `source`, `observed_at`, an optional
safe `version`, and an optional fixed `reason_code`. It has no raw env value, URL, host, path, bucket,
credential, request body, or runtime identity. `LivenessFact` independently contains component,
health evidence, observation time, and safe version/reason. `ExecutionState` exists so a disabled real
executor cannot be mislabeled as missing configuration.

The host gets a narrow injection interface, conceptually:

```text
RegisterComponentConfig(snapshot []ComponentConfigFact) error
RecordLiveness(fact LivenessFact)
```

`RegisterComponentConfig` replaces one validated snapshot atomically; it is not a public HTTP endpoint
and it is not callable by the simulator. The diagnostics package validates component/source ownership,
normalizes tokens, rejects an older generation, and copies the snapshot before returning an overview.
The host owns the truth; the service owns validation, storage and reduction.

### 2. Source ownership and registration points

| Component | Valid fact source | Registration point | Explicit `unconfigured` is allowed when |
|---|---|---|---|
| api | `server_boot` | `cmd/server/main.go`, after the app listener is successfully bound or an equivalent confirmed server-ready lifecycle event | the owning host has an explicit disabled/no-listener mode; a failure before serving remains `unknown` to an unavailable overview |
| database | `server_boot` | `main.go`, from successfully constructed database configuration/pool, without exposing the DSN | the host has a defined optional-database mode and explicitly selects it; current boot failure is not that mode |
| files | `router_storage` | `NewRouterWithOptions`, immediately after S3/local selection | both supported constructors explicitly yield no store |
| web | `web_capability` | a future authenticated capability registration owned by the deployed web host | the web host explicitly registers that diagnostics capability is omitted; no browser heartbeat is sufficient |
| daemon | `runtime_registry` | a future adapter over explicit runtime-registration configuration, with documented workspace/aggregation scope | the registry supplies an explicit empty/disabled declaration for that scope |
| executor | `execution_policy` | a future policy adapter after review | only when a real executor configuration owner explicitly says none is configured; policy-disabled is **not** this state |
| search | `search_host` | a future actual search host | only when that host explicitly declares its search integration absent |

The current `handler.ContentDiagnosticClient` web heartbeat and the `agent_runtime` HTTP/WebSocket
heartbeats remain liveness-only. A heartbeat must neither create a config fact nor convert `unknown` to
`configured`.

### 3. Reducer and precedence

The response keeps a component row but adds optional additive fields `config_state`, `health_state`,
`execution_state`, `config_observed_at`, and `config_reason`. The legacy `status` is a presentation
projection. The reducer has the following precedence:

| Condition, in order | config_state | health_state | legacy/presentation status | Required explanation |
|---|---|---|---|---|
| executor policy explicitly disabled | configured or unknown | any | `disabled` | `execution_disabled`; never call this unconfigured |
| explicit config is unconfigured | unconfigured | retained but not promotable | `unconfigured` | heartbeat is inconsistent and cannot turn row healthy |
| config is unknown | unknown | retain evidence if any | `unknown` | missing fact is not a negative assertion |
| configured and no liveness evidence | configured | unverified | `unverified` | configured, not yet verified |
| configured and fresh positive liveness | configured | healthy | `healthy` | observed liveness timestamp |
| configured and liveness expires | configured | stale | `unavailable` | `heartbeat_expired` |
| configured and current probe fails | configured | unavailable | `unavailable` | fixed probe failure code |
| invalid timestamp / clock skew | retained | unknown | `unknown` | `clock_skew` |

Thus an unexpected heartbeat plus explicit `unconfigured` is visible as an inconsistency but can never
make the component healthy. An old heartbeat is retained as evidence, not silently erased. Per-component
freshness comes from the liveness producer contract; it is not inferred from a config timestamp.

### 4. Existing data and old clients

- A process or persisted record with no `ComponentConfigFact` maps to `config_state=unknown`; it must
  not be upgraded from previous `unverified` to `unconfigured`.
- New server fields are additive. New web parsers default every missing field from old backends to
  `unknown` / `not_applicable`, preserving the existing `status` parser and the rest of the overview.
- Old clients ignore the additional JSON fields. A mixed deployment therefore stays conservative instead
  of manufacturing green or unconfigured rows.
- No migration or historical backfill belongs to this design. A later persistence decision must preserve
  source, generation and observation time, not reconstruct truth from old heartbeats.

### 5. Simulator and concurrency invariants

- Simulation may create a run and liveness-shaped test data only in its existing isolated test path; it
  cannot call `RegisterComponentConfig`, modify a production snapshot, or change execution policy.
- The production service stores facts and liveness under a mutex (or immutable atomic snapshot). Overview
  obtains a copied snapshot, then evaluates probes without holding the mutation lock. Reload publishes a
  whole generation; concurrent readers never observe a half-updated component set.
- Only validated host wiring obtains the registrar. HTTP callers, browser clients, daemon packets and
  background simulator code do not obtain it. A rejected source/component pair leaves the prior snapshot
  intact and emits only safe operational telemetry.

## Required non-UI verification after implementation

These are pure reducer/contract tests only; no DB, server, browser, real executor, local configuration
or network is needed.

1. Missing configuration + no heartbeat => `unknown`, not `unconfigured`.
2. Configured + no heartbeat => `unverified`.
3. Configured + fresh heartbeat => `healthy`; expiry => `unavailable/heartbeat_expired`.
4. Explicit unconfigured + fresh heartbeat stays `unconfigured` and records inconsistency safely.
5. Disabled executor stays `disabled` whether a config fact or synthetic heartbeat exists.
6. Unknown configuration + heartbeat stays presentation `unknown`, while preserving health evidence.
7. Old overview payload lacking the additive fields parses to safe defaults without losing components.
8. A simulated run cannot mutate the production config snapshot; concurrent snapshot replacement is
   race-safe and readers see either complete generation.

## UI impact and manual acceptance TODO

Affected future flow: `packages/views/content/diagnostics/index.tsx` overview card and the core
diagnostics response parser. This design changes no UI today. When an implementation card lands, a human
should manually verify the overview distinguishes: unknown configuration, configured-unverified, healthy,
expired/unavailable, and executor-disabled; and that it never shows a missing web/daemon/search heartbeat
as "unconfigured". Visual/UI acceptance is pending user validation; no computer-use or UI unit test is
authorized for this card.

## Implementation cards and rollback

See the linked design record for the two bounded cards. Each card is independently revertible by reverting
its own commit: no database migration, no config mutation, and no backwards-incompatible response removal
are permitted. This design itself is rolled back by reverting the documentation commit.
