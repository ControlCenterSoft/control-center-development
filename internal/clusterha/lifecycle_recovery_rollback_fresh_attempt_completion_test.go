package clusterha

import (
	"errors"
	"reflect"
	"testing"

	"control-center/internal/nodelifecycle"
)

func lifecycleRecoveryRollbackFreshAttemptCompletionFixture(
	t *testing.T,
	outcome LifecycleRecoveryRollbackFreshAttemptCommitOutcome,
) (
	LifecycleRecoveryRollbackFreshAttemptCompletionReceipt,
	LifecycleRecoveryRollbackFreshAttemptCompletionObservation,
) {
	t.Helper()
	attempt, source := lifecycleRecoveryRollbackFreshAttemptFixture(t, false)
	post := source.CurrentCAS
	switch outcome {
	case LifecycleRecoveryRollbackFreshAttemptApplied:
		post.State = attempt.TargetState
		post.Generation = attempt.PlannedLifecycleGeneration
		post.ResourceVersion = "rv-fresh-attempt-applied"
	case LifecycleRecoveryRollbackFreshAttemptCASLost:
		post.ResourceVersion = "rv-fresh-attempt-competing-write"
	case LifecycleRecoveryRollbackFreshAttemptAmbiguous:
		// A lost response with the old value still visible is deliberately not
		// safe to retry; reconciliation must establish durable state first.
	default:
		t.Fatalf("unsupported fixture outcome %q", outcome)
	}
	observation := LifecycleRecoveryRollbackFreshAttemptCompletionObservation{
		Attempt:           attempt,
		Source:            source,
		FinalCAS:          source.CurrentCAS,
		CurrentMembership: source.CurrentMembership,
		CurrentEvidence:   source.CurrentEvidence,
		CASResult: LifecycleRecoveryRollbackFreshAttemptCASResult{
			Outcome: outcome,
			PostCAS: post,
		},
	}
	receipt, err := BuildLifecycleRecoveryRollbackFreshAttemptCompletionReceipt(observation)
	if err != nil {
		t.Fatalf("BuildLifecycleRecoveryRollbackFreshAttemptCompletionReceipt() error = %v", err)
	}
	return receipt, observation
}

func TestBuildLifecycleRecoveryRollbackFreshAttemptCompletionAppliedIsDeterministic(t *testing.T) {
	first, observation := lifecycleRecoveryRollbackFreshAttemptCompletionFixture(
		t,
		LifecycleRecoveryRollbackFreshAttemptApplied,
	)
	second, err := BuildLifecycleRecoveryRollbackFreshAttemptCompletionReceipt(observation)
	if err != nil {
		t.Fatalf("BuildLifecycleRecoveryRollbackFreshAttemptCompletionReceipt(second) error = %v", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("completion receipt is not deterministic: first=%+v second=%+v", first, second)
	}
	if !first.AppliedProven || first.CASLostProven || !first.CompletionVerified ||
		first.ReconciliationRequired || first.FurtherAttemptAuthorized ||
		first.AutomaticRetryAuthorized || first.LifecycleStateMutationAuthorized ||
		first.MembershipMutationAuthorized || first.FailoverAuthorized ||
		first.GenericCommandAuthorized || first.HostMutationAuthorized {
		t.Fatalf("applied completion authority is not bounded: %+v", first)
	}
	if err := RevalidateLifecycleRecoveryRollbackFreshAttemptCompletionReceipt(first, observation); err != nil {
		t.Fatalf("RevalidateLifecycleRecoveryRollbackFreshAttemptCompletionReceipt() error = %v", err)
	}
}

func TestBuildLifecycleRecoveryRollbackFreshAttemptCompletionPreservesStandaloneSafety(t *testing.T) {
	attempt, source := lifecycleRecoveryRollbackFreshAttemptFixture(t, true)
	post := source.CurrentCAS
	post.State = attempt.TargetState
	post.Generation = attempt.PlannedLifecycleGeneration
	post.ResourceVersion = "rv-fresh-attempt-standalone-applied"
	observation := LifecycleRecoveryRollbackFreshAttemptCompletionObservation{
		Attempt:           attempt,
		Source:            source,
		FinalCAS:          source.CurrentCAS,
		CurrentMembership: source.CurrentMembership,
		CurrentEvidence:   source.CurrentEvidence,
		CASResult: LifecycleRecoveryRollbackFreshAttemptCASResult{
			Outcome: LifecycleRecoveryRollbackFreshAttemptApplied,
			PostCAS: post,
		},
	}
	receipt, err := BuildLifecycleRecoveryRollbackFreshAttemptCompletionReceipt(observation)
	if err != nil {
		t.Fatalf("BuildLifecycleRecoveryRollbackFreshAttemptCompletionReceipt() error = %v", err)
	}
	if !receipt.StandaloneDowntimeBound || receipt.QuorumRequired != 1 ||
		receipt.MinimumReadyNodes != 1 || receipt.HealthyVotesObserved != 1 ||
		receipt.ReadyNodesObserved != 1 {
		t.Fatalf("standalone completion boundary changed: %+v", receipt)
	}
}

func TestBuildLifecycleRecoveryRollbackFreshAttemptCompletionCASLostRequiresReconciliation(t *testing.T) {
	receipt, _ := lifecycleRecoveryRollbackFreshAttemptCompletionFixture(
		t,
		LifecycleRecoveryRollbackFreshAttemptCASLost,
	)
	if receipt.AppliedProven || !receipt.CASLostProven || receipt.CompletionVerified ||
		!receipt.ReconciliationRequired || receipt.FurtherAttemptAuthorized ||
		receipt.AutomaticRetryAuthorized {
		t.Fatalf("CAS-lost outcome is not reconciliation-only: %+v", receipt)
	}
}

func TestBuildLifecycleRecoveryRollbackFreshAttemptCompletionAmbiguousNeverRetries(t *testing.T) {
	receipt, _ := lifecycleRecoveryRollbackFreshAttemptCompletionFixture(
		t,
		LifecycleRecoveryRollbackFreshAttemptAmbiguous,
	)
	if receipt.AppliedProven || receipt.CASLostProven || receipt.CompletionVerified ||
		!receipt.ReconciliationRequired || receipt.FurtherAttemptAuthorized ||
		receipt.AutomaticRetryAuthorized {
		t.Fatalf("ambiguous outcome is not reconciliation-only: %+v", receipt)
	}
}

func TestBuildLifecycleRecoveryRollbackFreshAttemptCompletionRejectsFinalSafetyDrift(t *testing.T) {
	_, observation := lifecycleRecoveryRollbackFreshAttemptCompletionFixture(
		t,
		LifecycleRecoveryRollbackFreshAttemptApplied,
	)
	tests := []struct {
		name   string
		mutate func(*LifecycleRecoveryRollbackFreshAttemptCompletionObservation)
	}{
		{name: "lifecycle-cas", mutate: func(o *LifecycleRecoveryRollbackFreshAttemptCompletionObservation) {
			o.FinalCAS.ResourceVersion = "rv-fresh-attempt-final-drift"
		}},
		{name: "quorum", mutate: func(o *LifecycleRecoveryRollbackFreshAttemptCompletionObservation) {
			o.CurrentMembership.Members = append([]Member(nil), o.CurrentMembership.Members...)
			for index := range o.CurrentMembership.Members {
				member := &o.CurrentMembership.Members[index]
				if member.ID != o.Attempt.NodeID {
					member.Healthy = false
				}
			}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			changed := observation
			test.mutate(&changed)
			_, err := BuildLifecycleRecoveryRollbackFreshAttemptCompletionReceipt(changed)
			if !errors.Is(err, ErrStaleLifecycleRecoveryRollbackFreshAttemptCompletion) {
				t.Fatalf("error = %v, want ErrStaleLifecycleRecoveryRollbackFreshAttemptCompletion", err)
			}
		})
	}
}

func TestBuildLifecycleRecoveryRollbackFreshAttemptCompletionRejectsInconsistentCASResults(t *testing.T) {
	_, observation := lifecycleRecoveryRollbackFreshAttemptCompletionFixture(
		t,
		LifecycleRecoveryRollbackFreshAttemptApplied,
	)
	tests := []struct {
		name   string
		mutate func(*LifecycleRecoveryRollbackFreshAttemptCASResult)
	}{
		{name: "applied-with-precondition", mutate: func(result *LifecycleRecoveryRollbackFreshAttemptCASResult) {
			result.PostCAS = observation.FinalCAS
		}},
		{name: "cas-lost-with-precondition", mutate: func(result *LifecycleRecoveryRollbackFreshAttemptCASResult) {
			result.Outcome = LifecycleRecoveryRollbackFreshAttemptCASLost
			result.PostCAS = observation.FinalCAS
		}},
		{name: "unknown-outcome", mutate: func(result *LifecycleRecoveryRollbackFreshAttemptCASResult) {
			result.Outcome = "retry"
		}},
		{name: "wrong-node", mutate: func(result *LifecycleRecoveryRollbackFreshAttemptCASResult) {
			result.PostCAS.NodeID = "controller-other"
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			changed := observation
			test.mutate(&changed.CASResult)
			_, err := BuildLifecycleRecoveryRollbackFreshAttemptCompletionReceipt(changed)
			if !errors.Is(err, ErrInvalidLifecycleRecoveryRollbackFreshAttemptCompletion) {
				t.Fatalf("error = %v, want ErrInvalidLifecycleRecoveryRollbackFreshAttemptCompletion", err)
			}
		})
	}
}

func TestRevalidateLifecycleRecoveryRollbackFreshAttemptCompletionRejectsAuthorityTampering(t *testing.T) {
	receipt, observation := lifecycleRecoveryRollbackFreshAttemptCompletionFixture(
		t,
		LifecycleRecoveryRollbackFreshAttemptApplied,
	)
	tests := []struct {
		name   string
		mutate func(*LifecycleRecoveryRollbackFreshAttemptCompletionReceipt)
	}{
		{name: "further-attempt", mutate: func(r *LifecycleRecoveryRollbackFreshAttemptCompletionReceipt) {
			r.FurtherAttemptAuthorized = true
		}},
		{name: "automatic-retry", mutate: func(r *LifecycleRecoveryRollbackFreshAttemptCompletionReceipt) {
			r.AutomaticRetryAuthorized = true
		}},
		{name: "lifecycle-mutation", mutate: func(r *LifecycleRecoveryRollbackFreshAttemptCompletionReceipt) {
			r.LifecycleStateMutationAuthorized = true
		}},
		{name: "failover", mutate: func(r *LifecycleRecoveryRollbackFreshAttemptCompletionReceipt) {
			r.FailoverAuthorized = true
		}},
		{name: "generic-command", mutate: func(r *LifecycleRecoveryRollbackFreshAttemptCompletionReceipt) {
			r.GenericCommandAuthorized = true
		}},
		{name: "host-mutation", mutate: func(r *LifecycleRecoveryRollbackFreshAttemptCompletionReceipt) {
			r.HostMutationAuthorized = true
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			changed := receipt
			test.mutate(&changed)
			err := RevalidateLifecycleRecoveryRollbackFreshAttemptCompletionReceipt(changed, observation)
			if !errors.Is(err, ErrInvalidLifecycleRecoveryRollbackFreshAttemptCompletion) {
				t.Fatalf("error = %v, want ErrInvalidLifecycleRecoveryRollbackFreshAttemptCompletion", err)
			}
		})
	}
}

func TestRevalidateLifecycleRecoveryRollbackFreshAttemptCompletionRejectsConsumedAttempt(t *testing.T) {
	receipt, observation := lifecycleRecoveryRollbackFreshAttemptCompletionFixture(
		t,
		LifecycleRecoveryRollbackFreshAttemptApplied,
	)
	observation.FinalCAS = observation.CASResult.PostCAS
	err := RevalidateLifecycleRecoveryRollbackFreshAttemptCompletionReceipt(receipt, observation)
	if !errors.Is(err, ErrStaleLifecycleRecoveryRollbackFreshAttemptCompletion) {
		t.Fatalf("error = %v, want ErrStaleLifecycleRecoveryRollbackFreshAttemptCompletion", err)
	}
}

func TestBuildLifecycleRecoveryRollbackFreshAttemptCompletionRejectsWrongAppliedGeneration(t *testing.T) {
	_, observation := lifecycleRecoveryRollbackFreshAttemptCompletionFixture(
		t,
		LifecycleRecoveryRollbackFreshAttemptApplied,
	)
	observation.CASResult.PostCAS.State = nodelifecycle.StateReady
	observation.CASResult.PostCAS.Generation = observation.Attempt.ExpectedLifecycleGeneration
	_, err := BuildLifecycleRecoveryRollbackFreshAttemptCompletionReceipt(observation)
	if !errors.Is(err, ErrInvalidLifecycleRecoveryRollbackFreshAttemptCompletion) {
		t.Fatalf("error = %v, want ErrInvalidLifecycleRecoveryRollbackFreshAttemptCompletion", err)
	}
}
