//go:build dbtest

package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/multica-ai/multica/server/internal/auth"
	"github.com/multica-ai/multica/server/internal/middleware"
	"github.com/multica-ai/multica/server/internal/testutil"
)

func TestGoogleLoginSuccessfulExistingUser(t *testing.T) {
	t.Setenv("GOOGLE_CLIENT_ID", "test-client")
	t.Setenv("GOOGLE_CLIENT_SECRET", "test-secret")
	email := "google-login-success@example.com"
	userID := dbfx.User(t, "Google OAuth User", email)
	// Existing users can still sign in when new registrations are restricted.
	h := newTestHandler(Config{AllowSignup: false, AllowedEmailDomains: []string{"company.com"}})
	h.Queries = testHandler.Queries
	requests := 0
	h.googleOAuthHTTPClient = &http.Client{Transport: googleRoundTripper(func(req *http.Request) (*http.Response, error) {
		requests++
		switch req.URL.Host {
		case "oauth2.googleapis.com":
			if err := req.ParseForm(); err != nil {
				t.Fatal(err)
			}
			if req.Form.Get("code") != "test-code" || req.Form.Get("redirect_uri") != "http://localhost/auth/callback" {
				t.Fatalf("unexpected token exchange form: %v", req.Form)
			}
			return googleResponse(req, http.StatusOK, `{"access_token":"test-token"}`), nil
		case "www.googleapis.com":
			if req.Header.Get("Authorization") != "Bearer test-token" {
				t.Fatal("userinfo request did not use the exchanged token")
			}
			return googleResponse(req, http.StatusOK, `{"email":" GOOGLE-LOGIN-SUCCESS@EXAMPLE.COM "}`), nil
		default:
			t.Fatalf("unexpected Google OAuth request: %s", req.URL)
			return nil, nil
		}
	})}
	req := httptest.NewRequest(http.MethodPost, "/auth/google", strings.NewReader(`{"code":"test-code","redirect_uri":"http://localhost/auth/callback"}`))
	var got LoginResponse
	resp := testutil.Call(t, h.GoogleLogin, req).Want(http.StatusOK).JSON(&got)
	if requests != 2 || got.Token == "" || got.User.ID != userID || got.User.Email != email {
		t.Fatalf("unexpected successful login: requests=%d, token present=%t, user=%+v", requests, got.Token != "", got.User)
	}
	var authCookie, csrfCookie *http.Cookie
	for _, cookie := range resp.Result().Cookies() {
		switch cookie.Name {
		case auth.AuthCookieName:
			authCookie = cookie
		case auth.CSRFCookieName:
			csrfCookie = cookie
		}
	}
	if authCookie == nil || authCookie.Value != got.Token || csrfCookie == nil || csrfCookie.Value == "" {
		t.Fatal("successful Google login must return matching auth and CSRF cookies")
	}

	// Exercise the issued JWT through the same middleware that accepts browser sessions.
	meReq := httptest.NewRequest(http.MethodGet, "/users/me", nil)
	meReq.AddCookie(authCookie)
	protected := middleware.Auth(h.Queries, nil, nil)(http.HandlerFunc(h.GetMe))
	var me UserResponse
	testutil.Call(t, protected.ServeHTTP, meReq).Want(http.StatusOK).JSON(&me)
	if me.ID != userID || me.Email != email {
		t.Fatalf("Google session resolved to the wrong user: %+v", me)
	}

	csrfReq := httptest.NewRequest(http.MethodPost, "/users/me", nil)
	csrfReq.AddCookie(authCookie)
	csrfReq.Header.Set("X-CSRF-Token", csrfCookie.Value)
	if !auth.ValidateCSRF(csrfReq) {
		t.Fatal("Google login cookies must allow authenticated browser writes")
	}
}
