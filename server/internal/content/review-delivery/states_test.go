package reviewdelivery

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// The state machines, the conditional requirements and the two derived
// displays. No database: these are pure functions, and the matrices belong
// beside them rather than being re-run through a handler.

func TestTheControlledSetsAreExactlyWhatTheSOPGives(t *testing.T) {
	for _, item := range []struct {
		name string
		got  []string
		want []string
	}{
		{"channel", asStrings(Channels), []string{"xiaohongshu", "wechat_mp", "douyin", "shipinhao"}},
		{"review status", asStrings(ReviewStatuses), []string{
			"pending", "changes_requested", "approved", "rejected", "cancelled"}},
		{"delivery status", asStrings(DeliveryStatuses), []string{
			"draft", "ready", "scheduled", "handed_off", "cancelled", "held"}},
		{"publication status", asStrings(PublicationStatuses), []string{
			"reported_published", "verified_published", "failed", "removed", "unknown"}},
		{"handoff method", asStrings(HandoffMethods), []string{"export", "copy", "handed_to_operator"}},
		{"version match", asStrings(VersionMatches), []string{"matched", "differs", "unknown"}},
		{"subject kind", asStrings(SubjectKinds), []string{"review_request", "delivery_task"}},
	} {
		if strings.Join(item.got, ",") != strings.Join(item.want, ",") {
			t.Errorf("%s set is %v, SOP gives %v", item.name, item.got, item.want)
		}
	}
}

func asStrings[T ~string](values []T) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, string(value))
	}
	return out
}

func TestEachControlledSetRejectsAValueOutsideItAndNamesTheField(t *testing.T) {
	for _, item := range []struct {
		field    string
		validate func(string) error
		outside  string
	}{
		{"channel", ValidateChannel, "weibo"},
		{"status", ValidateReviewStatus, "in_review"},
		{"status", ValidateDeliveryStatus, "published"},
		{"status", ValidatePublicationStatus, "live"},
		{"handoff_method", ValidateHandoffMethod, "api_publish"},
		{"version_match", ValidateVersionMatch, "partial"},
		{"subject_kind", ValidateSubjectKind, "publication_record"},
	} {
		err := item.validate(item.outside)
		if !errors.Is(err, ErrInvalid) {
			t.Errorf("%s accepted %q", item.field, item.outside)
			continue
		}
		var fieldErr FieldError
		if !errors.As(err, &fieldErr) || fieldErr.Field != item.field {
			t.Errorf("%s rejection did not name the field: %v", item.field, err)
		}
	}
}

// SOP 7.1's review row: pending is the only start, the other four are terminal.
func TestReviewTransitionsAreExactlyTheSOPsFour(t *testing.T) {
	legal := map[ReviewStatus]bool{
		ReviewChangesRequested: true, ReviewApproved: true,
		ReviewRejected: true, ReviewCancelled: true,
	}
	for _, to := range ReviewStatuses {
		if got := CanTransitionReview(ReviewPending, to); got != legal[to] {
			t.Errorf("pending -> %s: got %v, want %v", to, got, legal[to])
		}
	}
	// No terminal status may move anywhere, including back to pending: a
	// cancelled request is re-submitted as a NEW one.
	for from := range legal {
		for _, to := range ReviewStatuses {
			if CanTransitionReview(from, to) {
				t.Errorf("terminal %s moved to %s", from, to)
			}
		}
	}
}

func TestDeliveryTransitionsAreExactlyTheContractsGraph(t *testing.T) {
	want := map[DeliveryStatus]map[DeliveryStatus]bool{
		DeliveryDraft:     {DeliveryReady: true, DeliveryCancelled: true},
		DeliveryReady:     {DeliveryScheduled: true, DeliveryHandedOff: true, DeliveryHeld: true, DeliveryCancelled: true},
		DeliveryScheduled: {DeliveryHandedOff: true, DeliveryHeld: true, DeliveryCancelled: true},
		DeliveryHeld:      {DeliveryReady: true, DeliveryCancelled: true},
		DeliveryHandedOff: {},
		DeliveryCancelled: {},
	}
	for from, allowed := range want {
		for _, to := range DeliveryStatuses {
			if got := CanTransitionDelivery(from, to); got != allowed[to] {
				t.Errorf("%s -> %s: got %v, want %v", from, to, got, allowed[to])
			}
		}
	}
}

// The named illegal moves, spelled out rather than left to the matrix above, so
// a failure says which product rule broke.
func TestTheNamedIllegalMovesAreRefused(t *testing.T) {
	if CanTransitionDelivery(DeliveryCancelled, DeliveryReady) {
		t.Error("a cancelled task came back to life")
	}
	if CanTransitionDelivery(DeliveryHandedOff, DeliveryHeld) {
		t.Error("a handed-off task was moved again")
	}
	if CanTransitionDelivery(DeliveryDraft, DeliveryHandedOff) {
		t.Error("a draft task skipped straight to handed off")
	}
	if CanTransitionReview(ReviewApproved, ReviewChangesRequested) {
		t.Error("an approved request was re-disposed")
	}
	if err := ValidateReviewDecision(ReviewPending, ReviewPending, ""); !errors.Is(err, ErrInvalid) {
		t.Error("pending was accepted as a disposition")
	}
}

func TestATransitionRefusalNamesBothEnds(t *testing.T) {
	err := ValidateDeliveryAdvance(DeliveryCancelled, DeliveryAdvance{
		To: DeliveryReady, Reason: "changed my mind",
	})
	var transitionErr TransitionError
	if !errors.As(err, &transitionErr) {
		t.Fatalf("got %v, want a TransitionError", err)
	}
	if transitionErr.From != "cancelled" || transitionErr.To != "ready" {
		t.Errorf("refusal says %q -> %q", transitionErr.From, transitionErr.To)
	}
	if !errors.Is(err, ErrInvalid) {
		t.Error("a transition refusal should answer as 400, not as storage")
	}
}

// SOP 9.2: "运营者可标记实际发布失败、延后或取消，并注明原因". Four cases, each
// naming its own missing field - a single combined case would let one half
// break behind the other.
func TestTheConditionallyRequiredFieldsAreNamedOneByOne(t *testing.T) {
	future := time.Now().Add(time.Hour)
	for _, item := range []struct {
		name  string
		from  DeliveryStatus
		input DeliveryAdvance
		field string
	}{
		{"scheduled without a time", DeliveryReady, DeliveryAdvance{To: DeliveryScheduled}, "scheduled_at"},
		{"handed off without a method", DeliveryReady, DeliveryAdvance{To: DeliveryHandedOff}, "handoff_method"},
		{"held without a reason", DeliveryReady, DeliveryAdvance{To: DeliveryHeld}, "reason"},
		{"cancelled without a reason", DeliveryReady, DeliveryAdvance{To: DeliveryCancelled}, "reason"},
	} {
		err := ValidateDeliveryAdvance(item.from, item.input)
		var fieldErr FieldError
		if !errors.As(err, &fieldErr) {
			t.Errorf("%s: got %v, want a named field", item.name, err)
			continue
		}
		if fieldErr.Field != item.field {
			t.Errorf("%s: named %q, want %q", item.name, fieldErr.Field, item.field)
		}
	}
	// And the same four succeed once the field is there.
	for _, ok := range []DeliveryAdvance{
		{To: DeliveryScheduled, ScheduledAt: &future},
		{To: DeliveryHandedOff, HandoffMethod: HandoffExport},
		{To: DeliveryHeld, Reason: "waiting for the cover image"},
		{To: DeliveryCancelled, Reason: "the piece was dropped"},
	} {
		if err := ValidateDeliveryAdvance(DeliveryReady, ok); err != nil {
			t.Errorf("%s with its field was still refused: %v", ok.To, err)
		}
	}
}

// SC-004. Three separate cases, not one: combined, either half could break
// behind the other.
func TestThePublicationRequirementsEachNameTheirOwnField(t *testing.T) {
	base := PublicationRecord{Channel: ChannelXiaohongshu, VersionMatch: VersionUnknown}
	for _, item := range []struct {
		name   string
		record PublicationRecord
		field  string
	}{
		{"reported without a link", withStatus(base, PublicationReported), "page_url_or_content_id"},
		{"verified without a link", withStatus(base, PublicationVerified), "page_url_or_content_id"},
		{"verified without a check", func() PublicationRecord {
			record := withStatus(base, PublicationVerified)
			record.PageURLOrContentID = "https://example.invalid/p/1"
			return record
		}(), "verification_note"},
		{"failed without a reason", withStatus(base, PublicationFailed), "receipt_note"},
		{"removed without a reason", withStatus(base, PublicationRemoved), "receipt_note"},
	} {
		err := ValidatePublicationRecord(item.record)
		var fieldErr FieldError
		if !errors.As(err, &fieldErr) {
			t.Errorf("%s: got %v, want a named field", item.name, err)
			continue
		}
		if fieldErr.Field != item.field {
			t.Errorf("%s: named %q, want %q", item.name, fieldErr.Field, item.field)
		}
	}
}

func withStatus(record PublicationRecord, status PublicationStatus) PublicationRecord {
	record.Status = status
	return record
}

// SOP 9.1 names unknown itself. Recording "I could not get the full text" must
// not be a failure and must not block anything.
func TestAnUnknownVersionMatchIsAcceptedAndIsNotAFailure(t *testing.T) {
	record := PublicationRecord{
		Channel: ChannelWechatMP, Status: PublicationReported,
		PageURLOrContentID: "content-id-42", VersionMatch: VersionUnknown,
	}
	if err := ValidatePublicationRecord(record); err != nil {
		t.Fatalf("unknown version match was refused: %v", err)
	}
	record.VersionMatch = "partial"
	err := ValidatePublicationRecord(record)
	var fieldErr FieldError
	if !errors.As(err, &fieldErr) || fieldErr.Field != "version_match" {
		t.Errorf("a fourth version match value was not named: %v", err)
	}
}

// There is no state machine over publication records, and that is the product
// rule, not an omission: a platform taking a piece down after it was verified
// is the ordinary case, and refusing it would stop people recording the truth.
func TestAPublicationRecordMayFollowAnyOtherStatus(t *testing.T) {
	for _, status := range PublicationStatuses {
		record := PublicationRecord{
			Channel: ChannelDouyin, Status: status,
			PageURLOrContentID: "https://example.invalid/p/1",
			VerificationNote:   "opened it myself",
			ReceiptNote:        "the platform took it down",
			VersionMatch:       VersionMatched,
		}
		if err := ValidatePublicationRecord(record); err != nil {
			t.Errorf("%s was refused with every field filled: %v", status, err)
		}
	}
}

// SOP 9.1: none of the three handover actions means published.
func TestNoHandoffMethodMeansPublished(t *testing.T) {
	for _, method := range HandoffMethods {
		if HandoffMeansPublished(method) {
			t.Errorf("%s was treated as a publication", method)
		}
	}
}

// Ruling Q2: draft may reference a pending review; everything past it may not.
func TestOnlyDraftIsExemptFromNeedingAnApprovedReview(t *testing.T) {
	if RequiresApprovedReview(DeliveryDraft) {
		t.Error("drafting a task required an approved review, which makes scheduling while waiting impossible")
	}
	for _, status := range []DeliveryStatus{
		DeliveryReady, DeliveryScheduled, DeliveryHandedOff, DeliveryHeld,
	} {
		if !RequiresApprovedReview(status) {
			t.Errorf("%s did not require an approved review", status)
		}
	}
}

// SOP 9.3. Four triggers, one comparison: written as four ifs at four call
// sites, one of them gets missed.
func TestEachOfTheFourTargetChangesSendsTheTaskBackToHeld(t *testing.T) {
	approved := DeliveryTarget{
		VersionID: "v3", AccountID: "acct-1", Channel: ChannelXiaohongshu,
		Attachments: []string{}, DeliveryConfig: map[string]string{"template": "a"},
	}
	for _, item := range []struct {
		name    string
		current DeliveryTarget
		expect  string
	}{
		{"a new draft chosen as the target", withVersion(approved, "v5"), "version"},
		{"a changed account", withAccount(approved, "acct-2"), "account"},
		{"a changed channel", withChannel(approved, ChannelDouyin), "channel"},
		{"changed attachments", withAttachments(approved, []string{"file-1"}), "attachments"},
		{"a changed channel configuration", withConfig(approved, map[string]string{"template": "b"}), "configuration"},
	} {
		hold, reason := ShouldHold(approved, item.current)
		if !hold {
			t.Errorf("%s did not hold the task", item.name)
			continue
		}
		if !strings.Contains(reason, item.expect) {
			t.Errorf("%s gave reason %q, which does not say what changed", item.name, reason)
		}
	}
	if hold, _ := ShouldHold(approved, approved); hold {
		t.Error("an unchanged target held the task, which would write a transition row saying nothing happened")
	}
}

func withVersion(target DeliveryTarget, version string) DeliveryTarget {
	target.VersionID = version
	return target
}
func withAccount(target DeliveryTarget, account string) DeliveryTarget {
	target.AccountID = account
	return target
}
func withChannel(target DeliveryTarget, channel Channel) DeliveryTarget {
	target.Channel = channel
	return target
}
func withAttachments(target DeliveryTarget, attachments []string) DeliveryTarget {
	target.Attachments = attachments
	return target
}
func withConfig(target DeliveryTarget, config map[string]string) DeliveryTarget {
	target.DeliveryConfig = config
	return target
}

func TestOnlyAReadyOrScheduledTaskCanBeHeldByATargetChange(t *testing.T) {
	for status, want := range map[DeliveryStatus]bool{
		DeliveryReady: true, DeliveryScheduled: true,
		DeliveryDraft: false, DeliveryHeld: false,
		DeliveryHandedOff: false, DeliveryCancelled: false,
	} {
		if got := HoldableStatus(status); got != want {
			t.Errorf("%s holdable: got %v, want %v", status, got, want)
		}
	}
}

// SOP 9.1's "到期后产生站内待办" is this comparison and nothing else.
func TestDueIsAComparisonAndNotAnEvent(t *testing.T) {
	now := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
	past := now.Add(-time.Hour)
	future := now.Add(time.Hour)
	for _, item := range []struct {
		name      string
		status    DeliveryStatus
		scheduled *time.Time
		want      bool
	}{
		{"scheduled and past", DeliveryScheduled, &past, true},
		{"scheduled and still ahead", DeliveryScheduled, &future, false},
		{"scheduled for exactly now", DeliveryScheduled, &now, true},
		{"already handed off", DeliveryHandedOff, &past, false},
		{"held with a past time", DeliveryHeld, &past, false},
		{"scheduled with no time at all", DeliveryScheduled, nil, false},
	} {
		if got := IsDue(item.status, item.scheduled, now); got != item.want {
			t.Errorf("%s: got %v, want %v", item.name, got, item.want)
		}
	}
}

// SOP 9.2's "待登记" - derived, and only ever about what nobody wrote down.
func TestPendingRegistrationIsOnlyAboutAMissingRecord(t *testing.T) {
	if !IsPendingRegistration(DeliveryHandedOff, 0) {
		t.Error("a handed-off task with no record was not pending registration")
	}
	if IsPendingRegistration(DeliveryHandedOff, 1) {
		t.Error("a task with a record was still pending registration")
	}
	for _, status := range []DeliveryStatus{
		DeliveryDraft, DeliveryReady, DeliveryScheduled, DeliveryHeld, DeliveryCancelled,
	} {
		if IsPendingRegistration(status, 0) {
			t.Errorf("%s was pending registration although nothing was handed over", status)
		}
	}
}

// The eight keys of SOP 8's snapshot, pinned. One more or one fewer is red.
func TestTheDeliverySnapshotHasExactlyEightKeys(t *testing.T) {
	snapshot := NewDeliverySnapshot(ChannelXiaohongshu, "work-1", "artifact-1", "v3", "acct-1", "")
	keys, err := SnapshotKeysOf(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != len(SnapshotKeys) {
		t.Fatalf("snapshot has %d keys (%v), SOP 8 gives %d (%v)",
			len(keys), keys, len(SnapshotKeys), SnapshotKeys)
	}
	present := map[string]bool{}
	for _, key := range keys {
		present[key] = true
	}
	for _, want := range SnapshotKeys {
		if !present[want] {
			t.Errorf("snapshot is missing %q", want)
		}
	}
}

// W-03 has not landed, so there is nothing an attachment could name. Asserting
// only "exactly eight keys" would catch a missing key and miss someone putting
// an id into this one.
func TestNothingCanPutAnAttachmentIntoASnapshot(t *testing.T) {
	for _, startSnapshot := range []string{"", "snap-1"} {
		snapshot := NewDeliverySnapshot(ChannelWechatMP, "work-1", "artifact-1", "v1", "acct-1", startSnapshot)
		if len(snapshot.Attachments) != 0 {
			t.Errorf("a new snapshot carried attachments %v", snapshot.Attachments)
		}
		if snapshot.Attachments == nil {
			t.Error("attachments is nil, which marshals to null; it has to be an empty array")
		}
	}
}
