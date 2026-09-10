package pxe

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"reflect"
	"testing"
)

func unattendedTestDigest(payload []byte) string {
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

func unattendedTestArtifact(name, artifactPath, payload string) BootArtifact {
	return BootArtifact{
		Name:   name,
		Path:   artifactPath,
		SHA256: unattendedTestDigest([]byte(payload)),
		Size:   int64(len(payload)),
	}
}

func unattendedWindowsFixture(t *testing.T, previous string) (Profile, AdmittedDeploymentPlan, BootArtifactAdmission, ArtifactManifest, map[string][]byte) {
	t.Helper()
	profile := Profile{Name: "windows-standard", OSFamily: "windows", Architecture: "amd64", Unattended: true}
	manifest, err := BuildArtifactManifest(profile.Name, previous, []BootArtifact{
		unattendedTestArtifact("wimboot", "windows/wimboot", "wimboot"),
		unattendedTestArtifact("bcd", "windows/BCD", "bcd"),
		unattendedTestArtifact("boot-sdi", "windows/boot.sdi", "sdi"),
		unattendedTestArtifact("boot-wim", "windows/boot.wim", "wim"),
	})
	if err != nil {
		t.Fatal(err)
	}
	admission, err := BuildBootArtifactAdmission(profile.Name, profile.OSFamily, profile.Architecture, manifest, []BootArtifactBinding{
		{Role: RoleWindowsWimboot, ArtifactName: "wimboot"},
		{Role: RoleWindowsBCD, ArtifactName: "bcd"},
		{Role: RoleWindowsBootSDI, ArtifactName: "boot-sdi"},
		{Role: RoleWindowsBootWIM, ArtifactName: "boot-wim"},
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
	plan, err := BuildAdmittedDeploymentPlan(profile, admission, manifest, payloads)
	if err != nil {
		t.Fatal(err)
	}
	return profile, plan, admission, manifest, payloads
}

func unattendedLinuxFixture(t *testing.T) (Profile, AdmittedDeploymentPlan, BootArtifactAdmission, ArtifactManifest, map[string][]byte) {
	t.Helper()
	profile := Profile{Name: "linux-arm64", OSFamily: "linux", Architecture: "arm64", Unattended: true}
	manifest, err := BuildArtifactManifest(profile.Name, "", []BootArtifact{
		unattendedTestArtifact("kernel", "linux/vmlinuz", "kernel"),
		unattendedTestArtifact("initrd", "linux/initrd", "initrd"),
	})
	if err != nil {
		t.Fatal(err)
	}
	admission, err := BuildBootArtifactAdmission(profile.Name, profile.OSFamily, profile.Architecture, manifest, []BootArtifactBinding{
		{Role: RoleLinuxKernel, ArtifactName: "kernel"},
		{Role: RoleLinuxInitrd, ArtifactName: "initrd"},
	})
	if err != nil {
		t.Fatal(err)
	}
	payloads := map[string][]byte{"kernel": []byte("kernel"), "initrd": []byte("initrd")}
	plan, err := BuildAdmittedDeploymentPlan(profile, admission, manifest, payloads)
	if err != nil {
		t.Fatal(err)
	}
	return profile, plan, admission, manifest, payloads
}

func TestUnattendedBindingIsDeterministicAndDefersSecrets(t *testing.T) {
	previous := unattendedTestDigest([]byte("previous-manifest"))
	profile, plan, admission, manifest, payloads := unattendedWindowsFixture(t, previous)
	template := []byte(`<unattend><password>{{secret:domain.join.password}}</password><token>{{secret:api-token}}</token><again>{{secret:domain.join.password}}</again></unattend>`)

	first, err := BuildUnattendedTemplateBinding(profile, plan, admission, manifest, payloads, ConfigWindowsUnattendXML, template)
	if err != nil {
		t.Fatal(err)
	}
	second, err := BuildUnattendedTemplateBinding(profile, plan, admission, manifest, payloads, ConfigWindowsUnattendXML, template)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("binding is not deterministic: %#v != %#v", first, second)
	}
	if want := []string{"api-token", "domain.join.password"}; !reflect.DeepEqual(first.SecretRefs, want) {
		t.Fatalf("secret refs = %#v, want %#v", first.SecretRefs, want)
	}
	if !first.SecretInjectionDeferred || !first.IntegrityVerified || first.ServingAuthorized || first.ProvisioningAuthorized || first.HostMutation || first.NetworkMutation {
		t.Fatalf("unexpected safety flags: %#v", first)
	}
	if first.RollbackManifestID != previous || first.RollbackAction != "restore-previous-boot-manifest" || first.RecoveryAction != "rebuild-binding-from-current-admission" {
		t.Fatalf("unexpected rollback/recovery metadata: %#v", first)
	}
	if err := VerifyUnattendedTemplateBinding(first, profile, plan, admission, manifest, payloads, template); err != nil {
		t.Fatalf("valid binding rejected: %v", err)
	}
}

func TestUnattendedBindingRejectsTemplateAndBootEvidenceDrift(t *testing.T) {
	profile, plan, admission, manifest, payloads := unattendedWindowsFixture(t, "")
	template := []byte(`<unattend><password>{{secret:domain.password}}</password></unattend>`)
	binding, err := BuildUnattendedTemplateBinding(profile, plan, admission, manifest, payloads, ConfigWindowsUnattendXML, template)
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyUnattendedTemplateBinding(binding, profile, plan, admission, manifest, payloads, []byte(`<unattend><password>{{secret:domain.password}}</password><drift/></unattend>`)); !errors.Is(err, ErrUnattendedTemplateBinding) {
		t.Fatalf("template drift: expected fail-closed error, got %v", err)
	}

	tamperedPayloads := map[string][]byte{
		"wimboot":  []byte("wimboot"),
		"bcd":      []byte("bcd"),
		"boot-sdi": []byte("sdi"),
		"boot-wim": []byte("tampered"),
	}
	if err := VerifyUnattendedTemplateBinding(binding, profile, plan, admission, manifest, tamperedPayloads, template); !errors.Is(err, ErrUnattendedTemplateBinding) {
		t.Fatalf("boot evidence drift: expected fail-closed error, got %v", err)
	}
}

func TestUnattendedBindingRejectsWrongKindDisabledProfileAndUnsafeFlags(t *testing.T) {
	profile, plan, admission, manifest, payloads := unattendedWindowsFixture(t, "")
	if _, err := BuildUnattendedTemplateBinding(profile, plan, admission, manifest, payloads, ConfigLinuxAutoinstall, []byte("config")); !errors.Is(err, ErrUnattendedTemplateBinding) {
		t.Fatalf("wrong kind: expected validation error, got %v", err)
	}

	disabled := profile
	disabled.Unattended = false
	if _, err := BuildUnattendedTemplateBinding(disabled, plan, admission, manifest, payloads, ConfigWindowsUnattendXML, []byte("config")); !errors.Is(err, ErrUnattendedTemplateBinding) {
		t.Fatalf("disabled unattended: expected validation error, got %v", err)
	}

	binding, err := BuildUnattendedTemplateBinding(profile, plan, admission, manifest, payloads, ConfigWindowsUnattendXML, []byte("config"))
	if err != nil {
		t.Fatal(err)
	}
	binding.ProvisioningAuthorized = true
	if err := VerifyUnattendedTemplateBinding(binding, profile, plan, admission, manifest, payloads, []byte("config")); !errors.Is(err, ErrUnattendedTemplateBinding) {
		t.Fatalf("unsafe flag: expected validation error, got %v", err)
	}
}

func TestUnattendedBindingRejectsMalformedOrUnsafeTemplateEncoding(t *testing.T) {
	profile, plan, admission, manifest, payloads := unattendedWindowsFixture(t, "")
	cases := [][]byte{
		{},
		{0xff, 0xfe},
		[]byte("abc\x00def"),
		[]byte(`{{secret domain.password}}`),
		[]byte(`{{secret:domain password}}`),
		[]byte(`{{secret:domain.password`),
		[]byte(`{{secret:}}`),
		[]byte(`{{secret:valid}}{{secret broken}}`),
	}
	for _, template := range cases {
		if _, err := BuildUnattendedTemplateBinding(profile, plan, admission, manifest, payloads, ConfigWindowsUnattendXML, template); !errors.Is(err, ErrUnattendedTemplateBinding) {
			t.Fatalf("template %q: expected validation error, got %v", template, err)
		}
	}
}

func TestUnattendedBindingSupportsLinuxAutoinstallWithoutInventingRollback(t *testing.T) {
	profile, plan, admission, manifest, payloads := unattendedLinuxFixture(t)
	template := []byte("#cloud-config\nautoinstall:\n  identity:\n    password: '{{secret:linux.password}}'\n")
	binding, err := BuildUnattendedTemplateBinding(profile, plan, admission, manifest, payloads, ConfigLinuxAutoinstall, template)
	if err != nil {
		t.Fatal(err)
	}
	if binding.RollbackManifestID != "" || binding.RollbackAction != "none-preprovisioning" {
		t.Fatalf("unexpected rollback metadata: %#v", binding)
	}
	if !reflect.DeepEqual(binding.SecretRefs, []string{"linux.password"}) {
		t.Fatalf("secret refs = %#v", binding.SecretRefs)
	}
}
