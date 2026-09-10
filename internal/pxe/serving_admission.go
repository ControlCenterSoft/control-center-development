package pxe

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
)

var ErrInstallMediaServingAdmission = errors.New("PXE install media serving admission validation failed")

const installMediaServingAdmissionVersion = "install-media-serving-admission-v1"

// InstallMediaServingAdmission is immutable, typed evidence that one exact
// published PXE media revision may be exposed by a separate bounded serving
// component. It does not start a server, change network configuration, or
// authorize provisioning/host mutation.
type InstallMediaServingAdmission struct {
	ServingAdmissionID            string                        `json:"servingAdmissionId"`
	AdmissionVersion              string                        `json:"admissionVersion"`
	PublicationReceiptID          string                        `json:"publicationReceiptId"`
	PublicationAdmissionID        string                        `json:"publicationAdmissionId"`
	PlanID                        string                        `json:"planId"`
	RecipeID                      string                        `json:"recipeId"`
	MediaSHA256                   string                        `json:"mediaSha256"`
	MediaSize                     int64                         `json:"mediaSize"`
	Target                        InstallMediaPublicationTarget `json:"target"`
	PreviousMediaRecipeID         string                        `json:"previousMediaRecipeId,omitempty"`
	RollbackAction                string                        `json:"rollbackAction"`
	RecoveryAction                string                        `json:"recoveryAction"`
	PublicationReceiptVerified    bool                          `json:"publicationReceiptVerified"`
	CurrentMediaIntegrityVerified bool                          `json:"currentMediaIntegrityVerified"`
	ServingAuthorized             bool                          `json:"servingAuthorized"`
	ProvisioningAuthorized        bool                          `json:"provisioningAuthorized"`
	HostMutation                  bool                          `json:"hostMutation"`
	NetworkMutation               bool                          `json:"networkMutation"`
}

type installMediaServingAdmissionDigest struct {
	AdmissionVersion       string                        `json:"admissionVersion"`
	PublicationReceiptID   string                        `json:"publicationReceiptId"`
	PublicationAdmissionID string                        `json:"publicationAdmissionId"`
	PlanID                 string                        `json:"planId"`
	RecipeID               string                        `json:"recipeId"`
	MediaSHA256            string                        `json:"mediaSha256"`
	MediaSize              int64                         `json:"mediaSize"`
	Target                 InstallMediaPublicationTarget `json:"target"`
	PreviousMediaRecipeID  string                        `json:"previousMediaRecipeId,omitempty"`
	RollbackAction         string                        `json:"rollbackAction"`
	RecoveryAction         string                        `json:"recoveryAction"`
}

// BuildInstallMediaServingAdmission revalidates the complete publication
// lineage and current read-back bytes before granting typed authority to serve
// one exact PXE slot/media revision. Offline media is never admitted here.
func BuildInstallMediaServingAdmission(
	receipt InstallMediaPublicationReceipt,
	publicationAdmission InstallMediaPublicationAdmission,
	plan AdmittedDeploymentPlan,
	recipe InstallMediaRecipe,
	buildReceipt BuiltInstallMediaReceipt,
	sourceMediaPayload []byte,
	currentPublishedMediaPayload []byte,
) (InstallMediaServingAdmission, error) {
	if receipt.Target.Channel != PublicationChannelPXE {
		return InstallMediaServingAdmission{}, fmt.Errorf(
			"%w: serving requires PXE publication channel",
			ErrInstallMediaServingAdmission,
		)
	}
	if err := VerifyInstallMediaPublicationReceipt(
		receipt,
		publicationAdmission,
		plan,
		recipe,
		buildReceipt,
		sourceMediaPayload,
		currentPublishedMediaPayload,
	); err != nil {
		return InstallMediaServingAdmission{}, fmt.Errorf(
			"%w: publication receipt: %v",
			ErrInstallMediaServingAdmission,
			err,
		)
	}
	if len(currentPublishedMediaPayload) == 0 {
		return InstallMediaServingAdmission{}, fmt.Errorf(
			"%w: current published media read-back must be non-empty",
			ErrInstallMediaServingAdmission,
		)
	}
	currentDigest := sha256.Sum256(currentPublishedMediaPayload)
	currentSHA256 := hex.EncodeToString(currentDigest[:])
	if currentSHA256 != receipt.MediaSHA256 || int64(len(currentPublishedMediaPayload)) != receipt.MediaSize {
		return InstallMediaServingAdmission{}, fmt.Errorf(
			"%w: current published media does not match publication receipt",
			ErrInstallMediaServingAdmission,
		)
	}

	rollbackAction := "disable-serving-slot"
	if receipt.PreviousMediaRecipeID != "" {
		rollbackAction = "restore-previous-serving-media"
	}
	recoveryAction := "rebuild-serving-admission-from-current-publication-evidence"
	digestInput := installMediaServingAdmissionDigest{
		AdmissionVersion:       installMediaServingAdmissionVersion,
		PublicationReceiptID:   receipt.PublicationReceiptID,
		PublicationAdmissionID: receipt.AdmissionID,
		PlanID:                 receipt.PlanID,
		RecipeID:               receipt.RecipeID,
		MediaSHA256:            receipt.MediaSHA256,
		MediaSize:              receipt.MediaSize,
		Target:                 receipt.Target,
		PreviousMediaRecipeID:  receipt.PreviousMediaRecipeID,
		RollbackAction:         rollbackAction,
		RecoveryAction:         recoveryAction,
	}
	encoded, err := json.Marshal(digestInput)
	if err != nil {
		return InstallMediaServingAdmission{}, fmt.Errorf(
			"%w: encode canonical serving admission: %v",
			ErrInstallMediaServingAdmission,
			err,
		)
	}
	digest := sha256.Sum256(encoded)

	return InstallMediaServingAdmission{
		ServingAdmissionID:            hex.EncodeToString(digest[:]),
		AdmissionVersion:              installMediaServingAdmissionVersion,
		PublicationReceiptID:          receipt.PublicationReceiptID,
		PublicationAdmissionID:        receipt.AdmissionID,
		PlanID:                        receipt.PlanID,
		RecipeID:                      receipt.RecipeID,
		MediaSHA256:                   receipt.MediaSHA256,
		MediaSize:                     receipt.MediaSize,
		Target:                        receipt.Target,
		PreviousMediaRecipeID:         receipt.PreviousMediaRecipeID,
		RollbackAction:                rollbackAction,
		RecoveryAction:                recoveryAction,
		PublicationReceiptVerified:    true,
		CurrentMediaIntegrityVerified: true,
		ServingAuthorized:             true,
		ProvisioningAuthorized:        false,
		HostMutation:                  false,
		NetworkMutation:               false,
	}, nil
}

// VerifyInstallMediaServingAdmission reconstructs the serving admission from
// current exact publication evidence. Any receipt, target, media, rollback
// lineage, or authority-flag drift fails closed.
func VerifyInstallMediaServingAdmission(
	admission InstallMediaServingAdmission,
	receipt InstallMediaPublicationReceipt,
	publicationAdmission InstallMediaPublicationAdmission,
	plan AdmittedDeploymentPlan,
	recipe InstallMediaRecipe,
	buildReceipt BuiltInstallMediaReceipt,
	sourceMediaPayload []byte,
	currentPublishedMediaPayload []byte,
) error {
	rebuilt, err := BuildInstallMediaServingAdmission(
		receipt,
		publicationAdmission,
		plan,
		recipe,
		buildReceipt,
		sourceMediaPayload,
		currentPublishedMediaPayload,
	)
	if err != nil {
		return err
	}
	if admission.AdmissionVersion != installMediaServingAdmissionVersion ||
		!admission.PublicationReceiptVerified ||
		!admission.CurrentMediaIntegrityVerified ||
		!admission.ServingAuthorized ||
		admission.ProvisioningAuthorized ||
		admission.HostMutation ||
		admission.NetworkMutation {
		return fmt.Errorf(
			"%w: serving admission safety flags do not match",
			ErrInstallMediaServingAdmission,
		)
	}
	normalizedAdmissionID, err := normalizeSHA256(admission.ServingAdmissionID)
	if err != nil || normalizedAdmissionID != admission.ServingAdmissionID {
		return fmt.Errorf(
			"%w: serving admission id is not canonical",
			ErrInstallMediaServingAdmission,
		)
	}
	if !reflect.DeepEqual(rebuilt, admission) {
		return fmt.Errorf(
			"%w: serving admission no longer matches exact publication evidence",
			ErrInstallMediaServingAdmission,
		)
	}
	return nil
}
