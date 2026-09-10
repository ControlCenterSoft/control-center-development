package pxe

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

// BuildBootSessionReadmission consumes only sealed, expired,
// definitely-absent reconciliation evidence. It revalidates current serving
// media and the exact immutable deployment intent before issuing a new
// short-lived, single-use admission with a distinct replay lineage.
func BuildBootSessionReadmission(
	previous BootSessionAdmission,
	candidate BootSessionConsumptionReceipt,
	reconciliation BootSessionConsumptionReconciliation,
	request BootSessionRequest,
	receipt InstallMediaServingReceipt,
	plan AdmittedDeploymentPlan,
	currentServedMediaPayload []byte,
	unattendedBinding *UnattendedTemplateBinding,
	unattendedTemplate []byte,
	issuedAtUnix int64,
) (BootSessionAdmission, BootSessionReadmissionReceipt, error) {
	if err := VerifyBootSessionConsumptionReconciliation(reconciliation); err != nil {
		return BootSessionAdmission{}, BootSessionReadmissionReceipt{},
			fmt.Errorf("%w: reconciliation evidence: %v", ErrBootSessionReadmission, err)
	}
	if err := validatePreviousBootSessionAdmissionForReadmission(previous); err != nil {
		return BootSessionAdmission{}, BootSessionReadmissionReceipt{}, err
	}
	candidateRequest := BootSessionConsumptionRequest{
		AttemptID:      candidate.AttemptID,
		ConsumerID:     candidate.ConsumerID,
		ConsumedAtUnix: candidate.ConsumedAtUnix,
	}
	if err := VerifyBootSessionConsumptionReceipt(candidate, candidateRequest, previous); err != nil {
		return BootSessionAdmission{}, BootSessionReadmissionReceipt{},
			fmt.Errorf("%w: candidate consumption evidence: %v", ErrBootSessionReadmission, err)
	}
	if reconciliation.State != BootSessionConsumptionReconciledDefinitelyAbsent ||
		!reconciliation.OldAdmissionExpired || !reconciliation.FreshAdmissionRequired ||
		!reconciliation.FreshServingEvidenceRequired || !reconciliation.RecoveryRequired ||
		reconciliation.RecoveryAction != "reissue-new-session-from-fresh-serving-evidence" ||
		reconciliation.ObservedReceiptID != "" {
		return BootSessionAdmission{}, BootSessionReadmissionReceipt{},
			fmt.Errorf("%w: reconciliation does not permit fresh readmission", ErrBootSessionReadmission)
	}
	if reconciliation.AdmissionID != previous.AdmissionID ||
		reconciliation.ReplayKey != previous.ReplayKey ||
		reconciliation.CandidateReceiptID != candidate.ReceiptID ||
		candidate.AdmissionID != previous.AdmissionID || candidate.ReplayKey != previous.ReplayKey ||
		reconciliation.AdmissionExpiresAtUnix != previous.ExpiresAtUnix {
		return BootSessionAdmission{}, BootSessionReadmissionReceipt{},
			fmt.Errorf("%w: reconciliation is not bound to the exact previous admission", ErrBootSessionReadmission)
	}
	if issuedAtUnix < reconciliation.ObservedAtUnix || issuedAtUnix < previous.ExpiresAtUnix ||
		issuedAtUnix <= previous.IssuedAtUnix {
		return BootSessionAdmission{}, BootSessionReadmissionReceipt{},
			fmt.Errorf("%w: fresh admission time predates sealed expiry/reconciliation evidence", ErrBootSessionReadmission)
	}
	previousTTL := previous.ExpiresAtUnix - previous.IssuedAtUnix
	if request.TTLSeconds <= 0 || request.TTLSeconds > previousTTL {
		return BootSessionAdmission{}, BootSessionReadmissionReceipt{},
			fmt.Errorf("%w: readmission lifetime must not exceed the previous bounded session", ErrBootSessionReadmission)
	}
	if request.MachineID != previous.MachineID || request.RequestID == previous.RequestID {
		return BootSessionAdmission{}, BootSessionReadmissionReceipt{},
			fmt.Errorf("%w: readmission requires the same machine and a distinct request identity", ErrBootSessionReadmission)
	}
	if plan.PlanID != previous.PlanID || plan.ProfileName != previous.ProfileName ||
		plan.OSFamily != previous.OSFamily || plan.Architecture != previous.Architecture {
		return BootSessionAdmission{}, BootSessionReadmissionReceipt{},
			fmt.Errorf("%w: deployment plan drift is not allowed during readmission", ErrBootSessionReadmission)
	}
	if receipt.MediaSHA256 != previous.MediaSHA256 || receipt.MediaSize != previous.MediaSize ||
		receipt.Target != previous.Target {
		return BootSessionAdmission{}, BootSessionReadmissionReceipt{},
			fmt.Errorf("%w: current serving media does not preserve the previous deployment intent", ErrBootSessionReadmission)
	}

	fresh, err := BuildBootSessionAdmission(
		request,
		receipt,
		plan,
		currentServedMediaPayload,
		unattendedBinding,
		unattendedTemplate,
		issuedAtUnix,
	)
	if err != nil {
		return BootSessionAdmission{}, BootSessionReadmissionReceipt{},
			fmt.Errorf("%w: fresh serving evidence validation failed: %v", ErrBootSessionReadmission, err)
	}
	if err := VerifyBootSessionAdmission(
		fresh,
		request,
		receipt,
		plan,
		currentServedMediaPayload,
		unattendedBinding,
		unattendedTemplate,
		issuedAtUnix,
	); err != nil {
		return BootSessionAdmission{}, BootSessionReadmissionReceipt{},
			fmt.Errorf("%w: fresh admission verification failed: %v", ErrBootSessionReadmission, err)
	}
	if fresh.AdmissionID == previous.AdmissionID || fresh.ReplayKey == previous.ReplayKey {
		return BootSessionAdmission{}, BootSessionReadmissionReceipt{},
			fmt.Errorf("%w: fresh admission must have a distinct admission and replay lineage", ErrBootSessionReadmission)
	}
	if fresh.UnattendedBindingVerified != previous.UnattendedBindingVerified ||
		fresh.UnattendedBindingID != previous.UnattendedBindingID ||
		fresh.UnattendedTemplateSHA256 != previous.UnattendedTemplateSHA256 {
		return BootSessionAdmission{}, BootSessionReadmissionReceipt{},
			fmt.Errorf("%w: unattended deployment intent drift is not allowed during readmission", ErrBootSessionReadmission)
	}

	recoveryAction := "consume-fresh-admission-once-or-reconcile"
	digestInput := bootSessionReadmissionReceiptDigest{
		ReceiptVersion:      bootSessionReadmissionReceiptVersion,
		ReconciliationID:    reconciliation.ReconciliationID,
		CandidateReceiptID:  candidate.ReceiptID,
		PreviousAdmissionID: previous.AdmissionID,
		PreviousReplayKey:   previous.ReplayKey,
		FreshAdmissionID:    fresh.AdmissionID,
		FreshReplayKey:      fresh.ReplayKey,
		MachineID:           fresh.MachineID,
		PlanID:              fresh.PlanID,
		ServingReceiptID:    fresh.ServingReceiptID,
		MediaSHA256:         fresh.MediaSHA256,
		MediaSize:           fresh.MediaSize,
		Target:              fresh.Target,
		IssuedAtUnix:        fresh.IssuedAtUnix,
		ExpiresAtUnix:       fresh.ExpiresAtUnix,
		RecoveryAction:      recoveryAction,
	}
	encoded, err := json.Marshal(digestInput)
	if err != nil {
		return BootSessionAdmission{}, BootSessionReadmissionReceipt{},
			fmt.Errorf("%w: encode canonical readmission receipt: %v", ErrBootSessionReadmission, err)
	}
	digest := sha256.Sum256(encoded)

	readmission := BootSessionReadmissionReceipt{
		ReceiptID:                    hex.EncodeToString(digest[:]),
		ReceiptVersion:               bootSessionReadmissionReceiptVersion,
		ReconciliationID:             reconciliation.ReconciliationID,
		CandidateReceiptID:           candidate.ReceiptID,
		PreviousAdmissionID:          previous.AdmissionID,
		PreviousReplayKey:            previous.ReplayKey,
		FreshAdmissionID:             fresh.AdmissionID,
		FreshReplayKey:               fresh.ReplayKey,
		MachineID:                    fresh.MachineID,
		PlanID:                       fresh.PlanID,
		ServingReceiptID:             fresh.ServingReceiptID,
		MediaSHA256:                  fresh.MediaSHA256,
		MediaSize:                    fresh.MediaSize,
		Target:                       fresh.Target,
		IssuedAtUnix:                 fresh.IssuedAtUnix,
		ExpiresAtUnix:                fresh.ExpiresAtUnix,
		FreshServingEvidenceVerified: true,
		FreshAdmissionIssued:         true,
		PreviousAdmissionReusable:    false,
		PreviousReplayAuthorized:     false,
		BootHandoffAuthorized:        false,
		ProvisioningAuthorized:       false,
		SecretInjectionAuthorized:    false,
		HostMutation:                 false,
		NetworkMutation:              false,
		ProductionMutation:           false,
		RecoveryAction:               recoveryAction,
	}
	if err := VerifyBootSessionReadmissionReceipt(readmission, previous, candidate, reconciliation, fresh); err != nil {
		return BootSessionAdmission{}, BootSessionReadmissionReceipt{}, err
	}
	return fresh, readmission, nil
}
