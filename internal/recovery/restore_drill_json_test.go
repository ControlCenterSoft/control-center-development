package recovery

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestRestoreDrillTransitionRequestJSONRejectsDuplicateFields(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "duplicate transition state",
			raw:  "{\"to\":\"RUNNING\",\"to\":\"FAILED\"}",
			want: "duplicate field \"to\"",
		},
		{
			name: "duplicate nested precondition resource version",
			raw:  "{\"to\":\"RUNNING\",\"precondition\":{\"object_id\":\"restore-001\",\"resource_version\":\"rv:1\",\"resource_version\":\"rv:2\"}}",
			want: "duplicate field \"resource_version\"",
		},
		{
			name: "duplicate evidence identity",
			raw:  "{\"to\":\"SUCCEEDED\",\"verification_evidence\":[{\"id\":\"evidence-1\",\"id\":\"evidence-2\"}]}",
			want: "duplicate field \"id\"",
		},
		{
			name: "multiple top-level documents",
			raw:  "{\"to\":\"RUNNING\"} {\"to\":\"FAILED\"}",
			want: "multiple JSON values are not allowed",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var request RestoreDrillTransitionRequest
			err := json.Unmarshal([]byte(test.raw), &request)
			if !errors.Is(err, ErrInvalidRestoreDrillTransition) {
				t.Fatalf("error = %v, want ErrInvalidRestoreDrillTransition", err)
			}
			if !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want detail %q", err, test.want)
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
