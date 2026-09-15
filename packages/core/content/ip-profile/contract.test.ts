// @vitest-environment node
import { describe, expect, it } from "vitest";
import { ApiError } from "@multica/core/api";
import {
  NO_REVISION,
  describeAccountError,
  parseAccount,
  parseAccountList,
  parseRevision,
  saveOutcome,
} from "./contract";

function apiError(status: number, body: unknown): ApiError {
  return new ApiError("boom", status, "", body);
}

describe("save outcome", () => {
  // A3. 409 is its own outcome. Folding it into `failed` is the mutation this
  // case exists to catch, and it is an easy fold to make: both are non-2xx.
  it("reads 409 as a retryable conflict, not a failure", () => {
    expect(saveOutcome(apiError(409, { code: "REVISION_CONFLICT" }))).toEqual({
      kind: "conflict",
    });
  });

  // A4. Everything else is a failure, and carries what the server said to do.
  it("reads 400 as a failure and keeps next_action", () => {
    const outcome = saveOutcome(
      apiError(400, { code: "INPUT_CONFLICT", next_action: "fix_input" }),
    );

    expect(outcome.kind).toBe("failed");
    expect(outcome.kind === "failed" && outcome.detail.nextAction).toBe("fix_input");
  });

  it("reads a network error with no response as a failure", () => {
    const outcome = saveOutcome(new Error("offline"));

    expect(outcome.kind).toBe("failed");
    expect(outcome.kind === "failed" && outcome.detail.code).toBe("");
  });

  it("does not mistake a 409-shaped body on another status for a conflict", () => {
    expect(saveOutcome(apiError(500, { code: "REVISION_CONFLICT" })).kind).toBe(
      "failed",
    );
  });
});

describe("diagnostic error objects", () => {
  it("reads code, next_action and trace id", () => {
    expect(
      describeAccountError(
        apiError(400, {
          code: "INPUT_CONFLICT",
          next_action: "fix_input",
          trace_id: "0123456789abcdef0123456789abcdef",
        }),
      ),
    ).toEqual({
      code: "INPUT_CONFLICT",
      nextAction: "fix_input",
      traceId: "0123456789abcdef0123456789abcdef",
    });
  });

  // A5. A malformed error body degrades to blank fields. The UI then shows its
  // generic message - never the string "undefined", and never a crash on a page
  // that is already reporting a problem.
  it("degrades to blank fields when the body is not an object", () => {
    expect(describeAccountError(apiError(500, "<html>502</html>"))).toEqual({
      code: "",
      nextAction: "",
      traceId: "",
    });
  });

  it("degrades to blank fields when the fields are the wrong type", () => {
    expect(
      describeAccountError(apiError(500, { code: 7, next_action: ["retry"] })),
    ).toEqual({ code: "", nextAction: "", traceId: "" });
  });

  it("returns blank fields for a non-ApiError", () => {
    expect(describeAccountError("just a string")).toEqual({
      code: "",
      nextAction: "",
      traceId: "",
    });
  });
});

describe("response parsing", () => {
  it("keeps a platform value it has never heard of", () => {
    // The controlled set governs writes. A backend that added a platform must
    // not blank out an older client's list.
    expect(parseAccount({
      account_id: "a",
      platform: "a_new_platform",
      display_name: "新",
    }).platform).toBe("a_new_platform");
  });

  it("falls back to an empty account when the shape is wrong", () => {
    expect(parseAccount({ account_id: 42 }).account_id).toBe("");
  });

  it("reads a null accounts array as empty", () => {
    expect(parseAccountList({ accounts: null })).toEqual([]);
  });

  it("falls back to an empty list when the body is not an object", () => {
    expect(parseAccountList("nope")).toEqual([]);
  });

  it("reads a blank persona prompt as a real value", () => {
    const revision = parseRevision({
      revision_id: "r1",
      account_id: "a",
      revision: 1,
      persona_prompt: "",
    });

    expect(revision.persona_prompt).toBe("");
    expect(revision.revision).toBe(1);
  });

  it("falls back to no revision when the body is malformed", () => {
    expect(parseRevision({ revision: "one" })).toEqual(NO_REVISION);
  });
});
