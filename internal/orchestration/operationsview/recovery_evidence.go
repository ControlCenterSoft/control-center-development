package operationsview

import (
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	RecoveryEvidenceContractVersion     = "ui.operations-recovery-evidence/v1"
	MaxRecoveryEvidenceIdentifierLength = 255
)

var ErrInvalidRecoveryEvidence = errors.New("invalid operations recovery evidence")

type RecoveryStrategy string

const (
	RecoveryStrategyRollback       RecoveryStrategy = "rollback"
	RecoveryStrategyRollforward    RecoveryStrategy = "rollforward"
	RecoveryStrategyManualRecovery RecoveryStrategy = "manual_recovery"
)

type RecoveryReadiness string

const (
	RecoveryReadinessReady       RecoveryReadiness = "ready"
	RecoveryReadinessDegraded    RecoveryReadiness = "degraded"
	RecoveryReadinessUnavailable RecoveryReadiness = "unavailable"
)

// RecoveryEvidence is read-only operator evidence for the recovery path of one
// exact immutable Change revision. Presence of ready evidence does not approve,
// queue or execute recovery and does not grant any execution authority.
type RecoveryEvidence struct {
	ContractVersion        string            `json:"contract_version"`
	RecoveryPlanID         string            `json:"recovery_plan_id"`
	ChangeID               string            `json:"change_id"`
	RevisionID             string            `json:"revision_id"`
	RevisionDigest         string            `json:"revision_digest"`
	Strategy               RecoveryStrategy  `json:"strategy"`
	Readiness              RecoveryReadiness `json:"readiness"`
	ObservedAt             time.Time         `json:"observed_at"`
	ValidatedAt            time.Time         `json:"validated_at"`
	VerificationDigest     string            `json:"verification_digest"`
	OperatorActionRequired bool              `json:"operator_action_required"`
}

type RecoveryEvidenceInput struct {
	RecoveryPlanID         string
	ChangeID               string
	RevisionID             string
	RevisionDigest         string
	Strategy               RecoveryStrategy
	Readiness              RecoveryReadiness
	ObservedAt             time.Time
	VerificationDigest     string
	OperatorActionRequired bool
}

// BuildRecoveryEvidence normalizes timestamps to UTC and binds validation to an
// explicit instant. Future-dated source evidence is rejected fail-closed.
func BuildRecoveryEvidence(input RecoveryEvidenceInput, now time.Time) (RecoveryEvidence, error) {
	if now.IsZero() {
		return RecoveryEvidence{}, fmt.Errorf("%w: validation time is required", ErrInvalidRecoveryEvidence)
	}
	now = now.UTC()
	evidence := RecoveryEvidence{
		ContractVersion:        RecoveryEvidenceContractVersion,
		RecoveryPlanID:         input.RecoveryPlanID,
		ChangeID:               input.ChangeID,
		RevisionID:             input.RevisionID,
		RevisionDigest:         input.RevisionDigest,
		Strategy:               input.Strategy,
		Readiness:              input.Readiness,
		ObservedAt:             input.ObservedAt.UTC(),
		ValidatedAt:            now,
		VerificationDigest:     input.VerificationDigest,
		OperatorActionRequired: input.OperatorActionRequired,
	}
	if evidence.ObservedAt.After(now) {
		return RecoveryEvidence{}, fmt.Errorf("%w: observed_at is future-dated", ErrInvalidRecoveryEvidence)
	}
	if err := ValidateRecoveryEvidence(evidence); err != nil {
		return RecoveryEvidence{}, err
	}
	return evidence, nil
}

// ValidateRecoveryEvidence validates persisted/provider evidence before it can
// become visible in an operator-facing read model.
func ValidateRecoveryEvidence(evidence RecoveryEvidence) error {
	if evidence.ContractVersion != RecoveryEvidenceContractVersion {
		return fmt.Errorf("%w: unexpected contract version", ErrInvalidRecoveryEvidence)
	}
	for field, value := range map[string]string{
		"recovery_plan_id": evidence.RecoveryPlanID,
		"change_id":        evidence.ChangeID,
		"revision_id":      evidence.RevisionID,
	} {
		if !validRecoveryIdentifier(value) {
			return fmt.Errorf("%w: canonical %s is required", ErrInvalidRecoveryEvidence, field)
		}
	}
	if !validRecoveryDigest(evidence.RevisionDigest) {
		return fmt.Errorf("%w: canonical sha256 revision digest is required", ErrInvalidRecoveryEvidence)
	}
	if !validRecoveryDigest(evidence.VerificationDigest) {
		return fmt.Errorf("%w: canonical sha256 verification digest is required", ErrInvalidRecoveryEvidence)
	}
	if !validRecoveryStrategy(evidence.Strategy) {
		return fmt.Errorf("%w: unsupported strategy %q", ErrInvalidRecoveryEvidence, evidence.Strategy)
	}
	if !validRecoveryReadiness(evidence.Readiness) {
		return fmt.Errorf("%w: unsupported readiness %q", ErrInvalidRecoveryEvidence, evidence.Readiness)
	}
	if evidence.ObservedAt.IsZero() || evidence.ObservedAt.Location() != time.UTC {
		return fmt.Errorf("%w: observed_at must be a non-zero UTC timestamp", ErrInvalidRecoveryEvidence)
	}
	if evidence.ValidatedAt.IsZero() || evidence.ValidatedAt.Location() != time.UTC {
		return fmt.Errorf("%w: validated_at must be a non-zero UTC timestamp", ErrInvalidRecoveryEvidence)
	}
	if evidence.ObservedAt.After(evidence.ValidatedAt) {
		return fmt.Errorf("%w: observed_at cannot be later than validated_at", ErrInvalidRecoveryEvidence)
	}
	if evidence.Strategy == RecoveryStrategyManualRecovery && !evidence.OperatorActionRequired {
		return fmt.Errorf("%w: manual recovery must require operator action", ErrInvalidRecoveryEvidence)
	}
	if evidence.Readiness != RecoveryReadinessReady && !evidence.OperatorActionRequired {
		return fmt.Errorf("%w: non-ready recovery must require operator action", ErrInvalidRecoveryEvidence)
	}
	return nil
}

func validRecoveryStrategy(strategy RecoveryStrategy) bool {
	switch strategy {
	case RecoveryStrategyRollback, RecoveryStrategyRollforward, RecoveryStrategyManualRecovery:
		return true
	default:
		return false
	}
}

func validRecoveryReadiness(readiness RecoveryReadiness) bool {
	switch readiness {
	case RecoveryReadinessReady, RecoveryReadinessDegraded, RecoveryReadinessUnavailable:
		return true
	default:
		return false
	}
}

func validRecoveryIdentifier(value string) bool {
	trimmed := strings.TrimSpace(value)
	return trimmed != "" && value == trimmed && len(value) <= MaxRecoveryEvidenceIdentifierLength
}

func validRecoveryDigest(value string) bool {
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
