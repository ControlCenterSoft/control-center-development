package clusterha

import (
	"errors"
	"reflect"
	"testing"

	"control-center/internal/nodelifecycle"
)

func lifecycleRecoveryRollbackFreshAdmissionGateFixture(
	t *testing.T,
	standalone bool,
) (
	LifecycleRecoveryRollbackFreshAdmissionGate,
	LifecycleRecoveryRollbackFreshAdmissionObservation,
) {
	t.Helper()
	handoff, reconciliationObservation := lifecycleRecoveryRollbackReconciliationFixture(t, standalone)
	observation := LifecycleRecoveryRollbackFreshAdmissionObservation{
		ReconciliationHandoff:     handoff,
		ReconciliationObservation: reconciliationObservation,
		CurrentCAS:                reconciliationObservation.ObservedCAS,
		CurrentMembership:         reconciliationObservation.CurrentMembership,
		CurrentEvidence:           reconciliationObservation.CurrentEvidence,
	}
	gate, err := BuildLifecycleRecoveryRollbackFreshAdmissionGate(observation)
	if err != nil {
		t.Fatalf("BuildLifecycleRecoveryRollbackFreshAdmissionGate() error = %v", err)
	}
	return gate, observation
}

func TestBuildLifecycleRecoveryRollbackFreshAdmissionGateIsDeterministicAndNonAuthorizing(t *testing.T) {
	first, observation := lifecycleRecoveryRollbackFreshAdmissionGateFixture(t, false)
	second, err := BuildLifecycleRecoveryRollbackFreshAdmissionGate(observation)
	if err != nil {
		t.Fatalf("BuildLifecycleRecoveryRollbackFreshAdmissionGate(second) error = %v", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("fresh-admission gate is not deterministic: first=%+v second=%+v", first, second)
	}
	if !first.Eligible || !first.FreshAdmissionRequired || first.PreviousAdmissionReusable ||
		first.RetryAuthorized || first.ExecutionAuthorized ||
		first.LifecycleStateMutationAuthorized || first.MembershipMutationAuthorized ||
		first.FailoverAuthorized || first.GenericCommandAuthorized || first.HostMutationAuthorized {
		t.Fatalf("fresh-admission gate granted unsafe authority: %+v", first)
	}
	if first.PreviousRollbackAdmissionID != observation.ReconciliationHandoff.RollbackAdmissionID ||
		first.ReconciliationHandoffID != observation.ReconciliationHandoff.ReconciliationHandoffID {
		t.Fatalf("fresh-admission gate lost reconciliation lineage: %+v", first)
	}
	if err := RevalidateLifecycleRecoveryRollbackFreshAdmissionGate(first, observation); err != nil {
		t.Fatalf("RevalidateLifecycleRecoveryRollbackFreshAdmissionGate() error = %v", err)
	}
}

func TestBuildLifecycleRecoveryRollbackFreshAdmissionGatePreservesStandaloneBoundary(t *testing.T) {
	gate, _ := lifecycleRecoveryRollbackFreshAdmissionGateFixture(t, true)
	if !gate.StandaloneDowntimeBound || gate.QuorumRequired != 1 || gate.QuorumObserved != 1 ||
		gate.MinimumReadyNodes != 1 || gate.HealthyVotesObserved != 1 || gate.ReadyNodesObserved != 1 {
		t.Fatalf("standalone fresh-admission boundary changed: %+v", gate)
	}
}

func TestBuildLifecycleRecoveryRollbackFreshAdmissionGateRejectsAppliedReconciliation(t *testing.T) {
	_, reconciliationObservation := lifecycleRecoveryRollbackReconciliationFixture(t, false)
	reconciliationObservation.ObservedCAS = LifecycleCASObservation{
		NodeID:          reconciliationObservation.Admission.NodeID,
		State:           reconciliationObservation.Admission.TargetState,
		Generation:      reconciliationObservation.Admission.PlannedLifecycleGeneration,
		ResourceVersion: "rv-rollback-applied-before-readmission",
	}
	handoff, err := BuildLifecycleRecoveryRollbackReconciliationHandoff(reconciliationObservation)
	if err != nil {
		t.Fatalf("BuildLifecycleRecoveryRollbackReconciliationHandoff() error = %v", err)
	}
	observation := LifecycleRecoveryRollbackFreshAdmissionObservation{
		ReconciliationHandoff:     handoff,
		ReconciliationObservation: reconciliationObservation,
		CurrentCAS:                reconciliationObservation.ObservedCAS,
		CurrentMembership:         reconciliationObservation.CurrentMembership,
		CurrentEvidence:           reconciliationObservation.CurrentEvidence,
	}
	_, err = BuildLifecycleRecoveryRollbackFreshAdmissionGate(observation)
	if !errors.Is(err, ErrLifecycleRecoveryRollbackFreshAdmissionNotEligible) {
		t.Fatalf("error = %v, want ErrLifecycleRecoveryRollbackFreshAdmissionNotEligible", err)
	}
}

func TestBuildLifecycleRecoveryRollbackFreshAdmissionGateRejectsCurrentCASDrift(t *testing.T) {
	_, observation := lifecycleRecoveryRollbackFreshAdmissionGateFixture(t, false)
	tests := []struct {
		name   string
		mutate func(*LifecycleCASObservation)
	}{
		{name: "resource-version", mutate: func(cas *LifecycleCASObservation) {
			cas.ResourceVersion = "rv-readmission-drift"
		}},
		{name: "state", mutate: func(cas *LifecycleCASObservation) {
			cas.State = nodelifecycle.StateReady
		}},
		{name: "generation", mutate: func(cas *LifecycleCASObservation) {
			cas.Generation++
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			changed := observation
			changed.CurrentCAS = observation.CurrentCAS
			test.mutate(&changed.CurrentCAS)
			_, err := BuildLifecycleRecoveryRollbackFreshAdmissionGate(changed)
			if !errors.Is(err, ErrStaleLifecycleRecoveryRollbackFreshAdmissionGate) {
				t.Fatalf("error = %v, want ErrStaleLifecycleRecoveryRollbackFreshAdmissionGate", err)
			}
		})
	}
}

func TestBuildLifecycleRecoveryRollbackFreshAdmissionGateRejectsHARevisionDrift(t *testing.T) {
	_, observation := lifecycleRecoveryRollbackFreshAdmissionGateFixture(t, false)
	advanced, err := AdvanceTransitionRevisionEvidence(observation.CurrentEvidence, ReconcilerRevisionObservation{
		Membership:              observation.CurrentMembership,
		ExpectedResourceVersion: observation.CurrentEvidence.Revision().ResourceVersion,
		ResourceVersion:         "rv-ha-readmission-advanced",
	})
	if err != nil {
		t.Fatalf("AdvanceTransitionRevisionEvidence() error = %v", err)
	}
	observation.CurrentEvidence = advanced
	_, err = BuildLifecycleRecoveryRollbackFreshAdmissionGate(observation)
	if !errors.Is(err, ErrStaleLifecycleRecoveryRollbackFreshAdmissionGate) {
		t.Fatalf("error = %v, want ErrStaleLifecycleRecoveryRollbackFreshAdmissionGate", err)
	}
}

func TestBuildLifecycleRecoveryRollbackFreshAdmissionGateRejectsQuorumDegradation(t *testing.T) {
	_, observation := lifecycleRecoveryRollbackFreshAdmissionGateFixture(t, false)
	for index := range observation.CurrentMembership.Members {
		if observation.CurrentMembership.Members[index].ID != observation.ReconciliationHandoff.NodeID {
			observation.CurrentMembership.Members[index].Healthy = false
		}
	}
	_, err := BuildLifecycleRecoveryRollbackFreshAdmissionGate(observation)
	if !errors.Is(err, ErrStaleLifecycleRecoveryRollbackFreshAdmissionGate) {
		t.Fatalf("error = %v, want ErrStaleLifecycleRecoveryRollbackFreshAdmissionGate", err)
	}
}

func TestRevalidateLifecycleRecoveryRollbackFreshAdmissionGateRejectsTampering(t *testing.T) {
	gate, observation := lifecycleRecoveryRollbackFreshAdmissionGateFixture(t, false)
	tests := []struct {
		name   string
		mutate func(*LifecycleRecoveryRollbackFreshAdmissionGate)
	}{
		{name: "reuse-old-admission", mutate: func(g *LifecycleRecoveryRollbackFreshAdmissionGate) {
			g.PreviousAdmissionReusable = true
		}},
		{name: "retry-authority", mutate: func(g *LifecycleRecoveryRollbackFreshAdmissionGate) {
			g.RetryAuthorized = true
		}},
		{name: "execution-authority", mutate: func(g *LifecycleRecoveryRollbackFreshAdmissionGate) {
			g.ExecutionAuthorized = true
		}},
		{name: "failover-authority", mutate: func(g *LifecycleRecoveryRollbackFreshAdmissionGate) {
			g.FailoverAuthorized = true
		}},
		{name: "resource-version", mutate: func(g *LifecycleRecoveryRollbackFreshAdmissionGate) {
			g.CurrentLifecycleResourceVersion = "rv-tampered"
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			changed := gate
			test.mutate(&changed)
			err := RevalidateLifecycleRecoveryRollbackFreshAdmissionGate(changed, observation)
			if !errors.Is(err, ErrInvalidLifecycleRecoveryRollbackFreshAdmissionGate) {
				t.Fatalf("error = %v, want ErrInvalidLifecycleRecoveryRollbackFreshAdmissionGate", err)
			}
		})
	}
}

func TestRevalidateLifecycleRecoveryRollbackFreshAdmissionGateRejectsReconciliationLineageDrift(t *testing.T) {
	gate, observation := lifecycleRecoveryRollbackFreshAdmissionGateFixture(t, false)
	observation.ReconciliationObservation.Source.Plan.RollbackPlanID = "chrrp-tampered-readmission-lineage"
	err := RevalidateLifecycleRecoveryRollbackFreshAdmissionGate(gate, observation)
	if !errors.Is(err, ErrInvalidLifecycleRecoveryRollbackFreshAdmissionGate) {
		t.Fatalf("error = %v, want ErrInvalidLifecycleRecoveryRollbackFreshAdmissionGate", err)
	}
}
