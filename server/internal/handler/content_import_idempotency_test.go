package handler

import (
	"net/http"
	"testing"
)

func TestHistoricalImportRequestIsOptionalForCompatibleCallers(t *testing.T) {
	httpRequest, err := http.NewRequest(http.MethodPost, "/", nil)
	if err != nil {
		t.Fatal(err)
	}
	request, enabled, err := historicalImportRequest(httpRequest,
		"create-artifact", "work-1", map[string]string{"title": "正文"})
	if err != nil {
		t.Fatalf("headerless request returned an error: %v", err)
	}
	if enabled || request.Key != "" {
		t.Fatalf("headerless request = %+v, enabled=%v; want ordinary write", request, enabled)
	}
}

func TestHistoricalImportRequestEnablesReplayWhenTheHeaderIsPresent(t *testing.T) {
	httpRequest, err := http.NewRequest(http.MethodPost, "/", nil)
	if err != nil {
		t.Fatal(err)
	}
	httpRequest.Header.Set("Idempotency-Key", "  imported-artifact  ")
	request, enabled, err := historicalImportRequest(httpRequest,
		"create-artifact", "work-1", map[string]string{"title": "正文"})
	if err != nil {
		t.Fatalf("keyed request returned an error: %v", err)
	}
	if !enabled || request.Key != "imported-artifact" || request.Fingerprint == "" {
		t.Fatalf("keyed request = %+v, enabled=%v; want replay request", request, enabled)
	}
}
