package recovery

import (
	"encoding/json"
	"os"
	"testing"
	"time"
)

func TestRestoreDrillEvidenceBindingBindsScopeID(t *testing.T) {
	restore, assessment, binding := currentRestoreDrillEvidenceBindingFixture(t)
	if binding.ScopeID != restore.ScopeID {
		t.Fatalf("scope_id = %q, want %q", binding.ScopeID, restore.ScopeID)
	}

	tampered := binding
	tampered.ScopeID = "site-b"
	if err := ValidateRestoreDrillEvidenceBindingCurrent(restore, assessment, tampered); err == nil {
		t.Fatal("persisted binding accepted after scope_id tamper")
	}
}

func TestRestoreDrillEvidenceBindingRejectsCrossScopeReplay(t *testing.T) {
	restore, assessment, binding := currentRestoreDrillEvidenceBindingFixture(t)

	otherScope := restore
	otherScope.ScopeID = "site-b"
	otherScope.Target.ScopeID = "site-b"
	otherAssessment, err := EvaluateRestoreDrillFreshness(
		otherScope,
		time.Duration(assessment.MaxAgeSeconds)*time.Second,
		assessment.CheckedAt,
	)
	if err != nil {
		t.Fatalf("EvaluateRestoreDrillFreshness(other scope) error = %v", err)
	}
	otherBinding, err := BuildRestoreDrillEvidenceBinding(otherScope, otherAssessment)
	if err != nil {
		t.Fatalf("BuildRestoreDrillEvidenceBinding(other scope) error = %v", err)
	}
	if otherBinding.ScopeID != otherScope.ScopeID {
		t.Fatalf("other scope_id = %q, want %q", otherBinding.ScopeID, otherScope.ScopeID)
	}
	if otherBinding.BindingID == binding.BindingID {
		t.Fatalf("cross-scope restore produced the same binding identity: %s", binding.BindingID)
	}
	if err := ValidateRestoreDrillEvidenceBindingCurrent(otherScope, otherAssessment, binding); err == nil {
		t.Fatal("binding accepted across restore scope boundary")
	}
}

func TestRestoreDrillEvidenceBindingSchemaRequiresScopeID(t *testing.T) {
	rawSchema, err := os.ReadFile("../../api/recovery-restore-drill-evidence-binding-v1.schema.json")
	if err != nil {
		t.Fatalf("read restore-drill evidence-binding schema: %v", err)
	}
	var schema struct {
		Required []string `json:"required"`
	}
	if err := json.Unmarshal(rawSchema, &schema); err != nil {
		t.Fatalf("decode restore-drill evidence-binding schema: %v", err)
	}
	for _, field := range schema.Required {
		if field == "scope_id" {
			return
		}
	}
	t.Fatal("scope_id is safety-relevant binding identity but is not required by the schema")
}
