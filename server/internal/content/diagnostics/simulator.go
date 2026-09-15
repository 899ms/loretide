package diagnostics

import("context";"encoding/json";"fmt";"math/rand/v2";"time";"go.opentelemetry.io/otel/trace")

// Kind classifies a scenario: "fault" is a business failure drawn from docs/13
// section 7, "shape" is a diagnostics self-check data shape that normal
// operation never produces. Read the field; never parse the id prefix.
// contracts/scenario-kind.md (specs/012).
type Scenario struct{ID string `json:"id"`;Expected string `json:"expected_code"`;Kind string `json:"kind"`}
var Scenarios=[]Scenario{{"normal","","fault"},{"slow","","fault"},{"timeout","TIMEOUT","fault"},{"cancel","CANCELLED","fault"},{"reconnect","NETWORK_UNAVAILABLE","fault"},{"duplicate","DUPLICATE","fault"},{"late","LATE_RESULT","fault"},{"file_missing","FILE_MISSING","fault"},{"file_changed","FILE_CHANGED","fault"},{"denied","AUTHORIZATION_DENIED","fault"},{"database","DATABASE_UNAVAILABLE","fault"},{"model_auth","MODEL_AUTH","fault"},{"model_quota","MODEL_QUOTA","fault"},{"schema","OUTPUT_SCHEMA","fault"},{"search","SEARCH_FAILED","fault"},{"clock_skew","CLOCK_SKEW","fault"},{"shape_concurrent","","shape"},{"shape_orphan","","shape"},{"shape_single_span","","shape"},{"shape_deep","TIMEOUT","shape"},{"shape_not_run","","shape"},{"shape_regression_failed","TIMEOUT","shape"},{"shape_undecidable","","shape"},{"shape_no_module","","shape"},{"shape_sink_failure","","shape"}}
type Receiver struct{seen map[int]bool;last int;Cancelled bool}
func(r *Receiver)Receive(seq int)string{if r.Cancelled{return "LATE_RESULT"};if r.seen==nil{r.seen=map[int]bool{}};if r.seen[seq]{return "DUPLICATE"};if seq<=r.last{return "LATE_RESULT"};r.seen[seq]=true;r.last=seq;return ""}
// Simulate only constructs test-owned data. The caller persists it atomically.
// Virtual time makes failures reproducible without consuming provider quota.
func Simulate(ctx context.Context,scope Scope,account,scenario string,seed int64,build string,testEnabled bool)(Run,error){
 if !testEnabled || !scope.Allows(scope.Workspace,account){return Run{},ErrDenied}
 expected:="";found:=false;for _,s:=range Scenarios{if s.ID==scenario{expected=s.Expected;found=true}};if !found{return Run{},ErrConflict}
 // Self-check data shapes branch out here, above the step loop. Everything
 // below this line is the business fault path and is unchanged by specs/012.
 if sh,ok:=shapeByID(scenario);ok{return simulateShape(ctx,scope,account,sh,expected,seed,build)}
 snapshot:=Snapshot{ConfigVersion:"fixture-v1",PersonaRef:"fixture:persona",SOPVersion:"fixture-v1",SkillVersion:"fixture-v1",RuleVersion:"fixture-v1",Executor:"simulator",ExecutorVersion:"1",Scope:"all",Preference:"all",Required:[]string{"fixture-a"},Excluded:[]string{},Grants:[]string{"fixture-a"},Hashes:map[string]string{"fixture-a":"sha256:fixture-v1"},Temperature:0,Budget:0,Timeout:30000}
 now:=time.Date(2026,1,1,0,0,0,0,time.UTC);run:=Run{ID:NewID(),Workspace:scope.Workspace,Account:account,Actor:scope.Actor,Scenario:scenario,Seed:seed,Created:time.Now().UTC(),Snapshot:CloneSnapshot(snapshot),Events:[]Event{},Gaps:[]string{},Status:"completed",Expected:expected,Regression:"not_run",Module:"diagnostics",Build:safeToken(build),Test:true}
 rng:=rand.New(rand.NewPCG(uint64(seed),42));operation:=NewID();ctx,_=Child(ctx)
 receiver:=Receiver{};attempt:=1;code:="";sequence:=0
 steps:=[]string{"web","api","database","queue","daemon","executor","tool","result","database"}
 prevStart:=now
 for i,component:=range steps{
  if err:=ctx.Err();err!=nil{code="CANCELLED"}
  child,parent:=Child(ctx);if i==0{parent=""};envelope:=Pack(child,operation,attempt,i+1);wire,_:=json.Marshal(envelope);transport,_,err:=DecodeQueuedEnvelope(context.Background(),wire);if err!=nil{return Run{},err};ctx=transport
  duration:=int64(1+rng.IntN(20));stepCode:="";skewed:=false
  switch scenario{
  case "slow":if component=="executor"{duration=5000}
  case "timeout":if component=="executor"{deadline,cancel:=context.WithDeadline(ctx,time.Unix(0,0));if deadline.Err()!=nil{stepCode="TIMEOUT";duration=int64(snapshot.Timeout+1)};cancel()}
  case "cancel":if component=="executor"{cancelled,cancel:=context.WithCancel(ctx);cancel();if cancelled.Err()!=nil{stepCode="CANCELLED"};receiver.Cancelled=true}
  case "reconnect":if component=="daemon"{stepCode="NETWORK_UNAVAILABLE";attempt++}
  case "duplicate":if component=="result"{_ =receiver.Receive(i+1);stepCode=receiver.Receive(i+1)}
  case "late":if component=="result"{receiver.Cancelled=true;stepCode=receiver.Receive(i+1)}
  case "file_missing","file_changed":if component=="tool"{current:=map[string]string{};if scenario=="file_changed"{current["fixture-a"]="changed"};run.Gaps=ReproductionGaps(snapshot,current,map[string]bool{"fixture-a":true});if len(run.Gaps)>0{if scenario=="file_missing"{stepCode="FILE_MISSING"}else{stepCode="FILE_CHANGED"}}}
  case "denied":if component=="tool"{run.Gaps=ReproductionGaps(snapshot,snapshot.Hashes,map[string]bool{});if len(run.Gaps)>0{stepCode="AUTHORIZATION_DENIED"}}
  case "model_auth":if component=="executor"{stepCode=ProviderCode(401)}
  case "model_quota":if component=="executor"{stepCode=ProviderCode(429)}
  case "schema":if component=="result"{var output struct{Text string `json:"text"`};if json.Unmarshal([]byte(`{"text":42}`),&output)!=nil{stepCode="OUTPUT_SCHEMA"}}
  case "search":if component=="tool"{stepCode="SEARCH_FAILED";run.Snapshot.Scope="local"}
  case "clock_skew":if component=="daemon"{stepCode="CLOCK_SKEW";run.Gaps=append(run.Gaps,"REMOTE_CLOCK_SKEW");skewed=true}
  }
  if stepCode!=""{code=stepCode};sequence++
  // occurred_at is the START of this step; the clock advances afterwards.
  // contracts/span-timing.md (specs/011). A clock-skewed step is reported
  // as starting before its parent - that is the fault being simulated, so
  // it is never corrected here and never clamped downstream (006 FR-014).
  start:=now
  if skewed{start=prevStart.Add(-time.Duration(1+duration)*time.Millisecond)}
  now=now.Add(time.Duration(duration)*time.Millisecond);prevStart=start
  e:=Event{ID:NewID(),Sequence:int64(sequence),Occurred:start,Received:run.Created,Workspace:scope.Workspace,Account:account,Actor:scope.Actor,ActorKind:"human",ObjectType:"simulation",ObjectID:run.ID,Version:"1",Action:"simulate",Outcome:"success",Code:stepCode,Operation:operation,Trace:trace.SpanContextFromContext(child).TraceID().String(),Span:trace.SpanContextFromContext(child).SpanID().String(),Parent:parent,Run:run.ID,Attempt:attempt,Step:fmt.Sprintf("%02d-%s",i,component),Component:component,Severity:"info",Duration:duration,Build:run.Build,Test:true}
  if i>1{e.ActorKind="system"};if component=="executor"{e.ActorKind="agent"}
  if stepCode!=""{e.Outcome="failed";e.Severity="error"};if stepCode=="CANCELLED"{e.Outcome="cancelled"};if stepCode=="DUPLICATE"||stepCode=="LATE_RESULT"{e.Outcome="ignored"};run.Events=append(run.Events,Sanitize(e))
  if stepCode!="" && stepCode!="NETWORK_UNAVAILABLE" && stepCode!="CLOCK_SKEW" && stepCode!="DUPLICATE" {break}
 }
 run.Actual=code;if code!=""{run.Status="failed"};return run,nil
}
func ProviderCode(status int)string{switch status{case 401,403:return "MODEL_AUTH";case 429:return "MODEL_QUOTA";default:return "INTERNAL"}}
func Evaluate(run *Run){run.Regression="failed";if run.Actual==run.Expected{run.Regression="passed"}}
