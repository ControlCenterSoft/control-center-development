package httpapi

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"control-center/internal/identity/audit"
)

func TestAuditEventsCursorIsBoundToQueryAndSession(t *testing.T) {
	fixture := newAuditEventsFixture(t)
	cookie := fixture.login(t, "auditor", "a secure test password")
	for index := 1; index <= 4; index++ {
		if err := fixture.log.Append(context.Background(), audit.Event{
			Action: "security.cursor", Outcome: "success", ActorID: "actor-1", CorrelationID: "change-42",
			Details: map[string]any{"index": index},
		}); err != nil {
			t.Fatal(err)
		}
	}

	first := fixture.get(t, cookie, "/api/v1/audit/events?limit=2&action=security.cursor&actor_id=actor-1&correlation_id=change-42")
	if first.Code != http.StatusOK {
		t.Fatalf("first page status=%d body=%s", first.Code, first.Body.String())
	}
	var page auditEventsResponse
	if err := json.Unmarshal(first.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Events) != 2 || page.NextCursor == "" {
		t.Fatalf("unexpected first page: %#v", page)
	}
	cursor := page.NextCursor

	sameQuery := fixture.get(t, cookie, "/api/v1/audit/events?limit=2&action=security.cursor&actor_id=actor-1&correlation_id=change-42&cursor="+cursor)
	if sameQuery.Code != http.StatusOK {
		t.Fatalf("same-query page status=%d body=%s", sameQuery.Code, sameQuery.Body.String())
	}

	for name, target := range map[string]string{
		"actor":       "/api/v1/audit/events?limit=2&action=security.cursor&actor_id=actor-2&correlation_id=change-42&cursor=" + cursor,
		"limit":       "/api/v1/audit/events?limit=3&action=security.cursor&actor_id=actor-1&correlation_id=change-42&cursor=" + cursor,
		"correlation": "/api/v1/audit/events?limit=2&action=security.cursor&actor_id=actor-1&correlation_id=change-43&cursor=" + cursor,
	} {
		t.Run(name, func(t *testing.T) {
			result := fixture.get(t, cookie, target)
			if result.Code != http.StatusBadRequest || !strings.Contains(result.Body.String(), "invalid_audit_query") {
				t.Fatalf("status=%d body=%s", result.Code, result.Body.String())
			}
		})
	}

	newCookie := fixture.login(t, "auditor", "a secure test password")
	otherSession := fixture.get(t, newCookie, "/api/v1/audit/events?limit=2&action=security.cursor&actor_id=actor-1&correlation_id=change-42&cursor="+cursor)
	if otherSession.Code != http.StatusBadRequest || !strings.Contains(otherSession.Body.String(), "invalid_audit_query") {
		t.Fatalf("cross-session cursor status=%d body=%s", otherSession.Code, otherSession.Body.String())
	}
}

func TestAuditEventsLegacyCursorCompatibilityIsUnfilteredOnly(t *testing.T) {
	fixture := newAuditEventsFixture(t)
	cookie := fixture.login(t, "auditor", "a secure test password")
	for index := 0; index < 2; index++ {
		if err := fixture.log.Append(context.Background(), audit.Event{Action: "security.legacy", Outcome: "success"}); err != nil {
			t.Fatal(err)
		}
	}
	legacy := base64.RawURLEncoding.EncodeToString([]byte(auditCursorVersionV1 + strconv.FormatInt(1<<30, 10)))

	unfiltered := fixture.get(t, cookie, "/api/v1/audit/events?limit=2&cursor="+legacy)
	if unfiltered.Code != http.StatusOK {
		t.Fatalf("legacy unfiltered status=%d body=%s", unfiltered.Code, unfiltered.Body.String())
	}

	filtered := fixture.get(t, cookie, "/api/v1/audit/events?limit=2&action=security.legacy&cursor="+legacy)
	if filtered.Code != http.StatusBadRequest || !strings.Contains(filtered.Body.String(), "invalid_audit_query") {
		t.Fatalf("legacy filtered status=%d body=%s", filtered.Code, filtered.Body.String())
	}
}
