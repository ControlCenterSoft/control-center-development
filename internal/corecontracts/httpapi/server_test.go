package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"control-center/internal/corecontracts"
)

func TestDistributedCoreReadAPI(t *testing.T) {
	repository := testRepository(t)
	guardCalls := 0
	guard := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			guardCalls++
			next.ServeHTTP(w, r)
		})
	}
	handler := New(testLogger(), repository, guard).Handler()

	result := request(handler, http.MethodGet, "/api/v1/core/objects?object_type=scope")
	if result.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", result.Code, result.Body.String())
	}
	var list struct {
		Items []corecontracts.StoredObject `json:"items"`
		Count int                          `json:"count"`
	}
	if err := json.Unmarshal(result.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if list.Count != 1 || list.Items[0].ObjectID != "global" {
		t.Fatalf("list = %#v", list)
	}

	result = request(handler, http.MethodGet, "/api/v1/core/objects/desired-a")
	if result.Code != http.StatusOK || !strings.Contains(result.Body.String(), `"resource_version"`) {
		t.Fatalf("get status=%d body=%s", result.Code, result.Body.String())
	}
	result = request(handler, http.MethodGet, "/api/v1/core/topology")
	if result.Code != http.StatusOK || !strings.Contains(result.Body.String(), `"scopes":[`) || !strings.Contains(result.Body.String(), `"management_zones":[]`) {
		t.Fatalf("topology status=%d body=%s", result.Code, result.Body.String())
	}
	if guardCalls != 3 {
		t.Fatalf("guard calls=%d, want 3", guardCalls)
	}
}

func TestDistributedCoreReadAPIRejectsInvalidRequests(t *testing.T) {
	handler := New(testLogger(), testRepository(t), func(next http.Handler) http.Handler { return next }).Handler()
	tests := []struct {
		method string
		path   string
		status int
	}{
		{http.MethodPost, "/api/v1/core/objects", http.StatusMethodNotAllowed},
		{http.MethodGet, "/api/v1/core/objects?unexpected=true", http.StatusBadRequest},
		{http.MethodGet, "/api/v1/core/objects?object_type=secret", http.StatusBadRequest},
		{http.MethodGet, "/api/v1/core/objects?scope_id=bad%20scope", http.StatusBadRequest},
		{http.MethodGet, "/api/v1/core/objects/bad%20id", http.StatusBadRequest},
		{http.MethodGet, "/api/v1/core/objects/missing", http.StatusNotFound},
		{http.MethodGet, "/api/v1/core/topology?unexpected=true", http.StatusBadRequest},
	}
	for _, test := range tests {
		result := request(handler, test.method, test.path)
		if result.Code != test.status {
			t.Errorf("%s %s status=%d want=%d body=%s", test.method, test.path, result.Code, test.status, result.Body.String())
		}
	}
}

func TestDistributedCoreReadAPIFailsClosed(t *testing.T) {
	anonymous := New(testLogger(), testRepository(t), nil).Handler()
	if result := request(anonymous, http.MethodGet, "/api/v1/core/objects"); result.Code != http.StatusUnauthorized {
		t.Fatalf("missing guard status=%d body=%s", result.Code, result.Body.String())
	}
	unready := New(testLogger(), nil, func(next http.Handler) http.Handler { return next }).Handler()
	if result := request(unready, http.MethodGet, "/api/v1/core/objects"); result.Code != http.StatusServiceUnavailable {
		t.Fatalf("missing repository status=%d body=%s", result.Code, result.Body.String())
	}
}

func testRepository(t *testing.T) *corecontracts.MemoryObjectRepository {
	t.Helper()
	now := time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)
	repository, err := corecontracts.NewMemoryObjectRepository([]corecontracts.StoredObject{corecontracts.LegacyGlobalScopeObject(now)})
	if err != nil {
		t.Fatal(err)
	}
	_, err = repository.Apply(context.Background(), corecontracts.MutationRequest{
		Operation: corecontracts.MutationCreate, ObjectType: corecontracts.ObjectDesiredState,
		ObjectID: "desired-a", ScopeID: "global", OwnerScope: "global",
		Document: json.RawMessage(`{"kind":"service.config","target_object_id":"service-a","spec":{}}`),
	}, "read-api-fixture")
	if err != nil {
		t.Fatal(err)
	}
	return repository
}

func request(handler http.Handler, method, path string) *httptest.ResponseRecorder {
	result := httptest.NewRecorder()
	handler.ServeHTTP(result, httptest.NewRequest(method, path, nil))
	return result
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
