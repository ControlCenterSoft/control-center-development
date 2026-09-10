package pxe

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
)

func bootSessionFixture(t *testing.T) (
	BootSessionRequest,
	InstallMediaServingReceipt,
	AdmittedDeploymentPlan,
	[]byte,
) {
	t.Helper()
	plan, recipe, payload, buildReceipt, publicationAdmission, publicationReceipt, servingAdmission, observation := servingReceiptFixture(t, "")
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
	return BootSessionRequest{
		RequestID:  "req-0001",
		MachineID:  "mac:02:00:00:00:00:01",
		TTLSeconds: 120,
	}, receipt, plan, payload
}

func TestBootSessionAdmissionDeterministicAndSingleUse(t *testing.T) {
	request, receipt, plan, payload := bootSessionFixture(t)
	const issuedAt = int64(1_800_000_000)

	first, err := BuildBootSessionAdmission(request, receipt, plan, payload, nil, nil, issuedAt)
	if err != nil {
		t.Fatal(err)
	}
	second, err := BuildBootSessionAdmission(request, receipt, plan, payload, nil, nil, issuedAt)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("boot-session admission is not deterministic: %#v != %#v", first, second)
	}
	if first.PlanID != plan.PlanID ||
		first.ServingReceiptID != receipt.ServingReceiptID ||
		first.ServingAdmissionID != receipt.ServingAdmissionID ||
		first.MediaSHA256 != receipt.MediaSHA256 ||
		first.MediaSize != receipt.MediaSize ||
		first.Target != receipt.Target ||
		first.ProfileName != plan.ProfileName {
		t.Fatalf("boot session did not bind exact serving evidence: %#v", first)
	}
	if first.ExpiresAtUnix != issuedAt+request.TTLSeconds ||
		first.MaxBootAttempts != 1 ||
		!first.SingleUse ||
		!first.AtomicConsumeRequired ||
		!first.ServingReceiptVerified ||
		first.UnattendedBindingVerified ||
		!first.BootAuthorized ||
		first.ProvisioningAuthorized ||
		first.SecretInjectionAuthorized ||
		first.HostMutation ||
		first.NetworkMutation {
		t.Fatalf("boot session authority is not bounded: %#v", first)
	}
	if err := VerifyBootSessionAdmission(first, request, receipt, plan, payload, nil, nil, issuedAt+1); err != nil {
		t.Fatalf("valid boot-session admission rejected: %v", err)
	}
}

func TestBootSessionAdmissionRejectsExpiryAndReplayContractTampering(t *testing.T) {
	request, receipt, plan, payload := bootSessionFixture(t)
	const issuedAt = int64(1_800_000_000)
	admission, err := BuildBootSessionAdmission(request, receipt, plan, payload, nil, nil, issuedAt)
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyBootSessionAdmission(admission, request, receipt, plan, payload, nil, nil, admission.ExpiresAtUnix); !errors.Is(err, ErrBootSessionAdmission) {
		t.Fatalf("expired session: expected fail-closed error, got %v", err)
	}

	cases := []BootSessionAdmission{admission, admission, admission, admission}
	cases[0].MaxBootAttempts = 2
	cases[1].AtomicConsumeRequired = false
	cases[2].ReplayKey = digestString("different-replay-key")
	cases[3].ProvisioningAuthorized = true
	for _, tampered := range cases {
		if err := VerifyBootSessionAdmission(tampered, request, receipt, plan, payload, nil, nil, issuedAt+1); !errors.Is(err, ErrBootSessionAdmission) {
			t.Fatalf("tampered replay/safety contract: expected fail-closed error, got %v", err)
		}
	}
}

func TestBootSessionAdmissionRejectsServingAndPlanDrift(t *testing.T) {
	request, receipt, plan, payload := bootSessionFixture(t)
	const issuedAt = int64(1_800_000_000)
	admission, err := BuildBootSessionAdmission(request, receipt, plan, payload, nil, nil, issuedAt)
	if err != nil {
		t.Fatal(err)
	}

	if err := VerifyBootSessionAdmission(admission, request, receipt, plan, []byte("tampered-active-media"), nil, nil, issuedAt+1); !errors.Is(err, ErrBootSessionAdmission) {
		t.Fatalf("served-media drift: expected fail-closed error, got %v", err)
	}
	tamperedReceipt := receipt
	tamperedReceipt.Target.Slot = "windows-other"
	if err := VerifyBootSessionAdmission(admission, request, tamperedReceipt, plan, payload, nil, nil, issuedAt+1); !errors.Is(err, ErrBootSessionAdmission) {
		t.Fatalf("serving-receipt drift: expected fail-closed error, got %v", err)
	}
	tamperedPlan := plan
	tamperedPlan.Architecture = "arm64"
	if err := VerifyBootSessionAdmission(admission, request, receipt, tamperedPlan, payload, nil, nil, issuedAt+1); !errors.Is(err, ErrBootSessionAdmission) {
		t.Fatalf("plan drift: expected fail-closed error, got %v", err)
	}
}

func TestBootSessionAdmissionBoundsIdentityAndLifetime(t *testing.T) {
	request, receipt, plan, payload := bootSessionFixture(t)
	const issuedAt = int64(1_800_000_000)

	invalid := []BootSessionRequest{
		{RequestID: "../escape", MachineID: request.MachineID, TTLSeconds: 60},
		{RequestID: request.RequestID, MachineID: "machine path", TTLSeconds: 60},
		{RequestID: request.RequestID, MachineID: request.MachineID, TTLSeconds: 0},
		{RequestID: request.RequestID, MachineID: request.MachineID, TTLSeconds: maxBootSessionTTLSeconds + 1},
	}
	for _, candidate := range invalid {
		if _, err := BuildBootSessionAdmission(candidate, receipt, plan, payload, nil, nil, issuedAt); !errors.Is(err, ErrBootSessionAdmission) {
			t.Fatalf("request %#v: expected bounded validation error, got %v", candidate, err)
		}
	}
}

func TestBootSessionAdmissionRequiresExactUnattendedBinding(t *testing.T) {
	plan := publicationTestPlan(t)
	plan.Steps = []Step{{Order: 1, Action: "apply-unattended-profile"}}
	plan.PlanID = bootSessionPlanID(t, plan)
	receipt, payload := bootSessionServingReceiptForPlan(t, plan)
	request := BootSessionRequest{RequestID: "req-unattended", MachineID: "node-0001", TTLSeconds: 60}
	const issuedAt = int64(1_800_000_000)

	if _, err := BuildBootSessionAdmission(request, receipt, plan, payload, nil, nil, issuedAt); !errors.Is(err, ErrBootSessionAdmission) {
		t.Fatalf("missing unattended binding: expected fail-closed error, got %v", err)
	}

	template := []byte(`<unattend><password>{{secret:domain.join.password}}</password></unattend>`)
	binding := bootSessionUnattendedBinding(t, plan, template)
	admission, err := BuildBootSessionAdmission(request, receipt, plan, payload, &binding, template, issuedAt)
	if err != nil {
		t.Fatal(err)
	}
	if !admission.UnattendedBindingVerified ||
		admission.UnattendedBindingID != binding.BindingID ||
		admission.UnattendedTemplateSHA256 != binding.TemplateSHA256 ||
		admission.SecretInjectionAuthorized {
		t.Fatalf("unexpected unattended boot admission: %#v", admission)
	}

	if err := VerifyBootSessionAdmission(admission, request, receipt, plan, payload, &binding, []byte(`<unattend><drift/></unattend>`), issuedAt+1); !errors.Is(err, ErrBootSessionAdmission) {
		t.Fatalf("unattended-template drift: expected fail-closed error, got %v", err)
	}
	tampered := binding
	tampered.RollbackAction = "caller-selected-action"
	if err := VerifyBootSessionAdmission(admission, request, receipt, plan, payload, &tampered, template, issuedAt+1); !errors.Is(err, ErrBootSessionAdmission) {
		t.Fatalf("unattended rollback drift: expected fail-closed error, got %v", err)
	}
}

func bootSessionPlanID(t *testing.T, plan AdmittedDeploymentPlan) string {
	t.Helper()
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
	return hex.EncodeToString(digest[:])
}

func bootSessionServingReceiptForPlan(t *testing.T, plan AdmittedDeploymentPlan) (InstallMediaServingReceipt, []byte) {
	t.Helper()
	recipe := publicationTestRecipe(t, plan, "")
	payload := []byte("verified-boot-session-media")
	buildReceipt, err := BuildBuiltInstallMediaReceipt(recipe, payload)
	if err != nil {
		t.Fatal(err)
	}
	publicationAdmission, err := BuildInstallMediaPublicationAdmission(
		plan,
		recipe,
		buildReceipt,
		payload,
		InstallMediaPublicationTarget{Channel: PublicationChannelPXE, Slot: "windows-stable"},
	)
	if err != nil {
		t.Fatal(err)
	}
	publicationReceipt, err := BuildInstallMediaPublicationReceipt(
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
	return receipt, payload
}

func bootSessionUnattendedBinding(t *testing.T, plan AdmittedDeploymentPlan, template []byte) UnattendedTemplateBinding {
	t.Helper()
	templateDigest := sha256.Sum256(template)
	refs, err := extractSecretRefs(template)
	if err != nil {
		t.Fatal(err)
	}
	rollback := "none-preprovisioning"
	if plan.RollbackManifestID != "" {
		rollback = "restore-previous-boot-manifest"
	}
	binding := UnattendedTemplateBinding{
		PlanID:                  plan.PlanID,
		ProfileName:             plan.ProfileName,
		OSFamily:                plan.OSFamily,
		Architecture:            plan.Architecture,
		AdmissionID:             plan.AdmissionID,
		ManifestID:              plan.ManifestID,
		RollbackManifestID:      plan.RollbackManifestID,
		ConfigKind:              ConfigWindowsUnattendXML,
		TemplateSHA256:          hex.EncodeToString(templateDigest[:]),
		TemplateSize:            int64(len(template)),
		SecretRefs:              refs,
		SecretInjectionDeferred: true,
		RollbackAction:          rollback,
		RecoveryAction:          "rebuild-binding-from-current-admission",
		IntegrityVerified:       true,
	}
	encoded, err := json.Marshal(unattendedTemplateBindingDigest{
		PlanID:             binding.PlanID,
		ProfileName:        binding.ProfileName,
		OSFamily:           binding.OSFamily,
		Architecture:       binding.Architecture,
		AdmissionID:        binding.AdmissionID,
		ManifestID:         binding.ManifestID,
		RollbackManifestID: binding.RollbackManifestID,
		ConfigKind:         binding.ConfigKind,
		TemplateSHA256:     binding.TemplateSHA256,
		TemplateSize:       binding.TemplateSize,
		SecretRefs:         binding.SecretRefs,
		RollbackAction:     binding.RollbackAction,
		RecoveryAction:     binding.RecoveryAction,
	})
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(encoded)
	binding.BindingID = hex.EncodeToString(digest[:])
	return binding
}
