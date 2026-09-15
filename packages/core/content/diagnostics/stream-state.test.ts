// @vitest-environment node
// Canonical layer for the live-stream state machine.
// FR map (specs/002-diag-package-stream-recovery/spec.md):
//   FR-001 a rotate page keeps the status connected and resumes with no delay
//   FR-002 a workspace or filter change restarts the cursor
//   FR-003 a real disconnect backs off 2s→30s and the next page clears the notice
//   FR-004 403/404 ends in denied and asks for no further reconnect
//   FR-007 gap is sticky until the workspace changes or the user clears it
import {describe,it,expect} from "vitest";
import {readFileSync} from "node:fs";
import {computeBackoff,initialStreamState,nextStreamState,type StreamState} from "./stream-state";
import {pageSchema,parseDiagnostic} from "./contract";

function connected(over:Partial<StreamState>={}):StreamState{return {...initialStreamState,status:"connected",...over}}
function page(cursor:number,over:{gap?:boolean;rotate?:boolean}={}){return {type:"page",cursor,gap:over.gap??false,rotate:over.rotate??false} as const}

describe("diagnostic stream state machine",()=>{
 // FR-003
 it("backs off from two seconds to a thirty second ceiling",()=>{
  let ms=0;const seen:number[]=[];
  for(let i=0;i<6;i++){ms=computeBackoff(ms);seen.push(ms)}
  expect(seen).toEqual([2000,4000,8000,16000,30000,30000]);
  expect(computeBackoff(0)).toBe(2000);
 });
 // FR-001
 it("treats a rotate page as a planned handover, not a disconnect",()=>{
  const {state,reconnectInMs}=nextStreamState(connected({cursor:3}),page(9,{rotate:true}));
  expect(state.status).toBe("connected");
  expect(state.notice).toBe("");
  expect(state.cursor).toBe(9);
  expect(reconnectInMs).toBe(0);
  // The resumed attempt must not downgrade the status the user sees.
  expect(nextStreamState(state,{type:"connect"}).state.status).toBe("connected");
 });
 // FR-001 / FR-003
 it("treats an unmarked end as a real disconnect and backs off",()=>{
  const first=nextStreamState(connected({cursor:4}),{type:"end"});
  expect(first.state.status).toBe("disconnected");
  expect(first.state.notice).toContain("断开");
  expect(first.state.cursor).toBe(4);
  expect(first.reconnectInMs).toBe(2000);
  const attempt=nextStreamState(first.state,{type:"connect"});
  expect(attempt.state.status).toBe("reconnecting");
  const second=nextStreamState(attempt.state,{type:"error"});
  expect(second.state.status).toBe("disconnected");
  expect(second.reconnectInMs).toBe(4000);
 });
 // FR-003
 it("clears the disconnect notice and the backoff on the first page back",()=>{
  const down=nextStreamState(connected(),{type:"end"}).state;
  const open=nextStreamState(down,{type:"open"}).state;
  expect(open.status).toBe("connected");
  const back=nextStreamState(open,page(5));
  expect(back.state.notice).toBe("");
  expect(back.state.backoffMs).toBe(0);
  expect(back.reconnectInMs).toBeNull();
  expect(nextStreamState(back.state,{type:"end"}).reconnectInMs).toBe(2000);
 });
 // FR-004
 it("stops reconnecting after an authorization failure and shows the next action",()=>{
  for(const status of [403,404]){
   const {state,reconnectInMs}=nextStreamState(connected({cursor:2}),{type:"error",status,detail:"AUTHORIZATION_DENIED · check_authorization"});
   expect(state.status).toBe("denied");
   expect(state.notice).toContain("check_authorization");
   expect(reconnectInMs).toBeNull();
   // A denied stream stays denied; nothing reconnects on its own.
   expect(nextStreamState(state,{type:"connect"}).reconnectInMs).toBeNull();
   expect(nextStreamState(state,{type:"error"}).reconnectInMs).toBeNull();
   expect(nextStreamState(state,page(3)).state.status).toBe("denied");
  }
 });
 it("keeps the cursor across pause and resumes from it",()=>{
  const paused=nextStreamState(connected({cursor:17}),{type:"pause"});
  expect(paused.state.status).toBe("paused");
  expect(paused.state.cursor).toBe(17);
  expect(paused.reconnectInMs).toBeNull();
  const resumed=nextStreamState(paused.state,{type:"resume"});
  expect(resumed.state.status).toBe("connecting");
  expect(resumed.state.cursor).toBe(17);
  expect(resumed.reconnectInMs).toBe(0);
 });
 // FR-002 / FR-007
 it("restarts the cursor on a workspace change and on a filter change",()=>{
  const live=connected({cursor:12,gap:true,notice:"缺口",backoffMs:8000});
  const workspace=nextStreamState(live,{type:"workspace-change"});
  expect(workspace.state).toMatchObject({status:"connecting",cursor:0,gap:false,notice:"",backoffMs:0});
  expect(workspace.reconnectInMs).toBe(0);
  const filter=nextStreamState(live,{type:"filter-change"});
  expect(filter.state.cursor).toBe(0);
  expect(filter.state.status).toBe("connecting");
  // A filter change is not a new workspace, so a recorded gap survives it.
  expect(filter.state.gap).toBe(true);
 });
 // FR-007
 it("keeps a reported gap until the user clears it",()=>{
  const gapped=nextStreamState(connected(),page(4,{gap:true})).state;
  expect(gapped.gap).toBe(true);
  expect(gapped.notice).toContain("缺口");
  const later=nextStreamState(gapped,page(5)).state;
  expect(later.gap).toBe(true);
  expect(later.notice).toContain("缺口");
  const cleared=nextStreamState(later,{type:"clear-gap"}).state;
  expect(cleared.gap).toBe(false);
  expect(cleared.notice).toBe("");
 });
 it("does not reconnect while paused",()=>{
  const paused=nextStreamState(connected(),{type:"pause"}).state;
  expect(nextStreamState(paused,{type:"end"}).reconnectInMs).toBeNull();
  expect(nextStreamState(paused,{type:"connect"}).state.status).toBe("paused");
 });
});

// Feature 013 (specs/013-diag-stream-rotation), FR-007. The other half of this
// assertion lives in TestContentDiagnosticStreamLastPageMatchesTheSharedFixture:
// it runs a full window against the real handler and compares the last page on
// the wire to this same file. Go cannot call this state machine and vitest
// cannot call the handler, so the fixture is the only shared truth between
// them - and asserting against real bytes rather than an object written here is
// the point. A 50ms window once vouched for a path production never takes;
// two hand-written mocks would vouch for each other the same way.
describe("the real last page of a planned window",()=>{
 const FIXTURE="../../../../specs/013-diag-stream-rotation/contracts/rotation-last-page.json";
 const wire=JSON.parse(readFileSync(new URL(FIXTURE,import.meta.url),"utf8")) as unknown;

 it("parses as a page carrying rotate",()=>{
  const parsed=parseDiagnostic(wire,pageSchema);
  expect(parsed.rotate).toBe(true);
 });

 it("does not put the stream into a disconnected state",()=>{
  const parsed=parseDiagnostic(wire,pageSchema);
  const {state,reconnectInMs}=nextStreamState(connected({cursor:3}),
   {type:"page",cursor:parsed.cursor,gap:parsed.gap,rotate:parsed.rotate});
  // The user must not see a handover. "connected" is the whole requirement;
  // "disconnected" is what 002-V05-01 observed 18 times in 448 seconds.
  expect(state.status).toBe("connected");
  expect(state.notice).toBe("");
  expect(reconnectInMs).toBe(0);
 });

 it("would report a disconnect if the same page arrived without the marker",()=>{
  // The negative that gives the positive its meaning: losing rotate is exactly
  // what the window-boundary defect did to this page, and this is what the user
  // saw as a result.
  const {state,reconnectInMs}=nextStreamState(connected({cursor:3}),{type:"end"});
  expect(state.status).toBe("disconnected");
  expect(reconnectInMs).toBeGreaterThan(0);
 });
});

