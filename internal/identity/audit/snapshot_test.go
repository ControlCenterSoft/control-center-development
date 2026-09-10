package audit

import (
	"context"
	"testing"
)

func TestMemoryLogRecordsReturnsDeepCopy(t *testing.T) {
	log := NewMemoryLog()
	input := map[string]any{
		"request": map[string]any{
			"path": "/api/v1/example",
			"labels": []any{
				"first",
				map[string]any{"result": "allowed"},
			},
		},
	}

	if err := log.Append(context.Background(), Event{
		Action:  "authorization.check",
		Outcome: "success",
		Details: input,
	}); err != nil {
		t.Fatalf("append first event: %v", err)
	}
	if err := log.Append(context.Background(), Event{
		Action:  "authorization.check",
		Outcome: "denied",
		Details: map[string]any{"reason": "missing permission"},
	}); err != nil {
		t.Fatalf("append second event: %v", err)
	}

	// Mutating the caller-owned input after Append must not affect persisted
	// audit state.
	input["request"].(map[string]any)["path"] = "/tampered-by-caller"

	firstSnapshot := log.Records()
	if len(firstSnapshot) != 2 {
		t.Fatalf("records length = %d, want 2", len(firstSnapshot))
	}

	request := firstSnapshot[0].Details["request"].(map[string]any)
	if got := request["path"]; got != "/api/v1/example" {
		t.Fatalf("stored request path = %v, want original value", got)
	}

	// Mutate every reference-bearing level returned by Records.
	request["path"] = "/tampered-snapshot"
	labels := request["labels"].([]any)
	labels[0] = "changed"
	labels[1].(map[string]any)["result"] = "changed"
	firstSnapshot[0].Details["new"] = "injected"
	firstSnapshot[1].Details["reason"] = "changed"

	secondSnapshot := log.Records()
	if got := secondSnapshot[0].Details["request"].(map[string]any)["path"]; got != "/api/v1/example" {
		t.Fatalf("second snapshot request path = %v, want original value", got)
	}
	secondLabels := secondSnapshot[0].Details["request"].(map[string]any)["labels"].([]any)
	if got := secondLabels[0]; got != "first" {
		t.Fatalf("second snapshot labels[0] = %v, want first", got)
	}
	if got := secondLabels[1].(map[string]any)["result"]; got != "allowed" {
		t.Fatalf("second snapshot nested result = %v, want allowed", got)
	}
	if _, ok := secondSnapshot[0].Details["new"]; ok {
		t.Fatal("second snapshot contains field injected through earlier snapshot")
	}
	if got := secondSnapshot[1].Details["reason"]; got != "missing permission" {
		t.Fatalf("second event reason = %v, want original value", got)
	}

	if err := Verify(secondSnapshot[0], ""); err != nil {
		t.Fatalf("verify first stored event after snapshot mutation: %v", err)
	}
	if err := Verify(secondSnapshot[1], secondSnapshot[0].Hash); err != nil {
		t.Fatalf("verify second stored event after snapshot mutation: %v", err)
	}
}

func TestMemoryLogRecordsPreservesNilDetails(t *testing.T) {
	log := NewMemoryLog()
	if err := log.Append(context.Background(), Event{
		Action:  "session.logout",
		Outcome: "success",
	}); err != nil {
		t.Fatalf("append event: %v", err)
	}

	records := log.Records()
	if len(records) != 1 {
		t.Fatalf("records length = %d, want 1", len(records))
	}
	if records[0].Details != nil {
		t.Fatalf("details = %#v, want nil", records[0].Details)
	}
	if err := Verify(records[0], ""); err != nil {
		t.Fatalf("verify event: %v", err)
	}
}
