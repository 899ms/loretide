package diagnostics

import("context";"log/slog";"net/http";"net/textproto";"regexp";"sort";"strings";"sync";"sync/atomic";"time")

var codes=map[string]string{
 "":"Completed", "AUTHORIZATION_DENIED":"Access denied", "FILE_MISSING":"File unavailable", "FILE_CHANGED":"File changed",
 "DATABASE_UNAVAILABLE":"Database write failed", "NETWORK_UNAVAILABLE":"Connection interrupted", "SEARCH_FAILED":"Search failed",
 "MODEL_AUTH":"Model authentication failed", "MODEL_QUOTA":"Model quota exhausted", "OUTPUT_SCHEMA":"Output format rejected",
 "TIMEOUT":"Operation timed out", "CANCELLED":"Operation cancelled", "DUPLICATE":"Duplicate event ignored", "LATE_RESULT":"Late result ignored",
 "INPUT_CONFLICT":"Input conflict", "UI_ERROR":"Browser rendering failed", "INTERNAL":"Internal error", "CLOCK_SKEW":"Clock skew detected",
}
var token=regexp.MustCompile(`^[a-zA-Z0-9_.:-]{0,100}$`)
var hexID=regexp.MustCompile(`^[a-f0-9]{16,32}$`)
func safeToken(v string)string{if token.MatchString(v){return v};return "[redacted]"}

// Request header admission, three tiers. The list is the rule; see
// specs/005-diag-trace-and-sanitize/contracts/request-sanitization.md.
// Anything unlisted is denied, which is what makes a header introduced
// tomorrow safe today.
type admission int
const (admitNone admission=iota;admitPresence;admitValue)
// admitValue: the value may be recorded where a field exists for it.
var headerValueAdmitted=map[string]bool{"Traceparent":true,"Tracestate":true,"X-Diagnostic-Trace":true,"X-Request-Id":true,"X-Workspace-Id":true,"Content-Type":true,"Content-Length":true}
// admitPresence: only the fact that the header was sent. Not the value, and
// not its length — a length is a value in disguise.
var headerPresenceAdmitted=map[string]bool{"User-Agent":true,"Accept":true}
// Denied by name. Redundant with the suffix patterns for X-Api-Key on purpose:
// if one list is mis-edited the other still refuses.
var headerDenied=map[string]bool{"Authorization":true,"Cookie":true,"Set-Cookie":true,"X-Api-Key":true,"Proxy-Authorization":true}
// Denied by shape. Matched on the final "-" segment, so x-api-token is denied
// while a header that merely ends in those letters is not swept up by accident
// (it is still denied, by the default).
var headerDeniedSuffix=map[string]bool{"token":true,"secret":true,"key":true,"password":true}
// headerAdmission decides one header name. Denial wins over both admit tiers.
func headerAdmission(name string)admission{
 canonical:=textproto.CanonicalMIMEHeaderKey(strings.TrimSpace(name))
 if canonical==""{return admitNone}
 if headerDenied[canonical]{return admitNone}
 if parts:=strings.Split(canonical,"-");len(parts)>1&&headerDeniedSuffix[strings.ToLower(parts[len(parts)-1])]{return admitNone}
 if headerValueAdmitted[canonical]{return admitValue}
 if headerPresenceAdmitted[canonical]{return admitPresence}
 return admitNone
}
// RequestIdentity is the only way a boundary should describe the request it is
// recording: the registered route pattern, the method, the status, and the
// presence-tier headers. Everything else about the request stays out.
func RequestIdentity(method,pattern string,status int,h http.Header)(string,int,[]string){
 route:=requestRoute(method,pattern)
 if status<100||status>599{status=0}
 return route,status,headersPresent(h)
}
// headersPresent lists the presence-tier headers that were sent. Names only,
// drawn from a compile-time map, so the result cannot carry request content.
func headersPresent(h http.Header)[]string{
 names:=[]string{}
 for name:=range h{if headerAdmission(name)==admitPresence{names=append(names,textproto.CanonicalMIMEHeaderKey(name))}}
 sort.Strings(names);return names
}
// routeShape is the registered route pattern plus its method. A pattern is
// fixed at registration, so it cannot carry a path parameter; anything that
// does not look like one is dropped whole rather than cleaned up.
var routeShape=regexp.MustCompile(`^[A-Z]{3,7} /[A-Za-z0-9/_{}.:-]*$`)
// requestRoute builds the request identity from the route pattern and method.
// It never falls back to the path as requested: an unmatched route has no safe
// identity, and "" says so honestly.
func requestRoute(method,pattern string)string{
 if method==""||pattern==""{return ""}
 candidate:=method+" "+pattern
 if !routeShape.MatchString(candidate)||strings.Contains(candidate,".."){return ""}
 return candidate
}
// safeRoute re-checks a route that reached Sanitize from anywhere, so a sink
// that built one by hand cannot bypass the rule.
func safeRoute(v string)string{if v==""{return ""};parts:=strings.SplitN(v," ",2);if len(parts)!=2{return ""};return requestRoute(parts[0],parts[1])}
// safePresence keeps only names the presence tier admits, whatever the caller
// put in the slice.
func safePresence(names []string)[]string{
 kept:=[]string{}
 for _,name:=range names{if headerAdmission(name)==admitPresence{kept=append(kept,textproto.CanonicalMIMEHeaderKey(name))}}
 sort.Strings(kept);if len(kept)==0{return nil};return kept
}
func oneOf(v string,allowed ...string)string{for _,a:=range allowed{if a==v{return v}};return "unknown"}
// nonModuleComponents are the delivery tiers an event can come from. They
// predate the content modules and name where in the stack something happened,
// not which feature it belonged to.
var nonModuleComponents = []string{"web", "api", "database", "queue", "daemon", "executor", "tool", "result"}

// contentModuleComponents are the content module names, and they are copied
// from scripts/content-boundaries.json - the registry the boundary checker
// already enforces. That file is the single source; this slice has to agree
// with it, and TestSanitizeComponentAllowlistFollowsTheModuleRegistry reads the
// registry and fails in either direction if it does not.
//
// Copied rather than embedded because the registry lives outside the server Go
// module and go:embed cannot reach past the module root. A generator would put
// the same text in the same place with a build step in between; the test is
// what actually keeps them equal either way.
//
// Without these names every ip-profile, workspace-core and topic-planning
// event reached the real sink as component "unknown" (Issue #108), so the panel
// and the export could not tell one content module's events from another's -
// and neither could a test that wanted to count them.
var contentModuleComponents = []string{
	"diagnostics", "workspace-core", "ip-profile", "source-inbox",
	"knowledge-base", "topic-planning", "work-editor", "agent-workflow",
	"review-delivery", "feedback-learning", "project-collab", "agent-gateway",
}

// componentAllowlist is the union. "diagnostics" is in both groups - it is a
// tier and a module - and oneOf does not mind the repeat.
var componentAllowlist = append(append([]string{}, nonModuleComponents...), contentModuleComponents...)

// Sanitize uses an allowlist. Model output, input text, paths, URLs, headers and
// nested attributes are never copied into the technical read model.
func Sanitize(e Event) Event {
 if _,ok:=codes[e.Code];!ok{e.Code="INTERNAL"};e.Message=codes[e.Code]
 e.Component=oneOf(e.Component,componentAllowlist...)
 e.Severity=oneOf(e.Severity,"debug","info","warn","error");e.ActorKind=oneOf(e.ActorKind,"human","agent","system")
 e.Outcome=oneOf(e.Outcome,"success","failed","cancelled","ignored","pending")
 e.Action=oneOf(e.Action,"simulate","execute","query","export","client_error","retry","cancel","cleanup","result")
 e.ObjectType=oneOf(e.ObjectType,"simulation","run","work","account","source","diagnostics")
 e.Step=safeToken(e.Step);e.Build=safeToken(e.Build);e.Version=safeToken(e.Version)
 for _,p:=range []*string{&e.Trace,&e.Span,&e.Parent,&e.Operation,&e.Run,&e.ID}{if *p!="" && !hexID.MatchString(*p){*p=""}}
 // Request identity and correlation attribute. Each is re-checked here rather
 // than trusted from the caller, so every sink shares one rule (FR-007).
 e.Route=safeRoute(e.Route);if e.Status<100||e.Status>599{e.Status=0}
 e.HeadersPresent=safePresence(e.HeadersPresent)
 if e.Upstream!=""&&!hexID.MatchString(e.Upstream){e.Upstream=""}
 e.Next="inspect_trace";e.Retryable=false
 switch e.Code{case "NETWORK_UNAVAILABLE","SEARCH_FAILED","TIMEOUT","DATABASE_UNAVAILABLE":e.Next="retry_simulation";e.Retryable=true;case "AUTHORIZATION_DENIED":e.Next="check_authorization";case "FILE_MISSING","FILE_CHANGED":e.Next="check_registered_file";case "MODEL_AUTH","MODEL_QUOTA":e.Next="check_local_client"}
 return e
}
type LogBuffer struct{mu sync.Mutex;events []Event;capacity int;Dropped atomic.Int64;Errors atomic.Int64}
func NewLogBuffer(capacity int)*LogBuffer{if capacity<1{capacity=1};return &LogBuffer{capacity:capacity}}
func(l *LogBuffer)Append(e Event){l.mu.Lock();defer l.mu.Unlock();if len(l.events)==l.capacity{l.events=l.events[1:];l.Dropped.Add(1)};l.events=append(l.events,Sanitize(e))}
func(l *LogBuffer)Events()[]Event{l.mu.Lock();defer l.mu.Unlock();return append([]Event{},l.events...)}
// SlogHandler intentionally discards message/attrs except finite enum fields.
type SlogHandler struct{Buffer *LogBuffer}
func(h SlogHandler)Enabled(context.Context,slog.Level)bool{return true}
func(h SlogHandler)Handle(_ context.Context,r slog.Record)error{e:=Event{Occurred:r.Time,Received:time.Now().UTC(),Severity:"info",Component:"diagnostics",ActorKind:"system",Action:"execute",Outcome:"pending"};if r.Level>=slog.LevelError{e.Severity="error"};r.Attrs(func(a slog.Attr)bool{switch a.Key{case "error_code":e.Code=a.Value.String();case "component":e.Component=a.Value.String();case "trace_id":e.Trace=a.Value.String()};return true});h.Buffer.Append(e);return nil}
func(h SlogHandler)WithAttrs([]slog.Attr)slog.Handler{return h}
func(h SlogHandler)WithGroup(string)slog.Handler{return h}
