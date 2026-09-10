package clusterha

import (
	"errors"
	"reflect"
	"testing"

	"control-center/internal/nodelifecycle"
)

func TestBuildLifecycleRecoveryRollbackPlanIsDeterministicAndBounded(t *testing.T) {
	handoff, observation := lifecycleExecutionRecoveryFixture(t, false)
	request := LifecycleRecoveryRollbackPlanRequest{
		RecoveryHandoff:     handoff,
		RecoveryObservation: observation,
	}
	first, err := BuildLifecycleRecoveryRollbackPlan(request)
	if err != nil {
		t.Fatalf("BuildLifecycleRecoveryRollbackPlan() error = %v", err)
	}
	second, err := BuildLifecycleRecoveryRollbackPlan(request)
	if err != nil {
		t.Fatalf("BuildLifecycleRecoveryRollbackPlan(second) error = %v", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("rollback plan is not deterministic: first=%+v second=%+v", first, second)
	}
	if first.ObservedLifecycleState != nodelifecycle.StateDraining ||
		first.RollbackTargetState != nodelifecycle.StateReady ||
		first.RollbackTransitionType != nodelifecycle.TransitionDesired {
		t.Fatalf("unexpected rollback boundary: %+v", first)
	}
	if first.ExpectedLifecycleGeneration != handoff.ObservedLifecycleGeneration ||
		first.PlannedLifecycleGeneration != handoff.ObservedLifecycleGeneration+1 ||
		first.ExpectedLifecycleResourceVersion != handoff.ObservedLifecycleResourceVersion {
		t.Fatalf("rollback CAS binding changed: %+v", first)
	}
	if !first.Accepted || !first.PlanOnly || !first.AuditedChangeJobRequired ||
		first.ExecutionAuthorized || first.LifecycleStateMutationAuthorized ||
		first.MembershipMutationAuthorized || first.FailoverAuthorized ||
		first.GenericCommandAuthorized || first.HostMutationAuthorized {
		t.Fatalf("unsafe rollback plan flags: %+v", first)
	}
	wantEvidence := []nodelifecycle.EvidenceCheck{
		nodelifecycle.CheckOperationCancelled,
		nodelifecycle.CheckSchedulingEnabled,
		nodelifecycle.CheckReadinessPassed,
	}
	if !reflect.DeepEqual(first.RequiredEvidence, wantEvidence) {
		t.Fatalf("required evidence = %v, want %v", first.RequiredEvidence, wantEvidence)
	}
	if err := RevalidateLifecycleRecoveryRollbackPlan(first, request); err != nil {
		t.Fatalf("RevalidateLifecycleRecoveryRollbackPlan() error = %v", err)
	}
}

func TestBuildLifecycleRecoveryRollbackPlanPreservesStandaloneBoundary(t *testing.T) {
	handoff, observation := lifecycleExecutionRecoveryFixture(t, true)
	plan, err := BuildLifecycleRecoveryRollbackPlan(LifecycleRecoveryRollbackPlanRequest{
		RecoveryHandoff:     handoff,
		RecoveryObservation: observation,
	})
	if err != nil {
		t.Fatalf("BuildLifecycleRecoveryRollbackPlan() error = %v", err)
	}
	if !plan.StandaloneDowntimeBound || plan.CurrentQuorum != 1 ||
		plan.CurrentHealthyVotes != 1 || plan.CurrentReadyVotes != 1 {
		t.Fatalf("standalone safety boundary changed: %+v", plan)
	}
}

func TestBuildLifecycleRecoveryRollbackPlanRejectsUnchangedCAS(t *testing.T) {
	admission, source := lifecycleExecutionAdmissionFixture(t, false)
	observation := LifecycleExecutionRecoveryObservation{
		Admission:         admission,
		Source:            source,
		ObservedCAS:       source.LifecycleCAS,
		CurrentMembership: source.CurrentMembership,
		CurrentEvidence:   source.CurrentEvidence,
	}
	handoff, err := BuildLifecycleExecutionRecoveryHandoff(observation)
	if err != nil {
		t.Fatalf("BuildLifecycleExecutionRecoveryHandoff() error = %v", err)
	}
	_, err = BuildLifecycleRecoveryRollbackPlan(LifecycleRecoveryRollbackPlanRequest{
		RecoveryHandoff:     handoff,
		RecoveryObservation: observation,
	})
	if !errors.Is(err, ErrLifecycleRecoveryRollbackNotRequired) {
		t.Fatalf("error = %v, want ErrLifecycleRecoveryRollbackNotRequired", err)
	}
}

func TestBuildLifecycleRecoveryRollbackPlanRejectsRecoveryObservationDrift(t *testing.T) {
	handoff, observation := lifecycleExecutionRecoveryFixture(t, false)
	observation.ObservedCAS.ResourceVersion = "rv-lifecycle-recovery-drift"
	_, err := BuildLifecycleRecoveryRollbackPlan(LifecycleRecoveryRollbackPlanRequest{
		RecoveryHandoff:     handoff,
		RecoveryObservation: observation,
	})
	if !errors.Is(err, ErrStaleLifecycleRecoveryRollbackPlan) {
		t.Fatalf("error = %v, want ErrStaleLifecycleRecoveryRollbackPlan", err)
	}
}

func TestBuildLifecycleRecoveryRollbackPlanRejectsHARevisionDrift(t *testing.T) {
	handoff, observation := lifecycleExecutionRecoveryFixture(t, false)
	advanced, err := AdvanceTransitionRevisionEvidence(observation.CurrentEvidence, ReconcilerRevisionObservation{
		Membership:              observation.CurrentMembership,
		ExpectedResourceVersion: observation.CurrentEvidence.Revision().ResourceVersion,
		ResourceVersion:         "rv-ha-rollback-plan-advanced",
	})
	if err != nil {
		t.Fatalf("AdvanceTransitionRevisionEvidence() error = %v", err)
	}
	observation.CurrentEvidence = advanced
	_, err = BuildLifecycleRecoveryRollbackPlan(LifecycleRecoveryRollbackPlanRequest{
		RecoveryHandoff:     handoff,
		RecoveryObservation: observation,
	})
	if !errors.Is(err, ErrStaleLifecycleRecoveryRollbackPlan) {
		t.Fatalf("error = %v, want ErrStaleLifecycleRecoveryRollbackPlan", err)
	}
}

func TestRevalidateLifecycleRecoveryRollbackPlanRejectsTampering(t *testing.T) {
	handoff, observation := lifecycleExecutionRecoveryFixture(t, false)
	request := LifecycleRecoveryRollbackPlanRequest{
		RecoveryHandoff:     handoff,
		RecoveryObservation: observation,
	}
	plan, err := BuildLifecycleRecoveryRollbackPlan(request)
	if err != nil {
		t.Fatalf("BuildLifecycleRecoveryRollbackPlan() error = %v", err)
	}
	tests := []struct {
		name   string
		mutate func(*LifecycleRecoveryRollbackPlan)
	}{
		{name: "execution-authority", mutate: func(p *LifecycleRecoveryRollbackPlan) { p.ExecutionAuthorized = true }},
		{name: "failover-authority", mutate: func(p *LifecycleRecoveryRollbackPlan) { p.FailoverAuthorized = true }},
		{name: "resource-version", mutate: func(p *LifecycleRecoveryRollbackPlan) {
			p.ExpectedLifecycleResourceVersion = "rv-tampered"
		}},
		{name: "minimum-ready", mutate: func(p *LifecycleRecoveryRollbackPlan) {
			p.CurrentReadyVotes = p.CurrentQuorum - 1
		}},
		{name: "required-evidence", mutate: func(p *LifecycleRecoveryRollbackPlan) {
			p.RequiredEvidence = append([]nodelifecycle.EvidenceCheck(nil), p.RequiredEvidence...)
			p.RequiredEvidence[0] = nodelifecycle.CheckRollbackVerified
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			changed := plan
			changed.RequiredEvidence = append([]nodelifecycle.EvidenceCheck(nil), plan.RequiredEvidence...)
			test.mutate(&changed)
			err := RevalidateLifecycleRecoveryRollbackPlan(changed, request)
			if !errors.Is(err, ErrInvalidLifecycleRecoveryRollbackPlan) {
				t.Fatalf("error = %v, want ErrInvalidLifecycleRecoveryRollbackPlan", err)
			}
		})
	}
}
