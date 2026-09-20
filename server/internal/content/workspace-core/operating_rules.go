package workspacecore

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"unicode/utf8"
)

// SOP 3.2's operating rules: what a brand has decided about its own cadence,
// its channel templates, who reviews, and when to go and look at the numbers.
//
// All four live in ONE key inside the workspace's settings JSON rather than in
// a table. That is the shape LT-009's timezone and LT-015's precheck switch
// already use, and the ruling on this card (Q1=A) kept it: no migration, no
// new table, nothing to add to the deletion chain - the settings go when the
// workspace row goes, because they are on it.
//
// One key and not four: the four are filled in on one form in one sitting, and
// splitting them would mean defending "a partial write must not wipe the rest"
// four times instead of once.
//
// The thing to get right in this file: a number that was never entered and a
// number that is zero are DIFFERENT. A cadence of 0 means "nothing goes out on
// this channel this week"; an absent cadence means nobody has decided yet. An
// observation window of 0 means "look the same day"; absent means there is no
// answer to "is it due". Every reader below returns two values for this
// reason, and 019 is why: `hasStoredAutoPrecheck` exists as a separate
// function because `false` is a boolean's zero value, and answering "has this
// been chosen" by truthiness turned "switched off" into "never chose".
//
// Contract: specs/029-operating-rules/contracts/operating-rules.md

// OperatingRulesKey is where the four settings live inside the workspace's
// settings JSONB. The "loretide." prefix keeps them clear of any upstream key,
// the same way loretide.timezone and loretide.auto_precheck do.
const OperatingRulesKey = "loretide.operating_rules"

// HomepageKey is where an account's public page link lives inside the
// ACCOUNT's settings JSONB - a different row and a different owner from the
// key above. SOP 3.2 asks for "账号名称或主页链接以便标识": the name is
// content_account.display_name already, and this is the other half.
const HomepageKey = "loretide.homepage"

// Platforms is SOP 3.2's channel set, and it is ip-profile's eight rather than
// review-delivery's four (ruling Q2=A): a brand that can open a Zhihu account
// should be able to write down how it posts there, even though delivery does
// not reach Zhihu yet.
//
// Restated here rather than imported: this module's declared dependencies are
// diagnostics and nothing else. A guard test reads ip-profile's source and
// holds these against it, the same way review-delivery and feedback-learning
// do for their own sets.
var Platforms = []string{
	"xiaohongshu", "douyin", "wechat_mp", "bilibili",
	"zhihu", "weibo", "kuaishou", "shipinhao",
}

// ReviewRules is exactly one value. SOP 3.2: "个人默认自己审核。后续团队模式
// 可要求不同成员复核。" The second sentence is explicitly later, and putting a
// second value here today would promise something nothing implements.
var ReviewRules = []string{"self"}

// ReviewRuleSelf is the default and, in this phase, the only value.
const ReviewRuleSelf = "self"

// MaxNoteRunes bounds a channel template note. Counted in runes, not bytes:
// counting bytes gives Chinese a third of the room English gets under the same
// number.
const MaxNoteRunes = 20000

// MaxHomepageRunes bounds a homepage link.
const MaxHomepageRunes = 2000

var (
	// ErrInvalid is a 400. It always carries which field was wrong.
	ErrInvalid = errors.New("invalid operating rules")
	// ErrNotFound is the 404 a refused or missing workspace produces. It is
	// deliberately the same for both.
	ErrNotFound = errors.New("not found")
	// ErrStorage is a 500.
	ErrStorage = errors.New("storage unavailable")
)

// FieldError names the field that was wrong, so a refusal can point at a box
// on the form instead of saying "invalid".
type FieldError struct {
	Field  string
	Reason string
}

func (e FieldError) Error() string {
	if e.Reason == "" {
		return fmt.Sprintf("%s is invalid", e.Field)
	}
	return fmt.Sprintf("%s: %s", e.Field, e.Reason)
}

func (e FieldError) Unwrap() error { return ErrInvalid }

func invalidField(field string) error { return FieldError{Field: field} }

// ChannelTemplate is what a brand has written down about posting on one
// channel. Free text on purpose (ruling accepted this): Xiaohongshu's
// constraint is "title under 20 characters, first image 3:4" and a WeChat
// article's is "must have a summary" - those do not decompose into one set of
// fields, and forcing them to would make people fill in boxes they have no
// answer for.
type ChannelTemplate struct {
	Note string `json:"note"`
}

// Observation is SOP 3.2's 反馈观察时点: how long after publishing to go and
// copy the numbers down.
//
// Default is a pointer because absent and zero are different answers, and
// by_channel overrides it per channel.
type Observation struct {
	Default   *int64           `json:"default,omitempty"`
	ByChannel map[string]int64 `json:"by_channel,omitempty"`
}

// Rules is the whole of SOP 3.2 that this card stores.
//
// Cadence and ByChannel are maps rather than structs with eight fields for one
// reason: an absent key is how "nobody decided" is expressed, and a struct of
// int64 would have no way to say it.
type Rules struct {
	Cadence     map[string]int64           `json:"cadence,omitempty"`
	Templates   map[string]ChannelTemplate `json:"templates,omitempty"`
	ReviewRule  string                     `json:"review_rule,omitempty"`
	Observation Observation                `json:"observation"`
}

// DefaultRules is what a brand that has never set anything reads as.
//
// Applied on the way OUT only, like timezoneFilled and autoPrecheckFilled:
// reading a workspace must never write to it. Note what is NOT filled in -
// there is no default cadence and no default observation window. Inventing
// either would be a rule the SOP never stated, sitting where no operator can
// see or change it.
func DefaultRules() Rules {
	return Rules{
		Cadence:     map[string]int64{},
		Templates:   map[string]ChannelTemplate{},
		ReviewRule:  ReviewRuleSelf,
		Observation: Observation{ByChannel: map[string]int64{}},
	}
}

// RulesFrom reads the rules out of a settings blob, filling defaults.
//
// Everything unreadable degrades to the defaults rather than to an error: a
// brand that predates this card, a settings column that is null, and a blob
// written by code that did not know this key all have to render a page.
func RulesFrom(settings any) Rules {
	rules := DefaultRules()
	blob, ok := settingsObject(settings)
	if !ok {
		return rules
	}
	raw, present := blob[OperatingRulesKey]
	if !present {
		return rules
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return rules
	}
	var stored Rules
	if err := json.Unmarshal(encoded, &stored); err != nil {
		return rules
	}
	if stored.Cadence != nil {
		rules.Cadence = stored.Cadence
	}
	if stored.Templates != nil {
		rules.Templates = stored.Templates
	}
	if stored.ReviewRule != "" {
		rules.ReviewRule = stored.ReviewRule
	}
	rules.Observation.Default = stored.Observation.Default
	if stored.Observation.ByChannel != nil {
		rules.Observation.ByChannel = stored.Observation.ByChannel
	}
	return rules
}

// settingsObject narrows a settings blob to something indexable. Arrays are
// excluded deliberately: typeof [] is object, and an array would otherwise
// index as nothing rather than be recognised as unusable.
func settingsObject(settings any) (map[string]any, bool) {
	blob, ok := settings.(map[string]any)
	return blob, ok
}

// ReadCadence answers how many pieces a week this channel is meant to get.
//
// TWO return values, not a *int64. A pointer can express "nobody decided", but
// forgetting to check it is either a panic or - worse - a silent zero that
// reads as a real decision. A second return value is at least conspicuous.
func ReadCadence(rules Rules, platform string) (int64, bool) {
	value, ok := rules.Cadence[platform]
	return value, ok
}

// ObservationSource says WHERE an observation window came from. Three values
// and not a bool: "this channel is set to 7 days" and "this channel has none,
// so the brand-wide 14 applies" are different things, and the page has to be
// able to say which.
type ObservationSource string

const (
	ObservationFromChannel ObservationSource = "channel"
	ObservationFromGlobal  ObservationSource = "global"
	ObservationUnset       ObservationSource = "none"
)

// ReadObservation answers how long after publishing to look, for one channel.
func ReadObservation(rules Rules, platform string) (int64, ObservationSource) {
	if days, ok := rules.Observation.ByChannel[platform]; ok {
		return days, ObservationFromChannel
	}
	if rules.Observation.Default != nil {
		return *rules.Observation.Default, ObservationFromGlobal
	}
	return 0, ObservationUnset
}

// TemplateNoteFor is the ONLY thing from this card that may enter a model's
// context.
//
// SOP 3.2: "模型上下文仅接收必要的渠道说明". Not the account name, not the
// homepage link, not the cadence, not the observation window, not the review
// rule. It is a function rather than a field read at the call site so there is
// one place this boundary is drawn and one place to test it.
func TemplateNoteFor(rules Rules, platform string) string {
	return rules.Templates[platform].Note
}

// ValidateRules checks a whole set before it is written, naming the first
// field that is wrong.
func ValidateRules(rules Rules) error {
	for platform, count := range rules.Cadence {
		if !oneOfString(platform, Platforms) {
			return FieldError{Field: "cadence", Reason: "unknown channel " + platform}
		}
		if count < 0 {
			return FieldError{Field: "cadence." + platform, Reason: "must not be negative"}
		}
	}
	for platform, template := range rules.Templates {
		if !oneOfString(platform, Platforms) {
			return FieldError{Field: "templates", Reason: "unknown channel " + platform}
		}
		if utf8.RuneCountInString(template.Note) > MaxNoteRunes {
			return FieldError{Field: "templates." + platform, Reason: "too long"}
		}
	}
	if rules.ReviewRule != "" && !oneOfString(rules.ReviewRule, ReviewRules) {
		return invalidField("review_rule")
	}
	if rules.Observation.Default != nil && *rules.Observation.Default < 0 {
		return FieldError{Field: "observation.default", Reason: "must not be negative"}
	}
	for platform, days := range rules.Observation.ByChannel {
		if !oneOfString(platform, Platforms) {
			return FieldError{Field: "observation.by_channel", Reason: "unknown channel " + platform}
		}
		if days < 0 {
			return FieldError{Field: "observation.by_channel." + platform, Reason: "must not be negative"}
		}
	}
	return nil
}

// ValidateHomepage checks an account's public page link.
//
// Only http and https. The link is STORED and never followed - nothing in this
// module fetches it - so the check is about what a person will click, not
// about what a server would request.
func ValidateHomepage(link string) error {
	if link == "" {
		return nil
	}
	if utf8.RuneCountInString(link) > MaxHomepageRunes {
		return FieldError{Field: "homepage", Reason: "too long"}
	}
	parsed, err := url.Parse(link)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return FieldError{Field: "homepage", Reason: "must be an http or https link"}
	}
	return nil
}

// HomepageFrom reads an account's link out of its settings blob.
//
// Two return values for the same reason ReadCadence has two: "" is a link
// somebody cleared and a link nobody ever entered, and a caller that wants to
// tell them apart has to be able to.
func HomepageFrom(settings any) (string, bool) {
	blob, ok := settingsObject(settings)
	if !ok {
		return "", false
	}
	raw, present := blob[HomepageKey]
	if !present {
		return "", false
	}
	link, isString := raw.(string)
	if !isString {
		return "", false
	}
	return link, true
}

func oneOfString(value string, allowed []string) bool {
	for _, candidate := range allowed {
		if candidate == value {
			return true
		}
	}
	return false
}
