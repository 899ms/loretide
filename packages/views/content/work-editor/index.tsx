"use client";

import { useEffect, useRef, useState, type ReactNode } from "react";
import { useT } from "@multica/views/i18n";
import {
  AI_ENTRY_POINTS,
  ARTIFACT_KINDS,
  canSaveVersion,
  comparisonPair,
  defaultComparisonTarget,
  draftBadge,
  draftIsUnsent,
  describeVersion,
  nextArtifactPosition,
  useArtifactVersionAction,
  useArtifactVersions,
  useAutosaveArtifact,
  useContentArtifacts,
  useContentWorks,
  useCreateContentArtifact,
  useCreateContentWork,
  type Artifact,
  type ArtifactKind,
  type ArtifactVersion,
  type Work,
} from "@multica/core/content/work-editor";
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
  SettingsRow,
  SettingsSaveState,
  SettingsSection,
} from "@multica/views/settings/layout";

// The work editor (specs/024, SOP 7), sitting on the topic card detail after
// the start block - which is where the ruling put its entry point.
//
// Composition is the account and topic pages': SettingsSection / SettingsCard /
// SettingsRow with one control per row, a save state beside a button. Nothing
// here is a new control and nothing sets a colour.
//
// Every rule lives in @multica/core/content/work-editor/editor-state.ts, which
// has node tests. This file is wiring: which hook, which string, which row.

type Translate = ReturnType<typeof useT<"common">>["t"];

/** How long the editor waits after the last keystroke before flushing. Long
 *  enough not to send a request per character, short enough that "it saves on
 *  its own" is true rather than aspirational. */
const AUTOSAVE_IDLE_MS = 800;

/**
 * What an injected block underneath the editor is told about the open document.
 *
 * It exists so another module's block - review-delivery's "submit for review",
 * for one - can sit under the version history without this module importing it.
 * work-editor's declared dependencies do not include review-delivery, and the
 * direction is deliberate: the reviewer depends on the writer, not the reverse.
 * The adapter is what puts the two together.
 */
export interface ArtifactExtrasContext {
  wsId: string;
  workId: string;
  artifactId: string;
  artifactKind: string;
  /** The version a reviewer would be given: the newest saved one. "" when the
   *  document has none yet. */
  latestVersionId: string;
  versionCount: number;
}

export function WorkSections({
  wsId,
  topicCardId,
  renderArtifactExtras,
}: {
  wsId: string;
  topicCardId: string;
  renderArtifactExtras?: (context: ArtifactExtrasContext) => ReactNode;
}) {
  const { t } = useT("common");
  const works = useContentWorks(wsId, topicCardId);
  const [openWorkId, setOpenWorkId] = useState("");

  const list = works.data ?? [];
  const open = list.find((work) => work.workId === openWorkId) ?? null;

  return (
    <>
      <SettingsSection
        title={t(($) => $.contentWorks.sectionTitle)}
        description={t(($) => $.contentWorks.sectionDescription)}
      >
        <SettingsCard>
          {list.length === 0 ? (
            <SettingsRow label={t(($) => $.contentWorks.empty)}>
              <span className="text-caption text-muted-foreground" />
            </SettingsRow>
          ) : (
            list.map((work) => (
              <WorkRow
                key={work.workId}
                work={work}
                open={work.workId === openWorkId}
                onToggle={() => setOpenWorkId(work.workId === openWorkId ? "" : work.workId)}
              />
            ))
          )}
          <CreateWorkRow wsId={wsId} topicCardId={topicCardId} onCreated={setOpenWorkId} />
        </SettingsCard>
      </SettingsSection>

      {open ? (
        <WorkDocuments
          key={open.workId}
          wsId={wsId}
          work={open}
          renderArtifactExtras={renderArtifactExtras}
        />
      ) : null}
    </>
  );
}

function WorkRow({
  work,
  open,
  onToggle,
}: {
  work: Work;
  open: boolean;
  onToggle: () => void;
}) {
  const { t } = useT("common");
  return (
    <SettingsRow label={work.title || work.workId}>
      <Button variant="outline" onClick={onToggle}>
        {open ? t(($) => $.contentWorks.close) : t(($) => $.contentWorks.open)}
      </Button>
    </SettingsRow>
  );
}

function CreateWorkRow({
  wsId,
  topicCardId,
  onCreated,
}: {
  wsId: string;
  topicCardId: string;
  onCreated: (workId: string) => void;
}) {
  const { t } = useT("common");
  const create = useCreateContentWork(wsId);
  const [title, setTitle] = useState("");

  return (
    <>
      <SettingsRow label={t(($) => $.contentWorks.titleLabel)} size="text">
        <Input
          value={title}
          onChange={(event) => setTitle(event.target.value)}
          placeholder={t(($) => $.contentWorks.titlePlaceholder)}
          aria-label={t(($) => $.contentWorks.titleLabel)}
        />
      </SettingsRow>
      <SettingsRow label={t(($) => $.contentWorks.create)}>
        <div className="flex items-center gap-3">
          <ActionState pending={create.isPending} failed={create.isError} />
          <Button
            disabled={create.isPending || !title.trim()}
            onClick={() =>
              create.mutate(
                // Started from no snapshot: SOP 6.2 requires writing to work
                // with nothing else in place.
                { topicCardId, snapshotId: "", title },
                {
                  onSuccess: (work) => {
                    setTitle("");
                    if (work.workId) onCreated(work.workId);
                  },
                },
              )
            }
          >
            {t(($) => $.contentWorks.create)}
          </Button>
        </div>
      </SettingsRow>
    </>
  );
}

function WorkDocuments({
  wsId,
  work,
  renderArtifactExtras,
}: {
  wsId: string;
  work: Work;
  renderArtifactExtras?: (context: ArtifactExtrasContext) => ReactNode;
}) {
  const { t } = useT("common");
  const artifacts = useContentArtifacts(wsId, work.workId);
  const create = useCreateContentArtifact(wsId, work.workId);
  const [openArtifactId, setOpenArtifactId] = useState("");
  const [kind, setKind] = useState<ArtifactKind>("body");
  const [title, setTitle] = useState("");

  const list = artifacts.data ?? [];
  const open = list.find((artifact) => artifact.artifactId === openArtifactId) ?? null;
  const kindItems = ARTIFACT_KINDS.map((value) => ({ value, label: kindLabel(t, value) }));

  return (
    <>
      <SettingsSection title={t(($) => $.contentWorks.documentsTitle)}>
        <SettingsCard>
          {list.length === 0 ? (
            <SettingsRow label={t(($) => $.contentWorks.documentsEmpty)}>
              <span className="text-caption text-muted-foreground" />
            </SettingsRow>
          ) : (
            list.map((artifact) => (
              <SettingsRow
                key={artifact.artifactId}
                label={artifact.title || artifact.artifactId}
                description={kindLabel(t, artifact.kind)}
              >
                <Button
                  variant="outline"
                  onClick={() =>
                    setOpenArtifactId(
                      artifact.artifactId === openArtifactId ? "" : artifact.artifactId,
                    )
                  }
                >
                  {artifact.artifactId === openArtifactId
                    ? t(($) => $.contentWorks.close)
                    : t(($) => $.contentWorks.open)}
                </Button>
              </SettingsRow>
            ))
          )}
          <SettingsRow label={t(($) => $.contentWorks.kindLabel)} size="select-wide">
            <Select
              items={kindItems}
              value={kind}
              onValueChange={(value) => setKind((value as ArtifactKind) ?? "body")}
            >
              <SelectTrigger aria-label={t(($) => $.contentWorks.kindLabel)}>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {ARTIFACT_KINDS.map((value) => (
                  <SelectItem key={value} value={value}>
                    {kindLabel(t, value)}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </SettingsRow>
          <SettingsRow label={t(($) => $.contentWorks.titleLabel)} size="text">
            <Input
              value={title}
              onChange={(event) => setTitle(event.target.value)}
              placeholder={t(($) => $.contentWorks.titlePlaceholder)}
              aria-label={t(($) => $.contentWorks.titleLabel)}
            />
          </SettingsRow>
          <SettingsRow label={t(($) => $.contentWorks.addDocument)}>
            <div className="flex items-center gap-3">
              <ActionState pending={create.isPending} failed={create.isError} />
              <Button
                disabled={create.isPending || !title.trim()}
                onClick={() =>
                  create.mutate(
                    { kind, title, position: nextArtifactPosition(list) },
                    {
                      onSuccess: (artifact) => {
                        setTitle("");
                        if (artifact.artifactId) setOpenArtifactId(artifact.artifactId);
                      },
                    },
                  )
                }
              >
                {t(($) => $.contentWorks.addDocument)}
              </Button>
            </div>
          </SettingsRow>
        </SettingsCard>
      </SettingsSection>

      {open ? (
        <ArtifactEditor
          key={open.artifactId}
          wsId={wsId}
          work={work}
          artifact={open}
          renderArtifactExtras={renderArtifactExtras}
        />
      ) : null}
    </>
  );
}

function ArtifactEditor({
  wsId,
  work,
  artifact,
  renderArtifactExtras,
}: {
  wsId: string;
  work: Work;
  artifact: Artifact;
  renderArtifactExtras?: (context: ArtifactExtrasContext) => ReactNode;
}) {
  const { t } = useT("common");
  const autosave = useAutosaveArtifact(wsId, work.workId);
  const versions = useArtifactVersions(wsId, work.workId, artifact.artifactId);
  const versionAction = useArtifactVersionAction(wsId, work.workId, artifact.artifactId);

  // The text being typed is client state. Putting it in the query cache would
  // make every keystroke a cache write and every refetch a chance to overwrite
  // what somebody is in the middle of writing.
  const [body, setBody] = useState(artifact.draftBody);
  const flushed = useRef(artifact.draftBody);

  // A restore replaces the stored draft. Adopt the server's copy when it moves
  // to something this editor did not send, and only then: doing it on every
  // change would fight the person typing.
  useEffect(() => {
    if (artifact.draftBody !== flushed.current) {
      flushed.current = artifact.draftBody;
      setBody(artifact.draftBody);
    }
  }, [artifact.draftBody]);

  // Autosave: flush once typing has settled. Not on every keystroke, and not
  // only on blur - "it saves on its own" has to be true while the page is open.
  useEffect(() => {
    if (body === flushed.current) return;
    const timer = setTimeout(() => {
      const pending = body;
      autosave.mutate(
        { artifactId: artifact.artifactId, draftBody: pending },
        { onSuccess: () => { flushed.current = pending; } },
      );
    }, AUTOSAVE_IDLE_MS);
    return () => clearTimeout(timer);
    // autosave is a stable mutation object from the query client.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [body, artifact.artifactId]);

  const history = versions.data ?? [];
  const badge = draftBadge(body, artifact);
  const unsent = draftIsUnsent(body, artifact);

  return (
    <>
      <SettingsSection title={t(($) => $.contentWorks.editorTitle)}>
        <SettingsCard>
          <SettingsRow
            label={t(($) => $.contentWorks.bodyLabel)}
            description={t(($) => $.contentWorks.draftStatus[badge])}
            size="text"
            align="start"
          >
            <Textarea
              value={body}
              onChange={(event) => setBody(event.target.value)}
              placeholder={t(($) => $.contentWorks.bodyPlaceholder)}
              aria-label={t(($) => $.contentWorks.bodyLabel)}
              rows={16}
            />
          </SettingsRow>
          <SettingsRow label={t(($) => $.contentWorks.saveVersion)}>
            <div className="flex items-center gap-3">
              {autosave.isPending || unsent ? (
                <span className="text-caption text-muted-foreground">
                  {t(($) => $.contentWorks.autosaving)}
                </span>
              ) : null}
              {autosave.isError ? (
                <span className="text-caption text-muted-foreground">
                  {t(($) => $.contentWorks.autosaveFailed)}
                </span>
              ) : null}
              <ActionState
                pending={versionAction.isPending}
                failed={versionAction.isError}
              />
              <Button
                disabled={
                  versionAction.isPending ||
                  autosave.isPending ||
                  unsent ||
                  !canSaveVersion(body, artifact, history)
                }
                onClick={() => versionAction.mutate({ action: "save" })}
              >
                {t(($) => $.contentWorks.saveVersion)}
              </Button>
            </div>
          </SettingsRow>
        </SettingsCard>
      </SettingsSection>

      <AIEntryPoints />

      <VersionHistory
        versions={history}
        pending={versionAction.isPending}
        onRestore={(versionId) => versionAction.mutate({ action: "restore", versionId })}
        onAdopt={(versionId) => versionAction.mutate({ action: "adopt", versionId })}
      />

      {/* Injected by the adapter, underneath the history: what a reviewer is
          asked to look at is a version, so the blocks that act on one belong
          after the list of them. Nothing is rendered when nobody injected. */}
      {renderArtifactExtras?.({
        wsId,
        workId: work.workId,
        artifactId: artifact.artifactId,
        artifactKind: artifact.kind,
        // history arrives newest first.
        latestVersionId: history[0]?.versionId ?? "",
        versionCount: history.length,
      })}
    </>
  );
}

// The three model-backed entry points. Present and disabled, with the reason
// written next to them: hiding them would say the product does not intend to do
// this, a blank would read as unfinished, and a spinner as "any moment now".
// There is no fabricated candidate anywhere in this file.
function AIEntryPoints() {
  const { t } = useT("common");
  return (
    <SettingsSection title={t(($) => $.contentWorks.aiTitle)}>
      <SettingsCard>
        {AI_ENTRY_POINTS.map((entry) => (
          <SettingsRow
            key={entry.id}
            label={t(($) => $.contentWorks.ai[entry.id])}
            description={t(($) => $.contentWorks.aiUnavailable)}
          >
            <Button variant="outline" disabled>
              {t(($) => $.contentWorks.ai[entry.id])}
            </Button>
          </SettingsRow>
        ))}
      </SettingsCard>
    </SettingsSection>
  );
}

function VersionHistory({
  versions,
  pending,
  onRestore,
  onAdopt,
}: {
  versions: ArtifactVersion[];
  pending: boolean;
  onRestore: (versionId: string) => void;
  onAdopt: (versionId: string) => void;
}) {
  const { t } = useT("common");
  const [selectedId, setSelectedId] = useState("");
  const [againstId, setAgainstId] = useState("");

  if (versions.length === 0) {
    return (
      <SettingsSection title={t(($) => $.contentWorks.historyTitle)}>
        <SettingsCard>
          <SettingsRow label={t(($) => $.contentWorks.historyEmpty)}>
            <span className="text-caption text-muted-foreground" />
          </SettingsRow>
        </SettingsCard>
      </SettingsSection>
    );
  }

  const select = (versionId: string) => {
    setSelectedId(versionId);
    setAgainstId(defaultComparisonTarget(versions, versionId));
  };
  const pair = comparisonPair(versions, selectedId, againstId);
  const againstItems = versions
    .filter((version) => version.versionId !== selectedId)
    .map((version) => ({
      value: version.versionId,
      label: t(($) => $.contentWorks.revision, { number: version.revision }),
    }));

  return (
    <>
      <SettingsSection
        title={t(($) => $.contentWorks.historyTitle)}
        description={t(($) => $.contentWorks.restoreHint)}
      >
        <SettingsCard>
          {versions.map((version) => {
            const described = describeVersion(version);
            return (
              <SettingsRow
                key={version.versionId}
                label={
                  <button
                    type="button"
                    onClick={() => select(version.versionId)}
                    data-active={version.versionId === selectedId}
                    // The selected row stays legible under hover because the
                    // weight carries the state and hover only moves colour.
                    className="text-left text-body data-[active=true]:font-semibold hover:text-foreground"
                  >
                    {t(($) => $.contentWorks.revision, { number: version.revision })}
                  </button>
                }
                // Two facts, two labels. SOP 7.1 asks for "来源与动作记录";
                // folding them would make "restored" look like an answer to
                // who wrote this, which it is not.
                description={
                  <>
                    {t(($) => $.contentWorks.sourceLabel)}
                    {": "}
                    {sourceLabel(t, described.source)}
                    {" · "}
                    {t(($) => $.contentWorks.actionLabel)}
                    {": "}
                    {actionLabel(t, described.action)}
                    {described.from ? (
                      <>
                        {" · "}
                        {t(($) => $.contentWorks.fromLabel)}
                        {": "}
                        {described.from}
                      </>
                    ) : null}
                  </>
                }
              >
                <div className="flex items-center gap-3">
                  <Button
                    variant="outline"
                    disabled={pending}
                    onClick={() => onRestore(version.versionId)}
                  >
                    {t(($) => $.contentWorks.restore)}
                  </Button>
                  <Button
                    variant="outline"
                    disabled={pending}
                    onClick={() => onAdopt(version.versionId)}
                  >
                    {t(($) => $.contentWorks.adopt)}
                  </Button>
                </div>
              </SettingsRow>
            );
          })}
        </SettingsCard>
      </SettingsSection>

      <SettingsSection
        title={t(($) => $.contentWorks.compareTitle)}
        description={t(($) => $.contentWorks.compareHint)}
      >
        <SettingsCard>
          <SettingsRow label={t(($) => $.contentWorks.compareAgainst)} size="select-wide">
            <Select
              items={againstItems}
              value={againstId}
              onValueChange={(value) => setAgainstId(value ?? "")}
            >
              <SelectTrigger aria-label={t(($) => $.contentWorks.compareAgainst)}>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {againstItems.map((item) => (
                  <SelectItem key={item.value} value={item.value}>
                    {item.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </SettingsRow>
          {pair ? (
            // Two full texts side by side, not a word-level diff: the plan
            // fixed this so the page PR would not decide to add a diff
            // dependency.
            <div className="px-4 py-3.5">
              <div className="grid gap-4 sm:grid-cols-2">
                <div className="min-w-0">
                  <div className="mb-1 text-caption text-muted-foreground">
                    {t(($) => $.contentWorks.compareLeft)}
                    {" \u00b7 "}
                    {t(($) => $.contentWorks.revision, { number: pair.left.revision })}
                  </div>
                  <Textarea value={pair.left.body} readOnly rows={12} />
                </div>
                <div className="min-w-0">
                  <div className="mb-1 text-caption text-muted-foreground">
                    {t(($) => $.contentWorks.compareRight)}
                    {" \u00b7 "}
                    {t(($) => $.contentWorks.revision, { number: pair.right.revision })}
                  </div>
                  <Textarea value={pair.right.body} readOnly rows={12} />
                </div>
              </div>
            </div>
          ) : (
            <SettingsRow label={t(($) => $.contentWorks.compareNone)}>
              <span className="text-caption text-muted-foreground" />
            </SettingsRow>
          )}
        </SettingsCard>
      </SettingsSection>
    </>
  );
}

function ActionState({ pending, failed }: { pending: boolean; failed: boolean }) {
  const { t } = useT("common");
  return (
    <SettingsSaveState
      status={pending ? "saving" : failed ? "error" : "idle"}
      savingLabel={t(($) => $.contentWorks.autosaving)}
      savedLabel={t(($) => $.contentWorks.draftStatus.saved)}
      errorLabel={t(($) => $.contentWorks.failed)}
    />
  );
}

// A kind, source or action this build has not heard of still has to render as
// something. The raw value is a worse label than a translated one and a better
// one than a blank cell.
function kindLabel(t: Translate, kind: string): string {
  switch (kind) {
    case "body":
      return t(($) => $.contentWorks.kinds.body);
    case "channel_draft":
      return t(($) => $.contentWorks.kinds.channel_draft);
    default:
      return kind;
  }
}

function sourceLabel(t: Translate, source: string): string {
  switch (source) {
    case "generated":
      return t(($) => $.contentWorks.sources.generated);
    case "edited":
      return t(($) => $.contentWorks.sources.edited);
    case "adopted":
      return t(($) => $.contentWorks.sources.adopted);
    default:
      return source;
  }
}

function actionLabel(t: Translate, action: string): string {
  switch (action) {
    case "saved":
      return t(($) => $.contentWorks.actions.saved);
    case "restored":
      return t(($) => $.contentWorks.actions.restored);
    case "adopted":
      return t(($) => $.contentWorks.actions.adopted);
    default:
      return action;
  }
}
