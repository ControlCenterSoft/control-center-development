package pxe

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
)

var (
	ErrBootSessionConsumption          = errors.New("PXE boot session consumption validation failed")
	ErrBootSessionConsumptionAmbiguous = errors.New("PXE boot session consumption outcome is ambiguous")
)

const bootSessionConsumptionReceiptVersion = "boot-session-consumption-receipt-v1"

type BootSessionConsumptionRequest struct {
	AttemptID      string `json:"attemptId"`
	ConsumerID     string `json:"consumerId"`
	ConsumedAtUnix int64  `json:"consumedAtUnix"`
}

type BootSessionConsumptionReceipt struct {
	ReceiptID                 string                        `json:"receiptId"`
	ReceiptVersion            string                        `json:"receiptVersion"`
	AdmissionID               string                        `json:"admissionId"`
	ReplayKey                 string                        `json:"replayKey"`
	RequestID                 string                        `json:"requestId"`
	MachineID                 string                        `json:"machineId"`
	AttemptID                 string                        `json:"attemptId"`
	ConsumerID                string                        `json:"consumerId"`
	PlanID                    string                        `json:"planId"`
	ServingReceiptID          string                        `json:"servingReceiptId"`
	MediaSHA256               string                        `json:"mediaSha256"`
	MediaSize                 int64                         `json:"mediaSize"`
	Target                    InstallMediaPublicationTarget `json:"target"`
	ConsumedAtUnix            int64                         `json:"consumedAtUnix"`
	SingleUse                 bool                          `json:"singleUse"`
	AtomicConsumeRequired     bool                          `json:"atomicConsumeRequired"`
	BootHandoffConsumed       bool                          `json:"bootHandoffConsumed"`
	ProvisioningAuthorized    bool                          `json:"provisioningAuthorized"`
	SecretInjectionAuthorized bool                          `json:"secretInjectionAuthorized"`
	HostMutation              bool                          `json:"hostMutation"`
	NetworkMutation           bool                          `json:"networkMutation"`
	ReplayAuthorized          bool                          `json:"replayAuthorized"`
	RollbackAction            string                        `json:"rollbackAction"`
	RecoveryAction            string                        `json:"recoveryAction"`
}

type BootSessionConsumptionStoreState string

const (
	BootSessionConsumptionCommitted       BootSessionConsumptionStoreState = "committed"
	BootSessionConsumptionAlreadyConsumed BootSessionConsumptionStoreState = "already-consumed"
	BootSessionConsumptionAmbiguous       BootSessionConsumptionStoreState = "ambiguous"
)

type BootSessionConsumptionStoreResult struct {
	State    BootSessionConsumptionStoreState
	Existing *BootSessionConsumptionReceipt
}

type BootSessionConsumptionStore interface {
	AtomicConsumeBootSession(candidate BootSessionConsumptionReceipt) (BootSessionConsumptionStoreResult, error)
}

type BootSessionConsumptionDecision struct {
	Status                BootSessionConsumptionStoreState `json:"status"`
	Receipt               BootSessionConsumptionReceipt    `json:"receipt"`
	AtomicConsumeVerified bool                              `json:"atomicConsumeVerified"`
	BootHandoffAuthorized bool                              `json:"bootHandoffAuthorized"`
	ReplayAuthorized      bool                              `json:"replayAuthorized"`
	RecoveryRequired      bool                              `json:"recoveryRequired"`
	RecoveryAction        string                            `json:"recoveryAction"`
}

type bootSessionConsumptionReceiptDigest struct {
	ReceiptVersion   string                        `json:"receiptVersion"`
	AdmissionID      string                        `json:"admissionId"`
	ReplayKey        string                        `json:"replayKey"`
	RequestID        string                        `json:"requestId"`
	MachineID        string                        `json:"machineId"`
	AttemptID        string                        `json:"attemptId"`
	ConsumerID       string                        `json:"consumerId"`
	PlanID           string                        `json:"planId"`
	ServingReceiptID string                        `json:"servingReceiptId"`
	MediaSHA256      string                        `json:"mediaSha256"`
	MediaSize        int64                         `json:"mediaSize"`
	Target           InstallMediaPublicationTarget `json:"target"`
	ConsumedAtUnix   int64                         `json:"consumedAtUnix"`
	RollbackAction   string                        `json:"rollbackAction"`
	RecoveryAction   string                        `json:"recoveryAction"`
}

func BuildBootSessionConsumptionReceipt(
	request BootSessionConsumptionRequest,
	admission BootSessionAdmission,
) (BootSessionConsumptionReceipt, error) {
	request.AttemptID = strings.TrimSpace(request.AttemptID)
	request.ConsumerID = strings.TrimSpace(request.ConsumerID)
	if !validBootSessionIdentity(request.AttemptID) || !validBootSessionIdentity(request.ConsumerID) {
		return BootSessionConsumptionReceipt{}, fmt.Errorf("%w: attempt and consumer identities must be bounded opaque identifiers", ErrBootSessionConsumption)
	}
	if request.ConsumedAtUnix < admission.IssuedAtUnix || request.ConsumedAtUnix >= admission.ExpiresAtUnix {
		return BootSessionConsumptionReceipt{}, fmt.Errorf("%w: consumption time is outside the admitted session lifetime", ErrBootSessionConsumption)
	}
	if admission.AdmissionVersion != bootSessionAdmissionVersion ||
		admission.MaxBootAttempts != 1 ||
		!admission.SingleUse ||
		!admission.AtomicConsumeRequired ||
		!admission.ServingReceiptVerified ||
		!admission.BootAuthorized ||
		admission.ProvisioningAuthorized ||
		admission.SecretInjectionAuthorized ||
		admission.HostMutation ||
		admission.NetworkMutation {
		return BootSessionConsumptionReceipt{}, fmt.Errorf("%w: admission safety contract does not permit bounded consumption", ErrBootSessionConsumption)
	}
	for _, value := range []string{admission.AdmissionID, admission.ReplayKey, admission.PlanID, admission.ServingReceiptID, admission.MediaSHA256} {
		if !bootSessionConsumptionCanonicalSHA256(value) {
			return BootSessionConsumptionReceipt{}, fmt.Errorf("%w: admission contains non-canonical digest identity", ErrBootSessionConsumption)
		}
	}

	rollbackAction := "expire-consumed-session-before-provisioning"
	recoveryAction := "reissue-new-session-from-fresh-serving-evidence"
	digestInput := bootSessionConsumptionReceiptDigest{
		ReceiptVersion:   bootSessionConsumptionReceiptVersion,
		AdmissionID:      admission.AdmissionID,
		ReplayKey:        admission.ReplayKey,
		RequestID:        admission.RequestID,
		MachineID:        admission.MachineID,
		AttemptID:        request.AttemptID,
		ConsumerID:       request.ConsumerID,
		PlanID:           admission.PlanID,
		ServingReceiptID: admission.ServingReceiptID,
		MediaSHA256:      admission.MediaSHA256,
		MediaSize:        admission.MediaSize,
		Target:           admission.Target,
		ConsumedAtUnix:   request.ConsumedAtUnix,
		RollbackAction:   rollbackAction,
		RecoveryAction:   recoveryAction,
	}
	encoded, err := json.Marshal(digestInput)
	if err != nil {
		return BootSessionConsumptionReceipt{}, fmt.Errorf("%w: encode canonical consumption receipt: %v", ErrBootSessionConsumption, err)
	}
	digest := sha256.Sum256(encoded)

	return BootSessionConsumptionReceipt{
		ReceiptID:                 hex.EncodeToString(digest[:]),
		ReceiptVersion:            bootSessionConsumptionReceiptVersion,
		AdmissionID:               admission.AdmissionID,
		ReplayKey:                 admission.ReplayKey,
		RequestID:                 admission.RequestID,
		MachineID:                 admission.MachineID,
		AttemptID:                 request.AttemptID,
		ConsumerID:                request.ConsumerID,
		PlanID:                    admission.PlanID,
		ServingReceiptID:          admission.ServingReceiptID,
		MediaSHA256:               admission.MediaSHA256,
		MediaSize:                 admission.MediaSize,
		Target:                    admission.Target,
		ConsumedAtUnix:            request.ConsumedAtUnix,
		SingleUse:                 true,
		AtomicConsumeRequired:     true,
		BootHandoffConsumed:       true,
		ProvisioningAuthorized:    false,
		SecretInjectionAuthorized: false,
		HostMutation:              false,
		NetworkMutation:           false,
		ReplayAuthorized:          false,
		RollbackAction:            rollbackAction,
		RecoveryAction:            recoveryAction,
	}, nil
}

func VerifyBootSessionConsumptionReceipt(
	receipt BootSessionConsumptionReceipt,
	request BootSessionConsumptionRequest,
	admission BootSessionAdmission,
) error {
	rebuilt, err := BuildBootSessionConsumptionReceipt(request, admission)
	if err != nil {
		return err
	}
	if receipt.ReceiptVersion != bootSessionConsumptionReceiptVersion ||
		!receipt.SingleUse ||
		!receipt.AtomicConsumeRequired ||
		!receipt.BootHandoffConsumed ||
		receipt.ProvisioningAuthorized ||
		receipt.SecretInjectionAuthorized ||
		receipt.HostMutation ||
		receipt.NetworkMutation ||
		receipt.ReplayAuthorized ||
		!bootSessionConsumptionCanonicalSHA256(receipt.ReceiptID) {
		return fmt.Errorf("%w: consumption receipt safety flags do not match", ErrBootSessionConsumption)
	}
	if !reflect.DeepEqual(rebuilt, receipt) {
		return fmt.Errorf("%w: consumption receipt no longer matches exact admission evidence", ErrBootSessionConsumption)
	}
	return nil
}

func ConsumeBootSessionAdmission(
	store BootSessionConsumptionStore,
	request BootSessionConsumptionRequest,
	admission BootSessionAdmission,
	sessionRequest BootSessionRequest,
	servingReceipt InstallMediaServingReceipt,
	plan AdmittedDeploymentPlan,
	currentServedMediaPayload []byte,
	unattendedBinding *UnattendedTemplateBinding,
	unattendedTemplate []byte,
) (BootSessionConsumptionDecision, error) {
	if store == nil {
		return BootSessionConsumptionDecision{}, fmt.Errorf("%w: atomic consumption store is required", ErrBootSessionConsumption)
	}
	if err := VerifyBootSessionAdmission(
		admission,
		sessionRequest,
		servingReceipt,
		plan,
		currentServedMediaPayload,
		unattendedBinding,
		unattendedTemplate,
		request.ConsumedAtUnix,
	); err != nil {
		return BootSessionConsumptionDecision{}, fmt.Errorf("%w: boot-session admission revalidation failed: %v", ErrBootSessionConsumption, err)
	}
	candidate, err := BuildBootSessionConsumptionReceipt(request, admission)
	if err != nil {
		return BootSessionConsumptionDecision{}, err
	}

	result, consumeErr := store.AtomicConsumeBootSession(candidate)
	if consumeErr != nil || result.State == BootSessionConsumptionAmbiguous {
		return BootSessionConsumptionDecision{
			Status:                BootSessionConsumptionAmbiguous,
			Receipt:               candidate,
			AtomicConsumeVerified: false,
			BootHandoffAuthorized: false,
			ReplayAuthorized:      false,
			RecoveryRequired:      true,
			RecoveryAction:        "reconcile-replay-key-before-reissuing-session",
		}, fmt.Errorf("%w: durable consume result must be reconciled before any boot handoff", ErrBootSessionConsumptionAmbiguous)
	}

	switch result.State {
	case BootSessionConsumptionCommitted:
		if result.Existing != nil {
			return BootSessionConsumptionDecision{}, fmt.Errorf("%w: committed result must not return pre-existing receipt", ErrBootSessionConsumption)
		}
		return BootSessionConsumptionDecision{
			Status:                BootSessionConsumptionCommitted,
			Receipt:               candidate,
			AtomicConsumeVerified: true,
			BootHandoffAuthorized: true,
			ReplayAuthorized:      false,
			RecoveryRequired:      false,
			RecoveryAction:        "none",
		}, nil
	case BootSessionConsumptionAlreadyConsumed:
		if result.Existing == nil {
			return BootSessionConsumptionDecision{}, fmt.Errorf("%w: already-consumed result requires persisted receipt", ErrBootSessionConsumption)
		}
		if err := VerifyBootSessionConsumptionReceipt(*result.Existing, request, admission); err != nil {
			return BootSessionConsumptionDecision{}, err
		}
		if !reflect.DeepEqual(candidate, *result.Existing) {
			return BootSessionConsumptionDecision{}, fmt.Errorf("%w: replay key is already bound to different consumption evidence", ErrBootSessionConsumption)
		}
		return BootSessionConsumptionDecision{
			Status:                BootSessionConsumptionAlreadyConsumed,
			Receipt:               *result.Existing,
			AtomicConsumeVerified: true,
			BootHandoffAuthorized: false,
			ReplayAuthorized:      false,
			RecoveryRequired:      false,
			RecoveryAction:        "none-already-consumed",
		}, nil
	default:
		return BootSessionConsumptionDecision{
			Status:                BootSessionConsumptionAmbiguous,
			Receipt:               candidate,
			AtomicConsumeVerified: false,
			BootHandoffAuthorized: false,
			ReplayAuthorized:      false,
			RecoveryRequired:      true,
			RecoveryAction:        "reconcile-replay-key-before-reissuing-session",
		}, fmt.Errorf("%w: unrecognized atomic consume state %q", ErrBootSessionConsumptionAmbiguous, result.State)
	}
}

func bootSessionConsumptionCanonicalSHA256(value string) bool {
	if len(value) != sha256.Size*2 || strings.ToLower(value) != value {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size
}
