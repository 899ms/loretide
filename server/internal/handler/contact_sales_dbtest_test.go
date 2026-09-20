//go:build dbtest

package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func clearContactSalesForEmail(t *testing.T, email string) {
	t.Helper()
	if _, err := testPool.Exec(context.Background(),
		`DELETE FROM contact_sales_inquiry WHERE business_email = $1`, email); err != nil {
		t.Fatalf("clear contact_sales_inquiry: %v", err)
	}
	t.Cleanup(func() {
		testPool.Exec(context.Background(),
			`DELETE FROM contact_sales_inquiry WHERE business_email = $1`, email)
	})
}

func TestCreateContactSalesHappyPath(t *testing.T) {
	body := validContactSalesRequest()
	clearContactSalesForEmail(t, body.BusinessEmail)

	w := httptest.NewRecorder()
	testHandler.CreateContactSales(w, newContactSalesRequest(body))

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var resp ContactSalesResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.ID == "" {
		t.Fatal("expected inquiry id in response")
	}
}

func TestCreateContactSalesRejectsFreeEmail(t *testing.T) {
	body := validContactSalesRequest()
	body.BusinessEmail = "ada@gmail.com"

	w := httptest.NewRecorder()
	testHandler.CreateContactSales(w, newContactSalesRequest(body))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCreateContactSalesRejectsInvalidEmail(t *testing.T) {
	body := validContactSalesRequest()
	body.BusinessEmail = "not-an-email"

	w := httptest.NewRecorder()
	testHandler.CreateContactSales(w, newContactSalesRequest(body))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCreateContactSalesRejectsUnknownCompanySize(t *testing.T) {
	body := validContactSalesRequest()
	body.CompanySize = "ten-ish"

	w := httptest.NewRecorder()
	testHandler.CreateContactSales(w, newContactSalesRequest(body))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCreateContactSalesRejectsUnknownUseCase(t *testing.T) {
	body := validContactSalesRequest()
	body.UseCase = "world-domination"

	w := httptest.NewRecorder()
	testHandler.CreateContactSales(w, newContactSalesRequest(body))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCreateContactSalesMissingFirstName(t *testing.T) {
	body := validContactSalesRequest()
	body.FirstName = "   "

	w := httptest.NewRecorder()
	testHandler.CreateContactSales(w, newContactSalesRequest(body))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCreateContactSalesPerEmailRateLimit(t *testing.T) {
	body := validContactSalesRequest()
	body.BusinessEmail = "ratelimit-ada@analytical-engine.example"
	clearContactSalesForEmail(t, body.BusinessEmail)

	for i := 0; i < contactSalesHourlyEmailCap; i++ {
		w := httptest.NewRecorder()
		testHandler.CreateContactSales(w, newContactSalesRequest(body))
		if w.Code != http.StatusCreated {
			t.Fatalf("iteration %d: expected 201, got %d: %s", i, w.Code, w.Body.String())
		}
	}

	w := httptest.NewRecorder()
	testHandler.CreateContactSales(w, newContactSalesRequest(body))
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d: %s", w.Code, w.Body.String())
	}
}

// Regression: net/mail.ParseAddress accepts display-name forms like
// `Ada <ada@gmail.com>`. Earlier versions of the handler ran the
// free-email check against the raw user input, so the domain it saw was
// `gmail.com>` (with the trailing angle bracket) — which is not in the
// block list, allowing a personal address to bypass the gate. The handler
// must canonicalize through the parsed addr.Address before the check.
func TestCreateContactSalesRejectsFreeEmailWithDisplayName(t *testing.T) {
	body := validContactSalesRequest()
	body.BusinessEmail = "Ada Lovelace <ada@gmail.com>"

	w := httptest.NewRecorder()
	testHandler.CreateContactSales(w, newContactSalesRequest(body))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCreateContactSalesNormalizesDisplayNameEmail(t *testing.T) {
	body := validContactSalesRequest()
	body.BusinessEmail = "Ada Lovelace <ada@display-name.example>"
	clearContactSalesForEmail(t, "ada@display-name.example")

	w := httptest.NewRecorder()
	testHandler.CreateContactSales(w, newContactSalesRequest(body))

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var stored string
	if err := testPool.QueryRow(context.Background(),
		`SELECT business_email FROM contact_sales_inquiry WHERE business_email = $1`,
		"ada@display-name.example").Scan(&stored); err != nil {
		t.Fatalf("expected row with canonical email persisted: %v", err)
	}
	if stored != "ada@display-name.example" {
		t.Fatalf("expected canonical email persisted, got %q", stored)
	}
}
