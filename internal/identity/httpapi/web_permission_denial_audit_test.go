package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"control-center/internal/identity/rbac"
)

func TestWebPermissionDenialRecordsAuthorizationEvidence(t *testing.T) {
	f := newHTTPFixture(t)
	cookie := f.login(t, "unbound")
	before := len(f.log.Records())

	request := httptest.NewRequest(http.MethodGet, "/overview", nil)
	request.AddCookie(cookie)
	result := httptest.NewRecorder()
	f.server.ServeHTTP(result, request)

	if result.Code != http.StatusForbidden {
		t.Fatalf("browser permission denial status=%d body=%s", result.Code, result.Body.String())
	}
	if strings.Contains(result.Body.String(), "permission_denied") {
		t.Fatalf("browser permission denial exposed API error envelope: %s", result.Body.String())
	}

	records := f.log.Records()
	if len(records) != before+1 {
		t.Fatalf("audit record count=%d, want %d", len(records), before+1)
	}
	denial := records[len(records)-1]
	if denial.Action != "authorization.check" || denial.Outcome != "denied" {
		t.Fatalf("audit action/outcome=%q/%q", denial.Action, denial.Outcome)
	}
	if denial.ActorID != "unbound-1" {
		t.Fatalf("audit actor=%q, want unbound-1", denial.ActorID)
	}
	if denial.SourceIP != "192.0.2.1" {
		t.Fatalf("audit source_ip=%q, want 192.0.2.1", denial.SourceIP)
	}
	permission, ok := denial.Details["permission"].(rbac.Permission)
	if !ok || permission != rbac.PermissionOverviewRead {
		t.Fatalf("audit permission=%#v, want %q", denial.Details["permission"], rbac.PermissionOverviewRead)
	}
	scope, ok := denial.Details["scope"].(rbac.Scope)
	if !ok || scope != rbac.GlobalScope() {
		t.Fatalf("audit scope=%#v, want %#v", denial.Details["scope"], rbac.GlobalScope())
	}
}
