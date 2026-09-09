package clusterha

import (
	"errors"
	"reflect"
	"testing"
)

func controller(id string, healthy, caughtUp bool) Member {
	return Member{ID: id, Kind: MemberController, Healthy: healthy, CaughtUp: caughtUp}
}

func witness(id string, healthy bool) Member {
	return Member{ID: id, Kind: MemberWitness, Healthy: healthy}
}

func standalone() Snapshot {
	return Snapshot{
		ClusterID:  "cluster-a",
		Generation: 7,
		LeaderID:   "controller-a",
		Members:    []Member{controller("controller-a", true, true)},
	}
}

func TestBuildMembershipPlanExpandsStandaloneToThreeControllers(t *testing.T) {
	request := ChangeRequest{
		Current:           standalone(),
		DesiredGeneration: 8,
		DesiredMembers: []Member{
			controller("controller-c", true, true),
			controller("controller-a", true, true),
			controller("controller-b", true, true),
		},
	}
	plan, err := BuildMembershipPlan(request)
	if err != nil {
		t.Fatalf("BuildMembershipPlan() error = %v", err)
	}
	if !plan.Accepted || !plan.PlanOnly || plan.HostMutation || plan.StateMutation {
		t.Fatalf("unexpected plan safety flags: %+v", plan)
	}
	if plan.CurrentProfile != ProfileStandalone || plan.DesiredProfile != ProfileThreeControllers {
		t.Fatalf("unexpected profiles: %s -> %s", plan.CurrentProfile, plan.DesiredProfile)
	}
	if plan.CurrentQuorum != 1 || plan.DesiredQuorum != 2 || plan.CurrentHealthyVotes != 1 || plan.DesiredReadyVotes != 3 {
		t.Fatalf("unexpected quorum accounting: %+v", plan)
	}
	if !reflect.DeepEqual(plan.AddMembers, []string{"controller-b", "controller-c"}) || len(plan.RemoveMembers) != 0 {
		t.Fatalf("unexpected membership diff: add=%v remove=%v", plan.AddMembers, plan.RemoveMembers)
	}
	wantChecks := []Check{CheckCurrentQuorum, CheckDesiredQuorum, CheckNewMembersReady}
	if !reflect.DeepEqual(plan.RequiredChecks, wantChecks) {
		t.Fatalf("checks = %v, want %v", plan.RequiredChecks, wantChecks)
	}
}

func TestBuildMembershipPlanSupportsTwoControllersPlusWitness(t *testing.T) {
	request := ChangeRequest{
		Current:           standalone(),
		DesiredGeneration: 8,
		DesiredMembers: []Member{
			controller("controller-a", true, true),
			controller("controller-b", true, true),
			witness("witness-a", true),
		},
	}
	plan, err := BuildMembershipPlan(request)
	if err != nil {
		t.Fatalf("BuildMembershipPlan() error = %v", err)
	}
	if plan.DesiredProfile != ProfileTwoPlusWitness || plan.DesiredQuorum != 2 {
		t.Fatalf("unexpected witness plan: %+v", plan)
	}
}

func TestBuildMembershipPlanRejectsUnsupportedEvenProfile(t *testing.T) {
	request := ChangeRequest{
		Current:           standalone(),
		DesiredGeneration: 8,
		DesiredMembers: []Member{
			controller("controller-a", true, true),
			controller("controller-b", true, true),
		},
	}
	_, err := BuildMembershipPlan(request)
	if !errors.Is(err, ErrInvalidMembership) {
		t.Fatalf("error = %v, want ErrInvalidMembership", err)
	}
}

func TestBuildMembershipPlanFailsClosedWhenCurrentQuorumIsLost(t *testing.T) {
	current := Snapshot{
		ClusterID:  "cluster-a",
		Generation: 10,
		LeaderID:   "controller-a",
		Members: []Member{
			controller("controller-a", true, true),
			controller("controller-b", false, true),
			controller("controller-c", false, true),
		},
	}
	request := ChangeRequest{
		Current:           current,
		DesiredGeneration: 11,
		DesiredMembers: []Member{
			controller("controller-a", true, true),
			controller("controller-b", true, true),
			controller("controller-d", true, true),
		},
	}
	_, err := BuildMembershipPlan(request)
	if !errors.Is(err, ErrUnsafeMembership) {
		t.Fatalf("error = %v, want ErrUnsafeMembership", err)
	}
}

func TestBuildMembershipPlanRejectsUnreadyNewController(t *testing.T) {
	request := ChangeRequest{
		Current:           standalone(),
		DesiredGeneration: 8,
		DesiredMembers: []Member{
			controller("controller-a", true, true),
			controller("controller-b", true, false),
			controller("controller-c", true, true),
		},
	}
	_, err := BuildMembershipPlan(request)
	if !errors.Is(err, ErrUnsafeMembership) {
		t.Fatalf("error = %v, want ErrUnsafeMembership", err)
	}
}

func TestBuildMembershipPlanRequiresFencingForUnhealthyControllerRemoval(t *testing.T) {
	current := Snapshot{
		ClusterID:  "cluster-a",
		Generation: 10,
		LeaderID:   "controller-a",
		Members: []Member{
			controller("controller-a", true, true),
			controller("controller-b", true, true),
			controller("controller-c", false, true),
		},
	}
	request := ChangeRequest{
		Current:           current,
		DesiredGeneration: 11,
		DesiredMembers:    []Member{controller("controller-a", true, true)},
	}
	_, err := BuildMembershipPlan(request)
	if !errors.Is(err, ErrUnsafeMembership) {
		t.Fatalf("error = %v, want ErrUnsafeMembership", err)
	}
	request.FencingConfirmed = true
	plan, err := BuildMembershipPlan(request)
	if err != nil {
		t.Fatalf("BuildMembershipPlan() with fencing error = %v", err)
	}
	if !plan.RequiresFencing || !reflect.DeepEqual(plan.RemoveMembers, []string{"controller-b", "controller-c"}) {
		t.Fatalf("unexpected fenced plan: %+v", plan)
	}
}

func TestBuildMembershipPlanRequiresConfirmedLeaderTransfer(t *testing.T) {
	current := Snapshot{
		ClusterID:  "cluster-a",
		Generation: 10,
		LeaderID:   "controller-a",
		Members: []Member{
			controller("controller-a", true, true),
			controller("controller-b", true, true),
			controller("controller-c", true, true),
		},
	}
	request := ChangeRequest{
		Current:           current,
		DesiredGeneration: 11,
		DesiredMembers:    []Member{controller("controller-b", true, true)},
	}
	_, err := BuildMembershipPlan(request)
	if !errors.Is(err, ErrUnsafeMembership) {
		t.Fatalf("error = %v, want ErrUnsafeMembership", err)
	}
	request.LeaderTransferConfirmed = true
	request.NextLeaderID = "controller-b"
	plan, err := BuildMembershipPlan(request)
	if err != nil {
		t.Fatalf("BuildMembershipPlan() with leader transfer error = %v", err)
	}
	if !plan.RequiresLeaderTransfer {
		t.Fatalf("expected leader transfer requirement: %+v", plan)
	}
}

func TestBuildMembershipPlanRejectsDuplicateMember(t *testing.T) {
	request := ChangeRequest{
		Current:           standalone(),
		DesiredGeneration: 8,
		DesiredMembers: []Member{
			controller("controller-a", true, true),
			controller("controller-b", true, true),
			controller("controller-b", true, true),
		},
	}
	_, err := BuildMembershipPlan(request)
	if !errors.Is(err, ErrInvalidMembership) {
		t.Fatalf("error = %v, want ErrInvalidMembership", err)
	}
}

func TestBuildMembershipPlanIDIsDeterministicAcrossMemberOrder(t *testing.T) {
	first := ChangeRequest{
		Current:           standalone(),
		DesiredGeneration: 8,
		DesiredMembers: []Member{
			controller("controller-a", true, true),
			controller("controller-b", true, true),
			controller("controller-c", true, true),
		},
	}
	second := first
	second.DesiredMembers = []Member{
		controller("controller-c", true, true),
		controller("controller-a", true, true),
		controller("controller-b", true, true),
	}
	planA, err := BuildMembershipPlan(first)
	if err != nil {
		t.Fatalf("BuildMembershipPlan(first) error = %v", err)
	}
	planB, err := BuildMembershipPlan(second)
	if err != nil {
		t.Fatalf("BuildMembershipPlan(second) error = %v", err)
	}
	if planA.PlanID != planB.PlanID {
		t.Fatalf("plan IDs differ: %s != %s", planA.PlanID, planB.PlanID)
	}
}
