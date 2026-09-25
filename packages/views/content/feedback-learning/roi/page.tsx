"use client";

import { useState } from "react";
import { useT } from "@multica/views/i18n";
import { Button } from "@multica/ui/components/ui/button";
import { SettingsContent, SettingsTab } from "@multica/views/settings/layout";
import { PageHeader } from "@multica/views/layout/page-header";
import { CostsBlock } from "./costs";
import { DealsBlock } from "./deals";
import { ImportsBlock } from "./imports";
import { LeadsBlock } from "./leads";
import { ReportBlock } from "./report";
import type { RoiFocus, RoiOption } from "./shared";

// Cost, lead, deal and ROI review (specs/034 PR 5, FR-080 to FR-082).
//
// Five blocks - costs, leads, deals, import, report - built from the settings
// page's components (SettingsSection / SettingsCard / SettingsRow) like the
// topic and marketing-node pages. No new control and no new colour: "not
// computable", a negative return and "source unknown" use the secondary text
// colour, because they are facts, not failures.
//
// Nothing on this page acts outside it: no budget change, payment, contact,
// automatic attribution or automatic exchange rate, and no AI call.

export type { RoiOption } from "./shared";

export interface RoiReviewPageProps {
  wsId: string;
  /** The brand's time zone; the report window is read in it (server side). */
  brandTimezone: string;
  accounts: RoiOption[];
  accountsLoading?: boolean;
  accountsFailed?: boolean;
  works: RoiOption[];
  worksLoading?: boolean;
  worksFailed?: boolean;
}

const BLOCKS = ["costs", "leads", "deals", "imports", "report"] as const;
type Block = (typeof BLOCKS)[number];

export function RoiReviewPage(props: RoiReviewPageProps) {
  const { t } = useT("common");
  return (
    <>
      <PageHeader className="xl:hidden">
        <span className="text-body font-medium">{t(($) => $.contentRoiReview.title)}</span>
      </PageHeader>
      <RoiReviewContent {...props} />
    </>
  );
}

function RoiReviewContent(props: RoiReviewPageProps) {
  const { t } = useT("common");
  const [block, setBlock] = useState<Block>("costs");
  const [focus, setFocus] = useState<RoiFocus | null>(null);
  const options = {
    accounts: props.accounts,
    accountsLoading: props.accountsLoading,
    accountsFailed: props.accountsFailed,
    works: props.works,
    worksLoading: props.worksLoading,
    worksFailed: props.worksFailed,
  };

  const open = (next: Block) => {
    setFocus(null);
    setBlock(next);
  };

  // A record in a report's provenance opens its own block, at that record,
  // with that revision marked.
  const focusRecord = (next: RoiFocus) => {
    setFocus(next);
    setBlock(next.kind === "cost" ? "costs" : next.kind === "lead" ? "leads" : "deals");
  };

  const focusKey = focus ? `${focus.kind}-${focus.id}-${focus.revision ?? ""}` : "none";

  return (
    <SettingsContent>
      <SettingsTab title={t(($) => $.contentRoiReview.title)} description={t(($) => $.contentRoiReview.description)}>
        <div className="flex flex-wrap gap-2" role="tablist" aria-label={t(($) => $.contentRoiReview.title)}>
          {BLOCKS.map((item) => (
            <Button
              key={item}
              role="tab"
              aria-selected={item === block}
              variant={item === block ? "brand" : "outline"}
              size="sm"
              onClick={() => open(item)}
            >
              {t(($) => $.contentRoiReview.blocks[item])}
            </Button>
          ))}
        </div>
        {/* Hidden rather than unmounted, so a half-filled form or an open
            report is still there when the person comes back. */}
        <div hidden={block !== "costs"} className="space-y-8">
          <CostsBlock key={focusKey} wsId={props.wsId} options={options} focus={focus} />
        </div>
        <div hidden={block !== "leads"} className="space-y-8">
          <LeadsBlock key={focusKey} wsId={props.wsId} options={options} focus={focus} />
        </div>
        <div hidden={block !== "deals"} className="space-y-8">
          <DealsBlock key={focusKey} wsId={props.wsId} options={options} focus={focus} />
        </div>
        <div hidden={block !== "imports"} className="space-y-8">
          <ImportsBlock wsId={props.wsId} />
        </div>
        <div hidden={block !== "report"} className="space-y-8">
          <ReportBlock wsId={props.wsId} brandTimezone={props.brandTimezone} options={options} onFocus={focusRecord} />
        </div>
      </SettingsTab>
    </SettingsContent>
  );
}
