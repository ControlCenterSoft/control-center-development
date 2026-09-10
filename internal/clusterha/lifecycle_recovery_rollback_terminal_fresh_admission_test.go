package clusterha

import (
	"errors"
	"reflect"
	"testing"

	"control-center/internal/nodelifecycle"
)

func lifecycleRecoveryRollbackTerminalFreshAdmissionFixture(
	t *testing.T,
) (
	LifecycleRecoveryRollbackTerminalFreshAdmission,
	LifecycleRecoveryRollbackTerminalFreshAdmissionRequest,
) {
	t.Helper()
	gate, gateObservation := lifecycleRecoveryRollbackTerminalRevalidationGateFixture(t)
	request := LifecycleRecoveryRollbackTerminalFreshAdmissionRequest{
		Gate:              gate,
		GateObservation:   gateObservation,
		CurrentCAS:        gateObservation.CurrentCAS,
		CurrentMembership: gateObservation.CurrentMembership,
		CurrentEvidence:   gateObservation.CurrentEvidence,
	}
	admission, err := BuildLifecycleRecoveryRollbackTerminalFreshAdmission(request)
	if err != nil {
		t.Fatalf("BuildLifecycleRecoveryRollbackTerminalFreshAdmission() error = %v", err)
	}
	return admission, request
}

func TestBuildLifecycleRecoveryRollbackTerminalFreshAdmissionIsDeterministicAndBounded(t *testing.T) {
	first, request := lifecycleRecoveryRollbackTerminalFreshAdmissionFixture(t)
	for i := 0; i < 100; i++ {
		next, err := BuildLifecycleRecoveryRollbackTerminalFreshAdmission(request)
		if err != nil {
			t.Fatalf("deterministic build %d error = %v", i, err)
		}
		if !reflect.DeepEqual(first, next) {
			t.Fatalf("terminal fresh admission changed at build %d: first=%+v next=%+v", i, first, next)
		}
	}
	if first.TerminalFreshAdmissionID == first.RevalidationGateID ||
		first.TerminalFreshAdmissionID == first.TerminalIntentReceiptID ||
		first.TerminalFreshAdmissionID == first.RecoveryIntentID ||
		first.TerminalFreshAdmissionID == first.PreviousFreshRollbackAttemptID ||
		first.TerminalFreshAdmissionID == first.PreviousRollbackAdmissionID {
		t.Fatalf("terminal fresh admission reused prior lineage: %+v", first)
	}
	if first.FromState != nodelifecycle.StateDraining || first.TargetState != nodelifecycle.StateReady ||
		first.TransitionType != nodelifecycle.TransitionDesired ||
		first.PlannedLifecycleGeneration != first.ExpectedLifecycleGeneration+1 {
		t.Fatalf("terminal fresh admission lifecycle boundary changed: %+v", first)
	}
	if !first.RecoveryOnly || !first.RollbackOnly || !first.NewAdmissionLineage ||
		first.PreviousAdmissionReusable || first.PreviousAttemptReusable ||
		first.AutomaticRetryAuthorized || !first.CASBound || !first.SingleUseByCAS ||
		!first.AuditedChangeJobRequired || !first.ExecutionAuthorized ||
		!first.LifecycleStateMutationAuthorized || first.MembershipMutationAuthorized ||
		first.FailoverAuthorized || first.GenericCommandAuthorized || first.HostMutationAuthorized {
		t.Fatalf("terminal fresh admission authority is not bounded: %+v", first)
	}
	if err := RevalidateLifecycleRecoveryRollbackTerminalFreshAdmission(first, request); err != nil {
		t.Fatalf("RevalidateLifecycleRecoveryRollbackTerminalFreshAdmission() error = %v", err)
	}
}

func TestBuildLifecycleRecoveryRollbackTerminalFreshAdmissionRejectsFinalCASDrift(t *testing.T) {
	_, baseline := lifecycleRecoveryRollbackTerminalFreshAdmissionFixture(t)
	tests := []struct {
		name   string
		mutate func(*LifecycleCASObservation)
	}{
		{name: "resource-version", mutate: func(cas *LifecycleCASObservation) {
			cas.ResourceVersion = "rv-terminal-fresh-admission-drift"
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
			request := baseline
			request.CurrentCAS = baseline.CurrentCAS
			test.mutate(&request.CurrentCAS)
			_, err := BuildLifecycleRecoveryRollbackTerminalFreshAdmission(request)
			if !errors.Is(err, ErrStaleLifecycleRecoveryRollbackTerminalFreshAdmission) {
				t.Fatalf("error = %v, want terminal fresh admission stale", err)
			}
		})
	}
}

func TestBuildLifecycleRecoveryRollbackTerminalFreshAdmissionRejectsFinalHARevisionDrift(t *testing.T) {
	_, request := lifecycleRecoveryRollbackTerminalFreshAdmissionFixture(t)
	advanced, err := AdvanceTransitionRevisionEvidence(request.CurrentEvidence, ReconcilerRevisionObservation{
		Membership:              request.CurrentMembership,
		ExpectedResourceVersion: request.CurrentEvidence.Revision().ResourceVersion,
		ResourceVersion:         "rv-terminal-fresh-admission-ha-advanced",
	})
	if err != nil {
		t.Fatalf("AdvanceTransitionRevisionEvidence() error = %v", err)
	}
	request.CurrentEvidence = advanced
	_, err = BuildLifecycleRecoveryRollbackTerminalFreshAdmission(request)
	if !errors.Is(err, ErrStaleLifecycleRecoveryRollbackTerminalFreshAdmission) {
		t.Fatalf("error = %v, want terminal fresh admission stale", err)
	}
}

func TestBuildLifecycleRecoveryRollbackTerminalFreshAdmissionRejectsQuorumDrift(t *testing.T) {
	_, request := lifecycleRecoveryRollbackTerminalFreshAdmissionFixture(t)
	request.CurrentMembership.Members = append([]Member(nil), request.CurrentMembership.Members...)
	for index := range request.CurrentMembership.Members {
		member := &request.CurrentMembership.Members[index]
		if member.ID != request.Gate.NodeID {
			member.Healthy = false
			member.CaughtUp = false
		}
	}
	_, err := BuildLifecycleRecoveryRollbackTerminalFreshAdmission(request)
	if !errors.Is(err, ErrStaleLifecycleRecoveryRollbackTerminalFreshAdmission) {
		t.Fatalf("error = %v, want terminal fresh admission stale", err)
	}
}

func TestRevalidateLifecycleRecoveryRollbackTerminalFreshAdmissionRejectsAuthorityTampering(t *testing.T) {
	admission, request := lifecycleRecoveryRollbackTerminalFreshAdmissionFixture(t)
	tests := []struct {
		name   string
		mutate func(*LifecycleRecoveryRollbackTerminalFreshAdmission)
	}{
		{name: "old-admission-reuse", mutate: func(a *LifecycleRecoveryRollbackTerminalFreshAdmission) {
			a.PreviousAdmissionReusable = true
		}},
		{name: "old-attempt-reuse", mutate: func(a *LifecycleRecoveryRollbackTerminalFreshAdmission) {
			a.PreviousAttemptReusable = true
		}},
		{name: "automatic-retry", mutate: func(a *LifecycleRecoveryRollbackTerminalFreshAdmission) {
			a.AutomaticRetryAuthorized = true
		}},
		{name: "membership-mutation", mutate: func(a *LifecycleRecoveryRollbackTerminalFreshAdmission) {
			a.MembershipMutationAuthorized = true
		}},
		{name: "failover", mutate: func(a *LifecycleRecoveryRollbackTerminalFreshAdmission) {
			a.FailoverAuthorized = true
		}},
		{name: "generic-command", mutate: func(a *LifecycleRecoveryRollbackTerminalFreshAdmission) {
			a.GenericCommandAuthorized = true
		}},
		{name: "host-mutation", mutate: func(a *LifecycleRecoveryRollbackTerminalFreshAdmission) {
			a.HostMutationAuthorized = true
		}},
		{name: "disable-single-use", mutate: func(a *LifecycleRecoveryRollbackTerminalFreshAdmission) {
			a.SingleUseByCAS = false
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			changed := admission
			test.mutate(&changed)
			err := RevalidateLifecycleRecoveryRollbackTerminalFreshAdmission(changed, request)
			if !errors.Is(err, ErrInvalidLifecycleRecoveryRollbackTerminalFreshAdmission) {
				t.Fatalf("error = %v, want terminal fresh admission invalid", err)
			}
		})
	}
}

func TestRevalidateLifecycleRecoveryRollbackTerminalFreshAdmissionRejectsGateLineageTampering(t *testing.T) {
	admission, request := lifecycleRecoveryRollbackTerminalFreshAdmissionFixture(t)
	request.Gate.PreviousRollbackAdmissionID = "chrra-tampered-terminal-previous-admission"
	err := RevalidateLifecycleRecoveryRollbackTerminalFreshAdmission(admission, request)
	if !errors.Is(err, ErrInvalidLifecycleRecoveryRollbackTerminalFreshAdmission) {
		t.Fatalf("error = %v, want terminal fresh admission invalid", err)
	}
}
