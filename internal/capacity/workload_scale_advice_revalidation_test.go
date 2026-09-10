package capacity

import (
	"errors"
	"testing"
)

func workloadScaleAdviceSnapshotFixture(t *testing.T, threshold float64, requiredWorkload float64) (
	WorkloadScaleAdviceSnapshot,
	WorkloadCurve,
	WorkloadCurveEfficiencyReport,
	[]WorkloadScaleScenarioRequest,
	BottleneckRequest,
	[]NodeProjection,
) {
	t.Helper()
	curve, efficiency := workloadScaleEvidence(t, threshold)
	requests := []WorkloadScaleScenarioRequest{
		{TargetResourceFactor: 4, RequiredWorkload: requiredWorkload, MinimumHeadroomPercent: 10},
		{TargetResourceFactor: 2, RequiredWorkload: requiredWorkload, MinimumHeadroomPercent: 10},
	}
	bottleneckRequest := workloadScaleBottleneckRequest()
	nodes := healthyScaleNodes()
	snapshot, err := CaptureWorkloadScaleAdviceSnapshot(curve, efficiency, requests, bottleneckRequest, nodes)
	if err != nil {
		t.Fatal(err)
	}
	return snapshot, curve, efficiency, requests, bottleneckRequest, nodes
}

func TestWorkloadScaleAdviceRevalidationIsDeterministicAndCurrent(t *testing.T) {
	snapshot, curve, efficiency, requests, bottleneckRequest, nodes := workloadScaleAdviceSnapshotFixture(t, 0.5, 150)

	first, err := RevalidateWorkloadScaleAdviceSnapshot(
		snapshot,
		curve,
		efficiency,
		[]WorkloadScaleScenarioRequest{requests[1], requests[0]},
		bottleneckRequest,
		[]NodeProjection{nodes[1], nodes[0]},
	)
	if err != nil {
		t.Fatal(err)
	}
	second, err := RevalidateWorkloadScaleAdviceSnapshot(snapshot, curve, efficiency, requests, bottleneckRequest, nodes)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("revalidation depends on input order: %#v != %#v", first, second)
	}
	if first.Status != WorkloadScaleAdviceEvidenceCurrent || first.Reason != "exact-evidence-current" {
		t.Fatalf("expected current evidence, got %#v", first)
	}
	if !first.RecommendationReusable || first.RecommendedAction != "none" {
		t.Fatalf("expected exact current recommendation to remain reusable: %#v", first)
	}
	if !first.AdvisoryOnly || first.ProductionMutation {
		t.Fatalf("revalidation crossed mutation boundary: %#v", first)
	}
}

func TestWorkloadScaleAdviceRevalidationInvalidatesBottleneckTelemetryDrift(t *testing.T) {
	snapshot, curve, efficiency, requests, bottleneckRequest, nodes := workloadScaleAdviceSnapshotFixture(t, 0.5, 150)
	nodes[0].BottleneckReserve = 12

	result, err := RevalidateWorkloadScaleAdviceSnapshot(snapshot, curve, efficiency, requests, bottleneckRequest, nodes)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != WorkloadScaleAdviceEvidenceStale || result.Reason != "bottleneck-evidence-drift" {
		t.Fatalf("expected bottleneck drift, got %#v", result)
	}
	if result.RecommendationReusable || result.RecommendedAction != "rebuild-advice-from-current-evidence" {
		t.Fatalf("stale bottleneck evidence must invalidate reuse: %#v", result)
	}
}

func TestWorkloadScaleAdviceRevalidationInvalidatesEfficiencyPolicyDrift(t *testing.T) {
	snapshot, curve, _, requests, bottleneckRequest, nodes := workloadScaleAdviceSnapshotFixture(t, 0.5, 100)
	changedEfficiency, err := AnalyzeWorkloadCurveEfficiency(curve, WorkloadCurveEfficiencyPolicy{
		MinimumEfficiencyRatio: 0.8,
		MinimumConfidence:      WorkloadProfileConfidenceMedium,
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := RevalidateWorkloadScaleAdviceSnapshot(snapshot, curve, changedEfficiency, requests, bottleneckRequest, nodes)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != WorkloadScaleAdviceEvidenceStale || result.Reason != "efficiency-evidence-drift" || result.RecommendationReusable {
		t.Fatalf("efficiency drift must invalidate reuse: %#v", result)
	}
}

func TestWorkloadScaleAdviceRevalidationInvalidatesScaleRequestDrift(t *testing.T) {
	snapshot, curve, efficiency, _, bottleneckRequest, nodes := workloadScaleAdviceSnapshotFixture(t, 0.5, 100)
	changedRequests := []WorkloadScaleScenarioRequest{
		{TargetResourceFactor: 2, RequiredWorkload: 140, MinimumHeadroomPercent: 10},
		{TargetResourceFactor: 4, RequiredWorkload: 140, MinimumHeadroomPercent: 10},
	}

	result, err := RevalidateWorkloadScaleAdviceSnapshot(snapshot, curve, efficiency, changedRequests, bottleneckRequest, nodes)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != WorkloadScaleAdviceEvidenceStale || result.Reason != "scale-options-evidence-drift" || result.RecommendationReusable {
		t.Fatalf("scale request drift must invalidate reuse: %#v", result)
	}
}

func TestWorkloadScaleAdviceRevalidationRejectsTamperedSavedDecision(t *testing.T) {
	snapshot, curve, efficiency, requests, bottleneckRequest, nodes := workloadScaleAdviceSnapshotFixture(t, 0.5, 150)
	snapshot.Advice.Reason = "tampered"

	_, err := RevalidateWorkloadScaleAdviceSnapshot(snapshot, curve, efficiency, requests, bottleneckRequest, nodes)
	if !errors.Is(err, ErrInvalidRecommendation) {
		t.Fatalf("expected tampered saved decision to fail closed, got %v", err)
	}
}

func TestWorkloadScaleAdviceRevalidationRejectsTamperedSnapshotIdentity(t *testing.T) {
	snapshot, curve, efficiency, requests, bottleneckRequest, nodes := workloadScaleAdviceSnapshotFixture(t, 0.5, 150)
	snapshot.SnapshotID = "wsas-000000000000000000000000"

	_, err := RevalidateWorkloadScaleAdviceSnapshot(snapshot, curve, efficiency, requests, bottleneckRequest, nodes)
	if !errors.Is(err, ErrInvalidRecommendation) {
		t.Fatalf("expected tampered snapshot identity to fail closed, got %v", err)
	}
}

func TestWorkloadScaleAdviceRevalidationRejectsUnsafeSavedFlags(t *testing.T) {
	snapshot, curve, efficiency, requests, bottleneckRequest, nodes := workloadScaleAdviceSnapshotFixture(t, 0.5, 150)
	snapshot.ProductionMutation = true

	_, err := RevalidateWorkloadScaleAdviceSnapshot(snapshot, curve, efficiency, requests, bottleneckRequest, nodes)
	if !errors.Is(err, ErrInvalidRecommendation) {
		t.Fatalf("expected unsafe saved flags to fail closed, got %v", err)
	}
}

func TestWorkloadScaleAdviceRevalidationDoesNotInventRecommendation(t *testing.T) {
	curve, efficiency := workloadScaleEvidence(t, 0.5)
	requests := []WorkloadScaleScenarioRequest{
		{TargetResourceFactor: 8, RequiredWorkload: 100, MinimumHeadroomPercent: 10},
	}
	bottleneckRequest := workloadScaleBottleneckRequest()
	nodes := healthyScaleNodes()
	snapshot, err := CaptureWorkloadScaleAdviceSnapshot(curve, efficiency, requests, bottleneckRequest, nodes)
	if err != nil {
		t.Fatal(err)
	}

	result, err := RevalidateWorkloadScaleAdviceSnapshot(snapshot, curve, efficiency, requests, bottleneckRequest, nodes)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != WorkloadScaleAdviceEvidenceCurrent || result.RecommendationReusable {
		t.Fatalf("current evidence without a recommendation must remain non-actionable: %#v", result)
	}
}
