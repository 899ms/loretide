package diagnostics

import("context";"encoding/json";"sort";"sync";"time")

type Component struct{Name string `json:"name"`;Status string `json:"status"`;LastSeen *time.Time `json:"last_seen"`;Version string `json:"version"`;Reason string `json:"reason"`}
type Metrics struct{Count int `json:"sample_count"`;Errors int `json:"errors"`;P50 int64 `json:"p50_ms"`;P95 *int64 `json:"p95_ms"`;Retries int `json:"retries"`;Cancelled int `json:"cancelled"`;Dropped int64 `json:"dropped"`;SinkErrors int64 `json:"sink_errors"`;QueueWait int64 `json:"queue_wait_ms"`}
type Overview struct{Components []Component `json:"components"`;Metrics Metrics `json:"metrics"`;Scenarios []Scenario `json:"scenarios"`;Instance string `json:"instance"`;Build string `json:"build"`;SimulationEnabled bool `json:"simulation_enabled"`;RetentionDays int `json:"retention_days"`;Capacity int `json:"capacity"`}
type Service struct{Store *Store;Build string;Enabled bool;mu sync.Mutex;heartbeats map[string]Component}
func NewService(store *Store,build string,enabled bool)*Service{return &Service{Store:store,Build:safeToken(build),Enabled:enabled,heartbeats:map[string]Component{}}}
func(s *Service)Heartbeat(name,version string,at time.Time){if oneOf(name,"web","daemon","executor","search","files")=="unknown"{return};s.mu.Lock();defer s.mu.Unlock();s.heartbeats[name]=Component{Name:name,Status:"healthy",LastSeen:&at,Version:safeToken(version)}}
func(s *Service)Overview(ctx context.Context,scope Scope)(Overview,error){
 if !scope.Allows(scope.Workspace,""){return Overview{},ErrDenied};now:=time.Now().UTC();o:=Overview{Components:[]Component{},Scenarios:Scenarios,Instance:"native-development",Build:s.Build,SimulationEnabled:s.Enabled,RetentionDays:int(s.Store.Retention.Hours()/24),Capacity:s.Store.MaxLogs}
 dbctx,cancel:=context.WithTimeout(ctx,time.Second);defer cancel();dbState:="healthy";if s.Store.Check(dbctx)!=nil{dbState="unavailable"}
 o.Components=append(o.Components,Component{"api","healthy",&now,s.Build,""},Component{"database",dbState,&now,"PostgreSQL","connectivity only"})
 s.mu.Lock();for _,name:=range []string{"web","files","daemon","executor","search"}{c,ok:=s.heartbeats[name];if !ok{c=Component{Name:name,Status:"unverified",Reason:"No real component heartbeat; simulation is separate"}}else if c.LastSeen.After(now.Add(5*time.Second)){c.Status="unknown";c.Reason="clock skew"}else if now.Sub(*c.LastSeen)>30*time.Second{c.Status="unavailable";c.Reason="heartbeat expired"};o.Components=append(o.Components,c)};s.mu.Unlock()
 p,err:=s.Store.Query(ctx,scope,Filter{Limit:100});if err!=nil&&dbState=="healthy"{return o,err};durations:=[]int64{};for _,e:=range p.Events{durations=append(durations,e.Duration);if e.Outcome=="failed"{o.Metrics.Errors++};if e.Attempt>1{o.Metrics.Retries++};if e.Code=="CANCELLED"{o.Metrics.Cancelled++};if e.Component=="queue"{o.Metrics.QueueWait+=e.Duration}}
 sort.Slice(durations,func(i,j int)bool{return durations[i]<durations[j]});o.Metrics.Count=len(durations);if len(durations)>0{o.Metrics.P50=durations[len(durations)/2]};if len(durations)>=20{v:=durations[(len(durations)-1)*95/100];o.Metrics.P95=&v};o.Metrics.Dropped=s.Store.Log.Dropped.Load();o.Metrics.SinkErrors=s.Store.Log.Errors.Load();return o,nil
}
func(s *Service)Run(ctx context.Context,scope Scope,account,scenario string,seed int64,original string)(Run,error){
 priorOperation:="";priorAttempt:=0
 if original!=""{prior,err:=s.Store.GetRun(ctx,scope,original);if err!=nil{return Run{},err};if len(prior.Events)>0{priorOperation=prior.Events[0].Operation;for _,e:=range prior.Events{if e.Attempt>priorAttempt{priorAttempt=e.Attempt}}}}
 run,err:=Simulate(ctx,scope,account,scenario,seed,s.Build,s.Enabled);if err!=nil{return run,err};run.Original=original
 if priorOperation!=""{for i:=range run.Events{run.Events[i].Operation=priorOperation;run.Events[i].Attempt+=priorAttempt}}
 if scenario=="database"{err=s.Store.CommitRun(ctx,scope,run,true);if err==nil{return run,ErrConflict};run.Actual="DATABASE_UNAVAILABLE";run.Status="failed";e:=run.Events[len(run.Events)-1];e.Code=run.Actual;e.Outcome="failed";e.Severity="error";run.Events[len(run.Events)-1]=Sanitize(e)}
 // A self-check shape may ask to be left unevaluated, so that "not_run"
 // reaches the database instead of living for one statement in memory
 // (specs/012, 006-V-1), or to have its sink writes fail, which is what
 // moves sink_errors and dropped. Both are carried by the scenario id and
 // so are already behind Simulate's isolation gate.
 sh,isShape:=shapeByID(run.Scenario)
 if !isShape||!sh.SkipEvaluate{Evaluate(&run)}
 if err=s.Store.CommitRun(ctx,scope,run,false);err!=nil{return run,err}
 for _,e:=range run.Events{if isShape&&sh.FailSink{s.Store.TechnicalFailingSink(ctx,e)}else{s.Store.Technical(ctx,e)}}
 if err=s.Store.PruneTechnical(ctx);err!=nil{s.Store.Log.Errors.Add(1)};return run,nil
}
type Export struct{Manifest []string `json:"manifest"`;Run Run `json:"run"`;Audit Page `json:"audit"`;Technical Page `json:"technical"`;Redacted bool `json:"redacted"`;Limits []string `json:"limits"`}
func(s *Service)Export(ctx context.Context,scope Scope,id string,download bool)(Export,error){
 r,err:=s.Store.GetRun(ctx,scope,id);if err!=nil{return Export{},err};audit,err:=s.Store.Query(ctx,scope,Filter{Kind:"audit",Run:id,Limit:100});if err!=nil{return Export{},err};tech,err:=s.Store.Query(ctx,scope,Filter{Run:id,Limit:100});if err!=nil{return Export{},err}
 bundle:=Export{[]string{"manifest","run-summary","authorized-snapshot-references","audit-events","technical-trace","reproduction-gaps"},r,audit,tech,true,[]string{"No source text, credentials, media copies or provider stdout","Technical events may have expired; no automatic upload","Real executor replay is unavailable"}}
 if download{e:=Event{ID:NewID(),Workspace:scope.Workspace,Account:r.Account,Actor:scope.Actor,ActorKind:"human",ObjectType:"diagnostics",ObjectID:r.ID,Action:"export",Outcome:"success",Run:r.ID,Occurred:time.Now().UTC(),Component:"diagnostics",Severity:"info",Test:true};if err=s.Store.Audit(ctx,scope,e);err!=nil{return Export{},err}}
 return bundle,nil
}
func(s *Service)ClientError(ctx context.Context,scope Scope,code string)(Event,error){if !scope.Allows(scope.Workspace,""){return Event{},ErrDenied};if code!="UI_ERROR"&&code!="NETWORK_UNAVAILABLE"{code="UI_ERROR"};e:=Sanitize(Event{ID:NewID(),Workspace:scope.Workspace,Actor:scope.Actor,ActorKind:"human",ObjectType:"diagnostics",Action:"client_error",Outcome:"failed",Code:code,Trace:NewID(),Operation:NewID(),Occurred:time.Now().UTC(),Received:time.Now().UTC(),Component:"web",Severity:"error",Build:s.Build});s.Store.Technical(ctx,e);return e,nil}
func EncodeSafe(v any)([]byte,error){return json.Marshal(v)}
