// @vitest-environment node
import { describe, expect, it } from "vitest";
import { EMPTY_BRIEF_REVISION, EMPTY_TOPIC_CARD, type BriefRevision } from "./contract";
import {
  actionKeepsReason,
  briefDraftDiffers,
  briefDraftFromRevision,
  briefDraftToInput,
  canAppendBrief,
  decisionInput,
  emptyBriefDraft,
  emptyTopicCardDraft,
  formatChannels,
  isTopicCardDraftReady,
  latestRevision,
  parseChannels,
  sortedRevisions,
  topicCardDraftToInput,
  topicStatusKey,
  accountSelectionChanged,
  accountSelectionOf,
  accountSelectionToWire,
  addTopicSource,
  removeTopicSource,
  topicSourceDraftDiffers,
  topicSourceDraftToInput,
  topicSourceSelectionOf,
  TOPIC_ACCOUNT_FILTER_ALL,
  TOPIC_ACCOUNT_FILTER_NONE,
  TOPIC_ACTIONS,
} from "./form-state";

function revision(overrides: Partial<BriefRevision>): BriefRevision {
  return { ...EMPTY_BRIEF_REVISION, ...overrides };
}

const filledCardDraft = {
  accountId: "",
  audienceProblemJudgment: " 写给刚起步的运营者 ",
  ipFit: "适合这个 IP",
  timing: "没有时效依据",
  existingContentRelation: "没有既有作品",
  evidenceGapsAndInvestment: "没有现成证据，预计两小时",
  channels: "知乎，公众号",
  recommendedAction: "开始",
};

describe("channels", () => {
  it("splits on the separators a Chinese keyboard produces", () => {
    expect(parseChannels("知乎，公众号、小红书, bilibili")).toEqual([
      "知乎",
      "公众号",
      "小红书",
      "bilibili",
    ]);
  });

  it("drops blanks and duplicates instead of storing them", () => {
    expect(parseChannels(" 知乎 ,, 知乎 ,  ")).toEqual(["知乎"]);
    expect(parseChannels("   ")).toEqual([]);
  });

  it("round-trips a stored list back into one line", () => {
    expect(parseChannels(formatChannels(["知乎", "公众号"]))).toEqual(["知乎", "公众号"]);
  });
});

describe("topic card draft", () => {
  it("trims every item and turns the channel line into a list", () => {
    const input = topicCardDraftToInput(filledCardDraft);
    expect(input.audienceProblemJudgment).toBe("写给刚起步的运营者");
    expect(input.channels).toEqual(["知乎", "公众号"]);
    expect(input.accountId).toBeNull();
  });

  it("sends the chosen account, and null when none is chosen", () => {
    expect(topicCardDraftToInput(filledCardDraft).accountId).toBeNull();
    expect(
      topicCardDraftToInput({ ...filledCardDraft, accountId: " account-1 " }).accountId,
    ).toBe("account-1");
  });

  it("accepts an honest 没有 but refuses a blank", () => {
    // §5.2 asks for the absence to be written down, so "没有" is a complete
    // answer and an empty box is not.
    expect(isTopicCardDraftReady({ ...filledCardDraft, timing: "没有" })).toBe(true);
    expect(isTopicCardDraftReady({ ...filledCardDraft, timing: "   " })).toBe(false);
    expect(isTopicCardDraftReady({ ...filledCardDraft, channels: " , " })).toBe(false);
    expect(isTopicCardDraftReady(emptyTopicCardDraft)).toBe(false);
  });
});

describe("decisions", () => {
  it("keeps the reason and note only where the server stores them", () => {
    expect(TOPIC_ACTIONS.filter(actionKeepsReason)).toEqual(["defer", "drop"]);
    expect(decisionInput("defer", "no_evidence", "等素材")).toEqual({
      action: "defer",
      reason: "no_evidence",
      note: "等素材",
    });
    // A start or a save carries neither: the server blanks both, so sending
    // them would look recorded and be discarded.
    expect(decisionInput("save", "no_evidence", "等素材")).toEqual({ action: "save" });
    expect(decisionInput("start", "", "")).toEqual({ action: "start" });
  });
});

describe("brief versions", () => {
  it("seeds the form from the version being changed, not from blank", () => {
    const draft = briefDraftFromRevision(
      revision({ audience: "运营者", channels: ["知乎", "公众号"], costLimit: "0" }),
    );
    expect(draft.audience).toBe("运营者");
    expect(draft.channels).toBe("知乎, 公众号");
    expect(draft.costLimit).toBe("0");
    expect(briefDraftFromRevision(undefined)).toEqual(emptyBriefDraft);
  });

  it("reports a change only when the stored values would differ", () => {
    const current = revision({ audience: "运营者", channels: ["知乎"] });
    const untouched = briefDraftFromRevision(current);
    expect(briefDraftDiffers(untouched, current)).toBe(false);
    // Whitespace alone is not a change: it is trimmed before it is stored.
    expect(briefDraftDiffers({ ...untouched, audience: " 运营者 " }, current)).toBe(false);
    expect(briefDraftDiffers({ ...untouched, audience: "新受众" }, current)).toBe(true);
    expect(briefDraftDiffers({ ...untouched, channels: "知乎，公众号" }, current)).toBe(true);
    expect(briefDraftDiffers(emptyBriefDraft, undefined)).toBe(false);
    expect(briefDraftDiffers({ ...emptyBriefDraft, audience: "x" }, undefined)).toBe(true);
  });

  it("sends the eleven items as the wire shape", () => {
    const input = briefDraftToInput({ ...emptyBriefDraft, audience: " 运营者 ", channels: "知乎" });
    expect(input.audience).toBe("运营者");
    expect(input.channels).toEqual(["知乎"]);
    expect(Object.keys(input).length).toBe(11);
  });

  it("orders versions oldest first whatever order they arrive in", () => {
    const list = [revision({ revision: 3 }), revision({ revision: 1 }), revision({ revision: 2 })];
    expect(sortedRevisions(list).map((item) => item.revision)).toEqual([1, 2, 3]);
    expect(latestRevision(list)?.revision).toBe(3);
    expect(latestRevision([])).toBeUndefined();
    // The input is not reordered in place; the list a component holds stays as
    // it was handed over.
    expect(list.map((item) => item.revision)).toEqual([3, 1, 2]);
  });

  it("allows appending only after the card has been started", () => {
    expect(canAppendBrief(undefined)).toBe(false);
    expect(canAppendBrief(EMPTY_TOPIC_CARD)).toBe(false);
    expect(canAppendBrief({ ...EMPTY_TOPIC_CARD, startedBriefRevisionId: "rev-1" })).toBe(true);
  });
});

describe("status labels", () => {
  it("names the five states and keeps a default branch for the rest", () => {
    for (const status of ["draft", "started", "saved", "deferred", "dropped"]) {
      expect(topicStatusKey(status)).toBe(status);
    }
    // A state this build has never heard of must still render as something.
    expect(topicStatusKey("archived_by_a_newer_server")).toBe("unknown");
    expect(topicStatusKey("")).toBe("unknown");
  });
});

describe("account selection", () => {
  it("treats an empty selection as no account, not as an empty id", () => {
    expect(accountSelectionToWire("")).toBeNull();
    expect(accountSelectionToWire("   ")).toBeNull();
    expect(accountSelectionToWire(" account-1 ")).toBe("account-1");
  });

  it("shows what the card stores, including nothing", () => {
    expect(accountSelectionOf("account-1")).toBe("account-1");
    expect(accountSelectionOf(null)).toBe("");
    expect(accountSelectionOf(undefined)).toBe("");
  });

  it("reports a change only when the link would really differ", () => {
    // Re-confirming the same account would write an audit event saying
    // nothing happened.
    expect(accountSelectionChanged("account-1", "account-1")).toBe(false);
    expect(accountSelectionChanged("", null)).toBe(false);
    expect(accountSelectionChanged("account-2", "account-1")).toBe(true);
    expect(accountSelectionChanged("", "account-1")).toBe(true);
    expect(accountSelectionChanged("account-1", null)).toBe(true);
  });

  it("keeps the three filter answers apart", () => {
    // "every card" is absent from the query string; "no account" needs a word
    // of its own because it has no id.
    expect(TOPIC_ACCOUNT_FILTER_ALL).toBe("");
    expect(TOPIC_ACCOUNT_FILTER_NONE).toBe("none");
    expect(TOPIC_ACCOUNT_FILTER_NONE).not.toBe(TOPIC_ACCOUNT_FILTER_ALL);
  });
});

describe("topic source references", () => {
  const card = {
    ...EMPTY_TOPIC_CARD,
    fitSourceIds: ["fit-1"],
    evidenceSourceIds: ["evidence-1"],
  };

  it("keeps an untouched column absent rather than clearing it", () => {
    const draft = addTopicSource({}, "fitSourceIds", " fit-2 ", card.fitSourceIds);
    expect(topicSourceSelectionOf(draft, "fitSourceIds", card.fitSourceIds)).toEqual([
      "fit-1",
      "fit-2",
    ]);
    expect(topicSourceDraftToInput(draft)).toEqual({
      fitSourceIds: ["fit-1", "fit-2"],
    });
    expect(topicSourceDraftDiffers(draft, card)).toBe(true);
  });

  it("normalizes duplicates and makes removing the last source an explicit clear", () => {
    const selected = addTopicSource({}, "evidenceSourceIds", " evidence-1 ", []);
    const deduplicated = addTopicSource(
      selected,
      "evidenceSourceIds",
      "evidence-1",
      [],
    );
    expect(topicSourceDraftToInput(deduplicated)).toEqual({
      evidenceSourceIds: ["evidence-1"],
    });
    const cleared = removeTopicSource(
      deduplicated,
      "evidenceSourceIds",
      "evidence-1",
      [],
    );
    expect(topicSourceDraftToInput(cleared)).toEqual({ evidenceSourceIds: [] });
  });

  it("does not create a write when a draft returns to the stored value", () => {
    const draft = addTopicSource({}, "fitSourceIds", "fit-1", []);
    expect(topicSourceDraftDiffers(draft, card)).toBe(false);
  });
});
