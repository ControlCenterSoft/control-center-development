package pxe

import (
	"errors"
	"fmt"
)

var ErrBootSessionNewDeploymentIntentConsumption = errors.New(
	"PXE new deployment intent consumption validation failed",
)

// BootSessionNewDeploymentIntentConsumptionDecision is an ephemeral decision
// returned only after the new deployment intent boundary and its boot-session
// admission have both been revalidated. It must not be persisted or replayed as
// a capability: only the atomic committed result authorizes one boot handoff.
type BootSessionNewDeploymentIntentConsumptionDecision struct {
	Status                       BootSessionConsumptionStoreState `json:"status"`
	IntentReceiptID              string                           `json:"intentReceiptId"`
	IntentID                     string                           `json:"intentId"`
	AdmissionID                  string                           `json:"admissionId"`
	ReplayKey                    string                           `json:"replayKey"`
	ConsumptionReceipt           BootSessionConsumptionReceipt    `json:"consumptionReceipt"`
	AtomicConsumeVerified        bool                             `json:"atomicConsumeVerified"`
	BootHandoffAuthorized        bool                             `json:"bootHandoffAuthorized"`
	ReplayAuthorized             bool                             `json:"replayAuthorized"`
	FurtherReadmissionAuthorized bool                             `json:"furtherReadmissionAuthorized"`
	AutomaticRetryAuthorized     bool                             `json:"automaticRetryAuthorized"`
	ProvisioningAuthorized       bool                             `json:"provisioningAuthorized"`
	SecretInjectionAuthorized    bool                             `json:"secretInjectionAuthorized"`
	HostMutation                 bool                             `json:"hostMutation"`
	NetworkMutation              bool                             `json:"networkMutation"`
	ProductionMutation           bool                             `json:"productionMutation"`
	RecoveryRequired             bool                             `json:"recoveryRequired"`
	RecoveryAction               string                           `json:"recoveryAction"`
}

// ConsumeBootSessionNewDeploymentIntentAdmission binds the separately approved
// new deployment intent to the exact single-use admission that may be consumed.
// The existing atomic replay-key store remains authoritative for consumption.
// Lost acknowledgement is reconciliation-only: this function never creates a
// readmission, retry, provisioning, secret-injection, or mutation authority.
func ConsumeBootSessionNewDeploymentIntentAdmission(
	store BootSessionConsumptionStore,
	consumeRequest BootSessionConsumptionRequest,
	intentReceipt BootSessionNewDeploymentIntentReceipt,
	newAdmission BootSessionAdmission,
	intent BootSessionNewDeploymentIntentRequest,
	terminalReconciliation BootSessionReadmissionConsumptionReconciliation,
	terminalConsumption BootSessionReadmissionConsumptionReceipt,
	readmission BootSessionReadmissionReceipt,
	previous BootSessionAdmission,
	previousCandidate BootSessionConsumptionReceipt,
	previousReconciliation BootSessionConsumptionReconciliation,
	terminalAdmission BootSessionAdmission,
	terminalCandidate BootSessionConsumptionReceipt,
	servingReceipt InstallMediaServingReceipt,
	plan AdmittedDeploymentPlan,
	currentServedMediaPayload []byte,
	unattendedBinding *UnattendedTemplateBinding,
	unattendedTemplate []byte,
) (BootSessionNewDeploymentIntentConsumptionDecision, error) {
	if err := VerifyBootSessionNewDeploymentIntentReceipt(
		intentReceipt,
		newAdmission,
		intent,
		terminalReconciliation,
		terminalConsumption,
		readmission,
		previous,
		previousCandidate,
		previousReconciliation,
		terminalAdmission,
		terminalCandidate,
		servingReceipt,
		plan,
		currentServedMediaPayload,
		unattendedBinding,
		unattendedTemplate,
	); err != nil {
		return BootSessionNewDeploymentIntentConsumptionDecision{}, fmt.Errorf(
			"%w: deployment intent boundary: %v",
			ErrBootSessionNewDeploymentIntentConsumption,
			err,
		)
	}
	if intentReceipt.NewAdmissionID != newAdmission.AdmissionID ||
		intentReceipt.NewReplayKey != newAdmission.ReplayKey ||
		intentReceipt.RequestID != newAdmission.RequestID ||
		intentReceipt.MachineID != newAdmission.MachineID ||
		intentReceipt.RecoveryAction != "consume-new-deployment-admission-once-or-reconcile" {
		return BootSessionNewDeploymentIntentConsumptionDecision{}, fmt.Errorf(
			"%w: intent/admission lineage does not permit atomic consumption",
			ErrBootSessionNewDeploymentIntentConsumption,
		)
	}

	decision, err := ConsumeBootSessionAdmission(
		store,
		consumeRequest,
		newAdmission,
		intent.BootRequest,
		servingReceipt,
		plan,
		currentServedMediaPayload,
		unattendedBinding,
		unattendedTemplate,
	)
	if err != nil {
		if errors.Is(err, ErrBootSessionConsumptionAmbiguous) {
			if lineageErr := validateNewDeploymentIntentConsumptionLineage(
				decision,
				intentReceipt,
				newAdmission,
			); lineageErr != nil {
				return BootSessionNewDeploymentIntentConsumptionDecision{}, lineageErr
			}
			return newBootSessionNewDeploymentIntentConsumptionDecision(
					intentReceipt,
					decision,
					false,
					true,
					"reconcile-new-deployment-intent-replay-key-before-any-further-admission",
				), fmt.Errorf(
					"%w: %w",
					ErrBootSessionNewDeploymentIntentConsumption,
					err,
				)
		}
		return BootSessionNewDeploymentIntentConsumptionDecision{}, fmt.Errorf(
			"%w: atomic boot-session consumption: %v",
			ErrBootSessionNewDeploymentIntentConsumption,
			err,
		)
	}
	if err := validateNewDeploymentIntentConsumptionLineage(
		decision,
		intentReceipt,
		newAdmission,
	); err != nil {
		return BootSessionNewDeploymentIntentConsumptionDecision{}, err
	}

	switch decision.Status {
	case BootSessionConsumptionCommitted:
		if !decision.AtomicConsumeVerified || !decision.BootHandoffAuthorized ||
			decision.ReplayAuthorized || decision.RecoveryRequired {
			return BootSessionNewDeploymentIntentConsumptionDecision{}, fmt.Errorf(
				"%w: committed consumption did not prove one atomic boot handoff",
				ErrBootSessionNewDeploymentIntentConsumption,
			)
		}
		return newBootSessionNewDeploymentIntentConsumptionDecision(
			intentReceipt,
			decision,
			true,
			false,
			"none",
		), nil
	case BootSessionConsumptionAlreadyConsumed:
		if !decision.AtomicConsumeVerified || decision.BootHandoffAuthorized ||
			decision.ReplayAuthorized || decision.RecoveryRequired {
			return BootSessionNewDeploymentIntentConsumptionDecision{}, fmt.Errorf(
				"%w: already-consumed replay unexpectedly carries authority",
				ErrBootSessionNewDeploymentIntentConsumption,
			)
		}
		return newBootSessionNewDeploymentIntentConsumptionDecision(
			intentReceipt,
			decision,
			false,
			false,
			"none-already-consumed",
		), nil
	default:
		return BootSessionNewDeploymentIntentConsumptionDecision{}, fmt.Errorf(
			"%w: unsupported atomic consumption state %q",
			ErrBootSessionNewDeploymentIntentConsumption,
			decision.Status,
		)
	}
}

func validateNewDeploymentIntentConsumptionLineage(
	decision BootSessionConsumptionDecision,
	intentReceipt BootSessionNewDeploymentIntentReceipt,
	newAdmission BootSessionAdmission,
) error {
	receipt := decision.Receipt
	if receipt.AdmissionID != newAdmission.AdmissionID ||
		receipt.ReplayKey != newAdmission.ReplayKey ||
		receipt.RequestID != newAdmission.RequestID ||
		receipt.MachineID != newAdmission.MachineID ||
		receipt.PlanID != newAdmission.PlanID ||
		receipt.ServingReceiptID != newAdmission.ServingReceiptID ||
		receipt.MediaSHA256 != newAdmission.MediaSHA256 ||
		receipt.MediaSize != newAdmission.MediaSize ||
		receipt.Target != newAdmission.Target ||
		intentReceipt.NewAdmissionID != receipt.AdmissionID ||
		intentReceipt.NewReplayKey != receipt.ReplayKey {
		return fmt.Errorf(
			"%w: atomic consumption receipt drifted from new deployment intent lineage",
			ErrBootSessionNewDeploymentIntentConsumption,
		)
	}
	return nil
}

func newBootSessionNewDeploymentIntentConsumptionDecision(
	intentReceipt BootSessionNewDeploymentIntentReceipt,
	decision BootSessionConsumptionDecision,
	bootHandoffAuthorized bool,
	recoveryRequired bool,
	recoveryAction string,
) BootSessionNewDeploymentIntentConsumptionDecision {
	return BootSessionNewDeploymentIntentConsumptionDecision{
		Status:                       decision.Status,
		IntentReceiptID:              intentReceipt.ReceiptID,
		IntentID:                     intentReceipt.IntentID,
		AdmissionID:                  intentReceipt.NewAdmissionID,
		ReplayKey:                    intentReceipt.NewReplayKey,
		ConsumptionReceipt:           decision.Receipt,
		AtomicConsumeVerified:        decision.AtomicConsumeVerified,
		BootHandoffAuthorized:        bootHandoffAuthorized,
		ReplayAuthorized:             false,
		FurtherReadmissionAuthorized: false,
		AutomaticRetryAuthorized:     false,
		ProvisioningAuthorized:       false,
		SecretInjectionAuthorized:    false,
		HostMutation:                 false,
		NetworkMutation:              false,
		ProductionMutation:           false,
		RecoveryRequired:             recoveryRequired,
		RecoveryAction:               recoveryAction,
	}
}
