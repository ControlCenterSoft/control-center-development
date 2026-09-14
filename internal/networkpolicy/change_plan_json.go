package networkpolicy

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// UnmarshalJSON keeps the ChangePlanRequest boundary fail-closed for ambiguous
// JSON objects. Go's encoding/json otherwise accepts duplicate object fields
// and silently keeps the last value.
func (request *ChangePlanRequest) UnmarshalJSON(document []byte) error {
	if err := rejectDuplicateJSONFields(document); err != nil {
		return err
	}

	type changePlanRequestAlias ChangePlanRequest
	decoder := json.NewDecoder(bytes.NewReader(document))
	decoder.DisallowUnknownFields()
	var decoded changePlanRequestAlias
	if err := decoder.Decode(&decoded); err != nil {
		return err
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return err
	}
	*request = ChangePlanRequest(decoded)
	return nil
}

func rejectDuplicateJSONFields(document []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(document))
	if err := scanJSONValueForDuplicateFields(decoder); err != nil {
		return err
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return err
	}
	return nil
}

func scanJSONValueForDuplicateFields(decoder *json.Decoder) error {
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
		seen := make(map[string]struct{})
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return fmt.Errorf("object field name must be a string")
			}
			if _, duplicate := seen[key]; duplicate {
				return fmt.Errorf("duplicate object field %q", key)
			}
			seen[key] = struct{}{}
			if err := scanJSONValueForDuplicateFields(decoder); err != nil {
				return err
			}
		}
		end, err := decoder.Token()
		if err != nil {
			return err
		}
		if end != json.Delim('}') {
			return fmt.Errorf("unterminated JSON object")
		}
	case '[':
		for decoder.More() {
			if err := scanJSONValueForDuplicateFields(decoder); err != nil {
				return err
			}
		}
		end, err := decoder.Token()
		if err != nil {
			return err
		}
		if end != json.Delim(']') {
			return fmt.Errorf("unterminated JSON array")
		}
	default:
		return fmt.Errorf("unexpected JSON delimiter %q", delim)
	}
	return nil
}
