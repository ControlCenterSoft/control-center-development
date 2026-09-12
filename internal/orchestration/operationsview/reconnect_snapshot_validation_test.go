package operationsview

import (
	"errors"
	"testing"
	"time"
)

func TestValidateJobReconnectSnapshotRejectsOmittedUnseenHead(t *testing.T) {
	current := reconnectRunningJob(4, reconnectTestNow.Add(4*time.Minute))
	snapshot, err := BuildJobReconnectSnapshot(JobReconnectInput{
		Job:            current,
		Timeline:       reconnectTimeline(),
		TimelineLoaded: true,
		AfterVersion:   1,
		EvaluatedAt:    reconnectTestNow.Add(5 * time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	snapshot.Events = []job.TimelineEntry{}
	if !errors.Is(ValidateJobReconnectSnapshot(snapshot), ErrInvalidJobReconnectSnapshot) {
		t.Fatalf("snapshot that omitted unseen timeline head was accepted: %#v", snapshot)
	}
}

func TestValidateJobReconnectSnapshotRejectsAlreadyObservedDelta(t *testing.T) {
	current := reconnectRunningJob(4, reconnectTestNow.Add(4*time.Minute))
	snapshot, err := BuildJobReconnectSnapshot(JobReconnectInput{
		Job:            current,
		Timeline:       reconnectTimeline(),
		TimelineLoaded: true,
		AfterVersion:   4,
		EvaluatedAt:    reconnectTestNow.Add(5 * time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	snapshot.Events = reconnectTimeline()[1:]
	if !errors.Is(ValidateJobReconnectSnapshot(snapshot), ErrInvalidJobReconnectSnapshot) {
		t.Fatalf("snapshot with already-observed event was accepted: %#v", snapshot)
	}
}
