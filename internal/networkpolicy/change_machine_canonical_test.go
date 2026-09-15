package networkpolicy

import (
	"errors"
	"testing"
	"time"
)

func TestChangeMachineRejectsNonCanonicalEventEvidenceIdentifiers(t *testing.T) {
	tests := []struct {
		name  string
		event func(time.Time) ChangeEvent
	}{
		{
			name: "event id",
			event: func(now time.Time) ChangeEvent {
				return ChangeEvent{ID: " event-01 ", Type: EventSnapshotCaptured, SnapshotID: "recovery-01", At: now.Add(time.Second)}
			},
		},
		{
			name: "snapshot id",
			event: func(now time.Time) ChangeEvent {
				return ChangeEvent{ID: "event-01", Type: EventSnapshotCaptured, SnapshotID: " recovery-01 ", At: now.Add(time.Second)}
			},
		},
		{
			name: "reason code",
			event: func(now time.Time) ChangeEvent {
				return ChangeEvent{ID: "event-01", Type: EventSnapshotFailed, ReasonCode: " snapshot_failed ", At: now.Add(time.Second)}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			machine, now := newTestChangeMachine(t)
			before := machine.Snapshot()
			if _, err := machine.Apply(test.event(now), before.Version); !errors.Is(err, ErrInvalidChangePlan) {
				t.Fatalf("Apply() error = %v, want ErrInvalidChangePlan", err)
			}
			after := machine.Snapshot()
			if after.State != before.State || after.Version != before.Version || !after.UpdatedAt.Equal(before.UpdatedAt) {
				t.Fatalf("rejected event mutated state: before=%#v after=%#v", before, after)
			}
		})
	}
}

func TestChangeMachineRejectsNonCanonicalProbeIdentifier(t *testing.T) {
	machine, now := newTestChangeMachine(t)
	applyEvent(t, machine, ChangeEvent{ID: "event-01", Type: EventSnapshotCaptured, SnapshotID: "recovery-01", At: now.Add(time.Second)})
	applyEvent(t, machine, ChangeEvent{ID: "event-02", Type: EventPreflightPassed, At: now.Add(2 * time.Second)})
	applyEvent(t, machine, ChangeEvent{ID: "event-03", Type: EventTemporaryApplied, At: now.Add(3 * time.Second)})

	before := machine.Snapshot()
	_, err := machine.Apply(ChangeEvent{
		ID: "event-04", Type: EventProbeFailed, ProbeID: " management-control ",
		ReasonCode: "control_path_lost", At: now.Add(4 * time.Second),
	}, before.Version)
	if !errors.Is(err, ErrInvalidChangePlan) {
		t.Fatalf("Apply() error = %v, want ErrInvalidChangePlan", err)
	}
	after := machine.Snapshot()
	if after.State != before.State || after.Version != before.Version || !after.UpdatedAt.Equal(before.UpdatedAt) {
		t.Fatalf("rejected probe event mutated state: before=%#v after=%#v", before, after)
	}
}
