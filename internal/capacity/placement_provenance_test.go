package capacity

import (
	"errors"
	"testing"
	"time"
)

func placementProvenanceFixture(t *testing.T) (PlacementRequest, []NodeProjection, PlacementAdviceSnapshot, []PlacementNodeProvenance, time.Time) {
	t.Helper()
	request, nodes := placementEvidenceFixture()
	placement, err := CapturePlacementAdviceSnapshot(request, nodes)
	if err != nil {
		t.Fatal(err)
	}
	evaluatedAt := time.Date(2026, 9, 10, 2, 0, 0, 0, time.UTC)
	provenance := make([]PlacementNodeProvenance, 0, len(nodes))
	for _, node := range nodes {
		provenance = append(provenance, PlacementNodeProvenance{
			NodeID:                 node.NodeID,
			ProfileObjectID:        "profile-" + node.NodeID,
			ProfileGeneration:      7,
			ProfileResourceVersion: "rv:profile:7",
			TelemetrySnapshotID:    "telemetry-" + node.NodeID,
			TelemetryRevision:      "rv:telemetry:11",
			TelemetryObservedAt:    evaluatedAt.Add(-5 * time.Minute),
			TelemetryMaxAgeSeconds: 3600,
		})
	}
	return request, nodes, placement, provenance, evaluatedAt
}

func TestPlacementProvenanceIsDeterministicAndAdvisoryOnly(t *testing.T) {
	_, nodes, placement, provenance, evaluatedAt := placementProvenanceFixture(t)
	first, err := CapturePlacementAdviceProvenanceSnapshot(placement, nodes, provenance, evaluatedAt)
	if err != nil {
		t.Fatal(err)
	}
	second, err := CapturePlacementAdviceProvenanceSnapshot(
		placement,
		[]NodeProjection{nodes[1], nodes[2], nodes[0]},
		[]PlacementNodeProvenance{provenance[1], provenance[2], provenance[0]},
		evaluatedAt,
	)
	if err != nil {
		t.Fatal(err)
	}
	if first.ProvenanceSnapshotID != second.ProvenanceSnapshotID || first.NodeProvenanceFingerprint != second.NodeProvenanceFingerprint {
		t.Fatalf("provenance depends on node order: %#v %#v", first, second)
	}
	if !first.AdvisoryOnly || first.ProductionMutation {
		t.Fatalf("provenance crossed advisory boundary: %#v", first)
	}
}

func TestPlacementProvenanceRevalidationKeepsFreshExactEvidenceCurrent(t *testing.T) {
	request, nodes, placement, provenance, evaluatedAt := placementProvenanceFixture(t)
	snapshot, err := CapturePlacementAdviceProvenanceSnapshot(placement, nodes, provenance, evaluatedAt)
	if err != nil {
		t.Fatal(err)
	}
	result, err := RevalidatePlacementAdviceProvenanceSnapshot(
		snapshot, placement, request, nodes, provenance, evaluatedAt.Add(10*time.Minute),
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != PlacementAdviceEvidenceCurrent || result.Reason != "exact-provenance-current" || !result.RecommendationReusable || result.RecommendedAction != "none" {
		t.Fatalf("exact provenance was not reusable: %#v", result)
	}
	if !result.AdvisoryOnly || result.ProductionMutation {
		t.Fatalf("revalidation crossed advisory boundary: %#v", result)
	}
}

func TestPlacementProvenanceRevalidationExplainsProfileRevisionDrift(t *testing.T) {
	request, nodes, placement, provenance, evaluatedAt := placementProvenanceFixture(t)
	snapshot, err := CapturePlacementAdviceProvenanceSnapshot(placement, nodes, provenance, evaluatedAt)
	if err != nil {
		t.Fatal(err)
	}
	provenance[1].ProfileGeneration++
	provenance[1].ProfileResourceVersion = "rv:profile:8"
	result, err := RevalidatePlacementAdviceProvenanceSnapshot(
		snapshot, placement, request, nodes, provenance, evaluatedAt.Add(10*time.Minute),
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != PlacementAdviceEvidenceStale || result.Reason != "capacity-profile-revision-drift" || result.StaleNodeID != "node-a" || result.RecommendationReusable {
		t.Fatalf("profile revision drift was not isolated: %#v", result)
	}
}

func TestPlacementProvenanceRevalidationExplainsTelemetryRevisionDrift(t *testing.T) {
	request, nodes, placement, provenance, evaluatedAt := placementProvenanceFixture(t)
	snapshot, err := CapturePlacementAdviceProvenanceSnapshot(placement, nodes, provenance, evaluatedAt)
	if err != nil {
		t.Fatal(err)
	}
	provenance[2].TelemetryRevision = "rv:telemetry:12"
	result, err := RevalidatePlacementAdviceProvenanceSnapshot(
		snapshot, placement, request, nodes, provenance, evaluatedAt.Add(10*time.Minute),
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != PlacementAdviceEvidenceStale || result.Reason != "telemetry-revision-drift" || result.StaleNodeID != "node-b" || result.RecommendationReusable {
		t.Fatalf("telemetry revision drift was not isolated: %#v", result)
	}
}

func TestPlacementProvenanceRevalidationExpiresTelemetryDeterministically(t *testing.T) {
	request, nodes, placement, provenance, evaluatedAt := placementProvenanceFixture(t)
	snapshot, err := CapturePlacementAdviceProvenanceSnapshot(placement, nodes, provenance, evaluatedAt)
	if err != nil {
		t.Fatal(err)
	}
	result, err := RevalidatePlacementAdviceProvenanceSnapshot(
		snapshot, placement, request, nodes, provenance, evaluatedAt.Add(2*time.Hour),
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != PlacementAdviceEvidenceStale || result.Reason != "telemetry-freshness-expired" || result.StaleNodeID != "node-a" || result.RecommendationReusable {
		t.Fatalf("expired telemetry remained reusable: %#v", result)
	}
}

func TestPlacementProvenanceRevalidationPrefersPlacementDrift(t *testing.T) {
	request, nodes, placement, provenance, evaluatedAt := placementProvenanceFixture(t)
	snapshot, err := CapturePlacementAdviceProvenanceSnapshot(placement, nodes, provenance, evaluatedAt)
	if err != nil {
		t.Fatal(err)
	}
	nodes[0].CurrentWorkload += 5
	result, err := RevalidatePlacementAdviceProvenanceSnapshot(
		snapshot, placement, request, nodes, provenance, evaluatedAt.Add(10*time.Minute),
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != PlacementAdviceEvidenceStale || result.Reason != "placement-evidence-stale" || result.RecommendationReusable {
		t.Fatalf("placement drift was not fail-closed: %#v", result)
	}
}

func TestPlacementProvenanceRejectsIncompleteCoverage(t *testing.T) {
	_, nodes, placement, provenance, evaluatedAt := placementProvenanceFixture(t)
	_, err := CapturePlacementAdviceProvenanceSnapshot(placement, nodes, provenance[:2], evaluatedAt)
	if !errors.Is(err, ErrInvalidRecommendation) {
		t.Fatalf("expected incomplete provenance rejection, got %v", err)
	}
}

func TestPlacementProvenanceRejectsTamperedSnapshot(t *testing.T) {
	request, nodes, placement, provenance, evaluatedAt := placementProvenanceFixture(t)
	snapshot, err := CapturePlacementAdviceProvenanceSnapshot(placement, nodes, provenance, evaluatedAt)
	if err != nil {
		t.Fatal(err)
	}
	snapshot.Nodes[0].TelemetryRevision = "rv:telemetry:tampered"
	_, err = RevalidatePlacementAdviceProvenanceSnapshot(
		snapshot, placement, request, nodes, provenance, evaluatedAt.Add(10*time.Minute),
	)
	if !errors.Is(err, ErrInvalidRecommendation) {
		t.Fatalf("expected tampered provenance rejection, got %v", err)
	}
}
