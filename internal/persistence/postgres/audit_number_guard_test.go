package postgres

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestValidateAuditJSONBCanonicalSizeAllowsBoundedNumbers(t *testing.T) {
	details := map[string]any{
		"integer":  json.Number("9007199254740993"),
		"decimal":  json.Number("1.2300"),
		"exponent": json.Number("1.25e3"),
		"small":    json.Number("1e-6"),
	}
	payload, err := json.Marshal(details)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateAuditJSONBCanonicalSize(details, len(payload)); err != nil {
		t.Fatalf("bounded details rejected: %v", err)
	}
}

func TestValidateAuditJSONBCanonicalSizeRejectsPositiveExponentAmplification(t *testing.T) {
	details := map[string]any{"value": json.Number("1e20000")}
	payload, err := json.Marshal(details)
	if err != nil {
		t.Fatal(err)
	}
	if len(payload) >= maxAuditJSONBCanonicalBytes {
		t.Fatalf("test payload must be compact, got %d bytes", len(payload))
	}
	if err := validateAuditJSONBCanonicalSize(details, len(payload)); err == nil || !strings.Contains(err.Error(), "jsonb representation exceeds") {
		t.Fatalf("expected amplification rejection, got %v", err)
	}
}

func TestValidateAuditJSONBCanonicalSizeRejectsNegativeExponentAmplification(t *testing.T) {
	details := map[string]any{"value": json.Number("1e-20000")}
	payload, err := json.Marshal(details)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateAuditJSONBCanonicalSize(details, len(payload)); err == nil || !strings.Contains(err.Error(), "jsonb representation exceeds") {
		t.Fatalf("expected amplification rejection, got %v", err)
	}
}

func TestValidateAuditJSONBCanonicalSizeAccountsForAggregateNumberGrowth(t *testing.T) {
	values := make([]any, 32)
	for i := range values {
		values[i] = json.Number("1e600")
	}
	details := map[string]any{"values": values}
	payload, err := json.Marshal(details)
	if err != nil {
		t.Fatal(err)
	}
	if len(payload) >= maxAuditJSONBCanonicalBytes {
		t.Fatalf("test payload must be compact, got %d bytes", len(payload))
	}
	if err := validateAuditJSONBCanonicalSize(details, len(payload)); err == nil || !strings.Contains(err.Error(), "jsonb representation exceeds") {
		t.Fatalf("expected aggregate amplification rejection, got %v", err)
	}
}

func TestEstimateExpandedJSONNumberBytes(t *testing.T) {
	tests := map[string]int{
		"1":        1,
		"-1":       2,
		"1.25e3":   4,
		"1e3":      4,
		"1e-3":     5,
		"-1e-3":    6,
		"12.34e1":  5,
		"12.34e-1": 5,
	}
	for input, want := range tests {
		got, err := estimateExpandedJSONNumberBytes(input)
		if err != nil {
			t.Fatalf("%s: %v", input, err)
		}
		if got != want {
			t.Fatalf("%s: got %d want %d", input, got, want)
		}
	}
}
