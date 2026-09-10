package pxe

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
)

var ErrBootSessionReadmissionConsumption = errors.New("PXE boot session readmission consumption validation failed")

const (
	bootSessionReadmissionConsumptionReceiptVersion = "boot-session-readmission-consumption-receipt-v1"
	bootSessionReadmissionRecoveryGeneration        = 1
	bootSessionReadmissionMaxRecoveryGenerations    = 1
)

// BootSessionReadmissionConsumptionReceipt seals the one permitted recovery
// generation at the point where its fresh admission is atomically consumed.
// The receipt is evidence-only: it never authorizes another handoff, retry,
// readmission, provisioning action, or host/network mutation.
type BootSessionReadmissionConsumptionReceipt struct {
	ReceiptID                    string                           `json:"receiptId"`
	ReceiptVersion               string                           `json:"receiptVersion"`
	RecoveryGeneration           int                              `json:"recoveryGeneration"`
	MaxRecoveryGenerations       int                              `json:"maxRecoveryGenerations"`
	ReadmissionReceiptID         string                           `json:"readmissionReceiptId"`
	ReconciliationID             string                           `json:"reconciliationId"`
	PreviousAdmissionID          string                           `json:"previousAdmissionId"`
	PreviousReplayKey            string                           `json:"previousReplayKey"`
	FreshAdmissionID             string                           `json:"freshAdmissionId"`
	FreshReplayKey               string                           `json:"freshReplayKey"`
	ConsumptionReceiptID         string                           `json:"consumptionReceiptId"`
	MachineID                    string                           `json:"machineId"`
	PlanID                       string                           `json:"planId"`
	ServingReceiptID             string                           `json:"servingReceiptId"`
	MediaSHA256                  string                           `json:"mediaSha256"`
	MediaSize                    int64                            `json:"mediaSize"`
	Target                       InstallMediaPublicationTarget    `json:"target"`
	AttemptID                    string                           `json:"attemptId"`
	ConsumerID                   string                           `json:"consumerId"`
	ConsumedAtUnix               int64                            `json:"consumedAtUnix"`
	Status                       BootSessionConsumptionStoreState `json:"status"`
	AtomicConsumeVerified        bool                             `json:"atomicConsumeVerified"`
	BootHandoffConsumed          bool                             `json:"bootHandoffConsumed"`
	BootHandoffAuthorized        bool                             `json:"bootHandoffAuthorized"`
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

type bootSessionReadmissionConsumptionReceiptDigest struct {
	ReceiptVersion         string                           `json:"receiptVersion"`
	RecoveryGeneration     int                              `json:"recoveryGeneration"`
	MaxRecoveryGenerations int                              `json:"maxRecoveryGenerations"`
	ReadmissionReceiptID   string                           `json:"readmissionReceiptId"`
	ReconciliationID       string                           `json:"reconciliationId"`
	PreviousAdmissionID    string                           `json:"previousAdmissionId"`
	PreviousReplayKey      string                           `json:"previousReplayKey"`
	FreshAdmissionID       string                           `json:"freshAdmissionId"`
	FreshReplayKey         string                           `json:"freshReplayKey"`
	ConsumptionReceiptID   string                           `json:"consumptionReceiptId"`
	MachineID              string                           `json:"machineId"`
	PlanID                 string                           `json:"planId"`
	ServingReceiptID       string                           `json:"servingReceiptId"`
	MediaSHA256            string                           `json:"mediaSha256"`
	MediaSize              int64                            `json:"mediaSize"`
	Target                 InstallMediaPublicationTarget    `json:"target"`
	AttemptID              string                           `json:"attemptId"`
	ConsumerID             string                           `json:"consumerId"`
	ConsumedAtUnix         int64                            `json:"consumedAtUnix"`
	Status                 BootSessionConsumptionStoreState `json:"status"`
	RecoveryRequired       bool                             `json:"recoveryRequired"`
	RecoveryAction         string                           `json:"recoveryAction"`
}

// BootSessionReadmissionConsumptionDecision carries the ephemeral result of
// the atomic consume. Only a newly committed consume may authorize this one
// boot handoff. The immutable receipt embedded in the decision never does.
type BootSessionReadmissionConsumptionDecision struct {
	Status                       BootSessionConsumptionStoreState         `json:"status"`
	Receipt                      BootSessionReadmissionConsumptionReceipt `json:"receipt"`
	ConsumptionReceipt           BootSessionConsumptionReceipt            `json:"consumptionReceipt"`
	AtomicConsumeVerified        bool                                     `json:"atomicConsumeVerified"`
	BootHandoffAuthorized        bool                                     `json:"bootHandoffAuthorized"`
	FurtherReadmissionAuthorized bool                                     `json:"furtherReadmissionAuthorized"`
	AutomaticRetryAuthorized     bool                                     `json:"automaticRetryAuthorized"`
	RecoveryRequired             bool                                     `json:"recoveryRequired"`
	RecoveryAction               string                                   `json:"recoveryAction"`
}

// ConsumeBootSessionReadmission consumes exactly the fresh admission sealed by
// one readmission receipt. It deliberately terminates automatic recovery at
// generation one: ambiguous durability requires operator reconciliation and
// never becomes authority to issue another admission automatically.
func ConsumeBootSessionReadmission(
	store BootSessionConsumptionStore,
	request BootSessionConsumptionRequest,
	previous BootSessionAdmission,
	candidate BootSessionConsumptionReceipt,
	reconciliation BootSessionConsumptionReconciliation,
	fresh BootSessionAdmission,
	readmission BootSessionReadmissionReceipt,
	sessionRequest BootSessionRequest,
	servingReceipt InstallMediaServingReceipt,
	plan AdmittedDeploymentPlan,
	currentServedMediaPayload []byte,
	unattendedBinding *UnattendedTemplateBinding,
	unattendedTemplate []byte,
) (BootSessionReadmissionConsumptionDecision, error) {
	if store == nil {
		return BootSessionReadmissionConsumptionDecision{},
			fmt.Errorf("%w: atomic consumption store is required", ErrBootSessionReadmissionConsumption)
	}
	if err := VerifyBootSessionReadmissionReceipt(
		readmission,
		previous,
		candidate,
		reconciliation,
		fresh,
	); err != nil {
		return BootSessionReadmissionConsumptionDecision{},
			fmt.Errorf("%w: readmission lineage: %v", ErrBootSessionReadmissionConsumption, err)
	}
	if sessionRequest.RequestID != fresh.RequestID || sessionRequest.MachineID != fresh.MachineID ||
		sessionRequest.TTLSeconds != fresh.ExpiresAtUnix-fresh.IssuedAtUnix {
		return BootSessionReadmissionConsumptionDecision{},
			fmt.Errorf("%w: session request is not the exact fresh admission request", ErrBootSessionReadmissionConsumption)
	}
	if err := VerifyBootSessionAdmission(
		fresh,
		sessionRequest,
		servingReceipt,
		plan,
		currentServedMediaPayload,
		unattendedBinding,
		unattendedTemplate,
		request.ConsumedAtUnix,
	); err != nil {
		return BootSessionReadmissionConsumptionDecision{},
			fmt.Errorf("%w: fresh admission revalidation: %v", ErrBootSessionReadmissionConsumption, err)
	}

	expected, err := BuildBootSessionConsumptionReceipt(request, fresh)
	if err != nil {
		return BootSessionReadmissionConsumptionDecision{},
			fmt.Errorf("%w: build fresh consumption evidence: %v", ErrBootSessionReadmissionConsumption, err)
	}
	result, consumeErr := store.AtomicConsumeBootSession(expected)
	if consumeErr != nil || result.State == BootSessionConsumptionAmbiguous {
		return ambiguousBootSessionReadmissionConsumption(readmission, fresh, expected, consumeErr)
	}

	switch result.State {
	case BootSessionConsumptionCommitted:
		if result.Existing != nil {
			return ambiguousBootSessionReadmissionConsumption(
				readmission,
				fresh,
				expected,
				fmt.Errorf("committed result unexpectedly returned existing evidence"),
			)
		}
		receipt, err := buildBootSessionReadmissionConsumptionReceipt(
			readmission,
			fresh,
			expected,
			BootSessionConsumptionCommitted,
		)
		if err != nil {
			return BootSessionReadmissionConsumptionDecision{}, err
		}
		return BootSessionReadmissionConsumptionDecision{
			Status:                       BootSessionConsumptionCommitted,
			Receipt:                      receipt,
			ConsumptionReceipt:           expected,
			AtomicConsumeVerified:        true,
			BootHandoffAuthorized:        true,
			FurtherReadmissionAuthorized: false,
			AutomaticRetryAuthorized:     false,
			RecoveryRequired:             false,
			RecoveryAction:               receipt.RecoveryAction,
		}, nil

	case BootSessionConsumptionAlreadyConsumed:
		if result.Existing == nil {
			return ambiguousBootSessionReadmissionConsumption(
				readmission,
				fresh,
				expected,
				fmt.Errorf("already-consumed result omitted persisted evidence"),
			)
		}
		existingRequest := BootSessionConsumptionRequest{
			AttemptID:      result.Existing.AttemptID,
			ConsumerID:     result.Existing.ConsumerID,
			ConsumedAtUnix: result.Existing.ConsumedAtUnix,
		}
		if verifyErr := VerifyBootSessionConsumptionReceipt(*result.Existing, existingRequest, fresh); verifyErr != nil ||
			!reflect.DeepEqual(expected, *result.Existing) {
			return ambiguousBootSessionReadmissionConsumption(
				readmission,
				fresh,
				expected,
				fmt.Errorf("persisted consumption does not match exact fresh admission"),
			)
		}
		receipt, err := buildBootSessionReadmissionConsumptionReceipt(
			readmission,
			fresh,
			*result.Existing,
			BootSessionConsumptionAlreadyConsumed,
		)
		if err != nil {
			return BootSessionReadmissionConsumptionDecision{}, err
		}
		return BootSessionReadmissionConsumptionDecision{
			Status:                       BootSessionConsumptionAlreadyConsumed,
			Receipt:                      receipt,
			ConsumptionReceipt:           *result.Existing,
			AtomicConsumeVerified:        true,
			BootHandoffAuthorized:        false,
			FurtherReadmissionAuthorized: false,
			AutomaticRetryAuthorized:     false,
			RecoveryRequired:             false,
			RecoveryAction:               receipt.RecoveryAction,
		}, nil

	default:
		return ambiguousBootSessionReadmissionConsumption(
			readmission,
			fresh,
			expected,
			fmt.Errorf("unrecognized atomic consume state %q", result.State),
		)
	}
}

func VerifyBootSessionReadmissionConsumptionReceipt(
	receipt BootSessionReadmissionConsumptionReceipt,
	readmission BootSessionReadmissionReceipt,
	previous BootSessionAdmission,
	candidate BootSessionConsumptionReceipt,
	reconciliation BootSessionConsumptionReconciliation,
	fresh BootSessionAdmission,
	consumption BootSessionConsumptionReceipt,
) error {
	if err := VerifyBootSessionReadmissionReceipt(
		readmission,
		previous,
		candidate,
		reconciliation,
		fresh,
	); err != nil {
		return fmt.Errorf("%w: readmission lineage: %v", ErrBootSessionReadmissionConsumption, err)
	}
	if receipt.ReceiptVersion != bootSessionReadmissionConsumptionReceiptVersion ||
		receipt.RecoveryGeneration != bootSessionReadmissionRecoveryGeneration ||
		receipt.MaxRecoveryGenerations != bootSessionReadmissionMaxRecoveryGenerations ||
		receipt.BootHandoffAuthorized || receipt.FurtherReadmissionAuthorized ||
		receipt.AutomaticRetryAuthorized || receipt.ProvisioningAuthorized ||
		receipt.SecretInjectionAuthorized || receipt.HostMutation || receipt.NetworkMutation ||
		receipt.ProductionMutation || !bootSessionConsumptionCanonicalSHA256(receipt.ReceiptID) {
		return fmt.Errorf("%w: terminal recovery safety contract drift", ErrBootSessionReadmissionConsumption)
	}
	rebuilt, err := buildBootSessionReadmissionConsumptionReceipt(
		readmission,
		fresh,
		consumption,
		receipt.Status,
	)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(rebuilt, receipt) {
		return fmt.Errorf("%w: consumption receipt lineage drift", ErrBootSessionReadmissionConsumption)
	}
	return nil
}

func ambiguousBootSessionReadmissionConsumption(
	readmission BootSessionReadmissionReceipt,
	fresh BootSessionAdmission,
	consumption BootSessionConsumptionReceipt,
	cause error,
) (BootSessionReadmissionConsumptionDecision, error) {
	receipt, err := buildBootSessionReadmissionConsumptionReceipt(
		readmission,
		fresh,
		consumption,
		BootSessionConsumptionAmbiguous,
	)
	if err != nil {
		return BootSessionReadmissionConsumptionDecision{}, err
	}
	if cause == nil {
		cause = ErrBootSessionConsumptionAmbiguous
	}
	return BootSessionReadmissionConsumptionDecision{
		Status:                       BootSessionConsumptionAmbiguous,
		Receipt:                      receipt,
		ConsumptionReceipt:           consumption,
		AtomicConsumeVerified:        false,
		BootHandoffAuthorized:        false,
		FurtherReadmissionAuthorized: false,
		AutomaticRetryAuthorized:     false,
		RecoveryRequired:             true,
		RecoveryAction:               receipt.RecoveryAction,
	}, fmt.Errorf("%w: %v", ErrBootSessionReadmissionConsumption, cause)
}

func buildBootSessionReadmissionConsumptionReceipt(
	readmission BootSessionReadmissionReceipt,
	fresh BootSessionAdmission,
	consumption BootSessionConsumptionReceipt,
	status BootSessionConsumptionStoreState,
) (BootSessionReadmissionConsumptionReceipt, error) {
	consumptionRequest := BootSessionConsumptionRequest{
		AttemptID:      consumption.AttemptID,
		ConsumerID:     consumption.ConsumerID,
		ConsumedAtUnix: consumption.ConsumedAtUnix,
	}
	if err := VerifyBootSessionConsumptionReceipt(consumption, consumptionRequest, fresh); err != nil {
		return BootSessionReadmissionConsumptionReceipt{},
			fmt.Errorf("%w: fresh consumption evidence: %v", ErrBootSessionReadmissionConsumption, err)
	}
	if readmission.FreshAdmissionID != fresh.AdmissionID ||
		readmission.FreshReplayKey != fresh.ReplayKey ||
		consumption.AdmissionID != fresh.AdmissionID || consumption.ReplayKey != fresh.ReplayKey {
		return BootSessionReadmissionConsumptionReceipt{},
			fmt.Errorf("%w: fresh consumption is not bound to exact readmission", ErrBootSessionReadmissionConsumption)
	}

	atomicVerified := false
	handoffConsumed := false
	recoveryRequired := false
	recoveryAction := ""
	switch status {
	case BootSessionConsumptionCommitted:
		atomicVerified = true
		handoffConsumed = true
		recoveryAction = "none-readmission-consumed-terminal"
	case BootSessionConsumptionAlreadyConsumed:
		atomicVerified = true
		handoffConsumed = true
		recoveryAction = "none-readmission-already-consumed-terminal"
	case BootSessionConsumptionAmbiguous:
		recoveryRequired = true
		recoveryAction = "operator-reconcile-readmitted-replay-key-no-automatic-reissue"
	default:
		return BootSessionReadmissionConsumptionReceipt{},
			fmt.Errorf("%w: unsupported consumption state %q", ErrBootSessionReadmissionConsumption, status)
	}

	digestInput := bootSessionReadmissionConsumptionReceiptDigest{
		ReceiptVersion:         bootSessionReadmissionConsumptionReceiptVersion,
		RecoveryGeneration:     bootSessionReadmissionRecoveryGeneration,
		MaxRecoveryGenerations: bootSessionReadmissionMaxRecoveryGenerations,
		ReadmissionReceiptID:   readmission.ReceiptID,
		ReconciliationID:       readmission.ReconciliationID,
		PreviousAdmissionID:    readmission.PreviousAdmissionID,
		PreviousReplayKey:      readmission.PreviousReplayKey,
		FreshAdmissionID:       fresh.AdmissionID,
		FreshReplayKey:         fresh.ReplayKey,
		ConsumptionReceiptID:   consumption.ReceiptID,
		MachineID:              fresh.MachineID,
		PlanID:                 fresh.PlanID,
		ServingReceiptID:       fresh.ServingReceiptID,
		MediaSHA256:            fresh.MediaSHA256,
		MediaSize:              fresh.MediaSize,
		Target:                 fresh.Target,
		AttemptID:              consumption.AttemptID,
		ConsumerID:             consumption.ConsumerID,
		ConsumedAtUnix:         consumption.ConsumedAtUnix,
		Status:                 status,
		RecoveryRequired:       recoveryRequired,
		RecoveryAction:         recoveryAction,
	}
	encoded, err := json.Marshal(digestInput)
	if err != nil {
		return BootSessionReadmissionConsumptionReceipt{},
			fmt.Errorf("%w: encode terminal readmission evidence: %v", ErrBootSessionReadmissionConsumption, err)
	}
	digest := sha256.Sum256(encoded)

	return BootSessionReadmissionConsumptionReceipt{
		ReceiptID:                    hex.EncodeToString(digest[:]),
		ReceiptVersion:               bootSessionReadmissionConsumptionReceiptVersion,
		RecoveryGeneration:           bootSessionReadmissionRecoveryGeneration,
		MaxRecoveryGenerations:       bootSessionReadmissionMaxRecoveryGenerations,
		ReadmissionReceiptID:         readmission.ReceiptID,
		ReconciliationID:             readmission.ReconciliationID,
		PreviousAdmissionID:          readmission.PreviousAdmissionID,
		PreviousReplayKey:            readmission.PreviousReplayKey,
		FreshAdmissionID:             fresh.AdmissionID,
		FreshReplayKey:               fresh.ReplayKey,
		ConsumptionReceiptID:         consumption.ReceiptID,
		MachineID:                    fresh.MachineID,
		PlanID:                       fresh.PlanID,
		ServingReceiptID:             fresh.ServingReceiptID,
		MediaSHA256:                  fresh.MediaSHA256,
		MediaSize:                    fresh.MediaSize,
		Target:                       fresh.Target,
		AttemptID:                    consumption.AttemptID,
		ConsumerID:                   consumption.ConsumerID,
		ConsumedAtUnix:               consumption.ConsumedAtUnix,
		Status:                       status,
		AtomicConsumeVerified:        atomicVerified,
		BootHandoffConsumed:          handoffConsumed,
		BootHandoffAuthorized:        false,
		FurtherReadmissionAuthorized: false,
		AutomaticRetryAuthorized:     false,
		ProvisioningAuthorized:       false,
		SecretInjectionAuthorized:    false,
		HostMutation:                 false,
		NetworkMutation:              false,
		ProductionMutation:           false,
		RecoveryRequired:             recoveryRequired,
		RecoveryAction:               recoveryAction,
	}, nil
}
