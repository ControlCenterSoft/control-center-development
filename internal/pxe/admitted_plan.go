package pxe

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
)

var ErrAdmittedDeploymentPlan = errors.New("PXE admitted deployment plan validation failed")

// AdmittedDeploymentPlan binds a deterministic deployment plan to one exact
// boot-artifact admission and verified immutable payload set. It is evidence
// only: it never authorizes serving, provisioning, host mutation, or network
// mutation.
type AdmittedDeploymentPlan struct {
	PlanID                    string `json:"planId"`
	ProfileName               string `json:"profileName"`
	OSFamily                  string `json:"osFamily"`
	Architecture              string `json:"architecture"`
	AdmissionID               string `json:"admissionId"`
	ManifestID                string `json:"manifestId"`
	RollbackManifestID        string `json:"rollbackManifestId,omitempty"`
	Steps                     []Step `json:"steps"`
	ArtifactIntegrityVerified bool   `json:"artifactIntegrityVerified"`
	ServingAuthorized         bool   `json:"servingAuthorized"`
	HostMutation              bool   `json:"hostMutation"`
	NetworkMutation           bool   `json:"networkMutation"`
}

type admittedDeploymentPlanDigest struct {
	ProfileName        string `json:"profileName"`
	OSFamily           string `json:"osFamily"`
	Architecture       string `json:"architecture"`
	AdmissionID        string `json:"admissionId"`
	ManifestID         string `json:"manifestId"`
	RollbackManifestID string `json:"rollbackManifestId,omitempty"`
	Steps              []Step `json:"steps"`
}

// BuildAdmittedDeploymentPlan verifies the exact manifest and payload bytes,
// then binds that evidence to the deterministic PXE deployment plan. The
// returned object remains plan-only and grants no serving authority.
func BuildAdmittedDeploymentPlan(profile Profile, admission BootArtifactAdmission, manifest ArtifactManifest, payloads map[string][]byte) (AdmittedDeploymentPlan, error) {
	basePlan, err := BuildPlan(profile)
	if err != nil {
		return AdmittedDeploymentPlan{}, fmt.Errorf("%w: build deployment plan: %v", ErrAdmittedDeploymentPlan, err)
	}
	if admission.ProfileID != basePlan.ProfileName || admission.OSFamily != basePlan.OSFamily || admission.Architecture != basePlan.Architecture {
		return AdmittedDeploymentPlan{}, fmt.Errorf("%w: profile and admission identities do not match", ErrAdmittedDeploymentPlan)
	}
	if err := VerifyBootArtifactPayloads(admission, manifest, payloads); err != nil {
		return AdmittedDeploymentPlan{}, fmt.Errorf("%w: verify artifact admission: %v", ErrAdmittedDeploymentPlan, err)
	}
	if admission.ManifestID != manifest.ManifestID || admission.RollbackManifestID != manifest.PreviousManifestID {
		return AdmittedDeploymentPlan{}, fmt.Errorf("%w: admission is not bound to the exact manifest lineage", ErrAdmittedDeploymentPlan)
	}

	steps := append([]Step(nil), basePlan.Steps...)
	digestInput := admittedDeploymentPlanDigest{
		ProfileName:        basePlan.ProfileName,
		OSFamily:           basePlan.OSFamily,
		Architecture:       basePlan.Architecture,
		AdmissionID:        admission.AdmissionID,
		ManifestID:         manifest.ManifestID,
		RollbackManifestID: manifest.PreviousManifestID,
		Steps:              steps,
	}
	encoded, err := json.Marshal(digestInput)
	if err != nil {
		return AdmittedDeploymentPlan{}, fmt.Errorf("%w: encode canonical plan: %v", ErrAdmittedDeploymentPlan, err)
	}
	digest := sha256.Sum256(encoded)

	return AdmittedDeploymentPlan{
		PlanID:                    hex.EncodeToString(digest[:]),
		ProfileName:               basePlan.ProfileName,
		OSFamily:                  basePlan.OSFamily,
		Architecture:              basePlan.Architecture,
		AdmissionID:               admission.AdmissionID,
		ManifestID:                manifest.ManifestID,
		RollbackManifestID:        manifest.PreviousManifestID,
		Steps:                     steps,
		ArtifactIntegrityVerified: true,
		ServingAuthorized:         false,
		HostMutation:              false,
		NetworkMutation:           false,
	}, nil
}

// VerifyAdmittedDeploymentPlan re-verifies the payload bytes and exact
// admission immediately before a separate serving layer consumes this evidence.
// It fails closed on any plan, admission, manifest, payload, or safety-flag
// drift and never grants serving authority itself.
func VerifyAdmittedDeploymentPlan(plan AdmittedDeploymentPlan, profile Profile, admission BootArtifactAdmission, manifest ArtifactManifest, payloads map[string][]byte) error {
	rebuilt, err := BuildAdmittedDeploymentPlan(profile, admission, manifest, payloads)
	if err != nil {
		return err
	}
	if !plan.ArtifactIntegrityVerified || plan.ServingAuthorized || plan.HostMutation || plan.NetworkMutation {
		return fmt.Errorf("%w: plan safety flags do not match", ErrAdmittedDeploymentPlan)
	}
	if !reflect.DeepEqual(rebuilt, plan) {
		return fmt.Errorf("%w: plan no longer matches exact admission evidence", ErrAdmittedDeploymentPlan)
	}
	return nil
}
