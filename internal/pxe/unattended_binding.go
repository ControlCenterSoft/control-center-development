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
	"unicode/utf8"
)

var ErrUnattendedTemplateBinding = errors.New("PXE unattended template binding validation failed")

const maxUnattendedTemplateBytes = 4 << 20

type UnattendedConfigKind string

const (
	ConfigWindowsUnattendXML UnattendedConfigKind = "windows-unattend-xml"
	ConfigLinuxAutoinstall   UnattendedConfigKind = "linux-autoinstall"
)

// UnattendedTemplateBinding binds one deterministic, non-rendered unattended
// template to exact admitted PXE evidence. Secret values are deliberately not
// accepted by this contract: the template may reference them only through
// {{secret:<ref>}} placeholders, which are resolved by a later bounded layer.
// The binding is evidence only and does not authorize serving or provisioning.
type UnattendedTemplateBinding struct {
	BindingID               string               `json:"bindingId"`
	PlanID                  string               `json:"planId"`
	ProfileName             string               `json:"profileName"`
	OSFamily                string               `json:"osFamily"`
	Architecture            string               `json:"architecture"`
	AdmissionID             string               `json:"admissionId"`
	ManifestID              string               `json:"manifestId"`
	RollbackManifestID      string               `json:"rollbackManifestId,omitempty"`
	ConfigKind              UnattendedConfigKind `json:"configKind"`
	TemplateSHA256          string               `json:"templateSha256"`
	TemplateSize            int64                `json:"templateSize"`
	SecretRefs              []string             `json:"secretRefs"`
	SecretInjectionDeferred bool                 `json:"secretInjectionDeferred"`
	RollbackAction          string               `json:"rollbackAction"`
	RecoveryAction          string               `json:"recoveryAction"`
	IntegrityVerified       bool                 `json:"integrityVerified"`
	ServingAuthorized       bool                 `json:"servingAuthorized"`
	ProvisioningAuthorized  bool                 `json:"provisioningAuthorized"`
	HostMutation            bool                 `json:"hostMutation"`
	NetworkMutation         bool                 `json:"networkMutation"`
}

type unattendedTemplateBindingDigest struct {
	PlanID             string               `json:"planId"`
	ProfileName        string               `json:"profileName"`
	OSFamily           string               `json:"osFamily"`
	Architecture       string               `json:"architecture"`
	AdmissionID        string               `json:"admissionId"`
	ManifestID         string               `json:"manifestId"`
	RollbackManifestID string               `json:"rollbackManifestId,omitempty"`
	ConfigKind         UnattendedConfigKind `json:"configKind"`
	TemplateSHA256     string               `json:"templateSha256"`
	TemplateSize       int64                `json:"templateSize"`
	SecretRefs         []string             `json:"secretRefs"`
	RollbackAction     string               `json:"rollbackAction"`
	RecoveryAction     string               `json:"recoveryAction"`
}

// BuildUnattendedTemplateBinding verifies the admitted boot plan again and
// binds it to one exact textual unattended template. It never accepts rendered
// secret material and performs no serving, provisioning, host, or network
// mutation.
func BuildUnattendedTemplateBinding(profile Profile, plan AdmittedDeploymentPlan, admission BootArtifactAdmission, manifest ArtifactManifest, bootPayloads map[string][]byte, kind UnattendedConfigKind, template []byte) (UnattendedTemplateBinding, error) {
	if err := VerifyAdmittedDeploymentPlan(plan, profile, admission, manifest, bootPayloads); err != nil {
		return UnattendedTemplateBinding{}, fmt.Errorf("%w: admitted plan: %v", ErrUnattendedTemplateBinding, err)
	}
	if !profile.Unattended {
		return UnattendedTemplateBinding{}, fmt.Errorf("%w: profile does not enable unattended provisioning", ErrUnattendedTemplateBinding)
	}
	if err := validateConfigKind(plan.OSFamily, kind); err != nil {
		return UnattendedTemplateBinding{}, err
	}
	if len(template) == 0 || len(template) > maxUnattendedTemplateBytes || !utf8.Valid(template) || strings.IndexByte(string(template), 0) >= 0 {
		return UnattendedTemplateBinding{}, fmt.Errorf("%w: template must be non-empty UTF-8 text within %d bytes", ErrUnattendedTemplateBinding, maxUnattendedTemplateBytes)
	}

	secretRefs, err := extractSecretRefs(template)
	if err != nil {
		return UnattendedTemplateBinding{}, err
	}
	templateDigest := sha256.Sum256(template)
	rollbackAction := "none-preprovisioning"
	if plan.RollbackManifestID != "" {
		rollbackAction = "restore-previous-boot-manifest"
	}
	recoveryAction := "rebuild-binding-from-current-admission"

	digestInput := unattendedTemplateBindingDigest{
		PlanID:             plan.PlanID,
		ProfileName:        plan.ProfileName,
		OSFamily:           plan.OSFamily,
		Architecture:       plan.Architecture,
		AdmissionID:        plan.AdmissionID,
		ManifestID:         plan.ManifestID,
		RollbackManifestID: plan.RollbackManifestID,
		ConfigKind:         kind,
		TemplateSHA256:     hex.EncodeToString(templateDigest[:]),
		TemplateSize:       int64(len(template)),
		SecretRefs:         secretRefs,
		RollbackAction:     rollbackAction,
		RecoveryAction:     recoveryAction,
	}
	encoded, err := json.Marshal(digestInput)
	if err != nil {
		return UnattendedTemplateBinding{}, fmt.Errorf("%w: encode canonical binding: %v", ErrUnattendedTemplateBinding, err)
	}
	bindingDigest := sha256.Sum256(encoded)

	return UnattendedTemplateBinding{
		BindingID:               hex.EncodeToString(bindingDigest[:]),
		PlanID:                  plan.PlanID,
		ProfileName:             plan.ProfileName,
		OSFamily:                plan.OSFamily,
		Architecture:            plan.Architecture,
		AdmissionID:             plan.AdmissionID,
		ManifestID:              plan.ManifestID,
		RollbackManifestID:      plan.RollbackManifestID,
		ConfigKind:              kind,
		TemplateSHA256:          digestInput.TemplateSHA256,
		TemplateSize:            digestInput.TemplateSize,
		SecretRefs:              append([]string(nil), secretRefs...),
		SecretInjectionDeferred: true,
		RollbackAction:          rollbackAction,
		RecoveryAction:          recoveryAction,
		IntegrityVerified:       true,
		ServingAuthorized:       false,
		ProvisioningAuthorized:  false,
		HostMutation:            false,
		NetworkMutation:         false,
	}, nil
}

// VerifyUnattendedTemplateBinding revalidates all exact boot evidence and the
// template bytes, then compares the complete immutable binding. Any evidence,
// template, or safety-flag drift fails closed.
func VerifyUnattendedTemplateBinding(binding UnattendedTemplateBinding, profile Profile, plan AdmittedDeploymentPlan, admission BootArtifactAdmission, manifest ArtifactManifest, bootPayloads map[string][]byte, template []byte) error {
	rebuilt, err := BuildUnattendedTemplateBinding(profile, plan, admission, manifest, bootPayloads, binding.ConfigKind, template)
	if err != nil {
		return err
	}
	if !binding.SecretInjectionDeferred || !binding.IntegrityVerified || binding.ServingAuthorized || binding.ProvisioningAuthorized || binding.HostMutation || binding.NetworkMutation {
		return fmt.Errorf("%w: binding safety flags do not match", ErrUnattendedTemplateBinding)
	}
	if !reflect.DeepEqual(rebuilt, binding) {
		return fmt.Errorf("%w: binding no longer matches exact deployment evidence", ErrUnattendedTemplateBinding)
	}
	return nil
}

func validateConfigKind(osFamily string, kind UnattendedConfigKind) error {
	switch osFamily {
	case "windows":
		if kind != ConfigWindowsUnattendXML {
			return fmt.Errorf("%w: windows requires %q", ErrUnattendedTemplateBinding, ConfigWindowsUnattendXML)
		}
	case "linux":
		if kind != ConfigLinuxAutoinstall {
			return fmt.Errorf("%w: linux requires %q", ErrUnattendedTemplateBinding, ConfigLinuxAutoinstall)
		}
	default:
		return fmt.Errorf("%w: unsupported OS family %q", ErrUnattendedTemplateBinding, osFamily)
	}
	return nil
}

func extractSecretRefs(template []byte) ([]string, error) {
	const marker = "{{secret"
	const prefix = "{{secret:"
	const suffix = "}}"
	text := string(template)
	refs := make(map[string]struct{})
	for offset := 0; ; {
		start := strings.Index(text[offset:], marker)
		if start < 0 {
			break
		}
		start += offset
		if !strings.HasPrefix(text[start:], prefix) {
			return nil, fmt.Errorf("%w: malformed secret reference", ErrUnattendedTemplateBinding)
		}
		valueStart := start + len(prefix)
		endRelative := strings.Index(text[valueStart:], suffix)
		if endRelative < 0 {
			return nil, fmt.Errorf("%w: unterminated secret reference", ErrUnattendedTemplateBinding)
		}
		end := valueStart + endRelative
		ref := text[valueStart:end]
		if !validSecretRef(ref) {
			return nil, fmt.Errorf("%w: invalid secret reference %q", ErrUnattendedTemplateBinding, ref)
		}
		refs[ref] = struct{}{}
		offset = end + len(suffix)
	}

	canonical := make([]string, 0, len(refs))
	for ref := range refs {
		canonical = append(canonical, ref)
	}
	sort.Strings(canonical)
	return canonical, nil
}

func validSecretRef(ref string) bool {
	if ref == "" || len(ref) > 128 || strings.TrimSpace(ref) != ref {
		return false
	}
	for _, r := range ref {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-' {
			continue
		}
		return false
	}
	return true
}
