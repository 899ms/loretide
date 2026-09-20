package handler

import (
	"testing"
)

func TestBoolArrayElement(t *testing.T) {
	yes, no := true, false
	// Empty string is the query's NULL, matching how every other nullable
	// column in this batch is encoded.
	if got := boolArrayElement(nil); got != "" {
		t.Fatalf("nil encodes as %q, want the empty string the query maps to NULL", got)
	}
	if got := boolArrayElement(&yes); got != "true" {
		t.Fatalf("true encodes as %q", got)
	}
	if got := boolArrayElement(&no); got != "false" {
		t.Fatalf("false encodes as %q", got)
	}
}
