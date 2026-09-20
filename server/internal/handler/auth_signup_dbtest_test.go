//go:build dbtest

package handler

import (
	"net/http"
	"testing"

	"github.com/multica-ai/multica/server/internal/testutil"
)

func TestEmailCodeAllowlistErrors(t *testing.T) {
	for _, path := range []string{"send-code", "verify-code"} {
		t.Run(path, func(t *testing.T) {
			email := path + "-allowlist-regression@example.com"
			h := newTestHandler(Config{AllowSignup: true, AllowedEmailDomains: []string{"company.com"}})
			h.Queries = testHandler.Queries
			dbfx.Cleanup(t, `DELETE FROM "user" WHERE email = $1`, email)
			body := map[string]string{"email": email}
			handler := h.SendCode
			if path == "verify-code" {
				// A code can remain valid after the instance's signup policy changes.
				body["code"] = "123456"
				dbfx.Insert(t, "verification_code", testutil.Cols{
					"email":      email,
					"code":       body["code"],
					"expires_at": testutil.Raw("now() + interval '10 minutes'"),
				})
				handler = h.VerifyCode
			}
			req := testutil.JSONRequest(http.MethodPost, "/auth/"+path, body)
			resp := testutil.Call(t, handler, req).Want(http.StatusForbidden)
			got := resp.Map()
			if got["error"] != ErrEmailNotAllowed.Error() {
				t.Fatalf("expected an actionable allowlist error, got %v", got)
			}
			if _, hasCode := got["code"]; hasCode {
				t.Fatal("email-code errors must retain their existing response shape")
			}
			if len(resp.Result().Cookies()) != 0 {
				t.Fatal("rejected signup must not establish an authenticated session")
			}
			if count := dbfx.Count(t, `SELECT count(*) FROM "user" WHERE email = $1`, email); count != 0 {
				t.Fatalf("rejected signup created %d users", count)
			}
		})
	}
}
