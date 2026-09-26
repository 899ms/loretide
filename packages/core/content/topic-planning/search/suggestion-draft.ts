import type { SearchSuggestion, SearchSuggestionContentInput, SearchSuggestionInput, SearchSuggestionRevisionInput } from "./suggestion-contract";

export type SuggestionContext = { workspaceId: string; workId: string; artifactId: string };
export type SuggestionDraftSession =
  | { mode: "idle" }
  | (SuggestionContext & { mode: "create"; themeId: string; baseVersionId: string; draft: SearchSuggestionContentInput })
  | (SuggestionContext & { mode: "revise"; suggestionId: string; themeId: string; baseVersionId: string; baseRevision: number; draft: SearchSuggestionContentInput });

export const IDLE_SUGGESTION_DRAFT: SuggestionDraftSession = { mode: "idle" };

export function beginSuggestionCreate(
  context: SuggestionContext,
  themeId: string,
  baseVersionId: string,
  draft: SearchSuggestionContentInput,
): SuggestionDraftSession {
  if (!context.workspaceId || !context.workId || !context.artifactId || !themeId || !baseVersionId) return IDLE_SUGGESTION_DRAFT;
  return { mode: "create", ...context, themeId, baseVersionId, draft: copyDraft(draft) };
}

export function beginSuggestionRevision(
  workspaceId: string,
  suggestion: SearchSuggestion,
  draft: SearchSuggestionContentInput,
): SuggestionDraftSession {
  return {
    mode: "revise", workspaceId, workId: suggestion.workId, artifactId: suggestion.artifactId,
    suggestionId: suggestion.suggestionId, themeId: suggestion.themeId, baseVersionId: suggestion.baseVersionId,
    baseRevision: suggestion.revision, draft: copyDraft(draft),
  };
}

export function updateSuggestionDraft(
  session: SuggestionDraftSession,
  draft: SearchSuggestionContentInput,
): SuggestionDraftSession {
  return session.mode === "idle" ? session : { ...session, draft: copyDraft(draft) };
}

export function suggestionCreateRequest(session: SuggestionDraftSession): SearchSuggestionInput | null {
  if (session.mode !== "create" || !session.workspaceId || !session.workId || !session.artifactId || !session.themeId || !session.baseVersionId) return null;
  return {
    ...copyDraft(session.draft), work_id: session.workId, artifact_id: session.artifactId,
    base_version_id: session.baseVersionId, theme_id: session.themeId,
  };
}

export function suggestionRevisionRequest(session: SuggestionDraftSession): { suggestionId: string; input: SearchSuggestionRevisionInput } | null {
  if (session.mode !== "revise" || !session.suggestionId || !Number.isInteger(session.baseRevision) || session.baseRevision < 1) return null;
  return { suggestionId: session.suggestionId, input: { ...copyDraft(session.draft), base_revision: session.baseRevision } };
}

export function suggestionDraftContextMatches(session: SuggestionDraftSession, context: SuggestionContext): boolean {
  return session.mode !== "idle" && session.workspaceId === context.workspaceId
    && session.workId === context.workId && session.artifactId === context.artifactId;
}

function copyDraft(draft: SearchSuggestionContentInput): SearchSuggestionContentInput {
  return { ...draft, aspects: [...draft.aspects], evidence_source_ids: [...draft.evidence_source_ids] };
}
