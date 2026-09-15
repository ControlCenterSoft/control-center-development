package ui

import (
	"errors"
	"testing"

	"control-center/internal/agent"
	"control-center/internal/capacity"
	"control-center/internal/corecontracts"
)

func TestBuildCapacityOverviewKeepsLowConfidenceOutOfSafeState(t *testing.T) {
	assessment := validCapacityOverviewAssessment()
	assessment.Confidence = capacity.Confidence{Level: capacity.ConfidenceLow, Score: 0.45}
	assessment.Action = capacity.ActionCollectEvidence
	assessment.Safe = true

	view, err := BuildCapacityOverview(CapacityOverviewInput{Loaded: true, Assessment: &assessment})
	if err != nil {
		t.Fatalf("BuildCapacityOverview() error = %v", err)
	}
	if view.State != CapacityOverviewNeedsEvidence {
		t.Fatalf("state = %q, want %q", view.State, CapacityOverviewNeedsEvidence)
	}
	if view.CurrentWorkload != 60 || view.ExpectedWorkload != 70 || view.SafeCapacity != 100 || view.TechnicalLimit != 150 || view.SafeReserve != 30 {
		t.Fatalf("capacity summary was not preserved: %#v", view)
	}
	if view.Confidence.Level != capacity.ConfidenceLow || view.Action != capacity.ActionCollectEvidence {
		t.Fatalf("confidence/action = %#v/%q", view.Confidence, view.Action)
	}
	if !view.AdvisoryOnly || view.ProductionMutation {
		t.Fatalf("read-only boundary changed: advisory=%v mutation=%v", view.AdvisoryOnly, view.ProductionMutation)
	}
}

func TestBuildCapacityOverviewDistinguishesSafeAndConstrained(t *testing.T) {
	t.Run("safe", func(t *testing.T) {
		assessment := validCapacityOverviewAssessment()
		view, err := BuildCapacityOverview(CapacityOverviewInput{Loaded: true, Assessment: &assessment})
		if err != nil {
			t.Fatalf("BuildCapacityOverview() error = %v", err)
		}
		if view.State != CapacityOverviewSafe {
			t.Fatalf("state = %q, want %q", view.State, CapacityOverviewSafe)
		}
	})

	t.Run("constrained", func(t *testing.T) {
		assessment := validCapacityOverviewAssessment()
		assessment.ExpectedWorkload = 120
		assessment.SafeReserve = -20
		assessment.Safe = false
		assessment.Action = capacity.ActionAddRoleCapacity

		view, err := BuildCapacityOverview(CapacityOverviewInput{Loaded: true, Assessment: &assessment})
		if err != nil {
			t.Fatalf("BuildCapacityOverview() error = %v", err)
		}
		if view.State != CapacityOverviewConstrained {
			t.Fatalf("state = %q, want %q", view.State, CapacityOverviewConstrained)
		}
	})
}

func TestBuildCapacityOverviewFailsClosed(t *testing.T) {
	t.Run("unavailable source remains explicit", func(t *testing.T) {
		view, err := BuildCapacityOverview(CapacityOverviewInput{Loaded: false})
		if err != nil {
			t.Fatalf("BuildCapacityOverview() error = %v", err)
		}
		if view.State != CapacityOverviewUnavailable || view.AssessmentID != "" {
			t.Fatalf("unexpected unavailable view: %#v", view)
		}
	})

	t.Run("loaded source requires assessment", func(t *testing.T) {
		_, err := BuildCapacityOverview(CapacityOverviewInput{Loaded: true})
		if !errors.Is(err, ErrInvalidCapacityOverview) {
			t.Fatalf("error = %v, want ErrInvalidCapacityOverview", err)
		}
	})

	t.Run("production mutation rejected", func(t *testing.T) {
		assessment := validCapacityOverviewAssessment()
		assessment.ProductionMutation = true
		_, err := BuildCapacityOverview(CapacityOverviewInput{Loaded: true, Assessment: &assessment})
		if !errors.Is(err, ErrInvalidCapacityOverview) {
			t.Fatalf("error = %v, want ErrInvalidCapacityOverview", err)
		}
	})

	t.Run("inconsistent reserve rejected", func(t *testing.T) {
		assessment := validCapacityOverviewAssessment()
		assessment.SafeReserve = 999
		_, err := BuildCapacityOverview(CapacityOverviewInput{Loaded: true, Assessment: &assessment})
		if !errors.Is(err, ErrInvalidCapacityOverview) {
			t.Fatalf("error = %v, want ErrInvalidCapacityOverview", err)
		}
	})
}

func validCapacityOverviewAssessment() capacity.Assessment {
	return capacity.Assessment{
		SchemaVersion:       capacity.AssessmentSchemaV1,
		AssessmentID:        "cap-ui-overview-001",
		ScopeID:             "site-a",
		RequiredRole:        corecontracts.RoleWorkerNode,
		WorkloadUnit:        capacity.WorkloadDevices,
		HealthyNodes:        2,
		FailureReserveNodes: 1,
		CurrentWorkload:     60,
		ExpectedWorkload:    70,
		SafeCapacity:        100,
		TechnicalLimit:      150,
		SafeReserve:         30,
		BottleneckNodeID:    "node-a",
		BottleneckMetric:    agent.MetricCPUUtilization,
		BottleneckTargetID:  "node-a",
		Confidence:          capacity.Confidence{Level: capacity.ConfidenceHigh, Score: 0.9},
		Safe:                true,
		Action:              capacity.ActionNone,
		AdvisoryOnly:        true,
		ProductionMutation:  false,
	}
}
