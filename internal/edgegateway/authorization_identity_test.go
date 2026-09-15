package edgegateway

import "testing"

func TestAuthorizationPlanIDBindsAuthorizationEvidence(t *testing.T) {
	baseRequest, baseInventory := validFixture()
	basePlan, err := BuildAuthorizationPlan(baseRequest, baseInventory)
	if err != nil {
		t.Fatal(err)
	}
	if basePlan.PlanID == "" {
		t.Fatal("base plan ID is empty")
	}

	tests := []struct {
		name   string
		mutate func(*AuthorizationRequest, *Inventory)
	}{
		{
			name: "edge role resource version",
			mutate: func(_ *AuthorizationRequest, inventory *Inventory) {
				inventory.RoleAssignments[0].ResourceVersion = "rv:edge:2"
			},
		},
		{
			name: "approval identity",
			mutate: func(request *AuthorizationRequest, _ *Inventory) {
				request.Approval.ID = "approval-edge-b"
			},
		},
		{
			name: "approval policy identity",
			mutate: func(request *AuthorizationRequest, _ *Inventory) {
				request.Approval.PolicyID = "edge-policy-v2"
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request, inventory := validFixture()
			test.mutate(&request, &inventory)

			plan, err := BuildAuthorizationPlan(request, inventory)
			if err != nil {
				t.Fatal(err)
			}
			if plan.PlanID == "" {
				t.Fatal("changed plan ID is empty")
			}
			if plan.PlanID == basePlan.PlanID {
				t.Fatalf("PlanID did not change after %s changed: %q", test.name, plan.PlanID)
			}
		})
	}
}
