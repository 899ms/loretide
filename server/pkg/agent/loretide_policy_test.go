package agent

import "testing"

func TestLoretideRejectsAllProviderFactories(t *testing.T) {
 t.Setenv("LORETIDE_EXECUTION_POLICY", "disabled")
 for _, provider := range []string{"codex", "claude", "pi", "hermes", "opencode", "openclaw", "gemini"} {
  if _, err := New(provider, Config{}); err == nil { t.Fatalf("provider %s enabled", provider) }
 }
 if _, err := NewRuntime("omp",Config{}); err == nil { t.Fatal("builtin bypassed gate") }
}
