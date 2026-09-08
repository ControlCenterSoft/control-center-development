package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"control-center/internal/agent"
)

const maxEnrollmentRequestBytes = 64 << 10

type enrollmentPayload struct {
	NodeID       string   `json:"node_id"`
	Hostname     string   `json:"hostname"`
	Capabilities []string `json:"capabilities,omitempty"`
}

func EnrollmentHandler() http.Handler {
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
		r.Body = http.MaxBytesReader(w, r.Body, maxEnrollmentRequestBytes)
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		var input enrollmentPayload
		if err := decoder.Decode(&input); err != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		if err := agentEOF(decoder); err != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		normalized, err := agent.NormalizeEnrollment(agent.EnrollmentRequest{
			NodeID: input.NodeID, Hostname: input.Hostname, Capabilities: input.Capabilities,
		})
		if err != nil {
			if errors.Is(err, agent.ErrInvalidEnrollment) {
				http.Error(w, "invalid enrollment", http.StatusUnprocessableEntity)
				return
			}
			http.Error(w, "unable to normalize enrollment", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(enrollmentPayload{
			NodeID: normalized.NodeID, Hostname: normalized.Hostname, Capabilities: normalized.Capabilities,
		})
	})
}

func agentEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); errors.Is(err, io.EOF) {
		return nil
	} else {
		return err
	}
}
