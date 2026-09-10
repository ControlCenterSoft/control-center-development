package capacity

import (
	"errors"
	"testing"
	"time"

	"control-center/internal/agent"
)

func derivedPlacementRevalidationFixture(t *testing.T) (
	PlacementRequest,
	PlacementNodeDerivationInput,
	PlacementAdviceDerivedEvidenceSnapshot,
	time.Time,
) {
	t.Helper()
	request, _, input, _, evaluatedAt := placementDerivationBindingFixture(t)
	saved, err := CapturePlacementAdviceFromDerivations(
		request,
		[]PlacementNodeDerivationInput{input},
		evaluatedAt,
	)
	if err != nil {
		t.Fatal(err)
	}
	return request, input, saved, evaluatedAt
}

func TestPlacementAdviceDerivedEvidenceRevalidationExactCurrent(t *testing.T) {
	request, input, saved, evaluatedAt := derivedPlacementRevalidationFixture(t)
	result, err := RevalidatePlacementAdviceDerivedEvidence(
		saved,
		request,
		[]PlacementNodeDerivationInput{input},
		evaluatedAt,
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != PlacementAdviceEvidenceCurrent ||
		result.Reason != "exact-derived-evidence-current" ||
		result.CurrentSnapshotID != saved.SnapshotID ||
		!result.RecommendationReusable ||
		result.RecommendedAction != "none" ||
		!result.AdvisoryOnly || result.ProductionMutation {
		t.Fatalf("exact evidence was not current and advisory-only: %#v", result)
	}
	if err := ValidatePlacementAdviceDerivedEvidenceRevalidation(result); err != nil {
		t.Fatal(err)
	}
}

func TestPlacementAdviceDerivedEvidenceRevalidationIsDeterministic(t *testing.T) {
	request, input, saved, evaluatedAt := derivedPlacementRevalidationFixture(t)
	first, err := RevalidatePlacementAdviceDerivedEvidence(saved, request, []PlacementNodeDerivationInput{input}, evaluatedAt)
	if err != nil {
		t.Fatal(err)
	}
	second, err := RevalidatePlacementAdviceDerivedEvidence(saved, request, []PlacementNodeDerivationInput{input}, evaluatedAt)
	if err != nil {
		t.Fatal(err)
	}
	if first.RevalidationID != second.RevalidationID || first != second {
		t.Fatalf("same exact evidence changed revalidation: %#v %#v", first, second)
	}
}

func TestPlacementAdviceDerivedEvidenceRevalidationReportsRequestConstraintDrift(t *testing.T) {
	request, input, saved, evaluatedAt := derivedPlacementRevalidationFixture(t)
	request.MinimumNodeReservePercent++
	result, err := RevalidatePlacementAdviceDerivedEvidence(saved, request, []PlacementNodeDerivationInput{input}, evaluatedAt)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != PlacementAdviceEvidenceStale ||
		result.Reason != "placement-request-or-constraint-drift" ||
		result.RecommendationReusable || result.CurrentSnapshotID == "" {
		t.Fatalf("request/constraint drift was not fail-closed: %#v", result)
	}
}

func TestPlacementAdviceDerivedEvidenceRevalidationReportsProfileRevisionDrift(t *testing.T) {
	request, input, saved, evaluatedAt := derivedPlacementRevalidationFixture(t)
	changed := input
	changed.Profile.ResourceVersion = "rv-profile-current"
	var err error
	changed.Evidence, err = DeriveNodeProjection(changed.Profile, changed.Telemetry, input.Evidence.EvaluatedAt)
	if err != nil {
		t.Fatal(err)
	}
	result, err := RevalidatePlacementAdviceDerivedEvidence(saved, request, []PlacementNodeDerivationInput{changed}, evaluatedAt)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != PlacementAdviceEvidenceStale ||
		result.Reason != "capacity-profile-revision-drift" ||
		result.StaleNodeID != input.Evidence.Projection.NodeID ||
		result.RecommendedAction != "refresh-capacity-profile-evidence" ||
		result.RecommendationReusable {
		t.Fatalf("profile drift was not explained fail-closed: %#v", result)
	}
}

func TestPlacementAdviceDerivedEvidenceRevalidationReportsTelemetryRevisionDrift(t *testing.T) {
	request, input, saved, evaluatedAt := derivedPlacementRevalidationFixture(t)
	changed := input
	changed.Telemetry.Revision = "rv-telemetry-current"
	var err error
	changed.Evidence, err = DeriveNodeProjection(changed.Profile, changed.Telemetry, input.Evidence.EvaluatedAt)
	if err != nil {
		t.Fatal(err)
	}
	result, err := RevalidatePlacementAdviceDerivedEvidence(saved, request, []PlacementNodeDerivationInput{changed}, evaluatedAt)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != PlacementAdviceEvidenceStale ||
		result.Reason != "telemetry-revision-drift" ||
		result.StaleNodeID != input.Evidence.Projection.NodeID ||
		result.RecommendedAction != "refresh-telemetry-evidence" ||
		result.RecommendationReusable {
		t.Fatalf("telemetry drift was not explained fail-closed: %#v", result)
	}
}

func TestPlacementAdviceDerivedEvidenceRevalidationReportsHiddenDerivationDrift(t *testing.T) {
	request, input, saved, evaluatedAt := derivedPlacementRevalidationFixture(t)
	changed := input
	changed.Telemetry.Observations = append([]agent.CapacityObservation(nil), input.Telemetry.Observations...)
	changed.Telemetry.Observations[1].Value = 62
	var err error
	changed.Evidence, err = DeriveNodeProjection(changed.Profile, changed.Telemetry, input.Evidence.EvaluatedAt)
	if err != nil {
		t.Fatal(err)
	}
	if changed.Evidence.Projection != input.Evidence.Projection {
		t.Fatalf("fixture no longer isolates derivation-only drift")
	}
	result, err := RevalidatePlacementAdviceDerivedEvidence(saved, request, []PlacementNodeDerivationInput{changed}, evaluatedAt)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != PlacementAdviceEvidenceStale ||
		result.Reason != "projection-derivation-drift" ||
		result.StaleNodeID != input.Evidence.Projection.NodeID ||
		result.CurrentDerivationFingerprint == result.DerivationFingerprint ||
		result.RecommendationReusable {
		t.Fatalf("hidden derivation drift was not bound: %#v", result)
	}
}

func TestPlacementAdviceDerivedEvidenceRevalidationReportsFreshnessExpiry(t *testing.T) {
	request, input, saved, _ := derivedPlacementRevalidationFixture(t)
	result, err := RevalidatePlacementAdviceDerivedEvidence(
		saved,
		request,
		[]PlacementNodeDerivationInput{input},
		input.Evidence.EvaluatedAt.Add(16*time.Minute),
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != PlacementAdviceEvidenceStale ||
		result.Reason != "telemetry-freshness-expired" ||
		result.CurrentSnapshotID != "" || result.RecommendationReusable ||
		result.RecommendedAction != "refresh-telemetry-evidence" {
		t.Fatalf("expired telemetry was not fail-closed: %#v", result)
	}
}

func TestPlacementAdviceDerivedEvidenceRevalidationRejectsUnsafeOrTamperedEvidence(t *testing.T) {
	request, input, saved, evaluatedAt := derivedPlacementRevalidationFixture(t)
	unsafe := saved
	unsafe.ProductionMutation = true
	if _, err := RevalidatePlacementAdviceDerivedEvidence(unsafe, request, []PlacementNodeDerivationInput{input}, evaluatedAt); !errors.Is(err, ErrInvalidRecommendation) {
		t.Fatalf("unsafe saved evidence error = %v", err)
	}

	result, err := RevalidatePlacementAdviceDerivedEvidence(saved, request, []PlacementNodeDerivationInput{input}, evaluatedAt)
	if err != nil {
		t.Fatal(err)
	}
	result.ProductionMutation = true
	if err := ValidatePlacementAdviceDerivedEvidenceRevalidation(result); !errors.Is(err, ErrInvalidRecommendation) {
		t.Fatalf("unsafe revalidation error = %v", err)
	}
}
