package audit

import (
	"strings"
	"testing"
	"time"
)

func TestPrepareNormalizesTopLevelAuditFields(t *testing.T) {
	prepared, err := Prepare(Event{
		ID:            "  event-123  ",
		OccurredAt:    time.Now(),
		Action:        "  auth.login  ",
		Outcome:       "  success  ",
		ActorID:       "  actor-123  ",
		SubjectID:     "  subject-123  ",
		SourceIP:      "  2001:db8::1  ",
		CorrelationID: "  request-123  ",
	}, "")
	if err != nil {
		t.Fatal(err)
	}
	if prepared.ID != "event-123" || prepared.Action != "auth.login" || prepared.Outcome != "success" || prepared.ActorID != "actor-123" || prepared.SubjectID != "subject-123" || prepared.SourceIP != "2001:db8::1" || prepared.CorrelationID != "request-123" {
		t.Fatalf("top-level fields were not normalized: %#v", prepared)
	}
	if err := Verify(prepared, ""); err != nil {
		t.Fatalf("normalized event did not verify: %v", err)
	}
}

func TestPrepareRejectsOverBudgetTopLevelAuditFields(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Event)
		want   string
	}{
		{name: "id", mutate: func(event *Event) { event.ID = strings.Repeat("i", maxAuditEventIDBytes+1) }, want: "audit id exceeds"},
		{name: "action", mutate: func(event *Event) { event.Action = strings.Repeat("a", maxAuditActionBytes+1) }, want: "audit action exceeds"},
		{name: "outcome", mutate: func(event *Event) { event.Outcome = strings.Repeat("o", maxAuditOutcomeBytes+1) }, want: "audit outcome exceeds"},
		{name: "actor_id", mutate: func(event *Event) { event.ActorID = strings.Repeat("a", maxAuditActorIDBytes+1) }, want: "audit actor_id exceeds"},
		{name: "subject_id", mutate: func(event *Event) { event.SubjectID = strings.Repeat("s", maxAuditSubjectIDBytes+1) }, want: "audit subject_id exceeds"},
		{name: "source_ip", mutate: func(event *Event) { event.SourceIP = strings.Repeat("1", maxAuditSourceIPBytes+1) }, want: "audit source_ip exceeds"},
		{name: "correlation_id", mutate: func(event *Event) { event.CorrelationID = strings.Repeat("c", maxAuditCorrelationIDBytes+1) }, want: "audit correlation_id exceeds"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := Event{ID: "event-123", Action: "auth.login", Outcome: "success"}
			tt.mutate(&event)
			_, err := Prepare(event, "")
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("expected %q error, got %v", tt.want, err)
			}
		})
	}
}

func TestPrepareAcceptsTopLevelAuditFieldBoundaries(t *testing.T) {
	prepared, err := Prepare(Event{
		ID:            strings.Repeat("i", maxAuditEventIDBytes),
		Action:        strings.Repeat("a", maxAuditActionBytes),
		Outcome:       strings.Repeat("o", maxAuditOutcomeBytes),
		ActorID:       strings.Repeat("a", maxAuditActorIDBytes),
		SubjectID:     strings.Repeat("s", maxAuditSubjectIDBytes),
		SourceIP:      strings.Repeat("1", maxAuditSourceIPBytes),
		CorrelationID: strings.Repeat("c", maxAuditCorrelationIDBytes),
	}, "")
	if err != nil {
		t.Fatalf("Prepare rejected boundary-sized fields: %v", err)
	}
	if err := Verify(prepared, ""); err != nil {
		t.Fatalf("boundary-sized event did not verify: %v", err)
	}
}

func TestPrepareRejectsOversizedPreviousHash(t *testing.T) {
	_, err := Prepare(Event{ID: "event-123", Action: "auth.login", Outcome: "success"}, strings.Repeat("f", maxAuditHashBytes+1))
	if err == nil || !strings.Contains(err.Error(), "previous_hash") {
		t.Fatalf("expected previous_hash bound error, got %v", err)
	}
}

func TestVerifyRejectsOverBudgetPersistedTopLevelAuditField(t *testing.T) {
	prepared, err := Prepare(Event{ID: "event-123", OccurredAt: time.Now(), Action: "auth.login", Outcome: "success"}, "")
	if err != nil {
		t.Fatal(err)
	}
	prepared.SubjectID = strings.Repeat("s", maxAuditSubjectIDBytes+1)
	if err := Verify(prepared, ""); err == nil || !strings.Contains(err.Error(), "subject_id") {
		t.Fatalf("Verify accepted over-budget persisted subject_id: %v", err)
	}
}
