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

var ErrBootSessionNewDeploymentIntent = errors.New("PXE new deployment intent validation failed")

const bootSessionNewDeploymentIntentReceiptVersion = "boot-session-new-deployment-intent-receipt-v1"

// BootSessionNewDeploymentIntentRequest is an explicit operator-controlled
// boundary after the terminal readmission recovery generation. It is not a
// retry/readmission request: IntentID and RequestID must start a new lineage.
type BootSessionNewDeploymentIntentRequest struct {
	IntentID         string             `json:"intentId"`
	OperatorApproved bool               `json:"operatorApproved"`
	BootRequest      BootSessionRequest `json:"bootRequest"`
}

// BootSessionNewDeploymentIntentReceipt seals the transition from an exhausted
// terminal recovery lineage to a separately approved deployment intent. The
// receipt is evidence-only; boot authority exists solely in NewAdmissionID's
// separately returned, short-lived BootSessionAdmission.
type BootSessionNewDeploymentIntentReceipt struct {
	ReceiptID                               string                        `json:"receiptId"`
	ReceiptVersion                          string                        `json:"receiptVersion"`
	IntentID                                string                        `json:"intentId"`
	RequestID                               string                        `json:"requestId"`
	TerminalReconciliationID                string                        `json:"terminalReconciliationId"`
	TerminalReadmissionConsumptionReceiptID string                        `json:"terminalReadmissionConsumptionReceiptId"`
	ReadmissionReceiptID                    string                        `json:"readmissionReceiptId"`
	PreviousAdmissionID                     string                        `json:"previousAdmissionId"`
	PreviousReplayKey                       string                        `json:"previousReplayKey"`
	TerminalAdmissionID                     string                        `json:"terminalAdmissionId"`
	TerminalReplayKey                       string                        `json:"terminalReplayKey"`
	NewAdmissionID                          string                        `json:"newAdmissionId"`
	NewReplayKey                            string                        `json:"newReplayKey"`
	MachineID                               string                        `json:"machineId"`
	PlanID                                  string                        `json:"planId"`
	ServingReceiptID                        string                        `json:"servingReceiptId"`
	MediaSHA256                             string                        `json:"mediaSha256"`
	MediaSize                               int64                         `json:"mediaSize"`
	Target                                  InstallMediaPublicationTarget `json:"target"`
	UnattendedBindingID                     string                        `json:"unattendedBindingId,omitempty"`
	UnattendedTemplateSHA256                string                        `json:"unattendedTemplateSha256,omitempty"`
	IssuedAtUnix                            int64                         `json:"issuedAtUnix"`
	ExpiresAtUnix                           int64                         `json:"expiresAtUnix"`
	OperatorApproved                        bool                          `json:"operatorApproved"`
	FreshServingEvidenceVerified            bool                          `json:"freshServingEvidenceVerified"`
	NewAdmissionIssued                      bool                          `json:"newAdmissionIssued"`
	PriorAdmissionsReusable                 bool                          `json:"priorAdmissionsReusable"`
	PriorReplayAuthorized                   bool                          `json:"priorReplayAuthorized"`
	FurtherReadmissionAuthorized            bool                          `json:"furtherReadmissionAuthorized"`
	AutomaticRetryAuthorized                bool                          `json:"automaticRetryAuthorized"`
	BootHandoffAuthorized                   bool                          `json:"bootHandoffAuthorized"`
	ProvisioningAuthorized                  bool                          `json:"provisioningAuthorized"`
	SecretInjectionAuthorized               bool                          `json:"secretInjectionAuthorized"`
	HostMutation                            bool                          `json:"hostMutation"`
	NetworkMutation                         bool                          `json:"networkMutation"`
	ProductionMutation                      bool                          `json:"productionMutation"`
	RecoveryAction                          string                        `json:"recoveryAction"`
}

type bootSessionNewDeploymentIntentReceiptDigest struct {
	ReceiptVersion                          string                        `json:"receiptVersion"`
	IntentID                                string                        `json:"intentId"`
	RequestID                               string                        `json:"requestId"`
	TerminalReconciliationID                string                        `json:"terminalReconciliationId"`
	TerminalReadmissionConsumptionReceiptID string                        `json:"terminalReadmissionConsumptionReceiptId"`
	ReadmissionReceiptID                    string                        `json:"readmissionReceiptId"`
	PreviousAdmissionID                     string                        `json:"previousAdmissionId"`
	PreviousReplayKey                       string                        `json:"previousReplayKey"`
	TerminalAdmissionID                     string                        `json:"terminalAdmissionId"`
	TerminalReplayKey                       string                        `json:"terminalReplayKey"`
	NewAdmissionID                          string                        `json:"newAdmissionId"`
	NewReplayKey                            string                        `json:"newReplayKey"`
	MachineID                               string                        `json:"machineId"`
	PlanID                                  string                        `json:"planId"`
	ServingReceiptID                        string                        `json:"servingReceiptId"`
	MediaSHA256                             string                        `json:"mediaSha256"`
	MediaSize                               int64                         `json:"mediaSize"`
	Target                                  InstallMediaPublicationTarget `json:"target"`
	UnattendedBindingID                     string                        `json:"unattendedBindingId,omitempty"`
	UnattendedTemplateSHA256                string                        `json:"unattendedTemplateSha256,omitempty"`
	IssuedAtUnix                            int64                         `json:"issuedAtUnix"`
	ExpiresAtUnix                           int64                         `json:"expiresAtUnix"`
	RecoveryAction                          string                        `json:"recoveryAction"`
}

// BuildBootSessionNewDeploymentIntent creates a new deployment lineage only
// after the terminal readmission generation is proven definitely absent and
// has expired. All current serving/media/unattended evidence is revalidated by
// BuildBootSessionAdmission. No authority is inherited from prior replay keys.
func BuildBootSessionNewDeploymentIntent(
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
	issuedAtUnix int64,
) (BootSessionAdmission, BootSessionNewDeploymentIntentReceipt, error) {
	if err := VerifyBootSessionReadmissionConsumptionReconciliation(
		terminalReconciliation,
		terminalConsumption,
		readmission,
		previous,
		previousCandidate,
		previousReconciliation,
		terminalAdmission,
		terminalCandidate,
	); err != nil {
		return BootSessionAdmission{}, BootSessionNewDeploymentIntentReceipt{},
			fmt.Errorf("%w: terminal reconciliation evidence: %v", ErrBootSessionNewDeploymentIntent, err)
	}
	if terminalReconciliation.State != BootSessionConsumptionReconciledDefinitelyAbsent ||
		!terminalReconciliation.TerminalGeneration || !terminalReconciliation.RecoveryRequired ||
		terminalReconciliation.RecoveryAction != "operator-close-terminal-readmission-or-create-new-deployment-intent" ||
		terminalReconciliation.ObservedReceiptID != "" ||
		terminalReconciliation.FurtherReadmissionAuthorized ||
		terminalReconciliation.AutomaticRetryAuthorized || terminalReconciliation.BootHandoffAuthorized {
		return BootSessionAdmission{}, BootSessionNewDeploymentIntentReceipt{},
			fmt.Errorf("%w: terminal recovery lineage is not eligible for a new deployment intent", ErrBootSessionNewDeploymentIntent)
	}

	intent.IntentID = strings.TrimSpace(intent.IntentID)
	intent.BootRequest.RequestID = strings.TrimSpace(intent.BootRequest.RequestID)
	intent.BootRequest.MachineID = strings.TrimSpace(intent.BootRequest.MachineID)
	if !intent.OperatorApproved || !validBootSessionIdentity(intent.IntentID) ||
		!validBootSessionIdentity(intent.BootRequest.RequestID) ||
		!validBootSessionIdentity(intent.BootRequest.MachineID) {
		return BootSessionAdmission{}, BootSessionNewDeploymentIntentReceipt{},
			fmt.Errorf("%w: explicit operator approval and bounded opaque intent identities are required", ErrBootSessionNewDeploymentIntent)
	}
	if intent.BootRequest.MachineID != previous.MachineID ||
		intent.BootRequest.MachineID != terminalAdmission.MachineID {
		return BootSessionAdmission{}, BootSessionNewDeploymentIntentReceipt{},
			fmt.Errorf("%w: new intent must remain bound to the same machine", ErrBootSessionNewDeploymentIntent)
	}
	if intent.IntentID == previous.RequestID || intent.IntentID == terminalAdmission.RequestID ||
		intent.IntentID == intent.BootRequest.RequestID ||
		intent.BootRequest.RequestID == previous.RequestID ||
		intent.BootRequest.RequestID == terminalAdmission.RequestID {
		return BootSessionAdmission{}, BootSessionNewDeploymentIntentReceipt{},
			fmt.Errorf("%w: new intent and request identities must not reuse exhausted request lineage", ErrBootSessionNewDeploymentIntent)
	}
	if issuedAtUnix < terminalAdmission.ExpiresAtUnix ||
		issuedAtUnix <= terminalReconciliation.ObservedAtUnix {
		return BootSessionAdmission{}, BootSessionNewDeploymentIntentReceipt{},
			fmt.Errorf("%w: exhausted terminal admission must expire before a new deployment intent is issued", ErrBootSessionNewDeploymentIntent)
	}

	newAdmission, err := BuildBootSessionAdmission(
		intent.BootRequest,
		servingReceipt,
		plan,
		currentServedMediaPayload,
		unattendedBinding,
		unattendedTemplate,
		issuedAtUnix,
	)
	if err != nil {
		return BootSessionAdmission{}, BootSessionNewDeploymentIntentReceipt{},
			fmt.Errorf("%w: current deployment evidence: %v", ErrBootSessionNewDeploymentIntent, err)
	}
	if newAdmission.AdmissionID == previous.AdmissionID ||
		newAdmission.AdmissionID == terminalAdmission.AdmissionID ||
		newAdmission.ReplayKey == previous.ReplayKey ||
		newAdmission.ReplayKey == terminalAdmission.ReplayKey {
		return BootSessionAdmission{}, BootSessionNewDeploymentIntentReceipt{},
			fmt.Errorf("%w: new deployment intent must start a distinct admission and replay lineage", ErrBootSessionNewDeploymentIntent)
	}

	receipt, err := newBootSessionNewDeploymentIntentReceipt(
		intent,
		terminalReconciliation,
		terminalConsumption,
		readmission,
		previous,
		terminalAdmission,
		newAdmission,
	)
	if err != nil {
		return BootSessionAdmission{}, BootSessionNewDeploymentIntentReceipt{}, err
	}
	return newAdmission, receipt, nil
}

// VerifyBootSessionNewDeploymentIntentReceipt rebuilds the complete boundary
// from exact terminal and current deployment evidence and rejects any lineage,
// authority, integrity, or recovery-semantics drift.
func VerifyBootSessionNewDeploymentIntentReceipt(
	receipt BootSessionNewDeploymentIntentReceipt,
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
) error {
	rebuiltAdmission, rebuiltReceipt, err := BuildBootSessionNewDeploymentIntent(
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
		receipt.IssuedAtUnix,
	)
	if err != nil {
		return err
	}
	if err := VerifyBootSessionAdmission(
		newAdmission,
		intent.BootRequest,
		servingReceipt,
		plan,
		currentServedMediaPayload,
		unattendedBinding,
		unattendedTemplate,
		newAdmission.IssuedAtUnix,
	); err != nil {
		return fmt.Errorf("%w: new boot admission: %v", ErrBootSessionNewDeploymentIntent, err)
	}
	if receipt.ReceiptVersion != bootSessionNewDeploymentIntentReceiptVersion ||
		!bootSessionConsumptionCanonicalSHA256(receipt.ReceiptID) ||
		receipt.ReceiptID != rebuiltReceipt.ReceiptID ||
		receipt.NewAdmissionID != newAdmission.AdmissionID || receipt.NewReplayKey != newAdmission.ReplayKey ||
		!receipt.OperatorApproved || !receipt.FreshServingEvidenceVerified || !receipt.NewAdmissionIssued ||
		receipt.PriorAdmissionsReusable || receipt.PriorReplayAuthorized ||
		receipt.FurtherReadmissionAuthorized || receipt.AutomaticRetryAuthorized ||
		receipt.BootHandoffAuthorized || receipt.ProvisioningAuthorized ||
		receipt.SecretInjectionAuthorized || receipt.HostMutation || receipt.NetworkMutation ||
		receipt.ProductionMutation ||
		receipt.RecoveryAction != "consume-new-deployment-admission-once-or-reconcile" {
		return fmt.Errorf("%w: new deployment intent safety contract drift", ErrBootSessionNewDeploymentIntent)
	}
	if !reflect.DeepEqual(rebuiltAdmission, newAdmission) || !reflect.DeepEqual(rebuiltReceipt, receipt) {
		return fmt.Errorf("%w: new deployment intent no longer matches exact evidence", ErrBootSessionNewDeploymentIntent)
	}
	return nil
}

func newBootSessionNewDeploymentIntentReceipt(
	intent BootSessionNewDeploymentIntentRequest,
	terminalReconciliation BootSessionReadmissionConsumptionReconciliation,
	terminalConsumption BootSessionReadmissionConsumptionReceipt,
	readmission BootSessionReadmissionReceipt,
	previous BootSessionAdmission,
	terminalAdmission BootSessionAdmission,
	newAdmission BootSessionAdmission,
) (BootSessionNewDeploymentIntentReceipt, error) {
	recoveryAction := "consume-new-deployment-admission-once-or-reconcile"
	digestInput := bootSessionNewDeploymentIntentReceiptDigest{
		ReceiptVersion:                          bootSessionNewDeploymentIntentReceiptVersion,
		IntentID:                                intent.IntentID,
		RequestID:                               intent.BootRequest.RequestID,
		TerminalReconciliationID:                terminalReconciliation.ReconciliationID,
		TerminalReadmissionConsumptionReceiptID: terminalConsumption.ReceiptID,
		ReadmissionReceiptID:                    readmission.ReceiptID,
		PreviousAdmissionID:                     previous.AdmissionID,
		PreviousReplayKey:                       previous.ReplayKey,
		TerminalAdmissionID:                     terminalAdmission.AdmissionID,
		TerminalReplayKey:                       terminalAdmission.ReplayKey,
		NewAdmissionID:                          newAdmission.AdmissionID,
		NewReplayKey:                            newAdmission.ReplayKey,
		MachineID:                               newAdmission.MachineID,
		PlanID:                                  newAdmission.PlanID,
		ServingReceiptID:                        newAdmission.ServingReceiptID,
		MediaSHA256:                             newAdmission.MediaSHA256,
		MediaSize:                               newAdmission.MediaSize,
		Target:                                  newAdmission.Target,
		UnattendedBindingID:                     newAdmission.UnattendedBindingID,
		UnattendedTemplateSHA256:                newAdmission.UnattendedTemplateSHA256,
		IssuedAtUnix:                            newAdmission.IssuedAtUnix,
		ExpiresAtUnix:                           newAdmission.ExpiresAtUnix,
		RecoveryAction:                          recoveryAction,
	}
	encoded, err := json.Marshal(digestInput)
	if err != nil {
		return BootSessionNewDeploymentIntentReceipt{},
			fmt.Errorf("%w: encode canonical new deployment intent: %v", ErrBootSessionNewDeploymentIntent, err)
	}
	digest := sha256.Sum256(encoded)
	return BootSessionNewDeploymentIntentReceipt{
		ReceiptID:                               hex.EncodeToString(digest[:]),
		ReceiptVersion:                          bootSessionNewDeploymentIntentReceiptVersion,
		IntentID:                                intent.IntentID,
		RequestID:                               intent.BootRequest.RequestID,
		TerminalReconciliationID:                terminalReconciliation.ReconciliationID,
		TerminalReadmissionConsumptionReceiptID: terminalConsumption.ReceiptID,
		ReadmissionReceiptID:                    readmission.ReceiptID,
		PreviousAdmissionID:                     previous.AdmissionID,
		PreviousReplayKey:                       previous.ReplayKey,
		TerminalAdmissionID:                     terminalAdmission.AdmissionID,
		TerminalReplayKey:                       terminalAdmission.ReplayKey,
		NewAdmissionID:                          newAdmission.AdmissionID,
		NewReplayKey:                            newAdmission.ReplayKey,
		MachineID:                               newAdmission.MachineID,
		PlanID:                                  newAdmission.PlanID,
		ServingReceiptID:                        newAdmission.ServingReceiptID,
		MediaSHA256:                             newAdmission.MediaSHA256,
		MediaSize:                               newAdmission.MediaSize,
		Target:                                  newAdmission.Target,
		UnattendedBindingID:                     newAdmission.UnattendedBindingID,
		UnattendedTemplateSHA256:                newAdmission.UnattendedTemplateSHA256,
		IssuedAtUnix:                            newAdmission.IssuedAtUnix,
		ExpiresAtUnix:                           newAdmission.ExpiresAtUnix,
		OperatorApproved:                        true,
		FreshServingEvidenceVerified:            true,
		NewAdmissionIssued:                      true,
		PriorAdmissionsReusable:                 false,
		PriorReplayAuthorized:                   false,
		FurtherReadmissionAuthorized:            false,
		AutomaticRetryAuthorized:                false,
		BootHandoffAuthorized:                   false,
		ProvisioningAuthorized:                  false,
		SecretInjectionAuthorized:               false,
		HostMutation:                            false,
		NetworkMutation:                         false,
		ProductionMutation:                      false,
		RecoveryAction:                          recoveryAction,
	}, nil
}
