package clusterha

import (
	"errors"
	"reflect"
	"testing"

	"control-center/internal/nodelifecycle"
)

func lifecycleRecoveryRollbackFreshAttemptReconciliationFixture(
	t *testing.T,
	completionOutcome LifecycleRecoveryRollbackFreshAttemptCommitOutcome,
) (
	LifecycleRecoveryRollbackFreshAttemptReconciliationReceipt,
	LifecycleRecoveryRollbackFreshAttemptReconciliationObservation,
) {
	t.Helper()
	completion, completionObservation := lifecycleRecoveryRollbackFreshAttemptCompletionFixture(
		t,
		completionOutcome,
	)
	observation := LifecycleRecoveryRollbackFreshAttemptReconciliationObservation{
		CompletionReceipt:     completion,
		CompletionObservation: completionObservation,
		CurrentCAS:            completionObservation.CASResult.PostCAS,
		CurrentMembership:     completionObservation.CurrentMembership,
		CurrentEvidence:       completionObservation.CurrentEvidence,
	}
	if completionOutcome == LifecycleRecoveryRollbackFreshAttemptAmbiguous {
		observation.CurrentCAS = completionObservation.FinalCAS
	}
	receipt, err := BuildLifecycleRecoveryRollbackFreshAttemptReconciliationReceipt(observation)
	if err != nil {
		t.Fatalf("BuildLifecycleRecoveryRollbackFreshAttemptReconciliationReceipt() error = %v", err)
	}
	return receipt, observation
}

func TestBuildLifecycleRecoveryRollbackFreshAttemptReconciliationDefinitelyNotAppliedIsDeterministic(t *testing.T) {
	first, observation := lifecycleRecoveryRollbackFreshAttemptReconciliationFixture(
		t,
		LifecycleRecoveryRollbackFreshAttemptAmbiguous,
	)
	if first.Outcome != LifecycleRecoveryRollbackFreshAttemptDefinitelyNotApplied ||
		first.CompletionConfirmed || !first.FreshAdmissionRequired || first.ReconciliationRequired ||
		first.FurtherAttemptAuthorized || first.AutomaticRetryAuthorized ||
		first.LifecycleStateMutationAuthorized || first.MembershipMutationAuthorized ||
		first.FailoverAuthorized || first.GenericCommandAuthorized || first.HostMutationAuthorized {
		t.Fatalf("definitely-not-applied reconciliation authority is not bounded: %+v", first)
	}
	for i := 0; i < 100; i++ {
		next, err := BuildLifecycleRecoveryRollbackFreshAttemptReconciliationReceipt(observation)
		if err != nil {
			t.Fatalf("deterministic build %d error = %v", i, err)
		}
		if !reflect.DeepEqual(first, next) {
			t.Fatalf("reconciliation receipt changed at build %d: first=%+v next=%+v", i, first, next)
		}
	}
	if err := RevalidateLifecycleRecoveryRollbackFreshAttemptReconciliationReceipt(first, observation); err != nil {
		t.Fatalf("RevalidateLifecycleRecoveryRollbackFreshAttemptReconciliationReceipt() error = %v", err)
	}
}

func TestBuildLifecycleRecoveryRollbackFreshAttemptReconciliationConfirmsApplied(t *testing.T) {
	completion, completionObservation := lifecycleRecoveryRollbackFreshAttemptCompletionFixture(
		t,
		LifecycleRecoveryRollbackFreshAttemptAmbiguous,
	)
	current := completionObservation.FinalCAS
	current.State = completion.TargetState
	current.Generation = completion.PlannedLifecycleGeneration
	current.ResourceVersion = "rv-fresh-reconciliation-applied"
	observation := LifecycleRecoveryRollbackFreshAttemptReconciliationObservation{
		CompletionReceipt:     completion,
		CompletionObservation: completionObservation,
		CurrentCAS:            current,
		CurrentMembership:     completionObservation.CurrentMembership,
		CurrentEvidence:       completionObservation.CurrentEvidence,
	}
	receipt, err := BuildLifecycleRecoveryRollbackFreshAttemptReconciliationReceipt(observation)
	if err != nil {
		t.Fatalf("BuildLifecycleRecoveryRollbackFreshAttemptReconciliationReceipt() error = %v", err)
	}
	if receipt.Outcome != LifecycleRecoveryRollbackFreshAttemptReconciledApplied ||
		!receipt.CompletionConfirmed || receipt.FreshAdmissionRequired || receipt.ReconciliationRequired ||
		receipt.FurtherAttemptAuthorized || receipt.AutomaticRetryAuthorized ||
		receipt.LifecycleStateMutationAuthorized || receipt.FailoverAuthorized {
		t.Fatalf("applied reconciliation is not evidence-only: %+v", receipt)
	}
}

func TestBuildLifecycleRecoveryRollbackFreshAttemptReconciliationCASLostABARemainsAmbiguous(t *testing.T) {
	completion, completionObservation := lifecycleRecoveryRollbackFreshAttemptCompletionFixture(
		t,
		LifecycleRecoveryRollbackFreshAttemptCASLost,
	)
	observation := LifecycleRecoveryRollbackFreshAttemptReconciliationObservation{
		CompletionReceipt:     completion,
		CompletionObservation: completionObservation,
		CurrentCAS:            completionObservation.FinalCAS,
		CurrentMembership:     completionObservation.CurrentMembership,
		CurrentEvidence:       completionObservation.CurrentEvidence,
	}
	receipt, err := BuildLifecycleRecoveryRollbackFreshAttemptReconciliationReceipt(observation)
	if err != nil {
		t.Fatalf("BuildLifecycleRecoveryRollbackFreshAttemptReconciliationReceipt() error = %v", err)
	}
	if receipt.Outcome != LifecycleRecoveryRollbackFreshAttemptReconciledAmbiguous ||
		receipt.CompletionConfirmed || receipt.FreshAdmissionRequired || !receipt.ReconciliationRequired ||
		receipt.FurtherAttemptAuthorized || receipt.AutomaticRetryAuthorized {
		t.Fatalf("CAS-lost ABA observation was treated as retry-safe: %+v", receipt)
	}
}

func TestBuildLifecycleRecoveryRollbackFreshAttemptReconciliationClassifiesSuperseded(t *testing.T) {
	completion, completionObservation := lifecycleRecoveryRollbackFreshAttemptCompletionFixture(
		t,
		LifecycleRecoveryRollbackFreshAttemptAmbiguous,
	)
	current := completionObservation.FinalCAS
	current.State = nodelifecycle.StateReady
	current.Generation = completion.PreviousLifecycleGeneration
	current.ResourceVersion = "rv-fresh-reconciliation-superseded"
	observation := LifecycleRecoveryRollbackFreshAttemptReconciliationObservation{
		CompletionReceipt:     completion,
		CompletionObservation: completionObservation,
		CurrentCAS:            current,
		CurrentMembership:     completionObservation.CurrentMembership,
		CurrentEvidence:       completionObservation.CurrentEvidence,
	}
	receipt, err := BuildLifecycleRecoveryRollbackFreshAttemptReconciliationReceipt(observation)
	if err != nil {
		t.Fatalf("BuildLifecycleRecoveryRollbackFreshAttemptReconciliationReceipt() error = %v", err)
	}
	if receipt.Outcome != LifecycleRecoveryRollbackFreshAttemptReconciledSuperseded ||
		receipt.CompletionConfirmed || receipt.FreshAdmissionRequired || !receipt.ReconciliationRequired {
		t.Fatalf("competing lifecycle write was not superseded: %+v", receipt)
	}
}

func TestBuildLifecycleRecoveryRollbackFreshAttemptReconciliationRejectsSemanticChangeWithOldResourceVersion(t *testing.T) {
	completion, completionObservation := lifecycleRecoveryRollbackFreshAttemptCompletionFixture(
		t,
		LifecycleRecoveryRollbackFreshAttemptAmbiguous,
	)
	current := completionObservation.FinalCAS
	current.State = nodelifecycle.StateReady
	current.Generation = completion.PreviousLifecycleGeneration
	observation := LifecycleRecoveryRollbackFreshAttemptReconciliationObservation{
		CompletionReceipt:     completion,
		CompletionObservation: completionObservation,
		CurrentCAS:            current,
		CurrentMembership:     completionObservation.CurrentMembership,
		CurrentEvidence:       completionObservation.CurrentEvidence,
	}
	receipt, err := BuildLifecycleRecoveryRollbackFreshAttemptReconciliationReceipt(observation)
	if err != nil {
		t.Fatalf("BuildLifecycleRecoveryRollbackFreshAttemptReconciliationReceipt() error = %v", err)
	}
	if receipt.Outcome != LifecycleRecoveryRollbackFreshAttemptReconciledAmbiguous ||
		!receipt.ReconciliationRequired || receipt.FreshAdmissionRequired || receipt.AutomaticRetryAuthorized {
		t.Fatalf("same-resource-version semantic change is not fail-closed: %+v", receipt)
	}
}

func TestBuildLifecycleRecoveryRollbackFreshAttemptReconciliationTracksAdvancedHARevision(t *testing.T) {
	completion, completionObservation := lifecycleRecoveryRollbackFreshAttemptCompletionFixture(
		t,
		LifecycleRecoveryRollbackFreshAttemptAmbiguous,
	)
	advanced, err := AdvanceTransitionRevisionEvidence(
		completionObservation.CurrentEvidence,
		ReconcilerRevisionObservation{
			Membership:              completionObservation.CurrentMembership,
			ExpectedResourceVersion: completionObservation.CurrentEvidence.Revision().ResourceVersion,
			ResourceVersion:         "rv-fresh-reconciliation-ha-advanced",
		},
	)
	if err != nil {
		t.Fatalf("AdvanceTransitionRevisionEvidence() error = %v", err)
	}
	observation := LifecycleRecoveryRollbackFreshAttemptReconciliationObservation{
		CompletionReceipt:     completion,
		CompletionObservation: completionObservation,
		CurrentCAS:            completionObservation.FinalCAS,
		CurrentMembership:     completionObservation.CurrentMembership,
		CurrentEvidence:       advanced,
	}
	receipt, err := BuildLifecycleRecoveryRollbackFreshAttemptReconciliationReceipt(observation)
	if err != nil {
		t.Fatalf("BuildLifecycleRecoveryRollbackFreshAttemptReconciliationReceipt() error = %v", err)
	}
	if !receipt.HARevisionAdvanced || receipt.Outcome != LifecycleRecoveryRollbackFreshAttemptDefinitelyNotApplied ||
		!receipt.FreshAdmissionRequired || receipt.AutomaticRetryAuthorized {
		t.Fatalf("advanced HA evidence was not preserved safely: %+v", receipt)
	}
}

func TestBuildLifecycleRecoveryRollbackFreshAttemptReconciliationPreservesDegradedSafetyAsEvidenceOnly(t *testing.T) {
	completion, completionObservation := lifecycleRecoveryRollbackFreshAttemptCompletionFixture(
		t,
		LifecycleRecoveryRollbackFreshAttemptAmbiguous,
	)
	degraded := completionObservation.CurrentMembership
	degraded.Members = append([]Member(nil), degraded.Members...)
	for index := range degraded.Members {
		member := &degraded.Members[index]
		if member.ID != completion.NodeID {
			member.Healthy = false
			member.CaughtUp = false
		}
	}
	advanced, err := AdvanceTransitionRevisionEvidence(
		completionObservation.CurrentEvidence,
		ReconcilerRevisionObservation{
			Membership:              degraded,
			ExpectedResourceVersion: completionObservation.CurrentEvidence.Revision().ResourceVersion,
			ResourceVersion:         "rv-fresh-reconciliation-ha-degraded",
		},
	)
	if err != nil {
		t.Fatalf("AdvanceTransitionRevisionEvidence() error = %v", err)
	}
	observation := LifecycleRecoveryRollbackFreshAttemptReconciliationObservation{
		CompletionReceipt:     completion,
		CompletionObservation: completionObservation,
		CurrentCAS:            completionObservation.FinalCAS,
		CurrentMembership:     degraded,
		CurrentEvidence:       advanced,
	}
	receipt, err := BuildLifecycleRecoveryRollbackFreshAttemptReconciliationReceipt(observation)
	if err != nil {
		t.Fatalf("BuildLifecycleRecoveryRollbackFreshAttemptReconciliationReceipt() error = %v", err)
	}
	if receipt.SafetyBoundarySatisfied || !receipt.FreshAdmissionRequired ||
		receipt.FurtherAttemptAuthorized || receipt.AutomaticRetryAuthorized ||
		receipt.LifecycleStateMutationAuthorized || receipt.FailoverAuthorized {
		t.Fatalf("degraded quorum evidence accidentally granted authority: %+v", receipt)
	}
}

func TestBuildLifecycleRecoveryRollbackFreshAttemptReconciliationRejectsNonReconciliationCompletion(t *testing.T) {
	completion, completionObservation := lifecycleRecoveryRollbackFreshAttemptCompletionFixture(
		t,
		LifecycleRecoveryRollbackFreshAttemptApplied,
	)
	_, err := BuildLifecycleRecoveryRollbackFreshAttemptReconciliationReceipt(
		LifecycleRecoveryRollbackFreshAttemptReconciliationObservation{
			CompletionReceipt:     completion,
			CompletionObservation: completionObservation,
			CurrentCAS:            completionObservation.CASResult.PostCAS,
			CurrentMembership:     completionObservation.CurrentMembership,
			CurrentEvidence:       completionObservation.CurrentEvidence,
		},
	)
	if !errors.Is(err, ErrInvalidLifecycleRecoveryRollbackFreshAttemptReconciliation) {
		t.Fatalf("error = %v, want ErrInvalidLifecycleRecoveryRollbackFreshAttemptReconciliation", err)
	}
}

func TestBuildLifecycleRecoveryRollbackFreshAttemptReconciliationRejectsCurrentEvidenceMismatch(t *testing.T) {
	_, observation := lifecycleRecoveryRollbackFreshAttemptReconciliationFixture(
		t,
		LifecycleRecoveryRollbackFreshAttemptAmbiguous,
	)
	tests := []struct {
		name   string
		mutate func(*LifecycleRecoveryRollbackFreshAttemptReconciliationObservation)
	}{
		{name: "wrong-node", mutate: func(o *LifecycleRecoveryRollbackFreshAttemptReconciliationObservation) {
			o.CurrentCAS.NodeID = "controller-other"
		}},
		{name: "membership-without-revision", mutate: func(o *LifecycleRecoveryRollbackFreshAttemptReconciliationObservation) {
			o.CurrentMembership.Members = append([]Member(nil), o.CurrentMembership.Members...)
			o.CurrentMembership.Members[0].Healthy = !o.CurrentMembership.Members[0].Healthy
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			changed := observation
			test.mutate(&changed)
			_, err := BuildLifecycleRecoveryRollbackFreshAttemptReconciliationReceipt(changed)
			if err == nil {
				t.Fatalf("BuildLifecycleRecoveryRollbackFreshAttemptReconciliationReceipt() unexpectedly succeeded")
			}
		})
	}
}

func TestRevalidateLifecycleRecoveryRollbackFreshAttemptReconciliationRejectsAuthorityTampering(t *testing.T) {
	receipt, observation := lifecycleRecoveryRollbackFreshAttemptReconciliationFixture(
		t,
		LifecycleRecoveryRollbackFreshAttemptAmbiguous,
	)
	tests := []struct {
		name   string
		mutate func(*LifecycleRecoveryRollbackFreshAttemptReconciliationReceipt)
	}{
		{name: "further-attempt", mutate: func(r *LifecycleRecoveryRollbackFreshAttemptReconciliationReceipt) {
			r.FurtherAttemptAuthorized = true
		}},
		{name: "automatic-retry", mutate: func(r *LifecycleRecoveryRollbackFreshAttemptReconciliationReceipt) {
			r.AutomaticRetryAuthorized = true
		}},
		{name: "lifecycle-mutation", mutate: func(r *LifecycleRecoveryRollbackFreshAttemptReconciliationReceipt) {
			r.LifecycleStateMutationAuthorized = true
		}},
		{name: "membership-mutation", mutate: func(r *LifecycleRecoveryRollbackFreshAttemptReconciliationReceipt) {
			r.MembershipMutationAuthorized = true
		}},
		{name: "failover", mutate: func(r *LifecycleRecoveryRollbackFreshAttemptReconciliationReceipt) {
			r.FailoverAuthorized = true
		}},
		{name: "generic-command", mutate: func(r *LifecycleRecoveryRollbackFreshAttemptReconciliationReceipt) {
			r.GenericCommandAuthorized = true
		}},
		{name: "host-mutation", mutate: func(r *LifecycleRecoveryRollbackFreshAttemptReconciliationReceipt) {
			r.HostMutationAuthorized = true
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			changed := receipt
			test.mutate(&changed)
			err := RevalidateLifecycleRecoveryRollbackFreshAttemptReconciliationReceipt(changed, observation)
			if !errors.Is(err, ErrInvalidLifecycleRecoveryRollbackFreshAttemptReconciliation) {
				t.Fatalf("error = %v, want ErrInvalidLifecycleRecoveryRollbackFreshAttemptReconciliation", err)
			}
		})
	}
}

func TestRevalidateLifecycleRecoveryRollbackFreshAttemptReconciliationRejectsDecisionTampering(t *testing.T) {
	receipt, observation := lifecycleRecoveryRollbackFreshAttemptReconciliationFixture(
		t,
		LifecycleRecoveryRollbackFreshAttemptAmbiguous,
	)
	changed := receipt
	changed.FreshAdmissionRequired = false
	changed.ReconciliationRequired = true
	err := RevalidateLifecycleRecoveryRollbackFreshAttemptReconciliationReceipt(changed, observation)
	if !errors.Is(err, ErrInvalidLifecycleRecoveryRollbackFreshAttemptReconciliation) {
		t.Fatalf("error = %v, want ErrInvalidLifecycleRecoveryRollbackFreshAttemptReconciliation", err)
	}
}
