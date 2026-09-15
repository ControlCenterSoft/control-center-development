package edgegateway

import (
	"bytes"
	"testing"
)

func TestDecodeRequestRejectsDuplicateAuthorizationJSONFields(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{
			name: "top level operation",
			body: `{"schema_version":"network.edge-gateway.authorization-request/v1","mode":"plan","operation":"routing","operation":"nat"}`,
		},
		{
			name: "nested target interface",
			body: `{"schema_version":"network.edge-gateway.authorization-request/v1","mode":"plan","operation":"routing","target":{"node_id":"node-edge-a","scope_id":"scope-site-a","site_id":"site-a","wan_interface_id":"nic-wan-a","wan_interface_id":"nic-wan-b","lan_interface_id":"nic-lan-a"}}`,
		},
		{
			name: "nested approval decision",
			body: `{"schema_version":"network.edge-gateway.authorization-request/v1","mode":"plan","operation":"routing","approval":{"id":"approval-edge-a","policy_id":"edge-policy-v1","decision":"approved","decision":"denied"}}`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := DecodeRequest(bytes.NewBufferString(test.body))
			if code, ok := CodeOf(err); !ok || code != DenyRequestInvalid {
				t.Fatalf("DecodeRequest() error=%v code=(%q,%t), want %q", err, code, ok, DenyRequestInvalid)
			}
		})
	}
}
