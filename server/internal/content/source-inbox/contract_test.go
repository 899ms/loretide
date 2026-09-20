package sourceinbox

import (
	"errors"
	"strings"
	"testing"
)

func TestValidateKindAndStatusAcceptOnlyTheirSets(t *testing.T) {
	for _, kind := range Kinds {
		if err := ValidateKind(string(kind)); err != nil {
			t.Errorf("ValidateKind(%q) = %v", kind, err)
		}
	}
	// One negative per shape a wrong value actually takes: a plausible word, a
	// case variant, and empty.
	for _, bad := range []string{"file", "PASTED_TEXT", "", "quick_note"} {
		if err := ValidateKind(bad); !errors.Is(err, ErrInvalid) {
			t.Errorf("ValidateKind(%q) = %v, want ErrInvalid", bad, err)
		}
	}
	for _, status := range Statuses {
		if err := ValidateStatus(string(status)); err != nil {
			t.Errorf("ValidateStatus(%q) = %v", status, err)
		}
	}
	for _, bad := range []string{"deleted", "Inbox", "", "pending"} {
		if err := ValidateStatus(bad); !errors.Is(err, ErrInvalid) {
			t.Errorf("ValidateStatus(%q) = %v, want ErrInvalid", bad, err)
		}
	}
}

// A 400 has to say which field. "参数错误" tells the person filling the form
// nothing about what to fix.
func TestInvalidInputNamesTheField(t *testing.T) {
	var fieldErr FieldError
	if !errors.As(ValidateKind("file"), &fieldErr) || fieldErr.Field != "kind" {
		t.Fatalf("ValidateKind did not name the field: %v", ValidateKind("file"))
	}
	if !errors.As(ValidateStatus("gone"), &fieldErr) || fieldErr.Field != "status" {
		t.Fatalf("ValidateStatus did not name the field: %v", ValidateStatus("gone"))
	}
}

func TestValidateNewChecksTheCombination(t *testing.T) {
	cases := []struct {
		name  string
		input NewSource
		field string
	}{
		{"url without a link", NewSource{Kind: KindURL}, "url"},
		{"url with a body", NewSource{Kind: KindURL, URL: "https://example.com/a", Content: "x"}, "content"},
		{"url that is not a url", NewSource{Kind: KindURL, URL: "not a url"}, "url"},
		// A javascript: link is not something a person can go back and read,
		// and storing one invites a page to follow it.
		{"url with a non-web scheme", NewSource{Kind: KindURL, URL: "javascript:alert(1)"}, "url"},
		{"pasted text with no body", NewSource{Kind: KindPastedText}, "content"},
		{"pasted text that is only spaces", NewSource{Kind: KindPastedText, Content: "   \n\t "}, "content"},
		{"pasted text with a link", NewSource{Kind: KindPastedText, Content: "x", URL: "https://example.com"}, "url"},
		{"unknown kind", NewSource{Kind: "file"}, "kind"},
		{"blank tag", NewSource{Kind: KindPastedText, Content: "x", Tags: []string{" "}}, "tags"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			err := ValidateNew(testCase.input)
			var fieldErr FieldError
			if !errors.As(err, &fieldErr) {
				t.Fatalf("ValidateNew() = %v, want a FieldError", err)
			}
			if fieldErr.Field != testCase.field {
				t.Errorf("named field %q, want %q", fieldErr.Field, testCase.field)
			}
		})
	}
}

func TestValidateNewAcceptsTheTwoGoodShapes(t *testing.T) {
	if err := ValidateNew(NewSource{Kind: KindPastedText, Content: "a note", Tags: []string{"x"}}); err != nil {
		t.Errorf("pasted text rejected: %v", err)
	}
	if err := ValidateNew(NewSource{Kind: KindURL, URL: "https://example.com/post/1", Annotation: "what it says"}); err != nil {
		t.Errorf("url rejected: %v", err)
	}
}

// Over the limit is refused, not truncated. The hash is computed over what is
// stored, so truncating would give a hash to a body nobody meant to save - and
// that hash is all duplicate detection has to go on.
func TestOverlongBodyIsRefusedRatherThanTruncated(t *testing.T) {
	err := ValidateNew(NewSource{Kind: KindPastedText, Content: strings.Repeat("x", MaxContentRunes+1)})
	var fieldErr FieldError
	if !errors.As(err, &fieldErr) || fieldErr.Field != "content" {
		t.Fatalf("ValidateNew() = %v, want a content FieldError", err)
	}
	if err := ValidateNew(NewSource{Kind: KindPastedText, Content: strings.Repeat("x", MaxContentRunes)}); err != nil {
		t.Errorf("exactly at the limit was rejected: %v", err)
	}
}

func TestContentHashIsStableAndDistinguishing(t *testing.T) {
	if ContentHash("a note") != ContentHash("a note") {
		t.Error("the same text hashed differently")
	}
	if ContentHash("a note") == ContentHash("a note ") {
		t.Error("a trailing space produced the same hash")
	}
	// No normalisation, deliberately: deciding that two near-identical texts
	// are "the same" is the merge judgement §4 leaves to a person.
	if ContentHash("A note") == ContentHash("a note") {
		t.Error("case folding happened; the hash must be over the exact bytes")
	}
	if len(ContentHash("")) != 64 {
		t.Errorf("hash is %d hex chars, want 64", len(ContentHash("")))
	}
}

func TestChangedFieldsNamesOnlyWhatThePatchTouches(t *testing.T) {
	title, status := "t", StatusArchived
	got := ChangedFields(Organize{Title: &title, Status: &status})
	want := []string{"title", "status"}
	if len(got) != len(want) {
		t.Fatalf("ChangedFields() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ChangedFields() = %v, want %v", got, want)
		}
	}
	if len(ChangedFields(Organize{})) != 0 {
		t.Error("an empty patch reported changed fields")
	}
}

func TestValidateOrganizeRejectsABadStatus(t *testing.T) {
	bad := Status("deleted")
	if err := ValidateOrganize(Organize{Status: &bad}); !errors.Is(err, ErrInvalid) {
		t.Errorf("ValidateOrganize() = %v, want ErrInvalid", err)
	}
}
