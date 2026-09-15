package recovery

import (
	"encoding/json"
	"fmt"
)

type restoreDrillTransitionRequestAlias RestoreDrillTransitionRequest

// UnmarshalJSON keeps RestoreDrillTransitionRequest deterministic at JSON
// boundaries. encoding/json otherwise accepts duplicate object keys using
// last-value-wins semantics, which can make safety-relevant transition input
// ambiguous between producers, audit evidence, and the typed state machine.
//
// Unknown fields retain the existing compatibility behavior here; callers
// that expose an HTTP boundary must still apply their normal strict
// unknown-field policy. Successor resource versions remain server-allocated
// and are not part of this request type.
func (request *RestoreDrillTransitionRequest) UnmarshalJSON(raw []byte) error {
	if err := rejectDuplicateFields(raw); err != nil {
		return fmt.Errorf("%w: decode transition request: %v", ErrInvalidRestoreDrillTransition, err)
	}
	var decoded restoreDrillTransitionRequestAlias
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return fmt.Errorf("%w: decode transition request: %v", ErrInvalidRestoreDrillTransition, err)
	}
	*request = RestoreDrillTransitionRequest(decoded)
	return nil
}
