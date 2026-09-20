"use client";

import { useState } from "react";
import { useT } from "@multica/views/i18n";
import {
  CONTENT_PLATFORMS,
  SAVED,
  discardDraft,
  editDraft,
  emptyDraftState,
  isDirty,
  saveOutcome,
  selectDraft,
  useAccountPersona,
  useContentAccounts,
  useCreateContentAccount,
  useSetAccountPersona,
  useUpdateContentAccount,
  type AccountServerValues,
  type DraftState,
  type SaveOutcome,
} from "@multica/core/content/ip-profile";
import { Button } from "@multica/ui/components/ui/button";
import { Input } from "@multica/ui/components/ui/input";
import { Textarea } from "@multica/ui/components/ui/textarea";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@multica/ui/components/ui/select";
import {
  SettingsCard,
  SettingsContent,
  SettingsRow,
  SettingsSaveState,
  SettingsSection,
  SettingsTab,
  type SettingsSaveStatus,
} from "@multica/views/settings/layout";
import { PageHeader } from "@multica/views/layout/page-header";
import { ExpressionProfileSections } from "./expression-profile";

// The account settings page: create accounts under a brand, switch between
// them, edit the platform and the persona prompt.
//
// Composition is borrowed wholesale from settings/components/workspace-tab.tsx
// — same problem (a few fields, a save, a save state), so the same
// SettingsCard/SettingsRow/SettingsSaveState arrangement. No new controls.
//
// Deliberately NOT gated on any feature flag. Several settings tabs are gated
// on the plugins flag and copying one of them is how that gate travels; this
// page has to work with plugins off.

// The 409 outcome renders through SettingsSaveState's `error` appearance with a
// different label. The shared component has no retryable state, and adding one
// would change every settings tab; what the requirement asks for is that the
// person reads a different sentence, and the label is the caller's to supply.
function saveStatusOf(outcome: SaveOutcome | null, pending: boolean): SettingsSaveStatus {
  if (pending) return "saving";
  if (!outcome) return "idle";
  return outcome.kind === "saved" ? "saved" : "error";
}

export interface AccountSettingsPageProps {
  wsId: string;
}

export function AccountSettingsPage({ wsId }: AccountSettingsPageProps) {
  const { t } = useT("common");
  return (
    <>
      <PageHeader className="xl:hidden">
        <span className="text-body font-medium">
          {t(($) => $.contentAccounts.title)}
        </span>
      </PageHeader>
      <AccountSettingsContent wsId={wsId} />
    </>
  );
}

function AccountSettingsContent({ wsId }: AccountSettingsPageProps) {
  const { t } = useT("common");
  const accounts = useContentAccounts(wsId);
  const [selectedId, setSelectedId] = useState("");
  const [drafts, setDrafts] = useState<DraftState>(emptyDraftState);

  const list = accounts.data ?? [];
  // An id that is no longer in the list (deleted elsewhere, or a brand switch)
  // must not stay selected, or the panel would edit something that is gone.
  const selected = list.find((account) => account.account_id === selectedId) ?? null;

  const select = (accountId: string) => {
    // Leaving an account drops its unsaved draft. The next visit reads the
    // server, which is what "switching accounts does not carry input over"
    // means from the other side.
    if (selectedId && selectedId !== accountId) {
      setDrafts((state) => discardDraft(state, selectedId));
    }
    setSelectedId(accountId);
  };

  return (
    <SettingsContent>
      <SettingsTab
        title={t(($) => $.contentAccounts.title)}
        description={t(($) => $.contentAccounts.description)}
      >
        <SettingsSection title={t(($) => $.contentAccounts.listTitle)}>
          <SettingsCard>
            {accounts.isError ? (
              <SettingsRow label={t(($) => $.contentAccounts.loadFailed)}>
                <span className="text-caption text-muted-foreground" />
              </SettingsRow>
            ) : list.length === 0 ? (
              <SettingsRow label={t(($) => $.contentAccounts.empty)}>
                <span className="text-caption text-muted-foreground" />
              </SettingsRow>
            ) : (
              list.map((account) => (
                <SettingsRow
                  key={account.account_id}
                  label={
                    <button
                      type="button"
                      onClick={() => select(account.account_id)}
                      data-active={account.account_id === selectedId}
                      // The active row stays legible under hover because the
                      // weight carries the state, and hover only moves color.
                      className="text-left text-body data-[active=true]:font-semibold hover:text-foreground"
                    >
                      {account.display_name}
                    </button>
                  }
                  description={platformLabel(t, account.platform)}
                >
                  <span className="text-caption text-muted-foreground" />
                </SettingsRow>
              ))
            )}
          </SettingsCard>
        </SettingsSection>

        <CreateAccountSection wsId={wsId} onCreated={select} />

        {selected ? (
          <AccountEditor
            key={selected.account_id}
            wsId={wsId}
            accountId={selected.account_id}
            platform={selected.platform}
            displayName={selected.display_name}
            drafts={drafts}
            setDrafts={setDrafts}
          />
        ) : null}
      </SettingsTab>
    </SettingsContent>
  );
}

type Translate = ReturnType<typeof useT<"common">>["t"];

function platformLabel(t: Translate, platform: string): string {
  // A platform this build has never heard of still has to render as something.
  // The raw value is a worse label than a translated one and a better one than
  // a blank cell.
  switch (platform) {
    case "xiaohongshu": return t(($) => $.contentAccounts.platforms.xiaohongshu);
    case "douyin": return t(($) => $.contentAccounts.platforms.douyin);
    case "wechat_mp": return t(($) => $.contentAccounts.platforms.wechat_mp);
    case "bilibili": return t(($) => $.contentAccounts.platforms.bilibili);
    case "zhihu": return t(($) => $.contentAccounts.platforms.zhihu);
    case "weibo": return t(($) => $.contentAccounts.platforms.weibo);
    case "kuaishou": return t(($) => $.contentAccounts.platforms.kuaishou);
    case "shipinhao": return t(($) => $.contentAccounts.platforms.shipinhao);
    default: return platform;
  }
}

// The Select needs both `items` (for its own value formatting) and rendered
// SelectItem children, the same pairing workspace-tab.tsx uses for timezones.
function platformItems(t: Translate) {
  return CONTENT_PLATFORMS.map((value) => ({ value, label: platformLabel(t, value) }));
}

function CreateAccountSection({
  wsId,
  onCreated,
}: {
  wsId: string;
  onCreated: (accountId: string) => void;
}) {
  const { t } = useT("common");
  const create = useCreateContentAccount(wsId);
  const [platform, setPlatform] = useState<string>("");
  const [displayName, setDisplayName] = useState("");
  const [outcome, setOutcome] = useState<SaveOutcome | null>(null);

  const submit = () => {
    setOutcome(null);
    create.mutate(
      { platform, displayName },
      {
        onSuccess: (account) => {
          setOutcome(SAVED);
          setPlatform("");
          setDisplayName("");
          // Waits for the server rather than guessing an id: the selection
          // moves to a real account or not at all.
          if (account.account_id) onCreated(account.account_id);
        },
        onError: (error) => setOutcome(saveOutcome(error)),
      },
    );
  };

  return (
    <SettingsSection title={t(($) => $.contentAccounts.createTitle)}>
      <SettingsCard>
        <SettingsRow label={t(($) => $.contentAccounts.platform)} size="select-wide">
          <Select
            items={platformItems(t)}
            value={platform}
            onValueChange={(value) => setPlatform(value ?? "")}
          >
            <SelectTrigger aria-label={t(($) => $.contentAccounts.platform)}>
              <SelectValue placeholder={t(($) => $.contentAccounts.platformPlaceholder)} />
            </SelectTrigger>
            <SelectContent>
              {CONTENT_PLATFORMS.map((value) => (
                <SelectItem key={value} value={value}>
                  {platformLabel(t, value)}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </SettingsRow>
        <SettingsRow label={t(($) => $.contentAccounts.displayName)} size="text">
          <Input
            value={displayName}
            onChange={(event) => setDisplayName(event.target.value)}
            placeholder={t(($) => $.contentAccounts.displayNamePlaceholder)}
          />
        </SettingsRow>
        <SettingsRow label={t(($) => $.contentAccounts.create)}>
          <div className="flex items-center gap-3">
            <SaveFeedback outcome={outcome} pending={create.isPending} />
            <Button
              onClick={submit}
              disabled={create.isPending || !platform || !displayName.trim()}
            >
              {t(($) => $.contentAccounts.create)}
            </Button>
          </div>
        </SettingsRow>
      </SettingsCard>
    </SettingsSection>
  );
}

function AccountEditor({
  wsId,
  accountId,
  platform,
  displayName,
  drafts,
  setDrafts,
}: {
  wsId: string;
  accountId: string;
  platform: string;
  displayName: string;
  drafts: DraftState;
  setDrafts: (update: (state: DraftState) => DraftState) => void;
}) {
  const { t } = useT("common");
  const persona = useAccountPersona(wsId, accountId);
  const updateAccount = useUpdateContentAccount(wsId);
  const setPersona = useSetAccountPersona(wsId);
  const [profileOutcome, setProfileOutcome] = useState<SaveOutcome | null>(null);
  const [personaOutcome, setPersonaOutcome] = useState<SaveOutcome | null>(null);

  const server: AccountServerValues = {
    platform,
    displayName,
    personaPrompt: persona.data?.persona_prompt ?? "",
  };
  const draft = selectDraft(drafts, accountId, server);
  const edit = (patch: Partial<typeof draft>) =>
    setDrafts((state) => editDraft(state, accountId, server, patch));

  const saveProfile = () => {
    setProfileOutcome(null);
    updateAccount.mutate(
      { accountId, platform: draft.platform, displayName: draft.displayName },
      {
        onSuccess: () => setProfileOutcome(SAVED),
        onError: (error) => setProfileOutcome(saveOutcome(error)),
      },
    );
  };

  const savePersona = () => {
    setPersonaOutcome(null);
    setPersona.mutate(
      { accountId, personaPrompt: draft.personaPrompt },
      {
        onSuccess: () => {
          setPersonaOutcome(SAVED);
          // The draft goes away on success, so what is on screen from here on
          // is the server's value, not a local copy of it.
          setDrafts((state) => discardDraft(state, accountId));
        },
        onError: (error) => setPersonaOutcome(saveOutcome(error)),
      },
    );
  };

  const revision = persona.data?.revision ?? 0;

  return (
    <>
      <SettingsSection title={t(($) => $.contentAccounts.profileTitle)}>
        <SettingsCard>
          <SettingsRow label={t(($) => $.contentAccounts.platform)} size="select-wide">
            <Select
              items={platformItems(t)}
              value={draft.platform}
              onValueChange={(value) => edit({ platform: value ?? draft.platform })}
            >
              <SelectTrigger aria-label={t(($) => $.contentAccounts.platform)}>
                <SelectValue placeholder={t(($) => $.contentAccounts.platformPlaceholder)} />
              </SelectTrigger>
              <SelectContent>
                {CONTENT_PLATFORMS.map((value) => (
                  <SelectItem key={value} value={value}>
                    {platformLabel(t, value)}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </SettingsRow>
          <SettingsRow label={t(($) => $.contentAccounts.displayName)} size="text">
            <Input
              value={draft.displayName}
              onChange={(event) => edit({ displayName: event.target.value })}
              placeholder={t(($) => $.contentAccounts.displayNamePlaceholder)}
            />
          </SettingsRow>
          <SettingsRow label={t(($) => $.contentAccounts.save)}>
            <div className="flex items-center gap-3">
              <SaveFeedback outcome={profileOutcome} pending={updateAccount.isPending} />
              <Button onClick={saveProfile} disabled={updateAccount.isPending}>
                {t(($) => $.contentAccounts.save)}
              </Button>
            </div>
          </SettingsRow>
        </SettingsCard>
      </SettingsSection>

      <SettingsSection
        title={t(($) => $.contentAccounts.personaTitle)}
        description={t(($) => $.contentAccounts.personaDescription)}
      >
        <SettingsCard>
          <SettingsRow
            label={
              revision > 0
                ? t(($) => $.contentAccounts.revision, { number: revision })
                : t(($) => $.contentAccounts.noRevision)
            }
            size="text"
            align="start"
          >
            <Textarea
              value={draft.personaPrompt}
              onChange={(event) => edit({ personaPrompt: event.target.value })}
              placeholder={t(($) => $.contentAccounts.personaPlaceholder)}
              rows={8}
            />
          </SettingsRow>
          <SettingsRow label={t(($) => $.contentAccounts.save)}>
            <div className="flex items-center gap-3">
              <SaveFeedback outcome={personaOutcome} pending={setPersona.isPending} />
              {/* An empty prompt is a legitimate save, so the button is not
                  disabled on blank input — only on "nothing changed". */}
              <Button
                onClick={savePersona}
                disabled={setPersona.isPending || !isDirty(drafts, accountId, server)}
              >
                {t(($) => $.contentAccounts.save)}
              </Button>
            </div>
          </SettingsRow>
        </SettingsCard>
      </SettingsSection>

      <ExpressionProfileSections wsId={wsId} accountId={accountId} />
    </>
  );
}

function SaveFeedback({
  outcome,
  pending,
}: {
  outcome: SaveOutcome | null;
  pending: boolean;
}) {
  const { t } = useT("common");
  const status = saveStatusOf(outcome, pending);
  const errorLabel =
    outcome?.kind === "conflict"
      ? t(($) => $.contentAccounts.conflict)
      : outcome?.kind === "failed" && outcome.detail.nextAction
        ? `${t(($) => $.contentAccounts.failed)} · ${t(($) => $.contentAccounts.nextAction, { action: outcome.detail.nextAction })}`
        : t(($) => $.contentAccounts.failed);

  return (
    <SettingsSaveState
      status={status}
      savingLabel={t(($) => $.contentAccounts.saving)}
      savedLabel={t(($) => $.contentAccounts.saved)}
      errorLabel={errorLabel}
    />
  );
}
