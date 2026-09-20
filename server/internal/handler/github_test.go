package handler

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5/pgtype"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestExtractIdentifiers(t *testing.T) {
	cases := []struct {
		name string
		in   []string
		want []string
	}{
		{
			name: "branch_name",
			in:   []string{"", "", "mul-1510/fix-login"},
			want: []string{"MUL-1510"},
		},
		{
			name: "single_character_prefix",
			in:   []string{"H-412: fix widget parity"},
			want: []string{"H-412"},
		},
		{
			name: "title_and_body",
			in:   []string{"Fix MUL-82", "Closes MUL-1510 and ABC-7", ""},
			want: []string{"MUL-82", "MUL-1510", "ABC-7"},
		},
		{
			name: "dedupe_across_fields",
			in:   []string{"MUL-1", "MUL-1 again", "mul-1/branch"},
			want: []string{"MUL-1"},
		},
		{
			name: "ignore_email_and_versions",
			in:   []string{"reply@user-1 v1.2-3 here", "", ""},
			// Word-boundary regex still matches "user-1"; identifier prefix is
			// any 2..10 letters/digits, so this is intentional. The downstream
			// workspace prefix check in lookupIssueByIdentifier filters it.
			want: []string{"USER-1"},
		},
		{
			name: "no_match",
			in:   []string{"plain text", "no idents", ""},
			want: []string{},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := extractIdentifiers(tc.in...)
			if len(got) == 0 && len(tc.want) == 0 {
				return
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("extractIdentifiers() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestExtractClosingIdentifiers(t *testing.T) {
	cases := []struct {
		name string
		in   []string
		want []string
	}{
		{
			name: "single_closes",
			in:   []string{"", "Closes MUL-1"},
			want: []string{"MUL-1"},
		},
		{
			name: "single_character_prefix",
			in:   []string{"", "Closes H-412"},
			want: []string{"H-412"},
		},
		{
			name: "all_keyword_inflections",
			in: []string{
				"",
				"close MUL-1\nclosed MUL-2\ncloses MUL-3\nfix MUL-4\nfixes MUL-5\nfixed MUL-6\nresolve MUL-7\nresolves MUL-8\nresolved MUL-9",
			},
			want: []string{"MUL-1", "MUL-2", "MUL-3", "MUL-4", "MUL-5", "MUL-6", "MUL-7", "MUL-8", "MUL-9"},
		},
		{
			name: "case_insensitive_and_colon",
			in:   []string{"CLOSES: MUL-1", "Fixes:MUL-2 resolves   MUL-3"},
			want: []string{"MUL-1", "MUL-2", "MUL-3"},
		},
		{
			name: "bare_reference_does_not_close",
			// The bug-report repro: only ABC-1 carries closing intent.
			// ABC-2/ABC-3 are linked (extractIdentifiers) but must not
			// appear in the closing set.
			in:   []string{"ABC-1: Lorem Ipsum", "Closes ABC-1. Follow up work planned in ABC-2. Unblocks ABC-3."},
			want: []string{"ABC-1"},
		},
		{
			name: "keyword_not_adjacent_does_not_close",
			// "Fix login MUL-1" — keyword present but the identifier is
			// not adjacent. Consistent with GitHub's closing-keyword
			// grammar; matches via extractIdentifiers for linking only.
			in:   []string{"Fix login MUL-1", ""},
			want: []string{},
		},
		{
			name: "dedupe_across_fields",
			in:   []string{"Closes MUL-1", "fixes mul-1"},
			want: []string{"MUL-1"},
		},
		{
			name: "no_match_on_disclosed_or_foreclose",
			// Word-boundary guards against keyword fragments embedded
			// in larger words ("Disclosed MUL-1", "Foreclose MUL-1").
			in:   []string{"Disclosed MUL-1 in foreclose MUL-2", ""},
			want: []string{},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := extractClosingIdentifiers(tc.in...)
			if len(got) == 0 && len(tc.want) == 0 {
				return
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("extractClosingIdentifiers() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestDerivePRState(t *testing.T) {
	cases := []struct {
		state  string
		draft  bool
		merged bool
		want   string
	}{
		{"open", false, false, "open"},
		{"open", true, false, "draft"},
		{"closed", false, false, "closed"},
		{"closed", false, true, "merged"},
		{"closed", true, true, "merged"}, // merged trumps draft
	}
	for _, tc := range cases {
		got := derivePRState(tc.state, tc.draft, tc.merged)
		if got != tc.want {
			t.Errorf("derivePRState(%q, draft=%v, merged=%v) = %q, want %q",
				tc.state, tc.draft, tc.merged, got, tc.want)
		}
	}
}

func TestIssuePullRequestResponseHidesUnavailableSnapshot(t *testing.T) {
	fetchedAt := pgtype.Timestamptz{Time: time.Now(), Valid: true}
	row := db.ListPullRequestsByIssueRow{
		State:               "open",
		HeadSha:             "B",
		SnapshotHeadSha:     "A",
		SnapshotFetchedAt:   fetchedAt,
		ApiMergeable:        pgtype.Text{String: "CONFLICTING", Valid: true},
		ApiMergeStateStatus: pgtype.Text{String: "DIRTY", Valid: true},
		ChecksRollupState:   pgtype.Text{String: "FAILURE", Valid: true},
		ChecksTotal:         1,
		ChecksFailed:        1,
		FailedCheckNames:    []string{"backend"},
	}

	// A synchronize webhook moved the row to B while the last stored snapshot
	// still belongs to A. Old data must not be presented as fresh B data.
	resp := issuePullRequestRowToResponse(row, true)
	if resp.SnapshotAvailable == nil || *resp.SnapshotAvailable {
		t.Fatal("mismatched-head snapshot must be marked unavailable")
	}
	if resp.Mergeable != nil || resp.ChecksRollup != nil || resp.ChecksFailed != 0 {
		t.Fatalf("mismatched-head snapshot leaked into response: %+v", resp)
	}

	// Even a current stored snapshot is hidden when no App private key is
	// configured. This covers deployments that disable the feature after data
	// was already written.
	row.SnapshotHeadSha = "B"
	resp = issuePullRequestRowToResponse(row, false)
	if resp.SnapshotAvailable == nil || *resp.SnapshotAvailable {
		t.Fatal("disabled snapshot feature must be marked unavailable")
	}
	if resp.Mergeable != nil || resp.ChecksRollup != nil || resp.ChecksFailed != 0 {
		t.Fatalf("disabled feature exposed last-known snapshot: %+v", resp)
	}

	resp = issuePullRequestRowToResponse(row, true)
	if resp.SnapshotAvailable == nil || !*resp.SnapshotAvailable {
		t.Fatal("enabled current-head snapshot must be available")
	}
	if resp.Mergeable == nil || *resp.Mergeable != "conflicting" || resp.ChecksFailed != 1 {
		t.Fatalf("current snapshot was not exposed: %+v", resp)
	}
}

func TestVerifyWebhookSignature(t *testing.T) {
	secret := "shared-secret"
	body := []byte(`{"action":"opened"}`)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	good := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	if !verifyWebhookSignature(secret, good, body) {
		t.Error("expected valid signature to verify")
	}
	if verifyWebhookSignature(secret, "sha256=deadbeef", body) {
		t.Error("expected bad hex to fail")
	}
	if verifyWebhookSignature(secret, "", body) {
		t.Error("expected empty header to fail")
	}
	if verifyWebhookSignature(secret, "sha1=whatever", body) {
		t.Error("expected non-sha256 prefix to fail")
	}
	if verifyWebhookSignature("other-secret", good, body) {
		t.Error("expected wrong secret to fail")
	}
}

func TestStateRoundTrip(t *testing.T) {
	t.Setenv("GITHUB_WEBHOOK_SECRET", "test-secret-123")
	wsID := "11111111-2222-3333-4444-555555555555"

	tok, err := signState(wsID)
	if err != nil {
		t.Fatalf("signState: %v", err)
	}
	if parts := strings.Split(tok, "."); len(parts) != 3 {
		t.Fatalf("default return state has %d parts, want legacy 3-part format", len(parts))
	}
	got, ok := verifyState(tok)
	if !ok {
		t.Fatal("verifyState rejected a freshly-signed token")
	}
	if got != wsID {
		t.Errorf("verifyState() = %q, want %q", got, wsID)
	}

	// Tampering with the workspace portion must fail (signature is bound
	// to it). Replace the leading UUID's first hex digit.
	tampered := "01111111" + tok[8:]
	if _, ok := verifyState(tampered); ok {
		t.Error("tampered state token should fail to verify")
	}

	// Wrong secret rejects.
	t.Setenv("GITHUB_WEBHOOK_SECRET", "different")
	if _, ok := verifyState(tok); ok {
		t.Error("token signed with old secret should fail under a new one")
	}
}

func TestStateRoundTripWithRepositoryReturnTarget(t *testing.T) {
	t.Setenv("GITHUB_WEBHOOK_SECRET", "test-secret-123")
	wsID := "11111111-2222-3333-4444-555555555555"

	tok, err := signStateForReturn(wsID, githubReturnToRepositories)
	if err != nil {
		t.Fatalf("signStateForReturn: %v", err)
	}
	if parts := strings.Split(tok, "."); len(parts) != 4 {
		t.Fatalf("repository return state has %d parts, want 4", len(parts))
	}
	gotWorkspaceID, gotReturnTo, ok := verifyStateWithReturn(tok)
	if !ok {
		t.Fatal("verifyStateWithReturn rejected a freshly-signed token")
	}
	if gotWorkspaceID != wsID || gotReturnTo != githubReturnToRepositories {
		t.Errorf(
			"verifyStateWithReturn() = (%q, %q), want (%q, %q)",
			gotWorkspaceID,
			gotReturnTo,
			wsID,
			githubReturnToRepositories,
		)
	}

	tampered := strings.Replace(tok, ".repositories.", ".github.", 1)
	if _, _, ok := verifyStateWithReturn(tampered); ok {
		t.Error("tampered return target should fail verification")
	}
}

func TestGitHubConnectRepositoryReturnTarget(t *testing.T) {
	t.Setenv("GITHUB_APP_SLUG", "multica-test")
	t.Setenv("GITHUB_WEBHOOK_SECRET", "test-secret-123")
	wsID := "11111111-2222-3333-4444-555555555555"

	req := httptest.NewRequest(
		http.MethodGet,
		"/api/workspaces/"+wsID+"/github/connect?return_to=repositories",
		nil,
	)
	req = withURLParam(req, "id", wsID)
	rec := httptest.NewRecorder()
	(&Handler{}).GitHubConnect(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GitHubConnect: got %d (%s)", rec.Code, rec.Body.String())
	}
	var body GitHubConnectResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode connect response: %v", err)
	}
	installURL, err := url.Parse(body.URL)
	if err != nil {
		t.Fatalf("parse install URL: %v", err)
	}
	_, returnTo, ok := verifyStateWithReturn(installURL.Query().Get("state"))
	if !ok || returnTo != githubReturnToRepositories {
		t.Fatalf("signed return target = %q, valid=%v, want repositories", returnTo, ok)
	}

	badReq := httptest.NewRequest(
		http.MethodGet,
		"/api/workspaces/"+wsID+"/github/connect?return_to=https://evil.example",
		nil,
	)
	badReq = withURLParam(badReq, "id", wsID)
	badRec := httptest.NewRecorder()
	(&Handler{}).GitHubConnect(badRec, badReq)
	if badRec.Code != http.StatusBadRequest {
		t.Fatalf("invalid return target: got %d, want 400", badRec.Code)
	}
}

func TestGitHubSetupCallbackRepositoryReturnTarget(t *testing.T) {
	t.Setenv("GITHUB_WEBHOOK_SECRET", "test-secret-123")
	t.Setenv("FRONTEND_ORIGIN", "https://app.multica.test/")
	wsID := "11111111-2222-3333-4444-555555555555"
	state, err := signStateForReturn(wsID, githubReturnToRepositories)
	if err != nil {
		t.Fatalf("signStateForReturn: %v", err)
	}

	req := httptest.NewRequest(
		http.MethodGet,
		"/api/github/setup?installation_id=not-a-number&state="+url.QueryEscape(state),
		nil,
	)
	rec := httptest.NewRecorder()
	(&Handler{}).GitHubSetupCallback(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("GitHubSetupCallback: got %d, want 302", rec.Code)
	}
	if got := rec.Header().Get("Location"); got != "https://app.multica.test/settings?tab=repositories&github_error=bad_installation_id" {
		t.Fatalf("redirect = %q, want repository settings error", got)
	}
}

func TestSignStateRequiresSecret(t *testing.T) {
	t.Setenv("GITHUB_WEBHOOK_SECRET", "")
	if _, err := signState("ws"); err == nil {
		t.Error("signState should error when secret is unset")
	}
}

// ── CI / mergeable_state tests ─────────────────────────────────────────────

func TestDerivePRMergeableState(t *testing.T) {
	cases := []struct {
		name           string
		action         string
		payload        string
		baseRefChanged bool
		wantValid      bool
		wantStr        string
		wantClear      bool
	}{
		{"opened_clears", "opened", "clean", false, false, "", true},
		{"synchronize_clears", "synchronize", "clean", false, false, "", true},
		{"reopened_clears", "reopened", "dirty", false, false, "", true},
		{"edited_base_changed_clears", "edited", "clean", true, false, "", true},
		{"edited_title_only_keeps_value", "edited", "clean", false, true, "clean", false},
		{"labeled_keeps_value", "labeled", "clean", false, true, "clean", false},
		{"labeled_empty_payload_preserves", "labeled", "", false, false, "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, clear := derivePRMergeableState(tc.action, tc.payload, tc.baseRefChanged)
			if got.Valid != tc.wantValid {
				t.Errorf("Valid=%v want %v", got.Valid, tc.wantValid)
			}
			if got.String != tc.wantStr {
				t.Errorf("String=%q want %q", got.String, tc.wantStr)
			}
			if clear != tc.wantClear {
				t.Errorf("clear=%v want %v", clear, tc.wantClear)
			}
		})
	}
}

// TestGitHubInstallationBroadcastRedaction guards Emacs' finding on PR #2886:
// the realtime payloads we publish on installation create / uninstall must
// not carry the numeric `installation_id`. The frontend uses these events
// only to invalidate the installations query, so an admin client recovers
// the management handle via the list endpoint — which already gates the
// numeric id by role.
func TestGitHubInstallationBroadcastRedaction(t *testing.T) {
	inst := db.GithubInstallation{
		InstallationID: 123456789,
		AccountLogin:   "broadcast-acct",
		AccountType:    "User",
	}
	got := githubInstallationToBroadcast(inst)
	if got.InstallationID != nil {
		t.Errorf("broadcast payload must omit installation_id, got %v", *got.InstallationID)
	}
	if got.AccountLogin != "broadcast-acct" {
		t.Errorf("expected account_login preserved, got %q", got.AccountLogin)
	}

	// Sanity: the JSON encoding actually drops the field (omitempty + nil
	// pointer). A future change to the response shape could re-introduce
	// the field through a different name; the JSON check is the real
	// assertion against the wire format clients see.
	raw, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal broadcast payload: %v", err)
	}
	var generic map[string]any
	if err := json.Unmarshal(raw, &generic); err != nil {
		t.Fatalf("unmarshal broadcast payload: %v", err)
	}
	if _, present := generic["installation_id"]; present {
		t.Errorf("installation_id leaked into broadcast JSON: %s", string(raw))
	}
}

// generateTestRSAKeyPEM mints an RSA-2048 key, returns its PKCS#1 PEM
// encoding (the format GitHub hands operators when they create the App)
// and the parsed *rsa.PrivateKey for verification.
func generateTestRSAKeyPEM(t *testing.T) (pemBytes []byte, key *rsa.PrivateKey) {
	t.Helper()
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate RSA key: %v", err)
	}
	der := x509.MarshalPKCS1PrivateKey(k)
	return pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: der}), k
}

// TestSignGitHubAppJWT_NotConfigured pins the contract that missing env
// vars produce ("", nil) — a soft "App auth not available" signal that
// fetchInstallationAccount uses to fall through to its unauthenticated
// path. Returning an error here would force every install on a vanilla
// self-host to log a noisy warning even though the deployment is
// intentionally not running App-authenticated calls.
func TestSignGitHubAppJWT_NotConfigured(t *testing.T) {
	t.Setenv("GITHUB_APP_ID", "")
	t.Setenv("GITHUB_APP_PRIVATE_KEY", "")
	tok, err := signGitHubAppJWT(time.Now())
	if err != nil {
		t.Fatalf("expected nil error when env not set, got %v", err)
	}
	if tok != "" {
		t.Errorf("expected empty token when env not set, got %q", tok)
	}

	// Half-configured (one var set, the other empty) is treated the same
	// as fully unset — we never want a partial config to claim the App
	// is wired up.
	t.Setenv("GITHUB_APP_ID", "12345")
	t.Setenv("GITHUB_APP_PRIVATE_KEY", "")
	tok, err = signGitHubAppJWT(time.Now())
	if err != nil || tok != "" {
		t.Errorf("partial config should return empty token, got tok=%q err=%v", tok, err)
	}
}

// TestSignGitHubAppJWT_InvalidPEM proves that a malformed private key is
// surfaced as an error, not silently swallowed. The setup-callback path
// catches and logs this so the operator gets a breadcrumb instead of an
// install that quietly never enriches the row.
func TestSignGitHubAppJWT_InvalidPEM(t *testing.T) {
	t.Setenv("GITHUB_APP_ID", "12345")
	t.Setenv("GITHUB_APP_PRIVATE_KEY", "not a real PEM block")
	if _, err := signGitHubAppJWT(time.Now()); err == nil {
		t.Error("expected error for malformed private key, got nil")
	}
}

// TestSignGitHubAppJWT_ClaimsAndSignature signs a token with a known key
// and verifies (a) the claims GitHub requires (`iss`, `iat`, `exp`) carry
// the values we set, (b) iat is back-dated for clock skew, (c) exp stays
// inside GitHub's 10-minute cap, and (d) the signature verifies against
// the matching public key.
func TestSignGitHubAppJWT_ClaimsAndSignature(t *testing.T) {
	pemBytes, key := generateTestRSAKeyPEM(t)
	t.Setenv("GITHUB_APP_ID", "424242")
	t.Setenv("GITHUB_APP_PRIVATE_KEY", string(pemBytes))

	now := time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC)
	tok, err := signGitHubAppJWT(now)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	if tok == "" {
		t.Fatal("expected non-empty token when fully configured")
	}

	// Inject the same `now` into the parser's clock so default exp/nbf
	// validation is anchored to the test-time, not real wall clock —
	// otherwise the test becomes a time bomb that fails for real once
	// the real time crosses the token's exp (now + 9m).
	parsed, err := jwt.Parse(
		tok,
		func(token *jwt.Token) (any, error) {
			if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
			}
			return &key.PublicKey, nil
		},
		jwt.WithTimeFunc(func() time.Time { return now }),
	)
	if err != nil || !parsed.Valid {
		t.Fatalf("verify token: err=%v valid=%v", err, parsed != nil && parsed.Valid)
	}
	claims, ok := parsed.Claims.(jwt.MapClaims)
	if !ok {
		t.Fatalf("claims type: %T", parsed.Claims)
	}
	if got, _ := claims["iss"].(string); got != "424242" {
		t.Errorf("iss = %q, want 424242", got)
	}
	iat := int64(claims["iat"].(float64))
	exp := int64(claims["exp"].(float64))
	if iat != now.Add(-60*time.Second).Unix() {
		t.Errorf("iat = %d, want %d (now - 60s for clock skew)", iat, now.Add(-60*time.Second).Unix())
	}
	if exp != now.Add(9*time.Minute).Unix() {
		t.Errorf("exp = %d, want %d (now + 9m, inside GitHub's 10m cap)", exp, now.Add(9*time.Minute).Unix())
	}
	if exp-iat > int64(10*time.Minute/time.Second) {
		t.Errorf("exp-iat = %d s, exceeds GitHub's 10m max", exp-iat)
	}
}

func TestFetchGitHubInstallationRepositories(t *testing.T) {
	pemBytes, key := generateTestRSAKeyPEM(t)
	t.Setenv("GITHUB_APP_ID", "424242")
	t.Setenv("GITHUB_APP_PRIVATE_KEY", string(pemBytes))

	const installationID int64 = 314159
	var tokenRevoked bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/app/installations/314159/access_tokens":
			bearer := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
			if bearer == "" {
				http.Error(w, "missing app jwt", http.StatusUnauthorized)
				return
			}
			if _, err := jwt.Parse(bearer, func(token *jwt.Token) (any, error) {
				return &key.PublicKey, nil
			}); err != nil {
				http.Error(w, "bad app jwt", http.StatusUnauthorized)
				return
			}
			var tokenRequest struct {
				Permissions map[string]string `json:"permissions"`
			}
			if err := json.NewDecoder(r.Body).Decode(&tokenRequest); err != nil {
				http.Error(w, "bad token request", http.StatusBadRequest)
				return
			}
			if !reflect.DeepEqual(tokenRequest.Permissions, map[string]string{"metadata": "read"}) {
				http.Error(w, "overbroad token permissions", http.StatusBadRequest)
				return
			}
			writeJSON(w, http.StatusCreated, map[string]any{"token": "installation-secret"})
		case r.Method == http.MethodGet && r.URL.Path == "/installation/repositories":
			if got := r.Header.Get("Authorization"); got != "Bearer installation-secret" {
				http.Error(w, "bad installation token", http.StatusUnauthorized)
				return
			}
			if r.URL.Query().Get("page") != "2" || r.URL.Query().Get("per_page") != "1" {
				http.Error(w, "bad pagination", http.StatusBadRequest)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{
				"total_count": 3,
				"repositories": []map[string]any{{
					"id":             9,
					"full_name":      "acme/private-repo",
					"html_url":       "https://github.com/acme/private-repo",
					"clone_url":      "https://github.com/acme/private-repo.git",
					"description":    "Private repository",
					"private":        true,
					"archived":       false,
					"default_branch": "main",
				}},
			})
		case r.Method == http.MethodDelete && r.URL.Path == "/installation/token":
			tokenRevoked = r.Header.Get("Authorization") == "Bearer installation-secret"
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	oldBase := githubAPIBase
	githubAPIBase = srv.URL
	t.Cleanup(func() { githubAPIBase = oldBase })

	got, err := fetchGitHubInstallationRepositories(
		context.Background(),
		installationID,
		2,
		1,
	)
	if err != nil {
		t.Fatalf("fetchGitHubInstallationRepositories: %v", err)
	}
	if len(got.Repositories) != 1 {
		t.Fatalf("repositories = %d, want 1", len(got.Repositories))
	}
	repository := got.Repositories[0]
	if repository.FullName != "acme/private-repo" || !repository.Private {
		t.Errorf("repository = %+v, want mapped private repository", repository)
	}
	if got.TotalCount != 3 || got.NextPage == nil || *got.NextPage != 3 {
		t.Errorf("pagination = total %d, next %v; want total 3, next 3", got.TotalCount, got.NextPage)
	}
	if !tokenRevoked {
		t.Error("installation token was not revoked after repository listing")
	}
}

// TestFetchInstallationAccount_AuthenticatedPopulatesRow simulates the
// GitHub `/app/installations/{id}` endpoint with a JWT-gated mock and
// verifies that fetchInstallationAccount, when fully configured,
// (a) sends a Bearer JWT, (b) parses the JSON response, and (c) returns
// the real account login instead of the "unknown" placeholder. This is
// the assertion that nails down the bug fix for MUL-3078.
func TestFetchInstallationAccount_AuthenticatedPopulatesRow(t *testing.T) {
	pemBytes, key := generateTestRSAKeyPEM(t)
	t.Setenv("GITHUB_APP_ID", "11111")
	t.Setenv("GITHUB_APP_PRIVATE_KEY", string(pemBytes))

	const wantInstallationID int64 = 7777777
	var sawAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawAuth = r.Header.Get("Authorization")
		expectedPath := fmt.Sprintf("/app/installations/%d", wantInstallationID)
		if r.URL.Path != expectedPath {
			t.Errorf("unexpected path: got %q want %q", r.URL.Path, expectedPath)
		}
		// Verify JWT signature using the matching public key — this is
		// what GitHub does on the real endpoint.
		bearer := strings.TrimPrefix(sawAuth, "Bearer ")
		if bearer == sawAuth {
			http.Error(w, "missing Bearer prefix", http.StatusUnauthorized)
			return
		}
		if _, err := jwt.Parse(bearer, func(token *jwt.Token) (any, error) {
			return &key.PublicKey, nil
		}); err != nil {
			http.Error(w, "bad jwt: "+err.Error(), http.StatusUnauthorized)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"account": map[string]any{
				"login":      "octocat",
				"type":       "Organization",
				"avatar_url": "https://example.com/o.png",
			},
		})
	}))
	t.Cleanup(srv.Close)

	oldBase := githubAPIBase
	githubAPIBase = srv.URL
	t.Cleanup(func() { githubAPIBase = oldBase })

	login, accountType, avatar := fetchInstallationAccount(context.Background(), wantInstallationID)
	if login != "octocat" {
		t.Errorf("login = %q, want %q (the bug repro: stayed as 'unknown' before the fix)", login, "octocat")
	}
	if accountType != "Organization" {
		t.Errorf("accountType = %q, want Organization", accountType)
	}
	if avatar == nil || *avatar != "https://example.com/o.png" {
		t.Errorf("avatar = %v, want pointer to https://example.com/o.png", avatar)
	}
	if !strings.HasPrefix(sawAuth, "Bearer ") {
		t.Errorf("expected Bearer auth header, got %q", sawAuth)
	}
}

// TestFetchInstallationAccount_UnauthenticatedFallsBack documents the
// degraded path: when the operator hasn't set GITHUB_APP_ID/PRIVATE_KEY,
// the call is made unauthenticated, GitHub returns 401, and the function
// returns the "unknown" placeholder. This is the input the webhook then
// upserts over once GitHub delivers `installation.created`.
func TestFetchInstallationAccount_UnauthenticatedFallsBack(t *testing.T) {
	t.Setenv("GITHUB_APP_ID", "")
	t.Setenv("GITHUB_APP_PRIVATE_KEY", "")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			http.Error(w, "auth required", http.StatusUnauthorized)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"account": map[string]any{"login": "should-not-see"}})
	}))
	t.Cleanup(srv.Close)
	oldBase := githubAPIBase
	githubAPIBase = srv.URL
	t.Cleanup(func() { githubAPIBase = oldBase })

	login, _, _ := fetchInstallationAccount(context.Background(), 999)
	if login != "unknown" {
		t.Errorf("login = %q, want unknown placeholder when auth not configured", login)
	}
}

// TestFetchInstallationAccount_EmptyAccountKeepsPlaceholder pins that a 200
// response with a missing `account.login` (e.g. GitHub returned a partial
// payload) still yields the safe "unknown" placeholder rather than writing
// an empty string — the frontend renders the literal value, so an empty
// string would surface as "已连接到 " (the bug we're fixing, in a different
// shape).
func TestFetchInstallationAccount_EmptyAccountKeepsPlaceholder(t *testing.T) {
	pemBytes, _ := generateTestRSAKeyPEM(t)
	t.Setenv("GITHUB_APP_ID", "1")
	t.Setenv("GITHUB_APP_PRIVATE_KEY", string(pemBytes))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"account": map[string]any{}})
	}))
	t.Cleanup(srv.Close)
	oldBase := githubAPIBase
	githubAPIBase = srv.URL
	t.Cleanup(func() { githubAPIBase = oldBase })

	login, accountType, avatar := fetchInstallationAccount(context.Background(), 12)
	if login != "unknown" {
		t.Errorf("expected 'unknown' placeholder for empty account.login, got %q", login)
	}
	if accountType != "User" {
		t.Errorf("expected default 'User' accountType, got %q", accountType)
	}
	if avatar != nil {
		t.Errorf("expected nil avatar, got %v", *avatar)
	}
}

// TestCloseIntentPolicyPermits pins the fail-closed invariant at the type
// level: the policy is an allowlist, so anything it was not able to prove is
// denied. The zero value is what every indeterminate read in
// resolveCloseIntentPolicy returns, and it must permit nothing.
func TestCloseIntentPolicyPermits(t *testing.T) {
	const wsA, wsB = "workspace-a", "workspace-b"

	for _, tc := range []struct {
		name   string
		policy closeIntentPolicy
		ws     string
		want   bool
	}{
		{
			name:   "zero value denies (what an indeterminate read returns)",
			policy: closeIntentPolicy{},
			ws:     wsA,
			want:   false,
		},
		{
			name:   "single-binding delivery is unrestricted",
			policy: closeIntentPolicy{unrestricted: true},
			ws:     wsA,
			want:   true,
		},
		{
			name:   "recorded owner may act",
			policy: closeIntentPolicy{owner: map[string]string{"ABC-100": wsA}},
			ws:     wsA,
			want:   true,
		},
		{
			// A workspace that grew a same-numbered issue after the scan is not
			// the recorded owner, so it still cannot act.
			name:   "workspace that is not the recorded owner may not act",
			policy: closeIntentPolicy{owner: map[string]string{"ABC-100": wsA}},
			ws:     wsB,
			want:   false,
		},
		{
			name:   "identifier absent from the allowlist is denied",
			policy: closeIntentPolicy{owner: map[string]string{"ABC-1": wsA}},
			ws:     wsA,
			want:   false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.policy.permits("ABC-100", tc.ws); got != tc.want {
				t.Errorf("permits(ABC-100, %s) = %v, want %v", tc.ws, got, tc.want)
			}
		})
	}
}
