package capacity

import (
	"errors"
	"testing"
	"time"

	"control-center/internal/agent"
)

func placementRebuildSatisfactionFixture(t *testing.T) (
	PlacementRebuildEvidenceReceipt,
	PlacementResourceReuseGate,
	PlacementResourceHeadroomFreshnessGate,
	PlacementResourceHeadroomEnvelope,
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
	gate, err := EvaluatePlacementResourceReuseGate(
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
	receipt, err := BuildPlacementRebuildEvidenceReceipt(gate, freshness, envelope)
	if err != nil {
		t.Fatal(err)
	}

	freshAt := blockedAt.Add(time.Minute)
	current := input
	current.Telemetry.SnapshotID = "snapshot-node-a-rebuild"
	current.Telemetry.Revision = "revision-node-a-rebuild"
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
	return receipt, gate, freshness, envelope, current, freshAt
}

func TestPlacementRebuildSatisfactionGatePermitsOnlyNewAdvisoryRecalculation(t *testing.T) {
	receipt, sourceGate, freshness, envelope, current, checkedAt :=
		placementRebuildSatisfactionFixture(t)
	got, err := BuildPlacementRebuildSatisfactionGate(
		receipt,
		sourceGate,
		freshness,
		envelope,
		[]PlacementNodeDerivationInput{current},
		checkedAt,
	)
	if err != nil {
		t.Fatal(err)
	}
	if got.SchemaVersion != PlacementRebuildSatisfactionSchemaV1 || got.GateID == "" ||
		got.Status != PlacementRebuildSatisfactionSatisfied || !got.AllRequirementsSatisfied ||
		!got.AdviceRecalculationPermitted || got.ReuseAuthorized || got.PlacementAuthorized ||
		!got.AdvisoryOnly || got.ProductionMutation ||
		got.RecommendedAction != "recalculate-advisory-placement" {
		t.Fatalf("unsafe or incomplete satisfaction gate: %#v", got)
	}
	if got.ReceiptID != receipt.ReceiptID || got.SourceResourceReuseGateID != sourceGate.GateID ||
		got.SourceHeadroomEnvelopeID != envelope.EnvelopeID ||
		got.SourceDerivedSnapshotID != envelope.DerivedSnapshotID || got.ScopeID != receipt.ScopeID {
		t.Fatalf("satisfaction lineage mismatch: %#v", got)
	}
	if len(got.Checks) != len(envelope.Candidates[0].Resources) {
		t.Fatalf("checks = %d, want %d", len(got.Checks), len(envelope.Candidates[0].Resources))
	}
	for _, check := range got.Checks {
		if !check.Satisfied || check.ObservedEvidence != agent.EvidenceMeasured ||
			check.Reason != "exact-current-measured-evidence" {
			t.Fatalf("fresh measured resource was not satisfied: %#v", check)
		}
	}
	if err := ValidatePlacementRebuildSatisfactionGate(
		got,
		receipt,
		sourceGate,
		freshness,
		envelope,
		[]PlacementNodeDerivationInput{current},
		checkedAt,
	); err != nil {
		t.Fatal(err)
	}
}

func TestPlacementRebuildSatisfactionGateBlocksNonMeasuredEvidence(t *testing.T) {
	receipt, sourceGate, freshness, envelope, current, checkedAt :=
		placementRebuildSatisfactionFixture(t)
	current.Telemetry.Observations = append(
		[]agent.CapacityObservation(nil),
		current.Telemetry.Observations...,
	)
	for index := range current.Telemetry.Observations {
		if current.Telemetry.Observations[index].Metric == agent.MetricStorageUsed {
			current.Telemetry.Observations[index].Evidence = agent.EvidenceBenchmark
		}
	}
	var err error
	current.Evidence, err = DeriveNodeProjection(current.Profile, current.Telemetry, checkedAt)
	if err != nil {
		t.Fatal(err)
	}

	got, err := BuildPlacementRebuildSatisfactionGate(
		receipt,
		sourceGate,
		freshness,
		envelope,
		[]PlacementNodeDerivationInput{current},
		checkedAt,
	)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != PlacementRebuildSatisfactionBlocked || got.AllRequirementsSatisfied ||
		got.AdviceRecalculationPermitted || got.ReuseAuthorized || got.PlacementAuthorized ||
		got.ProductionMutation || got.RecommendedAction != "collect-current-measured-evidence" {
		t.Fatalf("non-measured evidence escaped rebuild gate: %#v", got)
	}
	blocked := 0
	for _, check := range got.Checks {
		if !check.Satisfied {
			blocked++
			if check.ObservedEvidence != agent.EvidenceBenchmark ||
				check.Reason != "current-evidence-not-measured" {
				t.Fatalf("unexpected blocked check: %#v", check)
			}
		}
	}
	if blocked != 1 {
		t.Fatalf("blocked checks = %d, want 1", blocked)
	}
}

func TestPlacementRebuildSatisfactionGateRejectsStaleCurrentDerivation(t *testing.T) {
	receipt, sourceGate, freshness, envelope, current, checkedAt :=
		placementRebuildSatisfactionFixture(t)
	staleAt := checkedAt.Add(20 * time.Minute)
	if _, err := BuildPlacementRebuildSatisfactionGate(
		receipt,
		sourceGate,
		freshness,
		envelope,
		[]PlacementNodeDerivationInput{current},
		staleAt,
	); !errors.Is(err, ErrInvalidRecommendation) {
		t.Fatalf("stale current derivation error = %v", err)
	}
}

func TestPlacementRebuildSatisfactionGateIsDeterministicAndTamperEvident(t *testing.T) {
	receipt, sourceGate, freshness, envelope, current, checkedAt :=
		placementRebuildSatisfactionFixture(t)
	first, err := BuildPlacementRebuildSatisfactionGate(
		receipt,
		sourceGate,
		freshness,
		envelope,
		[]PlacementNodeDerivationInput{current},
		checkedAt,
	)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 100; i++ {
		next, err := BuildPlacementRebuildSatisfactionGate(
			receipt,
			sourceGate,
			freshness,
			envelope,
			[]PlacementNodeDerivationInput{current},
			checkedAt,
		)
		if err != nil {
			t.Fatal(err)
		}
		if next.GateID != first.GateID {
			t.Fatalf("same exact inputs changed gate at iteration %d", i)
		}
	}

	tampered := first
	tampered.ReuseAuthorized = true
	if err := ValidatePlacementRebuildSatisfactionGate(
		tampered,
		receipt,
		sourceGate,
		freshness,
		envelope,
		[]PlacementNodeDerivationInput{current},
		checkedAt,
	); !errors.Is(err, ErrInvalidRecommendation) {
		t.Fatalf("reuse authority tamper error = %v", err)
	}
}

func TestPlacementRebuildSatisfactionGateRequiresExactConstraintLineage(t *testing.T) {
	receipt, sourceGate, freshness, envelope, current, checkedAt :=
		placementRebuildSatisfactionFixture(t)
	current.Profile.Constraints = append([]Constraint(nil), current.Profile.Constraints...)
	current.Profile.Constraints[0].ID = "replacement-constraint"
	if _, err := BuildPlacementRebuildSatisfactionGate(
		receipt,
		sourceGate,
		freshness,
		envelope,
		[]PlacementNodeDerivationInput{current},
		checkedAt,
	); !errors.Is(err, ErrInvalidRecommendation) {
		t.Fatalf("constraint lineage error = %v", err)
	}
}

func TestPlacementRebuildSatisfactionGateMeasuredEvidenceProperty(t *testing.T) {
	receipt, sourceGate, freshness, envelope, current, checkedAt :=
		placementRebuildSatisfactionFixture(t)
	for _, evidence := range []agent.ObservationEvidence{
		agent.EvidenceMeasured,
		agent.EvidenceBenchmark,
	} {
		candidate := current
		candidate.Telemetry.Observations = append(
			[]agent.CapacityObservation(nil),
			current.Telemetry.Observations...,
		)
		for index := range candidate.Telemetry.Observations {
			if candidate.Telemetry.Observations[index].Metric == agent.MetricStorageUsed {
				candidate.Telemetry.Observations[index].Evidence = evidence
			}
		}
		var err error
		candidate.Evidence, err = DeriveNodeProjection(candidate.Profile, candidate.Telemetry, checkedAt)
		if err != nil {
			t.Fatal(err)
		}
		got, err := BuildPlacementRebuildSatisfactionGate(
			receipt,
			sourceGate,
			freshness,
			envelope,
			[]PlacementNodeDerivationInput{candidate},
			checkedAt,
		)
		if err != nil {
			t.Fatal(err)
		}
		want := evidence == agent.EvidenceMeasured
		if got.AdviceRecalculationPermitted != want || got.AllRequirementsSatisfied != want {
			t.Fatalf("evidence=%q permitted=%v want=%v", evidence, got.AdviceRecalculationPermitted, want)
		}
		if got.ReuseAuthorized || got.PlacementAuthorized || got.ProductionMutation {
			t.Fatalf("evidence=%q gained mutation authority: %#v", evidence, got)
		}
	}
}
