"use client";

import { useT } from "@multica/views/i18n";
import { Translation } from "react-i18next";
import { Component, useState, type ReactNode } from "react";
import {
  useDiagnostics,
  useDiagnosticStream,
  recordDiagnosticClientError,
  describeDiagnosticError,
  STREAM_EVENT_CAP,
  type DiagnosticDownload,
  type DiagnosticEvent,
  type DiagnosticRun,
} from "@multica/core/content/diagnostics";

import { Button } from "@multica/ui/components/ui/button";
import { Input } from "@multica/ui/components/ui/input";
import { Badge } from "@multica/ui/components/ui/badge";
import {
  Progress,
  ProgressLabel,
  ProgressValue,
} from "@multica/ui/components/ui/progress";
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
                <Button variant="outline" onClick={() => onTrace(e)}>
                  {t(($) => $.diagnostics.text006)}
                </Button>
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
  const onTrace = (event: DiagnosticEvent) => {
    setRunId(event.runId);
    setTrace(event.traceId);
    setComponent("");
    setSeverity("");
    setCode("");
    setFrom("");
    setUntil("");
    setAfter(0);
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
                          {c.reason || "当前连接可用"}
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
                      <div className="space-y-3">
                        {run.events.map((e, i) => (
                          <SettingsCard key={e.eventId}>
                            <SettingsRow
                              label={e.step}
                              size="text"
                              align="start"
                            >
                              <Progress
                                aria-label={e.step}
                                value={
                                  (e.durationMs /
                                    Math.max(
                                      ...run.events.map((x) => x.durationMs),
                                      1,
                                    )) *
                                  100
                                }
                              >
                                <ProgressLabel>
                                  {e.errorCode || e.outcome}{" "}
                                  {t(($) => $.diagnostics.text070)}
                                  {e.attempt}
                                </ProgressLabel>
                                <ProgressValue>
                                  {() => (
                                    <>
                                      {e.durationMs}{" "}
                                      {t(($) => $.diagnostics.text071)}
                                    </>
                                  )}
                                </ProgressValue>
                              </Progress>
                              <p className="break-all text-caption">
                                {t(($) => $.diagnostics.text072)}
                                {e.spanId} {t(($) => $.diagnostics.text073)}
                                {e.parentSpanId}{" "}
                                {t(($) => $.diagnostics.text074)}
                                {i + 1}
                              </p>
                              <p>
                                {e.safeMessage}{" "}
                                {t(($) => $.diagnostics.text075)}
                                {e.nextAction}
                              </p>
                            </SettingsRow>
                          </SettingsCard>
                        ))}
                      </div>
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
            {tab === 3 && !runId && trace && (
              <SettingsSection>
                <p>{trace}</p>
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
                          <TableCell>{r.regression}</TableCell>
                          <TableCell>
                            {r.module} {t(($) => $.diagnostics.text088)}
                            {r.build || "unknown"}
                          </TableCell>
                          <TableCell>
                            {r.originalRunId.slice(0, 8) || "—"}
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
