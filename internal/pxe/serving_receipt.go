package pxe

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
)

var ErrInstallMediaServingReceipt = errors.New("PXE install media serving receipt validation failed")

const installMediaServingReceiptVersion = "install-media-serving-receipt-v1"

// InstallMediaServingStateObservation is a bounded, infrastructure-neutral
// observation emitted by the serving component after it activates one logical
// slot. It carries no path, URL, listener address, command, or execution input.
type InstallMediaServingStateObservation struct {
	ServingAdmissionID string                        `json:"servingAdmissionId"`
	Target             InstallMediaPublicationTarget `json:"target"`
	MediaSHA256        string                        `json:"mediaSha256"`
	MediaSize          int64                         `json:"mediaSize"`
	ServingActive      bool                          `json:"servingActive"`
}

// InstallMediaServingReceipt is immutable proof that the bounded serving
// component reports one exact ServingAdmission as active and that read-back
// bytes for that logical slot still match the admitted media. The receipt is
// evidence-only and grants no boot, provisioning, host, or network authority.
type InstallMediaServingReceipt struct {
	ServingReceiptID             string                        `json:"servingReceiptId"`
	ReceiptVersion               string                        `json:"receiptVersion"`
	ServingAdmissionID           string                        `json:"servingAdmissionId"`
	PublicationReceiptID         string                        `json:"publicationReceiptId"`
	PublicationAdmissionID       string                        `json:"publicationAdmissionId"`
	PlanID                       string                        `json:"planId"`
	RecipeID                     string                        `json:"recipeId"`
	MediaSHA256                  string                        `json:"mediaSha256"`
	MediaSize                    int64                         `json:"mediaSize"`
	Target                       InstallMediaPublicationTarget `json:"target"`
	PreviousMediaRecipeID        string                        `json:"previousMediaRecipeId,omitempty"`
	RollbackAction               string                        `json:"rollbackAction"`
	RecoveryAction               string                        `json:"recoveryAction"`
	ServingAdmissionVerified     bool                          `json:"servingAdmissionVerified"`
	ActiveSlotVerified           bool                          `json:"activeSlotVerified"`
	ActiveMediaIntegrityVerified bool                          `json:"activeMediaIntegrityVerified"`
	ServingActive                bool                          `json:"servingActive"`
	ServingAuthorized            bool                          `json:"servingAuthorized"`
	BootAuthorized               bool                          `json:"bootAuthorized"`
	ProvisioningAuthorized       bool                          `json:"provisioningAuthorized"`
	HostMutation                 bool                          `json:"hostMutation"`
	NetworkMutation              bool                          `json:"networkMutation"`
}

type installMediaServingReceiptDigest struct {
	ReceiptVersion         string                        `json:"receiptVersion"`
	ServingAdmissionID     string                        `json:"servingAdmissionId"`
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

// BuildInstallMediaServingReceipt revalidates the complete serving admission
// lineage, exact active-slot observation, and current served bytes before
// sealing immutable completion evidence. It does not activate a slot or boot a
// client.
func BuildInstallMediaServingReceipt(
	admission InstallMediaServingAdmission,
	observation InstallMediaServingStateObservation,
	publicationReceipt InstallMediaPublicationReceipt,
	publicationAdmission InstallMediaPublicationAdmission,
	plan AdmittedDeploymentPlan,
	recipe InstallMediaRecipe,
	buildReceipt BuiltInstallMediaReceipt,
	sourceMediaPayload []byte,
	currentPublishedMediaPayload []byte,
	currentServedMediaPayload []byte,
) (InstallMediaServingReceipt, error) {
	if err := VerifyInstallMediaServingAdmission(
		admission,
		publicationReceipt,
		publicationAdmission,
		plan,
		recipe,
		buildReceipt,
		sourceMediaPayload,
		currentPublishedMediaPayload,
	); err != nil {
		return InstallMediaServingReceipt{}, fmt.Errorf(
			"%w: serving admission: %v",
			ErrInstallMediaServingReceipt,
			err,
		)
	}
	if !observation.ServingActive {
		return InstallMediaServingReceipt{}, fmt.Errorf(
			"%w: serving slot is not active",
			ErrInstallMediaServingReceipt,
		)
	}
	if observation.ServingAdmissionID != admission.ServingAdmissionID ||
		observation.Target != admission.Target {
		return InstallMediaServingReceipt{}, fmt.Errorf(
			"%w: active slot is not bound to the exact serving admission",
			ErrInstallMediaServingReceipt,
		)
	}
	normalizedObservedSHA, err := normalizeSHA256(observation.MediaSHA256)
	if err != nil || normalizedObservedSHA != observation.MediaSHA256 {
		return InstallMediaServingReceipt{}, fmt.Errorf(
			"%w: observed media digest is not canonical",
			ErrInstallMediaServingReceipt,
		)
	}
	if observation.MediaSHA256 != admission.MediaSHA256 ||
		observation.MediaSize != admission.MediaSize || observation.MediaSize <= 0 {
		return InstallMediaServingReceipt{}, fmt.Errorf(
			"%w: active slot media metadata does not match serving admission",
			ErrInstallMediaServingReceipt,
		)
	}
	if len(currentServedMediaPayload) == 0 {
		return InstallMediaServingReceipt{}, fmt.Errorf(
			"%w: active slot media read-back must be non-empty",
			ErrInstallMediaServingReceipt,
		)
	}
	servedDigest := sha256.Sum256(currentServedMediaPayload)
	servedSHA256 := hex.EncodeToString(servedDigest[:])
	if servedSHA256 != observation.MediaSHA256 ||
		int64(len(currentServedMediaPayload)) != observation.MediaSize {
		return InstallMediaServingReceipt{}, fmt.Errorf(
			"%w: active slot media read-back does not match observation",
			ErrInstallMediaServingReceipt,
		)
	}

	recoveryAction := "rebuild-serving-receipt-from-current-active-slot-evidence"
	digestInput := installMediaServingReceiptDigest{
		ReceiptVersion:         installMediaServingReceiptVersion,
		ServingAdmissionID:     admission.ServingAdmissionID,
		PublicationReceiptID:   admission.PublicationReceiptID,
		PublicationAdmissionID: admission.PublicationAdmissionID,
		PlanID:                 admission.PlanID,
		RecipeID:               admission.RecipeID,
		MediaSHA256:            admission.MediaSHA256,
		MediaSize:              admission.MediaSize,
		Target:                 admission.Target,
		PreviousMediaRecipeID:  admission.PreviousMediaRecipeID,
		RollbackAction:         admission.RollbackAction,
		RecoveryAction:         recoveryAction,
	}
	encoded, err := json.Marshal(digestInput)
	if err != nil {
		return InstallMediaServingReceipt{}, fmt.Errorf(
			"%w: encode canonical serving receipt: %v",
			ErrInstallMediaServingReceipt,
			err,
		)
	}
	digest := sha256.Sum256(encoded)

	return InstallMediaServingReceipt{
		ServingReceiptID:             hex.EncodeToString(digest[:]),
		ReceiptVersion:               installMediaServingReceiptVersion,
		ServingAdmissionID:           admission.ServingAdmissionID,
		PublicationReceiptID:         admission.PublicationReceiptID,
		PublicationAdmissionID:       admission.PublicationAdmissionID,
		PlanID:                       admission.PlanID,
		RecipeID:                     admission.RecipeID,
		MediaSHA256:                  admission.MediaSHA256,
		MediaSize:                    admission.MediaSize,
		Target:                       admission.Target,
		PreviousMediaRecipeID:        admission.PreviousMediaRecipeID,
		RollbackAction:               admission.RollbackAction,
		RecoveryAction:               recoveryAction,
		ServingAdmissionVerified:     true,
		ActiveSlotVerified:           true,
		ActiveMediaIntegrityVerified: true,
		ServingActive:                true,
		ServingAuthorized:            false,
		BootAuthorized:               false,
		ProvisioningAuthorized:       false,
		HostMutation:                 false,
		NetworkMutation:              false,
	}, nil
}

// VerifyInstallMediaServingReceipt reconstructs the completion evidence from
// current exact inputs. Any admission, slot, media, rollback-lineage, or safety
// flag drift fails closed.
func VerifyInstallMediaServingReceipt(
	receipt InstallMediaServingReceipt,
	admission InstallMediaServingAdmission,
	observation InstallMediaServingStateObservation,
	publicationReceipt InstallMediaPublicationReceipt,
	publicationAdmission InstallMediaPublicationAdmission,
	plan AdmittedDeploymentPlan,
	recipe InstallMediaRecipe,
	buildReceipt BuiltInstallMediaReceipt,
	sourceMediaPayload []byte,
	currentPublishedMediaPayload []byte,
	currentServedMediaPayload []byte,
) error {
	rebuilt, err := BuildInstallMediaServingReceipt(
		admission,
		observation,
		publicationReceipt,
		publicationAdmission,
		plan,
		recipe,
		buildReceipt,
		sourceMediaPayload,
		currentPublishedMediaPayload,
		currentServedMediaPayload,
	)
	if err != nil {
		return err
	}
	if receipt.ReceiptVersion != installMediaServingReceiptVersion ||
		!receipt.ServingAdmissionVerified ||
		!receipt.ActiveSlotVerified ||
		!receipt.ActiveMediaIntegrityVerified ||
		!receipt.ServingActive ||
		receipt.ServingAuthorized ||
		receipt.BootAuthorized ||
		receipt.ProvisioningAuthorized ||
		receipt.HostMutation ||
		receipt.NetworkMutation {
		return fmt.Errorf(
			"%w: serving receipt safety flags do not match",
			ErrInstallMediaServingReceipt,
		)
	}
	normalizedReceiptID, err := normalizeSHA256(receipt.ServingReceiptID)
	if err != nil || normalizedReceiptID != receipt.ServingReceiptID {
		return fmt.Errorf(
			"%w: serving receipt id is not canonical",
			ErrInstallMediaServingReceipt,
		)
	}
	if !reflect.DeepEqual(rebuilt, receipt) {
		return fmt.Errorf(
			"%w: serving receipt no longer matches exact active-slot evidence",
			ErrInstallMediaServingReceipt,
		)
	}
	return nil
}
