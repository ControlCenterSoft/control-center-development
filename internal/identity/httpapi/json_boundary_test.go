package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLoginRejectsDuplicateCredentialFields(t *testing.T) {
	fixture := newHTTPFixture(t)
	for _, body := range []string{
		`{"username":"does-not-exist","username":"viewer","password":"a secure test password"}`,
		`{"username":"viewer","password":"wrong but long password","password":"a secure test password"}`,
	} {
		request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		result := httptest.NewRecorder()
		fixture.server.ServeHTTP(result, request)
		if result.Code != http.StatusBadRequest || !strings.Contains(result.Body.String(), `"invalid_request"`) {
			t.Fatalf("duplicate credential field status=%d body=%s", result.Code, result.Body.String())
		}
		if len(result.Result().Cookies()) != 0 {
			t.Fatal("ambiguous login request issued a session cookie")
		}
	}
}

func TestPasswordChangeRejectsDuplicateCredentialFields(t *testing.T) {
	fixture := newHTTPFixture(t)
	cookie := fixture.login(t, "viewer")
	for _, body := range []string{
		`{"current_password":"wrong but long password","current_password":"a secure test password","new_password":"another secure test password"}`,
		`{"current_password":"a secure test password","new_password":"another secure test password","new_password":"replacement secure test password"}`,
	} {
		request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/password", strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		request.AddCookie(cookie)
		result := httptest.NewRecorder()
		fixture.server.ServeHTTP(result, request)
		if result.Code != http.StatusBadRequest || !strings.Contains(result.Body.String(), `"invalid_request"`) {
			t.Fatalf("duplicate password field status=%d body=%s", result.Code, result.Body.String())
		}
	}

	sessionRequest := httptest.NewRequest(http.MethodGet, "/api/v1/auth/session", nil)
	sessionRequest.AddCookie(cookie)
	sessionResult := httptest.NewRecorder()
	fixture.server.ServeHTTP(sessionResult, sessionRequest)
	if sessionResult.Code != http.StatusOK {
		t.Fatalf("ambiguous password request mutated current session: status=%d body=%s", sessionResult.Code, sessionResult.Body.String())
	}
}

func TestDuplicateJSONFieldValidationIsRecursive(t *testing.T) {
	for _, document := range []string{
		`{"outer":{"value":1,"value":2}}`,
		`{"outer":[{"value":1,"value":2}]}`,
	} {
		if err := rejectDuplicateJSONFields([]byte(document)); err == nil {
			t.Fatalf("expected duplicate-field rejection for %s", document)
		}
	}
}
