"use client";

import { useState } from "react";
import { useT } from "@multica/views/i18n";
import { useNavigation } from "@multica/views/navigation";
import {
  SAVED,
  saveOutcome,
  type SaveOutcome,
} from "@multica/core/content/ip-profile";
import {
  MARKETING_NODE_DATE_CERTAINTIES,
  MARKETING_NODE_KINDS,
  emptyMarketingNodeForm,
  marketingNodeFormChanged,
  marketingNodeFormErrors,
  marketingNodeFormFromRevision,
  marketingNodeFormToInput,
  parseMarketingNodeCsv,
  setMarketingNodeAccount,
  setMarketingNodeAccountRole,
  setMarketingNodeMaterial,
  topicStatusKey,
  useAdoptMarketingCandidate,
  useCancelMarketingNode,
  useConfirmMarketingNode,
  useContentTopics,
  useCreateMarketingNode,
  useDecideMarketingImpact,
  useEditMarketingCandidate,
  useImportMarketingNodes,
  useMarketingNode,
  useMarketingNodeCandidates,
  useMarketingNodeImpact,
  useMarketingNodeRevisions,
  useMarketingNodes,
  useReviseMarketingNode,
  useSyncMarketingCandidates,
  type CsvRowError,
  type ImpactItem,
  type ImportRowResult,
  type LeadDays,
  type MarketingNode,
  type MarketingNodeForm,
  type MarketingNodeFormField,
  type MarketingNodeInput,
  type NodeCandidate,
  type NodeChangeKind,
  type NodePhase,
  type NodeRevision,
  type TopicCard,
} from "@multica/core/content/topic-planning";
import { supportedTimezones } from "@multica/core/workspace";
import { Button } from "@multica/ui/components/ui/button";
import { Checkbox } from "@multica/ui/components/ui/checkbox";
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

// Marketing nodes (specs/033 PR 3).
//
// The composition is the topic page's, which is the settings page's:
// SettingsSection / SettingsCard / SettingsRow, one control per row, a save
// state beside the button. No control here is new and no style is tuned.
//
// What the page does NOT do is as deliberate as what it does. Nothing on it is
// generated: a candidate is what people filled in on the node and the account,
// arranged by fixed rules, and the angle is written by a person. Adopting a
// candidate creates or links one draft topic card and stops there - no brief,
// no start, no work, no review, no delivery.

type Translate = ReturnType<typeof useT<"common">>["t"];

/** An account of this brand, supplied by the Web adapter. */
export interface MarketingNodeAccountOption {
  accountId: string;
  name: string;
}

/** A source-inbox row, supplied by the Web adapter (topic-planning does not depend on source-inbox). */
export interface MarketingNodeSourceOption {
  sourceId: string;
  title: string;
  url: string;
  status: string;
}

export interface MarketingNodesPageProps {
  wsId: string;
  /** The brand's time zone; a new node starts with it (FR-012). */
  brandTimezone: string;
  accounts: MarketingNodeAccountOption[];
  accountsLoading?: boolean;
  accountsFailed?: boolean;
  sources: MarketingNodeSourceOption[];
  sourcesLoading?: boolean;
  sourcesFailed?: boolean;
  /** The topic page of this workspace, built from its slug (Issue #191). */
  topicsHref: string;
}

// ---------------------------------------------------------------------------
// Labels. A value this build does not know renders as its raw text.

function statusLabel(t: Translate, status: string): string {
  switch (status) {
    case "active":
      return t(($) => $.contentMarketingNodes.statuses.active);
    case "unconfirmed":
      return t(($) => $.contentMarketingNodes.statuses.unconfirmed);
    case "cancelled":
      return t(($) => $.contentMarketingNodes.statuses.cancelled);
    default:
      return status;
  }
}

function phaseLabel(t: Translate, phase: NodePhase): string {
  switch (phase.kind) {
    case "before_preparation":
      return t(($) => $.contentMarketingNodes.phases.before_preparation);
    case "preparing":
      return t(($) => $.contentMarketingNodes.phases.preparing);
    case "live":
      return t(($) => $.contentMarketingNodes.phases.live);
    case "ended":
      return t(($) => $.contentMarketingNodes.phases.ended);
    case "before_start_unknown_lead":
      return t(($) => $.contentMarketingNodes.phases.before_start_unknown_lead);
    default:
      return phase.raw;
  }
}

function kindLabel(t: Translate, kind: string): string {
  switch (kind) {
    case "holiday":
      return t(($) => $.contentMarketingNodes.kinds.holiday);
    case "industry":
      return t(($) => $.contentMarketingNodes.kinds.industry);
    case "brand_campaign":
      return t(($) => $.contentMarketingNodes.kinds.brand_campaign);
    case "marketing":
      return t(($) => $.contentMarketingNodes.kinds.marketing);
    default:
      return kind;
  }
}

function certaintyLabel(t: Translate, certainty: string): string {
  switch (certainty) {
    case "confirmed":
      return t(($) => $.contentMarketingNodes.certainties.confirmed);
    case "tentative":
      return t(($) => $.contentMarketingNodes.certainties.tentative);
    default:
      return certainty;
  }
}

function originLabel(t: Translate, origin: string): string {
  switch (origin) {
    case "manual":
      return t(($) => $.contentMarketingNodes.origins.manual);
    case "import":
      return t(($) => $.contentMarketingNodes.origins.import);
    default:
      return origin;
  }
}

function changeKindLabel(t: Translate, change: NodeChangeKind): string {
  switch (change.kind) {
    case "create":
      return t(($) => $.contentMarketingNodes.changeKinds.create);
    case "edit":
      return t(($) => $.contentMarketingNodes.changeKinds.edit);
    case "reschedule":
      return t(($) => $.contentMarketingNodes.changeKinds.reschedule);
    case "confirm":
      return t(($) => $.contentMarketingNodes.changeKinds.confirm);
    case "cancel":
      return t(($) => $.contentMarketingNodes.changeKinds.cancel);
    default:
      return change.raw;
  }
}

function candidateStatusLabel(t: Translate, status: string): string {
  switch (status) {
    case "open":
      return t(($) => $.contentMarketingNodes.candidates.statuses.open);
    case "adopted":
      return t(($) => $.contentMarketingNodes.candidates.statuses.adopted);
    case "dismissed":
      return t(($) => $.contentMarketingNodes.candidates.statuses.dismissed);
    default:
      return status;
  }
}

function cardStatusLabel(t: Translate, status: string): string {
  switch (topicStatusKey(status)) {
    case "draft":
      return t(($) => $.contentTopics.status.draft);
    case "started":
      return t(($) => $.contentTopics.status.started);
    case "saved":
      return t(($) => $.contentTopics.status.saved);
    case "deferred":
      return t(($) => $.contentTopics.status.deferred);
    case "dropped":
      return t(($) => $.contentTopics.status.dropped);
    default:
      return status;
  }
}

function fieldLabel(t: Translate, field: MarketingNodeFormField | string): string {
  switch (field) {
    case "name":
      return t(($) => $.contentMarketingNodes.fields.name);
    case "kind":
      return t(($) => $.contentMarketingNodes.fields.kind);
    case "starts_on":
      return t(($) => $.contentMarketingNodes.fields.startsOn);
    case "ends_on":
      return t(($) => $.contentMarketingNodes.fields.endsOn);
    case "timezone":
      return t(($) => $.contentMarketingNodes.fields.timezone);
    case "lead_days":
      return t(($) => $.contentMarketingNodes.fields.leadDays);
    case "accounts":
      return t(($) => $.contentMarketingNodes.fields.accounts);
    case "goal":
      return t(($) => $.contentMarketingNodes.fields.goal);
    case "material_source_ids":
      return t(($) => $.contentMarketingNodes.fields.materials);
    case "date_certainty":
      return t(($) => $.contentMarketingNodes.fields.dateCertainty);
    case "date_basis":
      return t(($) => $.contentMarketingNodes.fields.dateBasis);
    case "note":
      return t(($) => $.contentMarketingNodes.fields.note);
    default:
      return field;
  }
}

/** "Not set" and 0 read differently on purpose (FR-013). */
function leadLabel(t: Translate, lead: LeadDays): string {
  if (!lead.set) return t(($) => $.contentMarketingNodes.leadNotSet);
  if (lead.value === 0) return t(($) => $.contentMarketingNodes.leadZero);
  return t(($) => $.contentMarketingNodes.leadValue, { days: lead.value });
}

function preparationLabel(t: Translate, startsOn: string | null): string {
  return startsOn
    ? t(($) => $.contentMarketingNodes.preparationFrom, { date: startsOn })
    : t(($) => $.contentMarketingNodes.preparationNotSet);
}

function computedOnLabel(t: Translate, timing: { timezone: string; today: string }): string {
  return t(($) => $.contentMarketingNodes.computedOn, {
    timezone: timing.timezone,
    today: timing.today,
  });
}

function labeled(t: Translate, label: string, value: string): string {
  return t(($) => $.contentMarketingNodes.labeled, { label, value });
}

function withDetail(t: Translate, value: string, detail: string): string {
  return t(($) => $.contentMarketingNodes.withDetail, { value, detail });
}

function dateRange(startsOn: string, endsOn: string): string {
  return startsOn === endsOn ? startsOn : `${startsOn} – ${endsOn}`;
}

function accountLabel(accounts: MarketingNodeAccountOption[], accountId: string): string {
  return accounts.find((account) => account.accountId === accountId)?.name || accountId;
}

function sourceLabel(sources: MarketingNodeSourceOption[], sourceId: string): string {
  const source = sources.find((item) => item.sourceId === sourceId);
  return source?.title.trim() || source?.url.trim() || sourceId;
}

function cardLabel(cards: TopicCard[], topicCardId: string): string {
  const card = cards.find((item) => item.topicCardId === topicCardId);
  return card?.audienceProblemJudgment.trim() || topicCardId;
}

function formatTime(value: string): string {
  const time = new Date(value);
  return Number.isNaN(time.getTime()) ? value : time.toLocaleString();
}

// ---------------------------------------------------------------------------
// Shared pieces.

function saveStatusOf(outcome: SaveOutcome | null, pending: boolean): SettingsSaveStatus {
  if (pending) return "saving";
  if (!outcome) return "idle";
  return outcome.kind === "saved" ? "saved" : "error";
}

function SaveFeedback({ outcome, pending }: { outcome: SaveOutcome | null; pending: boolean }) {
  const { t } = useT("common");
  const errorLabel =
    outcome?.kind === "conflict"
      ? t(($) => $.contentMarketingNodes.conflict)
      : outcome?.kind === "failed" && outcome.detail.nextAction
        ? `${t(($) => $.contentMarketingNodes.failed)} · ${t(($) => $.contentMarketingNodes.nextAction, { action: outcome.detail.nextAction })}`
        : t(($) => $.contentMarketingNodes.failed);
  return (
    <SettingsSaveState
      status={saveStatusOf(outcome, pending)}
      savingLabel={t(($) => $.contentMarketingNodes.saving)}
      savedLabel={t(($) => $.contentMarketingNodes.saved)}
      errorLabel={errorLabel}
    />
  );
}

/** A value shown back read-only; an empty one says "not filled in", never blank. */
function ReadOnlyValue({ value }: { value: string }) {
  const { t } = useT("common");
  return (
    <span className="block whitespace-pre-wrap break-words text-body text-muted-foreground">
      {value.trim() === "" ? t(($) => $.contentMarketingNodes.blank) : value}
    </span>
  );
}

function ReadOnlyList({ items, empty }: { items: string[]; empty: string }) {
  if (items.length === 0) {
    return <span className="text-body text-muted-foreground">{empty}</span>;
  }
  return (
    <ul className="space-y-1">
      {items.map((item, index) => (
        <li key={index} className="whitespace-pre-wrap break-words text-body text-muted-foreground">
          {item}
        </li>
      ))}
    </ul>
  );
}

function StateRow({ label }: { label: string }) {
  return (
    <SettingsRow label={label}>
      <span className="text-caption text-muted-foreground" />
    </SettingsRow>
  );
}

// ---------------------------------------------------------------------------
// The page.

export function MarketingNodesPage(props: MarketingNodesPageProps) {
  const { t } = useT("common");
  return (
    <>
      <PageHeader className="xl:hidden">
        <span className="text-body font-medium">{t(($) => $.contentMarketingNodes.title)}</span>
      </PageHeader>
      <MarketingNodesContent {...props} />
    </>
  );
}

function sortedNodes(nodes: MarketingNode[]): MarketingNode[] {
  return [...nodes].sort(
    (a, b) =>
      a.current.startsOn.localeCompare(b.current.startsOn) ||
      a.current.name.localeCompare(b.current.name),
  );
}

function MarketingNodesContent(props: MarketingNodesPageProps) {
  const { t } = useT("common");
  const nodes = useMarketingNodes(props.wsId);
  const [selectedId, setSelectedId] = useState("");
  const list = sortedNodes(nodes.data ?? []);
  // A node that is no longer listed - another brand's, or gone - must not
  // stay selected, or the panel would act on something that is not there.
  const selected = list.find((node) => node.nodeId === selectedId) ?? null;

  return (
    <SettingsContent>
      <SettingsTab
        title={t(($) => $.contentMarketingNodes.title)}
        description={t(($) => $.contentMarketingNodes.description)}
      >
        <SettingsSection title={t(($) => $.contentMarketingNodes.listTitle)}>
          <SettingsCard>
            {nodes.isLoading ? (
              <StateRow label={t(($) => $.contentMarketingNodes.loading)} />
            ) : nodes.isError ? (
              <StateRow label={t(($) => $.contentMarketingNodes.loadFailed)} />
            ) : list.length === 0 ? (
              <StateRow label={t(($) => $.contentMarketingNodes.listEmpty)} />
            ) : (
              list.map((node) => (
                <SettingsRow
                  key={node.nodeId}
                  label={
                    <button
                      type="button"
                      onClick={() => setSelectedId(node.nodeId)}
                      data-active={node.nodeId === selectedId}
                      className="break-words text-left text-body data-[active=true]:font-semibold hover:text-foreground"
                    >
                      {node.current.name || node.nodeId}
                    </button>
                  }
                  description={[
                    kindLabel(t, node.current.kind),
                    dateRange(node.current.startsOn, node.current.endsOn),
                    statusLabel(t, node.status),
                    certaintyLabel(t, node.current.dateCertainty),
                    leadLabel(t, node.current.leadDays),
                  ].join(" · ")}
                  size="select-wide"
                >
                  <div className="text-caption text-muted-foreground">
                    <div>{phaseLabel(t, node.timing.phase)}</div>
                    <div>{computedOnLabel(t, node.timing)}</div>
                  </div>
                </SettingsRow>
              ))
            )}
          </SettingsCard>
        </SettingsSection>

        <CreateNodeSection {...props} onCreated={setSelectedId} />
        <ImportNodesSection wsId={props.wsId} brandTimezone={props.brandTimezone} />

        {selected ? <NodePanel key={selected.nodeId} {...props} listed={selected} /> : null}
      </SettingsTab>
    </SettingsContent>
  );
}

// ---------------------------------------------------------------------------
// The node form, shared by create and edit.

function NodeFormRows({
  form,
  onChange,
  disabled,
  accounts,
  accountsLoading,
  accountsFailed,
  sources,
  sourcesLoading,
  sourcesFailed,
}: {
  form: MarketingNodeForm;
  onChange: (next: MarketingNodeForm) => void;
  disabled: boolean;
} & Pick<
  MarketingNodesPageProps,
  "accounts" | "accountsLoading" | "accountsFailed" | "sources" | "sourcesLoading" | "sourcesFailed"
>) {
  const { t } = useT("common");
  const edit = (patch: Partial<MarketingNodeForm>) => onChange({ ...form, ...patch });
  const errors = marketingNodeFormErrors(form);
  const invalid = (field: MarketingNodeFormField) => errors.includes(field);
  const zones = supportedTimezones();
  // A zone stored by an older node or an import may not be in this runtime's
  // list; it still has to be selectable to be shown.
  const zoneItems = zones.includes(form.timezone) || form.timezone === "" ? zones : [form.timezone, ...zones];
  const kindItems = MARKETING_NODE_KINDS.map((kind) => ({ value: kind, label: kindLabel(t, kind) }));
  const certaintyItems = MARKETING_NODE_DATE_CERTAINTIES.map((value) => ({
    value,
    label: certaintyLabel(t, value),
  }));
  // A material the node already cites stays listed even when it is archived.
  const selectableSources = sources.filter(
    (source) => source.status !== "archived" || form.materialSourceIds.includes(source.sourceId),
  );
  const unlistedSourceIds = form.materialSourceIds.filter(
    (sourceId) => !sources.some((source) => source.sourceId === sourceId),
  );
  const unlistedAccountIds = form.accounts
    .map((account) => account.accountId)
    .filter((accountId) => !accounts.some((account) => account.accountId === accountId));

  return (
    <>
      <SettingsRow label={t(($) => $.contentMarketingNodes.fields.name)} size="text">
        <Input
          value={form.name}
          onChange={(event) => edit({ name: event.target.value })}
          placeholder={t(($) => $.contentMarketingNodes.placeholders.name)}
          aria-label={t(($) => $.contentMarketingNodes.fields.name)}
          aria-invalid={invalid("name")}
          disabled={disabled}
        />
      </SettingsRow>
      <SettingsRow label={t(($) => $.contentMarketingNodes.fields.kind)} size="select">
        <Select
          items={kindItems}
          value={form.kind}
          onValueChange={(value) => edit({ kind: value ?? form.kind })}
          disabled={disabled}
        >
          <SelectTrigger aria-label={t(($) => $.contentMarketingNodes.fields.kind)}>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {kindItems.map((item) => (
              <SelectItem key={item.value} value={item.value}>
                {item.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </SettingsRow>
      <SettingsRow label={t(($) => $.contentMarketingNodes.fields.startsOn)} size="text">
        <Input
          type="date"
          value={form.startsOn}
          onChange={(event) => edit({ startsOn: event.target.value })}
          aria-label={t(($) => $.contentMarketingNodes.fields.startsOn)}
          aria-invalid={invalid("starts_on")}
          disabled={disabled}
        />
      </SettingsRow>
      <SettingsRow
        label={t(($) => $.contentMarketingNodes.fields.endsOn)}
        description={t(($) => $.contentMarketingNodes.hints.endsOn)}
        size="text"
      >
        <Input
          type="date"
          value={form.endsOn}
          onChange={(event) => edit({ endsOn: event.target.value })}
          aria-label={t(($) => $.contentMarketingNodes.fields.endsOn)}
          aria-invalid={invalid("ends_on")}
          disabled={disabled}
        />
      </SettingsRow>
      <SettingsRow
        label={t(($) => $.contentMarketingNodes.fields.timezone)}
        description={t(($) => $.contentMarketingNodes.hints.timezone)}
        size="select-wide"
      >
        <Select
          items={zoneItems.map((zone) => ({ value: zone, label: zone }))}
          value={form.timezone === "" ? null : form.timezone}
          onValueChange={(value) => edit({ timezone: value ?? "" })}
          disabled={disabled}
        >
          <SelectTrigger
            aria-label={t(($) => $.contentMarketingNodes.fields.timezone)}
            aria-invalid={invalid("timezone")}
          >
            <SelectValue placeholder={t(($) => $.contentMarketingNodes.placeholders.timezone)} />
          </SelectTrigger>
          <SelectContent>
            {zoneItems.map((zone) => (
              <SelectItem key={zone} value={zone}>
                {zone}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </SettingsRow>
      <SettingsRow
        label={t(($) => $.contentMarketingNodes.fields.leadDays)}
        description={t(($) => $.contentMarketingNodes.hints.leadDays)}
        size="text"
      >
        <Input
          inputMode="numeric"
          value={form.leadDays}
          onChange={(event) => edit({ leadDays: event.target.value })}
          placeholder={t(($) => $.contentMarketingNodes.leadNotSet)}
          aria-label={t(($) => $.contentMarketingNodes.fields.leadDays)}
          aria-invalid={invalid("lead_days")}
          disabled={disabled}
        />
      </SettingsRow>
      <SettingsRow
        label={t(($) => $.contentMarketingNodes.fields.accounts)}
        description={t(($) => $.contentMarketingNodes.hints.accounts)}
        size="text"
        align="start"
      >
        <div className="space-y-3">
          {accountsLoading ? (
            <span className="text-body text-muted-foreground">
              {t(($) => $.contentMarketingNodes.accountsLoading)}
            </span>
          ) : accountsFailed ? (
            <span className="text-body text-muted-foreground">
              {t(($) => $.contentMarketingNodes.accountsLoadFailed)}
            </span>
          ) : accounts.length === 0 && unlistedAccountIds.length === 0 ? (
            <span className="text-body text-muted-foreground">
              {t(($) => $.contentMarketingNodes.accountsEmpty)}
            </span>
          ) : null}
          {[
            ...accounts.map((account) => ({ accountId: account.accountId, name: account.name || account.accountId })),
            ...unlistedAccountIds.map((accountId) => ({ accountId, name: accountId })),
          ].map((account) => {
            const chosen = form.accounts.find((item) => item.accountId === account.accountId);
            return (
              <div key={account.accountId} className="space-y-2">
                <label className="flex items-start gap-3 text-body text-muted-foreground">
                  <Checkbox
                    checked={!!chosen}
                    disabled={disabled}
                    onCheckedChange={(value) =>
                      onChange(setMarketingNodeAccount(form, account.accountId, value === true))
                    }
                    aria-label={account.name}
                  />
                  <span className="break-words">{account.name}</span>
                </label>
                {chosen ? (
                  <Input
                    value={chosen.role}
                    onChange={(event) =>
                      onChange(setMarketingNodeAccountRole(form, account.accountId, event.target.value))
                    }
                    placeholder={t(($) => $.contentMarketingNodes.placeholders.role)}
                    aria-label={t(($) => $.contentMarketingNodes.roleOf, { account: account.name })}
                    aria-invalid={invalid("accounts")}
                    disabled={disabled}
                  />
                ) : null}
              </div>
            );
          })}
        </div>
      </SettingsRow>
      <SettingsRow label={t(($) => $.contentMarketingNodes.fields.goal)} size="text" align="start">
        <Textarea
          value={form.goal}
          onChange={(event) => edit({ goal: event.target.value })}
          placeholder={t(($) => $.contentMarketingNodes.placeholders.goal)}
          aria-label={t(($) => $.contentMarketingNodes.fields.goal)}
          aria-invalid={invalid("goal")}
          rows={3}
          disabled={disabled}
        />
      </SettingsRow>
      <SettingsRow
        label={t(($) => $.contentMarketingNodes.fields.materials)}
        description={t(($) => $.contentMarketingNodes.hints.materials)}
        size="text"
        align="start"
      >
        <div className="space-y-2">
          {unlistedSourceIds.map((sourceId) => (
            <label key={sourceId} className="flex items-start gap-3 text-body text-muted-foreground">
              <Checkbox
                checked
                disabled={disabled}
                onCheckedChange={(value) => {
                  if (value === false) onChange(setMarketingNodeMaterial(form, sourceId, false));
                }}
                aria-label={sourceId}
              />
              <span className="break-words">{sourceId}</span>
            </label>
          ))}
          {sourcesLoading ? (
            <span className="text-body text-muted-foreground">
              {t(($) => $.contentMarketingNodes.materialsLoading)}
            </span>
          ) : sourcesFailed ? (
            <span className="text-body text-muted-foreground">
              {t(($) => $.contentMarketingNodes.materialsLoadFailed)}
            </span>
          ) : selectableSources.length === 0 ? (
            <span className="text-body text-muted-foreground">
              {t(($) => $.contentMarketingNodes.materialsEmpty)}
            </span>
          ) : (
            selectableSources.map((source) => (
              <label
                key={source.sourceId}
                className="flex items-start gap-3 text-body text-muted-foreground"
              >
                <Checkbox
                  checked={form.materialSourceIds.includes(source.sourceId)}
                  disabled={disabled}
                  onCheckedChange={(value) =>
                    onChange(setMarketingNodeMaterial(form, source.sourceId, value === true))
                  }
                  aria-label={sourceLabel(sources, source.sourceId)}
                />
                <span className="break-words">{sourceLabel(sources, source.sourceId)}</span>
              </label>
            ))
          )}
        </div>
      </SettingsRow>
      <SettingsRow label={t(($) => $.contentMarketingNodes.fields.dateCertainty)} size="select">
        <Select
          items={certaintyItems}
          value={form.dateCertainty}
          onValueChange={(value) => edit({ dateCertainty: value ?? form.dateCertainty })}
          disabled={disabled}
        >
          <SelectTrigger aria-label={t(($) => $.contentMarketingNodes.fields.dateCertainty)}>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {certaintyItems.map((item) => (
              <SelectItem key={item.value} value={item.value}>
                {item.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </SettingsRow>
      <SettingsRow label={t(($) => $.contentMarketingNodes.fields.dateBasis)} size="text" align="start">
        <Textarea
          value={form.dateBasis}
          onChange={(event) => edit({ dateBasis: event.target.value })}
          placeholder={t(($) => $.contentMarketingNodes.placeholders.dateBasis)}
          aria-label={t(($) => $.contentMarketingNodes.fields.dateBasis)}
          aria-invalid={invalid("date_basis")}
          rows={2}
          disabled={disabled}
        />
      </SettingsRow>
      <SettingsRow label={t(($) => $.contentMarketingNodes.fields.note)} size="text" align="start">
        <Textarea
          value={form.note}
          onChange={(event) => edit({ note: event.target.value })}
          placeholder={t(($) => $.contentMarketingNodes.placeholders.note)}
          aria-label={t(($) => $.contentMarketingNodes.fields.note)}
          aria-invalid={invalid("note")}
          rows={2}
          disabled={disabled}
        />
      </SettingsRow>
    </>
  );
}

/** Names the fields that block saving; the save button stays disabled until it is empty. */
function invalidFieldsLabel(t: Translate, errors: MarketingNodeFormField[]): string | undefined {
  if (errors.length === 0) return undefined;
  return t(($) => $.contentMarketingNodes.invalidFields, {
    fields: errors.map((field) => fieldLabel(t, field)).join(t(($) => $.contentMarketingNodes.listSeparator)),
  });
}

function CreateNodeSection({
  onCreated,
  ...props
}: MarketingNodesPageProps & { onCreated: (nodeId: string) => void }) {
  const { t } = useT("common");
  const create = useCreateMarketingNode(props.wsId);
  const [form, setForm] = useState<MarketingNodeForm>(() => emptyMarketingNodeForm(props.brandTimezone));
  const [outcome, setOutcome] = useState<SaveOutcome | null>(null);
  const errors = marketingNodeFormErrors(form);

  const submit = () => {
    setOutcome(null);
    create.mutate(
      { node: marketingNodeFormToInput(form), note: form.note },
      {
        onSuccess: (node) => {
          setOutcome(SAVED);
          setForm(emptyMarketingNodeForm(props.brandTimezone));
          if (node.nodeId) onCreated(node.nodeId);
        },
        onError: (error) => setOutcome(saveOutcome(error)),
      },
    );
  };

  return (
    <SettingsSection
      title={t(($) => $.contentMarketingNodes.createTitle)}
      description={t(($) => $.contentMarketingNodes.createDescription)}
    >
      <SettingsCard>
        <NodeFormRows
          form={form}
          onChange={setForm}
          disabled={create.isPending}
          accounts={props.accounts}
          accountsLoading={props.accountsLoading}
          accountsFailed={props.accountsFailed}
          sources={props.sources}
          sourcesLoading={props.sourcesLoading}
          sourcesFailed={props.sourcesFailed}
        />
        <SettingsRow label={t(($) => $.contentMarketingNodes.create)} description={invalidFieldsLabel(t, errors)}>
          <div className="flex items-center gap-3">
            <SaveFeedback outcome={outcome} pending={create.isPending} />
            <Button onClick={submit} disabled={create.isPending || errors.length > 0}>
              {t(($) => $.contentMarketingNodes.create)}
            </Button>
          </div>
        </SettingsRow>
      </SettingsCard>
    </SettingsSection>
  );
}

// ---------------------------------------------------------------------------
// Import: pasted CSV, checked here, sent as JSON; each row's result shown.

const IMPORT_MAX_ROWS = 100;

type ImportLine =
  | { line: number; kind: "local"; error: CsvRowError }
  | { line: number; kind: "server"; result: ImportRowResult };

function csvErrorLabel(t: Translate, line: number, error: CsvRowError): string {
  if (error.reason === "column_count") {
    return t(($) => $.contentMarketingNodes.import.rowColumnCount, {
      line,
      expected: error.expected,
      actual: error.actual,
    });
  }
  return t(($) => $.contentMarketingNodes.import.rowInvalid, { line, column: error.column });
}

function importResultLabel(t: Translate, line: number, result: ImportRowResult): string {
  switch (result.outcome.kind) {
    case "created":
      return t(($) => $.contentMarketingNodes.import.rowCreated, { line });
    case "duplicate":
      return t(($) => $.contentMarketingNodes.import.rowDuplicate, { line });
    case "invalid":
      return t(($) => $.contentMarketingNodes.import.rowInvalid, {
        line,
        column: result.field || "-",
      });
    default:
      return t(($) => $.contentMarketingNodes.import.rowOther, { line, outcome: result.outcome.raw });
  }
}

function ImportNodesSection({ wsId, brandTimezone }: { wsId: string; brandTimezone: string }) {
  const { t } = useT("common");
  const importNodes = useImportMarketingNodes(wsId);
  const [text, setText] = useState("");
  const [lines, setLines] = useState<ImportLine[]>([]);
  const [problem, setProblem] = useState("");
  const [outcome, setOutcome] = useState<SaveOutcome | null>(null);

  const submit = () => {
    setOutcome(null);
    setProblem("");
    setLines([]);
    const parsed = parseMarketingNodeCsv(text, { timezone: brandTimezone });
    if (!parsed.ok) {
      setProblem(
        parsed.error === "empty"
          ? t(($) => $.contentMarketingNodes.import.empty)
          : t(($) => $.contentMarketingNodes.import.missingColumns, { columns: parsed.missing.join(", ") }),
      );
      return;
    }
    const local: ImportLine[] = [];
    const rows: { line: number; input: MarketingNodeInput }[] = [];
    for (const row of parsed.rows) {
      if (row.ok) rows.push({ line: row.line, input: row.input });
      else local.push({ line: row.line, kind: "local", error: row.error });
    }
    if (rows.length > IMPORT_MAX_ROWS) {
      setProblem(t(($) => $.contentMarketingNodes.import.tooMany, { limit: IMPORT_MAX_ROWS, rows: rows.length }));
      return;
    }
    if (rows.length === 0) {
      setLines(local);
      setProblem(t(($) => $.contentMarketingNodes.import.noValidRows));
      return;
    }
    importNodes.mutate(
      rows.map((row) => row.input),
      {
        onSuccess: (results) => {
          setOutcome(SAVED);
          // The server numbers the rows it was sent from 1; map them back to
          // the pasted line so a result points at the text.
          const server: ImportLine[] = results.map((result) => ({
            line: rows[result.row - 1]?.line ?? result.row,
            kind: "server",
            result,
          }));
          setLines([...local, ...server].sort((a, b) => a.line - b.line));
        },
        onError: (error) => {
          setOutcome(saveOutcome(error));
          setLines(local);
        },
      },
    );
  };

  return (
    <SettingsSection
      title={t(($) => $.contentMarketingNodes.import.title)}
      description={t(($) => $.contentMarketingNodes.import.description)}
    >
      <SettingsCard>
        <SettingsRow
          label={t(($) => $.contentMarketingNodes.import.label)}
          description={t(($) => $.contentMarketingNodes.import.columns)}
          size="text"
          align="start"
        >
          <Textarea
            value={text}
            onChange={(event) => setText(event.target.value)}
            placeholder={t(($) => $.contentMarketingNodes.import.placeholder)}
            aria-label={t(($) => $.contentMarketingNodes.import.label)}
            rows={6}
            disabled={importNodes.isPending}
          />
        </SettingsRow>
        <SettingsRow label={t(($) => $.contentMarketingNodes.import.submit)} description={problem || undefined}>
          <div className="flex items-center gap-3">
            <SaveFeedback outcome={outcome} pending={importNodes.isPending} />
            <Button onClick={submit} disabled={importNodes.isPending || text.trim() === ""}>
              {t(($) => $.contentMarketingNodes.import.submit)}
            </Button>
          </div>
        </SettingsRow>
        {lines.length > 0 ? (
          <SettingsRow label={t(($) => $.contentMarketingNodes.import.resultsTitle)} size="text" align="start">
            <ReadOnlyList
              items={lines.map((line) =>
                line.kind === "local"
                  ? csvErrorLabel(t, line.line, line.error)
                  : importResultLabel(t, line.line, line.result),
              )}
              empty=""
            />
          </SettingsRow>
        ) : null}
      </SettingsCard>
    </SettingsSection>
  );
}

// ---------------------------------------------------------------------------
// One node: detail, status changes, edit, history, candidates, impact.

function NodePanel({ listed, ...props }: MarketingNodesPageProps & { listed: MarketingNode }) {
  // The detail query is what the writes refresh, so the panel reads it rather
  // than the list row it was selected from.
  const detail = useMarketingNode(props.wsId, listed.nodeId);
  const node = detail.data && detail.data.nodeId ? detail.data : listed;
  const cards = useContentTopics(props.wsId, "");
  const cardList = cards.data ?? [];
  // Held here, not in the edit form: the form remounts on every new revision
  // (including the one a 409 re-read brings in), and the conflict sentence
  // has to survive that.
  const [editOutcome, setEditOutcome] = useState<SaveOutcome | null>(null);
  return (
    <>
      <NodeDetailSection node={node} accounts={props.accounts} sources={props.sources} />
      <NodeStatusSection wsId={props.wsId} node={node} />
      <EditNodeSection
        key={`${node.nodeId}:${node.currentRevision}`}
        {...props}
        node={node}
        outcome={editOutcome}
        setOutcome={setEditOutcome}
      />
      <RevisionHistorySection wsId={props.wsId} node={node} accounts={props.accounts} />
      <CandidatesSection
        wsId={props.wsId}
        node={node}
        accounts={props.accounts}
        sources={props.sources}
        cards={cardList}
        topicsHref={props.topicsHref}
      />
      <ImpactSection
        wsId={props.wsId}
        node={node}
        accounts={props.accounts}
        cards={cardList}
        topicsHref={props.topicsHref}
      />
    </>
  );
}

function NodeDetailSection({
  node,
  accounts,
  sources,
}: {
  node: MarketingNode;
  accounts: MarketingNodeAccountOption[];
  sources: MarketingNodeSourceOption[];
}) {
  const { t } = useT("common");
  const current = node.current;
  const rows: { label: string; value: string }[] = [
    { label: t(($) => $.contentMarketingNodes.fields.name), value: current.name },
    { label: t(($) => $.contentMarketingNodes.fields.status), value: statusLabel(t, node.status) },
    { label: t(($) => $.contentMarketingNodes.fields.kind), value: kindLabel(t, current.kind) },
    { label: t(($) => $.contentMarketingNodes.fields.dates), value: dateRange(current.startsOn, current.endsOn) },
    { label: t(($) => $.contentMarketingNodes.fields.timezone), value: current.timezone },
    { label: t(($) => $.contentMarketingNodes.fields.leadDays), value: leadLabel(t, current.leadDays) },
    {
      label: t(($) => $.contentMarketingNodes.fields.preparation),
      value: preparationLabel(t, node.timing.preparationStartsOn),
    },
    { label: t(($) => $.contentMarketingNodes.fields.phase), value: phaseLabel(t, node.timing.phase) },
    { label: t(($) => $.contentMarketingNodes.fields.today), value: computedOnLabel(t, node.timing) },
    { label: t(($) => $.contentMarketingNodes.fields.goal), value: current.goal },
    {
      label: t(($) => $.contentMarketingNodes.fields.dateCertainty),
      value: certaintyLabel(t, current.dateCertainty),
    },
    { label: t(($) => $.contentMarketingNodes.fields.dateBasis), value: current.dateBasis },
    { label: t(($) => $.contentMarketingNodes.fields.origin), value: originLabel(t, node.origin) },
    {
      label: t(($) => $.contentMarketingNodes.fields.revision),
      value: t(($) => $.contentMarketingNodes.revisionNumber, { revision: node.currentRevision }),
    },
  ];
  return (
    <SettingsSection title={t(($) => $.contentMarketingNodes.detailTitle)}>
      <SettingsCard>
        {rows.map((row) => (
          <SettingsRow key={row.label} label={row.label} size="text" align="start">
            <ReadOnlyValue value={row.value} />
          </SettingsRow>
        ))}
        <SettingsRow label={t(($) => $.contentMarketingNodes.fields.accounts)} size="text" align="start">
          <ReadOnlyList
            items={current.accounts.map((account) =>
              account.role.trim()
                ? `${accountLabel(accounts, account.accountId)} · ${account.role}`
                : `${accountLabel(accounts, account.accountId)} · ${t(($) => $.contentMarketingNodes.blank)}`,
            )}
            empty={t(($) => $.contentMarketingNodes.brandLevel)}
          />
        </SettingsRow>
        <SettingsRow label={t(($) => $.contentMarketingNodes.fields.materials)} size="text" align="start">
          <ReadOnlyList
            items={current.materialSourceIds.map((sourceId) => sourceLabel(sources, sourceId))}
            empty={t(($) => $.contentMarketingNodes.materialsNone)}
          />
        </SettingsRow>
      </SettingsCard>
    </SettingsSection>
  );
}

function NodeStatusSection({ wsId, node }: { wsId: string; node: MarketingNode }) {
  const { t } = useT("common");
  const confirm = useConfirmMarketingNode(wsId);
  const cancel = useCancelMarketingNode(wsId);
  const [note, setNote] = useState("");
  const [outcome, setOutcome] = useState<SaveOutcome | null>(null);
  const cancelled = node.status === "cancelled";
  const pending = confirm.isPending || cancel.isPending;

  const run = (action: "confirm" | "cancel") => {
    setOutcome(null);
    const mutation = action === "confirm" ? confirm : cancel;
    mutation.mutate(
      { nodeId: node.nodeId, baseRevision: node.currentRevision, note },
      {
        onSuccess: () => {
          setOutcome(SAVED);
          setNote("");
        },
        onError: (error) => setOutcome(saveOutcome(error)),
      },
    );
  };

  return (
    <SettingsSection
      title={t(($) => $.contentMarketingNodes.statusTitle)}
      description={t(($) => $.contentMarketingNodes.statusDescription)}
    >
      <SettingsCard>
        <SettingsRow label={t(($) => $.contentMarketingNodes.fields.note)} size="text" align="start">
          <Textarea
            value={note}
            onChange={(event) => setNote(event.target.value)}
            placeholder={t(($) => $.contentMarketingNodes.placeholders.statusNote)}
            aria-label={t(($) => $.contentMarketingNodes.fields.note)}
            rows={2}
            disabled={pending || cancelled}
          />
        </SettingsRow>
        <SettingsRow
          label={t(($) => $.contentMarketingNodes.confirm)}
          description={t(($) => $.contentMarketingNodes.confirmHint)}
        >
          <Button
            variant="outline"
            onClick={() => run("confirm")}
            disabled={pending || node.status !== "unconfirmed"}
          >
            {t(($) => $.contentMarketingNodes.confirm)}
          </Button>
        </SettingsRow>
        <SettingsRow
          label={t(($) => $.contentMarketingNodes.cancel)}
          description={t(($) => $.contentMarketingNodes.cancelHint)}
        >
          <div className="flex items-center gap-3">
            <SaveFeedback outcome={outcome} pending={pending} />
            <Button variant="outline" onClick={() => run("cancel")} disabled={pending || cancelled}>
              {t(($) => $.contentMarketingNodes.cancel)}
            </Button>
          </div>
        </SettingsRow>
      </SettingsCard>
    </SettingsSection>
  );
}

function EditNodeSection({
  node,
  outcome,
  setOutcome,
  ...props
}: MarketingNodesPageProps & {
  node: MarketingNode;
  outcome: SaveOutcome | null;
  setOutcome: (outcome: SaveOutcome | null) => void;
}) {
  const { t } = useT("common");
  const revise = useReviseMarketingNode(props.wsId);
  // Every field starts from the current revision: a revise stores the whole
  // revision, so a field left out would be saved as empty (contract §2.1).
  const [form, setForm] = useState<MarketingNodeForm>(() => marketingNodeFormFromRevision(node.current));
  const cancelled = node.status === "cancelled";
  const errors = marketingNodeFormErrors(form);
  const changed = marketingNodeFormChanged(form, node.current);

  const submit = () => {
    setOutcome(null);
    revise.mutate(
      {
        nodeId: node.nodeId,
        baseRevision: node.currentRevision,
        node: marketingNodeFormToInput(form),
        note: form.note,
      },
      {
        onSuccess: () => setOutcome(SAVED),
        onError: (error) => setOutcome(saveOutcome(error)),
      },
    );
  };

  return (
    <SettingsSection
      title={t(($) => $.contentMarketingNodes.editTitle)}
      description={
        cancelled
          ? t(($) => $.contentMarketingNodes.editCancelled)
          : t(($) => $.contentMarketingNodes.editDescription, { revision: node.currentRevision })
      }
    >
      <SettingsCard>
        <NodeFormRows
          form={form}
          onChange={setForm}
          disabled={revise.isPending || cancelled}
          accounts={props.accounts}
          accountsLoading={props.accountsLoading}
          accountsFailed={props.accountsFailed}
          sources={props.sources}
          sourcesLoading={props.sourcesLoading}
          sourcesFailed={props.sourcesFailed}
        />
        <SettingsRow
          label={t(($) => $.contentMarketingNodes.saveRevision)}
          description={invalidFieldsLabel(t, errors)}
        >
          <div className="flex items-center gap-3">
            <SaveFeedback outcome={outcome} pending={revise.isPending} />
            <Button
              onClick={submit}
              disabled={revise.isPending || cancelled || !changed || errors.length > 0}
            >
              {t(($) => $.contentMarketingNodes.saveRevision)}
            </Button>
          </div>
        </SettingsRow>
      </SettingsCard>
    </SettingsSection>
  );
}

function revisionSummary(
  t: Translate,
  revision: NodeRevision,
  accounts: MarketingNodeAccountOption[],
): string[] {
  const lines = [
    `${revision.name} · ${kindLabel(t, revision.kind)} · ${statusLabel(t, revision.statusAfter)}`,
    `${withDetail(t, dateRange(revision.startsOn, revision.endsOn), revision.timezone)} · ${leadLabel(t, revision.leadDays)} · ${certaintyLabel(t, revision.dateCertainty)}`,
    labeled(
      t,
      t(($) => $.contentMarketingNodes.fields.accounts),
      revision.accounts.length === 0
        ? t(($) => $.contentMarketingNodes.brandLevel)
        : revision.accounts
            .map((account) =>
              account.role.trim()
                ? withDetail(t, accountLabel(accounts, account.accountId), account.role)
                : accountLabel(accounts, account.accountId),
            )
            .join(t(($) => $.contentMarketingNodes.listSeparator)),
    ),
    labeled(t, t(($) => $.contentMarketingNodes.fields.goal), revision.goal.trim() || t(($) => $.contentMarketingNodes.blank)),
  ];
  if (revision.note.trim()) {
    lines.push(labeled(t, t(($) => $.contentMarketingNodes.fields.note), revision.note));
  }
  return lines;
}

function RevisionHistorySection({
  wsId,
  node,
  accounts,
}: {
  wsId: string;
  node: MarketingNode;
  accounts: MarketingNodeAccountOption[];
}) {
  const { t } = useT("common");
  const revisions = useMarketingNodeRevisions(wsId, node.nodeId);
  const list = revisions.data ?? [];
  return (
    <SettingsSection title={t(($) => $.contentMarketingNodes.historyTitle)}>
      <SettingsCard>
        {revisions.isLoading ? (
          <StateRow label={t(($) => $.contentMarketingNodes.loading)} />
        ) : revisions.isError ? (
          <StateRow label={t(($) => $.contentMarketingNodes.loadFailed)} />
        ) : list.length === 0 ? (
          <StateRow label={t(($) => $.contentMarketingNodes.historyEmpty)} />
        ) : (
          list.map((revision) => (
            <SettingsRow
              key={revision.revisionId || revision.revision}
              label={t(($) => $.contentMarketingNodes.historyRow, {
                revision: revision.revision,
                change: changeKindLabel(t, revision.changeKind),
              })}
              description={`${revision.actor} · ${formatTime(revision.createdAt)}`}
              size="text"
              align="start"
            >
              <ReadOnlyList items={revisionSummary(t, revision, accounts)} empty="" />
            </SettingsRow>
          ))
        )}
      </SettingsCard>
    </SettingsSection>
  );
}

// ---------------------------------------------------------------------------
// Candidates.

function CandidatesSection({
  wsId,
  node,
  accounts,
  sources,
  cards,
  topicsHref,
}: {
  wsId: string;
  node: MarketingNode;
  accounts: MarketingNodeAccountOption[];
  sources: MarketingNodeSourceOption[];
  cards: TopicCard[];
  topicsHref: string;
}) {
  const { t } = useT("common");
  const candidates = useMarketingNodeCandidates(wsId, node.nodeId);
  const sync = useSyncMarketingCandidates(wsId);
  const [outcome, setOutcome] = useState<SaveOutcome | null>(null);
  const list = candidates.data ?? [];
  const canSync = node.status === "active";

  const runSync = () => {
    setOutcome(null);
    sync.mutate(node.nodeId, {
      onSuccess: () => setOutcome(SAVED),
      onError: (error) => setOutcome(saveOutcome(error)),
    });
  };

  return (
    <SettingsSection
      title={t(($) => $.contentMarketingNodes.candidates.title)}
      // Constant, whatever the list holds (FR-055).
      description={t(($) => $.contentMarketingNodes.candidates.notice)}
    >
      <SettingsCard>
        <SettingsRow
          label={t(($) => $.contentMarketingNodes.candidates.sync)}
          description={
            canSync
              ? t(($) => $.contentMarketingNodes.candidates.syncHint)
              : t(($) => $.contentMarketingNodes.candidates.syncUnavailable)
          }
        >
          <div className="flex items-center gap-3">
            <SaveFeedback outcome={outcome} pending={sync.isPending} />
            <Button variant="outline" onClick={runSync} disabled={sync.isPending || !canSync}>
              {t(($) => $.contentMarketingNodes.candidates.sync)}
            </Button>
          </div>
        </SettingsRow>
        {candidates.isLoading ? (
          <StateRow label={t(($) => $.contentMarketingNodes.loading)} />
        ) : candidates.isError ? (
          <StateRow label={t(($) => $.contentMarketingNodes.loadFailed)} />
        ) : list.length === 0 ? (
          <StateRow label={t(($) => $.contentMarketingNodes.candidates.empty)} />
        ) : null}
      </SettingsCard>
      {list.map((candidate) => (
        <CandidateCard
          key={candidate.candidateId}
          wsId={wsId}
          node={node}
          candidate={candidate}
          accounts={accounts}
          sources={sources}
          cards={cards}
          topicsHref={topicsHref}
        />
      ))}
    </SettingsSection>
  );
}

function profileValue(t: Translate, field: { value: string; status: string }): string {
  // Only what a person filled in; an empty field says so (FR-023).
  return field.value.trim() === "" || field.status === "pending"
    ? t(($) => $.contentMarketingNodes.blank)
    : field.value;
}

function leadShortLabel(t: Translate, candidate: NodeCandidate): string {
  const timing = candidate.timing;
  if (timing.leadShort === "unknown") return t(($) => $.contentMarketingNodes.candidates.leadUnknown);
  if (timing.leadShort === true && timing.leadDays.set) {
    return t(($) => $.contentMarketingNodes.candidates.leadShort, {
      left: timing.daysUntilStart,
      lead: timing.leadDays.value,
    });
  }
  return t(($) => $.contentMarketingNodes.candidates.leadEnough);
}

function gapLabel(
  t: Translate,
  gap: NodeCandidate["materialGaps"][number],
  sources: MarketingNodeSourceOption[],
): string {
  const source = gap.sourceId ? sourceLabel(sources, gap.sourceId) : "";
  switch (gap.kind.kind) {
    case "none":
      return t(($) => $.contentMarketingNodes.candidates.gapNone);
    case "archived":
      return t(($) => $.contentMarketingNodes.candidates.gapArchived, { source });
    case "missing":
      return t(($) => $.contentMarketingNodes.candidates.gapMissing, { source });
    default:
      return `${gap.kind.raw}${source ? ` · ${source}` : ""}`;
  }
}

function duplicateLabel(
  t: Translate,
  risk: NodeCandidate["duplicateRisks"][number],
  cards: TopicCard[],
): string {
  const card = cardLabel(cards, risk.topicCardId);
  switch (risk.reason.kind) {
    case "name_match":
      return t(($) => $.contentMarketingNodes.candidates.nameMatch, { card });
    case "adopted_from_node":
      return t(($) => $.contentMarketingNodes.candidates.adoptedFromNode, { card });
    default:
      return `${risk.reason.raw} · ${card}`;
  }
}

// Base UI's Select reads "" as nothing selected, so "choose a card" has no
// value of its own; the adopt button stays disabled until a card is chosen.
function CandidateCard({
  wsId,
  node,
  candidate,
  accounts,
  sources,
  cards,
  topicsHref,
}: {
  wsId: string;
  node: MarketingNode;
  candidate: NodeCandidate;
  accounts: MarketingNodeAccountOption[];
  sources: MarketingNodeSourceOption[];
  cards: TopicCard[];
  topicsHref: string;
}) {
  const { t } = useT("common");
  const navigation = useNavigation();
  const edit = useEditMarketingCandidate(wsId);
  const adopt = useAdoptMarketingCandidate(wsId);
  const [angle, setAngle] = useState(candidate.angle);
  const [dismissReason, setDismissReason] = useState(candidate.dismissReason);
  const [linkCardId, setLinkCardId] = useState("");
  const [outcome, setOutcome] = useState<SaveOutcome | null>(null);
  const pending = edit.isPending || adopt.isPending;
  const cancelled = node.status === "cancelled";
  const open = candidate.status === "open";
  const adopted = candidate.status === "adopted";

  const account = candidate.accountId
    ? accountLabel(accounts, candidate.accountId)
    : t(($) => $.contentMarketingNodes.brandLevel);
  // A brand-level candidate may link any card; an account's candidate only a
  // card with no account or the same one (the server answers 400 otherwise).
  const linkable = cards.filter(
    (card) => candidate.accountId === "" || !card.accountId || card.accountId === candidate.accountId,
  );
  const linkItems = linkable.map((card) => ({
    value: card.topicCardId,
    label: card.audienceProblemJudgment.trim() || card.topicCardId,
  }));

  const patch = (input: { angle?: string; status?: "open" | "dismissed"; dismissReason?: string }) => {
    setOutcome(null);
    edit.mutate(
      { nodeId: node.nodeId, candidateId: candidate.candidateId, patch: input },
      {
        onSuccess: () => setOutcome(SAVED),
        onError: (error) => setOutcome(saveOutcome(error)),
      },
    );
  };

  const runAdopt = (mode: "create" | "link") => {
    setOutcome(null);
    adopt.mutate(
      {
        nodeId: node.nodeId,
        candidateId: candidate.candidateId,
        adopt: mode === "link" ? { mode: "link", topicCardId: linkCardId } : { mode: "create" },
      },
      {
        onSuccess: () => setOutcome(SAVED),
        onError: (error) => setOutcome(saveOutcome(error)),
      },
    );
  };

  const relation = candidate.relation.account;
  const facts: { label: string; value: string[] }[] = [
    {
      label: t(($) => $.contentMarketingNodes.candidates.relation),
      value: [
        labeled(t, t(($) => $.contentMarketingNodes.fields.goal), candidate.relation.goal.trim() || t(($) => $.contentMarketingNodes.blank)),
        labeled(t, t(($) => $.contentMarketingNodes.candidates.role), candidate.relation.role.trim() || t(($) => $.contentMarketingNodes.blank)),
        ...(candidate.accountId === ""
          ? []
          : relation
            ? [
                labeled(t, t(($) => $.contentMarketingNodes.candidates.audience), profileValue(t, relation.audience)),
                labeled(t, t(($) => $.contentMarketingNodes.candidates.contentPillars), profileValue(t, relation.contentPillars)),
                labeled(t, t(($) => $.contentMarketingNodes.candidates.contentGoals), profileValue(t, relation.contentGoals)),
              ]
            : [t(($) => $.contentMarketingNodes.candidates.profileUnavailable)]),
      ],
    },
    {
      label: t(($) => $.contentMarketingNodes.candidates.timing),
      value: [
        `${phaseLabel(t, candidate.timing.phase)} · ${computedOnLabel(t, candidate.timing)}`,
        `${preparationLabel(t, candidate.timing.preparationStartsOn)} · ${leadLabel(t, candidate.timing.leadDays)}`,
        t(($) => $.contentMarketingNodes.candidates.daysUntilStart, { days: candidate.timing.daysUntilStart }),
        leadShortLabel(t, candidate),
      ],
    },
    {
      label: t(($) => $.contentMarketingNodes.candidates.collisions),
      value: candidate.collisions.map(
        (collision) => withDetail(t, collision.name, dateRange(collision.startsOn, collision.endsOn)),
      ),
    },
    {
      label: t(($) => $.contentMarketingNodes.candidates.gaps),
      value: candidate.materialGaps.map((gap) => gapLabel(t, gap, sources)),
    },
    {
      label: t(($) => $.contentMarketingNodes.candidates.duplicates),
      value: candidate.duplicateRisks.map((risk) => duplicateLabel(t, risk, cards)),
    },
    {
      label: t(($) => $.contentMarketingNodes.candidates.origin),
      value: [
        `${originLabel(t, candidate.origin)} · ${certaintyLabel(t, candidate.dateCertainty)}`,
        labeled(t, t(($) => $.contentMarketingNodes.fields.dateBasis), candidate.dateBasis.trim() || t(($) => $.contentMarketingNodes.blank)),
      ],
    },
  ];

  return (
    <SettingsCard>
      <SettingsRow
        label={account}
        description={[
          candidateStatusLabel(t, candidate.status),
          ...(candidate.inScope ? [] : [t(($) => $.contentMarketingNodes.candidates.outOfScope)]),
        ].join(" · ")}
      >
        <span className="text-caption text-muted-foreground" />
      </SettingsRow>
      {facts.map((fact) => (
        <SettingsRow key={fact.label} label={fact.label} size="text" align="start">
          <ReadOnlyList items={fact.value} empty={t(($) => $.contentMarketingNodes.candidates.none)} />
        </SettingsRow>
      ))}
      <SettingsRow
        label={t(($) => $.contentMarketingNodes.candidates.angle)}
        description={t(($) => $.contentMarketingNodes.candidates.angleHint)}
        size="text"
        align="start"
      >
        <div className="space-y-2">
          <Textarea
            value={angle}
            onChange={(event) => setAngle(event.target.value)}
            placeholder={t(($) => $.contentMarketingNodes.candidates.anglePlaceholder)}
            aria-label={t(($) => $.contentMarketingNodes.candidates.angle)}
            rows={3}
            disabled={pending || adopted}
          />
          <Button
            variant="outline"
            onClick={() => patch({ angle })}
            disabled={pending || adopted || angle === candidate.angle}
          >
            {t(($) => $.contentMarketingNodes.candidates.saveAngle)}
          </Button>
        </div>
      </SettingsRow>
      {adopted ? (
        <SettingsRow
          label={t(($) => $.contentMarketingNodes.candidates.adoptedCard, {
            card: cardLabel(cards, candidate.topicCardId),
          })}
          description={
            candidate.adoptedRevision !== null
              ? t(($) => $.contentMarketingNodes.candidates.adoptedAt, { revision: candidate.adoptedRevision })
              : undefined
          }
        >
          <Button variant="outline" onClick={() => navigation.push(topicsHref)}>
            {t(($) => $.contentMarketingNodes.goToTopics)}
          </Button>
        </SettingsRow>
      ) : (
        <>
          <SettingsRow
            label={t(($) => $.contentMarketingNodes.candidates.dismissReason)}
            description={
              candidate.status === "dismissed"
                ? t(($) => $.contentMarketingNodes.candidates.dismissedHint)
                : undefined
            }
            size="text"
            align="start"
          >
            <div className="space-y-2">
              <Textarea
                value={dismissReason}
                onChange={(event) => setDismissReason(event.target.value)}
                aria-label={t(($) => $.contentMarketingNodes.candidates.dismissReason)}
                rows={2}
                disabled={pending || !open}
              />
              {open ? (
                <Button
                  variant="outline"
                  onClick={() => patch({ status: "dismissed", dismissReason })}
                  disabled={pending}
                >
                  {t(($) => $.contentMarketingNodes.candidates.dismiss)}
                </Button>
              ) : (
                <Button variant="outline" onClick={() => patch({ status: "open" })} disabled={pending}>
                  {t(($) => $.contentMarketingNodes.candidates.reopen)}
                </Button>
              )}
            </div>
          </SettingsRow>
          <SettingsRow
            label={t(($) => $.contentMarketingNodes.candidates.adoptCreate)}
            // Beside the button, always (FR-056).
            description={t(($) => $.contentMarketingNodes.candidates.adoptNotice)}
          >
            <Button onClick={() => runAdopt("create")} disabled={pending || !open || cancelled}>
              {t(($) => $.contentMarketingNodes.candidates.adoptCreate)}
            </Button>
          </SettingsRow>
          <SettingsRow
            label={t(($) => $.contentMarketingNodes.candidates.adoptLink)}
            description={t(($) => $.contentMarketingNodes.candidates.adoptLinkHint)}
            size="select-wide"
          >
            <div className="space-y-2">
              <Select
                items={linkItems}
                value={linkCardId === "" ? null : linkCardId}
                onValueChange={(value) => setLinkCardId(value ?? "")}
                disabled={pending || !open || cancelled}
              >
                <SelectTrigger aria-label={t(($) => $.contentMarketingNodes.candidates.linkCard)}>
                  <SelectValue placeholder={t(($) => $.contentMarketingNodes.candidates.linkCardPlaceholder)} />
                </SelectTrigger>
                <SelectContent>
                  {linkItems.map((item) => (
                    <SelectItem key={item.value} value={item.value}>
                      {item.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <Button
                variant="outline"
                onClick={() => runAdopt("link")}
                disabled={pending || !open || cancelled || linkCardId === ""}
              >
                {t(($) => $.contentMarketingNodes.candidates.adoptLink)}
              </Button>
            </div>
          </SettingsRow>
        </>
      )}
      <SettingsRow label={t(($) => $.contentMarketingNodes.candidates.lastAction)}>
        <SaveFeedback outcome={outcome} pending={pending} />
      </SettingsRow>
    </SettingsCard>
  );
}

// ---------------------------------------------------------------------------
// Impact: adopted cards whose node was rescheduled or cancelled afterwards.
// Nothing here writes to a card; a decision is recorded on the candidate only.

function impactDates(t: Translate, dates: ImpactItem["before"]): string {
  return `${withDetail(t, dateRange(dates.startsOn, dates.endsOn), dates.timezone)} · ${leadLabel(t, dates.leadDays)}`;
}

function ImpactSection({
  wsId,
  node,
  accounts,
  cards,
  topicsHref,
}: {
  wsId: string;
  node: MarketingNode;
  accounts: MarketingNodeAccountOption[];
  cards: TopicCard[];
  topicsHref: string;
}) {
  const { t } = useT("common");
  const impact = useMarketingNodeImpact(wsId, node.nodeId);
  const list = impact.data ?? [];
  return (
    <SettingsSection
      title={t(($) => $.contentMarketingNodes.impact.title)}
      description={t(($) => $.contentMarketingNodes.impact.description)}
    >
      <SettingsCard>
        {impact.isLoading ? (
          <StateRow label={t(($) => $.contentMarketingNodes.loading)} />
        ) : impact.isError ? (
          <StateRow label={t(($) => $.contentMarketingNodes.loadFailed)} />
        ) : list.length === 0 ? (
          <StateRow label={t(($) => $.contentMarketingNodes.impact.empty)} />
        ) : null}
      </SettingsCard>
      {list.map((item) => (
        <ImpactCard
          key={item.candidateId}
          wsId={wsId}
          nodeId={node.nodeId}
          item={item}
          accounts={accounts}
          cards={cards}
          topicsHref={topicsHref}
        />
      ))}
    </SettingsSection>
  );
}

function ImpactCard({
  wsId,
  nodeId,
  item,
  accounts,
  cards,
  topicsHref,
}: {
  wsId: string;
  nodeId: string;
  item: ImpactItem;
  accounts: MarketingNodeAccountOption[];
  cards: TopicCard[];
  topicsHref: string;
}) {
  const { t } = useT("common");
  const navigation = useNavigation();
  const decide = useDecideMarketingImpact(wsId);
  const [note, setNote] = useState("");
  const [outcome, setOutcome] = useState<SaveOutcome | null>(null);

  const run = (decision: "kept" | "handled") => {
    setOutcome(null);
    decide.mutate(
      { nodeId, candidateId: item.candidateId, decision, note },
      {
        onSuccess: () => setOutcome(SAVED),
        onError: (error) => setOutcome(saveOutcome(error)),
      },
    );
  };

  return (
    <SettingsCard>
      <SettingsRow
        label={t(($) => $.contentMarketingNodes.impact.card, { card: cardLabel(cards, item.topicCardId) })}
        description={[
          item.accountId ? accountLabel(accounts, item.accountId) : t(($) => $.contentMarketingNodes.brandLevel),
          t(($) => $.contentMarketingNodes.impact.cardStatus, { status: cardStatusLabel(t, item.cardStatus) }),
          t(($) => $.contentMarketingNodes.impact.revisions, {
            adopted: item.adoptedRevision,
            current: item.currentRevision,
          }),
        ].join(" · ")}
      >
        <Button variant="outline" onClick={() => navigation.push(topicsHref)}>
          {t(($) => $.contentMarketingNodes.goToTopics)}
        </Button>
      </SettingsRow>
      <SettingsRow label={t(($) => $.contentMarketingNodes.impact.change)} size="text" align="start">
        <ReadOnlyList
          items={
            item.cancelled
              ? [t(($) => $.contentMarketingNodes.impact.nodeCancelled)]
              : [
                  t(($) => $.contentMarketingNodes.impact.before, { dates: impactDates(t, item.before) }),
                  t(($) => $.contentMarketingNodes.impact.after, { dates: impactDates(t, item.after) }),
                ]
          }
          empty=""
        />
      </SettingsRow>
      <SettingsRow label={t(($) => $.contentMarketingNodes.fields.note)} size="text" align="start">
        <Textarea
          value={note}
          onChange={(event) => setNote(event.target.value)}
          aria-label={t(($) => $.contentMarketingNodes.fields.note)}
          rows={2}
          disabled={decide.isPending}
        />
      </SettingsRow>
      <SettingsRow label={t(($) => $.contentMarketingNodes.impact.decide)} description={t(($) => $.contentMarketingNodes.impact.decideHint)}>
        <div className="flex flex-wrap items-center gap-3">
          <SaveFeedback outcome={outcome} pending={decide.isPending} />
          <Button variant="outline" onClick={() => run("kept")} disabled={decide.isPending}>
            {t(($) => $.contentMarketingNodes.impact.kept)}
          </Button>
          <Button variant="outline" onClick={() => run("handled")} disabled={decide.isPending}>
            {t(($) => $.contentMarketingNodes.impact.handled)}
          </Button>
        </div>
      </SettingsRow>
    </SettingsCard>
  );
}
