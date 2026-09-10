package clusterha

import (
	"errors"
	"reflect"
	"testing"

	"control-center/internal/nodelifecycle"
)

func lifecycleRecoveryRollbackReconciliationFixture(
	t *testing.T,
	standalone bool,
) (LifecycleRecoveryRollbackReconciliationHandoff, LifecycleRecoveryRollbackReconciliationObservation) {
	t.Helper()
	admission, source := lifecycleRecoveryRollbackAdmissionFixture(t, standalone)
	observation := LifecycleRecoveryRollbackReconciliationObservation{
		Admission:         admission,
		Source:            source,
		ObservedCAS:       source.CurrentCAS,
		CurrentMembership: source.PlanRequest.RecoveryObservation.CurrentMembership,
		CurrentEvidence:   source.PlanRequest.RecoveryObservation.CurrentEvidence,
	}
	handoff, err := BuildLifecycleRecoveryRollbackReconciliationHandoff(observation)
	if err != nil {
		t.Fatalf("BuildLifecycleRecoveryRollbackReconciliationHandoff() error = %v", err)
	}
	return handoff, observation
}

func TestBuildLifecycleRecoveryRollbackReconciliationHandoffNotAppliedRequiresFreshAdmission(t *testing.T) {
	first, observation := lifecycleRecoveryRollbackReconciliationFixture(t, false)
	second, err := BuildLifecycleRecoveryRollbackReconciliationHandoff(observation)
	if err != nil {
		t.Fatalf("BuildLifecycleRecoveryRollbackReconciliationHandoff(second) error = %v", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("reconciliation handoff is not deterministic: first=%+v second=%+v", first, second)
	}
	if first.Outcome != LifecycleRecoveryRollbackNotApplied || !first.SafetyBoundarySatisfied ||
		!first.FreshAdmissionRequired || first.CompletionReceiptRequired || first.ReconciliationRequired {
		t.Fatalf("unexpected not-applied decision: %+v", first)
	}
	if first.RetryAuthorized || first.LifecycleStateMutationAuthorized || first.MembershipMutationAuthorized ||
		first.FailoverAuthorized || first.GenericCommandAuthorized || first.HostMutationAuthorized {
		t.Fatalf("reconciliation handoff granted unsafe authority: %+v", first)
	}
	if err := RevalidateLifecycleRecoveryRollbackReconciliationHandoff(first, observation); err != nil {
		t.Fatalf("RevalidateLifecycleRecoveryRollbackReconciliationHandoff() error = %v", err)
	}
}

func TestBuildLifecycleRecoveryRollbackReconciliationHandoffAppliedRequiresCompletionReceipt(t *testing.T) {
	_, observation := lifecycleRecoveryRollbackReconciliationFixture(t, false)
	observation.ObservedCAS = LifecycleCASObservation{
		NodeID:          observation.Admission.NodeID,
		State:           observation.Admission.TargetState,
		Generation:      observation.Admission.PlannedLifecycleGeneration,
		ResourceVersion: "rv-rollback-applied",
	}
	handoff, err := BuildLifecycleRecoveryRollbackReconciliationHandoff(observation)
	if err != nil {
		t.Fatalf("BuildLifecycleRecoveryRollbackReconciliationHandoff() error = %v", err)
	}
	if handoff.Outcome != LifecycleRecoveryRollbackApplied || !handoff.SafetyBoundarySatisfied ||
		!handoff.CompletionReceiptRequired || handoff.FreshAdmissionRequired || handoff.ReconciliationRequired {
		t.Fatalf("unexpected applied decision: %+v", handoff)
	}
}

func TestBuildLifecycleRecoveryRollbackReconciliationHandoffRoutesAmbiguousOutcomesToReconciliation(t *testing.T) {
	_, observation := lifecycleRecoveryRollbackReconciliationFixture(t, false)
	tests := []LifecycleCASObservation{
		{
			NodeID:          observation.Admission.NodeID,
			State:           observation.Admission.TargetState,
			Generation:      observation.Admission.PlannedLifecycleGeneration,
			ResourceVersion: observation.Admission.ExpectedLifecycleResourceVersion,
		},
		{
			NodeID:          observation.Admission.NodeID,
			State:           observation.Admission.FromState,
			Generation:      observation.Admission.ExpectedLifecycleGeneration,
			ResourceVersion: "rv-rollback-ambiguous",
		},
		{
			NodeID:          observation.Admission.NodeID,
			State:           observation.Admission.TargetState,
			Generation:      observation.Admission.ExpectedLifecycleGeneration,
			ResourceVersion: "rv-rollback-crossed-boundary",
		},
	}
	for _, observed := range tests {
		changed := observation
		changed.ObservedCAS = observed
		handoff, err := BuildLifecycleRecoveryRollbackReconciliationHandoff(changed)
		if err != nil {
			t.Fatalf("BuildLifecycleRecoveryRollbackReconciliationHandoff() error = %v", err)
		}
		if handoff.Outcome != LifecycleRecoveryRollbackAmbiguous || !handoff.ReconciliationRequired ||
			handoff.FreshAdmissionRequired || handoff.CompletionReceiptRequired || handoff.RetryAuthorized {
			t.Fatalf("ambiguous outcome did not fail closed: %+v", handoff)
		}
	}
}

func TestBuildLifecycleRecoveryRollbackReconciliationHandoffRoutesSupersededOutcomesToReconciliation(t *testing.T) {
	_, observation := lifecycleRecoveryRollbackReconciliationFixture(t, false)
	tests := []LifecycleCASObservation{
		{
			NodeID:          "controller-foreign",
			State:           observation.Admission.FromState,
			Generation:      observation.Admission.ExpectedLifecycleGeneration,
			ResourceVersion: observation.Admission.ExpectedLifecycleResourceVersion,
		},
		{
			NodeID:          observation.Admission.NodeID,
			State:           nodelifecycle.StateReady,
			Generation:      observation.Admission.PlannedLifecycleGeneration + 1,
			ResourceVersion: "rv-rollback-superseded",
		},
	}
	for _, observed := range tests {
		changed := observation
		changed.ObservedCAS = observed
		handoff, err := BuildLifecycleRecoveryRollbackReconciliationHandoff(changed)
		if err != nil {
			t.Fatalf("BuildLifecycleRecoveryRollbackReconciliationHandoff() error = %v", err)
		}
		if handoff.Outcome != LifecycleRecoveryRollbackSuperseded || !handoff.ReconciliationRequired ||
			handoff.FreshAdmissionRequired || handoff.CompletionReceiptRequired || handoff.RetryAuthorized {
			t.Fatalf("superseded outcome did not fail closed: %+v", handoff)
		}
	}
}

func TestBuildLifecycleRecoveryRollbackReconciliationHandoffPreservesStandaloneBoundary(t *testing.T) {
	handoff, _ := lifecycleRecoveryRollbackReconciliationFixture(t, true)
	if !handoff.StandaloneDowntimeBound || handoff.QuorumRequired != 1 || handoff.QuorumObserved != 1 ||
		handoff.MinimumReadyNodes != 1 || handoff.HealthyVotesObserved != 1 || handoff.ReadyNodesObserved != 1 ||
		!handoff.SafetyBoundarySatisfied || !handoff.FreshAdmissionRequired {
		t.Fatalf("standalone reconciliation boundary changed: %+v", handoff)
	}
}

func TestBuildLifecycleRecoveryRollbackReconciliationHandoffRejectsHARevisionDrift(t *testing.T) {
	_, observation := lifecycleRecoveryRollbackReconciliationFixture(t, false)
	advanced, err := AdvanceTransitionRevisionEvidence(observation.CurrentEvidence, ReconcilerRevisionObservation{
		Membership:              observation.CurrentMembership,
		ExpectedResourceVersion: observation.CurrentEvidence.Revision().ResourceVersion,
		ResourceVersion:         "rv-ha-rollback-reconciliation-advanced",
	})
	if err != nil {
		t.Fatalf("AdvanceTransitionRevisionEvidence() error = %v", err)
	}
	observation.CurrentEvidence = advanced
	_, err = BuildLifecycleRecoveryRollbackReconciliationHandoff(observation)
	if !errors.Is(err, ErrStaleLifecycleRecoveryRollbackReconciliation) {
		t.Fatalf("error = %v, want ErrStaleLifecycleRecoveryRollbackReconciliation", err)
	}
}

func TestRevalidateLifecycleRecoveryRollbackReconciliationHandoffRejectsTampering(t *testing.T) {
	handoff, observation := lifecycleRecoveryRollbackReconciliationFixture(t, false)
	tests := []struct {
		name   string
		mutate func(*LifecycleRecoveryRollbackReconciliationHandoff)
	}{
		{name: "retry-authority", mutate: func(h *LifecycleRecoveryRollbackReconciliationHandoff) {
			h.RetryAuthorized = true
		}},
		{name: "lifecycle-authority", mutate: func(h *LifecycleRecoveryRollbackReconciliationHandoff) {
			h.LifecycleStateMutationAuthorized = true
		}},
		{name: "failover-authority", mutate: func(h *LifecycleRecoveryRollbackReconciliationHandoff) {
			h.FailoverAuthorized = true
		}},
		{name: "outcome", mutate: func(h *LifecycleRecoveryRollbackReconciliationHandoff) {
			h.Outcome = LifecycleRecoveryRollbackApplied
		}},
		{name: "action", mutate: func(h *LifecycleRecoveryRollbackReconciliationHandoff) {
			h.FreshAdmissionRequired = false
			h.ReconciliationRequired = true
		}},
		{name: "quorum-observed", mutate: func(h *LifecycleRecoveryRollbackReconciliationHandoff) {
			h.QuorumObserved++
		}},
		{name: "observed-resource-version", mutate: func(h *LifecycleRecoveryRollbackReconciliationHandoff) {
			h.ObservedLifecycleResourceVersion = "rv-tampered"
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			changed := handoff
			test.mutate(&changed)
			err := RevalidateLifecycleRecoveryRollbackReconciliationHandoff(changed, observation)
			if !errors.Is(err, ErrInvalidLifecycleRecoveryRollbackReconciliation) {
				t.Fatalf("error = %v, want ErrInvalidLifecycleRecoveryRollbackReconciliation", err)
			}
		})
	}
}

func TestRevalidateLifecycleRecoveryRollbackReconciliationHandoffRejectsSourceLineageDrift(t *testing.T) {
	handoff, observation := lifecycleRecoveryRollbackReconciliationFixture(t, false)
	observation.Source.Plan.RollbackPlanID = "chrrp-tampered-lineage"
	err := RevalidateLifecycleRecoveryRollbackReconciliationHandoff(handoff, observation)
	if !errors.Is(err, ErrInvalidLifecycleRecoveryRollbackReconciliation) {
		t.Fatalf("error = %v, want ErrInvalidLifecycleRecoveryRollbackReconciliation", err)
	}
}
