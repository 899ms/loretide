"use client";

import { useState } from "react";
import { useT } from "@multica/views/i18n";
import {
  PROFILE_GROUPS,
  SAVED,
  confirmGroup,
  draftToProfile,
  emptyProfileRead,
  isListField,
  profileReadState,
  isPendingWithValue,
  profileToDraft,
  saveOutcome,
  useAccountProfile,
  useSetAccountProfile,
  type ExpressionProfile,
  type ProfileDraft,
  type ProfileFieldKey,
  type ProfileGroup,
  type SaveOutcome,
} from "@multica/core/content/ip-profile";
import { Button } from "@multica/ui/components/ui/button";
import { Input } from "@multica/ui/components/ui/input";
import { Textarea } from "@multica/ui/components/ui/textarea";
import {
  SettingsCard,
  SettingsRow,
  SettingsSaveState,
  SettingsSection,
} from "@multica/views/settings/layout";

// SOP 3.1's expression profile, on the account settings page.
//
// Same composition as the rest of the page - SettingsSection, SettingsCard,
// SettingsRow, Textarea/Input, Button, SettingsSaveState - and no new controls.
// Everything that is a rule rather than a layout (grouping, what editing does
// to a confirmation, how a list survives a text box) lives in
// @multica/core/content/ip-profile/profile-draft.ts and is tested there.

type Translate = ReturnType<typeof useT<"common">>["t"];

export function ExpressionProfileSections({
  wsId,
  accountId,
}: {
  wsId: string;
  accountId: string;
}) {
  const { t } = useT("common");
  const profileRead = useAccountProfile(wsId, accountId);
  const setProfile = useSetAccountProfile(wsId);
  const [draft, setDraft] = useState<ProfileDraft | null>(null);
  const [confirming, setConfirming] = useState<string>("");
  const [outcome, setOutcome] = useState<SaveOutcome | null>(null);

  const state = profileReadState({
    enabled: !!accountId,
    isPending: profileRead.isPending,
    isError: profileRead.isError,
    data: profileRead.data,
  });

  // Until the read lands the page shows an all-pending profile, which is also
  // what a real account with no revisions reads as. One rendering, one shape.
  const server = profileRead.data ?? emptyProfileRead();
  const stored = server.profile;
  // The draft is only what the creator has typed since the last confirmation.
  // Dropping it on success is what makes the screen show the server's answer
  // rather than a local copy of it.
  const current = draft ?? profileToDraft(stored);
  const edited = draftToProfile(stored, current);

  const confirm = (group: ProfileGroup) => {
    setOutcome(null);
    setConfirming(group.id);
    setProfile.mutate(
      { accountId, profile: confirmGroup(edited, group) },
      {
        onSuccess: () => {
          setOutcome(SAVED);
          setDraft(null);
          setConfirming("");
        },
        onError: (error) => {
          setOutcome(saveOutcome(error));
          setConfirming("");
        },
      },
    );
  };

  // A failed read does not fall back to the empty profile. Rendering the form
  // against a profile nobody read would let someone confirm a group and append
  // a revision of blanks over fields that are actually filled in - the read
  // failed, so what is on screen is not the account's.
  if (state === "failed") {
    return (
      <SettingsSection
        title={t(($) => $.contentAccounts.expressionProfile.sectionTitle)}
        description={t(($) => $.contentAccounts.expressionProfile.readFailedHint)}
      >
        <SettingsCard>
          <SettingsRow label={t(($) => $.contentAccounts.expressionProfile.readFailed)}>
            <Button variant="outline" onClick={() => void profileRead.refetch()}>
              {t(($) => $.contentAccounts.expressionProfile.retry)}
            </Button>
          </SettingsRow>
        </SettingsCard>
      </SettingsSection>
    );
  }

  return (
    <>
      <SettingsSection
        title={t(($) => $.contentAccounts.expressionProfile.sectionTitle)}
        description={t(($) => $.contentAccounts.expressionProfile.sectionDescription)}
      >
        <SettingsCard>
          <SettingsRow
            label={t(($) => $.contentAccounts.expressionProfile.readyTitle)}
            description={readinessLine(t, server.readiness.can_start, server.readiness.missing)}
          >
            <span className="text-caption text-muted-foreground" />
          </SettingsRow>
          <SettingsRow
            label={t(($) => $.contentAccounts.expressionProfile.neutralTitle)}
            description={
              server.usesNeutralExpression
                ? t(($) => $.contentAccounts.expressionProfile.neutralOnHint)
                : null
            }
          >
            <span className="text-caption text-muted-foreground">
              {server.usesNeutralExpression
                ? t(($) => $.contentAccounts.expressionProfile.neutralOn)
                : t(($) => $.contentAccounts.expressionProfile.neutralOff)}
            </span>
          </SettingsRow>
          <SettingsRow
            label={t(($) => $.contentAccounts.expressionProfile.extractTitle)}
            description={t(($) => $.contentAccounts.expressionProfile.extractUnavailable)}
          >
            {/* Phase 1 runs no executors. The entry point exists so the flow is
                visible and so EP-08 has somewhere to land; it does nothing and
                says why rather than being hidden. */}
            <Button variant="outline" disabled>
              {t(($) => $.contentAccounts.expressionProfile.extractTitle)}
            </Button>
          </SettingsRow>
        </SettingsCard>
      </SettingsSection>

      {PROFILE_GROUPS.map((group) => (
        <SettingsSection
          key={group.id}
          title={t(($) => $.contentAccounts.expressionProfile.groups[group.id])}
        >
          <SettingsCard>
            {group.fields.map((key) => (
              <ProfileField
                key={key}
                fieldKey={key}
                value={current[key] ?? ""}
                edited={edited}
                onChange={(value) => setDraft({ ...current, [key]: value })}
              />
            ))}
            <SettingsRow label={t(($) => $.contentAccounts.expressionProfile.confirm)}>
              <div className="flex items-center gap-3">
                {confirming === group.id || outcome ? (
                  <ProfileSaveState
                    outcome={confirming === group.id ? null : outcome}
                    pending={confirming === group.id}
                  />
                ) : null}
                <Button
                  onClick={() => confirm(group)}
                  disabled={setProfile.isPending || !accountId}
                >
                  {t(($) => $.contentAccounts.expressionProfile.confirm)}
                </Button>
              </div>
            </SettingsRow>
          </SettingsCard>
        </SettingsSection>
      ))}
    </>
  );
}

function ProfileField({
  fieldKey,
  value,
  edited,
  onChange,
}: {
  fieldKey: ProfileFieldKey;
  value: string;
  edited: ExpressionProfile;
  onChange: (value: string) => void;
}) {
  const { t } = useT("common");
  const label = t(($) => $.contentAccounts.expressionProfile.fields[fieldKey]);
  const status = statusLabel(t, edited, fieldKey);
  const hours = fieldKey === "weekly_hours";

  return (
    <SettingsRow
      label={label}
      description={
        isListField(fieldKey) ? (
          <>
            {status}
            {" · "}
            {t(($) => $.contentAccounts.expressionProfile.listHint)}
          </>
        ) : (
          status
        )
      }
      size="text"
      align={hours ? "center" : "start"}
    >
      {hours ? (
        <Input
          type="number"
          min={0}
          max={168}
          inputMode="numeric"
          value={value}
          onChange={(event) => onChange(event.target.value)}
          placeholder={t(($) => $.contentAccounts.expressionProfile.hoursPlaceholder)}
          aria-label={label}
        />
      ) : (
        <Textarea
          value={value}
          onChange={(event) => onChange(event.target.value)}
          placeholder={t(
            ($) => $.contentAccounts.expressionProfile.placeholders[textPlaceholderKey(fieldKey)],
          )}
          rows={isListField(fieldKey) ? 4 : 3}
          aria-label={label}
        />
      )}
    </SettingsRow>
  );
}

/** Every field except the hours box has a placeholder; hours has its own. */
type PlaceholderKey = Exclude<ProfileFieldKey, "weekly_hours">;

function textPlaceholderKey(key: ProfileFieldKey): PlaceholderKey {
  return key as PlaceholderKey;
}

function statusLabel(t: Translate, profile: ExpressionProfile, key: ProfileFieldKey): string {
  const field = profile[key];
  if (field.status === "confirmed") {
    return t(($) => $.contentAccounts.expressionProfile.statusConfirmed);
  }
  // "Typed but not confirmed" is a different sentence from "nothing here yet",
  // because only one of them is waiting on a click.
  if (isPendingWithValue(field)) {
    return t(($) => $.contentAccounts.expressionProfile.statusPendingEdited);
  }
  return t(($) => $.contentAccounts.expressionProfile.statusPending);
}

function readinessLine(t: Translate, canStart: boolean, missing: string[]): string {
  if (canStart) return t(($) => $.contentAccounts.expressionProfile.readyYes);
  const names = missing.map((key) =>
    t(($) => $.contentAccounts.expressionProfile.fields[key as ProfileFieldKey]),
  );
  const separator = t(($) => $.contentAccounts.expressionProfile.fieldSeparator);
  return t(($) => $.contentAccounts.expressionProfile.readyNo, {
    fields: names.join(separator),
  });
}

function ProfileSaveState({
  outcome,
  pending,
}: {
  outcome: SaveOutcome | null;
  pending: boolean;
}) {
  const { t } = useT("common");
  const status = pending ? "saving" : !outcome ? "idle" : outcome.kind === "saved" ? "saved" : "error";
  return (
    <SettingsSaveState
      status={status}
      savingLabel={t(($) => $.contentAccounts.saving)}
      savedLabel={t(($) => $.contentAccounts.saved)}
      errorLabel={
        outcome?.kind === "conflict"
          ? t(($) => $.contentAccounts.conflict)
          : t(($) => $.contentAccounts.failed)
      }
    />
  );
}
