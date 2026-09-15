package networkpolicy

import (
	"testing"
	"time"
)

func TestChangePlanIdentityBindsSafetyRelevantInputs(t *testing.T) {
	baseRequest := validChangePlanRequest()
	base, err := BuildChangePlan(baseRequest)
	if err != nil {
		t.Fatalf("BuildChangePlan(base) error = %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*ChangePlanRequest)
	}{
		{
			name: "changed interface blast radius",
			mutate: func(request *ChangePlanRequest) {
				request.Interfaces[2].Changed = true
			},
		},
		{
			name: "probe semantics",
			mutate: func(request *ChangePlanRequest) {
				request.Probes[0].Kind = ProbeZoneReachability
			},
		},
		{
			name: "probe timeout",
			mutate: func(request *ChangePlanRequest) {
				request.Timeouts.Probe += time.Second
			},
		},
		{
			name: "rollback timeout",
			mutate: func(request *ChangePlanRequest) {
				request.Timeouts.Rollback += time.Second
			},
		},
		{
			name: "forwarding set",
			mutate: func(request *ChangePlanRequest) {
				request.Forwarding = append(request.Forwarding, ForwardingIntent{
					Source:              ZoneLAN,
					Destination:         ZoneWAN,
					ExplicitlyEnabled:   true,
					EdgeGatewayAssigned: true,
				})
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := validChangePlanRequest()
			test.mutate(&request)
			candidate, err := BuildChangePlan(request)
			if err != nil {
				t.Fatalf("BuildChangePlan(candidate) error = %v", err)
			}
			if candidate.PlanID == base.PlanID {
				t.Fatalf("safety-relevant change reused plan id %q", base.PlanID)
			}
		})
	}
}
