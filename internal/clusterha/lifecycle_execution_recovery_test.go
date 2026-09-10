package clusterha

import (
	"errors"
	"testing"
)

func lifecycleExecutionRecoveryFixture(
	t *testing.T,
	standalone bool,
) (LifecycleExecutionRecoveryHandoff, LifecycleExecutionRecoveryObservation) {
	t.Helper()
	admission, source := lifecycleExecutionAdmissionFixture(t, standalone)
	observed := source.LifecycleCAS
	observed.State = admission.TargetState
	observed.Generation = admission.ExpectedLifecycleGeneration
	observed.ResourceVersion = "rv-lifecycle-recovery-8"
	observation := LifecycleExecutionRecoveryObservation{
		Admission:         admission,
		Source:            source,
		ObservedCAS:       observed,
		CurrentMembership: source.CurrentMembership,
		CurrentEvidence:   source.CurrentEvidence,
	}
	handoff, err := BuildLifecycleExecutionRecoveryHandoff(observation)
	if err != nil {
		t.Fatalf("BuildLifecycleExecutionRecoveryHandoff() error = %v", err)
	}
	return handoff, observation
}

func TestBuildLifecycleExecutionRecoveryHandoffIsDeterministicAndBounded(t *testing.T) {
	first, observation := lifecycleExecutionRecoveryFixture(t, false)
	second, err := BuildLifecycleExecutionRecoveryHandoff(observation)
	if err != nil {
		t.Fatalf("BuildLifecycleExecutionRecoveryHandoff(second) error = %v", err)
	}
	if first != second {
		t.Fatalf("recovery handoff is not deterministic: first=%+v second=%+v", first, second)
	}
	if !first.RecoveryAssessmentRequired || !first.RollbackPlanningRequired ||
		first.RecoveryExecutionAuthorized || first.FurtherLifecycleAuthorized ||
		first.MembershipMutationAuthorized || first.FailoverAuthorized ||
		first.GenericCommandAuthorized || first.HostMutationAuthorized {
		t.Fatalf("unsafe recovery flags: %+v", first)
	}
}

func TestBuildLifecycleExecutionRecoveryHandoffPreservesStandaloneBoundary(t *testing.T) {
	handoff, observation := lifecycleExecutionRecoveryFixture(t, true)
	if !handoff.StandaloneDowntimeBound {
		t.Fatalf("standalone downtime evidence was lost: %+v", handoff)
	}
	if err := RevalidateLifecycleExecutionRecoveryHandoff(handoff, observation); err != nil {
		t.Fatalf("RevalidateLifecycleExecutionRecoveryHandoff() error = %v", err)
	}
}

func TestBuildLifecycleExecutionRecoveryHandoffAllowsUnchangedFailedCASWithoutRollbackPlan(t *testing.T) {
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
	if !handoff.RecoveryAssessmentRequired || handoff.RollbackPlanningRequired {
		t.Fatalf("unchanged CAS should require assessment but no rollback plan: %+v", handoff)
	}
}

func TestBuildLifecycleExecutionRecoveryHandoffRejectsExactCompletion(t *testing.T) {
	admission, source := lifecycleExecutionAdmissionFixture(t, false)
	observed := source.LifecycleCAS
	observed.State = admission.TargetState
	observed.Generation = admission.PlannedLifecycleGeneration
	observed.ResourceVersion = "rv-lifecycle-completed-8"
	_, err := BuildLifecycleExecutionRecoveryHandoff(LifecycleExecutionRecoveryObservation{
		Admission:         admission,
		Source:            source,
		ObservedCAS:       observed,
		CurrentMembership: source.CurrentMembership,
		CurrentEvidence:   source.CurrentEvidence,
	})
	if !errors.Is(err, ErrLifecycleExecutionRecoveryNotRequired) {
		t.Fatalf("error = %v, want ErrLifecycleExecutionRecoveryNotRequired", err)
	}
}

func TestBuildLifecycleExecutionRecoveryHandoffRejectsObservedBoundaryEscape(t *testing.T) {
	_, observation := lifecycleExecutionRecoveryFixture(t, false)
	tests := []struct {
		name   string
		mutate func(*LifecycleCASObservation)
	}{
		{name: "node", mutate: func(cas *LifecycleCASObservation) { cas.NodeID = "controller-z" }},
		{name: "generation", mutate: func(cas *LifecycleCASObservation) { cas.Generation += 2 }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			changed := observation
			changed.ObservedCAS = observation.ObservedCAS
			test.mutate(&changed.ObservedCAS)
			_, err := BuildLifecycleExecutionRecoveryHandoff(changed)
			if !errors.Is(err, ErrStaleLifecycleExecutionRecovery) {
				t.Fatalf("error = %v, want ErrStaleLifecycleExecutionRecovery", err)
			}
		})
	}
}

func TestBuildLifecycleExecutionRecoveryHandoffRejectsHAJournalMovement(t *testing.T) {
	_, observation := lifecycleExecutionRecoveryFixture(t, false)
	advanced, err := AdvanceTransitionRevisionEvidence(observation.CurrentEvidence, ReconcilerRevisionObservation{
		Membership:              observation.CurrentMembership,
		ExpectedResourceVersion: observation.CurrentEvidence.Revision().ResourceVersion,
		ResourceVersion:         "rv-ha-recovery-advanced",
	})
	if err != nil {
		t.Fatalf("AdvanceTransitionRevisionEvidence() error = %v", err)
	}
	observation.CurrentEvidence = advanced
	_, err = BuildLifecycleExecutionRecoveryHandoff(observation)
	if !errors.Is(err, ErrStaleLifecycleExecutionRecovery) {
		t.Fatalf("error = %v, want ErrStaleLifecycleExecutionRecovery", err)
	}
}

func TestRevalidateLifecycleExecutionRecoveryHandoffRejectsTampering(t *testing.T) {
	handoff, observation := lifecycleExecutionRecoveryFixture(t, false)
	tests := []struct {
		name   string
		mutate func(*LifecycleExecutionRecoveryHandoff)
	}{
		{name: "authorize-recovery-execution", mutate: func(h *LifecycleExecutionRecoveryHandoff) {
			h.RecoveryExecutionAuthorized = true
		}},
		{name: "authorize-failover", mutate: func(h *LifecycleExecutionRecoveryHandoff) { h.FailoverAuthorized = true }},
		{name: "rollback-requirement", mutate: func(h *LifecycleExecutionRecoveryHandoff) {
			h.RollbackPlanningRequired = false
		}},
		{name: "resource-version", mutate: func(h *LifecycleExecutionRecoveryHandoff) {
			h.ObservedLifecycleResourceVersion = "rv-tampered"
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			changed := handoff
			test.mutate(&changed)
			err := RevalidateLifecycleExecutionRecoveryHandoff(changed, observation)
			if !errors.Is(err, ErrInvalidLifecycleExecutionRecovery) {
				t.Fatalf("error = %v, want ErrInvalidLifecycleExecutionRecovery", err)
			}
		})
	}
}

func TestRevalidateLifecycleExecutionRecoveryHandoffRejectsObservedCASDrift(t *testing.T) {
	handoff, observation := lifecycleExecutionRecoveryFixture(t, false)
	observation.ObservedCAS.ResourceVersion = "rv-lifecycle-recovery-9"
	err := RevalidateLifecycleExecutionRecoveryHandoff(handoff, observation)
	if !errors.Is(err, ErrStaleLifecycleExecutionRecovery) {
		t.Fatalf("error = %v, want ErrStaleLifecycleExecutionRecovery", err)
	}
}
