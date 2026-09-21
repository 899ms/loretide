// @vitest-environment node
//
// Canonical home for the Today dashboard's section rules. The page renders
// what these return and holds no judgment of its own, so this file is where
// "what belongs in a section" is decided and where it is tested.

import { describe, expect, it } from "vitest";
import {
  accountsMissingConfig,
  capSection,
  deliveriesNeedingAction,
  estimatedEffort,
  reviewsNeedingAttention,
  SECTION_LIMIT,
  topicReason,
  sourceLabel,
  sourcesToOrganise,
  worksInProgress,
  worthWritingTopics,
} from "./sections";
import type {
  AccountLike,
  ArtifactLike,
  DeliveryTaskLike,
  ProfileLike,
  ReviewRequestLike,
  SourceLike,
  TopicCardLike,
  WorkLike,
} from "./types";

function card(overrides: Partial<TopicCardLike> = {}): TopicCardLike {
  return {
    topicCardId: "card-1",
    accountId: "acct-1",
    status: "draft",
    audienceProblemJudgment: "读者分不清两种做法",
    recommendedAction: "写一篇对比",
    channels: ["xiaohongshu"],
    createdAt: "2026-09-01T00:00:00Z",
    ...overrides,
  };
}

function profile(overrides: Partial<ProfileLike> = {}): ProfileLike {
  return {
    readiness: { can_start: true, missing: [] },
    weekly_hours: { value: 6, status: "confirmed" },
    ...overrides,
  };
}

function work(overrides: Partial<WorkLike> = {}): WorkLike {
  return {
    workId: "work-1",
    topicCardId: "card-1",
    title: "对比稿",
    updatedAt: "2026-09-01T00:00:00Z",
    ...overrides,
  };
}

function artifact(overrides: Partial<ArtifactLike> = {}): ArtifactLike {
  return {
    artifactId: "art-1",
    workId: "work-1",
    title: "正文",
    draftStatus: "working",
    updatedAt: "2026-09-01T00:00:00Z",
    ...overrides,
  };
}

function review(overrides: Partial<ReviewRequestLike> = {}): ReviewRequestLike {
  return {
    reviewRequestId: "rev-1",
    artifactId: "art-1",
    channel: "xiaohongshu",
    status: "pending",
    requestedAt: "2026-09-01T00:00:00Z",
    ...overrides,
  };
}

function delivery(overrides: Partial<DeliveryTaskLike> = {}): DeliveryTaskLike {
  return {
    deliveryTaskId: "del-1",
    artifactId: "art-1",
    channel: "xiaohongshu",
    status: "scheduled",
    scheduledAt: "2026-09-01T00:00:00Z",
    due: false,
    pendingRegistration: false,
    ...overrides,
  };
}

function account(overrides: Partial<AccountLike> = {}): AccountLike {
  return { account_id: "acct-1", display_name: "主账号", platform: "xiaohongshu", ...overrides };
}

describe("capSection", () => {
  it("shows at most the limit", () => {
    const items = Array.from({ length: 11 }, (_, index) => index);
    const section = capSection(items);
    expect(section.shown).toHaveLength(SECTION_LIMIT);
    expect(section.hidden).toBe(1);
  });

  // The total must be counted before truncation. Taken afterwards it reads
  // "10 of 10" on a list of 37, which is worse than no total at all because
  // it looks correct.
  it("counts the total before truncating", () => {
    const section = capSection(Array.from({ length: 37 }, (_, index) => index));
    expect(section.total).toBe(37);
    expect(section.shown).toHaveLength(10);
    expect(section.hidden).toBe(27);
  });

  it("reports nothing hidden when under the limit", () => {
    const section = capSection([1, 2, 3]);
    expect(section).toEqual({ shown: [1, 2, 3], total: 3, hidden: 0 });
  });
});

describe("worthWritingTopics", () => {
  it("lists a draft card", () => {
    const section = worthWritingTopics([card()], new Map([["acct-1", profile()]]));
    expect(section.shown.map((entry) => entry.topicCardId)).toEqual(["card-1"]);
  });

  // One negative per excluded status. `deferred` gets its own because it is
  // the one the ruling named explicitly: "come back to it later" reads like a
  // candidate, and it is not one.
  it.each(["started", "saved", "deferred", "dropped"])("excludes %s", (status) => {
    const section = worthWritingTopics([card({ status })], new Map([["acct-1", profile()]]));
    expect(section.shown).toEqual([]);
    expect(section.total).toBe(0);
  });

  it("puts the newest card first", () => {
    const section = worthWritingTopics(
      [
        card({ topicCardId: "old", createdAt: "2026-09-01T00:00:00Z" }),
        card({ topicCardId: "new", createdAt: "2026-09-10T00:00:00Z" }),
      ],
      new Map([["acct-1", profile()]]),
    );
    expect(section.shown.map((entry) => entry.topicCardId)).toEqual(["new", "old"]);
  });

  it("marks a card whose account profile could not be read", () => {
    const section = worthWritingTopics([card()], new Map(), new Set(["acct-1"]));
    expect(section.shown[0]?.failed).toBe(true);
  });
});

describe("estimatedEffort", () => {
  it("gives the hours when the field is confirmed", () => {
    expect(estimatedEffort("acct-1", profile())).toEqual({ kind: "hours", weeklyHours: 6 });
  });

  // A pending field holds a number, but nobody confirmed it. Showing it as the
  // estimate turns an unfinished setup step into a commitment.
  it("reports unconfirmed when the weekly hours are still pending", () => {
    const pending = profile({ weekly_hours: { value: 6, status: "pending" } });
    expect(estimatedEffort("acct-1", pending)).toEqual({ kind: "unconfirmed" });
  });

  it("reports no-account when the card has no account", () => {
    expect(estimatedEffort(null, profile())).toEqual({ kind: "no-account" });
  });

  it("reports unconfirmed when the profile is missing", () => {
    expect(estimatedEffort("acct-1", undefined)).toEqual({ kind: "unconfirmed" });
  });
});

describe("topicReason", () => {
  it("prefers the audience/problem judgment", () => {
    expect(topicReason(card())).toBe("读者分不清两种做法");
  });

  it("falls back to the recommended action", () => {
    expect(topicReason(card({ audienceProblemJudgment: "  " }))).toBe("写一篇对比");
  });
});

describe("worksInProgress", () => {
  it("lists a work with a working document", () => {
    const section = worksInProgress([work()], new Map([["work-1", [artifact()]]]));
    expect(section.shown[0]).toMatchObject({ workId: "work-1", artifactId: "art-1" });
  });

  it("excludes a work whose documents are all saved", () => {
    const saved = artifact({ draftStatus: "saved" });
    const section = worksInProgress([work()], new Map([["work-1", [saved]]]));
    expect(section.shown).toEqual([]);
  });

  // A work with no documents has nothing to continue, so it is not "in
  // progress" - it would otherwise pad the section with rows that lead nowhere.
  it("excludes a work with no documents", () => {
    const section = worksInProgress([work()], new Map([["work-1", []]]));
    expect(section.shown).toEqual([]);
  });

  it("names the most recently touched working document", () => {
    const section = worksInProgress(
      [work()],
      new Map([
        [
          "work-1",
          [
            artifact({ artifactId: "old", updatedAt: "2026-09-01T00:00:00Z" }),
            artifact({ artifactId: "new", updatedAt: "2026-09-09T00:00:00Z" }),
          ],
        ],
      ]),
    );
    expect(section.shown[0]?.artifactId).toBe("new");
  });

  it("keeps a work whose documents could not be read, marked failed", () => {
    const section = worksInProgress([work()], new Map(), new Set(["work-1"]));
    expect(section.shown[0]).toMatchObject({ workId: "work-1", failed: true });
  });

  // NEGATIVE TEST - specs/031 FR-034 / FR-035.
  //
  // A historical import qualifies on every other count: it has a document, the
  // document has an editing copy, and its updated_at is today because it was
  // just pasted in. Nothing in the section's own logic would exclude it, which
  // is exactly why this is asserted rather than assumed - §2's question is
  // "what am I still writing", and a piece published two years ago is not an
  // answer to it.
  it("excludes a historical import even when its document is being edited", () => {
    const imported = work({
      workId: "imported-1", topicCardId: "", historicalImport: true,
      updatedAt: "2026-09-20T00:00:00Z",
    });
    const section = worksInProgress(
      [imported, work()],
      new Map([
        ["imported-1", [artifact({ artifactId: "imported-art" })]],
        ["work-1", [artifact()]],
      ]),
    );
    expect(section.shown.map((entry) => entry.workId)).not.toContain("imported-1");
    // The positive half. Without it this passes against a section that
    // returns nothing at all.
    expect(section.shown.map((entry) => entry.workId)).toContain("work-1");
  });

  // The flag decides, not the empty topic card id. They are two different
  // facts, and a work could carry one without the other - a card deleted out
  // from under an ordinary work would leave it card-less and still very much
  // in progress.
  it("keeps a work that has no topic card but was not imported", () => {
    const orphan = work({ workId: "orphan-1", topicCardId: "" });
    const section = worksInProgress([orphan], new Map([["orphan-1", [artifact()]]]));
    expect(section.shown.map((entry) => entry.workId)).toContain("orphan-1");
  });

  // A backend deployed without migration 532 sends no flag at all. Absent must
  // read as "not an import", or every work on that deployment would vanish
  // from the block.
  it("treats a missing flag as not an import", () => {
    const section = worksInProgress([work()], new Map([["work-1", [artifact()]]]));
    expect(section.shown).toHaveLength(1);
  });

  // The input array is not reordered in place. worksInProgress used to slice
  // before sorting for this reason; the filter now returns the fresh array,
  // and that is easy to undo by accident.
  it("does not reorder its caller's array", () => {
    const works = [
      work({ workId: "older", updatedAt: "2026-09-01T00:00:00Z" }),
      work({ workId: "newer", updatedAt: "2026-09-09T00:00:00Z" }),
    ];
    worksInProgress(works, new Map());
    expect(works.map((entry) => entry.workId)).toEqual(["older", "newer"]);
  });
});

describe("reviewsNeedingAttention", () => {
  it.each(["pending", "changes_requested"])("includes %s", (status) => {
    const section = reviewsNeedingAttention([review({ status })]);
    expect(section.shown).toHaveLength(1);
  });

  it.each(["approved", "rejected", "cancelled"])("excludes %s", (status) => {
    const section = reviewsNeedingAttention([review({ status })]);
    expect(section.shown).toEqual([]);
  });

  it("puts the longest waiting first", () => {
    const section = reviewsNeedingAttention([
      review({ reviewRequestId: "new", requestedAt: "2026-09-10T00:00:00Z" }),
      review({ reviewRequestId: "old", requestedAt: "2026-09-01T00:00:00Z" }),
    ]);
    expect(section.shown.map((entry) => entry.reviewRequestId)).toEqual(["old", "new"]);
  });
});

describe("deliveriesNeedingAction", () => {
  it("includes a task the server marked due", () => {
    const section = deliveriesNeedingAction([delivery({ due: true })]);
    expect(section.shown[0]).toMatchObject({ reason: "due" });
  });

  it("includes a task the server marked pending registration", () => {
    const task = delivery({ status: "handed_off", pendingRegistration: true });
    const section = deliveriesNeedingAction([task]);
    expect(section.shown[0]).toMatchObject({ reason: "pending_registration" });
  });

  // The guard for FR-007. The scheduled time has passed and the status is
  // `scheduled`, which is exactly what a local recomputation would call due -
  // but the server said it is not. Without this case, recomputing and not
  // recomputing both pass.
  it("does not recompute due from the scheduled time", () => {
    const task = delivery({
      status: "scheduled",
      scheduledAt: "1999-01-01T00:00:00Z",
      due: false,
      pendingRegistration: false,
    });
    expect(deliveriesNeedingAction([task]).shown).toEqual([]);
  });

  it("excludes a task that is neither", () => {
    expect(deliveriesNeedingAction([delivery()]).shown).toEqual([]);
  });

  it("puts overdue handovers before unrecorded ones", () => {
    const section = deliveriesNeedingAction([
      delivery({ deliveryTaskId: "pending", status: "handed_off", pendingRegistration: true }),
      delivery({ deliveryTaskId: "due", due: true }),
    ]);
    expect(section.shown.map((entry) => entry.deliveryTaskId)).toEqual(["due", "pending"]);
  });
});

describe("accountsMissingConfig", () => {
  it("lists an account that cannot start and keeps the field names", () => {
    const blocked = profile({
      readiness: { can_start: false, missing: ["weekly_hours", "content_pillars"] },
    });
    const section = accountsMissingConfig([account()], new Map([["acct-1", blocked]]));
    expect(section.shown[0]?.missing).toEqual(["weekly_hours", "content_pillars"]);
  });

  it("excludes an account that can start", () => {
    const section = accountsMissingConfig([account()], new Map([["acct-1", profile()]]));
    expect(section.shown).toEqual([]);
  });

  // "Nothing missing" and "could not read it" must not look the same.
  it("keeps an account whose profile could not be read, marked failed", () => {
    const section = accountsMissingConfig([account()], new Map(), new Set(["acct-1"]));
    expect(section.shown[0]).toMatchObject({ accountId: "acct-1", failed: true, missing: [] });
  });

  it("drops an account whose profile is simply absent and did not fail", () => {
    expect(accountsMissingConfig([account()], new Map()).shown).toEqual([]);
  });
});

function source(overrides: Partial<SourceLike> = {}): SourceLike {
  return {
    sourceId: "src-1",
    kind: "pasted_text",
    title: "a clipping",
    url: "",
    status: "inbox",
    capturedAt: "2026-09-01T00:00:00Z",
    ...overrides,
  };
}

describe("sourcesToOrganise", () => {
  it("lists an item waiting to be organised", () => {
    const section = sourcesToOrganise([source()]);
    expect(section.shown.map((entry) => entry.sourceId)).toEqual(["src-1"]);
  });

  // One negative per excluded status. `organized` is dealt with and `archived`
  // is a decision already made; listing either turns a to-do list into an
  // everything list.
  it.each(["organized", "archived"])("excludes %s", (status) => {
    const section = sourcesToOrganise([source({ status })]);
    expect(section.shown).toEqual([]);
    expect(section.total).toBe(0);
  });

  // The opposite order from the topics section, deliberately: a clipping from
  // three weeks ago is the one at risk of never being looked at again.
  it("puts the oldest first", () => {
    const section = sourcesToOrganise([
      source({ sourceId: "new", capturedAt: "2026-09-10T00:00:00Z" }),
      source({ sourceId: "old", capturedAt: "2026-09-01T00:00:00Z" }),
    ]);
    expect(section.shown.map((entry) => entry.sourceId)).toEqual(["old", "new"]);
  });

  it("caps at ten and counts the total before truncating", () => {
    const many = Array.from({ length: 12 }, (_, index) =>
      source({ sourceId: `src-${index}`, capturedAt: `2026-09-${String(index + 1).padStart(2, "0")}T00:00:00Z` }),
    );
    const section = sourcesToOrganise(many);
    expect(section.shown).toHaveLength(SECTION_LIMIT);
    expect(section.total).toBe(12);
    expect(section.hidden).toBe(2);
  });
});

describe("sourceLabel", () => {
  it("prefers the title someone gave it", () => {
    expect(sourceLabel(source())).toBe("a clipping");
  });

  // A URL tells someone what it is; an id tells them nothing.
  it("falls back to the link before the id", () => {
    expect(sourceLabel(source({ title: "  ", url: "https://example.com/a" }))).toBe(
      "https://example.com/a",
    );
  });

  it("falls back to the id only when there is nothing else", () => {
    expect(sourceLabel(source({ title: "", url: "" }))).toBe("src-1");
  });
});
