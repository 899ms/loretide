import { RANK_RESULT_KINDS, SEARCH_METRICS, type RankObservationInput, type SearchMetricInput } from "./contract";

export const SEARCH_OBSERVATION_CHANNELS = ["xiaohongshu", "wechat_mp", "douyin", "shipinhao"] as const;
export type SearchAccountOption = { id: string; label: string; platform: string };
export type SearchPublicationOption = { publicationRecordId: string; channel: string; platformAccount: string };
export type SearchThemeOption = { themeId: string; platform: string; accountId: string; voided: boolean };
export type SearchFormResult<T> = { ok: true; input: T } | { ok: false; reason: "missing_target" | "unsupported_channel" | "platform_mismatch" | "account_mismatch" | "invalid_fields" };

export function buildSearchMetricRequest(input: {
  publication: SearchPublicationOption | null;
  accountId: string;
  accounts: readonly SearchAccountOption[];
  metric: string;
  value: string;
  unit: string;
  statWindow: string;
  sampledAt: string;
  evidenceNote: string;
}): SearchFormResult<SearchMetricInput> {
  if (!input.publication) return { ok: false, reason: "missing_target" };
  if (!isObservationChannel(input.publication.channel)) return { ok: false, reason: "unsupported_channel" };
  if (!isIn(SEARCH_METRICS, input.metric)) return { ok: false, reason: "invalid_fields" };
  const account = resolveAccount(input.accountId, input.accounts, input.publication.channel);
  if (account.reason) return { ok: false, reason: account.reason };
  const value = input.value.trim() === "" ? null : Number(input.value);
  if ((value !== null && (!Number.isInteger(value) || value < 0)) || !input.statWindow.trim() || !input.sampledAt || !validDate(input.sampledAt) || !input.evidenceNote.trim()) {
    return { ok: false, reason: "invalid_fields" };
  }
  return { ok: true, input: {
    publication_record_id: input.publication.publicationRecordId, platform: input.publication.channel,
    account_id: account.accountId, metric: input.metric, value, unit: input.unit,
    stat_window: input.statWindow, sampled_at: new Date(input.sampledAt).toISOString(), evidence_note: input.evidenceNote,
  } };
}

export function buildRankObservationRequest(input: {
  theme: SearchThemeOption | null;
  publication: SearchPublicationOption | null;
  accountId: string;
  accounts: readonly SearchAccountOption[];
  query: string;
  observedAt: string;
  conditions: string;
  resultKind: string;
  position: string;
  scannedDepth: string;
  evidenceNote: string;
}): SearchFormResult<RankObservationInput> {
  if (!input.theme && !input.publication) return { ok: false, reason: "missing_target" };
  if (input.theme?.voided) return { ok: false, reason: "missing_target" };
  if (input.theme && input.publication && input.theme.platform !== input.publication.channel) return { ok: false, reason: "platform_mismatch" };
  const platform = input.publication?.channel || input.theme?.platform || "";
  if (!isObservationChannel(platform)) return { ok: false, reason: "unsupported_channel" };
  const account = resolveAccount(input.accountId, input.accounts, platform);
  if (account.reason) return { ok: false, reason: account.reason };
  if (!input.query.trim() || !input.conditions.trim() || !input.evidenceNote.trim() || !input.observedAt || !validDate(input.observedAt) || !isIn(RANK_RESULT_KINDS, input.resultKind)) {
    return { ok: false, reason: "invalid_fields" };
  }
  const position = input.position.trim() === "" ? null : Number(input.position);
  const scannedDepth = input.scannedDepth.trim() === "" ? null : Number(input.scannedDepth);
  if (input.resultKind === "position" ? !isPositiveInteger(position) : !isPositiveInteger(scannedDepth)) return { ok: false, reason: "invalid_fields" };
  return { ok: true, input: {
    platform, account_id: account.accountId, query: input.query, theme_id: input.theme?.themeId ?? "",
    publication_record_id: input.publication?.publicationRecordId ?? "", observed_at: new Date(input.observedAt).toISOString(),
    conditions: input.conditions, result_kind: input.resultKind,
    position: input.resultKind === "position" ? position : null,
    scanned_depth: input.resultKind === "not_found" ? scannedDepth : null, evidence_note: input.evidenceNote,
  } };
}

function resolveAccount(accountId: string, accounts: readonly SearchAccountOption[], platform: string) {
  if (!accountId) return { reason: null, accountId: "" };
  const account = accounts.find((item) => item.id === accountId);
  if (!account || account.platform !== platform) {
    return { reason: "account_mismatch" as const, accountId: "" };
  }
  return { reason: null, accountId: account.id };
}

function isObservationChannel(platform: string): platform is (typeof SEARCH_OBSERVATION_CHANNELS)[number] {
  return (SEARCH_OBSERVATION_CHANNELS as readonly string[]).includes(platform);
}
function isIn<T extends readonly string[]>(values: T, value: string): value is T[number] { return (values as readonly string[]).includes(value); }
function validDate(value: string): boolean { return Number.isFinite(Date.parse(value)); }
function isPositiveInteger(value: number | null): value is number { return value !== null && Number.isInteger(value) && value > 0; }
