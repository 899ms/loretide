export interface SearchBriefCandidate {
  topicCardId: string;
  briefRevisionId: string;
}

export interface SearchThemeReferenceValidation {
  valid: boolean;
  missingSourceIds: string[];
  missingTopicCardIds: string[];
  missingBriefRevisionIds: string[];
}

/** Validate IDs against current workspace candidates, including brief ownership. */
export function validateSearchThemeReferences(input: {
  sourceIds: readonly string[];
  availableSourceIds: readonly string[];
  topicCardIds: readonly string[];
  availableTopicCardIds: readonly string[];
  briefRevisionIds: readonly string[];
  availableBriefs: readonly SearchBriefCandidate[];
}): SearchThemeReferenceValidation {
  const sourceCandidates = new Set(input.availableSourceIds);
  const topicCandidates = new Set(input.availableTopicCardIds);
  const selectedTopics = new Set(input.topicCardIds);
  const briefs = new Map(input.availableBriefs.map((brief) => [brief.briefRevisionId, brief.topicCardId]));
  const missingSourceIds = uniqueMissing(input.sourceIds, sourceCandidates);
  const missingTopicCardIds = uniqueMissing(input.topicCardIds, topicCandidates);
  const missingBriefRevisionIds = uniqueMissing(input.briefRevisionIds, new Set(
    [...briefs.entries()].filter(([, topicCardId]) => selectedTopics.has(topicCardId)).map(([briefId]) => briefId),
  ));
  return {
    valid: missingSourceIds.length === 0 && missingTopicCardIds.length === 0 && missingBriefRevisionIds.length === 0,
    missingSourceIds, missingTopicCardIds, missingBriefRevisionIds,
  };
}

export function validateSearchSuggestionEvidence(selectedIds: readonly string[], availableIds: readonly string[]) {
  const missingEvidenceSourceIds = uniqueMissing(selectedIds, new Set(availableIds));
  return { valid: missingEvidenceSourceIds.length === 0, missingEvidenceSourceIds };
}

function uniqueMissing(ids: readonly string[], candidates: ReadonlySet<string>): string[] {
  return [...new Set(ids.filter((id) => id && !candidates.has(id)))];
}
