package recovery

import (
	"encoding/json"
	"os"
	"testing"
)

func TestRestoreDrillEvidenceBindingSchemaMatchesSerializedContract(t *testing.T) {
	restore, assessment, _ := currentRestoreDrillEvidenceBindingFixture(t)
	binding, err := BuildRestoreDrillEvidenceBinding(restore, assessment)
	if err != nil {
		t.Fatalf("BuildRestoreDrillEvidenceBinding() error = %v", err)
	}

	encodedBinding, err := json.Marshal(binding)
	if err != nil {
		t.Fatalf("json.Marshal(binding) error = %v", err)
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(encodedBinding, &payload); err != nil {
		t.Fatalf("json.Unmarshal(binding) error = %v", err)
	}

	rawSchema, err := os.ReadFile("../../api/recovery-restore-drill-evidence-binding-v1.schema.json")
	if err != nil {
		t.Fatalf("read restore-drill evidence-binding schema: %v", err)
	}
	var schema struct {
		Required   []string                   `json:"required"`
		Properties map[string]json.RawMessage `json:"properties"`
	}
	if err := json.Unmarshal(rawSchema, &schema); err != nil {
		t.Fatalf("decode restore-drill evidence-binding schema: %v", err)
	}

	for field := range payload {
		if _, ok := schema.Properties[field]; !ok {
			t.Fatalf("serialized binding field %q is rejected by schema additionalProperties=false", field)
		}
	}

	required := make(map[string]struct{}, len(schema.Required))
	for _, field := range schema.Required {
		required[field] = struct{}{}
		if _, ok := payload[field]; !ok {
			t.Fatalf("schema requires field %q that serialized binding does not emit", field)
		}
	}
	if _, ok := required["owner_scope"]; !ok {
		t.Fatal("owner_scope is safety-relevant binding identity but is not required by the schema")
	}
}
