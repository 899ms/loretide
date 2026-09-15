"use client";
import {useWorkspaceId} from "@multica/core/hooks";
import {AccountSettingsPage} from "@multica/views/content/ip-profile";
export default function Page(){const wsId=useWorkspaceId();return <AccountSettingsPage key={wsId} wsId={wsId}/>}
