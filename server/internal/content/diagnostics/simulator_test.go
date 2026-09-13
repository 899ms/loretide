package diagnostics
import("context";"testing")
func TestEverySimulatedFaultAndDeterministicTime(t *testing.T){
 for _,scenario:=range Scenarios{if scenario.ID=="database"{continue};t.Run(scenario.ID,func(t *testing.T){
  scope:=Scope{Workspace:"w",Actor:"u"};a,err:=Simulate(context.Background(),scope,"",scenario.ID,7,"test",true);if err!=nil{t.Fatal(err)};b,_:=Simulate(context.Background(),scope,"",scenario.ID,7,"test",true)
  if a.Regression!="not_run"{t.Fatal("premature regression success")};Evaluate(&a);if a.Regression!="passed"{t.Fatalf("%s != %s",a.Actual,a.Expected)}
  for i,e:=range a.Events{if e.Duration!=b.Events[i].Duration || e.Occurred!=b.Events[i].Occurred{t.Fatal("non-deterministic fixture")}}
  if a.Snapshot.Preference!="all"{t.Fatal("preference changed")}
 })}
}
func TestSimulationCannotAuthorizeOrReplayHumanActions(t *testing.T){
 for _,enabled:=range []bool{false,true}{_,err:=Simulate(context.Background(),Scope{Workspace:"w",Actor:"u"},"ungranted","normal",1,"test",enabled);if err==nil{t.Fatal("unauthorized account")}}
 _,err:=Simulate(context.Background(),Scope{Workspace:"w",Actor:"u"},"","publish",1,"test",true);if err==nil{t.Fatal("human action accepted")}
 run:=Run{Expected:"TIMEOUT",Actual:""};Evaluate(&run);if run.Regression!="failed"{t.Fatal("regression ignores fault")}
}
