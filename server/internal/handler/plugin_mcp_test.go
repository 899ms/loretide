package handler

import (
	"testing"
)

// The gates in front of an mcp hook's tool list.
//
// Discovery itself reaches the plugin author's own MCP server, so these cover
// what has to hold BEFORE any outbound call happens — the checks that would
// otherwise be a comment claiming they exist. Every one of them refuses without
// the network being involved at all, which is the point: a wrong answer here is
// a request that should never have left the process.

const mcpToolboxManifest = `{
  "manifest_version": 1,
  "key": "com.example.toolbox",
  "name": "Toolbox",
  "version": "1.0.0",
  "author": { "name": "example" },
  "scopes": ["issues:read", "net:tools.example.com"],
  "contributes": {
    "hooks": [{
      "key": "toolbox",
      "name": "Toolbox",
      "description": "Tools from an external MCP server.",
      "triggers": ["agent"],
      "transport": { "type": "mcp", "url": "https://tools.example.com/mcp" }
    }]
  }
}`

// An http hook declares one endpoint the administrator already saw on the
// consent screen. Asking it for a tool list is a category error, not an empty
// list — answering `{"tools":[]}` would make the UI offer an approval panel for
// a hook that has nothing to approve.
const httpHookManifest = `{
  "manifest_version": 1,
  "key": "com.example.plainhook",
  "name": "Plain Hook",
  "version": "1.0.0",
  "author": { "name": "example" },
  "scopes": ["issues:read", "net:hooks.example.com"],
  "contributes": {
    "hooks": [{
      "key": "notify",
      "name": "Notify",
      "description": "Posts to an endpoint.",
      "triggers": ["manual"],
      "transport": { "type": "http", "url": "https://hooks.example.com/notify" }
    }]
  }
}`

// The `net:` scope is the consent, and a hook's own transport URL is not
// self-authorizing. A manifest can declare any endpoint it likes; what makes it
// reachable is the scope line the administrator read and approved, so a manifest
// that declares an mcp hook and no `net:` scope gets refused before any DNS
// lookup happens.
const scopelessMCPManifest = `{
  "manifest_version": 1,
  "key": "com.example.scopeless",
  "name": "Scopeless",
  "version": "1.0.0",
  "author": { "name": "example" },
  "scopes": ["issues:read"],
  "contributes": {
    "hooks": [{
      "key": "toolbox",
      "name": "Toolbox",
      "description": "Points somewhere it was never granted.",
      "triggers": ["agent"],
      "transport": { "type": "mcp", "url": "https://tools.example.com/mcp" }
    }]
  }
}`

// The daemon's credential route keys on "<installation>:<hook>". A malformed id
// must be refused rather than resolved as a bare installation id — otherwise a
// daemon holding an id for one contribution could name another by trimming it.
func TestPluginContributionIDNeedsBothHalves(t *testing.T) {
	refused := []string{
		"", "no-colon", ":toolbox", "installation:",
		"plugin:", "plugin:no-colon", "plugin::toolbox", "plugin:installation:",
		// Unprefixed. A workspace's own Remote MCP connection resolves its
		// credential from somewhere else entirely, so its id must not be
		// answerable here even when it happens to have the right shape.
		"1f0d9b7e-0000-0000-0000-000000000000:toolbox",
	}
	for _, contribution := range refused {
		if _, _, ok := splitPluginContributionID(contribution); ok {
			t.Fatalf("contribution %q was accepted; it must be refused", contribution)
		}
	}

	installationID, hookKey, ok := splitPluginContributionID("plugin:1f0d9b7e-0000-0000-0000-000000000000:toolbox")
	if !ok || installationID != "1f0d9b7e-0000-0000-0000-000000000000" || hookKey != "toolbox" {
		t.Fatalf("split = (%q, %q, %v), want the two halves", installationID, hookKey, ok)
	}
}
