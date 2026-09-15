package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFirstLoginBlocksSessionSecurityOperationsUntilPasswordChange(t *testing.T) {
	fixture := newHTTPFixture(t)
	loginBody, err := json.Marshal(map[string]string{"username": "admin", "password": "admin"})
	if err != nil {
		t.Fatal(err)
	}
	loginRequest := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(loginBody))
	loginRequest.Header.Set("Content-Type", "application/json")
	loginResult := httptest.NewRecorder()
	fixture.server.ServeHTTP(loginResult, loginRequest)
	if loginResult.Code != http.StatusOK {
		t.Fatalf("bootstrap login status=%d body=%s", loginResult.Code, loginResult.Body.String())
	}
	cookies := loginResult.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("bootstrap login cookies=%d", len(cookies))
	}
	cookie := cookies[0]

	sessionRequest := httptest.NewRequest(http.MethodGet, "/api/v1/auth/session", nil)
	sessionRequest.AddCookie(cookie)
	sessionResult := httptest.NewRecorder()
	fixture.server.ServeHTTP(sessionResult, sessionRequest)
	if sessionResult.Code != http.StatusOK || !strings.Contains(sessionResult.Body.String(), `"password_change_required":true`) {
		t.Fatalf("session status=%d body=%s", sessionResult.Code, sessionResult.Body.String())
	}

	protected := []struct {
		method string
		path   string
	}{
		{method: http.MethodGet, path: "/api/v1/auth/session-policy"},
		{method: http.MethodGet, path: "/api/v1/auth/sessions"},
		{method: http.MethodDelete, path: "/api/v1/auth/sessions/session-does-not-matter"},
		{method: http.MethodPost, path: "/api/v1/auth/sessions/revoke-all"},
	}
	for _, tc := range protected {
		request := httptest.NewRequest(tc.method, tc.path, nil)
		request.AddCookie(cookie)
		result := httptest.NewRecorder()
		fixture.server.ServeHTTP(result, request)
		if result.Code != http.StatusForbidden || !strings.Contains(result.Body.String(), "password_change_required") {
			t.Fatalf("%s %s status=%d body=%s", tc.method, tc.path, result.Code, result.Body.String())
		}
	}

	denials := 0
	for _, record := range fixture.log.Records() {
		if record.Action != "authorization.check" || record.Outcome != "denied" || record.ActorID != "admin-1" {
			continue
		}
		if reason, _ := record.Details["reason"].(string); reason == "password_change_required" {
			denials++
		}
	}
	if denials < len(protected) {
		t.Fatalf("password-change gate audit denials=%d want-at-least=%d", denials, len(protected))
	}
}
