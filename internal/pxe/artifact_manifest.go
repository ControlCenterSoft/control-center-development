package pxe

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"sort"
	"strings"
)

var (
	ErrInvalidArtifactManifest = errors.New("invalid PXE artifact manifest")
	ErrArtifactIntegrity       = errors.New("PXE artifact integrity check failed")
)

// BootArtifact describes one immutable boot artifact served by the PXE stack.
type BootArtifact struct {
	Name   string `json:"name"`
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

// ArtifactManifest is a deterministic, side-effect-free integrity contract for
// a complete PXE boot artifact set. PreviousManifestID identifies the exact
// immutable rollback target when one exists.
type ArtifactManifest struct {
	ManifestID         string         `json:"manifestId"`
	ProfileID          string         `json:"profileId"`
	PreviousManifestID string         `json:"previousManifestId,omitempty"`
	Artifacts          []BootArtifact `json:"artifacts"`
}

type artifactManifestDigest struct {
	ProfileID          string         `json:"profileId"`
	PreviousManifestID string         `json:"previousManifestId,omitempty"`
	Artifacts          []BootArtifact `json:"artifacts"`
}

// BuildArtifactManifest validates and canonicalizes an immutable PXE artifact
// set and returns a deterministic manifest ID. It performs no host, network, or
// storage mutations.
func BuildArtifactManifest(profileID, previousManifestID string, artifacts []BootArtifact) (ArtifactManifest, error) {
	profileID = strings.TrimSpace(profileID)
	if profileID == "" || len(artifacts) == 0 {
		return ArtifactManifest{}, ErrInvalidArtifactManifest
	}

	previousManifestID = strings.TrimSpace(previousManifestID)
	if previousManifestID != "" {
		normalized, err := normalizeSHA256(previousManifestID)
		if err != nil {
			return ArtifactManifest{}, fmt.Errorf("%w: previous manifest id: %v", ErrInvalidArtifactManifest, err)
		}
		previousManifestID = normalized
	}

	canonical := make([]BootArtifact, len(artifacts))
	seenNames := make(map[string]struct{}, len(artifacts))
	seenPaths := make(map[string]struct{}, len(artifacts))
	for i, artifact := range artifacts {
		artifact.Name = strings.TrimSpace(artifact.Name)
		artifact.Path = strings.TrimSpace(artifact.Path)
		if artifact.Name == "" || artifact.Size <= 0 {
			return ArtifactManifest{}, ErrInvalidArtifactManifest
		}
		if _, exists := seenNames[artifact.Name]; exists {
			return ArtifactManifest{}, fmt.Errorf("%w: duplicate artifact name %q", ErrInvalidArtifactManifest, artifact.Name)
		}
		seenNames[artifact.Name] = struct{}{}

		canonicalPath, err := canonicalArtifactPath(artifact.Path)
		if err != nil {
			return ArtifactManifest{}, fmt.Errorf("%w: artifact %q path: %v", ErrInvalidArtifactManifest, artifact.Name, err)
		}
		artifact.Path = canonicalPath
		if _, exists := seenPaths[artifact.Path]; exists {
			return ArtifactManifest{}, fmt.Errorf("%w: duplicate artifact path %q", ErrInvalidArtifactManifest, artifact.Path)
		}
		seenPaths[artifact.Path] = struct{}{}

		normalizedSHA, err := normalizeSHA256(artifact.SHA256)
		if err != nil {
			return ArtifactManifest{}, fmt.Errorf("%w: artifact %q checksum: %v", ErrInvalidArtifactManifest, artifact.Name, err)
		}
		artifact.SHA256 = normalizedSHA
		canonical[i] = artifact
	}

	sort.Slice(canonical, func(i, j int) bool {
		if canonical[i].Path == canonical[j].Path {
			return canonical[i].Name < canonical[j].Name
		}
		return canonical[i].Path < canonical[j].Path
	})

	digestInput := artifactManifestDigest{
		ProfileID:          profileID,
		PreviousManifestID: previousManifestID,
		Artifacts:          canonical,
	}
	encoded, err := json.Marshal(digestInput)
	if err != nil {
		return ArtifactManifest{}, fmt.Errorf("%w: encode canonical manifest: %v", ErrInvalidArtifactManifest, err)
	}
	digest := sha256.Sum256(encoded)

	return ArtifactManifest{
		ManifestID:         hex.EncodeToString(digest[:]),
		ProfileID:          profileID,
		PreviousManifestID: previousManifestID,
		Artifacts:          canonical,
	}, nil
}

// VerifyArtifactBytes validates the exact byte length and SHA-256 digest for an
// artifact before it is admitted to the PXE serving path.
func VerifyArtifactBytes(expected BootArtifact, payload []byte) error {
	if expected.Size <= 0 || int64(len(payload)) != expected.Size {
		return ErrArtifactIntegrity
	}
	normalizedSHA, err := normalizeSHA256(expected.SHA256)
	if err != nil {
		return ErrArtifactIntegrity
	}
	actual := sha256.Sum256(payload)
	if hex.EncodeToString(actual[:]) != normalizedSHA {
		return ErrArtifactIntegrity
	}
	return nil
}

func canonicalArtifactPath(value string) (string, error) {
	if value == "" || strings.Contains(value, "\\") || strings.HasPrefix(value, "/") {
		return "", errors.New("path must be a relative slash-separated path")
	}
	cleaned := path.Clean(value)
	if cleaned == "." || cleaned != value || strings.HasPrefix(cleaned, "../") || cleaned == ".." {
		return "", errors.New("path must already be canonical and must not traverse parents")
	}
	return cleaned, nil
}

func normalizeSHA256(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if len(value) != sha256.Size*2 {
		return "", errors.New("sha256 must contain 64 hexadecimal characters")
	}
	if _, err := hex.DecodeString(value); err != nil {
		return "", errors.New("sha256 contains non-hexadecimal characters")
	}
	return value, nil
}
