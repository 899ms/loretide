package diagnostics
import("context";"encoding/json";"log/slog";"strings";"testing")
func TestSecretsNeverEnterTechnicalLog(t *testing.T){
 b:=NewLogBuffer(2);h:=SlogHandler{b};l:=slog.New(h)
 l.ErrorContext(context.Background(),"Bearer secret-body", "Authorization","secret-token","nested",map[string]any{"cookie":"secret-cookie"},"path","C:/private/secret.txt","component","api","error_code","TIMEOUT")
 e:=Sanitize(Event{Message:"private-body",Code:"secret-code",Component:"https://secret?token=123",Trace:"secret-trace",Build:"/private/path"});b.Append(e)
 wire,_:=json.Marshal(b.Events());if strings.Contains(string(wire),"secret")||strings.Contains(string(wire),"private"){t.Fatal(string(wire))}
 b.Append(e);if len(b.Events())!=2 || b.Dropped.Load()!=1{t.Fatal("unbounded sink")}
}
