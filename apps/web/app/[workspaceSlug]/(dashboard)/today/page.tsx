"use client";

// The Today dashboard (SOP §2). This is an adapter: it reads four content
// modules' existing endpoints and renders what packages/core/today derives
// from them. It holds no judgment of its own - every "what belongs here" rule
// lives in core so a second surface reuses the rule and not this markup.
//
// It lives in apps/web rather than packages/views/content/ because `today` is
// not a registered content module, and no registered module depends on all
// four sources (topic-planning, work-editor, review-delivery, ip-profile).
// Registering one would mean editing the dependency table, which the spec
// forbids. The adapters list is what makes this file allowed to import them.

import { useQueries } from "@tanstack/react-query";
import { api } from "@multica/core/api";
import { useWorkspaceId } from "@multica/core/hooks";
import {
  accountKeys,
  parseProfileRead,
  useContentAccounts,
} from "@multica/core/content/ip-profile";
import { useContentTopics } from "@multica/core/content/topic-planning";
import {
  parseArtifacts,
  useContentWorks,
  workEditorKeys,
} from "@multica/core/content/work-editor";
import {
  useContentDeliveries,
  useContentReviews,
} from "@multica/core/content/review-delivery";
import {
  accountsMissingConfig,
  deliveriesNeedingAction,
  reviewsNeedingAttention,
  worksInProgress,
  worthWritingTopics,
} from "@multica/core/today";
import type {
  AccountGapEntry,
  DeliveryEntry,
  EstimatedEffort,
  ProfileLike,
  ReviewEntry,
  Section,
  TopicEntry,
  WorkEntry,
} from "@multica/core/today";
import { Button } from "@multica/ui/components/ui/button";
import { useT } from "@multica/views/i18n";
import { PageHeader } from "@multica/views/layout/page-header";
import {
  SettingsCard,
  SettingsContent,
  SettingsRow,
  SettingsSection,
  SettingsTab,
} from "@multica/views/settings/layout";
import { useNavigation } from "@multica/views/navigation";
import type { ReactNode } from "react";

type Translate = ReturnType<typeof useT<"common">>["t"];

export default function Page() {
  const wsId = useWorkspaceId();
  return <TodayPage key={wsId} wsId={wsId} />;
}

function TodayPage({ wsId }: { wsId: string }) {
  const { t } = useT("common");
  return (
    <>
      <PageHeader className="xl:hidden">
        <span className="text-body font-medium">{t(($) => $.contentToday.title)}</span>
      </PageHeader>
      <SettingsContent>
        <SettingsTab
          title={t(($) => $.contentToday.title)}
          description={t(($) => $.contentToday.description)}
        >
          {/* SOP §2's five, in the sentence's own order, then the sixth. */}
          <TopicsSection wsId={wsId} />
          <WorksSection wsId={wsId} />
          <ReviewsSection wsId={wsId} />
          <DeliveriesSection wsId={wsId} />
          <FeedbackSection />
          <AccountGapsSection wsId={wsId} />
        </SettingsTab>
      </SettingsContent>
    </>
  );
}

/**
 * Per-account expression profiles.
 *
 * Same query key and same queryFn as `useAccountProfile`, deliberately: two
 * observers of one key with different fetchers is a race, and sections 1 and 6
 * are supposed to share a single read rather than each fetching its own.
 *
 * A failed read is reported as failed rather than as an empty profile (#167),
 * which is what makes the per-row failure marks below reachable at all.
 */
function useAccountProfiles(wsId: string, accountIds: string[]) {
  const results = useQueries({
    queries: accountIds.map((accountId) => ({
      queryKey: accountKeys.profile(wsId, accountId),
      queryFn: async () => parseProfileRead(await api.getContentAccountProfile(accountId)),
    })),
  });
  const profiles = new Map<string, ProfileLike>();
  const failed = new Set<string>();
  accountIds.forEach((accountId, index) => {
    const result = results[index];
    if (result?.isError) failed.add(accountId);
    const data = result?.data;
    if (!data) return;
    profiles.set(accountId, {
      readiness: data.readiness,
      weekly_hours: data.profile.weekly_hours,
    });
  });
  return { profiles, failed, pending: results.some((result) => result.isPending) };
}

// ---------------------------------------------------------------- section 1

function TopicsSection({ wsId }: { wsId: string }) {
  const { t } = useT("common");
  const navigation = useNavigation();
  const topics = useContentTopics(wsId);
  const cards = topics.data ?? [];
  const accountIds = uniq(
    cards.filter((card) => card.status === "draft").map((card) => card.accountId ?? ""),
  ).filter(Boolean);
  const { profiles, failed, pending } = useAccountProfiles(wsId, accountIds);
  const section = worthWritingTopics(cards, profiles, failed);

  return (
    <SectionShell
      title={t(($) => $.contentToday.topics.title)}
      // EP-04c is not here, so nothing on this page recommends anything. Saying
      // so is the only honest thing to put where the recommendations would be.
      description={t(($) => $.contentToday.topics.unavailableHint)}
      state={queryState(topics.isPending || pending, topics.isError)}
      section={section}
      t={t}
      onSeeAll={() => navigation.push(`/${wsId}/topics`)}
      renderRow={(entry: TopicEntry) => (
        <SettingsRow
          key={entry.topicCardId}
          label={entry.reason || t(($) => $.contentToday.topics.noReason)}
          description={
            entry.failed ? t(($) => $.contentToday.rowFailed) : effortLabel(t, entry.effort)
          }
        >
          <Button variant="outline" onClick={() => navigation.push(`/${wsId}/topics`)}>
            {t(($) => $.contentToday.open)}
          </Button>
        </SettingsRow>
      )}
    />
  );
}

function effortLabel(t: Translate, effort: EstimatedEffort): string {
  const prefix = t(($) => $.contentToday.topics.effortLabel);
  if (effort.kind === "hours") {
    return `${prefix}: ${t(($) => $.contentToday.topics.effortHours, { hours: effort.weeklyHours })}`;
  }
  if (effort.kind === "no-account") {
    return `${prefix}: ${t(($) => $.contentToday.topics.effortNoAccount)}`;
  }
  return `${prefix}: ${t(($) => $.contentToday.topics.effortUnconfirmed)}`;
}

// ---------------------------------------------------------------- section 2

function WorksSection({ wsId }: { wsId: string }) {
  const { t } = useT("common");
  const navigation = useNavigation();
  const works = useContentWorks(wsId);
  const list = works.data ?? [];
  const results = useQueries({
    queries: list.map((work) => ({
      queryKey: workEditorKeys.artifacts(wsId, work.workId),
      queryFn: async () => parseArtifacts(await api.listContentArtifacts(work.workId)),
    })),
  });

  const artifacts = new Map<string, ReturnType<typeof parseArtifacts>>();
  const failed = new Set<string>();
  list.forEach((work, index) => {
    const result = results[index];
    if (result?.data) artifacts.set(work.workId, result.data);
    // A work whose documents could not be read stays in the list, marked.
    // Dropping it would make "nothing in progress" and "could not tell" look
    // identical.
    if (result?.isError) failed.add(work.workId);
  });
  const section = worksInProgress(list, artifacts, failed);

  return (
    <SectionShell
      title={t(($) => $.contentToday.works.title)}
      state={queryState(works.isPending || results.some((r) => r.isPending), works.isError)}
      section={section}
      t={t}
      onSeeAll={() => navigation.push(`/${wsId}/topics`)}
      renderRow={(entry: WorkEntry) => (
        <SettingsRow
          key={entry.workId}
          label={entry.workTitle || entry.workId}
          description={
            entry.failed
              ? t(($) => $.contentToday.rowFailed)
              : `${t(($) => $.contentToday.works.working)}: ${entry.artifactTitle}`
          }
        >
          <Button variant="outline" onClick={() => navigation.push(`/${wsId}/topics`)}>
            {t(($) => $.contentToday.open)}
          </Button>
        </SettingsRow>
      )}
    />
  );
}

// ---------------------------------------------------------------- section 3

function ReviewsSection({ wsId }: { wsId: string }) {
  const { t } = useT("common");
  const navigation = useNavigation();
  const reviews = useContentReviews(wsId);
  const section = reviewsNeedingAttention(reviews.data ?? []);
  return (
    <SectionShell
      title={t(($) => $.contentToday.reviews.title)}
      // Review sections live inside the topics page, under a card's work and
      // document. There is no route that addresses one review, so opening a
      // row lands on the topics page rather than on the row itself - which is
      // what the hint says, instead of implying a precise link.
      description={t(($) => $.contentToday.deepLinkHint)}
      state={queryState(reviews.isPending, reviews.isError)}
      section={section}
      t={t}
      onSeeAll={() => navigation.push(`/${wsId}/topics`)}
      renderRow={(entry: ReviewEntry) => (
        <SettingsRow
          key={entry.reviewRequestId}
          label={entry.channel}
          description={t(($) => $.contentToday.reviews.status[statusKey(entry.status)])}
        >
          <Button variant="outline" onClick={() => navigation.push(`/${wsId}/topics`)}>
            {t(($) => $.contentToday.open)}
          </Button>
        </SettingsRow>
      )}
    />
  );
}

function statusKey(status: string): "pending" | "changesRequested" {
  return status === "changes_requested" ? "changesRequested" : "pending";
}

// ---------------------------------------------------------------- section 4

function DeliveriesSection({ wsId }: { wsId: string }) {
  const { t } = useT("common");
  const navigation = useNavigation();
  const deliveries = useContentDeliveries(wsId);
  const section = deliveriesNeedingAction(deliveries.data ?? []);
  return (
    <SectionShell
      title={t(($) => $.contentToday.deliveries.title)}
      description={t(($) => $.contentToday.deepLinkHint)}
      state={queryState(deliveries.isPending, deliveries.isError)}
      section={section}
      t={t}
      onSeeAll={() => navigation.push(`/${wsId}/topics`)}
      renderRow={(entry: DeliveryEntry) => (
        <SettingsRow
          key={entry.deliveryTaskId}
          label={entry.channel}
          description={
            entry.reason === "due"
              ? t(($) => $.contentToday.deliveries.due)
              : t(($) => $.contentToday.deliveries.pendingRegistration)
          }
        >
          <Button variant="outline" onClick={() => navigation.push(`/${wsId}/topics`)}>
            {t(($) => $.contentToday.open)}
          </Button>
        </SettingsRow>
      )}
    />
  );
}

// ---------------------------------------------------------------- section 5

/**
 * SOP §2's fifth item. `feedback-learning` does not exist yet, so there is no
 * data source at all.
 *
 * The section is still here, stating why. Hiding it would say the product does
 * not intend to do this; a blank would read as unfinished; a spinner would read
 * as nearly ready. Same treatment the three AI entry points got in 024.
 */
function FeedbackSection() {
  const { t } = useT("common");
  return (
    <SettingsSection
      title={t(($) => $.contentToday.feedback.title)}
      description={t(($) => $.contentToday.feedback.unavailable)}
    >
      <SettingsCard>
        <SettingsRow label={t(($) => $.contentToday.feedback.unavailableShort)}>
          <span className="text-caption text-muted-foreground" />
        </SettingsRow>
      </SettingsCard>
    </SettingsSection>
  );
}

// ---------------------------------------------------------------- section 6

function AccountGapsSection({ wsId }: { wsId: string }) {
  const { t } = useT("common");
  const navigation = useNavigation();
  const accounts = useContentAccounts(wsId);
  const list = accounts.data ?? [];
  const { profiles, failed, pending } = useAccountProfiles(
    wsId,
    list.map((account) => account.account_id),
  );
  const section = accountsMissingConfig(list, profiles, failed);

  return (
    <SectionShell
      // Its own name, not SOP §2's fifth item. This is the business-setup gap
      // list; calling it "待补录的反馈" would quietly answer a different question.
      title={t(($) => $.contentToday.accountGaps.title)}
      state={queryState(accounts.isPending || pending, accounts.isError)}
      section={section}
      t={t}
      onSeeAll={() => navigation.push(`/${wsId}/accounts`)}
      renderRow={(entry: AccountGapEntry) => (
        <SettingsRow
          key={entry.accountId}
          label={entry.displayName || entry.accountId}
          description={
            entry.failed
              ? t(($) => $.contentToday.rowFailed)
              : `${t(($) => $.contentToday.accountGaps.missingLabel)}: ${entry.missing
                  .map((field) => missingFieldLabel(t, field))
                  .join(t(($) => $.contentToday.listSeparator))}`
          }
        >
          <Button variant="outline" onClick={() => navigation.push(`/${wsId}/accounts`)}>
            {t(($) => $.contentToday.open)}
          </Button>
        </SettingsRow>
      )}
    />
  );
}

function missingFieldLabel(t: Translate, field: string): string {
  switch (field) {
    case "audience":
      return t(($) => $.contentToday.accountGaps.fields.audience);
    case "content_pillars":
      return t(($) => $.contentToday.accountGaps.fields.contentPillars);
    case "primary_channels":
      return t(($) => $.contentToday.accountGaps.fields.primaryChannels);
    case "weekly_hours":
      return t(($) => $.contentToday.accountGaps.fields.weeklyHours);
    default:
      // A newer server may name a field this build has no label for. Showing
      // the raw field name beats dropping the item.
      return field;
  }
}

// ------------------------------------------------------------------- shared

type SectionState = "loading" | "error" | "ready";

function queryState(pending: boolean, error: boolean): SectionState {
  if (error) return "error";
  if (pending) return "loading";
  return "ready";
}

/**
 * One section's frame: heading, its own loading and error states, its rows,
 * and what the cap left out.
 *
 * Each section renders its own state so one failing source cannot blank the
 * page - that is the whole point of FR-015.
 */
function SectionShell<T>({
  title,
  description,
  state,
  section,
  renderRow,
  onSeeAll,
  t,
}: {
  title: string;
  description?: string;
  state: SectionState;
  section: Section<T>;
  renderRow: (entry: T) => ReactNode;
  onSeeAll: () => void;
  t: Translate;
}) {
  return (
    <SettingsSection title={title} description={description}>
      <SettingsCard>
        {state === "loading" ? (
          <SettingsRow label={t(($) => $.contentToday.loading)}>
            <span className="text-caption text-muted-foreground" />
          </SettingsRow>
        ) : state === "error" ? (
          <SettingsRow label={t(($) => $.contentToday.sectionFailed)}>
            <span className="text-caption text-muted-foreground" />
          </SettingsRow>
        ) : section.shown.length === 0 ? (
          <SettingsRow label={t(($) => $.contentToday.empty)}>
            <span className="text-caption text-muted-foreground" />
          </SettingsRow>
        ) : (
          section.shown.map((entry) => renderRow(entry))
        )}
        {state === "ready" && section.hidden > 0 ? (
          <SettingsRow
            label={t(($) => $.contentToday.more, {
              hidden: section.hidden,
              total: section.total,
            })}
          >
            <Button variant="outline" onClick={onSeeAll}>
              {t(($) => $.contentToday.seeAll)}
            </Button>
          </SettingsRow>
        ) : null}
      </SettingsCard>
    </SettingsSection>
  );
}

function uniq(values: string[]): string[] {
  return [...new Set(values)];
}
