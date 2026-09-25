import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "@multica/core/api";
import {
  parseRoiAdjustment,
  parseRoiAttribution,
  parseRoiCost,
  parseRoiCostHistory,
  parseRoiCostList,
  parseRoiDeal,
  parseRoiDealDetail,
  parseRoiDealList,
  parseRoiImportBatch,
  parseRoiImportList,
  parseRoiImportResult,
  parseRoiLead,
  parseRoiLeadDetail,
  parseRoiLeadList,
  parseRoiReportList,
  parseRoiReportVersion,
  parseRoiReportVersionList,
  parseRoiResult,
  parseRoiTouch,
  roiPath,
  type RoiAdjustment,
  type RoiAttribution,
  type RoiCost,
  type RoiCostHistory,
  type RoiDeal,
  type RoiDealDetail,
  type RoiImportBatch,
  type RoiImportResult,
  type RoiLead,
  type RoiLeadDetail,
  type RoiReportVersion,
  type RoiReportVersionHeader,
  type RoiResult,
  type RoiTouch,
} from "./contract";

// Server state for costs, leads, touches, deals and refunds/adjustments
// (specs/034 PR 1). TanStack Query owns it.
//
// Nothing here is optimistic: whether a write lands depends on the server's
// base_revision check and its duplicate check, and a money figure shown
// before either has answered would be a figure the server may refuse. Writes
// invalidate on success and the lists refetch.
//
// Every key carries workspaceId, so switching brands never serves the
// previous brand's records from cache.

export interface RoiListParams {
  from?: string;
  to?: string;
  accountId?: string;
  workId?: string;
  leadId?: string;
  includeInactive?: boolean;
}

function listQuery(params: RoiListParams): Record<string, string> {
  const query: Record<string, string> = {};
  if (params.from) query.from = params.from;
  if (params.to) query.to = params.to;
  if (params.accountId) query.account_id = params.accountId;
  if (params.workId) query.work_id = params.workId;
  if (params.leadId) query.lead_id = params.leadId;
  if (params.includeInactive) query.include_inactive = "true";
  return query;
}

export const roiKeys = {
  all: (workspaceId: string) => ["contentRoi", workspaceId] as const,
  costs: (workspaceId: string, params: RoiListParams = {}) =>
    ["contentRoi", workspaceId, "costs", params] as const,
  cost: (workspaceId: string, costId: string) => ["contentRoi", workspaceId, "cost", costId] as const,
  leads: (workspaceId: string, params: RoiListParams = {}) =>
    ["contentRoi", workspaceId, "leads", params] as const,
  lead: (workspaceId: string, leadId: string) => ["contentRoi", workspaceId, "lead", leadId] as const,
  deals: (workspaceId: string, params: RoiListParams = {}) =>
    ["contentRoi", workspaceId, "deals", params] as const,
  deal: (workspaceId: string, dealId: string) => ["contentRoi", workspaceId, "deal", dealId] as const,
  reports: (workspaceId: string) => ["contentRoi", workspaceId, "reports"] as const,
  reportVersions: (workspaceId: string, reportId: string) =>
    ["contentRoi", workspaceId, "reportVersions", reportId] as const,
  reportVersion: (workspaceId: string, reportId: string, versionNo: number) =>
    ["contentRoi", workspaceId, "reportVersion", reportId, versionNo] as const,
  imports: (workspaceId: string) => ["contentRoi", workspaceId, "imports"] as const,
  importBatch: (workspaceId: string, batchId: string) => ["contentRoi", workspaceId, "importBatch", batchId] as const,
};

export function useRoiCosts(workspaceId: string, params: RoiListParams = {}) {
  return useQuery<RoiCost[]>({
    queryKey: roiKeys.costs(workspaceId, params),
    queryFn: async () => parseRoiCostList(await api.contentROIGet("costs", listQuery(params))),
  });
}

export function useRoiCost(workspaceId: string, costId: string) {
  return useQuery<RoiCostHistory | null>({
    queryKey: roiKeys.cost(workspaceId, costId),
    queryFn: async () => parseRoiCostHistory(await api.contentROIGet(roiPath("costs", costId))),
    enabled: costId !== "",
  });
}

export function useRoiLeads(workspaceId: string, params: RoiListParams = {}) {
  return useQuery<RoiLead[]>({
    queryKey: roiKeys.leads(workspaceId, params),
    queryFn: async () => parseRoiLeadList(await api.contentROIGet("leads", listQuery(params))),
  });
}

export function useRoiLead(workspaceId: string, leadId: string) {
  return useQuery<RoiLeadDetail | null>({
    queryKey: roiKeys.lead(workspaceId, leadId),
    queryFn: async () => parseRoiLeadDetail(await api.contentROIGet(roiPath("leads", leadId))),
    enabled: leadId !== "",
  });
}

export function useRoiDeals(workspaceId: string, params: RoiListParams = {}) {
  return useQuery<RoiDeal[]>({
    queryKey: roiKeys.deals(workspaceId, params),
    queryFn: async () => parseRoiDealList(await api.contentROIGet("deals", listQuery(params))),
  });
}

export function useRoiDeal(workspaceId: string, dealId: string) {
  return useQuery<RoiDealDetail | null>({
    queryKey: roiKeys.deal(workspaceId, dealId),
    queryFn: async () => parseRoiDealDetail(await api.contentROIGet(roiPath("deals", dealId))),
    enabled: dealId !== "",
  });
}

/**
 * One write: a path under /api/content-roi and a body in the server's own
 * field names. Amounts in the body are strings ("3000.00"); a revision carries
 * base_revision. The server writes recorded_by and source_type itself.
 */
export interface RoiWrite {
  path: string;
  body: Record<string, unknown>;
}

function useRoiWrite<T>(workspaceId: string, parse: (data: unknown) => T) {
  const client = useQueryClient();
  return useMutation<T, Error, RoiWrite>({
    mutationFn: async ({ path, body }) => parse(await api.contentROIPost(path, body)),
    onSuccess: () => {
      void client.invalidateQueries({ queryKey: roiKeys.all(workspaceId) });
    },
  });
}

/** POST costs, or costs/{id}/revisions. */
export function useWriteRoiCost(workspaceId: string) {
  return useRoiWrite<RoiCost | null>(workspaceId, parseRoiCost);
}

/** POST leads, leads/{id}/revisions or leads/{id}/merge. */
export function useWriteRoiLead(workspaceId: string) {
  return useRoiWrite<RoiLead | null>(workspaceId, parseRoiLead);
}

/** POST leads/{id}/touches or leads/{id}/touches/{touchId}/revisions. */
export function useWriteRoiTouch(workspaceId: string) {
  return useRoiWrite<RoiTouch | null>(workspaceId, parseRoiTouch);
}

/** POST deals or deals/{id}/revisions. */
export function useWriteRoiDeal(workspaceId: string) {
  return useRoiWrite<RoiDeal | null>(workspaceId, parseRoiDeal);
}

/** POST deals/{id}/adjustments or deals/{id}/adjustments/{adjustmentId}/revisions. */
export function useWriteRoiAdjustment(workspaceId: string) {
  return useRoiWrite<RoiAdjustment | null>(workspaceId, parseRoiAdjustment);
}

/** POST deals/{id}/attribution: the next revision of the judgement on a
 *  deal (base_revision 0 for the first). */
export function useWriteRoiAttribution(workspaceId: string) {
  return useRoiWrite<RoiAttribution | null>(workspaceId, parseRoiAttribution);
}

/**
 * POST preview: compute a report once from the given parameters (contract
 * §5.1) and return it. Nothing is saved, so nothing is invalidated, and
 * nothing is cached: the brand is the one in the request header.
 */
export function useRoiPreview() {
  return useMutation<RoiResult | null, Error, Record<string, unknown>>({
    mutationFn: async (params) => parseRoiResult(await api.contentROIPost("preview", params)),
  });
}

// ---------------------------------------------------------------- report versions (PR 4)

/** GET reports: the latest version of each report. */
export function useRoiReports(workspaceId: string) {
  return useQuery<RoiReportVersionHeader[]>({
    queryKey: roiKeys.reports(workspaceId),
    queryFn: async () => parseRoiReportList(await api.contentROIGet("reports")),
  });
}

/** GET reports/{reportId}/versions. */
export function useRoiReportVersions(workspaceId: string, reportId: string) {
  return useQuery<{ reportId: string; versions: RoiReportVersionHeader[] } | null>({
    queryKey: roiKeys.reportVersions(workspaceId, reportId),
    queryFn: async () => parseRoiReportVersionList(await api.contentROIGet(roiPath("reports", reportId, "versions"))),
    enabled: reportId !== "",
  });
}

/** GET reports/{reportId}/versions/{versionNo}. The stored result is fixed;
 *  inputs_changed is derived by the server each time this is read, and every
 *  record write invalidates it through roiKeys.all. */
export function useRoiReportVersion(workspaceId: string, reportId: string, versionNo: number) {
  return useQuery<RoiReportVersion | null>({
    queryKey: roiKeys.reportVersion(workspaceId, reportId, versionNo),
    queryFn: async () => parseRoiReportVersion(
      await api.contentROIGet(roiPath("reports", reportId, "versions", String(versionNo))),
    ),
    enabled: reportId !== "" && versionNo > 0,
  });
}

/**
 * POST reports (a new report: {title, params}) or reports/{reportId}/versions
 * (the next version; an absent title or params reuse the previous
 * version's). Each generation adds a version and changes none.
 */
export function useGenerateRoiReport(workspaceId: string) {
  return useRoiWrite<RoiReportVersion | null>(workspaceId, parseRoiReportVersion);
}

// ---------------------------------------------------------------- imports (PR 3, page in PR 5)

/** GET imports: the batch list, newest first. */
export function useRoiImports(workspaceId: string) {
  return useQuery<RoiImportBatch[]>({
    queryKey: roiKeys.imports(workspaceId),
    queryFn: async () => parseRoiImportList(await api.contentROIGet("imports")),
  });
}

/** GET imports/{batchId}: one batch with every row's outcome. */
export function useRoiImport(workspaceId: string, batchId: string) {
  return useQuery<RoiImportBatch | null>({
    queryKey: roiKeys.importBatch(workspaceId, batchId),
    queryFn: async () => parseRoiImportBatch(await api.contentROIGet(roiPath("imports", batchId))),
    enabled: batchId !== "",
  });
}

/**
 * POST imports. `body` is toRoiImportBody's output. A dry run sends no key
 * and changes nothing, so it invalidates nothing. A real import sends the
 * caller's Idempotency-Key: the caller keeps one key per import action
 * (roiImportAttempt), so pressing import again with the same rows - after a
 * timeout, say - replays the first answer instead of writing twice.
 */
export function useImportRoiRows(workspaceId: string) {
  const client = useQueryClient();
  return useMutation<RoiImportResult | null, Error, { body: Record<string, unknown>; idempotencyKey?: string }>({
    mutationFn: async ({ body, idempotencyKey }) =>
      parseRoiImportResult(await api.contentROIImport(body, body.dry_run === true ? undefined : idempotencyKey)),
    onSuccess: (result) => {
      if (result && !result.dryRun) void client.invalidateQueries({ queryKey: roiKeys.all(workspaceId) });
    },
  });
}
