// Per-account form drafts.
//
// The rule this file exists for: switching accounts must not carry unsaved
// input across. One shared `{platform, displayName, personaPrompt}` in a
// component would do exactly that, and the failure is quiet - the field is
// populated, just with the wrong account's text - so it is easy to ship and
// hard to notice.
//
// Drafts are keyed by account id and are discarded, never persisted. They
// exist for the length of one visit to the page; anything worth keeping has
// been saved to the server.

export interface AccountDraft {
  platform: string;
  displayName: string;
  personaPrompt: string;
}

/** The server's current values for one account, used to seed a fresh draft. */
export interface AccountServerValues {
  platform: string;
  displayName: string;
  personaPrompt: string;
}

export type DraftState = Readonly<Record<string, AccountDraft>>;

export const emptyDraftState: DraftState = {};

/**
 * The draft to show for `accountId`.
 *
 * With no bucket for this account the server values are returned - NOT the
 * previously edited account's draft, and not a blank form. This is the whole
 * "switching accounts does not carry input over" requirement, stated as a
 * function so it can be asserted without a browser.
 */
export function selectDraft(
  state: DraftState,
  accountId: string,
  server: AccountServerValues,
): AccountDraft {
  const existing = state[accountId];
  if (existing) return existing;
  return {
    platform: server.platform,
    displayName: server.displayName,
    personaPrompt: server.personaPrompt,
  };
}

/** Edit one account's draft, seeding it from the server values if untouched. */
export function editDraft(
  state: DraftState,
  accountId: string,
  server: AccountServerValues,
  patch: Partial<AccountDraft>,
): DraftState {
  const current = selectDraft(state, accountId, server);
  return { ...state, [accountId]: { ...current, ...patch } };
}

/**
 * Drop one account's draft. Called after a successful save and when leaving an
 * account, so the next visit reads the server again rather than a stale local
 * copy that may now disagree with it.
 */
export function discardDraft(state: DraftState, accountId: string): DraftState {
  if (!(accountId in state)) return state;
  const next = { ...state };
  delete next[accountId];
  return next;
}

/** True when this account has edits that have not been saved. */
export function isDirty(
  state: DraftState,
  accountId: string,
  server: AccountServerValues,
): boolean {
  const draft = state[accountId];
  if (!draft) return false;
  return (
    draft.platform !== server.platform ||
    draft.displayName !== server.displayName ||
    draft.personaPrompt !== server.personaPrompt
  );
}
