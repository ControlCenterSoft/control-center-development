package job_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"control-center/internal/orchestration/job"
)

func TestCreateIdempotencyBindsRetryBudget(t *testing.T) {
	repository := job.NewMemoryRepository()
	ctx := context.Background()
	now := time.Unix(100, 0).UTC()

	request := job.CreateRequest{
		ID:             "job-idempotency-budget-1",
		ChangeID:       "change-idempotency-budget",
		ActionName:     "service.ensure",
		Input:          json.RawMessage(`{"name":"api"}`),
		IdempotencyKey: "job-idempotency-budget-key",
		MaxAttempts:    3,
		Now:            now,
	}
	created, wasCreated, err := repository.Create(ctx, request)
	if err != nil || !wasCreated {
		t.Fatalf("initial create = %#v created=%v err=%v", created, wasCreated, err)
	}

	exactReplay := request
	exactReplay.ID = "job-idempotency-budget-exact-replay"
	replayed, wasCreated, err := repository.Create(ctx, exactReplay)
	if err != nil || wasCreated {
		t.Fatalf("exact replay = %#v created=%v err=%v", replayed, wasCreated, err)
	}
	if replayed.ID != created.ID || replayed.MaxAttempts != created.MaxAttempts {
		t.Fatalf("exact replay returned %#v, want original %#v", replayed, created)
	}

	changedBudget := request
	changedBudget.ID = "job-idempotency-budget-conflict"
	changedBudget.MaxAttempts = request.MaxAttempts + 1
	if _, _, err := repository.Create(ctx, changedBudget); !errors.Is(err, job.ErrIdempotencyConflict) {
		t.Fatalf("changed retry budget error = %v, want ErrIdempotencyConflict", err)
	}

	persisted, err := repository.Get(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.MaxAttempts != request.MaxAttempts || persisted.Version != created.Version {
		t.Fatalf("conflicting replay mutated original job: %#v", persisted)
	}
}
