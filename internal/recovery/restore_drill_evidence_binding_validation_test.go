package recovery

import (
	"strings"
	"testing"
	"time"
)

func currentRestoreDrillEvidenceBindingFixture(t *testing.T) (RestoreMetadata, RestoreDrillFreshnessAssessment, RestoreDrillEvidenceBinding) {
	t.Helper()

	restore := validRestoreMetadata()
	assessment, err := EvaluateRestoreDrillFreshness(
		restore,
		24*time.Hour,
		restore.Verification.VerifiedAt.Add(2*time.Hour),
	)
	if err != nil {
		t.Fatalf("EvaluateRestoreDrillFreshness() error = %v", err)
	}
	binding, err := BuildRestoreDrillEvidenceBinding(restore, assessment)
	if err != nil {
		t.Fatalf("BuildRestoreDrillEvidenceBinding() error = %v", err)
	}
	return restore, assessment, binding
}

func TestValidateRestoreDrillEvidenceBindingCurrentAcceptsExactBinding(t *testing.T) {
	restore, assessment, binding := currentRestoreDrillEvidenceBindingFixture(t)

	if err := ValidateRestoreDrillEvidenceBindingCurrent(restore, assessment, binding); err != nil {
		t.Fatalf("ValidateRestoreDrillEvidenceBindingCurrent() error = %v", err)
	}
}

func TestValidateRestoreDrillEvidenceBindingCurrentRejectsPersistedBindingTamper(t *testing.T) {
	restore, assessment, original := currentRestoreDrillEvidenceBindingFixture(t)

	tests := []struct {
		name   string
		mutate func(*RestoreDrillEvidenceBinding)
	}{
		{name: "schema version", mutate: func(binding *RestoreDrillEvidenceBinding) {
			binding.SchemaVersion = "recovery.restore-drill-evidence-binding/v999"
		}},
		{name: "binding id", mutate: func(binding *RestoreDrillEvidenceBinding) { binding.BindingID += "-tampered" }},
		{name: "owner scope", mutate: func(binding *RestoreDrillEvidenceBinding) { binding.OwnerScope = "global" }},
		{name: "restore resource version", mutate: func(binding *RestoreDrillEvidenceBinding) { binding.RestoreResourceVersion += ":tampered" }},
		{name: "assessment id", mutate: func(binding *RestoreDrillEvidenceBinding) { binding.AssessmentID += "-tampered" }},
		{name: "evidence digest", mutate: func(binding *RestoreDrillEvidenceBinding) {
			binding.VerificationEvidenceDigest = "sha256:" + strings.Repeat("c", 64)
		}},
		{name: "verified at", mutate: func(binding *RestoreDrillEvidenceBinding) { binding.VerifiedAt = binding.VerifiedAt.Add(time.Second) }},
		{name: "advisory boundary", mutate: func(binding *RestoreDrillEvidenceBinding) { binding.AdvisoryOnly = false }},
		{name: "mutation boundary", mutate: func(binding *RestoreDrillEvidenceBinding) { binding.ProductionMutation = true }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			binding := original
			test.mutate(&binding)
			if err := ValidateRestoreDrillEvidenceBindingCurrent(restore, assessment, binding); err == nil {
				t.Fatal("tampered persisted binding accepted")
			}
		})
	}
}

func TestRestoreDrillEvidenceBindingBindsOwnerScope(t *testing.T) {
	restore, assessment, binding := currentRestoreDrillEvidenceBindingFixture(t)

	otherOwner := restore
	otherOwner.OwnerScope = "global"
	otherBinding, err := BuildRestoreDrillEvidenceBinding(otherOwner, assessment)
	if err != nil {
		t.Fatalf("BuildRestoreDrillEvidenceBinding(other owner) error = %v", err)
	}
	if otherBinding.OwnerScope != otherOwner.OwnerScope {
		t.Fatalf("owner scope = %q, want %q", otherBinding.OwnerScope, otherOwner.OwnerScope)
	}
	if otherBinding.BindingID == binding.BindingID {
		t.Fatalf("owner-scope drift did not change binding identity: %s", binding.BindingID)
	}
	if err := ValidateRestoreDrillEvidenceBindingCurrent(otherOwner, assessment, binding); err == nil {
		t.Fatal("binding accepted across restore ownership boundary")
	}
}

func TestValidateRestoreDrillEvidenceBindingCurrentRejectsRestoreStateDrift(t *testing.T) {
	restore, assessment, binding := currentRestoreDrillEvidenceBindingFixture(t)

	updated := restore
	updated.ResourceVersion = restore.ResourceVersion + ":next"
	if err := ValidateRestoreDrillEvidenceBindingCurrent(updated, assessment, binding); err == nil {
		t.Fatal("binding accepted after restore resource-version drift")
	}
}

func TestValidateRestoreDrillEvidenceBindingCurrentRejectsEvidenceDrift(t *testing.T) {
	restore, assessment, binding := currentRestoreDrillEvidenceBindingFixture(t)

	updated := restore
	updated.Verification.Evidence = append([]EvidenceReference(nil), restore.Verification.Evidence...)
	updated.Verification.Evidence[0].Digest = "sha256:" + strings.Repeat("d", 64)
	if err := ValidateRestoreDrillEvidenceBindingCurrent(updated, assessment, binding); err == nil {
		t.Fatal("binding accepted after verification evidence drift")
	}
}

func TestValidateRestoreDrillEvidenceBindingCurrentRejectsAssessmentDrift(t *testing.T) {
	restore, assessment, binding := currentRestoreDrillEvidenceBindingFixture(t)

	updated := assessment
	updated.State = RestoreDrillStale
	if err := ValidateRestoreDrillEvidenceBindingCurrent(restore, updated, binding); err == nil {
		t.Fatal("binding accepted after freshness assessment drift")
	}
}
