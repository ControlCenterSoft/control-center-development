package automation

import "testing"

func TestBuildPlanSelectsPlatformAdapter(t *testing.T) {
	linux, err := BuildPlan(Request{
		Target:    Target{ID: "node-linux", Platform: "linux"},
		Operation: EnsurePackage,
		Arguments: map[string]string{"name": "example-package", "state": "present"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if linux.Adapter != "ansible.linux" || linux.Action != EnsurePackage {
		t.Fatalf("linux plan = %#v", linux)
	}

	windows, err := BuildPlan(Request{
		Target:    Target{ID: "node-windows", Platform: "windows"},
		Operation: EnsureService,
		Arguments: map[string]string{"name": "ExampleService", "state": "started"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if windows.Adapter != "ansible.windows" {
		t.Fatalf("windows adapter = %q", windows.Adapter)
	}
}

func TestBuildPlanRejectsArbitraryCommandShape(t *testing.T) {
	_, err := BuildPlan(Request{
		Target:    Target{ID: "node-1", Platform: "linux"},
		Operation: EnsureService,
		Arguments: map[string]string{"name": "example", "command": "arbitrary-value"},
	})
	if err == nil {
		t.Fatal("unexpected command argument accepted")
	}
}

func TestBuildPlanRequiresTypedServiceState(t *testing.T) {
	for _, arguments := range []map[string]string{
		{"name": "example"},
		{"name": "example", "state": "restarted"},
	} {
		_, err := BuildPlan(Request{
			Target:    Target{ID: "node-1", Platform: "linux"},
			Operation: EnsureService,
			Arguments: arguments,
		})
		if err == nil {
			t.Fatalf("unexpected service arguments accepted: %#v", arguments)
		}
	}
}

func TestBuildPlanNormalizesTypedState(t *testing.T) {
	plan, err := BuildPlan(Request{
		Target:    Target{ID: "node-1", Platform: "windows"},
		Operation: EnsureService,
		Arguments: map[string]string{"name": "ExampleService", "state": " STARTED "},
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Inputs["state"] != "started" {
		t.Fatalf("state = %q", plan.Inputs["state"])
	}
}

func TestBuildPlanValidatesPackageDesiredState(t *testing.T) {
	valid := []map[string]string{
		{"name": "example", "state": "present"},
		{"name": "example", "state": "present", "version": "1.2.3"},
		{"name": "example", "state": "absent"},
		{"name": "example", "state": "latest"},
	}
	for _, arguments := range valid {
		if _, err := BuildPlan(Request{
			Target:    Target{ID: "node-1", Platform: "linux"},
			Operation: EnsurePackage,
			Arguments: arguments,
		}); err != nil {
			t.Fatalf("valid package arguments %#v rejected: %v", arguments, err)
		}
	}

	invalid := []map[string]string{
		{"name": "example"},
		{"name": "example", "state": "installed"},
		{"name": "example", "state": "absent", "version": "1.2.3"},
		{"name": "example", "state": "latest", "version": "1.2.3"},
		{"name": "example", "state": "present", "version": "   "},
	}
	for _, arguments := range invalid {
		if _, err := BuildPlan(Request{
			Target:    Target{ID: "node-1", Platform: "linux"},
			Operation: EnsurePackage,
			Arguments: arguments,
		}); err == nil {
			t.Fatalf("unexpected package arguments accepted: %#v", arguments)
		}
	}
}
