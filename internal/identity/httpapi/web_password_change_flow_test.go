package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestWebFirstLoginPasswordChangeRevokesBootstrapSession(t *testing.T) {
	f := newHTTPFixture(t)

	loginRequest := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"username":"admin","password":"admin"}`))
	loginRequest.Header.Set("Content-Type", "application/json")
	loginResult := httptest.NewRecorder()
	f.server.ServeHTTP(loginResult, loginRequest)
	if loginResult.Code != http.StatusOK {
		t.Fatalf("bootstrap login status=%d body=%s", loginResult.Code, loginResult.Body.String())
	}
	cookies := loginResult.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("bootstrap login cookies=%d", len(cookies))
	}
	bootstrapCookie := cookies[0]

	const replacement = "a browser selected admin password"
	form := url.Values{
		"current_password": {"admin"},
		"new_password":     {replacement},
	}
	changeRequest := httptest.NewRequest(http.MethodPost, "/web/password/change", strings.NewReader(form.Encode()))
	changeRequest.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	changeRequest.AddCookie(bootstrapCookie)
	changeResult := httptest.NewRecorder()
	f.server.ServeHTTP(changeResult, changeRequest)
	if changeResult.Code != http.StatusSeeOther {
		t.Fatalf("web password change status=%d body=%s", changeResult.Code, changeResult.Body.String())
	}
	if location := changeResult.Header().Get("Location"); location != "/login" {
		t.Fatalf("web password change location=%q", location)
	}

	staleRequest := httptest.NewRequest(http.MethodGet, "/api/v1/auth/session", nil)
	staleRequest.AddCookie(bootstrapCookie)
	staleResult := httptest.NewRecorder()
	f.server.ServeHTTP(staleResult, staleRequest)
	if staleResult.Code != http.StatusUnauthorized {
		t.Fatalf("bootstrap session remained valid after browser password change: status=%d body=%s", staleResult.Code, staleResult.Body.String())
	}

	reloginBody, err := json.Marshal(map[string]string{"username": "admin", "password": replacement})
	if err != nil {
		t.Fatal(err)
	}
	reloginRequest := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(string(reloginBody)))
	reloginRequest.Header.Set("Content-Type", "application/json")
	reloginResult := httptest.NewRecorder()
	f.server.ServeHTTP(reloginResult, reloginRequest)
	if reloginResult.Code != http.StatusOK {
		t.Fatalf("post-change login status=%d body=%s", reloginResult.Code, reloginResult.Body.String())
	}
	if !strings.Contains(reloginResult.Body.String(), `"password_change_required":false`) {
		t.Fatalf("post-change login did not clear first-login gate: body=%s", reloginResult.Body.String())
	}
	if strings.Contains(reloginResult.Body.String(), replacement) {
		t.Fatal("post-change login response disclosed the selected password")
	}
}
