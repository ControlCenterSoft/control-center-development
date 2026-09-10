package pxe

import (
	"errors"
	"testing"
)

func TestBootSessionReadmissionConsumptionCommitsOnceAndTerminatesRecovery(t *testing.T) {
	previous, candidate, reconciliation, request, servingReceipt, plan, payload := bootSessionReadmissionFixture(t)
	fresh, readmission, err := BuildBootSessionReadmission(
		previous,
		candidate,
		reconciliation,
		request,
		servingReceipt,
		plan,
		payload,
		nil,
		nil,
		reconciliation.ObservedAtUnix,
	)
	if err != nil {
		t.Fatal(err)
	}
	store, err := NewDirectoryBootSessionConsumptionStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	consumeRequest := BootSessionConsumptionRequest{
		AttemptID:      "attempt-readmission-terminal",
		ConsumerID:     "edge-readmission-terminal",
		ConsumedAtUnix: fresh.IssuedAtUnix + 1,
	}

	first, err := ConsumeBootSessionReadmission(
		store,
		consumeRequest,
		previous,
		candidate,
		reconciliation,
		fresh,
		readmission,
		request,
		servingReceipt,
		plan,
		payload,
		nil,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if first.Status != BootSessionConsumptionCommitted || !first.AtomicConsumeVerified ||
		!first.BootHandoffAuthorized || first.RecoveryRequired ||
		first.FurtherReadmissionAuthorized || first.AutomaticRetryAuthorized {
		t.Fatalf("first readmission consume was not bounded: %#v", first)
	}
	if first.Receipt.RecoveryGeneration != 1 || first.Receipt.MaxRecoveryGenerations != 1 ||
		!first.Receipt.AtomicConsumeVerified || !first.Receipt.BootHandoffConsumed ||
		first.Receipt.BootHandoffAuthorized || first.Receipt.FurtherReadmissionAuthorized ||
		first.Receipt.AutomaticRetryAuthorized || first.Receipt.ProvisioningAuthorized ||
		first.Receipt.SecretInjectionAuthorized || first.Receipt.HostMutation ||
		first.Receipt.NetworkMutation || first.Receipt.ProductionMutation {
		t.Fatalf("terminal receipt widened authority: %#v", first.Receipt)
	}
	if err := VerifyBootSessionReadmissionConsumptionReceipt(
		first.Receipt,
		readmission,
		previous,
		candidate,
		reconciliation,
		fresh,
		first.ConsumptionReceipt,
	); err != nil {
		t.Fatalf("valid terminal consumption receipt rejected: %v", err)
	}

	replay, err := ConsumeBootSessionReadmission(
		store,
		consumeRequest,
		previous,
		candidate,
		reconciliation,
		fresh,
		readmission,
		request,
		servingReceipt,
		plan,
		payload,
		nil,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if replay.Status != BootSessionConsumptionAlreadyConsumed || !replay.AtomicConsumeVerified ||
		replay.BootHandoffAuthorized || replay.RecoveryRequired ||
		replay.FurtherReadmissionAuthorized || replay.AutomaticRetryAuthorized {
		t.Fatalf("idempotent replay reopened boot authority: %#v", replay)
	}
}

func TestBootSessionReadmissionConsumptionAmbiguousRequiresOperatorWithoutRetryAuthority(t *testing.T) {
	previous, candidate, reconciliation, request, servingReceipt, plan, payload := bootSessionReadmissionFixture(t)
	fresh, readmission, err := BuildBootSessionReadmission(
		previous,
		candidate,
		reconciliation,
		request,
		servingReceipt,
		plan,
		payload,
		nil,
		nil,
		reconciliation.ObservedAtUnix,
	)
	if err != nil {
		t.Fatal(err)
	}
	store := &bootSessionReadmissionConsumptionStoreStub{
		result: BootSessionConsumptionStoreResult{State: BootSessionConsumptionAmbiguous},
		err:    errors.New("durability unavailable"),
	}
	consumeRequest := BootSessionConsumptionRequest{
		AttemptID:      "attempt-readmission-ambiguous",
		ConsumerID:     "edge-readmission-ambiguous",
		ConsumedAtUnix: fresh.IssuedAtUnix + 1,
	}

	decision, err := ConsumeBootSessionReadmission(
		store,
		consumeRequest,
		previous,
		candidate,
		reconciliation,
		fresh,
		readmission,
		request,
		servingReceipt,
		plan,
		payload,
		nil,
		nil,
	)
	if !errors.Is(err, ErrBootSessionReadmissionConsumption) {
		t.Fatalf("ambiguous durability: expected bounded error, got %v", err)
	}
	if store.calls != 1 || decision.Status != BootSessionConsumptionAmbiguous ||
		decision.AtomicConsumeVerified || decision.BootHandoffAuthorized ||
		!decision.RecoveryRequired || decision.FurtherReadmissionAuthorized ||
		decision.AutomaticRetryAuthorized ||
		decision.RecoveryAction != "operator-reconcile-readmitted-replay-key-no-automatic-reissue" {
		t.Fatalf("ambiguous readmission did not fail closed: %#v calls=%d", decision, store.calls)
	}
	if err := VerifyBootSessionReadmissionConsumptionReceipt(
		decision.Receipt,
		readmission,
		previous,
		candidate,
		reconciliation,
		fresh,
		decision.ConsumptionReceipt,
	); err != nil {
		t.Fatalf("ambiguous terminal evidence rejected: %v", err)
	}
}

func TestBootSessionReadmissionConsumptionRejectsLineageAndMediaDriftBeforeStore(t *testing.T) {
	previous, candidate, reconciliation, request, servingReceipt, plan, payload := bootSessionReadmissionFixture(t)
	fresh, readmission, err := BuildBootSessionReadmission(
		previous,
		candidate,
		reconciliation,
		request,
		servingReceipt,
		plan,
		payload,
		nil,
		nil,
		reconciliation.ObservedAtUnix,
	)
	if err != nil {
		t.Fatal(err)
	}
	consumeRequest := BootSessionConsumptionRequest{
		AttemptID:      "attempt-readmission-preflight",
		ConsumerID:     "edge-readmission-preflight",
		ConsumedAtUnix: fresh.IssuedAtUnix + 1,
	}

	forged := readmission
	forged.FreshReplayKey = previous.ReplayKey
	store := &bootSessionReadmissionConsumptionStoreStub{}
	if _, err := ConsumeBootSessionReadmission(
		store,
		consumeRequest,
		previous,
		candidate,
		reconciliation,
		fresh,
		forged,
		request,
		servingReceipt,
		plan,
		payload,
		nil,
		nil,
	); !errors.Is(err, ErrBootSessionReadmissionConsumption) {
		t.Fatalf("forged readmission lineage was accepted: %v", err)
	}
	if store.calls != 0 {
		t.Fatalf("store was touched before lineage validation: %d", store.calls)
	}

	if _, err := ConsumeBootSessionReadmission(
		store,
		consumeRequest,
		previous,
		candidate,
		reconciliation,
		fresh,
		readmission,
		request,
		servingReceipt,
		plan,
		[]byte("tampered-serving-media"),
		nil,
		nil,
	); !errors.Is(err, ErrBootSessionReadmissionConsumption) {
		t.Fatalf("serving-media drift was accepted: %v", err)
	}
	if store.calls != 0 {
		t.Fatalf("store was touched before media validation: %d", store.calls)
	}
}

func TestBootSessionReadmissionConsumptionTreatsMalformedCommittedResultAsAmbiguous(t *testing.T) {
	previous, candidate, reconciliation, request, servingReceipt, plan, payload := bootSessionReadmissionFixture(t)
	fresh, readmission, err := BuildBootSessionReadmission(
		previous,
		candidate,
		reconciliation,
		request,
		servingReceipt,
		plan,
		payload,
		nil,
		nil,
		reconciliation.ObservedAtUnix,
	)
	if err != nil {
		t.Fatal(err)
	}
	consumeRequest := BootSessionConsumptionRequest{
		AttemptID:      "attempt-readmission-malformed",
		ConsumerID:     "edge-readmission-malformed",
		ConsumedAtUnix: fresh.IssuedAtUnix + 1,
	}
	expected, err := BuildBootSessionConsumptionReceipt(consumeRequest, fresh)
	if err != nil {
		t.Fatal(err)
	}
	store := &bootSessionReadmissionConsumptionStoreStub{
		result: BootSessionConsumptionStoreResult{
			State:    BootSessionConsumptionCommitted,
			Existing: &expected,
		},
	}

	decision, err := ConsumeBootSessionReadmission(
		store,
		consumeRequest,
		previous,
		candidate,
		reconciliation,
		fresh,
		readmission,
		request,
		servingReceipt,
		plan,
		payload,
		nil,
		nil,
	)
	if !errors.Is(err, ErrBootSessionReadmissionConsumption) {
		t.Fatalf("malformed committed result was accepted: %v", err)
	}
	if decision.Status != BootSessionConsumptionAmbiguous || decision.BootHandoffAuthorized ||
		!decision.RecoveryRequired || decision.AutomaticRetryAuthorized ||
		decision.FurtherReadmissionAuthorized {
		t.Fatalf("malformed committed result did not fail closed: %#v", decision)
	}
}

func TestVerifyBootSessionReadmissionConsumptionReceiptRejectsGenerationAndAuthorityDrift(t *testing.T) {
	previous, candidate, reconciliation, request, servingReceipt, plan, payload := bootSessionReadmissionFixture(t)
	fresh, readmission, err := BuildBootSessionReadmission(
		previous,
		candidate,
		reconciliation,
		request,
		servingReceipt,
		plan,
		payload,
		nil,
		nil,
		reconciliation.ObservedAtUnix,
	)
	if err != nil {
		t.Fatal(err)
	}
	store, err := NewDirectoryBootSessionConsumptionStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	consumeRequest := BootSessionConsumptionRequest{
		AttemptID:      "attempt-readmission-verify",
		ConsumerID:     "edge-readmission-verify",
		ConsumedAtUnix: fresh.IssuedAtUnix + 1,
	}
	decision, err := ConsumeBootSessionReadmission(
		store,
		consumeRequest,
		previous,
		candidate,
		reconciliation,
		fresh,
		readmission,
		request,
		servingReceipt,
		plan,
		payload,
		nil,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}

	cases := []BootSessionReadmissionConsumptionReceipt{
		decision.Receipt,
		decision.Receipt,
		decision.Receipt,
		decision.Receipt,
	}
	cases[0].RecoveryGeneration = 2
	cases[1].FurtherReadmissionAuthorized = true
	cases[2].AutomaticRetryAuthorized = true
	cases[3].ProductionMutation = true
	for _, tampered := range cases {
		if err := VerifyBootSessionReadmissionConsumptionReceipt(
			tampered,
			readmission,
			previous,
			candidate,
			reconciliation,
			fresh,
			decision.ConsumptionReceipt,
		); !errors.Is(err, ErrBootSessionReadmissionConsumption) {
			t.Fatalf("unsafe terminal receipt accepted: %#v err=%v", tampered, err)
		}
	}
}

type bootSessionReadmissionConsumptionStoreStub struct {
	calls  int
	result BootSessionConsumptionStoreResult
	err    error
}

func (s *bootSessionReadmissionConsumptionStoreStub) AtomicConsumeBootSession(
	BootSessionConsumptionReceipt,
) (BootSessionConsumptionStoreResult, error) {
	s.calls++
	return s.result, s.err
}
