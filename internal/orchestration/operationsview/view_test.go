package operationsview

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"control-center/internal/orchestration/change"
	"control-center/internal/orchestration/events"
	"control-center/internal/orchestration/job"
	"control-center/internal/orchestration/policy"
)

func TestBuildKeepsOperationalReadModelBoundedAndRedactsJobInternals(t *testing.T) {
	now := time.Date(2026, 9, 12, 1, 45, 0, 0, time.UTC)
	snapshot := testSnapshot(change.StateExecuting, now.Add(-time.Minute))
	execution := testJob(job.StatusRunning, now.Add(-30*time.Second))
	execution.Input = json.RawMessage(`{"password":"do-not-expose"}`)
	execution.IdempotencyKey = "secret-idempotency-value"
	execution.Lease = &job.Lease{Token: "secret-lease-token", WorkerID: "worker-a", ExpiresAt: now.Add(time.Minute)}
	execution.LastError = "backend failed with secret-error-detail"

	view, err := Build(snapshot, &execution, now)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if view.ContractVersion != ContractVersion {
		t.Fatalf("contract version = %q", view.ContractVersion)
	}
	if view.Change.ID != snapshot.ID || view.Job == nil || view.Job.ID != execution.ID || !view.Change.ApprovalsSatisfied {
		t.Fatalf("unexpected linkage or approval evidence: %#v", view)
	}
	if view.Timeline.Completeness != TimelineSnapshotOnly || len(view.Timeline.Items) != 2 {
		t.Fatalf("timeline = %#v", view.Timeline)
	}
	encoded, err := json.Marshal(view)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"do-not-expose", "secret-idempotency-value", "secret-lease-token", "worker-a", "secret-error-detail"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("read model leaked %q: %s", forbidden, encoded)
		}
	}
}

func TestBuildFailsClosedOnMismatchedJobLinkage(t *testing.T) {
	now := time.Date(2026, 9, 12, 1, 46, 0, 0, time.UTC)
	snapshot := testSnapshot(change.StateQueued, now.Add(-time.Minute))
	execution := testJob(job.StatusQueued, now.Add(-30*time.Second))
	execution.ChangeID = "chg-other"

	if _, err := Build(snapshot, &execution, now); err == nil {
		t.Fatal("Build() accepted a job linked to another change")
	}
}

func TestBuildRequiresJobForExecutingStates(t *testing.T) {
	now := time.Date(2026, 9, 12, 1, 47, 0, 0, time.UTC)
	for _, state := range []change.State{change.StateQueued, change.StateExecuting, change.StateVerifying, change.StateSucceeded} {
		t.Run(string(state), func(t *testing.T) {
			if _, err := Build(testSnapshot(state, now.Add(-time.Minute)), nil, now); err == nil {
				t.Fatalf("Build() accepted %s without a linked job", state)
			}
		})
	}
}

func TestBuildMarksSucceededJobWithoutEvidenceForAttention(t *testing.T) {
	now := time.Date(2026, 9, 12, 1, 48, 0, 0, time.UTC)
	snapshot := testSnapshot(change.StateSucceeded, now.Add(-time.Minute))
	execution := testJob(job.StatusSucceeded, now.Add(-30*time.Second))

	view, err := Build(snapshot, &execution, now)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if view.ResultEvidence.Availability != EvidenceUnavailable || !view.AttentionRequired {
		t.Fatalf("missing result evidence was not fail-closed: %#v", view)
	}

	execution.Output = &events.Output{
		Health: []events.Health{{ResourceID: "node-a", Status: events.HealthHealthy, CheckedAt: now.Add(-20 * time.Second)}},
	}
	view, err = Build(snapshot, &execution, now)
	if err != nil {
		t.Fatalf("Build() with evidence error = %v", err)
	}
	if view.ResultEvidence.Availability != EvidenceAvailable || view.ResultEvidence.HealthCount != 1 || view.AttentionRequired {
		t.Fatalf("valid result evidence was not represented: %#v", view)
	}
}

func TestBuildSurfacesTerminalJobReconciliationLag(t *testing.T) {
	now := time.Date(2026, 9, 12, 1, 49, 0, 0, time.UTC)
	snapshot := testSnapshot(change.StateVerifying, now.Add(-time.Minute))
	execution := testJob(job.StatusSucceeded, now.Add(-30*time.Second))
	execution.Output = &events.Output{
		Health: []events.Health{{ResourceID: "node-a", Status: events.HealthHealthy, CheckedAt: now.Add(-20 * time.Second)}},
	}
	view, err := Build(snapshot, &execution, now)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if !view.ReconciliationPending || !view.AttentionRequired {
		t.Fatalf("terminal job reconciliation lag was hidden: %#v", view)
	}
}

func TestBuildRejectsImpossibleTerminalMismatch(t *testing.T) {
	now := time.Date(2026, 9, 12, 1, 50, 0, 0, time.UTC)
	snapshot := testSnapshot(change.StateSucceeded, now.Add(-time.Minute))
	execution := testJob(job.StatusFailed, now.Add(-30*time.Second))
	if _, err := Build(snapshot, &execution, now); err == nil {
		t.Fatal("Build() accepted a succeeded change paired with a failed job")
	}
}

func TestBuildRejectsGeneratedAtBeforeAuthoritativeEvidence(t *testing.T) {
	now := time.Date(2026, 9, 12, 1, 51, 0, 0, time.UTC)
	snapshot := testSnapshot(change.StateExecuting, now)
	execution := testJob(job.StatusRunning, now.Add(time.Second))
	if _, err := Build(snapshot, &execution, now.Add(500*time.Millisecond)); err == nil {
		t.Fatal("Build() accepted generated_at older than job evidence")
	}
}

func TestBuildDoesNotInventHistoryBeforeJobExists(t *testing.T) {
	now := time.Date(2026, 9, 12, 1, 52, 0, 0, time.UTC)
	snapshot := testSnapshot(change.StatePendingApproval, now.Add(-time.Minute))
	view, err := Build(snapshot, nil, now)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if view.Job != nil || len(view.Timeline.Items) != 1 || view.Timeline.Completeness != TimelineSnapshotOnly || view.Change.ApprovalsSatisfied {
		t.Fatalf("unexpected invented execution or approval evidence: %#v", view)
	}
}

func TestBuildRejectsPendingStateAfterApprovalRequirementIsSatisfied(t *testing.T) {
	now := time.Date(2026, 9, 12, 1, 53, 0, 0, time.UTC)
	snapshot := testSnapshot(change.StatePendingApproval, now.Add(-time.Minute))
	snapshot.Approvals = []policy.Approval{{
		Actor: "approver-a", Permissions: []string{"orchestration.changes.approve"}, ApprovedAt: now.Add(-90 * time.Second),
	}}
	if _, err := Build(snapshot, nil, now); err == nil {
		t.Fatal("Build() accepted pending approval state with satisfied approval evidence")
	}
}

func testSnapshot(state change.State, updatedAt time.Time) change.Snapshot {
	approvals := []policy.Approval(nil)
	if state != change.StatePendingApproval && state != change.StateRejected {
		approvals = []policy.Approval{{
			Actor: "approver-a", Permissions: []string{"orchestration.changes.approve"}, ApprovedAt: updatedAt.Add(-time.Minute),
		}}
	}
	return change.Snapshot{
		ID:         "chg-a",
		Action:     "node.update",
		Requester:  "operator-a",
		RevisionID: "rev-a",
		Risk:       policy.RiskHigh,
		State:      state,
		Decision: policy.Decision{
			Effect:   policy.EffectAllow,
			Risk:     policy.RiskHigh,
			Reason:   "qualified test decision",
			PolicyID: "policy-a",
			Requirement: policy.ApprovalRequirement{
				Minimum: 1, Permission: "orchestration.changes.approve", DistinctActors: true, ProhibitRequester: true,
			},
		},
		Approvals: approvals,
		Version:   4,
		UpdatedAt: updatedAt,
	}
}

func testJob(status job.Status, updatedAt time.Time) job.Job {
	return job.Job{
		ID:             "job-a",
		ChangeID:       "chg-a",
		ActionName:     "node.update",
		Input:          json.RawMessage(`{"node":"node-a"}`),
		IdempotencyKey: "change:key-a",
		Status:         status,
		Attempt:        1,
		MaxAttempts:    3,
		CreatedAt:      updatedAt.Add(-time.Minute),
		UpdatedAt:      updatedAt,
		Version:        2,
	}
}
