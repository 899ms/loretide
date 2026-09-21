// SOP 3.3's last sentence: "推荐依据来自 IP 方向与现有素材，并明确显示『暂无
// 个人表现数据』".
//
// One derivation, used in two places (specs/031 FR-025): the account page and
// the topic block on the workbench. It answers a single question - has anybody
// recorded a number yet - and it is deliberately as small as that sounds.
//
// What it is NOT:
//
//   - It is not a judgement about whether the data is ENOUGH. "Three readings
//     is too thin to draw conclusions from" is a real thought, and it is the
//     operator's to have; a threshold here would be a rule the SOP never
//     stated, sitting where nobody can see or change it. Same reasoning as
//     specs/026's FR-005b gave for refusing a default number of days.
//   - It is not a time window. Metrics from three years ago still count as
//     having data. §3.3 is about historical assets being present at all.
//   - It is not a summary. When there IS data this says nothing about it:
//     generating a performance summary is EP-08's business and constitution IX
//     keeps executors off until then.

/** What the two call sites need to render the notice. */
export interface PersonalPerformance {
  /** True when not one manual metric has been recorded in this scope.
   *
   *  The notice is shown on true and hidden on false - and on false NOTHING
   *  replaces it. */
  noData: boolean;
  /** How many were recorded. 0 exactly when `noData`. Carried so a caller can
   *  say how thin the data is without this file deciding what "thin" means. */
  metricCount: number;
}

/**
 * @param metricCount how many manual metric rows exist in the scope being
 *   described - the whole brand for the workbench, one account for the account
 *   page. A caller that could not read the count must not pass 0: see
 *   `personalPerformanceUnknown`.
 */
export function personalPerformance(metricCount: number): PersonalPerformance {
  // Negative and non-integer counts are not real answers from a count query,
  // so they are clamped rather than reasoned about. Clamping UP to 0 keeps the
  // invariant `metricCount === 0 <=> noData` true for every input.
  const count = Number.isFinite(metricCount) && metricCount > 0 ? Math.floor(metricCount) : 0;
  return { noData: count === 0, metricCount: count };
}

/**
 * What to show when the count could not be read at all.
 *
 * NOT `personalPerformance(0)`. A failed request is not evidence that there is
 * no data, and answering "暂无个人表现数据" on a 503 would state something we
 * did not check - the same mistake specs/021's `useAccountProfile` made when it
 * swallowed an error and rendered an empty profile (Issue #167).
 *
 * `noData` is false, so the notice is NOT shown; the caller renders its own
 * "读取失败，重试" instead.
 */
export const personalPerformanceUnknown: PersonalPerformance = { noData: false, metricCount: 0 };
