import {z} from "zod";
import {parseWithFallback} from "@multica/core/api/schema";
import {ApiError} from "@multica/core/api";

export const eventSchema = z.object({
  event_id:z.string(), sequence:z.number(), occurred_at:z.string(), received_at:z.string(), actor_kind:z.string(), actor_id:z.string(), workspace_id:z.string(), account_id:z.string(), object_type:z.string(), object_id:z.string(), object_version:z.string(), action:z.string(), outcome:z.string(), error_code:z.string(), operation_id:z.string(), trace_id:z.string(), span_id:z.string(), parent_span_id:z.string(), run_id:z.string(), attempt:z.number(), step:z.string(), component:z.string(), severity:z.string(), duration_ms:z.number(), safe_message:z.string(), retryable:z.boolean(), next_action:z.string(), build:z.string(),is_test:z.boolean(),
  // Feature 005 request identity. Optional so an older server, which does not
  // send them, still parses.
  route:z.string().optional().default(""), status:z.number().optional().default(0), headers_present:z.array(z.string()).optional().default([]), upstream_trace:z.string().optional().default(""),
}).transform(e=>({eventId:e.event_id,sequence:e.sequence,occurredAt:e.occurred_at,receivedAt:e.received_at,actorKind:e.actor_kind,actorId:e.actor_id,workspaceId:e.workspace_id,accountId:e.account_id,objectType:e.object_type,objectId:e.object_id,objectVersion:e.object_version,action:e.action,outcome:e.outcome,errorCode:e.error_code,operationId:e.operation_id,traceId:e.trace_id,spanId:e.span_id,parentSpanId:e.parent_span_id,runId:e.run_id,attempt:e.attempt,step:e.step,component:e.component,severity:e.severity,durationMs:e.duration_ms,safeMessage:e.safe_message,retryable:e.retryable,nextAction:e.next_action,build:e.build,isTest:e.is_test,route:e.route,status:e.status,headersPresent:e.headers_present,upstreamTrace:e.upstream_trace}));
export type DiagnosticEvent=z.output<typeof eventSchema>;
export const pageSchema=z.object({events:z.array(eventSchema),cursor:z.number().nonnegative(),gap:z.boolean(),has_more:z.boolean(),rotate:z.boolean().optional().default(false)}).transform(p=>({events:p.events,cursor:p.cursor,gap:p.gap,hasMore:p.has_more,rotate:p.rotate}));
export type DiagnosticPage=z.output<typeof pageSchema>;
export const snapshotSchema=z.object({config_version:z.string(),persona_ref:z.string(),sop_version:z.string(),skill_version:z.string(),rule_version:z.string(),executor:z.string(),executor_version:z.string(),source_scope:z.string(),saved_preference:z.string(),required_sources:z.array(z.string()),excluded_sources:z.array(z.string()),grants:z.array(z.string()),file_hashes:z.record(z.string(),z.string()),temperature:z.number(),budget:z.number(),timeout_ms:z.number()}).transform(s=>({configVersion:s.config_version,personaRef:s.persona_ref,sopVersion:s.sop_version,skillVersion:s.skill_version,ruleVersion:s.rule_version,executor:s.executor,executorVersion:s.executor_version,sourceScope:s.source_scope,savedPreference:s.saved_preference,requiredSources:s.required_sources,excludedSources:s.excluded_sources,grants:s.grants,fileHashes:s.file_hashes,temperature:s.temperature,budget:s.budget,timeoutMs:s.timeout_ms}));
export const runSchema=z.object({run_id:z.string(),workspace_id:z.string(),account_id:z.string(),actor_id:z.string(),scenario:z.string(),seed:z.number(),created_at:z.string(),snapshot:snapshotSchema,events:z.array(eventSchema).nullable(),reproduction_gaps:z.array(z.string()),status:z.string(),expected_code:z.string(),actual_code:z.string(),regression:z.string(),module:z.string(),build:z.string(),original_run_id:z.string(),is_test:z.boolean()}).transform(r=>({runId:r.run_id,workspaceId:r.workspace_id,accountId:r.account_id,actorId:r.actor_id,scenario:r.scenario,seed:r.seed,createdAt:r.created_at,snapshot:r.snapshot,events:r.events??[],reproductionGaps:r.reproduction_gaps,status:r.status,expectedCode:r.expected_code,actualCode:r.actual_code,regression:r.regression,module:r.module,build:r.build,originalRunId:r.original_run_id,isTest:r.is_test}));
export type DiagnosticRun=z.output<typeof runSchema>;
export const runsSchema=z.object({runs:z.array(runSchema)});
export const overviewSchema=z.object({components:z.array(z.object({name:z.string(),status:z.string(),last_seen:z.string().nullable(),version:z.string(),reason:z.string()}).transform(c=>({name:c.name,status:c.status,lastSeen:c.last_seen,version:c.version,reason:c.reason}))),metrics:z.object({sample_count:z.number(),errors:z.number(),p50_ms:z.number(),p95_ms:z.number().nullable(),retries:z.number(),cancelled:z.number(),dropped:z.number(),sink_errors:z.number(),queue_wait_ms:z.number()}).transform(m=>({sampleCount:m.sample_count,errors:m.errors,p50Ms:m.p50_ms,p95Ms:m.p95_ms,retries:m.retries,cancelled:m.cancelled,dropped:m.dropped,sinkErrors:m.sink_errors,queueWaitMs:m.queue_wait_ms})),scenarios:z.array(z.object({id:z.string(),expected_code:z.string()}).transform(s=>({id:s.id,expectedCode:s.expected_code}))),instance:z.string(),build:z.string(),simulation_enabled:z.boolean(),retention_days:z.number(),capacity:z.number()}).transform(o=>({...o,simulationEnabled:o.simulation_enabled,retentionDays:o.retention_days}));
export const exportSchema=z.object({manifest:z.array(z.string()),run:runSchema,audit:pageSchema,technical:pageSchema,redacted:z.boolean(),limits:z.array(z.string())});
export function parseDiagnostic<S extends z.ZodType>(raw:unknown,schema:S):z.output<S>{
 const parsed=parseWithFallback<z.output<S>|null>(raw,schema,null,{endpoint:"content-diagnostics"});
 if(parsed===null)throw new Error("OUTPUT_SCHEMA: diagnostic response is invalid");return parsed;
}
// The live stream keeps a bounded window so a long session cannot grow without
// limit; the page tells the user when the window is what they are looking at.
export const STREAM_EVENT_CAP=200;
export function mergeEvents(old:DiagnosticEvent[],incoming:DiagnosticEvent[]):DiagnosticEvent[]{return [...new Map([...old,...incoming].map(e=>[e.eventId,e])).values()].sort((a,b)=>a.sequence-b.sequence).slice(-STREAM_EVENT_CAP)}
export type DiagnosticFilter={kind?:string;traceId?:string;runId?:string;component?:string;errorCode?:string;severity?:string;from?:string;until?:string};
// Serializes the stream request the same way for the first connection and for
// every resume, so a reconnect cannot silently widen what the user asked for.
// Keys match the server-side parser in handler.diagnosticFilter.
export function streamQuery(filter:DiagnosticFilter|undefined,after:number):string{
 const params=new URLSearchParams();
 const pairs:[string,string|undefined][]=[["kind",filter?.kind],["trace_id",filter?.traceId],["run_id",filter?.runId],["component",filter?.component],["error_code",filter?.errorCode],["severity",filter?.severity],["from",filter?.from],["until",filter?.until]];
 for(const [key,value] of pairs)if(value)params.set(key,value);
 params.set("after",String(after));
 return params.toString();
}
// Reads the name the server chose for the bundle. The value reaches a download
// attribute, so any directory part is dropped rather than trusted.
export function parseContentDispositionFilename(header:string|null):string|null{
 if(!header)return null;
 const match=/filename\s*=\s*(?:"([^"]*)"|([^;]*))/i.exec(header);
 if(!match)return null;
 const name=(match[1]??match[2]??"").trim().split(/[\\/]/).pop()?.trim()??"";
 return name?name:null;
}
export function describeDiagnosticError(error:unknown):{message:string;traceId:string}{
 const schema=z.object({code:z.string().regex(/^[A-Z_]{1,64}$/),trace_id:z.string().regex(/^[a-f0-9]{32}$/).optional(),next_action:z.string().regex(/^[a-z_]{1,64}$/).optional()});
 if(error instanceof ApiError){const parsed=schema.safeParse(error.body);if(parsed.success)return{message:`${parsed.data.code} · ${parsed.data.next_action??"retry_query"}`,traceId:parsed.data.trace_id??""};return{message:`HTTP ${error.status} · retry_query`,traceId:""}}
 return{message:error instanceof Error&&error.message.startsWith("OUTPUT_SCHEMA")?"OUTPUT_SCHEMA · retry_query":"NETWORK_UNAVAILABLE · retry_query",traceId:""};
}
