"use client";
import {useWorkspaceId} from "@multica/core/hooks";
import {TopicPlanningPage} from "@multica/views/content/topic-planning";
export default function Page(){const wsId=useWorkspaceId();return <TopicPlanningPage key={wsId} wsId={wsId}/>}
