package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestEnrollmentHandlerNormalizesCapabilities(t *testing.T) {
	body := `{"node_id":" node-01 ","hostname":" host-01 ","capabilities":["Inventory","inventory"," PXE "]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agent/enrollment/normalize", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	EnrollmentHandler().ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}
	for _, want := range []string{`"node_id":"node-01"`, `"hostname":"host-01"`, `"capabilities":["inventory","pxe"]`} {
		if !strings.Contains(res.Body.String(), want) {
			t.Fatalf("body=%s missing %s", res.Body.String(), want)
		}
	}
}

func TestEnrollmentHandlerRejectsMissingNode(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agent/enrollment/normalize", strings.NewReader(`{"hostname":"host-01"}`))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	EnrollmentHandler().ServeHTTP(res, req)
	if res.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}
}

func TestEnrollmentHandlerRejectsUnknownField(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agent/enrollment/normalize", strings.NewReader(`{"node_id":"node-01","hostname":"host-01","secret":"x"}`))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	EnrollmentHandler().ServeHTTP(res, req)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}
}
