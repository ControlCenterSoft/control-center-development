package capacity

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
)

const WorkloadScaleBottleneckAdviceSchemaV1 = "capacity.workload-scale-bottleneck-advice/v1"

type WorkloadScaleBottleneckAdviceStatus string

const (
	WorkloadScaleAdviceRecommendationAvailable WorkloadScaleBottleneckAdviceStatus = "recommendation-available"
	WorkloadScaleAdviceRecommendationCapped    WorkloadScaleBottleneckAdviceStatus = "recommendation-capped-by-efficiency"
	WorkloadScaleAdviceInspectBottleneck       WorkloadScaleBottleneckAdviceStatus = "inspect-bottleneck"
	WorkloadScaleAdviceCollectEvidence         WorkloadScaleBottleneckAdviceStatus = "collect-evidence"
	WorkloadScaleAdviceInsufficient            WorkloadScaleBottleneckAdviceStatus = "insufficient-capacity"
	WorkloadScaleAdviceBlocked                 WorkloadScaleBottleneckAdviceStatus = "blocked"
)

type WorkloadScaleBottleneckAdvice struct {
	SchemaVersion              string                              `json:"schema_version"`
	AdviceID                   string                              `json:"advice_id"`
	OptionsID                  string                              `json:"options_id"`
	BottleneckReportID         string                              `json:"bottleneck_report_id"`
	CandidateScenarioID        string                              `json:"candidate_scenario_id,omitempty"`
	CandidateResourceFactor    *float64                            `json:"candidate_resource_factor,omitempty"`
	RecommendedScenarioID      string                              `json:"recommended_scenario_id,omitempty"`
	RecommendedResourceFactor  *float64                            `json:"recommended_resource_factor,omitempty"`
	BottleneckNodeID           string                              `json:"bottleneck_node_id,omitempty"`
	BottleneckMetric           string                              `json:"bottleneck_metric,omitempty"`
	BottleneckTargetID         string                              `json:"bottleneck_target_id,omitempty"`
	BottleneckSeverity         BottleneckSeverity                  `json:"bottleneck_severity,omitempty"`
	BottleneckReservePercent   *float64                            `json:"bottleneck_reserve_percent,omitempty"`
	BottleneckReason           string                              `json:"bottleneck_reason,omitempty"`
	NextSegmentStart           *float64                            `json:"next_segment_start,omitempty"`
	NextSegmentEnd             *float64                            `json:"next_segment_end,omitempty"`
	NextSegmentEfficiencyRatio *float64                            `json:"next_segment_efficiency_ratio,omitempty"`
	Status                     WorkloadScaleBottleneckAdviceStatus `json:"status"`
	Reason                     string                              `json:"reason"`
	RecommendedAction          string                              `json:"recommended_action"`
	AdvisoryOnly               bool                                `json:"advisory_only"`
	ProductionMutation         bool                                `json:"production_mutation"`
}

// BuildWorkloadScaleBottleneckAdvice binds bounded scale options to a freshly
// rebuilt resource-specific bottleneck report. It exposes a candidate only as
// advisory evidence and withholds a qualified recommendation when fleet
// evidence is unknown/unsafe or when the candidate crosses a measured segment
// with diminishing marginal efficiency.
func BuildWorkloadScaleBottleneckAdvice(
	curve WorkloadCurve,
	efficiency WorkloadCurveEfficiencyReport,
	requests []WorkloadScaleScenarioRequest,
	bottleneckRequest BottleneckRequest,
	nodes []NodeProjection,
) (WorkloadScaleBottleneckAdvice, error) {
	if strings.TrimSpace(bottleneckRequest.ScopeID) != curve.ScopeID || bottleneckRequest.WorkloadUnit != curve.WorkloadUnit {
		return WorkloadScaleBottleneckAdvice{}, fmt.Errorf("%w: scale and bottleneck evidence must use the same scope and workload unit", ErrInvalidRecommendation)
	}

	options, err := EvaluateWorkloadScaleOptions(curve, efficiency, requests)
	if err != nil {
		return WorkloadScaleBottleneckAdvice{}, err
	}
	bottleneck, err := BuildBottleneckReport(bottleneckRequest, nodes)
	if err != nil {
		return WorkloadScaleBottleneckAdvice{}, err
	}
	if !options.AdvisoryOnly || options.ProductionMutation || !bottleneck.AdvisoryOnly || bottleneck.ProductionMutation {
		return WorkloadScaleBottleneckAdvice{}, fmt.Errorf("%w: unsafe planner evidence", ErrInvalidRecommendation)
	}

	advice := newWorkloadScaleBottleneckAdvice(options, bottleneck)
	if len(bottleneck.Findings) > 0 {
		finding := bottleneck.Findings[0]
		reserve := finding.EffectiveReservePercent
		advice.BottleneckNodeID = finding.NodeID
		advice.BottleneckMetric = string(finding.Metric)
		advice.BottleneckTargetID = finding.TargetID
		advice.BottleneckSeverity = finding.Severity
		advice.BottleneckReservePercent = &reserve
		advice.BottleneckReason = finding.Reason
	}

	if options.RecommendedResourceFactor == nil || options.RecommendedScenarioID == "" {
		mapUnselectedScaleAdvice(&advice, options)
		return advice, nil
	}

	candidateFactor := *options.RecommendedResourceFactor
	advice.CandidateScenarioID = options.RecommendedScenarioID
	advice.CandidateResourceFactor = floatPointer(candidateFactor)

	if bottleneck.UnknownCount > 0 {
		advice.Status = WorkloadScaleAdviceCollectEvidence
		advice.Reason = "bottleneck_evidence_is_not_confident_enough"
		advice.RecommendedAction = "collect-evidence"
		return advice, nil
	}
	if !bottleneck.FleetAssessment.Safe {
		advice.Status = WorkloadScaleAdviceInspectBottleneck
		advice.Reason = "failure_reserve_capacity_is_not_safe"
		advice.RecommendedAction = "inspect-bottleneck-before-scaling"
		return advice, nil
	}
	if bottleneck.CriticalCount > 0 || bottleneck.WarningCount > 0 {
		advice.Status = WorkloadScaleAdviceInspectBottleneck
		advice.Reason = "resource_specific_bottleneck_requires_review"
		advice.RecommendedAction = "inspect-bottleneck-before-scaling"
		return advice, nil
	}

	degraded := firstDegradedEfficiencySegment(efficiency)
	if degraded != nil && candidateFactor > degraded.ResourceFactorStart+floatTolerance(candidateFactor, degraded.ResourceFactorStart) {
		setNextEfficiencyEvidence(&advice, degraded)
		advice.Status = WorkloadScaleAdviceInspectBottleneck
		advice.Reason = "candidate_crosses_diminishing_returns_segment"
		advice.RecommendedAction = "inspect-bottleneck-before-scaling"
		return advice, nil
	}

	advice.RecommendedScenarioID = options.RecommendedScenarioID
	advice.RecommendedResourceFactor = floatPointer(candidateFactor)
	if next := nextMeasuredEfficiencySegment(efficiency, candidateFactor); next != nil {
		setNextEfficiencyEvidence(&advice, next)
		if next.EfficiencyRatio+floatTolerance(next.EfficiencyRatio, efficiency.Policy.MinimumEfficiencyRatio) < efficiency.Policy.MinimumEfficiencyRatio {
			advice.Status = WorkloadScaleAdviceRecommendationCapped
			advice.Reason = "next_measured_segment_has_diminishing_returns"
			advice.RecommendedAction = "hold-at-recommended-factor-and-inspect-bottleneck"
			return advice, nil
		}
	}

	advice.Status = WorkloadScaleAdviceRecommendationAvailable
	advice.Reason = "candidate_meets_headroom_and_bottleneck_evidence_is_healthy"
	advice.RecommendedAction = "none"
	return advice, nil
}

func mapUnselectedScaleAdvice(advice *WorkloadScaleBottleneckAdvice, options WorkloadScaleOptions) {
	switch options.Status {
	case WorkloadScaleOptionsCollectEvidence:
		advice.Status = WorkloadScaleAdviceCollectEvidence
	case WorkloadScaleOptionsInsufficient:
		advice.Status = WorkloadScaleAdviceInsufficient
	case WorkloadScaleOptionsBlocked:
		advice.Status = WorkloadScaleAdviceBlocked
	default:
		advice.Status = WorkloadScaleAdviceBlocked
	}
	advice.Reason = options.Reason
	advice.RecommendedAction = options.RecommendedAction
}

func newWorkloadScaleBottleneckAdvice(options WorkloadScaleOptions, bottleneck BottleneckReport) WorkloadScaleBottleneckAdvice {
	canonical := struct {
		OptionsID          string `json:"options_id"`
		BottleneckReportID string `json:"bottleneck_report_id"`
	}{options.OptionsID, bottleneck.ReportID}
	encoded, _ := json.Marshal(canonical)
	digest := sha256.Sum256(encoded)
	return WorkloadScaleBottleneckAdvice{
		SchemaVersion:      WorkloadScaleBottleneckAdviceSchemaV1,
		AdviceID:           "wsba-" + hex.EncodeToString(digest[:])[:24],
		OptionsID:          options.OptionsID,
		BottleneckReportID: bottleneck.ReportID,
		AdvisoryOnly:       true,
		ProductionMutation: false,
	}
}

func firstDegradedEfficiencySegment(efficiency WorkloadCurveEfficiencyReport) *WorkloadCurveEfficiencySegment {
	for index := range efficiency.Segments {
		segment := efficiency.Segments[index]
		if segment.EfficiencyRatio+floatTolerance(segment.EfficiencyRatio, efficiency.Policy.MinimumEfficiencyRatio) < efficiency.Policy.MinimumEfficiencyRatio {
			return &segment
		}
	}
	return nil
}

func nextMeasuredEfficiencySegment(efficiency WorkloadCurveEfficiencyReport, factor float64) *WorkloadCurveEfficiencySegment {
	for index := range efficiency.Segments {
		segment := efficiency.Segments[index]
		if segment.ResourceFactorStart+floatTolerance(segment.ResourceFactorStart, factor) >= factor {
			return &segment
		}
	}
	return nil
}

func setNextEfficiencyEvidence(advice *WorkloadScaleBottleneckAdvice, segment *WorkloadCurveEfficiencySegment) {
	advice.NextSegmentStart = floatPointer(segment.ResourceFactorStart)
	advice.NextSegmentEnd = floatPointer(segment.ResourceFactorEnd)
	advice.NextSegmentEfficiencyRatio = floatPointer(segment.EfficiencyRatio)
}

func floatPointer(value float64) *float64 {
	return &value
}
