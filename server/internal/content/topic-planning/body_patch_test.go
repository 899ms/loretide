package topicplanning

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"
)

func TestTopicBodyPatchPreservesEachOmittedFieldAndClearsEachSuppliedField(t *testing.T) {
	tests := []struct {
		name  string
		get   func(TopicBodyPatch) PatchString
	}{
		{"audience_problem_judgment", func(p TopicBodyPatch) PatchString { return p.AudienceProblemJudgment }},
		{"ip_fit", func(p TopicBodyPatch) PatchString { return p.IPFit }},
		{"timing", func(p TopicBodyPatch) PatchString { return p.Timing }},
		{"existing_content_relation", func(p TopicBodyPatch) PatchString { return p.ExistingContentRelation }},
		{"evidence_gaps_and_investment", func(p TopicBodyPatch) PatchString { return p.EvidenceGapsAndInvestment }},
	}
	for _, test := range tests {
		for _, value := range []string{"edited", ""} {
			caseName := "edit"
			if value == "" {
				caseName = "clear"
			}
			t.Run(test.name+"/"+caseName, func(t *testing.T) {
				input := fmt.Sprintf(`{%q:%q}`, test.name, value)
				var patch TopicBodyPatch
				if err := json.Unmarshal([]byte(input), &patch); err != nil {
					t.Fatalf("decode patch: %v", err)
				}
				if err := patch.Validate(); err != nil {
					t.Fatalf("validate patch: %v", err)
				}
				if got := test.get(patch); !got.Set || got.Value != value {
					t.Fatalf("%s = %#v, want explicit %q", test.name, got, value)
				}
				set := 0
				for _, field := range []PatchString{
					patch.AudienceProblemJudgment, patch.IPFit, patch.Timing,
					patch.ExistingContentRelation, patch.EvidenceGapsAndInvestment,
				} {
					if field.Set {
						set++
					}
				}
				if set != 1 {
					t.Fatalf("set fields = %d, want exactly 1", set)
				}
			})
		}
	}
}

func TestTopicBodyPatchRejectsNullNonStringUnknownAndEmptyRequests(t *testing.T) {
	tests := map[string]string{
		"null":          `{"ip_fit":null}`,
		"non-string":    `{"timing":12}`,
		"unknown":       `{"channels":["xiaohongshu"]}`,
		"empty":         `{}`,
	}
	for name, input := range tests {
		t.Run(name, func(t *testing.T) {
			var patch TopicBodyPatch
			err := json.Unmarshal([]byte(input), &patch)
			if err == nil {
				err = patch.Validate()
			}
			if !errors.Is(err, ErrInvalid) {
				t.Fatalf("input %s error = %v, want ErrInvalid", input, err)
			}
		})
	}
}

func TestTopicBodyPatchRejectsTrailingJSONValue(t *testing.T) {
	var patch TopicBodyPatch
	if err := patch.UnmarshalJSON([]byte(`{"ip_fit":"fit"} {}`)); !errors.Is(err, ErrInvalid) {
		t.Fatalf("trailing JSON error = %v, want ErrInvalid", err)
	}
}
