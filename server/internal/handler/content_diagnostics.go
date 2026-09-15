package handler

import (
 "context"
 "encoding/json"
 "errors"
 "fmt"
 "net/http"
 "strconv"
 "time"

 "github.com/go-chi/chi/v5"
 "github.com/jackc/pgx/v5"
 "github.com/multica-ai/multica/server/internal/content/diagnostics"
 workspacecore "github.com/multica-ai/multica/server/internal/content/workspace-core"
 "github.com/multica-ai/multica/server/internal/middleware"
 "go.opentelemetry.io/otel/trace"
)

func(h *Handler)diagnosticScope(w http.ResponseWriter,r *http.Request)(diagnostics.Scope,bool){
 if h.ContentDiagnostics==nil{writeError(w,503,"diagnostics unavailable");return diagnostics.Scope{},false}
 if isMachineCredentialActor(r){diagnosticError(w,diagnostics.ErrDenied);return diagnostics.Scope{},false}
 workspace:=h.resolveWorkspaceID(r)
 actor,ok:=requireUserID(w,r);if !ok{return diagnostics.Scope{},false}
 // The decision now comes from content/workspace-core so the next content
 // module does not reimplement it. The MAPPING stays here and stays exactly as
 // it shipped: non-member 404, insufficient role 403. Both were verified by
 // 002-V05-11; folding them into one would be a regression wearing consistency
 // as a disguise. New modules use workspacecore.RefusalStatus instead.
 decision:=workspacecore.Authorize(r.Context(),h.diagnosticMembership(),h.diagnosticRefusalRecorder(),actor,workspace,"owner","admin")
 if !decision.Allowed{
  if decision.Reason==workspacecore.ReasonRole{diagnosticError(w,diagnostics.ErrDenied);return diagnostics.Scope{},false}
  writeJSON(w,404,map[string]any{"error":"workspace not found","code":"AUTHORIZATION_DENIED","trace_id":trace.SpanContextFromContext(r.Context()).TraceID().String(),"next_action":"check_authorization"});return diagnostics.Scope{},false
 }
 // Real account permissions are connected when the account domain ships.
 // Until then only workspace-owned diagnostic fixtures are accepted.
 scope:=diagnostics.Scope{Workspace:workspace,Actor:actor,Accounts:[]string{}}
 if !scope.Allows(workspace,r.URL.Query().Get("account_id")){diagnosticError(w,diagnostics.ErrDenied);return scope,false};return scope,true
}
func diagnosticError(w http.ResponseWriter,err error){status:=503;code:="DATABASE_UNAVAILABLE";if errors.Is(err,diagnostics.ErrDenied){status=403;code="AUTHORIZATION_DENIED"};if errors.Is(err,diagnostics.ErrConflict){status=409;code="INPUT_CONFLICT"};e:=diagnostics.Sanitize(diagnostics.Event{Code:code,Component:"diagnostics",Trace:w.Header().Get("X-Diagnostic-Trace")});writeJSON(w,status,map[string]any{"error":e.Message,"code":e.Code,"trace_id":e.Trace,"component":e.Component,"retryable":e.Retryable,"next_action":e.Next})}
func(h *Handler)ContentDiagnosticOverview(w http.ResponseWriter,r *http.Request){scope,ok:=h.diagnosticScope(w,r);if !ok{return};o,err:=h.ContentDiagnostics.Overview(r.Context(),scope);if err!=nil{diagnosticError(w,err);return};writeJSON(w,200,o)}
func(h *Handler)ContentDiagnosticRuns(w http.ResponseWriter,r *http.Request){scope,ok:=h.diagnosticScope(w,r);if !ok{return};if id:=r.URL.Query().Get("run_id");id!=""{run,err:=h.ContentDiagnostics.Store.GetRun(r.Context(),scope,id);if err!=nil{diagnosticError(w,err);return};writeJSON(w,200,run);return};runs,err:=h.ContentDiagnostics.Store.Runs(r.Context(),scope);if err!=nil{diagnosticError(w,err);return};writeJSON(w,200,map[string]any{"runs":runs})}
func diagnosticFilter(r *http.Request)(diagnostics.Filter,error){q:=r.URL.Query();f:=diagnostics.Filter{Kind:q.Get("kind"),Trace:q.Get("trace_id"),Run:q.Get("run_id"),Component:q.Get("component"),Code:q.Get("error_code"),Severity:q.Get("severity")};var err error;if q.Get("after")!=""{f.After,err=strconv.ParseInt(q.Get("after"),10,64);if err!=nil||f.After<0{return f,diagnostics.ErrConflict}};if q.Get("limit")!=""{f.Limit,err=strconv.Atoi(q.Get("limit"));if err!=nil||f.Limit<1||f.Limit>100{return f,diagnostics.ErrConflict}};for name,dst:=range map[string]*time.Time{"from":&f.From,"until":&f.Until}{if q.Get(name)!=""{*dst,err=time.Parse(time.RFC3339,q.Get(name));if err!=nil{return f,diagnostics.ErrConflict}}};return f,nil}
func(h *Handler)ContentDiagnosticEvents(w http.ResponseWriter,r *http.Request){scope,ok:=h.diagnosticScope(w,r);if !ok{return};f,err:=diagnosticFilter(r);if err!=nil{diagnosticError(w,err);return};page,err:=h.ContentDiagnostics.Store.Query(r.Context(),scope,f);if err!=nil{diagnosticError(w,err);return};writeJSON(w,200,page)}
func(h *Handler)ContentDiagnosticSimulate(w http.ResponseWriter,r *http.Request){scope,ok:=h.diagnosticScope(w,r);if !ok{return};var body struct{Scenario string `json:"scenario"`;Seed int64 `json:"seed"`;Account string `json:"account_id"`;Original string `json:"original_run_id"`};decoder:=json.NewDecoder(http.MaxBytesReader(w,r.Body,4096));decoder.DisallowUnknownFields();if decoder.Decode(&body)!=nil{diagnosticError(w,diagnostics.ErrConflict);return};run,err:=h.ContentDiagnostics.Run(r.Context(),scope,body.Account,body.Scenario,body.Seed,body.Original);if err!=nil{diagnosticError(w,err);return};writeJSON(w,201,run)}
func(h *Handler)ContentDiagnosticExport(w http.ResponseWriter,r *http.Request){scope,ok:=h.diagnosticScope(w,r);if !ok{return};download:=r.Method==http.MethodPost;bundle,err:=h.ContentDiagnostics.Export(r.Context(),scope,r.URL.Query().Get("run_id"),download);if err!=nil{diagnosticError(w,err);return};if download{w.Header().Set("Content-Disposition",`attachment; filename="loretide-diagnostics.json"`)};writeJSON(w,200,bundle)}
func(h *Handler)ContentDiagnosticClient(w http.ResponseWriter,r *http.Request){scope,ok:=h.diagnosticScope(w,r);if !ok{return};var body struct{Code string `json:"code"`;Heartbeat bool `json:"heartbeat"`};decoder:=json.NewDecoder(http.MaxBytesReader(w,r.Body,1024));decoder.DisallowUnknownFields();if decoder.Decode(&body)!=nil{diagnosticError(w,diagnostics.ErrConflict);return};if body.Heartbeat{h.ContentDiagnostics.Heartbeat("web","browser",time.Now().UTC());writeJSON(w,200,map[string]bool{"ok":true});return};event,err:=h.ContentDiagnostics.ClientError(r.Context(),scope,body.Code);if err!=nil{diagnosticError(w,err);return};writeJSON(w,201,event)}
// diagnosticStreamWindow bounds one stream connection so membership is
// rechecked from a fresh request regularly. Ending the window is planned, not
// a failure: the last page carries Rotate so the client resumes without
// reporting a disconnect. Tests shorten it.
var diagnosticStreamWindow=25*time.Second
// diagnosticStreamTick is how often a batch is written, and it is also the
// margin the rotation decision needs: the last page that still fits inside the
// window is the one written a full tick before the boundary.
const diagnosticStreamTick=time.Second
func(h *Handler)ContentDiagnosticStream(w http.ResponseWriter,r *http.Request){scope,ok:=h.diagnosticScope(w,r);if !ok{return};f,err:=diagnosticFilter(r);if err!=nil{diagnosticError(w,err);return};flusher,ok:=w.(http.Flusher);if !ok{writeError(w,503,"stream unavailable");return};w.Header().Set("Content-Type","application/x-ndjson");w.Header().Set("Cache-Control","no-store");w.Header().Set("X-Accel-Buffering","no");ticker:=time.NewTicker(diagnosticStreamTick);defer ticker.Stop();end:=time.Now().Add(diagnosticStreamWindow)
 for { // Recheck membership on each batch so revocation closes an active stream.
  member,err:=h.getWorkspaceMember(r.Context(),scope.Actor,scope.Workspace);if err!=nil||!roleAllowed(member.Role,"owner","admin"){return}
  page,err:=h.ContentDiagnostics.Store.Query(r.Context(),scope,f);if err!=nil{return}
  // Decide the handover before writing this page, not after it. Waiting for the
  // window to expire and then opening another round puts the rotate page
  // outside the window - two database queries past the boundary - where it
  // races whatever closes the connection there and is lost. A real browser saw
  // exactly that: 26 pages, none carrying rotate, the stream ending 11ms after
  // the 25s mark, and a disconnect notice every window (002-V05-01/02).
  // Deciding here also removes the deadline timer, and with it the coin flip
  // when the deadline and the ticker came ready in the same instant.
  page.Rotate=!time.Now().Add(diagnosticStreamTick).Before(end)
  b,err:=json.Marshal(page);if err!=nil{return};if _,err=fmt.Fprintf(w,"%s\n",b);err!=nil{return};flusher.Flush();f.After=page.Cursor
  // The planned last page has been flushed inside the window. Nothing further
  // happens: no query, no write, no waiting for a boundary that has not
  // arrived yet.
  if page.Rotate{return}
  select{case <-r.Context().Done():return;case <-ticker.C:}
 }
}

type diagnosticResponse struct{http.ResponseWriter;status int}
func(w *diagnosticResponse)WriteHeader(status int){w.status=status;w.ResponseWriter.WriteHeader(status)}
func(w *diagnosticResponse)Flush(){if f,ok:=w.ResponseWriter.(http.Flusher);ok{f.Flush()}}
// DiagnosticTrace records safe HTTP boundaries. Read queries are excluded from
// technical logging to avoid a live-log self-amplification loop.
func(h *Handler)DiagnosticTrace(next http.Handler)http.Handler{return http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
 // The trace and the X-Diagnostic-Trace header now come from the global
 // propagation middleware, so every API route carries them and this one only
 // opens its own span. Setting the header here too would restate a value the
 // middleware already owns (specs/005-diag-trace-and-sanitize, FR-003).
 ctx,parent:=diagnostics.Child(r.Context());r=r.WithContext(ctx);sc:=trace.SpanContextFromContext(ctx);start:=time.Now();wrapped:=&diagnosticResponse{w,200};next.ServeHTTP(wrapped,r)
 if r.Method==http.MethodGet && wrapped.status<400{return};if h.ContentDiagnostics==nil{return};ws:=h.resolveWorkspaceID(r);actor:=r.Header.Get("X-User-ID");if ws==""||actor==""{return};if _,err:=h.getWorkspaceMember(ctx,actor,ws);err!=nil{return}
 code:="";outcome:="success";if wrapped.status>=400{outcome="failed";switch wrapped.status{case 401,403,404:code="AUTHORIZATION_DENIED";case 400,409:code="INPUT_CONFLICT";default:code="INTERNAL"}}
 // The request identity is built from the registered route pattern, never the
 // path as requested, so a path parameter cannot reach the log (FR-006).
 // RouteContext is nil when a handler is reached outside the router, so the
 // pattern is read defensively; no pattern means no identity, never the path.
 pattern:="";if rctx:=chi.RouteContext(r.Context());rctx!=nil{pattern=rctx.RoutePattern()}
 route,status,present:=diagnostics.RequestIdentity(r.Method,pattern,wrapped.status,r.Header)
 h.ContentDiagnostics.Store.Technical(ctx,diagnostics.Event{Route:route,Status:status,HeadersPresent:present,ID:diagnostics.NewID(),Workspace:ws,Actor:actor,ActorKind:"human",ObjectType:"diagnostics",Action:"execute",Outcome:outcome,Code:code,Operation:diagnostics.NewID(),Trace:sc.TraceID().String(),Span:sc.SpanID().String(),Parent:parent,Occurred:start.UTC(),Received:time.Now().UTC(),Component:"api",Severity:"info",Duration:time.Since(start).Milliseconds(),Build:h.ContentDiagnostics.Build,Upstream:middleware.UpstreamTraceFromContext(ctx)})
})}

// diagnosticMembership adapts this package's membership lookup to the shape
// content/workspace-core expects, so that package depends on neither the
// generated db types nor this one.
type diagnosticMembershipFunc func(ctx context.Context,actor,workspace string)(string,bool,error)
func(f diagnosticMembershipFunc)RoleFor(ctx context.Context,actor,workspace string)(string,bool,error){return f(ctx,actor,workspace)}
func(h *Handler)diagnosticMembership()workspacecore.Membership{
 return diagnosticMembershipFunc(func(ctx context.Context,actor,workspace string)(string,bool,error){
  member,err:=h.getWorkspaceMember(ctx,actor,workspace)
  // Not found and malformed input both mean "no usable membership". Telling
  // them apart here would surface the difference in the response, which is
  // exactly what the refusal exists to hide.
  //
  // A storage failure is different in one respect only: it is returned so the
  // caller can LOG it. workspace-core treats a non-nil error exactly as it
  // treats found=false, so the decision, the reason and the response are
  // unchanged - see Decision.Err. Returning nil here threw away the reason a
  // local instance answered 404 with an empty database.
  if err!=nil{if errors.Is(err,pgx.ErrNoRows){return "",false,nil};return "",false,err}
  return member.Role,true,nil
 })
}
// diagnosticRefusalRecorder sends refusals to the technical log; nil when the
// service is not wired, which Authorize tolerates.
type diagnosticRecorderFunc func(ctx context.Context,event diagnostics.Event)
func(f diagnosticRecorderFunc)Technical(ctx context.Context,event diagnostics.Event){f(ctx,event)}
func(h *Handler)diagnosticRefusalRecorder()workspacecore.Recorder{
 if h.ContentDiagnostics==nil||h.ContentDiagnostics.Store==nil{return nil}
 store:=h.ContentDiagnostics.Store
 return diagnosticRecorderFunc(func(ctx context.Context,event diagnostics.Event){store.Technical(ctx,event)})
}
