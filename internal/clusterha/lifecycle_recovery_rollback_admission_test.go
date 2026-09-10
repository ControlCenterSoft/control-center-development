package clusterha

import (
	"errors"
	"reflect"
	"testing"

	"control-center/internal/nodelifecycle"
)

func lifecycleRecoveryRollbackAdmissionFixture(
	t *testing.T,
	standalone bool,
) (LifecycleRecoveryRollbackAdmission, LifecycleRecoveryRollbackAdmissionRequest) {
	t.Helper()
	handoff, observation := lifecycleExecutionRecoveryFixture(t, standalone)
	planRequest := LifecycleRecoveryRollbackPlanRequest{
		RecoveryHandoff:     handoff,
		RecoveryObservation: observation,
	}
	plan, err := BuildLifecycleRecoveryRollbackPlan(planRequest)
	if err != nil {
		t.Fatalf("BuildLifecycleRecoveryRollbackPlan() error = %v", err)
	}
	request := LifecycleRecoveryRollbackAdmissionRequest{
		Plan:        plan,
		PlanRequest: planRequest,
		CurrentCAS: LifecycleCASObservation{
			NodeID:          plan.NodeID,
			State:           plan.ObservedLifecycleState,
			Generation:      plan.ExpectedLifecycleGeneration,
			ResourceVersion: plan.ExpectedLifecycleResourceVersion,
		},
	}
	admission, err := BuildLifecycleRecoveryRollbackAdmission(request)
	if err != nil {
		t.Fatalf("BuildLifecycleRecoveryRollbackAdmission() error = %v", err)
	}
	return admission, request
}

func TestBuildLifecycleRecoveryRollbackAdmissionIsDeterministicAndBounded(t *testing.T) {
	first, request := lifecycleRecoveryRollbackAdmissionFixture(t, false)
	second, err := BuildLifecycleRecoveryRollbackAdmission(request)
	if err != nil {
		t.Fatalf("BuildLifecycleRecoveryRollbackAdmission(second) error = %v", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("rollback admission is not deterministic: first=%+v second=%+v", first, second)
	}
	if first.FromState != nodelifecycle.StateDraining || first.TargetState != nodelifecycle.StateReady ||
		first.TransitionType != nodelifecycle.TransitionDesired {
		t.Fatalf("unexpected rollback boundary: %+v", first)
	}
	if first.ExpectedLifecycleGeneration != request.Plan.ExpectedLifecycleGeneration ||
		first.PlannedLifecycleGeneration != request.Plan.PlannedLifecycleGeneration ||
		first.ExpectedLifecycleResourceVersion != request.Plan.ExpectedLifecycleResourceVersion {
		t.Fatalf("rollback CAS binding changed: %+v", first)
	}
	if !first.RecoveryOnly || !first.RollbackOnly || !first.CASBound || !first.SingleUseByCAS ||
		!first.AuditedChangeJobRequired || !first.ExecutionAuthorized ||
		!first.LifecycleStateMutationAuthorized || first.MembershipMutationAuthorized ||
		first.FailoverAuthorized || first.GenericCommandAuthorized || first.HostMutationAuthorized {
		t.Fatalf("unsafe rollback admission flags: %+v", first)
	}
	if first.MinimumReadyNodes != first.QuorumRequired ||
		first.ReadyNodesObserved < first.MinimumReadyNodes || first.HealthyVotesObserved < first.QuorumRequired {
		t.Fatalf("unsafe quorum/minimum-ready evidence: %+v", first)
	}
	if err := RevalidateLifecycleRecoveryRollbackAdmission(first, request); err != nil {
		t.Fatalf("RevalidateLifecycleRecoveryRollbackAdmission() error = %v", err)
	}
}

func TestBuildLifecycleRecoveryRollbackAdmissionPreservesStandaloneBoundary(t *testing.T) {
	admission, _ := lifecycleRecoveryRollbackAdmissionFixture(t, true)
	if !admission.StandaloneDowntimeBound || admission.QuorumRequired != 1 ||
		admission.MinimumReadyNodes != 1 || admission.HealthyVotesObserved != 1 || admission.ReadyNodesObserved != 1 {
		t.Fatalf("standalone safety boundary changed: %+v", admission)
	}
}

func TestBuildLifecycleRecoveryRollbackAdmissionRejectsLifecycleCASDriftOrReplay(t *testing.T) {
	_, request := lifecycleRecoveryRollbackAdmissionFixture(t, false)
	tests := []struct {
		name   string
		mutate func(*LifecycleCASObservation)
	}{
		{name: "resource-version", mutate: func(cas *LifecycleCASObservation) { cas.ResourceVersion = "rv-rollback-drift" }},
		{name: "state", mutate: func(cas *LifecycleCASObservation) { cas.State = nodelifecycle.StateReady }},
		{name: "generation", mutate: func(cas *LifecycleCASObservation) { cas.Generation++ }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			changed := request
			changed.CurrentCAS = request.CurrentCAS
			test.mutate(&changed.CurrentCAS)
			_, err := BuildLifecycleRecoveryRollbackAdmission(changed)
			if !errors.Is(err, ErrStaleLifecycleRecoveryRollbackAdmission) {
				t.Fatalf("error = %v, want ErrStaleLifecycleRecoveryRollbackAdmission", err)
			}
		})
	}
}

func TestBuildLifecycleRecoveryRollbackAdmissionRejectsHARevisionDrift(t *testing.T) {
	_, request := lifecycleRecoveryRollbackAdmissionFixture(t, false)
	observation := request.PlanRequest.RecoveryObservation
	advanced, err := AdvanceTransitionRevisionEvidence(observation.CurrentEvidence, ReconcilerRevisionObservation{
		Membership:              observation.CurrentMembership,
		ExpectedResourceVersion: observation.CurrentEvidence.Revision().ResourceVersion,
		ResourceVersion:         "rv-ha-rollback-admission-advanced",
	})
	if err != nil {
		t.Fatalf("AdvanceTransitionRevisionEvidence() error = %v", err)
	}
	observation.CurrentEvidence = advanced
	request.PlanRequest.RecoveryObservation = observation
	_, err = BuildLifecycleRecoveryRollbackAdmission(request)
	if !errors.Is(err, ErrStaleLifecycleRecoveryRollbackAdmission) {
		t.Fatalf("error = %v, want ErrStaleLifecycleRecoveryRollbackAdmission", err)
	}
}

func TestRevalidateLifecycleRecoveryRollbackAdmissionRejectsTampering(t *testing.T) {
	admission, request := lifecycleRecoveryRollbackAdmissionFixture(t, false)
	tests := []struct {
		name   string
		mutate func(*LifecycleRecoveryRollbackAdmission)
	}{
		{name: "membership-authority", mutate: func(a *LifecycleRecoveryRollbackAdmission) {
			a.MembershipMutationAuthorized = true
		}},
		{name: "failover-authority", mutate: func(a *LifecycleRecoveryRollbackAdmission) { a.FailoverAuthorized = true }},
		{name: "generic-command-authority", mutate: func(a *LifecycleRecoveryRollbackAdmission) {
			a.GenericCommandAuthorized = true
		}},
		{name: "host-mutation-authority", mutate: func(a *LifecycleRecoveryRollbackAdmission) {
			a.HostMutationAuthorized = true
		}},
		{name: "minimum-ready", mutate: func(a *LifecycleRecoveryRollbackAdmission) {
			a.MinimumReadyNodes = a.QuorumRequired + 1
		}},
		{name: "resource-version", mutate: func(a *LifecycleRecoveryRollbackAdmission) {
			a.ExpectedLifecycleResourceVersion = "rv-tampered"
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			changed := admission
			changed.RequiredEvidence = append([]nodelifecycle.EvidenceCheck(nil), admission.RequiredEvidence...)
			test.mutate(&changed)
			err := RevalidateLifecycleRecoveryRollbackAdmission(changed, request)
			if !errors.Is(err, ErrInvalidLifecycleRecoveryRollbackAdmission) {
				t.Fatalf("error = %v, want ErrInvalidLifecycleRecoveryRollbackAdmission", err)
			}
		})
	}
}

func TestRevalidateLifecycleRecoveryRollbackAdmissionRejectsPlanLineageDrift(t *testing.T) {
	admission, request := lifecycleRecoveryRollbackAdmissionFixture(t, false)
	request.Plan.RollbackPlanID = "chrrp-tampered-lineage"
	err := RevalidateLifecycleRecoveryRollbackAdmission(admission, request)
	if !errors.Is(err, ErrInvalidLifecycleRecoveryRollbackAdmission) {
		t.Fatalf("error = %v, want ErrInvalidLifecycleRecoveryRollbackAdmission", err)
	}
}
