//go:build dbtest

package handler

import (
	"context"
	"net/http"
	"testing"

	"github.com/multica-ai/multica/server/internal/testutil"
)

// TestInitiateListModels_ForceSkipsCatalogCache pins the contract behind the
// picker's Refresh action: a normal open stays instant on a warm catalog, while
// force=true creates a request the daemon must answer instead of returning the
// same cached snapshot again.
func TestInitiateListModels_ForceSkipsCatalogCache(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}

	runtimeID := dbfx.Runtime(t, "Force model refresh runtime")
	cache := NewInMemoryModelCatalogCache()
	unavailable := []UnavailableModelEntry{{
		ID:     "cc-update-required-1",
		Label:  "Fable 5.1 (disabled)",
		Reason: "Update Claude Code",
	}}
	if err := cache.Put(context.Background(), runtimeID, sampleCatalog(), unavailable, true); err != nil {
		t.Fatalf("seed catalog: %v", err)
	}
	store := NewInMemoryModelListStore()
	recorder := &pendingWorkRecorder{}
	h := *testHandler
	h.ModelCatalogCache = cache
	h.ModelListStore = store
	h.DaemonPendingWork = recorder

	request := func(query string) *http.Request {
		return withURLParam(
			newRequest(http.MethodPost, "/api/runtimes/"+runtimeID+"/models"+query, nil),
			"runtimeId",
			runtimeID,
		)
	}

	var cached ModelListRequest
	testutil.Call(t, h.InitiateListModels, request("")).Want(http.StatusOK).JSON(&cached)
	if cached.Status != ModelListCompleted || !cached.Cached {
		t.Fatalf("normal open = %+v, want completed cache hit", cached)
	}
	if len(cached.UnavailableModels) != 1 || cached.UnavailableModels[0].ID != unavailable[0].ID {
		t.Fatalf("normal cache hit lost unavailable models: %+v", cached.UnavailableModels)
	}
	if recorder.count() != 0 {
		t.Fatalf("normal cache hit queued %d daemon requests, want 0", recorder.count())
	}

	var forced ModelListRequest
	testutil.Call(t, h.InitiateListModels, request("?force=true")).Want(http.StatusOK).JSON(&forced)
	if forced.Status != ModelListPending || forced.Cached {
		t.Fatalf("forced refresh = %+v, want pending live request", forced)
	}
	if recorder.count() != 1 {
		t.Fatalf("forced refresh queued %d daemon requests, want 1", recorder.count())
	}
}
