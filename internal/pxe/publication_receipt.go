package pxe

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
)

var ErrInstallMediaPublicationReceipt = errors.New("PXE install media publication receipt validation failed")

const installMediaPublicationReceiptVersion = "install-media-publication-receipt-v1"

// InstallMediaPublicationReceipt is immutable evidence that an external,
// bounded publisher completed one exact InstallMediaPublicationAdmission and
// that read-back bytes at the logical target match the admitted media. The
// receipt grants no authority to publish again, serve, provision, or mutate
// hosts/networks.
type InstallMediaPublicationReceipt struct {
	PublicationReceiptID            string                        `json:"publicationReceiptId"`
	ReceiptVersion                  string                        `json:"receiptVersion"`
	AdmissionID                     string                        `json:"admissionId"`
	PlanID                          string                        `json:"planId"`
	BuildReceiptID                  string                        `json:"buildReceiptId"`
	RecipeID                        string                        `json:"recipeId"`
	MediaSHA256                     string                        `json:"mediaSha256"`
	MediaSize                       int64                         `json:"mediaSize"`
	Target                          InstallMediaPublicationTarget `json:"target"`
	PreviousMediaRecipeID           string                        `json:"previousMediaRecipeId,omitempty"`
	RollbackAction                  string                        `json:"rollbackAction"`
	RecoveryAction                  string                        `json:"recoveryAction"`
	AdmissionIntegrityVerified      bool                          `json:"admissionIntegrityVerified"`
	PublishedMediaIntegrityVerified bool                          `json:"publishedMediaIntegrityVerified"`
	PublicationCompleted            bool                          `json:"publicationCompleted"`
	PublicationAuthorized           bool                          `json:"publicationAuthorized"`
	ServingAuthorized               bool                          `json:"servingAuthorized"`
	ProvisioningAuthorized          bool                          `json:"provisioningAuthorized"`
	HostMutation                    bool                          `json:"hostMutation"`
	NetworkMutation                 bool                          `json:"networkMutation"`
}

type installMediaPublicationReceiptDigest struct {
	ReceiptVersion        string                        `json:"receiptVersion"`
	AdmissionID           string                        `json:"admissionId"`
	PlanID                string                        `json:"planId"`
	BuildReceiptID        string                        `json:"buildReceiptId"`
	RecipeID              string                        `json:"recipeId"`
	MediaSHA256           string                        `json:"mediaSha256"`
	MediaSize             int64                         `json:"mediaSize"`
	Target                InstallMediaPublicationTarget `json:"target"`
	PreviousMediaRecipeID string                        `json:"previousMediaRecipeId,omitempty"`
	RollbackAction        string                        `json:"rollbackAction"`
	RecoveryAction        string                        `json:"recoveryAction"`
}

// BuildInstallMediaPublicationReceipt revalidates the exact admission and its
// upstream plan/build evidence, then verifies publisher read-back bytes. It is
// evidence-only: publication has already occurred outside this contract and no
// serving or provisioning is authorized here.
func BuildInstallMediaPublicationReceipt(
	admission InstallMediaPublicationAdmission,
	plan AdmittedDeploymentPlan,
	recipe InstallMediaRecipe,
	buildReceipt BuiltInstallMediaReceipt,
	sourceMediaPayload []byte,
	publishedMediaPayload []byte,
) (InstallMediaPublicationReceipt, error) {
	if err := VerifyInstallMediaPublicationAdmission(
		admission,
		plan,
		recipe,
		buildReceipt,
		sourceMediaPayload,
	); err != nil {
		return InstallMediaPublicationReceipt{}, fmt.Errorf(
			"%w: publication admission: %v",
			ErrInstallMediaPublicationReceipt,
			err,
		)
	}
	if len(publishedMediaPayload) == 0 {
		return InstallMediaPublicationReceipt{}, fmt.Errorf(
			"%w: published media read-back must be non-empty",
			ErrInstallMediaPublicationReceipt,
		)
	}

	publishedDigest := sha256.Sum256(publishedMediaPayload)
	publishedSHA256 := hex.EncodeToString(publishedDigest[:])
	if publishedSHA256 != admission.MediaSHA256 || int64(len(publishedMediaPayload)) != admission.MediaSize {
		return InstallMediaPublicationReceipt{}, fmt.Errorf(
			"%w: published media does not match exact admitted bytes",
			ErrInstallMediaPublicationReceipt,
		)
	}

	rollbackAction := "keep-current-serving-media"
	recoveryAction := "republish-exact-admitted-media"
	digestInput := installMediaPublicationReceiptDigest{
		ReceiptVersion:        installMediaPublicationReceiptVersion,
		AdmissionID:           admission.AdmissionID,
		PlanID:                admission.PlanID,
		BuildReceiptID:        admission.ReceiptID,
		RecipeID:              admission.RecipeID,
		MediaSHA256:           admission.MediaSHA256,
		MediaSize:             admission.MediaSize,
		Target:                admission.Target,
		PreviousMediaRecipeID: admission.PreviousMediaRecipeID,
		RollbackAction:        rollbackAction,
		RecoveryAction:        recoveryAction,
	}
	encoded, err := json.Marshal(digestInput)
	if err != nil {
		return InstallMediaPublicationReceipt{}, fmt.Errorf(
			"%w: encode canonical publication receipt: %v",
			ErrInstallMediaPublicationReceipt,
			err,
		)
	}
	digest := sha256.Sum256(encoded)

	return InstallMediaPublicationReceipt{
		PublicationReceiptID:            hex.EncodeToString(digest[:]),
		ReceiptVersion:                  installMediaPublicationReceiptVersion,
		AdmissionID:                     admission.AdmissionID,
		PlanID:                          admission.PlanID,
		BuildReceiptID:                  admission.ReceiptID,
		RecipeID:                        admission.RecipeID,
		MediaSHA256:                     admission.MediaSHA256,
		MediaSize:                       admission.MediaSize,
		Target:                          admission.Target,
		PreviousMediaRecipeID:           admission.PreviousMediaRecipeID,
		RollbackAction:                  rollbackAction,
		RecoveryAction:                  recoveryAction,
		AdmissionIntegrityVerified:      true,
		PublishedMediaIntegrityVerified: true,
		PublicationCompleted:            true,
		PublicationAuthorized:           false,
		ServingAuthorized:               false,
		ProvisioningAuthorized:          false,
		HostMutation:                    false,
		NetworkMutation:                 false,
	}, nil
}

// VerifyInstallMediaPublicationReceipt reconstructs the completion receipt
// from current exact evidence. Any admission, target, media, rollback lineage,
// or safety-flag drift fails closed.
func VerifyInstallMediaPublicationReceipt(
	receipt InstallMediaPublicationReceipt,
	admission InstallMediaPublicationAdmission,
	plan AdmittedDeploymentPlan,
	recipe InstallMediaRecipe,
	buildReceipt BuiltInstallMediaReceipt,
	sourceMediaPayload []byte,
	publishedMediaPayload []byte,
) error {
	rebuilt, err := BuildInstallMediaPublicationReceipt(
		admission,
		plan,
		recipe,
		buildReceipt,
		sourceMediaPayload,
		publishedMediaPayload,
	)
	if err != nil {
		return err
	}
	if receipt.ReceiptVersion != installMediaPublicationReceiptVersion ||
		!receipt.AdmissionIntegrityVerified ||
		!receipt.PublishedMediaIntegrityVerified ||
		!receipt.PublicationCompleted ||
		receipt.PublicationAuthorized ||
		receipt.ServingAuthorized ||
		receipt.ProvisioningAuthorized ||
		receipt.HostMutation ||
		receipt.NetworkMutation {
		return fmt.Errorf(
			"%w: publication receipt safety flags do not match",
			ErrInstallMediaPublicationReceipt,
		)
	}
	normalizedReceiptID, err := normalizeSHA256(receipt.PublicationReceiptID)
	if err != nil || normalizedReceiptID != receipt.PublicationReceiptID {
		return fmt.Errorf(
			"%w: publication receipt id is not canonical",
			ErrInstallMediaPublicationReceipt,
		)
	}
	if !reflect.DeepEqual(rebuilt, receipt) {
		return fmt.Errorf(
			"%w: receipt no longer matches exact publication evidence",
			ErrInstallMediaPublicationReceipt,
		)
	}
	return nil
}
