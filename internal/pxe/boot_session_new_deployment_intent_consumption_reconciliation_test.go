package pxe

import (
	"errors"
	"reflect"
	"testing"
)

func TestNewDeploymentIntentConsumptionReconciliationConsumedIsTerminal(t *testing.T) {
	fixture, evidence, source := newDeploymentIntentReconciliationFixture(t)
	store, err := NewDirectoryBootSessionConsumptionStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if result, err := store.AtomicConsumeBootSession(source.ConsumptionReceipt); err != nil ||
		result.State != BootSessionConsumptionCommitted {
		t.Fatalf("persist candidate: result=%#v err=%v", result, err)
	}
	observedAt := source.ConsumptionReceipt.ConsumedAtUnix + 1
	first, err := ReconcileBootSessionNewDeploymentIntentConsumption(
		store, source, evidence, observedAt,
	)
	if err != nil {
		t.Fatal(err)
	}
	if first.State != BootSessionConsumptionReconciledConsumed ||
		first.ObservedReceiptID != source.ConsumptionReceipt.ReceiptID ||
		first.RecoveryRequired || first.RecoveryAction != "close-new-deployment-intent-as-consumed" {
		t.Fatalf("consumed reconciliation lost terminal evidence: %#v", first)
	}
	assertNewDeploymentIntentReconciliationNoAuthority(t, first)
	if err := VerifyBootSessionNewDeploymentIntentConsumptionReconciliation(
		first, source, evidence,
	); err != nil {
		t.Fatalf("valid consumed reconciliation rejected: %v", err)
	}
	for i := 0; i < 100; i++ {
		next, err := ReconcileBootSessionNewDeploymentIntentConsumption(
			store, source, evidence, observedAt,
		)
		if err != nil || !reflect.DeepEqual(first, next) {
			t.Fatalf("non-deterministic reconciliation: %#v err=%v", next, err)
		}
	}
	_ = fixture
}

func TestNewDeploymentIntentConsumptionReconciliationAbsentNeverRevivesAdmission(t *testing.T) {
	_, evidence, source := newDeploymentIntentReconciliationFixture(t)
	store, err := NewDirectoryBootSessionConsumptionStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, observedAt := range []int64{
		source.ConsumptionReceipt.ConsumedAtUnix + 1,
		evidence.NewAdmission.ExpiresAtUnix + 1,
	} {
		decision, err := ReconcileBootSessionNewDeploymentIntentConsumption(
			store, source, evidence, observedAt,
		)
		if err != nil {
			t.Fatal(err)
		}
		if decision.State != BootSessionConsumptionReconciledDefinitelyAbsent ||
			decision.ObservedReceiptID != "" || !decision.RecoveryRequired ||
			decision.RecoveryAction != "operator-close-or-start-separate-deployment-intent-boundary" {
			t.Fatalf("absent result escaped operator boundary: %#v", decision)
		}
		if decision.AdmissionExpired != (observedAt >= evidence.NewAdmission.ExpiresAtUnix) {
			t.Fatalf("admission expiry evidence drift: %#v", decision)
		}
		assertNewDeploymentIntentReconciliationNoAuthority(t, decision)
	}
}

func TestNewDeploymentIntentConsumptionReconciliationDifferentAttemptIsAmbiguous(t *testing.T) {
	_, evidence, source := newDeploymentIntentReconciliationFixture(t)
	store, err := NewDirectoryBootSessionConsumptionStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	other, err := BuildBootSessionConsumptionReceipt(
		BootSessionConsumptionRequest{
			AttemptID:      "attempt-competing-new-intent-consume-0002",
			ConsumerID:     "pxe-handoff-consumer-competing-0002",
			ConsumedAtUnix: source.ConsumptionReceipt.ConsumedAtUnix,
		},
		evidence.NewAdmission,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.AtomicConsumeBootSession(other); err != nil {
		t.Fatal(err)
	}
	decision, err := ReconcileBootSessionNewDeploymentIntentConsumption(
		store, source, evidence, source.ConsumptionReceipt.ConsumedAtUnix+1,
	)
	if !errors.Is(err, ErrBootSessionNewDeploymentIntentConsumptionReconciliation) {
		t.Fatalf("competing attempt was not ambiguous: %#v err=%v", decision, err)
	}
	if decision.State != BootSessionConsumptionReconciledAmbiguous ||
		!decision.RecoveryRequired ||
		decision.RecoveryAction != "operator-reconcile-new-deployment-intent-consumption" {
		t.Fatalf("competing attempt did not fail closed: %#v", decision)
	}
	assertNewDeploymentIntentReconciliationNoAuthority(t, decision)
}

func TestNewDeploymentIntentConsumptionReconciliationRejectsUnsafeSource(t *testing.T) {
	_, evidence, source := newDeploymentIntentReconciliationFixture(t)
	store, err := NewDirectoryBootSessionConsumptionStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	cases := []BootSessionNewDeploymentIntentConsumptionDecision{source, source, source}
	cases[0].AutomaticRetryAuthorized = true
	cases[1].IntentReceiptID = evidence.NewAdmission.AdmissionID
	cases[2].BootHandoffAuthorized = true
	for _, candidate := range cases {
		decision, err := ReconcileBootSessionNewDeploymentIntentConsumption(
			store, candidate, evidence, source.ConsumptionReceipt.ConsumedAtUnix+1,
		)
		if !errors.Is(err, ErrBootSessionNewDeploymentIntentConsumptionReconciliation) {
			t.Fatalf("unsafe source accepted: %#v err=%v", decision, err)
		}
		if !reflect.DeepEqual(decision, BootSessionNewDeploymentIntentConsumptionReconciliation{}) {
			t.Fatalf("unsafe source returned partial evidence: %#v", decision)
		}
	}
}

func TestVerifyNewDeploymentIntentConsumptionReconciliationRejectsAuthorityTampering(t *testing.T) {
	_, evidence, source := newDeploymentIntentReconciliationFixture(t)
	store, err := NewDirectoryBootSessionConsumptionStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	decision, err := ReconcileBootSessionNewDeploymentIntentConsumption(
		store, source, evidence, source.ConsumptionReceipt.ConsumedAtUnix+1,
	)
	if err != nil {
		t.Fatal(err)
	}
	cases := []BootSessionNewDeploymentIntentConsumptionReconciliation{
		decision, decision, decision, decision,
	}
	cases[0].CurrentAdmissionReusable = true
	cases[1].FreshAdmissionAuthorized = true
	cases[2].AutomaticRetryAuthorized = true
	cases[3].ProductionMutation = true
	for _, candidate := range cases {
		if err := VerifyBootSessionNewDeploymentIntentConsumptionReconciliation(
			candidate, source, evidence,
		); !errors.Is(err, ErrBootSessionNewDeploymentIntentConsumptionReconciliation) {
			t.Fatalf("authority tampering accepted: %#v err=%v", candidate, err)
		}
	}
}

func TestNewDeploymentIntentConsumptionReconciliationObservationErrorFailsClosed(t *testing.T) {
	_, evidence, source := newDeploymentIntentReconciliationFixture(t)
	decision, err := ReconcileBootSessionNewDeploymentIntentConsumption(
		newDeploymentIntentObservationErrorStore{},
		source,
		evidence,
		source.ConsumptionReceipt.ConsumedAtUnix+1,
	)
	if !errors.Is(err, ErrBootSessionNewDeploymentIntentConsumptionReconciliation) {
		t.Fatalf("observation error not surfaced: %#v err=%v", decision, err)
	}
	if decision.State != BootSessionConsumptionReconciledAmbiguous ||
		!decision.RecoveryRequired ||
		decision.RecoveryAction != "operator-reconcile-new-deployment-intent-consumption" {
		t.Fatalf("observation error did not fail closed: %#v", decision)
	}
	assertNewDeploymentIntentReconciliationNoAuthority(t, decision)
}

func newDeploymentIntentReconciliationFixture(
	t *testing.T,
) (
	bootSessionNewDeploymentIntentTestFixture,
	BootSessionNewDeploymentIntentConsumptionEvidence,
	BootSessionNewDeploymentIntentConsumptionDecision,
) {
	t.Helper()
	fixture := bootSessionNewDeploymentIntentFixture(t)
	admission, intentReceipt := bootSessionNewDeploymentIntentConsumptionFixture(t, fixture)
	evidence := BootSessionNewDeploymentIntentConsumptionEvidence{
		IntentReceipt: intentReceipt, NewAdmission: admission, Intent: fixture.intent,
		TerminalReconciliation: fixture.terminalReconciliation,
		TerminalConsumption: fixture.terminalConsumption, Readmission: fixture.readmission,
		Previous: fixture.previous, PreviousCandidate: fixture.previousCandidate,
		PreviousReconciliation: fixture.previousReconciliation,
		TerminalAdmission: fixture.terminalAdmission, TerminalCandidate: fixture.terminalCandidate,
		ServingReceipt: fixture.servingReceipt, Plan: fixture.plan, CurrentMedia: fixture.payload,
	}
	request := BootSessionConsumptionRequest{
		AttemptID: "attempt-new-deployment-intent-reconcile-0001",
		ConsumerID: "pxe-handoff-consumer-reconcile-0001",
		ConsumedAtUnix: admission.IssuedAtUnix + 1,
	}
	source, err := consumeBootSessionNewDeploymentIntentFixture(
		bootSessionNewDeploymentIntentAmbiguousStore{}, request, intentReceipt, admission, fixture,
	)
	if !errors.Is(err, ErrBootSessionNewDeploymentIntentConsumption) ||
		!errors.Is(err, ErrBootSessionConsumptionAmbiguous) {
		t.Fatalf("fixture did not produce ambiguous consumption: %#v err=%v", source, err)
	}
	return fixture, evidence, source
}

func assertNewDeploymentIntentReconciliationNoAuthority(
	t *testing.T,
	decision BootSessionNewDeploymentIntentConsumptionReconciliation,
) {
	t.Helper()
	if decision.CurrentAdmissionReusable || decision.FreshAdmissionAuthorized ||
		decision.FurtherReadmissionAuthorized || decision.AutomaticRetryAuthorized ||
		decision.BootHandoffAuthorized || decision.ProvisioningAuthorized ||
		decision.SecretInjectionAuthorized || decision.HostMutation ||
		decision.NetworkMutation || decision.ProductionMutation {
		t.Fatalf("reconciliation widened deployment authority: %#v", decision)
	}
}

type newDeploymentIntentObservationErrorStore struct{}

func (newDeploymentIntentObservationErrorStore) ObserveBootSessionConsumption(
	string,
) (BootSessionConsumptionObservation, error) {
	return BootSessionConsumptionObservation{State: BootSessionConsumptionReconciledAmbiguous},
		errors.New("durable observation unavailable")
}
