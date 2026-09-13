// Package diagnostics owns the content audit, trace and simulation read model.
// Consumers inject its small interfaces; this package never imports business modules.
package diagnostics

import (
 "context"
 "crypto/rand"
 "encoding/hex"
 "encoding/json"
 "errors"
 "time"

 "go.opentelemetry.io/otel/propagation"
 "go.opentelemetry.io/otel/trace"
)

var ErrDenied = errors.New("AUTHORIZATION_DENIED")
var ErrUnavailable = errors.New("DATABASE_UNAVAILABLE")
var ErrConflict = errors.New("INPUT_CONFLICT")

type Scope struct { Workspace string; Actor string; Accounts []string }
func (s Scope) Allows(workspace, account string) bool {
 if s.Actor=="" || s.Workspace=="" || workspace!=s.Workspace { return false }
 if account=="" {return true}; for _,a:=range s.Accounts {if a==account {return true}}; return false
}

type Event struct {
 ID string `json:"event_id"`; Sequence int64 `json:"sequence"`
 Occurred time.Time `json:"occurred_at"`; Received time.Time `json:"received_at"`
 Workspace string `json:"workspace_id"`; Account string `json:"account_id"`
 ActorKind string `json:"actor_kind"`; Actor string `json:"actor_id"`
 ObjectType string `json:"object_type"`; ObjectID string `json:"object_id"`; Version string `json:"object_version"`
 Action string `json:"action"`; Outcome string `json:"outcome"`; Code string `json:"error_code"`
 Operation string `json:"operation_id"`; Trace string `json:"trace_id"`; Span string `json:"span_id"`; Parent string `json:"parent_span_id"`
 Run string `json:"run_id"`; Attempt int `json:"attempt"`; Step string `json:"step"`
 Component string `json:"component"`; Severity string `json:"severity"`; Duration int64 `json:"duration_ms"`
 Message string `json:"safe_message"`; Retryable bool `json:"retryable"`; Next string `json:"next_action"`
 Build string `json:"build"`; Test bool `json:"is_test"`
}
type Snapshot struct {
 ConfigVersion string `json:"config_version"`; PersonaRef string `json:"persona_ref"`
 SOPVersion string `json:"sop_version"`; SkillVersion string `json:"skill_version"`; RuleVersion string `json:"rule_version"`
 Executor string `json:"executor"`; ExecutorVersion string `json:"executor_version"`
 Scope string `json:"source_scope"`; Preference string `json:"saved_preference"`
 Required []string `json:"required_sources"`; Excluded []string `json:"excluded_sources"`; Grants []string `json:"grants"`
 Hashes map[string]string `json:"file_hashes"`; Temperature float64 `json:"temperature"`; Budget int `json:"budget"`; Timeout int `json:"timeout_ms"`
}
type Run struct {
 ID string `json:"run_id"`; Workspace string `json:"workspace_id"`; Account string `json:"account_id"`; Actor string `json:"actor_id"`
 Scenario string `json:"scenario"`; Seed int64 `json:"seed"`; Created time.Time `json:"created_at"`; Snapshot Snapshot `json:"snapshot"`
 Events []Event `json:"events"`; Gaps []string `json:"reproduction_gaps"`; Status string `json:"status"`
 Expected string `json:"expected_code"`; Actual string `json:"actual_code"`; Regression string `json:"regression"`
 Module string `json:"module"`; Build string `json:"build"`; Original string `json:"original_run_id"`; Test bool `json:"is_test"`
}
type Envelope struct { Operation string `json:"operation_id"`; Carrier map[string]string `json:"carrier"`; Attempt int `json:"attempt"`; Sequence int `json:"sequence"` }
func NewID() string { var b [16]byte; if _,err:=rand.Read(b[:]);err!=nil {panic(err)}; return hex.EncodeToString(b[:]) }
func Child(ctx context.Context) (context.Context,string) {
 parent:=trace.SpanContextFromContext(ctx); var tid trace.TraceID
 if parent.IsValid(){tid=parent.TraceID()}else{_,_=rand.Read(tid[:])}
 var sid trace.SpanID; _,_=rand.Read(sid[:])
 return trace.ContextWithSpanContext(ctx,trace.NewSpanContext(trace.SpanContextConfig{TraceID:tid,SpanID:sid,TraceFlags:trace.FlagsSampled})),parent.SpanID().String()
}
func Pack(ctx context.Context, operation string, attempt, sequence int) Envelope { c:=propagation.MapCarrier{}; propagation.TraceContext{}.Inject(ctx,c);return Envelope{operation,c,attempt,sequence} }
func Unpack(ctx context.Context,e Envelope) (context.Context,error) {
 if len(e.Operation)!=32 || e.Attempt<1 || e.Sequence<1 {return ctx,ErrConflict}; if _,err:=hex.DecodeString(e.Operation);err!=nil{return ctx,ErrConflict}
 ctx=propagation.TraceContext{}.Extract(ctx,propagation.MapCarrier(e.Carrier));if !trace.SpanContextFromContext(ctx).IsValid(){return ctx,ErrConflict};return ctx,nil
}
func CloneSnapshot(s Snapshot) Snapshot {b,_:=json.Marshal(s);var copy Snapshot;_ =json.Unmarshal(b,&copy);return copy}
func ReproductionGaps(s Snapshot,current map[string]string,grants map[string]bool) []string {
 gaps:=[]string{};for _,id:=range s.Grants{if !grants[id]{gaps=append(gaps,"AUTHORIZATION_REVOKED:"+id)}}
 for id,hash:=range s.Hashes{if current[id]==""{gaps=append(gaps,"FILE_MISSING:"+id)}else if current[id]!=hash{gaps=append(gaps,"FILE_CHANGED:"+id)}};return gaps
}
