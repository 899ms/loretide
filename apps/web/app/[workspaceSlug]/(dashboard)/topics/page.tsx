"use client";
import {useWorkspaceId} from "@multica/core/hooks";
import {useContentAccounts} from "@multica/core/content/ip-profile";
import {TopicPlanningPage} from "@multica/views/content/topic-planning";
import {WorkSections} from "@multica/views/content/work-editor";
import {ReviewDeliverySections} from "@multica/views/content/review-delivery";
// Four content modules, composed here rather than by one importing another.
// The registry places the work editor downstream of topic planning, and
// review-delivery downstream of the work editor; neither of those edges runs in
// the direction an import would need, so the adapter is what puts them
// together. The accounts list comes from here for the same reason:
// review-delivery is not registered against ip-profile.
export default function Page(){
  const wsId=useWorkspaceId();
  const accounts=useContentAccounts(wsId);
  const options=(accounts.data??[]).map((account)=>({id:account.account_id,name:account.display_name}));
  return <TopicPlanningPage key={wsId} wsId={wsId} renderCardExtras={(topicCardId)=>
    <WorkSections wsId={wsId} topicCardId={topicCardId} renderArtifactExtras={(context)=>
      <ReviewDeliverySections
        wsId={context.wsId}
        artifactId={context.artifactId}
        artifactKind={context.artifactKind}
        latestVersionId={context.latestVersionId}
        versionCount={context.versionCount}
        accounts={options}
      />
    }/>
  }/>;
}
