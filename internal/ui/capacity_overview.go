package ui

import (
	"errors"
	"fmt"
	"math"
	"strings"

	"control-center/internal/agent"
	"control-center/internal/capacity"
	"control-center/internal/corecontracts"
)

const CapacityOverviewContractVersion = "ui.capacity-overview/v1"

var ErrInvalidCapacityOverview = errors.New("invalid capacity overview")

type CapacityOverviewState string

const (
	CapacityOverviewUnavailable   CapacityOverviewState = "unavailable"
	CapacityOverviewSafe          CapacityOverviewState = "safe"
	CapacityOverviewNeedsEvidence CapacityOverviewState = "needs-evidence"
	CapacityOverviewConstrained   CapacityOverviewState = "constrained"
)

type CapacityOverviewInput struct {
	Loaded     bool
	Assessment *capacity.Assessment
}

type CapacityOverview struct {
	ContractVersion    string                        `json:"contract_version"`
	State              CapacityOverviewState         `json:"state"`
	AssessmentID       string                        `json:"assessment_id,omitempty"`
	ScopeID            string                        `json:"scope_id,omitempty"`
	RequiredRole       corecontracts.NodeRole        `json:"required_role,omitempty"`
	WorkloadUnit       capacity.WorkloadUnit         `json:"workload_unit,omitempty"`
	HealthyNodes       int                           `json:"healthy_nodes,omitempty"`
	FailureReserve     int                           `json:"failure_reserve_nodes,omitempty"`
	CurrentWorkload    float64                       `json:"current_workload,omitempty"`
	ExpectedWorkload   float64                       `json:"expected_workload,omitempty"`
	SafeCapacity       float64                       `json:"safe_capacity,omitempty"`
	TechnicalLimit     float64                       `json:"technical_limit,omitempty"`
	SafeReserve        float64                       `json:"safe_reserve,omitempty"`
	Confidence         capacity.Confidence           `json:"confidence,omitempty"`
	BottleneckNodeID   string                        `json:"bottleneck_node_id,omitempty"`
	BottleneckMetric   agent.CapacityMetric          `json:"bottleneck_metric,omitempty"`
	BottleneckTarget   string                        `json:"bottleneck_target_id,omitempty"`
	Action             capacity.RecommendationAction `json:"action,omitempty"`
	AdvisoryOnly       bool                          `json:"advisory_only"`
	ProductionMutation bool                          `json:"production_mutation"`
}

// BuildCapacityOverview projects an already-computed Capacity assessment into
// a read-only UI contract. A numerically safe assessment that still requires
// evidence is never rendered as operator-safe.
func BuildCapacityOverview(input CapacityOverviewInput) (CapacityOverview, error) {
	view := CapacityOverview{
		ContractVersion: CapacityOverviewContractVersion,
		State:           CapacityOverviewUnavailable,
	}
	if !input.Loaded {
		return view, nil
	}
	if input.Assessment == nil {
		return CapacityOverview{}, fmt.Errorf("%w: loaded assessment is required", ErrInvalidCapacityOverview)
	}

	a := *input.Assessment
	if strings.TrimSpace(a.SchemaVersion) != capacity.AssessmentSchemaV1 {
		return CapacityOverview{}, fmt.Errorf("%w: unsupported assessment schema %q", ErrInvalidCapacityOverview, a.SchemaVersion)
	}
	if strings.TrimSpace(a.AssessmentID) == "" || a.AssessmentID != strings.TrimSpace(a.AssessmentID) || strings.TrimSpace(a.ScopeID) == "" || a.ScopeID != strings.TrimSpace(a.ScopeID) {
		return CapacityOverview{}, fmt.Errorf("%w: invalid assessment identity", ErrInvalidCapacityOverview)
	}
	if !a.RequiredRole.Valid() || !validCapacityWorkloadUnit(a.WorkloadUnit) {
		return CapacityOverview{}, fmt.Errorf("%w: invalid role or workload unit", ErrInvalidCapacityOverview)
	}
	if a.HealthyNodes <= 0 || a.FailureReserveNodes < 0 || a.FailureReserveNodes >= a.HealthyNodes {
		return CapacityOverview{}, fmt.Errorf("%w: invalid healthy-node or failure-reserve count", ErrInvalidCapacityOverview)
	}
	if !finiteCapacityUI(a.CurrentWorkload) || a.CurrentWorkload < 0 || !finiteCapacityUI(a.ExpectedWorkload) || a.ExpectedWorkload < 0 || !finiteCapacityUI(a.SafeCapacity) || a.SafeCapacity <= 0 || !finiteCapacityUI(a.TechnicalLimit) || a.TechnicalLimit <= a.SafeCapacity || !finiteCapacityUI(a.SafeReserve) {
		return CapacityOverview{}, fmt.Errorf("%w: invalid capacity values", ErrInvalidCapacityOverview)
	}
	if math.Abs((a.SafeCapacity-a.ExpectedWorkload)-a.SafeReserve) > 1e-9*math.Max(1, math.Abs(a.SafeReserve)) {
		return CapacityOverview{}, fmt.Errorf("%w: safe reserve does not match safe capacity and expected workload", ErrInvalidCapacityOverview)
	}
	if !validCapacityConfidence(a.Confidence) {
		return CapacityOverview{}, fmt.Errorf("%w: invalid confidence", ErrInvalidCapacityOverview)
	}
	if strings.TrimSpace(a.BottleneckNodeID) == "" || a.BottleneckNodeID != strings.TrimSpace(a.BottleneckNodeID) || strings.TrimSpace(a.BottleneckTargetID) == "" || a.BottleneckTargetID != strings.TrimSpace(a.BottleneckTargetID) {
		return CapacityOverview{}, fmt.Errorf("%w: invalid bottleneck identity", ErrInvalidCapacityOverview)
	}
	if _, ok := agent.DefinitionForCapacityMetric(a.BottleneckMetric); !ok {
		return CapacityOverview{}, fmt.Errorf("%w: unsupported bottleneck metric %q", ErrInvalidCapacityOverview, a.BottleneckMetric)
	}
	if !a.AdvisoryOnly || a.ProductionMutation {
		return CapacityOverview{}, fmt.Errorf("%w: capacity overview accepts advisory-only non-mutating assessments", ErrInvalidCapacityOverview)
	}
	if !validCapacityOverviewAction(a.Action) {
		return CapacityOverview{}, fmt.Errorf("%w: unsupported assessment action %q", ErrInvalidCapacityOverview, a.Action)
	}

	state := CapacityOverviewSafe
	if a.Action == capacity.ActionCollectEvidence {
		state = CapacityOverviewNeedsEvidence
	} else if !a.Safe || a.Action != capacity.ActionNone {
		state = CapacityOverviewConstrained
	}

	return CapacityOverview{
		ContractVersion:    CapacityOverviewContractVersion,
		State:              state,
		AssessmentID:       a.AssessmentID,
		ScopeID:            a.ScopeID,
		RequiredRole:       a.RequiredRole,
		WorkloadUnit:       a.WorkloadUnit,
		HealthyNodes:       a.HealthyNodes,
		FailureReserve:     a.FailureReserveNodes,
		CurrentWorkload:    a.CurrentWorkload,
		ExpectedWorkload:   a.ExpectedWorkload,
		SafeCapacity:       a.SafeCapacity,
		TechnicalLimit:     a.TechnicalLimit,
		SafeReserve:        a.SafeReserve,
		Confidence:         a.Confidence,
		BottleneckNodeID:   a.BottleneckNodeID,
		BottleneckMetric:   a.BottleneckMetric,
		BottleneckTarget:   a.BottleneckTargetID,
		Action:             a.Action,
		AdvisoryOnly:       true,
		ProductionMutation: false,
	}, nil
}

func validCapacityWorkloadUnit(unit capacity.WorkloadUnit) bool {
	switch unit {
	case capacity.WorkloadDevices,
		capacity.WorkloadConcurrentSessions,
		capacity.WorkloadJobsPerMinute,
		capacity.WorkloadRequestsPerSecond,
		capacity.WorkloadInstances:
		return true
	default:
		return false
	}
}

func validCapacityConfidence(confidence capacity.Confidence) bool {
	if !finiteCapacityUI(confidence.Score) || confidence.Score < 0 || confidence.Score > 1 {
		return false
	}
	switch confidence.Level {
	case capacity.ConfidenceLow, capacity.ConfidenceMedium, capacity.ConfidenceHigh, capacity.ConfidenceCertified:
		return true
	default:
		return false
	}
}

func validCapacityOverviewAction(action capacity.RecommendationAction) bool {
	switch action {
	case capacity.ActionNone, capacity.ActionCollectEvidence, capacity.ActionAddRoleCapacity:
		return true
	default:
		return false
	}
}

func finiteCapacityUI(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}
