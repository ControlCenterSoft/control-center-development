package job_test

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"control-center/internal/orchestration/events"
	"control-center/internal/orchestration/job"
)

func claimedJobForTimeValidation(t *testing.T) (*job.MemoryRepository, job.Job, time.Time) {
	t.Helper()
	repository := job.NewMemoryRepository()
	ctx := context.Background()
	now := time.Unix(1_000, 0).UTC()
	created, wasCreated, err := repository.Create(ctx, job.CreateRequest{
		ID:             "job-time-validation",
		ChangeID:       "change-time-validation",
		ActionName:     "service.ensure",
		Input:          json.RawMessage(`{"name":"api"}`),
		IdempotencyKey: "job-time-validation",
		MaxAttempts:    3,
		Now:            now,
	})
	if err != nil || !wasCreated {
		t.Fatalf("create = %#v, %v, created=%v", created, err, wasCreated)
	}
	claimed, ok, err := repository.Claim(ctx, "worker-time-validation", now, time.Minute)
	if err != nil || !ok {
		t.Fatalf("claim = %#v, %v, ok=%v", claimed, err, ok)
	}
	return repository, claimed, now
}

func requireJobUnchanged(t *testing.T, repository *job.MemoryRepository, before job.Job) {
	t.Helper()
	after, err := repository.Get(context.Background(), before.ID)
	if err != nil {
		t.Fatalf("get after rejected mutation: %v", err)
	}
	if !reflect.DeepEqual(after, before) {
		t.Fatalf("job mutated after rejected zero-time transition\nbefore=%#v\nafter=%#v", before, after)
	}
}

func TestJobMutationsRejectZeroTimeWithoutMutation(t *testing.T) {
	t.Run("renew lease", func(t *testing.T) {
		repository, claimed, _ := claimedJobForTimeValidation(t)
		if _, err := repository.RenewLease(context.Background(), claimed.ID, claimed.Lease.Token, time.Time{}, time.Minute); err == nil {
			t.Fatal("zero-time lease renewal must fail closed")
		}
		requireJobUnchanged(t, repository, claimed)
	})

	t.Run("succeed", func(t *testing.T) {
		repository, claimed, _ := claimedJobForTimeValidation(t)
		if _, err := repository.Succeed(context.Background(), claimed.ID, claimed.Lease.Token, events.Output{}, time.Time{}); err == nil {
			t.Fatal("zero-time success transition must fail closed")
		}
		requireJobUnchanged(t, repository, claimed)
	})

	t.Run("fail", func(t *testing.T) {
		repository, claimed, _ := claimedJobForTimeValidation(t)
		if _, err := repository.Fail(context.Background(), claimed.ID, claimed.Lease.Token, "temporary", events.Output{}, job.RetryPolicy{BaseDelay: time.Second}, time.Time{}); err == nil {
			t.Fatal("zero-time failure transition must fail closed")
		}
		requireJobUnchanged(t, repository, claimed)
	})

	t.Run("request cancel", func(t *testing.T) {
		repository, claimed, _ := claimedJobForTimeValidation(t)
		if _, err := repository.RequestCancel(context.Background(), claimed.ID, time.Time{}); err == nil {
			t.Fatal("zero-time cancellation must fail closed")
		}
		requireJobUnchanged(t, repository, claimed)
	})
}
