package handler

import (
 "encoding/json"
 "errors"
 "fmt"
 "net/http"
 "strconv"
 "time"

 "github.com/multica-ai/multica/server/internal/content/diagnostics"
 "go.opentelemetry.io/otel/trace"
)

func(h *Handler)diagnosticScope(w http.ResponseWriter,r *http.Request)(diagnostics.Scope,bool){
 if h.ContentDiagnostics==nil{writeError(w,503,"diagnostics unavailable");return diagnostics.Scope{},false}
 if isMachineCredentialActor(r){diagnosticError(w,diagnostics.ErrDenied);return diagnostics.Scope{},false}
 workspace:=h.resolveWorkspaceID(r)
 actor,ok:=requireUserID(w,r);if !ok{return diagnostics.Scope{},false}
 member,err:=h.getWorkspaceMember(r.Context(),actor,workspace);if err!=nil{writeJSON(w,404,map[string]any{"error":"workspace not found","code":"AUTHORIZATION_DENIED","trace_id":trace.SpanContextFromContext(r.Context()).TraceID().String(),"next_action":"check_authorization"});return diagnostics.Scope{},false};if !roleAllowed(member.Role,"owner","admin"){diagnosticError(w,diagnostics.ErrDenied);return diagnostics.Scope{},false}
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
func(h *Handler)ContentDiagnosticStream(w http.ResponseWriter,r *http.Request){scope,ok:=h.diagnosticScope(w,r);if !ok{return};f,err:=diagnosticFilter(r);if err!=nil{diagnosticError(w,err);return};flusher,ok:=w.(http.Flusher);if !ok{writeError(w,503,"stream unavailable");return};w.Header().Set("Content-Type","application/x-ndjson");w.Header().Set("Cache-Control","no-store");w.Header().Set("X-Accel-Buffering","no");ticker:=time.NewTicker(time.Second);defer ticker.Stop();deadline:=time.NewTimer(diagnosticStreamWindow);defer deadline.Stop();rotate:=false
 for { // Recheck membership on each batch so revocation closes an active stream.
  member,err:=h.getWorkspaceMember(r.Context(),scope.Actor,scope.Workspace);if err!=nil||!roleAllowed(member.Role,"owner","admin"){return}
  page,err:=h.ContentDiagnostics.Store.Query(r.Context(),scope,f);if err!=nil{return};page.Rotate=rotate;b,err:=json.Marshal(page);if err!=nil{return};if _,err=fmt.Fprintf(w,"%s\n",b);err!=nil{return};flusher.Flush();f.After=page.Cursor
  // The window closed, so this page was the planned last one.
  if rotate{return}
  select{case <-r.Context().Done():return;case <-deadline.C:rotate=true;case <-ticker.C:}
 }
}

type diagnosticResponse struct{http.ResponseWriter;status int}
func(w *diagnosticResponse)WriteHeader(status int){w.status=status;w.ResponseWriter.WriteHeader(status)}
func(w *diagnosticResponse)Flush(){if f,ok:=w.ResponseWriter.(http.Flusher);ok{f.Flush()}}
// DiagnosticTrace records safe HTTP boundaries. Read queries are excluded from
// technical logging to avoid a live-log self-amplification loop.
func(h *Handler)DiagnosticTrace(next http.Handler)http.Handler{return http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
 ctx,_:=diagnostics.Child(r.Context());r=r.WithContext(ctx);sc:=trace.SpanContextFromContext(ctx);w.Header().Set("X-Diagnostic-Trace",sc.TraceID().String());start:=time.Now();wrapped:=&diagnosticResponse{w,200};next.ServeHTTP(wrapped,r)
 if r.Method==http.MethodGet && wrapped.status<400{return};if h.ContentDiagnostics==nil{return};ws:=h.resolveWorkspaceID(r);actor:=r.Header.Get("X-User-ID");if ws==""||actor==""{return};if _,err:=h.getWorkspaceMember(ctx,actor,ws);err!=nil{return}
 code:="";outcome:="success";if wrapped.status>=400{outcome="failed";switch wrapped.status{case 401,403,404:code="AUTHORIZATION_DENIED";case 400,409:code="INPUT_CONFLICT";default:code="INTERNAL"}}
 h.ContentDiagnostics.Store.Technical(ctx,diagnostics.Event{ID:diagnostics.NewID(),Workspace:ws,Actor:actor,ActorKind:"human",ObjectType:"diagnostics",Action:"execute",Outcome:outcome,Code:code,Operation:diagnostics.NewID(),Trace:sc.TraceID().String(),Span:sc.SpanID().String(),Occurred:start.UTC(),Received:time.Now().UTC(),Component:"api",Severity:"info",Duration:time.Since(start).Milliseconds(),Build:h.ContentDiagnostics.Build})
})}
