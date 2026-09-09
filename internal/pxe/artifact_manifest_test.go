package pxe

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"
)

func checksumFor(payload string) string {
	sum := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(sum[:])
}

func TestBuildArtifactManifestDeterministicAcrossInputOrder(t *testing.T) {
	kernel := BootArtifact{Name: "kernel", Path: "linux/vmlinuz", SHA256: checksumFor("kernel"), Size: 6}
	initrd := BootArtifact{Name: "initrd", Path: "linux/initrd", SHA256: checksumFor("initrd"), Size: 6}

	first, err := BuildArtifactManifest("linux-standard", "", []BootArtifact{kernel, initrd})
	if err != nil {
		t.Fatal(err)
	}
	second, err := BuildArtifactManifest("linux-standard", "", []BootArtifact{initrd, kernel})
	if err != nil {
		t.Fatal(err)
	}
	if first.ManifestID != second.ManifestID {
		t.Fatalf("manifest id is not deterministic: %q != %q", first.ManifestID, second.ManifestID)
	}
	if first.Artifacts[0].Path != "linux/initrd" || first.Artifacts[1].Path != "linux/vmlinuz" {
		t.Fatalf("artifacts are not canonically sorted: %#v", first.Artifacts)
	}
}

func TestBuildArtifactManifestCarriesValidatedRollbackTarget(t *testing.T) {
	previous := checksumFor("previous-manifest")
	manifest, err := BuildArtifactManifest("windows-standard", previous, []BootArtifact{{
		Name:   "wimboot",
		Path:   "windows/wimboot",
		SHA256: checksumFor("wimboot"),
		Size:   7,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if manifest.PreviousManifestID != previous {
		t.Fatalf("rollback target = %q, want %q", manifest.PreviousManifestID, previous)
	}
}

func TestBuildArtifactManifestRejectsUnsafeOrDuplicatePaths(t *testing.T) {
	validSHA := checksumFor("payload")
	cases := []struct {
		name      string
		artifacts []BootArtifact
	}{
		{
			name:      "parent traversal",
			artifacts: []BootArtifact{{Name: "kernel", Path: "../vmlinuz", SHA256: validSHA, Size: 7}},
		},
		{
			name:      "non canonical",
			artifacts: []BootArtifact{{Name: "kernel", Path: "linux/../vmlinuz", SHA256: validSHA, Size: 7}},
		},
		{
			name: "duplicate path",
			artifacts: []BootArtifact{
				{Name: "kernel", Path: "linux/boot", SHA256: validSHA, Size: 7},
				{Name: "initrd", Path: "linux/boot", SHA256: validSHA, Size: 7},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := BuildArtifactManifest("profile", "", tc.artifacts); !errors.Is(err, ErrInvalidArtifactManifest) {
				t.Fatalf("error = %v, want ErrInvalidArtifactManifest", err)
			}
		})
	}
}

func TestBuildArtifactManifestRejectsMalformedChecksums(t *testing.T) {
	if _, err := BuildArtifactManifest("profile", "", []BootArtifact{{
		Name:   "kernel",
		Path:   "linux/vmlinuz",
		SHA256: "not-a-sha256",
		Size:   12,
	}}); !errors.Is(err, ErrInvalidArtifactManifest) {
		t.Fatalf("error = %v, want ErrInvalidArtifactManifest", err)
	}

	if _, err := BuildArtifactManifest("profile", "bad-rollback-id", []BootArtifact{{
		Name:   "kernel",
		Path:   "linux/vmlinuz",
		SHA256: checksumFor("kernel"),
		Size:   6,
	}}); !errors.Is(err, ErrInvalidArtifactManifest) {
		t.Fatalf("rollback id error = %v, want ErrInvalidArtifactManifest", err)
	}
}

func TestVerifyArtifactBytesRejectsTamperingAndTruncation(t *testing.T) {
	payload := []byte("wimboot")
	expected := BootArtifact{
		Name:   "wimboot",
		Path:   "windows/wimboot",
		SHA256: checksumFor(string(payload)),
		Size:   int64(len(payload)),
	}
	if err := VerifyArtifactBytes(expected, payload); err != nil {
		t.Fatalf("valid payload rejected: %v", err)
	}
	if err := VerifyArtifactBytes(expected, []byte("wimboat")); !errors.Is(err, ErrArtifactIntegrity) {
		t.Fatalf("tampered payload error = %v, want ErrArtifactIntegrity", err)
	}
	if err := VerifyArtifactBytes(expected, payload[:len(payload)-1]); !errors.Is(err, ErrArtifactIntegrity) {
		t.Fatalf("truncated payload error = %v, want ErrArtifactIntegrity", err)
	}
}
