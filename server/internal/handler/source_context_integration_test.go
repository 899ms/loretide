package handler

import (
	"errors"
	"github.com/multica-ai/multica/server/internal/service"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWriteSourceContextErrorHidesInternalDetails(t *testing.T) {
	recorder := httptest.NewRecorder()
	(&Handler{}).writeSourceContextError(recorder, errors.New("postgres password=do-not-leak"), service.SourceContextLimitUsage{})

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusInternalServerError)
	}
	if strings.Contains(recorder.Body.String(), "do-not-leak") {
		t.Fatalf("internal error leaked in response: %s", recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "failed to capture source context") {
		t.Fatalf("generic source-context error missing: %s", recorder.Body.String())
	}
}
