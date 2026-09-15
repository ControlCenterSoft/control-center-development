package recovery

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestRestoreDrillTransitionRequestJSONRejectsDuplicateFields(t *testing.T) {
	tests := []struct {
		name string
		raw  string
	}{
		{
			name: "duplicate transition state",
			raw:  `{"to":"RUNNING","to":"FAILED","precondition":{"object_id":"restore-001","resource_version":"rv:1"},"occurred_at":"2026-09-15T00:00:00Z"}`,
		},
		{
			name: "duplicate nested precondition resource version",
			raw:  `{"to":"RUNNING","precondition":{"object_id":"restore-001","resource_version":"rv:1","resource_version":"rv:2"},"occurred_at":"2026-09-15T00:00:00Z"}`,
		},
		{
			name: "duplicate evidence identity",
			raw:  `{"to":"SUCCEEDED","precondition":{"object_id":"restore-001","resource_version":"rv:1"},"occurred_at":"2026-09-15T00:00:00Z","verification_evidence":[{"id":"evidence-1","id":"evidence-2"}]}`,
		},
		{
			name: "multiple top-level documents",
			raw:  `{"to":"RUNNING"} {"to":"FAILED"}`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var request RestoreDrillTransitionRequest
			if err := json.Unmarshal([]byte(test.raw), &request); !errors.Is(err, ErrInvalidRestoreDrillTransition) {
				t.Fatalf("error = %v, want ErrInvalidRestoreDrillTransition", err)
			}
		})
	}
}

func TestRestoreDrillTransitionRequestJSONPreservesValidRequest(t *testing.T) {
	restore := plannedRestoreDrill()
	request := drillTransition(restore, RestoreRunning, restore.UpdatedAt.Add(time.Minute))
	request.ProviderOperationID = "provider-restore-drill-json-001"

	encoded, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	var decoded RestoreDrillTransitionRequest
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("valid request decode error = %v", err)
	}
	if !reflect.DeepEqual(decoded, request) {
		t.Fatalf("decoded request = %#v, want %#v", decoded, request)
	}
}
