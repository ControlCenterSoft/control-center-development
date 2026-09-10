package pxe

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
)

func receiptTestRecipe(t *testing.T, previous string) InstallMediaRecipe {
	t.Helper()
	recipe := InstallMediaRecipe{
		PlanID:              digestString("plan"),
		AdmissionID:         digestString("admission"),
		ManifestID:          digestString("manifest"),
		UnattendedBindingID: digestString("binding"),
		Source: InstallMediaSourceEvidence{
			Kind:   "windows-install-iso",
			SHA256: digestString("source"),
			Size:   123,
		},
		Parameters: InstallMediaBuildParameters{
			RecipeVersion:     "install-media-v1",
			FilesystemPolicy:  "iso9660-udf",
			VolumeLabel:       "CC-WINDOWS",
			TimestampUnix:     0,
			FileOrder:         "lexicographic",
			PermissionsPolicy: "normalized-readonly",
		},
		PreviousMediaRecipeID:     previous,
		RollbackAction:            "none-prebuild",
		RecoveryAction:            "rebuild-media-recipe-from-current-evidence",
		SourceIntegrityVerified:   true,
		BootIntegrityVerified:     true,
		TemplateIntegrityVerified: true,
		Reproducible:              true,
	}
	if previous != "" {
		recipe.RollbackAction = "restore-previous-media-recipe"
	}
	encoded, err := json.Marshal(installMediaRecipeDigest{
		PlanID:                recipe.PlanID,
		AdmissionID:           recipe.AdmissionID,
		ManifestID:            recipe.ManifestID,
		UnattendedBindingID:   recipe.UnattendedBindingID,
		Source:                recipe.Source,
		Parameters:            recipe.Parameters,
		PreviousMediaRecipeID: recipe.PreviousMediaRecipeID,
		RollbackAction:        recipe.RollbackAction,
		RecoveryAction:        recipe.RecoveryAction,
	})
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(encoded)
	recipe.RecipeID = hex.EncodeToString(digest[:])
	return recipe
}

func digestString(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}

func TestBuiltInstallMediaReceiptDeterministicAndNonAuthorizing(t *testing.T) {
	recipe := receiptTestRecipe(t, "")
	payload := []byte("deterministic-built-iso")
	first, err := BuildBuiltInstallMediaReceipt(recipe, payload)
	if err != nil {
		t.Fatal(err)
	}
	second, err := BuildBuiltInstallMediaReceipt(recipe, payload)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("receipt is not deterministic: %#v != %#v", first, second)
	}
	if first.RecipeID != recipe.RecipeID || first.MediaSize != int64(len(payload)) || !first.RecipeIntegrityVerified || !first.MediaIntegrityVerified || !first.ReproducibleRecipe {
		t.Fatalf("receipt did not seal exact build evidence: %#v", first)
	}
	if first.PublicationAuthorized || first.ServingAuthorized || first.ProvisioningAuthorized || first.HostMutation || first.NetworkMutation {
		t.Fatalf("unexpected authority or mutation flag: %#v", first)
	}
	if first.RollbackAction != "discard-unpublished-media" || first.RecoveryAction != "rebuild-media-from-exact-recipe" {
		t.Fatalf("unexpected rollback/recovery contract: %#v", first)
	}
	if err := VerifyBuiltInstallMediaReceipt(first, recipe, payload); err != nil {
		t.Fatalf("valid receipt rejected: %v", err)
	}
}

func TestBuiltInstallMediaReceiptRejectsOutputDrift(t *testing.T) {
	recipe := receiptTestRecipe(t, "")
	receipt, err := BuildBuiltInstallMediaReceipt(recipe, []byte("built-iso"))
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyBuiltInstallMediaReceipt(receipt, recipe, []byte("different-built-iso")); !errors.Is(err, ErrBuiltInstallMediaReceipt) {
		t.Fatalf("output drift: expected fail-closed error, got %v", err)
	}
	if _, err := BuildBuiltInstallMediaReceipt(recipe, nil); !errors.Is(err, ErrBuiltInstallMediaReceipt) {
		t.Fatalf("empty output: expected validation error, got %v", err)
	}
}

func TestBuiltInstallMediaReceiptRejectsRecipeIdentityDrift(t *testing.T) {
	recipe := receiptTestRecipe(t, "")
	payload := []byte("built-iso")
	receipt, err := BuildBuiltInstallMediaReceipt(recipe, payload)
	if err != nil {
		t.Fatal(err)
	}

	tampered := recipe
	tampered.Parameters.VolumeLabel = "CC-WINDOWS-DRIFT"
	if err := VerifyBuiltInstallMediaReceipt(receipt, tampered, payload); !errors.Is(err, ErrBuiltInstallMediaReceipt) {
		t.Fatalf("recipe drift: expected fail-closed error, got %v", err)
	}

	tampered = recipe
	tampered.BuildAuthorized = true
	if _, err := BuildBuiltInstallMediaReceipt(tampered, payload); !errors.Is(err, ErrBuiltInstallMediaReceipt) {
		t.Fatalf("unsafe recipe authority: expected validation error, got %v", err)
	}
}

func TestBuiltInstallMediaReceiptBindsRollbackLineage(t *testing.T) {
	previous := digestString("previous-recipe")
	recipe := receiptTestRecipe(t, previous)
	receipt, err := BuildBuiltInstallMediaReceipt(recipe, []byte("replacement-built-iso"))
	if err != nil {
		t.Fatal(err)
	}
	if receipt.PreviousMediaRecipeID != previous || receipt.RollbackAction != "restore-previous-media-recipe" {
		t.Fatalf("rollback lineage not sealed: %#v", receipt)
	}

	otherRecipe := receiptTestRecipe(t, digestString("other-previous-recipe"))
	if err := VerifyBuiltInstallMediaReceipt(receipt, otherRecipe, []byte("replacement-built-iso")); !errors.Is(err, ErrBuiltInstallMediaReceipt) {
		t.Fatalf("lineage drift: expected fail-closed error, got %v", err)
	}
}

func TestBuiltInstallMediaReceiptRejectsTamperedReceipt(t *testing.T) {
	recipe := receiptTestRecipe(t, "")
	payload := []byte("built-iso")
	receipt, err := BuildBuiltInstallMediaReceipt(recipe, payload)
	if err != nil {
		t.Fatal(err)
	}
	receipt.ServingAuthorized = true
	if err := VerifyBuiltInstallMediaReceipt(receipt, recipe, payload); !errors.Is(err, ErrBuiltInstallMediaReceipt) {
		t.Fatalf("unsafe receipt: expected fail-closed error, got %v", err)
	}
}
