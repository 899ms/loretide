package execenv

import (
 "log/slog"
 "os"
 "path/filepath"
 "testing"
)

func TestLoretideRejectsBeforeCredentialPreparation(t *testing.T) {
 t.Setenv("LORETIDE_EXECUTION_POLICY", "disabled")
 root := filepath.Join(t.TempDir(),"must-not-exist")
 if _,err := Prepare(PrepareParams{WorkspacesRoot:root},slog.Default()); err == nil {t.Fatal("prepare enabled")}
 if _,err := os.Stat(root); !os.IsNotExist(err) {t.Fatal("preparation touched filesystem")}
 if Reuse(ReuseParams{},slog.Default()) != nil {t.Fatal("reuse enabled")}
}
