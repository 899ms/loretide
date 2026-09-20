package handler

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"github.com/multica-ai/multica/server/internal/auth"
	"github.com/multica-ai/multica/server/internal/storage"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// mockStorage is a tiny in-memory Storage stand-in. Upload records the bytes
// keyed by the storage key so GetReader can round-trip them in tests; KeyFromURL
// strips the synthetic CDN host so consumers can pass either the URL or the
// raw key.
type mockStorage struct {
	mu                  sync.Mutex
	files               map[string][]byte
	presignCalls        []string
	presignDispositions []string
	getReaderCalls      int
	uploadStreamCalls   int
}

func (m *mockStorage) Upload(_ context.Context, key string, data []byte, _ string, _ string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.files == nil {
		m.files = map[string][]byte{}
	}
	m.files[key] = append([]byte(nil), data...)
	return fmt.Sprintf("https://cdn.example.com/%s", key), nil
}

func (m *mockStorage) UploadStream(ctx context.Context, key string, reader io.Reader, _ int64, contentType string, filename string) (string, error) {
	m.mu.Lock()
	m.uploadStreamCalls++
	m.mu.Unlock()
	data, err := io.ReadAll(reader)
	if err != nil {
		return "", err
	}
	return m.Upload(ctx, key, data, contentType, filename)
}

func (m *mockStorage) Delete(_ context.Context, key string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.files, key)
}
func (m *mockStorage) DeleteObject(ctx context.Context, key string) error {
	m.Delete(ctx, key)
	return nil
}
func (m *mockStorage) DeleteKeys(_ context.Context, _ []string) {}
func (m *mockStorage) ObjectURL(key string) string {
	return "https://cdn.example.com/" + key
}
func (m *mockStorage) KeyFromURL(rawURL string) string {
	for _, prefix := range []string{
		"https://cdn.example.com/",
		"http://rustfs:9000/test-bucket/",
		"https://s3.example.com/test-bucket/",
	} {
		if strings.HasPrefix(rawURL, prefix) {
			return strings.TrimPrefix(rawURL, prefix)
		}
	}
	return rawURL
}
func (m *mockStorage) CdnDomain() string { return "cdn.example.com" }

// mockStorageNoCdn is a mockStorage variant that returns an empty CdnDomain
// to simulate a private S3 / R2 / MinIO deployment where the operator has
// NOT configured a public-facing CDN domain. buildMarkdownURL must not
// persist `a.Url` for this shape — it would write a private bucket URL
// into markdown that no client can load.
type mockStorageNoCdn struct{ mockStorage }

func (m *mockStorageNoCdn) CdnDomain() string { return "" }
func (m *mockStorage) GetReader(_ context.Context, key string) (io.ReadCloser, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.getReaderCalls++
	if data, ok := m.files[key]; ok {
		return io.NopCloser(bytes.NewReader(data)), nil
	}
	return nil, fmt.Errorf("mockStorage GetReader: key not found: %q", key)
}
func (m *mockStorage) streamCopyCalls() (getReader, uploadStream int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.getReaderCalls, m.uploadStreamCalls
}
func (m *mockStorage) PresignGet(_ context.Context, key string, _ time.Duration) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.presignCalls = append(m.presignCalls, key)
	return "https://signed.example.com/" + key + "?X-Amz-Signature=mock", nil
}
func (m *mockStorage) PresignGetWithContentDisposition(_ context.Context, key string, _ time.Duration, contentDisposition string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.presignCalls = append(m.presignCalls, key)
	m.presignDispositions = append(m.presignDispositions, contentDisposition)
	u := url.URL{
		Scheme: "https",
		Host:   "signed.example.com",
		Path:   "/" + key,
	}
	q := u.Query()
	q.Set("X-Amz-Signature", "mock")
	if contentDisposition != "" {
		q.Set("response-content-disposition", contentDisposition)
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}
func (m *mockStorage) put(key string, data []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.files == nil {
		m.files = map[string][]byte{}
	}
	m.files[key] = append([]byte(nil), data...)
}

// seekableReadCloser adapts a *bytes.Reader (which is an io.ReadSeeker) into an
// io.ReadCloser, mirroring what LocalStorage.GetReader returns (an *os.File is
// seekable). Used to exercise proxyAttachmentDownload's http.ServeContent path.
type seekableReadCloser struct{ *bytes.Reader }

func (seekableReadCloser) Close() error { return nil }

// seekableMockStorage is a mockStorage whose GetReader returns a seekable body,
// so proxyAttachmentDownload takes the http.ServeContent branch (the local-disk
// shape) instead of the manual single-range fallback.
type seekableMockStorage struct{ mockStorage }

func (m *seekableMockStorage) GetReader(_ context.Context, key string) (io.ReadCloser, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if data, ok := m.files[key]; ok {
		return seekableReadCloser{bytes.NewReader(data)}, nil
	}
	return nil, fmt.Errorf("seekableMockStorage GetReader: key not found: %q", key)
}

// failingReader serves up to failAfter bytes and then errors on the next Read.
// It is forward-only (no Seek), so proxyAttachmentDownload routes it through the
// manual serveProxyRange path — letting us simulate a storage backend that dies
// mid-stream while the handler is discarding bytes to reach a Range start.
type failingReader struct {
	data      []byte
	pos       int
	failAfter int
}

func (f *failingReader) Read(p []byte) (int, error) {
	if f.pos >= f.failAfter {
		return 0, fmt.Errorf("simulated storage read failure at offset %d", f.pos)
	}
	if remaining := f.failAfter - f.pos; len(p) > remaining {
		p = p[:remaining]
	}
	n := copy(p, f.data[f.pos:])
	f.pos += n
	return n, nil
}

func (f *failingReader) Close() error { return nil }

// failingMockStorage returns a failingReader so the non-seekable Range path hits
// a read error partway through the skip-to-start CopyN.
type failingMockStorage struct {
	mockStorage
	failAfter int
}

func (m *failingMockStorage) GetReader(_ context.Context, key string) (io.ReadCloser, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if data, ok := m.files[key]; ok {
		return &failingReader{data: data, failAfter: m.failAfter}, nil
	}
	return nil, fmt.Errorf("failingMockStorage GetReader: key not found: %q", key)
}

// ---------------------------------------------------------------------------
// GetAttachmentContent tests (preview proxy)
// ---------------------------------------------------------------------------

// requireAttachmentDownloadHeaders asserts the security / disposition headers
// that proxyAttachmentDownload sets before choosing a Range branch are preserved
// on the final response, whether it went out as a 206 (partial) or a full 200.
// The Range/206 work must not drop Content-Type / Content-Disposition /
// Cache-Control: no-store / X-Content-Type-Options / preview CSP — the seekable
// (http.ServeContent) path in particular could silently clobber them.
func requireAttachmentDownloadHeaders(t *testing.T, header http.Header, wantFilename string) {
	t.Helper()
	if got, want := header.Get("Content-Type"), "application/octet-stream"; got != want {
		t.Fatalf("Content-Type = %q, want %q", got, want)
	}
	if got := header.Get("Content-Disposition"); got == "" || !strings.Contains(got, wantFilename) {
		t.Fatalf("Content-Disposition = %q, want non-empty containing %q", got, wantFilename)
	}
	if got, want := header.Get("Cache-Control"), "no-store"; got != want {
		t.Fatalf("Cache-Control = %q, want %q", got, want)
	}
	if got, want := header.Get("X-Content-Type-Options"), "nosniff"; got != want {
		t.Fatalf("X-Content-Type-Options = %q, want %q", got, want)
	}
	requireAttachmentPreviewCSP(t, header)
}

func requireAttachmentPreviewCSP(t *testing.T, header http.Header, extraAncestors ...string) {
	t.Helper()
	csp := header.Get("Content-Security-Policy")
	if csp == "" {
		t.Fatal("Content-Security-Policy header is missing")
	}
	for _, directive := range []string{
		"default-src 'none'",
		"frame-ancestors 'self'",
		"object-src 'none'",
		"base-uri 'none'",
		"form-action 'none'",
	} {
		if !strings.Contains(csp, directive) {
			t.Fatalf("Content-Security-Policy missing %q; got %q", directive, csp)
		}
	}
	for _, ancestor := range extraAncestors {
		if !strings.Contains(csp, ancestor) {
			t.Fatalf("Content-Security-Policy missing frame ancestor %q; got %q", ancestor, csp)
		}
	}
	if strings.Contains(csp, "frame-ancestors 'none'") {
		t.Fatalf("Content-Security-Policy still blocks same-origin previews: %q", csp)
	}
}

func TestAttachmentPreviewCSPHeader_AllowsConfiguredFrontendOrigins(t *testing.T) {
	csp := attachmentPreviewCSPHeader([]string{
		"https://app.example.test",
		" https://App.Example.Test/some/path ",
		"http://localhost:3000",
		"*",
		"javascript:alert(1)",
		"not a url",
	})

	for _, want := range []string{
		"frame-ancestors 'self' https://app.example.test http://localhost:3000",
		"default-src 'none'",
		"object-src 'none'",
	} {
		if !strings.Contains(csp, want) {
			t.Fatalf("Content-Security-Policy missing %q; got %q", want, csp)
		}
	}
	for _, reject := range []string{"*", "javascript:", "not a url", "some/path"} {
		if strings.Contains(csp, reject) {
			t.Fatalf("Content-Security-Policy includes rejected source %q; got %q", reject, csp)
		}
	}
	if strings.Count(csp, "https://app.example.test") != 1 {
		t.Fatalf("Content-Security-Policy should dedupe origins; got %q", csp)
	}
}

func testCloudFrontSigner(t *testing.T) *auth.CloudFrontSigner {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate CloudFront test key: %v", err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	})
	t.Setenv("CLOUDFRONT_KEY_PAIR_ID", "KTEST")
	t.Setenv("CLOUDFRONT_DOMAIN", "static.example.test")
	t.Setenv("COOKIE_DOMAIN", ".example.test")
	t.Setenv("CLOUDFRONT_PRIVATE_KEY", base64.StdEncoding.EncodeToString(pemBytes))
	t.Setenv("CLOUDFRONT_PRIVATE_KEY_SECRET", "")
	signer := auth.NewCloudFrontSignerFromEnv()
	if signer == nil {
		t.Fatal("expected CloudFront signer")
	}
	return signer
}

// rangeBody is a deterministic 4 KiB payload used by the Range tests so we can
// assert that a partial response's bytes match the corresponding slice of the
// full object.
func rangeBody() []byte {
	b := make([]byte, 4096)
	for i := range b {
		b[i] = byte(i % 251) // 251 is prime → no alignment with 256 boundaries
	}
	return b
}

func TestParseSingleByteRange(t *testing.T) {
	const size = 1000
	tests := []struct {
		name        string
		header      string
		size        int64
		wantOutcome rangeParseOutcome
		wantStart   int64
		wantLength  int64
	}{
		{"full closed range", "bytes=0-1023", 2000, rangeSatisfiable, 0, 1024},
		{"open-ended range", "bytes=500-", size, rangeSatisfiable, 500, 500},
		{"end clamped to eof", "bytes=900-5000", size, rangeSatisfiable, 900, 100},
		{"suffix range", "bytes=-200", size, rangeSatisfiable, 800, 200},
		{"suffix larger than size clamps", "bytes=-5000", size, rangeSatisfiable, 0, 1000},
		{"single byte", "bytes=0-0", size, rangeSatisfiable, 0, 1},
		{"last byte", "bytes=999-999", size, rangeSatisfiable, 999, 1},
		{"start at eof unsatisfiable", "bytes=1000-1001", size, rangeUnsatisfiable, 0, 0},
		{"start past eof unsatisfiable", "bytes=2000-", size, rangeUnsatisfiable, 0, 0},
		{"end before start", "bytes=500-499", size, rangeUnsatisfiable, 0, 0},
		{"missing unit", "0-100", size, rangeUnsatisfiable, 0, 0},
		// Multi-range is a valid-but-unsupported form: ignored → full body (200),
		// not 416. This is the must-fix that stops the non-seekable path from
		// diverging from the seekable (ServeContent) path on multi-range.
		{"multi-range ignored", "bytes=0-10,20-30", size, rangeUnsupported, 0, 0},
		{"empty spec", "bytes=", size, rangeUnsatisfiable, 0, 0},
		{"no dash", "bytes=100", size, rangeUnsatisfiable, 0, 0},
		{"suffix zero", "bytes=-0", size, rangeUnsatisfiable, 0, 0},
		{"negative garbage", "bytes=abc-def", size, rangeUnsatisfiable, 0, 0},
		// Empty object (size == 0): a WELL-FORMED range has no bytes to satisfy it,
		// so it is ignored → full (empty) 200, matching stdlib http.ServeContent
		// rather than diverging with a 416. This covers the maintainer's
		// zero-length nit. Genuinely malformed / unknown-unit ranges against an
		// empty object still yield rangeUnsatisfiable (416), also matching stdlib.
		{"suffix on empty object", "bytes=-100", 0, rangeUnsupported, 0, 0},
		{"start range on empty object", "bytes=0-", 0, rangeUnsupported, 0, 0},
		{"mid range on empty object", "bytes=5-10", 0, rangeUnsupported, 0, 0},
		{"closed range on empty object", "bytes=0-99", 0, rangeUnsupported, 0, 0},
		// stdlib checks start-vs-size before validating the end, so on an empty
		// object it ignores (200) even a range with a bad end or reversed bounds;
		// mirror that. The paired non-empty rows below lock the other half of the
		// contract: once the start is IN range, the end IS validated → 416.
		{"bad end on empty object", "bytes=0-abc", 0, rangeUnsupported, 0, 0},
		{"reversed on empty object", "bytes=10-5", 0, rangeUnsupported, 0, 0},
		{"bad end on non-empty", "bytes=0-abc", size, rangeUnsatisfiable, 0, 0},
		{"reversed on non-empty", "bytes=10-5", size, rangeUnsatisfiable, 0, 0},
		{"unknown unit on empty object", "items=0-10", 0, rangeUnsatisfiable, 0, 0},
		{"malformed start on empty object", "bytes=abc-def", 0, rangeUnsatisfiable, 0, 0},
		{"no dash on empty object", "bytes=100", 0, rangeUnsatisfiable, 0, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			start, length, outcome := parseSingleByteRange(tc.header, tc.size)
			if outcome != tc.wantOutcome {
				t.Fatalf("outcome = %v, want %v (start=%d length=%d)", outcome, tc.wantOutcome, start, length)
			}
			if outcome != rangeSatisfiable {
				return
			}
			if start != tc.wantStart || length != tc.wantLength {
				t.Fatalf("got (start=%d, length=%d), want (start=%d, length=%d)", start, length, tc.wantStart, tc.wantLength)
			}
		})
	}
}

func TestShouldProxyAttachmentURL(t *testing.T) {
	cases := []struct {
		raw  string
		want bool
	}{
		{"http://rustfs:9000/test-bucket/file.txt", true},
		{"http://localhost:9000/test-bucket/file.txt", true},
		{"http://127.0.0.1:9000/test-bucket/file.txt", true},
		{"http://10.0.2.15/test-bucket/file.txt", true},
		{"https://minio.internal/test-bucket/file.txt", true},
		{"/uploads/workspaces/abc/file.txt", true},
		{"https://s3.example.com/test-bucket/file.txt", false},
		{"https://bucket.s3.us-east-1.amazonaws.com/file.txt", false},
	}
	for _, tc := range cases {
		t.Run(tc.raw, func(t *testing.T) {
			if got := shouldProxyAttachmentURL(tc.raw); got != tc.want {
				t.Fatalf("shouldProxyAttachmentURL(%q) = %v, want %v", tc.raw, got, tc.want)
			}
		})
	}
}

// isTextPreviewable is the whitelist linkpin between the proxy and the
// client-side dispatcher. Regress against the most common content types so
// drifting one of the lists alone fails loud.
func TestIsTextPreviewable(t *testing.T) {
	t.Helper()
	cases := []struct {
		name        string
		contentType string
		filename    string
		want        bool
	}{
		{"markdown by ext", "application/octet-stream", "README.md", true},
		{"markdown by mime", "text/markdown", "README", true},
		{"plain text", "text/plain", "log.txt", true},
		{"json by mime", "application/json", "data.json", true},
		{"yaml by ext", "application/octet-stream", "config.yml", true},
		{"go source", "text/plain", "main.go", true},
		{"typescript", "application/octet-stream", "index.ts", true},
		{"html", "text/html", "page.html", true},
		{"dockerfile no ext", "application/octet-stream", "Dockerfile", true},
		{"makefile no ext", "application/octet-stream", "Makefile", true},
		{"env dotfile", "application/octet-stream", ".env", true},
		{"gitignore dotfile", "application/octet-stream", ".gitignore", true},
		{"dockerfile extension", "application/octet-stream", "service.dockerfile", true},
		{"makefile extension", "application/octet-stream", "rules.makefile", true},

		{"pdf rejected", "application/pdf", "doc.pdf", false},
		{"png rejected", "image/png", "shot.png", false},
		{"video rejected", "video/mp4", "clip.mp4", false},
		{"binary fallthrough", "application/octet-stream", "blob.bin", false},
		{"docx rejected", "application/vnd.openxmlformats-officedocument.wordprocessingml.document", "report.docx", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isTextPreviewable(tc.contentType, tc.filename); got != tc.want {
				t.Errorf("isTextPreviewable(%q, %q) = %v, want %v", tc.contentType, tc.filename, got, tc.want)
			}
		})
	}
}

// MUL-3192 — buildMarkdownURL must emit a durable, absolute-when-possible
// URL that loads natively in any client (web, desktop, mobile webview).
// `download_url` may be a short-lived signed URL and is unsafe to persist;
// `markdown_url` is the contract for "ok to embed in markdown body".
//
// Matrix:
//
//   - public CDN durable URL ............... reuse a.Url verbatim
//   - LocalStorage with PublicURL set ....... reuse a.Url (already absolute)
//   - CloudFront-signed mode ................ never reuse a.Url (raw S3),
//                                              prefer absolute API endpoint
//   - LocalStorage relative + PublicURL set . prefix to absolute API endpoint
//   - PublicURL unset ....................... fall back to site-relative
//                                              (web's Next rewrite handles it)
//   - signed URL (CloudFront-signed leaked
//     into a.Url somehow) ................... reject as durable, fall through
//                                              to API endpoint to avoid
//                                              re-opening MUL-3130

func TestIsDurablePublicURL(t *testing.T) {
	cases := []struct {
		name string
		url  string
		want bool
	}{
		{"absolute https no signature", "https://cdn.multica.test/foo.png", true},
		{"absolute http no signature", "http://cdn.multica.test/foo.png", true},
		{"absolute with port + path", "https://cdn.example.test:8080/a/b/c.png", true},
		{"empty string", "", false},
		{"site-relative", "/uploads/abc.png", false},
		{"protocol-relative", "//cdn.example/foo.png", false},
		{"data URL", "data:image/png;base64,abc", false},
		{"blob URL", "blob:https://app/abc", false},
		{"unsupported scheme", "ftp://server/foo", false},
		{"cloudfront-signed Signature", "https://cdn.example/foo.png?Signature=abc&Key-Pair-Id=K1", false},
		{"cloudfront-signed Key-Pair-Id alone", "https://cdn.example/foo.png?Key-Pair-Id=K1", false},
		{"s3-presigned X-Amz-Signature", "https://bucket.s3/foo.png?X-Amz-Signature=abc", false},
		{"s3-presigned X-Amz-Expires alone", "https://bucket.s3/foo.png?X-Amz-Expires=900", false},
		{"plain Expires query", "https://cdn.example/foo.png?Expires=99", false},
		{"unrelated query", "https://cdn.example/foo.png?cache=1", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isDurablePublicURL(tc.url); got != tc.want {
				t.Errorf("isDurablePublicURL(%q) = %v, want %v", tc.url, got, tc.want)
			}
		})
	}
}

// TestServeLocalUpload_RelaxesFrameAncestorsForPreview covers the self-hosted
// local-disk case where document previews (PDF/HTML) are fetched straight from
// the public /uploads/* static route. That route inherits the global
// "frame-ancestors 'none'" CSP from the middleware, which blocks iframe
// previews; ServeLocalUpload must overwrite it with the same relaxed preview
// policy the /api/attachments download endpoint uses. See MUL-3821 / #4477.
func TestServeLocalUpload_RelaxesFrameAncestorsForPreview(t *testing.T) {
	dir := t.TempDir()
	key := "workspaces/ws-1/preview.pdf"
	full := filepath.Join(dir, filepath.FromSlash(key))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	const body = "%PDF-1.7 local-disk preview"
	if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	t.Setenv("LOCAL_UPLOAD_DIR", dir)
	t.Setenv("LOCAL_UPLOAD_BASE_URL", "")
	local := storage.NewLocalStorageFromEnv()
	if local == nil {
		t.Fatal("NewLocalStorageFromEnv returned nil")
	}

	h := &Handler{
		Storage: local,
		cfg:     Config{AttachmentFrameAncestors: []string{"https://app.example.test"}},
	}

	req := httptest.NewRequest(http.MethodGet, "/uploads/"+key, nil)
	w := httptest.NewRecorder()
	// Simulate the global CSP middleware having already stamped the strict
	// policy on the response before the static route runs.
	w.Header().Set("Content-Security-Policy", "default-src 'self'; frame-ancestors 'none'")

	h.ServeLocalUpload(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%q", w.Code, w.Body.String())
	}
	if got := w.Body.String(); got != body {
		t.Fatalf("body = %q, want %q", got, body)
	}
	requireAttachmentPreviewCSP(t, w.Header(), "https://app.example.test")
}

// TestServeLocalUpload_NonLocalStorage404 guards the defensive branch: the
// /uploads/* route is only registered under local storage, but the handler
// must not serve anything when the backing store is not local disk.
func TestServeLocalUpload_NonLocalStorage404(t *testing.T) {
	h := &Handler{Storage: &mockStorage{}}
	req := httptest.NewRequest(http.MethodGet, "/uploads/workspaces/ws-1/x.png", nil)
	w := httptest.NewRecorder()
	h.ServeLocalUpload(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
}
