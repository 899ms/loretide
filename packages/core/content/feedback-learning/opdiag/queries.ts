import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "@multica/core/api";
import {
  opdiagPath,
  parseOpDiagReportList,
  parseOpDiagReportVersion,
  parseOpDiagReportVersionList,
  parseOpDiagWorkMark,
  parseOpDiagWorkMarkList,
  type OpDiagReportHeader,
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
