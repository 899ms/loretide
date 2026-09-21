// @vitest-environment node

import { describe, expect, it, vi } from "vitest";

import {
  createImportSession,
  deriveHistoricalImportTitle,
  editableHistoricalImportFields,
  runHistoricalImport,
  validateHistoricalImportDraft,
  type HistoricalImportDraft,
  type HistoricalImportOperations,
} from "./workflow";

const draft: HistoricalImportDraft = {
  title: "An older post",
  body: "The body copied from the published post.",
  channel: "xiaohongshu",
  publishedAt: "2024-01-02T03:04:00.000Z",
  platformAccount: "account-name",
  pageUrlOrContentId: "https://example.com/post/1",
};

function operations(
  overrides: Partial<HistoricalImportOperations> = {},
): HistoricalImportOperations {
  return {
    createWork: vi.fn(async () => "work-1"),
    createArtifact: vi.fn(async () => "artifact-1"),
    importVersion: vi.fn(async () => "version-1"),
    createPublication: vi.fn(async () => "publication-1"),
    ...overrides,
  };
}

describe("historical import workflow", () => {
  it("stops at the failed step and never starts a later step", async () => {
    const calls: string[] = [];
    const result = await runHistoricalImport(
      draft,
      createImportSession("session-1"),
      operations({
        createWork: async () => {
          calls.push("work");
          return "work-1";
        },
        createArtifact: async () => {
          calls.push("artifact");
          throw new Error("artifact unavailable");
        },
        importVersion: async () => {
          calls.push("version");
          return "version-1";
        },
        createPublication: async () => {
          calls.push("publication");
          return "publication-1";
        },
      }),
    );

    expect(calls).toEqual(["work", "artifact"]);
    expect(result.steps.map((step) => step.status)).toEqual([
      "completed",
      "failed",
      "not_started",
      "not_started",
    ]);
    expect(result.outputs.workId).toBe("work-1");
    expect(result.failure?.step).toBe("artifact");
  });

  it("reuses stable step keys and skips completed work on retry", async () => {
    const first = createImportSession("session-2");
    expect(first.steps.map((step) => step.idempotencyKey)).toEqual([
      "session-2:work",
      "session-2:artifact",
      "session-2:version",
      "session-2:publication",
    ]);

    const failed = await runHistoricalImport(
      draft,
      first,
      operations({
        createArtifact: async ({ checkpoint }) => {
          checkpoint("artifact-1");
          throw new Error("draft patch failed");
        },
      }),
    );
    const createWork = vi.fn(async () => {
      throw new Error("completed work must not run again");
    });
    const createArtifact = vi.fn(async ({ outputId }) => {
      expect(outputId).toBe("artifact-1");
      return "artifact-1";
    });

    const retried = await runHistoricalImport(
      draft,
      failed,
      operations({ createWork, createArtifact }),
    );

    expect(createWork).not.toHaveBeenCalled();
    expect(createArtifact).toHaveBeenCalledTimes(1);
    expect(retried.steps.every((step) => step.status === "completed")).toBe(true);
    expect(retried.steps.map((step) => step.idempotencyKey)).toEqual(
      first.steps.map((step) => step.idempotencyKey),
    );
    expect(retried.outputs).toEqual({
      workId: "work-1",
      artifactId: "artifact-1",
      versionId: "version-1",
      publicationRecordId: "publication-1",
    });
  });

  it("reuses the same work key and payload after its response is lost", async () => {
    const sent: Array<{ key: string; title: string }> = [];
    const session = createImportSession("response-loss");
    const responseLost = operations({
      createWork: async ({ draft: input, idempotencyKey }) => {
        sent.push({ key: idempotencyKey, title: deriveHistoricalImportTitle(input.title, input.body) });
        throw new Error("response lost after commit");
      },
    });
    const failed = await runHistoricalImport(draft, session, responseLost);
    const retry = operations({
      createWork: async ({ draft: input, idempotencyKey }) => {
        sent.push({ key: idempotencyKey, title: deriveHistoricalImportTitle(input.title, input.body) });
        return "work-after-commit";
      },
    });

    const recovered = await runHistoricalImport(draft, failed, retry);
    expect(sent).toEqual([
      { key: "response-loss:work", title: draft.title },
      { key: "response-loss:work", title: draft.title },
    ]);
    expect(recovered.outputs.workId).toBe("work-after-commit");
    expect(recovered.steps[0]?.status).toBe("completed");
  });

  it("keeps a checkpointed input locked and reports a replay conflict as failed", async () => {
    const first = await runHistoricalImport(
      draft,
      createImportSession("conflict-after-loss"),
      operations({
        createWork: async ({ checkpoint }) => {
          checkpoint("work-after-commit");
          throw new Error("response lost after commit");
        },
      }),
    );
    const later = vi.fn(async () => "must-not-run");
    const retried = await runHistoricalImport(
      { ...draft, title: "Changed after the committed request" },
      first,
      operations({
        createWork: async () => {
          throw new Error("409 Idempotency-Key conflicts with a different import request");
        },
        createArtifact: later,
      }),
    );

    expect(editableHistoricalImportFields(first)).not.toContain("title");
    expect(retried.steps[0]?.status).toBe("failed");
    expect(retried.failure?.step).toBe("work");
    expect(later).not.toHaveBeenCalled();
  });

  it("replaces a failed 400 snapshot with corrected publication input", async () => {
    const sent: string[] = [];
    const session = createImportSession("correct-400");
    session.steps = session.steps.map((step) => ({
      ...step,
      status: step.step === "publication" ? "not_started" : "completed",
    }));
    session.outputs = { workId: "work-1", artifactId: "artifact-1", versionId: "version-1", publicationRecordId: "" };
    const bad = { ...draft, pageUrlOrContentId: "" };
    const failed = await runHistoricalImport(bad, session, operations({
      createPublication: async ({ draft: input }) => {
        sent.push(input.pageUrlOrContentId);
        throw Object.assign(new Error("missing page url"), { status: 400 });
      },
    }));
    const corrected = await runHistoricalImport({ ...bad, pageUrlOrContentId: "post-42" }, failed, operations({
      createPublication: async ({ draft: input }) => {
        sent.push(input.pageUrlOrContentId);
        return "publication-1";
      },
    }));
    expect(sent).toEqual(["", "post-42"]);
    expect(corrected.steps[3]?.status).toBe("completed");
  });

  it("keeps a server 400 recoverable through a local validation miss", async () => {
    const session = createImportSession("recover-after-local-error");
    session.steps = session.steps.map((step) => ({ ...step, status: step.step === "publication" ? "not_started" : "completed" }));
    session.outputs = { workId: "w", artifactId: "a", versionId: "v", publicationRecordId: "" };
    const failed = await runHistoricalImport({ ...draft, pageUrlOrContentId: "" }, session, operations({
      createPublication: async () => { throw Object.assign(new Error("missing link"), { status: 400 }); },
    }));
    const locallyInvalid = await runHistoricalImport({ ...draft, pageUrlOrContentId: "fixed", channel: "" }, failed, operations());
    expect(locallyInvalid.failure?.step).toBe("publication");
    expect(editableHistoricalImportFields(locallyInvalid)).toContain("pageUrlOrContentId");
    const recovered = await runHistoricalImport({ ...draft, pageUrlOrContentId: "fixed" }, locallyInvalid, operations({
      createPublication: async ({ draft: input, idempotencyKey }) => {
        expect(input.pageUrlOrContentId).toBe("fixed");
        expect(idempotencyKey).toBe("recover-after-local-error:publication");
        return "p";
      },
    }));
    expect(recovered.steps[3]?.status).toBe("completed");
  });

  it("does not replace an unknown response-loss or 409 request snapshot", async () => {
    const sent: string[] = [];
    const loss = await runHistoricalImport(draft, createImportSession("loss-snapshot"), operations({
      createWork: async () => { throw new Error("response lost"); },
    }));
    const lossRecovered = await runHistoricalImport({ ...draft, title: "changed" }, loss, operations({
      createWork: async ({ draft: input }) => { sent.push(input.title); return "w"; },
    }));
    const conflict = await runHistoricalImport(draft, createImportSession("conflict-snapshot"), operations({
      createWork: async () => { throw Object.assign(new Error("conflict"), { status: 409 }); },
    }));
    const conflictRecovered = await runHistoricalImport({ ...draft, title: "changed" }, conflict, operations({
      createWork: async ({ draft: input }) => { sent.push(input.title); return "w"; },
    }));
    expect(sent).toEqual([draft.title, draft.title]);
    expect(lossRecovered.steps[0]?.status).toBe("completed");
    expect(conflictRecovered.steps[0]?.status).toBe("completed");
  });

  it("uses a readable prefix of the pasted body when the title is blank", () => {
    const body = `  First paragraph with   ordinary spacing.\n\n${"x".repeat(100)}`;
    const title = deriveHistoricalImportTitle("   ", body);

    expect(title).toBe(`First paragraph with ordinary spacing. ${"x".repeat(41)}`);
    expect([...title]).toHaveLength(80);
    expect(body.replace(/\s+/g, " ").trim().startsWith(title)).toBe(true);
  });

  it("keeps a supplied title instead of generating a different one", () => {
    expect(deriveHistoricalImportTitle("  My original title  ", draft.body)).toBe(
      "My original title",
    );
  });

  it("rejects more than 200000 Unicode code points before any step can run", async () => {
    const tooLong = { ...draft, body: "文".repeat(200_001) };
    expect(validateHistoricalImportDraft(tooLong)).toEqual({
      field: "body",
      reason: "too_long",
    });

    const ops = operations();
    const result = await runHistoricalImport(tooLong, createImportSession("session-3"), ops);

    expect(ops.createWork).not.toHaveBeenCalled();
    expect(ops.createArtifact).not.toHaveBeenCalled();
    expect(result.steps.every((step) => step.status === "not_started")).toBe(true);
    expect(result.validationError).toEqual({ field: "body", reason: "too_long" });
  });

  it("does not duplicate the publication link rule in local validation", () => {
    expect(validateHistoricalImportDraft({ ...draft, pageUrlOrContentId: "" })).toBeNull();
  });

  it("locks only inputs already used by completed steps after a later failure", () => {
    const session = createImportSession("session-4");
    session.steps = session.steps.map((step) => ({
      ...step,
      status: step.step === "publication" ? "failed" : "completed",
    }));
    session.outputs = {
      workId: "work-1",
      artifactId: "artifact-1",
      versionId: "version-1",
      publicationRecordId: "",
    };

    expect(editableHistoricalImportFields(session)).toEqual([
      "channel",
      "publishedAt",
      "platformAccount",
      "pageUrlOrContentId",
    ]);
  });
});
