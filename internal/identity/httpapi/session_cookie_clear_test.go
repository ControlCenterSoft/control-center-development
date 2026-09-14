package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestLogoutClearsSessionCookieWithSecurityAttributes(t *testing.T) {
	fixture := newHTTPFixture(t)
	cookie := fixture.login(t, "viewer")

	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	request.AddCookie(cookie)
	result := httptest.NewRecorder()
	fixture.server.ServeHTTP(result, request)
	if result.Code != http.StatusNoContent {
		t.Fatalf("logout status=%d body=%s", result.Code, result.Body.String())
	}

	cookies := result.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("clear cookie count=%d, want 1", len(cookies))
	}
	cleared := cookies[0]
	if cleared.Name != DefaultSessionCookie {
		t.Fatalf("clear cookie name=%q, want %q", cleared.Name, DefaultSessionCookie)
	}
	if cleared.Value != "" {
		t.Fatalf("clear cookie value=%q, want empty", cleared.Value)
	}
	if cleared.Path != "/" {
		t.Fatalf("clear cookie path=%q, want /", cleared.Path)
	}
	if cleared.MaxAge >= 0 {
		t.Fatalf("clear cookie MaxAge=%d, want negative", cleared.MaxAge)
	}
	if !cleared.HttpOnly || !cleared.Secure || cleared.SameSite != http.SameSiteStrictMode {
		t.Fatalf("clear cookie security attributes: HttpOnly=%v Secure=%v SameSite=%v", cleared.HttpOnly, cleared.Secure, cleared.SameSite)
	}
	if !cleared.Expires.Before(time.Now()) {
		t.Fatalf("clear cookie expiry=%s, want past timestamp", cleared.Expires)
	}
}
