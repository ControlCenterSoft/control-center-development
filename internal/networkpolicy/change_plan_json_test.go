package networkpolicy

import (
	"errors"
	"strings"
	"testing"
)

func TestDecodeChangePlanRequestRejectsDuplicateJSONFields(t *testing.T) {
	tests := []struct {
		name     string
		document string
		field    string
	}{
		{
			name:     "top level identity",
			document: `{"node_id":"node-a","node_id":"node-b"}`,
			field:    "node_id",
		},
		{
			name:     "interface object inside array",
			document: `{"interfaces":[{"interface_id":"eth0","interface_id":"eth1"}]}`,
			field:    "interface_id",
		},
		{
			name:     "nested timeout policy",
			document: `{"timeouts":{"probe":1000000000,"probe":2000000000}}`,
			field:    "probe",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := DecodeChangePlanRequest([]byte(test.document))
			if !errors.Is(err, ErrInvalidChangePlan) {
				t.Fatalf("DecodeChangePlanRequest() error = %v, want ErrInvalidChangePlan", err)
			}
			if !strings.Contains(err.Error(), `duplicate object field "`+test.field+`"`) {
				t.Fatalf("DecodeChangePlanRequest() error = %q, want duplicate field %q", err, test.field)
			}
		})
	}
}

func TestDecodeChangePlanRequestCustomUnmarshalPreservesStrictBoundary(t *testing.T) {
	t.Run("unknown field rejected", func(t *testing.T) {
		_, err := DecodeChangePlanRequest([]byte(`{"node_id":"node-a","unexpected":true}`))
		if !errors.Is(err, ErrInvalidChangePlan) {
			t.Fatalf("DecodeChangePlanRequest() error = %v, want ErrInvalidChangePlan", err)
		}
		if !strings.Contains(err.Error(), "unknown field") {
			t.Fatalf("DecodeChangePlanRequest() error = %q, want unknown-field evidence", err)
		}
	})

	t.Run("valid partial document still decodes", func(t *testing.T) {
		request, err := DecodeChangePlanRequest([]byte(`{"node_id":"node-a","revision_id":"rev-a"}`))
		if err != nil {
			t.Fatalf("DecodeChangePlanRequest() error = %v", err)
		}
		if request.NodeID != "node-a" || request.RevisionID != "rev-a" {
			t.Fatalf("DecodeChangePlanRequest() = %#v, want node-a/rev-a", request)
		}
	})
}
