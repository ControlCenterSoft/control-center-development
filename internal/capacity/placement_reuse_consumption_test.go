package capacity

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestPlacementAdviceReuseConsumptionAllowsOnlyExactCurrentLineage(t *testing.T) {
	current := placementReuseDecisionFixture(t)
	decision, err := EvaluatePlacementAdviceReuse(current)
	if err != nil {
		t.Fatal(err)
	}

	consumption, err := EvaluatePlacementAdviceReuseConsumption(decision, current)
	if err != nil {
		t.Fatal(err)
	}
	if consumption.Status != PlacementAdviceReuseAllowed || !consumption.ReuseAllowed ||
		consumption.DecisionID != decision.DecisionID ||
		consumption.DecisionRevalidationID != current.RevalidationID ||
		consumption.CurrentRevalidationID != current.RevalidationID ||
		consumption.Reason != "exact-current-reuse-decision-consumable" ||
		consumption.RecommendedAction != "none" || !consumption.AdvisoryOnly ||
		consumption.PlacementAuthorized || consumption.ProductionMutation {
		t.Fatalf("exact current decision was not safely consumable: %#v", consumption)
	}
	if err := ValidatePlacementAdviceReuseConsumption(consumption); err != nil {
		t.Fatal(err)
	}
}

func TestPlacementAdviceReuseConsumptionIsDeterministic(t *testing.T) {
	current := placementReuseDecisionFixture(t)
	decision, err := EvaluatePlacementAdviceReuse(current)
	if err != nil {
		t.Fatal(err)
	}
	first, err := EvaluatePlacementAdviceReuseConsumption(decision, current)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 100; i++ {
		next, err := EvaluatePlacementAdviceReuseConsumption(decision, current)
		if err != nil {
			t.Fatal(err)
		}
		if next != first {
			t.Fatalf("same inputs changed consumption at iteration %d: %#v %#v", i, first, next)
		}
	}
}

func TestPlacementAdviceReuseConsumptionBlocksConstraintDrift(t *testing.T) {
	request, input, saved, evaluatedAt := derivedPlacementRevalidationFixture(t)
	original, err := RevalidatePlacementAdviceDerivedEvidence(
		saved,
		request,
		[]PlacementNodeDerivationInput{input},
		evaluatedAt,
	)
	if err != nil {
		t.Fatal(err)
	}
	decision, err := EvaluatePlacementAdviceReuse(original)
	if err != nil {
		t.Fatal(err)
	}

	request.MinimumNodeReservePercent++
	current, err := RevalidatePlacementAdviceDerivedEvidence(
		saved,
		request,
		[]PlacementNodeDerivationInput{input},
		evaluatedAt,
	)
	if err != nil {
		t.Fatal(err)
	}
	consumption, err := EvaluatePlacementAdviceReuseConsumption(decision, current)
	if err != nil {
		t.Fatal(err)
	}
	if consumption.Status != PlacementAdviceReuseBlocked || consumption.ReuseAllowed ||
		consumption.Reason != "placement-request-or-constraint-drift" ||
		consumption.RecommendedAction != "rebuild-placement-advice-from-current-evidence" {
		t.Fatalf("constraint drift was not blocked at consumption: %#v", consumption)
	}
}

func TestPlacementAdviceReuseConsumptionBlocksFreshnessExpiry(t *testing.T) {
	request, input, saved, evaluatedAt := derivedPlacementRevalidationFixture(t)
	original, err := RevalidatePlacementAdviceDerivedEvidence(
		saved,
		request,
		[]PlacementNodeDerivationInput{input},
		evaluatedAt,
	)
	if err != nil {
		t.Fatal(err)
	}
	decision, err := EvaluatePlacementAdviceReuse(original)
	if err != nil {
		t.Fatal(err)
	}

	current, err := RevalidatePlacementAdviceDerivedEvidence(
		saved,
		request,
		[]PlacementNodeDerivationInput{input},
		input.Evidence.EvaluatedAt.Add(16*time.Minute),
	)
	if err != nil {
		t.Fatal(err)
	}
	consumption, err := EvaluatePlacementAdviceReuseConsumption(decision, current)
	if err != nil {
		t.Fatal(err)
	}
	if consumption.Status != PlacementAdviceReuseBlocked || consumption.ReuseAllowed ||
		consumption.Reason != "telemetry-freshness-expired" ||
		consumption.RecommendedAction != "refresh-telemetry-evidence" {
		t.Fatalf("expired telemetry remained consumable: %#v", consumption)
	}
}

func TestPlacementAdviceReuseConsumptionBlocksCrossLineageDecision(t *testing.T) {
	current := placementReuseDecisionFixture(t)
	decision, err := EvaluatePlacementAdviceReuse(current)
	if err != nil {
		t.Fatal(err)
	}
	decision.RevalidationID = "padr-" + strings.Repeat("f", 24)
	decision.DecisionID = placementAdviceReuseDecisionID(decision)
	if err := ValidatePlacementAdviceReuseDecision(decision); err != nil {
		t.Fatal(err)
	}

	consumption, err := EvaluatePlacementAdviceReuseConsumption(decision, current)
	if err != nil {
		t.Fatal(err)
	}
	if consumption.Status != PlacementAdviceReuseBlocked || consumption.ReuseAllowed ||
		consumption.Reason != "placement-reuse-lineage-drift" ||
		consumption.RecommendedAction != "rebuild-placement-advice-from-current-evidence" {
		t.Fatalf("cross-lineage decision remained consumable: %#v", consumption)
	}
}

func TestPlacementAdviceReuseConsumptionKeepsBlockedDecisionBlocked(t *testing.T) {
	current := placementReuseDecisionFixture(t)
	blockedEvidence := current
	blockedEvidence.RecommendationReusable = false
	blockedEvidence.RevalidationID = placementAdviceDerivedEvidenceRevalidationID(blockedEvidence)
	if err := ValidatePlacementAdviceDerivedEvidenceRevalidation(blockedEvidence); err != nil {
		t.Fatal(err)
	}
	decision, err := EvaluatePlacementAdviceReuse(blockedEvidence)
	if err != nil {
		t.Fatal(err)
	}

	consumption, err := EvaluatePlacementAdviceReuseConsumption(decision, current)
	if err != nil {
		t.Fatal(err)
	}
	if consumption.Status != PlacementAdviceReuseBlocked || consumption.ReuseAllowed ||
		consumption.Reason != "reuse-decision-not-allowed" ||
		consumption.RecommendedAction != "rebuild-placement-advice-from-current-evidence" {
		t.Fatalf("blocked decision escaped consumption gate: %#v", consumption)
	}
}

func TestPlacementAdviceReuseConsumptionRejectsUnsafeOrTamperedEvidence(t *testing.T) {
	current := placementReuseDecisionFixture(t)
	decision, err := EvaluatePlacementAdviceReuse(current)
	if err != nil {
		t.Fatal(err)
	}

	unsafeCurrent := current
	unsafeCurrent.ProductionMutation = true
	if _, err := EvaluatePlacementAdviceReuseConsumption(decision, unsafeCurrent); !errors.Is(err, ErrInvalidRecommendation) {
		t.Fatalf("unsafe current evidence error = %v", err)
	}

	consumption, err := EvaluatePlacementAdviceReuseConsumption(decision, current)
	if err != nil {
		t.Fatal(err)
	}
	consumption.PlacementAuthorized = true
	if err := ValidatePlacementAdviceReuseConsumption(consumption); !errors.Is(err, ErrInvalidRecommendation) {
		t.Fatalf("placement authority tamper error = %v", err)
	}

	consumption, err = EvaluatePlacementAdviceReuseConsumption(decision, current)
	if err != nil {
		t.Fatal(err)
	}
	consumption.CurrentRevalidationID = "padr-" + strings.Repeat("0", 24)
	consumption.ConsumptionID = placementAdviceReuseConsumptionID(consumption)
	if err := ValidatePlacementAdviceReuseConsumption(consumption); !errors.Is(err, ErrInvalidRecommendation) {
		t.Fatalf("current lineage tamper error = %v", err)
	}
}
