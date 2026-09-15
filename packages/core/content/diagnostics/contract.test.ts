// @vitest-environment node
// Canonical layer for the diagnostic wire contract and its pure helpers.
// FR map (specs/002-diag-package-stream-recovery/spec.md):
//   FR-001 rotate defaults to false and survives a page without the field
//   FR-002 streamQuery carries every filter key plus the cursor
//   FR-005 parseContentDispositionFilename reads the server-chosen file name
//   FR-008 mergeEvents de-duplicates by event_id, sorts by sequence, caps at STREAM_EVENT_CAP
// Feature 005 (specs/005-diag-trace-and-sanitize):
//   FR-012 route / status / headers_present / upstream_trace default when the
//          server has not sent them yet, and a malformed event still falls back
import {describe,it,expect} from "vitest";
import {pageSchema,parseDiagnostic,runSchema,overviewSchema,mergeEvents,streamQuery,parseContentDispositionFilename,STREAM_EVENT_CAP,type DiagnosticEvent} from "./contract";
function event(id:string,sequence:number):DiagnosticEvent{return {eventId:id,sequence,occurredAt:"",receivedAt:"",actorKind:"system",actorId:"",workspaceId:"ws-1",accountId:"",objectType:"diagnostics",objectId:"",objectVersion:"",action:"query",outcome:"success",errorCode:"",operationId:"",traceId:"",spanId:"",parentSpanId:"",runId:"",attempt:1,step:"",component:"api",severity:"info",durationMs:0,safeMessage:"",retryable:false,nextAction:"",build:"test",isTest:true,route:"",status:0,headersPresent:[],upstreamTrace:""}}
// The wire event a current server sends. Feature 005 adds four optional fields
// on top of it; an older server omits them entirely.
const wireEvent={event_id:"a".repeat(32),sequence:1,occurred_at:"2026-01-01T00:00:00Z",received_at:"2026-01-01T00:00:00Z",actor_kind:"system",actor_id:"",workspace_id:"ws-1",account_id:"",object_type:"diagnostics",object_id:"run",object_version:"1",action:"query",outcome:"success",error_code:"",operation_id:"b".repeat(32),trace_id:"c".repeat(32),span_id:"d".repeat(16),parent_span_id:"",run_id:"run",attempt:1,step:"api",component:"api",severity:"info",duration_ms:1,safe_message:"Completed",retryable:false,next_action:"inspect_trace",build:"test",is_test:true};
describe("diagnostic API contracts",()=>{
 it("rejects malformed success and absent run evidence",()=>{expect(()=>parseDiagnostic({events:"ok",cursor:1},pageSchema)).toThrow("OUTPUT_SCHEMA");expect(()=>parseDiagnostic({regression:"passed"},runSchema)).toThrow()});
 it("accepts additive fields without inventing results",()=>{expect(parseDiagnostic({events:[],cursor:0,gap:false,has_more:false,future:true},pageSchema).events).toEqual([]);expect(mergeEvents([],[])).toEqual([])});
 // FR-001
 it("reads rotate as false unless the page says otherwise",()=>{
  expect(parseDiagnostic({events:[],cursor:0,gap:false,has_more:false},pageSchema).rotate).toBe(false);
  expect(parseDiagnostic({events:[],cursor:7,gap:false,has_more:false,rotate:true},pageSchema).rotate).toBe(true);
  expect(()=>parseDiagnostic({events:[],gap:false,has_more:false,rotate:true},pageSchema)).toThrow("OUTPUT_SCHEMA");
 });
 // FR-002
 it("carries every filter key and the cursor into the stream query",()=>{
  expect(streamQuery(undefined,0)).toBe("after=0");
  expect(streamQuery({},12)).toBe("after=12");
  expect(streamQuery({severity:"error",component:""},3)).toBe("severity=error&after=3");
  expect(streamQuery({kind:"technical",traceId:"t1",runId:"r1",component:"api",errorCode:"TIMEOUT",severity:"error",from:"2026-01-01T00:00:00.000Z",until:"2026-01-02T00:00:00.000Z"},9))
   .toBe("kind=technical&trace_id=t1&run_id=r1&component=api&error_code=TIMEOUT&severity=error&from=2026-01-01T00%3A00%3A00.000Z&until=2026-01-02T00%3A00%3A00.000Z&after=9");
 });
 // FR-005
 it("reads the download file name from Content-Disposition or reports none",()=>{
  expect(parseContentDispositionFilename(`attachment; filename="loretide-diagnostics-run-1.json"`)).toBe("loretide-diagnostics-run-1.json");
  expect(parseContentDispositionFilename("attachment; filename=loretide-diagnostics.json")).toBe("loretide-diagnostics.json");
  expect(parseContentDispositionFilename("attachment")).toBeNull();
  expect(parseContentDispositionFilename(null)).toBeNull();
  expect(parseContentDispositionFilename(`attachment; filename="../../etc/passwd"`)).toBe("passwd");
  expect(parseContentDispositionFilename(`attachment; filename=""`)).toBeNull();
 });
 // FR-012 (feature 005)
 it("defaults the request-identity fields when an older server omits them",()=>{
  const page=parseDiagnostic({events:[wireEvent],cursor:0,gap:false,has_more:false},pageSchema);
  const parsed=page.events[0];
  expect(parsed?.route).toBe("");
  expect(parsed?.status).toBe(0);
  expect(parsed?.headersPresent).toEqual([]);
  expect(parsed?.upstreamTrace).toBe("");
 });
 // FR-012 (feature 005)
 it("carries the request identity through when the server sends it",()=>{
  const sent={...wireEvent,route:"GET /api/content-diagnostics/events",status:200,headers_present:["user-agent"],upstream_trace:"e".repeat(32)};
  const parsed=parseDiagnostic({events:[sent],cursor:0,gap:false,has_more:false},pageSchema).events[0];
  expect(parsed?.route).toBe("GET /api/content-diagnostics/events");
  expect(parsed?.status).toBe(200);
  expect(parsed?.headersPresent).toEqual(["user-agent"]);
  expect(parsed?.upstreamTrace).toBe("e".repeat(32));
 });
 // FR-012 (feature 005)
 it("still falls back when an event is malformed, new fields notwithstanding",()=>{
  const {event_id:_dropped,...missingId}=wireEvent;
  expect(()=>parseDiagnostic({events:[{...missingId,route:"GET /x",status:200}],cursor:0,gap:false,has_more:false},pageSchema)).toThrow("OUTPUT_SCHEMA");
 });
 // FR-008
 it("de-duplicates by event id, orders by sequence and keeps the newest page of events",()=>{
  const merged=mergeEvents([event("a",2),event("b",1)],[event("a",2),event("c",3)]);
  expect(merged.map(e=>e.eventId)).toEqual(["b","a","c"]);
  const flood=Array.from({length:STREAM_EVENT_CAP+5},(_,i)=>event(`e${i}`,i));
  const capped=mergeEvents([],flood);
  expect(capped).toHaveLength(STREAM_EVENT_CAP);
  expect(capped[0]?.sequence).toBe(5);
  expect(capped.at(-1)?.sequence).toBe(STREAM_EVENT_CAP+4);
 });
 // Feature 012 (specs/012-diag-simulator-shapes), FR-013 / SC-010.
 // Scenario gained a "kind" field. contracts/scenario-kind.md C-1/C-2: an
 // installed desktop build talks to older backends, and parseWithFallback
 // failing as a whole would take the components and metrics down with it -
 // so a missing or malformed kind must never sink the overview.
 const wireOverview={components:[{name:"api",status:"healthy",last_seen:"2026-01-01T00:00:00Z",version:"test",reason:""}],metrics:{sample_count:1,errors:0,p50_ms:1,p95_ms:null,retries:0,cancelled:0,dropped:0,sink_errors:0,queue_wait_ms:0},scenarios:[{id:"normal",expected_code:""}],instance:"native-development",build:"test",simulation_enabled:true,retention_days:7,capacity:10000};
 it("reads the scenario kind a current server sends",()=>{
  const parsed=parseDiagnostic({...wireOverview,scenarios:[{id:"normal",expected_code:"",kind:"fault"},{id:"shape_orphan",expected_code:"",kind:"shape"}]},overviewSchema);
  expect(parsed.scenarios.map(s=>s.kind)).toEqual(["fault","shape"]);
 });
 it("defaults the scenario kind to fault when an older server omits it",()=>{
  const parsed=parseDiagnostic(wireOverview,overviewSchema);
  expect(parsed.scenarios[0]?.kind).toBe("fault");
  // The rest of the overview has to survive intact: that is the whole reason
  // the field is optional rather than required.
  expect(parsed.components).toHaveLength(1);
  expect(parsed.metrics.sampleCount).toBe(1);
 });
 it("does not sink the overview when one scenario kind is malformed",()=>{
  const parsed=parseDiagnostic({...wireOverview,scenarios:[{id:"normal",expected_code:"",kind:42}]},overviewSchema);
  expect(parsed.scenarios[0]?.kind).toBe("fault");
  expect(parsed.components).toHaveLength(1);
  expect(parsed.metrics.sinkErrors).toBe(0);
 });
});
