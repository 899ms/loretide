import {useCallback,useEffect,useRef,useState} from "react";
import {useMutation,useQuery,useQueryClient} from "@tanstack/react-query";
import {ApiError,api} from "@multica/core/api";
import {describeDiagnosticError,eventSchema,exportSchema,mergeEvents,overviewSchema,pageSchema,parseContentDispositionFilename,parseDiagnostic,runSchema,runsSchema,streamQuery,type DiagnosticEvent,type DiagnosticFilter} from "./contract";
import {initialStreamState,nextStreamState,type StreamEvent,type StreamState} from "./stream-state";

// What a finished download hands the platform layer. The bytes are the
// server's own response body, so the saved file matches the export endpoint
// exactly; the name is the one the server asked for.
export type DiagnosticDownload={blob:Blob;filename:string};
const DOWNLOAD_FALLBACK_FILENAME="loretide-diagnostics.json";
const MAX_STREAM_LINE_BYTES=1024*1024;

export function useDiagnostics(wsId:string,runId:string,filter:string){
 const client=useQueryClient();const base=["contentDiagnostics",wsId];
 const overview=useQuery({queryKey:[...base,"overview"],queryFn:async()=>parseDiagnostic(await api.contentDiagnosticRequest("overview"),overviewSchema),refetchInterval:10000});
 const runs=useQuery({queryKey:[...base,"runs"],queryFn:async()=>parseDiagnostic(await api.contentDiagnosticRequest("runs"),runsSchema)});
 const detail=useQuery({queryKey:[...base,"run",runId],enabled:!!runId,queryFn:async()=>parseDiagnostic(await api.contentDiagnosticRequest("runs",new URLSearchParams({run_id:runId}).toString()),runSchema)});
 const events=useQuery({queryKey:[...base,"events",filter],queryFn:async()=>parseDiagnostic(await api.contentDiagnosticRequest("events",filter),pageSchema)});
 const simulate=useMutation({mutationFn:async(p:{scenario:string;seed:number;originalRunId?:string})=>parseDiagnostic(await api.contentDiagnosticRequest("simulate","",{scenario:p.scenario,seed:p.seed,original_run_id:p.originalRunId??""}),runSchema),onSuccess:()=>client.invalidateQueries({queryKey:base})});
 const preview=useQuery({queryKey:[...base,"export",runId],enabled:false,queryFn:async()=>parseDiagnostic(await api.contentDiagnosticRequest("export",new URLSearchParams({run_id:runId}).toString()),exportSchema)});
 // The download keeps the server's bytes rather than re-serializing a parsed
 // object: the bundle is read next to the server's logs, and a client-side
 // schema transform would rewrite every key. The preview above still parses.
 const download=useMutation<DiagnosticDownload>({mutationFn:async()=>{
  const response=await api.contentDiagnosticDownload(new URLSearchParams({run_id:runId}).toString());
  return{blob:await response.blob(),filename:parseContentDispositionFilename(response.headers.get("content-disposition"))??DOWNLOAD_FALLBACK_FILENAME};
 }});
 useEffect(()=>{const beat=()=>void api.contentDiagnosticRequest("client","",{heartbeat:true}).catch(()=>{});beat();const timer=setInterval(beat,10000);return()=>clearInterval(timer)},[wsId]);
 return{overview,runs,detail,events,simulate,preview,download};
}
// Drives the live NDJSON stream. The transitions live in stream-state.ts as a
// pure function; everything here is the side effects that function asks for:
// the fetch, the reconnect timer and the tab-visibility catch-up.
export function useDiagnosticStream(wsId:string,enabled:boolean,filter?:DiagnosticFilter){
 const client=useQueryClient();
 const stateRef=useRef<StreamState>(initialStreamState);
 const[state,setState]=useState<StreamState>(initialStreamState);
 // The filter is a fresh object on every render, so the effect keys off its
 // serialized form and reads the current value through a ref.
 const filterKey=streamQuery(filter,0);
 const filterRef=useRef(filter);filterRef.current=filter;
 const seenWsId=useRef(wsId);const seenFilterKey=useRef(filterKey);
 const query=useQuery<DiagnosticEvent[]>({queryKey:["contentDiagnostics",wsId,"stream"],queryFn:async()=>[],enabled:false,initialData:[]});
 const dispatch=useCallback((event:StreamEvent)=>{const transition=nextStreamState(stateRef.current,event);stateRef.current=transition.state;setState(transition.state);return transition},[]);
 const clearGap=useCallback(()=>{dispatch({type:"clear-gap"})},[dispatch]);
 useEffect(()=>{
  if(seenWsId.current!==wsId){seenWsId.current=wsId;seenFilterKey.current=filterKey;dispatch({type:"workspace-change"})}
  else if(seenFilterKey.current!==filterKey){seenFilterKey.current=filterKey;dispatch({type:"filter-change"})}
  if(!enabled){dispatch({type:"pause"});return}
  dispatch({type:"resume"});
  let stopped=false;let timer:ReturnType<typeof setTimeout>|undefined;let current:AbortController|undefined;
  const schedule=(delayMs:number|null)=>{if(stopped||delayMs===null)return;if(timer)clearTimeout(timer);timer=setTimeout(()=>void connect(),delayMs)};
  const connect=async()=>{
   if(stopped)return;
   const controller=new AbortController();current=controller;
   const live=()=>!stopped&&!controller.signal.aborted;
   dispatch({type:"connect"});
   try{
    const response=await api.contentDiagnosticStream(streamQuery(filterRef.current,stateRef.current.cursor),controller.signal);
    const reader=response.body?.getReader();if(!reader)throw new Error("stream unavailable");
    if(!live())return;
    dispatch({type:"open"});
    const decoder=new TextDecoder();let pending="";
    for(;;){
     if(!live())return;
     const chunk=await reader.read();if(chunk.done)break;
     pending+=decoder.decode(chunk.value,{stream:true});
     if(pending.length>MAX_STREAM_LINE_BYTES)throw new Error("oversized stream");
     let rotated=false;let split:number;
     while((split=pending.indexOf("\n"))>=0){
      const line=pending.slice(0,split);pending=pending.slice(split+1);
      if(!line)continue;
      const page=parseDiagnostic(JSON.parse(line),pageSchema);
      client.setQueryData<DiagnosticEvent[]>(["contentDiagnostics",wsId,"stream"],old=>mergeEvents(old??[],page.events));
      const transition=dispatch({type:"page",cursor:page.cursor,gap:page.gap,rotate:page.rotate});
      // The planned window closed. Drop this connection and resume from the
      // cursor at once, without changing what the user is looking at.
      if(page.rotate){rotated=true;controller.abort();schedule(transition.reconnectInMs);break}
     }
     if(rotated)return;
    }
    if(!live())return;
    schedule(dispatch({type:"end"}).reconnectInMs);
   }catch(error){
    if(!live())return;
    const status=error instanceof ApiError?error.status:undefined;
    schedule(dispatch({type:"error",status,detail:describeDiagnosticError(error).message}).reconnectInMs);
   }
  };
  // A backgrounded tab has its timers throttled, so a stream that dropped
  // while hidden would stay dropped well past its backoff. Catch up on return.
  const onVisibilityChange=()=>{if(document.visibilityState==="visible"&&stateRef.current.status==="disconnected")schedule(0)};
  if(typeof document!=="undefined")document.addEventListener("visibilitychange",onVisibilityChange);
  void connect();
  return()=>{stopped=true;current?.abort();if(timer)clearTimeout(timer);if(typeof document!=="undefined")document.removeEventListener("visibilitychange",onVisibilityChange)};
 },[enabled,wsId,filterKey,client,dispatch]);
 return{events:query.data,status:state.status,gap:state.gap,notice:state.notice,clearGap};
}
export async function recordDiagnosticClientError(){return parseDiagnostic(await api.contentDiagnosticRequest("client","",{code:"UI_ERROR"}),eventSchema)}
