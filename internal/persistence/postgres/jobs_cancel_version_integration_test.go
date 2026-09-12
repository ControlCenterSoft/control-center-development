package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"control-center/internal/orchestration/job"
)

func TestPostgresVersionedCancellationRejectsStaleJobVersion(t *testing.T) {
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
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	now := time.Now().UTC().Truncate(time.Microsecond)
	created, _, err := repository.Create(ctx, job.CreateRequest{
		ID:             "job-cancel-cas-" + suffix,
		ChangeID:       "change-cancel-cas-" + suffix,
		ActionName:     "service.ensure",
		Input:          json.RawMessage(`{}`),
		IdempotencyKey: "cancel-cas-" + suffix,
		MaxAttempts:    3,
		Now:            now,
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := repository.RequestCancelIfVersion(ctx, created.ID, created.Version+1, now.Add(time.Second)); !errors.Is(err, job.ErrVersionConflict) {
		t.Fatalf("stale version error = %v, want ErrVersionConflict", err)
	}
	unchanged, err := repository.Get(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if unchanged.Status != job.StatusQueued || unchanged.Version != created.Version {
		t.Fatalf("stale cancellation mutated job: %#v", unchanged)
	}

	cancelled, err := repository.RequestCancelIfVersion(ctx, created.ID, created.Version, now.Add(2*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if cancelled.Status != job.StatusCancelled || cancelled.Version != created.Version+1 {
		t.Fatalf("unexpected cancellation result: %#v", cancelled)
	}

	if _, err := repository.RequestCancelIfVersion(ctx, created.ID, created.Version, now.Add(3*time.Second)); !errors.Is(err, job.ErrVersionConflict) {
		t.Fatalf("replayed stale cancellation error = %v, want ErrVersionConflict", err)
	}
}
