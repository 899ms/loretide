"use client";

import { useState } from "react";
import { useT } from "@multica/views/i18n";
import {
  bulkSummary,
  canSubmitBulk,
  canSubmitDraft,
  draftProblem,
  draftToRequest,
  duplicateHint,
  emptyDraft,
  organizeActions,
  parseTagInput,
  SOURCE_KINDS,
  SOURCE_STATUSES,
  useBulkOrganizeContentSources,
  useContentSource,
  useContentSourceRevisions,
  useContentSources,
  useCreateContentSource,
  useOrganizeContentSource,
  type Source,
  type SourceDraft,
} from "@multica/core/content/source-inbox";
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
import { PageHeader } from "@multica/views/layout/page-header";
import {
  SettingsCard,
  SettingsContent,
  SettingsRow,
  SettingsSaveState,
  SettingsSection,
  SettingsTab,
} from "@multica/views/settings/layout";

// SOP §4's material inbox: collect, then organise.
//
// Everything that is a rule rather than a layout - what makes a draft valid,
// what the duplicate hint says, which organising moves are offered, how a bulk
// result reads - lives in @multica/core/content/source-inbox/draft.ts and is
// tested there. This file renders what those return.
//
// Nothing here fetches a link. The module cannot, the adapter does not, and
// this page does not either: a url is typed, checked for shape, and stored.

type Translate = ReturnType<typeof useT<"common">>["t"];

export interface SourceInboxPageProps {
  wsId: string;
}

export function SourceInboxPage({ wsId }: SourceInboxPageProps) {
  const { t } = useT("common");
  return (
    <>
      <PageHeader className="xl:hidden">
        <span className="text-body font-medium">{t(($) => $.contentSources.title)}</span>
      </PageHeader>
      <SourceInboxContent wsId={wsId} />
    </>
  );
}

function SourceInboxContent({ wsId }: SourceInboxPageProps) {
  const { t } = useT("common");
  const [status, setStatus] = useState("");
  const [tag, setTag] = useState("");
  const [openId, setOpenId] = useState("");
  const [selected, setSelected] = useState<string[]>([]);

  const sources = useContentSources(wsId, status, tag);
  const list = sources.data ?? [];
  const open = list.find((source) => source.sourceId === openId) ?? null;

  const toggle = (sourceId: string) =>
    setSelected((current) =>
      current.includes(sourceId)
        ? current.filter((id) => id !== sourceId)
        : [...current, sourceId],
    );

  return (
    <SettingsContent>
      <SettingsTab
        title={t(($) => $.contentSources.title)}
        description={t(($) => $.contentSources.description)}
      >
        <ParseUnavailableSection />
        <CollectSection wsId={wsId} onCollected={setOpenId} />

        <SettingsSection
          title={t(($) => $.contentSources.listTitle)}
          description={t(($) => $.contentSources.noDelete)}
        >
          <SettingsCard>
            <SettingsRow label={t(($) => $.contentSources.filterStatus)} size="select">
              <Select
                items={statusItems(t)}
                value={status}
                onValueChange={(value) => setStatus(value ?? "")}
              >
                <SelectTrigger aria-label={t(($) => $.contentSources.filterStatus)}>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {statusItems(t).map((item) => (
                    <SelectItem key={item.value} value={item.value}>
                      {item.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </SettingsRow>
            <SettingsRow label={t(($) => $.contentSources.filterTag)} size="text">
              <Input
                value={tag}
                onChange={(event) => setTag(event.target.value)}
                aria-label={t(($) => $.contentSources.filterTag)}
              />
            </SettingsRow>

            {sources.isPending ? (
              <SettingsRow label={t(($) => $.contentSources.loading)}>
                <span className="text-caption text-muted-foreground" />
              </SettingsRow>
            ) : sources.isError ? (
              <SettingsRow label={t(($) => $.contentSources.failed)}>
                <span className="text-caption text-muted-foreground" />
              </SettingsRow>
            ) : list.length === 0 ? (
              <SettingsRow label={t(($) => $.contentSources.empty)}>
                <span className="text-caption text-muted-foreground" />
              </SettingsRow>
            ) : (
              list.map((source) => (
                <SourceRow
                  key={source.sourceId}
                  source={source}
                  t={t}
                  checked={selected.includes(source.sourceId)}
                  onToggle={() => toggle(source.sourceId)}
                  open={source.sourceId === openId}
                  onOpen={() => setOpenId(source.sourceId === openId ? "" : source.sourceId)}
                />
              ))
            )}
          </SettingsCard>
        </SettingsSection>

        <BulkSection wsId={wsId} selected={selected} onDone={() => setSelected([])} />
        {open ? <SourceDetail key={open.sourceId} wsId={wsId} source={open} /> : null}
      </SettingsTab>
    </SettingsContent>
  );
}

// SOP §7.1 lists a parse status; nothing in this card advances it, so the
// column does not exist and this says so. Same treatment 024 gave the three AI
// entry points and 026 gave the feedback section: present, unavailable, with
// the reason written next to it.
function ParseUnavailableSection() {
  const { t } = useT("common");
  return (
    <SettingsSection title={t(($) => $.contentSources.parseTitle)}>
      <SettingsCard>
        <SettingsRow label={t(($) => $.contentSources.parseUnavailable)}>
          <span className="text-caption text-muted-foreground" />
        </SettingsRow>
      </SettingsCard>
    </SettingsSection>
  );
}

function statusItems(t: Translate): { value: string; label: string }[] {
  return [
    { value: "", label: t(($) => $.contentSources.filterAll) },
    ...SOURCE_STATUSES.map((value) => ({ value, label: statusLabel(t, value) })),
  ];
}

function statusLabel(t: Translate, status: string): string {
  switch (status) {
    case "inbox":
      return t(($) => $.contentSources.statuses.inbox);
    case "organized":
      return t(($) => $.contentSources.statuses.organized);
    case "archived":
      return t(($) => $.contentSources.statuses.archived);
    default:
      // A newer server may name a status this build has no label for. Showing
      // the raw value beats hiding the row.
      return status;
  }
}

function kindLabel(t: Translate, kind: string): string {
  switch (kind) {
    case "pasted_text":
      return t(($) => $.contentSources.kinds.pasted_text);
    case "url":
      return t(($) => $.contentSources.kinds.url);
    default:
      return kind;
  }
}

function SourceRow({
  source,
  t,
  checked,
  onToggle,
  open,
  onOpen,
}: {
  source: Source;
  t: Translate;
  checked: boolean;
  onToggle: () => void;
  open: boolean;
  onOpen: () => void;
}) {
  return (
    <SettingsRow
      label={
        // Weight carries the open state so hovering a selected row does not
        // visually downgrade it to a plain hover.
        <button
          type="button"
          onClick={onOpen}
          data-active={open}
          className="text-left text-body data-[active=true]:font-semibold hover:text-foreground"
        >
          {source.title || source.url || source.sourceId}
        </button>
      }
      description={
        <>
          {kindLabel(t, source.kind)}
          {" · "}
          {statusLabel(t, source.status)}
          {source.tags.length > 0 ? ` · ${source.tags.join(", ")}` : ""}
          {source.historicalImport ? ` · ${t(($) => $.contentSources.historicalLabel)}` : ""}
        </>
      }
    >
      <div className="flex items-center gap-3">
        <Checkbox
          checked={checked}
          onCheckedChange={onToggle}
          aria-label={t(($) => $.contentSources.selectRow)}
        />
        <Button variant="outline" onClick={onOpen}>
          {open ? t(($) => $.contentSources.close) : t(($) => $.contentSources.open)}
        </Button>
      </div>
    </SettingsRow>
  );
}

// ------------------------------------------------------------------ collect

function CollectSection({ wsId, onCollected }: { wsId: string; onCollected: (id: string) => void }) {
  const { t } = useT("common");
  const create = useCreateContentSource(wsId);
  const [draft, setDraft] = useState<SourceDraft>(emptyDraft());
  const [hint, setHint] = useState<string[]>([]);

  const edit = (patch: Partial<SourceDraft>) => setDraft((current) => ({ ...current, ...patch }));
  const problem = draftProblem(draft);
  const kindItems = SOURCE_KINDS.map((value) => ({ value, label: kindLabel(t, value) }));

  const submit = () => {
    setHint([]);
    create.mutate(draftToRequest(draft), {
      onSuccess: (created) => {
        // The hint is shown AFTER the item was created, never instead of
        // creating it: §4 asks the system to point out the repeat, and R-011
        // says both collections stay.
        setHint(duplicateHint(created.duplicates).sourceIds);
        setDraft(emptyDraft());
        onCollected(created.source.sourceId);
      },
    });
  };

  return (
    <SettingsSection title={t(($) => $.contentSources.collectTitle)}>
      <SettingsCard>
        <SettingsRow label={t(($) => $.contentSources.kindLabel)} size="select">
          <Select
            items={kindItems}
            value={draft.kind}
            onValueChange={(value) => edit({ kind: value ?? "pasted_text" })}
          >
            <SelectTrigger aria-label={t(($) => $.contentSources.kindLabel)}>
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

        {draft.kind === "url" ? (
          <SettingsRow
            label={t(($) => $.contentSources.urlLabel)}
            description={t(($) => $.contentSources.urlNotFetched)}
            size="text"
          >
            <Input
              value={draft.url}
              onChange={(event) => edit({ url: event.target.value })}
              placeholder={t(($) => $.contentSources.urlPlaceholder)}
              aria-label={t(($) => $.contentSources.urlLabel)}
            />
          </SettingsRow>
        ) : (
          <SettingsRow
            label={t(($) => $.contentSources.contentLabel)}
            size="text"
            align="start"
          >
            <Textarea
              value={draft.content}
              onChange={(event) => edit({ content: event.target.value })}
              placeholder={t(($) => $.contentSources.contentPlaceholder)}
              aria-label={t(($) => $.contentSources.contentLabel)}
              rows={8}
            />
          </SettingsRow>
        )}

        <SettingsRow label={t(($) => $.contentSources.titleLabel)} size="text">
          <Input
            value={draft.title}
            onChange={(event) => edit({ title: event.target.value })}
            aria-label={t(($) => $.contentSources.titleLabel)}
          />
        </SettingsRow>
        <SettingsRow label={t(($) => $.contentSources.tagsLabel)} size="text">
          <Input
            value={draft.tags}
            onChange={(event) => edit({ tags: event.target.value })}
            placeholder={t(($) => $.contentSources.tagsPlaceholder)}
            aria-label={t(($) => $.contentSources.tagsLabel)}
          />
        </SettingsRow>
        {/* §4 step 1's note and step 4's judgement are two fields, not one:
            "原文、自动摘要和个人判断分开保存". */}
        <SettingsRow label={t(($) => $.contentSources.annotationLabel)} size="text" align="start">
          <Textarea
            value={draft.annotation}
            onChange={(event) => edit({ annotation: event.target.value })}
            aria-label={t(($) => $.contentSources.annotationLabel)}
            rows={3}
          />
        </SettingsRow>
        <SettingsRow label={t(($) => $.contentSources.judgementLabel)} size="text" align="start">
          <Textarea
            value={draft.personalJudgement}
            onChange={(event) => edit({ personalJudgement: event.target.value })}
            aria-label={t(($) => $.contentSources.judgementLabel)}
            rows={3}
          />
        </SettingsRow>
        <SettingsRow
          label={t(($) => $.contentSources.historicalLabel)}
          description={t(($) => $.contentSources.historicalHint)}
        >
          <Checkbox
            checked={draft.historicalImport}
            onCheckedChange={(value) => edit({ historicalImport: value === true })}
            aria-label={t(($) => $.contentSources.historicalLabel)}
          />
        </SettingsRow>

        <SettingsRow
          label={t(($) => $.contentSources.collect)}
          description={problem === "" ? null : problemLabel(t, problem)}
        >
          <div className="flex items-center gap-3">
            <SettingsSaveState
              status={create.isPending ? "saving" : create.isError ? "error" : "idle"}
              savingLabel={t(($) => $.contentSources.collect)}
              savedLabel={t(($) => $.contentSources.collected)}
              errorLabel={t(($) => $.contentSources.failed)}
            />
            <Button disabled={!canSubmitDraft(draft, create.isPending)} onClick={submit}>
              {t(($) => $.contentSources.collect)}
            </Button>
          </div>
        </SettingsRow>

        {hint.length > 0 ? (
          <SettingsRow
            label={t(($) => $.contentSources.duplicateHint, { count: hint.length })}
            description={hint.join(", ")}
          >
            <span className="text-caption text-muted-foreground" />
          </SettingsRow>
        ) : null}
      </SettingsCard>
    </SettingsSection>
  );
}

function problemLabel(t: Translate, problem: string): string {
  switch (problem) {
    case "kind":
      return t(($) => $.contentSources.problem.kind);
    case "url":
      return t(($) => $.contentSources.problem.url);
    default:
      return t(($) => $.contentSources.problem.content);
  }
}

// --------------------------------------------------------------------- bulk

function BulkSection({
  wsId,
  selected,
  onDone,
}: {
  wsId: string;
  selected: string[];
  onDone: () => void;
}) {
  const { t } = useT("common");
  const bulk = useBulkOrganizeContentSources(wsId);
  const [tags, setTags] = useState("");
  const [status, setStatus] = useState("");
  const [summary, setSummary] = useState<ReturnType<typeof bulkSummary> | null>(null);

  const addTags = parseTagInput(tags);
  const items = statusItems(t);

  const apply = () => {
    setSummary(null);
    const body: Record<string, unknown> = { source_ids: selected };
    if (addTags.length > 0) body.add_tags = addTags;
    if (status !== "") body.status = status;
    bulk.mutate(body, {
      onSuccess: (results) => {
        // Per item, because a batch that half worked has to say which half.
        setSummary(bulkSummary(results));
        setTags("");
        setStatus("");
        onDone();
      },
    });
  };

  return (
    <SettingsSection
      title={t(($) => $.contentSources.bulkTitle)}
      description={t(($) => $.contentSources.selected, { count: selected.length })}
    >
      <SettingsCard>
        <SettingsRow label={t(($) => $.contentSources.bulkAddTags)} size="text">
          <Input
            value={tags}
            onChange={(event) => setTags(event.target.value)}
            placeholder={t(($) => $.contentSources.tagsPlaceholder)}
            aria-label={t(($) => $.contentSources.bulkAddTags)}
          />
        </SettingsRow>
        <SettingsRow label={t(($) => $.contentSources.bulkStatus)} size="select">
          <Select items={items} value={status} onValueChange={(value) => setStatus(value ?? "")}>
            <SelectTrigger aria-label={t(($) => $.contentSources.bulkStatus)}>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {items.map((item) => (
                <SelectItem key={item.value} value={item.value}>
                  {item.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </SettingsRow>
        <SettingsRow label={t(($) => $.contentSources.bulkApply)}>
          <Button
            disabled={bulk.isPending || !canSubmitBulk(selected, addTags, status)}
            onClick={apply}
          >
            {t(($) => $.contentSources.bulkApply)}
          </Button>
        </SettingsRow>
        {summary ? (
          <SettingsRow
            label={t(($) => $.contentSources.bulkResult, {
              ok: summary.ok,
              failed: summary.failed,
            })}
            description={
              summary.failedIds.length > 0
                ? t(($) => $.contentSources.bulkFailedIds, { ids: summary.failedIds.join(", ") })
                : null
            }
          >
            <span className="text-caption text-muted-foreground" />
          </SettingsRow>
        ) : null}
      </SettingsCard>
    </SettingsSection>
  );
}

// ------------------------------------------------------------------- detail

function SourceDetail({ wsId, source }: { wsId: string; source: Source }) {
  const { t } = useT("common");
  const detail = useContentSource(wsId, source.sourceId);
  const revisions = useContentSourceRevisions(wsId, source.sourceId);
  const organize = useOrganizeContentSource(wsId);

  const [title, setTitle] = useState(source.title);
  const [tags, setTags] = useState(source.tags.join(", "));
  const [annotation, setAnnotation] = useState(source.annotation);
  const [judgement, setJudgement] = useState(source.personalJudgement);

  const snapshot = detail.data?.snapshot ?? null;
  const history = revisions.data ?? [];

  const save = (patch: Record<string, unknown>) =>
    organize.mutate({ sourceId: source.sourceId, patch });

  return (
    <>
      <SettingsSection
        title={t(($) => $.contentSources.originalTitle)}
        description={t(($) => $.contentSources.originalReadOnly)}
      >
        <SettingsCard>
          <SettingsRow label={t(($) => $.contentSources.capturedAt)}>
            <span className="text-caption text-muted-foreground">{source.capturedAt}</span>
          </SettingsRow>
          <SettingsRow label={t(($) => $.contentSources.recordedBy)}>
            <span className="text-caption text-muted-foreground">{source.recordedBy}</span>
          </SettingsRow>
          {snapshot ? (
            <>
              {/* Read-only, always. The body is a fact about the collection and
                  the hash is taken over it; editing it here would make the two
                  disagree. */}
              <SettingsRow
                label={t(($) => $.contentSources.originalTitle)}
                size="text"
                align="start"
              >
                <Textarea value={snapshot.content} readOnly rows={12} />
              </SettingsRow>
              <SettingsRow label={t(($) => $.contentSources.hashLabel)}>
                <span className="text-caption break-all text-muted-foreground">
                  {snapshot.contentHash}
                </span>
              </SettingsRow>
            </>
          ) : (
            <SettingsRow
              label={t(($) => $.contentSources.noOriginal)}
              description={source.url}
            >
              <span className="text-caption text-muted-foreground" />
            </SettingsRow>
          )}
        </SettingsCard>
      </SettingsSection>

      <SettingsSection title={t(($) => $.contentSources.organizeTitle)}>
        <SettingsCard>
          <SettingsRow label={t(($) => $.contentSources.titleLabel)} size="text">
            <Input
              value={title}
              onChange={(event) => setTitle(event.target.value)}
              aria-label={t(($) => $.contentSources.titleLabel)}
            />
          </SettingsRow>
          <SettingsRow label={t(($) => $.contentSources.tagsLabel)} size="text">
            <Input
              value={tags}
              onChange={(event) => setTags(event.target.value)}
              aria-label={t(($) => $.contentSources.tagsLabel)}
            />
          </SettingsRow>
          <SettingsRow label={t(($) => $.contentSources.annotationLabel)} size="text" align="start">
            <Textarea
              value={annotation}
              onChange={(event) => setAnnotation(event.target.value)}
              aria-label={t(($) => $.contentSources.annotationLabel)}
              rows={3}
            />
          </SettingsRow>
          {/* Kept apart from the original above and from the note beside it. */}
          <SettingsRow label={t(($) => $.contentSources.judgementLabel)} size="text" align="start">
            <Textarea
              value={judgement}
              onChange={(event) => setJudgement(event.target.value)}
              aria-label={t(($) => $.contentSources.judgementLabel)}
              rows={3}
            />
          </SettingsRow>
          <SettingsRow label={t(($) => $.contentSources.save)}>
            <div className="flex items-center gap-3">
              <SettingsSaveState
                status={organize.isPending ? "saving" : organize.isError ? "error" : "idle"}
                savingLabel={t(($) => $.contentSources.save)}
                savedLabel={t(($) => $.contentSources.saved)}
                errorLabel={t(($) => $.contentSources.failed)}
              />
              <Button
                disabled={organize.isPending}
                onClick={() =>
                  save({
                    title,
                    tags: parseTagInput(tags),
                    annotation,
                    personal_judgement: judgement,
                  })
                }
              >
                {t(($) => $.contentSources.save)}
              </Button>
            </div>
          </SettingsRow>
          <SettingsRow
            label={t(($) => $.contentSources.filterStatus)}
            description={statusLabel(t, source.status)}
          >
            <div className="flex items-center gap-3">
              {organizeActions(source.status).map((next) => (
                <Button
                  key={next}
                  variant="outline"
                  disabled={organize.isPending}
                  onClick={() => save({ status: next })}
                >
                  {statusLabel(t, next)}
                </Button>
              ))}
            </div>
          </SettingsRow>
        </SettingsCard>
      </SettingsSection>

      <SettingsSection title={t(($) => $.contentSources.historyTitle)}>
        <SettingsCard>
          {history.length === 0 ? (
            <SettingsRow label={t(($) => $.contentSources.historyEmpty)}>
              <span className="text-caption text-muted-foreground" />
            </SettingsRow>
          ) : (
            history.map((revision) => (
              <SettingsRow
                key={revision.revisionId}
                label={revision.createdAt}
                description={`${t(($) => $.contentSources.changedFields)}: ${revision.changedFields
                  .map((field) => fieldLabel(t, field))
                  .join(", ")}`}
              >
                <span className="text-caption text-muted-foreground">{revision.actorId}</span>
              </SettingsRow>
            ))
          )}
        </SettingsCard>
      </SettingsSection>
    </>
  );
}

function fieldLabel(t: Translate, field: string): string {
  switch (field) {
    case "title":
      return t(($) => $.contentSources.fields.title);
    case "tags":
      return t(($) => $.contentSources.fields.tags);
    case "annotation":
      return t(($) => $.contentSources.fields.annotation);
    case "personal_judgement":
      return t(($) => $.contentSources.fields.personal_judgement);
    case "status":
      return t(($) => $.contentSources.fields.status);
    default:
      return field;
  }
}
