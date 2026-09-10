package pxe

import (
	"errors"
	"reflect"
	"testing"
)

func TestReconcileBootSessionReadmissionConsumptionConsumedClosesTerminalGeneration(t *testing.T) {
	previous, candidate, priorReconciliation, fresh, readmission, freshCandidate, terminal :=
		bootSessionTerminalReadmissionAmbiguousFixture(t)
	store, err := NewDirectoryBootSessionConsumptionStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.AtomicConsumeBootSession(freshCandidate); err != nil {
		t.Fatal(err)
	}
	observedAt := freshCandidate.ConsumedAtUnix + 1

	first, err := ReconcileBootSessionReadmissionConsumption(
		store, terminal, readmission, previous, candidate, priorReconciliation, fresh, freshCandidate, observedAt,
	)
	if err != nil {
		t.Fatal(err)
	}
	second, err := ReconcileBootSessionReadmissionConsumption(
		store, terminal, readmission, previous, candidate, priorReconciliation, fresh, freshCandidate, observedAt,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf(
			"stable durable evidence produced non-deterministic terminal reconciliation: first=%#v second=%#v",
			first,
			second,
		)
	}
	if first.State != BootSessionConsumptionReconciledConsumed ||
		first.ObservedReceiptID != freshCandidate.ReceiptID || first.RecoveryRequired ||
		first.RecoveryAction != "close-terminal-readmission-as-consumed" ||
		!first.TerminalGeneration || first.RecoveryGeneration != 1 || first.MaxRecoveryGenerations != 1 ||
		first.BootHandoffAuthorized || first.FurtherReadmissionAuthorized || first.AutomaticRetryAuthorized ||
		first.ProvisioningAuthorized || first.SecretInjectionAuthorized || first.HostMutation ||
		first.NetworkMutation || first.ProductionMutation {
		t.Fatalf("consumed terminal reconciliation widened authority: %#v", first)
	}
	if err := VerifyBootSessionReadmissionConsumptionReconciliation(
		first, terminal, readmission, previous, candidate, priorReconciliation, fresh, freshCandidate,
	); err != nil {
		t.Fatalf("valid consumed terminal reconciliation rejected: %v", err)
	}
}

func TestReconcileBootSessionReadmissionConsumptionDefinitelyAbsentNeverReissues(t *testing.T) {
	previous, candidate, priorReconciliation, fresh, readmission, freshCandidate, terminal :=
		bootSessionTerminalReadmissionAmbiguousFixture(t)
	store, err := NewDirectoryBootSessionConsumptionStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	decision, err := ReconcileBootSessionReadmissionConsumption(
		store,
		terminal,
		readmission,
		previous,
		candidate,
		priorReconciliation,
		fresh,
		freshCandidate,
		freshCandidate.ConsumedAtUnix+1,
	)
	if err != nil {
		t.Fatal(err)
	}
	if decision.State != BootSessionConsumptionReconciledDefinitelyAbsent ||
		decision.ObservedReceiptID != "" || !decision.RecoveryRequired ||
		decision.RecoveryAction != "operator-close-terminal-readmission-or-create-new-deployment-intent" ||
		decision.BootHandoffAuthorized || decision.FurtherReadmissionAuthorized ||
		decision.AutomaticRetryAuthorized || decision.ProvisioningAuthorized ||
		decision.SecretInjectionAuthorized || decision.HostMutation || decision.NetworkMutation ||
		decision.ProductionMutation {
		t.Fatalf("definitely-absent terminal generation enabled an automatic second recovery: %#v", decision)
	}
}

func TestReconcileBootSessionReadmissionConsumptionAmbiguousFailsClosed(t *testing.T) {
	previous, candidate, priorReconciliation, fresh, readmission, freshCandidate, terminal :=
		bootSessionTerminalReadmissionAmbiguousFixture(t)
	store := bootSessionReadmissionReconciliationStoreStub{
		observation: BootSessionConsumptionObservation{State: BootSessionConsumptionReconciledAmbiguous},
		err:         errors.New("durability unavailable"),
	}

	decision, err := ReconcileBootSessionReadmissionConsumption(
		store,
		terminal,
		readmission,
		previous,
		candidate,
		priorReconciliation,
		fresh,
		freshCandidate,
		freshCandidate.ConsumedAtUnix+1,
	)
	if !errors.Is(err, ErrBootSessionReadmissionConsumptionReconciliation) {
		t.Fatalf("ambiguous durability did not surface bounded reconciliation error: %v", err)
	}
	if decision.State != BootSessionConsumptionReconciledAmbiguous || !decision.RecoveryRequired ||
		decision.RecoveryAction != "operator-reconcile-terminal-readmission-consumption" ||
		decision.ObservedReceiptID != "" || decision.BootHandoffAuthorized ||
		decision.FurtherReadmissionAuthorized || decision.AutomaticRetryAuthorized ||
		decision.ProductionMutation {
		t.Fatalf("ambiguous terminal reconciliation did not fail closed: %#v", decision)
	}
}

func TestReconcileBootSessionReadmissionConsumptionDifferentValidAttemptStillConsumed(t *testing.T) {
	previous, candidate, priorReconciliation, fresh, readmission, freshCandidate, terminal :=
		bootSessionTerminalReadmissionAmbiguousFixture(t)
	persisted, err := BuildBootSessionConsumptionReceipt(BootSessionConsumptionRequest{
		AttemptID:      "attempt-readmission-observed-other",
		ConsumerID:     "edge-readmission-observed-other",
		ConsumedAtUnix: freshCandidate.ConsumedAtUnix,
	}, fresh)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.ReplayKey != freshCandidate.ReplayKey || persisted.ReceiptID == freshCandidate.ReceiptID {
		t.Fatal("fixture did not create distinct valid consumption evidence on the same replay key")
	}
	store, err := NewDirectoryBootSessionConsumptionStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.AtomicConsumeBootSession(persisted); err != nil {
		t.Fatal(err)
	}

	decision, err := ReconcileBootSessionReadmissionConsumption(
		store,
		terminal,
		readmission,
		previous,
		candidate,
		priorReconciliation,
		fresh,
		freshCandidate,
		freshCandidate.ConsumedAtUnix+1,
	)
	if err != nil {
		t.Fatal(err)
	}
	if decision.State != BootSessionConsumptionReconciledConsumed ||
		decision.ObservedReceiptID != persisted.ReceiptID ||
		decision.BootHandoffAuthorized || decision.FurtherReadmissionAuthorized ||
		decision.AutomaticRetryAuthorized || decision.RecoveryRequired {
		t.Fatalf("valid competing consumption was not treated as terminally consumed: %#v", decision)
	}
}

func TestReconcileBootSessionReadmissionConsumptionRejectsNonAmbiguousSource(t *testing.T) {
	previous, candidate, priorReconciliation, fresh, readmission, freshCandidate, _ :=
		bootSessionTerminalReadmissionAmbiguousFixture(t)
	committed, err := buildBootSessionReadmissionConsumptionReceipt(
		readmission, fresh, freshCandidate, BootSessionConsumptionCommitted,
	)
	if err != nil {
		t.Fatal(err)
	}
	store, err := NewDirectoryBootSessionConsumptionStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	decision, err := ReconcileBootSessionReadmissionConsumption(
		store,
		committed,
		readmission,
		previous,
		candidate,
		priorReconciliation,
		fresh,
		freshCandidate,
		freshCandidate.ConsumedAtUnix+1,
	)
	if !errors.Is(err, ErrBootSessionReadmissionConsumptionReconciliation) {
		t.Fatalf("non-ambiguous source was accepted: decision=%#v err=%v", decision, err)
	}
	if decision != (BootSessionReadmissionConsumptionReconciliation{}) {
		t.Fatalf("non-ambiguous source produced usable reconciliation evidence: %#v", decision)
	}
}

func TestVerifyBootSessionReadmissionConsumptionReconciliationRejectsAuthorityAndLineageDrift(t *testing.T) {
	previous, candidate, priorReconciliation, fresh, readmission, freshCandidate, terminal :=
		bootSessionTerminalReadmissionAmbiguousFixture(t)
	store, err := NewDirectoryBootSessionConsumptionStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	decision, err := ReconcileBootSessionReadmissionConsumption(
		store,
		terminal,
		readmission,
		previous,
		candidate,
		priorReconciliation,
		fresh,
		freshCandidate,
		freshCandidate.ConsumedAtUnix+1,
	)
	if err != nil {
		t.Fatal(err)
	}

	tamperedCases := []BootSessionReadmissionConsumptionReconciliation{
		decision, decision, decision, decision, decision,
	}
	tamperedCases[0].RecoveryGeneration = 2
	tamperedCases[1].FurtherReadmissionAuthorized = true
	tamperedCases[2].AutomaticRetryAuthorized = true
	tamperedCases[3].BootHandoffAuthorized = true
	tamperedCases[4].ReadmissionConsumptionReceiptID = freshCandidate.ReceiptID
	for _, tampered := range tamperedCases {
		if err := VerifyBootSessionReadmissionConsumptionReconciliation(
			tampered,
			terminal,
			readmission,
			previous,
			candidate,
			priorReconciliation,
			fresh,
			freshCandidate,
		); !errors.Is(err, ErrBootSessionReadmissionConsumptionReconciliation) {
			t.Fatalf("unsafe terminal reconciliation accepted: %#v err=%v", tampered, err)
		}
	}

	drift := decision
	drift.RecoveryAction = "reissue-new-session-from-fresh-serving-evidence"
	if err := VerifyBootSessionReadmissionConsumptionReconciliation(
		drift,
		terminal,
		readmission,
		previous,
		candidate,
		priorReconciliation,
		fresh,
		freshCandidate,
	); !errors.Is(err, ErrBootSessionReadmissionConsumptionReconciliation) {
		t.Fatalf("second-readmission recovery action was accepted: %v", err)
	}
}

type bootSessionReadmissionReconciliationStoreStub struct {
	observation BootSessionConsumptionObservation
	err         error
}

func (s bootSessionReadmissionReconciliationStoreStub) ObserveBootSessionConsumption(
	string,
) (BootSessionConsumptionObservation, error) {
	return s.observation, s.err
}

func bootSessionTerminalReadmissionAmbiguousFixture(t *testing.T) (
	BootSessionAdmission,
	BootSessionConsumptionReceipt,
	BootSessionConsumptionReconciliation,
	BootSessionAdmission,
	BootSessionReadmissionReceipt,
	BootSessionConsumptionReceipt,
	BootSessionReadmissionConsumptionReceipt,
) {
	t.Helper()
	previous, candidate, priorReconciliation, request, servingReceipt, plan, payload :=
		bootSessionReadmissionFixture(t)
	fresh, readmission, err := BuildBootSessionReadmission(
		previous,
		candidate,
		priorReconciliation,
		request,
		servingReceipt,
		plan,
		payload,
		nil,
		nil,
		priorReconciliation.ObservedAtUnix,
	)
	if err != nil {
		t.Fatal(err)
	}
	consumeRequest := BootSessionConsumptionRequest{
		AttemptID:      "attempt-terminal-reconciliation",
		ConsumerID:     "edge-terminal-reconciliation",
		ConsumedAtUnix: fresh.IssuedAtUnix + 1,
	}
	store := &bootSessionReadmissionConsumptionStoreStub{
		result: BootSessionConsumptionStoreResult{State: BootSessionConsumptionAmbiguous},
		err:    errors.New("commit acknowledgement lost"),
	}
	decision, consumeErr := ConsumeBootSessionReadmission(
		store,
		consumeRequest,
		previous,
		candidate,
		priorReconciliation,
		fresh,
		readmission,
		request,
		servingReceipt,
		plan,
		payload,
		nil,
		nil,
	)
	if !errors.Is(consumeErr, ErrBootSessionReadmissionConsumption) {
		t.Fatalf("fixture did not produce ambiguous terminal consumption: %v", consumeErr)
	}
	return previous, candidate, priorReconciliation, fresh, readmission, decision.ConsumptionReceipt, decision.Receipt
}
