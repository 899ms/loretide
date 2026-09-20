//go:build dbtest

package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/multica-ai/multica/server/internal/storage"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// createHandlerTestChatSession seeds a chat_session row owned by testUserID
// targeting the given agent and returns the session UUID. Cleanup runs after
// the test. Used by attachment / chat tests that need an existing session.
func createHandlerTestChatSession(t *testing.T, agentID string) string {
	t.Helper()

	var sessionID string
	if err := testPool.QueryRow(context.Background(), `
		INSERT INTO chat_session (
			workspace_id, agent_id, creator_id, title, status, explicitly_created_at
		)
		VALUES ($1, $2, $3, $4, 'active', now())
		RETURNING id
	`, testWorkspaceID, agentID, testUserID, "Handler Test Chat Session").Scan(&sessionID); err != nil {
		t.Fatalf("failed to create handler test chat session: %v", err)
	}
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM chat_session WHERE id = $1`, sessionID)
	})
	return sessionID
}

func TestUploadFileForeignWorkspace(t *testing.T) {
	origStorage := testHandler.Storage
	testHandler.Storage = &mockStorage{}
	defer func() { testHandler.Storage = origStorage }()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "test.txt")
	if err != nil {
		t.Fatal(err)
	}
	part.Write([]byte("hello world"))
	writer.Close()

	foreignWorkspaceID := "00000000-0000-0000-0000-000000000099"
	req := httptest.NewRequest("POST", "/api/upload-file", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("X-User-ID", testUserID)
	req.Header.Set("X-Workspace-ID", foreignWorkspaceID)

	w := httptest.NewRecorder()
	testHandler.UploadFile(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("UploadFile with foreign workspace: expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

// TestUploadFileResolvesWorkspaceViaSlugHeader is a regression test for the
// v2 workspace URL refactor (#1141). The frontend switched from sending
// X-Workspace-ID (UUID) to X-Workspace-Slug. For endpoints that sit outside
// the workspace middleware — like /api/upload-file — the handler-side
// resolver must accept the slug and translate it to a UUID, otherwise the
// handler silently falls through to the "no workspace context" branch and
// skips creating the DB attachment record. Files end up in S3 with no row
// in the attachment table, invisible to the UI.
func TestUploadFileResolvesWorkspaceViaSlugHeader(t *testing.T) {
	origStorage := testHandler.Storage
	testHandler.Storage = &mockStorage{}
	defer func() { testHandler.Storage = origStorage }()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "slug-upload.txt")
	if err != nil {
		t.Fatal(err)
	}
	part.Write([]byte("hello via slug"))
	writer.Close()

	req := httptest.NewRequest("POST", "/api/upload-file", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("X-User-ID", testUserID)
	// Intentionally NOT setting X-Workspace-ID — post-v2 clients only send slug.
	req.Header.Set("X-Workspace-Slug", handlerTestWorkspaceSlug)

	w := httptest.NewRecorder()
	testHandler.UploadFile(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("UploadFile with slug header: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// The workspace-aware branch returns the full AttachmentResponse (with
	// id, workspace_id, uploader, etc.). The no-workspace-context branch
	// returns only {filename, link}. Distinguish by checking the shape.
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v; body: %s", err, w.Body.String())
	}
	if _, ok := resp["id"]; !ok {
		t.Fatalf("expected attachment response with 'id' field (DB row created); got fallback link-only response: %s", w.Body.String())
	}
	if gotWs, _ := resp["workspace_id"].(string); gotWs != testWorkspaceID {
		t.Fatalf("attachment workspace_id mismatch: want %s, got %v", testWorkspaceID, resp["workspace_id"])
	}

	// Verify the row actually exists in the database.
	var count int
	if err := testPool.QueryRow(
		context.Background(),
		`SELECT count(*) FROM attachment WHERE workspace_id = $1 AND filename = $2`,
		testWorkspaceID,
		"slug-upload.txt",
	).Scan(&count); err != nil {
		t.Fatalf("query attachment count: %v", err)
	}
	if count != 1 {
		t.Fatalf("attachment row count: want 1, got %d", count)
	}

	// Clean up so reruns don't accumulate rows.
	if _, err := testPool.Exec(
		context.Background(),
		`DELETE FROM attachment WHERE workspace_id = $1 AND filename = $2`,
		testWorkspaceID,
		"slug-upload.txt",
	); err != nil {
		t.Fatalf("cleanup attachment: %v", err)
	}
}

// TestUploadFileResolvesWorkspaceViaIDHeaderStill confirms the legacy path
// (CLI / daemon clients sending X-Workspace-ID as a UUID) still works after
// the refactor. Prevents a regression in the CLI/daemon compat branch.
func TestUploadFileResolvesWorkspaceViaIDHeaderStill(t *testing.T) {
	origStorage := testHandler.Storage
	testHandler.Storage = &mockStorage{}
	defer func() { testHandler.Storage = origStorage }()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "uuid-upload.txt")
	if err != nil {
		t.Fatal(err)
	}
	part.Write([]byte("hello via uuid"))
	writer.Close()

	req := httptest.NewRequest("POST", "/api/upload-file", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("X-User-ID", testUserID)
	req.Header.Set("X-Workspace-ID", testWorkspaceID)

	w := httptest.NewRecorder()
	testHandler.UploadFile(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("UploadFile with UUID header: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Clean up.
	if _, err := testPool.Exec(
		context.Background(),
		`DELETE FROM attachment WHERE workspace_id = $1 AND filename = $2`,
		testWorkspaceID,
		"uuid-upload.txt",
	); err != nil {
		t.Fatalf("cleanup attachment: %v", err)
	}
}

// TestUploadFile_AttachesToChatSession verifies that a multipart upload with
// a chat_session_id form field creates an attachment row linked to that chat
// session (chat_message_id remains NULL — it is back-filled on send).
func TestUploadFile_AttachesToChatSession(t *testing.T) {
	origStorage := testHandler.Storage
	testHandler.Storage = &mockStorage{}
	defer func() { testHandler.Storage = origStorage }()

	agentID := createHandlerTestAgent(t, "ChatUploadAgent", []byte("[]"))
	sessionID := createHandlerTestChatSession(t, agentID)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "chat-upload.png")
	if err != nil {
		t.Fatal(err)
	}
	// Minimal PNG signature so content-type sniffs as image/png.
	part.Write([]byte("\x89PNG\r\n\x1a\nrest-of-bytes"))
	if err := writer.WriteField("chat_session_id", sessionID); err != nil {
		t.Fatal(err)
	}
	writer.Close()

	req := httptest.NewRequest("POST", "/api/upload-file", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("X-User-ID", testUserID)
	req.Header.Set("X-Workspace-ID", testWorkspaceID)

	w := httptest.NewRecorder()
	testHandler.UploadFile(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("UploadFile with chat_session_id: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp AttachmentResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v; body: %s", err, w.Body.String())
	}
	if resp.ChatSessionID == nil || *resp.ChatSessionID != sessionID {
		t.Fatalf("chat_session_id in response: want %s, got %v", sessionID, resp.ChatSessionID)
	}
	if resp.ChatMessageID != nil {
		t.Fatalf("chat_message_id should be NULL before send, got %v", resp.ChatMessageID)
	}
	if resp.IssueID != nil || resp.CommentID != nil {
		t.Fatalf("issue_id/comment_id should be NULL for chat-only upload: %+v", resp)
	}
	if resp.URL == "" {
		t.Fatal("expected non-empty url")
	}

	// Verify the DB row directly.
	var dbSession, dbMessage *string
	if err := testPool.QueryRow(
		context.Background(),
		`SELECT chat_session_id::text, chat_message_id::text FROM attachment WHERE id = $1`,
		resp.ID,
	).Scan(&dbSession, &dbMessage); err != nil {
		t.Fatalf("query attachment row: %v", err)
	}
	if dbSession == nil || *dbSession != sessionID {
		t.Fatalf("DB chat_session_id mismatch: want %s, got %v", sessionID, dbSession)
	}
	if dbMessage != nil {
		t.Fatalf("DB chat_message_id should be NULL, got %v", dbMessage)
	}

	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM attachment WHERE id = $1`, resp.ID)
	})
}

// TestUploadFile_RejectsForeignChatSession verifies a chat_session in another
// workspace (or owned by another user) is rejected with 403/404, preventing
// cross-tenant attachment binding.
func TestUploadFile_RejectsForeignChatSession(t *testing.T) {
	origStorage := testHandler.Storage
	testHandler.Storage = &mockStorage{}
	defer func() { testHandler.Storage = origStorage }()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, _ := writer.CreateFormFile("file", "evil.txt")
	part.Write([]byte("payload"))
	// Random non-existent UUID.
	writer.WriteField("chat_session_id", "00000000-0000-0000-0000-0000deadbeef")
	writer.Close()

	req := httptest.NewRequest("POST", "/api/upload-file", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("X-User-ID", testUserID)
	req.Header.Set("X-Workspace-ID", testWorkspaceID)

	w := httptest.NewRecorder()
	testHandler.UploadFile(w, req)
	if w.Code != http.StatusNotFound && w.Code != http.StatusForbidden && w.Code != http.StatusBadRequest {
		t.Fatalf("UploadFile with unknown chat_session_id: expected 4xx, got %d: %s", w.Code, w.Body.String())
	}
}

// seedPreviewAttachment inserts an attachment row + writes the bytes into the
// active mockStorage. Returns the new attachment id. Caller is responsible for
// installing the mockStorage on testHandler before calling.
func seedPreviewAttachment(t *testing.T, store *mockStorage, key, filename, contentType string, body []byte) string {
	t.Helper()
	// Register the body so GetReader can find it via KeyFromURL → key.
	url, err := store.Upload(context.Background(), key, body, contentType, filename)
	if err != nil {
		t.Fatalf("seed Upload: %v", err)
	}

	var id string
	if err := testPool.QueryRow(context.Background(), `
		INSERT INTO attachment (workspace_id, uploader_type, uploader_id, filename, url, content_type, size_bytes)
		VALUES ($1, 'member', $2, $3, $4, $5, $6)
		RETURNING id::text
	`, testWorkspaceID, testUserID, filename, url, contentType, len(body)).Scan(&id); err != nil {
		t.Fatalf("seed attachment row: %v", err)
	}
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM attachment WHERE id = $1`, id)
	})
	return id
}

func seedAttachmentURL(t *testing.T, rawURL, filename, contentType string, sizeBytes int64) string {
	t.Helper()
	var id string
	if err := testPool.QueryRow(context.Background(), `
		INSERT INTO attachment (workspace_id, uploader_type, uploader_id, filename, url, content_type, size_bytes)
		VALUES ($1, 'member', $2, $3, $4, $5, $6)
		RETURNING id::text
	`, testWorkspaceID, testUserID, filename, rawURL, contentType, sizeBytes).Scan(&id); err != nil {
		t.Fatalf("seed attachment row: %v", err)
	}
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM attachment WHERE id = $1`, id)
	})
	return id
}

func newPreviewRequest(t *testing.T, attachmentID, workspaceID string) (*http.Request, *httptest.ResponseRecorder) {
	t.Helper()
	req := httptest.NewRequest("GET", "/api/attachments/"+attachmentID+"/content", nil)
	req.Header.Set("X-User-ID", testUserID)
	req.Header.Set("X-Workspace-ID", workspaceID)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", attachmentID)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	return req, httptest.NewRecorder()
}

func newDownloadRequest(t *testing.T, attachmentID, workspaceID string) (*http.Request, *httptest.ResponseRecorder) {
	t.Helper()
	req := httptest.NewRequest("GET", "/api/attachments/"+attachmentID+"/download", nil)
	req.Header.Set("X-User-ID", testUserID)
	req.Header.Set("X-Workspace-ID", workspaceID)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", attachmentID)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	return req, httptest.NewRecorder()
}

func newDownloadRouter() http.Handler {
	// Mirrors the production router after MUL-3130: the download
	// route is registered under Auth-only with no
	// RequireWorkspaceMember wrapper. The handler self-resolves the
	// workspace from the attachment row and enforces membership
	// internally, so a native browser <img>/<video> resource load
	// with no X-Workspace-* headers is the supported call shape.
	r := chi.NewRouter()
	r.Get("/api/attachments/{id}/download", testHandler.DownloadAttachment)
	return r
}

func TestAttachmentToResponse_NonCloudFrontUsesDownloadEndpoint(t *testing.T) {
	origSigner := testHandler.CFSigner
	testHandler.CFSigner = nil
	t.Cleanup(func() { testHandler.CFSigner = origSigner })

	id := seedAttachmentURL(t, "http://rustfs:9000/test-bucket/private.txt", "private.txt", "text/plain", 5)
	att, err := testHandler.Queries.GetAttachment(context.Background(), db.GetAttachmentParams{
		ID:          parseUUID(id),
		WorkspaceID: parseUUID(testWorkspaceID),
	})
	if err != nil {
		t.Fatalf("GetAttachment: %v", err)
	}

	resp := testHandler.attachmentToResponse(att, attachmentURLModeSigned)
	if resp.URL != "http://rustfs:9000/test-bucket/private.txt" {
		t.Fatalf("stored url changed: %q", resp.URL)
	}
	if resp.DownloadURL != "/api/attachments/"+id+"/download" {
		t.Fatalf("download_url = %q, want unified endpoint", resp.DownloadURL)
	}
}

func TestGetAttachmentByID_AutoPublicEndpointReturnsPresignedDownloadURL(t *testing.T) {
	store := &mockStorageNoCdn{}
	origStorage := testHandler.Storage
	origCfg := testHandler.cfg
	origSigner := testHandler.CFSigner
	testHandler.Storage = store
	testHandler.cfg.AttachmentDownloadMode = "auto"
	testHandler.CFSigner = nil
	t.Cleanup(func() {
		testHandler.Storage = origStorage
		testHandler.cfg = origCfg
		testHandler.CFSigner = origSigner
	})

	key := "downloads/desktop-inline.png"
	id := seedAttachmentURL(
		t,
		"https://s3.example.com/test-bucket/"+key,
		"desktop-inline.png",
		"image/png",
		10,
	)

	req := httptest.NewRequest("GET", "/api/attachments/"+id, nil)
	req.Header.Set("X-User-ID", testUserID)
	req.Header.Set("X-Workspace-ID", testWorkspaceID)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", id)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	w := httptest.NewRecorder()

	testHandler.GetAttachmentByID(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	var resp AttachmentResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v; body=%s", err, w.Body.String())
	}
	parsed, err := url.Parse(resp.DownloadURL)
	if err != nil {
		t.Fatalf("parse download_url: %v", err)
	}
	if got := parsed.Query().Get("X-Amz-Signature"); got != "mock" {
		t.Fatalf("download_url = %q, want S3 presigned URL", resp.DownloadURL)
	}
	if got := parsed.Query().Get("response-content-disposition"); got != "" {
		t.Fatalf("response-content-disposition = %q, want inline-loadable URL", got)
	}
	if want := "/api/attachments/" + id + "/download"; resp.MarkdownURL != want {
		t.Fatalf("markdown_url = %q, want stable URL %q", resp.MarkdownURL, want)
	}
	// The single-attachment endpoint presigns the object twice: once inline for
	// download_url (asserted above) and once with a forced attachment disposition
	// for attachment_download_url.
	if len(store.presignCalls) != 2 || store.presignCalls[0] != key || store.presignCalls[1] != key {
		t.Fatalf("presign calls = %v, want [%s %s]", store.presignCalls, key, key)
	}
	dl, err := url.Parse(resp.AttachmentDownloadURL)
	if err != nil {
		t.Fatalf("parse attachment_download_url: %v", err)
	}
	if got := dl.Query().Get("X-Amz-Signature"); got != "mock" {
		t.Fatalf("attachment_download_url = %q, want an S3 presigned URL", resp.AttachmentDownloadURL)
	}
	if got := dl.Query().Get("response-content-disposition"); !strings.HasPrefix(got, "attachment") {
		t.Fatalf("attachment_download_url response-content-disposition = %q, want a forced attachment disposition", got)
	}
}

// TestGetAttachmentByID_CloudFrontModeSignsForcedAttachmentDownloadURL pins the
// CloudFront arm of GetAttachmentByID's download-URL switch — the one storage
// mode still unexercised at this layer. It asserts attachment_download_url is a
// CloudFront-signed URL carrying response-content-disposition=attachment, which
// SignedURLWithContentDisposition sets on the URL BEFORE signing, so the
// disposition is folded into the signed Resource (a client cannot strip or alter
// it without invalidating the Signature — that property is unit-tested in
// cloudfront_test.go). It also asserts the load-intent download_url sibling does
// NOT force an attachment, so the two intents stay distinct.
func TestGetAttachmentByID_CloudFrontModeSignsForcedAttachmentDownloadURL(t *testing.T) {
	origStorage := testHandler.Storage
	origCfg := testHandler.cfg
	origSigner := testHandler.CFSigner
	testHandler.Storage = &mockStorage{}
	testHandler.cfg.AttachmentDownloadMode = "cloudfront"
	testHandler.CFSigner = testCloudFrontSigner(t)
	t.Cleanup(func() {
		testHandler.Storage = origStorage
		testHandler.cfg = origCfg
		testHandler.CFSigner = origSigner
	})

	id := seedAttachmentURL(
		t,
		"https://static.example.test/downloads/cf-report.md",
		"cf report.md",
		"text/markdown",
		12,
	)

	req := httptest.NewRequest("GET", "/api/attachments/"+id, nil)
	req.Header.Set("X-User-ID", testUserID)
	req.Header.Set("X-Workspace-ID", testWorkspaceID)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", id)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	w := httptest.NewRecorder()

	testHandler.GetAttachmentByID(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	var resp AttachmentResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v; body=%s", err, w.Body.String())
	}

	dl, err := url.Parse(resp.AttachmentDownloadURL)
	if err != nil {
		t.Fatalf("parse attachment_download_url: %v", err)
	}
	if dl.Host != "static.example.test" {
		t.Fatalf("attachment_download_url host = %q, want the CloudFront domain", dl.Host)
	}
	if got := dl.Query().Get("response-content-disposition"); got != `attachment; filename="cf report.md"` {
		t.Fatalf("attachment_download_url response-content-disposition = %q, want a forced attachment disposition", got)
	}
	// Signed as a whole: Key-Pair-Id + Signature present. Because the disposition
	// was set before signing, it is inside the signed Resource, so a client cannot
	// strip or alter it without invalidating this Signature.
	if got := dl.Query().Get("Key-Pair-Id"); got != "KTEST" {
		t.Fatalf("attachment_download_url Key-Pair-Id = %q, want KTEST (CloudFront-signed)", got)
	}
	if dl.Query().Get("Signature") == "" {
		t.Fatalf("attachment_download_url missing CloudFront Signature: %q", resp.AttachmentDownloadURL)
	}

	// The load-intent sibling must NOT force an attachment, or inline preview breaks.
	inline, err := url.Parse(resp.DownloadURL)
	if err != nil {
		t.Fatalf("parse download_url: %v", err)
	}
	if got := inline.Query().Get("response-content-disposition"); strings.HasPrefix(got, "attachment") {
		t.Fatalf("download_url must stay load-intent, got forced attachment disposition %q", got)
	}
}

func TestDownloadAttachment_CloudFrontRedirectSignsAttachmentDisposition(t *testing.T) {
	origStorage := testHandler.Storage
	origCfg := testHandler.cfg
	origSigner := testHandler.CFSigner
	testHandler.Storage = &mockStorage{}
	testHandler.cfg.AttachmentDownloadMode = "cloudfront"
	testHandler.cfg.AttachmentFrameAncestors = []string{"https://app.example.test"}
	testHandler.CFSigner = testCloudFrontSigner(t)
	t.Cleanup(func() {
		testHandler.Storage = origStorage
		testHandler.cfg = origCfg
		testHandler.CFSigner = origSigner
	})

	id := seedAttachmentURL(t, "https://static.example.test/downloads/cloudfront.md", "cloud front.md", "text/markdown", 10)

	req, w := newDownloadRequest(t, id, testWorkspaceID)
	w.Header().Set("Content-Security-Policy", "default-src 'self'; frame-ancestors 'none'")
	testHandler.DownloadAttachment(w, req)

	if w.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302; body=%s", w.Code, w.Body.String())
	}
	loc := w.Header().Get("Location")
	parsed, err := url.Parse(loc)
	if err != nil {
		t.Fatalf("parse Location: %v", err)
	}
	if got := parsed.Query().Get("response-content-disposition"); got != `attachment; filename="cloud front.md"` {
		t.Fatalf("response-content-disposition = %q", got)
	}
	if got := parsed.Query().Get("Key-Pair-Id"); got != "KTEST" {
		t.Fatalf("Key-Pair-Id = %q", got)
	}
	requireAttachmentPreviewCSP(t, w.Header(), "https://app.example.test")
}

func TestDownloadAttachment_BareNavigationWithWorkspaceSlugQueryPassesMiddleware(t *testing.T) {
	store := &mockStorage{}
	origStorage := testHandler.Storage
	origCfg := testHandler.cfg
	origSigner := testHandler.CFSigner
	testHandler.Storage = store
	testHandler.cfg.AttachmentDownloadMode = "proxy"
	testHandler.CFSigner = nil
	t.Cleanup(func() {
		testHandler.Storage = origStorage
		testHandler.cfg = origCfg
		testHandler.CFSigner = origSigner
	})

	key := "downloads/bare-nav.txt"
	body := []byte("download body")
	store.put(key, body)
	id := seedAttachmentURL(t, "https://s3.example.com/test-bucket/"+key, "bare-nav.txt", "text/plain", int64(len(body)))

	req := httptest.NewRequest("GET", "/api/attachments/"+id+"/download?workspace_slug="+url.QueryEscape(handlerTestWorkspaceSlug), nil)
	req.Header.Set("X-User-ID", testUserID)
	w := httptest.NewRecorder()

	newDownloadRouter().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if got := w.Body.String(); got != string(body) {
		t.Fatalf("body = %q, want %q", got, body)
	}
	if req.Header.Get("X-Workspace-ID") != "" || req.Header.Get("X-Workspace-Slug") != "" {
		t.Fatalf("bare navigation test must not set custom workspace headers")
	}
}

// TestDownloadAttachment_BareNavigationServesMemberWithoutWorkspaceHeaders
// is the regression test for MUL-3130: a markdown image rendered as
// `<img src="/api/attachments/<id>/download">` produces a native browser
// resource load that cannot attach X-Workspace-Slug / X-Workspace-ID
// headers. After the fix the handler self-resolves the workspace from
// the attachment row, so a bare URL succeeds for a workspace member.
func TestDownloadAttachment_BareNavigationServesMemberWithoutWorkspaceHeaders(t *testing.T) {
	store := &mockStorage{}
	origStorage := testHandler.Storage
	origCfg := testHandler.cfg
	origSigner := testHandler.CFSigner
	testHandler.Storage = store
	testHandler.cfg.AttachmentDownloadMode = "proxy"
	testHandler.CFSigner = nil
	t.Cleanup(func() {
		testHandler.Storage = origStorage
		testHandler.cfg = origCfg
		testHandler.CFSigner = origSigner
	})

	key := "downloads/bare-nav.txt"
	body := []byte("download body")
	store.put(key, body)
	id := seedAttachmentURL(t, "https://s3.example.com/test-bucket/"+key, "bare-nav.txt", "text/plain", int64(len(body)))

	// Bare URL — no workspace_slug / workspace_id query, no
	// X-Workspace-* headers. This is what a browser <img> tag emits
	// when the markdown stores `/api/attachments/<id>/download`.
	req := httptest.NewRequest("GET", "/api/attachments/"+id+"/download", nil)
	req.Header.Set("X-User-ID", testUserID)
	w := httptest.NewRecorder()

	newDownloadRouter().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if got := w.Body.String(); got != string(body) {
		t.Fatalf("body = %q, want %q", got, body)
	}
	if req.Header.Get("X-Workspace-ID") != "" || req.Header.Get("X-Workspace-Slug") != "" {
		t.Fatalf("bare navigation test must not set custom workspace headers")
	}
}

// TestDownloadAttachment_BareNavigationDeniesNonMemberWith404 covers the
// IDOR boundary: a stray attachment ID belonging to a workspace the
// requester is NOT a member of must return 404, not 200 (would leak
// bytes) and not 403 (would confirm the ID exists). Mirrors
// ServeLocalUpload's deny shape.
func TestDownloadAttachment_BareNavigationDeniesNonMemberWith404(t *testing.T) {
	if testPool == nil {
		t.Skip("test database not available")
	}
	store := &mockStorage{}
	origStorage := testHandler.Storage
	origCfg := testHandler.cfg
	origSigner := testHandler.CFSigner
	testHandler.Storage = store
	testHandler.cfg.AttachmentDownloadMode = "proxy"
	testHandler.CFSigner = nil
	t.Cleanup(func() {
		testHandler.Storage = origStorage
		testHandler.cfg = origCfg
		testHandler.CFSigner = origSigner
	})

	// Seed an attachment that lives in a workspace testUserID is NOT
	// a member of. The workspace row has to exist so the FK on
	// attachment.workspace_id resolves; we tear both down on
	// cleanup.
	ctx := context.Background()
	var foreignWorkspaceID string
	if err := testPool.QueryRow(ctx, `
		INSERT INTO workspace (name, slug, description, issue_prefix)
		VALUES ('Bare-Nav Foreign', 'bare-nav-foreign', '', 'BNF')
		RETURNING id::text
	`).Scan(&foreignWorkspaceID); err != nil {
		t.Fatalf("seed foreign workspace: %v", err)
	}
	t.Cleanup(func() { testPool.Exec(ctx, `DELETE FROM workspace WHERE id = $1`, foreignWorkspaceID) })

	key := "downloads/bare-nav-foreign.txt"
	store.put(key, []byte("foreign-body"))
	var id string
	if err := testPool.QueryRow(ctx, `
		INSERT INTO attachment (workspace_id, uploader_type, uploader_id, filename, url, content_type, size_bytes)
		VALUES ($1, 'member', $2, $3, $4, $5, $6)
		RETURNING id::text
	`, foreignWorkspaceID, testUserID, "foreign.txt", "https://s3.example.com/test-bucket/"+key, "text/plain", 12).Scan(&id); err != nil {
		t.Fatalf("seed foreign attachment: %v", err)
	}
	t.Cleanup(func() { testPool.Exec(ctx, `DELETE FROM attachment WHERE id = $1`, id) })

	req := httptest.NewRequest("GET", "/api/attachments/"+id+"/download", nil)
	req.Header.Set("X-User-ID", testUserID)
	w := httptest.NewRecorder()

	newDownloadRouter().ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 for non-member; body=%s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "foreign-body") {
		t.Fatalf("response body leaked file contents: %q", w.Body.String())
	}
}

func TestDownloadAttachment_AutoInternalEndpointProxies(t *testing.T) {
	store := &mockStorage{}
	origStorage := testHandler.Storage
	origCfg := testHandler.cfg
	origSigner := testHandler.CFSigner
	testHandler.Storage = store
	testHandler.cfg.AttachmentDownloadMode = "auto"
	testHandler.cfg.AttachmentFrameAncestors = []string{"https://app.example.test"}
	testHandler.CFSigner = nil
	t.Cleanup(func() {
		testHandler.Storage = origStorage
		testHandler.cfg = origCfg
		testHandler.CFSigner = origSigner
	})

	key := "downloads/proxy-private.txt"
	body := []byte("private object")
	store.put(key, body)
	id := seedAttachmentURL(t, "http://rustfs:9000/test-bucket/"+key, "report.txt", "text/plain", int64(len(body)))

	req, w := newDownloadRequest(t, id, testWorkspaceID)
	w.Header().Set("Content-Security-Policy", "default-src 'self'; frame-ancestors 'none'")
	testHandler.DownloadAttachment(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if got := w.Body.String(); got != string(body) {
		t.Fatalf("body = %q, want %q", got, body)
	}
	if got := w.Header().Get("Location"); got != "" {
		t.Fatalf("Location should be empty for proxy download, got %q", got)
	}
	if got := w.Header().Get("Content-Disposition"); got != `attachment; filename="report.txt"` {
		t.Fatalf("Content-Disposition = %q", got)
	}
	if got := w.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", got)
	}
	if got := w.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("X-Content-Type-Options = %q, want nosniff", got)
	}
	requireAttachmentPreviewCSP(t, w.Header(), "https://app.example.test")
	if len(store.presignCalls) != 0 {
		t.Fatalf("internal endpoint should not presign, calls=%v", store.presignCalls)
	}
}

func TestDownloadAttachment_AutoPublicEndpointPresigns(t *testing.T) {
	store := &mockStorage{}
	origStorage := testHandler.Storage
	origCfg := testHandler.cfg
	origSigner := testHandler.CFSigner
	testHandler.Storage = store
	testHandler.cfg.AttachmentDownloadMode = "auto"
	testHandler.cfg.AttachmentFrameAncestors = []string{"https://app.example.test"}
	testHandler.CFSigner = nil
	t.Cleanup(func() {
		testHandler.Storage = origStorage
		testHandler.cfg = origCfg
		testHandler.CFSigner = origSigner
	})

	key := "downloads/public-private.txt"
	id := seedAttachmentURL(t, "https://s3.example.com/test-bucket/"+key, "public.txt", "text/plain", 10)

	req, w := newDownloadRequest(t, id, testWorkspaceID)
	w.Header().Set("Content-Security-Policy", "default-src 'self'; frame-ancestors 'none'")
	testHandler.DownloadAttachment(w, req)

	if w.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302; body=%s", w.Code, w.Body.String())
	}
	loc := w.Header().Get("Location")
	if !strings.Contains(loc, "X-Amz-Signature=mock") {
		t.Fatalf("Location = %q, want fake S3 signature", loc)
	}
	parsed, err := url.Parse(loc)
	if err != nil {
		t.Fatalf("parse Location: %v", err)
	}
	if got := parsed.Query().Get("response-content-disposition"); got != `attachment; filename="public.txt"` {
		t.Fatalf("response-content-disposition = %q", got)
	}
	if len(store.presignCalls) != 1 || store.presignCalls[0] != key {
		t.Fatalf("presign calls = %v, want [%s]", store.presignCalls, key)
	}
	if len(store.presignDispositions) != 1 || store.presignDispositions[0] != `attachment; filename="public.txt"` {
		t.Fatalf("presign dispositions = %v", store.presignDispositions)
	}
	requireAttachmentPreviewCSP(t, w.Header(), "https://app.example.test")
}

func TestDownloadAttachment_ExplicitProxyStreamsPublicEndpoint(t *testing.T) {
	store := &mockStorage{}
	origStorage := testHandler.Storage
	origCfg := testHandler.cfg
	origSigner := testHandler.CFSigner
	testHandler.Storage = store
	testHandler.cfg.AttachmentDownloadMode = "proxy"
	testHandler.CFSigner = nil
	t.Cleanup(func() {
		testHandler.Storage = origStorage
		testHandler.cfg = origCfg
		testHandler.CFSigner = origSigner
	})

	key := "downloads/forced-proxy.png"
	body := []byte("\x89PNG\r\n\x1a\nimage")
	store.put(key, body)
	id := seedAttachmentURL(t, "https://s3.example.com/test-bucket/"+key, "image.png", "image/png", int64(len(body)))

	req, w := newDownloadRequest(t, id, testWorkspaceID)
	testHandler.DownloadAttachment(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if got := w.Body.Bytes(); !bytes.Equal(got, body) {
		t.Fatalf("body mismatch: got %q want %q", got, body)
	}
	if got := w.Header().Get("Content-Disposition"); got != `inline; filename="image.png"` {
		t.Fatalf("Content-Disposition = %q", got)
	}
	requireAttachmentPreviewCSP(t, w.Header())
	if len(store.presignCalls) != 0 {
		t.Fatalf("forced proxy should not presign, calls=%v", store.presignCalls)
	}
}

// setProxyDownloadHandler swaps the handler into forced-proxy mode with the
// given storage backend and returns the seeded attachment id + body. It wires
// t.Cleanup to restore the original handler fields.
func setProxyDownloadHandler(t *testing.T, store storage.Storage, body []byte) string {
	t.Helper()
	origStorage := testHandler.Storage
	origCfg := testHandler.cfg
	origSigner := testHandler.CFSigner
	testHandler.Storage = store
	testHandler.cfg.AttachmentDownloadMode = "proxy"
	testHandler.CFSigner = nil
	t.Cleanup(func() {
		testHandler.Storage = origStorage
		testHandler.cfg = origCfg
		testHandler.CFSigner = origSigner
	})

	key := "downloads/range-sample.bin"
	if putter, ok := store.(interface{ put(string, []byte) }); ok {
		putter.put(key, body)
	} else {
		t.Fatalf("store %T does not support put()", store)
	}
	return seedAttachmentURL(t, "https://s3.example.com/test-bucket/"+key, "sample.bin", "application/octet-stream", int64(len(body)))
}

// runProxyRangeMatrix exercises the three acceptance paths (Range hit, 416,
// no-Range) against whichever storage backend is supplied, so both the
// http.ServeContent (seekable) and manual (non-seekable) branches are covered
// by identical assertions.
func runProxyRangeMatrix(t *testing.T, newStore func() storage.Storage) {
	body := rangeBody()

	t.Run("RangeHitReturns206", func(t *testing.T) {
		id := setProxyDownloadHandler(t, newStore(), body)
		req, w := newDownloadRequest(t, id, testWorkspaceID)
		req.Header.Set("Range", "bytes=0-1023")
		testHandler.DownloadAttachment(w, req)

		if w.Code != http.StatusPartialContent {
			t.Fatalf("status = %d, want 206; body=%s", w.Code, w.Body.String())
		}
		if got, want := w.Header().Get("Accept-Ranges"), "bytes"; got != want {
			t.Fatalf("Accept-Ranges = %q, want %q", got, want)
		}
		if got, want := w.Header().Get("Content-Range"), fmt.Sprintf("bytes 0-1023/%d", len(body)); got != want {
			t.Fatalf("Content-Range = %q, want %q", got, want)
		}
		if got, want := w.Header().Get("Content-Length"), "1024"; got != want {
			t.Fatalf("Content-Length = %q, want %q", got, want)
		}
		if got := w.Body.Bytes(); !bytes.Equal(got, body[:1024]) {
			t.Fatalf("partial body mismatch: len(got)=%d, want first 1024 bytes of object", len(got))
		}
		requireAttachmentDownloadHeaders(t, w.Header(), "sample.bin")
	})

	t.Run("MidStreamRangeMatchesFullSlice", func(t *testing.T) {
		id := setProxyDownloadHandler(t, newStore(), body)
		req, w := newDownloadRequest(t, id, testWorkspaceID)
		req.Header.Set("Range", "bytes=1000-1999")
		testHandler.DownloadAttachment(w, req)

		if w.Code != http.StatusPartialContent {
			t.Fatalf("status = %d, want 206; body=%s", w.Code, w.Body.String())
		}
		if got, want := w.Header().Get("Content-Range"), fmt.Sprintf("bytes 1000-1999/%d", len(body)); got != want {
			t.Fatalf("Content-Range = %q, want %q", got, want)
		}
		if got := w.Body.Bytes(); !bytes.Equal(got, body[1000:2000]) {
			t.Fatalf("mid-stream range bytes do not match object[1000:2000]")
		}
	})

	t.Run("SuffixRangeReturnsTail", func(t *testing.T) {
		id := setProxyDownloadHandler(t, newStore(), body)
		req, w := newDownloadRequest(t, id, testWorkspaceID)
		req.Header.Set("Range", "bytes=-500")
		testHandler.DownloadAttachment(w, req)

		if w.Code != http.StatusPartialContent {
			t.Fatalf("status = %d, want 206; body=%s", w.Code, w.Body.String())
		}
		start := len(body) - 500
		if got, want := w.Header().Get("Content-Range"), fmt.Sprintf("bytes %d-%d/%d", start, len(body)-1, len(body)); got != want {
			t.Fatalf("Content-Range = %q, want %q", got, want)
		}
		if got := w.Body.Bytes(); !bytes.Equal(got, body[start:]) {
			t.Fatalf("suffix range bytes do not match object tail")
		}
	})

	t.Run("UnsatisfiableRangeReturns416", func(t *testing.T) {
		id := setProxyDownloadHandler(t, newStore(), body)
		req, w := newDownloadRequest(t, id, testWorkspaceID)
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", len(body)+10, len(body)+20))
		testHandler.DownloadAttachment(w, req)

		if w.Code != http.StatusRequestedRangeNotSatisfiable {
			t.Fatalf("status = %d, want 416; body=%s", w.Code, w.Body.String())
		}
		if got, want := w.Header().Get("Content-Range"), fmt.Sprintf("bytes */%d", len(body)); got != want {
			t.Fatalf("Content-Range = %q, want %q", got, want)
		}
	})

	t.Run("NoRangeReturnsFull200", func(t *testing.T) {
		id := setProxyDownloadHandler(t, newStore(), body)
		req, w := newDownloadRequest(t, id, testWorkspaceID)
		testHandler.DownloadAttachment(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
		if got, want := w.Header().Get("Accept-Ranges"), "bytes"; got != want {
			t.Fatalf("Accept-Ranges = %q, want %q", got, want)
		}
		if got := w.Body.Bytes(); !bytes.Equal(got, body) {
			t.Fatalf("full body mismatch: len(got)=%d, want %d", len(got), len(body))
		}
		requireAttachmentDownloadHeaders(t, w.Header(), "sample.bin")
	})
}

// TestDownloadAttachment_ProxyRange_Seekable covers the http.ServeContent path
// taken when the storage backend returns a seekable reader (local disk).
func TestDownloadAttachment_ProxyRange_Seekable(t *testing.T) {
	runProxyRangeMatrix(t, func() storage.Storage { return &seekableMockStorage{} })
}

// TestDownloadAttachment_ProxyRange_NonSeekable covers the manual single-range
// fallback taken when the backend returns a forward-only stream (S3/MinIO).
func TestDownloadAttachment_ProxyRange_NonSeekable(t *testing.T) {
	runProxyRangeMatrix(t, func() storage.Storage { return &mockStorage{} })
}

// TestDownloadAttachment_NonSeekableRangeSkipFailureReturns502 is the must-fix
// regression (RAS-31 ①). When the storage read fails while the handler is
// skipping forward to the Range start — before any response header is written —
// it must reply with an honest 502, NOT net/http's default 200 OK + empty body.
// A resuming client reads that default 200 as "Range ignored, this is the full
// object", silently turning a transient storage error into a corrupt/empty
// download and defeating the resume feature itself.
func TestDownloadAttachment_NonSeekableRangeSkipFailureReturns502(t *testing.T) {
	body := rangeBody()
	// Die at offset 500, well before the Range start (1000), so the failure lands
	// inside the skip CopyN rather than the payload copy.
	store := &failingMockStorage{failAfter: 500}
	id := setProxyDownloadHandler(t, store, body)

	req, w := newDownloadRequest(t, id, testWorkspaceID)
	req.Header.Set("Range", "bytes=1000-1999")
	testHandler.DownloadAttachment(w, req)

	if w.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502; body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "failed to read attachment range") {
		t.Fatalf("502 body should carry the honest error, got %q", w.Body.String())
	}
	if got := w.Body.Bytes(); bytes.Contains(got, body[:64]) {
		t.Fatalf("502 response must not leak object bytes; got %q", got)
	}
}

// TestDownloadAttachment_NonSeekableMultiRangeServesFull200 is the should-fix
// (RAS-31 ②). A multi-range request the non-seekable path cannot serve must be
// ignored and answered with a full 200 (RFC 7233), not 416. Paired with the
// seekable test below, this proves the two backends no longer disagree on
// success vs failure for the same multi-range request.
func TestDownloadAttachment_NonSeekableMultiRangeServesFull200(t *testing.T) {
	body := rangeBody()
	id := setProxyDownloadHandler(t, &mockStorage{}, body)

	req, w := newDownloadRequest(t, id, testWorkspaceID)
	req.Header.Set("Range", "bytes=0-10,20-30")
	testHandler.DownloadAttachment(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (multi-range ignored); body=%s", w.Code, w.Body.String())
	}
	if got := w.Body.Bytes(); !bytes.Equal(got, body) {
		t.Fatalf("multi-range should serve full body: len(got)=%d, want %d", len(got), len(body))
	}
	if got := w.Header().Get("Content-Range"); got != "" {
		t.Fatalf("full 200 response must not carry Content-Range, got %q", got)
	}
	if got, want := w.Header().Get("Accept-Ranges"), "bytes"; got != want {
		t.Fatalf("Accept-Ranges = %q, want %q", got, want)
	}
	requireAttachmentDownloadHeaders(t, w.Header(), "sample.bin")
}

// TestDownloadAttachment_SeekableMultiRangeServes206 documents the other half of
// the ②-divergence: the seekable (http.ServeContent) path answers the same
// multi-range request with a successful 206 multipart. Together with the
// non-seekable 200 test above, it shows neither backend fails the request with
// 416 — the divergence the maintainer flagged is gone.
func TestDownloadAttachment_SeekableMultiRangeServes206(t *testing.T) {
	body := rangeBody()
	id := setProxyDownloadHandler(t, &seekableMockStorage{}, body)

	req, w := newDownloadRequest(t, id, testWorkspaceID)
	req.Header.Set("Range", "bytes=0-10,20-30")
	testHandler.DownloadAttachment(w, req)

	if w.Code != http.StatusPartialContent {
		t.Fatalf("status = %d, want 206 (multipart); body len=%d", w.Code, w.Body.Len())
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "multipart/byteranges") {
		t.Fatalf("Content-Type = %q, want multipart/byteranges", ct)
	}
}

// TestDownloadAttachment_NonSeekableEmptyObjectRangeServesFull200 covers the
// zero-length nit the maintainer flagged during re-review (same class as RAS-31
// ②). A WELL-FORMED Range against a 0-byte attachment used to collapse to 416 on
// the non-seekable path (parseSingleByteRange → rangeUnsatisfiable) while the
// seekable http.ServeContent path ignores the Range and returns an empty 200.
// parseSingleByteRange now classifies a well-formed range against an empty object
// as rangeUnsupported (→ empty 200), so neither backend hard-fails it. Malformed
// ranges are covered by the 416 companion test below.
func TestDownloadAttachment_NonSeekableEmptyObjectRangeServesFull200(t *testing.T) {
	// Both a start-based range and a suffix range must be ignored on an empty
	// object; the suffix form ("bytes=-100" against size 0) is the exact case the
	// maintainer referenced.
	for _, rangeHeader := range []string{"bytes=0-", "bytes=-100", "bytes=0-99"} {
		t.Run(rangeHeader, func(t *testing.T) {
			id := setProxyDownloadHandler(t, &mockStorage{}, []byte{})

			req, w := newDownloadRequest(t, id, testWorkspaceID)
			req.Header.Set("Range", rangeHeader)
			testHandler.DownloadAttachment(w, req)

			if w.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200 (Range ignored on empty object); body=%s", w.Code, w.Body.String())
			}
			if got := w.Body.Len(); got != 0 {
				t.Fatalf("empty object must serve an empty body, got %d bytes", got)
			}
			if got := w.Header().Get("Content-Length"); got != "0" {
				t.Fatalf("Content-Length = %q, want %q for an empty 200", got, "0")
			}
			if got := w.Header().Get("Content-Range"); got != "" {
				t.Fatalf("full 200 response must not carry Content-Range, got %q", got)
			}
			if got, want := w.Header().Get("Accept-Ranges"), "bytes"; got != want {
				t.Fatalf("Accept-Ranges = %q, want %q", got, want)
			}
			requireAttachmentDownloadHeaders(t, w.Header(), "sample.bin")
		})
	}
}

// TestDownloadAttachment_NonSeekableEmptyObjectMalformedRangeReturns416 is the
// guard against over-correcting the zero-length nit: a MALFORMED / unknown-unit
// Range against a 0-byte object must still return 416, not be swallowed into a
// 200. stdlib http.ServeContent returns 416 for these even on an empty object
// (it rejects the range before the empty-content short-circuit), so returning
// 200 here would introduce a NEW backend-dependent divergence — the exact
// regression a blanket "total == 0 → 200" would cause.
func TestDownloadAttachment_NonSeekableEmptyObjectMalformedRangeReturns416(t *testing.T) {
	for _, rangeHeader := range []string{"items=0-10", "bytes=abc-def", "bytes=100"} {
		t.Run(rangeHeader, func(t *testing.T) {
			id := setProxyDownloadHandler(t, &mockStorage{}, []byte{})

			req, w := newDownloadRequest(t, id, testWorkspaceID)
			req.Header.Set("Range", rangeHeader)
			testHandler.DownloadAttachment(w, req)

			if w.Code != http.StatusRequestedRangeNotSatisfiable {
				t.Fatalf("status = %d, want 416 for malformed range on empty object; body=%s", w.Code, w.Body.String())
			}
		})
	}
}

func TestGetAttachmentContent_HappyPath_Markdown(t *testing.T) {
	store := &mockStorage{}
	origStorage := testHandler.Storage
	testHandler.Storage = store
	defer func() { testHandler.Storage = origStorage }()

	body := []byte("# heading\n\nbody text\n")
	id := seedPreviewAttachment(t, store, "preview-md-key.md", "preview.md", "text/markdown", body)

	req, w := newPreviewRequest(t, id, testWorkspaceID)
	w.Header().Set("Content-Security-Policy", "default-src 'self'; frame-ancestors 'none'")
	testHandler.GetAttachmentContent(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if got := w.Body.String(); got != string(body) {
		t.Errorf("body = %q, want %q", got, body)
	}
	if got := w.Header().Get("Content-Type"); got != "text/plain; charset=utf-8" {
		t.Errorf("Content-Type = %q, want text/plain; charset=utf-8", got)
	}
	if got := w.Header().Get("X-Original-Content-Type"); got != "text/markdown" {
		t.Errorf("X-Original-Content-Type = %q, want text/markdown", got)
	}
	if got := w.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Errorf("X-Content-Type-Options = %q, want nosniff", got)
	}
	requireAttachmentPreviewCSP(t, w.Header())
}

// Even when http.DetectContentType returned "text/plain" instead of "text/markdown"
// (a known sniffer quirk), the extension whitelist still grants access.
func TestGetAttachmentContent_AcceptsByExtensionWhenContentTypeIsGeneric(t *testing.T) {
	store := &mockStorage{}
	origStorage := testHandler.Storage
	testHandler.Storage = store
	defer func() { testHandler.Storage = origStorage }()

	body := []byte("package main\n")
	id := seedPreviewAttachment(t, store, "main-go-key.go", "main.go", "application/octet-stream", body)

	req, w := newPreviewRequest(t, id, testWorkspaceID)
	testHandler.GetAttachmentContent(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
}

func TestGetAttachmentContent_Unsupported_PDF(t *testing.T) {
	store := &mockStorage{}
	origStorage := testHandler.Storage
	testHandler.Storage = store
	defer func() { testHandler.Storage = origStorage }()

	id := seedPreviewAttachment(t, store, "pdf-key.pdf", "manual.pdf", "application/pdf", []byte("%PDF-1.4\n"))

	req, w := newPreviewRequest(t, id, testWorkspaceID)
	testHandler.GetAttachmentContent(w, req)
	if w.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("status = %d, want 415; body=%s", w.Code, w.Body.String())
	}
}

func TestGetAttachmentContent_TooLarge(t *testing.T) {
	store := &mockStorage{}
	origStorage := testHandler.Storage
	testHandler.Storage = store
	defer func() { testHandler.Storage = origStorage }()

	// One byte over the limit. Allocate ASCII so io.ReadAll has work to do.
	big := bytes.Repeat([]byte("a"), maxPreviewTextSize+1)
	id := seedPreviewAttachment(t, store, "huge-key.txt", "huge.txt", "text/plain", big)

	req, w := newPreviewRequest(t, id, testWorkspaceID)
	testHandler.GetAttachmentContent(w, req)
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413; body=%s", w.Code, w.Body.String())
	}
}

func TestGetAttachmentContent_ForeignWorkspace(t *testing.T) {
	store := &mockStorage{}
	origStorage := testHandler.Storage
	testHandler.Storage = store
	defer func() { testHandler.Storage = origStorage }()

	id := seedPreviewAttachment(t, store, "ws-mismatch.md", "note.md", "text/markdown", []byte("# secret\n"))

	// Same attachment id, but request comes in scoped to a different workspace.
	foreign := "00000000-0000-0000-0000-000000000099"
	req, w := newPreviewRequest(t, id, foreign)
	testHandler.GetAttachmentContent(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body=%s", w.Code, w.Body.String())
	}
}

func TestGetAttachmentContent_NotFound(t *testing.T) {
	store := &mockStorage{}
	origStorage := testHandler.Storage
	testHandler.Storage = store
	defer func() { testHandler.Storage = origStorage }()

	req, w := newPreviewRequest(t, "00000000-0000-0000-0000-000000000abc", testWorkspaceID)
	testHandler.GetAttachmentContent(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body=%s", w.Code, w.Body.String())
	}
}

func TestBuildMarkdownURL_PublicCdnAbsoluteURLReusedVerbatim(t *testing.T) {
	origPublic := testHandler.cfg.PublicURL
	origSigner := testHandler.CFSigner
	origStorage := testHandler.Storage
	t.Cleanup(func() {
		testHandler.cfg.PublicURL = origPublic
		testHandler.CFSigner = origSigner
		testHandler.Storage = origStorage
	})
	testHandler.cfg.PublicURL = "https://api.multica.test"
	testHandler.CFSigner = nil
	// mockStorage.CdnDomain() returns "cdn.example.com" — that's the
	// operator-set signal that the URL host serves content publicly
	// without per-request auth. Without this, the new gate routes
	// through the API endpoint to be safe.
	testHandler.Storage = &mockStorage{}

	id := seedAttachmentURL(t, "https://cdn.multica.test/uploads/abc.png", "abc.png", "image/png", 1)
	att, err := testHandler.Queries.GetAttachment(context.Background(), db.GetAttachmentParams{
		ID:          parseUUID(id),
		WorkspaceID: parseUUID(testWorkspaceID),
	})
	if err != nil {
		t.Fatalf("GetAttachment: %v", err)
	}

	resp := testHandler.attachmentToResponse(att, attachmentURLModeSigned)
	if resp.MarkdownURL != "https://cdn.multica.test/uploads/abc.png" {
		t.Fatalf("markdown_url = %q, want raw a.Url passthrough", resp.MarkdownURL)
	}
}

// MUL-3192 review must-fix 1 — `att.url` for a private S3 / R2 / MinIO
// bucket is absolute https + unsigned but is NOT publicly readable. The
// generic "absolute http(s) without signature" check would have wrongly
// persisted it; the gate now also requires `Storage.CdnDomain()` to be
// set so the operator has explicitly opted into "URLs from this storage
// load directly".
func TestBuildMarkdownURL_PrivateBucketWithoutCdnDomainRoutesThroughAPIEndpoint(t *testing.T) {
	origPublic := testHandler.cfg.PublicURL
	origSigner := testHandler.CFSigner
	origStorage := testHandler.Storage
	t.Cleanup(func() {
		testHandler.cfg.PublicURL = origPublic
		testHandler.CFSigner = origSigner
		testHandler.Storage = origStorage
	})
	testHandler.cfg.PublicURL = "https://api.multica.test"
	testHandler.CFSigner = nil
	testHandler.Storage = &mockStorageNoCdn{}

	id := seedAttachmentURL(t, "https://prod.s3.amazonaws.com/key.png", "key.png", "image/png", 1)
	att, err := testHandler.Queries.GetAttachment(context.Background(), db.GetAttachmentParams{
		ID:          parseUUID(id),
		WorkspaceID: parseUUID(testWorkspaceID),
	})
	if err != nil {
		t.Fatalf("GetAttachment: %v", err)
	}

	resp := testHandler.attachmentToResponse(att, attachmentURLModeSigned)
	want := "https://api.multica.test/api/attachments/" + id + "/download"
	if resp.MarkdownURL != want {
		t.Fatalf("markdown_url = %q, want absolute API endpoint %q (private bucket without explicit CDN must not persist raw S3 URL)", resp.MarkdownURL, want)
	}
}

func TestBuildMarkdownURL_CloudFrontSignedModeNeverPersistsRawStorageURL(t *testing.T) {
	origPublic := testHandler.cfg.PublicURL
	origSigner := testHandler.CFSigner
	t.Cleanup(func() {
		testHandler.cfg.PublicURL = origPublic
		testHandler.CFSigner = origSigner
	})
	testHandler.cfg.PublicURL = "https://api.multica.test"
	testHandler.CFSigner = testCloudFrontSigner(t)

	// Raw S3 URL — private bucket, not loadable directly by clients.
	id := seedAttachmentURL(t, "https://prod.s3.amazonaws.com/key.png", "key.png", "image/png", 1)
	att, err := testHandler.Queries.GetAttachment(context.Background(), db.GetAttachmentParams{
		ID:          parseUUID(id),
		WorkspaceID: parseUUID(testWorkspaceID),
	})
	if err != nil {
		t.Fatalf("GetAttachment: %v", err)
	}

	resp := testHandler.attachmentToResponse(att, attachmentURLModeSigned)
	want := "https://api.multica.test/api/attachments/" + id + "/download"
	if resp.MarkdownURL != want {
		t.Fatalf("markdown_url = %q, want absolute API endpoint %q", resp.MarkdownURL, want)
	}
	// download_url is allowed to carry a TTL (CloudFront-signed); it's NOT
	// what the client persists, but it IS what the renderer uses for this
	// response. The two are intentionally distinct.
	if resp.DownloadURL == resp.MarkdownURL {
		t.Fatalf("download_url and markdown_url must differ in CloudFront-signed mode (got identical %q)", resp.DownloadURL)
	}
}

func TestBuildMarkdownURL_RelativeStorageURLPrefixedWithPublicURL(t *testing.T) {
	origPublic := testHandler.cfg.PublicURL
	origSigner := testHandler.CFSigner
	t.Cleanup(func() {
		testHandler.cfg.PublicURL = origPublic
		testHandler.CFSigner = origSigner
	})
	testHandler.cfg.PublicURL = "https://api.multica.test"
	testHandler.CFSigner = nil

	// LocalStorage without LOCAL_UPLOAD_BASE_URL stores a site-relative URL.
	id := seedAttachmentURL(t, "/uploads/abc.png", "abc.png", "image/png", 1)
	att, err := testHandler.Queries.GetAttachment(context.Background(), db.GetAttachmentParams{
		ID:          parseUUID(id),
		WorkspaceID: parseUUID(testWorkspaceID),
	})
	if err != nil {
		t.Fatalf("GetAttachment: %v", err)
	}

	resp := testHandler.attachmentToResponse(att, attachmentURLModeSigned)
	want := "https://api.multica.test/api/attachments/" + id + "/download"
	if resp.MarkdownURL != want {
		t.Fatalf("markdown_url = %q, want absolute API endpoint %q", resp.MarkdownURL, want)
	}
}

func TestBuildMarkdownURL_PublicURLUnsetFallsBackToSiteRelative(t *testing.T) {
	origPublic := testHandler.cfg.PublicURL
	origSigner := testHandler.CFSigner
	t.Cleanup(func() {
		testHandler.cfg.PublicURL = origPublic
		testHandler.CFSigner = origSigner
	})
	testHandler.cfg.PublicURL = ""
	testHandler.CFSigner = nil

	id := seedAttachmentURL(t, "/uploads/abc.png", "abc.png", "image/png", 1)
	att, err := testHandler.Queries.GetAttachment(context.Background(), db.GetAttachmentParams{
		ID:          parseUUID(id),
		WorkspaceID: parseUUID(testWorkspaceID),
	})
	if err != nil {
		t.Fatalf("GetAttachment: %v", err)
	}

	resp := testHandler.attachmentToResponse(att, attachmentURLModeSigned)
	want := "/api/attachments/" + id + "/download"
	if resp.MarkdownURL != want {
		t.Fatalf("markdown_url = %q, want site-relative fallback %q", resp.MarkdownURL, want)
	}
}

func TestBuildMarkdownURL_StripsTrailingSlashOnPublicURL(t *testing.T) {
	origPublic := testHandler.cfg.PublicURL
	origSigner := testHandler.CFSigner
	t.Cleanup(func() {
		testHandler.cfg.PublicURL = origPublic
		testHandler.CFSigner = origSigner
	})
	testHandler.cfg.PublicURL = "https://api.multica.test/"
	testHandler.CFSigner = nil

	id := seedAttachmentURL(t, "/uploads/abc.png", "abc.png", "image/png", 1)
	att, err := testHandler.Queries.GetAttachment(context.Background(), db.GetAttachmentParams{
		ID:          parseUUID(id),
		WorkspaceID: parseUUID(testWorkspaceID),
	})
	if err != nil {
		t.Fatalf("GetAttachment: %v", err)
	}

	resp := testHandler.attachmentToResponse(att, attachmentURLModeSigned)
	want := "https://api.multica.test/api/attachments/" + id + "/download"
	if resp.MarkdownURL != want {
		t.Fatalf("markdown_url = %q, want exactly one separator %q", resp.MarkdownURL, want)
	}
}
