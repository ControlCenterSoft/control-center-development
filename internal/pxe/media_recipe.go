package pxe

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
)

var ErrInstallMediaRecipe = errors.New("PXE install media recipe validation failed")

type InstallMediaSourceKind string

const (
	SourceWindowsInstallISO InstallMediaSourceKind = "windows-install-iso"
	SourceLinuxInstallISO   InstallMediaSourceKind = "linux-install-iso"

	mediaRecipeVersion    = "install-media-v1"
	mediaFilesystemPolicy = "iso9660-udf"
	mediaFileOrder        = "lexicographic"
	mediaPermissions      = "normalized-readonly"
)

// InstallMediaSourceEvidence describes immutable source installation media.
// The payload is never retained by the recipe; exact size and SHA-256 are
// revalidated before recipe evidence is emitted.
type InstallMediaSourceEvidence struct {
	Kind   InstallMediaSourceKind `json:"kind"`
	SHA256 string                 `json:"sha256"`
	Size   int64                  `json:"size"`
}

// InstallMediaBuildParameters contains only reproducible, infrastructure-neutral
// generation inputs. TimestampUnix must remain zero and ordering/permissions
// policies are fixed so the same logical inputs cannot silently pick up host
// clock, filesystem, or tool defaults.
type InstallMediaBuildParameters struct {
	RecipeVersion     string `json:"recipeVersion"`
	FilesystemPolicy  string `json:"filesystemPolicy"`
	VolumeLabel       string `json:"volumeLabel"`
	TimestampUnix     int64  `json:"timestampUnix"`
	FileOrder         string `json:"fileOrder"`
	PermissionsPolicy string `json:"permissionsPolicy"`
}

// InstallMediaRecipe is immutable evidence for a future install-media builder.
// It binds source media, admitted boot artifacts, unattended template evidence,
// reproducible parameters, and optional previous-recipe rollback lineage. It
// never builds, serves, provisions, mounts, or mutates anything itself.
type InstallMediaRecipe struct {
	RecipeID                  string                      `json:"recipeId"`
	PlanID                    string                      `json:"planId"`
	AdmissionID               string                      `json:"admissionId"`
	ManifestID                string                      `json:"manifestId"`
	UnattendedBindingID       string                      `json:"unattendedBindingId"`
	Source                    InstallMediaSourceEvidence  `json:"source"`
	Parameters                InstallMediaBuildParameters `json:"parameters"`
	PreviousMediaRecipeID     string                      `json:"previousMediaRecipeId,omitempty"`
	RollbackAction            string                      `json:"rollbackAction"`
	RecoveryAction            string                      `json:"recoveryAction"`
	SourceIntegrityVerified   bool                        `json:"sourceIntegrityVerified"`
	BootIntegrityVerified     bool                        `json:"bootIntegrityVerified"`
	TemplateIntegrityVerified bool                        `json:"templateIntegrityVerified"`
	Reproducible              bool                        `json:"reproducible"`
	BuildAuthorized           bool                        `json:"buildAuthorized"`
	ServingAuthorized         bool                        `json:"servingAuthorized"`
	ProvisioningAuthorized    bool                        `json:"provisioningAuthorized"`
	HostMutation              bool                        `json:"hostMutation"`
	NetworkMutation           bool                        `json:"networkMutation"`
}

type installMediaRecipeDigest struct {
	PlanID                string                      `json:"planId"`
	AdmissionID           string                      `json:"admissionId"`
	ManifestID            string                      `json:"manifestId"`
	UnattendedBindingID   string                      `json:"unattendedBindingId"`
	Source                InstallMediaSourceEvidence  `json:"source"`
	Parameters            InstallMediaBuildParameters `json:"parameters"`
	PreviousMediaRecipeID string                      `json:"previousMediaRecipeId,omitempty"`
	RollbackAction        string                      `json:"rollbackAction"`
	RecoveryAction        string                      `json:"recoveryAction"`
}

func DefaultInstallMediaBuildParameters(osFamily string) (InstallMediaBuildParameters, error) {
	var label string
	switch osFamily {
	case "windows":
		label = "CC-WINDOWS"
	case "linux":
		label = "CC-LINUX"
	default:
		return InstallMediaBuildParameters{}, fmt.Errorf("%w: unsupported OS family %q", ErrInstallMediaRecipe, osFamily)
	}
	return InstallMediaBuildParameters{
		RecipeVersion:     mediaRecipeVersion,
		FilesystemPolicy:  mediaFilesystemPolicy,
		VolumeLabel:       label,
		TimestampUnix:     0,
		FileOrder:         mediaFileOrder,
		PermissionsPolicy: mediaPermissions,
	}, nil
}

func BuildInstallMediaSourceEvidence(kind InstallMediaSourceKind, payload []byte) (InstallMediaSourceEvidence, error) {
	if len(payload) == 0 {
		return InstallMediaSourceEvidence{}, fmt.Errorf("%w: source media must be non-empty", ErrInstallMediaRecipe)
	}
	if kind != SourceWindowsInstallISO && kind != SourceLinuxInstallISO {
		return InstallMediaSourceEvidence{}, fmt.Errorf("%w: unsupported source media kind %q", ErrInstallMediaRecipe, kind)
	}
	digest := sha256.Sum256(payload)
	return InstallMediaSourceEvidence{
		Kind:   kind,
		SHA256: hex.EncodeToString(digest[:]),
		Size:   int64(len(payload)),
	}, nil
}

func VerifyInstallMediaSourceEvidence(source InstallMediaSourceEvidence, payload []byte) error {
	if source.Size <= 0 || int64(len(payload)) != source.Size {
		return fmt.Errorf("%w: source media size mismatch", ErrInstallMediaRecipe)
	}
	normalized, err := normalizeSHA256(source.SHA256)
	if err != nil {
		return fmt.Errorf("%w: source media checksum: %v", ErrInstallMediaRecipe, err)
	}
	if source.Kind != SourceWindowsInstallISO && source.Kind != SourceLinuxInstallISO {
		return fmt.Errorf("%w: unsupported source media kind %q", ErrInstallMediaRecipe, source.Kind)
	}
	digest := sha256.Sum256(payload)
	if hex.EncodeToString(digest[:]) != normalized {
		return fmt.Errorf("%w: source media checksum mismatch", ErrInstallMediaRecipe)
	}
	return nil
}

// BuildInstallMediaRecipe revalidates every upstream PXE evidence layer before
// emitting a deterministic generation recipe. The source payload is checked but
// not retained. A heavy ISO build remains a separate Release Engine concern.
func BuildInstallMediaRecipe(profile Profile, plan AdmittedDeploymentPlan, admission BootArtifactAdmission, manifest ArtifactManifest, bootPayloads map[string][]byte, binding UnattendedTemplateBinding, template []byte, source InstallMediaSourceEvidence, sourcePayload []byte, parameters InstallMediaBuildParameters, previousMediaRecipeID string) (InstallMediaRecipe, error) {
	if err := VerifyUnattendedTemplateBinding(binding, profile, plan, admission, manifest, bootPayloads, template); err != nil {
		return InstallMediaRecipe{}, fmt.Errorf("%w: unattended binding: %v", ErrInstallMediaRecipe, err)
	}
	if err := VerifyInstallMediaSourceEvidence(source, sourcePayload); err != nil {
		return InstallMediaRecipe{}, err
	}
	if err := validateInstallMediaSourceForOS(plan.OSFamily, source.Kind); err != nil {
		return InstallMediaRecipe{}, err
	}
	if err := validateInstallMediaBuildParameters(parameters); err != nil {
		return InstallMediaRecipe{}, err
	}

	previousMediaRecipeID = strings.TrimSpace(previousMediaRecipeID)
	if previousMediaRecipeID != "" {
		normalized, err := normalizeSHA256(previousMediaRecipeID)
		if err != nil {
			return InstallMediaRecipe{}, fmt.Errorf("%w: previous media recipe id: %v", ErrInstallMediaRecipe, err)
		}
		previousMediaRecipeID = normalized
	}

	rollbackAction := "none-prebuild"
	if previousMediaRecipeID != "" {
		rollbackAction = "restore-previous-media-recipe"
	}
	recoveryAction := "rebuild-media-recipe-from-current-evidence"

	digestInput := installMediaRecipeDigest{
		PlanID:                plan.PlanID,
		AdmissionID:           plan.AdmissionID,
		ManifestID:            plan.ManifestID,
		UnattendedBindingID:   binding.BindingID,
		Source:                source,
		Parameters:            parameters,
		PreviousMediaRecipeID: previousMediaRecipeID,
		RollbackAction:        rollbackAction,
		RecoveryAction:        recoveryAction,
	}
	encoded, err := json.Marshal(digestInput)
	if err != nil {
		return InstallMediaRecipe{}, fmt.Errorf("%w: encode canonical recipe: %v", ErrInstallMediaRecipe, err)
	}
	digest := sha256.Sum256(encoded)
	recipeID := hex.EncodeToString(digest[:])
	if previousMediaRecipeID == recipeID {
		return InstallMediaRecipe{}, fmt.Errorf("%w: previous media recipe cannot reference the new recipe", ErrInstallMediaRecipe)
	}

	return InstallMediaRecipe{
		RecipeID:                  recipeID,
		PlanID:                    plan.PlanID,
		AdmissionID:               plan.AdmissionID,
		ManifestID:                plan.ManifestID,
		UnattendedBindingID:       binding.BindingID,
		Source:                    source,
		Parameters:                parameters,
		PreviousMediaRecipeID:     previousMediaRecipeID,
		RollbackAction:            rollbackAction,
		RecoveryAction:            recoveryAction,
		SourceIntegrityVerified:   true,
		BootIntegrityVerified:     true,
		TemplateIntegrityVerified: true,
		Reproducible:              true,
		BuildAuthorized:           false,
		ServingAuthorized:         false,
		ProvisioningAuthorized:    false,
		HostMutation:              false,
		NetworkMutation:           false,
	}, nil
}

// VerifyInstallMediaRecipe rebuilds the complete recipe from current immutable
// inputs and fails closed on source, boot, template, lineage, parameter, or
// safety-flag drift.
func VerifyInstallMediaRecipe(recipe InstallMediaRecipe, profile Profile, plan AdmittedDeploymentPlan, admission BootArtifactAdmission, manifest ArtifactManifest, bootPayloads map[string][]byte, binding UnattendedTemplateBinding, template []byte, sourcePayload []byte) error {
	rebuilt, err := BuildInstallMediaRecipe(profile, plan, admission, manifest, bootPayloads, binding, template, recipe.Source, sourcePayload, recipe.Parameters, recipe.PreviousMediaRecipeID)
	if err != nil {
		return err
	}
	if !recipe.SourceIntegrityVerified || !recipe.BootIntegrityVerified || !recipe.TemplateIntegrityVerified || !recipe.Reproducible || recipe.BuildAuthorized || recipe.ServingAuthorized || recipe.ProvisioningAuthorized || recipe.HostMutation || recipe.NetworkMutation {
		return fmt.Errorf("%w: recipe safety flags do not match", ErrInstallMediaRecipe)
	}
	if !reflect.DeepEqual(rebuilt, recipe) {
		return fmt.Errorf("%w: recipe no longer matches exact generation evidence", ErrInstallMediaRecipe)
	}
	return nil
}

func validateInstallMediaSourceForOS(osFamily string, kind InstallMediaSourceKind) error {
	switch osFamily {
	case "windows":
		if kind != SourceWindowsInstallISO {
			return fmt.Errorf("%w: windows requires %q", ErrInstallMediaRecipe, SourceWindowsInstallISO)
		}
	case "linux":
		if kind != SourceLinuxInstallISO {
			return fmt.Errorf("%w: linux requires %q", ErrInstallMediaRecipe, SourceLinuxInstallISO)
		}
	default:
		return fmt.Errorf("%w: unsupported OS family %q", ErrInstallMediaRecipe, osFamily)
	}
	return nil
}

func validateInstallMediaBuildParameters(parameters InstallMediaBuildParameters) error {
	if parameters.RecipeVersion != mediaRecipeVersion || parameters.FilesystemPolicy != mediaFilesystemPolicy || parameters.TimestampUnix != 0 || parameters.FileOrder != mediaFileOrder || parameters.PermissionsPolicy != mediaPermissions {
		return fmt.Errorf("%w: non-reproducible build parameters", ErrInstallMediaRecipe)
	}
	if !validMediaVolumeLabel(parameters.VolumeLabel) {
		return fmt.Errorf("%w: invalid deterministic volume label", ErrInstallMediaRecipe)
	}
	return nil
}

func validMediaVolumeLabel(label string) bool {
	if label == "" || len(label) > 32 || strings.TrimSpace(label) != label {
		return false
	}
	for _, r := range label {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			continue
		}
		return false
	}
	return true
}
