// @vitest-environment node
import { describe, expect, it } from "vitest";
import { EMPTY_NODE_REVISION, type NodeRevision } from "./marketing-nodes";
import {
  emptyMarketingNodeForm,
  marketingNodeFormChanged,
  marketingNodeFormErrors,
  marketingNodeFormFromRevision,
  marketingNodeFormToInput,
  setMarketingNodeAccount,
  setMarketingNodeAccountRole,
  setMarketingNodeMaterial,
} from "./marketing-node-form";

function revision(overrides: Partial<NodeRevision> = {}): NodeRevision {
  return {
    ...EMPTY_NODE_REVISION,
    revisionId: "rev-2",
    nodeId: "node-1",
    workspaceId: "ws-1",
    revision: 2,
    name: "双十一",
    kind: "marketing",
    startsOn: "2026-11-11",
    endsOn: "2026-11-11",
    timezone: "Asia/Shanghai",
    leadDays: { set: true, value: 14 },
    accounts: [
      { accountId: "a1", role: "主推" },
      { accountId: "a2", role: "" },
    ],
    goal: "清库存",
    materialSourceIds: ["s1", "s2"],
    dateCertainty: "tentative",
    dateBasis: "往年同期",
    ...overrides,
  };
}

describe("a form started from the current revision", () => {
  it("carries every field over, so a whole-revision submit loses nothing", () => {
    const current = revision();
    const input = marketingNodeFormToInput(marketingNodeFormFromRevision(current));
    expect(input).toEqual({
      name: "双十一",
      kind: "marketing",
      startsOn: "2026-11-11",
      endsOn: "2026-11-11",
      timezone: "Asia/Shanghai",
      leadDays: { set: true, value: 14 },
      accounts: [
        { accountId: "a1", role: "主推" },
        { accountId: "a2", role: "" },
      ],
      goal: "清库存",
      materialSourceIds: ["s1", "s2"],
      dateCertainty: "tentative",
      dateBasis: "往年同期",
    });
  });

  it("keeps an unset lead time unset and a zero lead time zero", () => {
    const unset = marketingNodeFormFromRevision(revision({ leadDays: { set: false } }));
    const zero = marketingNodeFormFromRevision(revision({ leadDays: { set: true, value: 0 } }));
    expect(marketingNodeFormToInput(unset).leadDays).toEqual({ set: false });
    expect(marketingNodeFormToInput(zero).leadDays).toEqual({ set: true, value: 0 });
  });

  it("keeps untouched fields when only one field changes", () => {
    const current = revision();
    const form = { ...marketingNodeFormFromRevision(current), goal: "拉新" };
    const input = marketingNodeFormToInput(form);
    expect(input.goal).toBe("拉新");
    expect(input.leadDays).toEqual({ set: true, value: 14 });
    expect(input.accounts).toEqual(current.accounts);
    expect(input.materialSourceIds).toEqual(["s1", "s2"]);
    expect(input.dateCertainty).toBe("tentative");
    expect(input.dateBasis).toBe("往年同期");
  });

  it("reports no change until a field differs from the revision", () => {
    const current = revision();
    const form = marketingNodeFormFromRevision(current);
    expect(marketingNodeFormChanged(form, current)).toBe(false);
    expect(marketingNodeFormChanged({ ...form, note: "只是备注" }, current)).toBe(false);
    expect(marketingNodeFormChanged({ ...form, leadDays: "" }, current)).toBe(true);
    expect(marketingNodeFormChanged({ ...form, goal: "拉新" }, current)).toBe(true);
  });
});

describe("a new form", () => {
  it("defaults the time zone to the brand's", () => {
    expect(emptyMarketingNodeForm("America/Los_Angeles").timezone).toBe("America/Los_Angeles");
    expect(emptyMarketingNodeForm("Asia/Shanghai").timezone).toBe("Asia/Shanghai");
  });

  it("starts with the lead time not set, not zero", () => {
    expect(marketingNodeFormToInput(emptyMarketingNodeForm("Asia/Shanghai")).leadDays).toEqual({
      set: false,
    });
  });
});

describe("what the form refuses before saving", () => {
  const valid = marketingNodeFormFromRevision(revision());

  it("accepts the current revision as it is", () => {
    expect(marketingNodeFormErrors(valid)).toEqual([]);
  });

  it("names the end date when it is before the start date", () => {
    expect(marketingNodeFormErrors({ ...valid, endsOn: "2026-11-10" })).toContain("ends_on");
  });

  it("refuses a span longer than 366 days, both days included", () => {
    expect(
      marketingNodeFormErrors({ ...valid, startsOn: "2026-01-01", endsOn: "2027-01-01" }),
    ).toEqual([]);
    expect(
      marketingNodeFormErrors({ ...valid, startsOn: "2026-01-01", endsOn: "2027-01-02" }),
    ).toContain("ends_on");
  });

  it("refuses a lead time that is negative, fractional or over 365", () => {
    for (const leadDays of ["-1", "1.5", "366", "abc"]) {
      expect(marketingNodeFormErrors({ ...valid, leadDays })).toContain("lead_days");
    }
    for (const leadDays of ["", "0", "365"]) {
      expect(marketingNodeFormErrors({ ...valid, leadDays })).not.toContain("lead_days");
    }
  });

  it("refuses an empty, Local or unknown time zone", () => {
    for (const timezone of ["", "Local", "Mars/Olympus", "+08:00"]) {
      expect(marketingNodeFormErrors({ ...valid, timezone })).toContain("timezone");
    }
  });

  it("refuses an empty name and a date that is not a calendar day", () => {
    expect(marketingNodeFormErrors({ ...valid, name: "  " })).toContain("name");
    expect(marketingNodeFormErrors({ ...valid, startsOn: "2026-02-30" })).toContain("starts_on");
  });
});

describe("accounts and materials", () => {
  it("adds and removes an account without touching the others' roles", () => {
    const form = marketingNodeFormFromRevision(revision());
    const removed = setMarketingNodeAccount(form, "a1", false);
    expect(removed.accounts).toEqual([{ accountId: "a2", role: "" }]);
    const added = setMarketingNodeAccount(removed, "a3", true);
    expect(added.accounts).toEqual([
      { accountId: "a2", role: "" },
      { accountId: "a3", role: "" },
    ]);
    expect(setMarketingNodeAccount(added, "a3", true).accounts).toHaveLength(2);
    const roled = setMarketingNodeAccountRole(added, "a3", "陪跑");
    expect(roled.accounts).toEqual([
      { accountId: "a2", role: "" },
      { accountId: "a3", role: "陪跑" },
    ]);
  });

  it("adds and removes a material once", () => {
    const form = marketingNodeFormFromRevision(revision());
    expect(setMarketingNodeMaterial(form, "s1", true).materialSourceIds).toEqual(["s1", "s2"]);
    expect(setMarketingNodeMaterial(form, "s3", true).materialSourceIds).toEqual([
      "s1",
      "s2",
      "s3",
    ]);
    expect(setMarketingNodeMaterial(form, "s1", false).materialSourceIds).toEqual(["s2"]);
  });
});
