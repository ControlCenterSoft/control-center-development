package audit

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestAuditRedactionAndChain(t *testing.T) {
	log := NewMemoryLog()
	if err := log.Append(context.Background(), Event{Action: "auth.login", Outcome: "denied", Details: map[string]any{"password": "synthetic-password", "nested": map[string]any{"api_key": "synthetic-key"}, "header": "Bearer synthetic-token"}}); err != nil {
		t.Fatal(err)
	}
	if err := log.Append(context.Background(), Event{Action: "auth.login", Outcome: "success"}); err != nil {
		t.Fatal(err)
	}
	records := log.Records()
	serialized, _ := jsonMarshal(records)
	if strings.Contains(serialized, "synthetic-password") || strings.Contains(serialized, "synthetic-key") || strings.Contains(serialized, "synthetic-token") {
		t.Fatalf("secret leaked in audit: %s", serialized)
	}
	if records[1].PreviousHash != records[0].Hash || records[1].PreviousHash == "" {
		t.Fatal("audit chain was not linked")
	}
}

func TestPrepareRedactsTypedContainers(t *testing.T) {
	type typedPayload struct {
		Token   string            `json:"token"`
		Nested  map[string]string `json:"nested"`
		Headers []string          `json:"headers"`
		Visible string            `json:"visible"`
	}

	prepared, err := Prepare(Event{
		ID:         "event-typed-details",
		OccurredAt: time.Now(),
		Action:     "auth.login",
		Outcome:    "denied",
		Details: map[string]any{
			"payload": typedPayload{
				Token: "typed-secret-token",
				Nested: map[string]string{
					"api_key": "typed-secret-key",
					"visible": "nested-visible",
				},
				Headers: []string{"Bearer typed-bearer-token", "visible-header"},
				Visible: "top-visible",
			},
		},
	}, "")
	if err != nil {
		t.Fatal(err)
	}

	serialized, err := jsonMarshal(prepared.Details)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"typed-secret-token", "typed-secret-key", "typed-bearer-token"} {
		if strings.Contains(serialized, secret) {
			t.Fatalf("typed secret %q leaked in audit details: %s", secret, serialized)
		}
	}
	for _, visible := range []string{"nested-visible", "visible-header", "top-visible"} {
		if !strings.Contains(serialized, visible) {
			t.Fatalf("non-sensitive value %q was not preserved: %s", visible, serialized)
		}
	}
	if err := Verify(prepared, ""); err != nil {
		t.Fatalf("prepared typed audit event did not verify: %v", err)
	}
}

func TestPrepareRejectsUnsupportedAuditDetails(t *testing.T) {
	_, err := Prepare(Event{
		Action:  "audit.unsupported_details",
		Outcome: "denied",
		Details: map[string]any{"callback": func() {}},
	}, "")
	if err == nil {
		t.Fatal("Prepare accepted unsupported audit details")
	}
	if !strings.Contains(err.Error(), "not safely serializable") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRedactUnsupportedAuditDetailsReturnsSafeMarker(t *testing.T) {
	redacted := Redact(map[string]any{"callback": func() {}})
	serialized, err := jsonMarshal(redacted)
	if err != nil {
		t.Fatal(err)
	}
	if serialized != `{"redaction_error":"unsupported_detail_value"}` {
		t.Fatalf("unexpected safe marker: %s", serialized)
	}
}

func TestPrepareRejectsOverBudgetAuditDetails(t *testing.T) {
	tests := []struct {
		name    string
		details map[string]any
	}{
		{
			name:    "string",
			details: map[string]any{"value": strings.Repeat("x", maxAuditDetailStringBytes+1)},
		},
		{
			name:    "collection",
			details: map[string]any{"values": make([]string, maxAuditDetailCollectionItems+1)},
		},
		{
			name:    "nodes",
			details: map[string]any{"left": make([]any, maxAuditDetailCollectionItems), "right": make([]any, maxAuditDetailCollectionItems), "third": make([]any, maxAuditDetailCollectionItems), "fourth": make([]any, maxAuditDetailCollectionItems)},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Prepare(Event{Action: "audit.bounds", Outcome: "denied", Details: tt.details}, "")
			if err == nil {
				t.Fatal("Prepare accepted over-budget audit details")
			}
		})
	}
}

func TestPrepareRejectsExcessiveAuditDetailDepth(t *testing.T) {
	var value any = "leaf"
	for i := 0; i < maxAuditDetailDepth; i++ {
		value = map[string]any{"next": value}
	}
	_, err := Prepare(Event{Action: "audit.bounds", Outcome: "denied", Details: map[string]any{"root": value}}, "")
	if err == nil || !strings.Contains(err.Error(), "depth limit") {
		t.Fatalf("expected depth-limit error, got %v", err)
	}
}

func TestPrepareRejectsOversizedSerializedAuditDetails(t *testing.T) {
	values := make([]string, maxAuditDetailCollectionItems)
	for i := range values {
		values[i] = strings.Repeat("x", 128)
	}
	_, err := Prepare(Event{Action: "audit.bounds", Outcome: "denied", Details: map[string]any{"values": values}}, "")
	if err == nil || !strings.Contains(err.Error(), "byte limit") {
		t.Fatalf("expected byte-limit error, got %v", err)
	}
}

func TestPrepareAcceptsTypicalBoundedAuditDetails(t *testing.T) {
	prepared, err := Prepare(Event{
		Action:  "authorization.check",
		Outcome: "denied",
		Details: map[string]any{
			"permission":     "audit.events.read",
			"scope":          "global",
			"correlation_id": "request-123",
			"metadata":       map[string]any{"attempt": 1, "source": "api"},
		},
	}, "")
	if err != nil {
		t.Fatalf("Prepare rejected typical audit details: %v", err)
	}
	if err := Verify(prepared, ""); err != nil {
		t.Fatalf("bounded audit event did not verify: %v", err)
	}
}

func TestRedactOverBudgetAuditDetailsReturnsSafeMarker(t *testing.T) {
	redacted := Redact(map[string]any{"value": strings.Repeat("x", maxAuditDetailStringBytes+1)})
	serialized, err := jsonMarshal(redacted)
	if err != nil {
		t.Fatal(err)
	}
	if serialized != `{"redaction_error":"unsupported_detail_value"}` {
		t.Fatalf("unexpected safe marker: %s", serialized)
	}
}

func TestPrepareCanonicalizesTimestampBeforeHash(t *testing.T) {
	inputTime := time.Date(2026, 9, 8, 12, 34, 56, 123456789, time.FixedZone("test", 3*60*60))
	previousHash := strings.Repeat("a", maxAuditHashBytes)
	prepared, err := Prepare(Event{ID: "event-1", OccurredAt: inputTime, Action: "identity.login", Outcome: "success"}, previousHash)
	if err != nil {
		t.Fatal(err)
	}
	wantTime := inputTime.UTC().Truncate(time.Microsecond)
	if prepared.OccurredAt != wantTime {
		t.Fatalf("canonical timestamp = %s, want %s", prepared.OccurredAt, wantTime)
	}
	if err := Verify(prepared, previousHash); err != nil {
		t.Fatalf("prepared event did not verify: %v", err)
	}
	postgresRoundTrip := prepared
	postgresRoundTrip.OccurredAt = time.UnixMicro(prepared.OccurredAt.UnixMicro()).UTC()
	if err := Verify(postgresRoundTrip, previousHash); err != nil {
		t.Fatalf("PostgreSQL-microsecond round trip changed hash: %v", err)
	}
}
func TestVerifyRejectsBrokenAuditChain(t *testing.T) {
	first, err := Prepare(Event{ID: "event-1", OccurredAt: time.Now(), Action: "first", Outcome: "success"}, "")
	if err != nil {
		t.Fatal(err)
	}
	second, err := Prepare(Event{ID: "event-2", OccurredAt: time.Now(), Action: "second", Outcome: "success"}, first.Hash)
	if err != nil {
		t.Fatal(err)
	}
	if err := Verify(second, first.Hash); err != nil {
		t.Fatal(err)
	}
	if err := Verify(second, strings.Repeat("b", maxAuditHashBytes)); err == nil {
		t.Fatal("Verify accepted a broken predecessor link")
	}
	second.Outcome = "tampered"
	if err := Verify(second, first.Hash); err == nil {
		t.Fatal("Verify accepted tampered event content")
	}
}

func TestVerifyRejectsUnserializableEvent(t *testing.T) {
	prepared, err := Prepare(Event{ID: "event-1", OccurredAt: time.Now(), Action: "first", Outcome: "success"}, "")
	if err != nil {
		t.Fatal(err)
	}
	prepared.Details = map[string]any{"callback": func() {}}
	if err := Verify(prepared, ""); err == nil {
		t.Fatal("Verify accepted an unserializable audit event")
	}
}

func TestVerifyRejectsOverBudgetPersistedEvent(t *testing.T) {
	prepared, err := Prepare(Event{ID: "event-1", OccurredAt: time.Now(), Action: "first", Outcome: "success"}, "")
	if err != nil {
		t.Fatal(err)
	}
	prepared.Details = map[string]any{"value": strings.Repeat("x", maxAuditDetailStringBytes+1)}
	if err := Verify(prepared, ""); err == nil {
		t.Fatal("Verify accepted over-budget persisted audit details")
	}
}

func jsonMarshal(value any) (string, error) { b, err := json.Marshal(value); return string(b), err }
