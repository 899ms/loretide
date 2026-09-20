"use client";
import {useWorkspaceId} from "@multica/core/hooks";
import {TopicPlanningPage} from "@multica/views/content/topic-planning";
import {WorkSections} from "@multica/views/content/work-editor";
// The work editor is its own content module and the registry places it
// downstream of topic planning, so the two are composed here rather than by
// one importing the other.
export default function Page(){const wsId=useWorkspaceId();return <TopicPlanningPage key={wsId} wsId={wsId} renderCardExtras={(topicCardId)=><WorkSections wsId={wsId} topicCardId={topicCardId}/>}/>}
