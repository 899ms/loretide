import { describe, expect, it } from "vitest";
import { buildRankObservationRequest, buildSearchMetricRequest, type SearchAccountOption, type SearchPublicationOption, type SearchThemeOption } from "./page-requests";

const accounts: SearchAccountOption[] = [
  { id: "account-xhs", label: "XHS brand", platform: "xiaohongshu" },
  { id: "account-douyin", label: "Douyin brand", platform: "douyin" },
];
const publicationA: SearchPublicationOption = { publicationRecordId: "publication-a", channel: "xiaohongshu", platformAccount: "XHS brand" };
const publicationB: SearchPublicationOption = { publicationRecordId: "publication-b", channel: "douyin", platformAccount: "Douyin brand" };
const themeB: SearchThemeOption = { themeId: "theme-b", platform: "douyin", accountId: "account-douyin", voided: false };

describe("search performance request contracts", () => {
  it("builds the metric from its selected publication and account only", () => {
    const result = buildSearchMetricRequest({
      publication: publicationA, accountId: "account-xhs", accounts, metric: "search_impression",
      value: "0", unit: "count", statWindow: "7 days", sampledAt: "2026-09-27T12:00",
      evidenceNote: "Platform dashboard",
    });
    expect(result.ok && result.input).toMatchObject({ publication_record_id: "publication-a", platform: "xiaohongshu", account_id: "account-xhs", value: 0 });
  });

  it("derives observation platform and target from the observation publication, not the metric selection", () => {
    const result = buildRankObservationRequest({
      theme: themeB, publication: publicationB, accountId: "account-douyin", accounts,
      query: "brand query", observedAt: "2026-09-27T12:00", conditions: "logged out",
      resultKind: "position", position: "4", scannedDepth: "", evidenceNote: "Manual check",
    });
    expect(result.ok && result.input).toMatchObject({ publication_record_id: "publication-b", theme_id: "theme-b", platform: "douyin", account_id: "account-douyin", position: 4 });
  });

  it("rejects mismatched themes, unsupported channels and account/platform mismatches", () => {
    expect(buildRankObservationRequest({
      theme: themeB, publication: publicationA, accountId: "account-douyin", accounts,
      query: "q", observedAt: "2026-09-27T12:00", conditions: "c", resultKind: "position", position: "1", scannedDepth: "", evidenceNote: "e",
    })).toEqual({ ok: false, reason: "platform_mismatch" });
    expect(buildRankObservationRequest({
      theme: { ...themeB, platform: "bilibili" }, publication: null, accountId: "", accounts,
      query: "q", observedAt: "2026-09-27T12:00", conditions: "c", resultKind: "position", position: "1", scannedDepth: "", evidenceNote: "e",
    })).toEqual({ ok: false, reason: "unsupported_channel" });
    expect(buildSearchMetricRequest({
      publication: publicationA, accountId: "account-douyin", accounts, metric: "search_visit",
      value: "1", unit: "count", statWindow: "7 days", sampledAt: "2026-09-27T12:00", evidenceNote: "e",
    })).toEqual({ ok: false, reason: "account_mismatch" });
  });

  it("requires valid measurement values and evidence", () => {
    expect(buildRankObservationRequest({
      theme: null, publication: publicationA, accountId: "account-xhs", accounts,
      query: "q", observedAt: "2026-09-27T12:00", conditions: "c", resultKind: "not_found", position: "", scannedDepth: "0", evidenceNote: "e",
    })).toEqual({ ok: false, reason: "invalid_fields" });
  });
});
