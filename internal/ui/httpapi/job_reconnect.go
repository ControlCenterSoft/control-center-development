package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"control-center/internal/orchestration/job"
	"control-center/internal/orchestration/operationsview"
	productui "control-center/internal/ui"
)

// JobReconnectHandler exposes a bounded, read-only resume endpoint for one
// durable Job. Route-level authentication/RBAC must still be enforced by the
// caller; this handler itself never retries, cancels, claims or executes work.
func JobReconnectHandler(provider productui.JobReconnectProvider) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			writeJobReconnectJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
			return
		}

		jobID := r.URL.Query().Get("job_id")
		if strings.TrimSpace(jobID) == "" || jobID != strings.TrimSpace(jobID) || len(jobID) > 255 {
			writeJobReconnectJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_job_id"})
			return
		}
		afterVersion, err := parseReconnectAfterVersion(r.URL.Query().Get("after_version"))
		if err != nil {
			writeJobReconnectJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_after_version"})
			return
		}
		if provider == nil {
			writeJobReconnectJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "job_reconnect_unavailable"})
			return
		}

		snapshot, err := provider.JobReconnect(r.Context(), jobID, afterVersion)
		if err != nil {
			if errors.Is(err, job.ErrNotFound) {
				writeJobReconnectJSON(w, http.StatusNotFound, map[string]string{"error": "job_not_found"})
				return
			}
			writeJobReconnectJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "job_reconnect_unavailable"})
			return
		}
		if snapshot.JobID != jobID || operationsview.ValidateJobReconnectSnapshot(snapshot) != nil {
			writeJobReconnectJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "job_reconnect_unavailable"})
			return
		}

		switch snapshot.State {
		case operationsview.JobReconnectCurrent:
			writeJobReconnectJSON(w, http.StatusOK, snapshot)
		case operationsview.JobReconnectReloadRequired:
			writeJobReconnectJSON(w, http.StatusConflict, snapshot)
		case operationsview.JobReconnectUnavailable:
			writeJobReconnectJSON(w, http.StatusServiceUnavailable, snapshot)
		default:
			writeJobReconnectJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "job_reconnect_unavailable"})
		}
	})
}

func parseReconnectAfterVersion(raw string) (uint64, error) {
	if raw == "" {
		return 0, nil
	}
	if strings.TrimSpace(raw) != raw || strings.HasPrefix(raw, "+") {
		return 0, errors.New("after_version must be canonical")
	}
	value, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		return 0, err
	}
	return value, nil
}

func writeJobReconnectJSON(w http.ResponseWriter, status int, value any) {
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
