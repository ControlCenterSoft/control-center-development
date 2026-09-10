package pxe

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestDirectoryBootSessionConsumptionStoreObservesDefinitelyAbsent(t *testing.T) {
	receipt := bootSessionConsumptionStoreFixture(t, "attempt-observe-absent")
	store, err := NewDirectoryBootSessionConsumptionStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	observation, err := store.ObserveBootSessionConsumption(receipt.ReplayKey)
	if err != nil {
		t.Fatal(err)
	}
	if observation.State != BootSessionConsumptionReconciledDefinitelyAbsent || observation.Existing != nil {
		t.Fatalf("unexpected absent observation: %#v", observation)
	}
}

func TestDirectoryBootSessionConsumptionStoreObservesConsumedAcrossRestart(t *testing.T) {
	receipt := bootSessionConsumptionStoreFixture(t, "attempt-observe-restart")
	root := t.TempDir()
	store, err := NewDirectoryBootSessionConsumptionStore(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.AtomicConsumeBootSession(receipt); err != nil {
		t.Fatal(err)
	}

	reopened, err := NewDirectoryBootSessionConsumptionStore(root)
	if err != nil {
		t.Fatal(err)
	}
	observation, err := reopened.ObserveBootSessionConsumption(receipt.ReplayKey)
	if err != nil {
		t.Fatal(err)
	}
	if observation.State != BootSessionConsumptionReconciledConsumed || observation.Existing == nil {
		t.Fatalf("restart did not preserve consumed state: %#v", observation)
	}
	if !reflect.DeepEqual(receipt, *observation.Existing) {
		t.Fatalf("restart changed observed receipt: got=%#v want=%#v", *observation.Existing, receipt)
	}
}

func TestDirectoryBootSessionConsumptionStoreCorruptionIsAmbiguous(t *testing.T) {
	receipt := bootSessionConsumptionStoreFixture(t, "attempt-observe-corrupt")
	store, err := NewDirectoryBootSessionConsumptionStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(store.root, receipt.ReplayKey+".json")
	if err := os.WriteFile(path, []byte("{not-json\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	observation, err := store.ObserveBootSessionConsumption(receipt.ReplayKey)
	if err == nil || !errors.Is(err, ErrBootSessionConsumptionStore) {
		t.Fatalf("corrupt state was not rejected: observation=%#v err=%v", observation, err)
	}
	if observation.State != BootSessionConsumptionReconciledAmbiguous || observation.Existing != nil {
		t.Fatalf("corrupt state did not fail closed: %#v", observation)
	}
}

func TestReconcileBootSessionConsumptionDefinitelyAbsentRequiresFreshAdmission(t *testing.T) {
	admission, candidate := bootSessionConsumptionReconciliationFixture(t, "attempt-reconcile-absent")
	store, err := NewDirectoryBootSessionConsumptionStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	first, err := ReconcileBootSessionConsumption(store, candidate, admission, admission.ExpiresAtUnix)
	if err != nil {
		t.Fatal(err)
	}
	second, err := ReconcileBootSessionConsumption(store, candidate, admission, admission.ExpiresAtUnix)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("stable absent evidence produced non-deterministic reconciliation: first=%#v second=%#v", first, second)
	}
	if first.State != BootSessionConsumptionReconciledDefinitelyAbsent ||
		!first.OldAdmissionExpired || first.OldAdmissionReusable || !first.FreshAdmissionRequired || !first.FreshServingEvidenceRequired ||
		first.BootHandoffAuthorized || first.ReplayAuthorized || first.ProvisioningAuthorized ||
		first.SecretInjectionAuthorized || first.HostMutation || first.NetworkMutation ||
		!first.RecoveryRequired || first.RecoveryAction != "reissue-new-session-from-fresh-serving-evidence" {
		t.Fatalf("absent reconciliation weakened recovery boundary: %#v", first)
	}
	if !bootSessionConsumptionCanonicalSHA256(first.ReconciliationID) {
		t.Fatalf("reconciliation identity is not canonical SHA-256: %q", first.ReconciliationID)
	}
}

func TestReconcileBootSessionConsumptionAbsentBeforeExpiryDoesNotPermitReissue(t *testing.T) {
	admission, candidate := bootSessionConsumptionReconciliationFixture(t, "attempt-reconcile-unexpired")
	store, err := NewDirectoryBootSessionConsumptionStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	decision, err := ReconcileBootSessionConsumption(store, candidate, admission, candidate.ConsumedAtUnix+1)
	if err != nil {
		t.Fatal(err)
	}
	if decision.State != BootSessionConsumptionReconciledDefinitelyAbsent || decision.OldAdmissionExpired ||
		decision.OldAdmissionReusable || decision.FreshAdmissionRequired || decision.FreshServingEvidenceRequired ||
		decision.BootHandoffAuthorized || decision.ReplayAuthorized || !decision.RecoveryRequired ||
		decision.RecoveryAction != "wait-for-old-admission-expiry-before-reissuing-session" {
		t.Fatalf("unexpired absent state permitted unsafe reissue: %#v", decision)
	}
}

func TestReconcileBootSessionConsumptionConsumedNeverAuthorizesReplay(t *testing.T) {
	admission, candidate := bootSessionConsumptionReconciliationFixture(t, "attempt-reconcile-consumed")
	store, err := NewDirectoryBootSessionConsumptionStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.AtomicConsumeBootSession(candidate); err != nil {
		t.Fatal(err)
	}

	decision, err := ReconcileBootSessionConsumption(store, candidate, admission, admission.ExpiresAtUnix)
	if err != nil {
		t.Fatal(err)
	}
	if decision.State != BootSessionConsumptionReconciledConsumed || decision.ObservedReceiptID != candidate.ReceiptID {
		t.Fatalf("consumed reconciliation lost durable receipt lineage: %#v", decision)
	}
	if !decision.OldAdmissionExpired || decision.OldAdmissionReusable || !decision.FreshAdmissionRequired || !decision.FreshServingEvidenceRequired ||
		decision.BootHandoffAuthorized || decision.ReplayAuthorized || decision.ProvisioningAuthorized ||
		decision.SecretInjectionAuthorized || decision.HostMutation || decision.NetworkMutation ||
		decision.RecoveryRequired || decision.RecoveryAction != "do-not-reuse-consumed-session" {
		t.Fatalf("consumed reconciliation granted unsafe authority: %#v", decision)
	}
}

func TestReconcileBootSessionConsumptionDifferentAttemptStillConsumed(t *testing.T) {
	admission, persisted := bootSessionConsumptionReconciliationFixture(t, "attempt-persisted")
	candidate, err := BuildBootSessionConsumptionReceipt(BootSessionConsumptionRequest{
		AttemptID:      "attempt-later-observer",
		ConsumerID:     persisted.ConsumerID,
		ConsumedAtUnix: persisted.ConsumedAtUnix,
	}, admission)
	if err != nil {
		t.Fatal(err)
	}
	if candidate.ReplayKey != persisted.ReplayKey || candidate.ReceiptID == persisted.ReceiptID {
		t.Fatal("fixture did not create a distinct receipt on the same replay key")
	}
	store, err := NewDirectoryBootSessionConsumptionStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.AtomicConsumeBootSession(persisted); err != nil {
		t.Fatal(err)
	}

	decision, err := ReconcileBootSessionConsumption(store, candidate, admission, admission.ExpiresAtUnix)
	if err != nil {
		t.Fatal(err)
	}
	if decision.State != BootSessionConsumptionReconciledConsumed || decision.ObservedReceiptID != persisted.ReceiptID {
		t.Fatalf("existing valid consumption was not authoritative: %#v", decision)
	}
	if decision.BootHandoffAuthorized || decision.ReplayAuthorized || decision.OldAdmissionReusable {
		t.Fatalf("different-attempt reconciliation enabled replay: %#v", decision)
	}
}

func TestReconcileBootSessionConsumptionRejectsTamperedCandidate(t *testing.T) {
	admission, candidate := bootSessionConsumptionReconciliationFixture(t, "attempt-reconcile-tamper")
	candidate.ReplayAuthorized = true
	store, err := NewDirectoryBootSessionConsumptionStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	decision, err := ReconcileBootSessionConsumption(store, candidate, admission, admission.ExpiresAtUnix)
	if err == nil || !errors.Is(err, ErrBootSessionConsumptionReconciliation) {
		t.Fatalf("tampered candidate was accepted: decision=%#v err=%v", decision, err)
	}
	if decision != (BootSessionConsumptionReconciliation{}) {
		t.Fatalf("tampered candidate produced usable evidence: %#v", decision)
	}
}

func TestReconcileBootSessionConsumptionAmbiguousStoreFailsClosed(t *testing.T) {
	admission, candidate := bootSessionConsumptionReconciliationFixture(t, "attempt-reconcile-ambiguous")
	store := bootSessionConsumptionObservationStoreStub{
		observation: BootSessionConsumptionObservation{State: BootSessionConsumptionReconciledAmbiguous},
		err:         errors.New("durability unknown"),
	}

	decision, err := ReconcileBootSessionConsumption(store, candidate, admission, admission.ExpiresAtUnix)
	if err == nil || !errors.Is(err, ErrBootSessionConsumptionReconciliation) {
		t.Fatalf("ambiguous store was not surfaced: decision=%#v err=%v", decision, err)
	}
	if decision.State != BootSessionConsumptionReconciledAmbiguous ||
		decision.OldAdmissionReusable || decision.FreshAdmissionRequired || decision.FreshServingEvidenceRequired ||
		decision.BootHandoffAuthorized || decision.ReplayAuthorized || decision.ProvisioningAuthorized ||
		decision.SecretInjectionAuthorized || decision.HostMutation || decision.NetworkMutation ||
		!decision.RecoveryRequired || decision.RecoveryAction != "operator-reconcile-durable-consumption-store" {
		t.Fatalf("ambiguous reconciliation did not fail closed: %#v", decision)
	}
}

func TestReconcileBootSessionConsumptionRejectsUnknownObservationState(t *testing.T) {
	admission, candidate := bootSessionConsumptionReconciliationFixture(t, "attempt-reconcile-unknown")
	store := bootSessionConsumptionObservationStoreStub{
		observation: BootSessionConsumptionObservation{State: "future-state"},
	}

	decision, err := ReconcileBootSessionConsumption(store, candidate, admission, admission.ExpiresAtUnix)
	if err == nil || !errors.Is(err, ErrBootSessionConsumptionReconciliation) {
		t.Fatalf("unknown store state was accepted: decision=%#v err=%v", decision, err)
	}
	if decision.State != BootSessionConsumptionReconciledAmbiguous || decision.FreshAdmissionRequired ||
		decision.BootHandoffAuthorized || decision.ReplayAuthorized || !decision.RecoveryRequired {
		t.Fatalf("unknown observation did not fail closed: %#v", decision)
	}
}

type bootSessionConsumptionObservationStoreStub struct {
	observation BootSessionConsumptionObservation
	err         error
}

func (s bootSessionConsumptionObservationStoreStub) ObserveBootSessionConsumption(
	string,
) (BootSessionConsumptionObservation, error) {
	return s.observation, s.err
}

func bootSessionConsumptionReconciliationFixture(
	t *testing.T,
	attemptID string,
) (BootSessionAdmission, BootSessionConsumptionReceipt) {
	t.Helper()
	sessionRequest, servingReceipt, plan, payload := bootSessionFixture(t)
	const issuedAt = int64(1_800_000_000)
	admission, err := BuildBootSessionAdmission(sessionRequest, servingReceipt, plan, payload, nil, nil, issuedAt)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := BuildBootSessionConsumptionReceipt(BootSessionConsumptionRequest{
		AttemptID:      attemptID,
		ConsumerID:     "edge-reconcile-0001",
		ConsumedAtUnix: issuedAt + 1,
	}, admission)
	if err != nil {
		t.Fatal(err)
	}
	return admission, receipt
}
