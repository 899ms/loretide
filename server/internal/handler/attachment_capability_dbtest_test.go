//go:build dbtest

package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
)

// installProxyModeStorage puts the handler into proxy mode with an in-memory
// backend, which is the deployment shape (local disk / private object host)
// where capabilities are minted.
func installProxyModeStorage(t *testing.T) *mockStorage {
	t.Helper()
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
	return store
}

func TestDownloadAttachmentWithCapability_ServesWithoutAuthentication(t *testing.T) {
	store := installProxyModeStorage(t)
	body := []byte("quarterly numbers, do not leak")
	id := seedPreviewAttachment(t, store, "downloads/report.pdf", "Q3 report.pdf", "application/pdf", body)

	req, w := newCapabilityRequest(id, capabilityQuery(t, id, time.Now()))
	testHandler.DownloadAttachmentWithCapability(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if !bytes.Equal(w.Body.Bytes(), body) {
		t.Fatalf("body = %q, want %q", w.Body.String(), body)
	}
	// The original filename must survive to the save dialog — that is half
	// of what the bug report asked for.
	if disposition := w.Header().Get("Content-Disposition"); !strings.Contains(disposition, "Q3 report.pdf") {
		t.Fatalf("Content-Disposition = %q, want the original filename", disposition)
	}
	if got := w.Header().Get("Referrer-Policy"); got != "no-referrer" {
		t.Fatalf("Referrer-Policy = %q, want no-referrer", got)
	}
	if got := w.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", got)
	}
}

func TestDownloadAttachmentWithCapability_RejectsInvalidCapabilities(t *testing.T) {
	store := installProxyModeStorage(t)
	body := []byte("secret bytes")
	id := seedPreviewAttachment(t, store, "downloads/secret.txt", "secret.txt", "text/plain", body)

	expired := capabilityQuery(t, id, time.Now().Add(-2*attachmentCapabilityTTL))

	forged := capabilityQuery(t, id, time.Now())
	forged.Set("sig", strings.Repeat("0", len(forged.Get("sig"))))

	// A capability legitimately minted for a different attachment must not
	// unlock this one.
	otherID := seedPreviewAttachment(t, store, "downloads/other.txt", "other.txt", "text/plain", []byte("other"))
	crossed := capabilityQuery(t, otherID, time.Now())

	cases := []struct {
		name  string
		query url.Values
	}{
		{"expired", expired},
		{"forged signature", forged},
		{"capability for another attachment", crossed},
		{"no capability at all", url.Values{}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req, w := newCapabilityRequest(id, tc.query)
			testHandler.DownloadAttachmentWithCapability(w, req)

			if w.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want 403; body=%s", w.Code, w.Body.String())
			}
			if bytes.Contains(w.Body.Bytes(), body) {
				t.Fatal("rejected response leaked the attachment body")
			}
		})
	}
}

// Range support is what makes an interrupted download resumable (RAS-29). The
// capability route reuses proxyAttachmentDownload, so it must not regress.
func TestDownloadAttachmentWithCapability_PreservesRangeAndHeaders(t *testing.T) {
	store := installProxyModeStorage(t)
	body := bytes.Repeat([]byte("abcdefgh"), 512) // 4 KiB
	id := seedPreviewAttachment(t, store, "downloads/big.bin", "big.bin", "application/octet-stream", body)

	req, w := newCapabilityRequest(id, capabilityQuery(t, id, time.Now()))
	req.Header.Set("Range", "bytes=100-199")
	testHandler.DownloadAttachmentWithCapability(w, req)

	if w.Code != http.StatusPartialContent {
		t.Fatalf("status = %d, want 206; body len=%d", w.Code, w.Body.Len())
	}
	if got := w.Header().Get("Content-Range"); got != "bytes 100-199/4096" {
		t.Fatalf("Content-Range = %q", got)
	}
	if !bytes.Equal(w.Body.Bytes(), body[100:200]) {
		t.Fatalf("partial body mismatch: len=%d", w.Body.Len())
	}
	if disposition := w.Header().Get("Content-Disposition"); !strings.Contains(disposition, "big.bin") {
		t.Fatalf("Content-Disposition = %q, want the original filename", disposition)
	}
}

func TestGetAttachmentByID_ProxyModeReturnsRedeemableCapability(t *testing.T) {
	store := installProxyModeStorage(t)
	id := seedPreviewAttachment(t, store, "downloads/inline.png", "inline.png", "image/png", []byte("png-bytes"))

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

	// Site-relative on purpose: an absolute URL here would be picked up by
	// the inline-media re-sign path in packages/views/editor/attachment.tsx
	// and pinned into an <img> for far longer than the 60s TTL.
	if !strings.HasPrefix(resp.DownloadURL, "/api/attachments/"+id+"/signed-download?") {
		t.Fatalf("download_url = %q, want a site-relative capability path", resp.DownloadURL)
	}

	parsed, err := url.Parse(resp.DownloadURL)
	if err != nil {
		t.Fatalf("parse download_url: %v", err)
	}
	if !verifyAttachmentCapability(id, parsed.Query().Get("exp"), parsed.Query().Get("sig"), "", time.Now()) {
		t.Fatalf("minted capability does not verify: %q", resp.DownloadURL)
	}

	// markdown_url is persisted into comment bodies and must outlive the
	// session, so a 60-second capability must never reach it — that is the
	// exact class of bug MUL-3130 fixed.
	if strings.Contains(resp.MarkdownURL, "signed-download") {
		t.Fatalf("markdown_url = %q, must not embed a short-lived capability", resp.MarkdownURL)
	}
}

// TestGetAttachmentByID_ProxyModeDownloadURLForcesAttachment is the end-to-end
// proof for the download-intent field: the minted attachment_download_url is a
// site-relative dl=1 capability that verifies only under the attachment intent
// and, once redeemed, forces Content-Disposition: attachment even for an image
// — which the load-intent download_url path serves inline.
func TestGetAttachmentByID_ProxyModeDownloadURLForcesAttachment(t *testing.T) {
	store := installProxyModeStorage(t)
	body := []byte("png-bytes")
	id := seedPreviewAttachment(t, store, "downloads/pic.png", "pic.png", "image/png", body)

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

	// Site-relative dl=1 capability, distinct from the load-intent download_url,
	// and verifiable only under the attachment intent.
	if !strings.HasPrefix(resp.AttachmentDownloadURL, "/api/attachments/"+id+"/signed-download?") {
		t.Fatalf("attachment_download_url = %q, want a site-relative capability path", resp.AttachmentDownloadURL)
	}
	parsed, err := url.Parse(resp.AttachmentDownloadURL)
	if err != nil {
		t.Fatalf("parse attachment_download_url: %v", err)
	}
	if parsed.Query().Get("dl") != "1" {
		t.Fatalf("attachment_download_url missing dl=1: %q", resp.AttachmentDownloadURL)
	}
	if !verifyAttachmentCapability(id, parsed.Query().Get("exp"), parsed.Query().Get("sig"), attachmentCapabilityDownloadIntent, time.Now()) {
		t.Fatalf("download capability does not verify under the attachment intent: %q", resp.AttachmentDownloadURL)
	}
	// A 60-second capability must never leak into the persisted markdown_url.
	if strings.Contains(resp.MarkdownURL, "signed-download") {
		t.Fatalf("markdown_url = %q, must not embed a short-lived capability", resp.MarkdownURL)
	}

	// Redeeming it forces an attachment disposition even for an image.
	req2, w2 := newCapabilityRequest(id, parsed.Query())
	testHandler.DownloadAttachmentWithCapability(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("redeem status = %d, want 200; body=%s", w2.Code, w2.Body.String())
	}
	if !bytes.Equal(w2.Body.Bytes(), body) {
		t.Fatalf("redeemed body mismatch: got %q", w2.Body.String())
	}
	disposition := w2.Header().Get("Content-Disposition")
	if !strings.HasPrefix(disposition, "attachment") {
		t.Fatalf("Content-Disposition = %q, want it to force attachment", disposition)
	}
	if !strings.Contains(disposition, "pic.png") {
		t.Fatalf("Content-Disposition = %q, want the original filename", disposition)
	}
}

// List-shaped responses are held far longer than the capability TTL, so
// attachmentToResponse must keep emitting the stable endpoint.
func TestAttachmentToResponse_ProxyModeDoesNotMintCapability(t *testing.T) {
	store := installProxyModeStorage(t)
	id := seedPreviewAttachment(t, store, "downloads/listed.txt", "listed.txt", "text/plain", []byte("listed"))

	att, err := testHandler.Queries.GetAttachmentByIDOnly(context.Background(), parseUUID(id))
	if err != nil {
		t.Fatalf("GetAttachmentByIDOnly: %v", err)
	}

	resp := testHandler.attachmentToResponse(att, attachmentURLModeSigned)
	if want := "/api/attachments/" + id + "/download"; resp.DownloadURL != want {
		t.Fatalf("download_url = %q, want stable %q (no capability in list responses)", resp.DownloadURL, want)
	}
}

// The pre-existing authenticated route must stay authenticated. Adding a
// public capability entry point must not turn the original path into an open
// one for clients that predate this change.
func TestDownloadAttachment_StillRequiresAuthentication(t *testing.T) {
	store := installProxyModeStorage(t)
	id := seedPreviewAttachment(t, store, "downloads/guarded.txt", "guarded.txt", "text/plain", []byte("guarded"))

	req := httptest.NewRequest("GET", "/api/attachments/"+id+"/download", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", id)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	w := httptest.NewRecorder()

	testHandler.DownloadAttachment(w, req)

	if w.Code == http.StatusOK {
		t.Fatalf("unauthenticated request to the legacy download path succeeded: %s", w.Body.String())
	}
}
