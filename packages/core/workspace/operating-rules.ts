import { z } from "zod";
import { parseWithFallback } from "@multica/core/api/schema";

// The brand's operating rules (SOP 3.2): weekly cadence, channel templates,
// who reviews, and when to go and look at the numbers.
//
// They live at settings["loretide.operating_rules"] on the workspace, the same
// shape timezone.ts and auto-precheck.ts use. Unlike those two, nothing here
// merges on the client: the endpoint merges server-side, because a client that
// reads the whole blob and writes it back loses whatever somebody else changed
// in between.
//
// The one thing to get right in this file: a number nobody entered is NOT
// zero. A cadence of 0 means "nothing goes out on this channel this week"; an
// absent one means nobody has decided. Every reader below therefore answers
// with `stored` alongside the value, and there is no `?? 0` anywhere - the
// closest thing to it would turn "undecided" into a decision.
//
// Contract: specs/029-operating-rules/contracts/operating-rules.md

/** SOP 3.2's channel set: ip-profile's eight, not review-delivery's four. A
 *  brand that can open a Zhihu account can write down how it posts there, even
 *  though delivery does not reach Zhihu yet. `operating-rules.test.ts` holds
 *  this against the Go source. */
export const RULE_PLATFORMS = [
  "xiaohongshu", "douyin", "wechat_mp", "bilibili",
  "zhihu", "weibo", "kuaishou", "shipinhao",
] as const;
export type RulePlatform = (typeof RULE_PLATFORMS)[number];

/** Exactly one value. SOP 3.2 says team review is later work, and a second
 *  entry here would promise something nothing implements. */
export const REVIEW_RULES = ["self"] as const;
export type ReviewRule = (typeof REVIEW_RULES)[number];

export const OPERATING_RULES_SETTINGS_KEY = "loretide.operating_rules";

export interface ChannelTemplate {
  /** Free text. "Title under 20 characters, first image 3:4" and "must have a
   *  summary" do not decompose into one set of fields. */
  note: string;
}

export interface Observation {
  /** undefined means the brand has set no window at all - which is a real
   *  state, and not the same as 0 ("look the same day"). */
  default?: number;
  byChannel: Record<string, number>;
}

export interface OperatingRules {
  /** Missing key = nobody decided. Present 0 = nothing goes out. */
  cadence: Record<string, number>;
  templates: Record<string, ChannelTemplate>;
  reviewRule: string;
  observation: Observation;
}

// Lenient by design: an installed client talks to whatever backend is
// deployed, and a channel this build has not heard of must not blank the page.
const templateSchema = z.object({ note: z.string().optional() });

const rulesSchema = z.object({
  cadence: z.record(z.string(), z.number()).nullable().optional(),
  templates: z.record(z.string(), templateSchema).nullable().optional(),
  review_rule: z.string().optional(),
  observation: z
    .object({
      // Nullable AND optional, and both mean the same thing here: no window.
      // Neither is a number, so neither becomes one.
      default: z.number().nullable().optional(),
      by_channel: z.record(z.string(), z.number()).nullable().optional(),
    })
    .optional(),
});

/** What a brand that has never set anything reads as.
 *
 *  Note what is absent: no cadence and no observation window. Inventing either
 *  would be a rule the SOP never stated, sitting where no operator can see or
 *  change it - which is the sentence 027 wrote when it declined to write one. */
export function emptyOperatingRules(): OperatingRules {
  return {
    cadence: {},
    templates: {},
    reviewRule: REVIEW_RULES[0],
    observation: { byChannel: {} },
  };
}

export function parseOperatingRules(data: unknown): OperatingRules {
  // The fallback is typed explicitly: inferring it from `{}` would narrow the
  // result to an empty object and hide every field below behind a cast.
  const parsed = parseWithFallback<z.infer<typeof rulesSchema>>(data, rulesSchema, {}, {
    endpoint: "operating-rules",
  });
  const templates: Record<string, ChannelTemplate> = {};
  for (const [platform, template] of Object.entries(parsed.templates ?? {})) {
    templates[platform] = { note: template.note ?? "" };
  }
  return {
    cadence: parsed.cadence ?? {},
    templates,
    // A build that got no review rule shows the only one that exists rather
    // than an empty box.
    reviewRule: parsed.review_rule || REVIEW_RULES[0],
    observation: {
      // `?? undefined`, never `?? 0`. A response that carried no window has
      // not told us the window is zero.
      default: parsed.observation?.default ?? undefined,
      byChannel: parsed.observation?.by_channel ?? {},
    },
  };
}

/** The request body. snake_case on the wire, and `default` is omitted rather
 *  than sent as null when there is no window. */
export function operatingRulesToRequest(rules: OperatingRules): Record<string, unknown> {
  return {
    cadence: rules.cadence,
    templates: Object.fromEntries(
      Object.entries(rules.templates).map(([platform, template]) => [platform, { note: template.note }]),
    ),
    review_rule: rules.reviewRule,
    observation: {
      ...(rules.observation.default === undefined ? {} : { default: rules.observation.default }),
      by_channel: rules.observation.byChannel,
    },
  };
}

/**
 * How many pieces a week this channel is meant to get.
 *
 * Returns `stored` alongside the value for the same reason the Go side returns
 * two values: a caller that only looks at the number cannot tell "nothing goes
 * out" from "nobody decided", and those lead to different pages.
 */
export function readCadence(
  rules: OperatingRules,
  platform: string,
): { value: number; stored: boolean } {
  const value = rules.cadence[platform];
  if (typeof value !== "number") return { value: 0, stored: false };
  return { value, stored: true };
}

/** Where an observation window came from. Three values, not a boolean: "this
 *  channel is set to 7 days" and "this channel has none, so the brand's 14
 *  applies" are different, and the page has to be able to say which. */
export type ObservationSource = "channel" | "global" | "none";

export function readObservation(
  rules: OperatingRules,
  platform: string,
): { days: number; source: ObservationSource } {
  const channel = rules.observation.byChannel[platform];
  if (typeof channel === "number") return { days: channel, source: "channel" };
  if (typeof rules.observation.default === "number") {
    return { days: rules.observation.default, source: "global" };
  }
  return { days: 0, source: "none" };
}

/** The note for one channel, and nothing standing next to it. SOP 3.2:
 *  "模型上下文仅接收必要的渠道说明". */
export function templateNoteFor(rules: OperatingRules, platform: string): string {
  return rules.templates[platform]?.note ?? "";
}

/** Whether delivery can actually reach this channel today.
 *
 *  Eight channels can hold a template; four of them cannot be delivered to
 *  (review-delivery's set). That is not an error - a brand may write down how
 *  it posts to Zhihu before anything can hand a piece over - but the page has
 *  to say so, or somebody will think configuring a template published
 *  something. */
export const DELIVERABLE_PLATFORMS = ["xiaohongshu", "wechat_mp", "douyin", "shipinhao"] as const;

export function isDeliverable(platform: string): boolean {
  return (DELIVERABLE_PLATFORMS as readonly string[]).includes(platform);
}
