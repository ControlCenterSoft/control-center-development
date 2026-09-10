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

var ErrBootSessionAdmission = errors.New("PXE boot session admission validation failed")

const (
	bootSessionAdmissionVersion = "boot-session-admission-v1"
	maxBootSessionTTLSeconds    = int64(300)
)

// BootSessionRequest identifies one bounded, single-use boot handoff. RequestID
// and MachineID are opaque inventory identities, never paths, URLs, commands,
// host names, credentials, or caller-selected executables.
type BootSessionRequest struct {
	RequestID  string `json:"requestId"`
	MachineID  string `json:"machineId"`
	TTLSeconds int64  `json:"ttlSeconds"`
}

// BootSessionAdmission grants only one short-lived boot handoff from an exact
// active PXE serving receipt. A consumer must atomically consume ReplayKey
// before booting. Provisioning, secret injection, host mutation, and network
// mutation remain outside this contract.
type BootSessionAdmission struct {
	AdmissionID               string                        `json:"admissionId"`
	AdmissionVersion          string                        `json:"admissionVersion"`
	RequestID                 string                        `json:"requestId"`
	ReplayKey                 string                        `json:"replayKey"`
	MachineID                 string                        `json:"machineId"`
	ProfileName               string                        `json:"profileName"`
	OSFamily                  string                        `json:"osFamily"`
	Architecture              string                        `json:"architecture"`
	PlanID                    string                        `json:"planId"`
	ServingReceiptID          string                        `json:"servingReceiptId"`
	ServingAdmissionID        string                        `json:"servingAdmissionId"`
	MediaSHA256               string                        `json:"mediaSha256"`
	MediaSize                 int64                         `json:"mediaSize"`
	Target                    InstallMediaPublicationTarget `json:"target"`
	UnattendedBindingID       string                        `json:"unattendedBindingId,omitempty"`
	UnattendedTemplateSHA256  string                        `json:"unattendedTemplateSha256,omitempty"`
	IssuedAtUnix              int64                         `json:"issuedAtUnix"`
	ExpiresAtUnix             int64                         `json:"expiresAtUnix"`
	MaxBootAttempts           int                           `json:"maxBootAttempts"`
	SingleUse                 bool                          `json:"singleUse"`
	AtomicConsumeRequired     bool                          `json:"atomicConsumeRequired"`
	ServingReceiptVerified    bool                          `json:"servingReceiptVerified"`
	UnattendedBindingVerified bool                          `json:"unattendedBindingVerified"`
	BootAuthorized            bool                          `json:"bootAuthorized"`
	ProvisioningAuthorized    bool                          `json:"provisioningAuthorized"`
	SecretInjectionAuthorized bool                          `json:"secretInjectionAuthorized"`
	HostMutation              bool                          `json:"hostMutation"`
	NetworkMutation           bool                          `json:"networkMutation"`
	RollbackAction            string                        `json:"rollbackAction"`
	RecoveryAction            string                        `json:"recoveryAction"`
}

type bootSessionAdmissionDigest struct {
	AdmissionVersion         string                        `json:"admissionVersion"`
	RequestID                string                        `json:"requestId"`
	ReplayKey                string                        `json:"replayKey"`
	MachineID                string                        `json:"machineId"`
	ProfileName              string                        `json:"profileName"`
	OSFamily                 string                        `json:"osFamily"`
	Architecture             string                        `json:"architecture"`
	PlanID                   string                        `json:"planId"`
	ServingReceiptID         string                        `json:"servingReceiptId"`
	ServingAdmissionID       string                        `json:"servingAdmissionId"`
	MediaSHA256              string                        `json:"mediaSha256"`
	MediaSize                int64                         `json:"mediaSize"`
	Target                   InstallMediaPublicationTarget `json:"target"`
	UnattendedBindingID      string                        `json:"unattendedBindingId,omitempty"`
	UnattendedTemplateSHA256 string                        `json:"unattendedTemplateSha256,omitempty"`
	IssuedAtUnix             int64                         `json:"issuedAtUnix"`
	ExpiresAtUnix            int64                         `json:"expiresAtUnix"`
	RollbackAction           string                        `json:"rollbackAction"`
	RecoveryAction           string                        `json:"recoveryAction"`
}

type bootSessionReplayDigest struct {
	Version          string `json:"version"`
	RequestID        string `json:"requestId"`
	MachineID        string `json:"machineId"`
	PlanID           string `json:"planId"`
	ServingReceiptID string `json:"servingReceiptId"`
	IssuedAtUnix     int64  `json:"issuedAtUnix"`
}

// BuildBootSessionAdmission revalidates the exact active-slot receipt, current
// served bytes, immutable plan identity, and (when required) exact unattended
// template binding before granting one short-lived boot handoff.
func BuildBootSessionAdmission(
	request BootSessionRequest,
	receipt InstallMediaServingReceipt,
	plan AdmittedDeploymentPlan,
	currentServedMediaPayload []byte,
	unattendedBinding *UnattendedTemplateBinding,
	unattendedTemplate []byte,
	issuedAtUnix int64,
) (BootSessionAdmission, error) {
	request.RequestID = strings.TrimSpace(request.RequestID)
	request.MachineID = strings.TrimSpace(request.MachineID)
	if !validBootSessionIdentity(request.RequestID) || !validBootSessionIdentity(request.MachineID) {
		return BootSessionAdmission{}, fmt.Errorf("%w: request and machine identities must be bounded opaque identifiers", ErrBootSessionAdmission)
	}
	if issuedAtUnix <= 0 || request.TTLSeconds <= 0 || request.TTLSeconds > maxBootSessionTTLSeconds || issuedAtUnix > int64(^uint64(0)>>1)-request.TTLSeconds {
		return BootSessionAdmission{}, fmt.Errorf("%w: invalid boot-session lifetime", ErrBootSessionAdmission)
	}
	if err := verifyBootSessionPlan(plan); err != nil {
		return BootSessionAdmission{}, err
	}
	if err := verifyBootSessionServingReceipt(receipt, plan, currentServedMediaPayload); err != nil {
		return BootSessionAdmission{}, err
	}

	requiresUnattended := planRequiresUnattended(plan)
	bindingID := ""
	templateSHA := ""
	bindingVerified := false
	if requiresUnattended {
		if unattendedBinding == nil {
			return BootSessionAdmission{}, fmt.Errorf("%w: unattended plan requires exact template binding", ErrBootSessionAdmission)
		}
		if err := verifyBootSessionUnattendedBinding(*unattendedBinding, plan, unattendedTemplate); err != nil {
			return BootSessionAdmission{}, err
		}
		bindingID = unattendedBinding.BindingID
		templateSHA = unattendedBinding.TemplateSHA256
		bindingVerified = true
	} else if unattendedBinding != nil || len(unattendedTemplate) != 0 {
		return BootSessionAdmission{}, fmt.Errorf("%w: unattended evidence is not allowed for attended plan", ErrBootSessionAdmission)
	}

	replayInput := bootSessionReplayDigest{
		Version:          "boot-session-replay-v1",
		RequestID:        request.RequestID,
		MachineID:        request.MachineID,
		PlanID:           plan.PlanID,
		ServingReceiptID: receipt.ServingReceiptID,
		IssuedAtUnix:     issuedAtUnix,
	}
	replayEncoded, err := json.Marshal(replayInput)
	if err != nil {
		return BootSessionAdmission{}, fmt.Errorf("%w: encode replay key: %v", ErrBootSessionAdmission, err)
	}
	replayDigest := sha256.Sum256(replayEncoded)
	replayKey := hex.EncodeToString(replayDigest[:])

	expiresAtUnix := issuedAtUnix + request.TTLSeconds
	rollbackAction := "expire-session-before-provisioning"
	recoveryAction := "reissue-from-fresh-serving-evidence"
	digestInput := bootSessionAdmissionDigest{
		AdmissionVersion:         bootSessionAdmissionVersion,
		RequestID:                request.RequestID,
		ReplayKey:                replayKey,
		MachineID:                request.MachineID,
		ProfileName:              plan.ProfileName,
		OSFamily:                 plan.OSFamily,
		Architecture:             plan.Architecture,
		PlanID:                   plan.PlanID,
		ServingReceiptID:         receipt.ServingReceiptID,
		ServingAdmissionID:       receipt.ServingAdmissionID,
		MediaSHA256:              receipt.MediaSHA256,
		MediaSize:                receipt.MediaSize,
		Target:                   receipt.Target,
		UnattendedBindingID:      bindingID,
		UnattendedTemplateSHA256: templateSHA,
		IssuedAtUnix:             issuedAtUnix,
		ExpiresAtUnix:            expiresAtUnix,
		RollbackAction:           rollbackAction,
		RecoveryAction:           recoveryAction,
	}
	encoded, err := json.Marshal(digestInput)
	if err != nil {
		return BootSessionAdmission{}, fmt.Errorf("%w: encode canonical admission: %v", ErrBootSessionAdmission, err)
	}
	digest := sha256.Sum256(encoded)

	return BootSessionAdmission{
		AdmissionID:               hex.EncodeToString(digest[:]),
		AdmissionVersion:          bootSessionAdmissionVersion,
		RequestID:                 request.RequestID,
		ReplayKey:                 replayKey,
		MachineID:                 request.MachineID,
		ProfileName:               plan.ProfileName,
		OSFamily:                  plan.OSFamily,
		Architecture:              plan.Architecture,
		PlanID:                    plan.PlanID,
		ServingReceiptID:          receipt.ServingReceiptID,
		ServingAdmissionID:        receipt.ServingAdmissionID,
		MediaSHA256:               receipt.MediaSHA256,
		MediaSize:                 receipt.MediaSize,
		Target:                    receipt.Target,
		UnattendedBindingID:       bindingID,
		UnattendedTemplateSHA256:  templateSHA,
		IssuedAtUnix:              issuedAtUnix,
		ExpiresAtUnix:             expiresAtUnix,
		MaxBootAttempts:           1,
		SingleUse:                 true,
		AtomicConsumeRequired:     true,
		ServingReceiptVerified:    true,
		UnattendedBindingVerified: bindingVerified,
		BootAuthorized:            true,
		ProvisioningAuthorized:    false,
		SecretInjectionAuthorized: false,
		HostMutation:              false,
		NetworkMutation:           false,
		RollbackAction:            rollbackAction,
		RecoveryAction:            recoveryAction,
	}, nil
}

// VerifyBootSessionAdmission reconstructs the admission from current exact
// evidence and rejects expiry, evidence drift, unsafe authority drift, or any
// change to the single-use/replay contract.
func VerifyBootSessionAdmission(
	admission BootSessionAdmission,
	request BootSessionRequest,
	receipt InstallMediaServingReceipt,
	plan AdmittedDeploymentPlan,
	currentServedMediaPayload []byte,
	unattendedBinding *UnattendedTemplateBinding,
	unattendedTemplate []byte,
	evaluatedAtUnix int64,
) error {
	if evaluatedAtUnix < admission.IssuedAtUnix || evaluatedAtUnix >= admission.ExpiresAtUnix {
		return fmt.Errorf("%w: boot session is not current", ErrBootSessionAdmission)
	}
	rebuilt, err := BuildBootSessionAdmission(
		request,
		receipt,
		plan,
		currentServedMediaPayload,
		unattendedBinding,
		unattendedTemplate,
		admission.IssuedAtUnix,
	)
	if err != nil {
		return err
	}
	if admission.AdmissionVersion != bootSessionAdmissionVersion ||
		admission.MaxBootAttempts != 1 ||
		!admission.SingleUse ||
		!admission.AtomicConsumeRequired ||
		!admission.ServingReceiptVerified ||
		admission.BootAuthorized != true ||
		admission.ProvisioningAuthorized ||
		admission.SecretInjectionAuthorized ||
		admission.HostMutation ||
		admission.NetworkMutation {
		return fmt.Errorf("%w: admission safety flags do not match", ErrBootSessionAdmission)
	}
	if planRequiresUnattended(plan) != admission.UnattendedBindingVerified {
		return fmt.Errorf("%w: unattended verification flag does not match plan", ErrBootSessionAdmission)
	}
	for _, value := range []string{admission.AdmissionID, admission.ReplayKey, admission.ServingReceiptID, admission.MediaSHA256} {
		normalized, normalizeErr := normalizeSHA256(value)
		if normalizeErr != nil || normalized != value {
			return fmt.Errorf("%w: non-canonical digest identity", ErrBootSessionAdmission)
		}
	}
	if !reflect.DeepEqual(rebuilt, admission) {
		return fmt.Errorf("%w: admission no longer matches exact boot-session evidence", ErrBootSessionAdmission)
	}
	return nil
}

func verifyBootSessionPlan(plan AdmittedDeploymentPlan) error {
	if err := validateAdmittedPlanForMediaPublication(plan); err != nil {
		return fmt.Errorf("%w: deployment plan: %v", ErrBootSessionAdmission, err)
	}
	return nil
}

func verifyBootSessionServingReceipt(receipt InstallMediaServingReceipt, plan AdmittedDeploymentPlan, currentServedMediaPayload []byte) error {
	if receipt.PlanID != plan.PlanID || receipt.Target.Channel != PublicationChannelPXE {
		return fmt.Errorf("%w: serving receipt is not bound to exact PXE plan", ErrBootSessionAdmission)
	}
	if !receipt.ServingAdmissionVerified ||
		!receipt.ActiveSlotVerified ||
		!receipt.ActiveMediaIntegrityVerified ||
		!receipt.ServingActive ||
		receipt.ServingAuthorized ||
		receipt.BootAuthorized ||
		receipt.ProvisioningAuthorized ||
		receipt.HostMutation ||
		receipt.NetworkMutation {
		return fmt.Errorf("%w: serving receipt safety flags do not match", ErrBootSessionAdmission)
	}
	if err := validateInstallMediaPublicationTarget(receipt.Target); err != nil {
		return fmt.Errorf("%w: serving target: %v", ErrBootSessionAdmission, err)
	}
	wantRollback := "disable-serving-slot"
	if receipt.PreviousMediaRecipeID != "" {
		wantRollback = "restore-previous-serving-media"
	}
	if receipt.RollbackAction != wantRollback ||
		receipt.RecoveryAction != "rebuild-serving-receipt-from-current-active-slot-evidence" {
		return fmt.Errorf("%w: serving rollback/recovery contract drift", ErrBootSessionAdmission)
	}
	for _, value := range []string{
		receipt.ServingReceiptID,
		receipt.ServingAdmissionID,
		receipt.PublicationReceiptID,
		receipt.PublicationAdmissionID,
		receipt.PlanID,
		receipt.RecipeID,
		receipt.MediaSHA256,
	} {
		normalized, normalizeErr := normalizeSHA256(value)
		if normalizeErr != nil || normalized != value {
			return fmt.Errorf("%w: serving receipt contains non-canonical identity", ErrBootSessionAdmission)
		}
	}
	if len(currentServedMediaPayload) == 0 || int64(len(currentServedMediaPayload)) != receipt.MediaSize {
		return fmt.Errorf("%w: current served media size does not match receipt", ErrBootSessionAdmission)
	}
	mediaDigest := sha256.Sum256(currentServedMediaPayload)
	if hex.EncodeToString(mediaDigest[:]) != receipt.MediaSHA256 {
		return fmt.Errorf("%w: current served media checksum does not match receipt", ErrBootSessionAdmission)
	}
	digestInput := installMediaServingReceiptDigest{
		ReceiptVersion:         receipt.ReceiptVersion,
		ServingAdmissionID:     receipt.ServingAdmissionID,
		PublicationReceiptID:   receipt.PublicationReceiptID,
		PublicationAdmissionID: receipt.PublicationAdmissionID,
		PlanID:                 receipt.PlanID,
		RecipeID:               receipt.RecipeID,
		MediaSHA256:            receipt.MediaSHA256,
		MediaSize:              receipt.MediaSize,
		Target:                 receipt.Target,
		PreviousMediaRecipeID:  receipt.PreviousMediaRecipeID,
		RollbackAction:         receipt.RollbackAction,
		RecoveryAction:         receipt.RecoveryAction,
	}
	encoded, err := json.Marshal(digestInput)
	if err != nil {
		return fmt.Errorf("%w: encode serving receipt identity: %v", ErrBootSessionAdmission, err)
	}
	digest := sha256.Sum256(encoded)
	if receipt.ReceiptVersion != installMediaServingReceiptVersion || hex.EncodeToString(digest[:]) != receipt.ServingReceiptID {
		return fmt.Errorf("%w: serving receipt identity drift", ErrBootSessionAdmission)
	}
	return nil
}

func verifyBootSessionUnattendedBinding(binding UnattendedTemplateBinding, plan AdmittedDeploymentPlan, template []byte) error {
	if binding.PlanID != plan.PlanID ||
		binding.ProfileName != plan.ProfileName ||
		binding.OSFamily != plan.OSFamily ||
		binding.Architecture != plan.Architecture {
		return fmt.Errorf("%w: unattended binding is not bound to exact plan", ErrBootSessionAdmission)
	}
	if binding.AdmissionID != plan.AdmissionID ||
		binding.ManifestID != plan.ManifestID ||
		binding.RollbackManifestID != plan.RollbackManifestID {
		return fmt.Errorf("%w: unattended boot lineage does not match exact plan", ErrBootSessionAdmission)
	}
	if err := validateConfigKind(plan.OSFamily, binding.ConfigKind); err != nil {
		return fmt.Errorf("%w: unattended config kind: %v", ErrBootSessionAdmission, err)
	}
	wantRollback := "none-preprovisioning"
	if binding.RollbackManifestID != "" {
		wantRollback = "restore-previous-boot-manifest"
	}
	if binding.RollbackAction != wantRollback ||
		binding.RecoveryAction != "rebuild-binding-from-current-admission" {
		return fmt.Errorf("%w: unattended rollback/recovery contract drift", ErrBootSessionAdmission)
	}
	if !binding.SecretInjectionDeferred ||
		!binding.IntegrityVerified ||
		binding.ServingAuthorized ||
		binding.ProvisioningAuthorized ||
		binding.HostMutation ||
		binding.NetworkMutation {
		return fmt.Errorf("%w: unattended binding safety flags do not match", ErrBootSessionAdmission)
	}
	if len(template) == 0 || int64(len(template)) != binding.TemplateSize {
		return fmt.Errorf("%w: unattended template size does not match binding", ErrBootSessionAdmission)
	}
	templateDigest := sha256.Sum256(template)
	if hex.EncodeToString(templateDigest[:]) != binding.TemplateSHA256 {
		return fmt.Errorf("%w: unattended template checksum does not match binding", ErrBootSessionAdmission)
	}
	secretRefs, err := extractSecretRefs(template)
	if err != nil || !reflect.DeepEqual(secretRefs, binding.SecretRefs) {
		return fmt.Errorf("%w: unattended secret-reference evidence drift", ErrBootSessionAdmission)
	}
	digestInput := unattendedTemplateBindingDigest{
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
	}
	encoded, err := json.Marshal(digestInput)
	if err != nil {
		return fmt.Errorf("%w: encode unattended binding identity: %v", ErrBootSessionAdmission, err)
	}
	digest := sha256.Sum256(encoded)
	if hex.EncodeToString(digest[:]) != binding.BindingID {
		return fmt.Errorf("%w: unattended binding identity drift", ErrBootSessionAdmission)
	}
	return nil
}

func planRequiresUnattended(plan AdmittedDeploymentPlan) bool {
	for _, step := range plan.Steps {
		if step.Action == "apply-unattended-profile" || step.Action == "apply-autoinstall-profile" {
			return true
		}
	}
	return false
}

func validBootSessionIdentity(value string) bool {
	if value == "" || len(value) > 128 || strings.TrimSpace(value) != value {
		return false
	}
	for _, r := range value {
		if (r >= 'a' && r <= 'z') ||
			(r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') ||
			r == '.' || r == '_' || r == '-' || r == ':' {
			continue
		}
		return false
	}
	return true
}
