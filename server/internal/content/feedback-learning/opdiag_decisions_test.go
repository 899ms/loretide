package feedbacklearning

import (
	"context"
	"encoding/json"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// specs/035 PR 3 without a database: the controlled sets (T061), the
// judgement, suggestion and decision checks by name (T062, T064), the
// topic card key that makes a retry find the same card (FR-063a, T070 at
// the module level), and the two source guards (T075, T075a). The
// database cases - reject writes nothing else, one transaction per todo or
// proposal adoption, the three topic card steps, the retries, the proposal
// confirmation - run in the handler suite against the real schema.

// T061: every PR 3 set is exactly its contract §3 values, in order.
func TestOpDiagDecisionSetsAreExactly(t *testing.T) {
	for name, pair := range map[string][2][]string{
		"JudgementKinds":    {toStrings(JudgementKinds), {"judgement", "alternative_explanation", "limitation"}},
		"JudgementBases":    {toStrings(JudgementBases), {"evidence", "qualitative"}},
		"AuthorKinds":       {toStrings(AuthorKinds), {"human"}},
		"SuggestionTargets": {toStrings(SuggestionTargets), {"topic_card", "todo", "profile_proposal"}},
		"DecisionKinds":     {toStrings(DecisionKinds), {"adopt", "reject"}},
		"AdoptModes":        {toStrings(AdoptModes), {"create", "link"}},
		"EffectOutcomes":    {toStrings(EffectOutcomes), {"done", "failed"}},
		"EffectFailures":    {toStrings(EffectFailures), {"target_refused", "target_not_found", "storage"}},
		"ProposalStates":    {toStrings(ProposalStates), {"proposed", "confirmed", "dismissed"}},
		"TodoStates":        {toStrings(TodoStates), {"open", "done", "dropped"}},
		"TodoOrigins":       {toStrings(TodoOrigins), {"suggestion", "data_gap"}},
	} {
		if !slices.Equal(pair[0], pair[1]) {
			t.Errorf("%s = %v, want exactly %v", name, pair[0], pair[1])
		}
	}
	// FR-068: no target writes a business memory, under any spelling.
	for _, target := range SuggestionTargets {
		if strings.Contains(string(target), "memory") || strings.Contains(string(target), "conclusion") {
			t.Errorf("suggestion target %q", target)
		}
	}
	// FR-067: a proposal changes the eight text items, and those are
	// ip-profile's first eight (TestTheProfileKeysAgreeWithIPProfiles).
	if len(profileTextKeys) != 8 || slices.Contains(profileTextKeys, "primary_channels") {
		t.Fatalf("profileTextKeys = %v", profileTextKeys)
	}
}

func testFacts() versionFacts {
	return versionFacts{
		refs: []string{"scope", "account:a1/cadence", "gap/scope/profile_field_pending/account/a1/positioning"},
		gaps: []DiagnosisGap{{GapKey: "scope/profile_field_pending/account/a1/positioning", AccountID: "a1"}},
	}
}

// T062 / FR-050 / FR-051: a judgement is refused by name, in the
// contract's order.
func TestJudgementsAreRefusedByName(t *testing.T) {
	valid := JudgementInput{Kind: JudgementKindJudgement, Basis: BasisEvidence, EvidenceRefs: []string{"scope"}, Body: "节奏稳定"}
	others := map[string]OpDiagJudgement{
		"j1":     {JudgementID: "j1"},
		"voided": {JudgementID: "voided", Voided: true},
	}
	if err := validateJudgement(valid, testFacts(), "", others); err != nil {
		t.Fatalf("a valid judgement: %v", err)
	}
	qualitative := valid
	qualitative.Basis, qualitative.EvidenceRefs = BasisQualitative, nil
	if err := validateJudgement(qualitative, testFacts(), "", others); err != nil {
		t.Fatalf("a qualitative judgement: %v", err)
	}
	alternative := qualitative
	alternative.Kind, alternative.AboutJudgementID = JudgementKindAlternative, "j1"
	if err := validateJudgement(alternative, testFacts(), "", others); err != nil {
		t.Fatalf("an alternative explanation: %v", err)
	}
	for _, tc := range []struct {
		name  string
		edit  func(*JudgementInput)
		self  string
		field string
	}{
		{"kind", func(in *JudgementInput) { in.Kind = "score" }, "", "kind"},
		{"basis", func(in *JudgementInput) { in.Basis = "ai" }, "", "basis"},
		{"evidence without refs", func(in *JudgementInput) { in.EvidenceRefs = nil }, "", "evidence_refs"},
		{"qualitative with refs", func(in *JudgementInput) { in.Basis = BasisQualitative }, "", "evidence_refs"},
		{"about on a judgement", func(in *JudgementInput) { in.AboutJudgementID = "j1" }, "", "about_judgement_id"},
		{"about on a limitation", func(in *JudgementInput) {
			in.Kind, in.AboutJudgementID = JudgementKindLimitation, "j1"
		}, "", "about_judgement_id"},
		{"blank body", func(in *JudgementInput) { in.Body = "  " }, "", "body"},
		{"long body", func(in *JudgementInput) { in.Body = strings.Repeat("判", MaxAnnotationBodyRunes+1) }, "", "body"},
		{"a ref not in the version", func(in *JudgementInput) { in.EvidenceRefs = []string{"account:a9/cadence"} }, "", "evidence_refs"},
		{"a ref twice", func(in *JudgementInput) { in.EvidenceRefs = []string{"scope", "scope"} }, "", "evidence_refs"},
		{"about another version's judgement", func(in *JudgementInput) {
			in.Kind, in.AboutJudgementID = JudgementKindAlternative, "elsewhere"
		}, "", "about_judgement_id"},
		{"about a voided judgement", func(in *JudgementInput) {
			in.Kind, in.AboutJudgementID = JudgementKindAlternative, "voided"
		}, "", "about_judgement_id"},
		{"about itself", func(in *JudgementInput) {
			in.Kind, in.AboutJudgementID = JudgementKindAlternative, "j1"
		}, "j1", "about_judgement_id"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := valid
			in.EvidenceRefs = slices.Clone(valid.EvidenceRefs)
			tc.edit(&in)
			fieldErrorNaming(t, validateJudgement(in, testFacts(), tc.self, others), tc.field)
		})
	}
}

// T064 / FR-052 / FR-067 / contract §7.3: a suggestion's target is decoded
// by its kind, and a profile proposal changes only the eight text items,
// each once, one to eight of them.
func TestSuggestionTargetsAreHeldToTheirKind(t *testing.T) {
	judgements := map[string]OpDiagJudgement{"j1": {JudgementID: "j1"}, "jv": {JudgementID: "jv", Voided: true}}
	suggestion := func(kind SuggestionTarget, target string) SuggestionInput {
		return SuggestionInput{Body: "下周补两篇面料知识", TargetKind: kind, Target: json.RawMessage(target),
			JudgementIDs: []string{"j1"}, EvidenceRefs: []string{"account:a1/cadence"}}
	}
	for name, tc := range map[string]struct {
		in   SuggestionInput
		want SuggestionTargetParams
	}{
		"topic card":         {suggestion(TargetTopicCard, `{"account_id":"a1"}`), SuggestionTargetParams{AccountID: "a1"}},
		"topic card, no one": {suggestion(TargetTopicCard, `{}`), SuggestionTargetParams{}},
		"topic card, null":   {suggestion(TargetTopicCard, `null`), SuggestionTargetParams{}},
		"todo":               {suggestion(TargetTodo, `{"title":"补录指标","account_id":""}`), SuggestionTargetParams{Title: "补录指标"}},
		"proposal": {suggestion(TargetProfileProposal,
			`{"account_id":"a1","patches":[{"field":"content_pillars","value":"面料知识、穿搭"}]}`),
			SuggestionTargetParams{AccountID: "a1", Patches: []ProfilePatch{{Field: "content_pillars", Value: "面料知识、穿搭"}}}},
	} {
		got, err := validateSuggestion(tc.in, testFacts(), judgements)
		if err != nil || !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s = %+v, %v; want %+v", name, got, err, tc.want)
		}
	}
	patch := func(fields ...string) string {
		items := []string{}
		for _, field := range fields {
			items = append(items, `{"field":`+strconv.Quote(field)+`,"value":"x"}`)
		}
		return `{"account_id":"a1","patches":[` + strings.Join(items, ",") + `]}`
	}
	for _, tc := range []struct {
		name  string
		in    SuggestionInput
		field string
	}{
		{"target kind", suggestion("business_memory", `{}`), "target_kind"},
		{"a todo member on a topic card", suggestion(TargetTopicCard, `{"account_id":"a1","title":"x"}`), "target.title"},
		{"patches on a todo", suggestion(TargetTodo, `{"title":"x","patches":[]}`), "target.patches"},
		{"a wrong type", suggestion(TargetTopicCard, `{"account_id":1}`), "target.account_id"},
		{"not an object", suggestion(TargetTopicCard, `[1]`), "target"},
		{"todo without a title", suggestion(TargetTodo, `{"account_id":""}`), "target.title"},
		{"todo title too long", suggestion(TargetTodo, `{"title":"`+strings.Repeat("办", MaxTodoTitleRunes+1)+`"}`), "target.title"},
		{"proposal without an account", suggestion(TargetProfileProposal, `{"patches":[{"field":"audience","value":"x"}]}`), "target.account_id"},
		{"proposal without patches", suggestion(TargetProfileProposal, `{"account_id":"a1","patches":[]}`), "target.patches"},
		{"persona prompt", suggestion(TargetProfileProposal, patch("persona_prompt")), "target.patches.field"},
		{"primary channels", suggestion(TargetProfileProposal, patch("primary_channels")), "target.patches.field"},
		{"weekly hours", suggestion(TargetProfileProposal, patch("weekly_hours")), "target.patches.field"},
		{"style samples", suggestion(TargetProfileProposal, patch("style_samples")), "target.patches.field"},
		{"a field twice", suggestion(TargetProfileProposal, patch("audience", "audience")), "target.patches.field"},
		{"nine items", suggestion(TargetProfileProposal, patch(append(slices.Clone(profileTextKeys), "audience")...)), "target.patches"},
		{"a value too long", suggestion(TargetProfileProposal,
			`{"account_id":"a1","patches":[{"field":"audience","value":"`+strings.Repeat("长", MaxProfilePatchValueRunes+1)+`"}]}`),
			"target.patches.value"},
		{"blank body", func() SuggestionInput { in := suggestion(TargetTopicCard, `{}`); in.Body = ""; return in }(), "body"},
		{"a judgement of another version", func() SuggestionInput {
			in := suggestion(TargetTopicCard, `{}`)
			in.JudgementIDs = []string{"elsewhere"}
			return in
		}(), "judgement_ids"},
		{"a voided judgement", func() SuggestionInput {
			in := suggestion(TargetTopicCard, `{}`)
			in.JudgementIDs = []string{"jv"}
			return in
		}(), "judgement_ids"},
		{"a ref not in the version", func() SuggestionInput {
			in := suggestion(TargetTopicCard, `{}`)
			in.EvidenceRefs = []string{"brand/cadence"}
			return in
		}(), "evidence_refs"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := validateSuggestion(tc.in, testFacts(), judgements)
			fieldErrorNaming(t, err, tc.field)
		})
	}
	// All eight text items at once is the most a proposal changes.
	if _, err := validateSuggestion(suggestion(TargetProfileProposal, patch(profileTextKeys...)), testFacts(), judgements); err != nil {
		t.Fatalf("all eight text items: %v", err)
	}
}

// FR-060 / FR-062 / contract §7.1 steps 4 to 6: a decision is refused by
// name; only adopting a topic card takes a mode, and link takes a card.
func TestDecisionsAreRefusedByName(t *testing.T) {
	adoptCard := DecisionInput{SuggestionRevision: 1, Decision: DecisionAdopt, Mode: ModeCreate}
	for name, tc := range map[string]struct {
		in   DecisionInput
		kind SuggestionTarget
	}{
		"reject":       {DecisionInput{SuggestionRevision: 1, Decision: DecisionReject}, TargetTopicCard},
		"adopt create": {adoptCard, TargetTopicCard},
		"adopt link":   {DecisionInput{SuggestionRevision: 2, Decision: DecisionAdopt, Mode: ModeLink, LinkTargetID: "card-1"}, TargetTopicCard},
		"adopt todo":   {DecisionInput{SuggestionRevision: 1, Decision: DecisionAdopt}, TargetTodo},
		"adopt proposal": {DecisionInput{SuggestionRevision: 1, Decision: DecisionAdopt, Note: "确认后再改"},
			TargetProfileProposal},
	} {
		if err := validateDecision(tc.in, tc.kind); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	for _, tc := range []struct {
		name  string
		in    DecisionInput
		kind  SuggestionTarget
		field string
	}{
		{"decision", DecisionInput{SuggestionRevision: 1, Decision: "maybe"}, TargetTodo, "decision"},
		{"mode", DecisionInput{SuggestionRevision: 1, Decision: DecisionAdopt, Mode: "copy"}, TargetTopicCard, "mode"},
		{"no revision", DecisionInput{Decision: DecisionReject}, TargetTodo, "suggestion_revision"},
		{"a card without a mode", DecisionInput{SuggestionRevision: 1, Decision: DecisionAdopt}, TargetTopicCard, "mode"},
		{"a mode on a todo", DecisionInput{SuggestionRevision: 1, Decision: DecisionAdopt, Mode: ModeCreate}, TargetTodo, "mode"},
		{"a mode on a reject", DecisionInput{SuggestionRevision: 1, Decision: DecisionReject, Mode: ModeLink}, TargetTopicCard, "mode"},
		{"a link target on a reject", DecisionInput{SuggestionRevision: 1, Decision: DecisionReject, LinkTargetID: "c"}, TargetTopicCard, "link_target_id"},
		{"link without a card", DecisionInput{SuggestionRevision: 1, Decision: DecisionAdopt, Mode: ModeLink}, TargetTopicCard, "link_target_id"},
		{"create with a card", DecisionInput{SuggestionRevision: 1, Decision: DecisionAdopt, Mode: ModeCreate, LinkTargetID: "c"}, TargetTopicCard, "link_target_id"},
		{"note", DecisionInput{SuggestionRevision: 1, Decision: DecisionReject, Note: strings.Repeat("注", MaxDecisionNoteRunes+1)}, TargetTodo, "note"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fieldErrorNaming(t, validateDecision(tc.in, tc.kind), tc.field)
		})
	}
}

// An adopted decision with no outcome row reads "adopted, outcome not
// recorded" - the state a retry converges; one done outcome among failed
// ones reads done; a reject has no outcome to read.
func TestADecisionsOutcomeIsReadFromItsRows(t *testing.T) {
	done, failed := Effect{Outcome: OutcomeDone}, Effect{Outcome: OutcomeFailed}
	for _, tc := range []struct {
		decision DecisionKind
		effects  []Effect
		want     string
	}{
		{DecisionReject, nil, EffectReadsNone},
		{DecisionAdopt, nil, EffectReadsUnrecorded},
		{DecisionAdopt, []Effect{failed}, EffectReadsFailed},
		{DecisionAdopt, []Effect{failed, done}, EffectReadsDone},
	} {
		if got := effectState(tc.decision, tc.effects); got != tc.want {
			t.Errorf("%s with %d outcome(s) = %s, want %s", tc.decision, len(tc.effects), got, tc.want)
		}
	}
}

// keyedCards is TopicCardCreator the way topic-planning's CreateOnce
// behaves: one card per key, created the first time and answered after.
type keyedCards struct {
	byKey map[string]string
	keys  []string
	fail  error
}

func (c *keyedCards) CreateOnce(_ context.Context, _, _, key string, _ TopicCardDraft) (string, bool, error) {
	c.keys = append(c.keys, key)
	if c.fail != nil {
		return "", false, c.fail
	}
	if id, ok := c.byKey[key]; ok {
		return id, false, nil
	}
	id := "card-" + strconv.Itoa(len(c.byKey)+1)
	c.byKey[key] = id
	return id, true, nil
}

func (c *keyedCards) Exists(context.Context, string, string, string, string) error { return nil }

// FR-063a / Q3 supplement / T070 at the module level: the adoption, a retry
// after the outcome was lost, and the adoption of a later revision of the
// same suggestion all ask for the card under one key - so there is one
// card. A retry that asked under a new key would make a second.
func TestEveryAttemptForOneSuggestionAsksForTheSameCard(t *testing.T) {
	cards := &keyedCards{byKey: map[string]string{}}
	store := &DiagnosisStore{TopicCards: cards}
	first := OpDiagSuggestion{SuggestionID: "s1", Revision: 1, Body: "写一篇面料对比", Target: json.RawMessage(`{"account_id":"a1"}`)}
	later := first
	later.Revision, later.Body = 2, "改成两篇"
	ids := []string{}
	for _, suggestion := range []OpDiagSuggestion{first, first, later, later} {
		id, failure := store.createTopicCard(t.Context(), "ws", "actor", suggestion)
		if failure != "" {
			t.Fatalf("createTopicCard failed: %s", failure)
		}
		ids = append(ids, id)
	}
	if len(cards.byKey) != 1 || len(slices.Compact(slices.Clone(ids))) != 1 {
		t.Fatalf("cards %v from attempts %v; want one card for one suggestion", cards.byKey, ids)
	}
	for _, key := range cards.keys {
		if key != "opdiag-suggestion:s1" {
			t.Fatalf("asked under %q, want opdiag-suggestion:s1", key)
		}
	}
	// Another suggestion is another card.
	other := first
	other.SuggestionID = "s2"
	if id, _ := store.createTopicCard(t.Context(), "ws", "actor", other); id == ids[0] || len(cards.byKey) != 2 {
		t.Fatalf("another suggestion got %s; cards %v", id, cards.byKey)
	}
}

// T069: why creating the card failed is named, never guessed.
func TestAFailedCardNamesWhy(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want EffectFailure
	}{
		{ErrNotFound, FailureTargetNotFound},
		{FieldError{Field: "account_id", Reason: "x"}, FailureTargetRefused},
		{ErrInvalid, FailureTargetRefused},
		{ErrStorage, FailureStorage},
		{errors.New("connection reset"), FailureStorage},
	} {
		store := &DiagnosisStore{TopicCards: &keyedCards{byKey: map[string]string{}, fail: tc.err}}
		id, failure := store.createTopicCard(t.Context(), "ws", "actor", OpDiagSuggestion{SuggestionID: "s1"})
		if id != "" || failure != tc.want {
			t.Errorf("%v -> %q %q, want %q", tc.err, id, failure, tc.want)
		}
	}
	if _, failure := (&DiagnosisStore{}).createTopicCard(t.Context(), "ws", "actor", OpDiagSuggestion{}); failure != FailureStorage {
		t.Errorf("no creator wired -> %q, want storage", failure)
	}
}

// ---------------------------------------------------------------- source guards

// moduleFuncs parses the package's non-test files and answers each function
// and method declaration by name (methods of different receivers that share
// a name are all kept).
func moduleFuncs(t *testing.T) map[string][]*ast.FuncDecl {
	t.Helper()
	funcs := map[string][]*ast.FuncDecl{}
	fset := token.NewFileSet()
	for _, name := range nonTestFiles(t) {
		file, err := parser.ParseFile(fset, filepath.Join(moduleDir(t), name), nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range file.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok && fn.Body != nil {
				funcs[fn.Name.Name] = append(funcs[fn.Name.Name], fn)
			}
		}
	}
	return funcs
}

// calledNames answers every name a function body calls or selects,
// following calls into this package's own functions.
func calledNames(funcs map[string][]*ast.FuncDecl, root string) map[string]bool {
	seen := map[string]bool{}
	visited := map[string]bool{}
	var walk func(name string)
	walk = func(name string) {
		if visited[name] {
			return
		}
		visited[name] = true
		for _, fn := range funcs[name] {
			ast.Inspect(fn.Body, func(node ast.Node) bool {
				switch n := node.(type) {
				case *ast.SelectorExpr:
					seen[n.Sel.Name] = true
					walk(n.Sel.Name)
				case *ast.CallExpr:
					if ident, ok := n.Fun.(*ast.Ident); ok {
						seen[ident.Name] = true
						walk(ident.Name)
					}
				}
				return true
			})
		}
	}
	walk(root)
	return seen
}

// T075 / FR-061 / SC-006 / contract §9 "拒绝不写": nothing a reject runs -
// recordRejection and everything it calls here - reaches a method of
// either write interface, the adapter fields that hold them, or the
// inserts of the todo, proposal and outcome tables. And the reject branch
// is taken before anything that adopts.
func TestARejectCallsNoWriteInterface(t *testing.T) {
	funcs := moduleFuncs(t)
	if len(funcs["recordRejection"]) != 1 {
		t.Fatalf("recordRejection is gone (%d); the reject path cannot be checked", len(funcs["recordRejection"]))
	}
	forbidden := []string{"TopicCards", "Profiles", "insertEffect", "insertTodo", "insertProposal", "createTopicCard"}
	for _, iface := range []reflect.Type{reflect.TypeFor[TopicCardCreator](), reflect.TypeFor[DiagProfileWriter]()} {
		for i := range iface.NumMethod() {
			forbidden = append(forbidden, iface.Method(i).Name)
		}
	}
	called := calledNames(funcs, "recordRejection")
	if !called["insertDecision"] || !called["inTx"] {
		t.Fatalf("recordRejection no longer writes its decision through inTx: %v", called)
	}
	for _, name := range forbidden {
		if called[name] {
			t.Errorf("the reject path reaches %s", name)
		}
	}
	// In DecideSuggestion the reject branch returns before any adopt call.
	decide := funcs["DecideSuggestion"]
	if len(decide) != 1 {
		t.Fatal("DecideSuggestion is gone")
	}
	fset := token.NewFileSet()
	source, err := os.ReadFile(filepath.Join(moduleDir(t), "opdiag_decisions.go"))
	if err != nil {
		t.Fatal(err)
	}
	file, err := parser.ParseFile(fset, "opdiag_decisions.go", source, 0)
	if err != nil {
		t.Fatal(err)
	}
	var body string
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name == "DecideSuggestion" {
			body = string(source[fset.Position(fn.Body.Pos()).Offset:fset.Position(fn.Body.End()).Offset])
		}
	}
	reject := strings.Index(body, "return s.recordRejection(")
	firstAdopt := strings.Index(body, "s.adopt")
	if reject < 0 || firstAdopt < 0 || reject > firstAdopt {
		t.Fatalf("the reject branch is not taken before adopting (reject at %d, adopt at %d)", reject, firstAdopt)
	}
	for _, name := range forbidden {
		if strings.Contains(body[:reject], "."+name+"(") {
			t.Errorf("DecideSuggestion calls %s before it branches on reject", name)
		}
	}
}

// T075a (controller 2026-09-25): feedback-learning imports neither
// topic-planning nor ip-profile, in any Go file here, tests included. Cards
// and profiles are reached through TopicCardCreator and DiagProfileWriter,
// answered by handler adapters.
func TestThisModuleImportsNeitherTopicPlanningNorIPProfile(t *testing.T) {
	entries, err := os.ReadDir(moduleDir(t))
	if err != nil {
		t.Fatal(err)
	}
	scanned, tests := 0, 0
	fset := token.NewFileSet()
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
			continue
		}
		file, parseErr := parser.ParseFile(fset, filepath.Join(moduleDir(t), entry.Name()), nil, parser.ImportsOnly)
		if parseErr != nil {
			t.Fatal(parseErr)
		}
		scanned++
		if strings.HasSuffix(entry.Name(), "_test.go") {
			tests++
		}
		for _, spec := range file.Imports {
			path, _ := strconv.Unquote(spec.Path.Value)
			if strings.HasSuffix(path, "/content/topic-planning") || strings.HasSuffix(path, "/content/ip-profile") {
				t.Errorf("%s imports %s", entry.Name(), path)
			}
		}
	}
	if scanned < 30 || tests == 0 {
		t.Fatalf("scanned %d files (%d tests); the guard would pass vacuously", scanned, tests)
	}
}
