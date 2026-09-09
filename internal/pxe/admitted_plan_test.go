package pxe

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"
)

func admittedPlanDigest(payload []byte) string {
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

func admittedPlanArtifact(name, path, payload string) BootArtifact {
	return BootArtifact{Name: name, Path: path, SHA256: admittedPlanDigest([]byte(payload)), Size: int64(len(payload))}
}

func windowsAdmissionFixture(t *testing.T, previous string) (Profile, ArtifactManifest, BootArtifactAdmission, map[string][]byte) {
	t.Helper()
	profile := Profile{Name: "windows-standard", OSFamily: "windows", Architecture: "amd64", Unattended: true}
	manifest, err := BuildArtifactManifest(profile.Name, previous, []BootArtifact{
		admittedPlanArtifact("wimboot", "windows/wimboot", "wimboot"),
		admittedPlanArtifact("bcd", "windows/BCD", "bcd"),
		admittedPlanArtifact("boot-sdi", "windows/boot.sdi", "sdi"),
		admittedPlanArtifact("boot-wim", "windows/boot.wim", "wim"),
	})
	if err != nil {
		t.Fatal(err)
	}
	admission, err := BuildBootArtifactAdmission(profile.Name, profile.OSFamily, profile.Architecture, manifest, []BootArtifactBinding{
		{Role: RoleWindowsBootWIM, ArtifactName: "boot-wim"},
		{Role: RoleWindowsWimboot, ArtifactName: "wimboot"},
		{Role: RoleWindowsBCD, ArtifactName: "bcd"},
		{Role: RoleWindowsBootSDI, ArtifactName: "boot-sdi"},
	})
	if err != nil {
		t.Fatal(err)
	}
	payloads := map[string][]byte{
		"wimboot":  []byte("wimboot"),
		"bcd":      []byte("bcd"),
		"boot-sdi": []byte("sdi"),
		"boot-wim": []byte("wim"),
	}
	return profile, manifest, admission, payloads
}

func TestAdmittedDeploymentPlanBindsExactVerifiedWindowsArtifacts(t *testing.T) {
	previous := admittedPlanDigest([]byte("previous"))
	profile, manifest, admission, payloads := windowsAdmissionFixture(t, previous)
	plan, err := BuildAdmittedDeploymentPlan(profile, admission, manifest, payloads)
	if err != nil {
		t.Fatal(err)
	}
	if plan.AdmissionID != admission.AdmissionID || plan.ManifestID != manifest.ManifestID || plan.RollbackManifestID != previous {
		t.Fatalf("plan is not bound to exact admission lineage: %#v", plan)
	}
	if !plan.ArtifactIntegrityVerified || plan.ServingAuthorized || plan.HostMutation || plan.NetworkMutation {
		t.Fatalf("unexpected safety flags: %#v", plan)
	}
	if len(plan.Steps) < 2 || plan.Steps[1].Action != "wimboot-start-winpe" {
		t.Fatalf("unexpected windows plan steps: %#v", plan.Steps)
	}
	if err := VerifyAdmittedDeploymentPlan(plan, profile, admission, manifest, payloads); err != nil {
		t.Fatalf("valid plan rejected: %v", err)
	}
}

func TestAdmittedDeploymentPlanIsDeterministicAcrossPayloadMapOrder(t *testing.T) {
	profile, manifest, admission, payloads := windowsAdmissionFixture(t, "")
	first, err := BuildAdmittedDeploymentPlan(profile, admission, manifest, payloads)
	if err != nil {
		t.Fatal(err)
	}
	secondPayloads := map[string][]byte{
		"boot-wim": []byte("wim"), "boot-sdi": []byte("sdi"), "bcd": []byte("bcd"), "wimboot": []byte("wimboot"),
	}
	second, err := BuildAdmittedDeploymentPlan(profile, admission, manifest, secondPayloads)
	if err != nil {
		t.Fatal(err)
	}
	if first.PlanID != second.PlanID {
		t.Fatalf("plan id is not deterministic: %s != %s", first.PlanID, second.PlanID)
	}
}

func TestAdmittedDeploymentPlanRejectsProfileAdmissionMismatch(t *testing.T) {
	profile, manifest, admission, payloads := windowsAdmissionFixture(t, "")
	profile.Name = "other-profile"
	if _, err := BuildAdmittedDeploymentPlan(profile, admission, manifest, payloads); !errors.Is(err, ErrAdmittedDeploymentPlan) {
		t.Fatalf("error = %v, want ErrAdmittedDeploymentPlan", err)
	}
}

func TestAdmittedDeploymentPlanRejectsTamperedOrIncompletePayloads(t *testing.T) {
	profile, manifest, admission, payloads := windowsAdmissionFixture(t, "")
	tampered := map[string][]byte{}
	for name, payload := range payloads {
		tampered[name] = append([]byte(nil), payload...)
	}
	tampered["boot-wim"] = []byte("bad")
	if _, err := BuildAdmittedDeploymentPlan(profile, admission, manifest, tampered); !errors.Is(err, ErrAdmittedDeploymentPlan) {
		t.Fatalf("tampered payload error = %v", err)
	}
	delete(tampered, "boot-wim")
	if _, err := BuildAdmittedDeploymentPlan(profile, admission, manifest, tampered); !errors.Is(err, ErrAdmittedDeploymentPlan) {
		t.Fatalf("incomplete payload error = %v", err)
	}
}

func TestAdmittedDeploymentPlanRejectsManifestOrAdmissionDrift(t *testing.T) {
	profile, manifest, admission, payloads := windowsAdmissionFixture(t, "")
	plan, err := BuildAdmittedDeploymentPlan(profile, admission, manifest, payloads)
	if err != nil {
		t.Fatal(err)
	}
	tamperedAdmission := admission
	tamperedAdmission.ServingAuthorized = true
	if err := VerifyAdmittedDeploymentPlan(plan, profile, tamperedAdmission, manifest, payloads); !errors.Is(err, ErrAdmittedDeploymentPlan) {
		t.Fatalf("tampered admission error = %v", err)
	}
	tamperedManifest := manifest
	tamperedManifest.Artifacts = append([]BootArtifact(nil), manifest.Artifacts...)
	tamperedManifest.Artifacts[0].SHA256 = admittedPlanDigest([]byte("different"))
	if err := VerifyAdmittedDeploymentPlan(plan, profile, admission, tamperedManifest, payloads); !errors.Is(err, ErrAdmittedDeploymentPlan) {
		t.Fatalf("tampered manifest error = %v", err)
	}
}

func TestAdmittedDeploymentPlanManifestChangeCreatesNewIdentityAndRollbackTarget(t *testing.T) {
	profile, firstManifest, firstAdmission, firstPayloads := windowsAdmissionFixture(t, "")
	first, err := BuildAdmittedDeploymentPlan(profile, firstAdmission, firstManifest, firstPayloads)
	if err != nil {
		t.Fatal(err)
	}

	profile, secondManifest, secondAdmission, secondPayloads := windowsAdmissionFixture(t, firstManifest.ManifestID)
	second, err := BuildAdmittedDeploymentPlan(profile, secondAdmission, secondManifest, secondPayloads)
	if err != nil {
		t.Fatal(err)
	}
	if first.PlanID == second.PlanID {
		t.Fatal("manifest lineage change must create a new admitted plan identity")
	}
	if second.RollbackManifestID != first.ManifestID {
		t.Fatalf("rollback manifest = %q, want %q", second.RollbackManifestID, first.ManifestID)
	}
}

func TestVerifyAdmittedDeploymentPlanRejectsPlanTampering(t *testing.T) {
	profile, manifest, admission, payloads := windowsAdmissionFixture(t, "")
	plan, err := BuildAdmittedDeploymentPlan(profile, admission, manifest, payloads)
	if err != nil {
		t.Fatal(err)
	}
	plan.Steps = append([]Step(nil), plan.Steps...)
	plan.Steps[1].Action = "caller-selected-action"
	if err := VerifyAdmittedDeploymentPlan(plan, profile, admission, manifest, payloads); !errors.Is(err, ErrAdmittedDeploymentPlan) {
		t.Fatalf("tampered plan error = %v", err)
	}
}
