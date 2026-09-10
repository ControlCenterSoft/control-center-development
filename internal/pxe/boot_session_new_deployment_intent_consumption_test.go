package pxe

import (
	"errors"
	"reflect"
	"testing"
)

func TestBootSessionNewDeploymentIntentConsumptionAuthorizesExactlyOneHandoff(t *testing.T) {
	fixture := bootSessionNewDeploymentIntentFixture(t)
	admission, intentReceipt := bootSessionNewDeploymentIntentConsumptionFixture(t, fixture)
	store, err := NewDirectoryBootSessionConsumptionStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	request := BootSessionConsumptionRequest{
		AttemptID:      "attempt-new-deployment-intent-0001",
		ConsumerID:     "pxe-handoff-consumer-0001",
		ConsumedAtUnix: admission.IssuedAtUnix + 1,
	}

	first, err := consumeBootSessionNewDeploymentIntentFixture(
		store,
		request,
		intentReceipt,
		admission,
		fixture,
	)
	if err != nil {
		t.Fatal(err)
	}
	if first.Status != BootSessionConsumptionCommitted || !first.AtomicConsumeVerified ||
		!first.BootHandoffAuthorized || first.ReplayAuthorized ||
		first.FurtherReadmissionAuthorized || first.AutomaticRetryAuthorized ||
		first.ProvisioningAuthorized || first.SecretInjectionAuthorized ||
		first.HostMutation || first.NetworkMutation || first.ProductionMutation ||
		first.RecoveryRequired || first.RecoveryAction != "none" {
		t.Fatalf("first atomic consume widened or lost authority: %#v", first)
	}
	if first.IntentReceiptID != intentReceipt.ReceiptID ||
		first.IntentID != intentReceipt.IntentID ||
		first.AdmissionID != admission.AdmissionID ||
		first.ReplayKey != admission.ReplayKey ||
		first.ConsumptionReceipt.AdmissionID != admission.AdmissionID ||
		first.ConsumptionReceipt.ReplayKey != admission.ReplayKey {
		t.Fatalf("first atomic consume lost exact new-intent lineage: %#v", first)
	}

	second, err := consumeBootSessionNewDeploymentIntentFixture(
		store,
		request,
		intentReceipt,
		admission,
		fixture,
	)
	if err != nil {
		t.Fatal(err)
	}
	if second.Status != BootSessionConsumptionAlreadyConsumed ||
		!second.AtomicConsumeVerified || second.BootHandoffAuthorized ||
		second.ReplayAuthorized || second.FurtherReadmissionAuthorized ||
		second.AutomaticRetryAuthorized || second.RecoveryRequired ||
		second.RecoveryAction != "none-already-consumed" {
		t.Fatalf("replayed atomic consume regained authority: %#v", second)
	}
	if !reflect.DeepEqual(first.ConsumptionReceipt, second.ConsumptionReceipt) {
		t.Fatalf("idempotent replay returned different durable receipt")
	}
}

func TestBootSessionNewDeploymentIntentConsumptionAmbiguousIsReconciliationOnly(t *testing.T) {
	fixture := bootSessionNewDeploymentIntentFixture(t)
	admission, intentReceipt := bootSessionNewDeploymentIntentConsumptionFixture(t, fixture)
	request := BootSessionConsumptionRequest{
		AttemptID:      "attempt-new-deployment-intent-ambiguous-0001",
		ConsumerID:     "pxe-handoff-consumer-0002",
		ConsumedAtUnix: admission.IssuedAtUnix + 1,
	}

	decision, err := consumeBootSessionNewDeploymentIntentFixture(
		bootSessionNewDeploymentIntentAmbiguousStore{},
		request,
		intentReceipt,
		admission,
		fixture,
	)
	if !errors.Is(err, ErrBootSessionNewDeploymentIntentConsumption) ||
		!errors.Is(err, ErrBootSessionConsumptionAmbiguous) {
		t.Fatalf("ambiguous durable consume did not propagate reconciliation error: %v", err)
	}
	if decision.Status != BootSessionConsumptionAmbiguous ||
		decision.AtomicConsumeVerified || decision.BootHandoffAuthorized ||
		decision.ReplayAuthorized || decision.FurtherReadmissionAuthorized ||
		decision.AutomaticRetryAuthorized || decision.ProvisioningAuthorized ||
		decision.SecretInjectionAuthorized || decision.HostMutation ||
		decision.NetworkMutation || decision.ProductionMutation ||
		!decision.RecoveryRequired ||
		decision.RecoveryAction != "reconcile-new-deployment-intent-replay-key-before-any-further-admission" {
		t.Fatalf("ambiguous consume escaped fail-closed recovery semantics: %#v", decision)
	}
	if decision.IntentReceiptID != intentReceipt.ReceiptID ||
		decision.ConsumptionReceipt.ReplayKey != admission.ReplayKey {
		t.Fatalf("ambiguous consume lost exact intent/replay evidence: %#v", decision)
	}
}

func TestBootSessionNewDeploymentIntentConsumptionRejectsTamperedIntentBoundary(t *testing.T) {
	fixture := bootSessionNewDeploymentIntentFixture(t)
	admission, intentReceipt := bootSessionNewDeploymentIntentConsumptionFixture(t, fixture)
	intentReceipt.NewReplayKey = fixture.terminalAdmission.ReplayKey
	store, err := NewDirectoryBootSessionConsumptionStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	request := BootSessionConsumptionRequest{
		AttemptID:      "attempt-new-deployment-intent-tampered-0001",
		ConsumerID:     "pxe-handoff-consumer-0003",
		ConsumedAtUnix: admission.IssuedAtUnix + 1,
	}

	decision, err := consumeBootSessionNewDeploymentIntentFixture(
		store,
		request,
		intentReceipt,
		admission,
		fixture,
	)
	if !errors.Is(err, ErrBootSessionNewDeploymentIntentConsumption) {
		t.Fatalf("tampered new-intent boundary was accepted: decision=%#v err=%v", decision, err)
	}
	if !reflect.DeepEqual(decision, BootSessionNewDeploymentIntentConsumptionDecision{}) {
		t.Fatalf("tampered boundary returned a partial authority decision: %#v", decision)
	}
}

func TestBootSessionNewDeploymentIntentConsumptionRevalidatesCurrentMedia(t *testing.T) {
	fixture := bootSessionNewDeploymentIntentFixture(t)
	admission, intentReceipt := bootSessionNewDeploymentIntentConsumptionFixture(t, fixture)
	store, err := NewDirectoryBootSessionConsumptionStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	request := BootSessionConsumptionRequest{
		AttemptID:      "attempt-new-deployment-intent-media-0001",
		ConsumerID:     "pxe-handoff-consumer-0004",
		ConsumedAtUnix: admission.IssuedAtUnix + 1,
	}
	fixture.payload = []byte("tampered-current-served-media")

	decision, err := consumeBootSessionNewDeploymentIntentFixture(
		store,
		request,
		intentReceipt,
		admission,
		fixture,
	)
	if !errors.Is(err, ErrBootSessionNewDeploymentIntentConsumption) {
		t.Fatalf("current media integrity drift was accepted: decision=%#v err=%v", decision, err)
	}
	if decision.BootHandoffAuthorized {
		t.Fatalf("media integrity drift authorized boot handoff: %#v", decision)
	}
}

func TestBootSessionNewDeploymentIntentConsumptionRejectsDifferentAttemptOnConsumedReplayKey(t *testing.T) {
	fixture := bootSessionNewDeploymentIntentFixture(t)
	admission, intentReceipt := bootSessionNewDeploymentIntentConsumptionFixture(t, fixture)
	store, err := NewDirectoryBootSessionConsumptionStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	firstRequest := BootSessionConsumptionRequest{
		AttemptID:      "attempt-new-deployment-intent-first-0001",
		ConsumerID:     "pxe-handoff-consumer-0005",
		ConsumedAtUnix: admission.IssuedAtUnix + 1,
	}
	if _, err := consumeBootSessionNewDeploymentIntentFixture(
		store,
		firstRequest,
		intentReceipt,
		admission,
		fixture,
	); err != nil {
		t.Fatal(err)
	}
	secondRequest := firstRequest
	secondRequest.AttemptID = "attempt-new-deployment-intent-second-0002"

	decision, err := consumeBootSessionNewDeploymentIntentFixture(
		store,
		secondRequest,
		intentReceipt,
		admission,
		fixture,
	)
	if !errors.Is(err, ErrBootSessionNewDeploymentIntentConsumption) {
		t.Fatalf("different attempt reused consumed replay key: decision=%#v err=%v", decision, err)
	}
	if decision.BootHandoffAuthorized || decision.ReplayAuthorized ||
		decision.AutomaticRetryAuthorized || decision.FurtherReadmissionAuthorized {
		t.Fatalf("conflicting replay-key attempt received authority: %#v", decision)
	}
}

func bootSessionNewDeploymentIntentConsumptionFixture(
	t *testing.T,
	fixture bootSessionNewDeploymentIntentTestFixture,
) (BootSessionAdmission, BootSessionNewDeploymentIntentReceipt) {
	t.Helper()
	admission, receipt, err := BuildBootSessionNewDeploymentIntent(
		fixture.intent,
		fixture.terminalReconciliation,
		fixture.terminalConsumption,
		fixture.readmission,
		fixture.previous,
		fixture.previousCandidate,
		fixture.previousReconciliation,
		fixture.terminalAdmission,
		fixture.terminalCandidate,
		fixture.servingReceipt,
		fixture.plan,
		fixture.payload,
		nil,
		nil,
		fixture.issuedAt,
	)
	if err != nil {
		t.Fatal(err)
	}
	return admission, receipt
}

func consumeBootSessionNewDeploymentIntentFixture(
	store BootSessionConsumptionStore,
	request BootSessionConsumptionRequest,
	intentReceipt BootSessionNewDeploymentIntentReceipt,
	admission BootSessionAdmission,
	fixture bootSessionNewDeploymentIntentTestFixture,
) (BootSessionNewDeploymentIntentConsumptionDecision, error) {
	return ConsumeBootSessionNewDeploymentIntentAdmission(
		store,
		request,
		intentReceipt,
		admission,
		fixture.intent,
		fixture.terminalReconciliation,
		fixture.terminalConsumption,
		fixture.readmission,
		fixture.previous,
		fixture.previousCandidate,
		fixture.previousReconciliation,
		fixture.terminalAdmission,
		fixture.terminalCandidate,
		fixture.servingReceipt,
		fixture.plan,
		fixture.payload,
		nil,
		nil,
	)
}

type bootSessionNewDeploymentIntentAmbiguousStore struct{}

func (bootSessionNewDeploymentIntentAmbiguousStore) AtomicConsumeBootSession(
	BootSessionConsumptionReceipt,
) (BootSessionConsumptionStoreResult, error) {
	return BootSessionConsumptionStoreResult{State: BootSessionConsumptionAmbiguous}, nil
}
