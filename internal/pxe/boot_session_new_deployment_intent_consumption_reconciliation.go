package pxe

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
)

var ErrBootSessionNewDeploymentIntentConsumptionReconciliation = errors.New(
	"PXE new deployment intent consumption reconciliation failed",
)

const bootSessionNewDeploymentIntentConsumptionReconciliationVersion =
	"boot-session-new-deployment-intent-consumption-reconciliation-v1"

// BootSessionNewDeploymentIntentConsumptionEvidence is the exact immutable
// evidence needed to revalidate the new-intent boundary before observing its
// replay key. Callers cannot substitute a partial deployment lineage.
type BootSessionNewDeploymentIntentConsumptionEvidence struct {
	IntentReceipt          BootSessionNewDeploymentIntentReceipt
	NewAdmission           BootSessionAdmission
	Intent                 BootSessionNewDeploymentIntentRequest
	TerminalReconciliation BootSessionReadmissionConsumptionReconciliation
	TerminalConsumption    BootSessionReadmissionConsumptionReceipt
	Readmission            BootSessionReadmissionReceipt
	Previous               BootSessionAdmission
	PreviousCandidate      BootSessionConsumptionReceipt
	PreviousReconciliation BootSessionConsumptionReconciliation
	TerminalAdmission      BootSessionAdmission
	TerminalCandidate      BootSessionConsumptionReceipt
	ServingReceipt         InstallMediaServingReceipt
	Plan                   AdmittedDeploymentPlan
	CurrentMedia           []byte
	UnattendedBinding      *UnattendedTemplateBinding
	UnattendedTemplate     []byte
}

// BootSessionNewDeploymentIntentConsumptionReconciliation is evidence-only.
// Even definitely-absent does not revive the current admission/replay key.
type BootSessionNewDeploymentIntentConsumptionReconciliation struct {
	ReconciliationID              string                                    `json:"reconciliationId"`
	ReconciliationVersion         string                                    `json:"reconciliationVersion"`
	State                         BootSessionConsumptionReconciliationState `json:"state"`
	IntentReceiptID               string                                    `json:"intentReceiptId"`
	IntentID                      string                                    `json:"intentId"`
	AdmissionID                   string                                    `json:"admissionId"`
	ReplayKey                     string                                    `json:"replayKey"`
	CandidateConsumptionReceiptID string                                    `json:"candidateConsumptionReceiptId"`
	ObservedReceiptID             string                                    `json:"observedReceiptId,omitempty"`
	ObservedAtUnix                int64                                     `json:"observedAtUnix"`
	AdmissionExpiresAtUnix        int64                                     `json:"admissionExpiresAtUnix"`
	AdmissionExpired              bool                                      `json:"admissionExpired"`
	CurrentAdmissionReusable      bool                                      `json:"currentAdmissionReusable"`
	FreshAdmissionAuthorized      bool                                      `json:"freshAdmissionAuthorized"`
	FurtherReadmissionAuthorized  bool                                      `json:"furtherReadmissionAuthorized"`
	AutomaticRetryAuthorized      bool                                      `json:"automaticRetryAuthorized"`
	BootHandoffAuthorized         bool                                      `json:"bootHandoffAuthorized"`
	ProvisioningAuthorized        bool                                      `json:"provisioningAuthorized"`
	SecretInjectionAuthorized     bool                                      `json:"secretInjectionAuthorized"`
	HostMutation                  bool                                      `json:"hostMutation"`
	NetworkMutation               bool                                      `json:"networkMutation"`
	ProductionMutation            bool                                      `json:"productionMutation"`
	RecoveryRequired              bool                                      `json:"recoveryRequired"`
	RecoveryAction                string                                    `json:"recoveryAction"`
}

type bootSessionNewDeploymentIntentConsumptionReconciliationDigest struct {
	ReconciliationVersion         string                                    `json:"reconciliationVersion"`
	State                         BootSessionConsumptionReconciliationState `json:"state"`
	IntentReceiptID               string                                    `json:"intentReceiptId"`
	IntentID                      string                                    `json:"intentId"`
	AdmissionID                   string                                    `json:"admissionId"`
	ReplayKey                     string                                    `json:"replayKey"`
	CandidateConsumptionReceiptID string                                    `json:"candidateConsumptionReceiptId"`
	ObservedReceiptID             string                                    `json:"observedReceiptId,omitempty"`
	ObservedAtUnix                int64                                     `json:"observedAtUnix"`
	AdmissionExpiresAtUnix        int64                                     `json:"admissionExpiresAtUnix"`
	AdmissionExpired              bool                                      `json:"admissionExpired"`
	RecoveryRequired              bool                                      `json:"recoveryRequired"`
	RecoveryAction                string                                    `json:"recoveryAction"`
}

// ReconcileBootSessionNewDeploymentIntentConsumption resolves only a lost or
// ambiguous atomic-consume acknowledgement for the exact new-intent admission.
func ReconcileBootSessionNewDeploymentIntentConsumption(
	store BootSessionConsumptionReconciliationStore,
	source BootSessionNewDeploymentIntentConsumptionDecision,
	evidence BootSessionNewDeploymentIntentConsumptionEvidence,
	observedAtUnix int64,
) (BootSessionNewDeploymentIntentConsumptionReconciliation, error) {
	if store == nil {
		return BootSessionNewDeploymentIntentConsumptionReconciliation{}, fmt.Errorf(
			"%w: reconciliation store is required",
			ErrBootSessionNewDeploymentIntentConsumptionReconciliation,
		)
	}
	if err := verifyNewDeploymentIntentConsumptionEvidence(source, evidence); err != nil {
		return BootSessionNewDeploymentIntentConsumptionReconciliation{}, err
	}
	if observedAtUnix <= 0 || observedAtUnix < source.ConsumptionReceipt.ConsumedAtUnix {
		return BootSessionNewDeploymentIntentConsumptionReconciliation{}, fmt.Errorf(
			"%w: observation time precedes candidate consumption evidence",
			ErrBootSessionNewDeploymentIntentConsumptionReconciliation,
		)
	}

	observation, observeErr := store.ObserveBootSessionConsumption(evidence.NewAdmission.ReplayKey)
	if observeErr != nil || observation.State == BootSessionConsumptionReconciledAmbiguous {
		decision := newBootSessionNewDeploymentIntentConsumptionReconciliation(
			BootSessionConsumptionReconciledAmbiguous, source, evidence, "", observedAtUnix,
			true, "operator-reconcile-new-deployment-intent-consumption",
		)
		if observeErr != nil {
			return decision, fmt.Errorf(
				"%w: durable observation is ambiguous: %v",
				ErrBootSessionNewDeploymentIntentConsumptionReconciliation, observeErr,
			)
		}
		return decision, fmt.Errorf(
			"%w: durable observation is ambiguous",
			ErrBootSessionNewDeploymentIntentConsumptionReconciliation,
		)
	}

	switch observation.State {
	case BootSessionConsumptionReconciledConsumed:
		if observation.Existing == nil {
			return ambiguousNewDeploymentIntentConsumptionReconciliation(
				source, evidence, observedAtUnix, "consumed observation omitted persisted receipt",
			)
		}
		existing := *observation.Existing
		existingRequest := BootSessionConsumptionRequest{
			AttemptID: existing.AttemptID, ConsumerID: existing.ConsumerID,
			ConsumedAtUnix: existing.ConsumedAtUnix,
		}
		if err := VerifyBootSessionConsumptionReceipt(
			existing, existingRequest, evidence.NewAdmission,
		); err != nil || !reflect.DeepEqual(existing, source.ConsumptionReceipt) {
			return ambiguousNewDeploymentIntentConsumptionReconciliation(
				source, evidence, observedAtUnix,
				"persisted receipt does not prove the exact candidate consumption",
			)
		}
		return newBootSessionNewDeploymentIntentConsumptionReconciliation(
			BootSessionConsumptionReconciledConsumed, source, evidence, existing.ReceiptID,
			observedAtUnix, false, "close-new-deployment-intent-as-consumed",
		), nil

	case BootSessionConsumptionReconciledDefinitelyAbsent:
		if observation.Existing != nil {
			return ambiguousNewDeploymentIntentConsumptionReconciliation(
				source, evidence, observedAtUnix,
				"absent observation unexpectedly included persisted receipt",
			)
		}
		return newBootSessionNewDeploymentIntentConsumptionReconciliation(
			BootSessionConsumptionReconciledDefinitelyAbsent, source, evidence, "",
			observedAtUnix, true, "operator-close-or-start-separate-deployment-intent-boundary",
		), nil
	default:
		return ambiguousNewDeploymentIntentConsumptionReconciliation(
			source, evidence, observedAtUnix, "unrecognized durable observation state",
		)
	}
}

func VerifyBootSessionNewDeploymentIntentConsumptionReconciliation(
	decision BootSessionNewDeploymentIntentConsumptionReconciliation,
	source BootSessionNewDeploymentIntentConsumptionDecision,
	evidence BootSessionNewDeploymentIntentConsumptionEvidence,
) error {
	if err := verifyNewDeploymentIntentConsumptionEvidence(source, evidence); err != nil {
		return err
	}
	if decision.ReconciliationVersion !=
		bootSessionNewDeploymentIntentConsumptionReconciliationVersion ||
		!bootSessionConsumptionCanonicalSHA256(decision.ReconciliationID) ||
		!bootSessionConsumptionCanonicalSHA256(decision.IntentReceiptID) ||
		!bootSessionConsumptionCanonicalSHA256(decision.AdmissionID) ||
		!bootSessionConsumptionCanonicalSHA256(decision.ReplayKey) ||
		!bootSessionConsumptionCanonicalSHA256(decision.CandidateConsumptionReceiptID) ||
		decision.ObservedAtUnix <= 0 ||
		decision.ObservedAtUnix < source.ConsumptionReceipt.ConsumedAtUnix ||
		decision.AdmissionExpiresAtUnix != evidence.NewAdmission.ExpiresAtUnix ||
		decision.AdmissionExpired !=
			(decision.ObservedAtUnix >= evidence.NewAdmission.ExpiresAtUnix) {
		return fmt.Errorf(
			"%w: reconciliation identity or timing evidence drift",
			ErrBootSessionNewDeploymentIntentConsumptionReconciliation,
		)
	}
	if decision.ObservedReceiptID != "" &&
		!bootSessionConsumptionCanonicalSHA256(decision.ObservedReceiptID) {
		return fmt.Errorf(
			"%w: observed receipt identity must be canonical SHA-256",
			ErrBootSessionNewDeploymentIntentConsumptionReconciliation,
		)
	}
	if decision.IntentReceiptID != evidence.IntentReceipt.ReceiptID ||
		decision.IntentID != evidence.IntentReceipt.IntentID ||
		decision.AdmissionID != evidence.NewAdmission.AdmissionID ||
		decision.ReplayKey != evidence.NewAdmission.ReplayKey ||
		decision.CandidateConsumptionReceiptID != source.ConsumptionReceipt.ReceiptID {
		return fmt.Errorf(
			"%w: new deployment intent consumption lineage drift",
			ErrBootSessionNewDeploymentIntentConsumptionReconciliation,
		)
	}
	if decision.CurrentAdmissionReusable || decision.FreshAdmissionAuthorized ||
		decision.FurtherReadmissionAuthorized || decision.AutomaticRetryAuthorized ||
		decision.BootHandoffAuthorized || decision.ProvisioningAuthorized ||
		decision.SecretInjectionAuthorized || decision.HostMutation ||
		decision.NetworkMutation || decision.ProductionMutation {
		return fmt.Errorf(
			"%w: reconciliation must not grant deployment authority",
			ErrBootSessionNewDeploymentIntentConsumptionReconciliation,
		)
	}

	switch decision.State {
	case BootSessionConsumptionReconciledConsumed:
		if decision.ObservedReceiptID != source.ConsumptionReceipt.ReceiptID ||
			decision.RecoveryRequired ||
			decision.RecoveryAction != "close-new-deployment-intent-as-consumed" {
			return fmt.Errorf(
				"%w: invalid consumed reconciliation semantics",
				ErrBootSessionNewDeploymentIntentConsumptionReconciliation,
			)
		}
	case BootSessionConsumptionReconciledDefinitelyAbsent:
		if decision.ObservedReceiptID != "" || !decision.RecoveryRequired ||
			decision.RecoveryAction !=
				"operator-close-or-start-separate-deployment-intent-boundary" {
			return fmt.Errorf(
				"%w: absent result must require a separate operator boundary",
				ErrBootSessionNewDeploymentIntentConsumptionReconciliation,
			)
		}
	case BootSessionConsumptionReconciledAmbiguous:
		if decision.ObservedReceiptID != "" || !decision.RecoveryRequired ||
			decision.RecoveryAction != "operator-reconcile-new-deployment-intent-consumption" {
			return fmt.Errorf(
				"%w: ambiguous reconciliation must fail closed",
				ErrBootSessionNewDeploymentIntentConsumptionReconciliation,
			)
		}
	default:
		return fmt.Errorf(
			"%w: unknown reconciliation state",
			ErrBootSessionNewDeploymentIntentConsumptionReconciliation,
		)
	}

	expected := newBootSessionNewDeploymentIntentConsumptionReconciliation(
		decision.State, source, evidence, decision.ObservedReceiptID, decision.ObservedAtUnix,
		decision.RecoveryRequired, decision.RecoveryAction,
	)
	if !reflect.DeepEqual(expected, decision) {
		return fmt.Errorf(
			"%w: reconciliation digest or semantics drift",
			ErrBootSessionNewDeploymentIntentConsumptionReconciliation,
		)
	}
	return nil
}

func verifyNewDeploymentIntentConsumptionEvidence(
	source BootSessionNewDeploymentIntentConsumptionDecision,
	evidence BootSessionNewDeploymentIntentConsumptionEvidence,
) error {
	if err := VerifyBootSessionNewDeploymentIntentReceipt(
		evidence.IntentReceipt, evidence.NewAdmission, evidence.Intent,
		evidence.TerminalReconciliation, evidence.TerminalConsumption, evidence.Readmission,
		evidence.Previous, evidence.PreviousCandidate, evidence.PreviousReconciliation,
		evidence.TerminalAdmission, evidence.TerminalCandidate, evidence.ServingReceipt,
		evidence.Plan, evidence.CurrentMedia, evidence.UnattendedBinding, evidence.UnattendedTemplate,
	); err != nil {
		return fmt.Errorf(
			"%w: new deployment intent evidence: %v",
			ErrBootSessionNewDeploymentIntentConsumptionReconciliation, err,
		)
	}
	if source.Status != BootSessionConsumptionAmbiguous || source.AtomicConsumeVerified ||
		source.BootHandoffAuthorized || source.ReplayAuthorized ||
		source.FurtherReadmissionAuthorized || source.AutomaticRetryAuthorized ||
		source.ProvisioningAuthorized || source.SecretInjectionAuthorized ||
		source.HostMutation || source.NetworkMutation || source.ProductionMutation ||
		!source.RecoveryRequired || source.RecoveryAction !=
			"reconcile-new-deployment-intent-replay-key-before-any-further-admission" ||
		source.IntentReceiptID != evidence.IntentReceipt.ReceiptID ||
		source.IntentID != evidence.IntentReceipt.IntentID ||
		source.AdmissionID != evidence.NewAdmission.AdmissionID ||
		source.ReplayKey != evidence.NewAdmission.ReplayKey {
		return fmt.Errorf(
			"%w: source is not the exact fail-closed ambiguous consumption",
			ErrBootSessionNewDeploymentIntentConsumptionReconciliation,
		)
	}
	request := BootSessionConsumptionRequest{
		AttemptID: source.ConsumptionReceipt.AttemptID,
		ConsumerID: source.ConsumptionReceipt.ConsumerID,
		ConsumedAtUnix: source.ConsumptionReceipt.ConsumedAtUnix,
	}
	if err := VerifyBootSessionConsumptionReceipt(
		source.ConsumptionReceipt, request, evidence.NewAdmission,
	); err != nil {
		return fmt.Errorf(
			"%w: candidate consumption evidence: %v",
			ErrBootSessionNewDeploymentIntentConsumptionReconciliation, err,
		)
	}
	if err := validateNewDeploymentIntentConsumptionLineage(
		BootSessionConsumptionDecision{Receipt: source.ConsumptionReceipt},
		evidence.IntentReceipt,
		evidence.NewAdmission,
	); err != nil {
		return fmt.Errorf(
			"%w: candidate lineage: %v",
			ErrBootSessionNewDeploymentIntentConsumptionReconciliation, err,
		)
	}
	return nil
}

func ambiguousNewDeploymentIntentConsumptionReconciliation(
	source BootSessionNewDeploymentIntentConsumptionDecision,
	evidence BootSessionNewDeploymentIntentConsumptionEvidence,
	observedAtUnix int64,
	reason string,
) (BootSessionNewDeploymentIntentConsumptionReconciliation, error) {
	decision := newBootSessionNewDeploymentIntentConsumptionReconciliation(
		BootSessionConsumptionReconciledAmbiguous, source, evidence, "", observedAtUnix,
		true, "operator-reconcile-new-deployment-intent-consumption",
	)
	return decision, fmt.Errorf(
		"%w: %s", ErrBootSessionNewDeploymentIntentConsumptionReconciliation, reason,
	)
}

func newBootSessionNewDeploymentIntentConsumptionReconciliation(
	state BootSessionConsumptionReconciliationState,
	source BootSessionNewDeploymentIntentConsumptionDecision,
	evidence BootSessionNewDeploymentIntentConsumptionEvidence,
	observedReceiptID string,
	observedAtUnix int64,
	recoveryRequired bool,
	recoveryAction string,
) BootSessionNewDeploymentIntentConsumptionReconciliation {
	expiresAt := evidence.NewAdmission.ExpiresAtUnix
	digestInput := bootSessionNewDeploymentIntentConsumptionReconciliationDigest{
		ReconciliationVersion: bootSessionNewDeploymentIntentConsumptionReconciliationVersion,
		State: state, IntentReceiptID: evidence.IntentReceipt.ReceiptID,
		IntentID: evidence.IntentReceipt.IntentID, AdmissionID: evidence.NewAdmission.AdmissionID,
		ReplayKey: evidence.NewAdmission.ReplayKey,
		CandidateConsumptionReceiptID: source.ConsumptionReceipt.ReceiptID,
		ObservedReceiptID: observedReceiptID, ObservedAtUnix: observedAtUnix,
		AdmissionExpiresAtUnix: expiresAt, AdmissionExpired: observedAtUnix >= expiresAt,
		RecoveryRequired: recoveryRequired, RecoveryAction: recoveryAction,
	}
	encoded, _ := json.Marshal(digestInput)
	digest := sha256.Sum256(encoded)
	return BootSessionNewDeploymentIntentConsumptionReconciliation{
		ReconciliationID: hex.EncodeToString(digest[:]),
		ReconciliationVersion: bootSessionNewDeploymentIntentConsumptionReconciliationVersion,
		State: state, IntentReceiptID: evidence.IntentReceipt.ReceiptID,
		IntentID: evidence.IntentReceipt.IntentID, AdmissionID: evidence.NewAdmission.AdmissionID,
		ReplayKey: evidence.NewAdmission.ReplayKey,
		CandidateConsumptionReceiptID: source.ConsumptionReceipt.ReceiptID,
		ObservedReceiptID: observedReceiptID, ObservedAtUnix: observedAtUnix,
		AdmissionExpiresAtUnix: expiresAt, AdmissionExpired: observedAtUnix >= expiresAt,
		CurrentAdmissionReusable: false, FreshAdmissionAuthorized: false,
		FurtherReadmissionAuthorized: false, AutomaticRetryAuthorized: false,
		BootHandoffAuthorized: false, ProvisioningAuthorized: false,
		SecretInjectionAuthorized: false, HostMutation: false, NetworkMutation: false,
		ProductionMutation: false, RecoveryRequired: recoveryRequired, RecoveryAction: recoveryAction,
	}
}
