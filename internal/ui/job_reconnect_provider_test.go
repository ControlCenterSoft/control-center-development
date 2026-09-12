package ui

import (
	"context"
	"errors"
	"testing"
	"time"

	"control-center/internal/orchestration/job"
	"control-center/internal/orchestration/operationsview"
)

type reconnectJobReaderStub struct {
	value job.Job
	err   error
}

func (stub reconnectJobReaderStub) Get(context.Context, string) (job.Job, error) {
	return stub.value, stub.err
}

type reconnectTimelineReaderStub struct {
	entries []job.TimelineEntry
	err     error
}

func (stub reconnectTimelineReaderStub) List(context.Context, string) ([]job.TimelineEntry, error) {
	return append([]job.TimelineEntry(nil), stub.entries...), stub.err
}

func TestDurableJobReconnectProviderBuildsCurrentSnapshot(t *testing.T) {
	createdAt := time.Date(2026, 9, 12, 3, 40, 0, 0, time.UTC)
	current := job.Job{
		ID: "job-31", ChangeID: "change-31", ActionName: "service.ensure",
		Status: job.StatusRunning, Attempt: 1, MaxAttempts: 3,
		CreatedAt: createdAt, UpdatedAt: createdAt.Add(2 * time.Minute), Version: 2,
	}
	timeline := []job.TimelineEntry{
		{JobID: current.ID, Event: job.TimelineCreated, Status: job.StatusQueued, Attempt: 0, JobVersion: 1, OccurredAt: createdAt},
		{JobID: current.ID, Event: job.TimelineAttemptStarted, Status: job.StatusRunning, Attempt: 1, JobVersion: 2, OccurredAt: current.UpdatedAt},
	}
	provider := DurableJobReconnectProvider{
		Jobs:     reconnectJobReaderStub{value: current},
		Timeline: reconnectTimelineReaderStub{entries: timeline},
		Now:      func() time.Time { return createdAt.Add(3 * time.Minute) },
	}

	snapshot, err := provider.JobReconnect(context.Background(), current.ID, 1)
	if err != nil {
		t.Fatalf("JobReconnect() error = %v", err)
	}
	if snapshot.State != operationsview.JobReconnectCurrent || len(snapshot.Events) != 1 || snapshot.Events[0].JobVersion != 2 {
		t.Fatalf("unexpected reconnect snapshot: %#v", snapshot)
	}
}

func TestDurableJobReconnectProviderFailsClosedWhenTimelineReadFails(t *testing.T) {
	createdAt := time.Date(2026, 9, 12, 3, 40, 0, 0, time.UTC)
	current := job.Job{
		ID: "job-31", ChangeID: "change-31", ActionName: "service.ensure",
		Status: job.StatusQueued, Attempt: 0, MaxAttempts: 3,
		CreatedAt: createdAt, UpdatedAt: createdAt, Version: 1,
	}
	provider := DurableJobReconnectProvider{
		Jobs:     reconnectJobReaderStub{value: current},
		Timeline: reconnectTimelineReaderStub{err: errors.New("timeline unavailable")},
		Now:      func() time.Time { return createdAt.Add(time.Minute) },
	}

	snapshot, err := provider.JobReconnect(context.Background(), current.ID, 0)
	if err != nil {
		t.Fatalf("JobReconnect() error = %v", err)
	}
	if snapshot.State != operationsview.JobReconnectUnavailable || snapshot.SourceAvailable || !snapshot.ReloadRequired {
		t.Fatalf("timeline failure did not fail closed: %#v", snapshot)
	}
}

func TestDurableJobReconnectProviderRejectsMissingReadBoundary(t *testing.T) {
	provider := DurableJobReconnectProvider{Now: time.Now}
	if _, err := provider.JobReconnect(context.Background(), "job-31", 0); !errors.Is(err, ErrInvalidJobReconnectProvider) {
		t.Fatalf("error = %v, want ErrInvalidJobReconnectProvider", err)
	}
}
