"use client";

import { SettingsPage } from "@multica/views/settings";
import { OperatingRulesSections } from "@multica/views/content/workspace-core";

// The workspace tab's brand-specific block is wired here rather than inside
// the shared tab: SOP §3.2's operating rules (specs/029) fetch and mutate
// through their own hooks, and a shared upstream component whose render
// depends on a downstream module's data layer breaks that component's own
// tests (Issue #208). The app layer is where the two are allowed to meet.
export default function Page() {
  return (
    <SettingsPage
      renderWorkspaceExtras={({ wsId, canManage }) => (
        <OperatingRulesSections wsId={wsId} canManage={canManage} />
      )}
    />
  );
}
