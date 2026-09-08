package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"control-center/internal/market"
)

func TestMarketManifestList(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, manifestsPath, nil)
	rec := httptest.NewRecorder()
	New().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var response struct {
		Items []market.BuiltinManifest `json:"items"`
		Count int                      `json:"count"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Count != len(response.Items) || response.Count < 7 {
		t.Fatalf("response=%#v", response)
	}
}

func TestMarketManifestGet(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, manifestsPath+"/directory-services", nil)
	rec := httptest.NewRecorder()
	New().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var manifest market.BuiltinManifest
	if err := json.Unmarshal(rec.Body.Bytes(), &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.ID != "directory-services" || len(manifest.Providers) != 2 {
		t.Fatalf("manifest=%#v", manifest)
	}
}

func TestMarketManifestNotFound(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, manifestsPath+"/missing", nil)
	rec := httptest.NewRecorder()
	New().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status=%d", rec.Code)
	}
}
