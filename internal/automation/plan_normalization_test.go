package automation

import "testing"

func TestBuildPlanNormalizesTargetAndTypedInputs(t *testing.T) {
	plan, err := BuildPlan(Request{
		Target:    Target{ID: "  node-windows-01  ", Platform: "  WiNdOwS  "},
		Operation: InspectService,
		Arguments: map[string]string{"name": "  ExampleService  "},
	})
	if err != nil {
		t.Fatalf("BuildPlan() error = %v", err)
	}
	if plan.TargetID != "node-windows-01" {
		t.Fatalf("TargetID = %q, want canonical trimmed target", plan.TargetID)
	}
	if plan.Adapter != "ansible.windows" {
		t.Fatalf("Adapter = %q, want ansible.windows", plan.Adapter)
	}
	if plan.Action != InspectService {
		t.Fatalf("Action = %q, want %q", plan.Action, InspectService)
	}
	if got := plan.Inputs["name"]; got != "ExampleService" {
		t.Fatalf("Inputs[name] = %q, want trimmed typed input", got)
	}
}
