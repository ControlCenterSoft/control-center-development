package networkpolicy

import (
	"errors"
	"testing"
	"time"
)

// Regression guard for the staged-network safety boundary: once rollback is
// required, a stale/late success acknowledgement must never commit or close a
// rollback after the bounded rollback window has expired. The caller must
// record the timeout explicitly so the state closes fail-safe with durable
// evidence instead of accepting an out-of-window success.
func TestChangeMachineRejectsRollbackSuccessAfterDeadline(t *testing.T) {
	machine, now := newTestChangeMachine(t)
	applyEvent(t, machine, ChangeEvent{
		ID: "event-01", Type: EventSnapshotCaptured,
		SnapshotID: "recovery-01", At: now.Add(time.Second),
	})
	applyEvent(t, machine, ChangeEvent{
		ID: "event-02", Type: EventPreflightPassed,
		At: now.Add(2 * time.Second),
	})
	applyEvent(t, machine, ChangeEvent{
		ID: "event-03", Type: EventTemporaryApplied,
		At: now.Add(3 * time.Second),
	})

	rollback := applyEvent(t, machine, ChangeEvent{
		ID: "event-04", Type: EventProbeFailed,
		ProbeID: "management-control", ReasonCode: "control_path_lost",
		At: now.Add(4 * time.Second),
	})
	if rollback.State != ChangeStateRollbackPending {
		t.Fatalf("state = %s, want rollback_pending", rollback.State)
	}

	before := machine.Snapshot()
	lateAt := before.Deadline.Add(time.Nanosecond)
	got, err := machine.Apply(ChangeEvent{
		ID: "event-05", Type: EventRollbackSucceeded, At: lateAt,
	}, before.Version)
	if !errors.Is(err, ErrInvalidChangeTransition) {
		t.Fatalf("late rollback success error = %v, want ErrInvalidChangeTransition", err)
	}
	if got.State != before.State || got.Version != before.Version || got.ReasonCode != before.ReasonCode || !got.Deadline.Equal(before.Deadline) {
		t.Fatalf("late rollback success mutated snapshot: before=%#v after=%#v", before, got)
	}

	closed := applyEvent(t, machine, ChangeEvent{
		ID: "event-06", Type: EventPhaseTimedOut, At: before.Deadline,
	})
	if closed.State != ChangeStateFailedClosed {
		t.Fatalf("state = %s, want failed_closed", closed.State)
	}
	if closed.ReasonCode != "rollback_not_confirmed" {
		t.Fatalf("reason = %q, want rollback_not_confirmed", closed.ReasonCode)
	}
}
