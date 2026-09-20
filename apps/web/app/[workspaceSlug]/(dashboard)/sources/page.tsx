"use client";
import {useWorkspaceId} from "@multica/core/hooks";
import {SourceInboxPage} from "@multica/views/content/source-inbox";
export default function Page(){const wsId=useWorkspaceId();return <SourceInboxPage key={wsId} wsId={wsId}/>}
