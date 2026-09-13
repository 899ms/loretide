package diagnostics

import("context";"log/slog";"regexp";"sync";"sync/atomic";"time")

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
func oneOf(v string,allowed ...string)string{for _,a:=range allowed{if a==v{return v}};return "unknown"}
// Sanitize uses an allowlist. Model output, input text, paths, URLs, headers and
// nested attributes are never copied into the technical read model.
func Sanitize(e Event) Event {
 if _,ok:=codes[e.Code];!ok{e.Code="INTERNAL"};e.Message=codes[e.Code]
 e.Component=oneOf(e.Component,"web","api","database","queue","daemon","executor","tool","result","diagnostics")
 e.Severity=oneOf(e.Severity,"debug","info","warn","error");e.ActorKind=oneOf(e.ActorKind,"human","agent","system")
 e.Outcome=oneOf(e.Outcome,"success","failed","cancelled","ignored","pending")
 e.Action=oneOf(e.Action,"simulate","execute","query","export","client_error","retry","cancel","cleanup","result")
 e.ObjectType=oneOf(e.ObjectType,"simulation","run","work","account","source","diagnostics")
 e.Step=safeToken(e.Step);e.Build=safeToken(e.Build);e.Version=safeToken(e.Version)
 for _,p:=range []*string{&e.Trace,&e.Span,&e.Parent,&e.Operation,&e.Run,&e.ID}{if *p!="" && !hexID.MatchString(*p){*p=""}}
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
