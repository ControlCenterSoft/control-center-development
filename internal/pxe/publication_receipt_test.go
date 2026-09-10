package pxe

import (
	"errors"
	"reflect"
	"testing"
)

func publicationReceiptFixture(t *testing.T, previous string) (
	AdmittedDeploymentPlan,
	InstallMediaRecipe,
	[]byte,
	BuiltInstallMediaReceipt,
	InstallMediaPublicationAdmission,
) {
	t.Helper()
	plan := publicationTestPlan(t)
	recipe := publicationTestRecipe(t, plan, previous)
	payload := []byte("verified-published-install-media")
	buildReceipt, err := BuildBuiltInstallMediaReceipt(recipe, payload)
	if err != nil {
		t.Fatal(err)
	}
	admission, err := BuildInstallMediaPublicationAdmission(
		plan,
		recipe,
		buildReceipt,
		payload,
		InstallMediaPublicationTarget{
			Channel: PublicationChannelPXE,
			Slot:    "windows-stable",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	return plan, recipe, payload, buildReceipt, admission
}

func TestInstallMediaPublicationReceiptDeterministicAndBounded(t *testing.T) {
	plan, recipe, payload, buildReceipt, admission := publicationReceiptFixture(t, "")

	first, err := BuildInstallMediaPublicationReceipt(
		admission,
		plan,
		recipe,
		buildReceipt,
		payload,
		payload,
	)
	if err != nil {
		t.Fatal(err)
	}
	second, err := BuildInstallMediaPublicationReceipt(
		admission,
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
		t.Fatalf("publication receipt is not deterministic: %#v != %#v", first, second)
	}
	if first.AdmissionID != admission.AdmissionID ||
		first.PlanID != admission.PlanID ||
		first.BuildReceiptID != admission.ReceiptID ||
		first.RecipeID != admission.RecipeID ||
		first.MediaSHA256 != admission.MediaSHA256 ||
		first.MediaSize != admission.MediaSize ||
		first.Target != admission.Target {
		t.Fatalf("publication receipt did not bind exact evidence: %#v", first)
	}
	if !first.AdmissionIntegrityVerified ||
		!first.PublishedMediaIntegrityVerified ||
		!first.PublicationCompleted {
		t.Fatalf("publication completion evidence is incomplete: %#v", first)
	}
	if first.PublicationAuthorized ||
		first.ServingAuthorized ||
		first.ProvisioningAuthorized ||
		first.HostMutation ||
		first.NetworkMutation {
		t.Fatalf("publication receipt grants unsafe authority: %#v", first)
	}
	if first.RollbackAction != "keep-current-serving-media" ||
		first.RecoveryAction != "republish-exact-admitted-media" {
		t.Fatalf("unexpected rollback/recovery contract: %#v", first)
	}
	if err := VerifyInstallMediaPublicationReceipt(
		first,
		admission,
		plan,
		recipe,
		buildReceipt,
		payload,
		payload,
	); err != nil {
		t.Fatalf("valid publication receipt rejected: %v", err)
	}
}

func TestInstallMediaPublicationReceiptRejectsPublishedReadbackDrift(t *testing.T) {
	plan, recipe, payload, buildReceipt, admission := publicationReceiptFixture(t, "")
	sameLengthDrift := append([]byte(nil), payload...)
	sameLengthDrift[len(sameLengthDrift)-1] ^= 1

	for _, published := range [][]byte{nil, []byte("different-published-media"), sameLengthDrift} {
		if _, err := BuildInstallMediaPublicationReceipt(
			admission,
			plan,
			recipe,
			buildReceipt,
			payload,
			published,
		); !errors.Is(err, ErrInstallMediaPublicationReceipt) {
			t.Fatalf("published payload %#v: expected fail-closed error, got %v", published, err)
		}
	}
}

func TestInstallMediaPublicationReceiptRejectsUpstreamEvidenceDrift(t *testing.T) {
	plan, recipe, payload, buildReceipt, admission := publicationReceiptFixture(t, "")

	tamperedAdmission := admission
	tamperedAdmission.Target.Slot = "windows-other"
	if _, err := BuildInstallMediaPublicationReceipt(
		tamperedAdmission,
		plan,
		recipe,
		buildReceipt,
		payload,
		payload,
	); !errors.Is(err, ErrInstallMediaPublicationReceipt) {
		t.Fatalf("target drift: expected fail-closed error, got %v", err)
	}

	tamperedBuildReceipt := buildReceipt
	tamperedBuildReceipt.MediaSize++
	if _, err := BuildInstallMediaPublicationReceipt(
		admission,
		plan,
		recipe,
		tamperedBuildReceipt,
		payload,
		payload,
	); !errors.Is(err, ErrInstallMediaPublicationReceipt) {
		t.Fatalf("build receipt drift: expected fail-closed error, got %v", err)
	}

	tamperedPlan := plan
	tamperedPlan.Architecture = "arm64"
	if _, err := BuildInstallMediaPublicationReceipt(
		admission,
		tamperedPlan,
		recipe,
		buildReceipt,
		payload,
		payload,
	); !errors.Is(err, ErrInstallMediaPublicationReceipt) {
		t.Fatalf("plan drift: expected fail-closed error, got %v", err)
	}
}

func TestInstallMediaPublicationReceiptBindsRollbackLineageAndSafety(t *testing.T) {
	previous := digestString("previous-serving-media-recipe")
	plan, recipe, payload, buildReceipt, admission := publicationReceiptFixture(t, previous)
	receipt, err := BuildInstallMediaPublicationReceipt(
		admission,
		plan,
		recipe,
		buildReceipt,
		payload,
		payload,
	)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.PreviousMediaRecipeID != previous {
		t.Fatalf("rollback lineage not preserved: %#v", receipt)
	}

	tampered := receipt
	tampered.PreviousMediaRecipeID = digestString("different-previous-recipe")
	if err := VerifyInstallMediaPublicationReceipt(
		tampered,
		admission,
		plan,
		recipe,
		buildReceipt,
		payload,
		payload,
	); !errors.Is(err, ErrInstallMediaPublicationReceipt) {
		t.Fatalf("rollback lineage drift: expected fail-closed error, got %v", err)
	}

	tampered = receipt
	tampered.ServingAuthorized = true
	if err := VerifyInstallMediaPublicationReceipt(
		tampered,
		admission,
		plan,
		recipe,
		buildReceipt,
		payload,
		payload,
	); !errors.Is(err, ErrInstallMediaPublicationReceipt) {
		t.Fatalf("unsafe authority drift: expected fail-closed error, got %v", err)
	}

	tampered = receipt
	tampered.PublicationReceiptID = "ABC"
	if err := VerifyInstallMediaPublicationReceipt(
		tampered,
		admission,
		plan,
		recipe,
		buildReceipt,
		payload,
		payload,
	); !errors.Is(err, ErrInstallMediaPublicationReceipt) {
		t.Fatalf("non-canonical receipt id: expected fail-closed error, got %v", err)
	}
}
