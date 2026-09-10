package pxe

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

// VerifyBootSessionReadmissionReceipt verifies sealed lineage and safety flags.
// It does not replace VerifyBootSessionAdmission when current serving bytes are
// later consumed; consumers must revalidate the fresh admission at handoff.
func VerifyBootSessionReadmissionReceipt(
	receipt BootSessionReadmissionReceipt,
	previous BootSessionAdmission,
	candidate BootSessionConsumptionReceipt,
	reconciliation BootSessionConsumptionReconciliation,
	fresh BootSessionAdmission,
) error {
	if err := VerifyBootSessionConsumptionReconciliation(reconciliation); err != nil {
		return fmt.Errorf("%w: reconciliation evidence: %v", ErrBootSessionReadmission, err)
	}
	if err := validatePreviousBootSessionAdmissionForReadmission(previous); err != nil {
		return err
	}
	candidateRequest := BootSessionConsumptionRequest{
		AttemptID:      candidate.AttemptID,
		ConsumerID:     candidate.ConsumerID,
		ConsumedAtUnix: candidate.ConsumedAtUnix,
	}
	if err := VerifyBootSessionConsumptionReceipt(candidate, candidateRequest, previous); err != nil {
		return fmt.Errorf("%w: candidate consumption evidence: %v", ErrBootSessionReadmission, err)
	}
	if receipt.ReceiptVersion != bootSessionReadmissionReceiptVersion ||
		!bootSessionConsumptionCanonicalSHA256(receipt.ReceiptID) ||
		!receipt.FreshServingEvidenceVerified || !receipt.FreshAdmissionIssued ||
		receipt.PreviousAdmissionReusable || receipt.PreviousReplayAuthorized ||
		receipt.BootHandoffAuthorized || receipt.ProvisioningAuthorized ||
		receipt.SecretInjectionAuthorized || receipt.HostMutation || receipt.NetworkMutation ||
		receipt.ProductionMutation || receipt.RecoveryAction != "consume-fresh-admission-once-or-reconcile" {
		return fmt.Errorf("%w: readmission receipt safety contract drift", ErrBootSessionReadmission)
	}
	for _, value := range []string{
		receipt.ReconciliationID,
		receipt.CandidateReceiptID,
		receipt.PreviousAdmissionID,
		receipt.PreviousReplayKey,
		receipt.FreshAdmissionID,
		receipt.FreshReplayKey,
		receipt.PlanID,
		receipt.ServingReceiptID,
		receipt.MediaSHA256,
	} {
		if !bootSessionConsumptionCanonicalSHA256(value) {
			return fmt.Errorf("%w: readmission receipt contains non-canonical digest identity", ErrBootSessionReadmission)
		}
	}
	if reconciliation.State != BootSessionConsumptionReconciledDefinitelyAbsent ||
		!reconciliation.OldAdmissionExpired || !reconciliation.FreshAdmissionRequired ||
		!reconciliation.FreshServingEvidenceRequired || !reconciliation.RecoveryRequired ||
		reconciliation.RecoveryAction != "reissue-new-session-from-fresh-serving-evidence" ||
		reconciliation.ObservedReceiptID != "" || reconciliation.AdmissionID != previous.AdmissionID ||
		reconciliation.ReplayKey != previous.ReplayKey || reconciliation.CandidateReceiptID != candidate.ReceiptID ||
		candidate.AdmissionID != previous.AdmissionID || candidate.ReplayKey != previous.ReplayKey ||
		reconciliation.AdmissionExpiresAtUnix != previous.ExpiresAtUnix {
		return fmt.Errorf("%w: reconciliation no longer permits this readmission", ErrBootSessionReadmission)
	}
	if fresh.AdmissionVersion != bootSessionAdmissionVersion || fresh.AdmissionID == previous.AdmissionID ||
		fresh.ReplayKey == previous.ReplayKey || fresh.MachineID != previous.MachineID ||
		fresh.PlanID != previous.PlanID || fresh.MediaSHA256 != previous.MediaSHA256 ||
		fresh.MediaSize != previous.MediaSize || fresh.Target != previous.Target ||
		fresh.IssuedAtUnix < reconciliation.ObservedAtUnix || fresh.IssuedAtUnix < previous.ExpiresAtUnix ||
		fresh.IssuedAtUnix <= previous.IssuedAtUnix || fresh.ExpiresAtUnix <= fresh.IssuedAtUnix ||
		fresh.MaxBootAttempts != 1 || !fresh.SingleUse || !fresh.AtomicConsumeRequired ||
		!fresh.ServingReceiptVerified || !fresh.BootAuthorized || fresh.ProvisioningAuthorized ||
		fresh.SecretInjectionAuthorized || fresh.HostMutation || fresh.NetworkMutation {
		return fmt.Errorf("%w: fresh admission no longer matches bounded readmission semantics", ErrBootSessionReadmission)
	}
	if fresh.ExpiresAtUnix-fresh.IssuedAtUnix > previous.ExpiresAtUnix-previous.IssuedAtUnix ||
		fresh.UnattendedBindingVerified != previous.UnattendedBindingVerified ||
		fresh.UnattendedBindingID != previous.UnattendedBindingID ||
		fresh.UnattendedTemplateSHA256 != previous.UnattendedTemplateSHA256 {
		return fmt.Errorf("%w: fresh admission widened or changed deployment intent", ErrBootSessionReadmission)
	}
	if receipt.ReconciliationID != reconciliation.ReconciliationID ||
		receipt.CandidateReceiptID != candidate.ReceiptID ||
		receipt.PreviousAdmissionID != previous.AdmissionID || receipt.PreviousReplayKey != previous.ReplayKey ||
		receipt.FreshAdmissionID != fresh.AdmissionID || receipt.FreshReplayKey != fresh.ReplayKey ||
		receipt.MachineID != fresh.MachineID || receipt.PlanID != fresh.PlanID ||
		receipt.ServingReceiptID != fresh.ServingReceiptID || receipt.MediaSHA256 != fresh.MediaSHA256 ||
		receipt.MediaSize != fresh.MediaSize || receipt.Target != fresh.Target ||
		receipt.IssuedAtUnix != fresh.IssuedAtUnix || receipt.ExpiresAtUnix != fresh.ExpiresAtUnix {
		return fmt.Errorf("%w: readmission receipt lineage drift", ErrBootSessionReadmission)
	}

	digestInput := bootSessionReadmissionReceiptDigest{
		ReceiptVersion:      receipt.ReceiptVersion,
		ReconciliationID:    receipt.ReconciliationID,
		CandidateReceiptID:  receipt.CandidateReceiptID,
		PreviousAdmissionID: receipt.PreviousAdmissionID,
		PreviousReplayKey:   receipt.PreviousReplayKey,
		FreshAdmissionID:    receipt.FreshAdmissionID,
		FreshReplayKey:      receipt.FreshReplayKey,
		MachineID:           receipt.MachineID,
		PlanID:              receipt.PlanID,
		ServingReceiptID:    receipt.ServingReceiptID,
		MediaSHA256:         receipt.MediaSHA256,
		MediaSize:           receipt.MediaSize,
		Target:              receipt.Target,
		IssuedAtUnix:        receipt.IssuedAtUnix,
		ExpiresAtUnix:       receipt.ExpiresAtUnix,
		RecoveryAction:      receipt.RecoveryAction,
	}
	encoded, err := json.Marshal(digestInput)
	if err != nil {
		return fmt.Errorf("%w: encode readmission verification evidence: %v", ErrBootSessionReadmission, err)
	}
	digest := sha256.Sum256(encoded)
	if receipt.ReceiptID != hex.EncodeToString(digest[:]) {
		return fmt.Errorf("%w: readmission receipt digest drift", ErrBootSessionReadmission)
	}
	return nil
}

func validatePreviousBootSessionAdmissionForReadmission(previous BootSessionAdmission) error {
	if previous.AdmissionVersion != bootSessionAdmissionVersion || previous.ExpiresAtUnix <= previous.IssuedAtUnix ||
		previous.ExpiresAtUnix-previous.IssuedAtUnix > maxBootSessionTTLSeconds ||
		previous.MaxBootAttempts != 1 || !previous.SingleUse || !previous.AtomicConsumeRequired ||
		!previous.ServingReceiptVerified || !previous.BootAuthorized || previous.ProvisioningAuthorized ||
		previous.SecretInjectionAuthorized || previous.HostMutation || previous.NetworkMutation ||
		previous.RollbackAction != "expire-session-before-provisioning" ||
		previous.RecoveryAction != "reissue-from-fresh-serving-evidence" || previous.MediaSize <= 0 {
		return fmt.Errorf("%w: previous admission safety contract drift", ErrBootSessionReadmission)
	}
	if !validBootSessionIdentity(previous.RequestID) || !validBootSessionIdentity(previous.MachineID) {
		return fmt.Errorf("%w: previous admission has invalid request or machine identity", ErrBootSessionReadmission)
	}
	if err := validateInstallMediaPublicationTarget(previous.Target); err != nil {
		return fmt.Errorf("%w: previous publication target: %v", ErrBootSessionReadmission, err)
	}
	for _, value := range []string{
		previous.AdmissionID,
		previous.ReplayKey,
		previous.PlanID,
		previous.ServingReceiptID,
		previous.ServingAdmissionID,
		previous.MediaSHA256,
	} {
		if !bootSessionConsumptionCanonicalSHA256(value) {
			return fmt.Errorf("%w: previous admission contains non-canonical digest identity", ErrBootSessionReadmission)
		}
	}
	if previous.UnattendedBindingVerified {
		if !bootSessionConsumptionCanonicalSHA256(previous.UnattendedBindingID) ||
			!bootSessionConsumptionCanonicalSHA256(previous.UnattendedTemplateSHA256) {
			return fmt.Errorf("%w: previous unattended evidence is not canonical", ErrBootSessionReadmission)
		}
	} else if previous.UnattendedBindingID != "" || previous.UnattendedTemplateSHA256 != "" {
		return fmt.Errorf("%w: previous unattended evidence is inconsistent", ErrBootSessionReadmission)
	}

	replayInput := bootSessionReplayDigest{
		Version:          "boot-session-replay-v1",
		RequestID:        previous.RequestID,
		MachineID:        previous.MachineID,
		PlanID:           previous.PlanID,
		ServingReceiptID: previous.ServingReceiptID,
		IssuedAtUnix:     previous.IssuedAtUnix,
	}
	replayEncoded, err := json.Marshal(replayInput)
	if err != nil {
		return fmt.Errorf("%w: encode previous replay identity: %v", ErrBootSessionReadmission, err)
	}
	replayDigest := sha256.Sum256(replayEncoded)
	if previous.ReplayKey != hex.EncodeToString(replayDigest[:]) {
		return fmt.Errorf("%w: previous replay identity drift", ErrBootSessionReadmission)
	}

	admissionInput := bootSessionAdmissionDigest{
		AdmissionVersion:         previous.AdmissionVersion,
		RequestID:                previous.RequestID,
		ReplayKey:                previous.ReplayKey,
		MachineID:                previous.MachineID,
		ProfileName:              previous.ProfileName,
		OSFamily:                 previous.OSFamily,
		Architecture:             previous.Architecture,
		PlanID:                   previous.PlanID,
		ServingReceiptID:         previous.ServingReceiptID,
		ServingAdmissionID:       previous.ServingAdmissionID,
		MediaSHA256:              previous.MediaSHA256,
		MediaSize:                previous.MediaSize,
		Target:                   previous.Target,
		UnattendedBindingID:      previous.UnattendedBindingID,
		UnattendedTemplateSHA256: previous.UnattendedTemplateSHA256,
		IssuedAtUnix:             previous.IssuedAtUnix,
		ExpiresAtUnix:            previous.ExpiresAtUnix,
		RollbackAction:           previous.RollbackAction,
		RecoveryAction:           previous.RecoveryAction,
	}
	admissionEncoded, err := json.Marshal(admissionInput)
	if err != nil {
		return fmt.Errorf("%w: encode previous admission identity: %v", ErrBootSessionReadmission, err)
	}
	admissionDigest := sha256.Sum256(admissionEncoded)
	if previous.AdmissionID != hex.EncodeToString(admissionDigest[:]) {
		return fmt.Errorf("%w: previous admission identity drift", ErrBootSessionReadmission)
	}
	return nil
}
