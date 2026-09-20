package feedbacklearning

import (
	"errors"
	"strings"
	"testing"
)

// The controlled sets, the validation and the two derivations. No database:
// these are pure functions, and the matrices belong beside them rather than
// being re-run through a handler.

func TestTheControlledSetsAreExactlyWhatTheSOPGives(t *testing.T) {
	for _, item := range []struct {
		name string
		got  []string
		want []string
	}{
		{"platform", asStrings(Platforms), []string{"xiaohongshu", "wechat_mp", "douyin", "shipinhao"}},
		// SOP 10.1: 曝光、阅读、播放、完播、点赞、评论、收藏、分享、关注、私信、转化.
		{"metric", asStrings(Metrics), []string{
			"impression", "read", "play", "completion", "like", "comment",
			"favorite", "share", "follow", "direct_message", "conversion"}},
		{"metric source", asStrings(MetricSources), []string{"manual", "csv_import"}},
		// SOP 10.1: 评论、私信和线索.
		{"excerpt source", asStrings(ExcerptSources), []string{"comment", "private_message", "lead"}},
		// SOP 7.1's AI review row, all seven.
		{"review state", asStrings(ReviewStates), []string{
			"pending_data", "queued", "generating", "generated", "failed",
			"edited", "superseded"}},
	} {
		if strings.Join(item.got, ",") != strings.Join(item.want, ",") {
			t.Errorf("%s set is %v, SOP gives %v", item.name, item.got, item.want)
		}
	}
	if len(Metrics) != 11 {
		t.Errorf("the metric set has %d values; SOP 10.1 names exactly 11", len(Metrics))
	}
	if len(ExcerptSources) != 3 {
		t.Errorf("the excerpt source set has %d values; SOP 10.1 names exactly 3", len(ExcerptSources))
	}
}

func asStrings[T ~string](values []T) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, string(value))
	}
	return out
}

// The SOP named its values and stopped. An escape hatch turns a controlled set
// into a suggestion, and the first thing anybody puts in it is a name that
// belongs in the list.
func TestNeitherSetHasAnEscapeHatch(t *testing.T) {
	for _, forbidden := range []string{"other", "misc", "unknown", "custom"} {
		if oneOf(forbidden, Metrics) {
			t.Errorf("the metric set contains %q", forbidden)
		}
		if oneOf(forbidden, ExcerptSources) {
			t.Errorf("the excerpt source set contains %q", forbidden)
		}
	}
}

// SOP 10.1: "不同平台的阅读和播放分别保留，不直接合并排名".
func TestReadAndPlayAreTwoSeparateValues(t *testing.T) {
	if MetricRead == MetricPlay {
		t.Fatal("read and play are the same value")
	}
	if !oneOf("read", Metrics) || !oneOf("play", Metrics) {
		t.Fatal("read and play are not both in the set")
	}
}

func TestEachControlledSetRejectsAValueOutsideItAndNamesTheField(t *testing.T) {
	for _, item := range []struct {
		field    string
		validate func(string) error
		outside  string
	}{
		{"platform", ValidatePlatform, "weibo"},
		{"metric", ValidateMetric, "engagement"},
		{"source_type", ValidateMetricSource, "api_sync"},
		{"source_type", ValidateExcerptSource, "review"},
		{"state", ValidateReviewState, "done"},
	} {
		err := item.validate(item.outside)
		if !errors.Is(err, ErrInvalid) {
			t.Errorf("%s accepted %q", item.field, item.outside)
			continue
		}
		var fieldErr FieldError
		if !errors.As(err, &fieldErr) || fieldErr.Field != item.field {
			t.Errorf("%s rejection did not name the field: %v", item.field, err)
		}
	}
}

// SOP 10.1: "未知填空；0 只表示已确认的零值".
//
// This is the assertion the whole card turns on. Anything that treats an
// absent value as zero puts a number nobody observed into every later
// aggregate, and nothing downstream would report it.
func TestAnAbsentValueIsNotZero(t *testing.T) {
	zero := int64(0)
	nine := int64(9)
	if SameValue(nil, &zero) || SameValue(&zero, nil) {
		t.Error("an unknown value compared equal to a confirmed zero")
	}
	if !SameValue(nil, nil) {
		t.Error("two unknowns compared unequal")
	}
	if !SameValue(&zero, &zero) {
		t.Error("two zeroes compared unequal")
	}
	if SameValue(&zero, &nine) {
		t.Error("0 compared equal to 9")
	}
	// And they do not render the same either: a blank cell in a report is read
	// as zero by the next person along.
	if DescribeValue(nil) == DescribeValue(&zero) {
		t.Errorf("unknown and zero both render as %q", DescribeValue(nil))
	}
	if DescribeValue(nil) != "unknown" {
		t.Errorf("an absent value renders as %q, want unknown", DescribeValue(nil))
	}
	if DescribeValue(&zero) != "0" {
		t.Errorf("a confirmed zero renders as %q", DescribeValue(&zero))
	}
}

func validMetricInput() MetricInput {
	value := int64(1200)
	return MetricInput{
		PublicationRecordID: "pub-1", Platform: PlatformXiaohongshu,
		AccountID: "acct-1", Metric: MetricImpression, Value: &value,
		Unit: "次", StatWindow: "发布后 14 天累计",
		SampledAt: "2026-09-20T10:00:00Z",
	}
}

func TestAMetricRowWithNoValueIsStillValid(t *testing.T) {
	input := validMetricInput()
	input.Value = nil
	if err := ValidateMetricInput(input, 0); err != nil {
		t.Fatalf("a row with an unknown value was refused: %v", err)
	}
}

func TestAMetricRowNamesWhateverIsWrongWithIt(t *testing.T) {
	for _, item := range []struct {
		name   string
		broken func(*MetricInput)
		field  string
	}{
		{"no publication record", func(i *MetricInput) { i.PublicationRecordID = "" }, "publication_record_id"},
		{"a platform outside the set", func(i *MetricInput) { i.Platform = "weibo" }, "platform"},
		{"a metric outside the set", func(i *MetricInput) { i.Metric = "engagement" }, "metric"},
		{"no sample time", func(i *MetricInput) { i.SampledAt = "" }, "sampled_at"},
		{"a sample time that is not a timestamp", func(i *MetricInput) { i.SampledAt = "last tuesday" }, "sampled_at"},
		{"an over-long unit", func(i *MetricInput) { i.Unit = strings.Repeat("次", MaxShortRunes+1) }, "unit"},
	} {
		input := validMetricInput()
		item.broken(&input)
		err := ValidateMetricInput(input, 0)
		var fieldErr FieldError
		if !errors.As(err, &fieldErr) {
			t.Errorf("%s: got %v, want a named field", item.name, err)
			continue
		}
		if fieldErr.Field != item.field {
			t.Errorf("%s: named %q, want %q", item.name, fieldErr.Field, item.field)
		}
	}
}

// All or nothing, and the refusal says which line. "Some row is wrong" in a
// paste of forty rows is not a usable answer.
func TestABatchIsRefusedByItsFirstBadRowAndSaysWhichOne(t *testing.T) {
	good := validMetricInput()
	bad := validMetricInput()
	bad.Metric = "engagement"
	err := ValidateMetricBatch([]MetricInput{good, bad, good})
	var fieldErr FieldError
	if !errors.As(err, &fieldErr) {
		t.Fatalf("got %v, want a named field", err)
	}
	if fieldErr.Row != 2 || fieldErr.Field != "metric" {
		t.Errorf("refusal names row %d field %q, want row 2 metric", fieldErr.Row, fieldErr.Field)
	}
	if !strings.Contains(fieldErr.Error(), "row 2") {
		t.Errorf("the message does not say which row: %q", fieldErr.Error())
	}
	if err = ValidateMetricBatch([]MetricInput{good, good, good}); err != nil {
		t.Errorf("an all-good batch was refused: %v", err)
	}
	if err = ValidateMetricBatch(nil); !errors.Is(err, ErrInvalid) {
		t.Error("an empty batch was accepted")
	}
}

func validExcerpt() ExcerptInput {
	return ExcerptInput{
		PublicationRecordID: "pub-1", SourceType: ExcerptComment,
		RedactedExcerpt: "看完就去买了", OccurredAt: "2026-09-20T10:00:00Z",
	}
}

// R-045: "引用摘录与运营者判断分别保存". Either column alone is ordinary; both
// empty records nothing at all.
func TestAnExcerptMayCarryEitherHalfButNotNeither(t *testing.T) {
	quoteOnly := validExcerpt()
	if err := ValidateExcerptInput(quoteOnly); err != nil {
		t.Errorf("a quote with no reading was refused: %v", err)
	}
	readingOnly := validExcerpt()
	readingOnly.RedactedExcerpt = ""
	readingOnly.Interpretation = "转化点在第三段"
	if err := ValidateExcerptInput(readingOnly); err != nil {
		t.Errorf("a reading with no quote was refused: %v", err)
	}
	both := validExcerpt()
	both.Interpretation = "转化点在第三段"
	if err := ValidateExcerptInput(both); err != nil {
		t.Errorf("both halves together were refused: %v", err)
	}
	neither := validExcerpt()
	neither.RedactedExcerpt = ""
	var fieldErr FieldError
	if err := ValidateExcerptInput(neither); !errors.As(err, &fieldErr) {
		t.Error("an excerpt with nothing in it was accepted")
	}
}

func TestAnExcerptNamesWhateverIsWrongWithIt(t *testing.T) {
	for _, item := range []struct {
		name   string
		broken func(*ExcerptInput)
		field  string
	}{
		{"a source outside the set", func(i *ExcerptInput) { i.SourceType = "review" }, "source_type"},
		{"no occurrence time", func(i *ExcerptInput) { i.OccurredAt = "" }, "occurred_at"},
		{"too many tags", func(i *ExcerptInput) {
			i.Tags = make([]string, MaxTags+1)
			for index := range i.Tags {
				i.Tags[index] = "t"
			}
		}, "tags"},
	} {
		input := validExcerpt()
		item.broken(&input)
		err := ValidateExcerptInput(input)
		var fieldErr FieldError
		if !errors.As(err, &fieldErr) || fieldErr.Field != item.field {
			t.Errorf("%s: got %v, want a refusal naming %q", item.name, err, item.field)
		}
	}
}

// Counted in runes. A byte limit gives a Chinese excerpt a third of the room
// an English one gets.
func TestTheExcerptLimitIsCountedInRunes(t *testing.T) {
	atLimit := validExcerpt()
	atLimit.RedactedExcerpt = strings.Repeat("字", MaxNoteRunes)
	if err := ValidateExcerptInput(atLimit); err != nil {
		t.Errorf("an excerpt of exactly %d runes was refused: %v", MaxNoteRunes, err)
	}
	overLimit := validExcerpt()
	overLimit.RedactedExcerpt = strings.Repeat("字", MaxNoteRunes+1)
	if err := ValidateExcerptInput(overLimit); !errors.Is(err, ErrInvalid) {
		t.Error("an excerpt one rune over the limit was accepted")
	}
}

// SOP 2's fifth workbench item, and no time logic anywhere in it.
func TestNeedsRegistrationIsPublishedAndNothingRecorded(t *testing.T) {
	for _, item := range []struct {
		name   string
		status string
		count  int
		want   bool
	}{
		{"reported, nothing recorded", "reported_published", 0, true},
		{"verified, nothing recorded", "verified_published", 0, true},
		{"reported, already recorded", "reported_published", 1, false},
		{"failed", "failed", 0, false},
		{"removed", "removed", 0, false},
		{"unknown", "unknown", 0, false},
		{"a status this build has not heard of", "escalated", 0, false},
	} {
		if got := NeedsRegistration(item.status, item.count); got != item.want {
			t.Errorf("%s: got %v, want %v", item.name, got, item.want)
		}
	}
}

// This card produces exactly one state. The other six are reserved so EP-08
// does not have to widen the set, which would mean revalidating every stored
// value.
func TestTheReviewPlaceholderIsAlwaysPendingData(t *testing.T) {
	if got := ReviewStateFor(0); got != StatePendingData {
		t.Errorf("with no report the state is %q, want pending_data", got)
	}
	if len(ReviewStates) != 7 {
		t.Errorf("the review state set has %d values; SOP 7.1 gives 7", len(ReviewStates))
	}
}
