package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"control-center/internal/automation"
)

func TestAutomationPlanHandler(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/automation/plan", strings.NewReader(`{"target":{"id":"node-1","platform":"linux"},"operation":"package.ensure","arguments":{"name":"example","state":"present"}}`))
	rec := httptest.NewRecorder()
	New().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var plan automation.Plan
	if err := json.Unmarshal(rec.Body.Bytes(), &plan); err != nil {
		t.Fatal(err)
	}
	if plan.Adapter != "ansible.linux" || plan.Action != automation.EnsurePackage {
		t.Fatalf("plan=%#v", plan)
	}
}

func TestAutomationPlanHandlerRejectsArbitraryInput(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/automation/plan", strings.NewReader(`{"target":{"id":"node-1","platform":"linux"},"operation":"service.ensure","arguments":{"name":"svc","command":"unexpected"}}`))
	rec := httptest.NewRecorder()
	New().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d", rec.Code)
	}
}

func TestAutomationPlanHandlerRejectsDuplicateJSONFields(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{
			name: "top-level operation",
			body: `{"target":{"id":"node-1","platform":"linux"},"operation":"service.inspect","operation":"package.ensure","arguments":{"name":"example"}}`,
		},
		{
			name: "nested target platform",
			body: `{"target":{"id":"node-1","platform":"linux","platform":"windows"},"operation":"service.inspect","arguments":{"name":"svc"}}`,
		},
		{
			name: "arguments state",
			body: `{"target":{"id":"node-1","platform":"linux"},"operation":"package.ensure","arguments":{"name":"example","state":"present","state":"absent"}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/automation/plan", strings.NewReader(tt.body))
			rec := httptest.NewRecorder()
			New().ServeHTTP(rec, req)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
			}
			var body errorBody
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body.Error.Code != "INVALID_REQUEST" {
				t.Fatalf("error=%#v", body.Error)
			}
		})
	}
}

func TestAutomationPlanHandlerRejectsTrailingJSONValue(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/automation/plan", strings.NewReader(`{"target":{"id":"node-1","platform":"linux"},"operation":"service.inspect","arguments":{"name":"svc"}} {}`))
	rec := httptest.NewRecorder()
	New().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var body errorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Error.Code != "INVALID_REQUEST" {
		t.Fatalf("error=%#v", body.Error)
	}
}

func TestAutomationPlanHandlerRejectsGET(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/automation/plan", nil)
	rec := httptest.NewRecorder()
	New().ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed || rec.Header().Get("Allow") != http.MethodPost {
		t.Fatalf("status=%d allow=%q", rec.Code, rec.Header().Get("Allow"))
	}
}
