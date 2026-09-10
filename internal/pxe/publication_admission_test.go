package pxe

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
)

func publicationTestPlan(t *testing.T) AdmittedDeploymentPlan {
	t.Helper()
	plan := AdmittedDeploymentPlan{
		ProfileName:               "windows-amd64",
		OSFamily:                  "windows",
		Architecture:              "amd64",
		AdmissionID:               digestString("boot-admission"),
		ManifestID:                digestString("manifest"),
		RollbackManifestID:        digestString("previous-manifest"),
		Steps:                     nil,
		ArtifactIntegrityVerified: true,
	}
	encoded, err := json.Marshal(admittedDeploymentPlanDigest{
		ProfileName:        plan.ProfileName,
		OSFamily:           plan.OSFamily,
		Architecture:       plan.Architecture,
		AdmissionID:        plan.AdmissionID,
		ManifestID:         plan.ManifestID,
		RollbackManifestID: plan.RollbackManifestID,
		Steps:              plan.Steps,
	})
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(encoded)
	plan.PlanID = hex.EncodeToString(digest[:])
	return plan
}

func publicationTestRecipe(t *testing.T, plan AdmittedDeploymentPlan, previous string) InstallMediaRecipe {
	t.Helper()
	recipe := receiptTestRecipe(t, previous)
	recipe.PlanID = plan.PlanID
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

func TestInstallMediaPublicationAdmissionDeterministicAndBounded(t *testing.T) {
	plan := publicationTestPlan(t)
	recipe := publicationTestRecipe(t, plan, "")
	payload := []byte("verified-built-media")
	receipt, err := BuildBuiltInstallMediaReceipt(recipe, payload)
	if err != nil {
		t.Fatal(err)
	}
	target := InstallMediaPublicationTarget{Channel: PublicationChannelPXE, Slot: "windows-stable"}

	first, err := BuildInstallMediaPublicationAdmission(plan, recipe, receipt, payload, target)
	if err != nil {
		t.Fatal(err)
	}
	second, err := BuildInstallMediaPublicationAdmission(plan, recipe, receipt, payload, target)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("publication admission is not deterministic: %#v != %#v", first, second)
	}
	if first.PlanID != plan.PlanID || first.ReceiptID != receipt.ReceiptID || first.RecipeID != recipe.RecipeID || first.MediaSHA256 != receipt.MediaSHA256 || first.MediaSize != receipt.MediaSize || first.Target != target {
		t.Fatalf("publication admission did not bind exact evidence: %#v", first)
	}
	if !first.PlanIntegrityVerified || !first.ReceiptIntegrityVerified || !first.MediaIntegrityVerified || !first.PublicationAuthorized {
		t.Fatalf("publication admission missing bounded evidence: %#v", first)
	}
	if first.ServingAuthorized || first.ProvisioningAuthorized || first.HostMutation || first.NetworkMutation {
		t.Fatalf("publication admission grants unsafe authority: %#v", first)
	}
	if first.RollbackAction != "keep-current-serving-media" || first.RecoveryAction != "rebuild-publication-admission-from-current-evidence" {
		t.Fatalf("unexpected rollback/recovery contract: %#v", first)
	}
	if err := VerifyInstallMediaPublicationAdmission(first, plan, recipe, receipt, payload); err != nil {
		t.Fatalf("valid publication admission rejected: %v", err)
	}
}

func TestInstallMediaPublicationAdmissionRejectsTargetAndPlanDrift(t *testing.T) {
	plan := publicationTestPlan(t)
	recipe := publicationTestRecipe(t, plan, "")
	payload := []byte("verified-built-media")
	receipt, err := BuildBuiltInstallMediaReceipt(recipe, payload)
	if err != nil {
		t.Fatal(err)
	}

	invalidTargets := []InstallMediaPublicationTarget{
		{Channel: "caller-selected", Slot: "windows-stable"},
		{Channel: PublicationChannelPXE, Slot: "../windows"},
		{Channel: PublicationChannelPXE, Slot: "Windows-Stable"},
		{Channel: PublicationChannelPXE, Slot: "windows/stable"},
	}
	for _, target := range invalidTargets {
		if _, err := BuildInstallMediaPublicationAdmission(plan, recipe, receipt, payload, target); !errors.Is(err, ErrInstallMediaPublicationAdmission) {
			t.Fatalf("target %#v: expected fail-closed error, got %v", target, err)
		}
	}

	target := InstallMediaPublicationTarget{Channel: PublicationChannelPXE, Slot: "windows-stable"}
	admission, err := BuildInstallMediaPublicationAdmission(plan, recipe, receipt, payload, target)
	if err != nil {
		t.Fatal(err)
	}
	tamperedPlan := plan
	tamperedPlan.Architecture = "arm64"
	if err := VerifyInstallMediaPublicationAdmission(admission, tamperedPlan, recipe, receipt, payload); !errors.Is(err, ErrInstallMediaPublicationAdmission) {
		t.Fatalf("plan drift: expected fail-closed error, got %v", err)
	}
	tamperedPlan = plan
	tamperedPlan.ServingAuthorized = true
	if _, err := BuildInstallMediaPublicationAdmission(tamperedPlan, recipe, receipt, payload, target); !errors.Is(err, ErrInstallMediaPublicationAdmission) {
		t.Fatalf("unsafe plan authority: expected validation error, got %v", err)
	}
}

func TestInstallMediaPublicationAdmissionRejectsMediaAndReceiptDrift(t *testing.T) {
	plan := publicationTestPlan(t)
	recipe := publicationTestRecipe(t, plan, "")
	payload := []byte("verified-built-media")
	receipt, err := BuildBuiltInstallMediaReceipt(recipe, payload)
	if err != nil {
		t.Fatal(err)
	}
	target := InstallMediaPublicationTarget{Channel: PublicationChannelOffline, Slot: "windows-candidate"}
	admission, err := BuildInstallMediaPublicationAdmission(plan, recipe, receipt, payload, target)
	if err != nil {
		t.Fatal(err)
	}

	if err := VerifyInstallMediaPublicationAdmission(admission, plan, recipe, receipt, []byte("different-media")); !errors.Is(err, ErrInstallMediaPublicationAdmission) {
		t.Fatalf("media drift: expected fail-closed error, got %v", err)
	}
	tamperedReceipt := receipt
	tamperedReceipt.MediaSize++
	if err := VerifyInstallMediaPublicationAdmission(admission, plan, recipe, tamperedReceipt, payload); !errors.Is(err, ErrInstallMediaPublicationAdmission) {
		t.Fatalf("receipt drift: expected fail-closed error, got %v", err)
	}
	tamperedAdmission := admission
	tamperedAdmission.ServingAuthorized = true
	if err := VerifyInstallMediaPublicationAdmission(tamperedAdmission, plan, recipe, receipt, payload); !errors.Is(err, ErrInstallMediaPublicationAdmission) {
		t.Fatalf("unsafe admission: expected fail-closed error, got %v", err)
	}
}

func TestInstallMediaPublicationAdmissionBindsRollbackLineageAndTarget(t *testing.T) {
	plan := publicationTestPlan(t)
	previous := digestString("previous-recipe")
	recipe := publicationTestRecipe(t, plan, previous)
	payload := []byte("replacement-built-media")
	receipt, err := BuildBuiltInstallMediaReceipt(recipe, payload)
	if err != nil {
		t.Fatal(err)
	}
	target := InstallMediaPublicationTarget{Channel: PublicationChannelPXE, Slot: "windows-next"}
	admission, err := BuildInstallMediaPublicationAdmission(plan, recipe, receipt, payload, target)
	if err != nil {
		t.Fatal(err)
	}
	if admission.PreviousMediaRecipeID != previous {
		t.Fatalf("rollback lineage not bound: %#v", admission)
	}

	tampered := admission
	tampered.Target.Slot = "windows-other"
	if err := VerifyInstallMediaPublicationAdmission(tampered, plan, recipe, receipt, payload); !errors.Is(err, ErrInstallMediaPublicationAdmission) {
		t.Fatalf("target drift: expected fail-closed error, got %v", err)
	}
	tampered = admission
	tampered.PreviousMediaRecipeID = digestString("other-previous")
	if err := VerifyInstallMediaPublicationAdmission(tampered, plan, recipe, receipt, payload); !errors.Is(err, ErrInstallMediaPublicationAdmission) {
		t.Fatalf("lineage drift: expected fail-closed error, got %v", err)
	}
}
