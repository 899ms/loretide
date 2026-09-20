package handler

import (
	"bytes"
	"context"
	"crypto/sha256"
	"github.com/go-chi/chi/v5"
	"github.com/multica-ai/multica/server/internal/auth"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"
)

const capabilityTestAttachmentID = "11111111-2222-3333-4444-555555555555"

// capabilityQuery returns just the query string of a freshly minted
// capability, so tests can vary one field at a time.
func capabilityQuery(t *testing.T, attachmentID string, now time.Time) url.Values {
	t.Helper()
	path := attachmentCapabilityPath(attachmentID, now)
	idx := strings.Index(path, "?")
	if idx < 0 {
		t.Fatalf("capability path has no query: %q", path)
	}
	values, err := url.ParseQuery(path[idx+1:])
	if err != nil {
		t.Fatalf("parse capability query: %v", err)
	}
	return values
}

// newCapabilityRequest deliberately sets NO authentication headers. The whole
// point of the capability route is that it works for a native download that
// carries neither a Bearer token nor a session cookie.
func newCapabilityRequest(attachmentID string, query url.Values) (*http.Request, *httptest.ResponseRecorder) {
	req := httptest.NewRequest("GET", "/api/attachments/"+attachmentID+"/signed-download?"+query.Encode(), nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", attachmentID)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	return req, httptest.NewRecorder()
}

// ---------------------------------------------------------------------------
// Signature unit tests (no DB)
// ---------------------------------------------------------------------------

func TestAttachmentCapability_RoundTrips(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	query := capabilityQuery(t, capabilityTestAttachmentID, now)

	if !verifyAttachmentCapability(capabilityTestAttachmentID, query.Get("exp"), query.Get("sig"), "", now) {
		t.Fatal("freshly minted capability did not verify")
	}
	// Still valid one second before the TTL elapses.
	if !verifyAttachmentCapability(
		capabilityTestAttachmentID, query.Get("exp"), query.Get("sig"), "",
		now.Add(attachmentCapabilityTTL-time.Second),
	) {
		t.Fatal("capability expired before its TTL elapsed")
	}
}

func TestAttachmentCapability_FailsClosed(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	valid := capabilityQuery(t, capabilityTestAttachmentID, now)

	// A capability minted for a DIFFERENT attachment must not verify against
	// this one — the id is inside the signed message, not just the URL.
	other := capabilityQuery(t, "99999999-8888-7777-6666-555555555555", now)

	tampered := []byte(valid.Get("sig"))
	if tampered[0] == 'a' {
		tampered[0] = 'b'
	} else {
		tampered[0] = 'a'
	}

	cases := []struct {
		name     string
		id       string
		exp      string
		sig      string
		now      time.Time
		expected bool
	}{
		{"valid", capabilityTestAttachmentID, valid.Get("exp"), valid.Get("sig"), now, true},
		{"expired", capabilityTestAttachmentID, valid.Get("exp"), valid.Get("sig"), now.Add(attachmentCapabilityTTL + time.Second), false},
		{"tampered signature", capabilityTestAttachmentID, valid.Get("exp"), string(tampered), now, false},
		{"extended expiry", capabilityTestAttachmentID, strconv.FormatInt(now.Add(24*time.Hour).Unix(), 10), valid.Get("sig"), now, false},
		{"signature for another attachment", capabilityTestAttachmentID, other.Get("exp"), other.Get("sig"), now, false},
		{"missing signature", capabilityTestAttachmentID, valid.Get("exp"), "", now, false},
		{"missing expiry", capabilityTestAttachmentID, "", valid.Get("sig"), now, false},
		{"missing id", "", valid.Get("exp"), valid.Get("sig"), now, false},
		{"non-numeric expiry", capabilityTestAttachmentID, "not-a-number", valid.Get("sig"), now, false},
		{"non-hex signature", capabilityTestAttachmentID, valid.Get("exp"), "zzzz", now, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := verifyAttachmentCapability(tc.id, tc.exp, tc.sig, "", tc.now); got != tc.expected {
				t.Fatalf("verify = %v, want %v", got, tc.expected)
			}
		})
	}
}

// TestAttachmentDownloadCapability_IntentIsDomainSeparated pins the follow-up
// download-intent split (dl=1): a forced-attachment capability verifies only
// under the attachment intent, and a load-intent link cannot be flipped to a
// forced download by appending dl=1 — its signature does not cover the intent.
func TestAttachmentDownloadCapability_IntentIsDomainSeparated(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	exp := now.Add(attachmentCapabilityTTL).Unix()
	expStr := strconv.FormatInt(exp, 10)

	loadSig := signAttachmentCapability(capabilityTestAttachmentID, exp)
	dlSig := signAttachmentCapabilityIntent(capabilityTestAttachmentID, exp, attachmentCapabilityDownloadIntent)

	if loadSig == dlSig {
		t.Fatal("load-intent and download-intent signatures must differ")
	}

	// The download capability verifies only under the download intent.
	if !verifyAttachmentCapability(capabilityTestAttachmentID, expStr, dlSig, attachmentCapabilityDownloadIntent, now) {
		t.Fatal("download capability did not verify under its own intent")
	}
	if verifyAttachmentCapability(capabilityTestAttachmentID, expStr, dlSig, "", now) {
		t.Fatal("download signature must not verify as a load-intent link")
	}

	// A load-intent link cannot be promoted to a forced-attachment download by
	// tacking on dl=1 (which the handler maps to the attachment intent).
	if verifyAttachmentCapability(capabilityTestAttachmentID, expStr, loadSig, attachmentCapabilityDownloadIntent, now) {
		t.Fatal("load signature must not verify as a download (dl=1) capability")
	}
	if !verifyAttachmentCapability(capabilityTestAttachmentID, expStr, loadSig, "", now) {
		t.Fatal("load capability must still verify under the empty intent")
	}

	// The minted download path carries dl=1 and a distinct signature.
	path := attachmentDownloadCapabilityPath(capabilityTestAttachmentID, now)
	if !strings.Contains(path, "dl=1") {
		t.Fatalf("download capability path missing dl=1: %q", path)
	}
	if strings.Contains(attachmentCapabilityPath(capabilityTestAttachmentID, now), "dl=1") {
		t.Fatal("load-intent capability path must not carry dl=1")
	}
}

// The capability key must not be the JWT secret, nor a bare hash of it. If the
// two signing domains ever shared a key, a capability signature and a session
// signature would be interchangeable.
func TestAttachmentCapability_KeyIsDomainSeparatedFromJWTSecret(t *testing.T) {
	key := attachmentCapabilitySigningKey()
	if bytes.Equal(key, auth.JWTSecret()) {
		t.Fatal("capability key equals the raw JWT secret")
	}
	bare := sha256.Sum256(auth.JWTSecret())
	if bytes.Equal(key, bare[:]) {
		t.Fatal("capability key is an undomained hash of the JWT secret")
	}
}

// ---------------------------------------------------------------------------
// Route tests
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// Minting
// ---------------------------------------------------------------------------
