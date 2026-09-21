package topicplanning

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestNormalizeSourceIDs(t *testing.T) {
	t.Run("nil returns empty non-nil slice", func(t *testing.T) {
		got, err := NormalizeSourceIDs("fit_source_ids", nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got == nil || len(got) != 0 {
			t.Fatalf("NormalizeSourceIDs(nil) = %v, want empty non-nil slice", got)
		}
	})

	t.Run("discards empty and whitespace-only entries", func(t *testing.T) {
		raw := []string{"", "   ", "\t", "\n", "src-1", "  ", "src-2", ""}
		got, err := NormalizeSourceIDs("fit_source_ids", raw)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := []string{"src-1", "src-2"}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("NormalizeSourceIDs() = %v, want %v", got, want)
		}
	})

	t.Run("deduplicates preserving first-seen order", func(t *testing.T) {
		raw := []string{"src-1", "src-2", "src-1", "src-3", "src-2", "src-4"}
		got, err := NormalizeSourceIDs("evidence_source_ids", raw)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := []string{"src-1", "src-2", "src-3", "src-4"}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("NormalizeSourceIDs() = %v, want %v", got, want)
		}
	})

	t.Run("order of operations: trim before deduplication", func(t *testing.T) {
		// If deduplication happened before trim, "src-1" and "  src-1  " would be
		// treated as distinct and both preserved. Trimming first collapses them.
		raw := []string{"src-1", "  src-1  ", "src-2"}
		got, err := NormalizeSourceIDs("fit_source_ids", raw)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := []string{"src-1", "src-2"}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("NormalizeSourceIDs() = %v, want %v", got, want)
		}
	})

	t.Run("enforces 50 limit and names the field in FieldError", func(t *testing.T) {
		fifty := make([]string, 50)
		for i := 0; i < 50; i++ {
			fifty[i] = fmt.Sprintf("src-%d", i)
		}
		got, err := NormalizeSourceIDs("fit_source_ids", fifty)
		if err != nil {
			t.Fatalf("unexpected error for 50 items: %v", err)
		}
		if len(got) != 50 {
			t.Fatalf("len = %d, want 50", len(got))
		}

		fiftyOne := make([]string, 51)
		for i := 0; i < 51; i++ {
			fiftyOne[i] = fmt.Sprintf("src-%d", i)
		}
		_, err = NormalizeSourceIDs("fit_source_ids", fiftyOne)
		if err == nil {
			t.Fatalf("expected error for 51 items, got nil")
		}
		if !errors.Is(err, ErrInvalid) {
			t.Fatalf("expected ErrInvalid, got %v", err)
		}
		var fieldErr FieldError
		if !errors.As(err, &fieldErr) {
			t.Fatalf("expected FieldError, got %T: %v", err, err)
		}
		if fieldErr.Field != "fit_source_ids" {
			t.Fatalf("fieldErr.Field = %q, want %q", fieldErr.Field, "fit_source_ids")
		}
	})

	t.Run("limit evaluates after deduplication and whitespace stripping", func(t *testing.T) {
		// 60 inputs, but 15 duplicates and 5 blanks -> 40 distinct items. Must pass.
		raw := make([]string, 0, 60)
		for i := 0; i < 40; i++ {
			raw = append(raw, fmt.Sprintf("src-%d", i))
		}
		for i := 0; i < 15; i++ {
			raw = append(raw, fmt.Sprintf("src-%d", i))
		}
		raw = append(raw, "", "   ", " ", "\t", "\n")
		got, err := NormalizeSourceIDs("evidence_source_ids", raw)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 40 {
			t.Fatalf("len = %d, want 40", len(got))
		}
	})
}

// TestTopicCardSourceReferenceFieldsContract verifies that TopicCard has exactly
// two source reference fields (fit_source_ids and evidence_source_ids), both []string,
// and no third source reference field (neither for existing content relation nor learning).
// Contract: contracts/topic-source-refs.md §1 & §7; FR-001a, FR-001b, SC-001.
func TestTopicCardSourceReferenceFieldsContract(t *testing.T) {
	typ := reflect.TypeOf(TopicCard{})

	type refField struct {
		goName   string
		jsonName string
		typeName string
	}
	var sourceFields []refField

	for i := 0; i < typ.NumField(); i++ {
		f := typ.Field(i)
		jsonTag := f.Tag.Get("json")
		jsonName := strings.Split(jsonTag, ",")[0]

		if strings.Contains(strings.ToLower(f.Name), "source") || strings.Contains(strings.ToLower(jsonName), "source") {
			sourceFields = append(sourceFields, refField{
				goName:   f.Name,
				jsonName: jsonName,
				typeName: f.Type.String(),
			})
		}
	}

	if len(sourceFields) != 2 {
		t.Fatalf("TopicCard has %d source reference fields, want exactly 2 (found: %+v)", len(sourceFields), sourceFields)
	}

	expected := map[string]refField{
		"FitSourceIDs": {
			goName:   "FitSourceIDs",
			jsonName: "fit_source_ids",
			typeName: "[]string",
		},
		"EvidenceSourceIDs": {
			goName:   "EvidenceSourceIDs",
			jsonName: "evidence_source_ids",
			typeName: "[]string",
		},
	}

	for _, sf := range sourceFields {
		exp, ok := expected[sf.goName]
		if !ok {
			t.Errorf("unexpected source field on TopicCard: %+v", sf)
			continue
		}
		if sf.jsonName != exp.jsonName {
			t.Errorf("field %s json tag = %q, want %q", sf.goName, sf.jsonName, exp.jsonName)
		}
		if sf.typeName != exp.typeName {
			t.Errorf("field %s type = %q, want %q", sf.goName, sf.typeName, exp.typeName)
		}
	}
}
