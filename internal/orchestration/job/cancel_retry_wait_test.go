package job_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"control-center/internal/orchestration/events"
	"control-center/internal/orchestration/job"
)

func TestVersionedCancellationClearsRetrySchedule(t *testing.T) {
	repository := job.NewMemoryRepository()
	ctx := context.Background()
	now := time.Unix(1_000, 0).UTC()

	created, _, err := repository.Create(ctx, job.CreateRequest{
		ID:             "job-cancel-retry",
		ChangeID:       "change-cancel-retry",
		ActionName:     "service.ensure",
		Input:          json.RawMessage(`{}`),
		IdempotencyKey: "cancel-retry",
		MaxAttempts:    3,
		Now:            now,
	})
	if err != nil {
		t.Fatal(err)
	}
	claimed, ok, err := repository.Claim(ctx, "worker", now.Add(time.Second), time.Minute)
	if err != nil || !ok {
		t.Fatalf("claim ok=%v err=%v", ok, err)
	}
	retried, err := repository.Fail(ctx, created.ID, claimed.Lease.Token, "temporary failure", events.Output{}, job.RetryPolicy{BaseDelay: 10 * time.Minute}, now.Add(2*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if retried.Status != job.StatusRetryWait || retried.NextAttemptAt.IsZero() {
		t.Fatalf("retry fixture = %#v", retried)
	}

	cancelled, err := repository.RequestCancelIfVersion(ctx, retried.ID, retried.Version, now.Add(3*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if cancelled.Status != job.StatusCancelled {
		t.Fatalf("status = %s, want %s", cancelled.Status, job.StatusCancelled)
	}
	if !cancelled.NextAttemptAt.IsZero() {
		t.Fatalf("cancelled retry job kept next attempt at %s", cancelled.NextAttemptAt)
	}

	persisted, err := repository.Get(ctx, retried.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !persisted.NextAttemptAt.IsZero() {
		t.Fatalf("persisted cancelled retry job kept next attempt at %s", persisted.NextAttemptAt)
	}
}
