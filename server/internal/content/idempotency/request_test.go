package idempotency

import (
	"errors"
	"testing"
)

func TestRequestValidatesAndFingerprintsCanonicalInput(t *testing.T) {
	first, err := NewRequest("create-work", "", "key-1", struct {
		Title  string `json:"title"`
		Import bool   `json:"historical_import"`
	}{Title: "历史作品", Import: true})
	if err != nil {
		t.Fatalf("NewRequest(first): %v", err)
	}
	second, err := NewRequest("create-work", "", "key-1", struct {
		Title  string `json:"title"`
		Import bool   `json:"historical_import"`
	}{Title: "历史作品", Import: true})
	if err != nil {
		t.Fatalf("NewRequest(second): %v", err)
	}
	if first.Fingerprint != second.Fingerprint {
		t.Fatalf("same canonical input has fingerprints %q and %q", first.Fingerprint, second.Fingerprint)
	}

	different, err := NewRequest("create-work", "", "key-1", struct {
		Title  string `json:"title"`
		Import bool   `json:"historical_import"`
	}{Title: "changed", Import: true})
	if err != nil {
		t.Fatalf("NewRequest(different): %v", err)
	}
	if first.Fingerprint == different.Fingerprint {
		t.Fatal("different input reused the same fingerprint")
	}
}

func TestRequestRejectsMissingOrOversizedKey(t *testing.T) {
	if _, err := NewRequest("create-work", "", "", map[string]string{}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("missing key error = %v, want ErrInvalid", err)
	}
	tooLong := make([]byte, MaxKeyBytes+1)
	for i := range tooLong {
		tooLong[i] = 'a'
	}
	if _, err := NewRequest("create-work", "", string(tooLong), map[string]string{}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("oversized key error = %v, want ErrInvalid", err)
	}
}
