package postgres

import (
	"strings"
	"testing"
)

func TestValidateStoredAuditDetailsSize(t *testing.T) {
	for _, size := range []int{0, 1, maxAuditJSONBCanonicalBytes} {
		if err := validateStoredAuditDetailsSize(size); err != nil {
			t.Fatalf("size %d rejected: %v", size, err)
		}
	}
	if err := validateStoredAuditDetailsSize(maxAuditJSONBCanonicalBytes + 1); err == nil {
		t.Fatal("oversized stored details accepted")
	}
}

func TestDecodeAuditDetailsRejectsOversizedPayloadBeforeJSONDecode(t *testing.T) {
	payload := strings.Repeat("x", maxAuditJSONBCanonicalBytes+1)
	_, err := decodeAuditDetails(payload)
	if err == nil {
		t.Fatal("oversized payload accepted")
	}
	if !strings.Contains(err.Error(), "stored audit details exceed") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDecodeAuditDetailsAcceptsBoundaryPayload(t *testing.T) {
	payload := `{"value":"` + strings.Repeat("x", maxAuditJSONBCanonicalBytes-len(`{"value":""}`)) + `"}`
	if len(payload) != maxAuditJSONBCanonicalBytes {
		t.Fatalf("fixture size=%d, want %d", len(payload), maxAuditJSONBCanonicalBytes)
	}
	details, err := decodeAuditDetails(payload)
	if err != nil {
		t.Fatalf("boundary payload rejected: %v", err)
	}
	if got := details["value"].(string); len(got) == 0 {
		t.Fatal("boundary payload decoded empty value")
	}
}
