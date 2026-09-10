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

var ErrBootSessionSeparateDeploymentIntent = errors.New(
	"PXE separate deployment intent validation failed",
)

const bootSessionSeparateDeploymentIntentReceiptVersion =
	"boot-session-separate-deployment-intent-receipt-v1"

// BootSessionSeparateDeploymentIntentRequest starts a fresh operator-controlled
// deployment lineage after a previous new-intent consume was proven absent and
// its admission expired. It is never an automatic retry of the prior replay key.
type BootSessionSeparateDeploymentIntentRequest struct {
	IntentID         string             `json:"intentId"`
	OperatorApproved bool               `json:"operatorApproved"`
	BootRequest      BootSessionRequest `json:"bootRequest"`
}

// BootSessionSeparateDeploymentIntentReceipt seals the recovery boundary. The
// receipt is evidence-only: only NewAdmissionID identifies the separately
// returned short-lived single-use BootSessionAdmission.
type BootSessionSeparateDeploymentIntentReceipt struct {
	ReceiptID                      string                        `json:"receiptId"`
	ReceiptVersion                 string                        `json:"receiptVersion"`
	PreviousReconciliationID       string                        `json:"previousReconciliationId"`
	PreviousIntentReceiptID        string                        `json:"previousIntentReceiptId"`
	PreviousIntentID               string                        `json:"previousIntentId"`
	PreviousAdmissionID            string                        `json:"previousAdmissionId"`
	PreviousReplayKey              string                        `json:"previousReplayKey"`
	IntentID                       string                        `json:"intentId"`
	RequestID                      string                        `json:"requestId"`
	NewAdmissionID                 string                        `json:"newAdmissionId"`
	NewReplayKey                   string                        `json:"newReplayKey"`
	MachineID                      string                        `json:"machineId"`
	PlanID                         string                        `json:"planId"`
	ServingReceiptID               string                        `json:"servingReceiptId"`
	MediaSHA256                    string                        `json:"mediaSha256"`
	MediaSize                      int64                         `json:"mediaSize"`
	Target                         InstallMediaPublicationTarget `json:"target"`
	UnattendedBindingID            string                        `json:"unattendedBindingId,omitempty"`
	UnattendedTemplateSHA256       string                        `json:"unattendedTemplateSha256,omitempty"`
	PreviousObservedAtUnix         int64                         `json:"previousObservedAtUnix"`
	PreviousAdmissionExpiresAtUnix int64                         `json:"previousAdmissionExpiresAtUnix"`
	IssuedAtUnix                   int64                         `json:"issuedAtUnix"`
	ExpiresAtUnix                  int64                         `json:"expiresAtUnix"`
	OperatorApproved               bool                          `json:"operatorApproved"`
	PreviousAdmissionExpired       bool                          `json:"previousAdmissionExpired"`
	FreshServingEvidenceVerified   bool                          `json:"freshServingEvidenceVerified"`
	NewAdmissionIssued             bool                          `json:"newAdmissionIssued"`
	PreviousAdmissionReusable      bool                          `json:"previousAdmissionReusable"`
	PreviousReplayAuthorized       bool                          `json:"previousReplayAuthorized"`
	AutomaticRetryAuthorized       bool                          `json:"automaticRetryAuthorized"`
	BootHandoffAuthorized          bool                          `json:"bootHandoffAuthorized"`
	ProvisioningAuthorized         bool                          `json:"provisioningAuthorized"`
	SecretInjectionAuthorized      bool                          `json:"secretInjectionAuthorized"`
	HostMutation                   bool                          `json:"hostMutation"`
	NetworkMutation                bool                          `json:"networkMutation"`
	ProductionMutation             bool                          `json:"productionMutation"`
	RecoveryAction                 string                        `json:"recoveryAction"`
}

type bootSessionSeparateDeploymentIntentReceiptDigest struct {
	ReceiptVersion                 string                        `json:"receiptVersion"`
	PreviousReconciliationID       string                        `json:"previousReconciliationId"`
	PreviousIntentReceiptID        string                        `json:"previousIntentReceiptId"`
	PreviousIntentID               string                        `json:"previousIntentId"`
	PreviousAdmissionID            string                        `json:"previousAdmissionId"`
	PreviousReplayKey              string                        `json:"previousReplayKey"`
	IntentID                       string                        `json:"intentId"`
	RequestID                      string                        `json:"requestId"`
	NewAdmissionID                 string                        `json:"newAdmissionId"`
	NewReplayKey                   string                        `json:"newReplayKey"`
	MachineID                      string                        `json:"machineId"`
	PlanID                         string                        `json:"planId"`
	ServingReceiptID               string                        `json:"servingReceiptId"`
	MediaSHA256                    string                        `json:"mediaSha256"`
	MediaSize                      int64                         `json:"mediaSize"`
	Target                         InstallMediaPublicationTarget `json:"target"`
	UnattendedBindingID            string                        `json:"unattendedBindingId,omitempty"`
	UnattendedTemplateSHA256       string                        `json:"unattendedTemplateSha256,omitempty"`
	PreviousObservedAtUnix         int64                         `json:"previousObservedAtUnix"`
	PreviousAdmissionExpiresAtUnix int64                         `json:"previousAdmissionExpiresAtUnix"`
	IssuedAtUnix                   int64                         `json:"issuedAtUnix"`
	ExpiresAtUnix                  int64                         `json:"expiresAtUnix"`
	RecoveryAction                 string                        `json:"recoveryAction"`
}

// BuildBootSessionSeparateDeploymentIntent creates one new admission only after
// the previous new-intent consumption is authoritatively definitely absent and
// its admission has expired. Current serving bytes and unattended evidence are
// revalidated by BuildBootSessionAdmission before any boot capability is issued.
func BuildBootSessionSeparateDeploymentIntent(
	request BootSessionSeparateDeploymentIntentRequest,
	previousReconciliation BootSessionNewDeploymentIntentConsumptionReconciliation,
	previousSource BootSessionNewDeploymentIntentConsumptionDecision,
	previousEvidence BootSessionNewDeploymentIntentConsumptionEvidence,
	servingReceipt InstallMediaServingReceipt,
	plan AdmittedDeploymentPlan,
	currentServedMediaPayload []byte,
	unattendedBinding *UnattendedTemplateBinding,
	unattendedTemplate []byte,
	issuedAtUnix int64,
) (BootSessionAdmission, BootSessionSeparateDeploymentIntentReceipt, error) {
	if err := VerifyBootSessionNewDeploymentIntentConsumptionReconciliation(
		previousReconciliation,
		previousSource,
		previousEvidence,
	); err != nil {
		return BootSessionAdmission{}, BootSessionSeparateDeploymentIntentReceipt{}, fmt.Errorf(
			"%w: previous consumption reconciliation: %v",
			ErrBootSessionSeparateDeploymentIntent,
			err,
		)
	}
	if previousReconciliation.State != BootSessionConsumptionReconciledDefinitelyAbsent ||
		previousReconciliation.ObservedReceiptID != "" ||
		!previousReconciliation.RecoveryRequired ||
		!previousReconciliation.AdmissionExpired ||
		previousReconciliation.RecoveryAction !=
			"operator-close-or-start-separate-deployment-intent-boundary" ||
		previousReconciliation.CurrentAdmissionReusable ||
		previousReconciliation.FreshAdmissionAuthorized ||
		previousReconciliation.FurtherReadmissionAuthorized ||
		previousReconciliation.AutomaticRetryAuthorized ||
		previousReconciliation.BootHandoffAuthorized ||
		previousReconciliation.ProvisioningAuthorized ||
		previousReconciliation.SecretInjectionAuthorized ||
		previousReconciliation.HostMutation || previousReconciliation.NetworkMutation ||
		previousReconciliation.ProductionMutation {
		return BootSessionAdmission{}, BootSessionSeparateDeploymentIntentReceipt{}, fmt.Errorf(
			"%w: previous lineage is not terminal definitely-absent recovery evidence",
			ErrBootSessionSeparateDeploymentIntent,
		)
	}
	if issuedAtUnix <= previousReconciliation.ObservedAtUnix ||
		issuedAtUnix < previousReconciliation.AdmissionExpiresAtUnix {
		return BootSessionAdmission{}, BootSessionSeparateDeploymentIntentReceipt{}, fmt.Errorf(
			"%w: a separate deployment intent must begin after terminal observation and expiry",
			ErrBootSessionSeparateDeploymentIntent,
		)
	}

	request.IntentID = strings.TrimSpace(request.IntentID)
	request.BootRequest.RequestID = strings.TrimSpace(request.BootRequest.RequestID)
	request.BootRequest.MachineID = strings.TrimSpace(request.BootRequest.MachineID)
	if !request.OperatorApproved || !validBootSessionIdentity(request.IntentID) ||
		!validBootSessionIdentity(request.BootRequest.RequestID) ||
		!validBootSessionIdentity(request.BootRequest.MachineID) {
		return BootSessionAdmission{}, BootSessionSeparateDeploymentIntentReceipt{}, fmt.Errorf(
			"%w: explicit operator approval and bounded opaque identities are required",
			ErrBootSessionSeparateDeploymentIntent,
		)
	}
	if request.BootRequest.MachineID != previousEvidence.NewAdmission.MachineID {
		return BootSessionAdmission{}, BootSessionSeparateDeploymentIntentReceipt{}, fmt.Errorf(
			"%w: separate deployment intent must remain bound to the same machine",
			ErrBootSessionSeparateDeploymentIntent,
		)
	}
	if request.IntentID == previousEvidence.IntentReceipt.IntentID ||
		request.IntentID == previousEvidence.NewAdmission.RequestID ||
		request.IntentID == request.BootRequest.RequestID ||
		request.BootRequest.RequestID == previousEvidence.IntentReceipt.IntentID ||
		request.BootRequest.RequestID == previousEvidence.NewAdmission.RequestID {
		return BootSessionAdmission{}, BootSessionSeparateDeploymentIntentReceipt{}, fmt.Errorf(
			"%w: separate intent/request identities must not reuse prior deployment lineage",
			ErrBootSessionSeparateDeploymentIntent,
		)
	}

	newAdmission, err := BuildBootSessionAdmission(
		request.BootRequest,
		servingReceipt,
		plan,
		currentServedMediaPayload,
		unattendedBinding,
		unattendedTemplate,
		issuedAtUnix,
	)
	if err != nil {
		return BootSessionAdmission{}, BootSessionSeparateDeploymentIntentReceipt{}, fmt.Errorf(
			"%w: current deployment evidence: %v",
			ErrBootSessionSeparateDeploymentIntent,
			err,
		)
	}
	if newAdmission.AdmissionID == previousEvidence.NewAdmission.AdmissionID ||
		newAdmission.ReplayKey == previousEvidence.NewAdmission.ReplayKey ||
		newAdmission.AdmissionID == previousEvidence.TerminalAdmission.AdmissionID ||
		newAdmission.ReplayKey == previousEvidence.TerminalAdmission.ReplayKey ||
		newAdmission.AdmissionID == previousEvidence.Previous.AdmissionID ||
		newAdmission.ReplayKey == previousEvidence.Previous.ReplayKey {
		return BootSessionAdmission{}, BootSessionSeparateDeploymentIntentReceipt{}, fmt.Errorf(
			"%w: separate deployment intent must mint a disjoint admission/replay lineage",
			ErrBootSessionSeparateDeploymentIntent,
		)
	}

	receipt, err := newBootSessionSeparateDeploymentIntentReceipt(
		request,
		previousReconciliation,
		previousEvidence,
		newAdmission,
	)
	if err != nil {
		return BootSessionAdmission{}, BootSessionSeparateDeploymentIntentReceipt{}, err
	}
	return newAdmission, receipt, nil
}

// VerifyBootSessionSeparateDeploymentIntentReceipt rebuilds the complete
// operator boundary from exact prior reconciliation and fresh deployment
// evidence and rejects any lineage, integrity, timing, or authority drift.
func VerifyBootSessionSeparateDeploymentIntentReceipt(
	receipt BootSessionSeparateDeploymentIntentReceipt,
	newAdmission BootSessionAdmission,
	request BootSessionSeparateDeploymentIntentRequest,
	previousReconciliation BootSessionNewDeploymentIntentConsumptionReconciliation,
	previousSource BootSessionNewDeploymentIntentConsumptionDecision,
	previousEvidence BootSessionNewDeploymentIntentConsumptionEvidence,
	servingReceipt InstallMediaServingReceipt,
	plan AdmittedDeploymentPlan,
	currentServedMediaPayload []byte,
	unattendedBinding *UnattendedTemplateBinding,
	unattendedTemplate []byte,
) error {
	rebuiltAdmission, rebuiltReceipt, err := BuildBootSessionSeparateDeploymentIntent(
		request,
		previousReconciliation,
		previousSource,
		previousEvidence,
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
		request.BootRequest,
		servingReceipt,
		plan,
		currentServedMediaPayload,
		unattendedBinding,
		unattendedTemplate,
		newAdmission.IssuedAtUnix,
	); err != nil {
		return fmt.Errorf(
			"%w: new admission: %v",
			ErrBootSessionSeparateDeploymentIntent,
			err,
		)
	}
	if receipt.ReceiptVersion != bootSessionSeparateDeploymentIntentReceiptVersion ||
		!bootSessionConsumptionCanonicalSHA256(receipt.ReceiptID) ||
		receipt.ReceiptID != rebuiltReceipt.ReceiptID ||
		receipt.PreviousReconciliationID != previousReconciliation.ReconciliationID ||
		receipt.PreviousIntentReceiptID != previousEvidence.IntentReceipt.ReceiptID ||
		receipt.PreviousAdmissionID != previousEvidence.NewAdmission.AdmissionID ||
		receipt.PreviousReplayKey != previousEvidence.NewAdmission.ReplayKey ||
		receipt.NewAdmissionID != newAdmission.AdmissionID ||
		receipt.NewReplayKey != newAdmission.ReplayKey ||
		!receipt.OperatorApproved || !receipt.PreviousAdmissionExpired ||
		!receipt.FreshServingEvidenceVerified || !receipt.NewAdmissionIssued ||
		receipt.PreviousAdmissionReusable || receipt.PreviousReplayAuthorized ||
		receipt.AutomaticRetryAuthorized || receipt.BootHandoffAuthorized ||
		receipt.ProvisioningAuthorized || receipt.SecretInjectionAuthorized ||
		receipt.HostMutation || receipt.NetworkMutation || receipt.ProductionMutation ||
		receipt.RecoveryAction != "consume-separate-deployment-admission-once-or-reconcile" {
		return fmt.Errorf(
			"%w: separate deployment intent safety contract drift",
			ErrBootSessionSeparateDeploymentIntent,
		)
	}
	if !reflect.DeepEqual(rebuiltAdmission, newAdmission) ||
		!reflect.DeepEqual(rebuiltReceipt, receipt) {
		return fmt.Errorf(
			"%w: separate deployment intent no longer matches exact evidence",
			ErrBootSessionSeparateDeploymentIntent,
		)
	}
	return nil
}

func newBootSessionSeparateDeploymentIntentReceipt(
	request BootSessionSeparateDeploymentIntentRequest,
	previousReconciliation BootSessionNewDeploymentIntentConsumptionReconciliation,
	previousEvidence BootSessionNewDeploymentIntentConsumptionEvidence,
	newAdmission BootSessionAdmission,
) (BootSessionSeparateDeploymentIntentReceipt, error) {
	recoveryAction := "consume-separate-deployment-admission-once-or-reconcile"
	digestInput := bootSessionSeparateDeploymentIntentReceiptDigest{
		ReceiptVersion:                 bootSessionSeparateDeploymentIntentReceiptVersion,
		PreviousReconciliationID:       previousReconciliation.ReconciliationID,
		PreviousIntentReceiptID:        previousEvidence.IntentReceipt.ReceiptID,
		PreviousIntentID:               previousEvidence.IntentReceipt.IntentID,
		PreviousAdmissionID:            previousEvidence.NewAdmission.AdmissionID,
		PreviousReplayKey:              previousEvidence.NewAdmission.ReplayKey,
		IntentID:                       request.IntentID,
		RequestID:                      request.BootRequest.RequestID,
		NewAdmissionID:                 newAdmission.AdmissionID,
		NewReplayKey:                   newAdmission.ReplayKey,
		MachineID:                      newAdmission.MachineID,
		PlanID:                         newAdmission.PlanID,
		ServingReceiptID:               newAdmission.ServingReceiptID,
		MediaSHA256:                    newAdmission.MediaSHA256,
		MediaSize:                      newAdmission.MediaSize,
		Target:                         newAdmission.Target,
		UnattendedBindingID:            newAdmission.UnattendedBindingID,
		UnattendedTemplateSHA256:       newAdmission.UnattendedTemplateSHA256,
		PreviousObservedAtUnix:         previousReconciliation.ObservedAtUnix,
		PreviousAdmissionExpiresAtUnix: previousReconciliation.AdmissionExpiresAtUnix,
		IssuedAtUnix:                   newAdmission.IssuedAtUnix,
		ExpiresAtUnix:                  newAdmission.ExpiresAtUnix,
		RecoveryAction:                 recoveryAction,
	}
	encoded, err := json.Marshal(digestInput)
	if err != nil {
		return BootSessionSeparateDeploymentIntentReceipt{}, fmt.Errorf(
			"%w: encode canonical separate deployment intent: %v",
			ErrBootSessionSeparateDeploymentIntent,
			err,
		)
	}
	digest := sha256.Sum256(encoded)
	return BootSessionSeparateDeploymentIntentReceipt{
		ReceiptID:                        hex.EncodeToString(digest[:]),
		ReceiptVersion:                   bootSessionSeparateDeploymentIntentReceiptVersion,
		PreviousReconciliationID:         previousReconciliation.ReconciliationID,
		PreviousIntentReceiptID:          previousEvidence.IntentReceipt.ReceiptID,
		PreviousIntentID:                 previousEvidence.IntentReceipt.IntentID,
		PreviousAdmissionID:              previousEvidence.NewAdmission.AdmissionID,
		PreviousReplayKey:                previousEvidence.NewAdmission.ReplayKey,
		IntentID:                         request.IntentID,
		RequestID:                        request.BootRequest.RequestID,
		NewAdmissionID:                   newAdmission.AdmissionID,
		NewReplayKey:                     newAdmission.ReplayKey,
		MachineID:                        newAdmission.MachineID,
		PlanID:                           newAdmission.PlanID,
		ServingReceiptID:                 newAdmission.ServingReceiptID,
		MediaSHA256:                      newAdmission.MediaSHA256,
		MediaSize:                        newAdmission.MediaSize,
		Target:                           newAdmission.Target,
		UnattendedBindingID:              newAdmission.UnattendedBindingID,
		UnattendedTemplateSHA256:         newAdmission.UnattendedTemplateSHA256,
		PreviousObservedAtUnix:           previousReconciliation.ObservedAtUnix,
		PreviousAdmissionExpiresAtUnix:   previousReconciliation.AdmissionExpiresAtUnix,
		IssuedAtUnix:                     newAdmission.IssuedAtUnix,
		ExpiresAtUnix:                    newAdmission.ExpiresAtUnix,
		OperatorApproved:                 true,
		PreviousAdmissionExpired:         true,
		FreshServingEvidenceVerified:     true,
		NewAdmissionIssued:               true,
		PreviousAdmissionReusable:        false,
		PreviousReplayAuthorized:         false,
		AutomaticRetryAuthorized:         false,
		BootHandoffAuthorized:            false,
		ProvisioningAuthorized:           false,
		SecretInjectionAuthorized:        false,
		HostMutation:                     false,
		NetworkMutation:                  false,
		ProductionMutation:               false,
		RecoveryAction:                   recoveryAction,
	}, nil
}
