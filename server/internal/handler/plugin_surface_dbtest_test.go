//go:build dbtest

package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestServePluginSurfaceRejectsConfiguredAppOriginBeforeOpeningToken(t *testing.T) {
	h := pluginSurfaceTokenHandler(t)
	h.cfg.AppURL = "https://app.example.test"
	h.cfg.PluginSurfaceOrigin = "https://app.example.test"
	token, err := h.mintPluginSurfaceToken(validSurfaceClaims())
	if err != nil {
		t.Fatalf("mintPluginSurfaceToken: %v", err)
	}

	request := pluginHandlerRequest(http.MethodGet, "/plugin-surfaces/"+token, nil, map[string]string{"token": token})
	request.Host = "app.example.test"
	recorder := httptest.NewRecorder()
	h.ServePluginSurface(recorder, request)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("shared app/content origin: status=%d, want %d", recorder.Code, http.StatusNotFound)
	}
}
