package operationsview

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestJobRetryAdmissionSchemaMatchesImplementationBounds(t *testing.T) {
	path := filepath.Join("..", "..", "..", "api", "operations-job-retry-admission-v1.schema.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read retry-admission schema: %v", err)
	}

	var schema struct {
		Properties struct {
			ContractVersion struct {
				Const string `json:"const"`
			} `json:"contract_version"`
			JobID struct {
				MaxLength int `json:"maxLength"`
			} `json:"job_id"`
			ChangeID struct {
				MaxLength int `json:"maxLength"`
			} `json:"change_id"`
			ActionName struct {
				MaxLength int `json:"maxLength"`
			} `json:"action_name"`
			RevisionID struct {
				MaxLength int `json:"maxLength"`
			} `json:"revision_id"`
			PolicyID struct {
				MaxLength int `json:"maxLength"`
			} `json:"policy_id"`
			Blockers struct {
				MaxItems int `json:"maxItems"`
			} `json:"blockers"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatalf("decode retry-admission schema: %v", err)
	}

	if schema.Properties.ContractVersion.Const != JobRetryAdmissionContractVersion {
		t.Fatalf("contract version drift: schema=%q code=%q", schema.Properties.ContractVersion.Const, JobRetryAdmissionContractVersion)
	}
	for name, bound := range map[string]int{
		"job_id":      schema.Properties.JobID.MaxLength,
		"change_id":   schema.Properties.ChangeID.MaxLength,
		"action_name": schema.Properties.ActionName.MaxLength,
		"revision_id": schema.Properties.RevisionID.MaxLength,
		"policy_id":   schema.Properties.PolicyID.MaxLength,
	} {
		if bound != MaxJobRetryAdmissionIdentifierLength {
			t.Fatalf("%s maxLength drift: schema=%d code=%d", name, bound, MaxJobRetryAdmissionIdentifierLength)
		}
	}
	if schema.Properties.Blockers.MaxItems != MaxJobRetryAdmissionBlockers {
		t.Fatalf("blocker bound drift: schema=%d code=%d", schema.Properties.Blockers.MaxItems, MaxJobRetryAdmissionBlockers)
	}
}
