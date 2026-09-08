package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDomainInventoryAgentAPIRBAC(t *testing.T) {
	fixture := newResourceAuthFixture(t)
	tests := []struct {
		name string
		path string
		body string
	}{
		{name: "domain", path: "/api/v1/domain/provider/resolve", body: `{"preferred":"auto","requirements":{"WindowsDomainJoin":true}}`},
		{name: "domain lifecycle", path: "/api/v1/domain/lifecycle/plan", body: `{"provider":"samba-ad-dc","operation":"health-preflight","domain_name":"example.test","target":{"node_id":"dc-01","platform":"linux"}}`},
		{name: "inventory", path: "/api/v1/inventory/normalize", body: `{"hostname":"node-1","platform":"linux","machine_id":"machine-1"}`},
		{name: "agent", path: "/api/v1/agent/enrollment/normalize", body: `{"node_id":"node-1","hostname":"node-1","capabilities":["inventory"]}`},
	}

	for _, test := range tests {
		t.Run(test.name+" anonymous", func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, test.path, strings.NewReader(test.body))
			request.Header.Set("Content-Type", "application/json")
			result := httptest.NewRecorder()
			fixture.handler.ServeHTTP(result, request)
			if result.Code != http.StatusUnauthorized {
				t.Fatalf("status=%d want=%d body=%s", result.Code, http.StatusUnauthorized, result.Body.String())
			}
		})

		t.Run(test.name+" viewer", func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, test.path, strings.NewReader(test.body))
			request.Header.Set("Content-Type", "application/json")
			request.AddCookie(fixture.login(t, "viewer"))
			result := httptest.NewRecorder()
			fixture.handler.ServeHTTP(result, request)
			if result.Code != http.StatusForbidden {
				t.Fatalf("status=%d want=%d body=%s", result.Code, http.StatusForbidden, result.Body.String())
			}
		})

		t.Run(test.name+" operator", func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, test.path, strings.NewReader(test.body))
			request.Header.Set("Content-Type", "application/json")
			request.AddCookie(fixture.login(t, "operator"))
			result := httptest.NewRecorder()
			fixture.handler.ServeHTTP(result, request)
			if result.Code != http.StatusOK {
				t.Fatalf("status=%d want=%d body=%s", result.Code, http.StatusOK, result.Body.String())
			}
		})
	}
}
