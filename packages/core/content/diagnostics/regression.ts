// Derives the regression verdict and the four locators from a run.
//
// The backend's Evaluate() has two states (passed/failed) and initialises every
// run to the literal "not_run" (simulator.go:15). The panel used to render that
// raw token straight into a table cell, so a run that was never evaluated looked
// like just another value in the verdict column. These functions turn the raw
// field into an explicit state that can never be misread as a pass.
//
// Contract and truth table:
// specs/006-diag-trace-waterfall-regression/contracts/regression-verdict.md
import type {DiagnosticRun} from "./contract";

export type RegressionVerdictState = "passed" | "failed" | "not_run" | "undecidable";

export type RegressionVerdict = {
  verdict: RegressionVerdictState;
  /** The untouched backend value, so "undecidable" can be rechecked in place. */
  rawRegression: string;
  expectedCode: string;
  actualCode: string;
  /** Both codes present, so a pass can be justified rather than asserted. */
  basisAvailable: boolean;
};

export type LocatorState = "present" | "missing";
export type OriginalRunState = "linkable" | "self" | "unreadable" | "missing";

export type RunLinkage = {
  originalRun: {runId: string; state: OriginalRunState};
  scenario: {value: string; state: LocatorState};
  module: {value: string; state: LocatorState};
  build: {value: string; state: LocatorState};
};

/**
 * The backend writes "not_run" at creation and "" is the zero value from a
 * payload written outside that path. Both mean the same thing to a reader.
 */
const NEVER_EVALUATED = new Set(["", "not_run"]);

/**
 * status currently takes only "completed" and "failed" (simulator.go:15, :44).
 * Treating anything else as unfinished means a future "running" lands on
 * "undecidable" by itself, which is the safe direction.
 */
const FINISHED = new Set(["completed", "failed"]);

/** Run ids come from NewID(): 16 random bytes hex-encoded (contract.go:56). */
const RUN_ID_PATTERN = /^[0-9a-f]{32}$/;

export function describeRegressionVerdict(run: DiagnosticRun): RegressionVerdict {
  const raw = run.regression ?? "";
  const finished = FINISHED.has(run.status ?? "");

  let verdict: RegressionVerdictState;
  if (NEVER_EVALUATED.has(raw)) {
    verdict = "not_run";
  } else if (raw === "failed") {
    verdict = "failed";
  } else if (raw === "passed" && finished) {
    verdict = "passed";
  } else {
    // Unknown enum values and a pass that contradicts its own status both land
    // here. They mean the same thing to the reader - do not trust this, look at
    // the raw values - so a fifth state would add a label nobody could act on.
    verdict = "undecidable";
  }

  const expectedCode = run.expectedCode ?? "";
  const actualCode = run.actualCode ?? "";
  return {
    verdict,
    rawRegression: raw,
    expectedCode,
    actualCode,
    basisAvailable: expectedCode !== "" && actualCode !== "",
  };
}

function locator(value: string | undefined): {value: string; state: LocatorState} {
  const text = value ?? "";
  return {value: text, state: text === "" ? "missing" : "present"};
}

export function describeRunLinkage(
  run: DiagnosticRun,
  readableRunIds: ReadonlySet<string>,
): RunLinkage {
  const originalRunId = run.originalRunId ?? "";
  let state: OriginalRunState;
  if (originalRunId === "") {
    // Not a broken link: this run is the original fault. Saying "not recorded"
    // here would send the reader looking for something that never existed.
    state = "self";
  } else if (!RUN_ID_PATTERN.test(originalRunId)) {
    state = "missing";
  } else if (readableRunIds.has(originalRunId)) {
    state = "linkable";
  } else {
    // Well-formed but outside what this viewer can read: retention or scope.
    state = "unreadable";
  }

  return {
    originalRun: {runId: originalRunId, state},
    scenario: locator(run.scenario),
    module: locator(run.module),
    build: locator(run.build),
  };
}
