"use client";

import { useState, type ReactNode } from "react";
import { useT } from "@multica/views/i18n";
import {
  APPROVE_AND_SCHEDULE_STEPS,
  DELIVERY_STATUSES,
  HANDOFF_METHODS,
  PUBLICATION_STATUSES,
  REVIEW_AI_ENTRY_POINTS,
  REVIEW_CHANNELS,
  VERSION_MATCHES,
  advanceReady,
  canApproveAndSchedule,
  currentPublication,
  deliveryMoveOptions,
  isDecided,
  publicationSummary,
  recordReady,
  reviewMoves,
  submitBlocker,
  useAdvanceContentDelivery,
  useContentDeliveries,
  useContentPublications,
  useContentReview,
  useContentReviews,
  useCreateContentDelivery,
  useDecideContentReview,
  useRecordContentPublication,
  useSubmitContentReview,
  type DeliveryTask,
  type PublicationRecord,
} from "@multica/core/content/review-delivery";
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

// Review, delivery and publication records (specs/025, SOP 8-9.3), sitting on
// the document editor underneath the version history - which is where the thing
// being reviewed is.
//
// Composition is the account, topic and work pages': SettingsSection /
// SettingsCard / SettingsRow with one control per row, a save state beside a
// button. Nothing here is a new control and nothing sets a colour.
//
// Every rule lives in @multica/core/content/review-delivery/page-state.ts and
// states.ts, which have node tests. This file is wiring: which hook, which
// string, which row.
//
// Two things this page must keep saying out loud, because getting them wrong
// costs real work:
//
//   - A planned time is a to-do, not a schedule. Nothing runs at it.
//   - Handing a piece over is not publishing it. Only a record says that.

type Translate = ReturnType<typeof useT<"common">>["t"];

/** An account the piece can be delivered to. Passed in by the adapter: this
 *  module's declared dependencies do not include ip-profile, so the page is
 *  given the list rather than fetching it. */
export interface DeliveryAccountOption {
  id: string;
  name: string;
}

/** What a block rendered after this module's sections is given. The records
 *  come from the same query key PublicationSection reads, so the slot gets the
 *  list without a second request and without either component reaching into
 *  the other. */
export interface PublicationExtrasContext {
  wsId: string;
  artifactId: string;
  records: PublicationRecord[];
}

export interface ReviewDeliverySectionsProps {
  wsId: string;
  artifactId: string;
  artifactKind: string;
  /** The version a submission would freeze: the newest one. "" when the
   *  document has none yet. */
  latestVersionId: string;
  versionCount: number;
  accounts: DeliveryAccountOption[];
  /** Optional block rendered after this module's sections.
   *
   *  A slot rather than an import: the registry places feedback-learning
   *  DOWNSTREAM of this module, so this file cannot reach it. The web adapter
   *  composes the two, exactly the way work-editor's renderArtifactExtras lets
   *  this module sit under the version history. */
  renderPublicationExtras?: (context: PublicationExtrasContext) => ReactNode;
}

export function ReviewDeliverySections({
  wsId,
  artifactId,
  artifactKind,
  latestVersionId,
  versionCount,
  accounts,
  renderPublicationExtras,
}: ReviewDeliverySectionsProps) {
  return (
    <>
      <ReviewSection
        wsId={wsId}
        artifactId={artifactId}
        artifactKind={artifactKind}
        latestVersionId={latestVersionId}
        versionCount={versionCount}
        accounts={accounts}
      />
      <DeliverySection wsId={wsId} artifactId={artifactId} />
      <PublicationSection wsId={wsId} artifactId={artifactId} />
      <ReviewAIEntryPoints />
      <PublicationExtras
        wsId={wsId}
        artifactId={artifactId}
        render={renderPublicationExtras}
      />
    </>
  );
}

/**
 * Whatever hangs off the publication records, rendered after this module's own
 * sections.
 *
 * Same query key and same queryFn as PublicationSection's, deliberately:
 * TanStack Query serves both observers from one fetch, and that is what lets
 * the slot receive the records without either reaching into the other's state
 * or issuing a second request. Two observers with DIFFERENT fetchers would be
 * a race; two with the same one are not.
 */
function PublicationExtras({
  wsId,
  artifactId,
  render,
}: {
  wsId: string;
  artifactId: string;
  render?: (context: PublicationExtrasContext) => ReactNode;
}) {
  const records = useContentPublications(wsId, artifactId);
  if (!render) return null;
  return <>{render({ wsId, artifactId, records: records.data ?? [] })}</>;
}

// ---------------------------------------------------------------- review ---

function ReviewSection({
  wsId,
  artifactId,
  artifactKind,
  latestVersionId,
  versionCount,
  accounts,
}: ReviewDeliverySectionsProps) {
  const { t } = useT("common");
  const reviews = useContentReviews(wsId, artifactId);
  const submit = useSubmitContentReview(wsId);
  const [channel, setChannel] = useState<string>(REVIEW_CHANNELS[0]);
  const [accountId, setAccountId] = useState(accounts[0]?.id ?? "");
  const [openReviewId, setOpenReviewId] = useState("");

  const list = reviews.data ?? [];
  const blocker = submitBlocker(artifactKind, versionCount, accountId);
  const accountItems = accounts.map((account) => ({
    value: account.id,
    label: account.name || account.id,
  }));
  const channelItems = REVIEW_CHANNELS.map((value) => ({
    value,
    label: channelLabel(t, value),
  }));

  return (
    <>
      <SettingsSection
        title={t(($) => $.contentReviews.sectionTitle)}
        description={t(($) => $.contentReviews.sectionDescription)}
      >
        <SettingsCard>
          {/* Only a channel cut can be submitted, and the reason is written
              here rather than left as a dead button. SOP 8's object of review
              is a specific channel's version. */}
          <SettingsRow label={t(($) => $.contentReviews.channelLabel)} size="select-wide">
            <Select
              items={channelItems}
              value={channel}
              onValueChange={(value) => setChannel(value ?? REVIEW_CHANNELS[0])}
            >
              <SelectTrigger aria-label={t(($) => $.contentReviews.channelLabel)}>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {channelItems.map((item) => (
                  <SelectItem key={item.value} value={item.value}>
                    {item.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </SettingsRow>
          <SettingsRow label={t(($) => $.contentReviews.accountLabel)} size="select-wide">
            <Select
              items={accountItems}
              value={accountId}
              onValueChange={(value) => setAccountId(value ?? "")}
            >
              <SelectTrigger aria-label={t(($) => $.contentReviews.accountLabel)}>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {accountItems.map((item) => (
                  <SelectItem key={item.value} value={item.value}>
                    {item.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </SettingsRow>
          <SettingsRow
            label={t(($) => $.contentReviews.submit)}
            description={blocker ? t(($) => $.contentReviews.blocked[blocker]) : undefined}
          >
            <div className="flex items-center gap-3">
              <ActionState pending={submit.isPending} failed={submit.isError} />
              <Button
                disabled={submit.isPending || blocker !== null}
                onClick={() =>
                  submit.mutate({
                    artifactId,
                    versionId: latestVersionId,
                    accountId,
                    channel,
                  })
                }
              >
                {t(($) => $.contentReviews.submit)}
              </Button>
            </div>
          </SettingsRow>
        </SettingsCard>
      </SettingsSection>

      <SettingsSection
        title={t(($) => $.contentReviews.historyTitle)}
        description={t(($) => $.contentReviews.historyHint)}
      >
        <SettingsCard>
          {list.length === 0 ? (
            <SettingsRow label={t(($) => $.contentReviews.historyEmpty)}>
              <span className="text-caption text-muted-foreground" />
            </SettingsRow>
          ) : (
            list.map((request) => (
              <SettingsRow
                key={request.reviewRequestId}
                label={
                  <button
                    type="button"
                    onClick={() =>
                      setOpenReviewId(
                        request.reviewRequestId === openReviewId ? "" : request.reviewRequestId,
                      )
                    }
                    data-active={request.reviewRequestId === openReviewId}
                    // The selected row stays legible under hover because the
                    // weight carries the state and hover only moves colour.
                    className="text-left text-body data-[active=true]:font-semibold hover:text-foreground"
                  >
                    {reviewStatusLabel(t, request.status)}
                  </button>
                }
                description={
                  <>
                    {channelLabel(t, request.channel)}
                    {" · "}
                    {t(($) => $.contentReviews.boundVersion)}
                    {": "}
                    {request.versionId}
                  </>
                }
              >
                <span className="text-caption text-muted-foreground">
                  {request.requestedAt}
                </span>
              </SettingsRow>
            ))
          )}
        </SettingsCard>
      </SettingsSection>

      {openReviewId ? (
        <ReviewDetail key={openReviewId} wsId={wsId} reviewId={openReviewId} artifactId={artifactId} />
      ) : null}
    </>
  );
}

function ReviewDetail({
  wsId,
  reviewId,
  artifactId,
}: {
  wsId: string;
  reviewId: string;
  artifactId: string;
}) {
  const { t } = useT("common");
  const detail = useContentReview(wsId, reviewId);
  const decide = useDecideContentReview(wsId);
  const createTask = useCreateContentDelivery(wsId);
  const [note, setNote] = useState("");

  const request = detail.data?.review;
  const transitions = detail.data?.transitions ?? [];
  if (!request || !request.reviewRequestId) return null;

  const decided = isDecided(request.status);
  const moves = reviewMoves(request.status);
  const snapshot = request.snapshot;

  return (
    <>
      {/* The frozen snapshot, read-only. It is what was submitted, not what
          the document says now: SOP 8 keeps an approval with the combination it
          was granted for. */}
      <SettingsSection
        title={t(($) => $.contentReviews.snapshotTitle)}
        description={t(($) => $.contentReviews.snapshotHint)}
      >
        <SettingsCard>
          <SettingsRow label={t(($) => $.contentReviews.snapshot.channel)}>
            <span className="text-caption text-muted-foreground">
              {channelLabel(t, snapshot.channel)}
            </span>
          </SettingsRow>
          <SettingsRow label={t(($) => $.contentReviews.snapshot.versionId)}>
            <span className="text-caption text-muted-foreground">{snapshot.versionId}</span>
          </SettingsRow>
          <SettingsRow label={t(($) => $.contentReviews.snapshot.accountId)}>
            <span className="text-caption text-muted-foreground">{snapshot.accountId}</span>
          </SettingsRow>
          <SettingsRow
            label={t(($) => $.contentReviews.snapshot.startSnapshotId)}
            description={
              snapshot.startSnapshotId ? undefined : t(($) => $.contentReviews.snapshot.noStart)
            }
          >
            <span className="text-caption text-muted-foreground">
              {snapshot.startSnapshotId}
            </span>
          </SettingsRow>
          {/* Empty until W-03. Stated rather than hidden, so the absence reads
              as "not yet" instead of "there were none". */}
          <SettingsRow
            label={t(($) => $.contentReviews.snapshot.attachments)}
            description={t(($) => $.contentReviews.snapshot.attachmentsPending)}
          >
            <span className="text-caption text-muted-foreground">
              {snapshot.attachments.length}
            </span>
          </SettingsRow>
        </SettingsCard>
      </SettingsSection>

      <SettingsSection
        title={t(($) => $.contentReviews.decisionTitle)}
        description={
          decided
            ? t(($) => $.contentReviews.decisionClosed)
            : t(($) => $.contentReviews.decisionHint)
        }
      >
        <SettingsCard>
          {decided ? (
            <SettingsRow
              label={reviewStatusLabel(t, request.status)}
              description={request.decisionNote || undefined}
            >
              <span className="text-caption text-muted-foreground">{request.decidedBy}</span>
            </SettingsRow>
          ) : (
            <>
              <SettingsRow
                label={t(($) => $.contentReviews.noteLabel)}
                size="text"
                align="start"
              >
                <Textarea
                  value={note}
                  onChange={(event) => setNote(event.target.value)}
                  placeholder={t(($) => $.contentReviews.notePlaceholder)}
                  aria-label={t(($) => $.contentReviews.noteLabel)}
                  rows={4}
                />
              </SettingsRow>
              {moves.map((move) => (
                <SettingsRow key={move.status} label={reviewStatusLabel(t, move.status)}>
                  <div className="flex items-center gap-3">
                    <Button
                      variant="outline"
                      disabled={decide.isPending || !move.allowed}
                      onClick={() =>
                        decide.mutate(
                          { reviewId, status: move.status, note },
                          { onSuccess: () => setNote("") },
                        )
                      }
                    >
                      {reviewStatusLabel(t, move.status)}
                    </Button>
                  </div>
                </SettingsRow>
              ))}
              {/* SOP 8's personal mode: one click, two records. The two calls
                  run in order and the second only after the first landed - the
                  backend never sees them as one action. */}
              <SettingsRow
                label={t(($) => $.contentReviews.approveAndSchedule)}
                description={t(($) => $.contentReviews.approveAndScheduleHint, {
                  steps: APPROVE_AND_SCHEDULE_STEPS.length,
                })}
              >
                <div className="flex items-center gap-3">
                  <ActionState
                    pending={decide.isPending || createTask.isPending}
                    failed={decide.isError || createTask.isError}
                  />
                  <Button
                    disabled={
                      decide.isPending ||
                      createTask.isPending ||
                      !canApproveAndSchedule(request.status)
                    }
                    onClick={() =>
                      decide.mutate(
                        { reviewId, status: "approved", note },
                        {
                          onSuccess: () => {
                            setNote("");
                            createTask.mutate({
                              artifactId,
                              channel: request.channel,
                              reviewRequestId: reviewId,
                            });
                          },
                        },
                      )
                    }
                  >
                    {t(($) => $.contentReviews.approveAndSchedule)}
                  </Button>
                </div>
              </SettingsRow>
            </>
          )}
        </SettingsCard>
      </SettingsSection>

      <SettingsSection title={t(($) => $.contentReviews.transitionsTitle)}>
        <SettingsCard>
          {transitions.length === 0 ? (
            <SettingsRow label={t(($) => $.contentReviews.transitionsEmpty)}>
              <span className="text-caption text-muted-foreground" />
            </SettingsRow>
          ) : (
            transitions.map((transition) => (
              <SettingsRow
                key={transition.transitionId}
                label={
                  transition.fromStatus
                    ? `${statusLabel(t, transition.fromStatus)} → ${statusLabel(t, transition.toStatus)}`
                    : statusLabel(t, transition.toStatus)
                }
                description={transition.reason || undefined}
              >
                <span className="text-caption text-muted-foreground">
                  {transition.actorId}
                </span>
              </SettingsRow>
            ))
          )}
        </SettingsCard>
      </SettingsSection>
    </>
  );
}

// -------------------------------------------------------------- delivery ---

function DeliverySection({ wsId, artifactId }: { wsId: string; artifactId: string }) {
  const { t } = useT("common");
  const tasks = useContentDeliveries(wsId, artifactId);
  const [openTaskId, setOpenTaskId] = useState("");

  const list = tasks.data ?? [];
  const open = list.find((task) => task.deliveryTaskId === openTaskId) ?? null;

  return (
    <>
      <SettingsSection
        title={t(($) => $.contentDeliveries.sectionTitle)}
        description={t(($) => $.contentDeliveries.sectionDescription)}
      >
        <SettingsCard>
          {list.length === 0 ? (
            <SettingsRow label={t(($) => $.contentDeliveries.empty)}>
              <span className="text-caption text-muted-foreground" />
            </SettingsRow>
          ) : (
            list.map((task) => (
              <SettingsRow
                key={task.deliveryTaskId}
                label={
                  <button
                    type="button"
                    onClick={() =>
                      setOpenTaskId(task.deliveryTaskId === openTaskId ? "" : task.deliveryTaskId)
                    }
                    data-active={task.deliveryTaskId === openTaskId}
                    className="text-left text-body data-[active=true]:font-semibold hover:text-foreground"
                  >
                    {deliveryStatusLabel(t, task.status)}
                  </button>
                }
                description={<TaskFlags task={task} />}
              >
                <span className="text-caption text-muted-foreground">
                  {channelLabel(t, task.channel)}
                </span>
              </SettingsRow>
            ))
          )}
        </SettingsCard>
      </SettingsSection>

      {open ? <DeliveryDetail key={open.deliveryTaskId} wsId={wsId} task={open} /> : null}
    </>
  );
}

/** The two derived flags, spelled out. Both come from the server; neither is a
 *  stored column, and neither asks a platform anything. */
function TaskFlags({ task }: { task: DeliveryTask }) {
  const { t } = useT("common");
  const parts: string[] = [];
  if (task.scheduledAt) {
    parts.push(`${t(($) => $.contentDeliveries.scheduledAtLabel)}: ${task.scheduledAt}`);
  }
  if (task.due) parts.push(t(($) => $.contentDeliveries.due));
  if (task.pendingRegistration) parts.push(t(($) => $.contentDeliveries.pendingRegistration));
  return <>{parts.join(" · ")}</>;
}

function DeliveryDetail({ wsId, task }: { wsId: string; task: DeliveryTask }) {
  const { t } = useT("common");
  const advance = useAdvanceContentDelivery(wsId);
  const [to, setTo] = useState("");
  const [scheduledAt, setScheduledAt] = useState("");
  const [handoffMethod, setHandoffMethod] = useState("");
  const [reason, setReason] = useState("");

  const options = deliveryMoveOptions(task.status, DELIVERY_STATUSES);
  const chosen = options.find((option) => option.status === to) ?? null;
  const statusItems = options.map((option) => ({
    value: option.status,
    label: deliveryStatusLabel(t, option.status),
  }));
  const handoffItems = HANDOFF_METHODS.map((value) => ({
    value,
    label: handoffLabel(t, value),
  }));
  const ready = advanceReady(to, { scheduledAt, handoffMethod, reason });

  return (
    <SettingsSection
      title={t(($) => $.contentDeliveries.detailTitle)}
      // The sentence that has to be here: someone who sets a time assumes the
      // system will act on it. Nothing does, and a displayed time that is never
      // acted on is the one thing on this page that can cost a real publication.
      description={t(($) => $.contentDeliveries.scheduledAtExplainer)}
    >
      <SettingsCard>
        <SettingsRow
          label={t(($) => $.contentDeliveries.currentStatus)}
          description={<TaskFlags task={task} />}
        >
          <span className="text-caption text-muted-foreground">
            {deliveryStatusLabel(t, task.status)}
          </span>
        </SettingsRow>

        {task.status === "held" ? (
          // SOP 9.3: the approved snapshot no longer describes what is about to
          // go out. Two ways forward, and "keep delivering the approved one" is
          // a deliberate action with a recorded reason, never a default.
          <SettingsRow
            label={t(($) => $.contentDeliveries.heldTitle)}
            description={t(($) => $.contentDeliveries.heldHint, {
              reviewId: task.reviewRequestId,
            })}
          >
            <div className="flex items-center gap-3">
              <ActionState pending={advance.isPending} failed={advance.isError} />
              <Button
                variant="outline"
                disabled={advance.isPending}
                onClick={() =>
                  advance.mutate({
                    deliveryId: task.deliveryTaskId,
                    keepApprovedSnapshot: true,
                    reason,
                  })
                }
              >
                {t(($) => $.contentDeliveries.keepApproved)}
              </Button>
            </div>
          </SettingsRow>
        ) : null}

        <SettingsRow label={t(($) => $.contentDeliveries.moveTo)} size="select-wide">
          <Select items={statusItems} value={to} onValueChange={(value) => setTo(value ?? "")}>
            <SelectTrigger aria-label={t(($) => $.contentDeliveries.moveTo)}>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {options.map((option) => (
                // Illegal moves stay visible and disabled. Hiding them would
                // say the product does not have the move at all.
                <SelectItem key={option.status} value={option.status} disabled={!option.allowed}>
                  {deliveryStatusLabel(t, option.status)}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </SettingsRow>

        {chosen?.requires === "scheduled_at" ? (
          <SettingsRow
            label={t(($) => $.contentDeliveries.scheduledAtLabel)}
            description={t(($) => $.contentDeliveries.scheduledAtExplainer)}
            size="text"
          >
            <Input
              value={scheduledAt}
              onChange={(event) => setScheduledAt(event.target.value)}
              placeholder={t(($) => $.contentDeliveries.scheduledAtPlaceholder)}
              aria-label={t(($) => $.contentDeliveries.scheduledAtLabel)}
            />
          </SettingsRow>
        ) : null}

        {chosen?.requires === "handoff_method" ? (
          <SettingsRow
            label={t(($) => $.contentDeliveries.handoffLabel)}
            // SOP 9.1: none of the three means published. Saying it here is
            // the difference between "I handed it over" and "it went out".
            description={t(($) => $.contentDeliveries.handoffNotPublished)}
            size="select-wide"
          >
            <Select
              items={handoffItems}
              value={handoffMethod}
              onValueChange={(value) => setHandoffMethod(value ?? "")}
            >
              <SelectTrigger aria-label={t(($) => $.contentDeliveries.handoffLabel)}>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {handoffItems.map((item) => (
                  <SelectItem key={item.value} value={item.value}>
                    {item.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </SettingsRow>
        ) : null}

        {chosen?.requires === "reason" ? (
          <SettingsRow
            label={t(($) => $.contentDeliveries.reasonLabel)}
            description={t(($) => $.contentDeliveries.reasonHint)}
            size="text"
            align="start"
          >
            <Textarea
              value={reason}
              onChange={(event) => setReason(event.target.value)}
              placeholder={t(($) => $.contentDeliveries.reasonPlaceholder)}
              aria-label={t(($) => $.contentDeliveries.reasonLabel)}
              rows={3}
            />
          </SettingsRow>
        ) : null}

        <SettingsRow label={t(($) => $.contentDeliveries.apply)}>
          <div className="flex items-center gap-3">
            <ActionState pending={advance.isPending} failed={advance.isError} />
            <Button
              disabled={advance.isPending || !chosen?.allowed || !ready}
              onClick={() =>
                advance.mutate(
                  {
                    deliveryId: task.deliveryTaskId,
                    status: to,
                    scheduledAt,
                    handoffMethod,
                    reason,
                  },
                  {
                    onSuccess: () => {
                      setTo("");
                      setScheduledAt("");
                      setHandoffMethod("");
                      setReason("");
                    },
                  },
                )
              }
            >
              {t(($) => $.contentDeliveries.apply)}
            </Button>
          </div>
        </SettingsRow>
      </SettingsCard>
    </SettingsSection>
  );
}

// ----------------------------------------------------------- publication ---

function PublicationSection({ wsId, artifactId }: { wsId: string; artifactId: string }) {
  const { t } = useT("common");
  const records = useContentPublications(wsId, artifactId);
  const tasks = useContentDeliveries(wsId, artifactId);
  const record = useRecordContentPublication(wsId);

  const [status, setStatus] = useState("");
  const [pageUrlOrContentId, setPageUrl] = useState("");
  const [verificationNote, setVerificationNote] = useState("");
  const [receiptNote, setReceiptNote] = useState("");
  const [declaredBy, setDeclaredBy] = useState("");
  const [publishedAt, setPublishedAt] = useState("");
  const [platformAccount, setPlatformAccount] = useState("");
  const [editNote, setEditNote] = useState("");
  const [versionMatch, setVersionMatch] = useState<string>("unknown");

  const list = records.data ?? [];
  const summary = publicationSummary(list, tasks.data ?? []);
  const current = currentPublication(list);
  const ready = recordReady(status, { pageUrlOrContentId, verificationNote, receiptNote });
  const statusItems = PUBLICATION_STATUSES.map((value) => ({
    value,
    label: publicationStatusLabel(t, value),
  }));
  const matchItems = VERSION_MATCHES.map((value) => ({
    value,
    label: versionMatchLabel(t, value),
  }));
  // The channel of the piece: taken from a task if there is one, otherwise the
  // first channel. A record always belongs to a channel.
  const channel = tasks.data?.[0]?.channel ?? REVIEW_CHANNELS[0];

  return (
    <>
      <SettingsSection
        title={t(($) => $.contentPublications.sectionTitle)}
        // SOP 9.2: the workbench does not guess what the platform did.
        description={t(($) => $.contentPublications.sectionDescription)}
      >
        <SettingsCard>
          <SettingsRow
            label={t(($) => $.contentPublications.currentLabel)}
            description={
              summary.pendingRegistration
                ? t(($) => $.contentPublications.pendingRegistrationHint)
                : undefined
            }
          >
            <span className="text-caption text-muted-foreground">
              {summary.pendingRegistration
                ? t(($) => $.contentPublications.pendingRegistration)
                : current
                  ? publicationStatusLabel(t, current.status)
                  : t(($) => $.contentPublications.nothingRecorded)}
            </span>
          </SettingsRow>
        </SettingsCard>
      </SettingsSection>

      <SettingsSection
        title={t(($) => $.contentPublications.formTitle)}
        description={t(($) => $.contentPublications.formHint)}
      >
        <SettingsCard>
          <SettingsRow label={t(($) => $.contentPublications.statusLabel)} size="select-wide">
            <Select
              items={statusItems}
              value={status}
              onValueChange={(value) => setStatus(value ?? "")}
            >
              <SelectTrigger aria-label={t(($) => $.contentPublications.statusLabel)}>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {statusItems.map((item) => (
                  <SelectItem key={item.value} value={item.value}>
                    {item.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </SettingsRow>

          {/* Which fields are required depends on the status, so the required
              ones are marked here rather than surfaced as a 400 afterwards. */}
          <SettingsRow
            label={t(($) => $.contentPublications.pageUrlLabel)}
            description={
              status === "reported_published" || status === "verified_published"
                ? t(($) => $.contentPublications.required)
                : undefined
            }
            size="text"
          >
            <Input
              value={pageUrlOrContentId}
              onChange={(event) => setPageUrl(event.target.value)}
              placeholder={t(($) => $.contentPublications.pageUrlPlaceholder)}
              aria-label={t(($) => $.contentPublications.pageUrlLabel)}
            />
          </SettingsRow>
          <SettingsRow
            label={t(($) => $.contentPublications.verificationLabel)}
            description={
              status === "verified_published"
                ? t(($) => $.contentPublications.required)
                : t(($) => $.contentPublications.verificationHint)
            }
            size="text"
            align="start"
          >
            <Textarea
              value={verificationNote}
              onChange={(event) => setVerificationNote(event.target.value)}
              placeholder={t(($) => $.contentPublications.verificationPlaceholder)}
              aria-label={t(($) => $.contentPublications.verificationLabel)}
              rows={3}
            />
          </SettingsRow>
          <SettingsRow
            label={t(($) => $.contentPublications.receiptLabel)}
            description={
              status === "failed" || status === "removed"
                ? t(($) => $.contentPublications.required)
                : t(($) => $.contentPublications.receiptHint)
            }
            size="text"
            align="start"
          >
            <Textarea
              value={receiptNote}
              onChange={(event) => setReceiptNote(event.target.value)}
              placeholder={t(($) => $.contentPublications.receiptPlaceholder)}
              aria-label={t(($) => $.contentPublications.receiptLabel)}
              rows={3}
            />
          </SettingsRow>
          <SettingsRow
            label={t(($) => $.contentPublications.declaredByLabel)}
            description={t(($) => $.contentPublications.declaredByHint)}
            size="text"
          >
            <Input
              value={declaredBy}
              onChange={(event) => setDeclaredBy(event.target.value)}
              placeholder={t(($) => $.contentPublications.declaredByPlaceholder)}
              aria-label={t(($) => $.contentPublications.declaredByLabel)}
            />
          </SettingsRow>
          <SettingsRow label={t(($) => $.contentPublications.publishedAtLabel)} size="text">
            <Input
              value={publishedAt}
              onChange={(event) => setPublishedAt(event.target.value)}
              placeholder={t(($) => $.contentPublications.publishedAtPlaceholder)}
              aria-label={t(($) => $.contentPublications.publishedAtLabel)}
            />
          </SettingsRow>
          <SettingsRow label={t(($) => $.contentPublications.platformAccountLabel)} size="text">
            <Input
              value={platformAccount}
              onChange={(event) => setPlatformAccount(event.target.value)}
              placeholder={t(($) => $.contentPublications.platformAccountPlaceholder)}
              aria-label={t(($) => $.contentPublications.platformAccountLabel)}
            />
          </SettingsRow>
          <SettingsRow
            label={t(($) => $.contentPublications.editNoteLabel)}
            description={t(($) => $.contentPublications.editNoteHint)}
            size="text"
            align="start"
          >
            <Textarea
              value={editNote}
              onChange={(event) => setEditNote(event.target.value)}
              placeholder={t(($) => $.contentPublications.editNotePlaceholder)}
              aria-label={t(($) => $.contentPublications.editNoteLabel)}
              rows={3}
            />
          </SettingsRow>
          <SettingsRow
            label={t(($) => $.contentPublications.versionMatchLabel)}
            // unknown is SOP 9.1's own value for "I could not get the full
            // text". Neutral wording on purpose: it is not a failure.
            description={t(($) => $.contentPublications.versionMatchHint)}
            size="select-wide"
          >
            <Select
              items={matchItems}
              value={versionMatch}
              onValueChange={(value) => setVersionMatch(value ?? "unknown")}
            >
              <SelectTrigger aria-label={t(($) => $.contentPublications.versionMatchLabel)}>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {matchItems.map((item) => (
                  <SelectItem key={item.value} value={item.value}>
                    {item.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </SettingsRow>
          <SettingsRow label={t(($) => $.contentPublications.record)}>
            <div className="flex items-center gap-3">
              <ActionState pending={record.isPending} failed={record.isError} />
              <Button
                disabled={record.isPending || !ready}
                onClick={() =>
                  record.mutate(
                    {
                      artifactId,
                      deliveryTaskId: tasks.data?.[0]?.deliveryTaskId ?? "",
                      channel,
                      status,
                      declaredBy,
                      pageUrlOrContentId,
                      receiptNote,
                      verificationNote,
                      publishedAt,
                      platformAccount,
                      platformEdited: editNote.trim() !== "",
                      editNote,
                      versionMatch,
                    },
                    {
                      onSuccess: () => {
                        setStatus("");
                        setPageUrl("");
                        setVerificationNote("");
                        setReceiptNote("");
                        setDeclaredBy("");
                        setPublishedAt("");
                        setPlatformAccount("");
                        setEditNote("");
                        setVersionMatch("unknown");
                      },
                    },
                  )
                }
              >
                {t(($) => $.contentPublications.record)}
              </Button>
            </div>
          </SettingsRow>
        </SettingsCard>
      </SettingsSection>

      <SettingsSection
        title={t(($) => $.contentPublications.historyTitle)}
        // The current status IS the newest row. There is no stored field to
        // disagree with the stream.
        description={t(($) => $.contentPublications.historyHint)}
      >
        <SettingsCard>
          {list.length === 0 ? (
            <SettingsRow label={t(($) => $.contentPublications.historyEmpty)}>
              <span className="text-caption text-muted-foreground" />
            </SettingsRow>
          ) : (
            list.map((entry) => <PublicationRow key={entry.publicationRecordId} record={entry} />)
          )}
        </SettingsCard>
      </SettingsSection>

    </>
  );
}

function PublicationRow({ record }: { record: PublicationRecord }) {
  const { t } = useT("common");
  const parts: string[] = [versionMatchLabel(t, record.versionMatch)];
  if (record.declaredBy) {
    parts.push(`${t(($) => $.contentPublications.declaredByLabel)}: ${record.declaredBy}`);
  }
  if (record.pageUrlOrContentId) parts.push(record.pageUrlOrContentId);
  if (record.platformEdited) parts.push(t(($) => $.contentPublications.platformEdited));
  return (
    <SettingsRow
      label={publicationStatusLabel(t, record.status)}
      description={parts.join(" · ")}
    >
      <span className="text-caption text-muted-foreground">{record.createdAt}</span>
    </SettingsRow>
  );
}

// ------------------------------------------------------------------- ai ----

// Present and disabled, with the reason written next to them: hiding them would
// say the product does not intend to do this, a blank would read as unfinished,
// and a spinner as "any moment now". Nothing here fabricates a verification.
function ReviewAIEntryPoints() {
  const { t } = useT("common");
  return (
    <SettingsSection title={t(($) => $.contentReviews.aiTitle)}>
      <SettingsCard>
        {REVIEW_AI_ENTRY_POINTS.map((entry) => (
          <SettingsRow
            key={entry.id}
            label={t(($) => $.contentReviews.ai[entry.id])}
            description={t(($) => $.contentReviews.aiUnavailable)}
          >
            <Button variant="outline" disabled>
              {t(($) => $.contentReviews.ai[entry.id])}
            </Button>
          </SettingsRow>
        ))}
      </SettingsCard>
    </SettingsSection>
  );
}

function ActionState({ pending, failed }: { pending: boolean; failed: boolean }) {
  const { t } = useT("common");
  return (
    <SettingsSaveState
      status={pending ? "saving" : failed ? "error" : "idle"}
      savingLabel={t(($) => $.contentReviews.saving)}
      savedLabel={t(($) => $.contentReviews.saved)}
      errorLabel={t(($) => $.contentReviews.failed)}
    />
  );
}

// A value this build has not heard of still has to render as something. The raw
// value is a worse label than a translated one and a better one than a blank.

function channelLabel(t: Translate, channel: string): string {
  switch (channel) {
    case "xiaohongshu":
      return t(($) => $.contentReviews.channels.xiaohongshu);
    case "wechat_mp":
      return t(($) => $.contentReviews.channels.wechat_mp);
    case "douyin":
      return t(($) => $.contentReviews.channels.douyin);
    case "shipinhao":
      return t(($) => $.contentReviews.channels.shipinhao);
    default:
      return channel;
  }
}

function reviewStatusLabel(t: Translate, status: string): string {
  switch (status) {
    case "pending":
      return t(($) => $.contentReviews.statuses.pending);
    case "changes_requested":
      return t(($) => $.contentReviews.statuses.changes_requested);
    case "approved":
      return t(($) => $.contentReviews.statuses.approved);
    case "rejected":
      return t(($) => $.contentReviews.statuses.rejected);
    case "cancelled":
      return t(($) => $.contentReviews.statuses.cancelled);
    default:
      return status;
  }
}

function deliveryStatusLabel(t: Translate, status: string): string {
  switch (status) {
    case "draft":
      return t(($) => $.contentDeliveries.statuses.draft);
    case "ready":
      return t(($) => $.contentDeliveries.statuses.ready);
    case "scheduled":
      return t(($) => $.contentDeliveries.statuses.scheduled);
    case "handed_off":
      return t(($) => $.contentDeliveries.statuses.handed_off);
    case "cancelled":
      return t(($) => $.contentDeliveries.statuses.cancelled);
    case "held":
      return t(($) => $.contentDeliveries.statuses.held);
    default:
      return status;
  }
}

/** A transition row can be about either subject, so it tries both label sets
 *  before falling back to the raw value. */
function statusLabel(t: Translate, status: string): string {
  const review = reviewStatusLabel(t, status);
  if (review !== status) return review;
  return deliveryStatusLabel(t, status);
}

function handoffLabel(t: Translate, method: string): string {
  switch (method) {
    case "export":
      return t(($) => $.contentDeliveries.handoffMethods.export);
    case "copy":
      return t(($) => $.contentDeliveries.handoffMethods.copy);
    case "handed_to_operator":
      return t(($) => $.contentDeliveries.handoffMethods.handed_to_operator);
    default:
      return method;
  }
}

function publicationStatusLabel(t: Translate, status: string): string {
  switch (status) {
    case "reported_published":
      return t(($) => $.contentPublications.statuses.reported_published);
    case "verified_published":
      return t(($) => $.contentPublications.statuses.verified_published);
    case "failed":
      return t(($) => $.contentPublications.statuses.failed);
    case "removed":
      return t(($) => $.contentPublications.statuses.removed);
    case "unknown":
      return t(($) => $.contentPublications.statuses.unknown);
    default:
      return status;
  }
}

function versionMatchLabel(t: Translate, match: string): string {
  switch (match) {
    case "matched":
      return t(($) => $.contentPublications.versionMatches.matched);
    case "differs":
      return t(($) => $.contentPublications.versionMatches.differs);
    case "unknown":
      return t(($) => $.contentPublications.versionMatches.unknown);
    default:
      return match;
  }
}
