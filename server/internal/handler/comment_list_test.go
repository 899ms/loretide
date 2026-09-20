package handler

import (
	"encoding/json"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"
)

// cursorQuery builds a properly URL-encoded query string for the recent +
// thread-cursor path. RFC3339Nano timestamps contain `:` and may contain `+`,
// both of which need escaping so they survive `(*url.URL).Query()` parsing on
// the server side.
//
// `before` and `beforeID` here name a *thread* (last_activity_at, root_id),
// not a single row — the recent path is thread-grouped (#2340).
func cursorQuery(recent int, before, beforeID string) string {
	v := url.Values{}
	if recent > 0 {
		v.Set("recent", strconv.Itoa(recent))
	}
	if before != "" {
		v.Set("before", before)
	}
	if beforeID != "" {
		v.Set("before_id", beforeID)
	}
	return v.Encode()
}

// nextThreadCursor reads the (before, before-id) headers the recent path
// emits when there is likely an older page to scroll to. Empty pair means
// the server signalled "no more threads".
func nextThreadCursor(w *httptest.ResponseRecorder) (string, string) {
	return w.Header().Get("X-Multica-Next-Before"), w.Header().Get("X-Multica-Next-Before-Id")
}

// commentListFixture seeds an issue with a known comment graph for the
// thread / recent / cursor tests. The shape:
//
//	root1 (oldest)
//	├── r1a
//	└── r1b
//	    └── r1b1   (nested reply — defends Elon's point 2: recursive root walk)
//	root2 (newer, separate thread)
//	├── r2a
//	└── r2b (newest overall)
//
// Each comment is inserted with an explicit created_at so ordering and
// cursor behavior are deterministic.
type commentListFixture struct {
	IssueID string
	Root1   string
	R1a     string
	R1b     string
	R1b1    string
	Root2   string
	R2a     string
	R2b     string
	Base    time.Time
}

func decodeComments(t *testing.T, body []byte) []CommentResponse {
	t.Helper()
	var resp []CommentResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("decode comments: %v", err)
	}
	return resp
}

func ids(rows []CommentResponse) []string {
	out := make([]string, len(rows))
	for i, c := range rows {
		out[i] = c.ID
	}
	return out
}

func eqIDs(t *testing.T, got, want []string, ctx string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s: ids len got=%d want=%d\ngot=%v\nwant=%v", ctx, len(got), len(want), got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("%s: ids[%d] got=%s want=%s\ngot=%v\nwant=%v", ctx, i, got[i], want[i], got, want)
		}
	}
}

func assertRootStat(t *testing.T, c CommentResponse, label string, wantReplies int, wantLastActivity time.Time) {
	t.Helper()
	if c.ReplyCount == nil {
		t.Fatalf("%s: reply_count missing", label)
	}
	if *c.ReplyCount != wantReplies {
		t.Fatalf("%s: reply_count got=%d want=%d", label, *c.ReplyCount, wantReplies)
	}
	if c.LastActivityAt == nil {
		t.Fatalf("%s: last_activity_at missing", label)
	}
	got, err := time.Parse(time.RFC3339, *c.LastActivityAt)
	if err != nil {
		t.Fatalf("%s: last_activity_at parse %q: %v", label, *c.LastActivityAt, err)
	}
	if !got.Equal(wantLastActivity) {
		t.Fatalf("%s: last_activity_at got=%s want=%s", label, got.UTC(), wantLastActivity.UTC())
	}
}

// nextReplyCursor reads the (before, before-id) headers the thread + tail
// path emits when there is likely an older page of replies inside the same
// thread. Same wire shape as the thread-cursor headers — context decides
// which (the caller knows whether they used --recent or --tail).
func nextReplyCursor(w *httptest.ResponseRecorder) (string, string) {
	return w.Header().Get("X-Multica-Next-Before"), w.Header().Get("X-Multica-Next-Before-Id")
}
