import {useEffect,useRef,useState} from "react";
import {useMutation,useQuery,useQueryClient} from "@tanstack/react-query";
import {api} from "@multica/core/api";
import {eventSchema,exportSchema,mergeEvents,overviewSchema,pageSchema,parseDiagnostic,runSchema,runsSchema,type DiagnosticEvent} from "./contract";

export function useDiagnostics(wsId:string,runId:string,filter:string){
 const client=useQueryClient();const base=["contentDiagnostics",wsId];
 const overview=useQuery({queryKey:[...base,"overview"],queryFn:async()=>parseDiagnostic(await api.contentDiagnosticRequest("overview"),overviewSchema),refetchInterval:10000});
 const runs=useQuery({queryKey:[...base,"runs"],queryFn:async()=>parseDiagnostic(await api.contentDiagnosticRequest("runs"),runsSchema)});
 const detail=useQuery({queryKey:[...base,"run",runId],enabled:!!runId,queryFn:async()=>parseDiagnostic(await api.contentDiagnosticRequest("runs",new URLSearchParams({run_id:runId}).toString()),runSchema)});
 const events=useQuery({queryKey:[...base,"events",filter],queryFn:async()=>parseDiagnostic(await api.contentDiagnosticRequest("events",filter),pageSchema)});
 const simulate=useMutation({mutationFn:async(p:{scenario:string;seed:number;originalRunId?:string})=>parseDiagnostic(await api.contentDiagnosticRequest("simulate","",{scenario:p.scenario,seed:p.seed,original_run_id:p.originalRunId??""}),runSchema),onSuccess:()=>client.invalidateQueries({queryKey:base})});
 const preview=useQuery({queryKey:[...base,"export",runId],enabled:false,queryFn:async()=>parseDiagnostic(await api.contentDiagnosticRequest("export",new URLSearchParams({run_id:runId}).toString()),exportSchema)});
 const download=useMutation({mutationFn:async()=>parseDiagnostic(await api.contentDiagnosticRequest("export",new URLSearchParams({run_id:runId}).toString(),{}),exportSchema)});
 useEffect(()=>{const beat=()=>void api.contentDiagnosticRequest("client","",{heartbeat:true}).catch(()=>{});beat();const timer=setInterval(beat,10000);return()=>clearInterval(timer)},[wsId]);
 return{overview,runs,detail,events,simulate,preview,download};
}
export function useDiagnosticStream(wsId:string,enabled:boolean){
 const client=useQueryClient();const cursor=useRef(0);const [status,setStatus]=useState("paused");const[gap,setGap]=useState(false);const[notice,setNotice]=useState("");
 const key=["contentDiagnostics",wsId,"stream"];
 const query=useQuery<DiagnosticEvent[]>({queryKey:key,queryFn:async()=>[],enabled:false,initialData:[]});
 useEffect(()=>{cursor.current=0;setGap(false);setNotice("")},[wsId]);
 useEffect(()=>{
  if(!enabled){setStatus("paused");return};const controller=new AbortController();let timer:ReturnType<typeof setTimeout>|undefined;let stopped=false;
  const active=()=>!stopped&&!controller.signal.aborted;
  const reconnect=()=>{if(active())timer=setTimeout(()=>void connect(),2000)};
  const connect=async()=>{if(!active())return;setStatus(cursor.current>0?"reconnecting":"connecting");try{
   const response=await api.contentDiagnosticStream(`after=${cursor.current}`,controller.signal);const reader=response.body?.getReader();if(!reader)throw new Error("stream unavailable");if(!active())return;setStatus("connected");setNotice("");const decoder=new TextDecoder();let pending="";
   while(active()){const chunk=await reader.read();if(chunk.done)break;pending+=decoder.decode(chunk.value,{stream:true});if(pending.length>1024*1024)throw new Error("oversized stream");let split:number;while((split=pending.indexOf("\n"))>=0){const line=pending.slice(0,split);pending=pending.slice(split+1);if(!line)continue;const page=parseDiagnostic(JSON.parse(line),pageSchema);if(page.gap){setGap(true);setNotice("实时流存在保留期缺口；已保留可读取的后续记录。")}cursor.current=page.cursor;client.setQueryData<DiagnosticEvent[]>(["contentDiagnostics",wsId,"stream"],old=>mergeEvents(old??[],page.events));}}
   if(active()){setStatus("disconnected");setNotice("实时流已断开，正在从上次游标重新连接。");}
  }catch{if(active()){setStatus("disconnected");setNotice("实时流连接失败，正在从上次游标重新连接。")}}finally{reconnect()}};
  void connect();return()=>{stopped=true;controller.abort();if(timer)clearTimeout(timer)};
 },[enabled,wsId,client]);return{events:query.data,status,gap,notice};
}
export async function recordDiagnosticClientError(){return parseDiagnostic(await api.contentDiagnosticRequest("client","",{code:"UI_ERROR"}),eventSchema)}
