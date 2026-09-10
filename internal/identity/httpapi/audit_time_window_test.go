package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"control-center/internal/identity/audit"
)

func TestAuditEventsTimeWindowUsesInclusiveFromExclusiveTo(t *testing.T) {
	fixture := newAuditEventsFixture(t)
	cookie := fixture.login(t, "auditor", "a secure test password")
	from := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	to := from.Add(2 * time.Hour)
	for _, event := range []audit.Event{
		{ID: "11111111111111111111111111111111", OccurredAt: from.Add(-time.Microsecond), Action: "security.window", Outcome: "success"},
		{ID: "22222222222222222222222222222222", OccurredAt: from, Action: "security.window", Outcome: "success"},
		{ID: "33333333333333333333333333333333", OccurredAt: from.Add(time.Hour), Action: "security.window", Outcome: "success"},
		{ID: "44444444444444444444444444444444", OccurredAt: to, Action: "security.window", Outcome: "success"},
	} {
		if err := fixture.log.Append(context.Background(), event); err != nil {
			t.Fatal(err)
		}
	}

	target := "/api/v1/audit/events?action=security.window&from=" + from.Format(time.RFC3339Nano) + "&to=" + to.Format(time.RFC3339Nano)
	result := fixture.get(t, cookie, target)
	if result.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", result.Code, result.Body.String())
	}
	var response auditEventsResponse
	if err := json.Unmarshal(result.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Events) != 2 {
		t.Fatalf("events=%d want=2 body=%s", len(response.Events), result.Body.String())
	}
	if response.Events[0].ID != "33333333333333333333333333333333" || response.Events[1].ID != "22222222222222222222222222222222" {
		t.Fatalf("unexpected window response: %#v", response.Events)
	}

	records := fixture.log.Records()
	evidence := records[len(records)-1]
	if evidence.Action != "audit.events_list" || evidence.Outcome != "success" {
		t.Fatalf("unexpected evidence event: %#v", evidence)
	}
	if evidence.Details["from"] != from.Format(time.RFC3339Nano) || evidence.Details["to"] != to.Format(time.RFC3339Nano) {
		t.Fatalf("time window missing from audit evidence: %#v", evidence.Details)
	}
}

func TestAuditEventsRejectsInvalidOrUnboundedTimeWindows(t *testing.T) {
	fixture := newAuditEventsFixture(t)
	cookie := fixture.login(t, "auditor", "a secure test password")
	for _, target := range []string{
		"/api/v1/audit/events?from=2026-09-01T00:00:00Z",
		"/api/v1/audit/events?to=2026-09-02T00:00:00Z",
		"/api/v1/audit/events?from=not-a-time&to=2026-09-02T00:00:00Z",
		"/api/v1/audit/events?from=2026-09-02T00:00:00Z&to=2026-09-01T00:00:00Z",
		"/api/v1/audit/events?from=2026-09-01T00:00:00Z&to=2026-10-03T00:00:00Z",
		"/api/v1/audit/events?from=" + strings.Repeat("1", 65) + "&to=2026-09-02T00:00:00Z",
	} {
		result := fixture.get(t, cookie, target)
		if result.Code != http.StatusBadRequest || !strings.Contains(result.Body.String(), "invalid_audit_query") {
			t.Fatalf("%s status=%d body=%s", target, result.Code, result.Body.String())
		}
	}
}
