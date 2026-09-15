package sitesync

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
)

var (
	ErrInvalidRecord   = errors.New("invalid site state record")
	ErrWrongAuthority  = errors.New("wrong state authority")
	ErrStaleGeneration = errors.New("stale state generation")
	ErrStateConflict   = errors.New("site state conflict")
)

// StateKind separates desired state, which is distributed top-down, from
// actual state, which is reported bottom-up by a Site Controller.
type StateKind string

const (
	DesiredState StateKind = "desired"
	ActualState  StateKind = "actual"
)

// Authority identifies the only side allowed to originate a record kind.
type Authority string

const (
	GlobalAuthority Authority = "global"
	SiteAuthority   Authority = "site"
)

// Record is a compact synchronization envelope. PayloadHash represents the
// canonical payload without coupling synchronization logic to a concrete
// resource schema.
type Record struct {
	SiteID          string
	ResourceID      string
	Kind            StateKind
	Authority       Authority
	Generation      uint64
	ResourceVersion string
	PayloadHash     string
}

func (r Record) Validate() error {
	for field, value := range map[string]string{
		"site_id":          r.SiteID,
		"resource_id":      r.ResourceID,
		"resource_version": r.ResourceVersion,
		"payload_hash":     r.PayloadHash,
	} {
		if err := validateCanonicalToken(field, value); err != nil {
			return err
		}
	}
	if r.Generation == 0 {
		return fmt.Errorf("%w: generation is required", ErrInvalidRecord)
	}
	switch r.Kind {
	case DesiredState:
		if r.Authority != GlobalAuthority {
			return fmt.Errorf("%w: desired state must originate globally", ErrWrongAuthority)
		}
	case ActualState:
		if r.Authority != SiteAuthority {
			return fmt.Errorf("%w: actual state must originate at the site", ErrWrongAuthority)
		}
	default:
		return fmt.Errorf("%w: unknown kind %q", ErrInvalidRecord, r.Kind)
	}
	return nil
}

// Direction makes the synchronization direction explicit for callers and
// observability: desired state flows global -> site, actual state site -> global.
func (r Record) Direction() (string, error) {
	if err := r.Validate(); err != nil {
		return "", err
	}
	if r.Kind == DesiredState {
		return "global-to-site", nil
	}
	return "site-to-global", nil
}

// Reconcile compares one authoritative incoming revision with the currently
// stored revision. Equal revisions are idempotent; same-generation divergence
// is an explicit conflict and must never be silently overwritten.
func Reconcile(current, incoming Record) (Record, bool, error) {
	if err := current.Validate(); err != nil {
		return Record{}, false, err
	}
	if err := incoming.Validate(); err != nil {
		return Record{}, false, err
	}
	if current.SiteID != incoming.SiteID || current.ResourceID != incoming.ResourceID || current.Kind != incoming.Kind {
		return Record{}, false, fmt.Errorf("%w: identity mismatch", ErrInvalidRecord)
	}
	if current.Authority != incoming.Authority {
		return Record{}, false, ErrWrongAuthority
	}
	if incoming.Generation < current.Generation {
		return Record{}, false, ErrStaleGeneration
	}
	if incoming.Generation == current.Generation {
		if incoming.ResourceVersion == current.ResourceVersion && incoming.PayloadHash == current.PayloadHash {
			return current, false, nil
		}
		return Record{}, false, fmt.Errorf(
			"%w: %s/%s generation %d diverged",
			ErrStateConflict,
			current.SiteID,
			current.ResourceID,
			current.Generation,
		)
	}
	return incoming, true, nil
}

func validateCanonicalToken(field, value string) error {
	if value == "" || strings.TrimSpace(value) != value {
		return fmt.Errorf("%w: %s is required and must not have surrounding whitespace", ErrInvalidRecord, field)
	}
	for _, r := range value {
		if unicode.IsSpace(r) || unicode.IsControl(r) {
			return fmt.Errorf("%w: %s contains whitespace or control characters", ErrInvalidRecord, field)
		}
	}
	return nil
}
