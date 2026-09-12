package operationsview

import (
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	MaintenanceWindowContractVersion     = "ui.operations-maintenance-window/v1"
	MaxMaintenanceWindowIdentifierLength = 255
)

var ErrInvalidMaintenanceWindow = errors.New("invalid operations maintenance window")

type MaintenanceWindowState string

const (
	MaintenanceWindowScheduled MaintenanceWindowState = "scheduled"
	MaintenanceWindowActive    MaintenanceWindowState = "active"
	MaintenanceWindowExpired   MaintenanceWindowState = "expired"
)

// MaintenanceWindowEvidence binds one operator-visible time window to an exact
// immutable Change revision. It is scheduling evidence only: the contract does
// not approve a Change and does not authorize a Job to start, continue, retry
// or bypass a preflight/policy check.
type MaintenanceWindowEvidence struct {
	ContractVersion string                 `json:"contract_version"`
	WindowID        string                 `json:"window_id"`
	ChangeID        string                 `json:"change_id"`
	RevisionID      string                 `json:"revision_id"`
	RevisionDigest  string                 `json:"revision_digest"`
	StartsAt        time.Time              `json:"starts_at"`
	EndsAt          time.Time              `json:"ends_at"`
	ObservedAt      time.Time              `json:"observed_at"`
	EvaluatedAt     time.Time              `json:"evaluated_at"`
	State           MaintenanceWindowState `json:"state"`
}

type MaintenanceWindowInput struct {
	WindowID       string
	ChangeID       string
	RevisionID     string
	RevisionDigest string
	StartsAt       time.Time
	EndsAt         time.Time
	ObservedAt     time.Time
}

// BuildMaintenanceWindowEvidence normalizes timestamps to UTC and derives the
// window state at one explicit evaluation instant. Future-dated source evidence
// is rejected fail-closed.
func BuildMaintenanceWindowEvidence(input MaintenanceWindowInput, now time.Time) (MaintenanceWindowEvidence, error) {
	if now.IsZero() {
		return MaintenanceWindowEvidence{}, fmt.Errorf("%w: evaluation time is required", ErrInvalidMaintenanceWindow)
	}
	now = now.UTC()
	evidence := MaintenanceWindowEvidence{
		ContractVersion: MaintenanceWindowContractVersion,
		WindowID:        input.WindowID,
		ChangeID:        input.ChangeID,
		RevisionID:      input.RevisionID,
		RevisionDigest:  input.RevisionDigest,
		StartsAt:        input.StartsAt.UTC(),
		EndsAt:          input.EndsAt.UTC(),
		ObservedAt:      input.ObservedAt.UTC(),
		EvaluatedAt:     now,
	}
	if evidence.ObservedAt.After(now) {
		return MaintenanceWindowEvidence{}, fmt.Errorf("%w: observed_at is future-dated", ErrInvalidMaintenanceWindow)
	}
	evidence.State = maintenanceWindowState(evidence.StartsAt, evidence.EndsAt, now)
	if err := ValidateMaintenanceWindowEvidence(evidence); err != nil {
		return MaintenanceWindowEvidence{}, err
	}
	return evidence, nil
}

// ValidateMaintenanceWindowEvidence is suitable for persisted/provider
// evidence before it reaches an operator-facing read model.
func ValidateMaintenanceWindowEvidence(evidence MaintenanceWindowEvidence) error {
	if evidence.ContractVersion != MaintenanceWindowContractVersion {
		return fmt.Errorf("%w: unexpected contract version", ErrInvalidMaintenanceWindow)
	}
	for field, value := range map[string]string{
		"window_id":   evidence.WindowID,
		"change_id":   evidence.ChangeID,
		"revision_id": evidence.RevisionID,
	} {
		if !validMaintenanceIdentifier(value) {
			return fmt.Errorf("%w: canonical %s is required", ErrInvalidMaintenanceWindow, field)
		}
	}
	if !validMaintenanceDigest(evidence.RevisionDigest) {
		return fmt.Errorf("%w: canonical sha256 revision digest is required", ErrInvalidMaintenanceWindow)
	}
	for field, value := range map[string]time.Time{
		"starts_at":    evidence.StartsAt,
		"ends_at":      evidence.EndsAt,
		"observed_at":  evidence.ObservedAt,
		"evaluated_at": evidence.EvaluatedAt,
	} {
		if value.IsZero() || value.Location() != time.UTC {
			return fmt.Errorf("%w: %s must be a non-zero UTC timestamp", ErrInvalidMaintenanceWindow, field)
		}
	}
	if !evidence.EndsAt.After(evidence.StartsAt) {
		return fmt.Errorf("%w: ends_at must be later than starts_at", ErrInvalidMaintenanceWindow)
	}
	if evidence.ObservedAt.After(evidence.EvaluatedAt) {
		return fmt.Errorf("%w: observed_at cannot be later than evaluated_at", ErrInvalidMaintenanceWindow)
	}
	expected := maintenanceWindowState(evidence.StartsAt, evidence.EndsAt, evidence.EvaluatedAt)
	if evidence.State != expected {
		return fmt.Errorf("%w: state %q is inconsistent with evaluated_at", ErrInvalidMaintenanceWindow, evidence.State)
	}
	return nil
}

func maintenanceWindowState(startsAt, endsAt, now time.Time) MaintenanceWindowState {
	if now.Before(startsAt) {
		return MaintenanceWindowScheduled
	}
	if now.Before(endsAt) {
		return MaintenanceWindowActive
	}
	return MaintenanceWindowExpired
}

func validMaintenanceIdentifier(value string) bool {
	trimmed := strings.TrimSpace(value)
	return trimmed != "" && value == trimmed && len(value) <= MaxMaintenanceWindowIdentifierLength
}

func validMaintenanceDigest(value string) bool {
	const prefix = "sha256:"
	if !strings.HasPrefix(value, prefix) {
		return false
	}
	digest := strings.TrimPrefix(value, prefix)
	if len(digest) != 64 || digest != strings.ToLower(digest) {
		return false
	}
	_, err := hex.DecodeString(digest)
	return err == nil
}
