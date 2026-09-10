package capacity

import (
	"errors"
	"testing"

	"control-center/internal/corecontracts"
)

func placementEvidenceFixture() (PlacementRequest, []NodeProjection) {
	request := PlacementRequest{
		ScopeID:                   "site-a",
		RequiredRole:              corecontracts.RoleWorkerNode,
		WorkloadUnit:              WorkloadDevices,
		IncrementalWorkload:       20,
		FailureReserveNodes:       1,
		MinimumNodeReservePercent: 20,
	}
	nodes := []NodeProjection{
		projection("node-c", 60, 10, 30),
		projection("node-a", 100, 40, 25),
		projection("node-b", 80, 20, 10),
	}
	return request, nodes
}

func TestPlacementAdviceSnapshotIsDeterministicAndNonAuthorizing(t *testing.T) {
	request, nodes := placementEvidenceFixture()
	first, err := CapturePlacementAdviceSnapshot(request, nodes)
	if err != nil {
		t.Fatal(err)
	}
	second, err := CapturePlacementAdviceSnapshot(request, []NodeProjection{nodes[2], nodes[0], nodes[1]})
	if err != nil {
		t.Fatal(err)
	}
	if first.SnapshotID != second.SnapshotID || first.NodeEvidenceFingerprint != second.NodeEvidenceFingerprint || first.Advice.AdviceID != second.Advice.AdviceID {
		t.Fatalf("snapshot depends on node order: %#v %#v", first, second)
	}
	if first.SchemaVersion != PlacementAdviceSnapshotSchemaV1 || !first.AdvisoryOnly || first.ProductionMutation {
		t.Fatalf("snapshot crossed advisory boundary: %#v", first)
	}
	if first.Advice.RecommendedNodeID != "node-c" {
		t.Fatalf("unexpected recommendation: %#v", first.Advice)
	}
}

func TestPlacementAdviceRevalidationKeepsExactEvidenceCurrent(t *testing.T) {
	request, nodes := placementEvidenceFixture()
	snapshot, err := CapturePlacementAdviceSnapshot(request, nodes)
	if err != nil {
		t.Fatal(err)
	}
	result, err := RevalidatePlacementAdviceSnapshot(snapshot, request, []NodeProjection{nodes[1], nodes[2], nodes[0]})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != PlacementAdviceEvidenceCurrent || result.Reason != "exact-evidence-current" || !result.RecommendationReusable || result.RecommendedAction != "none" {
		t.Fatalf("exact evidence was not reusable: %#v", result)
	}
	if result.CurrentRecommendedNodeID != "node-c" || !result.AdvisoryOnly || result.ProductionMutation {
		t.Fatalf("revalidation crossed advisory boundary: %#v", result)
	}
}

func TestPlacementAdviceRevalidationInvalidatesConstraintDrift(t *testing.T) {
	request, nodes := placementEvidenceFixture()
	snapshot, err := CapturePlacementAdviceSnapshot(request, nodes)
	if err != nil {
		t.Fatal(err)
	}
	request.MinimumNodeReservePercent = 35
	result, err := RevalidatePlacementAdviceSnapshot(snapshot, request, nodes)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != PlacementAdviceEvidenceStale || result.Reason != "placement-request-or-constraint-drift" || result.RecommendationReusable {
		t.Fatalf("constraint drift did not stale recommendation: %#v", result)
	}
}

func TestPlacementAdviceRevalidationInvalidatesTelemetryDrift(t *testing.T) {
	request, nodes := placementEvidenceFixture()
	snapshot, err := CapturePlacementAdviceSnapshot(request, nodes)
	if err != nil {
		t.Fatal(err)
	}
	nodes[0].CurrentWorkload += 5
	result, err := RevalidatePlacementAdviceSnapshot(snapshot, request, nodes)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != PlacementAdviceEvidenceStale || result.Reason != "node-capacity-or-telemetry-evidence-drift" || result.RecommendationReusable {
		t.Fatalf("telemetry drift did not stale recommendation: %#v", result)
	}
}

func TestPlacementAdviceSnapshotRejectsTamperedDecision(t *testing.T) {
	request, nodes := placementEvidenceFixture()
	snapshot, err := CapturePlacementAdviceSnapshot(request, nodes)
	if err != nil {
		t.Fatal(err)
	}
	snapshot.Advice.RecommendedNodeID = "node-a"
	_, err = RevalidatePlacementAdviceSnapshot(snapshot, request, nodes)
	if !errors.Is(err, ErrInvalidRecommendation) {
		t.Fatalf("expected integrity rejection, got %v", err)
	}
}

func TestPlacementAdviceSnapshotRejectsUnsafeMutationFlag(t *testing.T) {
	request, nodes := placementEvidenceFixture()
	snapshot, err := CapturePlacementAdviceSnapshot(request, nodes)
	if err != nil {
		t.Fatal(err)
	}
	snapshot.ProductionMutation = true
	_, err = RevalidatePlacementAdviceSnapshot(snapshot, request, nodes)
	if !errors.Is(err, ErrInvalidRecommendation) {
		t.Fatalf("expected unsafe snapshot rejection, got %v", err)
	}
}

func TestPlacementAdviceRevalidationDoesNotReuseUnsafeFleetRecommendation(t *testing.T) {
	request := PlacementRequest{
		ScopeID:                   "site-a",
		RequiredRole:              corecontracts.RoleWorkerNode,
		WorkloadUnit:              WorkloadDevices,
		IncrementalWorkload:       40,
		FailureReserveNodes:       1,
		MinimumNodeReservePercent: 0,
	}
	nodes := []NodeProjection{
		projection("node-a", 50, 40, 20),
		projection("node-b", 50, 40, 20),
		projection("node-c", 50, 40, 20),
	}
	snapshot, err := CapturePlacementAdviceSnapshot(request, nodes)
	if err != nil {
		t.Fatal(err)
	}
	result, err := RevalidatePlacementAdviceSnapshot(snapshot, request, nodes)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != PlacementAdviceEvidenceCurrent || result.RecommendationReusable || result.CurrentRecommendedNodeID != "" {
		t.Fatalf("unsafe fleet recommendation became reusable: %#v", result)
	}
}

func TestPlacementAdviceRevalidationIDIsDeterministic(t *testing.T) {
	request, nodes := placementEvidenceFixture()
	snapshot, err := CapturePlacementAdviceSnapshot(request, nodes)
	if err != nil {
		t.Fatal(err)
	}
	first, err := RevalidatePlacementAdviceSnapshot(snapshot, request, nodes)
	if err != nil {
		t.Fatal(err)
	}
	second, err := RevalidatePlacementAdviceSnapshot(snapshot, request, []NodeProjection{nodes[2], nodes[0], nodes[1]})
	if err != nil {
		t.Fatal(err)
	}
	if first.RevalidationID != second.RevalidationID {
		t.Fatalf("revalidation id depends on node order: %#v %#v", first, second)
	}
}
