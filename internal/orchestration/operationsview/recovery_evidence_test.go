package operationsview

import (
	"strings"
	"testing"
	"time"
)

func TestBuildRecoveryEvidenceReadyRollback(t *testing.T) {
	now := time.Date(2026, 9, 12, 1, 0, 0, 0, time.UTC)
	evidence, err := BuildRecoveryEvidence(RecoveryEvidenceInput{
		RecoveryPlanID:         "recovery-31",
		ChangeID:               "change-31",
		RevisionID:             "revision-31",
		RevisionDigest:         "sha256:" + strings.Repeat("a", 64),
		Strategy:               RecoveryStrategyRollback,
		Readiness:              RecoveryReadinessReady,
		ObservedAt:             now.Add(-time.Minute),
		VerificationDigest:     "sha256:" + strings.Repeat("b", 64),
		OperatorActionRequired: false,
	}, now)
	if err != nil {
		t.Fatalf("BuildRecoveryEvidence() error = %v", err)
	}
	if evidence.ValidatedAt != now || evidence.ObservedAt != now.Add(-time.Minute) {
		t.Fatalf("unexpected timestamps: %#v", evidence)
	}
	if evidence.ContractVersion != RecoveryEvidenceContractVersion {
		t.Fatalf("unexpected contract version %q", evidence.ContractVersion)
	}
}

func TestBuildRecoveryEvidenceRejectsFutureObservation(t *testing.T) {
	now := time.Date(2026, 9, 12, 1, 0, 0, 0, time.UTC)
	_, err := BuildRecoveryEvidence(RecoveryEvidenceInput{
		RecoveryPlanID:         "recovery-31",
		ChangeID:               "change-31",
		RevisionID:             "revision-31",
		RevisionDigest:         "sha256:" + strings.Repeat("a", 64),
		Strategy:               RecoveryStrategyRollback,
		Readiness:              RecoveryReadinessReady,
		ObservedAt:             now.Add(time.Second),
		VerificationDigest:     "sha256:" + strings.Repeat("b", 64),
		OperatorActionRequired: false,
	}, now)
	if err == nil {
		t.Fatal("BuildRecoveryEvidence() expected future-dated evidence error")
	}
}

func TestValidateRecoveryEvidenceRequiresOperatorForManualRecovery(t *testing.T) {
	now := time.Date(2026, 9, 12, 1, 0, 0, 0, time.UTC)
	evidence := RecoveryEvidence{
		ContractVersion:        RecoveryEvidenceContractVersion,
		RecoveryPlanID:         "recovery-31",
		ChangeID:               "change-31",
		RevisionID:             "revision-31",
		RevisionDigest:         "sha256:" + strings.Repeat("a", 64),
		Strategy:               RecoveryStrategyManualRecovery,
		Readiness:              RecoveryReadinessReady,
		ObservedAt:             now.Add(-time.Minute),
		ValidatedAt:            now,
		VerificationDigest:     "sha256:" + strings.Repeat("b", 64),
		OperatorActionRequired: false,
	}
	if err := ValidateRecoveryEvidence(evidence); err == nil {
		t.Fatal("ValidateRecoveryEvidence() expected manual recovery operator-action error")
	}
}

func TestValidateRecoveryEvidenceRequiresOperatorForNonReadyState(t *testing.T) {
	now := time.Date(2026, 9, 12, 1, 0, 0, 0, time.UTC)
	for _, readiness := range []RecoveryReadiness{RecoveryReadinessDegraded, RecoveryReadinessUnavailable} {
		evidence := RecoveryEvidence{
			ContractVersion:        RecoveryEvidenceContractVersion,
			RecoveryPlanID:         "recovery-31",
			ChangeID:               "change-31",
			RevisionID:             "revision-31",
			RevisionDigest:         "sha256:" + strings.Repeat("a", 64),
			Strategy:               RecoveryStrategyRollback,
			Readiness:              readiness,
			ObservedAt:             now.Add(-time.Minute),
			ValidatedAt:            now,
			VerificationDigest:     "sha256:" + strings.Repeat("b", 64),
			OperatorActionRequired: false,
		}
		if err := ValidateRecoveryEvidence(evidence); err == nil {
			t.Fatalf("ValidateRecoveryEvidence() readiness %q expected operator-action error", readiness)
		}
	}
}

func TestValidateRecoveryEvidenceRejectsNonCanonicalDigest(t *testing.T) {
	now := time.Date(2026, 9, 12, 1, 0, 0, 0, time.UTC)
	evidence := RecoveryEvidence{
		ContractVersion:        RecoveryEvidenceContractVersion,
		RecoveryPlanID:         "recovery-31",
		ChangeID:               "change-31",
		RevisionID:             "revision-31",
		RevisionDigest:         "SHA256:" + strings.Repeat("A", 64),
		Strategy:               RecoveryStrategyRollback,
		Readiness:              RecoveryReadinessReady,
		ObservedAt:             now.Add(-time.Minute),
		ValidatedAt:            now,
		VerificationDigest:     "sha256:" + strings.Repeat("b", 64),
		OperatorActionRequired: false,
	}
	if err := ValidateRecoveryEvidence(evidence); err == nil {
		t.Fatal("ValidateRecoveryEvidence() expected digest validation error")
	}
}
