package topicplanning

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// Real PostgreSQL tests for search themes (specs/036 PR 1: T014 to T019;
// FR-010, FR-013, FR-015, FR-016, FR-103; SC-001, SC-012). They run through
// scripts/test-go-db.sh --suite topic-planning and skip without its
// database, like the rest of this package's fixture.

// newSearchFixture is the topic fixture plus the search theme migrations,
// found by name: the numbers are provisional until merge.
func newSearchFixture(t *testing.T) topicFixture {
	t.Helper()
	fx := newTopicFixture(t)
	_, current, _, _ := runtime.Caller(0)
	dir := filepath.Join(filepath.Dir(current), "..", "..", "..", "migrations")
	matches, err := filepath.Glob(filepath.Join(dir, "*_content_search_theme_revision*.up.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 3 {
		t.Fatalf("found %d search theme migrations, want 3", len(matches))
	}
	number := func(path string) int {
		n, _ := strconv.Atoi(strings.SplitN(filepath.Base(path), "_", 2)[0])
		return n
	}
	sort.Slice(matches, func(i, j int) bool { return number(matches[i]) < number(matches[j]) })
	for _, path := range matches {
		sql, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatal(readErr)
		}
		fx.db.Exec(t, string(sql))
	}
	return fx
}

// fakeSources is the material port: this brand's materials by id.
type fakeSources struct {
	byWorkspace map[string][]string
	reads       int
}

func (s *fakeSources) Exists(_ context.Context, workspaceID, sourceID string) (bool, error) {
	s.reads++
	for _, id := range s.byWorkspace[workspaceID] {
		if id == sourceID {
			return true, nil
		}
	}
	return false, nil
}

func seedSearchAccount(t *testing.T, fx topicFixture, workspace, accountID, platform string) {
	t.Helper()
	fx.db.Exec(t, `INSERT INTO content_account (account_id, workspace_id, platform, display_name, settings)
		VALUES ($1, $2, $3, $4, '{}'::jsonb)`, accountID, workspace, platform, "账号 "+accountID)
}

func seedSearchCardAndBrief(t *testing.T, fx topicFixture, workspace, cardID, briefID string) {
	t.Helper()
	fx.db.Exec(t, `INSERT INTO content_topic_card (topic_card_id, workspace_id, audience_problem_judgment, ip_fit,
		timing, existing_content_relation, evidence_gaps_and_investment, channels, recommended_action)
		VALUES ($1, $2, '', '', '', '', '', '[]'::jsonb, '')`, cardID, workspace)
	fx.db.Exec(t, `INSERT INTO content_brief_revision (brief_revision_id, topic_card_id, workspace_id, revision,
		audience, core_problem, claim_and_boundaries, channels, format, structure, citation_requirements,
		source_scope, deliverable, time_limit, cost_limit)
		VALUES ($1, $2, $3, 1, '', '', '', '[]'::jsonb, '', '', '', '', '', '', '')`, briefID, cardID, workspace)
}

func themeRowJSON(t *testing.T, fx topicFixture, themeID string, revision int64) string {
	t.Helper()
	var text string
	if err := fx.store.DB.QueryRow(t.Context(), `SELECT row_to_json(r)::text FROM content_search_theme_revision r
		WHERE theme_id = $1 AND revision = $2`, themeID, revision).Scan(&text); err != nil {
		t.Fatalf("read theme %s r%d: %v", themeID, revision, err)
	}
	return text
}

func wantConflict(t *testing.T, err error) {
	t.Helper()
	conflict, ok := errors.AsType[SearchConflict](err)
	if !ok || conflict.Field != "base_revision" {
		t.Fatalf("err = %v, want a 409 naming base_revision", err)
	}
}

// T014 / FR-010 / FR-016 / SC-001: create is revision 1 with everything read
// back; a revision leaves revision 1 byte for byte; a stale base is 409;
// archiving is revision 3; the list hides it unless asked.
func TestSearchThemeRevisionsAreAppendOnly(t *testing.T) {
	fx := newSearchFixture(t)
	ctx := t.Context()
	const workspace, actor = "ws-theme", "actor-a"
	seedSearchAccount(t, fx, workspace, "acct-xhs", "xiaohongshu")
	seedSearchCardAndBrief(t, fx, workspace, "card-1", "brief-1")
	fx.store.Sources = &fakeSources{byWorkspace: map[string][]string{workspace: {"src-1"}}}

	content := validTheme()
	content.AccountID = "acct-xhs"
	content.Origin, content.OriginNote = OriginCustomerQuestion, "9 月私信里有 6 个人问过"
	content.Questions = []string{"羊绒大衣能机洗吗", "羊绒大衣起球怎么办"}
	content.SourceIDs = []string{"src-1"}
	content.TopicCardIDs = []string{"card-1"}
	content.BriefRevisionIDs = []string{"brief-1"}
	created, err := fx.store.CreateSearchTheme(ctx, workspace, actor, content)
	if err != nil {
		t.Fatal(err)
	}
	if created.Revision != 1 || created.Voided || created.RecordedBy != actor || created.CreatedAt.IsZero() ||
		!reflect.DeepEqual(created.ThemeContent, content) {
		t.Fatalf("created = %+v", created)
	}
	if created.SearchVolume.Reason != UnknownNoDataSource || created.DataOrigin != DataOriginManualOnly {
		t.Fatalf("derived fields = %+v", created)
	}
	read, err := fx.store.GetSearchTheme(ctx, workspace, actor, created.ThemeID)
	if err != nil || !reflect.DeepEqual(read.ThemeContent, content) || read.Revision != 1 {
		t.Fatalf("read back %+v, %v", read, err)
	}
	first := themeRowJSON(t, fx, created.ThemeID, 1)

	edited := content
	edited.Keywords = []string{"羊绒护理", "羊绒"}
	second, err := fx.store.ReviseSearchTheme(ctx, workspace, actor, created.ThemeID,
		ThemeRevisionRequest{BaseRevision: 1, Content: edited})
	if err != nil || second.Revision != 2 || !reflect.DeepEqual(second.Keywords, edited.Keywords) {
		t.Fatalf("revision 2 = %+v, %v", second, err)
	}
	if got := themeRowJSON(t, fx, created.ThemeID, 1); got != first {
		t.Fatalf("revision 1 changed:\n%s\n%s", first, got)
	}
	_, err = fx.store.ReviseSearchTheme(ctx, workspace, actor, created.ThemeID,
		ThemeRevisionRequest{BaseRevision: 1, Content: edited})
	wantConflict(t, err)

	archived, err := fx.store.ReviseSearchTheme(ctx, workspace, actor, created.ThemeID,
		ThemeRevisionRequest{BaseRevision: 2, Voided: true, Content: edited})
	if err != nil || archived.Revision != 3 || !archived.Voided {
		t.Fatalf("archive = %+v, %v", archived, err)
	}
	history, err := fx.store.ListSearchThemeRevisions(ctx, workspace, actor, created.ThemeID)
	if err != nil || len(history) != 3 || history[0].Revision != 3 || history[2].Revision != 1 {
		t.Fatalf("history = %+v, %v", history, err)
	}
	if !reflect.DeepEqual(history[2].ThemeContent, content) || history[2].RecordedBy != actor {
		t.Fatalf("revision 1 in the history = %+v", history[2])
	}
	listed, err := fx.store.ListSearchThemes(ctx, workspace, actor, ThemeFilter{})
	if err != nil || len(listed) != 0 {
		t.Fatalf("an archived theme is listed by default: %+v %v", listed, err)
	}
	listed, err = fx.store.ListSearchThemes(ctx, workspace, actor, ThemeFilter{IncludeArchived: true})
	if err != nil || len(listed) != 1 || listed[0].Revision != 3 {
		t.Fatalf("include_archived = %+v %v", listed, err)
	}
	if n := fx.db.Count(t, `SELECT count(*) FROM content_operation_audit WHERE workspace_id = $1`, workspace); n != 3 {
		t.Fatalf("%d audit rows, want one per write (3)", n)
	}

	if _, err = fx.store.GetSearchTheme(ctx, workspace, actor, "no-such-theme"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing theme = %v", err)
	}
	if _, err = fx.store.ReviseSearchTheme(ctx, workspace, actor, "no-such-theme",
		ThemeRevisionRequest{BaseRevision: 1, Content: edited}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("revising a missing theme = %v", err)
	}
	if _, err = fx.store.GetSearchTheme(ctx, "ws-other", actor, created.ThemeID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("another brand reads the theme: %v", err)
	}
}

// T015 / FR-013 / SC-001: a material, topic card or brief that is another
// brand's is refused exactly like one that does not exist, naming the field;
// nothing is written.
func TestSearchThemeReferencesAreThisBrandsOnly(t *testing.T) {
	fx := newSearchFixture(t)
	ctx := t.Context()
	const workspace, other, actor = "ws-refs", "ws-refs-other", "actor-a"
	seedSearchCardAndBrief(t, fx, workspace, "card-own", "brief-own")
	seedSearchCardAndBrief(t, fx, other, "card-foreign", "brief-foreign")
	fx.store.Sources = &fakeSources{byWorkspace: map[string][]string{workspace: {"src-own"}, other: {"src-foreign"}}}

	for _, tc := range []struct {
		field            string
		foreign, missing func(*ThemeContent)
	}{
		{"source_ids",
			func(c *ThemeContent) { c.SourceIDs = []string{"src-own", "src-foreign"} },
			func(c *ThemeContent) { c.SourceIDs = []string{"src-own", "src-none"} }},
		{"topic_card_ids",
			func(c *ThemeContent) { c.TopicCardIDs = []string{"card-own", "card-foreign"} },
			func(c *ThemeContent) { c.TopicCardIDs = []string{"card-own", "card-none"} }},
		{"brief_revision_ids",
			func(c *ThemeContent) { c.BriefRevisionIDs = []string{"brief-foreign"} },
			func(c *ThemeContent) { c.BriefRevisionIDs = []string{"brief-none"} }},
	} {
		refusals := []string{}
		for _, edit := range []func(*ThemeContent){tc.foreign, tc.missing} {
			content := validTheme()
			edit(&content)
			_, err := fx.store.CreateSearchTheme(ctx, workspace, actor, content)
			wantSearchField(t, err, tc.field)
			encoded, _ := json.Marshal(err)
			refusals = append(refusals, string(encoded))
		}
		if refusals[0] != refusals[1] {
			t.Errorf("%s: foreign %s, missing %s - the refusal tells them apart", tc.field, refusals[0], refusals[1])
		}
	}
	if n := fx.db.Count(t, `SELECT count(*) FROM content_search_theme_revision`); n != 0 {
		t.Fatalf("%d themes written by refused requests", n)
	}
	content := validTheme()
	content.SourceIDs, content.TopicCardIDs, content.BriefRevisionIDs = []string{"src-own"}, []string{"card-own"}, []string{"brief-own"}
	if _, err := fx.store.CreateSearchTheme(ctx, workspace, actor, content); err != nil {
		t.Fatalf("this brand's own references refused: %v", err)
	}
}

// T016: an account that is not here is 400 account_id; one on another
// platform is 400 platform.
func TestSearchThemeAccountMustExistAndShareThePlatform(t *testing.T) {
	fx := newSearchFixture(t)
	ctx := t.Context()
	const workspace, actor = "ws-account", "actor-a"
	seedSearchAccount(t, fx, workspace, "acct-douyin", "douyin")
	seedSearchAccount(t, fx, "ws-account-other", "acct-foreign", "xiaohongshu")

	for accountID, field := range map[string]string{"acct-none": "account_id", "acct-foreign": "account_id", "acct-douyin": "platform"} {
		content := validTheme()
		content.AccountID = accountID
		_, err := fx.store.CreateSearchTheme(ctx, workspace, actor, content)
		wantSearchField(t, err, field)
	}
	content := validTheme()
	content.AccountID, content.Platform = "acct-douyin", "douyin"
	if _, err := fx.store.CreateSearchTheme(ctx, workspace, actor, content); err != nil {
		t.Fatalf("a douyin theme on the douyin account: %v", err)
	}
}

// T017: two writers of revision 2 at once. Both are held after every check
// has passed, so only the unique index can decide: one wins, the other gets
// the same 409 a stale base gets, and exactly two revisions exist.
func TestConcurrentSearchThemeRevisionsLeaveExactlyOneWinner(t *testing.T) {
	fx := newSearchFixture(t)
	const workspace, actor = "ws-race", "actor-a"
	created, err := fx.store.CreateSearchTheme(t.Context(), workspace, actor, validTheme())
	if err != nil {
		t.Fatal(err)
	}
	var arrived sync.WaitGroup
	arrived.Add(2)
	release := make(chan struct{})
	go func() {
		waited := make(chan struct{})
		go func() { arrived.Wait(); close(waited) }()
		select {
		case <-waited:
		case <-time.After(10 * time.Second):
			t.Error("both writers never reached the insert together; the index layer was not exercised")
		}
		close(release)
	}()
	ctx := context.WithValue(context.Background(), themeInsertHook{}, func() {
		arrived.Done()
		<-release
	})
	errs := make([]error, 2)
	var done sync.WaitGroup
	for i := range 2 {
		done.Add(1)
		go func() {
			defer done.Done()
			edited := validTheme()
			edited.Note = "writer " + strconv.Itoa(i)
			_, errs[i] = fx.store.ReviseSearchTheme(ctx, workspace, actor, created.ThemeID,
				ThemeRevisionRequest{BaseRevision: 1, Content: edited})
		}()
	}
	done.Wait()
	wins, conflicts := 0, 0
	for _, err := range errs {
		switch {
		case err == nil:
			wins++
		case errors.As(err, new(SearchConflict)):
			conflicts++
		default:
			t.Errorf("unexpected error: %v", err)
		}
	}
	if wins != 1 || conflicts != 1 {
		t.Fatalf("wins=%d conflicts=%d (%v), want one of each", wins, conflicts, errs)
	}
	if n := fx.db.Count(t, `SELECT count(*) FROM content_search_theme_revision WHERE theme_id = $1`, created.ThemeID); n != 2 {
		t.Fatalf("%d revisions stored, want 2", n)
	}
}

// T018 / FR-015: filters by platform, account and topic card; ordered by
// name, then theme_id.
func TestSearchThemeListFiltersAndOrder(t *testing.T) {
	fx := newSearchFixture(t)
	ctx := t.Context()
	const workspace, actor = "ws-list", "actor-a"
	seedSearchAccount(t, fx, workspace, "acct-xhs", "xiaohongshu")
	seedSearchCardAndBrief(t, fx, workspace, "card-1", "brief-1")
	created := []string{}
	for _, spec := range []struct{ name, platform, account, card string }{
		{"乙 洗护", "xiaohongshu", "acct-xhs", "card-1"},
		{"甲 保养", "douyin", "", ""},
		{"乙 洗护", "xiaohongshu", "", "card-1"},
		{"丙 选购", "zhihu", "", ""},
	} {
		content := validTheme()
		content.Name, content.Platform, content.AccountID = spec.name, spec.platform, spec.account
		if spec.card != "" {
			content.TopicCardIDs = []string{spec.card}
		}
		theme, err := fx.store.CreateSearchTheme(ctx, workspace, actor, content)
		if err != nil {
			t.Fatal(err)
		}
		created = append(created, theme.ThemeID)
	}
	// Two themes share the name "乙 洗护": theme_id breaks the tie.
	sameName := []string{created[0], created[2]}
	sort.Strings(sameName)
	idsOf := func(filter ThemeFilter) []string {
		views, err := fx.store.ListSearchThemes(ctx, workspace, actor, filter)
		if err != nil {
			t.Fatal(err)
		}
		out := []string{}
		for _, view := range views {
			out = append(out, view.ThemeID)
		}
		return out
	}
	for name, tc := range map[string]struct {
		filter ThemeFilter
		want   []string
	}{
		// By name in byte order (丙 U+4E19 < 乙 U+4E59 < 甲 U+7532), then id.
		"all, by name then id": {ThemeFilter{}, []string{created[3], sameName[0], sameName[1], created[1]}},
		"platform":             {ThemeFilter{Platform: "xiaohongshu"}, sameName},
		"account":              {ThemeFilter{AccountID: "acct-xhs"}, []string{created[0]}},
		"topic card":           {ThemeFilter{TopicCardID: "card-1"}, sameName},
		"nothing":              {ThemeFilter{Platform: "weibo"}, []string{}},
	} {
		if got := idsOf(tc.filter); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: %v, want %v", name, got, tc.want)
		}
	}
	if _, err := ParseThemeFilter("taobao", "", "", ""); err == nil {
		t.Error("an unknown platform filter was accepted")
	}
}

// T019 / FR-103 / SC-012: once the workspace deletion has committed, the
// fence refuses creating a theme and writing a revision with ErrNotFound -
// a 404, not a 503 - before any reference is read, and no row is left.
func TestSearchThemeWritesAreFencedByWorkspaceDeletion(t *testing.T) {
	fx := newSearchFixture(t)
	ctx := t.Context()
	const workspace, actor = "ws-fence", "actor-a"
	seedSearchAccount(t, fx, workspace, "acct-xhs", "xiaohongshu")
	sources := &fakeSources{byWorkspace: map[string][]string{workspace: {"src-1"}}}
	fx.store.Sources = sources
	content := validTheme()
	content.AccountID = "acct-xhs"
	content.SourceIDs = []string{"src-1"}
	created, err := fx.store.CreateSearchTheme(ctx, workspace, actor, content)
	if err != nil {
		t.Fatal(err)
	}

	guard := &fenceRecordingGuard{denied: true}
	accounts := &fenceCheckingAccounts{testAccountReader: fx.store.Accounts.(testAccountReader), guard: guard}
	fx.store.Guard, fx.store.Accounts = guard, accounts
	sources.reads = 0
	if _, err = fx.store.CreateSearchTheme(ctx, workspace, actor, content); !errors.Is(err, ErrNotFound) {
		t.Fatalf("create after the deletion = %v, want ErrNotFound", err)
	}
	if _, err = fx.store.ReviseSearchTheme(ctx, workspace, actor, created.ThemeID,
		ThemeRevisionRequest{BaseRevision: 1, Content: content}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("revision after the deletion = %v, want ErrNotFound", err)
	}
	if accounts.reads != 0 || sources.reads != 0 {
		t.Fatalf("references read after the fence refused: accounts %d, materials %d", accounts.reads, sources.reads)
	}
	if n := fx.db.Count(t, `SELECT count(*) FROM content_search_theme_revision WHERE workspace_id = $1`, workspace); n != 1 {
		t.Fatalf("%d theme rows, want only the one from before the deletion", n)
	}
}
