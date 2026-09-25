package topicplanning

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"testing"

	ipprofile "github.com/multica-ai/multica/server/internal/content/ip-profile"
)

// specs/036 PR 1, no database: the controlled sets, strict decoding, the
// field rules and the read-time "unknown" (T008 to T011; FR-002, FR-011 to
// FR-014, FR-017, FR-080; SC-002).

func validTheme() ThemeContent {
	return ThemeContent{
		Name: "羊绒大衣怎么洗", Platform: "xiaohongshu", BusinessGoal: "让想买羊绒大衣的人知道能到店护理",
		Questions: []string{"羊绒大衣能机洗吗"}, Keywords: []string{"羊绒", "大衣清洗"},
		Intent: IntentSolve, Origin: OriginManualKeyword,
	}
}

func wantSearchField(t *testing.T, err error, field string) {
	t.Helper()
	fieldErr, ok := errors.AsType[FieldError](err)
	if !ok || fieldErr.Field != field {
		t.Fatalf("err = %v, want a FieldError naming %q", err, field)
	}
	if strings.ContainsFunc(fieldErr.Field, func(r rune) bool { return r >= 'A' && r <= 'Z' }) {
		t.Fatalf("a Go name reached the field: %q", fieldErr.Field)
	}
}

func themeJSON(t *testing.T, extra map[string]any) []byte {
	t.Helper()
	body := map[string]any{}
	raw, _ := json.Marshal(validTheme())
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatal(err)
	}
	for key, value := range extra {
		body[key] = value
	}
	out, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// T008: a value outside each set is refused naming its field; the platform
// set is ip-profile's own, read from the same variable.
func TestSearchThemeControlledSetsRefuseOutsiders(t *testing.T) {
	for _, tc := range []struct {
		field string
		edit  func(*ThemeContent)
	}{
		{"platform", func(c *ThemeContent) { c.Platform = "taobao" }},
		{"platform", func(c *ThemeContent) { c.Platform = "Xiaohongshu" }},
		{"intent", func(c *ThemeContent) { c.Intent = "navigational" }},
		{"intent", func(c *ThemeContent) { c.Intent = "" }},
		{"origin", func(c *ThemeContent) { c.Origin = "online_research" }},
		{"origin", func(c *ThemeContent) { c.Origin = "" }},
	} {
		content := validTheme()
		tc.edit(&content)
		_, err := NormalizeThemeContent(content)
		wantSearchField(t, err, tc.field)
	}
	for _, platform := range ipprofile.Platforms {
		content := validTheme()
		content.Platform = string(platform)
		if _, err := NormalizeThemeContent(content); err != nil {
			t.Errorf("platform %s refused: %v", platform, err)
		}
	}
	for _, intent := range SearchIntents {
		content := validTheme()
		content.Intent = intent
		if _, err := NormalizeThemeContent(content); err != nil {
			t.Errorf("intent %s refused: %v", intent, err)
		}
	}
}

// The migration's CHECK lists repeat the Go sets as a backstop; they must be
// the same sets, read from the migration rather than restated here.
func TestSearchThemeMigrationChecksMatchTheGoSets(t *testing.T) {
	_, current, _, _ := runtime.Caller(0)
	matches, err := filepath.Glob(filepath.Join(filepath.Dir(current), "..", "..", "..", "migrations", "*_content_search_theme_revision.up.sql"))
	if err != nil || len(matches) != 1 {
		t.Fatalf("theme table migration: %v %v", matches, err)
	}
	raw, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatal(err)
	}
	sql := string(raw)
	checked := func(column string) []string {
		found := regexp.MustCompile(`(?s)\b` + column + `\s+text NOT NULL CHECK \(` + column + ` IN \((.*?)\)\)`).FindStringSubmatch(sql)
		if found == nil {
			t.Fatalf("no CHECK on %s", column)
		}
		values := []string{}
		for _, quoted := range regexp.MustCompile(`'([^']+)'`).FindAllStringSubmatch(found[1], -1) {
			values = append(values, quoted[1])
		}
		return values
	}
	strs := func(values any) []string {
		out := []string{}
		list := reflect.ValueOf(values)
		for i := range list.Len() {
			out = append(out, list.Index(i).String())
		}
		return out
	}
	for column, want := range map[string][]string{
		"platform": strs(ipprofile.Platforms), "intent": strs(SearchIntents), "origin": strs(ThemeOrigins),
	} {
		if got := checked(column); !slices.Equal(got, want) {
			t.Errorf("%s CHECK = %v, Go set = %v", column, got, want)
		}
	}
	lower := strings.ToLower(sql)
	for _, column := range []string{"search_volume", "competition", "rank", "score"} {
		if regexp.MustCompile(`(?m)^\s+` + column + `\s`).MatchString(lower) {
			t.Errorf("the theme table has a %s column (FR-014)", column)
		}
	}
}

// T009 / FR-002 / FR-014 / SC-002: every research or search-data field is
// refused by its own name; so is any other unknown member and a wrongly
// typed value, by JSON path. The fields the server writes are accepted and
// dropped.
func TestSearchThemeDecodingRefusesUnknownFieldsByName(t *testing.T) {
	for _, field := range []string{"search_volume", "competition", "rank", "scope", "budget", "online", "research_scope", "score", "theme_idd"} {
		_, err := DecodeThemeCreate(themeJSON(t, map[string]any{field: 1}))
		wantSearchField(t, err, field)
		_, err = DecodeThemeRevision(themeJSON(t, map[string]any{field: 1, "base_revision": 1}))
		wantSearchField(t, err, field)
	}
	for field, value := range map[string]any{"name": 1, "questions": "one", "keywords": []int{1}, "voided": "yes"} {
		_, err := DecodeThemeRevision(themeJSON(t, map[string]any{field: value, "base_revision": 1}))
		wantSearchField(t, err, field)
	}
	_, err := DecodeThemeRevision(themeJSON(t, map[string]any{"base_revision": "1"}))
	wantSearchField(t, err, "base_revision")
	_, err = DecodeThemeRevision(themeJSON(t, nil))
	wantSearchField(t, err, "base_revision")
	_, err = DecodeThemeCreate(themeJSON(t, map[string]any{"voided": true}))
	wantSearchField(t, err, "voided")
	if _, err = DecodeThemeCreate(append(themeJSON(t, nil), []byte(`{}`)...)); !errors.Is(err, ErrInvalid) {
		t.Errorf("a second JSON value = %v, want ErrInvalid", err)
	}

	content, err := DecodeThemeCreate(themeJSON(t, map[string]any{"recorded_by": "someone else", "data_origin": "ai"}))
	if err != nil {
		t.Fatalf("server-written fields refused: %v", err)
	}
	if !reflect.DeepEqual(content, validTheme()) {
		t.Fatalf("decoded %+v, want %+v", content, validTheme())
	}
	req, err := DecodeThemeRevision(themeJSON(t, map[string]any{"base_revision": 2, "voided": true, "recorded_by": "x"}))
	if err != nil || req.BaseRevision != 2 || !req.Voided {
		t.Fatalf("revision = %+v, %v", req, err)
	}
}

// T010 / FR-012 / FR-017: the field rules.
func TestSearchThemeFieldRules(t *testing.T) {
	for _, tc := range []struct {
		name, field string
		edit        func(*ThemeContent)
	}{
		{"no question and no keyword", "questions", func(c *ThemeContent) { c.Questions, c.Keywords = nil, []string{" ", ""} }},
		{"customer question without a note", "origin_note", func(c *ThemeContent) { c.Origin, c.OriginNote = OriginCustomerQuestion, "  " }},
		{"authorized material without a source", "source_ids", func(c *ThemeContent) { c.Origin, c.SourceIDs = OriginAuthorizedMaterial, []string{" "} }},
		{"empty name", "name", func(c *ThemeContent) { c.Name = "  " }},
		{"long name", "name", func(c *ThemeContent) { c.Name = strings.Repeat("羊", MaxThemeNameRunes+1) }},
		{"long goal", "business_goal", func(c *ThemeContent) { c.BusinessGoal = strings.Repeat("目", MaxThemeTextRunes+1) }},
		{"long question", "questions", func(c *ThemeContent) { c.Questions = []string{strings.Repeat("问", MaxThemeQuestionRunes+1)} }},
		{"too many questions", "questions", func(c *ThemeContent) { c.Questions = numbered("问", MaxThemeQuestions+1) }},
		{"long keyword", "keywords", func(c *ThemeContent) { c.Keywords = []string{strings.Repeat("词", MaxThemeKeywordRunes+1)} }},
		{"too many keywords", "keywords", func(c *ThemeContent) { c.Keywords = numbered("词", MaxThemeKeywords+1) }},
		{"long origin note", "origin_note", func(c *ThemeContent) { c.OriginNote = strings.Repeat("注", MaxThemeTextRunes+1) }},
		{"too many sources", "source_ids", func(c *ThemeContent) { c.SourceIDs = numbered("s", MaxThemeReferenceIDs+1) }},
		{"too many cards", "topic_card_ids", func(c *ThemeContent) { c.TopicCardIDs = numbered("c", MaxThemeReferenceIDs+1) }},
		{"too many briefs", "brief_revision_ids", func(c *ThemeContent) { c.BriefRevisionIDs = numbered("b", MaxThemeReferenceIDs+1) }},
		{"long note", "note", func(c *ThemeContent) { c.Note = strings.Repeat("注", MaxThemeTextRunes+1) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			content := validTheme()
			tc.edit(&content)
			_, err := NormalizeThemeContent(content)
			wantSearchField(t, err, tc.field)
		})
	}

	// At the limits exactly, each rule passes.
	content := validTheme()
	content.Name = strings.Repeat("羊", MaxThemeNameRunes)
	content.Questions = numbered("问", MaxThemeQuestions)
	content.Keywords = numbered("词", MaxThemeKeywords)
	content.Origin, content.OriginNote = OriginCustomerQuestion, "9 月私信里有 6 个人问过"
	if _, err := NormalizeThemeContent(content); err != nil {
		t.Fatalf("a theme at every limit refused: %v", err)
	}
	content = validTheme()
	content.Questions = nil
	content.Origin, content.SourceIDs = OriginAuthorizedMaterial, []string{" src-1 ", "src-1"}
	normalized, err := NormalizeThemeContent(content)
	if err != nil || !slices.Equal(normalized.SourceIDs, []string{"src-1"}) {
		t.Fatalf("keywords only, one material: %+v %v", normalized.SourceIDs, err)
	}
}

func numbered(prefix string, n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = prefix + strings.Repeat("x", i%7) + string(rune('A'+i%26)) + string(rune('a'+i/26))
	}
	return out
}

// FR-017: NFC, trim, drop empty, dedupe in first-seen order; nothing else.
func TestSearchTermsAreOnlyNormalizedAndDeduplicated(t *testing.T) {
	decomposed := "café" // "café" as e + combining acute
	got := NormalizeSearchTerms([]string{"羊绒 ", "羊绒", " ", "Cashmere", "cashmere", decomposed, "café", "大衣", "羊绒"})
	want := []string{"羊绒", "Cashmere", "cashmere", "café", "大衣"}
	if !slices.Equal(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
	content := validTheme()
	content.Keywords = []string{"羊绒 ", "羊绒", "Cashmere", "cashmere"}
	content.Questions = []string{" 羊绒大衣能机洗吗 ", "羊绒大衣能机洗吗"}
	normalized, err := NormalizeThemeContent(content)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(normalized.Keywords, []string{"羊绒", "Cashmere", "cashmere"}) ||
		!slices.Equal(normalized.Questions, []string{"羊绒大衣能机洗吗"}) {
		t.Fatalf("normalized %q / %q", normalized.Keywords, normalized.Questions)
	}
}

// T011 / FR-014 / FR-080 / SC-002: any theme reads search_volume and
// competition as unknown with a reason, and data_origin manual_only. The
// stored and request types have nowhere to hold a number for them.
func TestSearchThemeReadsVolumeAndCompetitionAsUnknown(t *testing.T) {
	view := ViewTheme(SearchTheme{ThemeID: "t1", Revision: 3, ThemeContent: validTheme()})
	raw, err := json.Marshal(view)
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err = json.Unmarshal(raw, &body); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"search_volume", "competition"} {
		value, _ := body[field].(map[string]any)
		if value["status"] != "unknown" || value["reason"] != "no_data_source" || len(value) != 2 {
			t.Errorf("%s = %v, want {unknown, no_data_source}", field, body[field])
		}
	}
	if body["data_origin"] != "manual_only" {
		t.Errorf("data_origin = %v", body["data_origin"])
	}
	for _, absent := range []string{"rank", "score", "current_rank"} {
		if _, ok := body[absent]; ok {
			t.Errorf("a theme read carries %s", absent)
		}
	}
}

// forbiddenThemeFieldNames are names no stored or request field of a theme
// may have: a number for any of them would be invented (FR-014, FR-036).
var forbiddenThemeFieldNames = []string{"searchvolume", "competition", "rank", "score", "density", "keywordcount", "seo", "avg", "best"}

func structFieldNames(t reflect.Type) []string {
	names := []string{}
	for i := range t.NumField() {
		field := t.Field(i)
		names = append(names, field.Name, strings.Split(field.Tag.Get("json"), ",")[0])
		if field.Anonymous && field.Type.Kind() == reflect.Struct {
			names = append(names, structFieldNames(field.Type)...)
		}
	}
	return names
}

func TestSearchThemeStoredAndRequestTypesHaveNoSearchDataFields(t *testing.T) {
	for _, typ := range []reflect.Type{
		reflect.TypeFor[ThemeContent](), reflect.TypeFor[SearchTheme](), reflect.TypeFor[themeCreateWire](),
		reflect.TypeFor[themeRevisionWire](), reflect.TypeFor[ThemeRevisionRequest](),
	} {
		for _, name := range structFieldNames(typ) {
			flat := strings.ToLower(strings.ReplaceAll(name, "_", ""))
			for _, forbidden := range forbiddenThemeFieldNames {
				if strings.Contains(flat, forbidden) {
					t.Errorf("%s has a field %q (%s)", typ.Name(), name, forbidden)
				}
			}
		}
	}
	// The view carries search_volume and competition, and only as unknowns.
	view := reflect.TypeFor[SearchThemeView]()
	for _, name := range []string{"SearchVolume", "Competition"} {
		field, ok := view.FieldByName(name)
		if !ok || field.Type != reflect.TypeFor[SearchUnknown]() {
			t.Errorf("SearchThemeView.%s is %v, want SearchUnknown", name, field.Type)
		}
	}
}
