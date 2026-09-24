"use client";

import { useT } from "@multica/views/i18n";
import { Translation } from "react-i18next";
import { Component, useState, type ReactNode } from "react";
import {
  useDiagnostics,
  useDiagnosticStream,
  recordDiagnosticClientError,
  describeDiagnosticError,
  buildTraceWaterfall,
  describeRegressionVerdict,
  describeRunLinkage,
  describeTraceJump,
  describeObjectVersions,
  STREAM_EVENT_CAP,
  type DiagnosticDownload,
  type DiagnosticEvent,
  type DiagnosticRun,
} from "@multica/core/content/diagnostics";

import { Button } from "@multica/ui/components/ui/button";
import { Input } from "@multica/ui/components/ui/input";
import { Badge } from "@multica/ui/components/ui/badge";
import { Progress } from "@multica/ui/components/ui/progress";
import { PageHeader } from "@multica/views/layout/page-header";
import { Alert, AlertDescription } from "@multica/ui/components/ui/alert";
import {
  Tabs,
  TabsList,
  TabsTrigger,
  TabsContent,
} from "@multica/ui/components/ui/tabs";
import {
  Select,
  SelectTrigger,
  SelectValue,
  SelectContent,
  SelectItem,
} from "@multica/ui/components/ui/select";
import {
  Table,
  TableHeader,
  TableBody,
  TableRow,
  TableHead,
  TableCell,
} from "@multica/ui/components/ui/table";
import {
  SettingsContent,
  SettingsTab,
  SettingsSection,
  SettingsCard,
  SettingsRow,
} from "@multica/views/settings/layout";

type Props = {
  wsId: string;
  copy: (text: string) => Promise<void>;
  // Receives the export response as the server sent it, so the saved file is
  // byte-for-byte what the endpoint returned. Injected by the platform layer:
  // this package holds no platform APIs, which is what lets desktop adopt the
  // same page later.
  download: (result: DiagnosticDownload) => void;
};
const tabs = [
  "概览",
  "操作时间线",
  "技术日志",
  "单次追踪",
  "运行详情",
  "故障复现",
  "回归结果",
  "诊断导出",
];
const labels: Record<string, string> = {
  normal: "正常",
  slow: "慢响应",
  timeout: "超时",
  cancel: "取消",
  reconnect: "断线与重连",
  duplicate: "重复消息",
  late: "迟到消息",
  file_missing: "文件缺失",
  file_changed: "文件变化",
  denied: "授权拒绝",
  database: "数据库写失败",
  model_auth: "模型认证失败",
  model_quota: "模型额度不足",
  schema: "输出格式错误",
  search: "搜索失败",
  clock_skew: "时钟偏差",
};
// Fixed component reason codes the diagnostics server can emit
// (server/internal/content/diagnostics). Any other code is shown as-is.
const componentReasonCodes = [
  "config_unknown",
  "liveness_unverified",
  "config_unconfigured",
  "storage_not_configured",
  "heartbeat_without_configuration",
  "heartbeat_expired",
  "clock_skew",
  "execution_disabled",
  "database_connectivity_unavailable",
  "heartbeat_missing_timestamp",
] as const;
type ComponentReasonCode = (typeof componentReasonCodes)[number];
function isComponentReasonCode(reason: string): reason is ComponentReasonCode {
  return (componentReasonCodes as readonly string[]).includes(reason);
}
// The "view trace" affordance. An event whose trace id is absent or malformed
// has nothing to jump to, so the button is disabled and says why. Jumping
// anyway would clear every filter and show the whole technical log, which the
// reader would take for the cause of the failure they were looking at.
function TraceJumpCell({
  event,
  onTrace,
}: {
  event: DiagnosticEvent;
  onTrace: (e: DiagnosticEvent) => void;
}) {
  const { t } = useT("common");
  const jump = describeTraceJump(event);
  const blocked = jump.kind !== "filter";
  return (
    <div className="space-y-1">
      <Button
        variant="outline"
        disabled={blocked}
        onClick={() => onTrace(event)}
      >
        {t(($) => $.diagnostics.text006)}
      </Button>
      {blocked && (
        <p className="text-caption text-muted-foreground">
          {t(($) => $.diagnostics.text115)}
        </p>
      )}
    </div>
  );
}
// The object this trace touched, and the versions recorded for it. Grouped from
// the events already on screen: the event query has no object dimension, so
// reaching further would mean a new server read path (FR-019). That bounds what
// can be shown, which is why the scope note below is not optional - without it a
// reader takes this for the object's full version history.
function ObjectVersions({ events }: { events: DiagnosticEvent[] }) {
  const { t } = useT("common");
  const first = events[0];
  const group = describeObjectVersions(events, {
    objectType: first?.objectType ?? "",
    objectId: first?.objectId ?? "",
  });
  if (group.state === "unknownObject") return null;
  return (
    <SettingsCard>
      <SettingsRow label={t(($) => $.diagnostics.text116)} size="text">
        <div className="space-y-1">
          <p className="break-all">
            {group.objectType} {t(($) => $.diagnostics.text003)}
            {group.objectId}
          </p>
          {group.state === "present" ? (
            <p className="break-all">
              {group.versions
                .map((v) => `${v.version} (${v.count})`)
                .join(" / ")}
            </p>
          ) : (
            <p className="text-muted-foreground">
              {t(($) => $.diagnostics.text117)}
            </p>
          )}
          <p className="text-caption text-muted-foreground">
            {t(($) => $.diagnostics.text118)}
          </p>
        </div>
      </SettingsRow>
    </SettingsCard>
  );
}
function Events({
  events,
  onTrace,
}: {
  events: DiagnosticEvent[];
  onTrace: (e: DiagnosticEvent) => void;
}) {
  const { t } = useT("common");
  return (
    <div className="overflow-x-auto">
      <Table>
        <TableHeader>
          <TableRow>
            {["时间 / 来源", "操作与对象", "组件 / 步骤", "结果", "追踪"].map(
              (x) => (
                <TableHead key={x}>{x}</TableHead>
              ),
            )}
          </TableRow>
        </TableHeader>
        <TableBody>
          {events.map((e) => (
            <TableRow key={e.eventId}>
              <TableCell>
                {e.receivedAt}
                <br />
                {e.actorKind} {t(($) => $.diagnostics.text001)}
                {e.actorId.slice(0, 8)}
              </TableCell>
              <TableCell>
                {e.action} {t(($) => $.diagnostics.text002)}
                {e.objectType}
                <br />
                {e.objectId.slice(0, 12)} {t(($) => $.diagnostics.text003)}
                {e.objectVersion}
              </TableCell>
              <TableCell>
                {e.component} {t(($) => $.diagnostics.text004)}
                {e.step}
                <br />
                {t(($) => $.diagnostics.text005)}
                {e.attempt}
              </TableCell>
              <TableCell>
                {e.outcome}
                <br />
                {e.errorCode || "—"}
              </TableCell>
              <TableCell>
                <TraceJumpCell event={e} onTrace={onTrace} />
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
      {events.length === 0 && (
        <p className="p-4 text-muted-foreground">
          {t(($) => $.diagnostics.text007)}
        </p>
      )}
    </div>
  );
}
// The verdict column. A run that was never evaluated carries the backend's own
// literal "not_run" (simulator.go:15), which this table used to print raw next
// to real verdicts. describeRegressionVerdict turns it into one of four explicit
// states, and nothing uncertain is allowed to read as a pass (FR-005).
function RegressionVerdictCell({ run }: { run: DiagnosticRun }) {
  const { t } = useT("common");
  const verdict = describeRegressionVerdict(run);
  return (
    <div className="space-y-1">
      <Badge
        variant={
          verdict.verdict === "passed"
            ? "secondary"
            : verdict.verdict === "failed"
              ? "destructive"
              : "outline"
        }
      >
        {verdict.verdict === "passed" && t(($) => $.diagnostics.text103)}
        {verdict.verdict === "failed" && t(($) => $.diagnostics.text104)}
        {verdict.verdict === "not_run" && t(($) => $.diagnostics.text105)}
        {verdict.verdict === "undecidable" && t(($) => $.diagnostics.text106)}
      </Badge>
      {/* A pass has to carry its own basis, or "passed" is just an assertion. */}
      {verdict.verdict === "passed" && (
        <p className="text-caption">
          {t(($) => $.diagnostics.text107)}{" "}
          {t(($) => $.diagnostics.text109)}
          {verdict.expectedCode || "—"} {t(($) => $.diagnostics.text110)}
          {verdict.actualCode || "—"}
        </p>
      )}
      {/* Undecidable is only actionable if the raw value is visible. */}
      {verdict.verdict === "undecidable" && (
        <p className="break-all text-caption">
          {t(($) => $.diagnostics.text108)}
          {verdict.rawRegression || "—"}
        </p>
      )}
    </div>
  );
}

// Module and build, each marked "not recorded" when absent rather than left
// blank, so a missing value is distinguishable from an unread one (FR-007).
function RunLocators({
  run,
  readableRunIds,
}: {
  run: DiagnosticRun;
  readableRunIds: ReadonlySet<string>;
}) {
  const { t } = useT("common");
  const linkage = describeRunLinkage(run, readableRunIds);
  return (
    <span className="break-all">
      {linkage.module.state === "present"
        ? linkage.module.value
        : t(($) => $.diagnostics.text113)}{" "}
      {t(($) => $.diagnostics.text088)}
      {linkage.build.state === "present"
        ? linkage.build.value
        : t(($) => $.diagnostics.text113)}
    </span>
  );
}

// The original fault. "First run" and "broken link" mean opposite things to a
// reader, so self / unreadable / missing are kept apart (FR-007). The jump is an
// in-page switch through the existing run selection, not a new route (FR-008).
function OriginalRunCell({
  run,
  readableRunIds,
  onOpen,
}: {
  run: DiagnosticRun;
  readableRunIds: ReadonlySet<string>;
  onOpen: (runId: string) => void;
}) {
  const { t } = useT("common");
  const { originalRun } = describeRunLinkage(run, readableRunIds);
  if (originalRun.state === "self") {
    return <span>{t(($) => $.diagnostics.text111)}</span>;
  }
  if (originalRun.state === "linkable") {
    return (
      <Button
        variant="outline"
        aria-label={t(($) => $.diagnostics.text114)}
        onClick={() => onOpen(originalRun.runId)}
      >
        {originalRun.runId.slice(0, 8)}
      </Button>
    );
  }
  if (originalRun.state === "unreadable") {
    return (
      <span className="text-caption">
        {t(($) => $.diagnostics.text112)}
      </span>
    );
  }
  return <span>{t(($) => $.diagnostics.text113)}</span>;
}

// The trace waterfall. Hierarchy, offsets and collapsing are computed by
// buildTraceWaterfall in @multica/core; this component only draws the rows it is
// handed, because constitution principle II forbids UI unit tests and anything
// that needs proving has to sit where a test can reach it.
//
// UI policy gap (docs/development/design/README.md): a time-positioned bar has
// no counterpart in packages/ui. The closest upstream component is Progress,
// which fills from its own left edge and cannot express an offset from the run
// start. Progress is kept as the bar itself; the horizontal offset is a layout
// wrapper around it, using semantic tokens and the --text-* scale only. No new
// control is introduced.
function TraceWaterfall({ run }: { run: DiagnosticRun }) {
  const { t } = useT("common");
  // Expanding is a per-view glance, not a preference: it dies with the run being
  // looked at, so it stays local rather than going to a store (research D5).
  const [expanded, setExpanded] = useState(false);
  // Expanding lifts the cap for this run only; the default bound still applies
  // on first render so a large run cannot flood the page (FR-004).
  const waterfall = buildTraceWaterfall(
    run.events,
    expanded ? { cap: run.events.length } : {},
  );

  if (waterfall.rows.length === 0) {
    return <p>{t(($) => $.diagnostics.text100)}</p>;
  }

  // The axis spans from the run start to the last span's end, so a gap between
  // two bars is real waiting rather than a rendering artefact.
  const totalMs = Math.max(
    ...waterfall.rows.map((r) => r.startOffsetMs + r.durationMs),
    1,
  );

  return (
    <div className="space-y-3">
      {waterfall.rows.map((row) => {
        if (row.kind === "collapsed") {
          return (
            <SettingsCard key={row.eventId}>
              <SettingsRow
                label={`${t(($) => $.diagnostics.text098)}${row.collapsedCount}${t(($) => $.diagnostics.text099)}`}
                size="text"
                align="start"
              >
                <Button variant="outline" onClick={() => setExpanded(!expanded)}>
                  {expanded
                    ? t(($) => $.diagnostics.text102)
                    : t(($) => $.diagnostics.text101)}
                </Button>
              </SettingsRow>
            </SettingsCard>
          );
        }
        const event = run.events.find((e) => e.eventId === row.eventId);
        const offsetPercent = (row.startOffsetMs / totalMs) * 100;
        // A zero-duration span still has to be visible, so the bar keeps a floor
        // width rather than collapsing to nothing.
        const widthPercent = Math.max((row.durationMs / totalMs) * 100, 1.5);
        return (
          <div
            key={row.eventId}
            style={{ marginInlineStart: `${Math.min(row.depth, 8) * 16}px` }}
          >
            <SettingsCard>
              <SettingsRow
                label={event?.step ?? row.spanId}
                size="text"
                align="start"
              >
                <p className="text-caption">
                  {event?.errorCode || event?.outcome}{" "}
                  {t(($) => $.diagnostics.text070)}
                  {event?.attempt} · {row.startOffsetMs}
                  {t(($) => $.diagnostics.text071)} → {row.durationMs}
                  {t(($) => $.diagnostics.text071)}
                </p>
                <div className="flex w-full items-center">
                  <div
                    aria-hidden
                    style={{ width: `${offsetPercent}%` }}
                    className="shrink-0"
                  />
                  <div style={{ width: `${widthPercent}%` }} className="shrink-0">
                    <Progress
                      aria-label={event?.step ?? row.spanId}
                      value={100}
                    />
                  </div>
                </div>
                {row.anomaly && (
                  <Badge variant="outline">
                    {row.anomaly === "clockSkew" &&
                      t(($) => $.diagnostics.text093)}
                    {row.anomaly === "orphan" && t(($) => $.diagnostics.text094)}
                    {row.anomaly === "cycle" && t(($) => $.diagnostics.text095)}
                    {row.anomaly === "invalidTime" &&
                      t(($) => $.diagnostics.text096)}
                    {row.anomaly === "invalidDuration" &&
                      t(($) => $.diagnostics.text097)}
                  </Badge>
                )}
                <p className="break-all text-caption">
                  {t(($) => $.diagnostics.text072)}
                  {row.spanId} {t(($) => $.diagnostics.text073)}
                  {row.parentSpanId}
                </p>
                <p>
                  {event?.safeMessage} {t(($) => $.diagnostics.text075)}
                  {event?.nextAction}
                </p>
              </SettingsRow>
            </SettingsCard>
          </div>
        );
      })}
    </div>
  );
}

function Snapshot({ run }: { run: DiagnosticRun }) {
  const { t } = useT("common");
  return (
    <>
      <SettingsCard>
        <SettingsRow
          label={t(($) => $.diagnostics.text008)}
          size="text"
          align="start"
        >
          <span className="break-all">
            {run.snapshot.sourceScope} {t(($) => $.diagnostics.text009)}
            {run.snapshot.savedPreference}
          </span>
        </SettingsRow>
        <SettingsRow
          label={t(($) => $.diagnostics.text010)}
          size="text"
          align="start"
        >
          <span className="break-all">
            {run.snapshot.executor} {t(($) => $.diagnostics.text011)}
            {run.snapshot.budget}
          </span>
        </SettingsRow>
        <SettingsRow
          label={t(($) => $.diagnostics.text012)}
          size="text"
          align="start"
        >
          <span className="break-all">
            {run.snapshot.configVersion} {t(($) => $.diagnostics.text013)}
            {run.snapshot.sopVersion} {t(($) => $.diagnostics.text014)}
            {run.snapshot.skillVersion} {t(($) => $.diagnostics.text015)}
            {run.snapshot.ruleVersion}
          </span>
        </SettingsRow>
        <SettingsRow
          label={t(($) => $.diagnostics.text016)}
          size="text"
          align="start"
        >
          <span className="break-all">{run.snapshot.personaRef}</span>
        </SettingsRow>
        <SettingsRow
          label={t(($) => $.diagnostics.text017)}
          size="text"
          align="start"
        >
          <span className="break-all">
            {run.snapshot.grants.join(", ")} {t(($) => $.diagnostics.text018)}
            {run.snapshot.requiredSources.join(", ")}{" "}
            {t(($) => $.diagnostics.text019)}
            {run.snapshot.excludedSources.join(", ") || "无"}
          </span>
        </SettingsRow>
        <SettingsRow
          label={t(($) => $.diagnostics.text020)}
          size="text"
          align="start"
        >
          <span className="break-all">
            {run.snapshot.temperature} {t(($) => $.diagnostics.text021)}
            {run.snapshot.timeoutMs} {t(($) => $.diagnostics.text022)}
          </span>
        </SettingsRow>
      </SettingsCard>
      <SettingsCard>
        {Object.entries(run.snapshot.fileHashes).map(([path, hash]) => (
          <SettingsRow
            key={path}
            label={<span className="break-all">{path}</span>}
            size="text"
          >
            <span className="break-all">{hash}</span>
          </SettingsRow>
        ))}
      </SettingsCard>
      <p className="mt-4">
        {t(($) => $.diagnostics.text023)}
        {run.reproductionGaps.join("；") ||
          "固定测试资料完整；真实文件与执行器尚未接入"}
      </p>
    </>
  );
}

export function DiagnosticsPage(props: Props) {
  const { t } = useT("common");
  return (
    <>
      <PageHeader className="xl:hidden">
        <span className="text-body font-medium">
          {t(($) => $.diagnostics.text024)}
        </span>
      </PageHeader>
      <DiagnosticBoundary wsId={props.wsId}>
        <DiagnosticsContent {...props} />
      </DiagnosticBoundary>
    </>
  );
}
function DiagnosticsContent({ wsId, copy, download }: Props) {
  const { t } = useT("common");
  const [crash, setCrash] = useState(false);
  const [tab, setTab] = useState(0);
  const [runId, setRunId] = useState("");
  const [scenario, setScenario] = useState("normal");
  const [seed, setSeed] = useState(42);
  const [after, setAfter] = useState(0);
  const [component, setComponent] = useState("");
  const [severity, setSeverity] = useState("");
  const [code, setCode] = useState("");
  const [trace, setTrace] = useState("");
  const [from, setFrom] = useState("");
  const [until, setUntil] = useState("");
  const [live, setLive] = useState(false);
  const [notice, setNotice] = useState("");
  const params = new URLSearchParams({
    kind: tab === 1 ? "audit" : "technical",
    after: String(after),
    limit: "25",
    component,
    severity,
    error_code: code,
    trace_id: trace,
  });
  if (from) params.set("from", new Date(from).toISOString());
  if (until) params.set("until", new Date(until).toISOString());
  const d = useDiagnostics(wsId, runId, params.toString());
  // Which original runs can actually be opened, taken from the runs already
  // fetched for this table. Deriving it here rather than asking the server keeps
  // this feature from adding a data path or widening any scope filter (FR-012).
  const readableRunIds = new Set(
    (d.runs.data?.runs ?? []).map((x) => x.runId),
  );
  // The live stream answers the same question as the paged technical log, so a
  // resume after a rotation or a dropped connection keeps the same filter.
  const stream = useDiagnosticStream(wsId, live, {
    kind: "technical",
    traceId: trace,
    component,
    severity,
    errorCode: code,
    from: from ? new Date(from).toISOString() : "",
    until: until ? new Date(until).toISOString() : "",
  });
  const overview = d.overview.data;
  const run = d.detail.data;
  // Where to look next is derived in core so it can be asserted (principle II
  // forbids testing this component). An event with no usable trace id yields
  // "unavailable" rather than an empty filter, which used to drop the reader
  // into the unfiltered technical log right after they asked for one trace.
  const onTrace = (event: DiagnosticEvent) => {
    const jump = describeTraceJump(event);
    if (jump.kind !== "filter") return;
    setRunId(jump.filter.runId);
    setTrace(jump.filter.traceId);
    setComponent("");
    setSeverity("");
    setCode("");
    setFrom("");
    setUntil("");
    setAfter(jump.filter.after);
    setTab(3);
  };
  const simulate = async (originalRunId?: string) => {
    try {
      const result = await d.simulate.mutateAsync({
        scenario,
        seed,
        originalRunId,
      });
      setRunId(result.runId);
      setNotice(
        `模拟运行已保存：${result.actualCode || "正常"}；回归 ${result.regression}`,
      );
      setTab(4);
    } catch {
      setNotice("模拟失败。请查看错误和追踪，不会调用真实模型。");
    }
  };
  const error =
    d.overview.error ||
    d.events.error ||
    d.runs.error ||
    d.detail.error ||
    d.simulate.error ||
    d.preview.error ||
    d.download.error;
  if (crash) throw new Error("diagnostic UI fixture");
  return (
    <SettingsContent>
      <SettingsTab
        title={t(($) => $.diagnostics.text024)}
        description={t(($) => $.diagnostics.text025)}
      >
        <Alert>
          <AlertDescription>{t(($) => $.diagnostics.text026)}</AlertDescription>
        </Alert>
        <Tabs
          value={tab}
          onValueChange={(value) => {
            setTab(Number(value));
            setAfter(0);
          }}
        >
          <TabsList
            aria-label={t(($) => $.diagnostics.text024)}
            variant="default"
            className="max-w-full overflow-x-auto overflow-y-hidden"
          >
            {tabs.map((x, i) => (
              <TabsTrigger key={x} value={i}>
                {x}
              </TabsTrigger>
            ))}
          </TabsList>
          <TabsContent value={tab}>
            {error && (
              <Alert variant="destructive">
                <AlertDescription>
                  {describeDiagnosticError(error).message}{" "}
                  {describeDiagnosticError(error).traceId}
                  <Button
                    variant="outline"
                    onClick={() => {
                      void d.overview.refetch();
                      void d.events.refetch();
                      void d.runs.refetch();
                    }}
                  >
                    {t(($) => $.diagnostics.text027)}
                  </Button>
                </AlertDescription>
              </Alert>
            )}
            {notice && (
              <Alert role="status">
                <AlertDescription>{notice}</AlertDescription>
              </Alert>
            )}
            {tab === 0 && (
              <SettingsSection>
                <SettingsSection title={t(($) => $.diagnostics.text028)}>
                  {null}
                </SettingsSection>
                <p>
                  {t(($) => $.diagnostics.text029)}
                  {overview?.instance ?? "读取中"}{" "}
                  {t(($) => $.diagnostics.text030)}
                  {overview?.build || "unknown"}
                </p>
                <SettingsCard>
                  {overview?.components.map((c) => (
                    <SettingsRow
                      key={c.name}
                      label={c.name}
                      description={
                        <>
                          {c.reason
                            ? isComponentReasonCode(c.reason)
                              ? t(
                                  ($) =>
                                    $.diagnostics.reasons[
                                      c.reason as ComponentReasonCode
                                    ],
                                )
                              : c.reason
                            : c.status === "healthy"
                              ? t(($) => $.diagnostics.reasonHealthy)
                              : null}
                          <br />
                          {t(($) => $.diagnostics.text032)}
                          {c.lastSeen ?? "未收到"}
                          <br />
                          {t(($) => $.diagnostics.text033)}
                          {c.version || "未验证"}
                        </>
                      }
                    >
                      <Badge
                        variant={
                          c.status === "healthy" ? "secondary" : "outline"
                        }
                      >
                        {c.status}
                      </Badge>
                    </SettingsRow>
                  ))}
                </SettingsCard>
                <SettingsSection title={t(($) => $.diagnostics.text034)}>
                  {null}
                </SettingsSection>
                <p>
                  {t(($) => $.diagnostics.text035)}
                  {overview?.metrics.sampleCount ?? 0}{" "}
                  {t(($) => $.diagnostics.text036)}
                  {overview?.metrics.errors ?? 0}{" "}
                  {t(($) => $.diagnostics.text037)}
                  {overview?.metrics.p50Ms ?? 0}{" "}
                  {t(($) => $.diagnostics.text038)}
                  {overview?.metrics.p95Ms ?? "样本不足"}{" "}
                  {t(($) => $.diagnostics.text039)}
                  {overview?.metrics.queueWaitMs ?? 0}{" "}
                  {t(($) => $.diagnostics.text040)}
                </p>
                <p>
                  {t(($) => $.diagnostics.text041)}
                  {overview?.metrics.retries ?? 0}{" "}
                  {t(($) => $.diagnostics.text042)}
                  {overview?.metrics.cancelled ?? 0}{" "}
                  {t(($) => $.diagnostics.text043)}
                  {overview?.metrics.dropped ?? 0}{" "}
                  {t(($) => $.diagnostics.text044)}
                  {overview?.metrics.sinkErrors ?? 0}
                </p>
                <p>
                  {t(($) => $.diagnostics.text045)}
                  {overview?.retentionDays ?? 7}{" "}
                  {t(($) => $.diagnostics.text046)}
                  {overview?.capacity ?? 10000}{" "}
                  {t(($) => $.diagnostics.text047)}
                </p>
              </SettingsSection>
            )}
            {(tab === 1 || tab === 2) && (
              <SettingsSection>
                <SettingsCard>
                  <SettingsRow
                    label={t(($) => $.diagnostics.text048)}
                    size="text"
                  >
                    <Input
                      aria-label={t(($) => $.diagnostics.text048)}
                      value={component}
                      onChange={(e) => {
                        setComponent(e.target.value);
                        setAfter(0);
                      }}
                    />
                  </SettingsRow>
                  <SettingsRow
                    label={t(($) => $.diagnostics.text049)}
                    size="text"
                  >
                    <Select
                      items={[
                        { value: "", label: "全部" },
                        ...["info", "warn", "error"].map((value) => ({
                          value,
                          label: value,
                        })),
                      ]}
                      value={severity}
                      onValueChange={(value) => {
                        setSeverity(value ?? "");
                        setAfter(0);
                      }}
                    >
                      <SelectTrigger
                        aria-label={t(($) => $.diagnostics.text049)}
                      >
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        {["", "info", "warn", "error"].map((x) => (
                          <SelectItem key={x} value={x}>
                            {x || "全部"}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  </SettingsRow>
                  <SettingsRow
                    label={t(($) => $.diagnostics.text050)}
                    size="text"
                  >
                    <Input
                      aria-label={t(($) => $.diagnostics.text050)}
                      value={code}
                      onChange={(e) => {
                        setCode(e.target.value);
                        setAfter(0);
                      }}
                    />
                  </SettingsRow>
                  <SettingsRow
                    label={t(($) => $.diagnostics.text051)}
                    size="text"
                  >
                    <Input
                      aria-label={t(($) => $.diagnostics.text051)}
                      value={trace}
                      onChange={(e) => {
                        setTrace(e.target.value);
                        setAfter(0);
                      }}
                    />
                  </SettingsRow>
                  <SettingsRow
                    label={t(($) => $.diagnostics.text052)}
                    size="text"
                  >
                    <Input
                      aria-label={t(($) => $.diagnostics.text052)}
                      type="datetime-local"
                      value={from}
                      onChange={(e) => {
                        setFrom(e.target.value);
                        setAfter(0);
                      }}
                    />
                  </SettingsRow>
                  <SettingsRow
                    label={t(($) => $.diagnostics.text053)}
                    size="text"
                  >
                    <Input
                      aria-label={t(($) => $.diagnostics.text053)}
                      type="datetime-local"
                      value={until}
                      onChange={(e) => {
                        setUntil(e.target.value);
                        setAfter(0);
                      }}
                    />
                  </SettingsRow>
                </SettingsCard>
                <Events
                  events={d.events.data?.events ?? []}
                  onTrace={onTrace}
                />
                {d.events.data?.gap && (
                  <p role="alert">{t(($) => $.diagnostics.text054)}</p>
                )}
                <div className="flex gap-2">
                  <Button variant="outline" onClick={() => setAfter(0)}>
                    {t(($) => $.diagnostics.text055)}
                  </Button>
                  <Button
                    variant="outline"
                    disabled={!d.events.data?.hasMore}
                    onClick={() => setAfter(d.events.data?.cursor ?? 0)}
                  >
                    {t(($) => $.diagnostics.text056)}
                  </Button>
                  <Button
                    variant="outline"
                    onClick={() => {
                      setTrace("");
                      setComponent("");
                      setSeverity("");
                      setCode("");
                      setFrom("");
                      setUntil("");
                      setAfter(0);
                    }}
                  >
                    {t(($) => $.diagnostics.text057)}
                  </Button>
                </div>
                {tab === 2 && (
                  <>
                    <SettingsSection title={t(($) => $.diagnostics.text058)}>
                      {null}
                    </SettingsSection>
                    <Button variant="outline" onClick={() => setLive(!live)}>
                      {live ? "暂停实时流" : "连接 / 继续实时流"}
                    </Button>
                    <p>
                      {t(($) => $.diagnostics.text059)}
                      {stream.status} {stream.gap ? "· 检测到保留期缺口" : ""}
                    </p>
                    {stream.notice && (
                      <Alert variant="destructive">
                        <AlertDescription>{stream.notice}</AlertDescription>
                      </Alert>
                    )}
                    {stream.gap && (
                      <Button variant="outline" onClick={stream.clearGap}>
                        清除缺口提示
                      </Button>
                    )}
                    {stream.events.length >= STREAM_EVENT_CAP && (
                      <p role="status">仅显示最近 {STREAM_EVENT_CAP} 条</p>
                    )}
                    <Events events={stream.events} onTrace={onTrace} />
                  </>
                )}
              </SettingsSection>
            )}
            {(tab === 3 || tab === 4 || tab === 7) && (
              <SettingsSection>
                <SettingsRow
                  label={t(($) => $.diagnostics.text060)}
                  size="text"
                >
                  <Select
                    items={[
                      { value: "", label: t(($) => $.diagnostics.text061) },
                      ...(d.runs.data?.runs ?? []).map((r) => ({
                        value: r.runId,
                        label:
                          (labels[r.scenario] ?? r.scenario) +
                          " · " +
                          r.runId.slice(0, 8) +
                          " · " +
                          r.regression,
                      })),
                    ]}
                    value={runId}
                    onValueChange={(value) => setRunId(value ?? "")}
                  >
                    <SelectTrigger aria-label={t(($) => $.diagnostics.text060)}>
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="">
                        {t(($) => $.diagnostics.text061)}
                      </SelectItem>
                      {d.runs.data?.runs.map((r) => (
                        <SelectItem value={r.runId} key={r.runId}>
                          {labels[r.scenario] ?? r.scenario}{" "}
                          {t(($) => $.diagnostics.text062)}
                          {r.runId.slice(0, 8)}{" "}
                          {t(($) => $.diagnostics.text063)}
                          {r.regression}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </SettingsRow>
                {run && tab !== 7 && (
                  <>
                    <p>
                      {t(($) => $.diagnostics.text064)}
                      {run.runId} {t(($) => $.diagnostics.text065)}
                      {run.status} {t(($) => $.diagnostics.text066)}
                      <strong>{t(($) => $.diagnostics.text067)}</strong>
                    </p>
                    <Button
                      variant="outline"
                      onClick={() =>
                        void copy(run.events[0]?.traceId ?? run.runId)
                          .then(() => setNotice("追踪编号已复制"))
                          .catch(() => setNotice("复制失败，请手动选择编号"))
                      }
                    >
                      {t(($) => $.diagnostics.text068)}
                    </Button>
                    {tab === 4 ? (
                      <Snapshot run={run} />
                    ) : (
                      <TraceWaterfall run={run} />
                    )}
                  </>
                )}
                {tab === 7 && run && (
                  <>
                    <p>{t(($) => $.diagnostics.text076)}</p>
                    <Button
                      variant="outline"
                      onClick={() => void d.preview.refetch()}
                    >
                      {t(($) => $.diagnostics.text077)}
                    </Button>
                    {d.preview.data && (
                      <>
                        <ul className="list-inside list-disc">
                          {d.preview.data.manifest.map((x) => (
                            <li key={x}>{x}</li>
                          ))}
                        </ul>
                        <p>{d.preview.data.limits.join("；")}</p>
                        <Button
                          variant="outline"
                          disabled={d.download.isPending}
                          onClick={() =>
                            void d.download
                              .mutateAsync()
                              .then(download)
                              .catch((cause: unknown) =>
                                setNotice(
                                  `导出失败，未生成文件：${describeDiagnosticError(cause).message}`,
                                ),
                              )
                          }
                        >
                          {t(($) => $.diagnostics.text078)}
                        </Button>
                      </>
                    )}
                  </>
                )}
              </SettingsSection>
            )}
            {/* The single-trace view, for whichever way the reader arrived.
                This used to require !runId, but onTrace always sets a run id,
                so arriving through "view trace" - the documented path - skipped
                this section entirely and the object/version card was reachable
                only by reloading and typing a trace id by hand (008-J-1). The
                condition is now just "a trace is selected": the waterfall above
                still renders from the run, and these read the same already
                fetched events, which the query filters by trace_id. No new
                data path. */}
            {tab === 3 && trace && (
              <SettingsSection>
                <p>{trace}</p>
                <ObjectVersions events={d.events.data?.events ?? []} />
                <Events
                  events={d.events.data?.events ?? []}
                  onTrace={onTrace}
                />
              </SettingsSection>
            )}
            {tab === 5 && (
              <SettingsSection
                title={t(($) => $.diagnostics.text079)}
                description={t(($) => $.diagnostics.text080)}
              >
                <SettingsCard>
                  <SettingsRow
                    label={t(($) => $.diagnostics.text081)}
                    size="text"
                  >
                    <Select
                      items={(overview?.scenarios ?? []).map((s) => ({
                        value: s.id,
                        label: labels[s.id] ?? s.id,
                      }))}
                      value={scenario}
                      onValueChange={(value) => setScenario(value ?? "normal")}
                    >
                      <SelectTrigger
                        aria-label={t(($) => $.diagnostics.text081)}
                      >
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        {overview?.scenarios.map((s) => (
                          <SelectItem key={s.id} value={s.id}>
                            {labels[s.id] ?? s.id}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  </SettingsRow>
                  <SettingsRow
                    label={t(($) => $.diagnostics.text082)}
                    size="text"
                  >
                    <Input
                      aria-label={t(($) => $.diagnostics.text082)}
                      type="number"
                      value={seed}
                      onChange={(e) => setSeed(Number(e.target.value))}
                    />
                  </SettingsRow>
                  <SettingsRow label={null}>
                    <Button
                      variant="outline"
                      disabled={
                        overview?.simulationEnabled !== true ||
                        d.simulate.isPending
                      }
                      onClick={() => void simulate()}
                    >
                      {t(($) => $.diagnostics.text083)}
                    </Button>
                  </SettingsRow>
                  <SettingsRow label={null}>
                    <Button
                      variant="outline"
                      disabled={overview?.simulationEnabled !== true}
                      onClick={() => setCrash(true)}
                    >
                      {t(($) => $.diagnostics.uiFault)}
                    </Button>
                  </SettingsRow>
                  {runId && (
                    <SettingsRow label={null}>
                      <Button
                        variant="outline"
                        disabled={d.simulate.isPending}
                        onClick={() => void simulate(runId)}
                      >
                        {t(($) => $.diagnostics.text084)}
                      </Button>
                    </SettingsRow>
                  )}
                </SettingsCard>
              </SettingsSection>
            )}
            {tab === 6 && (
              <SettingsSection
                title={t(($) => $.diagnostics.text085)}
                description={t(($) => $.diagnostics.text086)}
              >
                <div className="overflow-x-auto">
                  <Table>
                    <TableHeader>
                      <TableRow>
                        {[
                          "场景",
                          "预期 / 实际",
                          "回归",
                          "模块 / 构建",
                          "原故障",
                          "详情",
                        ].map((x) => (
                          <TableHead key={x}>{x}</TableHead>
                        ))}
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {d.runs.data?.runs.map((r) => (
                        <TableRow key={r.runId}>
                          <TableCell>
                            {labels[r.scenario] ?? r.scenario}
                          </TableCell>
                          <TableCell>
                            {r.expectedCode || "正常"}{" "}
                            {t(($) => $.diagnostics.text087)}
                            {r.actualCode || "正常"}
                          </TableCell>
                          <TableCell>
                            <RegressionVerdictCell run={r} />
                          </TableCell>
                          <TableCell>
                            <RunLocators run={r} readableRunIds={readableRunIds} />
                          </TableCell>
                          <TableCell>
                            <OriginalRunCell
                              run={r}
                              readableRunIds={readableRunIds}
                              onOpen={(id) => {
                                setRunId(id);
                                setTab(4);
                              }}
                            />
                          </TableCell>
                          <TableCell>
                            <Button
                              variant="outline"
                              onClick={() => {
                                setRunId(r.runId);
                                setTab(4);
                              }}
                            >
                              {t(($) => $.diagnostics.text089)}
                            </Button>
                          </TableCell>
                        </TableRow>
                      ))}
                    </TableBody>
                  </Table>
                </div>
              </SettingsSection>
            )}
          </TabsContent>
        </Tabs>
      </SettingsTab>
    </SettingsContent>
  );
}
class DiagnosticBoundary extends Component<
  { children: ReactNode; wsId: string },
  { failed: boolean; trace: string }
> {
  state = { failed: false, trace: "" };
  static getDerivedStateFromError() {
    return { failed: true };
  }
  componentDidCatch() {
    void recordDiagnosticClientError()
      .then((e) => this.setState({ trace: e.traceId }))
      .catch(() => {});
  }
  render() {
    if (this.state.failed)
      return (
        <Translation ns="common">
          {(t) => (
            <SettingsContent>
              <SettingsTab title={t(($) => $.diagnostics.text090)}>
                <Alert variant="destructive">
                  <AlertDescription>
                    <p>
                      {t(($) => $.diagnostics.text091)}
                      {this.state.trace || "暂时无法写入诊断记录"}
                    </p>
                    <Button
                      variant="outline"
                      onClick={() =>
                        this.setState({ failed: false, trace: "" })
                      }
                    >
                      {t(($) => $.diagnostics.text092)}
                    </Button>
                  </AlertDescription>
                </Alert>
              </SettingsTab>
            </SettingsContent>
          )}
        </Translation>
      );
    return this.props.children;
  }
}
