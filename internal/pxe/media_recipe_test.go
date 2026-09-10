package pxe

import (
	"errors"
	"reflect"
	"testing"
)

func TestInstallMediaRecipeIsDeterministicAndNonAuthorizing(t *testing.T) {
	profile, plan, admission, manifest, payloads := unattendedWindowsFixture(t, "")
	template := []byte(`<unattend><password>{{secret:domain.join.password}}</password></unattend>`)
	binding, err := BuildUnattendedTemplateBinding(profile, plan, admission, manifest, payloads, ConfigWindowsUnattendXML, template)
	if err != nil {
		t.Fatal(err)
	}
	sourcePayload := []byte("windows-install-iso-payload")
	source, err := BuildInstallMediaSourceEvidence(SourceWindowsInstallISO, sourcePayload)
	if err != nil {
		t.Fatal(err)
	}
	parameters, err := DefaultInstallMediaBuildParameters(profile.OSFamily)
	if err != nil {
		t.Fatal(err)
	}

	first, err := BuildInstallMediaRecipe(profile, plan, admission, manifest, payloads, binding, template, source, sourcePayload, parameters, "")
	if err != nil {
		t.Fatal(err)
	}
	second, err := BuildInstallMediaRecipe(profile, plan, admission, manifest, payloads, binding, template, source, sourcePayload, parameters, "")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("recipe is not deterministic: %#v != %#v", first, second)
	}
	if !first.SourceIntegrityVerified || !first.BootIntegrityVerified || !first.TemplateIntegrityVerified || !first.Reproducible {
		t.Fatalf("expected verified reproducible evidence: %#v", first)
	}
	if first.BuildAuthorized || first.ServingAuthorized || first.ProvisioningAuthorized || first.HostMutation || first.NetworkMutation {
		t.Fatalf("unexpected authority or mutation flag: %#v", first)
	}
	if first.PlanID != plan.PlanID || first.AdmissionID != plan.AdmissionID || first.ManifestID != plan.ManifestID || first.UnattendedBindingID != binding.BindingID {
		t.Fatalf("recipe did not bind exact upstream evidence: %#v", first)
	}
	if first.RollbackAction != "none-prebuild" || first.RecoveryAction != "rebuild-media-recipe-from-current-evidence" {
		t.Fatalf("unexpected rollback/recovery action: %#v", first)
	}
	if err := VerifyInstallMediaRecipe(first, profile, plan, admission, manifest, payloads, binding, template, sourcePayload); err != nil {
		t.Fatalf("valid recipe rejected: %v", err)
	}
}

func TestInstallMediaRecipeRejectsSourceAndParameterDrift(t *testing.T) {
	profile, plan, admission, manifest, payloads := unattendedWindowsFixture(t, "")
	template := []byte("<unattend/>")
	binding, err := BuildUnattendedTemplateBinding(profile, plan, admission, manifest, payloads, ConfigWindowsUnattendXML, template)
	if err != nil {
		t.Fatal(err)
	}
	sourcePayload := []byte("windows-install-iso-payload")
	source, err := BuildInstallMediaSourceEvidence(SourceWindowsInstallISO, sourcePayload)
	if err != nil {
		t.Fatal(err)
	}
	parameters, err := DefaultInstallMediaBuildParameters(profile.OSFamily)
	if err != nil {
		t.Fatal(err)
	}
	recipe, err := BuildInstallMediaRecipe(profile, plan, admission, manifest, payloads, binding, template, source, sourcePayload, parameters, "")
	if err != nil {
		t.Fatal(err)
	}

	if err := VerifyInstallMediaRecipe(recipe, profile, plan, admission, manifest, payloads, binding, template, []byte("tampered-source-media")); !errors.Is(err, ErrInstallMediaRecipe) {
		t.Fatalf("source drift: expected fail-closed error, got %v", err)
	}

	tamperedRecipe := recipe
	tamperedRecipe.Parameters.TimestampUnix = 1
	if err := VerifyInstallMediaRecipe(tamperedRecipe, profile, plan, admission, manifest, payloads, binding, template, sourcePayload); !errors.Is(err, ErrInstallMediaRecipe) {
		t.Fatalf("timestamp drift: expected fail-closed error, got %v", err)
	}

	badParameters := parameters
	badParameters.VolumeLabel = "cc-windows"
	if _, err := BuildInstallMediaRecipe(profile, plan, admission, manifest, payloads, binding, template, source, sourcePayload, badParameters, ""); !errors.Is(err, ErrInstallMediaRecipe) {
		t.Fatalf("non-canonical volume label: expected validation error, got %v", err)
	}
}

func TestInstallMediaRecipeRejectsBootAndTemplateEvidenceDrift(t *testing.T) {
	profile, plan, admission, manifest, payloads := unattendedWindowsFixture(t, "")
	template := []byte(`<unattend><token>{{secret:join.token}}</token></unattend>`)
	binding, err := BuildUnattendedTemplateBinding(profile, plan, admission, manifest, payloads, ConfigWindowsUnattendXML, template)
	if err != nil {
		t.Fatal(err)
	}
	sourcePayload := []byte("windows-install-iso-payload")
	source, err := BuildInstallMediaSourceEvidence(SourceWindowsInstallISO, sourcePayload)
	if err != nil {
		t.Fatal(err)
	}
	parameters, err := DefaultInstallMediaBuildParameters(profile.OSFamily)
	if err != nil {
		t.Fatal(err)
	}
	recipe, err := BuildInstallMediaRecipe(profile, plan, admission, manifest, payloads, binding, template, source, sourcePayload, parameters, "")
	if err != nil {
		t.Fatal(err)
	}

	tamperedPayloads := make(map[string][]byte, len(payloads))
	for name, payload := range payloads {
		tamperedPayloads[name] = append([]byte(nil), payload...)
	}
	tamperedPayloads["boot-wim"] = []byte("tampered")
	if err := VerifyInstallMediaRecipe(recipe, profile, plan, admission, manifest, tamperedPayloads, binding, template, sourcePayload); !errors.Is(err, ErrInstallMediaRecipe) {
		t.Fatalf("boot evidence drift: expected fail-closed error, got %v", err)
	}

	if err := VerifyInstallMediaRecipe(recipe, profile, plan, admission, manifest, payloads, binding, []byte("<unattend><drift/></unattend>"), sourcePayload); !errors.Is(err, ErrInstallMediaRecipe) {
		t.Fatalf("template evidence drift: expected fail-closed error, got %v", err)
	}
}

func TestInstallMediaRecipeBindsPreviousRecipeRollback(t *testing.T) {
	profile, plan, admission, manifest, payloads := unattendedWindowsFixture(t, "")
	template := []byte("<unattend/>")
	binding, err := BuildUnattendedTemplateBinding(profile, plan, admission, manifest, payloads, ConfigWindowsUnattendXML, template)
	if err != nil {
		t.Fatal(err)
	}
	sourcePayload := []byte("windows-install-iso-payload")
	source, err := BuildInstallMediaSourceEvidence(SourceWindowsInstallISO, sourcePayload)
	if err != nil {
		t.Fatal(err)
	}
	parameters, err := DefaultInstallMediaBuildParameters(profile.OSFamily)
	if err != nil {
		t.Fatal(err)
	}
	previous := unattendedTestDigest([]byte("previous-media-recipe"))
	recipe, err := BuildInstallMediaRecipe(profile, plan, admission, manifest, payloads, binding, template, source, sourcePayload, parameters, previous)
	if err != nil {
		t.Fatal(err)
	}
	if recipe.PreviousMediaRecipeID != previous || recipe.RollbackAction != "restore-previous-media-recipe" {
		t.Fatalf("unexpected rollback lineage: %#v", recipe)
	}
	if err := VerifyInstallMediaRecipe(recipe, profile, plan, admission, manifest, payloads, binding, template, sourcePayload); err != nil {
		t.Fatalf("valid rollback lineage rejected: %v", err)
	}
}

func TestInstallMediaRecipeSupportsLinuxAndRejectsCrossOSSource(t *testing.T) {
	profile, plan, admission, manifest, payloads := unattendedLinuxFixture(t)
	template := []byte("#cloud-config\nautoinstall:\n  identity:\n    password: '{{secret:linux.password}}'\n")
	binding, err := BuildUnattendedTemplateBinding(profile, plan, admission, manifest, payloads, ConfigLinuxAutoinstall, template)
	if err != nil {
		t.Fatal(err)
	}
	sourcePayload := []byte("linux-install-iso-payload")
	linuxSource, err := BuildInstallMediaSourceEvidence(SourceLinuxInstallISO, sourcePayload)
	if err != nil {
		t.Fatal(err)
	}
	parameters, err := DefaultInstallMediaBuildParameters(profile.OSFamily)
	if err != nil {
		t.Fatal(err)
	}
	recipe, err := BuildInstallMediaRecipe(profile, plan, admission, manifest, payloads, binding, template, linuxSource, sourcePayload, parameters, "")
	if err != nil {
		t.Fatal(err)
	}
	if recipe.Source.Kind != SourceLinuxInstallISO || recipe.Parameters.VolumeLabel != "CC-LINUX" {
		t.Fatalf("unexpected Linux recipe: %#v", recipe)
	}

	windowsSource, err := BuildInstallMediaSourceEvidence(SourceWindowsInstallISO, sourcePayload)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := BuildInstallMediaRecipe(profile, plan, admission, manifest, payloads, binding, template, windowsSource, sourcePayload, parameters, ""); !errors.Is(err, ErrInstallMediaRecipe) {
		t.Fatalf("cross-OS source: expected validation error, got %v", err)
	}
}

func TestInstallMediaRecipeRejectsUnsafeFlags(t *testing.T) {
	profile, plan, admission, manifest, payloads := unattendedWindowsFixture(t, "")
	template := []byte("<unattend/>")
	binding, err := BuildUnattendedTemplateBinding(profile, plan, admission, manifest, payloads, ConfigWindowsUnattendXML, template)
	if err != nil {
		t.Fatal(err)
	}
	sourcePayload := []byte("windows-install-iso-payload")
	source, err := BuildInstallMediaSourceEvidence(SourceWindowsInstallISO, sourcePayload)
	if err != nil {
		t.Fatal(err)
	}
	parameters, err := DefaultInstallMediaBuildParameters(profile.OSFamily)
	if err != nil {
		t.Fatal(err)
	}
	recipe, err := BuildInstallMediaRecipe(profile, plan, admission, manifest, payloads, binding, template, source, sourcePayload, parameters, "")
	if err != nil {
		t.Fatal(err)
	}
	recipe.BuildAuthorized = true
	if err := VerifyInstallMediaRecipe(recipe, profile, plan, admission, manifest, payloads, binding, template, sourcePayload); !errors.Is(err, ErrInstallMediaRecipe) {
		t.Fatalf("unsafe build authorization: expected validation error, got %v", err)
	}
}
