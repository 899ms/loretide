package diagnostics

import (
	"context"
	"encoding/json"
	"go.opentelemetry.io/otel/trace"
	"testing"
)

func TestTracePropagationAndUntrustedIdentity(t *testing.T) {
	ctx, _ := Child(context.Background())
	e := Pack(ctx, NewID(), 1, 1)
	wire, _ := json.Marshal(e)
	var next Envelope
	_ = json.Unmarshal(wire, &next)
	ctx2, err := Unpack(context.Background(), next)
	if err != nil {
		t.Fatal(err)
	}
	child, parent := Child(ctx2)
	if trace.SpanContextFromContext(child).TraceID() != trace.SpanContextFromContext(ctx).TraceID() || parent != trace.SpanContextFromContext(ctx).SpanID().String() {
		t.Fatal("broken parent")
	}
	next.Carrier["traceparent"] = "forged"
	if _, err := Unpack(context.Background(), next); err == nil {
		t.Fatal("invalid carrier accepted")
	}
	if (Scope{Workspace: "brand-a", Actor: "human", Accounts: []string{"a"}}).Allows("brand-b", "a") {
		t.Fatal("cross brand")
	}
	if (Scope{Workspace: "brand-a", Actor: "human", Accounts: []string{"a"}}).Allows("brand-a", "b") {
		t.Fatal("cross account")
	}
}
func TestSnapshotImmutableAndRevocation(t *testing.T) {
	s := Snapshot{Hashes: map[string]string{"a": "one"}, Grants: []string{"a"}, Preference: "all", Scope: "local"}
	saved := CloneSnapshot(s)
	s.Hashes["a"] = "two"
	if saved.Hashes["a"] != "one" || saved.Preference != "all" {
		t.Fatal("snapshot mutable")
	}
	if len(ReproductionGaps(saved, map[string]string{}, map[string]bool{})) != 2 {
		t.Fatal("gaps missing")
	}
	var r Run
	if err := json.Unmarshal([]byte(`{"run_id":"x","future_field":true}`), &r); err != nil {
		t.Fatal(err)
	}
}
func TestDiagnosticLimitsRejectInvalidConfiguration(t *testing.T) {
	s := NewStore(nil, nil)
	for _, v := range []string{"-1", "0", "oops", "100001"} {
		if s.ConfigureLimits(v, "") == nil {
			t.Fatal(v)
		}
	}
	if s.ConfigureLimits("1000", "7") != nil {
		t.Fatal("valid limits rejected")
	}
}
