package clusterha

import (
	"errors"
	"reflect"
	"testing"
)

func lifecycleRecoveryRollbackTerminalIntentFixture(
	t *testing.T,
	action LifecycleRecoveryRollbackTerminalIntentAction,
) (
	LifecycleRecoveryRollbackTerminalIntentReceipt,
	LifecycleRecoveryRollbackTerminalIntentRequest,
) {
	t.Helper()
	reconciliation, observation := lifecycleRecoveryRollbackFreshAttemptReconciliationFixture(
		t,
		LifecycleRecoveryRollbackFreshAttemptAmbiguous,
	)
	request := LifecycleRecoveryRollbackTerminalIntentRequest{
		ReconciliationReceipt:     reconciliation,
		ReconciliationObservation: observation,
		RecoveryIntentID:          "rollback-recovery-intent-0001",
		Action:                    action,
		OperatorApproved:          true,
	}
	receipt, err := BuildLifecycleRecoveryRollbackTerminalIntentReceipt(request)
	if err != nil {
		t.Fatalf("BuildLifecycleRecoveryRollbackTerminalIntentReceipt() error = %v", err)
	}
	return receipt, request
}

func TestBuildLifecycleRecoveryRollbackTerminalIntentRevalidationIsDeterministicAndEvidenceOnly(t *testing.T) {
	first, request := lifecycleRecoveryRollbackTerminalIntentFixture(
		t,
		LifecycleRecoveryRollbackTerminalIntentRevalidate,
	)
	if first.SourceOutcome != LifecycleRecoveryRollbackFreshAttemptDefinitelyNotApplied ||
		!first.OperatorApproved || !first.FreshAdmissionRequested || first.RecoveryClosureRequested ||
		first.FreshAdmissionAuthorized || first.FurtherAttemptAuthorized || first.AutomaticRetryAuthorized ||
		first.LifecycleStateMutationAuthorized || first.MembershipMutationAuthorized ||
		first.FailoverAuthorized || first.GenericCommandAuthorized || first.HostMutationAuthorized {
		t.Fatalf("revalidation intent authority is not bounded: %+v", first)
	}
	for i := 0; i < 100; i++ {
		next, err := BuildLifecycleRecoveryRollbackTerminalIntentReceipt(request)
		if err != nil {
			t.Fatalf("deterministic build %d error = %v", i, err)
		}
		if !reflect.DeepEqual(first, next) {
			t.Fatalf("terminal intent changed at build %d: first=%+v next=%+v", i, first, next)
		}
	}
	if err := RevalidateLifecycleRecoveryRollbackTerminalIntentReceipt(first, request); err != nil {
		t.Fatalf("RevalidateLifecycleRecoveryRollbackTerminalIntentReceipt() error = %v", err)
	}
}

func TestBuildLifecycleRecoveryRollbackTerminalIntentSupportsOperatorClosure(t *testing.T) {
	receipt, _ := lifecycleRecoveryRollbackTerminalIntentFixture(
		t,
		LifecycleRecoveryRollbackTerminalIntentClose,
	)
	if receipt.FreshAdmissionRequested || !receipt.RecoveryClosureRequested ||
		receipt.FreshAdmissionAuthorized || receipt.FurtherAttemptAuthorized ||
		receipt.AutomaticRetryAuthorized || receipt.LifecycleStateMutationAuthorized {
		t.Fatalf("operator closure accidentally grants authority: %+v", receipt)
	}
}

func TestBuildLifecycleRecoveryRollbackTerminalIntentRejectsUnsafeRequest(t *testing.T) {
	_, baseline := lifecycleRecoveryRollbackTerminalIntentFixture(
		t,
		LifecycleRecoveryRollbackTerminalIntentRevalidate,
	)
	tests := []struct {
		name   string
		mutate func(*LifecycleRecoveryRollbackTerminalIntentRequest)
	}{
		{name: "approval-required", mutate: func(r *LifecycleRecoveryRollbackTerminalIntentRequest) {
			r.OperatorApproved = false
		}},
		{name: "unsupported-action", mutate: func(r *LifecycleRecoveryRollbackTerminalIntentRequest) {
			r.Action = "retry_now"
		}},
		{name: "reused-fresh-attempt-lineage", mutate: func(r *LifecycleRecoveryRollbackTerminalIntentRequest) {
			r.RecoveryIntentID = r.ReconciliationReceipt.FreshRollbackAttemptID
		}},
		{name: "reused-reconciliation-lineage", mutate: func(r *LifecycleRecoveryRollbackTerminalIntentRequest) {
			r.RecoveryIntentID = r.ReconciliationReceipt.ReconciliationReceiptID
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := baseline
			test.mutate(&request)
			_, err := BuildLifecycleRecoveryRollbackTerminalIntentReceipt(request)
			if !errors.Is(err, ErrInvalidLifecycleRecoveryRollbackTerminalIntent) {
				t.Fatalf("error = %v, want ErrInvalidLifecycleRecoveryRollbackTerminalIntent", err)
			}
		})
	}
}

func TestBuildLifecycleRecoveryRollbackTerminalIntentRejectsNonTerminalSource(t *testing.T) {
	_, request := lifecycleRecoveryRollbackTerminalIntentFixture(
		t,
		LifecycleRecoveryRollbackTerminalIntentRevalidate,
	)
	request.ReconciliationReceipt.Outcome = LifecycleRecoveryRollbackFreshAttemptReconciledAmbiguous
	request.ReconciliationReceipt.FreshAdmissionRequired = false
	request.ReconciliationReceipt.ReconciliationRequired = true
	_, err := BuildLifecycleRecoveryRollbackTerminalIntentReceipt(request)
	if err == nil {
		t.Fatal("non-terminal source unexpectedly produced a terminal recovery intent")
	}
}

func TestRevalidateLifecycleRecoveryRollbackTerminalIntentRejectsAuthorityTampering(t *testing.T) {
	receipt, request := lifecycleRecoveryRollbackTerminalIntentFixture(
		t,
		LifecycleRecoveryRollbackTerminalIntentRevalidate,
	)
	tests := []struct {
		name   string
		mutate func(*LifecycleRecoveryRollbackTerminalIntentReceipt)
	}{
		{name: "fresh-admission", mutate: func(r *LifecycleRecoveryRollbackTerminalIntentReceipt) {
			r.FreshAdmissionAuthorized = true
		}},
		{name: "automatic-retry", mutate: func(r *LifecycleRecoveryRollbackTerminalIntentReceipt) {
			r.AutomaticRetryAuthorized = true
		}},
		{name: "membership-mutation", mutate: func(r *LifecycleRecoveryRollbackTerminalIntentReceipt) {
			r.MembershipMutationAuthorized = true
		}},
		{name: "failover", mutate: func(r *LifecycleRecoveryRollbackTerminalIntentReceipt) {
			r.FailoverAuthorized = true
		}},
		{name: "action", mutate: func(r *LifecycleRecoveryRollbackTerminalIntentReceipt) {
			r.Action = LifecycleRecoveryRollbackTerminalIntentClose
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			changed := receipt
			test.mutate(&changed)
			if err := RevalidateLifecycleRecoveryRollbackTerminalIntentReceipt(changed, request); err == nil {
				t.Fatal("tampered terminal recovery intent unexpectedly revalidated")
			}
		})
	}
}
