package networkpolicy

import (
	"errors"
	"strings"
	"testing"
)

func TestBuildChangePlanRequiresControlPlaneProbeOnChangedManagementInterface(t *testing.T) {
	request := validChangePlanRequest()
	request.Interfaces[2].Changed = true
	request.Probes[2].Kind = ProbeLinkState
	request.Probes = append(request.Probes, ConnectivityProbe{
		ID:          "lan-control",
		Kind:        ProbeControlPlane,
		InterfaceID: "service-01",
		Zone:        ZoneLAN,
	})

	_, err := BuildChangePlan(request)
	if !errors.Is(err, ErrInvalidChangePlan) {
		t.Fatalf("BuildChangePlan() error = %v, want %v", err, ErrInvalidChangePlan)
	}
	if !strings.Contains(err.Error(), "changed management interface") {
		t.Fatalf("BuildChangePlan() error = %v, want changed management interface reason", err)
	}
}

func TestBuildChangePlanAcceptsChangedManagementInterfaceWithExactControlPlaneProbe(t *testing.T) {
	request := validChangePlanRequest()
	request.Interfaces[2].Changed = true

	plan, err := BuildChangePlan(request)
	if err != nil {
		t.Fatalf("BuildChangePlan() error = %v", err)
	}
	if plan.PlanID == "" {
		t.Fatal("BuildChangePlan() returned plan without identity")
	}
}
