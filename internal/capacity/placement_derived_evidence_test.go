package capacity

import (
	"errors"
	"testing"
	"time"

	"control-center/internal/agent"
)

func TestPlacementAdviceFromDerivationsBindsStrongEvidence(t *testing.T) {
	request, _, input, _, evaluatedAt := placementDerivationBindingFixture(t)
	snapshot, err := CapturePlacementAdviceFromDerivations(
		request,
		[]PlacementNodeDerivationInput{input},
		evaluatedAt,
	)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.SchemaVersion != PlacementAdviceDerivedEvidenceSchemaV1 || snapshot.SnapshotID == "" {
		t.Fatalf("unexpected derived placement identity: %#v", snapshot)
	}
	if snapshot.Placement.Advice.RecommendedNodeID != input.Evidence.Projection.NodeID {
		t.Fatalf("unexpected recommendation: %#v", snapshot.Placement.Advice)
	}
	if snapshot.DerivationProvenance.Provenance.PlacementSnapshotID != snapshot.Placement.SnapshotID {
		t.Fatalf("placement/provenance binding mismatch: %#v", snapshot)
	}
	if !snapshot.AdvisoryOnly || snapshot.ProductionMutation ||
		!snapshot.Placement.AdvisoryOnly || snapshot.Placement.ProductionMutation ||
		!snapshot.DerivationProvenance.AdvisoryOnly || snapshot.DerivationProvenance.ProductionMutation {
		t.Fatalf("derived evidence crossed advisory boundary: %#v", snapshot)
	}
	if err := ValidatePlacementAdviceDerivedEvidence(snapshot); err != nil {
		t.Fatal(err)
	}
}

func TestPlacementAdviceFromDerivationsIsDeterministic(t *testing.T) {
	request, _, input, _, evaluatedAt := placementDerivationBindingFixture(t)
	first, err := CapturePlacementAdviceFromDerivations(
		request,
		[]PlacementNodeDerivationInput{input},
		evaluatedAt,
	)
	if err != nil {
		t.Fatal(err)
	}
	second, err := CapturePlacementAdviceFromDerivations(
		request,
		[]PlacementNodeDerivationInput{input},
		evaluatedAt,
	)
	if err != nil {
		t.Fatal(err)
	}
	if first.SnapshotID != second.SnapshotID ||
		first.Placement.SnapshotID != second.Placement.SnapshotID ||
		first.DerivationProvenance.SnapshotID != second.DerivationProvenance.SnapshotID {
		t.Fatalf("same exact inputs changed evidence: %#v %#v", first, second)
	}
}

func TestPlacementAdviceFromDerivationsRejectsTamperedProjectionEvidence(t *testing.T) {
	request, _, input, _, evaluatedAt := placementDerivationBindingFixture(t)
	input.Evidence.Projection.SafeCapacity++
	_, err := CapturePlacementAdviceFromDerivations(
		request,
		[]PlacementNodeDerivationInput{input},
		evaluatedAt,
	)
	if !errors.Is(err, ErrInvalidRecommendation) {
		t.Fatalf("tampered derivation error = %v", err)
	}
}

func TestPlacementAdviceFromDerivationsBindsHiddenObservationChanges(t *testing.T) {
	request, _, input, _, evaluatedAt := placementDerivationBindingFixture(t)
	first, err := CapturePlacementAdviceFromDerivations(
		request,
		[]PlacementNodeDerivationInput{input},
		evaluatedAt,
	)
	if err != nil {
		t.Fatal(err)
	}

	changed := input
	changed.Telemetry.Observations = append(
		[]agent.CapacityObservation(nil),
		input.Telemetry.Observations...,
	)
	changed.Telemetry.Observations[1].Value = 62
	changed.Evidence, err = DeriveNodeProjection(
		changed.Profile,
		changed.Telemetry,
		changed.Evidence.EvaluatedAt,
	)
	if err != nil {
		t.Fatal(err)
	}
	if changed.Evidence.Projection != input.Evidence.Projection {
		t.Fatalf("fixture no longer isolates hidden observation drift")
	}

	second, err := CapturePlacementAdviceFromDerivations(
		request,
		[]PlacementNodeDerivationInput{changed},
		evaluatedAt,
	)
	if err != nil {
		t.Fatal(err)
	}
	if first.Placement.SnapshotID != second.Placement.SnapshotID {
		t.Fatalf("equivalent projections unexpectedly changed placement snapshot")
	}
	if first.DerivationProvenance.DerivationFingerprint == second.DerivationProvenance.DerivationFingerprint ||
		first.SnapshotID == second.SnapshotID {
		t.Fatalf("hidden evidence drift was not bound into strong snapshot")
	}
}

func TestPlacementAdviceDerivedEvidenceRejectsCrossWiredOrUnsafeEnvelope(t *testing.T) {
	request, _, input, _, evaluatedAt := placementDerivationBindingFixture(t)
	snapshot, err := CapturePlacementAdviceFromDerivations(
		request,
		[]PlacementNodeDerivationInput{input},
		evaluatedAt,
	)
	if err != nil {
		t.Fatal(err)
	}

	crossWired := snapshot
	crossWired.DerivationProvenance.Provenance.PlacementSnapshotID = "pas-cross-wired"
	if err := ValidatePlacementAdviceDerivedEvidence(crossWired); !errors.Is(err, ErrInvalidRecommendation) {
		t.Fatalf("cross-wired envelope error = %v", err)
	}

	unsafe := snapshot
	unsafe.ProductionMutation = true
	if err := ValidatePlacementAdviceDerivedEvidence(unsafe); !errors.Is(err, ErrInvalidRecommendation) {
		t.Fatalf("unsafe envelope error = %v", err)
	}
}

func TestPlacementAdviceFromDerivationsRejectsDuplicateOrExpiredEvidence(t *testing.T) {
	request, _, input, _, evaluatedAt := placementDerivationBindingFixture(t)
	if _, err := CapturePlacementAdviceFromDerivations(
		request,
		[]PlacementNodeDerivationInput{input, input},
		evaluatedAt,
	); !errors.Is(err, ErrInvalidRecommendation) {
		t.Fatalf("duplicate derivation error = %v", err)
	}
	if _, err := CapturePlacementAdviceFromDerivations(
		request,
		[]PlacementNodeDerivationInput{input},
		input.Evidence.EvaluatedAt.Add(16*time.Minute),
	); !errors.Is(err, ErrInvalidRecommendation) {
		t.Fatalf("expired derivation error = %v", err)
	}
}
