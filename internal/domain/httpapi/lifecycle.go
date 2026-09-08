package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"

	"control-center/internal/domain"
)

// LifecyclePlanHandler exposes planning only. Execution and provider
// credentials are intentionally outside this boundary.
func LifecyclePlanHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if !strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "application/json") {
			http.Error(w, "application/json required", http.StatusUnsupportedMediaType)
			return
		}

		r.Body = http.MaxBytesReader(w, r.Body, maxProviderRequestBytes)
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		var request domain.LifecyclePlanRequest
		if err := decoder.Decode(&request); err != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		if err := ensureEOF(decoder); err != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}

		plan, err := domain.BuildLifecyclePlan(request)
		if err != nil {
			http.Error(w, "invalid lifecycle plan request", http.StatusUnprocessableEntity)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(plan)
	})
}
