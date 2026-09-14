package nodelifecycle

import (
	"errors"
	"reflect"
	"testing"
)

func TestBuildDecommissionPlanIsSafeAndDeterministic(t *testing.T) {
	current := lifecycleInState(StateMaintenance)
	plan, err := BuildDecommissionPlan(current)
	if err != nil {
		t.Fatalf("BuildDecommissionPlan() error = %v", err)
	}
	if plan.ContractVersion != DecommissionPlanContractV1 || plan.PlanID == "" || len(plan.Steps) != 5 {
		t.Fatalf("unexpected decommission plan: %#v", plan)
	}
	if plan.NodeID != current.ObjectID || plan.ScopeID != current.ScopeID || plan.OwnerScope != current.OwnerScope {
		t.Fatalf("plan identity does not match lifecycle: %#v", plan)
	}
	if plan.BasedOnGeneration != current.Generation || plan.BasedOnResourceVersion != current.ResourceVersion {
		t.Fatalf("plan precondition evidence does not match lifecycle: %#v", plan)
	}
	if !plan.PlanOnly || !plan.RequiresApprovedChange || !plan.RequiresDurableJob || !plan.RequiresAudit {
		t.Fatalf("missing safety gates: %#v", plan)
	}
	if plan.LifecycleMutation || plan.PlacementMutation || plan.HostMutation {
		t.Fatalf("planner has effects: %#v", plan)
	}

	if got := plan.Steps[0].RequiredEvidence; !reflect.DeepEqual(got, []EvidenceCheck{
		CheckOperationApproved, CheckRemovalApproved,
	}) {
		t.Fatalf("approval evidence = %#v", got)
	}
	if got := plan.Steps[1].RequiredEvidence; !reflect.DeepEqual(got, drainChecks()) {
		t.Fatalf("drain evidence = %#v", got)
	}
	if got := plan.Steps[2].RequiredEvidence; !reflect.DeepEqual(got, append(checks(CheckOperationApproved, CheckRemovalApproved), drainChecks()...)) {
		t.Fatalf("enter-removing evidence = %#v", got)
	}
	if got := plan.Steps[4].RequiredEvidence; !reflect.DeepEqual(got, []EvidenceCheck{
		CheckRemovalVerified, CheckRetirementApproved,
	}) {
		t.Fatalf("retirement evidence = %#v", got)
	}

	again, err := BuildDecommissionPlan(current)
	if err != nil {
		t.Fatal(err)
	}
	if again.PlanID != plan.PlanID {
		t.Fatalf("plan id is not deterministic: %q != %q", again.PlanID, plan.PlanID)
	}
}

func TestBuildDecommissionPlanIdentityBindsLifecycleOwnershipAndVersion(t *testing.T) {
	current := lifecycleInState(StateMaintenance)
	base, err := BuildDecommissionPlan(current)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name   string
		mutate func(*NodeLifecycle)
	}{
		{name: "scope", mutate: func(n *NodeLifecycle) { n.ScopeID = "site-b" }},
		{name: "owner scope", mutate: func(n *NodeLifecycle) { n.OwnerScope = "team-b" }},
		{name: "generation", mutate: func(n *NodeLifecycle) { n.Generation++ }},
		{name: "resource version", mutate: func(n *NodeLifecycle) { n.ResourceVersion = "rv:node-1:next" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			changed := current
			test.mutate(&changed)
			plan, err := BuildDecommissionPlan(changed)
			if err != nil {
				t.Fatalf("BuildDecommissionPlan() error = %v", err)
			}
			if plan.PlanID == base.PlanID {
				t.Fatalf("plan id did not change for %s", test.name)
			}
		})
	}
}

func TestBuildDecommissionPlanFailsClosedOutsideMaintenance(t *testing.T) {
	for _, state := range []State{StateReady, StateDegraded, StateDraining, StateRemoving, StateOffline, StateRetired} {
		t.Run(string(state), func(t *testing.T) {
			if _, err := BuildDecommissionPlan(lifecycleInState(state)); !errors.Is(err, ErrInvalidOperationPlan) {
				t.Fatalf("error = %v, want ErrInvalidOperationPlan", err)
			}
		})
	}
}
