package capacity

import (
	"errors"
	"testing"
	"time"

	"control-center/internal/agent"
)

func placementRecalculatedAdviceSupersessionFixture(t *testing.T) (
	PlacementRequest,
	PlacementRebuildEvidenceReceipt,
	PlacementResourceReuseGate,
	PlacementResourceHeadroomFreshnessGate,
	PlacementResourceHeadroomEnvelope,
	PlacementRebuildSatisfactionGate,
	PlacementNodeDerivationInput,
	time.Time,
) {
	t.Helper()
	request, derived, input, envelope, _, decision, _ := placementResourceReuseGateFixture(t)
	blockedAt := contractNow.Add(59*time.Minute + time.Second)
	freshness, err := BuildPlacementResourceHeadroomFreshnessGate(
		envelope,
		request,
		derived,
		[]PlacementNodeDerivationInput{input},
		blockedAt,
	)
	if err != nil {
		t.Fatal(err)
	}
	sourceGate, err := EvaluatePlacementResourceReuseGate(
		decision,
		freshness,
		envelope,
		request,
		derived,
		[]PlacementNodeDerivationInput{input},
		blockedAt,
	)
	if err != nil {
		t.Fatal(err)
	}
	blockedReceipt, err := BuildPlacementRebuildEvidenceReceipt(sourceGate, freshness, envelope)
	if err != nil {
		t.Fatal(err)
	}

	freshAt := blockedAt.Add(time.Minute)
	current := input
	current.Telemetry.SnapshotID = "snapshot-node-a-supersession"
	current.Telemetry.Revision = "revision-node-a-supersession"
	current.Telemetry.ObservedAt = freshAt
	current.Telemetry.Observations = append(
		[]agent.CapacityObservation(nil),
		input.Telemetry.Observations...,
	)
	for index := range current.Telemetry.Observations {
		current.Telemetry.Observations[index].ObservedAt = freshAt
		current.Telemetry.Observations[index].Evidence = agent.EvidenceMeasured
	}
	current.Evidence, err = DeriveNodeProjection(current.Profile, current.Telemetry, freshAt)
	if err != nil {
		t.Fatal(err)
	}
	satisfaction, err := BuildPlacementRebuildSatisfactionGate(
		blockedReceipt,
		sourceGate,
		freshness,
		envelope,
		[]PlacementNodeDerivationInput{current},
		freshAt,
	)
	if err != nil {
		t.Fatal(err)
	}
	return request, blockedReceipt, sourceGate, freshness, envelope, satisfaction, current, freshAt
}

func TestPlacementRecalculatedAdviceSupersessionSealsFreshAdvisoryLineage(t *testing.T) {
	request, blockedReceipt, sourceGate, freshness, envelope, gate, current, recalculatedAt :=
		placementRecalculatedAdviceSupersessionFixture(t)

	got, err := BuildPlacementRecalculatedAdviceSupersessionReceipt(
		gate,
		blockedReceipt,
		sourceGate,
		freshness,
		envelope,
		request,
		[]PlacementNodeDerivationInput{current},
		recalculatedAt,
	)
	if err != nil {
		t.Fatal(err)
	}
	if got.SchemaVersion != PlacementRecalculatedAdviceSupersessionSchemaV1 ||
		got.SupersessionID == "" || !got.SupersedesSource || !got.FreshEvidenceRecalculated ||
		got.SourceReuseAuthorized || got.ReuseAuthorized || got.PlacementAuthorized ||
		!got.AdvisoryOnly || got.ProductionMutation {
		t.Fatalf("unsafe or incomplete supersession receipt: %#v", got)
	}
	if got.SourceReceiptID != blockedReceipt.ReceiptID || got.SatisfactionGateID != gate.GateID ||
		got.SourceDerivedSnapshotID != blockedReceipt.DerivedSnapshotID ||
		got.SourcePlacementSnapshotID != blockedReceipt.PlacementSnapshotID ||
		got.SourceAdviceID != blockedReceipt.AdviceID || got.ScopeID != blockedReceipt.ScopeID {
		t.Fatalf("source lineage mismatch: %#v", got)
	}
	if got.NewDerivedSnapshotID == got.SourceDerivedSnapshotID ||
		got.NewHeadroomEnvelopeID == envelope.EnvelopeID {
		t.Fatalf("recalculation reused stale evidence lineage: %#v", got)
	}
	if got.Reason != "fresh-evidence-advice-recalculated" ||
		got.RecommendedAction != "review-recalculated-advisory-placement" {
		t.Fatalf("unexpected supersession disposition: %#v", got)
	}
	if err := ValidatePlacementRecalculatedAdviceSupersessionReceipt(
		got,
		gate,
		blockedReceipt,
		sourceGate,
		freshness,
		envelope,
		request,
		[]PlacementNodeDerivationInput{current},
		recalculatedAt,
	); err != nil {
		t.Fatal(err)
	}
}

func TestPlacementRecalculatedAdviceSupersessionMatchesExactRecalculation(t *testing.T) {
	request, blockedReceipt, sourceGate, freshness, envelope, gate, current, recalculatedAt :=
		placementRecalculatedAdviceSupersessionFixture(t)
	got, err := BuildPlacementRecalculatedAdviceSupersessionReceipt(
		gate,
		blockedReceipt,
		sourceGate,
		freshness,
		envelope,
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
	if got.NewDerivedSnapshotID != derived.SnapshotID ||
		got.NewPlacementSnapshotID != derived.Placement.SnapshotID ||
		got.NewAdviceID != derived.Placement.Advice.AdviceID ||
		got.NewHeadroomEnvelopeID != headroom.EnvelopeID ||
		got.RecommendedNodeID != derived.Placement.Advice.RecommendedNodeID ||
		got.Action != derived.Placement.Advice.Action {
		t.Fatalf("supersession does not bind exact recalculation: %#v", got)
	}
	if got.AdviceChanged != (got.NewAdviceID != got.SourceAdviceID) ||
		got.PlacementSnapshotChanged != (got.NewPlacementSnapshotID != got.SourcePlacementSnapshotID) {
		t.Fatalf("change indicators do not match exact identifiers: %#v", got)
	}
	for _, candidate := range headroom.Candidates {
		for _, resource := range candidate.Resources {
			if resource.Evidence != agent.EvidenceMeasured {
				t.Fatalf("recalculated constraint is not measured: %#v", resource)
			}
		}
	}
}

func TestPlacementRecalculatedAdviceSupersessionIsDeterministic(t *testing.T) {
	request, blockedReceipt, sourceGate, freshness, envelope, gate, current, recalculatedAt :=
		placementRecalculatedAdviceSupersessionFixture(t)
	first, err := BuildPlacementRecalculatedAdviceSupersessionReceipt(
		gate,
		blockedReceipt,
		sourceGate,
		freshness,
		envelope,
		request,
		[]PlacementNodeDerivationInput{current},
		recalculatedAt,
	)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 100; i++ {
		next, err := BuildPlacementRecalculatedAdviceSupersessionReceipt(
			gate,
			blockedReceipt,
			sourceGate,
			freshness,
			envelope,
			request,
			[]PlacementNodeDerivationInput{current},
			recalculatedAt,
		)
		if err != nil {
			t.Fatal(err)
		}
		if next != first {
			t.Fatalf("same exact inputs changed receipt at iteration %d", i)
		}
	}
}

func TestPlacementRecalculatedAdviceSupersessionRejectsUnsafeOrStaleGate(t *testing.T) {
	request, blockedReceipt, sourceGate, freshness, envelope, gate, current, recalculatedAt :=
		placementRecalculatedAdviceSupersessionFixture(t)

	tampered := gate
	tampered.ReuseAuthorized = true
	if _, err := BuildPlacementRecalculatedAdviceSupersessionReceipt(
		tampered,
		blockedReceipt,
		sourceGate,
		freshness,
		envelope,
		request,
		[]PlacementNodeDerivationInput{current},
		recalculatedAt,
	); !errors.Is(err, ErrInvalidRecommendation) {
		t.Fatalf("unsafe gate error = %v", err)
	}

	blocked := gate
	blocked.Status = PlacementRebuildSatisfactionBlocked
	blocked.AllRequirementsSatisfied = false
	blocked.AdviceRecalculationPermitted = false
	blocked.Reason = "current-rebuild-evidence-incomplete"
	blocked.RecommendedAction = "collect-current-measured-evidence"
	if _, err := BuildPlacementRecalculatedAdviceSupersessionReceipt(
		blocked,
		blockedReceipt,
		sourceGate,
		freshness,
		envelope,
		request,
		[]PlacementNodeDerivationInput{current},
		recalculatedAt,
	); !errors.Is(err, ErrInvalidRecommendation) {
		t.Fatalf("blocked gate error = %v", err)
	}
}

func TestPlacementRecalculatedAdviceSupersessionRejectsScopeDrift(t *testing.T) {
	request, blockedReceipt, sourceGate, freshness, envelope, gate, current, recalculatedAt :=
		placementRecalculatedAdviceSupersessionFixture(t)
	request.ScopeID = "different-scope"
	if _, err := BuildPlacementRecalculatedAdviceSupersessionReceipt(
		gate,
		blockedReceipt,
		sourceGate,
		freshness,
		envelope,
		request,
		[]PlacementNodeDerivationInput{current},
		recalculatedAt,
	); !errors.Is(err, ErrInvalidRecommendation) {
		t.Fatalf("scope drift error = %v", err)
	}
}

func TestPlacementRecalculatedAdviceSupersessionRejectsAuthorityTampering(t *testing.T) {
	request, blockedReceipt, sourceGate, freshness, envelope, gate, current, recalculatedAt :=
		placementRecalculatedAdviceSupersessionFixture(t)
	got, err := BuildPlacementRecalculatedAdviceSupersessionReceipt(
		gate,
		blockedReceipt,
		sourceGate,
		freshness,
		envelope,
		request,
		[]PlacementNodeDerivationInput{current},
		recalculatedAt,
	)
	if err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*PlacementRecalculatedAdviceSupersessionReceipt){
		"source-reuse": func(value *PlacementRecalculatedAdviceSupersessionReceipt) {
			value.SourceReuseAuthorized = true
		},
		"reuse": func(value *PlacementRecalculatedAdviceSupersessionReceipt) {
			value.ReuseAuthorized = true
		},
		"placement": func(value *PlacementRecalculatedAdviceSupersessionReceipt) {
			value.PlacementAuthorized = true
		},
		"production": func(value *PlacementRecalculatedAdviceSupersessionReceipt) {
			value.ProductionMutation = true
		},
	} {
		t.Run(name, func(t *testing.T) {
			tampered := got
			mutate(&tampered)
			if err := ValidatePlacementRecalculatedAdviceSupersessionReceipt(
				tampered,
				gate,
				blockedReceipt,
				sourceGate,
				freshness,
				envelope,
				request,
				[]PlacementNodeDerivationInput{current},
				recalculatedAt,
			); !errors.Is(err, ErrInvalidRecommendation) {
				t.Fatalf("authority tamper error = %v", err)
			}
		})
	}
}
