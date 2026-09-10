package clusterha

import (
	"errors"
	"reflect"
	"testing"
)

func lifecycleRecoveryRollbackTerminalRevalidationGateFixture(
	t *testing.T,
) (
	LifecycleRecoveryRollbackTerminalRevalidationGate,
	LifecycleRecoveryRollbackTerminalRevalidationObservation,
) {
	t.Helper()
	intent, intentRequest := lifecycleRecoveryRollbackTerminalIntentFixture(
		t,
		LifecycleRecoveryRollbackTerminalIntentRevalidate,
	)
	observation := LifecycleRecoveryRollbackTerminalRevalidationObservation{
		IntentReceipt:     intent,
		IntentRequest:     intentRequest,
		CurrentCAS:        intentRequest.ReconciliationObservation.CurrentCAS,
		CurrentMembership: intentRequest.ReconciliationObservation.CurrentMembership,
		CurrentEvidence:   intentRequest.ReconciliationObservation.CurrentEvidence,
	}
	gate, err := BuildLifecycleRecoveryRollbackTerminalRevalidationGate(observation)
	if err != nil {
		t.Fatalf("BuildLifecycleRecoveryRollbackTerminalRevalidationGate() error = %v", err)
	}
	return gate, observation
}

func TestBuildLifecycleRecoveryRollbackTerminalRevalidationGateIsDeterministicAndEvidenceOnly(t *testing.T) {
	first, observation := lifecycleRecoveryRollbackTerminalRevalidationGateFixture(t)
	if !first.SafetyBoundarySatisfied || !first.FreshAdmissionEligible ||
		!first.RequiresNewAdmissionLineage || first.PreviousAdmissionReusable ||
		first.FreshAdmissionAuthorized || first.FurtherAttemptAuthorized ||
		first.AutomaticRetryAuthorized || first.LifecycleStateMutationAuthorized ||
		first.MembershipMutationAuthorized || first.FailoverAuthorized ||
		first.GenericCommandAuthorized || first.HostMutationAuthorized {
		t.Fatalf("terminal revalidation authority is not bounded: %+v", first)
	}
	for i := 0; i < 100; i++ {
		next, err := BuildLifecycleRecoveryRollbackTerminalRevalidationGate(observation)
		if err != nil {
			t.Fatalf("deterministic build %d error = %v", i, err)
		}
		if !reflect.DeepEqual(first, next) {
			t.Fatalf("terminal revalidation changed at build %d: first=%+v next=%+v", i, first, next)
		}
	}
	if err := RevalidateLifecycleRecoveryRollbackTerminalRevalidationGate(first, observation); err != nil {
		t.Fatalf("RevalidateLifecycleRecoveryRollbackTerminalRevalidationGate() error = %v", err)
	}
}

func TestBuildLifecycleRecoveryRollbackTerminalRevalidationGateRejectsClosure(t *testing.T) {
	intent, intentRequest := lifecycleRecoveryRollbackTerminalIntentFixture(
		t,
		LifecycleRecoveryRollbackTerminalIntentClose,
	)
	_, err := BuildLifecycleRecoveryRollbackTerminalRevalidationGate(
		LifecycleRecoveryRollbackTerminalRevalidationObservation{
			IntentReceipt:     intent,
			IntentRequest:     intentRequest,
			CurrentCAS:        intentRequest.ReconciliationObservation.CurrentCAS,
			CurrentMembership: intentRequest.ReconciliationObservation.CurrentMembership,
			CurrentEvidence:   intentRequest.ReconciliationObservation.CurrentEvidence,
		},
	)
	if !errors.Is(err, ErrLifecycleRecoveryRollbackTerminalRevalidationNotEligible) {
		t.Fatalf("error = %v, want terminal revalidation not eligible", err)
	}
}

func TestBuildLifecycleRecoveryRollbackTerminalRevalidationGateRejectsLifecycleDrift(t *testing.T) {
	_, observation := lifecycleRecoveryRollbackTerminalRevalidationGateFixture(t)
	observation.CurrentCAS.ResourceVersion += "-moved"
	_, err := BuildLifecycleRecoveryRollbackTerminalRevalidationGate(observation)
	if !errors.Is(err, ErrStaleLifecycleRecoveryRollbackTerminalRevalidationGate) {
		t.Fatalf("error = %v, want stale terminal revalidation gate", err)
	}
}

func TestBuildLifecycleRecoveryRollbackTerminalRevalidationGateRejectsQuorumLoss(t *testing.T) {
	_, observation := lifecycleRecoveryRollbackTerminalRevalidationGateFixture(t)
	for i := range observation.CurrentMembership.Members {
		observation.CurrentMembership.Members[i].Healthy = false
		observation.CurrentMembership.Members[i].CaughtUp = false
	}
	_, err := BuildLifecycleRecoveryRollbackTerminalRevalidationGate(observation)
	if !errors.Is(err, ErrStaleLifecycleRecoveryRollbackTerminalRevalidationGate) {
		t.Fatalf("error = %v, want stale terminal revalidation gate", err)
	}
}

func TestRevalidateLifecycleRecoveryRollbackTerminalRevalidationGateRejectsAuthorityTampering(t *testing.T) {
	gate, observation := lifecycleRecoveryRollbackTerminalRevalidationGateFixture(t)
	tests := []struct {
		name   string
		mutate func(*LifecycleRecoveryRollbackTerminalRevalidationGate)
	}{
		{name: "fresh-admission", mutate: func(g *LifecycleRecoveryRollbackTerminalRevalidationGate) {
			g.FreshAdmissionAuthorized = true
		}},
		{name: "automatic-retry", mutate: func(g *LifecycleRecoveryRollbackTerminalRevalidationGate) {
			g.AutomaticRetryAuthorized = true
		}},
		{name: "membership-mutation", mutate: func(g *LifecycleRecoveryRollbackTerminalRevalidationGate) {
			g.MembershipMutationAuthorized = true
		}},
		{name: "failover", mutate: func(g *LifecycleRecoveryRollbackTerminalRevalidationGate) {
			g.FailoverAuthorized = true
		}},
		{name: "old-admission-reuse", mutate: func(g *LifecycleRecoveryRollbackTerminalRevalidationGate) {
			g.PreviousAdmissionReusable = true
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			changed := gate
			test.mutate(&changed)
			if err := RevalidateLifecycleRecoveryRollbackTerminalRevalidationGate(
				changed,
				observation,
			); err == nil {
				t.Fatal("tampered terminal revalidation gate unexpectedly revalidated")
			}
		})
	}
}
