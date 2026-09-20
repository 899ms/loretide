// @vitest-environment node
import { describe, expect, it } from "vitest";

import { emptyProfile, profileReadiness, type ExpressionProfile } from "./profile";
import {
  PROFILE_GROUPS,
  confirmGroup,
  draftDiffers,
  draftToProfile,
  parseHours,
  profileToDraft,
  splitEntries,
  type ProfileFieldKey,
} from "./profile-draft";

function group(id: string) {
  const found = PROFILE_GROUPS.find((candidate) => candidate.id === id);
  if (!found) throw new Error(`no group ${id}`);
  return found;
}

describe("PROFILE_GROUPS", () => {
  it("covers every field of the profile exactly once", () => {
    const listed = PROFILE_GROUPS.flatMap((entry) => entry.fields);
    const all = Object.keys(emptyProfile()) as ProfileFieldKey[];
    expect([...listed].sort()).toEqual([...all].sort());
    expect(new Set(listed).size).toBe(listed.length);
  });
});

describe("profileToDraft / draftToProfile", () => {
  it("round-trips a stored profile without changing a status", () => {
    const profile: ExpressionProfile = {
      ...emptyProfile(),
      audience: { value: "new managers", status: "confirmed" },
      primary_channels: { values: ["xiaohongshu", "zhihu"], status: "confirmed" },
      weekly_hours: { value: 6, status: "confirmed" },
    };
    const draft = profileToDraft(profile);
    expect(draft.primary_channels).toBe("xiaohongshu\nzhihu");
    expect(draft.weekly_hours).toBe("6");
    expect(draftToProfile(profile, draft)).toEqual(profile);
  });

  it("shows an unfilled time budget as blank, not as a confident zero", () => {
    expect(profileToDraft(emptyProfile()).weekly_hours).toBe("");
  });

  it("takes the confirmation back when a confirmed answer is edited", () => {
    const profile: ExpressionProfile = {
      ...emptyProfile(),
      audience: { value: "new managers", status: "confirmed" },
      positioning: { value: "a practitioner", status: "confirmed" },
    };
    const draft = { ...profileToDraft(profile), audience: "new managers and leads" };
    const next = draftToProfile(profile, draft);
    expect(next.audience).toEqual({ value: "new managers and leads", status: "pending" });
    // An untouched field in the same group keeps its confirmation.
    expect(next.positioning.status).toBe("confirmed");
  });

  it("un-confirms a channel list that changed and keeps one that only reordered nothing", () => {
    const profile: ExpressionProfile = {
      ...emptyProfile(),
      primary_channels: { values: ["xiaohongshu"], status: "confirmed" },
    };
    const same = draftToProfile(profile, {
      ...profileToDraft(profile),
      primary_channels: "  xiaohongshu  \n\n",
    });
    expect(same.primary_channels).toEqual({ values: ["xiaohongshu"], status: "confirmed" });

    const changed = draftToProfile(profile, {
      ...profileToDraft(profile),
      primary_channels: "xiaohongshu\nzhihu",
    });
    expect(changed.primary_channels.status).toBe("pending");
  });

  it("never invents content for a field the creator left blank", () => {
    const next = draftToProfile(emptyProfile(), profileToDraft(emptyProfile()));
    expect(next).toEqual(emptyProfile());
  });
});

describe("splitEntries / parseHours", () => {
  it("drops blank lines and surrounding space", () => {
    expect(splitEntries(" a \n\n  \nb\n")).toEqual(["a", "b"]);
    expect(splitEntries("")).toEqual([]);
  });

  it.each([
    ["", 0],
    ["  ", 0],
    ["abc", 0],
    ["-3", 0],
    ["6", 6],
    ["6.7", 6],
    ["Infinity", 0],
  ])("reads %j hours as %i", (text, expected) => {
    expect(parseHours(text)).toBe(expected);
  });
});

describe("confirmGroup", () => {
  it("confirms the group's filled fields and leaves the rest of the profile alone", () => {
    const base = draftToProfile(emptyProfile(), {
      ...profileToDraft(emptyProfile()),
      audience: "new managers",
      common_questions: "how do I run a one-on-one",
      positioning: "a practitioner",
    });
    const next = confirmGroup(base, group("audience"));
    expect(next.audience.status).toBe("confirmed");
    expect(next.common_questions.status).toBe("confirmed");
    // Outside the group, nothing moved - that is what makes this a group
    // confirmation rather than a whole-profile one.
    expect(next.positioning).toEqual({ value: "a practitioner", status: "pending" });
  });

  it("leaves a blank field pending instead of confirming nothing", () => {
    const base = draftToProfile(emptyProfile(), {
      ...profileToDraft(emptyProfile()),
      audience: "new managers",
    });
    const next = confirmGroup(base, group("audience"));
    expect(next.audience.status).toBe("confirmed");
    expect(next.common_questions).toEqual({ value: "", status: "pending" });
  });

  it("cannot confirm a zero-hour budget, so it cannot satisfy the start condition", () => {
    const base = draftToProfile(emptyProfile(), {
      ...profileToDraft(emptyProfile()),
      primary_channels: "xiaohongshu",
      weekly_hours: "0",
    });
    const next = confirmGroup(base, group("capacity"));
    expect(next.primary_channels.status).toBe("confirmed");
    expect(next.weekly_hours).toEqual({ value: 0, status: "pending" });
    expect(profileReadiness(next).missing).toContain("weekly_hours");
  });

  it("re-confirming a group is the same profile again", () => {
    const base = confirmGroup(
      draftToProfile(emptyProfile(), {
        ...profileToDraft(emptyProfile()),
        audience: "new managers",
      }),
      group("audience"),
    );
    expect(confirmGroup(base, group("audience"))).toEqual(base);
  });
});

describe("draftDiffers", () => {
  it("is false for an untouched draft and true once a field changes", () => {
    const profile: ExpressionProfile = {
      ...emptyProfile(),
      audience: { value: "new managers", status: "confirmed" },
    };
    const draft = profileToDraft(profile);
    expect(draftDiffers(profile, draft)).toBe(false);
    expect(draftDiffers(profile, { ...draft, experience: "ten years" })).toBe(true);
  });
});
