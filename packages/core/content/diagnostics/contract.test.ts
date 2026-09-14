// @vitest-environment node
// Canonical layer for the diagnostic wire contract and its pure helpers.
// FR map (specs/002-diag-package-stream-recovery/spec.md):
//   FR-001 rotate defaults to false and survives a page without the field
//   FR-002 streamQuery carries every filter key plus the cursor
//   FR-005 parseContentDispositionFilename reads the server-chosen file name
//   FR-008 mergeEvents de-duplicates by event_id, sorts by sequence, caps at STREAM_EVENT_CAP
import {describe,it,expect} from "vitest";
import {pageSchema,parseDiagnostic,runSchema,mergeEvents,streamQuery,parseContentDispositionFilename,STREAM_EVENT_CAP,type DiagnosticEvent} from "./contract";
function event(id:string,sequence:number):DiagnosticEvent{return {eventId:id,sequence,occurredAt:"",receivedAt:"",actorKind:"system",actorId:"",workspaceId:"ws-1",accountId:"",objectType:"diagnostics",objectId:"",objectVersion:"",action:"query",outcome:"success",errorCode:"",operationId:"",traceId:"",spanId:"",parentSpanId:"",runId:"",attempt:1,step:"",component:"api",severity:"info",durationMs:0,safeMessage:"",retryable:false,nextAction:"",build:"test",isTest:true}}
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
});
