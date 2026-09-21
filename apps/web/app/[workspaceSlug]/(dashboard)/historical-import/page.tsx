"use client";

import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { ApiError, api } from "@multica/core/api";
import { useAuthStore } from "@multica/core/auth";
import { useWorkspaceId } from "@multica/core/hooks";
import {
  parseArtifact,
  parseArtifactVersion,
  parseWork,
  workEditorKeys,
} from "@multica/core/content/work-editor";
import {
  parsePublication,
} from "@multica/core/content/review-delivery";
import { feedbackKeys } from "@multica/core/content/feedback-learning";
import { Button } from "@multica/ui/components/ui/button";
import { Input } from "@multica/ui/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@multica/ui/components/ui/select";
import { Textarea } from "@multica/ui/components/ui/textarea";
import { WorkSections } from "@multica/views/content/work-editor";
import { useT } from "@multica/views/i18n";
import { PageHeader } from "@multica/views/layout/page-header";
import {
  SettingsCard,
  SettingsContent,
  SettingsRow,
  SettingsSaveState,
  SettingsSection,
  SettingsTab,
} from "@multica/views/settings/layout";

import {
  HISTORICAL_IMPORT_STEPS,
  createImportSession,
  deriveHistoricalImportTitle,
  editableHistoricalImportFields,
  runHistoricalImport,
  validateHistoricalImportDraft,
  type HistoricalImportDraft,
  type HistoricalImportOperationContext,
  type HistoricalImportOperations,
  type HistoricalImportSession,
} from "./workflow";

const CHANNELS = ["xiaohongshu", "wechat_mp", "douyin", "shipinhao"] as const;

const EMPTY_DRAFT: HistoricalImportDraft = {
  title: "",
  body: "",
  channel: "",
  publishedAt: "",
  platformAccount: "",
  pageUrlOrContentId: "",
};

type Translate = ReturnType<typeof useT<"common">>["t"];

function newSession(): HistoricalImportSession {
  const random = globalThis.crypto?.randomUUID?.() ?? `${Date.now()}-${Math.random()}`;
  return createImportSession(random);
}
export default function Page() {
  const wsId = useWorkspaceId();
  return <HistoricalImportPage key={wsId} wsId={wsId} />;
}
function HistoricalImportPage({ wsId }: { wsId: string }) {
  const { t } = useT("common");
  const queryClient = useQueryClient();
  const user = useAuthStore((state) => state.user);
  const [draft, setDraft] = useState<HistoricalImportDraft>(EMPTY_DRAFT);
  const [session, setSession] = useState<HistoricalImportSession>(newSession);

  const operations: HistoricalImportOperations = {
    createWork: async ({ draft: input }) => {
      const historicalWork = {
        topic_card_id: "",
        snapshot_id: "",
        title: deriveHistoricalImportTitle(input.title, input.body),
        historical_import: true,
      };
      const work = parseWork(
        await api.createContentWork(historicalWork),
      );
      await queryClient.invalidateQueries({ queryKey: workEditorKeys.all(wsId) });
      return work.workId;
    },
    createArtifact: async (context) => {
      const artifactId = await ensureHistoricalArtifact(context);
      await queryClient.invalidateQueries({
        queryKey: workEditorKeys.artifacts(wsId, context.outputs.workId),
      });
      return artifactId;
    },
    importVersion: async ({ outputs }) => {
      const version = parseArtifactVersion(
        await api.importContentArtifactVersion(outputs.workId, outputs.artifactId),
      );
      await queryClient.invalidateQueries({
        queryKey: workEditorKeys.versions(wsId, outputs.workId, outputs.artifactId),
      });
      return version.versionId;
    },
    createPublication: async ({ draft: input, outputs }) => {
      const publication = parsePublication(
        await api.recordContentPublication({
          artifact_id: outputs.artifactId,
          delivery_task_id: "",
          channel: input.channel,
          status: "reported_published",
          declared_by: user?.name || user?.email || user?.id || "",
          page_url_or_content_id: input.pageUrlOrContentId,
          receipt_note: "",
          verification_note: "",
          published_at: toRFC3339(input.publishedAt),
          platform_account: input.platformAccount,
          platform_edited: false,
          edit_note: "",
          version_match: "unknown",
          version_id: outputs.versionId,
          historical_import: true,
        }),
      );
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ["contentPublications", wsId] }),
        queryClient.invalidateQueries({ queryKey: feedbackKeys.pending(wsId) }),
      ]);
      return publication.publicationRecordId;
    },
  };

  const run = useMutation({
    mutationFn: async ({
      submittedDraft,
      current,
    }: {
      submittedDraft: HistoricalImportDraft;
      current: HistoricalImportSession;
    }) => runHistoricalImport(submittedDraft, current, operations, setSession),
    onSuccess: setSession,
  });

  const validation = validateHistoricalImportDraft(draft);
  const finished = session.steps.every((step) => step.status === "completed");
  const editableFields = new Set(editableHistoricalImportFields(session));
  const failedField = errorField(session.failure?.error);
  const submit = () => run.mutate({ submittedDraft: draft, current: session });

  return (
    <>
      <PageHeader className="xl:hidden">
        <span className="text-body font-medium">
          {t(($) => $.historicalImport.title)}
        </span>
      </PageHeader>
      <SettingsContent>
        <SettingsTab
          title={t(($) => $.historicalImport.title)}
          description={t(($) => $.historicalImport.description)}
        >
          <SettingsSection
            title={t(($) => $.historicalImport.formTitle)}
            description={t(($) => $.historicalImport.formDescription)}
          >
            <SettingsCard>
              <SettingsRow label={t(($) => $.historicalImport.titleLabel)} size="text">
                <Input
                  value={draft.title}
                  onChange={(event) => setDraft({ ...draft, title: event.target.value })}
                  placeholder={t(($) => $.historicalImport.titlePlaceholder)}
                  aria-label={t(($) => $.historicalImport.titleLabel)}
                  disabled={run.isPending || finished || !editableFields.has("title")}
                />
              </SettingsRow>
              <SettingsRow
                label={t(($) => $.historicalImport.bodyLabel)}
                description={
                  validation?.field === "body"
                    ? validation.reason === "too_long"
                      ? t(($) => $.historicalImport.bodyTooLong)
                      : t(($) => $.historicalImport.required)
                    : t(($) => $.historicalImport.bodyHint)
                }
                size="text"
                align="start"
              >
                <Textarea
                  value={draft.body}
                  onChange={(event) => setDraft({ ...draft, body: event.target.value })}
                  placeholder={t(($) => $.historicalImport.bodyPlaceholder)}
                  aria-label={t(($) => $.historicalImport.bodyLabel)}
                  aria-invalid={validation?.field === "body"}
                  rows={12}
                  disabled={run.isPending || finished || !editableFields.has("body")}
                />
              </SettingsRow>
              <SettingsRow label={t(($) => $.historicalImport.channelLabel)} size="select-wide">
                <Select
                  items={CHANNELS.map((value) => ({ value, label: channelLabel(t, value) }))}
                  value={draft.channel}
                  onValueChange={(value) => setDraft({ ...draft, channel: value ?? "" })}
                  disabled={run.isPending || finished || !editableFields.has("channel")}
                >
                  <SelectTrigger
                    aria-label={t(($) => $.historicalImport.channelLabel)}
                    aria-invalid={validation?.field === "channel"}
                  >
                    <SelectValue placeholder={t(($) => $.historicalImport.channelPlaceholder)} />
                  </SelectTrigger>
                  <SelectContent>
                    {CHANNELS.map((value) => (
                      <SelectItem key={value} value={value}>
                        {channelLabel(t, value)}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </SettingsRow>
              <SettingsRow label={t(($) => $.historicalImport.publishedAtLabel)} size="text">
                <Input
                  type="datetime-local"
                  value={draft.publishedAt}
                  onChange={(event) => setDraft({ ...draft, publishedAt: event.target.value })}
                  aria-label={t(($) => $.historicalImport.publishedAtLabel)}
                  aria-invalid={validation?.field === "published_at"}
                  disabled={run.isPending || finished || !editableFields.has("publishedAt")}
                />
              </SettingsRow>
              <SettingsRow label={t(($) => $.historicalImport.platformAccountLabel)} size="text">
                <Input
                  value={draft.platformAccount}
                  onChange={(event) =>
                    setDraft({ ...draft, platformAccount: event.target.value })
                  }
                  placeholder={t(($) => $.historicalImport.platformAccountPlaceholder)}
                  aria-label={t(($) => $.historicalImport.platformAccountLabel)}
                  aria-invalid={validation?.field === "platform_account"}
                  disabled={run.isPending || finished || !editableFields.has("platformAccount")}
                />
              </SettingsRow>
              <SettingsRow
                label={t(($) => $.historicalImport.pageUrlLabel)}
                description={
                  failedField === "page_url_or_content_id"
                    ? t(($) => $.historicalImport.pageUrlRequired)
                    : t(($) => $.historicalImport.pageUrlHint)
                }
                size="text"
              >
                <Input
                  value={draft.pageUrlOrContentId}
                  onChange={(event) =>
                    setDraft({ ...draft, pageUrlOrContentId: event.target.value })
                  }
                  placeholder={t(($) => $.historicalImport.pageUrlPlaceholder)}
                  aria-label={t(($) => $.historicalImport.pageUrlLabel)}
                  aria-invalid={failedField === "page_url_or_content_id"}
                  disabled={run.isPending || finished || !editableFields.has("pageUrlOrContentId")}
                />
              </SettingsRow>
              <SettingsRow label={t(($) => $.historicalImport.submit)}>
                <div className="flex items-center gap-3">
                  <SettingsSaveState
                    status={
                      run.isPending
                        ? "saving"
                        : session.failure
                          ? "error"
                          : finished
                            ? "saved"
                            : "idle"
                    }
                    savingLabel={t(($) => $.historicalImport.running)}
                    savedLabel={t(($) => $.historicalImport.completed)}
                    errorLabel={t(($) => $.historicalImport.failed)}
                  />
                  {finished ? (
                    <Button
                      variant="outline"
                      onClick={() => {
                        setDraft(EMPTY_DRAFT);
                        setSession(newSession());
                      }}
                    >
                      {t(($) => $.historicalImport.importAnother)}
                    </Button>
                  ) : (
                    <Button disabled={run.isPending || validation !== null} onClick={submit}>
                      {session.failure
                        ? t(($) => $.historicalImport.retry)
                        : t(($) => $.historicalImport.submit)}
                    </Button>
                  )}
                </div>
              </SettingsRow>
            </SettingsCard>
          </SettingsSection>

          <ImportProgress session={session} />
          <WorkSections wsId={wsId} topicCardId="" allowCreate={false} />
        </SettingsTab>
      </SettingsContent>
    </>
  );
}

async function ensureHistoricalArtifact({
  draft,
  outputs,
  outputId,
  checkpoint,
}: HistoricalImportOperationContext): Promise<string> {
  let artifactId = outputId;
  if (!artifactId) {
    const artifact = parseArtifact(
      await api.createContentArtifact(outputs.workId, {
        kind: "body",
        title: deriveHistoricalImportTitle(draft.title, draft.body),
        position: 0,
      }),
    );
    artifactId = artifact.artifactId;
    checkpoint(artifactId);
  }

  parseArtifact(
    await api.patchContentArtifact(outputs.workId, artifactId, {
      draft_body: draft.body,
    }),
  );
  return artifactId;
}

function ImportProgress({
  session,
}: {
  session: HistoricalImportSession;
}) {
  const { t } = useT("common");
  const completed = session.steps.filter((step) => step.status === "completed").length;
  const partial = completed > 0 || Object.values(session.outputs).some(Boolean);

  return (
    <SettingsSection
      title={t(($) => $.historicalImport.progressTitle)}
      description={t(($) => $.historicalImport.progressDescription)}
    >
      <SettingsCard>
        {session.steps.map((step) => (
          <SettingsRow
            key={step.step}
            label={t(($) => $.historicalImport.steps[step.step])}
            description={stepStatusLabel(t, step.status)}
          >
            <span className="text-caption text-muted-foreground" />
          </SettingsRow>
        ))}
        {session.failure ? (
          <SettingsRow
            label={t(($) => $.historicalImport.failureTitle)}
            description={
              partial
                ? t(($) => $.historicalImport.partialFailureHint)
                : t(($) => $.historicalImport.failureHint)
            }
          >
            <span className="text-caption text-muted-foreground">
              {t(($) => $.historicalImport.failedAt, {
                step: t(($) => $.historicalImport.steps[session.failure!.step]),
              })}
            </span>
          </SettingsRow>
        ) : null}
      </SettingsCard>
    </SettingsSection>
  );
}

function stepStatusLabel(
  t: Translate,
  status: "not_started" | "completed" | "failed",
): string {
  if (status === "completed") return t(($) => $.historicalImport.status.completed);
  if (status === "failed") return t(($) => $.historicalImport.status.failed);
  return t(($) => $.historicalImport.status.notStarted);
}

function errorField(error: unknown): string {
  if (!(error instanceof ApiError) || !error.body || typeof error.body !== "object") return "";
  const field = (error.body as { field?: unknown }).field;
  return typeof field === "string" ? field : "";
}

function toRFC3339(value: string): string {
  const parsed = new Date(value);
  return Number.isNaN(parsed.getTime()) ? value : parsed.toISOString();
}

function channelLabel(t: Translate, channel: string): string {
  switch (channel) {
    case "xiaohongshu":
      return t(($) => $.historicalImport.channels.xiaohongshu);
    case "wechat_mp":
      return t(($) => $.historicalImport.channels.wechat_mp);
    case "douyin":
      return t(($) => $.historicalImport.channels.douyin);
    case "shipinhao":
      return t(($) => $.historicalImport.channels.shipinhao);
    default:
      return channel;
  }
}
