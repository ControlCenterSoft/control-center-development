package clusterha

import (
	"errors"
	"reflect"
	"testing"

	"control-center/internal/corecontracts"
	"control-center/internal/nodelifecycle"
)

func lifecyclePlan(nodeID string, target nodelifecycle.State) nodelifecycle.TransitionPlan {
	return nodelifecycle.TransitionPlan{
		PlanID:         "nlp-test-plan",
		Accepted:       true,
		PlanOnly:       true,
		HostMutation:   false,
		StateMutation:  false,
		NodeID:         nodeID,
		To:             target,
		LifecycleTarget: "",
	}
}

func controllerAssignment(nodeID string) corecontracts.RoleAssignment {
	return corecontracts.RoleAssignment{
		ObjectMetadata: corecontracts.ObjectMetadata{
			ObjectID: "role-controller-" + nodeID,
			ScopeID:  "scope-a",
		},
		TargetNodeID:      nodeID,
		ServiceIdentityID: "control-plane-a",
		Role:              corecontracts.RoleControllerClusterMember,
	}
}

func threeControllerSnapshot() Snapshot {
	return Snapshot{
		ClusterID:  "cluster-a",
		Generation: 20,
		LeaderID:   "controller-a",
		Members: []Member{
			controller("controller-a", true, true),
			controller("controller-b", true, true),
			controller("controller-c", true, true),
		},
	}
}

func preflightRequest(nodeID string, target nodelifecycle.State) LifecyclePreflightRequest {
	return LifecyclePreflightRequest{
		LifecyclePlan:            lifecyclePlan(nodeID, target),
		RoleAssignments:          []corecontracts.RoleAssignment{controllerAssignment(nodeID)},
		ClusterScopeID:           "scope-a",
		ClusterServiceIdentityID: "control-plane-a",
		Membership:               threeControllerSnapshot(),
	}
}

func TestBuildLifecyclePreflightAllowsStandbyDrainWithQuorum(t *testing.T) {
	request := preflightRequest("controller-b", nodelifecycle.StateDraining)
	plan, err := BuildLifecyclePreflight(request)
	if err != nil {
		t.Fatalf("BuildLifecyclePreflight() error = %v", err)
	}
	if !plan.Accepted || !plan.PlanOnly || plan.HostMutation || plan.StateMutation {
		t.Fatalf("unexpected plan safety flags: %+v", plan)
	}
	if !plan.ControllerBound || plan.CurrentQuorum != 2 || plan.CurrentHealthyVotes != 3 || plan.HealthyVotesAfterDrain != 2 {
		t.Fatalf("unexpected quorum accounting: %+v", plan)
	}
	wantChecks := []LifecycleCheck{CheckLifecyclePlanBound, CheckControllerRoleBound, CheckQuorumAfterDrain}
	if !reflect.DeepEqual(plan.RequiredChecks, wantChecks) {
		t.Fatalf("checks = %v, want %v", plan.RequiredChecks, wantChecks)
	}
}

func TestBuildLifecyclePreflightRequiresLeaderTransfer(t *testing.T) {
	request := preflightRequest("controller-a", nodelifecycle.StateDraining)
	_, err := BuildLifecyclePreflight(request)
	if !errors.Is(err, ErrUnsafeLifecyclePreflight) {
		t.Fatalf("error = %v, want ErrUnsafeLifecyclePreflight", err)
	}
	request.LeaderTransferConfirmed = true
	request.NextLeaderID = "controller-b"
	plan, err := BuildLifecyclePreflight(request)
	if err != nil {
		t.Fatalf("BuildLifecyclePreflight() with transfer error = %v", err)
	}
	if !plan.RequiresLeaderTransfer || !containsLifecycleCheck(plan.RequiredChecks, CheckLifecycleLeaderTransfer) {
		t.Fatalf("leader transfer requirement missing: %+v", plan)
	}
}

func TestBuildLifecyclePreflightRejectsUnreadyNextLeader(t *testing.T) {
	request := preflightRequest("controller-a", nodelifecycle.StateDraining)
	request.Membership.Members[1].CaughtUp = false
	request.LeaderTransferConfirmed = true
	request.NextLeaderID = "controller-b"
	_, err := BuildLifecyclePreflight(request)
	if !errors.Is(err, ErrUnsafeLifecyclePreflight) {
		t.Fatalf("error = %v, want ErrUnsafeLifecyclePreflight", err)
	}
}

func TestBuildLifecyclePreflightRequiresFencingForUnavailableController(t *testing.T) {
	request := preflightRequest("controller-c", nodelifecycle.StateDraining)
	request.Membership.Members[2].Healthy = false
	_, err := BuildLifecyclePreflight(request)
	if !errors.Is(err, ErrUnsafeLifecyclePreflight) {
		t.Fatalf("error = %v, want ErrUnsafeLifecyclePreflight", err)
	}
	request.FencingConfirmed = true
	plan, err := BuildLifecyclePreflight(request)
	if err != nil {
		t.Fatalf("BuildLifecyclePreflight() with fencing error = %v", err)
	}
	if !plan.RequiresFencing || !containsLifecycleCheck(plan.RequiredChecks, CheckLifecycleFencing) {
		t.Fatalf("fencing requirement missing: %+v", plan)
	}
}

func TestBuildLifecyclePreflightRequiresStandaloneDowntimeAcknowledgement(t *testing.T) {
	request := preflightRequest("controller-a", nodelifecycle.StateDraining)
	request.Membership = standalone()
	_, err := BuildLifecyclePreflight(request)
	if !errors.Is(err, ErrUnsafeLifecyclePreflight) {
		t.Fatalf("error = %v, want ErrUnsafeLifecyclePreflight", err)
	}
	request.StandaloneDowntimeAcknowledged = true
	request.LeaderTransferConfirmed = true
	request.NextLeaderID = "controller-b"
	_, err = BuildLifecyclePreflight(request)
	if !errors.Is(err, ErrUnsafeLifecyclePreflight) {
		t.Fatalf("standalone leader must not accept an unavailable next leader: %v", err)
	}
}

func TestBuildLifecyclePreflightAllowsStandaloneDrainWithOnlyDowntimeAck(t *testing.T) {
	request := preflightRequest("controller-a", nodelifecycle.StateDraining)
	request.Membership = standalone()
	request.StandaloneDowntimeAcknowledged = true
	plan, err := BuildLifecyclePreflight(request)
	if err != nil {
		t.Fatalf("BuildLifecyclePreflight() error = %v", err)
	}
	if !plan.RequiresStandaloneDowntime || plan.RequiresLeaderTransfer {
		t.Fatalf("unexpected standalone plan: %+v", plan)
	}
}

func TestBuildLifecyclePreflightRequiresBoundMembershipChangeForReplacement(t *testing.T) {
	request := preflightRequest("controller-b", nodelifecycle.StateReplacing)
	_, err := BuildLifecyclePreflight(request)
	if !errors.Is(err, ErrUnsafeLifecyclePreflight) {
		t.Fatalf("error = %v, want ErrUnsafeLifecyclePreflight", err)
	}
	change := ChangeRequest{
		Current:           request.Membership,
		DesiredGeneration: request.Membership.Generation + 1,
		DesiredMembers: []Member{
			controller("controller-a", true, true),
			controller("controller-c", true, true),
			controller("controller-d", true, true),
		},
	}
	request.MembershipChange = &change
	plan, err := BuildLifecyclePreflight(request)
	if err != nil {
		t.Fatalf("BuildLifecyclePreflight() with membership change error = %v", err)
	}
	if plan.MembershipPlanID == "" || !containsLifecycleCheck(plan.RequiredChecks, CheckMembershipChangeBound) {
		t.Fatalf("membership binding missing: %+v", plan)
	}
}

func TestBuildLifecyclePreflightRejectsMembershipChangeBasedOnDifferentSnapshot(t *testing.T) {
	request := preflightRequest("controller-b", nodelifecycle.StateReplacing)
	change := ChangeRequest{
		Current:           request.Membership,
		DesiredGeneration: request.Membership.Generation + 1,
		DesiredMembers: []Member{
			controller("controller-a", true, true),
			controller("controller-c", true, true),
			controller("controller-d", true, true),
		},
	}
	change.Current.Generation--
	request.MembershipChange = &change
	_, err := BuildLifecyclePreflight(request)
	if !errors.Is(err, ErrInvalidLifecyclePreflight) {
		t.Fatalf("error = %v, want ErrInvalidLifecyclePreflight", err)
	}
}

func TestBuildLifecyclePreflightRejectsControllerBindingMismatch(t *testing.T) {
	request := preflightRequest("controller-b", nodelifecycle.StateDraining)
	request.ClusterServiceIdentityID = "other-control-plane"
	_, err := BuildLifecyclePreflight(request)
	if !errors.Is(err, ErrUnsafeLifecyclePreflight) {
		t.Fatalf("error = %v, want ErrUnsafeLifecyclePreflight", err)
	}
}

func TestBuildLifecyclePreflightDoesNotGateNonControllerNode(t *testing.T) {
	request := preflightRequest("worker-a", nodelifecycle.StateDraining)
	request.RoleAssignments = []corecontracts.RoleAssignment{{
		ObjectMetadata: corecontracts.ObjectMetadata{ObjectID: "role-worker-a", ScopeID: "scope-a"},
		TargetNodeID: "worker-a",
		Role:         corecontracts.RoleWorkerNode,
	}}
	plan, err := BuildLifecyclePreflight(request)
	if err != nil {
		t.Fatalf("BuildLifecyclePreflight() error = %v", err)
	}
	if plan.ControllerBound || !reflect.DeepEqual(plan.RequiredChecks, []LifecycleCheck{CheckLifecyclePlanBound}) {
		t.Fatalf("unexpected non-controller plan: %+v", plan)
	}
}

func TestBuildLifecyclePreflightIDChangesWhenObservedMembershipChanges(t *testing.T) {
	request := preflightRequest("controller-b", nodelifecycle.StateDraining)
	first, err := BuildLifecyclePreflight(request)
	if err != nil {
		t.Fatalf("BuildLifecyclePreflight(first) error = %v", err)
	}
	request.Membership.Members[2].Healthy = false
	request.FencingConfirmed = false
	second, err := BuildLifecyclePreflight(request)
	if err != nil {
		t.Fatalf("BuildLifecyclePreflight(second) error = %v", err)
	}
	if first.PlanID == second.PlanID {
		t.Fatalf("plan ID did not bind observed membership change: %s", first.PlanID)
	}
}

func containsLifecycleCheck(values []LifecycleCheck, target LifecycleCheck) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
