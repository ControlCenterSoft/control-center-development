package capacity

import (
	"errors"
	"testing"

	"control-center/internal/agent"
	"control-center/internal/corecontracts"
)

func projection(id string, safe, current, reserve float64) NodeProjection {
	return NodeProjection{NodeID: id, Role: corecontracts.RoleWorkerNode, Healthy: true, CurrentWorkload: current, SafeCapacity: safe, TechnicalLimit: safe * 1.25, Confidence: Confidence{Level: ConfidenceHigh, Score: .85}, BottleneckMetric: agent.MetricCPUUtilization, BottleneckTargetID: id, BottleneckReserve: reserve}
}

func TestAssessmentPreservesLargestNodeFailureReserve(t *testing.T) {
	r := AssessmentRequest{ScopeID: "site-a", RequiredRole: corecontracts.RoleWorkerNode, WorkloadUnit: WorkloadDevices, FailureReserveNodes: 1, ExpectedWorkload: 145}
	a, err := BuildAssessment(r, []NodeProjection{projection("node-b", 80, 50, 30), projection("node-a", 100, 60, 20), projection("node-c", 70, 20, 10)})
	if err != nil {
		t.Fatal(err)
	}
	if a.SafeCapacity != 150 || a.SafeReserve != 5 || !a.Safe || a.BottleneckNodeID != "node-c" || a.Action != ActionNone {
		t.Fatalf("unexpected assessment: %#v", a)
	}
	if !a.AdvisoryOnly || a.ProductionMutation {
		t.Fatalf("assessment became authoritative: %#v", a)
	}
}

func TestAssessmentIsDeterministicAndReportsUnsafeCapacity(t *testing.T) {
	r := AssessmentRequest{ScopeID: "site-a", RequiredRole: corecontracts.RoleWorkerNode, WorkloadUnit: WorkloadDevices, FailureReserveNodes: 1, ExpectedWorkload: 200}
	nodes := []NodeProjection{projection("node-z", 100, 50, 5), projection("node-a", 100, 50, 5), projection("node-b", 80, 40, 20)}
	a, err := BuildAssessment(r, nodes)
	if err != nil {
		t.Fatal(err)
	}
	b, err := BuildAssessment(r, []NodeProjection{nodes[2], nodes[0], nodes[1]})
	if err != nil {
		t.Fatal(err)
	}
	if a != b {
		t.Fatalf("assessment depends on order: %#v %#v", a, b)
	}
	if a.Safe || a.Action != ActionAddRoleCapacity || a.BottleneckNodeID != "node-a" {
		t.Fatalf("unsafe capacity not reported: %#v", a)
	}
}

func TestAssessmentLowConfidenceAlwaysCollectsEvidence(t *testing.T) {
	tests := []struct {
		name             string
		expectedWorkload float64
		wantSafe         bool
	}{
		{name: "safe", expectedWorkload: 80, wantSafe: true},
		{name: "unsafe", expectedWorkload: 120, wantSafe: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			node := projection("node-a", 100, 50, 20)
			node.Confidence = Confidence{Level: ConfidenceLow, Score: .4}

			a, err := BuildAssessment(AssessmentRequest{
				ScopeID:          "site-a",
				RequiredRole:     corecontracts.RoleWorkerNode,
				WorkloadUnit:     WorkloadDevices,
				ExpectedWorkload: test.expectedWorkload,
			}, []NodeProjection{node})
			if err != nil {
				t.Fatal(err)
			}
			if a.Safe != test.wantSafe || a.Action != ActionCollectEvidence {
				t.Fatalf("low-confidence assessment crossed evidence gate: %#v", a)
			}
			if !a.AdvisoryOnly || a.ProductionMutation {
				t.Fatalf("low-confidence assessment became authoritative: %#v", a)
			}
		})
	}
}

func TestAssessmentFailsClosedWhenReserveConsumesFleet(t *testing.T) {
	_, err := BuildAssessment(AssessmentRequest{ScopeID: "site-a", RequiredRole: corecontracts.RoleWorkerNode, WorkloadUnit: WorkloadDevices, FailureReserveNodes: 1, ExpectedWorkload: 1}, []NodeProjection{projection("node-a", 100, 1, 50)})
	if !errors.Is(err, ErrInvalidRecommendation) {
		t.Fatalf("expected validation error, got %v", err)
	}
}
