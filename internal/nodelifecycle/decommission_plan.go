package nodelifecycle

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

const DecommissionPlanContractV1 = "node.decommission-plan/v1"

// DecommissionPlan is a side-effect-free instruction for retiring a drained
// node. An executor must still admit every mutating lifecycle step through an
// approved Change, durable Job, fresh evidence and optimistic concurrency.
type DecommissionPlan struct {
	ContractVersion        string              `json:"contract_version"`
	PlanID                 string              `json:"plan_id"`
	NodeID                 string              `json:"node_id"`
	ScopeID                string              `json:"scope_id"`
	OwnerScope             string              `json:"owner_scope"`
	BasedOnGeneration      uint64              `json:"based_on_generation"`
	BasedOnResourceVersion string              `json:"based_on_resource_version"`
	Steps                  []OperationPlanStep `json:"steps"`
	RequiresApprovedChange bool                `json:"requires_approved_change"`
	RequiresDurableJob     bool                `json:"requires_durable_job"`
	RequiresAudit          bool                `json:"requires_audit"`
	PlanOnly               bool                `json:"plan_only"`
	LifecycleMutation      bool                `json:"lifecycle_mutation"`
	PlacementMutation      bool                `json:"placement_mutation"`
	HostMutation           bool                `json:"host_mutation"`
}

// BuildDecommissionPlan coordinates maintenance -> removing -> retired without
// applying any transition. Maintenance is required so decommission cannot
// bypass the drain contract.
func BuildDecommissionPlan(current NodeLifecycle) (DecommissionPlan, error) {
	if err := Validate(current); err != nil {
		return DecommissionPlan{}, fmt.Errorf("%w: lifecycle: %v", ErrInvalidOperationPlan, err)
	}
	if current.State != StateMaintenance {
		return DecommissionPlan{}, invalidOperation("decommission requires maintenance state after a completed drain")
	}

	steps := []OperationPlanStep{
		{Order: 1, Action: "removal.verify-approved", RequiredEvidence: []EvidenceCheck{CheckOperationApproved, CheckRemovalApproved}},
		{Order: 2, Action: "removal.verify-drained", RequiredEvidence: drainChecks()},
		{Order: 3, Action: "lifecycle.enter-removing", RequiredEvidence: append(checks(CheckOperationApproved, CheckRemovalApproved), drainChecks()...)},
		{Order: 4, Action: "removal.verify-complete", RequiredEvidence: []EvidenceCheck{CheckRemovalVerified}},
		{Order: 5, Action: "lifecycle.retire-removed-node", RequiredEvidence: []EvidenceCheck{CheckRemovalVerified, CheckRetirementApproved}},
	}

	fingerprintInput := struct {
		NodeID          string `json:"node_id"`
		ScopeID         string `json:"scope_id"`
		OwnerScope      string `json:"owner_scope"`
		Generation      uint64 `json:"generation"`
		ResourceVersion string `json:"resource_version"`
	}{
		NodeID:          current.ObjectID,
		ScopeID:         current.ScopeID,
		OwnerScope:      current.OwnerScope,
		Generation:      current.Generation,
		ResourceVersion: current.ResourceVersion,
	}
	encoded, err := json.Marshal(fingerprintInput)
	if err != nil {
		return DecommissionPlan{}, invalidOperation("fingerprint: %v", err)
	}
	digest := sha256.Sum256(encoded)

	return DecommissionPlan{
		ContractVersion:        DecommissionPlanContractV1,
		PlanID:                 "ndp-" + hex.EncodeToString(digest[:])[:24],
		NodeID:                 current.ObjectID,
		ScopeID:                current.ScopeID,
		OwnerScope:             current.OwnerScope,
		BasedOnGeneration:      current.Generation,
		BasedOnResourceVersion: current.ResourceVersion,
		Steps:                  steps,
		RequiresApprovedChange: true,
		RequiresDurableJob:     true,
		RequiresAudit:          true,
		PlanOnly:               true,
	}, nil
}
