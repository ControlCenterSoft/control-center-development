package clusterha

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"control-center/internal/corecontracts"
	"control-center/internal/nodelifecycle"
)

var (
	ErrInvalidLifecyclePreflight = errors.New("invalid cluster lifecycle preflight")
	ErrUnsafeLifecyclePreflight  = errors.New("unsafe cluster lifecycle transition")
)

type LifecycleCheck string

const (
	CheckLifecyclePlanBound      LifecycleCheck = "lifecycle-plan-bound"
	CheckControllerRoleBound     LifecycleCheck = "controller-role-bound"
	CheckQuorumAfterDrain        LifecycleCheck = "quorum-after-drain"
	CheckStandaloneDowntime      LifecycleCheck = "standalone-downtime-acknowledged"
	CheckLifecycleLeaderTransfer LifecycleCheck = "lifecycle-leader-transfer"
	CheckLifecycleFencing        LifecycleCheck = "lifecycle-fencing"
	CheckMembershipChangeBound   LifecycleCheck = "membership-change-bound"
)

// LifecyclePreflightRequest binds one already-valid node lifecycle plan to an
// exact controller-cluster snapshot. It is intentionally side-effect free: the
// returned plan is safety evidence for a later audited executor, never an
// execution authorization.
type LifecyclePreflightRequest struct {
	LifecyclePlan                 nodelifecycle.TransitionPlan `json:"lifecycle_plan"`
	RoleAssignments               []corecontracts.RoleAssignment `json:"role_assignments"`
	ClusterScopeID                string `json:"cluster_scope_id"`
	ClusterServiceIdentityID      string `json:"cluster_service_identity_id"`
	Membership                    Snapshot `json:"membership"`
	StandaloneDowntimeAcknowledged bool `json:"standalone_downtime_acknowledged"`
	LeaderTransferConfirmed       bool `json:"leader_transfer_confirmed"`
	NextLeaderID                  string `json:"next_leader_id,omitempty"`
	FencingConfirmed              bool `json:"fencing_confirmed"`
	MembershipChange              *ChangeRequest `json:"membership_change,omitempty"`
}

type LifecyclePreflightPlan struct {
	PlanID                    string `json:"plan_id"`
	Accepted                  bool `json:"accepted"`
	PlanOnly                  bool `json:"plan_only"`
	HostMutation              bool `json:"host_mutation"`
	StateMutation             bool `json:"state_mutation"`
	NodeID                    string `json:"node_id"`
	LifecyclePlanID           string `json:"lifecycle_plan_id"`
	LifecycleTarget           nodelifecycle.State `json:"lifecycle_target"`
	ControllerBound           bool `json:"controller_bound"`
	ClusterID                 string `json:"cluster_id"`
	ClusterGeneration         uint64 `json:"cluster_generation"`
	ClusterProfile            Profile `json:"cluster_profile"`
	CurrentQuorum             int `json:"current_quorum"`
	CurrentHealthyVotes       int `json:"current_healthy_votes"`
	HealthyVotesAfterDrain    int `json:"healthy_votes_after_drain"`
	RequiresStandaloneDowntime bool `json:"requires_standalone_downtime"`
	RequiresLeaderTransfer    bool `json:"requires_leader_transfer"`
	RequiresFencing           bool `json:"requires_fencing"`
	MembershipPlanID          string `json:"membership_plan_id,omitempty"`
	RequiredChecks            []LifecycleCheck `json:"required_checks"`
}

func BuildLifecyclePreflight(request LifecyclePreflightRequest) (LifecyclePreflightPlan, error) {
	lifecycle := request.LifecyclePlan
	if !lifecycle.Accepted || !lifecycle.PlanOnly || lifecycle.HostMutation || lifecycle.StateMutation {
		return LifecyclePreflightPlan{}, fmt.Errorf("%w: lifecycle plan must be accepted, plan-only and mutation-free", ErrInvalidLifecyclePreflight)
	}
	if lifecycle.PlanID == "" || lifecycle.NodeID == "" || !lifecycle.To.Valid() {
		return LifecyclePreflightPlan{}, fmt.Errorf("%w: lifecycle plan identity and target are required", ErrInvalidLifecyclePreflight)
	}
	if !disruptiveLifecycleTarget(lifecycle.To) {
		return LifecyclePreflightPlan{}, fmt.Errorf("%w: lifecycle target %q does not require HA preflight", ErrInvalidLifecyclePreflight, lifecycle.To)
	}

	profile, members, err := validateSnapshot(request.Membership)
	if err != nil {
		return LifecyclePreflightPlan{}, err
	}
	currentHealthy := healthyVotes(request.Membership.Members)
	currentQuorum := quorum(len(request.Membership.Members))
	if currentHealthy < currentQuorum {
		return LifecyclePreflightPlan{}, fmt.Errorf("%w: current quorum is lost: healthy=%d quorum=%d", ErrUnsafeLifecyclePreflight, currentHealthy, currentQuorum)
	}

	controllerBound, err := validateControllerBinding(request, lifecycle.NodeID)
	if err != nil {
		return LifecyclePreflightPlan{}, err
	}
	checks := []LifecycleCheck{CheckLifecyclePlanBound}
	if !controllerBound {
		return buildLifecyclePreflightResult(request, profile, currentQuorum, currentHealthy, currentHealthy, false, false, false, "", checks)
	}
	checks = append(checks, CheckControllerRoleBound)

	member, exists := members[lifecycle.NodeID]
	if !exists || member.Kind != MemberController {
		return LifecyclePreflightPlan{}, fmt.Errorf("%w: controller-bound node %q is not a controller member of cluster %q", ErrUnsafeLifecyclePreflight, lifecycle.NodeID, request.Membership.ClusterID)
	}

	healthyAfterDrain := currentHealthy
	if member.Healthy {
		healthyAfterDrain--
	}
	requiresStandalone := profile == ProfileStandalone
	if requiresStandalone {
		if !request.StandaloneDowntimeAcknowledged {
			return LifecyclePreflightPlan{}, fmt.Errorf("%w: standalone controller maintenance requires explicit downtime acknowledgement", ErrUnsafeLifecyclePreflight)
		}
		checks = append(checks, CheckStandaloneDowntime)
	} else {
		if healthyAfterDrain < currentQuorum {
			return LifecyclePreflightPlan{}, fmt.Errorf("%w: draining controller %q would lose quorum: remaining=%d quorum=%d", ErrUnsafeLifecyclePreflight, lifecycle.NodeID, healthyAfterDrain, currentQuorum)
		}
		checks = append(checks, CheckQuorumAfterDrain)
	}

	requiresLeaderTransfer := request.Membership.LeaderID == lifecycle.NodeID
	if requiresLeaderTransfer {
		if !request.LeaderTransferConfirmed {
			return LifecyclePreflightPlan{}, fmt.Errorf("%w: draining current leader requires confirmed leader transfer", ErrUnsafeLifecyclePreflight)
		}
		next, exists := members[request.NextLeaderID]
		if request.NextLeaderID == "" || request.NextLeaderID == lifecycle.NodeID || !exists || next.Kind != MemberController || !memberReady(next) {
			return LifecyclePreflightPlan{}, fmt.Errorf("%w: next leader %q is not a different ready controller", ErrUnsafeLifecyclePreflight, request.NextLeaderID)
		}
		checks = append(checks, CheckLifecycleLeaderTransfer)
	} else if request.LeaderTransferConfirmed || request.NextLeaderID != "" {
		return LifecyclePreflightPlan{}, fmt.Errorf("%w: leader transfer metadata is only valid for the current leader", ErrInvalidLifecyclePreflight)
	}

	requiresFencing := !member.Healthy
	if requiresFencing {
		if !request.FencingConfirmed {
			return LifecyclePreflightPlan{}, fmt.Errorf("%w: unavailable controller requires fencing before lifecycle mutation", ErrUnsafeLifecyclePreflight)
		}
		checks = append(checks, CheckLifecycleFencing)
	} else if request.FencingConfirmed {
		return LifecyclePreflightPlan{}, fmt.Errorf("%w: fencing confirmation has no unavailable controller", ErrInvalidLifecyclePreflight)
	}

	membershipPlanID := ""
	if lifecycle.To == nodelifecycle.StateReplacing || lifecycle.To == nodelifecycle.StateRemoving || lifecycle.To == nodelifecycle.StateRetired {
		if request.MembershipChange == nil {
			return LifecyclePreflightPlan{}, fmt.Errorf("%w: lifecycle target %q requires a bound membership change", ErrUnsafeLifecyclePreflight, lifecycle.To)
		}
		if !sameSnapshot(request.Membership, request.MembershipChange.Current) {
			return LifecyclePreflightPlan{}, fmt.Errorf("%w: membership change is not based on the exact lifecycle snapshot", ErrInvalidLifecyclePreflight)
		}
		membershipPlan, planErr := BuildMembershipPlan(*request.MembershipChange)
		if planErr != nil {
			return LifecyclePreflightPlan{}, fmt.Errorf("%w: membership change: %v", ErrUnsafeLifecyclePreflight, planErr)
		}
		if !containsString(membershipPlan.RemoveMembers, lifecycle.NodeID) {
			return LifecyclePreflightPlan{}, fmt.Errorf("%w: membership change does not remove lifecycle target %q", ErrUnsafeLifecyclePreflight, lifecycle.NodeID)
		}
		membershipPlanID = membershipPlan.PlanID
		checks = append(checks, CheckMembershipChangeBound)
	}

	return buildLifecyclePreflightResult(request, profile, currentQuorum, currentHealthy, healthyAfterDrain, requiresStandalone, requiresLeaderTransfer, requiresFencing, membershipPlanID, checks)
}

func validateControllerBinding(request LifecyclePreflightRequest, nodeID string) (bool, error) {
	controllerBound := false
	for _, assignment := range request.RoleAssignments {
		if assignment.TargetNodeID != nodeID || !controllerRole(assignment.Role) {
			continue
		}
		controllerBound = true
		if request.ClusterScopeID == "" || request.ClusterServiceIdentityID == "" {
			return false, fmt.Errorf("%w: cluster scope and service identity are required for controller-bound nodes", ErrInvalidLifecyclePreflight)
		}
		if assignment.ScopeID != request.ClusterScopeID || assignment.ServiceIdentityID != request.ClusterServiceIdentityID {
			return false, fmt.Errorf("%w: controller assignment %q is bound to a different control plane", ErrUnsafeLifecyclePreflight, assignment.ObjectID)
		}
	}
	return controllerBound, nil
}

func controllerRole(role corecontracts.NodeRole) bool {
	switch role {
	case corecontracts.RoleGlobalController, corecontracts.RoleSiteController, corecontracts.RoleControllerClusterMember:
		return true
	default:
		return false
	}
}

func disruptiveLifecycleTarget(state nodelifecycle.State) bool {
	switch state {
	case nodelifecycle.StateDraining,
		nodelifecycle.StateMaintenance,
		nodelifecycle.StateUpdating,
		nodelifecycle.StateReplacing,
		nodelifecycle.StateRemoving,
		nodelifecycle.StateRetired:
		return true
	default:
		return false
	}
}

func sameSnapshot(left, right Snapshot) bool {
	if left.ClusterID != right.ClusterID || left.Generation != right.Generation || left.LeaderID != right.LeaderID || len(left.Members) != len(right.Members) {
		return false
	}
	leftMembers := append([]Member(nil), left.Members...)
	rightMembers := append([]Member(nil), right.Members...)
	sort.Slice(leftMembers, func(i, j int) bool { return leftMembers[i].ID < leftMembers[j].ID })
	sort.Slice(rightMembers, func(i, j int) bool { return rightMembers[i].ID < rightMembers[j].ID })
	for i := range leftMembers {
		if leftMembers[i] != rightMembers[i] {
			return false
		}
	}
	return true
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func buildLifecyclePreflightResult(request LifecyclePreflightRequest, profile Profile, currentQuorum, currentHealthy, healthyAfterDrain int, requiresStandalone, requiresLeaderTransfer, requiresFencing bool, membershipPlanID string, checks []LifecycleCheck) (LifecyclePreflightPlan, error) {
	checks = append([]LifecycleCheck(nil), checks...)
	fingerprintInput := struct {
		LifecyclePlanID             string `json:"lifecycle_plan_id"`
		NodeID                      string `json:"node_id"`
		LifecycleTarget             nodelifecycle.State `json:"lifecycle_target"`
		ClusterID                   string `json:"cluster_id"`
		ClusterGeneration           uint64 `json:"cluster_generation"`
		ClusterScopeID              string `json:"cluster_scope_id"`
		ClusterServiceIdentityID    string `json:"cluster_service_identity_id"`
		StandaloneDowntimeAcknowledged bool `json:"standalone_downtime_acknowledged"`
		LeaderTransferConfirmed     bool `json:"leader_transfer_confirmed"`
		NextLeaderID                string `json:"next_leader_id"`
		FencingConfirmed            bool `json:"fencing_confirmed"`
		MembershipPlanID            string `json:"membership_plan_id"`
		RequiredChecks              []LifecycleCheck `json:"required_checks"`
	}{
		LifecyclePlanID: lifecyclePlanID(request),
		NodeID: request.LifecyclePlan.NodeID,
		LifecycleTarget: request.LifecyclePlan.To,
		ClusterID: request.Membership.ClusterID,
		ClusterGeneration: request.Membership.Generation,
		ClusterScopeID: request.ClusterScopeID,
		ClusterServiceIdentityID: request.ClusterServiceIdentityID,
		StandaloneDowntimeAcknowledged: request.StandaloneDowntimeAcknowledged,
		LeaderTransferConfirmed: request.LeaderTransferConfirmed,
		NextLeaderID: request.NextLeaderID,
		FencingConfirmed: request.FencingConfirmed,
		MembershipPlanID: membershipPlanID,
		RequiredChecks: checks,
	}
	encoded, err := json.Marshal(fingerprintInput)
	if err != nil {
		return LifecyclePreflightPlan{}, fmt.Errorf("%w: fingerprint: %v", ErrInvalidLifecyclePreflight, err)
	}
	digest := sha256.Sum256(encoded)
	return LifecyclePreflightPlan{
		PlanID: "chlp-" + hex.EncodeToString(digest[:])[:24],
		Accepted: true,
		PlanOnly: true,
		HostMutation: false,
		StateMutation: false,
		NodeID: request.LifecyclePlan.NodeID,
		LifecyclePlanID: request.LifecyclePlan.PlanID,
		LifecycleTarget: request.LifecyclePlan.To,
		ControllerBound: len(checks) > 1,
		ClusterID: request.Membership.ClusterID,
		ClusterGeneration: request.Membership.Generation,
		ClusterProfile: profile,
		CurrentQuorum: currentQuorum,
		CurrentHealthyVotes: currentHealthy,
		HealthyVotesAfterDrain: healthyAfterDrain,
		RequiresStandaloneDowntime: requiresStandalone,
		RequiresLeaderTransfer: requiresLeaderTransfer,
		RequiresFencing: requiresFencing,
		MembershipPlanID: membershipPlanID,
		RequiredChecks: checks,
	}, nil
}

func lifecyclePlanID(request LifecyclePreflightRequest) string { return request.LifecyclePlan.PlanID }
