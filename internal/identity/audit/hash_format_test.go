package audit

import (
	"strings"
	"testing"
	"time"
)

func TestPrepareRequiresCanonicalPreviousHash(t *testing.T) {
	valid := strings.Repeat("a", maxAuditHashBytes)
	prepared, err := Prepare(Event{ID: "event-2", Action: "audit.chain", Outcome: "success"}, valid)
	if err != nil {
		t.Fatalf("Prepare rejected canonical predecessor hash: %v", err)
	}
	if prepared.PreviousHash != valid {
		t.Fatalf("PreviousHash = %q, want canonical predecessor", prepared.PreviousHash)
	}
	if len(prepared.Hash) != maxAuditHashBytes {
		t.Fatalf("Hash length = %d, want %d", len(prepared.Hash), maxAuditHashBytes)
	}
	for i := 0; i < len(prepared.Hash); i++ {
		ch := prepared.Hash[i]
		if (ch < '0' || ch > '9') && (ch < 'a' || ch > 'f') {
			t.Fatalf("Hash contains non-lowercase-hex byte %q", ch)
		}
	}

	for _, tt := range []struct {
		name string
		hash string
	}{
		{name: "short", hash: strings.Repeat("a", maxAuditHashBytes-1)},
		{name: "long", hash: strings.Repeat("a", maxAuditHashBytes+1)},
		{name: "uppercase", hash: strings.Repeat("A", maxAuditHashBytes)},
		{name: "non_hex", hash: strings.Repeat("g", maxAuditHashBytes)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := Prepare(Event{ID: "event-2", Action: "audit.chain", Outcome: "success"}, tt.hash); err == nil {
				t.Fatalf("Prepare accepted malformed predecessor hash %q", tt.hash)
			}
		})
	}

	if _, err := Prepare(Event{ID: "genesis", Action: "audit.chain", Outcome: "success"}, ""); err != nil {
		t.Fatalf("Prepare rejected empty genesis predecessor: %v", err)
	}
}

func TestVerifyRejectsMalformedStoredHash(t *testing.T) {
	prepared, err := Prepare(Event{ID: "event-1", OccurredAt: time.Now(), Action: "audit.chain", Outcome: "success"}, "")
	if err != nil {
		t.Fatal(err)
	}

	for _, tt := range []struct {
		name string
		hash string
	}{
		{name: "empty", hash: ""},
		{name: "short", hash: prepared.Hash[:maxAuditHashBytes-1]},
		{name: "uppercase", hash: strings.ToUpper(prepared.Hash)},
		{name: "non_hex", hash: strings.Repeat("g", maxAuditHashBytes)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tampered := prepared
			tampered.Hash = tt.hash
			if err := Verify(tampered, ""); err == nil || !strings.Contains(err.Error(), "audit hash") {
				t.Fatalf("Verify accepted malformed stored hash %q: %v", tt.hash, err)
			}
		})
	}
}

func TestVerifyRejectsMalformedPredecessorEvenWithMatchingEventHash(t *testing.T) {
	first, err := Prepare(Event{ID: "event-1", OccurredAt: time.Now(), Action: "audit.chain", Outcome: "success"}, "")
	if err != nil {
		t.Fatal(err)
	}
	second, err := Prepare(Event{ID: "event-2", OccurredAt: time.Now(), Action: "audit.chain", Outcome: "success"}, first.Hash)
	if err != nil {
		t.Fatal(err)
	}

	for _, malformed := range []string{strings.ToUpper(first.Hash), strings.Repeat("g", maxAuditHashBytes)} {
		tampered := second
		tampered.PreviousHash = malformed
		tampered.Hash, err = hashEvent(tampered)
		if err != nil {
			t.Fatalf("build tampered event fixture: %v", err)
		}
		if err := Verify(tampered, malformed); err == nil || !strings.Contains(err.Error(), "previous_hash") {
			t.Fatalf("Verify accepted malformed predecessor with internally matching event hash: %v", err)
		}
	}
}

func TestVerifyRejectsMalformedExpectedPreviousHash(t *testing.T) {
	first, err := Prepare(Event{ID: "event-1", OccurredAt: time.Now(), Action: "audit.chain", Outcome: "success"}, "")
	if err != nil {
		t.Fatal(err)
	}
	second, err := Prepare(Event{ID: "event-2", OccurredAt: time.Now(), Action: "audit.chain", Outcome: "success"}, first.Hash)
	if err != nil {
		t.Fatal(err)
	}

	for _, malformed := range []string{first.Hash[:maxAuditHashBytes-1], strings.ToUpper(first.Hash), strings.Repeat("z", maxAuditHashBytes)} {
		if err := Verify(second, malformed); err == nil || !strings.Contains(err.Error(), "expected_previous_hash") {
			t.Fatalf("Verify accepted malformed expected predecessor %q: %v", malformed, err)
		}
	}
}
