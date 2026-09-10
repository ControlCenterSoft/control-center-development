package pxe

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
)

var ErrBuiltInstallMediaReceipt = errors.New("PXE built install media receipt validation failed")

const builtInstallMediaReceiptVersion = "built-install-media-v1"

// BuiltInstallMediaReceipt is immutable integrity evidence for externally built
// installation media. It binds the exact reproducible recipe to the resulting
// media bytes without authorizing publication, serving, provisioning, or any
// host/network mutation.
type BuiltInstallMediaReceipt struct {
	ReceiptID               string `json:"receiptId"`
	ReceiptVersion          string `json:"receiptVersion"`
	RecipeID                string `json:"recipeId"`
	MediaSHA256             string `json:"mediaSha256"`
	MediaSize               int64  `json:"mediaSize"`
	PreviousMediaRecipeID   string `json:"previousMediaRecipeId,omitempty"`
	RollbackAction          string `json:"rollbackAction"`
	RecoveryAction          string `json:"recoveryAction"`
	RecipeIntegrityVerified bool   `json:"recipeIntegrityVerified"`
	MediaIntegrityVerified  bool   `json:"mediaIntegrityVerified"`
	ReproducibleRecipe      bool   `json:"reproducibleRecipe"`
	PublicationAuthorized   bool   `json:"publicationAuthorized"`
	ServingAuthorized       bool   `json:"servingAuthorized"`
	ProvisioningAuthorized  bool   `json:"provisioningAuthorized"`
	HostMutation            bool   `json:"hostMutation"`
	NetworkMutation         bool   `json:"networkMutation"`
}

type builtInstallMediaReceiptDigest struct {
	ReceiptVersion        string `json:"receiptVersion"`
	RecipeID              string `json:"recipeId"`
	MediaSHA256           string `json:"mediaSha256"`
	MediaSize             int64  `json:"mediaSize"`
	PreviousMediaRecipeID string `json:"previousMediaRecipeId,omitempty"`
	RollbackAction        string `json:"rollbackAction"`
	RecoveryAction        string `json:"recoveryAction"`
}

// BuildBuiltInstallMediaReceipt validates the immutable recipe identity and the
// exact output bytes emitted by an external deterministic builder. It performs
// no build, publication, serving, provisioning, host, or network action.
func BuildBuiltInstallMediaReceipt(recipe InstallMediaRecipe, mediaPayload []byte) (BuiltInstallMediaReceipt, error) {
	if err := validateInstallMediaRecipeForReceipt(recipe); err != nil {
		return BuiltInstallMediaReceipt{}, err
	}
	if len(mediaPayload) == 0 {
		return BuiltInstallMediaReceipt{}, fmt.Errorf("%w: built media must be non-empty", ErrBuiltInstallMediaReceipt)
	}

	mediaDigest := sha256.Sum256(mediaPayload)
	mediaSHA256 := hex.EncodeToString(mediaDigest[:])
	rollbackAction := "discard-unpublished-media"
	if recipe.PreviousMediaRecipeID != "" {
		rollbackAction = "restore-previous-media-recipe"
	}
	recoveryAction := "rebuild-media-from-exact-recipe"

	digestInput := builtInstallMediaReceiptDigest{
		ReceiptVersion:        builtInstallMediaReceiptVersion,
		RecipeID:              recipe.RecipeID,
		MediaSHA256:           mediaSHA256,
		MediaSize:             int64(len(mediaPayload)),
		PreviousMediaRecipeID: recipe.PreviousMediaRecipeID,
		RollbackAction:        rollbackAction,
		RecoveryAction:        recoveryAction,
	}
	encoded, err := json.Marshal(digestInput)
	if err != nil {
		return BuiltInstallMediaReceipt{}, fmt.Errorf("%w: encode canonical receipt: %v", ErrBuiltInstallMediaReceipt, err)
	}
	receiptDigest := sha256.Sum256(encoded)

	return BuiltInstallMediaReceipt{
		ReceiptID:               hex.EncodeToString(receiptDigest[:]),
		ReceiptVersion:          builtInstallMediaReceiptVersion,
		RecipeID:                recipe.RecipeID,
		MediaSHA256:             mediaSHA256,
		MediaSize:               int64(len(mediaPayload)),
		PreviousMediaRecipeID:   recipe.PreviousMediaRecipeID,
		RollbackAction:          rollbackAction,
		RecoveryAction:          recoveryAction,
		RecipeIntegrityVerified: true,
		MediaIntegrityVerified:  true,
		ReproducibleRecipe:      true,
		PublicationAuthorized:   false,
		ServingAuthorized:       false,
		ProvisioningAuthorized:  false,
		HostMutation:            false,
		NetworkMutation:         false,
	}, nil
}

// VerifyBuiltInstallMediaReceipt rebuilds the receipt from the exact current
// recipe and media bytes. Any recipe/output/lineage/safety drift fails closed.
func VerifyBuiltInstallMediaReceipt(receipt BuiltInstallMediaReceipt, recipe InstallMediaRecipe, mediaPayload []byte) error {
	rebuilt, err := BuildBuiltInstallMediaReceipt(recipe, mediaPayload)
	if err != nil {
		return err
	}
	if receipt.ReceiptVersion != builtInstallMediaReceiptVersion || !receipt.RecipeIntegrityVerified || !receipt.MediaIntegrityVerified || !receipt.ReproducibleRecipe || receipt.PublicationAuthorized || receipt.ServingAuthorized || receipt.ProvisioningAuthorized || receipt.HostMutation || receipt.NetworkMutation {
		return fmt.Errorf("%w: receipt safety flags do not match", ErrBuiltInstallMediaReceipt)
	}
	normalizedMediaSHA, err := normalizeSHA256(receipt.MediaSHA256)
	if err != nil || normalizedMediaSHA != receipt.MediaSHA256 {
		return fmt.Errorf("%w: receipt media checksum is not canonical", ErrBuiltInstallMediaReceipt)
	}
	normalizedReceiptID, err := normalizeSHA256(receipt.ReceiptID)
	if err != nil || normalizedReceiptID != receipt.ReceiptID {
		return fmt.Errorf("%w: receipt id is not canonical", ErrBuiltInstallMediaReceipt)
	}
	if !reflect.DeepEqual(rebuilt, receipt) {
		return fmt.Errorf("%w: receipt no longer matches exact build evidence", ErrBuiltInstallMediaReceipt)
	}
	return nil
}

func validateInstallMediaRecipeForReceipt(recipe InstallMediaRecipe) error {
	normalizedRecipeID, err := normalizeSHA256(recipe.RecipeID)
	if err != nil || normalizedRecipeID != recipe.RecipeID {
		return fmt.Errorf("%w: recipe id is not canonical", ErrBuiltInstallMediaReceipt)
	}
	if !recipe.SourceIntegrityVerified || !recipe.BootIntegrityVerified || !recipe.TemplateIntegrityVerified || !recipe.Reproducible || recipe.BuildAuthorized || recipe.ServingAuthorized || recipe.ProvisioningAuthorized || recipe.HostMutation || recipe.NetworkMutation {
		return fmt.Errorf("%w: recipe safety flags do not match", ErrBuiltInstallMediaReceipt)
	}
	if recipe.PreviousMediaRecipeID != "" {
		normalizedPrevious, err := normalizeSHA256(recipe.PreviousMediaRecipeID)
		if err != nil || normalizedPrevious != recipe.PreviousMediaRecipeID || normalizedPrevious == recipe.RecipeID {
			return fmt.Errorf("%w: previous recipe id is invalid", ErrBuiltInstallMediaReceipt)
		}
	}

	digestInput := installMediaRecipeDigest{
		PlanID:                recipe.PlanID,
		AdmissionID:           recipe.AdmissionID,
		ManifestID:            recipe.ManifestID,
		UnattendedBindingID:   recipe.UnattendedBindingID,
		Source:                recipe.Source,
		Parameters:            recipe.Parameters,
		PreviousMediaRecipeID: recipe.PreviousMediaRecipeID,
		RollbackAction:        recipe.RollbackAction,
		RecoveryAction:        recipe.RecoveryAction,
	}
	encoded, err := json.Marshal(digestInput)
	if err != nil {
		return fmt.Errorf("%w: encode canonical recipe identity: %v", ErrBuiltInstallMediaReceipt, err)
	}
	digest := sha256.Sum256(encoded)
	if hex.EncodeToString(digest[:]) != recipe.RecipeID {
		return fmt.Errorf("%w: recipe identity drift", ErrBuiltInstallMediaReceipt)
	}
	return nil
}
