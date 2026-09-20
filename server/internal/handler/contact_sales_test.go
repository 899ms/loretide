package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func newContactSalesRequest(body CreateContactSalesRequest) *http.Request {
	var buf bytes.Buffer
	json.NewEncoder(&buf).Encode(body)
	req := httptest.NewRequest("POST", "/api/contact-sales", &buf)
	req.Header.Set("Content-Type", "application/json")
	return req
}

func validContactSalesRequest() CreateContactSalesRequest {
	return CreateContactSalesRequest{
		FirstName:       "Ada",
		LastName:        "Lovelace",
		BusinessEmail:   "ada@analytical-engine.example",
		CompanyName:     "Analytical Engine Co.",
		CompanySize:     "11-50",
		CountryRegion:   "United Kingdom",
		UseCase:         "evaluate",
		Goals:           "We want to compound agent productivity across the team.",
		ConsentOutreach: true,
		ConsentUpdates:  false,
	}
}

func TestIsBusinessEmailDomain(t *testing.T) {
	cases := []struct {
		email string
		want  bool
	}{
		{"ada@multica.ai", true},
		{"ada@example.com", true},
		{"ada@gmail.com", false},
		{"ada@Gmail.COM", false},
		{"ada@yahoo.co.uk", false},
		{"ada@qq.com", false},
		{"weird-no-at", false},
		{"ada@", false},
	}
	for _, c := range cases {
		got := isBusinessEmailDomain(c.email)
		if got != c.want {
			t.Errorf("isBusinessEmailDomain(%q) = %v, want %v", c.email, got, c.want)
		}
	}
}

func TestCanonicalBusinessEmail(t *testing.T) {
	cases := []struct {
		name   string
		input  string
		want   string
		wantOk bool
	}{
		{"plain", "ada@multica.ai", "ada@multica.ai", true},
		{"uppercase normalized", "Ada@Multica.AI", "ada@multica.ai", true},
		{"trim whitespace", "  ada@multica.ai  ", "ada@multica.ai", true},
		{"display name stripped", "Ada Lovelace <ada@multica.ai>", "ada@multica.ai", true},
		{"angle-bracketed", "<ada@multica.ai>", "ada@multica.ai", true},
		{"empty", "", "", false},
		{"only whitespace", "   ", "", false},
		{"missing at", "no-at-sign", "", false},
		{"missing local", "@multica.ai", "", false},
		{"missing domain", "ada@", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := canonicalBusinessEmail(c.input)
			if ok != c.wantOk || got != c.want {
				t.Errorf("canonicalBusinessEmail(%q) = (%q, %v), want (%q, %v)",
					c.input, got, ok, c.want, c.wantOk)
			}
		})
	}
}
