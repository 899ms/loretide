package workspacecore

import "time"

// Is it time to go and look at the numbers for this piece?
//
// SOP 3.2 names 反馈观察时点 but says nothing about what happens when it cannot
// be answered, so this file is mostly about that case. 027 refused to invent a
// default number of days and left the question unanswerable on purpose; this
// card supplies the setting, and the three-valued answer below is what replaces
// the refusal.
//
// Contract: specs/029-operating-rules/contracts/operating-rules.md section 3

// Due is the whole answer. THREE values, not a bool.
//
// The same shape 025 used for version_match{matched,differs,unknown} and 027
// used for a metric value that is null rather than zero. It keeps showing up
// because the alternative keeps being wrong in the same way: collapsing "no
// answer" into one of the two real answers manufactures a fact.
type Due string

const (
	// DuePassed - the window has elapsed. Go and copy the numbers down.
	DuePassed Due = "passed"
	// DueNotYet - published, but it is too early to expect numbers.
	DueNotYet Due = "not_yet"
	// DueUnknown - there is no way to tell. Either the brand has set no
	// window, or the publication record carries no publication time (025
	// allows that: somebody writing down a piece that went out last month may
	// not know the hour).
	//
	// DueUnknown is NOT a quieter DueNotYet. A caller that treats it as one
	// makes every record without a publication time disappear from the
	// workbench - and those are precisely the ones somebody should look at.
	DueUnknown Due = "unknown"
)

// ObservationDue answers for one publication record.
//
// publishedAt is a pointer because 025's column is nullable. hasDays is
// separate from days for the reason the whole of this card is: 0 is a real
// window ("look the same day") and absent is not a window at all.
//
// There is no default number of days anywhere in this function, and a guard
// test scans the package to keep it that way. A hard-coded 14 here would be a
// rule the SOP never stated, sitting where no operator can see or change it -
// which is the exact sentence 027 wrote when it declined to write one.
func ObservationDue(publishedAt *time.Time, days int64, hasDays bool, now time.Time) Due {
	if !hasDays {
		return DueUnknown
	}
	if publishedAt == nil {
		return DueUnknown
	}
	if !publishedAt.Add(time.Duration(days) * 24 * time.Hour).After(now) {
		return DuePassed
	}
	return DueNotYet
}

// ObservationDueFor is the whole question in one call: given a brand's rules
// and a channel, is this record's window up?
//
// It exists so callers outside this package - feedback-learning's pending
// derivation, when it lands in PR 3 - ask one thing instead of reassembling
// the window and the comparison themselves. Two call sites reassembling it is
// two definitions of due.
func ObservationDueFor(rules Rules, platform string, publishedAt *time.Time, now time.Time) Due {
	days, source := ReadObservation(rules, platform)
	return ObservationDue(publishedAt, days, source != ObservationUnset, now)
}
