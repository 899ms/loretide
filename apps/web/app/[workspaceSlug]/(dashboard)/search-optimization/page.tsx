"use client";

import { Suspense, useEffect, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { useSearchParams } from "next/navigation";
import { useWorkspaceId } from "@multica/core/hooks";
import { useContentAccounts } from "@multica/core/content/ip-profile";
import { CONTENT_PLATFORMS } from "@multica/core/content/ip-profile";
import { useContentTopics } from "@multica/core/content/topic-planning";
import { useContentSources } from "@multica/core/content/source-inbox";
import { useArtifactVersions, useContentArtifacts, useContentWorks } from "@multica/core/content/work-editor";
import { useContentPublications } from "@multica/core/content/review-delivery";
import { useSearchThemes } from "@multica/core/content/topic-planning";
import { useRankObservations } from "@multica/core/content/feedback-learning";
import { SearchPerformanceSections } from "@multica/views/content/feedback-learning";
import { SearchSuggestionsSection, SearchThemesSection } from "@multica/views/content/topic-planning";
import { PageHeader } from "@multica/views/layout/page-header";
import { useT } from "@multica/views/i18n";

type Option = { id: string; label: string; platform: string };

function SearchOptimizationAdapter() {
  const { t } = useT("common");
  const wsId = useWorkspaceId();
  const params = useSearchParams();
  const client = useQueryClient();
  const initialWorkId = params.get("work_id") ?? "";
  const initialArtifactId = params.get("artifact_id") ?? "";
  const [activeWorkId, setActiveWorkId] = useState(initialWorkId);
  const [activeArtifactId, setActiveArtifactId] = useState(initialArtifactId);
  const [selectedThemeId, setSelectedThemeId] = useState("");
  const accounts = useContentAccounts(wsId);
  const topics = useContentTopics(wsId, "");
  const sources = useContentSources(wsId);
  const works = useContentWorks(wsId);
  const publications = useContentPublications(wsId);
  const artifacts = useContentArtifacts(wsId, activeWorkId);
  const versions = useArtifactVersions(wsId, activeWorkId, activeArtifactId);
  const themes = useSearchThemes(wsId, { includeArchived: true });
  const rankObservations = useRankObservations(wsId, { themeId: selectedThemeId });
  useEffect(() => {
    setActiveWorkId(initialWorkId);
    setActiveArtifactId(initialArtifactId);
  }, [initialWorkId, initialArtifactId]);
  const accountOptions: Option[] = (accounts.data ?? []).map((account) => ({ id: account.account_id, label: account.display_name || account.account_id, platform: account.platform }));
  const topicCards = topics.data ?? [];
  const sourceList = sources.data ?? [];
  const workList = works.data ?? [];
  const publicationList = publications.data ?? [];
  const artifactList = artifacts.data ?? [];
  const versionList = versions.data ?? [];
  const themeList = themes.data ?? [];
  const sourceOptions = sourceList.map((source) => ({ id: source.sourceId, label: source.title || source.sourceId }));
  function refreshOnConflict() {
    void client.invalidateQueries({ queryKey: ["contentSearch", wsId] });
    void client.invalidateQueries({ queryKey: ["contentWorks", wsId] });
    void client.invalidateQueries({ queryKey: ["contentPublications", wsId] });
  }

  return (
    <>
      <PageHeader><h1 className="min-w-0 flex-1 truncate text-title">{t(($) => $.search_optimization.title)}</h1></PageHeader>
      <main className="space-y-6 p-4">
        <p className="text-body text-muted-foreground">{t(($) => $.search_optimization.description)}</p>
        <SearchThemesSection wsId={wsId} topicCards={topicCards} sources={sourceOptions} accounts={accountOptions} platforms={[...CONTENT_PLATFORMS]} topicCardsLoading={topics.isPending} topicCardsError={topics.isError} sourcesLoading={sources.isPending} sourcesError={sources.isError} accountsLoading={accounts.isPending} accountsError={accounts.isError} rankObservations={rankObservations.data ?? []} rankObservationsLoading={rankObservations.isPending} rankObservationsError={rankObservations.isError} onSelectedThemeChange={setSelectedThemeId} onConflict={refreshOnConflict} />
        <SearchSuggestionsSection key={`${wsId}:${activeWorkId}:${activeArtifactId}`} wsId={wsId} works={workList} artifacts={artifactList} versions={versionList} themes={themeList} sources={sourceOptions} workId={activeWorkId} artifactId={activeArtifactId} worksLoading={works.isPending} worksError={works.isError} artifactsLoading={artifacts.isPending} artifactsError={artifacts.isError} versionsLoading={versions.isPending} versionsError={versions.isError} themesLoading={themes.isPending} themesError={themes.isError} sourcesLoading={sources.isPending} sourcesError={sources.isError} onWorkIdChange={setActiveWorkId} onArtifactIdChange={setActiveArtifactId} onConflict={refreshOnConflict} />
        <SearchPerformanceSections wsId={wsId} publications={publicationList} accounts={accountOptions} themes={themeList} publicationsLoading={publications.isPending} publicationsError={publications.isError} accountsLoading={accounts.isPending} accountsError={accounts.isError} themesLoading={themes.isPending} themesError={themes.isError} onConflict={refreshOnConflict} />
      </main>
    </>
  );
}

function Loading() {
  const { t } = useT("common");
  return <div className="p-4 text-caption text-muted-foreground">{t(($) => $.search_optimization.loading)}</div>;
}

export default function Page() {
  return <Suspense fallback={<Loading />}><SearchOptimizationAdapter /></Suspense>;
}
