package handler

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/daemonws"
	"github.com/multica-ai/multica/server/internal/events"
	"github.com/multica-ai/multica/server/pkg/protocol"
	"sync"
	"testing"
	"time"
)

// fakeLivenessStore lets tests drive every Available / Touch / IsAliveBatch
// branch of recordHeartbeat without spinning up Redis. It records call counts
// so we can assert the gate behavior without any DB-time dependence.
type fakeLivenessStore struct {
	mu          sync.Mutex
	available   bool
	touchErr    error
	touched     []string
	aliveResult map[string]bool
	aliveOK     bool
	forgotten   []string
}

func (f *fakeLivenessStore) Available() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.available
}

func (f *fakeLivenessStore) Touch(_ context.Context, runtimeID string, _ time.Duration) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.touched = append(f.touched, runtimeID)
	return f.touchErr
}

func (f *fakeLivenessStore) IsAliveBatch(_ context.Context, ids []string) (map[string]bool, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.aliveOK {
		return nil, false
	}
	out := make(map[string]bool, len(ids))
	for _, id := range ids {
		out[id] = f.aliveResult[id]
	}
	return out, true
}

func (f *fakeLivenessStore) Forget(_ context.Context, runtimeID string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.forgotten = append(f.forgotten, runtimeID)
}

func (f *fakeLivenessStore) touchCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.touched)
}

type recordingHeartbeatScheduler struct {
	ids []pgtype.UUID
	err error
}

func (s *recordingHeartbeatScheduler) Schedule(_ context.Context, id, _ pgtype.UUID) error {
	s.ids = append(s.ids, id)
	return s.err
}

func pgUUID(s string) (pgtype.UUID, error) {
	var u pgtype.UUID
	if err := u.Scan(s); err != nil {
		return u, err
	}
	return u, nil
}

func TestRecordHeartbeatLeaseThrottlesDBScheduling(t *testing.T) {
	runtimeID := uuid.NewString()
	fake := &fakeLivenessStore{available: true, aliveOK: true}
	scheduler := &recordingHeartbeatScheduler{}
	h := &Handler{LivenessStore: fake, HeartbeatScheduler: scheduler}
	lease := daemonws.NewRuntimeLease("workspace-1", "online", time.Now().Add(-2*runtimeHeartbeatDBFlushInterval), true)

	if err := h.recordHeartbeatLease(context.Background(), runtimeID, lease); err != nil {
		t.Fatalf("first recordHeartbeatLease: %v", err)
	}
	if err := h.recordHeartbeatLease(context.Background(), runtimeID, lease); err != nil {
		t.Fatalf("second recordHeartbeatLease: %v", err)
	}

	if len(scheduler.ids) != 1 {
		t.Fatalf("scheduled DB writes = %d, want 1 within one flush window", len(scheduler.ids))
	}
	if fake.touchCount() != 2 {
		t.Fatalf("Redis touches = %d, want 2", fake.touchCount())
	}
	state := lease.Snapshot()
	if !state.LastSeenAtValid || time.Since(state.LastSeenAt) > time.Second {
		t.Fatalf("lease DB watermark was not advanced: %+v", state)
	}
}

func TestRecordHeartbeatLeaseScheduleFailureKeepsStaleWatermark(t *testing.T) {
	runtimeID := uuid.NewString()
	fake := &fakeLivenessStore{available: true, aliveOK: true}
	injected := errors.New("injected schedule failure")
	scheduler := &recordingHeartbeatScheduler{err: injected}
	h := &Handler{LivenessStore: fake, HeartbeatScheduler: scheduler}
	stale := time.Now().Add(-2 * runtimeHeartbeatDBFlushInterval)
	lease := daemonws.NewRuntimeLease("workspace-1", "online", stale, true)

	if err := h.recordHeartbeatLease(context.Background(), runtimeID, lease); !errors.Is(err, injected) {
		t.Fatalf("recordHeartbeatLease error = %v, want injected failure", err)
	}
	if got := lease.Snapshot().LastSeenAt; !got.Equal(stale) {
		t.Fatalf("failed schedule advanced lease watermark: got %s want %s", got, stale)
	}
}

// TestNotifyRuntimeRecovered_PublishesKnownWorkspaceWithoutLookup pins the
// no-query contract: a handler with no Queries can still publish the refresh.
func TestNotifyRuntimeRecovered_PublishesKnownWorkspaceWithoutLookup(t *testing.T) {
	const workspaceID = "11111111-1111-1111-1111-111111111111"
	h := Handler{Bus: events.New()}
	var refreshes []events.Event
	h.Bus.Subscribe(protocol.EventDaemonRegister, func(event events.Event) {
		refreshes = append(refreshes, event)
	})

	h.NotifyRuntimeRecovered(context.Background(), workspaceID)

	if len(refreshes) != 1 {
		t.Fatalf("recovery refreshes = %d, want 1", len(refreshes))
	}
	if refreshes[0].WorkspaceID != workspaceID {
		t.Fatalf("refresh workspace_id = %q, want %q", refreshes[0].WorkspaceID, workspaceID)
	}
}
