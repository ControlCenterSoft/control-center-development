package audit

import (
	"context"
	"testing"
	"time"
)

func TestMemoryLogReadFiltersByBoundedTimeWindow(t *testing.T) {
	log := NewMemoryLog()
	from := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	to := from.Add(2 * time.Hour)
	for _, event := range []Event{
		{ID: "before", OccurredAt: from.Add(-time.Microsecond), Action: "security.window", Outcome: "success"},
		{ID: "at-from", OccurredAt: from, Action: "security.window", Outcome: "success"},
		{ID: "inside", OccurredAt: from.Add(time.Hour), Action: "security.window", Outcome: "success"},
		{ID: "before-to", OccurredAt: to.Add(-time.Microsecond), Action: "security.window", Outcome: "success"},
		{ID: "at-to", OccurredAt: to, Action: "security.window", Outcome: "success"},
	} {
		if err := log.Append(context.Background(), event); err != nil {
			t.Fatal(err)
		}
	}

	page, err := log.Read(context.Background(), Query{From: from, To: to})
	if err != nil {
		t.Fatal(err)
	}
	if page.HasMore || len(page.Entries) != 3 {
		t.Fatalf("window page has_more=%v len=%d", page.HasMore, len(page.Entries))
	}
	want := []string{"before-to", "inside", "at-from"}
	for index, entry := range page.Entries {
		if entry.Event.ID != want[index] {
			t.Fatalf("entry[%d]=%q want=%q", index, entry.Event.ID, want[index])
		}
	}
}

func TestNormalizeQueryRejectsInvalidTimeWindows(t *testing.T) {
	from := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	for _, query := range []Query{
		{From: from},
		{To: from.Add(time.Hour)},
		{From: from, To: from},
		{From: from.Add(time.Hour), To: from},
		{From: from, To: from.Add(MaxReadWindow + time.Nanosecond)},
		{From: from, To: from.Add(time.Nanosecond)},
	} {
		if _, err := NormalizeQuery(query); err == nil {
			t.Fatalf("NormalizeQuery accepted invalid time window: %#v", query)
		}
	}
}

func TestNormalizeQueryAcceptsMaximumWindowAndCanonicalizesUTC(t *testing.T) {
	zone := time.FixedZone("test", 3*60*60)
	from := time.Date(2026, 9, 1, 12, 0, 0, 123456789, zone)
	to := from.Add(MaxReadWindow)
	normalized, err := NormalizeQuery(Query{From: from, To: to})
	if err != nil {
		t.Fatal(err)
	}
	if normalized.From.Location() != time.UTC || normalized.To.Location() != time.UTC {
		t.Fatalf("window was not canonicalized to UTC: from=%s to=%s", normalized.From, normalized.To)
	}
	if normalized.From.Nanosecond()%1_000 != 0 || normalized.To.Nanosecond()%1_000 != 0 {
		t.Fatalf("window was not canonicalized to PostgreSQL microseconds: from=%s to=%s", normalized.From, normalized.To)
	}
	if got := normalized.To.Sub(normalized.From); got != MaxReadWindow {
		t.Fatalf("window=%s want=%s", got, MaxReadWindow)
	}
}
