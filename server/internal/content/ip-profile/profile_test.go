package ipprofile

import (
	"context"
	"errors"
	"testing"
)

// Contract: specs/021-account-expression-profile/contracts/expression-profile.md

func confirmed(value string) TextField { return TextField{Value: value, Status: FieldConfirmed} }

func startableProfile() ExpressionProfile {
	return ExpressionProfile{
		Audience:        confirmed("刚开始做副业的设计师"),
		ContentPillars:  confirmed("工具评测、接单经验"),
		PrimaryChannels: ListField{Values: []string{"xiaohongshu"}, Status: FieldConfirmed},
		WeeklyHours:     HoursField{Value: 6, Status: FieldConfirmed},
	}
}

// A3, and the heart of the card. SOP 3.1 names four things that must never be
// guessed; each gets its own case, because one blanket assertion cannot say
// which of the four leaked.
func TestAnEmptyProfileIsAllPendingAndCarriesNothing(t *testing.T) {
	stored := NormalizeProfile(ExpressionProfile{})

	for name, field := range map[string]TextField{
		"identity — positioning":        stored.Positioning,
		"experience":                    stored.Experience,
		"results — content goals":       stored.ContentGoals,
		"commercial — common questions": stored.CommonQuestions,
		"audience":                      stored.Audience,
		"content pillars":               stored.ContentPillars,
		"expression style":              stored.ExpressionStyle,
		"forbidden expressions":         stored.ForbiddenExpressions,
	} {
		t.Run(name, func(t *testing.T) {
			if field.Value != "" {
				t.Errorf("the system wrote %q into a field nobody filled in", field.Value)
			}
			if field.Status != FieldPending {
				t.Errorf("status = %q, want %q", field.Status, FieldPending)
			}
		})
	}
	if len(stored.PrimaryChannels.Values) != 0 || len(stored.StyleSamples.Values) != 0 {
		t.Error("the system invented a list entry")
	}
	if stored.WeeklyHours.Value != 0 || stored.WeeklyHours.Status != FieldPending {
		t.Errorf("weekly hours = %+v, want an unfilled pending field", stored.WeeklyHours)
	}
}

// Marking a blank field confirmed records a decision the creator did not make.
// The status is lowered rather than the request refused: a profile somebody is
// halfway through is the normal case, and one blank must not fail the save.
func TestABlankFieldCannotBeConfirmed(t *testing.T) {
	stored := NormalizeProfile(ExpressionProfile{
		Audience:        TextField{Value: "   ", Status: FieldConfirmed},
		PrimaryChannels: ListField{Values: []string{"", "  "}, Status: FieldConfirmed},
		StyleSamples:    ListField{Values: []string{}, Status: FieldConfirmed},
	})

	if stored.Audience.Status != FieldPending {
		t.Errorf("a whitespace-only value was confirmed")
	}
	if stored.PrimaryChannels.Status != FieldPending || len(stored.PrimaryChannels.Values) != 0 {
		t.Errorf("a list of blanks was confirmed: %+v", stored.PrimaryChannels)
	}
	if stored.StyleSamples.Status != FieldPending {
		t.Errorf("an empty sample list was confirmed")
	}
}

// Saying nothing about a status is not confirming.
func TestAValueWithNoStatusIsPending(t *testing.T) {
	stored := NormalizeProfile(ExpressionProfile{Audience: TextField{Value: "设计师"}})

	if stored.Audience.Status != FieldPending {
		t.Errorf("status = %q, want %q", stored.Audience.Status, FieldPending)
	}
	if stored.Audience.Value != "设计师" {
		t.Errorf("normalising changed the value to %q", stored.Audience.Value)
	}
}

func TestNormalisingKeepsWhatTheCreatorTyped(t *testing.T) {
	stored := NormalizeProfile(startableProfile())

	if stored.Audience.Value != "刚开始做副业的设计师" || stored.Audience.Status != FieldConfirmed {
		t.Errorf("a confirmed answer was altered: %+v", stored.Audience)
	}
	if stored.WeeklyHours.Value != 6 || stored.WeeklyHours.Status != FieldConfirmed {
		t.Errorf("weekly hours changed: %+v", stored.WeeklyHours)
	}
}

// A8 / A9. Shape problems are refused; being incomplete never is.
func TestValidateRefusesOnlyUnusableShapes(t *testing.T) {
	if err := ValidateProfile(ExpressionProfile{}); err != nil {
		t.Errorf("an empty profile was refused: %v — incomplete is the normal state", err)
	}
	if err := ValidateProfile(startableProfile()); err != nil {
		t.Errorf("a filled profile was refused: %v", err)
	}

	for name, profile := range map[string]ExpressionProfile{
		"a status nothing recognises": {Audience: TextField{Value: "x", Status: "maybe"}},
		"a channel outside the controlled set": {
			PrimaryChannels: ListField{Values: []string{"myspace"}, Status: FieldConfirmed},
		},
		"negative hours":          {WeeklyHours: HoursField{Value: -1, Status: FieldConfirmed}},
		"more hours than a week":  {WeeklyHours: HoursField{Value: MaxWeeklyHours + 1, Status: FieldConfirmed}},
		"an over-long answer":     {Audience: TextField{Value: overlong(), Status: FieldConfirmed}},
		"too many list entries":   {StyleSamples: ListField{Values: manyEntries(), Status: FieldConfirmed}},
		"an over-long list entry": {StyleSamples: ListField{Values: []string{overlong()}, Status: FieldConfirmed}},
	} {
		t.Run(name, func(t *testing.T) {
			if err := ValidateProfile(profile); !errors.Is(err, ErrProfile) {
				t.Errorf("got %v, want ErrProfile", err)
			}
		})
	}
}

func overlong() string {
	runes := make([]rune, MaxProfileFieldRunes+1)
	for i := range runes {
		runes[i] = '字'
	}
	return string(runes)
}

func manyEntries() []string {
	entries := make([]string, MaxProfileListEntries+1)
	for i := range entries {
		entries[i] = "sample"
	}
	return entries
}

// A5 / A6. The readiness matrix. Every row says what it expects next to what it
// is given, and the missing list is asserted, not just the boolean — a caller
// has to be able to tell the creator what is still needed.
func TestReadinessMatrix(t *testing.T) {
	for _, tc := range []struct {
		name     string
		profile  func(ExpressionProfile) ExpressionProfile
		canStart bool
		missing  []string
	}{
		{
			name:     "all four confirmed",
			profile:  func(p ExpressionProfile) ExpressionProfile { return p },
			canStart: true, missing: []string{},
		},
		{
			name:     "no audience",
			profile:  func(p ExpressionProfile) ExpressionProfile { p.Audience = TextField{}; return p },
			canStart: false, missing: []string{MissingAudience},
		},
		{
			name:     "no content direction",
			profile:  func(p ExpressionProfile) ExpressionProfile { p.ContentPillars = TextField{}; return p },
			canStart: false, missing: []string{MissingPillars},
		},
		{
			name:     "no channel",
			profile:  func(p ExpressionProfile) ExpressionProfile { p.PrimaryChannels = ListField{}; return p },
			canStart: false, missing: []string{MissingChannels},
		},
		{
			name:     "no time budget",
			profile:  func(p ExpressionProfile) ExpressionProfile { p.WeeklyHours = HoursField{}; return p },
			canStart: false, missing: []string{MissingWeeklyHours},
		},
		{
			// Confirming zero hours says there is no time to spend. Reading that
			// as "ready to start" would be absurd; the contract records that
			// this is an interpretation.
			name: "zero hours, confirmed",
			profile: func(p ExpressionProfile) ExpressionProfile {
				p.WeeklyHours = HoursField{Value: 0, Status: FieldConfirmed}
				return p
			},
			canStart: false, missing: []string{MissingWeeklyHours},
		},
		{
			// The point of confirmation. A typed but unconfirmed answer is
			// exactly 3.1's "pending", and letting it count would make
			// confirming decorative.
			name: "everything typed, nothing confirmed",
			profile: func(ExpressionProfile) ExpressionProfile {
				return ExpressionProfile{
					Audience:        TextField{Value: "设计师", Status: FieldPending},
					ContentPillars:  TextField{Value: "工具评测", Status: FieldPending},
					PrimaryChannels: ListField{Values: []string{"xiaohongshu"}, Status: FieldPending},
					WeeklyHours:     HoursField{Value: 6, Status: FieldPending},
				}
			},
			canStart: false,
			missing:  []string{MissingAudience, MissingPillars, MissingChannels, MissingWeeklyHours},
		},
		{
			name:     "nothing at all",
			profile:  func(ExpressionProfile) ExpressionProfile { return ExpressionProfile{} },
			canStart: false,
			missing:  []string{MissingAudience, MissingPillars, MissingChannels, MissingWeeklyHours},
		},
		{
			// The other seven fields are inputs to quality, not gates.
			name: "the four are there and nothing else is",
			profile: func(p ExpressionProfile) ExpressionProfile {
				p.Experience = TextField{}
				p.Positioning = TextField{}
				p.ExpressionStyle = TextField{}
				p.ForbiddenExpressions = TextField{}
				p.ContentGoals = TextField{}
				p.CommonQuestions = TextField{}
				p.StyleSamples = ListField{}
				return p
			},
			canStart: true, missing: []string{},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			readiness := ProfileReadiness(tc.profile(startableProfile()))
			if readiness.CanStart != tc.canStart {
				t.Errorf("canStart = %v, want %v (missing %v)", readiness.CanStart, tc.canStart, readiness.Missing)
			}
			if len(readiness.Missing) != len(tc.missing) {
				t.Fatalf("missing = %v, want %v", readiness.Missing, tc.missing)
			}
			for i, want := range tc.missing {
				if readiness.Missing[i] != want {
					t.Errorf("missing[%d] = %q, want %q", i, readiness.Missing[i], want)
				}
			}
		})
	}
}

// A7. No confirmed sample means neutral expression — and it marks without
// blocking: 3.1 says a missing sample must not stop material being recorded or
// writing being done by hand, so it never appears in the readiness decision.
func TestNeutralExpressionIsMarkedButNeverBlocks(t *testing.T) {
	without := startableProfile()
	if !UsesNeutralExpression(without) {
		t.Error("an account with no style sample was not marked neutral")
	}
	if !ProfileReadiness(without).CanStart {
		t.Error("a missing style sample blocked the start condition")
	}

	with := startableProfile()
	with.StyleSamples = ListField{Values: []string{"一段我写过的开头"}, Status: FieldConfirmed}
	if UsesNeutralExpression(with) {
		t.Error("an account with a confirmed sample was still marked neutral")
	}

	// A sample typed but not confirmed is still pending, so it is still neutral.
	typed := startableProfile()
	typed.StyleSamples = ListField{Values: []string{"草稿"}, Status: FieldPending}
	if !UsesNeutralExpression(typed) {
		t.Error("an unconfirmed sample counted as a style sample")
	}
}

type profileRevisionStore struct {
	current    Revision
	currentErr error
	inserted   []Revision
	nextHook   func(*profileRevisionStore)
}

func (s *profileRevisionStore) NextRevision(context.Context, string, string) (int64, error) {
	if s.nextHook != nil {
		hook := s.nextHook
		s.nextHook = nil
		hook(s)
	}
	if s.current.Revision > 0 {
		return s.current.Revision + 1, nil
	}
	return 1, nil
}

func (s *profileRevisionStore) InsertRevision(_ context.Context, revision Revision) (Revision, error) {
	s.inserted = append(s.inserted, revision)
	s.current = revision
	return revision, nil
}

func (s *profileRevisionStore) GetRevision(context.Context, string, string) (Revision, error) {
	return Revision{}, ErrNotFound
}

func (s *profileRevisionStore) CurrentRevision(context.Context, string, string) (Revision, error) {
	if s.currentErr != nil {
		return Revision{}, s.currentErr
	}
	if s.current.RevisionID == "" {
		return Revision{}, ErrNotFound
	}
	return s.current, nil
}

func (s *profileRevisionStore) ListRevisions(context.Context, string, string) ([]Revision, error) {
	return append([]Revision(nil), s.inserted...), nil
}

func TestSetProfileNormalizesValidBlankContentBeforeValidation(t *testing.T) {
	store := &profileRevisionStore{}
	service := &Service{RevisionStore: store, NewID: func() string { return "rev-profile" }}

	written, err := service.SetProfile(t.Context(), "ws", "actor", "acct", ExpressionProfile{
		Audience:        TextField{Value: "   ", Status: FieldConfirmed},
		PrimaryChannels: ListField{Values: []string{"", "  "}, Status: FieldConfirmed},
	})
	if err != nil {
		t.Fatalf("valid blank content should normalize to pending: %v", err)
	}
	if written.Profile.Audience.Status != FieldPending {
		t.Fatalf("stored blank audience = %+v, want pending", written.Profile.Audience)
	}
	if written.Profile.PrimaryChannels.Status != FieldPending || len(written.Profile.PrimaryChannels.Values) != 0 {
		t.Fatalf("stored channels = %+v, want empty and pending", written.Profile.PrimaryChannels)
	}
}

func TestSetProfileRejectsInvalidRawShapeBeforeNormalization(t *testing.T) {
	for name, profile := range map[string]ExpressionProfile{
		"unknown channel": {
			PrimaryChannels: ListField{Values: []string{"myspace"}, Status: FieldConfirmed},
		},
		"unknown status on nonblank text": {
			Audience: TextField{Value: "designers", Status: "maybe"},
		},
		"unknown status on blank text": {
			Audience: TextField{Value: "   ", Status: "maybe"},
		},
		"unknown status on empty list": {
			PrimaryChannels: ListField{Values: []string{}, Status: "maybe"},
		},
		"unknown status on blank-only list": {
			PrimaryChannels: ListField{Values: []string{"", "  "}, Status: "maybe"},
		},
		"too many blank list entries": {
			StyleSamples: ListField{Values: blankEntries(MaxProfileListEntries + 1), Status: FieldPending},
		},
	} {
		t.Run(name, func(t *testing.T) {
			store := &profileRevisionStore{}
			service := &Service{RevisionStore: store, NewID: func() string { return "rev-profile" }}

			_, err := service.SetProfile(t.Context(), "ws", "actor", "acct", profile)
			if !errors.Is(err, ErrProfile) {
				t.Fatalf("error = %v, want ErrProfile", err)
			}
			if len(store.inserted) != 0 {
				t.Fatalf("invalid profile inserted %d revisions", len(store.inserted))
			}
		})
	}
}

func blankEntries(count int) []string {
	entries := make([]string, count)
	for i := range entries {
		entries[i] = " "
	}
	return entries
}

func TestRevisionWritesCarryTheOtherHalfForward(t *testing.T) {
	original := Revision{
		RevisionID: "rev-1", AccountID: "acct", WorkspaceID: "ws", Revision: 1,
		PersonaPrompt: "original persona",
		Profile:       ExpressionProfile{Audience: confirmed("designers")},
	}
	store := &profileRevisionStore{current: original}
	service := &Service{RevisionStore: store, NewID: func() string { return "rev-next" }}

	profileWrite, err := service.SetProfile(t.Context(), "ws", "actor", "acct",
		ExpressionProfile{ContentPillars: confirmed("tools")})
	if err != nil {
		t.Fatalf("set profile: %v", err)
	}
	if profileWrite.PersonaPrompt != original.PersonaPrompt {
		t.Errorf("profile write carried persona %q, want %q", profileWrite.PersonaPrompt, original.PersonaPrompt)
	}

	promptWrite, err := service.SetPersonaPrompt(t.Context(), "ws", "actor", "acct", "next persona")
	if err != nil {
		t.Fatalf("set prompt: %v", err)
	}
	if promptWrite.Profile.ContentPillars != profileWrite.Profile.ContentPillars {
		t.Errorf("prompt write did not carry profile: got %+v want %+v",
			promptWrite.Profile.ContentPillars, profileWrite.Profile.ContentPillars)
	}
	if original.Profile.Audience != confirmed("designers") || original.PersonaPrompt != "original persona" {
		t.Fatalf("old revision changed: %+v", original)
	}
}

func TestRevisionWritesPropagateHistoryReadErrorsWithoutInserting(t *testing.T) {
	readErr := errors.New("revision storage unavailable")
	for name, write := range map[string]func(*Service) error{
		"profile": func(service *Service) error {
			_, err := service.SetProfile(t.Context(), "ws", "actor", "acct", ExpressionProfile{})
			return err
		},
		"persona": func(service *Service) error {
			_, err := service.SetPersonaPrompt(t.Context(), "ws", "actor", "acct", "persona")
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			store := &profileRevisionStore{currentErr: readErr}
			service := &Service{RevisionStore: store, NewID: func() string { return "rev-next" }}

			if err := write(service); !errors.Is(err, readErr) {
				t.Fatalf("error = %v, want history read error", err)
			}
			if len(store.inserted) != 0 {
				t.Fatalf("history read failure inserted %d revisions", len(store.inserted))
			}
		})
	}
}

func TestRevisionWritesRefreshTheCarriedHalfAfterChoosingTheNextNumber(t *testing.T) {
	latest := Revision{
		RevisionID: "rev-2", AccountID: "acct", WorkspaceID: "ws", Revision: 2,
		PersonaPrompt: "latest persona",
		Profile:       ExpressionProfile{Audience: confirmed("latest audience")},
	}

	t.Run("profile write sees a persona that landed during next-revision lookup", func(t *testing.T) {
		store := &profileRevisionStore{
			current: Revision{RevisionID: "rev-1", Revision: 1, PersonaPrompt: "stale persona"},
			nextHook: func(store *profileRevisionStore) {
				store.current = latest
			},
		}
		service := &Service{RevisionStore: store, NewID: func() string { return "rev-3" }}

		written, err := service.SetProfile(t.Context(), "ws", "actor", "acct", ExpressionProfile{})
		if err != nil {
			t.Fatalf("set profile: %v", err)
		}
		if written.PersonaPrompt != latest.PersonaPrompt {
			t.Fatalf("carried persona = %q, want latest %q", written.PersonaPrompt, latest.PersonaPrompt)
		}
	})

	t.Run("persona write sees a profile that landed during next-revision lookup", func(t *testing.T) {
		store := &profileRevisionStore{
			current: Revision{
				RevisionID: "rev-1", Revision: 1,
				Profile: ExpressionProfile{Audience: confirmed("stale audience")},
			},
			nextHook: func(store *profileRevisionStore) {
				store.current = latest
			},
		}
		service := &Service{RevisionStore: store, NewID: func() string { return "rev-3" }}

		written, err := service.SetPersonaPrompt(t.Context(), "ws", "actor", "acct", "new persona")
		if err != nil {
			t.Fatalf("set persona: %v", err)
		}
		if written.Profile.Audience != latest.Profile.Audience {
			t.Fatalf("carried audience = %+v, want latest %+v",
				written.Profile.Audience, latest.Profile.Audience)
		}
	})
}
