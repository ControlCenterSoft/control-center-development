package operationsview

import (
	"errors"
	"testing"
	"time"

	"control-center/internal/orchestration/job"
)

var reconnectTestNow = time.Date(2026, 9, 12, 3, 40, 0, 0, time.UTC)

func reconnectRunningJob(version uint64, updatedAt time.Time) job.Job {
	return job.Job{
		ID:          "job-31",
		ChangeID:    "change-31",
		ActionName:  "service.ensure",
		Status:      job.StatusRunning,
		Attempt:     1,
		MaxAttempts: 3,
		CreatedAt:   reconnectTestNow,
		UpdatedAt:   updatedAt,
		Version:     version,
	}
}

func reconnectTimeline() []job.TimelineEntry {
	return []job.TimelineEntry{
		{
			JobID:      "job-31",
			Event:      job.TimelineCreated,
			Status:     job.StatusQueued,
			Attempt:    0,
			JobVersion: 1,
			OccurredAt: reconnectTestNow,
		},
		{
			JobID:      "job-31",
			Event:      job.TimelineAttemptStarted,
			Status:     job.StatusRunning,
			Attempt:    1,
			JobVersion: 2,
			OccurredAt: reconnectTestNow.Add(time.Minute),
		},
	}
}

func TestBuildJobReconnectSnapshotReturnsBoundedDeltaAcrossLeaseOnlyVersions(t *testing.T) {
	current := reconnectRunningJob(4, reconnectTestNow.Add(4*time.Minute))
	snapshot, err := BuildJobReconnectSnapshot(JobReconnectInput{
		Job:            current,
		Timeline:       reconnectTimeline(),
		TimelineLoaded: true,
		AfterVersion:   1,
		EvaluatedAt:    reconnectTestNow.Add(5 * time.Minute),
	})
	if err != nil {
		t.Fatalf("BuildJobReconnectSnapshot() error = %v", err)
	}
	if snapshot.State != JobReconnectCurrent || snapshot.ReloadRequired {
		t.Fatalf("unexpected reconnect state: %#v", snapshot)
	}
	if snapshot.ResumeVersion != 4 || snapshot.TimelineHeadVersion != 2 || snapshot.TimelineHeadStatus != job.StatusRunning || snapshot.TimelineHeadAttempt != 1 {
		t.Fatalf("unexpected resume/head evidence: %#v", snapshot)
	}
	if len(snapshot.Events) != 1 || snapshot.Events[0].JobVersion != 2 {
		t.Fatalf("unexpected timeline delta: %#v", snapshot.Events)
	}
	if snapshot.ExecutionAuthorized || snapshot.ProductionMutationAllowed {
		t.Fatal("reconnect snapshot granted mutation authority")
	}
	if err := ValidateJobReconnectSnapshot(snapshot); err != nil {
		t.Fatalf("ValidateJobReconnectSnapshot() error = %v", err)
	}
}

func TestBuildJobReconnectSnapshotAcceptsCurrentResumeVersionWithoutLifecycleEvent(t *testing.T) {
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
	if snapshot.State != JobReconnectCurrent || len(snapshot.Events) != 0 || snapshot.ResumeVersion != current.Version {
		t.Fatalf("unexpected current resume snapshot: %#v", snapshot)
	}
	if err := ValidateJobReconnectSnapshot(snapshot); err != nil {
		t.Fatal(err)
	}
}

func TestBuildJobReconnectSnapshotRequiresReloadForFutureCursor(t *testing.T) {
	current := reconnectRunningJob(4, reconnectTestNow.Add(4*time.Minute))
	snapshot, err := BuildJobReconnectSnapshot(JobReconnectInput{
		Job:            current,
		Timeline:       reconnectTimeline(),
		TimelineLoaded: true,
		AfterVersion:   5,
		EvaluatedAt:    reconnectTestNow.Add(5 * time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.State != JobReconnectReloadRequired || !snapshot.ReloadRequired || len(snapshot.Events) != 0 {
		t.Fatalf("future cursor did not require reload: %#v", snapshot)
	}
	if err := ValidateJobReconnectSnapshot(snapshot); err != nil {
		t.Fatal(err)
	}
}

func TestBuildJobReconnectSnapshotFailsClosedWhenTimelineUnavailable(t *testing.T) {
	current := reconnectRunningJob(4, reconnectTestNow.Add(4*time.Minute))
	snapshot, err := BuildJobReconnectSnapshot(JobReconnectInput{
		Job:            current,
		TimelineLoaded: false,
		AfterVersion:   1,
		EvaluatedAt:    reconnectTestNow.Add(5 * time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.State != JobReconnectUnavailable || snapshot.SourceAvailable || !snapshot.ReloadRequired || len(snapshot.Events) != 0 {
		t.Fatalf("unavailable source did not fail closed: %#v", snapshot)
	}
	if err := ValidateJobReconnectSnapshot(snapshot); err != nil {
		t.Fatal(err)
	}
}

func TestBuildJobReconnectSnapshotRejectsTimelineHeadMismatch(t *testing.T) {
	current := reconnectRunningJob(4, reconnectTestNow.Add(4*time.Minute))
	_, err := BuildJobReconnectSnapshot(JobReconnectInput{
		Job:            current,
		Timeline:       reconnectTimeline()[:1],
		TimelineLoaded: true,
		EvaluatedAt:    reconnectTestNow.Add(5 * time.Minute),
	})
	if !errors.Is(err, ErrInvalidJobReconnectSnapshot) {
		t.Fatalf("error = %v, want ErrInvalidJobReconnectSnapshot", err)
	}
}

func TestBuildJobReconnectSnapshotRequiresReloadInsteadOfTruncatingLargeDelta(t *testing.T) {
	timeline := make([]job.TimelineEntry, 0, MaxReconnectTimelineEvents+1)
	for version := 1; version <= MaxReconnectTimelineEvents+1; version++ {
		timeline = append(timeline, job.TimelineEntry{
			JobID:      "job-31",
			Event:      job.TimelineObserved,
			Status:     job.StatusRunning,
			Attempt:    1,
			JobVersion: uint64(version),
			OccurredAt: reconnectTestNow.Add(time.Duration(version) * time.Second),
		})
	}
	current := reconnectRunningJob(uint64(MaxReconnectTimelineEvents+2), reconnectTestNow.Add(time.Duration(MaxReconnectTimelineEvents+2)*time.Second))
	snapshot, err := BuildJobReconnectSnapshot(JobReconnectInput{
		Job:            current,
		Timeline:       timeline,
		TimelineLoaded: true,
		AfterVersion:   0,
		EvaluatedAt:    current.UpdatedAt.Add(time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.State != JobReconnectReloadRequired || !snapshot.ReloadRequired || len(snapshot.Events) != 0 {
		t.Fatalf("large delta was partially returned instead of reload: %#v", snapshot)
	}
	if err := ValidateJobReconnectSnapshot(snapshot); err != nil {
		t.Fatal(err)
	}
}

func TestValidateJobReconnectSnapshotRejectsAuthorityTampering(t *testing.T) {
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
	snapshot.ExecutionAuthorized = true
	if !errors.Is(ValidateJobReconnectSnapshot(snapshot), ErrInvalidJobReconnectSnapshot) {
		t.Fatalf("authority-bearing reconnect snapshot accepted: %#v", snapshot)
	}
}
