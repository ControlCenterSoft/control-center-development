package capacity

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestPlacementRebuildEvidenceReceiptExplainsStaleConstraints(t *testing.T) {
	request, derived, input, envelope, _, decision, _ := placementResourceReuseGateFixture(t)
	checkedAt := contractNow.Add(59*time.Minute + time.Second)
	freshness, err := BuildPlacementResourceHeadroomFreshnessGate(
		envelope,
		request,
		derived,
		[]PlacementNodeDerivationInput{input},
		checkedAt,
	)
	if err != nil {
		t.Fatal(err)
	}
	gate, err := EvaluatePlacementResourceReuseGate(
		decision,
		freshness,
		envelope,
		request,
		derived,
		[]PlacementNodeDerivationInput{input},
		checkedAt,
	)
	if err != nil {
		t.Fatal(err)
	}
	if gate.BlockedBy != PlacementResourceReuseBlockResource {
		t.Fatalf("fixture did not produce resource block: %#v", gate)
	}

	receipt, err := BuildPlacementRebuildEvidenceReceipt(gate, freshness, envelope)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.SchemaVersion != PlacementRebuildEvidenceReceiptSchemaV1 || receipt.ReceiptID == "" ||
		!receipt.RebuildRequired || receipt.ReuseAuthorized || receipt.RebuildAuthorized ||
		receipt.PlacementAuthorized || !receipt.AdvisoryOnly || receipt.ProductionMutation {
		t.Fatalf("unsafe rebuild receipt: %#v", receipt)
	}
	if len(receipt.Requirements) == 0 {
		t.Fatal("stale resource block produced no evidence requirements")
	}
	for _, requirement := range receipt.Requirements {
		if requirement.RequiredEvidence != PlacementRebuildEvidenceCurrentTelemetry ||
			requirement.Reason != "constraint-observation-stale" {
			t.Fatalf("stale constraint was not explained: %#v", requirement)
		}
	}
	if err := ValidatePlacementRebuildEvidenceReceipt(receipt, gate, freshness, envelope); err != nil {
		t.Fatal(err)
	}
}

func TestPlacementRebuildEvidenceReceiptExplainsLineageBlock(t *testing.T) {
	request, derived, input, envelope, freshness, decision, checkedAt :=
		placementResourceReuseGateFixture(t)
	decision.SnapshotID = "pade-" + strings.Repeat("f", 24)
	decision.CurrentSnapshotID = decision.SnapshotID
	decision.DecisionID = placementAdviceReuseDecisionID(decision)
	if err := ValidatePlacementAdviceReuseDecision(decision); err != nil {
		t.Fatal(err)
	}
	gate, err := EvaluatePlacementResourceReuseGate(
		decision,
		freshness,
		envelope,
		request,
		derived,
		[]PlacementNodeDerivationInput{input},
		checkedAt,
	)
	if err != nil {
		t.Fatal(err)
	}
	if gate.BlockedBy != PlacementResourceReuseBlockLineage {
		t.Fatalf("fixture did not produce lineage block: %#v", gate)
	}

	receipt, err := BuildPlacementRebuildEvidenceReceipt(gate, freshness, envelope)
	if err != nil {
		t.Fatal(err)
	}
	if len(receipt.Requirements) != len(envelope.Candidates[0].Resources) {
		t.Fatalf("lineage receipt did not require full resource revalidation: %#v", receipt)
	}
	for _, requirement := range receipt.Requirements {
		if requirement.RequiredEvidence != PlacementRebuildEvidenceLineage ||
			requirement.Reason != "lineage-revalidation-required" {
			t.Fatalf("lineage requirement is not explicit: %#v", requirement)
		}
	}
}

func TestPlacementRebuildEvidenceReceiptRejectsReusableGate(t *testing.T) {
	request, derived, input, envelope, freshness, decision, checkedAt :=
		placementResourceReuseGateFixture(t)
	gate, err := EvaluatePlacementResourceReuseGate(
		decision,
		freshness,
		envelope,
		request,
		derived,
		[]PlacementNodeDerivationInput{input},
		checkedAt,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := BuildPlacementRebuildEvidenceReceipt(
		gate, freshness, envelope,
	); !errors.Is(err, ErrInvalidRecommendation) {
		t.Fatalf("allowed reuse gate error = %v", err)
	}
}

func TestPlacementRebuildEvidenceReceiptIsDeterministicAndTamperEvident(t *testing.T) {
	request, derived, input, envelope, _, decision, _ := placementResourceReuseGateFixture(t)
	checkedAt := contractNow.Add(59*time.Minute + time.Second)
	freshness, err := BuildPlacementResourceHeadroomFreshnessGate(
		envelope,
		request,
		derived,
		[]PlacementNodeDerivationInput{input},
		checkedAt,
	)
	if err != nil {
		t.Fatal(err)
	}
	gate, err := EvaluatePlacementResourceReuseGate(
		decision,
		freshness,
		envelope,
		request,
		derived,
		[]PlacementNodeDerivationInput{input},
		checkedAt,
	)
	if err != nil {
		t.Fatal(err)
	}
	first, err := BuildPlacementRebuildEvidenceReceipt(gate, freshness, envelope)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 100; i++ {
		next, err := BuildPlacementRebuildEvidenceReceipt(gate, freshness, envelope)
		if err != nil {
			t.Fatal(err)
		}
		if next.ReceiptID != first.ReceiptID || len(next.Requirements) != len(first.Requirements) {
			t.Fatalf("same inputs changed receipt at iteration %d: %#v %#v", i, first, next)
		}
	}

	tampered := first
	tampered.PlacementAuthorized = true
	if err := ValidatePlacementRebuildEvidenceReceipt(
		tampered, gate, freshness, envelope,
	); !errors.Is(err, ErrInvalidRecommendation) {
		t.Fatalf("placement authority tamper error = %v", err)
	}

	tampered = first
	tampered.Requirements = append([]PlacementRebuildEvidenceRequirement(nil), first.Requirements...)
	tampered.Requirements[0].RequiredEvidence = PlacementRebuildEvidenceLineage
	if err := ValidatePlacementRebuildEvidenceReceipt(
		tampered, gate, freshness, envelope,
	); !errors.Is(err, ErrInvalidRecommendation) {
		t.Fatalf("requirement tamper error = %v", err)
	}
}

func TestPlacementRebuildEvidenceReceiptRequiresExactFreshnessLineage(t *testing.T) {
	request, derived, input, envelope, _, decision, _ := placementResourceReuseGateFixture(t)
	checkedAt := contractNow.Add(59*time.Minute + time.Second)
	freshness, err := BuildPlacementResourceHeadroomFreshnessGate(
		envelope,
		request,
		derived,
		[]PlacementNodeDerivationInput{input},
		checkedAt,
	)
	if err != nil {
		t.Fatal(err)
	}
	gate, err := EvaluatePlacementResourceReuseGate(
		decision,
		freshness,
		envelope,
		request,
		derived,
		[]PlacementNodeDerivationInput{input},
		checkedAt,
	)
	if err != nil {
		t.Fatal(err)
	}

	tamperedFreshness := freshness
	tamperedFreshness.ScopeID = "other-scope"
	tamperedFreshness.GateID = placementResourceHeadroomFreshnessGateID(tamperedFreshness)
	if _, err := BuildPlacementRebuildEvidenceReceipt(
		gate, tamperedFreshness, envelope,
	); !errors.Is(err, ErrInvalidRecommendation) {
		t.Fatalf("cross-lineage freshness error = %v", err)
	}
}
