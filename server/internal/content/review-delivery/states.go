package reviewdelivery

import "time"

// The state machines, the conditional-required rules, and the two derived
// displays. All pure functions: no database, no clock of their own.
//
// They are here rather than inline in store.go so that every rule has one
// place to be true and one place to be tested. SOP 9.3's automatic hold is the
// clearest case: it has four triggers (a new draft chosen as the delivery
// target, changed attachments, a changed account, a changed channel config),
// and written as four ifs at four call sites one of them would be missed.
// Written as one comparison, they are four inputs to it.

// reviewTransitions is SOP 7.1's review row. pending is the only start; the
// other four are terminal and cannot reach each other.
var reviewTransitions = map[ReviewStatus][]ReviewStatus{
	ReviewPending: {ReviewChangesRequested, ReviewApproved, ReviewRejected, ReviewCancelled},
	// Terminal. Re-submitting is a NEW request, not a revival of this one.
	ReviewChangesRequested: {},
	ReviewApproved:         {},
	ReviewRejected:         {},
	ReviewCancelled:        {},
}

// deliveryTransitions is SOP 7.1's delivery row plus this card's completion of
// where held enters from and returns to - 7.1 says only "可 cancelled / held".
var deliveryTransitions = map[DeliveryStatus][]DeliveryStatus{
	DeliveryDraft:     {DeliveryReady, DeliveryCancelled},
	DeliveryReady:     {DeliveryScheduled, DeliveryHandedOff, DeliveryHeld, DeliveryCancelled},
	DeliveryScheduled: {DeliveryHandedOff, DeliveryHeld, DeliveryCancelled},
	DeliveryHeld:      {DeliveryReady, DeliveryCancelled},
	// Terminal.
	DeliveryHandedOff: {},
	DeliveryCancelled: {},
}

// InitialReviewStatus is the only status a request can be created in.
const InitialReviewStatus = ReviewPending

// InitialDeliveryStatus is the only status a task can be created in.
const InitialDeliveryStatus = DeliveryDraft

// CanTransitionReview reports whether one review status may follow another.
func CanTransitionReview(from, to ReviewStatus) bool {
	for _, allowed := range reviewTransitions[from] {
		if allowed == to {
			return true
		}
	}
	return false
}

// CanTransitionDelivery reports whether one delivery status may follow another.
func CanTransitionDelivery(from, to DeliveryStatus) bool {
	for _, allowed := range deliveryTransitions[from] {
		if allowed == to {
			return true
		}
	}
	return false
}

// TransitionError says which move was refused, so the 400 can name both ends
// rather than only that something was wrong.
type TransitionError struct {
	From string
	To   string
}

func (e TransitionError) Error() string { return "cannot move from " + e.From + " to " + e.To }
func (e TransitionError) Unwrap() error { return ErrInvalid }

// PublicationRecords have no state machine, deliberately. Each row is one
// independent observation: a removed after a verified_published is a platform
// taking the piece down, which is exactly what someone needs to be able to
// record. "只前进不删" lands as "rows are never updated or deleted", not as
// "the status cannot go backwards".

// DeliveryAdvance is one requested move of a delivery task.
type DeliveryAdvance struct {
	To            DeliveryStatus
	ScheduledAt   *time.Time
	HandoffMethod HandoffMethod
	Reason        string
}

// ValidateDeliveryAdvance checks the controlled values, then the conditional
// requirements, then the transition itself - the contract's order 3, 5, 6.
//
// The conditional requirements are NOT column constraints: the same column is
// required or not depending on the target status, and a CHECK could only ever
// answer 23514, never "the thing you left out is the scheduled time".
func ValidateDeliveryAdvance(from DeliveryStatus, advance DeliveryAdvance) error {
	if err := ValidateDeliveryStatus(string(advance.To)); err != nil {
		return err
	}
	if advance.HandoffMethod != "" {
		if err := ValidateHandoffMethod(string(advance.HandoffMethod)); err != nil {
			return err
		}
	}
	if err := ValidateNote("reason", advance.Reason); err != nil {
		return err
	}
	switch advance.To {
	case DeliveryScheduled:
		if advance.ScheduledAt == nil || advance.ScheduledAt.IsZero() {
			return invalidField("scheduled_at")
		}
	case DeliveryHandedOff:
		if advance.HandoffMethod == "" {
			return invalidField("handoff_method")
		}
	case DeliveryHeld, DeliveryCancelled:
		// SOP 9.2: "运营者可标记实际发布失败、延后或取消，并注明原因".
		if advance.Reason == "" {
			return invalidField("reason")
		}
	}
	if !CanTransitionDelivery(from, advance.To) {
		return TransitionError{From: string(from), To: string(advance.To)}
	}
	return nil
}

// ValidateReviewDecision checks a disposition the same way.
func ValidateReviewDecision(from, to ReviewStatus, note string) error {
	if err := ValidateReviewStatus(string(to)); err != nil {
		return err
	}
	if err := ValidateNote("decision_note", note); err != nil {
		return err
	}
	if to == ReviewPending {
		// Not a disposition. Listed separately from the transition check so the
		// message is about what was asked for, not about a missing edge.
		return invalidField("status")
	}
	if !CanTransitionReview(from, to) {
		return TransitionError{From: string(from), To: string(to)}
	}
	return nil
}

// ValidatePublicationRecord checks the controlled values and the conditional
// requirements. There is no transition to check.
//
// Which fields are required comes from SOP 9.1 and 9.2, not from a guess:
// a page link or content id for the two published statuses, a verification note
// for the verified one, and a reason for failed / removed. The reason lands in
// receipt_note because the ruled field list has no reason column - flagged in
// the spec's 裁决记录 as the one place this card chose a field itself.
func ValidatePublicationRecord(record PublicationRecord) error {
	if err := ValidatePublicationStatus(string(record.Status)); err != nil {
		return err
	}
	if err := ValidateChannel(string(record.Channel)); err != nil {
		return err
	}
	if record.VersionMatch == "" {
		record.VersionMatch = VersionUnknown
	}
	if err := ValidateVersionMatch(string(record.VersionMatch)); err != nil {
		return err
	}
	for field, value := range map[string]string{
		"declared_by": record.DeclaredBy, "receipt_note": record.ReceiptNote,
		"verification_note": record.VerificationNote, "edit_note": record.EditNote,
	} {
		if err := ValidateNote(field, value); err != nil {
			return err
		}
	}
	for field, value := range map[string]string{
		"page_url_or_content_id": record.PageURLOrContentID,
		"platform_account":       record.PlatformAccount,
	} {
		if err := ValidateRef(field, value); err != nil {
			return err
		}
	}
	switch record.Status {
	case PublicationReported:
		if record.PageURLOrContentID == "" {
			return invalidField("page_url_or_content_id")
		}
	case PublicationVerified:
		if record.PageURLOrContentID == "" {
			return invalidField("page_url_or_content_id")
		}
		if record.VerificationNote == "" {
			return invalidField("verification_note")
		}
	case PublicationFailed, PublicationRemoved:
		if record.ReceiptNote == "" {
			return invalidField("receipt_note")
		}
	}
	return nil
}

// RequiresApprovedReview reports whether a delivery status may only be reached
// with an approved review behind it.
//
// draft is exempt: scheduling while the review is still pending is an ordinary
// thing to want, and requiring approval to even draft a task would make
// "一边等审一边排期" impossible.
func RequiresApprovedReview(status DeliveryStatus) bool {
	return status != DeliveryDraft
}

// DeliveryTarget is what a task is currently pointed at. It exists so SOP 9.3's
// four triggers are four fields of one comparison instead of four call sites.
type DeliveryTarget struct {
	VersionID   string
	AccountID   string
	Channel     Channel
	Attachments []string
	// DeliveryConfig is SOP 8's "实质性渠道配置".
	DeliveryConfig map[string]string
}

// TargetOf reads the target a snapshot froze.
func TargetOf(snapshot DeliverySnapshot) DeliveryTarget {
	return DeliveryTarget{
		VersionID: snapshot.VersionID, AccountID: snapshot.AccountID,
		Channel: snapshot.Channel, Attachments: snapshot.Attachments,
		DeliveryConfig: snapshot.DeliveryConfig,
	}
}

// ShouldHold reports whether SOP 9.3 puts this task back into held: the
// approved snapshot it references no longer describes what is about to be
// delivered.
//
// The returned reason names which of the four changed, because the transition
// row has to say why and "target changed" would not help anyone reading it a
// month later.
func ShouldHold(approved, current DeliveryTarget) (bool, string) {
	switch {
	case approved.VersionID != current.VersionID:
		return true, "delivery target moved to another version"
	case approved.AccountID != current.AccountID:
		return true, "delivery target account changed"
	case approved.Channel != current.Channel:
		return true, "delivery target channel changed"
	case !sameStrings(approved.Attachments, current.Attachments):
		return true, "delivery target attachments changed"
	case !sameConfig(approved.DeliveryConfig, current.DeliveryConfig):
		return true, "delivery target channel configuration changed"
	}
	return false, ""
}

// HoldableStatuses are the two a task can be held from. A task that has already
// been handed off or cancelled is past the point SOP 9.3 is about.
func HoldableStatus(status DeliveryStatus) bool {
	return status == DeliveryReady || status == DeliveryScheduled
}

func sameStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func sameConfig(left, right map[string]string) bool {
	if len(left) != len(right) {
		return false
	}
	for key, value := range left {
		if right[key] != value {
			return false
		}
	}
	return true
}

// IsDue is SOP 9.1's "到期后产生站内待办", as a comparison.
//
// Nothing schedules anything. A task is due when its planned time has passed
// and it has not been handed over - which is a fact about the moment it is
// read, not an event that happened in the background.
func IsDue(status DeliveryStatus, scheduledAt *time.Time, now time.Time) bool {
	if status != DeliveryScheduled || scheduledAt == nil || scheduledAt.IsZero() {
		return false
	}
	return !scheduledAt.After(now)
}

// IsPendingRegistration is SOP 9.2's "待登记", as a comparison.
//
// It is derived and never stored, because the SOP's own next clause is
// "不推断平台状态": a stored value would be the system asserting it knows what
// the platform did. All this says is "someone handed this over and nobody has
// written down what happened".
func IsPendingRegistration(status DeliveryStatus, publicationRecords int) bool {
	return status == DeliveryHandedOff && publicationRecords == 0
}

// HandoffMeansPublished is false, always, for every handoff method.
//
// It exists as a function so SOP 9.1's "导出成功、复制完成或交接给他人都不自动等于
// 发布成功" has something a test can point at, and so that a future reader who
// wants "handed off" to count as published has to change a function that says
// why it does not.
func HandoffMeansPublished(HandoffMethod) bool { return false }
