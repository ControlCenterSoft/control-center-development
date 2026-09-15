package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWebFirstLoginRedirectRecordsAuthorizationDenial(t *testing.T) {
	f := newHTTPFixture(t)

	loginRequest := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"username":"admin","password":"admin"}`))
	loginRequest.Header.Set("Content-Type", "application/json")
	loginResult := httptest.NewRecorder()
	f.server.ServeHTTP(loginResult, loginRequest)
	if loginResult.Code != http.StatusOK {
		t.Fatalf("admin login status=%d body=%s", loginResult.Code, loginResult.Body.String())
	}
	cookies := loginResult.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("admin login cookies=%d", len(cookies))
	}
	before := len(f.log.Records())

	overviewRequest := httptest.NewRequest(http.MethodGet, "/overview", nil)
	overviewRequest.AddCookie(cookies[0])
	overviewResult := httptest.NewRecorder()
	f.server.ServeHTTP(overviewResult, overviewRequest)
	if overviewResult.Code != http.StatusSeeOther || overviewResult.Header().Get("Location") != "/password/change" {
		t.Fatalf("first-login overview status=%d location=%q body=%s", overviewResult.Code, overviewResult.Header().Get("Location"), overviewResult.Body.String())
	}
	if strings.Contains(overviewResult.Body.String(), "password_change_required") {
		t.Fatalf("browser redirect exposed API denial reason: %s", overviewResult.Body.String())
	}

	records := f.log.Records()
	if len(records) != before+1 {
		t.Fatalf("audit record count=%d, want %d", len(records), before+1)
	}
	denial := records[len(records)-1]
	if denial.Action != "authorization.check" || denial.Outcome != "denied" {
		t.Fatalf("audit action/outcome=%q/%q", denial.Action, denial.Outcome)
	}
	if denial.ActorID != "admin-1" {
		t.Fatalf("audit actor=%q, want admin-1", denial.ActorID)
	}
	if denial.SourceIP != "192.0.2.1" {
		t.Fatalf("audit source_ip=%q, want 192.0.2.1", denial.SourceIP)
	}
	if reason, ok := denial.Details["reason"].(string); !ok || reason != "password_change_required" {
		t.Fatalf("audit reason=%v, want password_change_required", denial.Details["reason"])
	}
}
