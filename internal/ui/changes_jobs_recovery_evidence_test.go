package ui

import (
	"errors"
	"strings"
	"testing"
	"time"

	"control-center/internal/orchestration/change"
	"control-center/internal/orchestration/operationsview"
)

func TestApplyVerifiedRecoveryEvidenceBindsExactRevision(t *testing.T) {
	now := time.Date(2026, 9, 12, 1, 20, 0, 0, time.UTC)
	view, err := BuildChangesJobsView(ChangesJobsInput{
		Changes:       []change.Snapshot{validChangeSnapshot("change-a", "service.ensure", now.Add(-10*time.Minute))},
		ChangesLoaded: true,
		JobsLoaded:    true,
		Now:           now,
	})
	if err != nil {
		t.Fatal(err)
	}
	digest := "sha256:" + strings.Repeat("a", 64)
	evidence, err := operationsview.BuildRecoveryEvidence(operationsview.RecoveryEvidenceInput{
		RecoveryPlanID:         "recovery-a",
		ChangeID:               "change-a",
		RevisionID:             "revision-a",
		RevisionDigest:         digest,
		Strategy:               operationsview.RecoveryStrategyRollback,
		Readiness:              operationsview.RecoveryReadinessReady,
		ObservedAt:             now.Add(-time.Minute),
		VerificationDigest:     "sha256:" + strings.Repeat("b", 64),
		OperatorActionRequired: false,
	}, now)
	if err != nil {
		t.Fatal(err)
	}

	enriched, err := ApplyVerifiedRecoveryEvidence(view, map[string]string{"revision-a": digest}, []operationsview.RecoveryEvidence{evidence}, true, now)
	if err != nil {
		t.Fatal(err)
	}
	if enriched.Changes[0].RecoveryEvidence != EvidenceAvailable {
		t.Fatalf("recovery evidence=%q", enriched.Changes[0].RecoveryEvidence)
	}
	if view.Changes[0].RecoveryEvidence != EvidenceUnavailable {
		t.Fatalf("source view mutated: %#v", view.Changes[0])
	}
}

func TestApplyVerifiedRecoveryEvidenceRejectsDigestMismatch(t *testing.T) {
	now := time.Date(2026, 9, 12, 1, 20, 0, 0, time.UTC)
	view, err := BuildChangesJobsView(ChangesJobsInput{
		Changes:       []change.Snapshot{validChangeSnapshot("change-a", "service.ensure", now.Add(-10*time.Minute))},
		ChangesLoaded: true,
		JobsLoaded:    true,
		Now:           now,
	})
	if err != nil {
		t.Fatal(err)
	}
	trusted := "sha256:" + strings.Repeat("a", 64)
	evidence, err := operationsview.BuildRecoveryEvidence(operationsview.RecoveryEvidenceInput{
		RecoveryPlanID:         "recovery-a",
		ChangeID:               "change-a",
		RevisionID:             "revision-a",
		RevisionDigest:         "sha256:" + strings.Repeat("b", 64),
		Strategy:               operationsview.RecoveryStrategyRollback,
		Readiness:              operationsview.RecoveryReadinessReady,
		ObservedAt:             now.Add(-time.Minute),
		VerificationDigest:     "sha256:" + strings.Repeat("c", 64),
		OperatorActionRequired: false,
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ApplyVerifiedRecoveryEvidence(view, map[string]string{"revision-a": trusted}, []operationsview.RecoveryEvidence{evidence}, true, now); !errors.Is(err, ErrInvalidChangesJobsView) {
		t.Fatalf("err=%v", err)
	}
}

func TestApplyVerifiedRecoveryEvidenceRejectsRevisionMismatch(t *testing.T) {
	now := time.Date(2026, 9, 12, 1, 20, 0, 0, time.UTC)
	view, err := BuildChangesJobsView(ChangesJobsInput{
		Changes:       []change.Snapshot{validChangeSnapshot("change-a", "service.ensure", now.Add(-10*time.Minute))},
		ChangesLoaded: true,
		JobsLoaded:    true,
		Now:           now,
	})
	if err != nil {
		t.Fatal(err)
	}
	digest := "sha256:" + strings.Repeat("a", 64)
	evidence, err := operationsview.BuildRecoveryEvidence(operationsview.RecoveryEvidenceInput{
		RecoveryPlanID:         "recovery-a",
		ChangeID:               "change-a",
		RevisionID:             "revision-b",
		RevisionDigest:         digest,
		Strategy:               operationsview.RecoveryStrategyRollback,
		Readiness:              operationsview.RecoveryReadinessReady,
		ObservedAt:             now.Add(-time.Minute),
		VerificationDigest:     "sha256:" + strings.Repeat("b", 64),
		OperatorActionRequired: false,
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ApplyVerifiedRecoveryEvidence(view, map[string]string{"revision-b": digest}, []operationsview.RecoveryEvidence{evidence}, true, now); !errors.Is(err, ErrInvalidChangesJobsView) {
		t.Fatalf("err=%v", err)
	}
}

func TestApplyVerifiedRecoveryEvidenceFailsClosedWhenSourceNotLoaded(t *testing.T) {
	now := time.Date(2026, 9, 12, 1, 20, 0, 0, time.UTC)
	view, err := BuildChangesJobsView(ChangesJobsInput{
		Changes:       []change.Snapshot{validChangeSnapshot("change-a", "service.ensure", now.Add(-10*time.Minute))},
		ChangesLoaded: true,
		JobsLoaded:    true,
		Now:           now,
	})
	if err != nil {
		t.Fatal(err)
	}
	closed, err := ApplyVerifiedRecoveryEvidence(view, nil, nil, false, now)
	if err != nil {
		t.Fatal(err)
	}
	if closed.Changes[0].RecoveryEvidence != EvidenceUnavailable {
		t.Fatalf("unloaded recovery evidence was invented: %#v", closed.Changes[0])
	}
}
