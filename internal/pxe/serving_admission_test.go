package pxe

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"reflect"
	"testing"
)

func servingAdmissionFixture(t *testing.T, previous string) (
	AdmittedDeploymentPlan,
	InstallMediaRecipe,
	[]byte,
	BuiltInstallMediaReceipt,
	InstallMediaPublicationAdmission,
	InstallMediaPublicationReceipt,
) {
	t.Helper()
	payload := []byte("verified-published-install-media")
	digest := sha256.Sum256(payload)
	mediaSHA := hex.EncodeToString(digest[:])
	plan := AdmittedDeploymentPlan{PlanID: "plan"}
	recipe := InstallMediaRecipe{RecipeID: "recipe"}
	buildReceipt := BuiltInstallMediaReceipt{ReceiptID: "build-receipt"}
	publicationAdmission := InstallMediaPublicationAdmission{
		AdmissionID:           "publication-admission",
		PlanID:                plan.PlanID,
		ReceiptID:             buildReceipt.ReceiptID,
		RecipeID:              recipe.RecipeID,
		MediaSHA256:           mediaSHA,
		MediaSize:             int64(len(payload)),
		Target:                InstallMediaPublicationTarget{Channel: PublicationChannelPXE, Slot: "windows-stable"},
		PreviousMediaRecipeID: previous,
	}
	receipt, err := BuildInstallMediaPublicationReceipt(
		publicationAdmission,
		plan,
		recipe,
		buildReceipt,
		payload,
		payload,
	)
	if err != nil {
		t.Fatal(err)
	}
	return plan, recipe, payload, buildReceipt, publicationAdmission, receipt
}

func TestInstallMediaServingAdmissionDeterministicAndBounded(t *testing.T) {
	plan, recipe, payload, buildReceipt, publicationAdmission, receipt := servingAdmissionFixture(
		t,
		"previous-recipe",
	)
	first, err := BuildInstallMediaServingAdmission(
		receipt,
		publicationAdmission,
		plan,
		recipe,
		buildReceipt,
		payload,
		payload,
	)
	if err != nil {
		t.Fatal(err)
	}
	second, err := BuildInstallMediaServingAdmission(
		receipt,
		publicationAdmission,
		plan,
		recipe,
		buildReceipt,
		payload,
		payload,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("serving admission is not deterministic: %#v != %#v", first, second)
	}
	if first.PublicationReceiptID != receipt.PublicationReceiptID ||
		first.PublicationAdmissionID != receipt.AdmissionID ||
		first.MediaSHA256 != receipt.MediaSHA256 ||
		first.MediaSize != receipt.MediaSize ||
		first.Target != receipt.Target {
		t.Fatalf("serving admission did not bind exact publication evidence: %#v", first)
	}
	if !first.PublicationReceiptVerified ||
		!first.CurrentMediaIntegrityVerified ||
		!first.ServingAuthorized ||
		first.ProvisioningAuthorized ||
		first.HostMutation ||
		first.NetworkMutation {
		t.Fatalf("serving admission authority is not bounded: %#v", first)
	}
	if first.RollbackAction != "restore-previous-serving-media" {
		t.Fatalf("unexpected rollback action: %#v", first)
	}
}

func TestInstallMediaServingAdmissionUsesDisableRollbackWithoutPreviousMedia(t *testing.T) {
	plan, recipe, payload, buildReceipt, publicationAdmission, receipt := servingAdmissionFixture(t, "")
	got, err := BuildInstallMediaServingAdmission(
		receipt,
		publicationAdmission,
		plan,
		recipe,
		buildReceipt,
		payload,
		payload,
	)
	if err != nil {
		t.Fatal(err)
	}
	if got.RollbackAction != "disable-serving-slot" {
		t.Fatalf("unexpected rollback action: %#v", got)
	}
}

func TestInstallMediaServingAdmissionRejectsOfflineMedia(t *testing.T) {
	plan, recipe, payload, buildReceipt, publicationAdmission, receipt := servingAdmissionFixture(t, "")
	receipt.Target.Channel = PublicationChannelOffline
	if _, err := BuildInstallMediaServingAdmission(
		receipt,
		publicationAdmission,
		plan,
		recipe,
		buildReceipt,
		payload,
		payload,
	); !errors.Is(err, ErrInstallMediaServingAdmission) {
		t.Fatalf("expected offline serving to fail closed, got %v", err)
	}
}

func TestInstallMediaServingAdmissionRejectsCurrentMediaDrift(t *testing.T) {
	plan, recipe, payload, buildReceipt, publicationAdmission, receipt := servingAdmissionFixture(t, "")
	if _, err := BuildInstallMediaServingAdmission(
		receipt,
		publicationAdmission,
		plan,
		recipe,
		buildReceipt,
		payload,
		[]byte("tampered-media"),
	); !errors.Is(err, ErrInstallMediaServingAdmission) {
		t.Fatalf("expected current media drift to fail closed, got %v", err)
	}
}

func TestInstallMediaServingAdmissionRejectsPublicationLineageDrift(t *testing.T) {
	plan, recipe, payload, buildReceipt, publicationAdmission, receipt := servingAdmissionFixture(t, "")
	publicationAdmission.Target.Slot = "windows-other"
	if _, err := BuildInstallMediaServingAdmission(
		receipt,
		publicationAdmission,
		plan,
		recipe,
		buildReceipt,
		payload,
		payload,
	); !errors.Is(err, ErrInstallMediaServingAdmission) {
		t.Fatalf("expected publication lineage drift to fail closed, got %v", err)
	}
}

func TestVerifyInstallMediaServingAdmissionRejectsTampering(t *testing.T) {
	plan, recipe, payload, buildReceipt, publicationAdmission, receipt := servingAdmissionFixture(
		t,
		"previous-recipe",
	)
	got, err := BuildInstallMediaServingAdmission(
		receipt,
		publicationAdmission,
		plan,
		recipe,
		buildReceipt,
		payload,
		payload,
	)
	if err != nil {
		t.Fatal(err)
	}

	tampered := got
	tampered.ProvisioningAuthorized = true
	if err := VerifyInstallMediaServingAdmission(
		tampered,
		receipt,
		publicationAdmission,
		plan,
		recipe,
		buildReceipt,
		payload,
		payload,
	); !errors.Is(err, ErrInstallMediaServingAdmission) {
		t.Fatalf("expected authority tampering to fail closed, got %v", err)
	}

	tampered = got
	tampered.PreviousMediaRecipeID = "different-previous-recipe"
	if err := VerifyInstallMediaServingAdmission(
		tampered,
		receipt,
		publicationAdmission,
		plan,
		recipe,
		buildReceipt,
		payload,
		payload,
	); !errors.Is(err, ErrInstallMediaServingAdmission) {
		t.Fatalf("expected rollback lineage tampering to fail closed, got %v", err)
	}
}
