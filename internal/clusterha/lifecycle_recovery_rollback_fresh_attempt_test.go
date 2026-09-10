package clusterha

import (
	"errors"
	"reflect"
	"testing"

	"control-center/internal/nodelifecycle"
)

func lifecycleRecoveryRollbackFreshAttemptFixture(
	t *testing.T,
	standalone bool,
) (
	LifecycleRecoveryRollbackFreshAttempt,
	LifecycleRecoveryRollbackFreshAttemptRequest,
) {
	t.Helper()
	gate, gateObservation := lifecycleRecoveryRollbackFreshAdmissionGateFixture(t, standalone)
	request := LifecycleRecoveryRollbackFreshAttemptRequest{
		Gate:              gate,
		GateObservation:   gateObservation,
		CurrentCAS:        gateObservation.CurrentCAS,
		CurrentMembership: gateObservation.CurrentMembership,
		CurrentEvidence:   gateObservation.CurrentEvidence,
	}
	attempt, err := BuildLifecycleRecoveryRollbackFreshAttempt(request)
	if err != nil {
		t.Fatalf("BuildLifecycleRecoveryRollbackFreshAttempt() error = %v", err)
	}
	return attempt, request
}

func TestBuildLifecycleRecoveryRollbackFreshAttemptIsDeterministicAndBounded(t *testing.T) {
	first, request := lifecycleRecoveryRollbackFreshAttemptFixture(t, false)
	second, err := BuildLifecycleRecoveryRollbackFreshAttempt(request)
	if err != nil {
		t.Fatalf("BuildLifecycleRecoveryRollbackFreshAttempt(second) error = %v", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("fresh rollback attempt is not deterministic: first=%+v second=%+v", first, second)
	}
	if first.FreshRollbackAttemptID == first.PreviousRollbackAdmissionID ||
		first.FreshAdmissionGateID != request.Gate.FreshAdmissionGateID ||
		first.PreviousRollbackAdmissionID != request.Gate.PreviousRollbackAdmissionID {
		t.Fatalf("fresh rollback attempt lost distinct recovery lineage: %+v", first)
	}
	if !first.RecoveryOnly || !first.RollbackOnly || !first.FreshAttemptAuthorized ||
		first.PreviousAdmissionReusable || first.GenericRetryAuthorized ||
		!first.CASBound || !first.SingleUseByCAS || !first.AuditedChangeJobRequired ||
		!first.ExecutionAuthorized || !first.LifecycleStateMutationAuthorized ||
		first.MembershipMutationAuthorized || first.FailoverAuthorized ||
		first.GenericCommandAuthorized || first.HostMutationAuthorized {
		t.Fatalf("fresh rollback attempt authority is not bounded: %+v", first)
	}
	if err := RevalidateLifecycleRecoveryRollbackFreshAttempt(first, request); err != nil {
		t.Fatalf("RevalidateLifecycleRecoveryRollbackFreshAttempt() error = %v", err)
	}
}

func TestBuildLifecycleRecoveryRollbackFreshAttemptPreservesStandaloneBoundary(t *testing.T) {
	attempt, _ := lifecycleRecoveryRollbackFreshAttemptFixture(t, true)
	if !attempt.StandaloneDowntimeBound || attempt.QuorumRequired != 1 ||
		attempt.MinimumReadyNodes != 1 || attempt.HealthyVotesObserved != 1 ||
		attempt.ReadyNodesObserved != 1 {
		t.Fatalf("standalone fresh rollback attempt boundary changed: %+v", attempt)
	}
}

func TestBuildLifecycleRecoveryRollbackFreshAttemptRejectsFinalCASDrift(t *testing.T) {
	_, request := lifecycleRecoveryRollbackFreshAttemptFixture(t, false)
	tests := []struct {
		name   string
		mutate func(*LifecycleCASObservation)
	}{
		{name: "resource-version", mutate: func(cas *LifecycleCASObservation) {
			cas.ResourceVersion = "rv-fresh-attempt-drift"
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
			changed := request
			changed.CurrentCAS = request.CurrentCAS
			test.mutate(&changed.CurrentCAS)
			_, err := BuildLifecycleRecoveryRollbackFreshAttempt(changed)
			if !errors.Is(err, ErrStaleLifecycleRecoveryRollbackFreshAttempt) {
				t.Fatalf("error = %v, want ErrStaleLifecycleRecoveryRollbackFreshAttempt", err)
			}
		})
	}
}

func TestBuildLifecycleRecoveryRollbackFreshAttemptRejectsFinalHARevisionDrift(t *testing.T) {
	_, request := lifecycleRecoveryRollbackFreshAttemptFixture(t, false)
	advanced, err := AdvanceTransitionRevisionEvidence(request.CurrentEvidence, ReconcilerRevisionObservation{
		Membership:              request.CurrentMembership,
		ExpectedResourceVersion: request.CurrentEvidence.Revision().ResourceVersion,
		ResourceVersion:         "rv-fresh-attempt-ha-advanced",
	})
	if err != nil {
		t.Fatalf("AdvanceTransitionRevisionEvidence() error = %v", err)
	}
	request.CurrentEvidence = advanced
	_, err = BuildLifecycleRecoveryRollbackFreshAttempt(request)
	if !errors.Is(err, ErrStaleLifecycleRecoveryRollbackFreshAttempt) {
		t.Fatalf("error = %v, want ErrStaleLifecycleRecoveryRollbackFreshAttempt", err)
	}
}

func TestBuildLifecycleRecoveryRollbackFreshAttemptRejectsQuorumDegradation(t *testing.T) {
	_, request := lifecycleRecoveryRollbackFreshAttemptFixture(t, false)
	request.CurrentMembership.Members = append([]Member(nil), request.CurrentMembership.Members...)
	for index := range request.CurrentMembership.Members {
		member := &request.CurrentMembership.Members[index]
		if member.ID != request.Gate.NodeID {
			member.Healthy = false
		}
	}
	_, err := BuildLifecycleRecoveryRollbackFreshAttempt(request)
	if !errors.Is(err, ErrStaleLifecycleRecoveryRollbackFreshAttempt) {
		t.Fatalf("error = %v, want ErrStaleLifecycleRecoveryRollbackFreshAttempt", err)
	}
}

func TestRevalidateLifecycleRecoveryRollbackFreshAttemptRejectsConsumedCAS(t *testing.T) {
	attempt, request := lifecycleRecoveryRollbackFreshAttemptFixture(t, false)
	request.CurrentCAS = LifecycleCASObservation{
		NodeID:          attempt.NodeID,
		State:           attempt.TargetState,
		Generation:      attempt.PlannedLifecycleGeneration,
		ResourceVersion: "rv-fresh-attempt-consumed",
	}
	err := RevalidateLifecycleRecoveryRollbackFreshAttempt(attempt, request)
	if !errors.Is(err, ErrStaleLifecycleRecoveryRollbackFreshAttempt) {
		t.Fatalf("error = %v, want ErrStaleLifecycleRecoveryRollbackFreshAttempt", err)
	}
}

func TestRevalidateLifecycleRecoveryRollbackFreshAttemptRejectsAuthorityTampering(t *testing.T) {
	attempt, request := lifecycleRecoveryRollbackFreshAttemptFixture(t, false)
	tests := []struct {
		name   string
		mutate func(*LifecycleRecoveryRollbackFreshAttempt)
	}{
		{name: "reuse-old-admission", mutate: func(a *LifecycleRecoveryRollbackFreshAttempt) {
			a.PreviousAdmissionReusable = true
		}},
		{name: "generic-retry", mutate: func(a *LifecycleRecoveryRollbackFreshAttempt) {
			a.GenericRetryAuthorized = true
		}},
		{name: "disable-fresh-boundary", mutate: func(a *LifecycleRecoveryRollbackFreshAttempt) {
			a.FreshAttemptAuthorized = false
		}},
		{name: "failover-authority", mutate: func(a *LifecycleRecoveryRollbackFreshAttempt) {
			a.FailoverAuthorized = true
		}},
		{name: "generic-command-authority", mutate: func(a *LifecycleRecoveryRollbackFreshAttempt) {
			a.GenericCommandAuthorized = true
		}},
		{name: "host-authority", mutate: func(a *LifecycleRecoveryRollbackFreshAttempt) {
			a.HostMutationAuthorized = true
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			changed := attempt
			test.mutate(&changed)
			err := RevalidateLifecycleRecoveryRollbackFreshAttempt(changed, request)
			if !errors.Is(err, ErrInvalidLifecycleRecoveryRollbackFreshAttempt) {
				t.Fatalf("error = %v, want ErrInvalidLifecycleRecoveryRollbackFreshAttempt", err)
			}
		})
	}
}

func TestRevalidateLifecycleRecoveryRollbackFreshAttemptRejectsGateLineageTampering(t *testing.T) {
	attempt, request := lifecycleRecoveryRollbackFreshAttemptFixture(t, false)
	request.Gate.PreviousRollbackAdmissionID = "chrra-tampered-previous-admission"
	err := RevalidateLifecycleRecoveryRollbackFreshAttempt(attempt, request)
	if !errors.Is(err, ErrInvalidLifecycleRecoveryRollbackFreshAttempt) {
		t.Fatalf("error = %v, want ErrInvalidLifecycleRecoveryRollbackFreshAttempt", err)
	}
}
