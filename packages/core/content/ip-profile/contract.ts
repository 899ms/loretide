import { z } from "zod";
import { ApiError } from "@multica/core/api";
import { parseWithFallback } from "@multica/core/api/schema";

// Response shapes and the failure vocabulary for the account settings page.
//
// Everything crossing the network is parsed with `parseWithFallback` and a
// lenient schema: an installed client talks to whatever backend is deployed,
// and a page that white-screens on an unexpected field is worse than one that
// renders what it understood.

export const accountSchema = z.object({
  account_id: z.string(),
  workspace_id: z.string().optional(),
  // Deliberately `z.string()` and not an enum: a backend that adds a platform
  // must not blank out the page for a client that predates it. The controlled
  // set governs what this page can WRITE; what it can READ is wider.
  platform: z.string(),
  display_name: z.string(),
  settings: z.record(z.string(), z.unknown()).optional(),
  created_at: z.string().optional(),
  updated_at: z.string().optional(),
});

export const accountListSchema = z.object({
  accounts: z.array(accountSchema).nullable().optional(),
});

export const revisionSchema = z.object({
  revision_id: z.string(),
  account_id: z.string(),
  revision: z.number(),
  persona_prompt: z.string(),
  created_at: z.string().optional(),
});

export type ContentAccount = z.infer<typeof accountSchema>;
export type PersonaRevision = z.infer<typeof revisionSchema>;

const EMPTY_ACCOUNT: ContentAccount = {
  account_id: "",
  platform: "",
  display_name: "",
};

// A revision with no id is how "this account has never had a prompt set"
// reaches the UI. The prompt is blank, which is a legitimate value in its own
// right (SOP 3.1 leaves unconfirmed items pending), so the two cases render
// identically and neither is an error.
export const NO_REVISION: PersonaRevision = {
  revision_id: "",
  account_id: "",
  revision: 0,
  persona_prompt: "",
};

export function parseAccount(data: unknown): ContentAccount {
  return parseWithFallback(data, accountSchema, EMPTY_ACCOUNT, {
    endpoint: "content-accounts/detail",
  });
}

export function parseAccountList(data: unknown): ContentAccount[] {
  const parsed = parseWithFallback(
    data,
    accountListSchema,
    { accounts: [] as ContentAccount[] },
    { endpoint: "content-accounts/list" },
  );
  return parsed.accounts ?? [];
}

export function parseRevision(data: unknown): PersonaRevision {
  return parseWithFallback(data, revisionSchema, NO_REVISION, {
    endpoint: "content-accounts/persona",
  });
}

// The diagnostic error object, per the onboarding contract section 3. Every
// field is optional here even though the server sends them: this schema runs
// against a body that has ALREADY failed, and a strict schema would turn a
// malformed error into no error message at all.
const diagnosticErrorSchema = z.object({
  error: z.string().optional(),
  code: z.string().optional(),
  trace_id: z.string().optional(),
  next_action: z.string().optional(),
  retryable: z.boolean().optional(),
});

export interface AccountErrorDescription {
  /** Machine-readable code, "" when the body carried none. */
  code: string;
  /** What the server says to do next, "" when absent. */
  nextAction: string;
  traceId: string;
}

/**
 * Describe a failure for display. Never throws: the caller is already on a
 * failure path, and a parse error here would replace a bad message with a
 * crash.
 */
export function describeAccountError(error: unknown): AccountErrorDescription {
  const empty: AccountErrorDescription = { code: "", nextAction: "", traceId: "" };
  if (!(error instanceof ApiError)) return empty;
  type DiagnosticError = z.infer<typeof diagnosticErrorSchema>;
  const parsed = parseWithFallback<DiagnosticError>(
    error.body,
    diagnosticErrorSchema,
    {},
    { endpoint: "content-accounts/error" },
  );
  return {
    code: parsed.code ?? "",
    nextAction: parsed.next_action ?? "",
    traceId: parsed.trace_id ?? "",
  };
}

/**
 * What happened to a save, in the page's own vocabulary.
 *
 * `conflict` is separate from `failed` because the two mean opposite things to
 * the person looking at the screen: a 409 says the write did not happen and
 * pressing save again will work, while `failed` says something is wrong. If
 * this distinction lived only in a JSX branch there would be nothing to test.
 */
export type SaveOutcome =
  | { kind: "saved" }
  | { kind: "conflict" }
  | { kind: "failed"; detail: AccountErrorDescription };

export const SAVED: SaveOutcome = { kind: "saved" };

export function saveOutcome(error: unknown): SaveOutcome {
  if (error instanceof ApiError && error.status === 409) return { kind: "conflict" };
  return { kind: "failed", detail: describeAccountError(error) };
}
