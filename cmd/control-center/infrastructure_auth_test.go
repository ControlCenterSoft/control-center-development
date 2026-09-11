package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	productui "control-center/internal/ui"
)

func TestInfrastructureInventoryEndpointIsAbsentWithoutAuthoritativeProvider(t *testing.T) {
	fixture := newResourceAuthFixture(t)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/ui/infrastructure", nil)
	request.AddCookie(fixture.login(t, "viewer"))
	result := httptest.NewRecorder()
	fixture.handler.ServeHTTP(result, request)
	if result.Code != http.StatusNotFound {
		t.Fatalf("status=%d want=%d body=%s", result.Code, http.StatusNotFound, result.Body.String())
	}
}

func TestInfrastructureInventoryEndpointRequiresResourcesRead(t *testing.T) {
	generatedAt := time.Date(2026, 9, 12, 1, 0, 0, 0, time.UTC)
	provider := productui.InfrastructureInventoryProviderFunc(func(context.Context) (productui.InfrastructureInventory, error) {
		return productui.InfrastructureInventory{
			ContractVersion: productui.InfrastructureInventoryContractVersion,
			State:           productui.InventoryViewCurrent,
			GeneratedAt:     &generatedAt,
			SiteCount:       1,
			NodeCount:       0,
			Sites: []productui.SiteInventory{{
				ID: "site-a", Name: "Alpha", ScopeID: "scope-a", NodeCount: 0, Nodes: []productui.NodeInventory{},
			}},
		}, nil
	})
	fixture := newResourceAuthFixtureWithProductOptions(t, withInfrastructureInventoryProvider(provider))

	tests := []struct {
		name       string
		username   string
		wantStatus int
	}{
		{name: "anonymous", wantStatus: http.StatusUnauthorized},
		{name: "unbound", username: "unbound", wantStatus: http.StatusForbidden},
		{name: "viewer", username: "viewer", wantStatus: http.StatusOK},
		{name: "auditor", username: "auditor", wantStatus: http.StatusOK},
		{name: "operator", username: "operator", wantStatus: http.StatusOK},
		{name: "administrator", username: "admin", wantStatus: http.StatusOK},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/api/v1/ui/infrastructure", nil)
			if test.username != "" {
				request.AddCookie(fixture.login(t, test.username))
			}
			result := httptest.NewRecorder()
			fixture.handler.ServeHTTP(result, request)
			if result.Code != test.wantStatus {
				t.Fatalf("status=%d want=%d body=%s", result.Code, test.wantStatus, result.Body.String())
			}
		})
	}
}
