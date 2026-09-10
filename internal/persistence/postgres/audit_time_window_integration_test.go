package postgres

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"control-center/internal/identity/audit"
)

func TestPostgresAuditReadBoundedTimeWindow(t *testing.T) {
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
	correlationID := fmt.Sprintf("integration-audit-window-%d", time.Now().UnixNano())
	from := time.Now().UTC().Truncate(time.Microsecond).Add(-4 * time.Hour)
	to := from.Add(2 * time.Hour)
	for _, occurredAt := range []time.Time{from.Add(-time.Microsecond), from, from.Add(time.Hour), to} {
		if err := log.Append(ctx, audit.Event{
			OccurredAt: occurredAt, Action: "integration.audit-window", Outcome: "success", CorrelationID: correlationID,
		}); err != nil {
			t.Fatal(err)
		}
	}

	page, err := log.Read(ctx, audit.Query{CorrelationID: correlationID, From: from, To: to})
	if err != nil {
		t.Fatal(err)
	}
	if page.HasMore || len(page.Entries) != 2 {
		t.Fatalf("time window has_more=%v len=%d", page.HasMore, len(page.Entries))
	}
	for _, entry := range page.Entries {
		if entry.Event.OccurredAt.Before(from) || !entry.Event.OccurredAt.Before(to) {
			t.Fatalf("event escaped requested time window: %#v", entry.Event)
		}
	}
}
