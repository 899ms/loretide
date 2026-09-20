import type { Artifact, ArtifactVersion } from "./contract";

// The editor's rules, away from the editor.
//
// Everything here is a decision the page would otherwise take inside JSX: which
// badge to show, whether autosave has anything to send, which two versions a
// comparison is between, where a new document goes in the order. This
// repository writes no UI unit tests, so a rule left in a component is a rule
// nothing can check.
//
// Contract: specs/024-work-editor-manual/contracts/work-and-versions.md

/**
 * Whether the editor holds text the server has not been told about yet.
 *
 * Compared against the server's own copy of the draft, not against its
 * draft_status: the status describes what the server last stored, and between
 * a keystroke and the next autosave the two say different things. The editor is
 * the one that knows.
 */
export function draftIsUnsent(localBody: string, artifact: Artifact | undefined): boolean {
  if (!artifact) return false;
  return localBody !== artifact.draftBody;
}

/**
 * What the badge says, from the reader's point of view.
 *
 * `saved` means "what you see is the latest version". Anything else - unsent
 * keystrokes, or a stored draft that has moved past the last version - is
 * `working`. Falling towards `working` is deliberate: telling someone their
 * work is saved when it is not is the failure that costs them the work.
 */
export function draftBadge(
  localBody: string,
  artifact: Artifact | undefined,
): "working" | "saved" {
  if (!artifact) return "working";
  if (draftIsUnsent(localBody, artifact)) return "working";
  return artifact.draftStatus === "saved" ? "saved" : "working";
}

/** Whether "save as a version" can do anything. Saving an unchanged document
 *  again would add a version identical to the last one, which is noise in a
 *  history whose whole job is to be readable. */
export function canSaveVersion(
  localBody: string,
  artifact: Artifact | undefined,
  versions: ArtifactVersion[],
): boolean {
  if (!artifact) return false;
  if (draftIsUnsent(localBody, artifact)) return true;
  if (versions.length === 0) return localBody !== "";
  return artifact.draftStatus === "working";
}

/**
 * The two versions a side-by-side comparison is between.
 *
 * Side by side, not a diff: SOP 7 asks for the history to be readable, and a
 * diff needs a library this phase does not add. Two full texts answer "what
 * changed" without one.
 *
 * `left` is the older of the pair and `right` the newer, whichever order the
 * caller picked them in, so the reader never has to work out which way round
 * they are.
 */
export function comparisonPair(
  versions: ArtifactVersion[],
  selectedId: string,
  againstId: string,
): { left: ArtifactVersion; right: ArtifactVersion } | null {
  const selected = versions.find((version) => version.versionId === selectedId);
  const against = versions.find((version) => version.versionId === againstId);
  if (!selected || !against || selected.versionId === against.versionId) return null;
  return selected.revision < against.revision
    ? { left: selected, right: against }
    : { left: against, right: selected };
}

/** What a comparison defaults to when the reader has picked only one side: the
 *  version before it, because "what changed in this version" is the question
 *  somebody clicking a history row is asking. */
export function defaultComparisonTarget(
  versions: ArtifactVersion[],
  selectedId: string,
): string {
  const index = versions.findIndex((version) => version.versionId === selectedId);
  if (index < 0) return "";
  // versions arrive newest first, so the previous version is the NEXT entry.
  const previous = versions[index + 1];
  return previous ? previous.versionId : "";
}

/** Where a new document goes: after the last one. Positions are explicit
 *  because two documents of the same kind are legitimate and creation time
 *  cannot say which order their author wants. */
export function nextArtifactPosition(artifacts: Artifact[]): number {
  return artifacts.reduce((highest, artifact) => Math.max(highest, artifact.position), 0) + 1;
}

/**
 * The three model-backed entry points, with the reason each is unavailable.
 *
 * They are listed rather than written into the page so that the page cannot
 * quietly render two of them, and so the count is something a reader can check.
 * Constitution IX keeps real executors disabled; EP-08 is what turns these on.
 */
export const AI_ENTRY_POINTS = [
  { id: "rewriteSelection" },
  { id: "polishWhole" },
  { id: "candidateVersions" },
] as const;

export type AIEntryPointId = (typeof AI_ENTRY_POINTS)[number]["id"];

/** Every entry point is unavailable in this phase. A function rather than a
 *  constant `false` so the day one of them turns on, the callers already ask. */
export function aiEntryPointEnabled(_id: AIEntryPointId): boolean {
  return false;
}
