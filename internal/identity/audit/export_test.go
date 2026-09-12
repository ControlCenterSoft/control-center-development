package audit

import (
	"context"
	"strings"
	"testing"
)

func TestReadExportReadsAcrossPagesAndIsBounded(t *testing.T) {
	log := NewMemoryLog()
	for index := 1; index <= 235; index++ {
		if err := log.Append(context.Background(), Event{
			Action: "security.test", Outcome: "success", ActorID: "actor-a",
			Details: map[string]any{"index": index},
		}); err != nil {
			t.Fatal(err)
		}
	}

	got, err := ReadExport(context.Background(), log, ExportRequest{
		Query: Query{Action: "security.test"}, Limit: 220,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Events) != 220 || !got.Truncated || got.NextBeforeSequenceID != 16 {
		t.Fatalf("export len=%d truncated=%v next=%d", len(got.Events), got.Truncated, got.NextBeforeSequenceID)
	}

	tail, err := ReadExport(context.Background(), log, ExportRequest{
		Query: Query{Action: "security.test", BeforeSequenceID: got.NextBeforeSequenceID}, Limit: 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(tail.Events) != 15 || tail.Truncated {
		t.Fatalf("tail len=%d truncated=%v", len(tail.Events), tail.Truncated)
	}
}

func TestReadExportPreservesFiltersAndRedaction(t *testing.T) {
	log := NewMemoryLog()
	if err := log.Append(context.Background(), Event{
		Action: "security.test", Outcome: "success", ActorID: "actor-a",
		Details: map[string]any{"password": "synthetic-secret", "safe": "ok"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := log.Append(context.Background(), Event{Action: "other", Outcome: "success", ActorID: "actor-b"}); err != nil {
		t.Fatal(err)
	}

	got, err := ReadExport(context.Background(), log, ExportRequest{
		Query: Query{Action: "security.test", ActorID: "actor-a"}, Limit: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Events) != 1 {
		t.Fatalf("export len=%d", len(got.Events))
	}
	if got.Events[0].Details["password"] != "[REDACTED]" {
		t.Fatalf("unexpected export details: %#v", got.Events[0].Details)
	}
	if strings.Contains(got.Events[0].Details["password"].(string), "synthetic-secret") {
		t.Fatal("audit export leaked a secret")
	}
}

type brokenExportReader struct{}

func (brokenExportReader) Read(context.Context, Query) (Page, error) {
	return Page{HasMore: true}, nil
}

func TestReadExportRejectsInvalidInputs(t *testing.T) {
	if _, err := ReadExport(context.Background(), nil, ExportRequest{}); err == nil {
		t.Fatal("nil reader was accepted")
	}
	if _, err := ReadExport(context.Background(), NewMemoryLog(), ExportRequest{Limit: MaxExportLimit + 1}); err == nil {
		t.Fatal("oversized export was accepted")
	}
	if _, err := ReadExport(context.Background(), NewMemoryLog(), ExportRequest{Query: Query{Limit: 1}}); err == nil {
		t.Fatal("ambiguous query limit was accepted")
	}
	if _, err := ReadExport(context.Background(), brokenExportReader{}, ExportRequest{Limit: 1}); err == nil {
		t.Fatal("broken pagination contract was accepted")
	}
}
