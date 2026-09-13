package diagnostics
import("context";"encoding/json";"net/http";"net/http/httptest";"strings";"sync/atomic";"testing";"github.com/gorilla/websocket";"go.opentelemetry.io/otel/trace")
func TestWebSocketQueuePropagationDuplicateAndRevocation(t *testing.T){
 var allowed atomic.Bool;allowed.Store(true);server:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){ServeSimulationTransport(w,r,true,allowed.Load)}));defer server.Close()
 conn,_,err:=websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL,"http"),nil);if err!=nil{t.Fatal(err)};defer conn.Close();ctx,_:=Child(context.Background());msg:=Pack(ctx,NewID(),2,1);wire,_:=json.Marshal(msg);queue:=make(chan []byte,1);queue<-wire;worker,e,err:=DecodeQueuedEnvelope(context.Background(),<-queue);if err!=nil{t.Fatal(err)}
 for i:=0;i<2;i++{if conn.WriteJSON(e)!=nil{t.Fatal("write")};var reply TransportReply;if conn.ReadJSON(&reply)!=nil{t.Fatal("read")};if i==1&&reply.Code!="DUPLICATE"{t.Fatal("duplicate accepted")};received,err:=Unpack(context.Background(),reply.Envelope);if err!=nil||trace.SpanContextFromContext(received).TraceID()!=trace.SpanContextFromContext(worker).TraceID(){t.Fatal("trace discontinuity")}}
 allowed.Store(false);_ =conn.WriteJSON(e);var reply TransportReply;if conn.ReadJSON(&reply)==nil{t.Fatal("revoked socket continued")}
}
func TestTransportNonTestDenied(t *testing.T){w:=httptest.NewRecorder();ServeSimulationTransport(w,httptest.NewRequest("GET","/",nil),false,func()bool{return true});if w.Code!=403{t.Fatal(w.Code)}}
