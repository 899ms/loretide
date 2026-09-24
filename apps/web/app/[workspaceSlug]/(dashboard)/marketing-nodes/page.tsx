"use client";
import {useWorkspaceId} from "@multica/core/hooks";
import {paths, useCurrentWorkspace, useRequiredWorkspaceSlug} from "@multica/core/paths";
import {getWorkspaceTimezone} from "@multica/core/workspace";
import {useContentAccounts} from "@multica/core/content/ip-profile";
import {useContentSources} from "@multica/core/content/source-inbox";
import {MarketingNodesPage} from "@multica/views/content/topic-planning";
// Marketing nodes (specs/033 PR 3, plan D1). An adapter for the same reason
// topics/page.tsx is one: topic-planning is not registered against
// source-inbox, so the materials list is read here and passed in, and the
// accounts list comes the same way. The link to the topic page is built from
// the workspace slug, never the workspace id (Issue #191).
export default function Page(){
  const wsId=useWorkspaceId();
  const slug=useRequiredWorkspaceSlug();
  const workspace=useCurrentWorkspace();
  const accounts=useContentAccounts(wsId);
  const sources=useContentSources(wsId);
  return <MarketingNodesPage
    key={wsId}
    wsId={wsId}
    brandTimezone={getWorkspaceTimezone(workspace ?? undefined)}
    accounts={(accounts.data??[]).map((account)=>({accountId:account.account_id,name:account.display_name}))}
    accountsLoading={accounts.isLoading}
    accountsFailed={accounts.isError}
    sources={(sources.data??[]).map((source)=>({sourceId:source.sourceId,title:source.title,url:source.url,status:source.status}))}
    sourcesLoading={sources.isLoading}
    sourcesFailed={sources.isError}
    topicsHref={paths.workspace(slug).topics()}
  />;
}
