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

var ErrInstallMediaPublicationAdmission = errors.New("PXE install media publication admission validation failed")

type InstallMediaPublicationChannel string

const (
	PublicationChannelPXE     InstallMediaPublicationChannel = "pxe-install-media"
	PublicationChannelOffline InstallMediaPublicationChannel = "offline-install-media"

	installMediaPublicationAdmissionVersion = "install-media-publication-v1"
)

// InstallMediaPublicationTarget is a logical, infrastructure-neutral target.
// Slot is an opaque allowlisted identifier, never a caller-selected path, URL,
// command, protocol endpoint, or host name.
type InstallMediaPublicationTarget struct {
	Channel InstallMediaPublicationChannel `json:"channel"`
	Slot    string                         `json:"slot"`
}

// InstallMediaPublicationAdmission is immutable evidence that one exact media
// receipt may be handed to a separate bounded publisher for one logical target.
// It does not publish, serve, provision, or mutate hosts/networks itself.
type InstallMediaPublicationAdmission struct {
	AdmissionID              string                        `json:"admissionId"`
	AdmissionVersion         string                        `json:"admissionVersion"`
	PlanID                   string                        `json:"planId"`
	ReceiptID                string                        `json:"receiptId"`
	RecipeID                 string                        `json:"recipeId"`
	MediaSHA256              string                        `json:"mediaSha256"`
	MediaSize                int64                         `json:"mediaSize"`
	Target                   InstallMediaPublicationTarget `json:"target"`
	PreviousMediaRecipeID    string                        `json:"previousMediaRecipeId,omitempty"`
	RollbackAction           string                        `json:"rollbackAction"`
	RecoveryAction           string                        `json:"recoveryAction"`
	PlanIntegrityVerified    bool                          `json:"planIntegrityVerified"`
	ReceiptIntegrityVerified bool                          `json:"receiptIntegrityVerified"`
	MediaIntegrityVerified   bool                          `json:"mediaIntegrityVerified"`
	PublicationAuthorized    bool                          `json:"publicationAuthorized"`
	ServingAuthorized        bool                          `json:"servingAuthorized"`
	ProvisioningAuthorized   bool                          `json:"provisioningAuthorized"`
	HostMutation             bool                          `json:"hostMutation"`
	NetworkMutation          bool                          `json:"networkMutation"`
}

type installMediaPublicationAdmissionDigest struct {
	AdmissionVersion      string                        `json:"admissionVersion"`
	PlanID                string                        `json:"planId"`
	ReceiptID             string                        `json:"receiptId"`
	RecipeID              string                        `json:"recipeId"`
	MediaSHA256           string                        `json:"mediaSha256"`
	MediaSize             int64                         `json:"mediaSize"`
	Target                InstallMediaPublicationTarget `json:"target"`
	PreviousMediaRecipeID string                        `json:"previousMediaRecipeId,omitempty"`
	RollbackAction        string                        `json:"rollbackAction"`
	RecoveryAction        string                        `json:"recoveryAction"`
}

// BuildInstallMediaPublicationAdmission revalidates exact plan, recipe,
// receipt, media bytes, and a typed logical publication target before emitting
// bounded evidence for a later publisher. No publication or serving occurs.
func BuildInstallMediaPublicationAdmission(plan AdmittedDeploymentPlan, recipe InstallMediaRecipe, receipt BuiltInstallMediaReceipt, mediaPayload []byte, target InstallMediaPublicationTarget) (InstallMediaPublicationAdmission, error) {
	if err := validateAdmittedPlanForMediaPublication(plan); err != nil {
		return InstallMediaPublicationAdmission{}, err
	}
	if recipe.PlanID != plan.PlanID {
		return InstallMediaPublicationAdmission{}, fmt.Errorf("%w: recipe is not bound to the exact deployment plan", ErrInstallMediaPublicationAdmission)
	}
	if err := VerifyBuiltInstallMediaReceipt(receipt, recipe, mediaPayload); err != nil {
		return InstallMediaPublicationAdmission{}, fmt.Errorf("%w: built media receipt: %v", ErrInstallMediaPublicationAdmission, err)
	}
	if receipt.RecipeID != recipe.RecipeID || receipt.MediaSize <= 0 {
		return InstallMediaPublicationAdmission{}, fmt.Errorf("%w: receipt identity is incomplete", ErrInstallMediaPublicationAdmission)
	}
	if err := validateInstallMediaPublicationTarget(target); err != nil {
		return InstallMediaPublicationAdmission{}, err
	}

	rollbackAction := "keep-current-serving-media"
	recoveryAction := "rebuild-publication-admission-from-current-evidence"
	digestInput := installMediaPublicationAdmissionDigest{
		AdmissionVersion:      installMediaPublicationAdmissionVersion,
		PlanID:                plan.PlanID,
		ReceiptID:             receipt.ReceiptID,
		RecipeID:              receipt.RecipeID,
		MediaSHA256:           receipt.MediaSHA256,
		MediaSize:             receipt.MediaSize,
		Target:                target,
		PreviousMediaRecipeID: receipt.PreviousMediaRecipeID,
		RollbackAction:        rollbackAction,
		RecoveryAction:        recoveryAction,
	}
	encoded, err := json.Marshal(digestInput)
	if err != nil {
		return InstallMediaPublicationAdmission{}, fmt.Errorf("%w: encode canonical admission: %v", ErrInstallMediaPublicationAdmission, err)
	}
	digest := sha256.Sum256(encoded)

	return InstallMediaPublicationAdmission{
		AdmissionID:              hex.EncodeToString(digest[:]),
		AdmissionVersion:         installMediaPublicationAdmissionVersion,
		PlanID:                   plan.PlanID,
		ReceiptID:                receipt.ReceiptID,
		RecipeID:                 receipt.RecipeID,
		MediaSHA256:              receipt.MediaSHA256,
		MediaSize:                receipt.MediaSize,
		Target:                   target,
		PreviousMediaRecipeID:    receipt.PreviousMediaRecipeID,
		RollbackAction:           rollbackAction,
		RecoveryAction:           recoveryAction,
		PlanIntegrityVerified:    true,
		ReceiptIntegrityVerified: true,
		MediaIntegrityVerified:   true,
		PublicationAuthorized:    true,
		ServingAuthorized:        false,
		ProvisioningAuthorized:   false,
		HostMutation:             false,
		NetworkMutation:          false,
	}, nil
}

// VerifyInstallMediaPublicationAdmission reconstructs the bounded admission
// from current immutable evidence. Any target, plan, receipt, media, lineage,
// or safety-flag drift fails closed.
func VerifyInstallMediaPublicationAdmission(admission InstallMediaPublicationAdmission, plan AdmittedDeploymentPlan, recipe InstallMediaRecipe, receipt BuiltInstallMediaReceipt, mediaPayload []byte) error {
	rebuilt, err := BuildInstallMediaPublicationAdmission(plan, recipe, receipt, mediaPayload, admission.Target)
	if err != nil {
		return err
	}
	if admission.AdmissionVersion != installMediaPublicationAdmissionVersion || !admission.PlanIntegrityVerified || !admission.ReceiptIntegrityVerified || !admission.MediaIntegrityVerified || !admission.PublicationAuthorized || admission.ServingAuthorized || admission.ProvisioningAuthorized || admission.HostMutation || admission.NetworkMutation {
		return fmt.Errorf("%w: admission safety flags do not match", ErrInstallMediaPublicationAdmission)
	}
	normalizedAdmissionID, err := normalizeSHA256(admission.AdmissionID)
	if err != nil || normalizedAdmissionID != admission.AdmissionID {
		return fmt.Errorf("%w: admission id is not canonical", ErrInstallMediaPublicationAdmission)
	}
	if !reflect.DeepEqual(rebuilt, admission) {
		return fmt.Errorf("%w: admission no longer matches exact publication evidence", ErrInstallMediaPublicationAdmission)
	}
	return nil
}

func validateAdmittedPlanForMediaPublication(plan AdmittedDeploymentPlan) error {
	normalizedPlanID, err := normalizeSHA256(plan.PlanID)
	if err != nil || normalizedPlanID != plan.PlanID {
		return fmt.Errorf("%w: deployment plan id is not canonical", ErrInstallMediaPublicationAdmission)
	}
	if !plan.ArtifactIntegrityVerified || plan.ServingAuthorized || plan.HostMutation || plan.NetworkMutation {
		return fmt.Errorf("%w: deployment plan safety flags do not match", ErrInstallMediaPublicationAdmission)
	}
	digestInput := admittedDeploymentPlanDigest{
		ProfileName:        plan.ProfileName,
		OSFamily:           plan.OSFamily,
		Architecture:       plan.Architecture,
		AdmissionID:        plan.AdmissionID,
		ManifestID:         plan.ManifestID,
		RollbackManifestID: plan.RollbackManifestID,
		Steps:              plan.Steps,
	}
	encoded, err := json.Marshal(digestInput)
	if err != nil {
		return fmt.Errorf("%w: encode deployment plan identity: %v", ErrInstallMediaPublicationAdmission, err)
	}
	digest := sha256.Sum256(encoded)
	if hex.EncodeToString(digest[:]) != plan.PlanID {
		return fmt.Errorf("%w: deployment plan identity drift", ErrInstallMediaPublicationAdmission)
	}
	return nil
}

func validateInstallMediaPublicationTarget(target InstallMediaPublicationTarget) error {
	if target.Channel != PublicationChannelPXE && target.Channel != PublicationChannelOffline {
		return fmt.Errorf("%w: unsupported publication channel %q", ErrInstallMediaPublicationAdmission, target.Channel)
	}
	if !validInstallMediaPublicationSlot(target.Slot) {
		return fmt.Errorf("%w: invalid publication slot %q", ErrInstallMediaPublicationAdmission, target.Slot)
	}
	return nil
}

func validInstallMediaPublicationSlot(slot string) bool {
	if len(slot) == 0 || len(slot) > 64 || strings.Contains(slot, "..") || !isPublicationSlotAlphaNum(slot[0]) || !isPublicationSlotAlphaNum(slot[len(slot)-1]) {
		return false
	}
	for i := 0; i < len(slot); i++ {
		ch := slot[i]
		if isPublicationSlotAlphaNum(ch) || ch == '-' || ch == '.' {
			continue
		}
		return false
	}
	return true
}

func isPublicationSlotAlphaNum(ch byte) bool {
	return ch >= 'a' && ch <= 'z' || ch >= '0' && ch <= '9'
}
