package pxe

import (
	"errors"
	"reflect"
	"testing"
)

func servingReceiptFixture(t *testing.T, previous string) (
	AdmittedDeploymentPlan,
	InstallMediaRecipe,
	[]byte,
	BuiltInstallMediaReceipt,
	InstallMediaPublicationAdmission,
	InstallMediaPublicationReceipt,
	InstallMediaServingAdmission,
	InstallMediaServingStateObservation,
) {
	t.Helper()
	plan, recipe, payload, buildReceipt, publicationAdmission, publicationReceipt := servingAdmissionFixture(t, previous)
	servingAdmission, err := BuildInstallMediaServingAdmission(
		publicationReceipt,
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
	observation := InstallMediaServingStateObservation{
		ServingAdmissionID: servingAdmission.ServingAdmissionID,
		Target:             servingAdmission.Target,
		MediaSHA256:        servingAdmission.MediaSHA256,
		MediaSize:          servingAdmission.MediaSize,
		ServingActive:      true,
	}
	return plan, recipe, payload, buildReceipt, publicationAdmission, publicationReceipt, servingAdmission, observation
}

func TestInstallMediaServingReceiptDeterministicAndBounded(t *testing.T) {
	previous := digestString("previous-serving-media-recipe")
	plan, recipe, payload, buildReceipt, publicationAdmission, publicationReceipt, servingAdmission, observation := servingReceiptFixture(t, previous)

	first, err := BuildInstallMediaServingReceipt(
		servingAdmission,
		observation,
		publicationReceipt,
		publicationAdmission,
		plan,
		recipe,
		buildReceipt,
		payload,
		payload,
		payload,
	)
	if err != nil {
		t.Fatal(err)
	}
	second, err := BuildInstallMediaServingReceipt(
		servingAdmission,
		observation,
		publicationReceipt,
		publicationAdmission,
		plan,
		recipe,
		buildReceipt,
		payload,
		payload,
		payload,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("serving receipt is not deterministic: %#v != %#v", first, second)
	}
	if first.ServingAdmissionID != servingAdmission.ServingAdmissionID ||
		first.PublicationReceiptID != servingAdmission.PublicationReceiptID ||
		first.PublicationAdmissionID != servingAdmission.PublicationAdmissionID ||
		first.PlanID != servingAdmission.PlanID ||
		first.RecipeID != servingAdmission.RecipeID ||
		first.MediaSHA256 != servingAdmission.MediaSHA256 ||
		first.MediaSize != servingAdmission.MediaSize ||
		first.Target != servingAdmission.Target ||
		first.PreviousMediaRecipeID != previous ||
		first.RollbackAction != servingAdmission.RollbackAction {
		t.Fatalf("serving receipt did not bind exact active serving evidence: %#v", first)
	}
	if !first.ServingAdmissionVerified ||
		!first.ActiveSlotVerified ||
		!first.ActiveMediaIntegrityVerified ||
		!first.ServingActive ||
		first.ServingAuthorized ||
		first.BootAuthorized ||
		first.ProvisioningAuthorized ||
		first.HostMutation ||
		first.NetworkMutation {
		t.Fatalf("serving receipt authority is not evidence-only: %#v", first)
	}
	if first.RollbackAction != "restore-previous-serving-media" {
		t.Fatalf("unexpected rollback action: %#v", first)
	}
}

func TestInstallMediaServingReceiptUsesDisableRollbackWithoutPreviousMedia(t *testing.T) {
	plan, recipe, payload, buildReceipt, publicationAdmission, publicationReceipt, servingAdmission, observation := servingReceiptFixture(t, "")
	got, err := BuildInstallMediaServingReceipt(
		servingAdmission,
		observation,
		publicationReceipt,
		publicationAdmission,
		plan,
		recipe,
		buildReceipt,
		payload,
		payload,
		payload,
	)
	if err != nil {
		t.Fatal(err)
	}
	if got.RollbackAction != "disable-serving-slot" || got.PreviousMediaRecipeID != "" {
		t.Fatalf("unexpected no-previous-media rollback lineage: %#v", got)
	}
}

func TestInstallMediaServingReceiptRejectsInactiveSlot(t *testing.T) {
	plan, recipe, payload, buildReceipt, publicationAdmission, publicationReceipt, servingAdmission, observation := servingReceiptFixture(t, "")
	observation.ServingActive = false
	if _, err := BuildInstallMediaServingReceipt(
		servingAdmission,
		observation,
		publicationReceipt,
		publicationAdmission,
		plan,
		recipe,
		buildReceipt,
		payload,
		payload,
		payload,
	); !errors.Is(err, ErrInstallMediaServingReceipt) {
		t.Fatalf("expected inactive serving slot to fail closed, got %v", err)
	}
}

func TestInstallMediaServingReceiptRejectsSlotDrift(t *testing.T) {
	plan, recipe, payload, buildReceipt, publicationAdmission, publicationReceipt, servingAdmission, observation := servingReceiptFixture(t, "")
	observation.Target.Slot = "windows-other"
	if _, err := BuildInstallMediaServingReceipt(
		servingAdmission,
		observation,
		publicationReceipt,
		publicationAdmission,
		plan,
		recipe,
		buildReceipt,
		payload,
		payload,
		payload,
	); !errors.Is(err, ErrInstallMediaServingReceipt) {
		t.Fatalf("expected active-slot drift to fail closed, got %v", err)
	}
}

func TestInstallMediaServingReceiptRejectsObservedMediaDrift(t *testing.T) {
	plan, recipe, payload, buildReceipt, publicationAdmission, publicationReceipt, servingAdmission, observation := servingReceiptFixture(t, "")
	observation.MediaSHA256 = digestString("different-serving-media")
	if _, err := BuildInstallMediaServingReceipt(
		servingAdmission,
		observation,
		publicationReceipt,
		publicationAdmission,
		plan,
		recipe,
		buildReceipt,
		payload,
		payload,
		payload,
	); !errors.Is(err, ErrInstallMediaServingReceipt) {
		t.Fatalf("expected observed media drift to fail closed, got %v", err)
	}
}

func TestInstallMediaServingReceiptRejectsServedMediaReadbackDrift(t *testing.T) {
	plan, recipe, payload, buildReceipt, publicationAdmission, publicationReceipt, servingAdmission, observation := servingReceiptFixture(t, "")
	if _, err := BuildInstallMediaServingReceipt(
		servingAdmission,
		observation,
		publicationReceipt,
		publicationAdmission,
		plan,
		recipe,
		buildReceipt,
		payload,
		payload,
		[]byte("tampered-active-media"),
	); !errors.Is(err, ErrInstallMediaServingReceipt) {
		t.Fatalf("expected served media read-back drift to fail closed, got %v", err)
	}
}

func TestInstallMediaServingReceiptRejectsServingAdmissionDrift(t *testing.T) {
	plan, recipe, payload, buildReceipt, publicationAdmission, publicationReceipt, servingAdmission, observation := servingReceiptFixture(t, "")
	servingAdmission.Target.Slot = "windows-other"
	if _, err := BuildInstallMediaServingReceipt(
		servingAdmission,
		observation,
		publicationReceipt,
		publicationAdmission,
		plan,
		recipe,
		buildReceipt,
		payload,
		payload,
		payload,
	); !errors.Is(err, ErrInstallMediaServingReceipt) {
		t.Fatalf("expected serving-admission drift to fail closed, got %v", err)
	}
}

func TestVerifyInstallMediaServingReceiptRejectsTampering(t *testing.T) {
	previous := digestString("previous-serving-media-recipe")
	plan, recipe, payload, buildReceipt, publicationAdmission, publicationReceipt, servingAdmission, observation := servingReceiptFixture(t, previous)
	receipt, err := BuildInstallMediaServingReceipt(
		servingAdmission,
		observation,
		publicationReceipt,
		publicationAdmission,
		plan,
		recipe,
		buildReceipt,
		payload,
		payload,
		payload,
	)
	if err != nil {
		t.Fatal(err)
	}

	tampered := receipt
	tampered.BootAuthorized = true
	if err := VerifyInstallMediaServingReceipt(
		tampered,
		servingAdmission,
		observation,
		publicationReceipt,
		publicationAdmission,
		plan,
		recipe,
		buildReceipt,
		payload,
		payload,
		payload,
	); !errors.Is(err, ErrInstallMediaServingReceipt) {
		t.Fatalf("expected boot-authority tampering to fail closed, got %v", err)
	}

	tampered = receipt
	tampered.PreviousMediaRecipeID = digestString("different-previous-serving-media")
	if err := VerifyInstallMediaServingReceipt(
		tampered,
		servingAdmission,
		observation,
		publicationReceipt,
		publicationAdmission,
		plan,
		recipe,
		buildReceipt,
		payload,
		payload,
		payload,
	); !errors.Is(err, ErrInstallMediaServingReceipt) {
		t.Fatalf("expected rollback-lineage tampering to fail closed, got %v", err)
	}
}
