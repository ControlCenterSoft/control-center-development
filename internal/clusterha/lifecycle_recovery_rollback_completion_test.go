package clusterha

import (
	"errors"
	"reflect"
	"testing"

	"control-center/internal/nodelifecycle"
)

func lifecycleRecoveryRollbackCompletionFixture(
	t *testing.T,
	standalone bool,
) (LifecycleRecoveryRollbackCompletionReceipt, LifecycleRecoveryRollbackCompletionObservation) {
	t.Helper()
	admission, request := lifecycleRecoveryRollbackAdmissionFixture(t, standalone)
	observation := LifecycleRecoveryRollbackCompletionObservation{
		Admission: admission,
		Source:    request,
		PostCAS: LifecycleCASObservation{
			NodeID:          admission.NodeID,
			State:           admission.TargetState,
			Generation:      admission.PlannedLifecycleGeneration,
			ResourceVersion: "rv-rollback-completed",
		},
		CurrentMembership: request.PlanRequest.RecoveryObservation.CurrentMembership,
		CurrentEvidence:   request.PlanRequest.RecoveryObservation.CurrentEvidence,
	}
	receipt, err := BuildLifecycleRecoveryRollbackCompletionReceipt(observation)
	if err != nil {
		t.Fatalf("BuildLifecycleRecoveryRollbackCompletionReceipt() error = %v", err)
	}
	return receipt, observation
}

func TestBuildLifecycleRecoveryRollbackCompletionReceiptIsDeterministicAndBounded(t *testing.T) {
	first, observation := lifecycleRecoveryRollbackCompletionFixture(t, false)
	second, err := BuildLifecycleRecoveryRollbackCompletionReceipt(observation)
	if err != nil {
		t.Fatalf("BuildLifecycleRecoveryRollbackCompletionReceipt(second) error = %v", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("rollback completion is not deterministic: first=%+v second=%+v", first, second)
	}
	if first.FromState != nodelifecycle.StateDraining || first.TargetState != nodelifecycle.StateReady ||
		first.TransitionType != nodelifecycle.TransitionDesired {
		t.Fatalf("unexpected rollback completion boundary: %+v", first)
	}
	if first.PreviousLifecycleGeneration != observation.Admission.ExpectedLifecycleGeneration ||
		first.AppliedLifecycleGeneration != observation.Admission.PlannedLifecycleGeneration ||
		first.PreviousLifecycleResourceVersion != observation.Admission.ExpectedLifecycleResourceVersion ||
		first.AppliedLifecycleResourceVersion != observation.PostCAS.ResourceVersion {
		t.Fatalf("rollback completion CAS binding changed: %+v", first)
	}
	if !first.CompletionVerified || first.ReconciliationRequired || first.FurtherLifecycleAuthorized ||
		first.MembershipMutationAuthorized || first.FailoverAuthorized || first.GenericCommandAuthorized ||
		first.HostMutationAuthorized {
		t.Fatalf("unsafe rollback completion flags: %+v", first)
	}
	if first.QuorumRequired != observation.Admission.QuorumRequired ||
		first.MinimumReadyNodes != observation.Admission.MinimumReadyNodes ||
		first.HealthyVotesObserved < first.QuorumRequired || first.ReadyNodesObserved < first.MinimumReadyNodes {
		t.Fatalf("unsafe quorum/minimum-ready completion evidence: %+v", first)
	}
	if err := RevalidateLifecycleRecoveryRollbackCompletionReceipt(first, observation); err != nil {
		t.Fatalf("RevalidateLifecycleRecoveryRollbackCompletionReceipt() error = %v", err)
	}
}

func TestBuildLifecycleRecoveryRollbackCompletionReceiptPreservesStandaloneBoundary(t *testing.T) {
	receipt, _ := lifecycleRecoveryRollbackCompletionFixture(t, true)
	if !receipt.StandaloneDowntimeBound || receipt.QuorumRequired != 1 || receipt.MinimumReadyNodes != 1 ||
		receipt.HealthyVotesObserved != 1 || receipt.ReadyNodesObserved != 1 {
		t.Fatalf("standalone completion safety boundary changed: %+v", receipt)
	}
}

func TestBuildLifecycleRecoveryRollbackCompletionReceiptRoutesAmbiguousOutcomeToReconciliation(t *testing.T) {
	_, observation := lifecycleRecoveryRollbackCompletionFixture(t, false)
	tests := []struct {
		name   string
		mutate func(*LifecycleCASObservation)
	}{
		{name: "not-applied", mutate: func(post *LifecycleCASObservation) {
			post.State = observation.Admission.FromState
			post.Generation = observation.Admission.ExpectedLifecycleGeneration
			post.ResourceVersion = observation.Admission.ExpectedLifecycleResourceVersion
		}},
		{name: "generation-drift", mutate: func(post *LifecycleCASObservation) { post.Generation++ }},
		{name: "resource-version-not-advanced", mutate: func(post *LifecycleCASObservation) {
			post.ResourceVersion = observation.Admission.ExpectedLifecycleResourceVersion
		}},
		{name: "foreign-node", mutate: func(post *LifecycleCASObservation) { post.NodeID = "controller-foreign" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			changed := observation
			test.mutate(&changed.PostCAS)
			_, err := BuildLifecycleRecoveryRollbackCompletionReceipt(changed)
			if !errors.Is(err, ErrLifecycleRecoveryRollbackReconciliationRequired) {
				t.Fatalf("error = %v, want ErrLifecycleRecoveryRollbackReconciliationRequired", err)
			}
		})
	}
}

func TestBuildLifecycleRecoveryRollbackCompletionReceiptRejectsHARevisionDrift(t *testing.T) {
	_, observation := lifecycleRecoveryRollbackCompletionFixture(t, false)
	advanced, err := AdvanceTransitionRevisionEvidence(observation.CurrentEvidence, ReconcilerRevisionObservation{
		Membership:              observation.CurrentMembership,
		ExpectedResourceVersion: observation.CurrentEvidence.Revision().ResourceVersion,
		ResourceVersion:         "rv-ha-rollback-completion-advanced",
	})
	if err != nil {
		t.Fatalf("AdvanceTransitionRevisionEvidence() error = %v", err)
	}
	observation.CurrentEvidence = advanced
	_, err = BuildLifecycleRecoveryRollbackCompletionReceipt(observation)
	if !errors.Is(err, ErrStaleLifecycleRecoveryRollbackCompletion) {
		t.Fatalf("error = %v, want ErrStaleLifecycleRecoveryRollbackCompletion", err)
	}
}

func TestRevalidateLifecycleRecoveryRollbackCompletionReceiptRejectsTampering(t *testing.T) {
	receipt, observation := lifecycleRecoveryRollbackCompletionFixture(t, false)
	tests := []struct {
		name   string
		mutate func(*LifecycleRecoveryRollbackCompletionReceipt)
	}{
		{name: "membership-authority", mutate: func(r *LifecycleRecoveryRollbackCompletionReceipt) {
			r.MembershipMutationAuthorized = true
		}},
		{name: "failover-authority", mutate: func(r *LifecycleRecoveryRollbackCompletionReceipt) {
			r.FailoverAuthorized = true
		}},
		{name: "generic-command-authority", mutate: func(r *LifecycleRecoveryRollbackCompletionReceipt) {
			r.GenericCommandAuthorized = true
		}},
		{name: "host-mutation-authority", mutate: func(r *LifecycleRecoveryRollbackCompletionReceipt) {
			r.HostMutationAuthorized = true
		}},
		{name: "reconciliation", mutate: func(r *LifecycleRecoveryRollbackCompletionReceipt) {
			r.ReconciliationRequired = true
		}},
		{name: "applied-resource-version", mutate: func(r *LifecycleRecoveryRollbackCompletionReceipt) {
			r.AppliedLifecycleResourceVersion = "rv-tampered"
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			changed := receipt
			test.mutate(&changed)
			err := RevalidateLifecycleRecoveryRollbackCompletionReceipt(changed, observation)
			if !errors.Is(err, ErrInvalidLifecycleRecoveryRollbackCompletion) {
				t.Fatalf("error = %v, want ErrInvalidLifecycleRecoveryRollbackCompletion", err)
			}
		})
	}
}

func TestRevalidateLifecycleRecoveryRollbackCompletionReceiptRejectsSourceLineageDrift(t *testing.T) {
	receipt, observation := lifecycleRecoveryRollbackCompletionFixture(t, false)
	observation.Source.Plan.RollbackPlanID = "chrrp-tampered-lineage"
	err := RevalidateLifecycleRecoveryRollbackCompletionReceipt(receipt, observation)
	if !errors.Is(err, ErrInvalidLifecycleRecoveryRollbackCompletion) {
		t.Fatalf("error = %v, want ErrInvalidLifecycleRecoveryRollbackCompletion", err)
	}
}
