package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"control-center/internal/orchestration/change"
	orchestrationapi "control-center/internal/orchestration/httpapi"
	"control-center/internal/orchestration/job"
	"control-center/internal/orchestration/policy"
)

func TestPostgresVersionedCancellationClearsRetrySchedule(t *testing.T) {
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
		"cancel-retry-test",
		"revision-cancel-retry-"+suffix,
		"revision-cancel-retry-fingerprint-"+suffix,
		json.RawMessage(`{"generation":1}`),
		now,
	)
	if err != nil {
		t.Fatal(err)
	}
	changeID := "change-cancel-retry-" + suffix
	persistedChange := orchestrationapi.PersistedChange{
		Snapshot: change.Snapshot{
			ID:         changeID,
			Action:     "service.ensure",
			Requester:  "cancel-retry-test",
			RevisionID: revision.ID,
			Risk:       policy.RiskMedium,
			State:      change.StateQueued,
			Decision: policy.Decision{
				Effect:   policy.EffectAllow,
				Risk:     policy.RiskMedium,
				Reason:   "integration fixture",
				PolicyID: "cancel-retry-policy",
			},
			Version:   1,
			UpdatedAt: now,
		},
		Input:          json.RawMessage(`{}`),
		IdempotencyKey: "change-cancel-retry-key-" + suffix,
		Fingerprint:    "change-cancel-retry-fingerprint-" + suffix,
	}
	if _, created, err := state.CreateChange(ctx, persistedChange); err != nil || !created {
		t.Fatalf("create change created=%v err=%v", created, err)
	}

	created, _, err := repository.Create(ctx, job.CreateRequest{
		ID:             "job-cancel-retry-" + suffix,
		ChangeID:       changeID,
		ActionName:     "service.ensure",
		Input:          json.RawMessage(`{}`),
		IdempotencyKey: "cancel-retry-" + suffix,
		MaxAttempts:    3,
		Now:            now,
	})
	if err != nil {
		t.Fatal(err)
	}

	retryAt := now.Add(10 * time.Minute)
	if _, err := db.ExecContext(ctx, `UPDATE cc_jobs SET status='retry_wait',attempt=1,next_attempt_at=$2,last_error='temporary failure',updated_at=$3,version=version+1 WHERE id=$1`, created.ID, retryAt, now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	retried, err := repository.Get(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if retried.Status != job.StatusRetryWait || !retried.NextAttemptAt.Equal(retryAt) {
		t.Fatalf("retry fixture = %#v", retried)
	}

	cancelled, err := repository.RequestCancelIfVersion(ctx, retried.ID, retried.Version, now.Add(2*time.Second))
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
