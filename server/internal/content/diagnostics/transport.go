package diagnostics

import("context";"encoding/json";"errors";"net/http";"time";"github.com/gorilla/websocket")

type TransportReply struct{Envelope Envelope `json:"envelope"`;Code string `json:"code"`}
// ServeSimulationTransport is mounted only by an authenticated test adapter.
// authorize must recheck the current lease/scope, not trust IDs in messages.
func ServeSimulationTransport(w http.ResponseWriter,r *http.Request,enabled bool,authorize func()bool){
 if !enabled||authorize==nil||!authorize(){http.Error(w,"simulation transport denied",403);return}
 upgrader:=websocket.Upgrader{CheckOrigin:func(r *http.Request)bool{return r.Header.Get("Origin")==""}}
 conn,err:=upgrader.Upgrade(w,r,nil);if err!=nil{return};defer conn.Close();conn.SetReadLimit(4096);receiver:=Receiver{}
 for{_ =conn.SetReadDeadline(time.Now().Add(5*time.Second));var envelope Envelope;if conn.ReadJSON(&envelope)!=nil{return};if !authorize(){_ =conn.WriteControl(websocket.CloseMessage,websocket.FormatCloseMessage(1008,"authorization revoked"),time.Now().Add(time.Second));return};if _,err=Unpack(context.Background(),envelope);err!=nil{return};code:=receiver.Receive(envelope.Sequence);_ =conn.SetWriteDeadline(time.Now().Add(time.Second));if conn.WriteJSON(TransportReply{envelope,code})!=nil{return}}
}
// DecodeQueuedEnvelope validates the same contract used by HTTP and WS adapters.
func DecodeQueuedEnvelope(ctx context.Context,wire []byte)(context.Context,Envelope,error){if len(wire)>4096{return ctx,Envelope{},ErrConflict};var e Envelope;if json.Unmarshal(wire,&e)!=nil{return ctx,e,ErrConflict};next,err:=Unpack(ctx,e);return next,e,err}
func ClassifyTransport(err error)string{if errors.Is(err,context.Canceled){return "CANCELLED"};if errors.Is(err,context.DeadlineExceeded){return "TIMEOUT"};return "NETWORK_UNAVAILABLE"}
