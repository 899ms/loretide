package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"math"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
)

func makePropertyDef(propType string, options []PropertyOption) db.IssueProperty {
	cfg, _ := json.Marshal(PropertyConfig{Options: options})
	return db.IssueProperty{Type: propType, Config: cfg}
}

// withIssuePropertyParams sets both chi URL params in one route context —
// withURLParam builds a fresh context per call, so chaining it would drop
// the first param.
func withIssuePropertyParams(req *http.Request, issueID, propertyID string) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", issueID)
	rctx.URLParams.Add("propertyId", propertyID)
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
}

func TestValidatePropertyValueUnit(t *testing.T) {
	textDef := makePropertyDef("text", nil)
	if _, err := validatePropertyValue(textDef, json.RawMessage(`"  "`)); err == nil {
		t.Fatalf("blank text accepted")
	}
	if _, err := validatePropertyValue(textDef, json.RawMessage(`"`+strings.Repeat("x", 2001)+`"`)); err == nil {
		t.Fatalf("overlong text accepted")
	}
	if _, err := validatePropertyValue(textDef, json.RawMessage(`null`)); err == nil {
		t.Fatalf("null accepted")
	}
	boolDef := makePropertyDef("checkbox", nil)
	if _, err := validatePropertyValue(boolDef, json.RawMessage(`"true"`)); err == nil {
		t.Fatalf("string into checkbox accepted")
	}
	if _, err := validatePropertyValue(boolDef, json.RawMessage(`false`)); err != nil {
		t.Fatalf("false rejected: %v", err)
	}
}

func TestValidatePropertyNameReserved(t *testing.T) {
	for _, name := range []string{"status", "Priority", "due date", "Due_Date", "START DATE", "labels"} {
		if _, err := validatePropertyName(name); err == nil {
			t.Fatalf("reserved name %q accepted", name)
		}
	}
	if _, err := validatePropertyName("Severity"); err != nil {
		t.Fatalf("legit name rejected: %v", err)
	}
}

func TestParsePropertiesFilterNoValueUnit(t *testing.T) {
	defID := uuid.NewString()
	w := httptest.NewRecorder()
	var args []any
	addArg := func(v any) string {
		args = append(args, v)
		return fmt.Sprintf("$%d", len(args))
	}

	// "__none__" compiles to the marker object, and the predicate turns that
	// marker into a key-absence check rather than a containment pattern.
	groups, ok := parsePropertiesFilterParam(w, fmt.Sprintf(`{"%s":["__none__"]}`, defID))
	if !ok {
		t.Fatalf("no-value parse failed: %s", w.Body.String())
	}
	if len(groups) != 1 || len(groups[0]) != 1 {
		t.Fatalf("expected one group with one alternative, got %d / %d", len(groups), len(groups[0]))
	}
	if parsed, isMarker := parseNoPropertyValuePattern(groups[0][0]); !isMarker || parsed != defID {
		t.Fatalf("expected no-value marker for %s, got %q isMarker=%v", defID, parsed, isMarker)
	}
	sql := propertiesFilterPredicate(groups, addArg)
	if !strings.Contains(sql, "NOT (i.properties ? $1)") {
		t.Fatalf("no-value predicate wrong: %s", sql)
	}

	// A normal containment alternative is never mistaken for the marker.
	groups, ok = parsePropertiesFilterParam(w, fmt.Sprintf(`{"%s":["true"]}`, defID))
	if !ok {
		t.Fatalf("containment parse failed: %s", w.Body.String())
	}
	if _, isMarker := parseNoPropertyValuePattern(groups[0][0]); isMarker {
		t.Fatalf("checkbox true alternative misdetected as the marker")
	}

	// Mixed true + no value: containment OR key-absence.
	groups, ok = parsePropertiesFilterParam(w, fmt.Sprintf(`{"%s":["true","__none__"]}`, defID))
	if !ok {
		t.Fatalf("mixed parse failed: %s", w.Body.String())
	}
	args = nil
	sql = propertiesFilterPredicate(groups, addArg)
	if !strings.Contains(sql, "@> $1") || !strings.Contains(sql, "NOT (i.properties ? ") {
		t.Fatalf("mixed predicate wrong: %s", sql)
	}

	// Duplicate sentinels collapse to a single marker.
	groups, ok = parsePropertiesFilterParam(w, fmt.Sprintf(`{"%s":["__none__","__none__"]}`, defID))
	if !ok {
		t.Fatalf("duplicate parse failed: %s", w.Body.String())
	}
	if len(groups[0]) != 1 {
		t.Fatalf("duplicate sentinel produced %d alternatives, want 1", len(groups[0]))
	}

	// A numeric filter value also emits the stored jsonb number form, so a
	// number property matches the scalar instead of only the "3.5" string.
	groups, ok = parsePropertiesFilterParam(w, fmt.Sprintf(`{"%s":["3.5"]}`, defID))
	if !ok {
		t.Fatalf("numeric parse failed: %s", w.Body.String())
	}
	hasNumber := false
	for _, alt := range groups[0] {
		var m map[string]json.RawMessage
		if err := json.Unmarshal(alt, &m); err != nil {
			continue
		}
		var num float64
		if err := json.Unmarshal(m[defID], &num); err == nil && num == 3.5 {
			hasNumber = true
		}
	}
	if !hasNumber {
		t.Fatalf("numeric filter value did not emit a jsonb number containment form: %v", groups[0])
	}
	// A date value must NOT be misread as a number.
	groups, ok = parsePropertiesFilterParam(w, fmt.Sprintf(`{"%s":["2026-08-19"]}`, defID))
	if !ok {
		t.Fatalf("date parse failed: %s", w.Body.String())
	}
	for _, alt := range groups[0] {
		var m map[string]json.RawMessage
		if err := json.Unmarshal(alt, &m); err != nil {
			continue
		}
		var num float64
		if err := json.Unmarshal(m[defID], &num); err == nil {
			t.Fatalf("date value misread as a number: %v", alt)
		}
	}

	// NaN / Infinity parse as floats but are not valid JSON — they must be
	// skipped, not marshaled into a 400.
	for _, bad := range []string{"NaN", "Infinity", "-Infinity"} {
		groups, ok = parsePropertiesFilterParam(w, fmt.Sprintf(`{"%s":[%q]}`, defID, bad))
		if !ok {
			t.Fatalf("non-finite parse of %q failed: %s", bad, w.Body.String())
		}
		for _, alt := range groups[0] {
			var m map[string]json.RawMessage
			if err := json.Unmarshal(alt, &m); err != nil {
				continue
			}
			var num float64
			if err := json.Unmarshal(m[defID], &num); err == nil && (math.IsNaN(num) || math.IsInf(num, 0)) {
				t.Fatalf("non-finite value %q leaked a number containment form: %v", bad, alt)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Actor property types (MUL-6286)
// ---------------------------------------------------------------------------

// decodePropertiesBag reads the `{"properties": {...}}` envelope the value
// endpoints return. A fresh struct per call matters: json.Decode merges into a
// pre-populated map, so reusing one would keep keys from an earlier response.
func decodePropertiesBag(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var resp struct {
		Properties map[string]any `json:"properties"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode properties bag: %v", err)
	}
	return resp.Properties
}

func TestParseActorRefUnit(t *testing.T) {
	memberID := uuid.NewString()
	ref, err := parseActorRef("member:" + memberID)
	if err != nil {
		t.Fatalf("member reference rejected: %v", err)
	}
	if ref.Kind != "member" || ref.ID != memberID {
		t.Fatalf("unexpected parse: %+v", ref)
	}
	// String() is the canonical storage form, so it must round-trip exactly.
	if ref.String() != "member:"+memberID {
		t.Fatalf("String() round-trip broken: %q", ref.String())
	}

	// uuid.Parse also accepts uppercase, braces and the urn: form. Every
	// consumer compares reference strings exactly, so anything not stored in
	// canonical form would render as Unknown and never match a filter.
	canonical := "6ba7b810-9dad-11d1-80b4-00c04fd430c8"
	for _, variant := range []string{
		strings.ToUpper(canonical),
		"{" + canonical + "}",
		"urn:uuid:" + canonical,
		strings.ReplaceAll(canonical, "-", ""),
	} {
		got, err := parseActorRef("member:" + variant)
		if err != nil {
			t.Fatalf("uuid variant %q rejected: %v", variant, err)
		}
		if got.String() != "member:"+canonical {
			t.Fatalf("uuid variant %q not canonicalized: %q", variant, got.String())
		}
	}

	cases := []struct {
		name  string
		value string
		want  string
	}{
		{"no colon", uuid.NewString(), `"<kind>:<uuid>"`},
		{"empty string", "", `"<kind>:<uuid>"`},
		// "agent" and "squad" are assignee kinds deliberately left out of the
		// V1 value range; both must read as unknown, not silently accepted.
		{"agent kind", "agent:" + uuid.NewString(), "unknown actor kind"},
		{"squad kind", "squad:" + uuid.NewString(), "unknown actor kind"},
		{"user kind", "user:" + uuid.NewString(), "unknown actor kind"},
		{"empty kind", ":" + uuid.NewString(), "unknown actor kind"},
		{"kind is case-sensitive", "Member:" + uuid.NewString(), "unknown actor kind"},
		{"non-uuid id", "member:not-a-uuid", "must be a UUID"},
		{"empty id", "member:", "must be a UUID"},
		{"nested kind", "member:agent:" + uuid.NewString(), "must be a UUID"},
	}
	for _, tc := range cases {
		_, err := parseActorRef(tc.value)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("%s: expected error containing %q, got %v", tc.name, tc.want, err)
		}
	}
}

// TestParseActorRefListUnit pins the multi_actor list contract, including the
// one place it deliberately differs from multi_select: the caller's order
// survives instead of being canonicalized.
func TestParseActorRefListUnit(t *testing.T) {
	// Fixed ids so the insertion order below is neither ascending nor
	// descending — any sort applied to the result would reorder it.
	first := "member:11111111-1111-4111-8111-111111111111"
	second := "member:22222222-2222-4222-8222-222222222222"
	third := "member:00000000-0000-4000-8000-000000000000"

	if _, err := parseActorRefList(nil); err == nil {
		t.Fatalf("nil list accepted")
	}
	if _, err := parseActorRefList([]any{}); err == nil {
		t.Fatalf("empty array accepted")
	}

	refs, err := parseActorRefList([]any{first, second, third, second})
	if err != nil {
		t.Fatalf("valid list rejected: %v", err)
	}
	got := make([]string, len(refs))
	for i, ref := range refs {
		got[i] = ref.String()
	}
	if len(got) != 3 {
		t.Fatalf("duplicate not dropped: %v", got)
	}
	if got[0] != first || got[1] != second || got[2] != third {
		t.Fatalf("insertion order not preserved: %v", got)
	}
	// Explicit: unlike multi_select there is no canonical order to sort to.
	if sort.StringsAreSorted(got) {
		t.Fatalf("list was canonicalized to sorted order: %v", got)
	}

	over := make([]any, 0, maxPropertyActorValues+1)
	for i := 0; i < maxPropertyActorValues+1; i++ {
		over = append(over, "member:"+uuid.NewString())
	}
	_, err = parseActorRefList(over)
	if err == nil || !strings.Contains(err.Error(), fmt.Sprintf("more than %d", maxPropertyActorValues)) {
		t.Fatalf("over-cap list: expected cap error, got %v", err)
	}
	if refs, err := parseActorRefList(over[:maxPropertyActorValues]); err != nil || len(refs) != maxPropertyActorValues {
		t.Fatalf("list at the cap rejected: %d refs, %v", len(refs), err)
	}

	if _, err := parseActorRefList([]any{first, 42}); err == nil {
		t.Fatalf("non-string element accepted")
	}
	if _, err := parseActorRefList([]any{first, "squad:" + uuid.NewString()}); err == nil {
		t.Fatalf("unknown kind inside a list accepted")
	}
}

func TestValidatePropertyValueActorUnit(t *testing.T) {
	actorDef := makePropertyDef("actor", nil)
	multiDef := makePropertyDef("multi_actor", nil)
	// Fixed ids: memberRef sorts AFTER secondRef, so the insertion order
	// asserted below is proof that no canonicalizing sort ran.
	memberRef := "member:99999999-9999-4999-8999-999999999999"
	secondRef := "member:00000000-0000-4000-8000-000000000000"

	stored, err := validatePropertyValue(actorDef, json.RawMessage(`"`+memberRef+`"`))
	if err != nil {
		t.Fatalf("actor value rejected: %v", err)
	}
	if string(stored) != `"`+memberRef+`"` {
		t.Fatalf("actor value not stored as a plain string: %s", stored)
	}

	// actor is a single reference: no array, no number, no object, no bare id.
	for _, raw := range []string{
		`["` + memberRef + `"]`,
		`3`,
		`true`,
		`{"kind":"member","id":"` + uuid.NewString() + `"}`,
		`null`,
		`"` + uuid.NewString() + `"`,
		`"agent:` + uuid.NewString() + `"`,
		`"squad:` + uuid.NewString() + `"`,
	} {
		if _, err := validatePropertyValue(actorDef, json.RawMessage(raw)); err == nil {
			t.Fatalf("actor accepted %s", raw)
		}
	}

	// multi_actor is always an array, even for a single reference.
	for _, raw := range []string{
		`"` + memberRef + `"`,
		`[]`,
		`3`,
		`[3]`,
		`null`,
	} {
		if _, err := validatePropertyValue(multiDef, json.RawMessage(raw)); err == nil {
			t.Fatalf("multi_actor accepted %s", raw)
		}
	}

	// Duplicates collapse and the caller's order survives.
	stored, err = validatePropertyValue(multiDef, json.RawMessage(
		`["`+memberRef+`","`+secondRef+`","`+memberRef+`"]`))
	if err != nil {
		t.Fatalf("multi_actor value rejected: %v", err)
	}
	if string(stored) != `["`+memberRef+`","`+secondRef+`"]` {
		t.Fatalf("multi_actor not deduped in insertion order: %s", stored)
	}
}

func TestParsePropertiesFilterOperatorUnit(t *testing.T) {
	defID := uuid.NewString()
	w := httptest.NewRecorder()
	var args []any
	addArg := func(v any) string {
		args = append(args, v)
		return fmt.Sprintf("$%d", len(args))
	}

	// A contains member compiles to the operator pattern (with the LIKE
	// wildcards escaped) and renders as an ILIKE predicate, never as a
	// containment pattern or the no-value marker.
	groups, ok := parsePropertiesFilterParam(w, fmt.Sprintf(`{"%s":[{"op":"contains","value":"50%% off"}]}`, defID))
	if !ok {
		t.Fatalf("contains parse failed: %s", w.Body.String())
	}
	if len(groups) != 1 || len(groups[0]) != 1 {
		t.Fatalf("expected one group with one alternative, got %d / %d", len(groups), len(groups[0]))
	}
	if _, isMarker := parseNoPropertyValuePattern(groups[0][0]); isMarker {
		t.Fatalf("operator alternative misdetected as the no-value marker")
	}
	pattern, isOp := parseOperatorPattern(groups[0][0])
	if !isOp || pattern.Op != "contains" || pattern.Def != defID {
		t.Fatalf("expected contains operator pattern, got %+v isOp=%v", pattern, isOp)
	}
	if pattern.Value != `50\% off` {
		t.Fatalf("contains value not LIKE-escaped: %q", pattern.Value)
	}
	args = nil
	sql := propertiesFilterPredicate(groups, addArg)
	if !strings.Contains(sql, "jsonb_typeof") || !strings.Contains(sql, "= 'string'") ||
		!strings.Contains(sql, "ILIKE") || strings.Contains(sql, "@>") {
		t.Fatalf("contains predicate wrong: %s", sql)
	}

	// Numeric ops validate their value and render the typed comparison.
	groups, ok = parsePropertiesFilterParam(w, fmt.Sprintf(`{"%s":[{"op":"gte","value":"3.5"}]}`, defID))
	if !ok {
		t.Fatalf("gte parse failed: %s", w.Body.String())
	}
	args = nil
	sql = propertiesFilterPredicate(groups, addArg)
	if !strings.Contains(sql, "CASE WHEN") || !strings.Contains(sql, "::numeric END >= $") ||
		!strings.Contains(sql, "::numeric)") {
		t.Fatalf("gte predicate wrong: %s", sql)
	}
	// The canonical decimal stays a string bind and is explicitly cast to
	// numeric, matching the static open_only path without float8 demotion.
	last := args[len(args)-1]
	if value, isString := last.(string); !isString || value != "3.5" {
		t.Fatalf("gte bind arg must be canonical string 3.5, got %T %v", last, last)
	}

	// ParseFloat accepts forms Postgres ::numeric rejects (hex-float,
	// underscores); the compiled pattern must store the canonical plain
	// decimal or the static open_only unroll would 500 on the cast.
	groups, ok = parsePropertiesFilterParam(w, fmt.Sprintf(`{"%s":[{"op":"gt","value":"0x1p4"}]}`, defID))
	if !ok {
		t.Fatalf("hex-float parse failed: %s", w.Body.String())
	}
	pattern, isOp = parseOperatorPattern(groups[0][0])
	if !isOp || pattern.Value != "16" {
		t.Fatalf("hex-float bound not canonicalized to 16: %+v isOp=%v", pattern, isOp)
	}

	// Date ops render the lexicographic string comparison.
	groups, ok = parsePropertiesFilterParam(w, fmt.Sprintf(`{"%s":[{"op":"before","value":"2026-02-01"}]}`, defID))
	if !ok {
		t.Fatalf("before parse failed: %s", w.Body.String())
	}
	args = nil
	sql = propertiesFilterPredicate(groups, addArg)
	if !strings.Contains(sql, "= 'string' AND") || !strings.Contains(sql, "< $") {
		t.Fatalf("before predicate wrong: %s", sql)
	}

	// An operator composes OR-style with legacy equality members and the
	// no-value sentinel in one group.
	groups, ok = parsePropertiesFilterParam(w, fmt.Sprintf(`{"%s":["3.5",{"op":"lt","value":"10"},"__none__"]}`, defID))
	if !ok {
		t.Fatalf("mixed parse failed: %s", w.Body.String())
	}
	// "3.5" expands to 3 containment forms, the operator is 1, the marker 1.
	if len(groups[0]) != 5 {
		t.Fatalf("expected 5 alternatives, got %d", len(groups[0]))
	}

	// Invalid members are rejected with a 400.
	for name, raw := range map[string]string{
		"unknown op":    `{"op":"regex","value":"x"}`,
		"empty value":   `{"op":"contains","value":""}`,
		"non-numeric":   `{"op":"gt","value":"abc"}`,
		"NaN":           `{"op":"lt","value":"NaN"}`,
		"bad date":      `{"op":"before","value":"02/01/2026"}`,
		"missing value": `{"op":"contains"}`,
	} {
		w = httptest.NewRecorder()
		_, ok := parsePropertiesFilterParam(w, fmt.Sprintf(`{"%s":[%s]}`, defID, raw))
		if ok {
			t.Fatalf("%s: expected rejection", name)
		}
		if w.Code != http.StatusBadRequest {
			t.Fatalf("%s: expected 400, got %d", name, w.Code)
		}
	}
}

// TestPropertyContainsPrefilterCompilationUnit pins which contains needles get
// the bigram prefilter that migration 446's index serves. The prefilter matches
// against the jsonb text form of the whole properties object, so a needle that
// jsonb escapes on serialization (", \, control characters) must compile
// without it — the alternative would silently drop matching rows.
func TestPropertyContainsPrefilterCompilationUnit(t *testing.T) {
	defID := uuid.NewString()
	compile := func(t *testing.T, needle string) (propertyOperatorPattern, json.RawMessage, string) {
		t.Helper()
		raw, err := json.Marshal(map[string]any{defID: []any{map[string]any{"op": "contains", "value": needle}}})
		if err != nil {
			t.Fatalf("marshal filter: %v", err)
		}
		w := httptest.NewRecorder()
		groups, ok := parsePropertiesFilterParam(w, string(raw))
		if !ok {
			t.Fatalf("parse %q: %s", needle, w.Body.String())
		}
		pattern, isOp := parseOperatorPattern(groups[0][0])
		if !isOp {
			t.Fatalf("%q did not compile to an operator pattern", needle)
		}
		var args []any
		addArg := func(v any) string {
			args = append(args, v)
			return fmt.Sprintf("$%d", len(args))
		}
		return pattern, groups[0][0], propertiesFilterPredicate(groups, addArg)
	}

	for _, tc := range []struct {
		name          string
		needle        string
		wantPrefilter string
	}{
		{"plain ascii", "world", "world"},
		// LIKE wildcards are escaped identically on both sides, so they stay
		// prefilterable — the escape is a backslash in the pattern, not in the
		// serialized value.
		{"like wildcards", "50% off_now", `50\% off\_now`},
		{"cjk", "中文属性", "中文属性"},
		// Short needles are prefiltered too: pg_bigm indexes 1- and
		// 2-character keywords, which is the capability it exists for over
		// pg_trgm, and one CJK character is already a word.
		{"single character", "x", "x"},
		{"single cjk character", "文", "文"},
		{"double quote", `say "hi"`, ""},
		{"backslash", `C:\logs`, ""},
		{"tab", "col\tvalue", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pattern, alt, sql := compile(t, tc.needle)
			if pattern.Prefilter != tc.wantPrefilter {
				t.Fatalf("prefilter for %q: got %q, want %q", tc.needle, pattern.Prefilter, tc.wantPrefilter)
			}
			// The static open_only unroll keys off the presence of the JSON
			// member, so an unsafe needle must omit it rather than carry "".
			var members map[string]json.RawMessage
			if err := json.Unmarshal(alt, &members); err != nil {
				t.Fatalf("decode alternative: %v", err)
			}
			_, hasMember := members["prefilter"]
			if hasMember != (tc.wantPrefilter != "") {
				t.Fatalf("alternative %s: prefilter member present=%v, want %v", alt, hasMember, tc.wantPrefilter != "")
			}
			hasClause := strings.Contains(sql, "LOWER(i.properties::text) LIKE LOWER(")
			if hasClause != (tc.wantPrefilter != "") {
				t.Fatalf("predicate %s: prefilter clause present=%v, want %v", sql, hasClause, tc.wantPrefilter != "")
			}
			// The per-key check is authoritative on every path, prefiltered or
			// not.
			if !strings.Contains(sql, "ILIKE") || !strings.Contains(sql, "jsonb_typeof") {
				t.Fatalf("predicate lost its per-key contains check: %s", sql)
			}
		})
	}
}
