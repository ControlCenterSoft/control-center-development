package pxe

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
)

var ErrBootSessionReadmissionConsumptionReconciliation = errors.New(
	"PXE terminal readmission consumption reconciliation failed",
)

const bootSessionReadmissionConsumptionReconciliationVersion = "boot-session-readmission-consumption-reconciliation-v1"

// BootSessionReadmissionConsumptionReconciliation seals the durable observation
// of the one and only readmission recovery generation. It is evidence-only and
// cannot authorize another readmission, boot handoff, provisioning action, or
// host/network mutation.
type BootSessionReadmissionConsumptionReconciliation struct {
	ReconciliationID                string                                    `json:"reconciliationId"`
	ReconciliationVersion           string                                    `json:"reconciliationVersion"`
	State                           BootSessionConsumptionReconciliationState `json:"state"`
	ReadmissionConsumptionReceiptID string                                    `json:"readmissionConsumptionReceiptId"`
	ReadmissionReceiptID            string                                    `json:"readmissionReceiptId"`
	FreshAdmissionID                string                                    `json:"freshAdmissionId"`
	FreshReplayKey                  string                                    `json:"freshReplayKey"`
	CandidateConsumptionReceiptID   string                                    `json:"candidateConsumptionReceiptId"`
	ObservedReceiptID               string                                    `json:"observedReceiptId,omitempty"`
	ObservedAtUnix                  int64                                     `json:"observedAtUnix"`
	RecoveryGeneration              int                                       `json:"recoveryGeneration"`
	MaxRecoveryGenerations          int                                       `json:"maxRecoveryGenerations"`
	TerminalGeneration              bool                                      `json:"terminalGeneration"`
	BootHandoffAuthorized           bool                                      `json:"bootHandoffAuthorized"`
	FurtherReadmissionAuthorized    bool                                      `json:"furtherReadmissionAuthorized"`
	AutomaticRetryAuthorized        bool                                      `json:"automaticRetryAuthorized"`
	ProvisioningAuthorized          bool                                      `json:"provisioningAuthorized"`
	SecretInjectionAuthorized       bool                                      `json:"secretInjectionAuthorized"`
	HostMutation                    bool                                      `json:"hostMutation"`
	NetworkMutation                 bool                                      `json:"networkMutation"`
	ProductionMutation              bool                                      `json:"productionMutation"`
	RecoveryRequired                bool                                      `json:"recoveryRequired"`
	RecoveryAction                  string                                    `json:"recoveryAction"`
}

type bootSessionReadmissionConsumptionReconciliationDigest struct {
	ReconciliationVersion           string                                    `json:"reconciliationVersion"`
	State                           BootSessionConsumptionReconciliationState `json:"state"`
	ReadmissionConsumptionReceiptID string                                    `json:"readmissionConsumptionReceiptId"`
	ReadmissionReceiptID            string                                    `json:"readmissionReceiptId"`
	FreshAdmissionID                string                                    `json:"freshAdmissionId"`
	FreshReplayKey                  string                                    `json:"freshReplayKey"`
	CandidateConsumptionReceiptID   string                                    `json:"candidateConsumptionReceiptId"`
	ObservedReceiptID               string                                    `json:"observedReceiptId,omitempty"`
	ObservedAtUnix                  int64                                     `json:"observedAtUnix"`
	RecoveryGeneration              int                                       `json:"recoveryGeneration"`
	MaxRecoveryGenerations          int                                       `json:"maxRecoveryGenerations"`
	TerminalGeneration              bool                                      `json:"terminalGeneration"`
	RecoveryRequired                bool                                      `json:"recoveryRequired"`
	RecoveryAction                  string                                    `json:"recoveryAction"`
}

// ReconcileBootSessionReadmissionConsumption resolves only an ambiguous consume
// from the terminal readmission generation. A definitely-absent observation
// never becomes authority for a second readmission; further boot activity must
// start from a separate operator-controlled deployment intent.
func ReconcileBootSessionReadmissionConsumption(
	store BootSessionConsumptionReconciliationStore,
	terminal BootSessionReadmissionConsumptionReceipt,
	readmission BootSessionReadmissionReceipt,
	previous BootSessionAdmission,
	previousCandidate BootSessionConsumptionReceipt,
	previousReconciliation BootSessionConsumptionReconciliation,
	fresh BootSessionAdmission,
	freshCandidate BootSessionConsumptionReceipt,
	observedAtUnix int64,
) (BootSessionReadmissionConsumptionReconciliation, error) {
	if store == nil {
		return BootSessionReadmissionConsumptionReconciliation{},
			fmt.Errorf("%w: reconciliation store is required", ErrBootSessionReadmissionConsumptionReconciliation)
	}
	if err := VerifyBootSessionReadmissionConsumptionReceipt(
		terminal,
		readmission,
		previous,
		previousCandidate,
		previousReconciliation,
		fresh,
		freshCandidate,
	); err != nil {
		return BootSessionReadmissionConsumptionReconciliation{},
			fmt.Errorf("%w: terminal consumption evidence: %v", ErrBootSessionReadmissionConsumptionReconciliation, err)
	}
	if terminal.Status != BootSessionConsumptionAmbiguous || terminal.AtomicConsumeVerified ||
		terminal.BootHandoffConsumed || !terminal.RecoveryRequired ||
		terminal.RecoveryAction != "operator-reconcile-readmitted-replay-key-no-automatic-reissue" {
		return BootSessionReadmissionConsumptionReconciliation{},
			fmt.Errorf(
				"%w: only ambiguous terminal generation evidence may be reconciled",
				ErrBootSessionReadmissionConsumptionReconciliation,
			)
	}
	if observedAtUnix <= 0 || observedAtUnix < freshCandidate.ConsumedAtUnix {
		return BootSessionReadmissionConsumptionReconciliation{},
			fmt.Errorf(
				"%w: observation time must not precede terminal consumption evidence",
				ErrBootSessionReadmissionConsumptionReconciliation,
			)
	}

	observation, observeErr := store.ObserveBootSessionConsumption(fresh.ReplayKey)
	if observeErr != nil || observation.State == BootSessionConsumptionReconciledAmbiguous {
		decision, err := newBootSessionReadmissionConsumptionReconciliation(
			BootSessionConsumptionReconciledAmbiguous,
			terminal,
			readmission,
			fresh,
			freshCandidate,
			"",
			observedAtUnix,
			true,
			"operator-reconcile-terminal-readmission-consumption",
		)
		if err != nil {
			return BootSessionReadmissionConsumptionReconciliation{}, err
		}
		if observeErr != nil {
			return decision, fmt.Errorf(
				"%w: durable observation is ambiguous: %v",
				ErrBootSessionReadmissionConsumptionReconciliation,
				observeErr,
			)
		}
		return decision, fmt.Errorf(
			"%w: durable observation is ambiguous",
			ErrBootSessionReadmissionConsumptionReconciliation,
		)
	}

	switch observation.State {
	case BootSessionConsumptionReconciledConsumed:
		if observation.Existing == nil {
			return ambiguousBootSessionReadmissionConsumptionReconciliation(
				terminal, readmission, fresh, freshCandidate, observedAtUnix,
				"consumed observation omitted persisted receipt",
			)
		}
		existing := *observation.Existing
		existingRequest := BootSessionConsumptionRequest{
			AttemptID:      existing.AttemptID,
			ConsumerID:     existing.ConsumerID,
			ConsumedAtUnix: existing.ConsumedAtUnix,
		}
		if err := VerifyBootSessionConsumptionReceipt(existing, existingRequest, fresh); err != nil {
			return ambiguousBootSessionReadmissionConsumptionReconciliation(
				terminal, readmission, fresh, freshCandidate, observedAtUnix,
				"persisted receipt no longer matches exact fresh admission",
			)
		}
		return newBootSessionReadmissionConsumptionReconciliation(
			BootSessionConsumptionReconciledConsumed,
			terminal,
			readmission,
			fresh,
			freshCandidate,
			existing.ReceiptID,
			observedAtUnix,
			false,
			"close-terminal-readmission-as-consumed",
		)

	case BootSessionConsumptionReconciledDefinitelyAbsent:
		if observation.Existing != nil {
			return ambiguousBootSessionReadmissionConsumptionReconciliation(
				terminal, readmission, fresh, freshCandidate, observedAtUnix,
				"absent observation unexpectedly included persisted receipt",
			)
		}
		return newBootSessionReadmissionConsumptionReconciliation(
			BootSessionConsumptionReconciledDefinitelyAbsent,
			terminal,
			readmission,
			fresh,
			freshCandidate,
			"",
			observedAtUnix,
			true,
			"operator-close-terminal-readmission-or-create-new-deployment-intent",
		)

	default:
		return ambiguousBootSessionReadmissionConsumptionReconciliation(
			terminal, readmission, fresh, freshCandidate, observedAtUnix,
			"unrecognized durable observation state",
		)
	}
}

func VerifyBootSessionReadmissionConsumptionReconciliation(
	decision BootSessionReadmissionConsumptionReconciliation,
	terminal BootSessionReadmissionConsumptionReceipt,
	readmission BootSessionReadmissionReceipt,
	previous BootSessionAdmission,
	previousCandidate BootSessionConsumptionReceipt,
	previousReconciliation BootSessionConsumptionReconciliation,
	fresh BootSessionAdmission,
	freshCandidate BootSessionConsumptionReceipt,
) error {
	if err := VerifyBootSessionReadmissionConsumptionReceipt(
		terminal,
		readmission,
		previous,
		previousCandidate,
		previousReconciliation,
		fresh,
		freshCandidate,
	); err != nil {
		return fmt.Errorf("%w: terminal consumption evidence: %v", ErrBootSessionReadmissionConsumptionReconciliation, err)
	}
	if terminal.Status != BootSessionConsumptionAmbiguous || terminal.AtomicConsumeVerified ||
		terminal.BootHandoffConsumed || !terminal.RecoveryRequired {
		return fmt.Errorf(
			"%w: source evidence is not an ambiguous terminal generation",
			ErrBootSessionReadmissionConsumptionReconciliation,
		)
	}
	if decision.ReconciliationVersion != bootSessionReadmissionConsumptionReconciliationVersion ||
		!bootSessionConsumptionCanonicalSHA256(decision.ReconciliationID) ||
		!bootSessionConsumptionCanonicalSHA256(decision.ReadmissionConsumptionReceiptID) ||
		!bootSessionConsumptionCanonicalSHA256(decision.ReadmissionReceiptID) ||
		!bootSessionConsumptionCanonicalSHA256(decision.FreshAdmissionID) ||
		!bootSessionConsumptionCanonicalSHA256(decision.FreshReplayKey) ||
		!bootSessionConsumptionCanonicalSHA256(decision.CandidateConsumptionReceiptID) ||
		decision.RecoveryGeneration != bootSessionReadmissionRecoveryGeneration ||
		decision.MaxRecoveryGenerations != bootSessionReadmissionMaxRecoveryGenerations ||
		!decision.TerminalGeneration || decision.ObservedAtUnix <= 0 ||
		decision.ObservedAtUnix < freshCandidate.ConsumedAtUnix {
		return fmt.Errorf(
			"%w: reconciliation identity or terminal generation drift",
			ErrBootSessionReadmissionConsumptionReconciliation,
		)
	}
	if decision.ObservedReceiptID != "" &&
		!bootSessionConsumptionCanonicalSHA256(decision.ObservedReceiptID) {
		return fmt.Errorf(
			"%w: observed receipt identity must be canonical SHA-256",
			ErrBootSessionReadmissionConsumptionReconciliation,
		)
	}
	if decision.ReadmissionConsumptionReceiptID != terminal.ReceiptID ||
		decision.ReadmissionReceiptID != readmission.ReceiptID ||
		decision.FreshAdmissionID != fresh.AdmissionID || decision.FreshReplayKey != fresh.ReplayKey ||
		decision.CandidateConsumptionReceiptID != freshCandidate.ReceiptID {
		return fmt.Errorf("%w: reconciliation lineage drift", ErrBootSessionReadmissionConsumptionReconciliation)
	}
	if decision.BootHandoffAuthorized || decision.FurtherReadmissionAuthorized ||
		decision.AutomaticRetryAuthorized || decision.ProvisioningAuthorized ||
		decision.SecretInjectionAuthorized || decision.HostMutation || decision.NetworkMutation ||
		decision.ProductionMutation {
		return fmt.Errorf(
			"%w: terminal reconciliation must not grant deployment authority",
			ErrBootSessionReadmissionConsumptionReconciliation,
		)
	}

	switch decision.State {
	case BootSessionConsumptionReconciledConsumed:
		if decision.ObservedReceiptID == "" || decision.RecoveryRequired ||
			decision.RecoveryAction != "close-terminal-readmission-as-consumed" {
			return fmt.Errorf("%w: invalid consumed terminal reconciliation", ErrBootSessionReadmissionConsumptionReconciliation)
		}
	case BootSessionConsumptionReconciledDefinitelyAbsent:
		if decision.ObservedReceiptID != "" || !decision.RecoveryRequired ||
			decision.RecoveryAction != "operator-close-terminal-readmission-or-create-new-deployment-intent" {
			return fmt.Errorf(
				"%w: absent terminal generation must not authorize readmission",
				ErrBootSessionReadmissionConsumptionReconciliation,
			)
		}
	case BootSessionConsumptionReconciledAmbiguous:
		if decision.ObservedReceiptID != "" || !decision.RecoveryRequired ||
			decision.RecoveryAction != "operator-reconcile-terminal-readmission-consumption" {
			return fmt.Errorf(
				"%w: ambiguous terminal reconciliation must fail closed",
				ErrBootSessionReadmissionConsumptionReconciliation,
			)
		}
	default:
		return fmt.Errorf("%w: unknown reconciliation state", ErrBootSessionReadmissionConsumptionReconciliation)
	}

	expected, err := newBootSessionReadmissionConsumptionReconciliation(
		decision.State,
		terminal,
		readmission,
		fresh,
		freshCandidate,
		decision.ObservedReceiptID,
		decision.ObservedAtUnix,
		decision.RecoveryRequired,
		decision.RecoveryAction,
	)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(expected, decision) {
		return fmt.Errorf("%w: reconciliation digest or semantics drift", ErrBootSessionReadmissionConsumptionReconciliation)
	}
	return nil
}

func ambiguousBootSessionReadmissionConsumptionReconciliation(
	terminal BootSessionReadmissionConsumptionReceipt,
	readmission BootSessionReadmissionReceipt,
	fresh BootSessionAdmission,
	freshCandidate BootSessionConsumptionReceipt,
	observedAtUnix int64,
	reason string,
) (BootSessionReadmissionConsumptionReconciliation, error) {
	decision, err := newBootSessionReadmissionConsumptionReconciliation(
		BootSessionConsumptionReconciledAmbiguous,
		terminal,
		readmission,
		fresh,
		freshCandidate,
		"",
		observedAtUnix,
		true,
		"operator-reconcile-terminal-readmission-consumption",
	)
	if err != nil {
		return BootSessionReadmissionConsumptionReconciliation{}, err
	}
	return decision, fmt.Errorf("%w: %s", ErrBootSessionReadmissionConsumptionReconciliation, reason)
}

func newBootSessionReadmissionConsumptionReconciliation(
	state BootSessionConsumptionReconciliationState,
	terminal BootSessionReadmissionConsumptionReceipt,
	readmission BootSessionReadmissionReceipt,
	fresh BootSessionAdmission,
	freshCandidate BootSessionConsumptionReceipt,
	observedReceiptID string,
	observedAtUnix int64,
	recoveryRequired bool,
	recoveryAction string,
) (BootSessionReadmissionConsumptionReconciliation, error) {
	digestInput := bootSessionReadmissionConsumptionReconciliationDigest{
		ReconciliationVersion:           bootSessionReadmissionConsumptionReconciliationVersion,
		State:                           state,
		ReadmissionConsumptionReceiptID: terminal.ReceiptID,
		ReadmissionReceiptID:            readmission.ReceiptID,
		FreshAdmissionID:                fresh.AdmissionID,
		FreshReplayKey:                  fresh.ReplayKey,
		CandidateConsumptionReceiptID:   freshCandidate.ReceiptID,
		ObservedReceiptID:               observedReceiptID,
		ObservedAtUnix:                  observedAtUnix,
		RecoveryGeneration:              bootSessionReadmissionRecoveryGeneration,
		MaxRecoveryGenerations:          bootSessionReadmissionMaxRecoveryGenerations,
		TerminalGeneration:              true,
		RecoveryRequired:                recoveryRequired,
		RecoveryAction:                  recoveryAction,
	}
	encoded, err := json.Marshal(digestInput)
	if err != nil {
		return BootSessionReadmissionConsumptionReconciliation{},
			fmt.Errorf(
				"%w: encode canonical terminal reconciliation: %v",
				ErrBootSessionReadmissionConsumptionReconciliation,
				err,
			)
	}
	digest := sha256.Sum256(encoded)

	return BootSessionReadmissionConsumptionReconciliation{
		ReconciliationID:                hex.EncodeToString(digest[:]),
		ReconciliationVersion:           bootSessionReadmissionConsumptionReconciliationVersion,
		State:                           state,
		ReadmissionConsumptionReceiptID: terminal.ReceiptID,
		ReadmissionReceiptID:            readmission.ReceiptID,
		FreshAdmissionID:                fresh.AdmissionID,
		FreshReplayKey:                  fresh.ReplayKey,
		CandidateConsumptionReceiptID:   freshCandidate.ReceiptID,
		ObservedReceiptID:               observedReceiptID,
		ObservedAtUnix:                  observedAtUnix,
		RecoveryGeneration:              bootSessionReadmissionRecoveryGeneration,
		MaxRecoveryGenerations:          bootSessionReadmissionMaxRecoveryGenerations,
		TerminalGeneration:              true,
		BootHandoffAuthorized:           false,
		FurtherReadmissionAuthorized:    false,
		AutomaticRetryAuthorized:        false,
		ProvisioningAuthorized:          false,
		SecretInjectionAuthorized:       false,
		HostMutation:                    false,
		NetworkMutation:                 false,
		ProductionMutation:              false,
		RecoveryRequired:                recoveryRequired,
		RecoveryAction:                  recoveryAction,
	}, nil
}
