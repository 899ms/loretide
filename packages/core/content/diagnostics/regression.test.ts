// @vitest-environment node
// Canonical layer for the regression verdict and run linkage.
// FR map (specs/006-diag-trace-waterfall-regression/spec.md):
//   FR-005 four explicit states; "not run" and "undecidable" never read as passed
//   FR-006 a pass carries its basis (expected vs actual) so it can be rechecked
//   FR-007 four locators, each with its own availability state
//   FR-012 linkage takes the readable set from the caller and issues no request
// Truth table: contracts/regression-verdict.md
import {describe, it, expect} from "vitest";
import {describeRegressionVerdict, describeRunLinkage} from "./regression";
import type {DiagnosticRun} from "./contract";

const RUN_ID = "0123456789abcdef0123456789abcdef";
const ORIGINAL_ID = "fedcba9876543210fedcba9876543210";

function run(over: Partial<DiagnosticRun> = {}): DiagnosticRun {
  return {
    runId: RUN_ID, workspaceId: "w", accountId: "", actorId: "a",
    scenario: "timeout", seed: 42, createdAt: "2026-01-01T00:00:00.000Z",
    snapshot: {} as DiagnosticRun["snapshot"], events: [], reproductionGaps: [],
    status: "completed", expectedCode: "TIMEOUT", actualCode: "TIMEOUT",
    regression: "passed", module: "diagnostics", build: "abc1234",
    originalRunId: "", isTest: true,
    ...over,
  };
}

describe("describeRegressionVerdict truth table", () => {
  // The backend's own literal for "never evaluated" (simulator.go:15). Missing
  // this row is what would send a never-evaluated run to "undecidable" and
  // defeat the whole user story.
  it('reads the backend initial value "not_run" as not run', () => {
    expect(describeRegressionVerdict(run({regression: "not_run"})).verdict).toBe("not_run");
  });

  // Zero value, only reachable from a payload written outside this code path.
  it("reads an empty string as not run", () => {
    expect(describeRegressionVerdict(run({regression: ""})).verdict).toBe("not_run");
  });

  it("reads passed on a completed run as passed", () => {
    expect(describeRegressionVerdict(run({regression: "passed", status: "completed"})).verdict).toBe("passed");
  });

  it("reads passed on an unfinished run as undecidable", () => {
    const verdict = describeRegressionVerdict(run({regression: "passed", status: "running"}));
    expect(verdict.verdict).toBe("undecidable");
    expect(verdict.rawRegression).toBe("passed");
  });

  it("reads failed as failed regardless of status", () => {
    for (const status of ["completed", "failed", "running"]) {
      expect(describeRegressionVerdict(run({regression: "failed", status})).verdict).toBe("failed");
    }
  });

  it("reads an unknown value as undecidable and keeps the original", () => {
    const verdict = describeRegressionVerdict(run({regression: "quarantined"}));
    expect(verdict.verdict).toBe("undecidable");
    expect(verdict.rawRegression).toBe("quarantined");
  });
});

describe("describeRegressionVerdict invariants", () => {
  // The one thing this function exists to prevent. Anything uncertain must fall
  // away from "passed", never toward it (D13-V09).
  it("returns passed only for passed on a finished run", () => {
    const regressions = ["", "not_run", "passed", "failed", "quarantined", "PASSED", "pass"];
    const statuses = ["completed", "failed", "running", "", "queued"];
    for (const regression of regressions) {
      for (const status of statuses) {
        const {verdict} = describeRegressionVerdict(run({regression, status}));
        const shouldPass = regression === "passed" && (status === "completed" || status === "failed");
        expect(verdict === "passed", `regression=${regression} status=${status}`).toBe(shouldPass);
      }
    }
  });

  it("keeps the raw value for every input, including empty", () => {
    for (const regression of ["", "not_run", "passed", "failed", "weird"]) {
      expect(describeRegressionVerdict(run({regression})).rawRegression).toBe(regression);
    }
  });

  it("reports whether the basis is available so a pass can be rechecked", () => {
    expect(describeRegressionVerdict(run({expectedCode: "TIMEOUT", actualCode: "TIMEOUT"})).basisAvailable).toBe(true);
    expect(describeRegressionVerdict(run({expectedCode: "", actualCode: ""})).basisAvailable).toBe(false);
  });

  it("is pure: the same run yields an equal result every time", () => {
    const subject = run({regression: "not_run"});
    expect(describeRegressionVerdict(subject)).toEqual(describeRegressionVerdict(subject));
  });
});

describe("describeRunLinkage", () => {
  const readable = new Set([RUN_ID, ORIGINAL_ID]);

  it("marks an empty originalRunId as being the original fault itself", () => {
    expect(describeRunLinkage(run({originalRunId: ""}), readable).originalRun.state).toBe("self");
  });

  it("marks a readable original as linkable", () => {
    const linkage = describeRunLinkage(run({originalRunId: ORIGINAL_ID}), readable);
    expect(linkage.originalRun.state).toBe("linkable");
    expect(linkage.originalRun.runId).toBe(ORIGINAL_ID);
  });

  it("marks a well-formed but unreadable original as unreadable, not missing", () => {
    const linkage = describeRunLinkage(run({originalRunId: ORIGINAL_ID}), new Set([RUN_ID]));
    expect(linkage.originalRun.state).toBe("unreadable");
  });

  it("marks a malformed original id as missing", () => {
    expect(describeRunLinkage(run({originalRunId: "nope"}), readable).originalRun.state).toBe("missing");
    expect(describeRunLinkage(run({originalRunId: "ABCDEF"}), readable).originalRun.state).toBe("missing");
  });

  // "First run" and "broken link" mean opposite things to a reader; conflating
  // them is exactly what FR-007 forbids.
  it("does not conflate self with missing", () => {
    const self = describeRunLinkage(run({originalRunId: ""}), readable);
    const missing = describeRunLinkage(run({originalRunId: "nope"}), readable);
    expect(self.originalRun).not.toEqual(missing.originalRun);
  });

  it("marks empty scenario, module and build as missing without inventing a value", () => {
    const linkage = describeRunLinkage(run({scenario: "", module: "", build: ""}), readable);
    for (const item of [linkage.scenario, linkage.module, linkage.build]) {
      expect(item.state).toBe("missing");
      expect(item.value).toBe("");
    }
  });

  it("passes present locators through unchanged", () => {
    const linkage = describeRunLinkage(run({scenario: "timeout", module: "diagnostics", build: "abc1234"}), readable);
    expect(linkage.scenario).toEqual({value: "timeout", state: "present"});
    expect(linkage.module).toEqual({value: "diagnostics", state: "present"});
    expect(linkage.build).toEqual({value: "abc1234", state: "present"});
  });

  it("does not mutate the readable set it is given", () => {
    const given = new Set([RUN_ID]);
    describeRunLinkage(run({originalRunId: ORIGINAL_ID}), given);
    expect([...given]).toEqual([RUN_ID]);
  });
});
