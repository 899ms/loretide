//go:build !windows

package processtree

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestCombinedOutputKillsDescendantsHoldingOutput(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	marker := filepath.Join(dir, "descendant-survived")
	script := filepath.Join(dir, "helper.sh")
	body := "#!/bin/sh\n(sleep 0.5; echo leaked > \"$1\") &\nwait\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	started := time.Now()
	_, err := CombinedOutput(ctx, exec.Command(script, marker), 250*time.Millisecond)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("CombinedOutput error = %v, want deadline exceeded", err)
	}
	// The bound is deliberately loose. controller.finish polls until the process
	// group is gone, and a killed descendant stays in the group as a zombie
	// until the host's init reaps it. A sandbox init that reaps on an interval
	// rather than immediately adds ~2s here with nothing wrong in this package,
	// so 3s separates "bounded" from "hung" without pinning the host's reaper.
	if elapsed := time.Since(started); elapsed > 3*time.Second {
		t.Fatalf("CombinedOutput took %s, want a hard bounded return", elapsed)
	}

	time.Sleep(600 * time.Millisecond)
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("descendant survived process-tree cancellation, stat error = %v", err)
	}
}
