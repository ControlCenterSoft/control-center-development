package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"control-center/internal/orchestration/change"
	orchestrationapi "control-center/internal/orchestration/httpapi"
	"control-center/internal/orchestration/job"
	"control-center/internal/orchestration/policy"
)

func TestPostgresJobCreateIdempotencyBindsRetryBudget(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set; PostgreSQL integration test skipped")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db, err := Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	var jobsTablePresent bool
	if err := db.QueryRowContext(ctx, `SELECT to_regclass('cc_jobs') IS NOT NULL`).Scan(&jobsTablePresent); err != nil || !jobsTablePresent {
		t.Fatalf("cc_jobs migration is required: present=%v err=%v", jobsTablePresent, err)
	}

	repository, err := NewJobRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	state, err := NewOrchestrationState(db)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	suffix := fmt.Sprintf("%d", now.UnixNano())

	revision, err := state.CreateRevision(
		ctx,
		"job-idempotency-budget-test",
		"revision-job-idempotency-budget-"+suffix,
		"revision-job-idempotency-budget-fingerprint-"+suffix,
		json.RawMessage(`{"generation":1}`),
		now,
	)
	if err != nil {
		t.Fatal(err)
	}
	changeID := "change-job-idempotency-budget-" + suffix
	persistedChange := orchestrationapi.PersistedChange{
		Snapshot: change.Snapshot{
			ID:         changeID,
			Action:     "service.ensure",
			Requester:  "job-idempotency-budget-test",
			RevisionID: revision.ID,
			Risk:       policy.RiskMedium,
			State:      change.StateQueued,
			Decision: policy.Decision{
				Effect:   policy.EffectAllow,
				Risk:     policy.RiskMedium,
				Reason:   "integration fixture",
				PolicyID: "job-idempotency-budget-policy",
			},
			Version:   1,
			UpdatedAt: now,
		},
		Input:          json.RawMessage(`{"name":"api"}`),
		IdempotencyKey: "change-job-idempotency-budget-key-" + suffix,
		Fingerprint:    "change-job-idempotency-budget-fingerprint-" + suffix,
	}
	if _, created, err := state.CreateChange(ctx, persistedChange); err != nil || !created {
		t.Fatalf("create change created=%v err=%v", created, err)
	}

	request := job.CreateRequest{
		ID:             "job-idempotency-budget-" + suffix,
		ChangeID:       changeID,
		ActionName:     "service.ensure",
		Input:          json.RawMessage(`{"name":"api"}`),
		IdempotencyKey: "job-idempotency-budget-key-" + suffix,
		MaxAttempts:    3,
		Now:            now,
	}
	created, wasCreated, err := repository.Create(ctx, request)
	if err != nil || !wasCreated {
		t.Fatalf("initial create = %#v created=%v err=%v", created, wasCreated, err)
	}

	exactReplay := request
	exactReplay.ID = "job-idempotency-budget-exact-replay-" + suffix
	replayed, wasCreated, err := repository.Create(ctx, exactReplay)
	if err != nil || wasCreated {
		t.Fatalf("exact replay = %#v created=%v err=%v", replayed, wasCreated, err)
	}
	if replayed.ID != created.ID || replayed.MaxAttempts != created.MaxAttempts {
		t.Fatalf("exact replay returned %#v, want original %#v", replayed, created)
	}

	changedBudget := request
	changedBudget.ID = "job-idempotency-budget-conflict-" + suffix
	changedBudget.MaxAttempts = request.MaxAttempts + 1
	if _, _, err := repository.Create(ctx, changedBudget); !errors.Is(err, job.ErrIdempotencyConflict) {
		t.Fatalf("changed retry budget error = %v, want ErrIdempotencyConflict", err)
	}

	persisted, err := repository.Get(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.MaxAttempts != request.MaxAttempts || persisted.Version != created.Version {
		t.Fatalf("conflicting replay mutated durable job: %#v", persisted)
	}
}
