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
	"control-center/internal/orchestration/events"
	orchestrationapi "control-center/internal/orchestration/httpapi"
	"control-center/internal/orchestration/job"
	"control-center/internal/orchestration/policy"
)

const postgresManualRetryDigest = "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func TestPostgresManualRetryCreatesOneAtomicChildLineage(t *testing.T) {
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

	for _, table := range []string{"cc_jobs", "cc_job_manual_retry_lineage"} {
		var present bool
		if err := db.QueryRowContext(ctx, `SELECT to_regclass($1) IS NOT NULL`, table).Scan(&present); err != nil || !present {
			t.Fatalf("%s migration is required: present=%v err=%v", table, present, err)
		}
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
		"manual-retry-test",
		"revision-manual-retry-"+suffix,
		"revision-manual-retry-fingerprint-"+suffix,
		json.RawMessage(`{"generation":1}`),
		now,
	)
	if err != nil {
		t.Fatal(err)
	}
	changeID := "change-manual-retry-" + suffix
	persistedChange := orchestrationapi.PersistedChange{
		Snapshot: change.Snapshot{
			ID:         changeID,
			Action:     "service.ensure",
			Requester:  "manual-retry-test",
			RevisionID: revision.ID,
			Risk:       policy.RiskMedium,
			State:      change.StateQueued,
			Decision: policy.Decision{
				Effect:   policy.EffectAllow,
				Risk:     policy.RiskMedium,
				Reason:   "integration fixture",
				PolicyID: "manual-retry-policy",
			},
			Version:   1,
			UpdatedAt: now,
		},
		Input:          json.RawMessage(`{"target":"node-1"}`),
		IdempotencyKey: "change-manual-retry-key-" + suffix,
		Fingerprint:    "change-manual-retry-fingerprint-" + suffix,
	}
	if _, created, err := state.CreateChange(ctx, persistedChange); err != nil || !created {
		t.Fatalf("create change created=%v err=%v", created, err)
	}

	source, created, err := repository.Create(ctx, job.CreateRequest{
		ID:             "job-manual-retry-source-" + suffix,
		ChangeID:       changeID,
		ActionName:     "service.ensure",
		Input:          json.RawMessage(`{"target":"node-1","private":"must-not-project"}`),
		IdempotencyKey: "source-manual-retry-key-" + suffix,
		MaxAttempts:    1,
		Now:            now,
	})
	if err != nil || !created {
		t.Fatalf("create source created=%v err=%v", created, err)
	}
	persistedChange.JobID = source.ID
	persistedChange.Snapshot.Version = 2
	persistedChange.Snapshot.UpdatedAt = now.Add(time.Millisecond)
	if err := state.UpdateChange(ctx, persistedChange); err != nil {
		t.Fatalf("bind source job: %v", err)
	}

	claimed, ok, err := repository.Claim(ctx, "worker-manual-retry", now.Add(time.Second), time.Minute)
	if err != nil || !ok || claimed.ID != source.ID {
		t.Fatalf("claim source=%#v ok=%v err=%v", claimed, ok, err)
	}
	failed, err := repository.Fail(ctx, claimed.ID, claimed.Lease.Token, "private failure detail", events.Output{}, job.RetryPolicy{}, now.Add(2*time.Second))
	if err != nil || failed.Status != job.StatusFailed {
		t.Fatalf("fail source=%#v err=%v", failed, err)
	}

	request := job.ManualRetryRequest{
		SourceJobID:             failed.ID,
		ExpectedSourceVersion:   failed.Version,
		RetryJobID:              "job-manual-retry-child-" + suffix,
		RetryIdempotencyKey:     "child-manual-retry-key-" + suffix,
		ReviewedAdmissionID:     postgresManualRetryDigest,
		RevalidationAdmissionID: "sha256:abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789",
		RevisionID:              revision.ID,
		RevisionDigest:          postgresManualRetryDigest,
		PolicyID:                "manual-retry-policy",
		PolicyDigest:            postgresManualRetryDigest,
		RetryHistoryDigest:      postgresManualRetryDigest,
		RequestedAt:             now.Add(3 * time.Second),
	}
	retry, lineage, wasCreated, err := repository.CreateManualRetry(ctx, request)
	if err != nil || !wasCreated {
		t.Fatalf("manual retry=%#v lineage=%#v created=%v err=%v", retry, lineage, wasCreated, err)
	}
	if retry.Status != job.StatusQueued || retry.Version != 1 || retry.Attempt != 0 {
		t.Fatalf("unexpected child retry state: %#v", retry)
	}
	if lineage.RootJobID != source.ID || lineage.SourceJobID != source.ID || lineage.SourceJobVersion != failed.Version || lineage.RetryJobID != retry.ID {
		t.Fatalf("unexpected lineage: %#v", lineage)
	}
	unchanged, err := repository.Get(ctx, source.ID)
	if err != nil || unchanged.Version != failed.Version || unchanged.Status != job.StatusFailed {
		t.Fatalf("source changed during manual retry: %#v err=%v", unchanged, err)
	}

	request.RequestedAt = request.RequestedAt.Add(time.Minute)
	replayed, replayedLineage, wasCreated, err := repository.CreateManualRetry(ctx, request)
	if err != nil || wasCreated || replayed.ID != retry.ID || replayedLineage.RetryJobID != retry.ID {
		t.Fatalf("exact replay retry=%#v lineage=%#v created=%v err=%v", replayed, replayedLineage, wasCreated, err)
	}

	second := request
	second.ReviewedAdmissionID = "sha256:1111111111111111111111111111111111111111111111111111111111111111"
	second.RevalidationAdmissionID = "sha256:2222222222222222222222222222222222222222222222222222222222222222"
	second.RetryJobID = "job-manual-retry-duplicate-" + suffix
	second.RetryIdempotencyKey = "duplicate-manual-retry-key-" + suffix
	if _, _, _, err := repository.CreateManualRetry(ctx, second); !errors.Is(err, job.ErrManualRetrySourceAlreadyRetried) {
		t.Fatalf("duplicate source lineage error=%v, want ErrManualRetrySourceAlreadyRetried", err)
	}
}
