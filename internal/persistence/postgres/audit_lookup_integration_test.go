package postgres

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"control-center/internal/identity/audit"
)

func TestPostgresAuditReadExactEventAndCorrelationFilters(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set; PostgreSQL integration test skipped")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db, err := Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	log, err := NewAuditLog(db)
	if err != nil {
		t.Fatal(err)
	}
	correlationID := fmt.Sprintf("integration-audit-lookup-%d", time.Now().UnixNano())
	for _, action := range []string{"integration.audit-lookup.first", "integration.audit-lookup.second"} {
		if err := log.Append(ctx, audit.Event{
			Action: action, Outcome: "success", CorrelationID: correlationID,
		}); err != nil {
			t.Fatal(err)
		}
	}

	correlated, err := log.Read(ctx, audit.Query{CorrelationID: correlationID})
	if err != nil {
		t.Fatal(err)
	}
	if correlated.HasMore || len(correlated.Entries) != 2 {
		t.Fatalf("correlation lookup has_more=%v len=%d", correlated.HasMore, len(correlated.Entries))
	}
	for _, entry := range correlated.Entries {
		if entry.Event.CorrelationID != correlationID {
			t.Fatalf("unexpected correlation id: %#v", entry.Event)
		}
	}

	target := correlated.Entries[0].Event
	byEvent, err := log.Read(ctx, audit.Query{EventID: target.ID})
	if err != nil {
		t.Fatal(err)
	}
	if byEvent.HasMore || len(byEvent.Entries) != 1 || byEvent.Entries[0].Event.ID != target.ID {
		t.Fatalf("event lookup returned %#v", byEvent)
	}
}
