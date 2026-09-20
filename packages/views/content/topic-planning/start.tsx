"use client";

import { useState } from "react";
import { useT } from "@multica/views/i18n";
import {
  CONTENT_SCOPES,
  getAccountScope,
  useAccountProfile,
  useContentAccounts,
  useSetAccountScope,
  SAVED,
  saveOutcome,
  type ContentScope,
  type SaveOutcome,
} from "@multica/core/content/ip-profile";
import {
  canSubmitStart,
  latestRevision,
  pendingSnapshotRows,
  recordedSnapshotRows,
  scopeDiffersFromPreference,
  sortedRevisions,
  startBlocker,
  startFormDraft,
  useBriefStarts,
  useContentBriefs,
  useStartBrief,
  useStartSnapshot,
  type PendingSnapshotField,
  type StartFormDraft,
  type TopicCard,
} from "@multica/core/content/topic-planning";
import {
  isAutoPrecheckEnabled,
  useWorkspaceList,
} from "@multica/core/workspace";
import { Button } from "@multica/ui/components/ui/button";
import { Input } from "@multica/ui/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@multica/ui/components/ui/select";
import {
  SettingsCard,
  SettingsRow,
  SettingsSaveState,
  SettingsSection,
  type SettingsSaveStatus,
} from "@multica/views/settings/layout";

// "Start a run" (EP-04b): fix the configuration this piece of work begins from.
//
// The block sits under the card's decision actions, which is where the two
// meanings of "start" meet: EP-04a's action accepts the topic and freezes the
// first brief, and this one takes a brief revision up for work. They are
// different verbs on the same card, so they are adjacent and separately
// labelled rather than merged.
//
// Everything here is an existing control. The three things this block is
// careful about — a readiness gap named field by field, a scope that is
// seeded from the account but belongs to this start, and nine snapshot fields
// that have no source yet — are shown as rows, not as invented widgets.

type Translate = ReturnType<typeof useT<"common">>["t"];

function pendingFieldLabel(t: Translate, field: PendingSnapshotField): string {
  switch (field) {
    case "config_version":
      return t(($) => $.contentTopics.start.pending.config_version);
    case "sop_version":
      return t(($) => $.contentTopics.start.pending.sop_version);
    case "skill_version":
      return t(($) => $.contentTopics.start.pending.skill_version);
    case "rule_version":
      return t(($) => $.contentTopics.start.pending.rule_version);
    case "executor_version":
      return t(($) => $.contentTopics.start.pending.executor_version);
    case "required_sources":
      return t(($) => $.contentTopics.start.pending.required_sources);
    case "excluded_sources":
      return t(($) => $.contentTopics.start.pending.excluded_sources);
    case "grants":
      return t(($) => $.contentTopics.start.pending.grants);
    default:
      return t(($) => $.contentTopics.start.pending.file_hashes);
  }
}

function pendingFieldReason(t: Translate, field: PendingSnapshotField): string {
  switch (field) {
    case "required_sources":
    case "excluded_sources":
      return t(($) => $.contentTopics.start.reasons.material);
    case "file_hashes":
      return t(($) => $.contentTopics.start.reasons.files);
    case "grants":
      return t(($) => $.contentTopics.start.reasons.grants);
    default:
      return t(($) => $.contentTopics.start.reasons.versions);
  }
}

function snapshotFieldLabel(t: Translate, field: string): string {
  switch (field) {
    case "source_scope":
      return t(($) => $.contentTopics.start.fields.source_scope);
    case "saved_preference":
      return t(($) => $.contentTopics.start.fields.saved_preference);
    case "persona_ref":
      return t(($) => $.contentTopics.start.fields.persona_ref);
    case "executor":
      return t(($) => $.contentTopics.start.fields.executor);
    case "temperature":
      return t(($) => $.contentTopics.start.fields.temperature);
    case "budget":
      return t(($) => $.contentTopics.start.fields.budget);
    case "timeout_ms":
      return t(($) => $.contentTopics.start.fields.timeout_ms);
    case "auto_precheck":
      return t(($) => $.contentTopics.start.fields.auto_precheck);
    default:
      return t(($) => $.contentTopics.start.fields.uses_neutral_expression);
  }
}

function scopeLabel(t: Translate, scope: string): string {
  switch (scope) {
    case "local":
      return t(($) => $.contentTopics.start.scopes.local);
    case "web":
      return t(($) => $.contentTopics.start.scopes.web);
    case "all":
      return t(($) => $.contentTopics.start.scopes.all);
    // A scope a newer server introduced still renders as what it is.
    default:
      return scope || t(($) => $.contentTopics.start.notRecorded);
  }
}

function readOnly(value: string, fallback: string) {
  return (
    <span className="block break-words text-body text-muted-foreground">
      {value.trim() === "" ? fallback : value}
    </span>
  );
}

function saveStatusOf(
  outcome: SaveOutcome | null,
  pending: boolean,
): SettingsSaveStatus {
  if (pending) return "saving";
  if (!outcome) return "idle";
  return outcome.kind === "saved" ? "saved" : "error";
}

export interface StartRunSectionProps {
  wsId: string;
  card: TopicCard;
  /** The brand's accounts, for naming the one this card is written for. */
  accountOptions: { value: string; label: string }[];
}

export function StartRunSection({
  wsId,
  card,
  accountOptions,
}: StartRunSectionProps) {
  const { t } = useT("common");
  const accountId = card.accountId ?? "";
  const briefs = useContentBriefs(wsId, card.topicCardId);
  const revisions = sortedRevisions(briefs.data ?? []);
  const newest = latestRevision(revisions);
  const profile = useAccountProfile(wsId, accountId);
  const { workspaces } = useWorkspaceList();
  const workspace = workspaces.find((item) => item.id === wsId);
  const account = accountOptions.find((option) => option.value === accountId);
  // The preference lives in the account's settings blob, which the profile
  // response does not carry; the accounts query is already cached by the page
  // above, so reading it here costs nothing.
  const accounts = useContentAccounts(wsId);
  const savedPreference = getAccountScope(
    (accounts.data ?? []).find((item) => item.account_id === accountId),
  );
  const [draft, setDraft] = useState<StartFormDraft | null>(null);
  const form =
    draft ??
    startFormDraft({
      savedPreference,
      latestRevisionId: newest?.briefRevisionId ?? "",
    });
  const edit = (patch: Partial<StartFormDraft>) =>
    setDraft({ ...form, ...patch });

  const start = useStartBrief(wsId, card.topicCardId);
  const rememberScope = useSetAccountScope(wsId);
  const [outcome, setOutcome] = useState<SaveOutcome | null>(null);
  const [startedId, setStartedId] = useState("");

  const readiness = profile.data?.readiness;
  const canStart = readiness?.can_start === true;
  const blocker = startBlocker({
    accountId,
    revisionId: form.revisionId,
    canStart,
  });

  const submit = () => {
    setOutcome(null);
    start.mutate(
      {
        revisionId: form.revisionId,
        accountId,
        sourceScope: form.sourceScope,
        projectId: form.projectId,
      },
      {
        onSuccess: (snapshot) => {
          setOutcome(SAVED);
          setStartedId(snapshot.snapshotId);
          // "Remember what was chosen last time". Deliberately after the
          // start and deliberately not awaited: the snapshot has already
          // recorded the scope it used, so a failure here costs the next
          // start's default and nothing that was just fixed.
          if (form.sourceScope !== savedPreference) {
            rememberScope.mutate({ accountId, scope: form.sourceScope });
          }
        },
        onError: (error) => setOutcome(saveOutcome(error)),
      },
    );
  };

  return (
    <>
      <SettingsSection
        title={t(($) => $.contentTopics.start.title)}
        description={t(($) => $.contentTopics.start.description)}
      >
        <SettingsCard>
          <SettingsRow
            label={t(($) => $.contentTopics.start.account)}
            description={
              accountId
                ? t(($) => $.contentTopics.start.accountHint)
                : t(($) => $.contentTopics.start.accountMissing)
            }
          >
            {readOnly(
              account?.label ?? accountId,
              t(($) => $.contentTopics.noAccount),
            )}
          </SettingsRow>

          <ReadinessRow
            accountId={accountId}
            canStart={canStart}
            missing={readiness?.missing ?? []}
            // A failed profile read is not a readiness verdict. Without this
            // the row would say "cannot start" and name no field, because the
            // missing list is empty for the same reason the readiness is:
            // nothing was read.
            failed={profile.isError}
            onRetry={() => void profile.refetch()}
          />

          <SettingsRow
            label={t(($) => $.contentTopics.start.revision)}
            description={t(($) => $.contentTopics.start.revisionHint)}
            size="select-wide"
          >
            <Select
              items={revisions.map((revision) => ({
                value: revision.briefRevisionId,
                label: t(($) => $.contentTopics.revision, {
                  number: revision.revision,
                }),
              }))}
              value={form.revisionId}
              onValueChange={(value) => edit({ revisionId: value ?? "" })}
            >
              <SelectTrigger
                aria-label={t(($) => $.contentTopics.start.revision)}
              >
                <SelectValue
                  placeholder={t(($) => $.contentTopics.briefEmpty)}
                />
              </SelectTrigger>
              <SelectContent>
                {revisions.map((revision) => (
                  <SelectItem
                    key={revision.briefRevisionId}
                    value={revision.briefRevisionId}
                  >
                    {t(($) => $.contentTopics.revision, {
                      number: revision.revision,
                    })}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </SettingsRow>

          <SettingsRow
            label={t(($) => $.contentTopics.start.scope)}
            description={t(($) => $.contentTopics.start.scopeHint, {
              preference: scopeLabel(t, savedPreference),
            })}
            size="select-wide"
          >
            <Select
              items={CONTENT_SCOPES.map((scope) => ({
                value: scope,
                label: scopeLabel(t, scope),
              }))}
              value={form.sourceScope}
              onValueChange={(value) =>
                edit({
                  sourceScope: (value ?? savedPreference) as ContentScope,
                })
              }
            >
              <SelectTrigger aria-label={t(($) => $.contentTopics.start.scope)}>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {CONTENT_SCOPES.map((scope) => (
                  <SelectItem key={scope} value={scope}>
                    {scopeLabel(t, scope)}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </SettingsRow>

          <SettingsRow
            label={t(($) => $.contentTopics.start.project)}
            description={t(($) => $.contentTopics.start.projectHint)}
            size="text"
          >
            <Input
              value={form.projectId}
              onChange={(event) => edit({ projectId: event.target.value })}
              placeholder={t(($) => $.contentTopics.start.projectPlaceholder)}
            />
          </SettingsRow>

          <SettingsRow label={t(($) => $.contentTopics.start.precheck)}>
            {readOnly(
              isAutoPrecheckEnabled(workspace)
                ? t(($) => $.contentTopics.start.precheckOn)
                : t(($) => $.contentTopics.start.precheckOff),
              "",
            )}
          </SettingsRow>
          <SettingsRow
            label={t(($) => $.contentTopics.start.neutral)}
            description={t(($) => $.contentTopics.start.neutralHint)}
          >
            {readOnly(
              profile.data?.usesNeutralExpression
                ? t(($) => $.contentTopics.start.neutralOn)
                : t(($) => $.contentTopics.start.neutralOff),
              "",
            )}
          </SettingsRow>

          <SettingsRow
            label={t(($) => $.contentTopics.start.run)}
            description={blockerHint(t, blocker)}
          >
            <div className="flex items-center gap-3">
              <SettingsSaveState
                status={saveStatusOf(outcome, start.isPending)}
                savingLabel={t(($) => $.contentTopics.start.starting)}
                savedLabel={t(($) => $.contentTopics.start.started)}
                errorLabel={
                  outcome?.kind === "failed" && outcome.detail.nextAction
                    ? `${t(($) => $.contentTopics.start.failed)} · ${t(($) => $.contentTopics.nextAction, { action: outcome.detail.nextAction })}`
                    : t(($) => $.contentTopics.start.failed)
                }
              />
              <Button
                onClick={submit}
                disabled={
                  !canSubmitStart({
                    accountId,
                    revisionId: form.revisionId,
                    canStart,
                    pending: start.isPending,
                  })
                }
              >
                {t(($) => $.contentTopics.start.run)}
              </Button>
            </div>
          </SettingsRow>
        </SettingsCard>
      </SettingsSection>

      <PendingInputsSection />

      <StartHistorySection
        wsId={wsId}
        card={card}
        revisionId={form.revisionId}
        startedId={startedId}
      />
    </>
  );
}

function blockerHint(
  t: Translate,
  blocker: ReturnType<typeof startBlocker>,
): string {
  switch (blocker) {
    case "account":
      return t(($) => $.contentTopics.start.blockedAccount);
    case "revision":
      return t(($) => $.contentTopics.start.blockedRevision);
    case "readiness":
      return t(($) => $.contentTopics.start.blockedReadiness);
    default:
      return t(($) => $.contentTopics.start.notIdempotent);
  }
}

// The four minimum conditions, named one by one. "Configuration incomplete"
// would leave a person to guess which of them it meant, and the four already
// have names on the account page — the same keys are used here.
function ReadinessRow({
  accountId,
  canStart,
  missing,
  failed,
  onRetry,
}: {
  accountId: string;
  canStart: boolean;
  missing: string[];
  failed: boolean;
  onRetry: () => void;
}) {
  const { t } = useT("common");
  if (accountId && failed) {
    return (
      <SettingsRow
        label={t(($) => $.contentTopics.start.readiness)}
        description={t(($) => $.contentAccounts.expressionProfile.readFailedHint)}
      >
        <Button variant="outline" onClick={onRetry}>
          {t(($) => $.contentAccounts.expressionProfile.retry)}
        </Button>
      </SettingsRow>
    );
  }
  if (!accountId) {
    return (
      <SettingsRow label={t(($) => $.contentTopics.start.readiness)}>
        {readOnly(
          "",
          t(($) => $.contentTopics.start.readinessNoAccount),
        )}
      </SettingsRow>
    );
  }
  if (canStart) {
    return (
      <SettingsRow label={t(($) => $.contentTopics.start.readiness)}>
        <span className="block text-body text-muted-foreground">
          {t(($) => $.contentAccounts.expressionProfile.readyYes)}
        </span>
      </SettingsRow>
    );
  }
  const names = missing.map((key) =>
    t(($) => $.contentAccounts.expressionProfile.fields[key as "audience"]),
  );
  return (
    <SettingsRow
      label={t(($) => $.contentTopics.start.readiness)}
      description={t(($) => $.contentTopics.start.readinessFix)}
      align="start"
    >
      <span className="block text-body text-muted-foreground">
        {names.length === 0
          ? t(($) => $.contentTopics.start.readinessUnknown)
          : t(($) => $.contentAccounts.expressionProfile.readyNo, {
              fields: names.join(
                t(($) => $.contentAccounts.expressionProfile.fieldSeparator),
              ),
            })}
      </span>
    </SettingsRow>
  );
}

// The nine snapshot fields nothing fills yet, each with the reason. Named
// rather than hidden: a blank row with no explanation reads as a bug, and an
// invented value would be worse than either.
function PendingInputsSection() {
  const { t } = useT("common");
  return (
    <SettingsSection
      title={t(($) => $.contentTopics.start.pendingTitle)}
      description={t(($) => $.contentTopics.start.pendingDescription)}
    >
      <SettingsCard>
        {pendingSnapshotRows(undefined).map((row) => (
          <SettingsRow
            key={row.field}
            label={pendingFieldLabel(t, row.field)}
            description={pendingFieldReason(t, row.field)}
          >
            <span className="text-caption text-muted-foreground">
              {t(($) => $.contentTopics.unavailable.badge)}
            </span>
          </SettingsRow>
        ))}
      </SettingsCard>
    </SettingsSection>
  );
}

// Every start of the selected revision, and what one of them fixed.
function StartHistorySection({
  wsId,
  card,
  revisionId,
  startedId,
}: {
  wsId: string;
  card: TopicCard;
  revisionId: string;
  startedId: string;
}) {
  const { t } = useT("common");
  const starts = useBriefStarts(wsId, card.topicCardId, revisionId);
  const [viewingId, setViewingId] = useState("");
  const selectedId = viewingId || startedId;
  const snapshot = useStartSnapshot(wsId, card.topicCardId, selectedId);
  const list = starts.data ?? [];

  return (
    <>
      <SettingsSection
        title={t(($) => $.contentTopics.start.historyTitle)}
        description={t(($) => $.contentTopics.start.historyDescription, {
          count: list.length,
        })}
      >
        <SettingsCard>
          {starts.isError ? (
            <SettingsRow label={t(($) => $.contentTopics.start.historyFailed)}>
              <span className="text-caption text-muted-foreground" />
            </SettingsRow>
          ) : list.length === 0 ? (
            <SettingsRow label={t(($) => $.contentTopics.start.historyEmpty)}>
              <span className="text-caption text-muted-foreground" />
            </SettingsRow>
          ) : (
            list.map((item) => (
              <SettingsRow
                key={item.snapshotId}
                label={
                  <button
                    type="button"
                    onClick={() => setViewingId(item.snapshotId)}
                    data-active={item.snapshotId === selectedId}
                    className="text-left text-body data-[active=true]:font-semibold hover:text-foreground"
                  >
                    {item.createdAt || item.snapshotId}
                  </button>
                }
                description={scopeLabel(t, item.snapshot.source_scope)}
              >
                <span className="text-caption text-muted-foreground">
                  {item.projectId || t(($) => $.contentTopics.start.noProject)}
                </span>
              </SettingsRow>
            ))
          )}
        </SettingsCard>
      </SettingsSection>

      {selectedId && snapshot.data?.snapshotId ? (
        <SettingsSection
          title={t(($) => $.contentTopics.start.snapshotTitle)}
          description={t(($) => $.contentTopics.start.snapshotDescription)}
        >
          <SettingsCard>
            {recordedSnapshotRows(snapshot.data.snapshot).map((row) => (
              <SettingsRow
                key={row.field}
                label={snapshotFieldLabel(t, row.field)}
                description={
                  row.extension
                    ? t(($) => $.contentTopics.start.extensionKey)
                    : undefined
                }
              >
                {readOnly(
                  row.field === "source_scope" ||
                    row.field === "saved_preference"
                    ? scopeLabel(t, row.value)
                    : row.value,
                  t(($) => $.contentTopics.start.notRecorded),
                )}
              </SettingsRow>
            ))}
            {scopeDiffersFromPreference(snapshot.data.snapshot) ? (
              <SettingsRow label={t(($) => $.contentTopics.start.scopeDiffers)}>
                <span className="text-caption text-muted-foreground" />
              </SettingsRow>
            ) : null}
          </SettingsCard>
        </SettingsSection>
      ) : null}
    </>
  );
}
