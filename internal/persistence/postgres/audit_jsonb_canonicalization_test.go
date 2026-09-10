package postgres

import (
	"encoding/json"
	"testing"
)

func TestDecodeAuditDetailsPreservesJSONNumbers(t *testing.T) {
	details, err := decodeAuditDetails(`{"large_integer":9007199254740993,"fraction":1.2300,"exponent":1.25e3}`)
	if err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]string{
		"large_integer": "9007199254740993",
		"fraction":      "1.2300",
		"exponent":      "1.25e3",
	} {
		number, ok := details[key].(json.Number)
		if !ok {
			t.Fatalf("%s decoded as %T, want json.Number", key, details[key])
		}
		if number.String() != want {
			t.Fatalf("%s=%q, want %q", key, number.String(), want)
		}
	}
}

func TestDecodeAuditDetailsPreservesNilAndEmptyObject(t *testing.T) {
	nilDetails, err := decodeAuditDetails(`null`)
	if err != nil {
		t.Fatal(err)
	}
	if nilDetails != nil {
		t.Fatalf("null decoded as %#v, want nil map", nilDetails)
	}

	emptyDetails, err := decodeAuditDetails(`{}`)
	if err != nil {
		t.Fatal(err)
	}
	if emptyDetails == nil || len(emptyDetails) != 0 {
		t.Fatalf("{} decoded as %#v, want non-nil empty map", emptyDetails)
	}
}
