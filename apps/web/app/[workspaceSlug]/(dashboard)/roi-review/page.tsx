"use client";
import {useWorkspaceId} from "@multica/core/hooks";
import {useCurrentWorkspace} from "@multica/core/paths";
import {getWorkspaceTimezone} from "@multica/core/workspace";
import {useContentAccounts} from "@multica/core/content/ip-profile";
import {useContentWorks} from "@multica/core/content/work-editor";
import {RoiReviewPage} from "@multica/views/content/feedback-learning";
// Cost, lead, deal and ROI review (specs/034 PR 5). An adapter for the same
// reason marketing-nodes/page.tsx is one: feedback-learning is not registered
// against ip-profile or work-editor, so the brand's accounts and works are
// read here and passed in as plain {id, name} options. Records store those
// ids as strings; whether an id exists is the server's check.
export default function Page(){
  const wsId=useWorkspaceId();
  const workspace=useCurrentWorkspace();
  const accounts=useContentAccounts(wsId);
  const works=useContentWorks(wsId);
  return <RoiReviewPage
    key={wsId}
    wsId={wsId}
    brandTimezone={getWorkspaceTimezone(workspace ?? undefined)}
    accounts={(accounts.data??[]).map((account)=>({id:account.account_id,name:account.display_name}))}
    accountsLoading={accounts.isLoading}
    accountsFailed={accounts.isError}
    works={(works.data??[]).map((work)=>({id:work.workId,name:work.title}))}
    worksLoading={works.isLoading}
    worksFailed={works.isError}
  />;
}
