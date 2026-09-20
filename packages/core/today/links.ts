// Where the Today dashboard's rows send a person.
//
// It exists because the page got this wrong (Issue #191): it built
// `/${wsId}/topics` from `useWorkspaceId()`, which returns the workspace's
// UUID, while the route segment is the workspace SLUG. Every link on the page
// resolved to no workspace and landed on "no access".
//
// Putting the destinations behind one function that takes a slug means the
// mistake has one place to be made rather than twelve, and a test can name it.

import { paths } from "@multica/core/paths";

export interface TodayLinks {
  topics: string;
  accounts: string;
  sources: string;
}

/**
 * The three destinations the dashboard links to, for one workspace.
 *
 * `slug` is the workspace SLUG - the thing that appears in the URL - never its
 * id. `useRequiredWorkspaceSlug()` is where a page gets one;
 * `useWorkspaceId()` is not.
 */
export function todayLinks(slug: string): TodayLinks {
  const workspace = paths.workspace(slug);
  return {
    topics: workspace.topics(),
    accounts: workspace.accounts(),
    sources: workspace.sources(),
  };
}
