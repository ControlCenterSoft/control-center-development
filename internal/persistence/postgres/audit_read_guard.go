package postgres

import "fmt"

func validateStoredAuditDetailsSize(size int) error {
	if size > maxAuditJSONBCanonicalBytes {
		return fmt.Errorf("stored audit details exceed %d-byte limit", maxAuditJSONBCanonicalBytes)
	}
	return nil
}
