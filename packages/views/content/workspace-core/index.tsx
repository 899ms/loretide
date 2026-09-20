"use client";

import { useState } from "react";
import { useT } from "@multica/views/i18n";
import {
  RULE_PLATFORMS,
  cadenceDisplay,
  canSaveDraft,
  draftFromRules,
  draftProblem,
  draftToRules,
  isDeliverable,
  observationDisplay,
  useOperatingRules,
  useSaveOperatingRules,
  type OperatingRules,
  type RulesDraft,
  type RulesDraftProblem,
} from "@multica/core/workspace";
import { Button } from "@multica/ui/components/ui/button";
import { Input } from "@multica/ui/components/ui/input";
import { Textarea } from "@multica/ui/components/ui/textarea";
import {
  SettingsCard,
  SettingsRow,
  SettingsSaveState,
  SettingsSection,
} from "@multica/views/settings/layout";

// SOP 3.2's operating rules, as four sections inside the workspace settings
// tab - beside the timezone and the automatic precheck switch, which are the
// two parts of 3.2 that landed before this card.
//
// Every rule lives in @multica/core/workspace: operating-rules.ts reads a
// stored value, rules-form.ts decides what may be submitted and how a value
// should read. Both have node tests. This file is wiring: which hook, which
// string, which row.
//
// Three things this page has to keep saying out loud:
//
//   - An empty box and a zero are different. "Not set" is an invitation;
//     "nothing this week" is a plan. The page never prints one as the other.
//   - A channel template is not a delivery capability. Four of the eight
//     channels cannot be handed a piece today, and the rows say so.
//   - Nothing here reminds anybody of anything. There is no scheduler behind
//     these numbers, and pretending otherwise would be the one way this page
//     could mislead somebody into missing a deadline it never had.

type Translate = ReturnType<typeof useT<"common">>["t"];

export interface OperatingRulesSectionsProps {
  wsId: string;
  /** False while the viewer cannot change workspace settings. The rows stay
   *  visible and readable - what a brand has decided is not privileged - but
   *  the inputs and the save are inert. */
  canManage?: boolean;
}

export function OperatingRulesSections({ wsId, canManage = true }: OperatingRulesSectionsProps) {
  const { t } = useT("common");
  const rules = useOperatingRules(wsId);
  const save = useSaveOperatingRules(wsId);
  // Populated from the server on first read and kept locally afterwards, so
  // an in-progress edit is not overwritten by a background refetch.
  const [draft, setDraft] = useState<RulesDraft | null>(null);

  if (rules.isPending) {
    return (
      <RuleShell title={t(($) => $.operatingRules.sectionTitle)}>
        <SettingsRow label={t(($) => $.operatingRules.loading)}>
          <span className="text-caption text-muted-foreground" />
        </SettingsRow>
      </RuleShell>
    );
  }
  if (rules.isError || !rules.data) {
    return (
      <RuleShell title={t(($) => $.operatingRules.sectionTitle)}>
        <SettingsRow label={t(($) => $.operatingRules.loadFailed)}>
          <span className="text-caption text-muted-foreground" />
        </SettingsRow>
      </RuleShell>
    );
  }

  const stored = rules.data;
  const current = draft ?? draftFromRules(stored);
  const problem = draftProblem(current);
  const ready = canSaveDraft(current) && canManage;
  const edit = (patch: (state: RulesDraft) => RulesDraft) =>
    setDraft((state) => patch(state ?? draftFromRules(stored)));

  const submit = () => {
    if (!ready) return;
    save.mutate(draftToRules(current, stored.reviewRule), {
      // The draft goes away on success, so what is on screen from here on is
      // the server's answer rather than a local copy of it.
      onSuccess: () => setDraft(null),
    });
  };

  return (
    <>
      <CadenceSection rules={stored} draft={current} edit={edit} canManage={canManage} />
      <TemplateSection draft={current} edit={edit} canManage={canManage} />
      <ReviewRuleSection rules={stored} />
      <ObservationSection rules={stored} draft={current} edit={edit} canManage={canManage} />

      <SettingsSection title={t(($) => $.operatingRules.saveTitle)}>
        <SettingsCard>
          <SettingsRow
            label={t(($) => $.operatingRules.saveLabel)}
            description={problem ? problemLabel(t, problem) : t(($) => $.operatingRules.saveHint)}
          >
            <div className="flex items-center gap-2">
              <SettingsSaveState
                status={save.isPending ? "saving" : save.isError ? "error" : "idle"}
                savingLabel={t(($) => $.operatingRules.saving)}
                savedLabel={t(($) => $.operatingRules.saved)}
                errorLabel={t(($) => $.operatingRules.failed)}
              />
              <Button onClick={submit} disabled={!ready || save.isPending}>
                {t(($) => $.operatingRules.save)}
              </Button>
            </div>
          </SettingsRow>
        </SettingsCard>
      </SettingsSection>
    </>
  );
}

function RuleShell({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <SettingsSection title={title}>
      <SettingsCard>{children}</SettingsCard>
    </SettingsSection>
  );
}

// ----------------------------------------------------------------- cadence

function CadenceSection({
  rules,
  draft,
  edit,
  canManage,
}: {
  rules: OperatingRules;
  draft: RulesDraft;
  edit: (patch: (state: RulesDraft) => RulesDraft) => void;
  canManage: boolean;
}) {
  const { t } = useT("common");
  return (
    <SettingsSection
      title={t(($) => $.operatingRules.cadenceTitle)}
      // SOP 3.2: "排期用于提醒运营者手动处理，不触发平台发布". This build does
      // not even do the reminding, and the row below says so rather than
      // leaving somebody to assume a number they typed will chase them.
      description={t(($) => $.operatingRules.cadenceHint)}
    >
      <SettingsCard>
        <SettingsRow label={t(($) => $.operatingRules.noReminders)}>
          <span className="text-caption text-muted-foreground" />
        </SettingsRow>
        {RULE_PLATFORMS.map((platform) => (
          <SettingsRow
            key={platform}
            label={platformLabel(t, platform)}
            // The whole reason readCadence returns `stored`: an empty box and
            // a zero must not print the same.
            description={cadenceLabel(t, rules, platform)}
            size="text"
          >
            <Input
              value={draft.cadence[platform] ?? ""}
              disabled={!canManage}
              inputMode="numeric"
              onChange={(event) => {
                const next = event.target.value;
                edit((state) => ({ ...state, cadence: { ...state.cadence, [platform]: next } }));
              }}
              placeholder={t(($) => $.operatingRules.cadencePlaceholder)}
              aria-label={platformLabel(t, platform)}
            />
          </SettingsRow>
        ))}
      </SettingsCard>
    </SettingsSection>
  );
}

function cadenceLabel(t: Translate, rules: OperatingRules, platform: string): string {
  const display = cadenceDisplay(rules, platform);
  switch (display.kind) {
    case "unset":
      return t(($) => $.operatingRules.cadenceUnset);
    case "paused":
      return t(($) => $.operatingRules.cadencePaused);
    case "count":
      return t(($) => $.operatingRules.cadenceCount, { n: display.value });
    default:
      // A kind a newer core added. Saying nothing beats guessing.
      return "";
  }
}

// --------------------------------------------------------------- templates

function TemplateSection({
  draft,
  edit,
  canManage,
}: {
  draft: RulesDraft;
  edit: (patch: (state: RulesDraft) => RulesDraft) => void;
  canManage: boolean;
}) {
  const { t } = useT("common");
  return (
    <SettingsSection
      title={t(($) => $.operatingRules.templatesTitle)}
      description={t(($) => $.operatingRules.templatesHint)}
    >
      <SettingsCard>
        {/* Eight channels can hold a note; four of them cannot be handed a
            piece today. Saying so once here, and again on each row that is
            affected, is cheaper than somebody concluding that writing a
            template published something. */}
        <SettingsRow label={t(($) => $.operatingRules.templatesNotDelivery)}>
          <span className="text-caption text-muted-foreground" />
        </SettingsRow>
        <SettingsRow label={t(($) => $.operatingRules.templatesNoCredentials)}>
          <span className="text-caption text-muted-foreground" />
        </SettingsRow>
        {RULE_PLATFORMS.map((platform) => (
          <SettingsRow
            key={platform}
            label={platformLabel(t, platform)}
            description={
              isDeliverable(platform)
                ? undefined
                : t(($) => $.operatingRules.templateUndeliverable)
            }
            size="text"
            align="start"
          >
            <Textarea
              value={draft.templates[platform] ?? ""}
              disabled={!canManage}
              onChange={(event) => {
                const next = event.target.value;
                edit((state) => ({ ...state, templates: { ...state.templates, [platform]: next } }));
              }}
              placeholder={t(($) => $.operatingRules.templatePlaceholder)}
              aria-label={platformLabel(t, platform)}
              rows={2}
            />
          </SettingsRow>
        ))}
      </SettingsCard>
    </SettingsSection>
  );
}

// ------------------------------------------------------------- review rule

/**
 * SOP 3.2: "个人默认自己审核。后续团队模式可要求不同成员复核."
 *
 * Read-only, and with no member picker anywhere - not even a disabled one. A
 * greyed-out control is a promise, and nothing behind this page could keep it.
 */
function ReviewRuleSection({ rules }: { rules: OperatingRules }) {
  const { t } = useT("common");
  return (
    <SettingsSection
      title={t(($) => $.operatingRules.reviewTitle)}
      description={t(($) => $.operatingRules.reviewHint)}
    >
      <SettingsCard>
        <SettingsRow label={t(($) => $.operatingRules.reviewCurrent)}>
          <span className="text-caption text-foreground">{reviewRuleLabel(t, rules.reviewRule)}</span>
        </SettingsRow>
        <SettingsRow
          label={t(($) => $.operatingRules.reviewTeamLabel)}
          description={t(($) => $.operatingRules.reviewTeamLater)}
        >
          <span className="text-caption text-muted-foreground" />
        </SettingsRow>
      </SettingsCard>
    </SettingsSection>
  );
}

// ------------------------------------------------------------- observation

function ObservationSection({
  rules,
  draft,
  edit,
  canManage,
}: {
  rules: OperatingRules;
  draft: RulesDraft;
  edit: (patch: (state: RulesDraft) => RulesDraft) => void;
  canManage: boolean;
}) {
  const { t } = useT("common");
  return (
    <SettingsSection
      title={t(($) => $.operatingRules.observationTitle)}
      description={t(($) => $.operatingRules.observationHint)}
    >
      <SettingsCard>
        <SettingsRow label={t(($) => $.operatingRules.noReminders)}>
          <span className="text-caption text-muted-foreground" />
        </SettingsRow>
        <SettingsRow
          label={t(($) => $.operatingRules.observationDefaultLabel)}
          description={t(($) => $.operatingRules.observationDefaultHint)}
          size="text"
        >
          <Input
            value={draft.observationDefault}
            disabled={!canManage}
            inputMode="numeric"
            onChange={(event) => {
              const next = event.target.value;
              edit((state) => ({ ...state, observationDefault: next }));
            }}
            placeholder={t(($) => $.operatingRules.observationPlaceholder)}
            aria-label={t(($) => $.operatingRules.observationDefaultLabel)}
          />
        </SettingsRow>
        {RULE_PLATFORMS.map((platform) => (
          <SettingsRow
            key={platform}
            label={platformLabel(t, platform)}
            // Says WHERE the window came from. Showing "7" without saying
            // whether it is this channel's own or the brand's leaves the
            // reader to guess, and the two lead to different edits.
            description={observationLabel(t, rules, platform)}
            size="text"
          >
            <Input
              value={draft.observationByChannel[platform] ?? ""}
              disabled={!canManage}
              inputMode="numeric"
              onChange={(event) => {
                const next = event.target.value;
                edit((state) => ({
                  ...state,
                  observationByChannel: { ...state.observationByChannel, [platform]: next },
                }));
              }}
              placeholder={t(($) => $.operatingRules.observationChannelPlaceholder)}
              aria-label={platformLabel(t, platform)}
            />
          </SettingsRow>
        ))}
      </SettingsCard>
    </SettingsSection>
  );
}

function observationLabel(t: Translate, rules: OperatingRules, platform: string): string {
  const display = observationDisplay(rules, platform);
  if (display.kind === "unset") return t(($) => $.operatingRules.observationUnset);
  if (display.source === "channel") {
    return t(($) => $.operatingRules.observationFromChannel, { days: display.days });
  }
  return t(($) => $.operatingRules.observationFromGlobal, { days: display.days });
}

// ------------------------------------------------------------------ shared

function problemLabel(t: Translate, problem: RulesDraftProblem): string {
  const reason =
    problem.reason === "negative"
      ? t(($) => $.operatingRules.problems.negative)
      : problem.reason === "not-a-whole-number"
        ? t(($) => $.operatingRules.problems.notAWholeNumber)
        : t(($) => $.operatingRules.problems.notANumber);
  return `${fieldLabel(t, problem.field)}: ${reason}`;
}

/** Turns a server-shaped field name into something on this page. The names
 *  match what the server puts in its 400, so a refusal that does get through
 *  points at the same row this would. */
function fieldLabel(t: Translate, field: string): string {
  const [section, platform] = field.split(".");
  if (section === "cadence" && platform) {
    return `${t(($) => $.operatingRules.cadenceTitle)} · ${platformLabel(t, platform)}`;
  }
  if (field === "observation.default") {
    return t(($) => $.operatingRules.observationDefaultLabel);
  }
  if (field.startsWith("observation.by_channel.")) {
    const channel = field.slice("observation.by_channel.".length);
    return `${t(($) => $.operatingRules.observationTitle)} · ${platformLabel(t, channel)}`;
  }
  return field;
}

// A value this build has not heard of still has to render as something. The
// raw value is a worse label than a translated one and a better one than a
// blank.
function platformLabel(t: Translate, platform: string): string {
  switch (platform) {
    case "xiaohongshu":
      return t(($) => $.operatingRules.platforms.xiaohongshu);
    case "douyin":
      return t(($) => $.operatingRules.platforms.douyin);
    case "wechat_mp":
      return t(($) => $.operatingRules.platforms.wechat_mp);
    case "bilibili":
      return t(($) => $.operatingRules.platforms.bilibili);
    case "zhihu":
      return t(($) => $.operatingRules.platforms.zhihu);
    case "weibo":
      return t(($) => $.operatingRules.platforms.weibo);
    case "kuaishou":
      return t(($) => $.operatingRules.platforms.kuaishou);
    case "shipinhao":
      return t(($) => $.operatingRules.platforms.shipinhao);
    default:
      return platform;
  }
}

function reviewRuleLabel(t: Translate, rule: string): string {
  switch (rule) {
    case "self":
      return t(($) => $.operatingRules.reviewSelf);
    default:
      return rule;
  }
}
