package capacity

import (
	"errors"
	"reflect"
	"testing"

	"control-center/internal/corecontracts"
)

func workloadScaleBottleneckRequest() BottleneckRequest {
	return BottleneckRequest{
		ScopeID:                "site-a",
		RequiredRole:           corecontracts.RoleWorkerNode,
		WorkloadUnit:           WorkloadRequestsPerSecond,
		FailureReserveNodes:    0,
		WarningReservePercent:  20,
		CriticalReservePercent: 5,
	}
}

func healthyScaleNodes() []NodeProjection {
	return []NodeProjection{
		projection("node-b", 220, 40, 35),
		projection("node-a", 180, 30, 45),
	}
}

func TestWorkloadScaleBottleneckAdviceDeterministicAndExplainable(t *testing.T) {
	curve, efficiency := workloadScaleEvidence(t, 0.5)
	requests := []WorkloadScaleScenarioRequest{
		{TargetResourceFactor: 4, RequiredWorkload: 150, MinimumHeadroomPercent: 10},
		{TargetResourceFactor: 2, RequiredWorkload: 150, MinimumHeadroomPercent: 10},
	}
	nodes := healthyScaleNodes()

	first, err := BuildWorkloadScaleBottleneckAdvice(curve, efficiency, requests, workloadScaleBottleneckRequest(), nodes)
	if err != nil {
		t.Fatal(err)
	}
	second, err := BuildWorkloadScaleBottleneckAdvice(
		curve,
		efficiency,
		[]WorkloadScaleScenarioRequest{requests[1], requests[0]},
		workloadScaleBottleneckRequest(),
		[]NodeProjection{nodes[1], nodes[0]},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("advice depends on input order: %#v %#v", first, second)
	}
	if first.Status != WorkloadScaleAdviceRecommendationAvailable || first.RecommendedAction != "none" {
		t.Fatalf("unexpected advice: %#v", first)
	}
	if first.RecommendedResourceFactor == nil || !closeFloat(*first.RecommendedResourceFactor, 2) {
		t.Fatalf("expected factor 2, got %#v", first.RecommendedResourceFactor)
	}
	if first.BottleneckNodeID != "node-b" || first.BottleneckMetric == "" || first.BottleneckTargetID != "node-b" || first.BottleneckReason == "" {
		t.Fatalf("missing resource-specific explanation: %#v", first)
	}
	if !first.AdvisoryOnly || first.ProductionMutation {
		t.Fatalf("advice crossed mutation boundary: %#v", first)
	}
}

func TestWorkloadScaleBottleneckAdviceCapsAtDegradingNextSegment(t *testing.T) {
	curve, efficiency := workloadScaleEvidence(t, 0.8)
	advice, err := BuildWorkloadScaleBottleneckAdvice(curve, efficiency, []WorkloadScaleScenarioRequest{
		{TargetResourceFactor: 2, RequiredWorkload: 100, MinimumHeadroomPercent: 10},
		{TargetResourceFactor: 4, RequiredWorkload: 100, MinimumHeadroomPercent: 10},
	}, workloadScaleBottleneckRequest(), healthyScaleNodes())
	if err != nil {
		t.Fatal(err)
	}
	if advice.Status != WorkloadScaleAdviceRecommendationCapped || advice.RecommendedAction != "hold-at-recommended-factor-and-inspect-bottleneck" {
		t.Fatalf("expected efficiency cap: %#v", advice)
	}
	if advice.RecommendedResourceFactor == nil || !closeFloat(*advice.RecommendedResourceFactor, 2) {
		t.Fatalf("expected factor 2 to remain the bounded recommendation: %#v", advice)
	}
	if advice.NextSegmentStart == nil || advice.NextSegmentEnd == nil || advice.NextSegmentEfficiencyRatio == nil ||
		!closeFloat(*advice.NextSegmentStart, 2) || !closeFloat(*advice.NextSegmentEnd, 4) || !closeFloat(*advice.NextSegmentEfficiencyRatio, 0.75) {
		t.Fatalf("missing exact degrading segment evidence: %#v", advice)
	}
}

func TestWorkloadScaleBottleneckAdviceRejectsCandidateCrossingDegradedSegment(t *testing.T) {
	curve, efficiency := workloadScaleEvidence(t, 0.8)
	advice, err := BuildWorkloadScaleBottleneckAdvice(curve, efficiency, []WorkloadScaleScenarioRequest{
		{TargetResourceFactor: 2, RequiredWorkload: 220, MinimumHeadroomPercent: 10},
		{TargetResourceFactor: 4, RequiredWorkload: 220, MinimumHeadroomPercent: 10},
	}, workloadScaleBottleneckRequest(), healthyScaleNodes())
	if err != nil {
		t.Fatal(err)
	}
	if advice.Status != WorkloadScaleAdviceInspectBottleneck || advice.Reason != "candidate_crosses_diminishing_returns_segment" {
		t.Fatalf("expected degraded candidate to be held for inspection: %#v", advice)
	}
	if advice.CandidateResourceFactor == nil || !closeFloat(*advice.CandidateResourceFactor, 4) || advice.RecommendedResourceFactor != nil {
		t.Fatalf("degraded candidate must not become a recommendation: %#v", advice)
	}
}

func TestWorkloadScaleBottleneckAdviceRequiresConfidence(t *testing.T) {
	curve, efficiency := workloadScaleEvidence(t, 0.5)
	nodes := healthyScaleNodes()
	nodes[0].Confidence = Confidence{Level: ConfidenceLow, Score: 0.4}
	advice, err := BuildWorkloadScaleBottleneckAdvice(curve, efficiency, []WorkloadScaleScenarioRequest{
		{TargetResourceFactor: 2, RequiredWorkload: 100, MinimumHeadroomPercent: 10},
	}, workloadScaleBottleneckRequest(), nodes)
	if err != nil {
		t.Fatal(err)
	}
	if advice.Status != WorkloadScaleAdviceCollectEvidence || advice.RecommendedAction != "collect-evidence" || advice.RecommendedResourceFactor != nil {
		t.Fatalf("low-confidence bottleneck must withhold recommendation: %#v", advice)
	}
}

func TestWorkloadScaleBottleneckAdviceRequiresHealthyResourceReserve(t *testing.T) {
	curve, efficiency := workloadScaleEvidence(t, 0.5)
	nodes := healthyScaleNodes()
	nodes[0].BottleneckReserve = 12
	advice, err := BuildWorkloadScaleBottleneckAdvice(curve, efficiency, []WorkloadScaleScenarioRequest{
		{TargetResourceFactor: 2, RequiredWorkload: 100, MinimumHeadroomPercent: 10},
	}, workloadScaleBottleneckRequest(), nodes)
	if err != nil {
		t.Fatal(err)
	}
	if advice.Status != WorkloadScaleAdviceInspectBottleneck || advice.RecommendedResourceFactor != nil {
		t.Fatalf("warning bottleneck must withhold recommendation: %#v", advice)
	}
	if advice.BottleneckNodeID != "node-b" || advice.BottleneckSeverity != BottleneckWarning || advice.BottleneckReservePercent == nil || !closeFloat(*advice.BottleneckReservePercent, 12) {
		t.Fatalf("warning bottleneck evidence is not explained: %#v", advice)
	}
}

func TestWorkloadScaleBottleneckAdvicePreservesNoExtrapolation(t *testing.T) {
	curve, efficiency := workloadScaleEvidence(t, 0.5)
	advice, err := BuildWorkloadScaleBottleneckAdvice(curve, efficiency, []WorkloadScaleScenarioRequest{
		{TargetResourceFactor: 8, RequiredWorkload: 100, MinimumHeadroomPercent: 10},
	}, workloadScaleBottleneckRequest(), healthyScaleNodes())
	if err != nil {
		t.Fatal(err)
	}
	if advice.Status != WorkloadScaleAdviceCollectEvidence || advice.CandidateResourceFactor != nil || advice.RecommendedResourceFactor != nil {
		t.Fatalf("out-of-curve request must remain evidence-only: %#v", advice)
	}
}

func TestWorkloadScaleBottleneckAdviceRejectsScopeOrUnitMismatch(t *testing.T) {
	curve, efficiency := workloadScaleEvidence(t, 0.5)
	request := workloadScaleBottleneckRequest()
	request.ScopeID = "site-b"
	_, err := BuildWorkloadScaleBottleneckAdvice(curve, efficiency, []WorkloadScaleScenarioRequest{
		{TargetResourceFactor: 2, RequiredWorkload: 100, MinimumHeadroomPercent: 10},
	}, request, healthyScaleNodes())
	if !errors.Is(err, ErrInvalidRecommendation) {
		t.Fatalf("expected scope mismatch to fail closed, got %v", err)
	}

	request = workloadScaleBottleneckRequest()
	request.WorkloadUnit = WorkloadDevices
	_, err = BuildWorkloadScaleBottleneckAdvice(curve, efficiency, []WorkloadScaleScenarioRequest{
		{TargetResourceFactor: 2, RequiredWorkload: 100, MinimumHeadroomPercent: 10},
	}, request, healthyScaleNodes())
	if !errors.Is(err, ErrInvalidRecommendation) {
		t.Fatalf("expected workload-unit mismatch to fail closed, got %v", err)
	}
}
