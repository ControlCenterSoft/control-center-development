package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"control-center/internal/inventory"
)

const maxInventoryRequestBytes = 128 << 10

type devicePayload struct {
	Hostname  string             `json:"hostname"`
	Platform  inventory.Platform `json:"platform"`
	MachineID string             `json:"machine_id,omitempty"`
	Serial    string             `json:"serial,omitempty"`
	Addresses []string           `json:"addresses,omitempty"`
	Tags      []string           `json:"tags,omitempty"`
}

func NormalizeHandler() http.Handler {
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
		r.Body = http.MaxBytesReader(w, r.Body, maxInventoryRequestBytes)
		payload, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		if err := rejectDuplicateInventoryJSONKeys(payload); err != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}

		decoder := json.NewDecoder(bytes.NewReader(payload))
		decoder.DisallowUnknownFields()
		var input devicePayload
		if err := decoder.Decode(&input); err != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		if err := inventoryEOF(decoder); err != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		device, err := inventory.NormalizeDevice(inventory.Device{
			Hostname: input.Hostname, Platform: input.Platform, MachineID: input.MachineID,
			Serial: input.Serial, Addresses: input.Addresses, Tags: input.Tags,
		})
		if err != nil {
			if errors.Is(err, inventory.ErrInvalidDevice) {
				http.Error(w, "invalid device", http.StatusUnprocessableEntity)
				return
			}
			http.Error(w, "unable to normalize device", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(devicePayload{
			Hostname: device.Hostname, Platform: device.Platform, MachineID: device.MachineID,
			Serial: device.Serial, Addresses: device.Addresses, Tags: device.Tags,
		})
	})
}

func rejectDuplicateInventoryJSONKeys(payload []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	if err := scanInventoryJSONValue(decoder); err != nil {
		return err
	}
	return inventoryEOF(decoder)
}

func scanInventoryJSONValue(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}

	switch delim {
	case '{':
		seen := map[string]struct{}{}
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return errors.New("invalid JSON object key")
			}
			if _, exists := seen[key]; exists {
				return errors.New("duplicate JSON object key")
			}
			seen[key] = struct{}{}
			if err := scanInventoryJSONValue(decoder); err != nil {
				return err
			}
		}
		end, err := decoder.Token()
		if err != nil {
			return err
		}
		if end != json.Delim('}') {
			return errors.New("invalid JSON object terminator")
		}
	case '[':
		for decoder.More() {
			if err := scanInventoryJSONValue(decoder); err != nil {
				return err
			}
		}
		end, err := decoder.Token()
		if err != nil {
			return err
		}
		if end != json.Delim(']') {
			return errors.New("invalid JSON array terminator")
		}
	default:
		return errors.New("invalid JSON delimiter")
	}
	return nil
}

func inventoryEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); errors.Is(err, io.EOF) {
		return nil
	} else {
		return err
	}
}
