package pxe

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
)

var ErrBootArtifactAdmission = errors.New("PXE boot artifact admission failed")

type BootArtifactRole string

const (
	RoleWindowsWimboot BootArtifactRole = "windows-wimboot"
	RoleWindowsBCD     BootArtifactRole = "windows-bcd"
	RoleWindowsBootSDI BootArtifactRole = "windows-boot-sdi"
	RoleWindowsBootWIM BootArtifactRole = "windows-boot-wim"
	RoleLinuxKernel    BootArtifactRole = "linux-kernel"
	RoleLinuxInitrd    BootArtifactRole = "linux-initrd"
)

type BootArtifactBinding struct {
	Role         BootArtifactRole `json:"role"`
	ArtifactName string           `json:"artifactName"`
}

// BootArtifactAdmission binds an OS/architecture profile to one exact immutable
// artifact manifest. It is a side-effect-free integrity contract and never
// authorizes serving, provisioning, host mutation, or network mutation.
type BootArtifactAdmission struct {
	AdmissionID        string                `json:"admissionId"`
	ProfileID          string                `json:"profileId"`
	OSFamily           string                `json:"osFamily"`
	Architecture       string                `json:"architecture"`
	ManifestID         string                `json:"manifestId"`
	RollbackManifestID string                `json:"rollbackManifestId,omitempty"`
	Bindings           []BootArtifactBinding `json:"bindings"`
	ServingAuthorized  bool                  `json:"servingAuthorized"`
	HostMutation       bool                  `json:"hostMutation"`
	NetworkMutation    bool                  `json:"networkMutation"`
}

type bootArtifactAdmissionDigest struct {
	ProfileID          string                `json:"profileId"`
	OSFamily           string                `json:"osFamily"`
	Architecture       string                `json:"architecture"`
	ManifestID         string                `json:"manifestId"`
	RollbackManifestID string                `json:"rollbackManifestId,omitempty"`
	Bindings           []BootArtifactBinding `json:"bindings"`
}

func BuildBootArtifactAdmission(profileID, osFamily, architecture string, manifest ArtifactManifest, bindings []BootArtifactBinding) (BootArtifactAdmission, error) {
	profileID = strings.TrimSpace(profileID)
	osFamily = strings.ToLower(strings.TrimSpace(osFamily))
	architecture = strings.ToLower(strings.TrimSpace(architecture))
	if profileID == "" || manifest.ProfileID != profileID {
		return BootArtifactAdmission{}, fmt.Errorf("%w: profile identity mismatch", ErrBootArtifactAdmission)
	}
	if err := validateExactArtifactManifest(manifest); err != nil {
		return BootArtifactAdmission{}, err
	}

	required, err := requiredBootArtifactRoles(osFamily, architecture)
	if err != nil {
		return BootArtifactAdmission{}, err
	}
	if len(bindings) != len(required) {
		return BootArtifactAdmission{}, fmt.Errorf("%w: expected %d role bindings, got %d", ErrBootArtifactAdmission, len(required), len(bindings))
	}

	artifactsByName := make(map[string]BootArtifact, len(manifest.Artifacts))
	for _, artifact := range manifest.Artifacts {
		artifactsByName[artifact.Name] = artifact
	}
	requiredSet := make(map[BootArtifactRole]struct{}, len(required))
	for _, role := range required {
		requiredSet[role] = struct{}{}
	}

	canonical := make([]BootArtifactBinding, len(bindings))
	seenRoles := make(map[BootArtifactRole]struct{}, len(bindings))
	seenArtifacts := make(map[string]struct{}, len(bindings))
	for i, binding := range bindings {
		binding.ArtifactName = strings.TrimSpace(binding.ArtifactName)
		if _, ok := requiredSet[binding.Role]; !ok {
			return BootArtifactAdmission{}, fmt.Errorf("%w: role %q is not allowed for %s/%s", ErrBootArtifactAdmission, binding.Role, osFamily, architecture)
		}
		if _, exists := seenRoles[binding.Role]; exists {
			return BootArtifactAdmission{}, fmt.Errorf("%w: duplicate role %q", ErrBootArtifactAdmission, binding.Role)
		}
		seenRoles[binding.Role] = struct{}{}
		if binding.ArtifactName == "" {
			return BootArtifactAdmission{}, fmt.Errorf("%w: empty artifact name for role %q", ErrBootArtifactAdmission, binding.Role)
		}
		if _, exists := artifactsByName[binding.ArtifactName]; !exists {
			return BootArtifactAdmission{}, fmt.Errorf("%w: artifact %q is not present in manifest", ErrBootArtifactAdmission, binding.ArtifactName)
		}
		if _, exists := seenArtifacts[binding.ArtifactName]; exists {
			return BootArtifactAdmission{}, fmt.Errorf("%w: artifact %q is bound to multiple roles", ErrBootArtifactAdmission, binding.ArtifactName)
		}
		seenArtifacts[binding.ArtifactName] = struct{}{}
		canonical[i] = binding
	}
	for _, role := range required {
		if _, exists := seenRoles[role]; !exists {
			return BootArtifactAdmission{}, fmt.Errorf("%w: missing role %q", ErrBootArtifactAdmission, role)
		}
	}
	sort.Slice(canonical, func(i, j int) bool { return canonical[i].Role < canonical[j].Role })

	digestInput := bootArtifactAdmissionDigest{
		ProfileID:          profileID,
		OSFamily:           osFamily,
		Architecture:       architecture,
		ManifestID:         manifest.ManifestID,
		RollbackManifestID: manifest.PreviousManifestID,
		Bindings:           canonical,
	}
	encoded, err := json.Marshal(digestInput)
	if err != nil {
		return BootArtifactAdmission{}, fmt.Errorf("%w: encode canonical admission: %v", ErrBootArtifactAdmission, err)
	}
	digest := sha256.Sum256(encoded)
	return BootArtifactAdmission{
		AdmissionID:        hex.EncodeToString(digest[:]),
		ProfileID:          profileID,
		OSFamily:           osFamily,
		Architecture:       architecture,
		ManifestID:         manifest.ManifestID,
		RollbackManifestID: manifest.PreviousManifestID,
		Bindings:           canonical,
		ServingAuthorized:  false,
		HostMutation:       false,
		NetworkMutation:    false,
	}, nil
}

// VerifyBootArtifactPayloads revalidates the admission and verifies the exact
// bytes of every required artifact. Successful verification is transient
// integrity evidence only; it does not grant serving or provisioning authority.
func VerifyBootArtifactPayloads(admission BootArtifactAdmission, manifest ArtifactManifest, payloads map[string][]byte) error {
	rebuilt, err := BuildBootArtifactAdmission(admission.ProfileID, admission.OSFamily, admission.Architecture, manifest, admission.Bindings)
	if err != nil {
		return err
	}
	if rebuilt.AdmissionID != admission.AdmissionID || rebuilt.ManifestID != admission.ManifestID || rebuilt.RollbackManifestID != admission.RollbackManifestID || admission.ServingAuthorized || admission.HostMutation || admission.NetworkMutation {
		return fmt.Errorf("%w: admission identity or safety flags do not match", ErrBootArtifactAdmission)
	}
	if len(payloads) != len(admission.Bindings) {
		return fmt.Errorf("%w: payload set must exactly match required bindings", ErrBootArtifactAdmission)
	}
	artifactsByName := make(map[string]BootArtifact, len(manifest.Artifacts))
	for _, artifact := range manifest.Artifacts {
		artifactsByName[artifact.Name] = artifact
	}
	for _, binding := range admission.Bindings {
		payload, exists := payloads[binding.ArtifactName]
		if !exists {
			return fmt.Errorf("%w: payload for artifact %q is missing", ErrBootArtifactAdmission, binding.ArtifactName)
		}
		if err := VerifyArtifactBytes(artifactsByName[binding.ArtifactName], payload); err != nil {
			return fmt.Errorf("%w: artifact %q: %v", ErrBootArtifactAdmission, binding.ArtifactName, err)
		}
	}
	return nil
}

func validateExactArtifactManifest(manifest ArtifactManifest) error {
	normalizedID, err := normalizeSHA256(manifest.ManifestID)
	if err != nil || normalizedID != manifest.ManifestID {
		return fmt.Errorf("%w: manifest id is not canonical", ErrBootArtifactAdmission)
	}
	rebuilt, err := BuildArtifactManifest(manifest.ProfileID, manifest.PreviousManifestID, manifest.Artifacts)
	if err != nil {
		return fmt.Errorf("%w: invalid artifact manifest: %v", ErrBootArtifactAdmission, err)
	}
	if rebuilt.ManifestID != manifest.ManifestID || rebuilt.ProfileID != manifest.ProfileID || rebuilt.PreviousManifestID != manifest.PreviousManifestID || !reflect.DeepEqual(rebuilt.Artifacts, manifest.Artifacts) {
		return fmt.Errorf("%w: manifest content does not match exact canonical identity", ErrBootArtifactAdmission)
	}
	return nil
}

func requiredBootArtifactRoles(osFamily, architecture string) ([]BootArtifactRole, error) {
	switch osFamily {
	case "windows":
		if architecture != "amd64" {
			return nil, fmt.Errorf("%w: windows PXE currently requires amd64", ErrBootArtifactAdmission)
		}
		return []BootArtifactRole{RoleWindowsWimboot, RoleWindowsBCD, RoleWindowsBootSDI, RoleWindowsBootWIM}, nil
	case "linux":
		if architecture != "amd64" && architecture != "arm64" {
			return nil, fmt.Errorf("%w: unsupported linux architecture %q", ErrBootArtifactAdmission, architecture)
		}
		return []BootArtifactRole{RoleLinuxKernel, RoleLinuxInitrd}, nil
	default:
		return nil, fmt.Errorf("%w: unsupported OS family %q", ErrBootArtifactAdmission, osFamily)
	}
}
