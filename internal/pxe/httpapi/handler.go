package httpapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"control-center/internal/pxe"
)

type errorBody struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func New() http.Handler {
	return http.HandlerFunc(planPXE)
}

func planPXE(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method is not allowed")
		return
	}
	var profile pxe.Profile
	if err := decodeJSON(w, r, &profile); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid PXE profile")
		return
	}
	plan, err := pxe.BuildPlan(profile)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_PXE_PROFILE", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, plan)
}

func decodeJSON(w http.ResponseWriter, r *http.Request, target any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	payload, err := io.ReadAll(r.Body)
	if err != nil {
		return err
	}
	if err := rejectDuplicateProfileFields(payload); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return err
	}
	return nil
}

func rejectDuplicateProfileFields(payload []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delim, ok := token.(json.Delim)
	if !ok || delim != '{' {
		return fmt.Errorf("PXE profile must be a JSON object")
	}

	seen := make(map[string]struct{})
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		field, ok := token.(string)
		if !ok {
			return fmt.Errorf("PXE profile field name must be a string")
		}
		if _, exists := seen[field]; exists {
			return fmt.Errorf("duplicate PXE profile field %q", field)
		}
		seen[field] = struct{}{}

		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return err
		}
	}
	if _, err := decoder.Token(); err != nil {
		return err
	}
	return nil
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	body := errorBody{}
	body.Error.Code = code
	body.Error.Message = message
	writeJSON(w, status, body)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
