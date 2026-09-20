package handler

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

// installClawHubFixture serves a complete successful ClawHub bundle for slug
// and swaps clawHubAPIBase to it. The returned counter tracks how many
// requests reached the fixture, letting tests assert that a rejected refresh
// never contacted the upstream.
func installClawHubFixture(t *testing.T, slug, displayName, summary string, files map[string]string) *int {
	t.Helper()

	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		switch {
		case r.URL.Path == "/api/v1/skills/"+slug:
			writeJSON(w, http.StatusOK, map[string]any{
				"skill": map[string]any{
					"slug": slug, "displayName": displayName, "summary": summary,
					"tags": map[string]string{"latest": "1.0.0"},
				},
			})
		case r.URL.Path == "/api/v1/skills/"+slug+"/versions/1.0.0":
			fileList := make([]map[string]any, 0, len(files))
			for p, c := range files {
				fileList = append(fileList, map[string]any{"path": p, "size": len(c)})
			}
			writeJSON(w, http.StatusOK, map[string]any{
				"version": map[string]any{"version": "1.0.0", "files": fileList},
			})
		case r.URL.Path == "/api/v1/skills/"+slug+"/file":
			if content, ok := files[r.URL.Query().Get("path")]; ok {
				w.Write([]byte(content))
				return
			}
			http.NotFound(w, r)
		default:
			http.NotFound(w, r)
		}
	}))
	prev := clawHubAPIBase
	clawHubAPIBase = srv.URL + "/api/v1"
	t.Cleanup(func() { clawHubAPIBase = prev; srv.Close() })
	return &requests
}

func clawhubOriginConfig(slug string) string {
	return fmt.Sprintf(`{"origin": {"type": "clawhub", "source_url": "https://clawhub.ai/acme/%s", "slug": "%s"}}`, slug, slug)
}

func TestParseSkillOrigin(t *testing.T) {
	cases := []struct {
		name   string
		config string
		wantOK bool
		want   skillOriginRef
	}{
		{name: "nil config", config: "", wantOK: false},
		{name: "empty object", config: "{}", wantOK: false},
		{name: "origin not an object", config: `{"origin": "github"}`, wantOK: false},
		{name: "origin without type", config: `{"origin": {"source_url": "https://github.com/a/b"}}`, wantOK: false},
		{
			name:   "origin without source_url",
			config: `{"origin": {"type": "github"}}`,
			wantOK: true,
			want:   skillOriginRef{Type: "github", SourceURL: ""},
		},
		{
			name:   "full origin with whitespace",
			config: `{"origin": {"type": "skills_sh", "source_url": " https://skills.sh/a/b/c "}}`,
			wantOK: true,
			want:   skillOriginRef{Type: "skills_sh", SourceURL: "https://skills.sh/a/b/c"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var raw []byte
			if tc.config != "" {
				raw = []byte(tc.config)
			}
			got, ok := parseSkillOrigin(raw)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tc.wantOK)
			}
			if ok && got != tc.want {
				t.Fatalf("origin = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestFetchImportedSkillFromOrigin_RejectsNonRefreshable(t *testing.T) {
	client := &http.Client{}
	cases := []skillOriginRef{
		{Type: "manual", SourceURL: "https://github.com/a/b"},
		{Type: "runtime_local", SourceURL: ""},
		{Type: "github", SourceURL: ""},
		{Type: "github", SourceURL: "https://clawhub.ai/acme/x"},  // host mismatch
		{Type: "clawhub", SourceURL: "https://github.com/acme/x"}, // host mismatch
		{Type: "skills_sh", SourceURL: "https://example.com/a/b"}, // unsupported host
	}
	for _, origin := range cases {
		if _, err := fetchImportedSkillFromOrigin(t.Context(), client, origin); !errors.Is(err, errSkillNotRefreshable) {
			t.Fatalf("origin %+v: err = %v, want errSkillNotRefreshable", origin, err)
		}
	}
}

func TestFetchImportedSkillFromOrigin_GitHubDispatch(t *testing.T) {
	client, _ := newGitHubFixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.Header.Get("X-Test-Original-Host") {
		case "api.github.com":
			switch r.URL.Path {
			case "/repos/acme/skills/commits/main":
				w.Write([]byte("deadbeef"))
			case "/repos/acme/skills/contents/foo":
				writeJSON(w, http.StatusOK, []githubContentEntry{})
			default:
				http.NotFound(w, r)
			}
		case "raw.githubusercontent.com":
			if r.URL.Path == "/acme/skills/main/foo/SKILL.md" {
				w.Write([]byte("---\nname: foo\ndescription: refreshed\n---\nbody"))
				return
			}
			http.NotFound(w, r)
		default:
			http.NotFound(w, r)
		}
	})

	origin := skillOriginRef{Type: "github", SourceURL: "https://github.com/acme/skills/tree/main/foo"}
	imported, err := fetchImportedSkillFromOrigin(t.Context(), client, origin)
	if err != nil {
		t.Fatalf("fetchImportedSkillFromOrigin: %v", err)
	}
	if imported.name != "foo" || imported.description != "refreshed" {
		t.Fatalf("imported = %q / %q, want foo / refreshed", imported.name, imported.description)
	}
	// The fresh origin re-records the re-resolved ref and path.
	if imported.origin["type"] != "github" || imported.origin["ref"] != "main" || imported.origin["path"] != "foo" {
		t.Fatalf("origin = %v, want github/main/foo", imported.origin)
	}
}
