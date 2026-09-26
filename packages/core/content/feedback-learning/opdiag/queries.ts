import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "@multica/core/api";
import {
  OPDIAG_ACCOUNTS_KEY_PREFIX,
  OPDIAG_TOPIC_CARDS_KEY_PREFIX,
  opdiagPath,
  parseOpDiagAnnotations,
  parseOpDiagDecision,
  parseOpDiagJudgement,
  parseOpDiagProposal,
  parseOpDiagProposalList,
  parseOpDiagSuggestion,
  parseOpDiagTodo,
  parseOpDiagTodoList,
  type OpDiagAnnotations,
  type OpDiagDecision,
  type OpDiagJudgement,
  type OpDiagProposal,
  type OpDiagSuggestion,
  type OpDiagTodo,
  parseOpDiagPreview,
  parseOpDiagReportList,
  parseOpDiagReportVersion,
  parseOpDiagReportVersionList,
  parseOpDiagWorkMark,
  parseOpDiagWorkMarkList,
  type OpDiagReportHeader,
  type OpDiagResult,
  type OpDiagReportVersion,
  type OpDiagWorkMark,
} from "./contract";

// Server state for the operating diagnosis (specs/035 PR 1). TanStack Query
// owns it.
//
// Nothing here is optimistic: a generated version is whatever the server
// computed from the inputs it read, and a mark may be refused. Writes
// invalidate on success and the reads refetch. A mark invalidates the report
// versions too, because it can change what their inputs_changed says.
//
// Every key carries workspaceId, so switching brands never serves the
// previous brand's reports from cache.

export const opdiagKeys = {
  all: (workspaceId: string) => ["contentOpDiag", workspaceId] as const,
  reports: (workspaceId: string) => ["contentOpDiag", workspaceId, "reports"] as const,
  reportVersions: (workspaceId: string, reportId: string) =>
    ["contentOpDiag", workspaceId, "reportVersions", reportId] as const,
  reportVersion: (workspaceId: string, reportId: string, versionNo: number) =>
    ["contentOpDiag", workspaceId, "reportVersion", reportId, versionNo] as const,
  workMarks: (workspaceId: string, filter: OpDiagWorkMarkFilter = {}) =>
    ["contentOpDiag", workspaceId, "workMarks", filter] as const,
};

export interface OpDiagWorkMarkFilter {
  workId?: string;
  accountId?: string;
}

/** GET reports: the latest version of each report. */
export function useOpDiagReports(workspaceId: string) {
  return useQuery<OpDiagReportHeader[]>({
    queryKey: opdiagKeys.reports(workspaceId),
    queryFn: async () => parseOpDiagReportList(await api.contentOpDiagGet("reports")),
  });
}

/** GET reports/{reportId}/versions. */
export function useOpDiagReportVersions(workspaceId: string, reportId: string) {
  return useQuery<{ reportId: string; versions: OpDiagReportHeader[] } | null>({
    queryKey: opdiagKeys.reportVersions(workspaceId, reportId),
    queryFn: async () => parseOpDiagReportVersionList(
      await api.contentOpDiagGet(opdiagPath("reports", reportId, "versions")),
    ),
    enabled: reportId !== "",
  });
}

/** GET reports/{reportId}/versions/{versionNo}: the stored version, with
 *  inputs_changed derived by the server on this read. */
export function useOpDiagReportVersion(workspaceId: string, reportId: string, versionNo: number) {
  return useQuery<OpDiagReportVersion | null>({
    queryKey: opdiagKeys.reportVersion(workspaceId, reportId, versionNo),
    queryFn: async () => parseOpDiagReportVersion(
      await api.contentOpDiagGet(opdiagPath("reports", reportId, "versions", String(versionNo))),
    ),
    enabled: reportId !== "" && versionNo > 0,
  });
}

/** GET work-marks, by work or by account. */
export function useOpDiagWorkMarks(workspaceId: string, filter: OpDiagWorkMarkFilter = {}) {
  const query: Record<string, string> = {};
  if (filter.workId) query.work_id = filter.workId;
  if (filter.accountId) query.account_id = filter.accountId;
  return useQuery<OpDiagWorkMark[]>({
    queryKey: opdiagKeys.workMarks(workspaceId, filter),
    queryFn: async () => parseOpDiagWorkMarkList(await api.contentOpDiagGet("work-marks", query)),
  });
}

/** One write: a path under /api/content-operating-diagnosis and a body in
 *  the server's own field names. */
export interface OpDiagWrite {
  path: string;
  body: Record<string, unknown>;
}

function useOpDiagWrite<T>(workspaceId: string, parse: (data: unknown) => T) {
  const client = useQueryClient();
  return useMutation<T, Error, OpDiagWrite>({
    mutationFn: async ({ path, body }) => parse(await api.contentOpDiagPost(path, body)),
    onSuccess: () => {
      void client.invalidateQueries({ queryKey: opdiagKeys.all(workspaceId) });
    },
  });
}

/**
 * POST reports (a new report: {title, params}) or
 * reports/{reportId}/versions (the next version; an absent title or params
 * reuse the previous version's). Each generation adds a version and changes
 * none.
 */
export function useGenerateOpDiagReport(workspaceId: string) {
  return useOpDiagWrite<OpDiagReportVersion | null>(workspaceId, parseOpDiagReportVersion);
}

/** POST work-marks: one more mark; the latest per (work, kind, item) is the
 *  current one. */
export function useRecordOpDiagWorkMark(workspaceId: string) {
  return useOpDiagWrite<OpDiagWorkMark | null>(workspaceId, parseOpDiagWorkMark);
}

/** POST preview ({params}): the result the server would store, computed
 *  now and stored nowhere. Nothing to invalidate. */
export function usePreviewOpDiag() {
  return useMutation<OpDiagResult | null, Error, { params: Record<string, unknown> }>({
    mutationFn: async (body) => parseOpDiagPreview(await api.contentOpDiagPost("preview", body)),
  });
}

// ---------------------------------------------------------------- PR 3

export interface OpDiagProposalFilter {
  accountId?: string;
  state?: string;
}

export interface OpDiagTodoFilter {
  accountId?: string;
  state?: string;
}

export const opdiagDecisionKeys = {
  annotations: (workspaceId: string, reportId: string, versionNo: number) =>
    ["contentOpDiag", workspaceId, "annotations", reportId, versionNo] as const,
  proposals: (workspaceId: string, filter: OpDiagProposalFilter = {}) =>
    ["contentOpDiag", workspaceId, "proposals", filter] as const,
  todos: (workspaceId: string, filter: OpDiagTodoFilter = {}) =>
    ["contentOpDiag", workspaceId, "todos", filter] as const,
};

/** GET reports/{reportId}/versions/{versionNo}/annotations. */
export function useOpDiagAnnotations(workspaceId: string, reportId: string, versionNo: number) {
  return useQuery<OpDiagAnnotations | null>({
    queryKey: opdiagDecisionKeys.annotations(workspaceId, reportId, versionNo),
    queryFn: async () => parseOpDiagAnnotations(
      await api.contentOpDiagGet(opdiagPath("reports", reportId, "versions", String(versionNo), "annotations")),
    ),
    enabled: reportId !== "" && versionNo > 0,
  });
}

/** GET profile-proposals, each with "current value -> proposed value". */
export function useOpDiagProfileProposals(workspaceId: string, filter: OpDiagProposalFilter = {}) {
  const query: Record<string, string> = {};
  if (filter.accountId) query.account_id = filter.accountId;
  if (filter.state) query.state = filter.state;
  return useQuery<OpDiagProposal[]>({
    queryKey: opdiagDecisionKeys.proposals(workspaceId, filter),
    queryFn: async () => parseOpDiagProposalList(await api.contentOpDiagGet("profile-proposals", query)),
  });
}

/** GET todos. */
export function useOpDiagTodos(workspaceId: string, filter: OpDiagTodoFilter = {}) {
  const query: Record<string, string> = {};
  if (filter.accountId) query.account_id = filter.accountId;
  if (filter.state) query.state = filter.state;
  return useQuery<OpDiagTodo[]>({
    queryKey: opdiagDecisionKeys.todos(workspaceId, filter),
    queryFn: async () => parseOpDiagTodoList(await api.contentOpDiagGet("todos", query)),
  });
}

/**
 * A PR 3 write. Not optimistic: an adoption crosses into another module
 * and can fail, a confirmation can answer 409 because the profile moved
 * on. On success every diagnosis query refetches, and so do the lists of
 * the modules the write reached.
 */
function useOpDiagDecisionWrite<T>(workspaceId: string, parse: (data: unknown) => T, alsoRefetch: string[] = []) {
  const client = useQueryClient();
  return useMutation<T, Error, OpDiagWrite>({
    mutationFn: async ({ path, body }) => parse(await api.contentOpDiagPost(path, body)),
    onSuccess: () => {
      void client.invalidateQueries({ queryKey: opdiagKeys.all(workspaceId) });
      for (const prefix of alsoRefetch) {
        void client.invalidateQueries({ queryKey: [prefix, workspaceId] });
      }
    },
  });
}

/** POST reports/{id}/versions/{no}/judgements, or judgements/{id}/revisions. */
export function useWriteOpDiagJudgement(workspaceId: string) {
  return useOpDiagDecisionWrite<OpDiagJudgement | null>(workspaceId, parseOpDiagJudgement);
}

/** POST reports/{id}/versions/{no}/suggestions, or suggestions/{id}/revisions. */
export function useWriteOpDiagSuggestion(workspaceId: string) {
  return useOpDiagDecisionWrite<OpDiagSuggestion | null>(workspaceId, parseOpDiagSuggestion);
}

/**
 * POST suggestions/{id}/decisions ({suggestion_revision, decision, mode,
 * link_target_id, note}) or decisions/{id}/retry ({mode, link_target_id}).
 * An adoption may have created a topic card, so the card lists refetch.
 */
export function useDecideOpDiagSuggestion(workspaceId: string) {
  return useOpDiagDecisionWrite<OpDiagDecision | null>(workspaceId, parseOpDiagDecision, [OPDIAG_TOPIC_CARDS_KEY_PREFIX]);
}

/**
 * POST profile-proposals/{id}/confirm ({base_revision_id}) or .../dismiss
 * ({}). A confirmation writes an account profile revision, so the account
 * queries refetch.
 */
export function useSettleOpDiagProposal(workspaceId: string) {
  return useOpDiagDecisionWrite<OpDiagProposal | null>(workspaceId, parseOpDiagProposal, [OPDIAG_ACCOUNTS_KEY_PREFIX]);
}

/** POST todos (a gap of a version) or todos/{id}/revisions. */
export function useWriteOpDiagTodo(workspaceId: string) {
  return useOpDiagDecisionWrite<OpDiagTodo | null>(workspaceId, parseOpDiagTodo);
}
