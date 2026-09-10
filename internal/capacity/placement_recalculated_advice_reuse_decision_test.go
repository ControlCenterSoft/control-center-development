package capacity

import (
	"errors"
	"testing"
	"time"
)

func placementRecalculatedAdviceReuseDecisionFixture(t *testing.T) (
	PlacementRecalculatedAdviceRevalidationGate,
	PlacementRecalculatedAdviceSupersessionReceipt,
	PlacementRequest,
	PlacementAdviceDerivedEvidenceSnapshot,
	PlacementResourceHeadroomEnvelope,
	PlacementNodeDerivationInput,
) {
	t.Helper()
	supersession, request, derived, headroom, current, recalculatedAt :=
		placementRecalculatedAdviceRevalidationFixture(t)
	gate, err := RevalidatePlacementRecalculatedAdviceSupersession(
		supersession,
		request,
		derived,
		headroom,
		[]PlacementNodeDerivationInput{current},
		recalculatedAt.Add(30*time.Minute),
	)
	if err != nil {
		t.Fatal(err)
	}
	return gate, supersession, request, derived, headroom, current
}

func TestPlacementRecalculatedAdviceReuseDecisionAllowsFreshAdvisoryOnly(t *testing.T) {
	gate, supersession, request, derived, headroom, current :=
		placementRecalculatedAdviceReuseDecisionFixture(t)
	got, err := BuildPlacementRecalculatedAdviceReuseDecision(
		gate, supersession, request, derived, headroom, []PlacementNodeDerivationInput{current},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !got.FreshAdvisoryReuseAllowed || got.SourceReuseAuthorized ||
		got.PlacementAuthorized || !got.AdvisoryOnly || got.ProductionMutation {
		t.Fatalf("decision expanded authority: %#v", got)
	}
	if got.RevalidationID != gate.RevalidationID || got.SupersessionID != supersession.SupersessionID ||
		got.DerivedSnapshotID != supersession.NewDerivedSnapshotID ||
		got.PlacementSnapshotID != supersession.NewPlacementSnapshotID ||
		got.AdviceID != supersession.NewAdviceID ||
		got.SourceDerivedSnapshotID != supersession.SourceDerivedSnapshotID ||
		got.SourcePlacementSnapshotID != supersession.SourcePlacementSnapshotID ||
		got.SourceAdviceID != supersession.SourceAdviceID {
		t.Fatalf("decision lineage mismatch: %#v", got)
	}
	if got.SourceDerivedSnapshotID == got.DerivedSnapshotID ||
		got.SafetyScoreBand != PlacementSafetyHeadroom || got.EffectiveSafetyMarginPct <= 0 {
		t.Fatalf("decision lost fresh safety headroom: %#v", got)
	}
	if err := ValidatePlacementRecalculatedAdviceReuseDecision(
		got, gate, supersession, request, derived, headroom, []PlacementNodeDerivationInput{current},
	); err != nil {
		t.Fatal(err)
	}
}

func TestPlacementRecalculatedAdviceReuseDecisionIsDeterministic(t *testing.T) {
	gate, supersession, request, derived, headroom, current :=
		placementRecalculatedAdviceReuseDecisionFixture(t)
	first, err := BuildPlacementRecalculatedAdviceReuseDecision(
		gate, supersession, request, derived, headroom, []PlacementNodeDerivationInput{current},
	)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 100; i++ {
		next, err := BuildPlacementRecalculatedAdviceReuseDecision(
			gate, supersession, request, derived, headroom, []PlacementNodeDerivationInput{current},
		)
		if err != nil {
			t.Fatal(err)
		}
		if next != first {
			t.Fatalf("same exact inputs changed decision at iteration %d", i)
		}
	}
}

func TestPlacementRecalculatedAdviceReuseDecisionRejectsStaleGate(t *testing.T) {
	supersession, request, derived, headroom, current, recalculatedAt :=
		placementRecalculatedAdviceRevalidationFixture(t)
	gate, err := RevalidatePlacementRecalculatedAdviceSupersession(
		supersession, request, derived, headroom, []PlacementNodeDerivationInput{current},
		recalculatedAt.Add(2*time.Hour),
	)
	if err != nil {
		t.Fatal(err)
	}
	_, err = BuildPlacementRecalculatedAdviceReuseDecision(
		gate, supersession, request, derived, headroom, []PlacementNodeDerivationInput{current},
	)
	if gate.Status != PlacementRecalculatedAdviceStale ||
		!errors.Is(err, ErrInvalidRecommendation) {
		t.Fatalf("stale gate was accepted: gate=%#v err=%v", gate, err)
	}
}

func TestPlacementRecalculatedAdviceReuseDecisionRejectsBlockedGate(t *testing.T) {
	request, blockedReceipt, sourceGate, freshness, sourceEnvelope, satisfaction, current, recalculatedAt :=
		placementRecalculatedAdviceSupersessionFixture(t)
	request.IncrementalWorkload *= 1000000
	supersession, err := BuildPlacementRecalculatedAdviceSupersessionReceipt(
		satisfaction, blockedReceipt, sourceGate, freshness, sourceEnvelope,
		request, []PlacementNodeDerivationInput{current}, recalculatedAt,
	)
	if err != nil {
		t.Fatal(err)
	}
	derived, err := CapturePlacementAdviceFromDerivations(
		request, []PlacementNodeDerivationInput{current}, recalculatedAt,
	)
	if err != nil {
		t.Fatal(err)
	}
	headroom, err := BuildPlacementResourceHeadroomEnvelope(
		request, derived, []PlacementNodeDerivationInput{current}, recalculatedAt,
	)
	if err != nil {
		t.Fatal(err)
	}
	gate, err := RevalidatePlacementRecalculatedAdviceSupersession(
		supersession, request, derived, headroom, []PlacementNodeDerivationInput{current},
		recalculatedAt.Add(30*time.Minute),
	)
	if err != nil {
		t.Fatal(err)
	}
	_, err = BuildPlacementRecalculatedAdviceReuseDecision(
		gate, supersession, request, derived, headroom, []PlacementNodeDerivationInput{current},
	)
	if gate.Status != PlacementRecalculatedAdviceBlocked ||
		!errors.Is(err, ErrInvalidRecommendation) {
		t.Fatalf("blocked gate was accepted: gate=%#v err=%v", gate, err)
	}
}

func TestPlacementRecalculatedAdviceReuseDecisionRejectsGateAuthorityTampering(t *testing.T) {
	gate, supersession, request, derived, headroom, current :=
		placementRecalculatedAdviceReuseDecisionFixture(t)
	gate.SourceReuseAuthorized = true
	gate.RevalidationID = placementRecalculatedAdviceRevalidationID(gate)
	_, err := BuildPlacementRecalculatedAdviceReuseDecision(
		gate, supersession, request, derived, headroom, []PlacementNodeDerivationInput{current},
	)
	if !errors.Is(err, ErrInvalidRecommendation) {
		t.Fatalf("tampered gate error = %v", err)
	}
}

func TestPlacementRecalculatedAdviceReuseDecisionRejectsAuthorityExpansion(t *testing.T) {
	gate, supersession, request, derived, headroom, current :=
		placementRecalculatedAdviceReuseDecisionFixture(t)
	got, err := BuildPlacementRecalculatedAdviceReuseDecision(
		gate, supersession, request, derived, headroom, []PlacementNodeDerivationInput{current},
	)
	if err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*PlacementRecalculatedAdviceReuseDecision){
		"source reuse": func(v *PlacementRecalculatedAdviceReuseDecision) { v.SourceReuseAuthorized = true },
		"placement":    func(v *PlacementRecalculatedAdviceReuseDecision) { v.PlacementAuthorized = true },
		"production":   func(v *PlacementRecalculatedAdviceReuseDecision) { v.ProductionMutation = true },
	} {
		t.Run(name, func(t *testing.T) {
			tampered := got
			mutate(&tampered)
			tampered.DecisionID = placementRecalculatedAdviceReuseDecisionID(tampered)
			if err := validatePlacementRecalculatedAdviceReuseDecision(tampered);
				!errors.Is(err, ErrInvalidRecommendation) {
				t.Fatalf("authority expansion error = %v", err)
			}
		})
	}
}

func TestPlacementRecalculatedAdviceReuseDecisionRejectsSafetyOrSourceIdentityLoss(t *testing.T) {
	gate, supersession, request, derived, headroom, current :=
		placementRecalculatedAdviceReuseDecisionFixture(t)
	got, err := BuildPlacementRecalculatedAdviceReuseDecision(
		gate, supersession, request, derived, headroom, []PlacementNodeDerivationInput{current},
	)
	if err != nil {
		t.Fatal(err)
	}
	marginLost := got
	marginLost.EffectiveSafetyMarginPct = 0
	marginLost.DecisionID = placementRecalculatedAdviceReuseDecisionID(marginLost)
	if err := validatePlacementRecalculatedAdviceReuseDecision(marginLost);
		!errors.Is(err, ErrInvalidRecommendation) {
		t.Fatalf("lost safety margin error = %v", err)
	}
	sourceReused := got
	sourceReused.SourceDerivedSnapshotID = sourceReused.DerivedSnapshotID
	sourceReused.DecisionID = placementRecalculatedAdviceReuseDecisionID(sourceReused)
	if err := validatePlacementRecalculatedAdviceReuseDecision(sourceReused);
		!errors.Is(err, ErrInvalidRecommendation) {
		t.Fatalf("source identity reuse error = %v", err)
	}
}
