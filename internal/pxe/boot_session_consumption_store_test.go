package pxe

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
)

func TestDirectoryBootSessionConsumptionStorePersistsAcrossRestart(t *testing.T) {
	receipt := bootSessionConsumptionStoreFixture(t, "attempt-restart")
	root := t.TempDir()

	store, err := NewDirectoryBootSessionConsumptionStore(root)
	if err != nil {
		t.Fatal(err)
	}
	first, err := store.AtomicConsumeBootSession(receipt)
	if err != nil {
		t.Fatal(err)
	}
	if first.State != BootSessionConsumptionCommitted || first.Existing != nil {
		t.Fatalf("first durable consume was not committed: %#v", first)
	}

	reopened, err := NewDirectoryBootSessionConsumptionStore(root)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := reopened.AtomicConsumeBootSession(receipt)
	if err != nil {
		t.Fatal(err)
	}
	if replay.State != BootSessionConsumptionAlreadyConsumed || replay.Existing == nil {
		t.Fatalf("restart did not preserve single-use evidence: %#v", replay)
	}
	if !reflect.DeepEqual(receipt, *replay.Existing) {
		t.Fatalf("restart changed durable receipt: got=%#v want=%#v", *replay.Existing, receipt)
	}
}

func TestDirectoryBootSessionConsumptionStoreConcurrentCAS(t *testing.T) {
	receipt := bootSessionConsumptionStoreFixture(t, "attempt-concurrent-store")
	store, err := NewDirectoryBootSessionConsumptionStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	const workers = 32
	results := make(chan BootSessionConsumptionStoreResult, workers)
	errs := make(chan error, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, consumeErr := store.AtomicConsumeBootSession(receipt)
			if consumeErr != nil {
				errs <- consumeErr
				return
			}
			results <- result
		}()
	}
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		t.Fatalf("concurrent durable consume failed: %v", err)
	}

	committed := 0
	alreadyConsumed := 0
	for result := range results {
		switch result.State {
		case BootSessionConsumptionCommitted:
			committed++
		case BootSessionConsumptionAlreadyConsumed:
			alreadyConsumed++
		default:
			t.Fatalf("unexpected durable state: %#v", result)
		}
	}
	if committed != 1 || alreadyConsumed != workers-1 {
		t.Fatalf("durable single-use CAS violated: committed=%d already=%d", committed, alreadyConsumed)
	}
}

func TestDirectoryBootSessionConsumptionStoreRejectsUnsafeCandidate(t *testing.T) {
	receipt := bootSessionConsumptionStoreFixture(t, "attempt-unsafe")
	receipt.ReplayAuthorized = true
	store, err := NewDirectoryBootSessionConsumptionStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	result, err := store.AtomicConsumeBootSession(receipt)
	if err == nil || !errors.Is(err, ErrBootSessionConsumptionStore) {
		t.Fatalf("unsafe receipt was not rejected: result=%#v err=%v", result, err)
	}
	if result.State != BootSessionConsumptionAmbiguous {
		t.Fatalf("unsafe receipt did not fail closed: %#v", result)
	}
	entries, readErr := os.ReadDir(store.root)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if len(entries) != 0 {
		t.Fatalf("unsafe receipt created durable state: %#v", entries)
	}
}

func TestDirectoryBootSessionConsumptionStoreRejectsCorruptExistingRecord(t *testing.T) {
	receipt := bootSessionConsumptionStoreFixture(t, "attempt-corrupt")
	store, err := NewDirectoryBootSessionConsumptionStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(store.root, receipt.ReplayKey+".json")
	if err := os.WriteFile(path, []byte("{not-json\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	result, err := store.AtomicConsumeBootSession(receipt)
	if err == nil || !errors.Is(err, ErrBootSessionConsumptionStore) {
		t.Fatalf("corrupt durable state was not rejected: result=%#v err=%v", result, err)
	}
	if result.State != BootSessionConsumptionAmbiguous || result.Existing != nil {
		t.Fatalf("corrupt durable state did not fail closed: %#v", result)
	}
}

func TestDirectoryBootSessionConsumptionStoreRejectsUnknownRecordFields(t *testing.T) {
	receipt := bootSessionConsumptionStoreFixture(t, "attempt-unknown-field")
	store, err := NewDirectoryBootSessionConsumptionStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	record := map[string]any{
		"recordVersion": bootSessionConsumptionStoreRecordVersion,
		"receipt":       receipt,
		"unsafe":        true,
	}
	encoded, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(store.root, receipt.ReplayKey+".json")
	if err := os.WriteFile(path, append(encoded, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}

	result, err := store.AtomicConsumeBootSession(receipt)
	if err == nil || !errors.Is(err, ErrBootSessionConsumptionStore) {
		t.Fatalf("unknown durable field was not rejected: result=%#v err=%v", result, err)
	}
	if result.State != BootSessionConsumptionAmbiguous {
		t.Fatalf("unknown durable field did not fail closed: %#v", result)
	}
}

func TestDirectoryBootSessionConsumptionStoreKeepsReplayKeyBound(t *testing.T) {
	receipt := bootSessionConsumptionStoreFixture(t, "attempt-original")
	store, err := NewDirectoryBootSessionConsumptionStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.AtomicConsumeBootSession(receipt); err != nil {
		t.Fatal(err)
	}

	conflict := receipt
	conflict.AttemptID = "attempt-conflict"
	result, err := store.AtomicConsumeBootSession(conflict)
	if err != nil {
		t.Fatal(err)
	}
	if result.State != BootSessionConsumptionAlreadyConsumed || result.Existing == nil {
		t.Fatalf("existing replay binding was not preserved: %#v", result)
	}
	if result.Existing.AttemptID != receipt.AttemptID {
		t.Fatalf("conflicting candidate replaced durable evidence: %#v", *result.Existing)
	}
}

func bootSessionConsumptionStoreFixture(t *testing.T, attemptID string) BootSessionConsumptionReceipt {
	t.Helper()
	sessionRequest, servingReceipt, plan, payload := bootSessionFixture(t)
	const issuedAt = int64(1_800_000_000)
	admission, err := BuildBootSessionAdmission(sessionRequest, servingReceipt, plan, payload, nil, nil, issuedAt)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := BuildBootSessionConsumptionReceipt(BootSessionConsumptionRequest{
		AttemptID:      attemptID,
		ConsumerID:     "edge-store-0001",
		ConsumedAtUnix: issuedAt + 1,
	}, admission)
	if err != nil {
		t.Fatal(err)
	}
	return receipt
}
