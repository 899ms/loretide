package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// Phase 1 of MUL-5372 / GitHub #5999: bulk responses stop pre-signing
// attachment download URLs for callers that advertise they can resolve the
// stable path themselves. The whole design rests on the server default never
// moving, so most of what these tests pin is what happens when a caller says
// NOTHING.

func requestWithCapabilities(caps string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/api/issues/x/comments", nil)
	if caps != "" {
		r.Header.Set("X-Client-Capabilities", caps)
	}
	return r
}

// TestAttachmentURLMode_DefaultsToSignedWithoutCapability is the compatibility
// promise itself: a caller that does not advertise the capability — every
// installed mobile build, every third-party script — must get exactly the
// pre-signed URL it gets today.
func TestAttachmentURLMode_DefaultsToSignedWithoutCapability(t *testing.T) {
	for _, tc := range []struct {
		name string
		caps string
	}{
		{"no header at all", ""},
		{"unrelated capabilities", "chat_draft_restore,something_else"},
		{"near-miss token", "stable_attachment_url"},
		{"empty header", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := attachmentURLModeFromRequest(requestWithCapabilities(tc.caps)); got != attachmentURLModeSigned {
				t.Errorf("caps %q resolved to stable mode; the server default must never move", tc.caps)
			}
		})
	}
	// A nil request has no declaration to read either.
	if got := attachmentURLModeFromRequest(nil); got != attachmentURLModeSigned {
		t.Errorf("nil request must resolve to signed mode")
	}
}

func TestAttachmentURLMode_HonorsAdvertisedCapability(t *testing.T) {
	for _, caps := range []string{
		ClientCapabilityStableAttachmentURLs,
		"chat_draft_restore," + ClientCapabilityStableAttachmentURLs,
		ClientCapabilityStableAttachmentURLs + ",other",
		"  " + ClientCapabilityStableAttachmentURLs + "  ",
	} {
		if got := attachmentURLModeFromRequest(requestWithCapabilities(caps)); got != attachmentURLModeStable {
			t.Errorf("caps %q did not resolve to stable mode", caps)
		}
	}
}
