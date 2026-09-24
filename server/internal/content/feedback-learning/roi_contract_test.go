package feedbacklearning

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"testing"
)

// specs/034 PR 1: controlled sets (SC-014, FR-075), the platform table
// against ip-profile (T010) and FR-019's field-combination table (T013).

func setValues[T ~string](values []T) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, string(value))
	}
	return out
}

// Each set has exactly the values the contract gives it, in order, and no
// escape hatch. The evidence types are the one R-061 is most emphatic about:
// six, and "other" is not one of them.
func TestROIControlledSetsAreExactlyTheContracts(t *testing.T) {
	cases := []struct {
		name string
		got  []string
		want []string
	}{
		{"evidence_type", setValues(EvidenceTypes), []string{
			"platform_linked_content", "content_comment", "customer_statement",
			"dedicated_channel", "account_only", "unknown"}},
		{"pricing", setValues(Pricings), []string{"amount", "labor_time"}},
		{"role", setValues(TouchRoles), []string{"first_touch", "pre_booking", "other"}},
		{"gross_basis", setValues(GrossBases), []string{"none", "stated_gross_profit", "cogs"}},
		{"kind", setValues(AdjustmentKinds), []string{"refund", "adjustment"}},
		{"source_type", setValues(RecordSources), []string{"manual", "import"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if !slices.Equal(tc.got, tc.want) {
				t.Fatalf("%s = %v, want exactly %v", tc.name, tc.got, tc.want)
			}
			for _, escape := range []string{"other_source", "misc", "custom", "unspecified"} {
				if slices.Contains(tc.got, escape) {
					t.Errorf("%s has an escape hatch %q", tc.name, escape)
				}
			}
		})
	}
	if len(EvidenceTypes) != 6 || slices.Contains(setValues(EvidenceTypes), "other") {
		t.Fatalf("evidence types must be exactly six with no other, got %v", EvidenceTypes)
	}
}

// One value outside each set, and the refusal names the field it came in.
func TestAValueOutsideEachROISetIsRefusedByName(t *testing.T) {
	amount := "10.00"
	now := "2026-09-01T10:00:00Z"
	cases := []struct {
		field string
		err   error
	}{
		{"pricing", func() error {
			_, err := ValidateCost(CostInput{Category: "拍摄", Pricing: "fixed", Amount: &amount, Currency: "CNY", IncurredAt: now})
			return err
		}()},
		{"currency", func() error {
			_, err := ValidateCost(CostInput{Category: "拍摄", Pricing: "amount", Amount: &amount, Currency: "RMB", IncurredAt: now})
			return err
		}()},
		{"evidence_type", func() error {
			_, err := ValidateTouch(TouchInput{EvidenceType: "other", Role: "other", OccurredAt: now})
			return err
		}()},
		{"role", func() error {
			_, err := ValidateTouch(TouchInput{EvidenceType: "unknown", Role: "last_touch", OccurredAt: now})
			return err
		}()},
		{"platform", func() error {
			_, err := ValidateTouch(TouchInput{EvidenceType: "customer_statement", Platform: "tiktok", Role: "other", OccurredAt: now})
			return err
		}()},
		{"gross_basis", func() error {
			_, err := ValidateDeal(DealInput{Amount: &amount, Currency: "CNY", ClosedAt: now, GrossBasis: "net"})
			return err
		}()},
		{"kind", func() error {
			_, err := ValidateAdjustment(AdjustmentInput{Kind: "chargeback", Amount: &amount, Currency: "CNY", OccurredAt: now})
			return err
		}()},
	}
	for _, tc := range cases {
		t.Run(tc.field, func(t *testing.T) {
			assertField(t, tc.err, tc.field)
		})
	}
}

func assertField(t *testing.T, err error, field string) {
	t.Helper()
	fieldErr, ok := errors.AsType[FieldError](err)
	if !ok {
		t.Fatalf("err = %v, want a FieldError naming %q", err, field)
	}
	if fieldErr.Field != field {
		t.Fatalf("refusal named %q, want %q (%v)", fieldErr.Field, field, err)
	}
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("a field error must still be ErrInvalid: %v", err)
	}
}

// T010: the touch platforms are ip-profile's eight, word for word and in its
// order. Read from the Go source, not imported: feedback-learning does not
// depend on ip-profile.
func TestTouchPlatformsAreIPProfilesEight(t *testing.T) {
	source, err := os.ReadFile(filepath.Join(moduleDir(t), "..", "ip-profile", "account.go"))
	if err != nil {
		t.Fatal(err)
	}
	declaration := regexp.MustCompile(`Platform[A-Za-z]* +Platform += +"([a-z_]+)"`)
	var theirs []string
	for _, match := range declaration.FindAllStringSubmatch(string(source), -1) {
		theirs = append(theirs, match[1])
	}
	if len(theirs) != 8 {
		t.Fatalf("read %d platforms out of ip-profile, want 8: %v", len(theirs), theirs)
	}
	if ours := setValues(TouchPlatforms); !slices.Equal(ours, theirs) {
		t.Fatalf("touch platforms %v, ip-profile has %v", ours, theirs)
	}
}

// T013: FR-019, every cell. For each evidence type, each of the three fields
// (work/publication, account, platform) set and unset, and the refusal - when
// there is one - names that field.
func TestTheEvidenceFieldTableHoldsCellByCell(t *testing.T) {
	type fields struct{ platform, account, work, publication string }
	full := fields{"douyin", "acct-1", "work-1", ""}
	cases := []struct {
		name     string
		evidence EvidenceType
		in       fields
		field    string // "" means accepted
	}{
		// platform_linked_content: work/publication at least one, account optional, platform required.
		{"linked/all", EvidencePlatformLinkedContent, full, ""},
		{"linked/publication-only", EvidencePlatformLinkedContent, fields{"douyin", "", "", "pub-1"}, ""},
		{"linked/no-work", EvidencePlatformLinkedContent, fields{"douyin", "acct-1", "", ""}, "work_id"},
		{"linked/no-account", EvidencePlatformLinkedContent, fields{"douyin", "", "work-1", ""}, ""},
		{"linked/no-platform", EvidencePlatformLinkedContent, fields{"", "acct-1", "work-1", ""}, "platform"},
		// content_comment: same row as linked content.
		{"comment/all", EvidenceContentComment, full, ""},
		{"comment/no-work", EvidenceContentComment, fields{"douyin", "acct-1", "", ""}, "work_id"},
		{"comment/no-account", EvidenceContentComment, fields{"douyin", "", "work-1", ""}, ""},
		{"comment/no-platform", EvidenceContentComment, fields{"", "acct-1", "work-1", ""}, "platform"},
		// customer_statement: everything optional - a customer may say only
		// "saw you somewhere".
		{"statement/all", EvidenceCustomerStatement, full, ""},
		{"statement/no-work", EvidenceCustomerStatement, fields{"douyin", "acct-1", "", ""}, ""},
		{"statement/no-account", EvidenceCustomerStatement, fields{"douyin", "", "", ""}, ""},
		{"statement/nothing", EvidenceCustomerStatement, fields{}, ""},
		// dedicated_channel: everything optional.
		{"channel/all", EvidenceDedicatedChannel, full, ""},
		{"channel/no-work", EvidenceDedicatedChannel, fields{"douyin", "acct-1", "", ""}, ""},
		{"channel/no-account", EvidenceDedicatedChannel, fields{"douyin", "", "", ""}, ""},
		{"channel/nothing", EvidenceDedicatedChannel, fields{}, ""},
		// account_only: work must be empty, account and platform required.
		{"account-only/ok", EvidenceAccountOnly, fields{"douyin", "acct-1", "", ""}, ""},
		{"account-only/work", EvidenceAccountOnly, full, "work_id"},
		{"account-only/publication", EvidenceAccountOnly, fields{"douyin", "acct-1", "", "pub-1"}, "publication_record_id"},
		{"account-only/no-account", EvidenceAccountOnly, fields{"douyin", "", "", ""}, "account_id"},
		{"account-only/no-platform", EvidenceAccountOnly, fields{"", "acct-1", "", ""}, "platform"},
		// unknown: all three must be empty.
		{"unknown/ok", EvidenceUnknown, fields{}, ""},
		{"unknown/platform", EvidenceUnknown, fields{"douyin", "", "", ""}, "platform"},
		{"unknown/account", EvidenceUnknown, fields{"", "acct-1", "", ""}, "account_id"},
		{"unknown/work", EvidenceUnknown, fields{"", "", "work-1", ""}, "work_id"},
		{"unknown/publication", EvidenceUnknown, fields{"", "", "", "pub-1"}, "publication_record_id"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			touch, err := ValidateTouch(TouchInput{
				EvidenceType: string(tc.evidence), Platform: tc.in.platform,
				AccountID: tc.in.account, WorkID: tc.in.work, PublicationRecordID: tc.in.publication,
				Role: "first_touch", OccurredAt: "2026-09-01T10:00:00Z",
			})
			if tc.field == "" {
				if err != nil {
					t.Fatalf("refused a legal combination: %v", err)
				}
				// Accepted as given: nothing fills a work in, nothing
				// changes the evidence type (FR-021).
				if touch.WorkID != tc.in.work || touch.EvidenceType != tc.evidence {
					t.Fatalf("stored %q/%q, sent %q/%q", touch.WorkID, touch.EvidenceType, tc.in.work, tc.evidence)
				}
				return
			}
			assertField(t, err, tc.field)
		})
	}
}

// FR-012: a cost has no "counts as cost of goods sold" field; cost of goods
// lives only in a deal's gross basis.
func TestACostHasNoCostOfGoodsField(t *testing.T) {
	for _, forbidden := range []string{"cogs", "cost_of_goods", "sales_cost", "counts_as"} {
		for _, tag := range jsonTags(CostRevision{}) {
			if tag == forbidden {
				t.Errorf("CostRevision has %q", tag)
			}
		}
		for _, tag := range jsonTags(CostInput{}) {
			if tag == forbidden {
				t.Errorf("CostInput has %q", tag)
			}
		}
	}
}
