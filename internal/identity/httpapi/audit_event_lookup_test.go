package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"control-center/internal/identity/audit"
)

func TestAuditEventsLookupByExactEventID(t *testing.T) {
	fixture := newAuditEventsFixture(t)
	cookie := fixture.login(t, "auditor", "a secure test password")
	if err := fixture.log.Append(context.Background(), audit.Event{
		Action: "security.lookup.target", Outcome: "success", CorrelationID: "change-exact",
	}); err != nil {
		t.Fatal(err)
	}
	records := fixture.log.Records()
	targetID := records[len(records)-1].ID
	if err := fixture.log.Append(context.Background(), audit.Event{
		Action: "security.lookup.other", Outcome: "success", CorrelationID: "change-exact",
	}); err != nil {
		t.Fatal(err)
	}

	result := fixture.get(t, cookie, "/api/v1/audit/events?event_id="+targetID)
	if result.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", result.Code, result.Body.String())
	}
	var response auditEventsResponse
	if err := json.Unmarshal(result.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Events) != 1 || response.Events[0].ID != targetID || response.NextCursor != "" {
		t.Fatalf("unexpected exact lookup response: %#v", response)
	}
}

func TestAuditEventsLookupByExactCorrelationIDIsBounded(t *testing.T) {
	fixture := newAuditEventsFixture(t)
	cookie := fixture.login(t, "auditor", "a secure test password")
	for index := 0; index < 3; index++ {
		if err := fixture.log.Append(context.Background(), audit.Event{
			Action: "security.lookup.correlated", Outcome: "success", CorrelationID: "change-42",
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := fixture.log.Append(context.Background(), audit.Event{
		Action: "security.lookup.correlated", Outcome: "success", CorrelationID: "change-420",
	}); err != nil {
		t.Fatal(err)
	}

	first := fixture.get(t, cookie, "/api/v1/audit/events?limit=2&correlation_id=change-42")
	if first.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", first.Code, first.Body.String())
	}
	var page auditEventsResponse
	if err := json.Unmarshal(first.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Events) != 2 || page.NextCursor == "" {
		t.Fatalf("unexpected first correlation page: %#v", page)
	}
	for _, event := range page.Events {
		if event.CorrelationID != "change-42" {
			t.Fatalf("non-exact correlation match leaked into response: %#v", event)
		}
	}

	second := fixture.get(t, cookie, "/api/v1/audit/events?limit=2&correlation_id=change-42&cursor="+page.NextCursor)
	if second.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", second.Code, second.Body.String())
	}
	if err := json.Unmarshal(second.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Events) != 1 || page.NextCursor != "" || page.Events[0].CorrelationID != "change-42" {
		t.Fatalf("unexpected second correlation page: %#v", page)
	}
}

func TestAuditEventsLookupRejectsOverlongExactIdentifiers(t *testing.T) {
	fixture := newAuditEventsFixture(t)
	cookie := fixture.login(t, "auditor", "a secure test password")
	for _, target := range []string{
		"/api/v1/audit/events?event_id=" + strings.Repeat("e", 65),
		"/api/v1/audit/events?correlation_id=" + strings.Repeat("c", 257),
	} {
		result := fixture.get(t, cookie, target)
		if result.Code != http.StatusBadRequest || !strings.Contains(result.Body.String(), "invalid_audit_query") {
			t.Fatalf("%s status=%d body=%s", target, result.Code, result.Body.String())
		}
	}
}
