package pxe

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

var ErrBootSessionConsumptionReconciliation = errors.New("PXE boot session consumption reconciliation failed")

const bootSessionConsumptionReconciliationVersion = "boot-session-consumption-reconciliation-v1"

type BootSessionConsumptionReconciliationState string

const (
	BootSessionConsumptionReconciledConsumed         BootSessionConsumptionReconciliationState = "consumed"
	BootSessionConsumptionReconciledDefinitelyAbsent BootSessionConsumptionReconciliationState = "definitely-absent"
	BootSessionConsumptionReconciledAmbiguous        BootSessionConsumptionReconciliationState = "ambiguous"
)

type BootSessionConsumptionObservation struct {
	State    BootSessionConsumptionReconciliationState
	Existing *BootSessionConsumptionReceipt
}

type BootSessionConsumptionReconciliationStore interface {
	ObserveBootSessionConsumption(replayKey string) (BootSessionConsumptionObservation, error)
}

type BootSessionConsumptionReconciliation struct {
	ReconciliationID             string                                    `json:"reconciliationId"`
	ReconciliationVersion        string                                    `json:"reconciliationVersion"`
	State                        BootSessionConsumptionReconciliationState `json:"state"`
	AdmissionID                  string                                    `json:"admissionId"`
	ReplayKey                    string                                    `json:"replayKey"`
	CandidateReceiptID           string                                    `json:"candidateReceiptId"`
	ObservedReceiptID            string                                    `json:"observedReceiptId,omitempty"`
	ObservedAtUnix               int64                                     `json:"observedAtUnix"`
	AdmissionExpiresAtUnix       int64                                     `json:"admissionExpiresAtUnix"`
	OldAdmissionExpired          bool                                      `json:"oldAdmissionExpired"`
	OldAdmissionReusable         bool                                      `json:"oldAdmissionReusable"`
	FreshAdmissionRequired       bool                                      `json:"freshAdmissionRequired"`
	FreshServingEvidenceRequired bool                                      `json:"freshServingEvidenceRequired"`
	FreshAdmissionAuthorized     bool                                      `json:"freshAdmissionAuthorized"`
	BootHandoffAuthorized        bool                                      `json:"bootHandoffAuthorized"`
	ReplayAuthorized             bool                                      `json:"replayAuthorized"`
	ProvisioningAuthorized       bool                                      `json:"provisioningAuthorized"`
	SecretInjectionAuthorized    bool                                      `json:"secretInjectionAuthorized"`
	HostMutation                 bool                                      `json:"hostMutation"`
	NetworkMutation              bool                                      `json:"networkMutation"`
	ProductionMutation           bool                                      `json:"productionMutation"`
	RecoveryRequired             bool                                      `json:"recoveryRequired"`
	RecoveryAction               string                                    `json:"recoveryAction"`
}

type bootSessionConsumptionReconciliationDigest struct {
	ReconciliationVersion        string                                    `json:"reconciliationVersion"`
	State                        BootSessionConsumptionReconciliationState `json:"state"`
	AdmissionID                  string                                    `json:"admissionId"`
	ReplayKey                    string                                    `json:"replayKey"`
	CandidateReceiptID           string                                    `json:"candidateReceiptId"`
	ObservedReceiptID            string                                    `json:"observedReceiptId,omitempty"`
	ObservedAtUnix               int64                                     `json:"observedAtUnix"`
	AdmissionExpiresAtUnix       int64                                     `json:"admissionExpiresAtUnix"`
	OldAdmissionExpired          bool                                      `json:"oldAdmissionExpired"`
	FreshAdmissionRequired       bool                                      `json:"freshAdmissionRequired"`
	FreshServingEvidenceRequired bool                                      `json:"freshServingEvidenceRequired"`
	RecoveryRequired             bool                                      `json:"recoveryRequired"`
	RecoveryAction               string                                    `json:"recoveryAction"`
}

func (s *DirectoryBootSessionConsumptionStore) ObserveBootSessionConsumption(
	replayKey string,
) (BootSessionConsumptionObservation, error) {
	if s == nil || s.root == "" {
		return BootSessionConsumptionObservation{State: BootSessionConsumptionReconciledAmbiguous},
			fmt.Errorf("%w: initialized store is required", ErrBootSessionConsumptionStore)
	}
	if !bootSessionConsumptionCanonicalSHA256(replayKey) {
		return BootSessionConsumptionObservation{State: BootSessionConsumptionReconciledAmbiguous},
			fmt.Errorf("%w: replay key must be a canonical SHA-256 identity", ErrBootSessionConsumptionStore)
	}

	finalPath := filepath.Join(s.root, replayKey+".json")
	if _, err := os.Lstat(finalPath); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return BootSessionConsumptionObservation{State: BootSessionConsumptionReconciledDefinitelyAbsent}, nil
		}
		return BootSessionConsumptionObservation{State: BootSessionConsumptionReconciledAmbiguous},
			fmt.Errorf("%w: inspect durable consumption record: %v", ErrBootSessionConsumptionStore, err)
	}

	existing, err := s.loadExisting(replayKey)
	if err != nil {
		return BootSessionConsumptionObservation{State: BootSessionConsumptionReconciledAmbiguous}, err
	}
	return BootSessionConsumptionObservation{
		State:    BootSessionConsumptionReconciledConsumed,
		Existing: &existing,
	}, nil
}

func ReconcileBootSessionConsumption(
	store BootSessionConsumptionReconciliationStore,
	candidate BootSessionConsumptionReceipt,
	admission BootSessionAdmission,
	observedAtUnix int64,
) (BootSessionConsumptionReconciliation, error) {
	if store == nil {
		return BootSessionConsumptionReconciliation{},
			fmt.Errorf("%w: reconciliation store is required", ErrBootSessionConsumptionReconciliation)
	}
	if observedAtUnix <= 0 || observedAtUnix < candidate.ConsumedAtUnix {
		return BootSessionConsumptionReconciliation{},
			fmt.Errorf("%w: observation time must not precede the candidate consumption evidence", ErrBootSessionConsumptionReconciliation)
	}
	if err := validateBootSessionConsumptionStoreReceipt(candidate); err != nil {
		return BootSessionConsumptionReconciliation{},
			fmt.Errorf("%w: candidate receipt integrity failed: %v", ErrBootSessionConsumptionReconciliation, err)
	}
	candidateRequest := BootSessionConsumptionRequest{
		AttemptID:      candidate.AttemptID,
		ConsumerID:     candidate.ConsumerID,
		ConsumedAtUnix: candidate.ConsumedAtUnix,
	}
	if err := VerifyBootSessionConsumptionReceipt(candidate, candidateRequest, admission); err != nil {
		return BootSessionConsumptionReconciliation{},
			fmt.Errorf("%w: candidate receipt no longer matches exact admission: %v", ErrBootSessionConsumptionReconciliation, err)
	}

	observation, observeErr := store.ObserveBootSessionConsumption(candidate.ReplayKey)
	if observeErr != nil || observation.State == BootSessionConsumptionReconciledAmbiguous {
		decision, decisionErr := newBootSessionConsumptionReconciliation(
			BootSessionConsumptionReconciledAmbiguous,
			candidate,
			"",
			observedAtUnix,
			admission.ExpiresAtUnix,
			observedAtUnix >= admission.ExpiresAtUnix,
			false,
			false,
			true,
			"operator-reconcile-durable-consumption-store",
		)
		if decisionErr != nil {
			return BootSessionConsumptionReconciliation{}, decisionErr
		}
		if observeErr != nil {
			return decision, fmt.Errorf("%w: durable observation is ambiguous: %v", ErrBootSessionConsumptionReconciliation, observeErr)
		}
		return decision, fmt.Errorf("%w: durable observation is ambiguous", ErrBootSessionConsumptionReconciliation)
	}

	switch observation.State {
	case BootSessionConsumptionReconciledConsumed:
		if observation.Existing == nil {
			return bootSessionConsumptionReconciliationAmbiguous(
				candidate, observedAtUnix, admission.ExpiresAtUnix, observedAtUnix >= admission.ExpiresAtUnix,
				"consumed observation omitted persisted receipt",
			)
		}
		existing := *observation.Existing
		existingRequest := BootSessionConsumptionRequest{
			AttemptID:      existing.AttemptID,
			ConsumerID:     existing.ConsumerID,
			ConsumedAtUnix: existing.ConsumedAtUnix,
		}
		if err := VerifyBootSessionConsumptionReceipt(existing, existingRequest, admission); err != nil {
			return bootSessionConsumptionReconciliationAmbiguous(
				candidate, observedAtUnix, admission.ExpiresAtUnix, observedAtUnix >= admission.ExpiresAtUnix,
				"persisted receipt no longer matches exact admission",
			)
		}
		return newBootSessionConsumptionReconciliation(
			BootSessionConsumptionReconciledConsumed,
			candidate,
			existing.ReceiptID,
			observedAtUnix,
			admission.ExpiresAtUnix,
			observedAtUnix >= admission.ExpiresAtUnix,
			true,
			true,
			false,
			"do-not-reuse-consumed-session",
		)
	case BootSessionConsumptionReconciledDefinitelyAbsent:
		if observation.Existing != nil {
			return bootSessionConsumptionReconciliationAmbiguous(
				candidate, observedAtUnix, admission.ExpiresAtUnix, observedAtUnix >= admission.ExpiresAtUnix,
				"absent observation unexpectedly included persisted receipt",
			)
		}
		oldAdmissionExpired := observedAtUnix >= admission.ExpiresAtUnix
		if !oldAdmissionExpired {
			return newBootSessionConsumptionReconciliation(
				BootSessionConsumptionReconciledDefinitelyAbsent,
				candidate,
				"",
				observedAtUnix,
				admission.ExpiresAtUnix,
				false,
				false,
				false,
				true,
				"wait-for-old-admission-expiry-before-reissuing-session",
			)
		}
		return newBootSessionConsumptionReconciliation(
			BootSessionConsumptionReconciledDefinitelyAbsent,
			candidate,
			"",
			observedAtUnix,
			admission.ExpiresAtUnix,
			true,
			true,
			true,
			true,
			"reissue-new-session-from-fresh-serving-evidence",
		)
	default:
		return bootSessionConsumptionReconciliationAmbiguous(
			candidate, observedAtUnix, admission.ExpiresAtUnix, observedAtUnix >= admission.ExpiresAtUnix,
			"unrecognized durable observation state",
		)
	}
}

func bootSessionConsumptionReconciliationAmbiguous(
	candidate BootSessionConsumptionReceipt,
	observedAtUnix int64,
	admissionExpiresAtUnix int64,
	oldAdmissionExpired bool,
	reason string,
) (BootSessionConsumptionReconciliation, error) {
	decision, err := newBootSessionConsumptionReconciliation(
		BootSessionConsumptionReconciledAmbiguous,
		candidate,
		"",
		observedAtUnix,
		admissionExpiresAtUnix,
		oldAdmissionExpired,
		false,
		false,
		true,
		"operator-reconcile-durable-consumption-store",
	)
	if err != nil {
		return BootSessionConsumptionReconciliation{}, err
	}
	return decision, fmt.Errorf("%w: %s", ErrBootSessionConsumptionReconciliation, reason)
}

func newBootSessionConsumptionReconciliation(
	state BootSessionConsumptionReconciliationState,
	candidate BootSessionConsumptionReceipt,
	observedReceiptID string,
	observedAtUnix int64,
	admissionExpiresAtUnix int64,
	oldAdmissionExpired bool,
	freshAdmissionRequired bool,
	freshServingEvidenceRequired bool,
	recoveryRequired bool,
	recoveryAction string,
) (BootSessionConsumptionReconciliation, error) {
	digestInput := bootSessionConsumptionReconciliationDigest{
		ReconciliationVersion:        bootSessionConsumptionReconciliationVersion,
		State:                        state,
		AdmissionID:                  candidate.AdmissionID,
		ReplayKey:                    candidate.ReplayKey,
		CandidateReceiptID:           candidate.ReceiptID,
		ObservedReceiptID:            observedReceiptID,
		ObservedAtUnix:               observedAtUnix,
		AdmissionExpiresAtUnix:       admissionExpiresAtUnix,
		OldAdmissionExpired:          oldAdmissionExpired,
		FreshAdmissionRequired:       freshAdmissionRequired,
		FreshServingEvidenceRequired: freshServingEvidenceRequired,
		RecoveryRequired:             recoveryRequired,
		RecoveryAction:               recoveryAction,
	}
	encoded, err := json.Marshal(digestInput)
	if err != nil {
		return BootSessionConsumptionReconciliation{},
			fmt.Errorf("%w: encode canonical reconciliation evidence: %v", ErrBootSessionConsumptionReconciliation, err)
	}
	digest := sha256.Sum256(encoded)

	decision := BootSessionConsumptionReconciliation{
		ReconciliationID:             hex.EncodeToString(digest[:]),
		ReconciliationVersion:        bootSessionConsumptionReconciliationVersion,
		State:                        state,
		AdmissionID:                  candidate.AdmissionID,
		ReplayKey:                    candidate.ReplayKey,
		CandidateReceiptID:           candidate.ReceiptID,
		ObservedReceiptID:            observedReceiptID,
		ObservedAtUnix:               observedAtUnix,
		AdmissionExpiresAtUnix:       admissionExpiresAtUnix,
		OldAdmissionExpired:          oldAdmissionExpired,
		OldAdmissionReusable:         false,
		FreshAdmissionRequired:       freshAdmissionRequired,
		FreshServingEvidenceRequired: freshServingEvidenceRequired,
		FreshAdmissionAuthorized:     false,
		BootHandoffAuthorized:        false,
		ReplayAuthorized:             false,
		ProvisioningAuthorized:       false,
		SecretInjectionAuthorized:    false,
		HostMutation:                 false,
		NetworkMutation:              false,
		ProductionMutation:           false,
		RecoveryRequired:             recoveryRequired,
		RecoveryAction:               recoveryAction,
	}
	if err := VerifyBootSessionConsumptionReconciliation(decision); err != nil {
		return BootSessionConsumptionReconciliation{}, err
	}
	return decision, nil
}

func VerifyBootSessionConsumptionReconciliation(
	decision BootSessionConsumptionReconciliation,
) error {
	if decision.ReconciliationVersion != bootSessionConsumptionReconciliationVersion ||
		!bootSessionConsumptionCanonicalSHA256(decision.ReconciliationID) ||
		!bootSessionConsumptionCanonicalSHA256(decision.AdmissionID) ||
		!bootSessionConsumptionCanonicalSHA256(decision.ReplayKey) ||
		!bootSessionConsumptionCanonicalSHA256(decision.CandidateReceiptID) {
		return fmt.Errorf("%w: invalid reconciliation identity or version", ErrBootSessionConsumptionReconciliation)
	}
	if decision.ObservedReceiptID != "" && !bootSessionConsumptionCanonicalSHA256(decision.ObservedReceiptID) {
		return fmt.Errorf("%w: observed receipt identity must be canonical SHA-256", ErrBootSessionConsumptionReconciliation)
	}
	if decision.ObservedAtUnix <= 0 || decision.AdmissionExpiresAtUnix <= 0 ||
		decision.OldAdmissionExpired != (decision.ObservedAtUnix >= decision.AdmissionExpiresAtUnix) {
		return fmt.Errorf("%w: inconsistent admission expiry evidence", ErrBootSessionConsumptionReconciliation)
	}
	if decision.OldAdmissionReusable || decision.FreshAdmissionAuthorized ||
		decision.BootHandoffAuthorized || decision.ReplayAuthorized || decision.ProvisioningAuthorized ||
		decision.SecretInjectionAuthorized || decision.HostMutation || decision.NetworkMutation ||
		decision.ProductionMutation {
		return fmt.Errorf("%w: reconciliation must not grant deployment authority", ErrBootSessionConsumptionReconciliation)
	}

	switch decision.State {
	case BootSessionConsumptionReconciledConsumed:
		if decision.ObservedReceiptID == "" || !decision.FreshAdmissionRequired ||
			!decision.FreshServingEvidenceRequired || decision.RecoveryRequired ||
			decision.RecoveryAction != "do-not-reuse-consumed-session" {
			return fmt.Errorf("%w: invalid consumed reconciliation semantics", ErrBootSessionConsumptionReconciliation)
		}
	case BootSessionConsumptionReconciledDefinitelyAbsent:
		if decision.ObservedReceiptID != "" || !decision.RecoveryRequired {
			return fmt.Errorf("%w: invalid absent reconciliation evidence", ErrBootSessionConsumptionReconciliation)
		}
		if decision.OldAdmissionExpired {
			if !decision.FreshAdmissionRequired || !decision.FreshServingEvidenceRequired ||
				decision.RecoveryAction != "reissue-new-session-from-fresh-serving-evidence" {
				return fmt.Errorf("%w: expired absent reconciliation must require fresh evidence", ErrBootSessionConsumptionReconciliation)
			}
		} else if decision.FreshAdmissionRequired || decision.FreshServingEvidenceRequired ||
			decision.RecoveryAction != "wait-for-old-admission-expiry-before-reissuing-session" {
			return fmt.Errorf("%w: unexpired absent reconciliation must wait for expiry", ErrBootSessionConsumptionReconciliation)
		}
	case BootSessionConsumptionReconciledAmbiguous:
		if decision.ObservedReceiptID != "" || decision.FreshAdmissionRequired ||
			decision.FreshServingEvidenceRequired || !decision.RecoveryRequired ||
			decision.RecoveryAction != "operator-reconcile-durable-consumption-store" {
			return fmt.Errorf("%w: ambiguous reconciliation must fail closed", ErrBootSessionConsumptionReconciliation)
		}
	default:
		return fmt.Errorf("%w: unknown reconciliation state", ErrBootSessionConsumptionReconciliation)
	}

	digestInput := bootSessionConsumptionReconciliationDigest{
		ReconciliationVersion:        decision.ReconciliationVersion,
		State:                        decision.State,
		AdmissionID:                  decision.AdmissionID,
		ReplayKey:                    decision.ReplayKey,
		CandidateReceiptID:           decision.CandidateReceiptID,
		ObservedReceiptID:            decision.ObservedReceiptID,
		ObservedAtUnix:               decision.ObservedAtUnix,
		AdmissionExpiresAtUnix:       decision.AdmissionExpiresAtUnix,
		OldAdmissionExpired:          decision.OldAdmissionExpired,
		FreshAdmissionRequired:       decision.FreshAdmissionRequired,
		FreshServingEvidenceRequired: decision.FreshServingEvidenceRequired,
		RecoveryRequired:             decision.RecoveryRequired,
		RecoveryAction:               decision.RecoveryAction,
	}
	encoded, err := json.Marshal(digestInput)
	if err != nil {
		return fmt.Errorf("%w: encode reconciliation verification evidence: %v", ErrBootSessionConsumptionReconciliation, err)
	}
	digest := sha256.Sum256(encoded)
	if decision.ReconciliationID != hex.EncodeToString(digest[:]) {
		return fmt.Errorf("%w: reconciliation digest drift", ErrBootSessionConsumptionReconciliation)
	}
	return nil
}
