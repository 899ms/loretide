package topicplanning

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

// specs/033 PR 2, T031 and T033: the read-time rules of a candidate, without
// a database.

func window(id string, status NodeStatus, starts, ends string, leadDays *int, accounts ...string) NodeWindow {
	return NodeWindow{NodeID: id, Name: "节点 " + id, Status: status, StartsOn: starts, EndsOn: ends,
		LeadDays: leadDays, AccountIDs: accounts}
}

func collidingIDs(collisions []NodeCollision) []string {
	ids := []string{}
	for _, c := range collisions {
		ids = append(ids, c.NodeID)
	}
	return ids
}

func TestCollisionsNeedOverlapAndASharedAccount(t *testing.T) {
	// Self: prep 2026-11-01 (lead 10) through 2026-11-11, account a1.
	self := window("self", NodeStatusActive, "2026-11-11", "2026-11-11", lead(10), "a1")
	others := []NodeWindow{
		self, // itself is never a collision
		window("same-account", NodeStatusActive, "2026-11-05", "2026-11-06", lead(0), "a1", "a2"),
		window("other-account", NodeStatusActive, "2026-11-05", "2026-11-06", lead(0), "a2"),
		window("brand-level", NodeStatusActive, "2026-11-08", "2026-11-08", nil),
		window("cancelled", NodeStatusCancelled, "2026-11-05", "2026-11-06", lead(0), "a1"),
		window("unconfirmed", NodeStatusUnconfirmed, "2026-11-05", "2026-11-06", lead(0), "a1"),
		window("later", NodeStatusActive, "2026-11-20", "2026-11-21", lead(0), "a1"),
		// No lead: its window starts on its start day, the 12th, after self ends.
		window("unset-lead", NodeStatusActive, "2026-11-12", "2026-11-13", nil, "a1"),
		// Its end day is self's preparation start: touching endpoints overlap.
		window("touching", NodeStatusActive, "2026-10-30", "2026-11-01", nil, "a1"),
	}
	got := collidingIDs(FindCollisions(self, "a1", others))
	want := []string{"same-account", "brand-level", "touching"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("collisions for a1 = %v, want %v", got, want)
	}

	// With a lead on the unset node its window would reach back into self's.
	others[7].LeadDays = lead(3)
	if got := collidingIDs(FindCollisions(self, "a1", others[7:8])); len(got) != 1 {
		t.Fatalf("lead 3 on the later node = %v, want one collision", got)
	}

	// A brand-level node (self lists no account) collides with every node in
	// its window, whatever accounts they list.
	brandSelf := window("brand-self", NodeStatusActive, "2026-11-11", "2026-11-11", lead(10))
	if got := collidingIDs(FindCollisions(brandSelf, "", others[:4])); !reflect.DeepEqual(got,
		[]string{"self", "same-account", "other-account", "brand-level"}) {
		t.Fatalf("brand-level self collisions = %v", got)
	}
}

func TestCandidateTimingSaysWhetherTheLeadIsShort(t *testing.T) {
	// 2026-11-10T16:30Z is 11-11 00:30 in Shanghai.
	now := time.Date(2026, 11, 10, 16, 30, 0, 0, time.UTC)
	content := NodeContent{Name: "双十二", StartsOn: "2026-11-14", EndsOn: "2026-11-14",
		Timezone: "Asia/Shanghai", LeadDays: lead(14)}
	timing, err := ComputeCandidateTiming(now, content)
	if err != nil {
		t.Fatal(err)
	}
	if timing.DaysUntilStart != 3 || timing.LeadShort == nil || !*timing.LeadShort ||
		timing.LeadDays == nil || *timing.LeadDays != 14 || timing.Today != "2026-11-11" {
		t.Fatalf("3 days left of 14 = %+v", timing)
	}

	content.LeadDays = lead(2)
	if timing, _ = ComputeCandidateTiming(now, content); timing.LeadShort == nil || *timing.LeadShort {
		t.Fatalf("3 days left of 2 = %+v, want not short", timing)
	}

	// Not set is not an answer: null, never false.
	content.LeadDays = nil
	if timing, _ = ComputeCandidateTiming(now, content); timing.LeadShort != nil || timing.LeadDays != nil {
		t.Fatalf("unset lead = %+v, want lead_short null", timing)
	}

	// A node that has started is not "short" any more.
	content.LeadDays = lead(14)
	content.StartsOn, content.EndsOn = "2026-11-11", "2026-11-12"
	if timing, _ = ComputeCandidateTiming(now, content); timing.LeadShort == nil || *timing.LeadShort ||
		timing.DaysUntilStart != 0 || timing.Phase != PhaseLive {
		t.Fatalf("live node = %+v", timing)
	}
}

func TestNameMatchPatternEscapesAndSkipsShortNames(t *testing.T) {
	for _, name := range []string{"", " ", "6", " 双 "} {
		if _, ok := NameMatchPattern(name); ok {
			t.Errorf("name %q is matched; fewer than 2 characters must not be", name)
		}
	}
	for name, want := range map[string]string{
		" 双十一 ":  "%双十一%",
		"100%好物": `%100\%好物%`,
		"a_b":    `%a\_b%`,
		`C:\节日`:  `%C:\\节日%`,
	} {
		got, ok := NameMatchPattern(name)
		if !ok || got != want {
			t.Errorf("NameMatchPattern(%q) = %q, %v; want %q", name, got, ok, want)
		}
	}
}

func TestDuplicateRiskScopeIsTheAccountOrTheWholeBrand(t *testing.T) {
	accountQuery, accountArgs := duplicateRiskQuery("ws", "a1", "%双十一%")
	brandQuery, brandArgs := duplicateRiskQuery("ws", "", "%双十一%")
	for name, query := range map[string]string{"account": accountQuery, "brand": brandQuery} {
		if !strings.Contains(query, "status <> 'dropped'") || !strings.Contains(query, "workspace_id = $1") {
			t.Errorf("%s query does not exclude dropped cards within the brand:\n%s", name, query)
		}
		for _, column := range []string{"audience_problem_judgment", "ip_fit", "timing",
			"existing_content_relation", "evidence_gaps_and_investment"} {
			if !strings.Contains(query, column+" ILIKE $2 ESCAPE") {
				t.Errorf("%s query does not match %s", name, column)
			}
		}
	}
	if !strings.Contains(accountQuery, "account_id = $3") || !reflect.DeepEqual(accountArgs, []any{"ws", "%双十一%", "a1"}) {
		t.Errorf("account query = %s %v", accountQuery, accountArgs)
	}
	if strings.Contains(brandQuery, "account_id") || len(brandArgs) != 2 {
		t.Errorf("brand-level query is narrowed to an account: %s %v", brandQuery, brandArgs)
	}
}

func TestAdoptionTimingIsTheNodeFieldsInAFixedTemplate(t *testing.T) {
	content := NodeContent{Name: "双十一", StartsOn: "2026-11-11", EndsOn: "2026-11-12",
		Timezone: "Asia/Shanghai", LeadDays: lead(14)}
	if got := AdoptionTiming(content); got != "双十一｜2026-11-11–2026-11-12（Asia/Shanghai）｜准备期自 2026-10-28" {
		t.Errorf("with lead = %q", got)
	}
	content.LeadDays = nil
	if got := AdoptionTiming(content); got != "双十一｜2026-11-11–2026-11-12（Asia/Shanghai）｜准备期未设置" {
		t.Errorf("unset lead = %q", got)
	}
}

func TestCandidateRequestsAreDecodedStrictly(t *testing.T) {
	fieldOf := func(err error) string {
		var fieldErr FieldError
		if errors.As(err, &fieldErr) {
			return fieldErr.Field
		}
		return ""
	}
	for body, field := range map[string]string{
		`{}`:                          "",
		`{"angle":null}`:              "",
		`{"angle":"x","extra":1}`:     "",
		`{"angle":"x"} {"angle":"y"}`: "",
		`{"status":"adopted"}`:        "status",
		`{"angle":"` + strings.Repeat("角", MaxCandidateAngleLength+1) + `"}`: "angle",
	} {
		_, err := DecodeCandidatePatch([]byte(body))
		if !errors.Is(err, ErrInvalid) || fieldOf(err) != field {
			t.Errorf("patch %.40q = %v, want invalid on %q", body, err, field)
		}
	}
	patch, err := DecodeCandidatePatch([]byte(`{"angle":""}`))
	if err != nil || !patch.Angle.Set || patch.Angle.Value != "" || patch.Status.Set {
		t.Fatalf("explicit empty angle = %+v, %v; want set to empty and status untouched", patch, err)
	}

	for body, field := range map[string]string{
		`{"mode":"clone"}`:                      "mode",
		`{"mode":"link"}`:                       "topic_card_id",
		`{"mode":"create","topic_card_id":"t"}`: "topic_card_id",
		`{"mode":"create","x":1}`:               "",
	} {
		_, err := DecodeAdopt([]byte(body))
		if !errors.Is(err, ErrInvalid) || fieldOf(err) != field {
			t.Errorf("adopt %s = %v, want invalid on %q", body, err, field)
		}
	}
	if req, err := DecodeAdopt([]byte(`{"mode":"link","topic_card_id":" t1 "}`)); err != nil || req.TopicCardID != "t1" {
		t.Fatalf("link = %+v, %v", req, err)
	}

	for body, field := range map[string]string{
		`{"decision":"ignored"}`: "decision",
		`{"decision":"kept","note":"` + strings.Repeat("注", MaxImpactNoteLength+1) + `"}`: "note",
		`{"decision":"kept","by":"x"}`: "",
	} {
		_, err := DecodeImpactDecision([]byte(body))
		if !errors.Is(err, ErrInvalid) || fieldOf(err) != field {
			t.Errorf("impact decision %.40q = %v, want invalid on %q", body, err, field)
		}
	}
}

// T033 / FR-045: the two material ports answer "does it exist in this brand"
// and nothing else. Their methods take a context, a workspace id and a source
// id - no account. A material has no account owner today; the day it gets
// one, the account must be passed and the answer must go through
// workspace-core.CanRead, and this test is the reminder.
func TestSourcePortsTakeNoAccount(t *testing.T) {
	contextType := reflect.TypeFor[context.Context]()
	stringType := reflect.TypeFor[string]()
	for _, port := range []reflect.Type{reflect.TypeFor[SourceReader](), reflect.TypeFor[SourceStatusReader]()} {
		if port.NumMethod() == 0 {
			t.Fatalf("%s has no methods; the check below would pass on nothing", port.Name())
		}
		for i := range port.NumMethod() {
			method := port.Method(i)
			in := method.Type
			if in.NumIn() != 3 || in.In(0) != contextType || in.In(1) != stringType || in.In(2) != stringType {
				t.Errorf("%s.%s is %s: a port that takes an account (or anything beyond workspace and source "+
					"ids) must, in the same PR, route the answer through workspace-core.CanRead (specs/033 FR-045)",
					port.Name(), method.Name, in)
			}
		}
	}
}
