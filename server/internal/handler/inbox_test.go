package handler

import (
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestInboxListBodyPreview(t *testing.T) {
	issue := pgtype.UUID{Bytes: uuid.New(), Valid: true}
	text := func(s string) pgtype.Text { return pgtype.Text{String: s, Valid: true} }
	long := strings.Repeat("a", 5000)
	// Every CJK character is three bytes in UTF-8: a byte-based cut would land
	// mid-character and produce invalid UTF-8.
	longCJK := strings.Repeat("评论内容", 500)

	cases := []struct {
		name      string
		notifType string
		issueID   pgtype.UUID
		body      pgtype.Text
		want      *string
	}{
		{"null body stays null", "new_comment", issue, pgtype.Text{}, nil},
		{"short comment is untouched", "new_comment", issue, text("looks good"), ptr("looks good")},
		{"exactly at the limit is untouched", "new_comment", issue,
			text(strings.Repeat("a", inboxListBodyPreviewLimit)),
			ptr(strings.Repeat("a", inboxListBodyPreviewLimit))},
		{"one past the limit is cut, ellipsis included", "new_comment", issue,
			text(strings.Repeat("a", inboxListBodyPreviewLimit+1)),
			ptr(strings.Repeat("a", inboxListBodyPreviewLimit-1) + "…")},
		{"long comment is cut to the limit", "new_comment", issue, text(long),
			ptr(strings.Repeat("a", inboxListBodyPreviewLimit-1) + "…")},
		// Issue-less notifications render their body in the detail pane from
		// the list cache, so shortening them would lose content.
		{"comment without an issue keeps its full body", "new_comment", pgtype.UUID{}, text(long), ptr(long)},
		// Other types are out of scope even when issue-backed: their body is
		// not merely a preview of something the issue page shows.
		{"other types keep their full body", "task_failed", issue, text(long), ptr(long)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := inboxListBody(tc.notifType, tc.issueID, tc.body)
			switch {
			case tc.want == nil && got != nil:
				t.Fatalf("body = %q, want nil", *got)
			case tc.want != nil && got == nil:
				t.Fatalf("body = nil, want %d characters", utf8.RuneCountInString(*tc.want))
			case tc.want != nil && *got != *tc.want:
				t.Fatalf("body = %d characters %q…, want %d characters",
					utf8.RuneCountInString(*got), truncateForLog(*got),
					utf8.RuneCountInString(*tc.want))
			}
		})
	}

	t.Run("multi-byte text is cut on a character boundary", func(t *testing.T) {
		got := inboxListBody("new_comment", issue, text(longCJK))
		if got == nil {
			t.Fatal("body = nil")
		}
		if !utf8.ValidString(*got) {
			t.Fatalf("preview is not valid UTF-8: %q", truncateForLog(*got))
		}
		if n := utf8.RuneCountInString(*got); n != inboxListBodyPreviewLimit {
			t.Fatalf("preview = %d characters, want %d", n, inboxListBodyPreviewLimit)
		}
		if !strings.HasSuffix(*got, "…") {
			t.Fatalf("preview does not end with an ellipsis: %q", truncateForLog(*got))
		}
	})
}

func truncateForLog(s string) string {
	if r := []rune(s); len(r) > 40 {
		return string(r[:40])
	}
	return s
}
