package handler

import (
	"encoding/json"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/channelmedia"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"strings"
	"testing"
)

func TestMergeIssueChannelMediaDescriptionProtectsLegacyClient(t *testing.T) {
	const id = "22222222-2222-4222-8222-222222222222"
	block := channelmedia.Block(id, "diagram.png", true)
	current := channelmedia.Append("Original", block)
	attachmentID := pgtype.UUID{Bytes: uuid.MustParse(id), Valid: true}

	got := mergeIssueChannelMediaDescription(current, "Original with local edit", nil, []db.Attachment{{
		ID: attachmentID, Filename: "diagram.png", ContentType: "image/png",
	}})
	want := channelmedia.Append("Original with local edit", block)
	if got != want {
		t.Fatalf("merged description = %q, want %q", got, want)
	}
}

func TestMergeIssueChannelMediaDescriptionAllowsKnownMediaDeletion(t *testing.T) {
	const id = "33333333-3333-4333-8333-333333333333"
	current := channelmedia.Append("Original", channelmedia.Block(id, "diagram.png", true))
	attachmentID := pgtype.UUID{Bytes: uuid.MustParse(id), Valid: true}

	got := mergeIssueChannelMediaDescription(current, "Original without image", &current, []db.Attachment{{
		ID: attachmentID, Filename: "diagram.png", ContentType: "image/png",
	}})
	if got != "Original without image" {
		t.Fatalf("explicit deletion was not preserved: %q", got)
	}
}

func TestMergeIssueChannelMediaDescriptionRestoresMarkerForRetainedLink(t *testing.T) {
	const id = "55555555-5555-4555-8555-555555555555"
	current := channelmedia.Append("Original", channelmedia.Block(id, "diagram.png", true))
	attachmentID := pgtype.UUID{Bytes: uuid.MustParse(id), Valid: true}
	incoming := channelmedia.Append("Local edit", "![]("+channelmedia.DownloadPath(id)+")")

	got := mergeIssueChannelMediaDescription(current, incoming, &current, []db.Attachment{{
		ID: attachmentID, Filename: "diagram.png", ContentType: "image/png",
	}})
	if !strings.Contains(got, incoming) || !channelmedia.HasMarker(got, id) {
		t.Fatalf("retained media lost provenance: %q", got)
	}
}

func TestMergeIssueChannelMediaDescriptionDoesNotResurrectDeletedAttachment(t *testing.T) {
	const id = "66666666-6666-4666-8666-666666666666"
	current := channelmedia.Append("Original", channelmedia.Block(id, "diagram.png", true))

	got := mergeIssueChannelMediaDescription(current, "Local edit", nil, nil)
	if got != "Local edit" {
		t.Fatalf("deleted attachment was resurrected: %q", got)
	}
}

func TestRefreshUntouchedNullableIssueParamsKeepsValidatedAssigneePair(t *testing.T) {
	oldID := pgtype.UUID{Bytes: uuid.MustParse("77777777-7777-4777-8777-777777777777"), Valid: true}
	concurrentID := pgtype.UUID{Bytes: uuid.MustParse("88888888-8888-4888-8888-888888888888"), Valid: true}
	params := db.UpdateIssueParams{
		AssigneeType: pgtype.Text{String: "member", Valid: true},
		AssigneeID:   oldID,
	}
	current := db.Issue{
		AssigneeType: pgtype.Text{String: "agent", Valid: true},
		AssigneeID:   concurrentID,
	}

	refreshUntouchedNullableIssueParams(&params, current, map[string]json.RawMessage{
		"assignee_id": json.RawMessage(`"77777777-7777-4777-8777-777777777777"`),
	})

	if params.AssigneeType.String != "member" || params.AssigneeID != oldID {
		t.Fatalf("validated assignee pair was recombined: type=%#v id=%#v", params.AssigneeType, params.AssigneeID)
	}
}
