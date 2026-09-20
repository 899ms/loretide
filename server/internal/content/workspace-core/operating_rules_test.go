package workspacecore

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// The rules a page asks about before it enables a button, and the one thing
// this card cannot get wrong: a number nobody entered is not zero.

func days(value int64) *int64 { return &value }

func TestTheControlledSetsAreExactlyWhatTheSOPNames(t *testing.T) {
	if len(Platforms) != 8 {
		t.Fatalf("Platforms has %d entries, want 8", len(Platforms))
	}
	// SOP 3.2 says "个人默认自己审核。后续团队模式可要求不同成员复核。" - the
	// second sentence is later work, and a second value here today would
	// promise something nothing implements.
	if len(ReviewRules) != 1 || ReviewRules[0] != ReviewRuleSelf {
		t.Fatalf("ReviewRules = %v, want exactly [self]", ReviewRules)
	}
	for _, forbidden := range []string{"other", "misc", "custom", "team"} {
		for _, platform := range Platforms {
			if platform == forbidden {
				t.Errorf("Platforms contains an escape hatch %q", forbidden)
			}
		}
		for _, rule := range ReviewRules {
			if rule == forbidden {
				t.Errorf("ReviewRules contains an escape hatch %q", forbidden)
			}
		}
	}
}

func TestDefaultsFillWhatTheSOPGivesAndNothingElse(t *testing.T) {
	rules := DefaultRules()
	if rules.ReviewRule != ReviewRuleSelf {
		t.Errorf("review rule default = %q, want %q", rules.ReviewRule, ReviewRuleSelf)
	}
	// No default cadence and no default observation window. Inventing either
	// would be a rule the SOP never stated (FR-024).
	if len(rules.Cadence) != 0 {
		t.Errorf("a brand that set nothing has a cadence: %v", rules.Cadence)
	}
	if rules.Observation.Default != nil {
		t.Errorf("a brand that set nothing has an observation window: %v", *rules.Observation.Default)
	}
}

// The single silent-corruption risk on this card, and the reason every reader
// returns two values. 019 hit the same thing on a boolean.
func TestUnsetIsNotZero(t *testing.T) {
	rules := DefaultRules()
	rules.Cadence["xiaohongshu"] = 0

	zero, storedZero := ReadCadence(rules, "xiaohongshu")
	_, storedAbsent := ReadCadence(rules, "douyin")

	if zero != 0 || !storedZero {
		t.Errorf("a stored 0 read back as (%d, %v), want (0, true)", zero, storedZero)
	}
	if storedAbsent {
		t.Error("a channel nobody set reads as stored; 'nothing goes out' and 'nobody decided' are different")
	}
	if storedZero == storedAbsent {
		t.Error("a stored 0 is indistinguishable from an absent value")
	}
}

func TestObservationSourceIsThreeWayNotBoolean(t *testing.T) {
	rules := DefaultRules()
	rules.Observation.Default = days(14)
	rules.Observation.ByChannel["douyin"] = 7

	for _, tc := range []struct {
		name     string
		platform string
		want     int64
		source   ObservationSource
	}{
		{"the channel set its own", "douyin", 7, ObservationFromChannel},
		{"the channel falls back to the brand", "xiaohongshu", 14, ObservationFromGlobal},
	} {
		t.Run(tc.name, func(t *testing.T) {
			value, source := ReadObservation(rules, tc.platform)
			if value != tc.want || source != tc.source {
				t.Errorf("= (%d, %q), want (%d, %q)", value, source, tc.want, tc.source)
			}
		})
	}

	// Nothing set anywhere: not 0-from-global, not a silent zero.
	bare := DefaultRules()
	if _, source := ReadObservation(bare, "xiaohongshu"); source != ObservationUnset {
		t.Errorf("source with nothing set = %q, want %q", source, ObservationUnset)
	}
}

func TestAStoredZeroObservationIsARealWindow(t *testing.T) {
	rules := DefaultRules()
	rules.Observation.Default = days(0)
	value, source := ReadObservation(rules, "xiaohongshu")
	if value != 0 || source != ObservationFromGlobal {
		t.Errorf("= (%d, %q), want (0, global) - 'look the same day' is a decision", value, source)
	}
}

func TestValidateNamesTheFieldThatIsWrong(t *testing.T) {
	for _, tc := range []struct {
		name  string
		rules func(Rules) Rules
		field string
	}{
		{"a negative cadence", func(r Rules) Rules {
			r.Cadence["xiaohongshu"] = -1
			return r
		}, "cadence.xiaohongshu"},
		{"a channel outside the eight", func(r Rules) Rules {
			r.Cadence["twitter"] = 3
			return r
		}, "cadence"},
		{"a review rule that is not self", func(r Rules) Rules {
			r.ReviewRule = "team"
			return r
		}, "review_rule"},
		{"a negative global window", func(r Rules) Rules {
			r.Observation.Default = days(-3)
			return r
		}, "observation.default"},
		{"a negative channel window", func(r Rules) Rules {
			r.Observation.ByChannel["douyin"] = -1
			return r
		}, "observation.by_channel.douyin"},
		{"a template on an unknown channel", func(r Rules) Rules {
			r.Templates["twitter"] = ChannelTemplate{Note: "x"}
			return r
		}, "templates"},
		{"a note past the rune limit", func(r Rules) Rules {
			r.Templates["zhihu"] = ChannelTemplate{Note: strings.Repeat("字", MaxNoteRunes+1)}
			return r
		}, "templates.zhihu"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateRules(tc.rules(DefaultRules()))
			var field FieldError
			if !errors.As(err, &field) {
				t.Fatalf("err = %v, want a FieldError", err)
			}
			if field.Field != tc.field {
				t.Errorf("named %q, want %q", field.Field, tc.field)
			}
			if !errors.Is(err, ErrInvalid) {
				t.Error("does not unwrap to ErrInvalid, so the handler cannot map it to 400")
			}
		})
	}
}

func TestValidateAcceptsWhatTheSOPAllows(t *testing.T) {
	rules := DefaultRules()
	// 0 is legal on both: "nothing goes out this week" and "look the same day".
	rules.Cadence["xiaohongshu"] = 0
	rules.Observation.Default = days(0)
	// A note exactly at the limit, counted in runes.
	rules.Templates["zhihu"] = ChannelTemplate{Note: strings.Repeat("字", MaxNoteRunes)}
	// A template on a channel delivery cannot reach today is fine (FR-012a).
	rules.Templates["bilibili"] = ChannelTemplate{Note: "长视频，封面 16:9"}
	if err := ValidateRules(rules); err != nil {
		t.Fatalf("ValidateRules = %v, want nil", err)
	}
}

// FR-012a: eight channels can hold a template; four of them cannot be
// delivered to yet. That is a fact the page has to say out loud, not something
// storage should refuse.
func TestATemplateMayLiveOnAChannelDeliveryCannotReach(t *testing.T) {
	undeliverable := []string{"bilibili", "zhihu", "weibo", "kuaishou"}
	rules := DefaultRules()
	for _, platform := range undeliverable {
		rules.Templates[platform] = ChannelTemplate{Note: "写法"}
	}
	if err := ValidateRules(rules); err != nil {
		t.Fatalf("refused a template on a channel that cannot be delivered to: %v", err)
	}
	for _, platform := range undeliverable {
		if TemplateNoteFor(rules, platform) == "" {
			t.Errorf("%s lost its note", platform)
		}
	}
}

func TestRulesSurviveARoundTripThroughTheSettingsBlob(t *testing.T) {
	rules := DefaultRules()
	rules.Cadence["xiaohongshu"] = 3
	rules.Cadence["wechat_mp"] = 0
	rules.Templates["xiaohongshu"] = ChannelTemplate{Note: "标题 20 字内"}
	rules.Observation.Default = days(14)
	rules.Observation.ByChannel["douyin"] = 7

	encoded, err := json.Marshal(rules)
	if err != nil {
		t.Fatal(err)
	}
	var stored any
	if err := json.Unmarshal(encoded, &stored); err != nil {
		t.Fatal(err)
	}
	read := RulesFrom(map[string]any{OperatingRulesKey: stored})

	if value, ok := ReadCadence(read, "wechat_mp"); value != 0 || !ok {
		t.Errorf("a stored 0 did not survive the round trip: (%d, %v)", value, ok)
	}
	if _, ok := ReadCadence(read, "douyin"); ok {
		t.Error("an absent cadence came back as stored")
	}
	if value, source := ReadObservation(read, "douyin"); value != 7 || source != ObservationFromChannel {
		t.Errorf("channel window = (%d, %q)", value, source)
	}
	if TemplateNoteFor(read, "xiaohongshu") != "标题 20 字内" {
		t.Error("the note did not survive")
	}
}

func TestAnUnreadableBlobDegradesToDefaults(t *testing.T) {
	// Every one of these is reachable from a real row: a brand that predates
	// this card, a null settings column, and a blob written by something that
	// did not know the key.
	for _, settings := range []any{nil, "not an object", []any{1, 2}, map[string]any{}, map[string]any{OperatingRulesKey: 7}} {
		rules := RulesFrom(settings)
		if rules.ReviewRule != ReviewRuleSelf {
			t.Errorf("settings %v degraded to review rule %q", settings, rules.ReviewRule)
		}
		if rules.Observation.Default != nil {
			t.Errorf("settings %v invented an observation window", settings)
		}
	}
}

func TestHomepageOnlyTakesWebLinks(t *testing.T) {
	for _, link := range []string{
		"https://www.xiaohongshu.com/user/profile/x",
		"http://example.com/me",
		"",
	} {
		if err := ValidateHomepage(link); err != nil {
			t.Errorf("ValidateHomepage(%q) = %v, want nil", link, err)
		}
	}
	// Not a fetch concern - nothing here follows the link - but a person will
	// click it, and these are what a click should never do.
	for _, link := range []string{
		"javascript:alert(1)",
		"file:///etc/passwd",
		"data:text/html,<script>",
		"ftp://example.com",
		"www.example.com",
	} {
		var field FieldError
		err := ValidateHomepage(link)
		if !errors.As(err, &field) || field.Field != "homepage" {
			t.Errorf("ValidateHomepage(%q) = %v, want a homepage FieldError", link, err)
		}
	}
}

func TestHomepageReadDistinguishesClearedFromNeverSet(t *testing.T) {
	cleared, storedCleared := HomepageFrom(map[string]any{HomepageKey: ""})
	_, storedNever := HomepageFrom(map[string]any{})
	if cleared != "" || !storedCleared {
		t.Errorf("a cleared link read as (%q, %v), want (\"\", true)", cleared, storedCleared)
	}
	if storedNever {
		t.Error("a link nobody entered reads as stored")
	}
}

// SOP 3.2: "模型上下文仅接收必要的渠道说明". The note, and nothing standing
// next to it.
func TestOnlyTheChannelNoteCanReachAModel(t *testing.T) {
	rules := DefaultRules()
	rules.Cadence["xiaohongshu"] = 3
	rules.Templates["xiaohongshu"] = ChannelTemplate{Note: "标题 20 字内"}
	rules.Observation.Default = days(14)
	rules.ReviewRule = ReviewRuleSelf

	note := TemplateNoteFor(rules, "xiaohongshu")
	if note != "标题 20 字内" {
		t.Fatalf("note = %q", note)
	}
	for _, leaked := range []string{"3", "14", ReviewRuleSelf, "cadence", "observation"} {
		if strings.Contains(note, leaked) {
			t.Errorf("the model context carries %q, which is not a channel note", leaked)
		}
	}
}
