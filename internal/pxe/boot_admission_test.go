package pxe

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"
)

func testDigest(payload []byte) string {
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

func testArtifact(name, path, payload string) BootArtifact {
	return BootArtifact{Name: name, Path: path, SHA256: testDigest([]byte(payload)), Size: int64(len(payload))}
}

func TestWindowsBootArtifactAdmissionAndVerification(t *testing.T) {
	previous := testDigest([]byte("previous"))
	manifest, err := BuildArtifactManifest("windows-standard", previous, []BootArtifact{
		testArtifact("wimboot", "windows/wimboot", "wimboot"),
		testArtifact("bcd", "windows/BCD", "bcd"),
		testArtifact("boot-sdi", "windows/boot.sdi", "sdi"),
		testArtifact("boot-wim", "windows/boot.wim", "wim"),
	})
	if err != nil {
		t.Fatal(err)
	}
	bindings := []BootArtifactBinding{
		{Role: RoleWindowsBootWIM, ArtifactName: "boot-wim"},
		{Role: RoleWindowsWimboot, ArtifactName: "wimboot"},
		{Role: RoleWindowsBootSDI, ArtifactName: "boot-sdi"},
		{Role: RoleWindowsBCD, ArtifactName: "bcd"},
	}
	first, err := BuildBootArtifactAdmission("windows-standard", "Windows", "AMD64", manifest, bindings)
	if err != nil {
		t.Fatal(err)
	}
	second, err := BuildBootArtifactAdmission("windows-standard", "windows", "amd64", manifest, []BootArtifactBinding{bindings[1], bindings[3], bindings[0], bindings[2]})
	if err != nil {
		t.Fatal(err)
	}
	if first.AdmissionID != second.AdmissionID {
		t.Fatalf("admission id is not deterministic: %s != %s", first.AdmissionID, second.AdmissionID)
	}
	if first.RollbackManifestID != previous {
		t.Fatalf("rollback manifest = %q, want %q", first.RollbackManifestID, previous)
	}
	if first.ServingAuthorized || first.HostMutation || first.NetworkMutation {
		t.Fatalf("unsafe authority flags: %#v", first)
	}
	payloads := map[string][]byte{"wimboot": []byte("wimboot"), "bcd": []byte("bcd"), "boot-sdi": []byte("sdi"), "boot-wim": []byte("wim")}
	if err := VerifyBootArtifactPayloads(first, manifest, payloads); err != nil {
		t.Fatalf("valid payload set rejected: %v", err)
	}
}

func TestLinuxArm64BootArtifactAdmission(t *testing.T) {
	manifest, err := BuildArtifactManifest("linux-arm64", "", []BootArtifact{
		testArtifact("kernel", "linux/vmlinuz", "kernel"),
		testArtifact("initrd", "linux/initrd", "initrd"),
	})
	if err != nil {
		t.Fatal(err)
	}
	admission, err := BuildBootArtifactAdmission("linux-arm64", "linux", "arm64", manifest, []BootArtifactBinding{
		{Role: RoleLinuxInitrd, ArtifactName: "initrd"},
		{Role: RoleLinuxKernel, ArtifactName: "kernel"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyBootArtifactPayloads(admission, manifest, map[string][]byte{"kernel": []byte("kernel"), "initrd": []byte("initrd")}); err != nil {
		t.Fatal(err)
	}
}

func TestBootArtifactAdmissionRejectsUnsafeProfilesAndBindings(t *testing.T) {
	manifest, err := BuildArtifactManifest("windows-standard", "", []BootArtifact{
		testArtifact("wimboot", "windows/wimboot", "wimboot"),
		testArtifact("bcd", "windows/BCD", "bcd"),
		testArtifact("boot-sdi", "windows/boot.sdi", "sdi"),
		testArtifact("boot-wim", "windows/boot.wim", "wim"),
	})
	if err != nil {
		t.Fatal(err)
	}
	valid := []BootArtifactBinding{{Role: RoleWindowsWimboot, ArtifactName: "wimboot"}, {Role: RoleWindowsBCD, ArtifactName: "bcd"}, {Role: RoleWindowsBootSDI, ArtifactName: "boot-sdi"}, {Role: RoleWindowsBootWIM, ArtifactName: "boot-wim"}}
	cases := []struct {
		name, profile, os, arch string
		bindings                []BootArtifactBinding
	}{
		{name: "profile mismatch", profile: "other", os: "windows", arch: "amd64", bindings: valid},
		{name: "windows arm64", profile: "windows-standard", os: "windows", arch: "arm64", bindings: valid},
		{name: "missing role", profile: "windows-standard", os: "windows", arch: "amd64", bindings: valid[:3]},
		{name: "duplicate role", profile: "windows-standard", os: "windows", arch: "amd64", bindings: []BootArtifactBinding{{Role: RoleWindowsWimboot, ArtifactName: "wimboot"}, {Role: RoleWindowsWimboot, ArtifactName: "bcd"}, {Role: RoleWindowsBootSDI, ArtifactName: "boot-sdi"}, {Role: RoleWindowsBootWIM, ArtifactName: "boot-wim"}}},
		{name: "unknown artifact", profile: "windows-standard", os: "windows", arch: "amd64", bindings: []BootArtifactBinding{{Role: RoleWindowsWimboot, ArtifactName: "missing"}, {Role: RoleWindowsBCD, ArtifactName: "bcd"}, {Role: RoleWindowsBootSDI, ArtifactName: "boot-sdi"}, {Role: RoleWindowsBootWIM, ArtifactName: "boot-wim"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := BuildBootArtifactAdmission(tc.profile, tc.os, tc.arch, manifest, tc.bindings); !errors.Is(err, ErrBootArtifactAdmission) {
				t.Fatalf("error = %v, want ErrBootArtifactAdmission", err)
			}
		})
	}
}

func TestBootArtifactAdmissionRejectsTamperedManifestAndPayloadSet(t *testing.T) {
	manifest, err := BuildArtifactManifest("linux-standard", "", []BootArtifact{
		testArtifact("kernel", "linux/vmlinuz", "kernel"),
		testArtifact("initrd", "linux/initrd", "initrd"),
	})
	if err != nil {
		t.Fatal(err)
	}
	bindings := []BootArtifactBinding{{Role: RoleLinuxKernel, ArtifactName: "kernel"}, {Role: RoleLinuxInitrd, ArtifactName: "initrd"}}
	admission, err := BuildBootArtifactAdmission("linux-standard", "linux", "amd64", manifest, bindings)
	if err != nil {
		t.Fatal(err)
	}

	tamperedManifest := manifest
	tamperedManifest.Artifacts = append([]BootArtifact(nil), manifest.Artifacts...)
	tamperedManifest.Artifacts[0].SHA256 = testDigest([]byte("different"))
	if _, err := BuildBootArtifactAdmission("linux-standard", "linux", "amd64", tamperedManifest, bindings); !errors.Is(err, ErrBootArtifactAdmission) {
		t.Fatalf("tampered manifest error = %v", err)
	}

	if err := VerifyBootArtifactPayloads(admission, manifest, map[string][]byte{"kernel": []byte("kernel")}); !errors.Is(err, ErrBootArtifactAdmission) {
		t.Fatalf("missing payload error = %v", err)
	}
	if err := VerifyBootArtifactPayloads(admission, manifest, map[string][]byte{"kernel": []byte("tamper"), "initrd": []byte("initrd")}); !errors.Is(err, ErrBootArtifactAdmission) {
		t.Fatalf("tampered payload error = %v", err)
	}
	if err := VerifyBootArtifactPayloads(admission, manifest, map[string][]byte{"kernel": []byte("kernel"), "initrd": []byte("initrd"), "extra": []byte("extra")}); !errors.Is(err, ErrBootArtifactAdmission) {
		t.Fatalf("extra payload error = %v", err)
	}
}

func TestBootArtifactAdmissionRejectsModifiedSafetyFlags(t *testing.T) {
	manifest, err := BuildArtifactManifest("linux-standard", "", []BootArtifact{
		testArtifact("kernel", "linux/vmlinuz", "kernel"),
		testArtifact("initrd", "linux/initrd", "initrd"),
	})
	if err != nil {
		t.Fatal(err)
	}
	admission, err := BuildBootArtifactAdmission("linux-standard", "linux", "amd64", manifest, []BootArtifactBinding{{Role: RoleLinuxKernel, ArtifactName: "kernel"}, {Role: RoleLinuxInitrd, ArtifactName: "initrd"}})
	if err != nil {
		t.Fatal(err)
	}
	admission.ServingAuthorized = true
	if err := VerifyBootArtifactPayloads(admission, manifest, map[string][]byte{"kernel": []byte("kernel"), "initrd": []byte("initrd")}); !errors.Is(err, ErrBootArtifactAdmission) {
		t.Fatalf("modified authority error = %v", err)
	}
}
