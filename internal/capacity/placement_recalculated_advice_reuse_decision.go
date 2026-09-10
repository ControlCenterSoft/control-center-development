package capacity

import (
	"encoding/json"
	"fmt"
	"reflect"
	"time"
)

const PlacementRecalculatedAdviceReuseDecisionSchemaV1 = "capacity.placement-recalculated-advice-reuse-decision/v1"

// PlacementRecalculatedAdviceReuseDecision is the bounded consumer decision
// for a freshly revalidated recalculated placement recommendation. It permits
// reuse only as advisory Planner input. It never restores source-evidence reuse
// and never grants placement or production-mutation authority.
type PlacementRecalculatedAdviceReuseDecision struct {
	SchemaVersion             string                   `json:"schema_version"`
	DecisionID                string                   `json:"decision_id"`
	RevalidationID            string                   `json:"revalidation_id"`
	SupersessionID            string                   `json:"supersession_id"`
	SourceDerivedSnapshotID   string                   `json:"source_derived_snapshot_id"`
	SourcePlacementSnapshotID string                   `json:"source_placement_snapshot_id"`
	SourceAdviceID            string                   `json:"source_advice_id"`
	DerivedSnapshotID         string                   `json:"derived_snapshot_id"`
	PlacementSnapshotID       string                   `json:"placement_snapshot_id"`
	AdviceID                  string                   `json:"advice_id"`
	HeadroomEnvelopeID        string                   `json:"headroom_envelope_id"`
	ResourceFreshnessGateID   string                   `json:"resource_freshness_gate_id"`
	ScopeID                   string                   `json:"scope_id"`
	CheckedAt                 time.Time                `json:"checked_at"`
	RecommendedNodeID         string                   `json:"recommended_node_id"`
	SafetyScoreBand           PlacementSafetyScoreBand `json:"safety_score_band"`
	EffectiveSafetyMarginPct  float64                  `json:"effective_safety_margin_percent"`
	FreshAdvisoryReuseAllowed bool                     `json:"fresh_advisory_reuse_allowed"`
	SourceReuseAuthorized     bool                     `json:"source_reuse_authorized"`
	PlacementAuthorized       bool                     `json:"placement_authorized"`
	AdvisoryOnly              bool                     `json:"advisory_only"`
	ProductionMutation        bool                     `json:"production_mutation"`
	Reason                    string                   `json:"reason"`
	RecommendedAction         string                   `json:"recommended_action"`
}

// BuildPlacementRecalculatedAdviceReuseDecision consumes only an exact current
// revalidation gate. The gate is reconstructed from the supersession and fresh
// resource evidence before a decision is emitted. Stale or blocked evidence is
// rejected rather than converted into a reusable advisory capability.
func BuildPlacementRecalculatedAdviceReuseDecision(
	gate PlacementRecalculatedAdviceRevalidationGate,
	supersession PlacementRecalculatedAdviceSupersessionReceipt,
	request PlacementRequest,
	derived PlacementAdviceDerivedEvidenceSnapshot,
	envelope PlacementResourceHeadroomEnvelope,
	inputs []PlacementNodeDerivationInput,
) (PlacementRecalculatedAdviceReuseDecision, error) {
	if err := ValidatePlacementRecalculatedAdviceRevalidationGate(
		gate,
		supersession,
		request,
		derived,
		envelope,
		inputs,
		gate.CheckedAt,
	); err != nil {
		return PlacementRecalculatedAdviceReuseDecision{}, err
	}
	if gate.Status != PlacementRecalculatedAdviceCurrent ||
		!gate.FreshReuseDecisionEligible {
		return PlacementRecalculatedAdviceReuseDecision{}, fmt.Errorf(
			"%w: fresh advisory reuse requires current recalculated advice",
			ErrInvalidRecommendation,
		)
	}
	if gate.RecommendedNodeID != supersession.RecommendedNodeID ||
		gate.DerivedSnapshotID != supersession.NewDerivedSnapshotID ||
		gate.PlacementSnapshotID != supersession.NewPlacementSnapshotID ||
		gate.AdviceID != supersession.NewAdviceID ||
		gate.HeadroomEnvelopeID != supersession.NewHeadroomEnvelopeID ||
		gate.ScopeID != supersession.ScopeID {
		return PlacementRecalculatedAdviceReuseDecision{}, fmt.Errorf(
			"%w: revalidation does not match recalculated supersession lineage",
			ErrInvalidRecommendation,
		)
	}

	candidate, ok := placementResourceHeadroomCandidateByNode(
		envelope.Candidates,
		gate.RecommendedNodeID,
	)
	if !ok || !candidate.Eligible || candidate.ScoreBand != PlacementSafetyHeadroom ||
		candidate.ScoreBand != gate.SafetyScoreBand ||
		candidate.EffectiveSafetyMarginPercent != gate.EffectiveSafetyMarginPct ||
		candidate.EffectiveSafetyMarginPercent <=
			floatTolerance(candidate.EffectiveSafetyMarginPercent) {
		return PlacementRecalculatedAdviceReuseDecision{}, fmt.Errorf(
			"%w: fresh advisory reuse requires exact positive safety headroom",
			ErrInvalidRecommendation,
		)
	}

	result := PlacementRecalculatedAdviceReuseDecision{
		SchemaVersion:             PlacementRecalculatedAdviceReuseDecisionSchemaV1,
		RevalidationID:            gate.RevalidationID,
		SupersessionID:            supersession.SupersessionID,
		SourceDerivedSnapshotID:   supersession.SourceDerivedSnapshotID,
		SourcePlacementSnapshotID: supersession.SourcePlacementSnapshotID,
		SourceAdviceID:            supersession.SourceAdviceID,
		DerivedSnapshotID:         gate.DerivedSnapshotID,
		PlacementSnapshotID:       gate.PlacementSnapshotID,
		AdviceID:                  gate.AdviceID,
		HeadroomEnvelopeID:        gate.HeadroomEnvelopeID,
		ResourceFreshnessGateID:   gate.ResourceFreshnessGateID,
		ScopeID:                   gate.ScopeID,
		CheckedAt:                 gate.CheckedAt,
		RecommendedNodeID:         gate.RecommendedNodeID,
		SafetyScoreBand:           gate.SafetyScoreBand,
		EffectiveSafetyMarginPct:  gate.EffectiveSafetyMarginPct,
		FreshAdvisoryReuseAllowed: true,
		SourceReuseAuthorized:     false,
		PlacementAuthorized:       false,
		AdvisoryOnly:              true,
		ProductionMutation:        false,
		Reason:                    "current-recalculated-advice-reuse-allowed",
		RecommendedAction:         "reuse-current-recalculated-advice-for-planning-only",
	}
	result.DecisionID = placementRecalculatedAdviceReuseDecisionID(result)
	if err := validatePlacementRecalculatedAdviceReuseDecision(result); err != nil {
		return PlacementRecalculatedAdviceReuseDecision{}, err
	}
	return result, nil
}

// ValidatePlacementRecalculatedAdviceReuseDecision reconstructs the exact
// decision from its immutable inputs and rejects any authority or lineage
// expansion after creation.
func ValidatePlacementRecalculatedAdviceReuseDecision(
	decision PlacementRecalculatedAdviceReuseDecision,
	gate PlacementRecalculatedAdviceRevalidationGate,
	supersession PlacementRecalculatedAdviceSupersessionReceipt,
	request PlacementRequest,
	derived PlacementAdviceDerivedEvidenceSnapshot,
	envelope PlacementResourceHeadroomEnvelope,
	inputs []PlacementNodeDerivationInput,
) error {
	expected, err := BuildPlacementRecalculatedAdviceReuseDecision(
		gate,
		supersession,
		request,
		derived,
		envelope,
		inputs,
	)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(expected, decision) {
		return fmt.Errorf(
			"%w: recalculated advice reuse decision was modified",
			ErrInvalidRecommendation,
		)
	}
	return nil
}

func validatePlacementRecalculatedAdviceReuseDecision(
	decision PlacementRecalculatedAdviceReuseDecision,
) error {
	if decision.SchemaVersion != PlacementRecalculatedAdviceReuseDecisionSchemaV1 ||
		!decision.FreshAdvisoryReuseAllowed || decision.SourceReuseAuthorized ||
		decision.PlacementAuthorized || !decision.AdvisoryOnly || decision.ProductionMutation {
		return fmt.Errorf(
			"%w: unsafe recalculated advice reuse decision",
			ErrInvalidRecommendation,
		)
	}
	if decision.DecisionID == "" || decision.RevalidationID == "" ||
		decision.SupersessionID == "" || decision.SourceDerivedSnapshotID == "" ||
		decision.SourcePlacementSnapshotID == "" || decision.SourceAdviceID == "" ||
		decision.DerivedSnapshotID == "" || decision.PlacementSnapshotID == "" ||
		decision.AdviceID == "" || decision.HeadroomEnvelopeID == "" ||
		decision.ResourceFreshnessGateID == "" || decision.ScopeID == "" ||
		decision.CheckedAt.IsZero() || decision.CheckedAt.Location() != time.UTC ||
		decision.RecommendedNodeID == "" || decision.SafetyScoreBand != PlacementSafetyHeadroom ||
		decision.EffectiveSafetyMarginPct <= floatTolerance(decision.EffectiveSafetyMarginPct) {
		return fmt.Errorf(
			"%w: incomplete recalculated advice reuse decision",
			ErrInvalidRecommendation,
		)
	}
	if decision.SourceDerivedSnapshotID == decision.DerivedSnapshotID {
		return fmt.Errorf(
			"%w: fresh advisory decision cannot reuse source derived evidence",
			ErrInvalidRecommendation,
		)
	}
	if decision.Reason != "current-recalculated-advice-reuse-allowed" ||
		decision.RecommendedAction != "reuse-current-recalculated-advice-for-planning-only" {
		return fmt.Errorf(
			"%w: invalid recalculated advice reuse disposition",
			ErrInvalidRecommendation,
		)
	}
	if placementRecalculatedAdviceReuseDecisionID(decision) != decision.DecisionID {
		return fmt.Errorf(
			"%w: recalculated advice reuse decision integrity mismatch",
			ErrInvalidRecommendation,
		)
	}
	return nil
}

func placementRecalculatedAdviceReuseDecisionID(
	decision PlacementRecalculatedAdviceReuseDecision,
) string {
	canonical := decision
	canonical.DecisionID = ""
	encoded, _ := json.Marshal(canonical)
	return "prard-" + placementEvidenceSHA256Hex(encoded)[:24]
}
