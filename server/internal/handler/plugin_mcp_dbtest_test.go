//go:build dbtest

package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func installMCPPlugin(t *testing.T, manifest string, scopes []string) string {
	t.Helper()
	withPluginsV1Flag(t, testHandler, true)
	versionID := withLocalPluginSource(t, manifest)
	body, _ := json.Marshal(map[string]any{"version_id": versionID, "granted_scopes": scopes})
	recorder := httptest.NewRecorder()
	testHandler.InstallPlugin(recorder, pluginHandlerRequest(http.MethodPost, "/plugins", body, map[string]string{"id": testWorkspaceID}))
	if recorder.Code != http.StatusCreated {
		t.Fatalf("install: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var installed struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &installed); err != nil {
		t.Fatalf("decode installation: %v", err)
	}
	t.Cleanup(func() {
		cleanup := httptest.NewRecorder()
		testHandler.UninstallPlugin(cleanup, pluginHandlerRequest(http.MethodDelete, "/plugins",
			nil, map[string]string{"id": testWorkspaceID, "installationId": installed.ID}))
	})
	return installed.ID
}

func listMCPTools(t *testing.T, installationID, hookKey string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	testHandler.ListPluginMCPTools(recorder, pluginHandlerRequest(http.MethodGet, "/mcp/tools", nil, map[string]string{
		"id": testWorkspaceID, "installationId": installationID, "hookKey": hookKey,
	}))
	return recorder
}

func TestMCPToolListRejectsAnHTTPTransportHook(t *testing.T) {
	installationID := installMCPPlugin(t, httpHookManifest, []string{"issues:read", "net:hooks.example.com"})

	recorder := listMCPTools(t, installationID, "notify")
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("an http hook must not answer a tool list: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestMCPToolListRejectsAnUnknownHook(t *testing.T) {
	installationID := installMCPPlugin(t, mcpToolboxManifest, []string{"issues:read", "net:tools.example.com"})

	recorder := listMCPTools(t, installationID, "does-not-exist")
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status=%d body=%s, want 404", recorder.Code, recorder.Body.String())
	}
}

// Refused at PUBLISH, not at discovery: manifest validation already requires
// every hook transport URL to sit inside a declared `net:` scope, and that check
// does not care which transport it is. The equivalent check inside
// DiscoverMCPHookTools is a second layer behind this one, not the gate.
//
// The gate used to sit at install, because that was the first moment anybody
// parsed the manifest. Now the artifact is parsed when the author hands it over,
// so a manifest like this never becomes something an administrator can be shown.
func TestMCPHookWithoutANetScopeCannotBePublished(t *testing.T) {
	withPluginsV1Flag(t, testHandler, true)
	root := t.TempDir()
	writeLocalPluginManifest(t, root, scopelessMCPManifest)
	previousDir := testHandler.PluginService.LocalDir
	testHandler.PluginService.LocalDir = root
	t.Cleanup(func() { testHandler.PluginService.LocalDir = previousDir })

	body, _ := json.Marshal(map[string]string{"name": "hello"})
	recorder := httptest.NewRecorder()
	testHandler.PublishLocalPluginPackage(recorder,
		pluginHandlerRequest(http.MethodPost, "/plugins/packages/local", body, map[string]string{"id": testWorkspaceID}))
	if recorder.Code == http.StatusCreated {
		t.Fatal("a hook pointing outside its declared net: scopes was published")
	}
	if !strings.Contains(recorder.Body.String(), "net: scope") {
		t.Fatalf("the refusal does not name the reason: %s", recorder.Body.String())
	}
}
