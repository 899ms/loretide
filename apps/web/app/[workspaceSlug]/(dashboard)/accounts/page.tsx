"use client";

import { useContentMetrics } from "@multica/core/content/feedback-learning";
import { useWorkspaceId } from "@multica/core/hooks";
import { AccountSettingsPage } from "@multica/views/content/ip-profile";

export default function Page() {
  const wsId = useWorkspaceId();
  const metrics = useContentMetrics(wsId);
  return (
    <AccountSettingsPage
      key={wsId}
      wsId={wsId}
      performance={{
        pending: metrics.isPending,
        failed: metrics.isError,
        metricAccountIds: (metrics.data ?? []).map((metric) => metric.accountId),
      }}
    />
  );
}
