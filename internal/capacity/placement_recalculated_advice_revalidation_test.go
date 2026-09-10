package capacity

import (
	"errors"
	"testing"
	"time"
)

func placementRecalculatedAdviceRevalidationFixture(t *testing.T) (
	PlacementRecalculatedAdviceSupersessionReceipt,
	PlacementRequest,
	PlacementAdviceDerivedEvidenceSnapshot,
	PlacementResourceHeadroomEnvelope,
	PlacementNodeDerivationInput,
	time.Time,
) {
	t.Helper()
	request, blockedReceipt, sourceGate, freshness, sourceEnvelope, satisfaction, current, recalculatedAt :=
		placementRecalculatedAdviceSupersessionFixture(t)
	supersession, err := BuildPlacementRecalculatedAdviceSupersessionReceipt(
		satisfaction,
		blockedReceipt,
		sourceGate,
		freshness,
		sourceEnvelope,
		request,
		[]PlacementNodeDerivationInput{current},
		recalculatedAt,
	)
	if err != nil {
		t.Fatal(err)
	}
	derived, err := CapturePlacementAdviceFromDerivations(
		request,
		[]PlacementNodeDerivationInput{current},
		recalculatedAt,
	)
	if err != nil {
		t.Fatal(err)
	}
	headroom, err := BuildPlacementResourceHeadroomEnvelope(
		request,
		derived,
		[]PlacementNodeDerivationInput{current},
		recalculatedAt,
	)
	if err != nil {
		t.Fatal(err)
	}
	return supersession, request, derived, headroom, current, recalculatedAt
}

func TestPlacementRecalculatedAdviceRevalidationCurrentIsAdvisoryOnly(t *testing.T) {
	supersession, request, derived, headroom, current, recalculatedAt :=
		placementRecalculatedAdviceRevalidationFixture(t)
	checkedAt := recalculatedAt.Add(30 * time.Minute)

	got, err := RevalidatePlacementRecalculatedAdviceSupersession(
		supersession,
		request,
		derived,
		headroom,
		[]PlacementNodeDerivationInput{current},
		checkedAt,
	)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != PlacementRecalculatedAdviceCurrent ||
		!got.FreshReuseDecisionEligible || got.RecommendedNodeID == "" ||
		got.SafetyScoreBand != PlacementSafetyHeadroom || got.EffectiveSafetyMarginPct <= 0 {
		t.Fatalf("expected current advice with positive safe headroom: %#v", got)
	}
	if got.SourceReuseAuthorized || got.ReuseAuthorized || got.PlacementAuthorized ||
		!got.AdvisoryOnly || got.ProductionMutation {
		t.Fatalf("revalidation granted unsafe authority: %#v", got)
	}
	if got.SupersessionID != supersession.SupersessionID ||
		got.DerivedSnapshotID != supersession.NewDerivedSnapshotID ||
		got.PlacementSnapshotID != supersession.NewPlacementSnapshotID ||
		got.AdviceID != supersession.NewAdviceID ||
		got.HeadroomEnvelopeID != supersession.NewHeadroomEnvelopeID {
		t.Fatalf("revalidation lineage mismatch: %#v", got)
	}
	if err := ValidatePlacementRecalculatedAdviceRevalidationGate(
		got,
		supersession,
		request,
		derived,
		headroom,
		[]PlacementNodeDerivationInput{current},
		checkedAt,
	); err != nil {
		t.Fatal(err)
	}
}

func TestPlacementRecalculatedAdviceRevalidationIsDeterministic(t *testing.T) {
	supersession, request, derived, headroom, current, recalculatedAt :=
		placementRecalculatedAdviceRevalidationFixture(t)
	checkedAt := recalculatedAt.Add(30 * time.Minute)
	first, err := RevalidatePlacementRecalculatedAdviceSupersession(
		supersession,
		request,
		derived,
		headroom,
		[]PlacementNodeDerivationInput{current},
		checkedAt,
	)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 100; i++ {
		next, err := RevalidatePlacementRecalculatedAdviceSupersession(
			supersession,
			request,
			derived,
			headroom,
			[]PlacementNodeDerivationInput{current},
			checkedAt,
		)
		if err != nil {
			t.Fatal(err)
		}
		if next != first {
			t.Fatalf("same exact inputs changed revalidation at iteration %d", i)
		}
	}
}

func TestPlacementRecalculatedAdviceRevalidationMarksExpiredEvidenceStale(t *testing.T) {
	supersession, request, derived, headroom, current, recalculatedAt :=
		placementRecalculatedAdviceRevalidationFixture(t)
	checkedAt := recalculatedAt.Add(2 * time.Hour)

	got, err := RevalidatePlacementRecalculatedAdviceSupersession(
		supersession,
		request,
		derived,
		headroom,
		[]PlacementNodeDerivationInput{current},
		checkedAt,
	)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != PlacementRecalculatedAdviceStale || got.FreshReuseDecisionEligible ||
		got.Reason != "recalculated-advice-evidence-stale" ||
		got.RecommendedAction != "collect-current-placement-evidence" {
		t.Fatalf("expected stale evidence-only disposition: %#v", got)
	}
}

func TestPlacementRecalculatedAdviceRevalidationBlocksNoSafePlacement(t *testing.T) {
	request, blockedReceipt, sourceGate, freshness, sourceEnvelope, satisfaction, current, recalculatedAt :=
		placementRecalculatedAdviceSupersessionFixture(t)
	request.IncrementalWorkload *= 1000000

	supersession, err := BuildPlacementRecalculatedAdviceSupersessionReceipt(
		satisfaction,
		blockedReceipt,
		sourceGate,
		freshness,
		sourceEnvelope,
		request,
		[]PlacementNodeDerivationInput{current},
		recalculatedAt,
	)
	if err != nil {
		t.Fatal(err)
	}
	derived, err := CapturePlacementAdviceFromDerivations(
		request,
		[]PlacementNodeDerivationInput{current},
		recalculatedAt,
	)
	if err != nil {
		t.Fatal(err)
	}
	headroom, err := BuildPlacementResourceHeadroomEnvelope(
		request,
		derived,
		[]PlacementNodeDerivationInput{current},
		recalculatedAt,
	)
	if err != nil {
		t.Fatal(err)
	}
	if supersession.Action == ActionNone || supersession.RecommendedNodeID != "" {
		t.Fatalf("fixture unexpectedly produced a safe placement: %#v", supersession)
	}

	got, err := RevalidatePlacementRecalculatedAdviceSupersession(
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
	if got.Status != PlacementRecalculatedAdviceBlocked || got.FreshReuseDecisionEligible ||
		got.Reason != "recalculated-advice-has-no-safe-placement" {
		t.Fatalf("expected capacity/evidence block: %#v", got)
	}
}

func TestPlacementRecalculatedAdviceRevalidationRejectsLineageDrift(t *testing.T) {
	supersession, request, derived, headroom, current, recalculatedAt :=
		placementRecalculatedAdviceRevalidationFixture(t)
	tampered := headroom
	tampered.AdviceID = "pa-000000000000000000000000"

	_, err := RevalidatePlacementRecalculatedAdviceSupersession(
		supersession,
		request,
		derived,
		tampered,
		[]PlacementNodeDerivationInput{current},
		recalculatedAt.Add(30*time.Minute),
	)
	if !errors.Is(err, ErrInvalidRecommendation) {
		t.Fatalf("lineage drift error = %v", err)
	}
}

func TestPlacementRecalculatedAdviceRevalidationRejectsSupersessionAuthorityTampering(t *testing.T) {
	supersession, request, derived, headroom, current, recalculatedAt :=
		placementRecalculatedAdviceRevalidationFixture(t)
	tampered := supersession
	tampered.ReuseAuthorized = true

	_, err := RevalidatePlacementRecalculatedAdviceSupersession(
		tampered,
		request,
		derived,
		headroom,
		[]PlacementNodeDerivationInput{current},
		recalculatedAt.Add(30*time.Minute),
	)
	if !errors.Is(err, ErrInvalidRecommendation) {
		t.Fatalf("unsafe supersession error = %v", err)
	}
}

func TestPlacementRecalculatedAdviceRevalidationRejectsVerdictTampering(t *testing.T) {
	supersession, request, derived, headroom, current, recalculatedAt :=
		placementRecalculatedAdviceRevalidationFixture(t)
	checkedAt := recalculatedAt.Add(30 * time.Minute)
	got, err := RevalidatePlacementRecalculatedAdviceSupersession(
		supersession,
		request,
		derived,
		headroom,
		[]PlacementNodeDerivationInput{current},
		checkedAt,
	)
	if err != nil {
		t.Fatal(err)
	}
	tampered := got
	tampered.ReuseAuthorized = true
	if err := ValidatePlacementRecalculatedAdviceRevalidationGate(
		tampered,
		supersession,
		request,
		derived,
		headroom,
		[]PlacementNodeDerivationInput{current},
		checkedAt,
	); !errors.Is(err, ErrInvalidRecommendation) {
		t.Fatalf("verdict tamper error = %v", err)
	}
}
