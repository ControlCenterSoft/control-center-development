package pxe

import (
	"errors"
	"reflect"
	"sync"
	"testing"
)

type bootSessionConsumptionTestStore struct {
	mu         sync.Mutex
	receipts   map[string]BootSessionConsumptionReceipt
	writes     int
	forceState BootSessionConsumptionStoreState
	forceErr   error
}

func (s *bootSessionConsumptionTestStore) AtomicConsumeBootSession(candidate BootSessionConsumptionReceipt) (BootSessionConsumptionStoreResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.forceErr != nil {
		return BootSessionConsumptionStoreResult{}, s.forceErr
	}
	if s.forceState == BootSessionConsumptionAmbiguous {
		return BootSessionConsumptionStoreResult{State: BootSessionConsumptionAmbiguous}, nil
	}
	if s.receipts == nil {
		s.receipts = make(map[string]BootSessionConsumptionReceipt)
	}
	if existing, ok := s.receipts[candidate.ReplayKey]; ok {
		copy := existing
		return BootSessionConsumptionStoreResult{State: BootSessionConsumptionAlreadyConsumed, Existing: &copy}, nil
	}
	s.receipts[candidate.ReplayKey] = candidate
	s.writes++
	return BootSessionConsumptionStoreResult{State: BootSessionConsumptionCommitted}, nil
}

func TestBootSessionConsumptionReceiptDeterministicAndBounded(t *testing.T) {
	sessionRequest, servingReceipt, plan, payload := bootSessionFixture(t)
	const issuedAt = int64(1_800_000_000)
	admission, err := BuildBootSessionAdmission(sessionRequest, servingReceipt, plan, payload, nil, nil, issuedAt)
	if err != nil {
		t.Fatal(err)
	}
	request := BootSessionConsumptionRequest{AttemptID: "attempt-0001", ConsumerID: "edge-0001", ConsumedAtUnix: issuedAt + 1}

	first, err := BuildBootSessionConsumptionReceipt(request, admission)
	if err != nil {
		t.Fatal(err)
	}
	second, err := BuildBootSessionConsumptionReceipt(request, admission)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("consumption receipt is not deterministic: %#v != %#v", first, second)
	}
	if first.AdmissionID != admission.AdmissionID || first.ReplayKey != admission.ReplayKey || first.PlanID != admission.PlanID || first.ServingReceiptID != admission.ServingReceiptID || first.MediaSHA256 != admission.MediaSHA256 || first.Target != admission.Target {
		t.Fatalf("consumption receipt is not bound to exact boot evidence: %#v", first)
	}
	if !first.SingleUse || !first.AtomicConsumeRequired || !first.BootHandoffConsumed || first.ProvisioningAuthorized || first.SecretInjectionAuthorized || first.HostMutation || first.NetworkMutation || first.ReplayAuthorized {
		t.Fatalf("consumption receipt authority is not bounded: %#v", first)
	}
	if err := VerifyBootSessionConsumptionReceipt(first, request, admission); err != nil {
		t.Fatalf("valid receipt rejected: %v", err)
	}
}

func TestConsumeBootSessionAdmissionAuthorizesExactlyOnce(t *testing.T) {
	sessionRequest, servingReceipt, plan, payload := bootSessionFixture(t)
	const issuedAt = int64(1_800_000_000)
	admission, err := BuildBootSessionAdmission(sessionRequest, servingReceipt, plan, payload, nil, nil, issuedAt)
	if err != nil {
		t.Fatal(err)
	}
	request := BootSessionConsumptionRequest{AttemptID: "attempt-0001", ConsumerID: "edge-0001", ConsumedAtUnix: issuedAt + 1}
	store := &bootSessionConsumptionTestStore{}

	first, err := ConsumeBootSessionAdmission(store, request, admission, sessionRequest, servingReceipt, plan, payload, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if first.Status != BootSessionConsumptionCommitted || !first.AtomicConsumeVerified || !first.BootHandoffAuthorized || first.ReplayAuthorized || first.RecoveryRequired {
		t.Fatalf("first consume did not produce one-shot authorization: %#v", first)
	}

	replay, err := ConsumeBootSessionAdmission(store, request, admission, sessionRequest, servingReceipt, plan, payload, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if replay.Status != BootSessionConsumptionAlreadyConsumed || !replay.AtomicConsumeVerified || replay.BootHandoffAuthorized || replay.ReplayAuthorized || replay.RecoveryRequired {
		t.Fatalf("exact replay was not safely idempotent: %#v", replay)
	}
	if store.writes != 1 || !reflect.DeepEqual(first.Receipt, replay.Receipt) {
		t.Fatalf("exact replay changed durable consumption evidence: writes=%d first=%#v replay=%#v", store.writes, first.Receipt, replay.Receipt)
	}
}

func TestConsumeBootSessionAdmissionConcurrentSingleAuthorization(t *testing.T) {
	sessionRequest, servingReceipt, plan, payload := bootSessionFixture(t)
	const issuedAt = int64(1_800_000_000)
	admission, err := BuildBootSessionAdmission(sessionRequest, servingReceipt, plan, payload, nil, nil, issuedAt)
	if err != nil {
		t.Fatal(err)
	}
	request := BootSessionConsumptionRequest{AttemptID: "attempt-concurrent", ConsumerID: "edge-0001", ConsumedAtUnix: issuedAt + 1}
	store := &bootSessionConsumptionTestStore{}

	const workers = 32
	decisions := make(chan BootSessionConsumptionDecision, workers)
	errs := make(chan error, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			decision, consumeErr := ConsumeBootSessionAdmission(store, request, admission, sessionRequest, servingReceipt, plan, payload, nil, nil)
			if consumeErr != nil {
				errs <- consumeErr
				return
			}
			decisions <- decision
		}()
	}
	wg.Wait()
	close(decisions)
	close(errs)
	for err := range errs {
		t.Fatalf("concurrent consume failed: %v", err)
	}

	authorized := 0
	for decision := range decisions {
		if decision.BootHandoffAuthorized {
			authorized++
		}
	}
	if authorized != 1 || store.writes != 1 {
		t.Fatalf("single-use CAS violated: authorized=%d writes=%d", authorized, store.writes)
	}
}

func TestConsumeBootSessionAdmissionRejectsConflictingReplayBinding(t *testing.T) {
	sessionRequest, servingReceipt, plan, payload := bootSessionFixture(t)
	const issuedAt = int64(1_800_000_000)
	admission, err := BuildBootSessionAdmission(sessionRequest, servingReceipt, plan, payload, nil, nil, issuedAt)
	if err != nil {
		t.Fatal(err)
	}
	store := &bootSessionConsumptionTestStore{}
	first := BootSessionConsumptionRequest{AttemptID: "attempt-0001", ConsumerID: "edge-0001", ConsumedAtUnix: issuedAt + 1}
	if _, err := ConsumeBootSessionAdmission(store, first, admission, sessionRequest, servingReceipt, plan, payload, nil, nil); err != nil {
		t.Fatal(err)
	}

	conflicting := first
	conflicting.AttemptID = "attempt-0002"
	decision, err := ConsumeBootSessionAdmission(store, conflicting, admission, sessionRequest, servingReceipt, plan, payload, nil, nil)
	if !errors.Is(err, ErrBootSessionConsumption) {
		t.Fatalf("conflicting replay binding: expected fail-closed error, got decision=%#v err=%v", decision, err)
	}
	if decision.BootHandoffAuthorized || store.writes != 1 {
		t.Fatalf("conflicting replay gained authorization or wrote twice: %#v writes=%d", decision, store.writes)
	}
}

func TestConsumeBootSessionAdmissionFailsClosedOnAmbiguousOutcome(t *testing.T) {
	sessionRequest, servingReceipt, plan, payload := bootSessionFixture(t)
	const issuedAt = int64(1_800_000_000)
	admission, err := BuildBootSessionAdmission(sessionRequest, servingReceipt, plan, payload, nil, nil, issuedAt)
	if err != nil {
		t.Fatal(err)
	}
	request := BootSessionConsumptionRequest{AttemptID: "attempt-0001", ConsumerID: "edge-0001", ConsumedAtUnix: issuedAt + 1}

	for _, store := range []*bootSessionConsumptionTestStore{
		{forceState: BootSessionConsumptionAmbiguous},
		{forceErr: errors.New("commit acknowledgement lost")},
	} {
		decision, err := ConsumeBootSessionAdmission(store, request, admission, sessionRequest, servingReceipt, plan, payload, nil, nil)
		if !errors.Is(err, ErrBootSessionConsumptionAmbiguous) {
			t.Fatalf("ambiguous consume: expected reconciliation error, got decision=%#v err=%v", decision, err)
		}
		if decision.BootHandoffAuthorized || decision.AtomicConsumeVerified || !decision.RecoveryRequired || decision.ReplayAuthorized || decision.RecoveryAction != "reconcile-replay-key-before-reissuing-session" {
			t.Fatalf("ambiguous consume did not fail closed: %#v", decision)
		}
	}
}

func TestConsumeBootSessionAdmissionRevalidatesCurrentEvidence(t *testing.T) {
	sessionRequest, servingReceipt, plan, payload := bootSessionFixture(t)
	const issuedAt = int64(1_800_000_000)
	admission, err := BuildBootSessionAdmission(sessionRequest, servingReceipt, plan, payload, nil, nil, issuedAt)
	if err != nil {
		t.Fatal(err)
	}
	request := BootSessionConsumptionRequest{AttemptID: "attempt-0001", ConsumerID: "edge-0001", ConsumedAtUnix: issuedAt + 1}
	store := &bootSessionConsumptionTestStore{}

	if decision, err := ConsumeBootSessionAdmission(store, request, admission, sessionRequest, servingReceipt, plan, []byte("drifted-media"), nil, nil); !errors.Is(err, ErrBootSessionConsumption) || decision.BootHandoffAuthorized {
		t.Fatalf("served-media drift: expected fail-closed rejection, got decision=%#v err=%v", decision, err)
	}
	expired := request
	expired.ConsumedAtUnix = admission.ExpiresAtUnix
	if decision, err := ConsumeBootSessionAdmission(store, expired, admission, sessionRequest, servingReceipt, plan, payload, nil, nil); !errors.Is(err, ErrBootSessionConsumption) || decision.BootHandoffAuthorized {
		t.Fatalf("expired admission: expected fail-closed rejection, got decision=%#v err=%v", decision, err)
	}
	if store.writes != 0 {
		t.Fatalf("invalid evidence reached atomic store: writes=%d", store.writes)
	}
}

func TestConsumeBootSessionAdmissionRejectsTamperedPersistedReceipt(t *testing.T) {
	sessionRequest, servingReceipt, plan, payload := bootSessionFixture(t)
	const issuedAt = int64(1_800_000_000)
	admission, err := BuildBootSessionAdmission(sessionRequest, servingReceipt, plan, payload, nil, nil, issuedAt)
	if err != nil {
		t.Fatal(err)
	}
	request := BootSessionConsumptionRequest{AttemptID: "attempt-0001", ConsumerID: "edge-0001", ConsumedAtUnix: issuedAt + 1}
	candidate, err := BuildBootSessionConsumptionReceipt(request, admission)
	if err != nil {
		t.Fatal(err)
	}
	candidate.ReplayAuthorized = true
	store := &bootSessionConsumptionTestStore{receipts: map[string]BootSessionConsumptionReceipt{admission.ReplayKey: candidate}}

	decision, err := ConsumeBootSessionAdmission(store, request, admission, sessionRequest, servingReceipt, plan, payload, nil, nil)
	if !errors.Is(err, ErrBootSessionConsumption) {
		t.Fatalf("tampered persisted receipt: expected fail-closed error, got decision=%#v err=%v", decision, err)
	}
	if decision.BootHandoffAuthorized {
		t.Fatalf("tampered persisted receipt gained boot authorization: %#v", decision)
	}
}
