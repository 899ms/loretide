export const HISTORICAL_IMPORT_MAX_BODY_RUNES = 200_000;
export const HISTORICAL_IMPORT_TITLE_RUNES = 80;

export const HISTORICAL_IMPORT_STEPS = [
  "work",
  "artifact",
  "version",
  "publication",
] as const;

export type HistoricalImportStep = (typeof HISTORICAL_IMPORT_STEPS)[number];
export type HistoricalImportStepStatus = "not_started" | "completed" | "failed";

export interface HistoricalImportDraft {
  title: string;
  body: string;
  channel: string;
  publishedAt: string;
  platformAccount: string;
  pageUrlOrContentId: string;
}

export type HistoricalImportDraftField = keyof HistoricalImportDraft;

export interface HistoricalImportOutputs {
  workId: string;
  artifactId: string;
  versionId: string;
  publicationRecordId: string;
}

export interface HistoricalImportStepState {
  step: HistoricalImportStep;
  status: HistoricalImportStepStatus;
  idempotencyKey: string;
  input: HistoricalImportDraft | null;
}

export interface HistoricalImportProblem {
  field: "body" | "channel" | "published_at" | "platform_account";
  reason: "missing" | "too_long";
}

export interface HistoricalImportFailure {
  step: HistoricalImportStep;
  error: unknown;
}

export interface HistoricalImportSession {
  sessionId: string;
  declaredBy: string;
  steps: HistoricalImportStepState[];
  outputs: HistoricalImportOutputs;
  validationError: HistoricalImportProblem | null;
  failure: HistoricalImportFailure | null;
}

export interface HistoricalImportOperationContext {
  draft: HistoricalImportDraft;
  outputs: HistoricalImportOutputs;
  outputId: string;
  idempotencyKey: string;
  declaredBy: string;
  checkpoint: (outputId: string) => void;
}

export interface HistoricalImportOperations {
  createWork: (context: HistoricalImportOperationContext) => Promise<string>;
  createArtifact: (context: HistoricalImportOperationContext) => Promise<string>;
  importVersion: (context: HistoricalImportOperationContext) => Promise<string>;
  createPublication: (context: HistoricalImportOperationContext) => Promise<string>;
}

const OUTPUT_FIELDS: Record<HistoricalImportStep, keyof HistoricalImportOutputs> = {
  work: "workId",
  artifact: "artifactId",
  version: "versionId",
  publication: "publicationRecordId",
};

const OPERATION_FIELDS: Record<
  HistoricalImportStep,
  keyof HistoricalImportOperations
> = {
  work: "createWork",
  artifact: "createArtifact",
  version: "importVersion",
  publication: "createPublication",
};

export function createImportSession(sessionId: string, declaredBy = ""): HistoricalImportSession {
  return {
    sessionId,
    declaredBy,
    steps: HISTORICAL_IMPORT_STEPS.map((step) => ({
      step,
      status: "not_started",
      idempotencyKey: `${sessionId}:${step}`,
      input: null,
    })),
    outputs: {
      workId: "",
      artifactId: "",
      versionId: "",
      publicationRecordId: "",
    },
    validationError: null,
    failure: null,
  };
}

export function validateHistoricalImportDraft(
  draft: HistoricalImportDraft,
): HistoricalImportProblem | null {
  if (!draft.body.trim()) return { field: "body", reason: "missing" };
  if ([...draft.body].length > HISTORICAL_IMPORT_MAX_BODY_RUNES) {
    return { field: "body", reason: "too_long" };
  }
  if (!draft.channel.trim()) return { field: "channel", reason: "missing" };
  if (!draft.publishedAt.trim()) return { field: "published_at", reason: "missing" };
  if (!draft.platformAccount.trim()) {
    return { field: "platform_account", reason: "missing" };
  }
  return null;
}

/**
 * Keeps a retry honest: once a step has created an object, its input cannot
 * silently change while a later step is retried. Inputs not used yet remain
 * editable so the operator can correct the failed step.
 */
export function editableHistoricalImportFields(
  session: HistoricalImportSession,
): HistoricalImportDraftField[] {
  const fields: HistoricalImportDraftField[] = [];
  // A checkpoint is durable evidence that its request may already have
  // committed even if the response was lost. Lock the inputs at that point:
  // changing them under the same stable key must surface the server's 409, not
  // create a misleading second client-side attempt.
  const canCorrect = isKnownClientFailure(session.failure?.error);
  const frozen = (step: HistoricalImportStep) =>
    session.steps.find((entry) => entry.step === step)?.input !== null &&
    !(canCorrect && session.failure?.step === step);
  if (!session.outputs.workId && !frozen("work")) fields.push("title");
  if (!session.outputs.artifactId && !frozen("work") && !frozen("artifact")) fields.push("body");
  if (!session.outputs.publicationRecordId && !frozen("publication")) {
    fields.push("channel", "publishedAt", "platformAccount", "pageUrlOrContentId");
  }
  return fields;
}

function isKnownClientFailure(error: unknown): boolean {
  return !!error && typeof error === "object" &&
    typeof (error as { status?: unknown }).status === "number" &&
    (error as { status: number }).status >= 400 && (error as { status: number }).status < 500 &&
    (error as { status: number }).status !== 409;
}

export function deriveHistoricalImportTitle(title: string, body: string): string {
  const supplied = title.trim();
  if (supplied) return supplied;

  const readable = body.replace(/\s+/gu, " ").trim();
  return [...readable].slice(0, HISTORICAL_IMPORT_TITLE_RUNES).join("");
}

export async function runHistoricalImport(
  draft: HistoricalImportDraft,
  session: HistoricalImportSession,
  operations: HistoricalImportOperations,
  onChange: (session: HistoricalImportSession) => void = () => undefined,
): Promise<HistoricalImportSession> {
  const validationError = validateHistoricalImportDraft(draft);
  if (validationError) {
    // Keep the server failure that owns a frozen request snapshot. A local
    // validation miss while correcting that request must not erase the fact
    // that its fields remain eligible for a corrected retry.
    const invalid = { ...session, validationError };
    onChange(invalid);
    return invalid;
  }

  let current: HistoricalImportSession = {
    ...session,
    validationError: null,
  };

  for (const step of HISTORICAL_IMPORT_STEPS) {
    const stepState = current.steps.find((entry) => entry.step === step);
    if (!stepState || stepState.status === "completed") continue;

    const outputField = OUTPUT_FIELDS[step];
    const replaceFailedInput = stepState.status === "failed" && isKnownClientFailure(current.failure?.error);
    const frozenDraft = !stepState.input || replaceFailedInput ? { ...draft } : stepState.input;
    if (!stepState.input || replaceFailedInput) {
      current = {
        ...current,
        steps: current.steps.map((entry) =>
          entry.step === step ? { ...entry, input: frozenDraft } : entry,
        ),
      };
      onChange(current);
    }
    const checkpoint = (outputId: string) => {
      if (!outputId) return;
      current = {
        ...current,
        outputs: { ...current.outputs, [outputField]: outputId },
      };
      onChange(current);
    };

    try {
      const operation = operations[OPERATION_FIELDS[step]];
      const outputId = await operation({
        draft: frozenDraft,
        outputs: current.outputs,
        outputId: current.outputs[outputField],
        idempotencyKey: stepState.idempotencyKey,
        declaredBy: current.declaredBy,
        checkpoint,
      });
      if (!outputId) throw new Error(`${step} returned no id`);
      checkpoint(outputId);
      current = {
        ...current,
        steps: current.steps.map((entry) =>
          entry.step === step ? { ...entry, status: "completed" } : entry,
        ),
        failure: null,
      };
      onChange(current);
    } catch (error) {
      current = {
        ...current,
        steps: current.steps.map((entry) =>
          entry.step === step ? { ...entry, status: "failed" } : entry,
        ),
        failure: { step, error },
      };
      onChange(current);
      break;
    }
  }

  return current;
}
