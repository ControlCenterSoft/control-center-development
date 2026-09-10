package capacity

import (
	"errors"
	"testing"
	"time"
)

func placementReuseDecisionFixture(t *testing.T) PlacementAdviceDerivedEvidenceRevalidation {
	t.Helper()
	request, input, saved, evaluatedAt := derivedPlacementRevalidationFixture(t)
	revalidation, err := RevalidatePlacementAdviceDerivedEvidence(
		saved,
		request,
		[]PlacementNodeDerivationInput{input},
		evaluatedAt,
	)
	if err != nil {
		t.Fatal(err)
	}
	return revalidation
}

func TestPlacementAdviceReuseDecisionAllowsExactCurrentReusableEvidence(t *testing.T) {
	revalidation := placementReuseDecisionFixture(t)
	if !revalidation.RecommendationReusable {
		t.Fatalf("fixture no longer produces reusable exact evidence: %#v", revalidation)
	}
	decision, err := EvaluatePlacementAdviceReuse(revalidation)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Status != PlacementAdviceReuseAllowed || !decision.ReuseAllowed ||
		decision.RevalidationID != revalidation.RevalidationID ||
		decision.SnapshotID != revalidation.SnapshotID ||
		decision.CurrentSnapshotID != revalidation.SnapshotID ||
		decision.Reason != "exact-current-derived-evidence-reusable" ||
		decision.RecommendedAction != "none" || !decision.AdvisoryOnly ||
		decision.PlacementAuthorized || decision.ProductionMutation {
		t.Fatalf("exact current evidence was not bounded for reuse: %#v", decision)
	}
	if err := ValidatePlacementAdviceReuseDecision(decision); err != nil {
		t.Fatal(err)
	}
}

func TestPlacementAdviceReuseDecisionIsDeterministic(t *testing.T) {
	revalidation := placementReuseDecisionFixture(t)
	first, err := EvaluatePlacementAdviceReuse(revalidation)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 100; i++ {
		next, err := EvaluatePlacementAdviceReuse(revalidation)
		if err != nil {
			t.Fatal(err)
		}
		if next != first {
			t.Fatalf("same revalidation changed reuse decision at iteration %d: %#v %#v", i, first, next)
		}
	}
}

func TestPlacementAdviceReuseDecisionBlocksRequestConstraintDrift(t *testing.T) {
	request, input, saved, evaluatedAt := derivedPlacementRevalidationFixture(t)
	request.MinimumNodeReservePercent++
	revalidation, err := RevalidatePlacementAdviceDerivedEvidence(
		saved,
		request,
		[]PlacementNodeDerivationInput{input},
		evaluatedAt,
	)
	if err != nil {
		t.Fatal(err)
	}
	decision, err := EvaluatePlacementAdviceReuse(revalidation)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Status != PlacementAdviceReuseBlocked || decision.ReuseAllowed ||
		decision.Reason != "placement-request-or-constraint-drift" ||
		decision.RecommendedAction != "rebuild-placement-advice-from-current-evidence" ||
		decision.PlacementAuthorized || decision.ProductionMutation {
		t.Fatalf("constraint drift was not fail-closed: %#v", decision)
	}
}

func TestPlacementAdviceReuseDecisionPreservesFreshnessRecoveryAction(t *testing.T) {
	request, input, saved, _ := derivedPlacementRevalidationFixture(t)
	revalidation, err := RevalidatePlacementAdviceDerivedEvidence(
		saved,
		request,
		[]PlacementNodeDerivationInput{input},
		input.Evidence.EvaluatedAt.Add(16*time.Minute),
	)
	if err != nil {
		t.Fatal(err)
	}
	decision, err := EvaluatePlacementAdviceReuse(revalidation)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Status != PlacementAdviceReuseBlocked || decision.ReuseAllowed ||
		decision.CurrentSnapshotID != "" ||
		decision.Reason != "telemetry-freshness-expired" ||
		decision.RecommendedAction != "refresh-telemetry-evidence" {
		t.Fatalf("freshness recovery action was not preserved: %#v", decision)
	}
}

func TestPlacementAdviceReuseDecisionBlocksCurrentButNonReusableEvidence(t *testing.T) {
	revalidation := placementReuseDecisionFixture(t)
	revalidation.RecommendationReusable = false
	revalidation.RevalidationID = placementAdviceDerivedEvidenceRevalidationID(revalidation)
	if err := ValidatePlacementAdviceDerivedEvidenceRevalidation(revalidation); err != nil {
		t.Fatal(err)
	}
	decision, err := EvaluatePlacementAdviceReuse(revalidation)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Status != PlacementAdviceReuseBlocked || decision.ReuseAllowed ||
		decision.Reason != "current-derived-evidence-not-reusable" ||
		decision.RecommendedAction != "rebuild-placement-advice-from-current-evidence" {
		t.Fatalf("non-reusable current evidence escaped consumer gate: %#v", decision)
	}
}

func TestPlacementAdviceReuseDecisionRejectsCurrentLineageDrift(t *testing.T) {
	revalidation := placementReuseDecisionFixture(t)
	decision, err := EvaluatePlacementAdviceReuse(revalidation)
	if err != nil {
		t.Fatal(err)
	}
	decision.CurrentDerivationFingerprint = "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
	decision.DecisionID = placementAdviceReuseDecisionID(decision)
	if err := ValidatePlacementAdviceReuseDecision(decision); !errors.Is(err, ErrInvalidRecommendation) {
		t.Fatalf("current lineage drift error = %v", err)
	}
}

func TestPlacementAdviceReuseDecisionRejectsTamperedInputAndAuthority(t *testing.T) {
	revalidation := placementReuseDecisionFixture(t)
	unsafe := revalidation
	unsafe.ProductionMutation = true
	if _, err := EvaluatePlacementAdviceReuse(unsafe); !errors.Is(err, ErrInvalidRecommendation) {
		t.Fatalf("unsafe revalidation error = %v", err)
	}

	decision, err := EvaluatePlacementAdviceReuse(revalidation)
	if err != nil {
		t.Fatal(err)
	}
	decision.PlacementAuthorized = true
	if err := ValidatePlacementAdviceReuseDecision(decision); !errors.Is(err, ErrInvalidRecommendation) {
		t.Fatalf("placement authority tamper error = %v", err)
	}
}
