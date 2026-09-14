package edgegateway

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

// UnmarshalJSON keeps the Edge Gateway authorization boundary fail-closed for
// ambiguous JSON objects. Go's encoding/json otherwise accepts duplicate
// object fields and silently keeps the last value.
func (request *AuthorizationRequest) UnmarshalJSON(document []byte) error {
	if err := rejectDuplicateAuthorizationJSONFields(document); err != nil {
		return err
	}

	type authorizationRequestAlias AuthorizationRequest
	decoder := json.NewDecoder(bytes.NewReader(document))
	decoder.DisallowUnknownFields()
	var decoded authorizationRequestAlias
	if err := decoder.Decode(&decoded); err != nil {
		return err
	}
	if err := ensureAuthorizationJSONEOF(decoder); err != nil {
		return err
	}
	*request = AuthorizationRequest(decoded)
	return nil
}

func rejectDuplicateAuthorizationJSONFields(document []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(document))
	if err := scanAuthorizationJSONValue(decoder); err != nil {
		return err
	}
	return ensureAuthorizationJSONEOF(decoder)
}

func scanAuthorizationJSONValue(decoder *json.Decoder) error {
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
			if err := scanAuthorizationJSONValue(decoder); err != nil {
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
			if err := scanAuthorizationJSONValue(decoder); err != nil {
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

func ensureAuthorizationJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("unexpected trailing JSON value")
		}
		return err
	}
	return nil
}
