package capacity

import (
	"encoding/json"
	"fmt"
	"reflect"
	"time"
)

const PlacementRecalculatedAdviceRevalidationSchemaV1 = "capacity.placement-recalculated-advice-revalidation/v1"

type PlacementRecalculatedAdviceRevalidationStatus string

const (
	PlacementRecalculatedAdviceCurrent PlacementRecalculatedAdviceRevalidationStatus = "current"
	PlacementRecalculatedAdviceStale   PlacementRecalculatedAdviceRevalidationStatus = "stale"
	PlacementRecalculatedAdviceBlocked PlacementRecalculatedAdviceRevalidationStatus = "blocked"
)

// PlacementRecalculatedAdviceRevalidationGate revalidates only the fresh
// advisory lineage produced by a recalculated supersession receipt. It can
// establish that a new advisory is current enough for a separate reuse
// decision, but never grants reuse, placement, or production-mutation authority.
type PlacementRecalculatedAdviceRevalidationGate struct {
	SchemaVersion              string                                        `json:"schema_version"`
	RevalidationID             string                                        `json:"revalidation_id"`
	SupersessionID             string                                        `json:"supersession_id"`
	DerivedSnapshotID          string                                        `json:"derived_snapshot_id"`
	PlacementSnapshotID        string                                        `json:"placement_snapshot_id"`
	AdviceID                   string                                        `json:"advice_id"`
	HeadroomEnvelopeID         string                                        `json:"headroom_envelope_id"`
	ResourceFreshnessGateID    string                                        `json:"resource_freshness_gate_id"`
	ScopeID                    string                                        `json:"scope_id"`
	CheckedAt                  time.Time                                     `json:"checked_at"`
	Status                     PlacementRecalculatedAdviceRevalidationStatus `json:"status"`
	RecommendedNodeID          string                                        `json:"recommended_node_id,omitempty"`
	SafetyScoreBand            PlacementSafetyScoreBand                      `json:"safety_score_band,omitempty"`
	EffectiveSafetyMarginPct   float64                                       `json:"effective_safety_margin_percent"`
	FreshReuseDecisionEligible bool                                          `json:"fresh_reuse_decision_eligible"`
	SourceReuseAuthorized      bool                                          `json:"source_reuse_authorized"`
	ReuseAuthorized            bool                                          `json:"reuse_authorized"`
	PlacementAuthorized        bool                                          `json:"placement_authorized"`
	AdvisoryOnly               bool                                          `json:"advisory_only"`
	ProductionMutation         bool                                          `json:"production_mutation"`
	Reason                     string                                        `json:"reason"`
	RecommendedAction          string                                        `json:"recommended_action"`
}

// RevalidatePlacementRecalculatedAdviceSupersession checks the exact new
// derived snapshot and headroom envelope sealed by supersession, then applies
// per-constraint telemetry freshness and a positive safe-headroom boundary at
// checkedAt. Old source identifiers remain non-reusable. A current result only
// permits evaluation by a future, separate reuse-decision gate.
func RevalidatePlacementRecalculatedAdviceSupersession(
	supersession PlacementRecalculatedAdviceSupersessionReceipt,
	request PlacementRequest,
	derived PlacementAdviceDerivedEvidenceSnapshot,
	envelope PlacementResourceHeadroomEnvelope,
	inputs []PlacementNodeDerivationInput,
	checkedAt time.Time,
) (PlacementRecalculatedAdviceRevalidationGate, error) {
	if checkedAt.IsZero() {
		return PlacementRecalculatedAdviceRevalidationGate{}, fmt.Errorf(
			"%w: checked_at is required",
			ErrInvalidRecommendation,
		)
	}
	checkedAt = checkedAt.UTC()
	if checkedAt.Before(supersession.RecalculatedAt) {
		return PlacementRecalculatedAdviceRevalidationGate{}, fmt.Errorf(
			"%w: checked_at precedes recalculated advice",
			ErrInvalidRecommendation,
		)
	}
	if err := validatePlacementRecalculatedAdviceSupersessionReceipt(supersession); err != nil {
		return PlacementRecalculatedAdviceRevalidationGate{}, err
	}
	if supersession.NewDerivedSnapshotID != derived.SnapshotID ||
		supersession.NewPlacementSnapshotID != derived.Placement.SnapshotID ||
		supersession.NewAdviceID != derived.Placement.Advice.AdviceID ||
		supersession.NewHeadroomEnvelopeID != envelope.EnvelopeID ||
		supersession.ScopeID != derived.Placement.Advice.ScopeID ||
		supersession.ScopeID != envelope.ScopeID ||
		envelope.DerivedSnapshotID != derived.SnapshotID ||
		envelope.PlacementSnapshotID != derived.Placement.SnapshotID ||
		envelope.AdviceID != derived.Placement.Advice.AdviceID ||
		!envelope.EvaluatedAt.Equal(supersession.RecalculatedAt) {
		return PlacementRecalculatedAdviceRevalidationGate{}, fmt.Errorf(
			"%w: recalculated advice lineage does not match exact fresh artifacts",
			ErrInvalidRecommendation,
		)
	}

	freshness, err := BuildPlacementResourceHeadroomFreshnessGate(
		envelope,
		request,
		derived,
		inputs,
		checkedAt,
	)
	if err != nil {
		return PlacementRecalculatedAdviceRevalidationGate{}, err
	}

	result := PlacementRecalculatedAdviceRevalidationGate{
		SchemaVersion:           PlacementRecalculatedAdviceRevalidationSchemaV1,
		SupersessionID:          supersession.SupersessionID,
		DerivedSnapshotID:       derived.SnapshotID,
		PlacementSnapshotID:     derived.Placement.SnapshotID,
		AdviceID:                derived.Placement.Advice.AdviceID,
		HeadroomEnvelopeID:      envelope.EnvelopeID,
		ResourceFreshnessGateID: freshness.GateID,
		ScopeID:                 supersession.ScopeID,
		CheckedAt:               checkedAt,
		Status:                  PlacementRecalculatedAdviceBlocked,
		RecommendedNodeID:       supersession.RecommendedNodeID,
		SourceReuseAuthorized:   false,
		ReuseAuthorized:         false,
		PlacementAuthorized:     false,
		AdvisoryOnly:            true,
		ProductionMutation:      false,
		Reason:                  "recalculated-advice-has-no-safe-placement",
		RecommendedAction:       "review-capacity-or-evidence-action",
	}

	if hasStalePlacementResourceEvidence(freshness) {
		result.Status = PlacementRecalculatedAdviceStale
		result.Reason = "recalculated-advice-evidence-stale"
		result.RecommendedAction = "collect-current-placement-evidence"
	} else if !freshness.AllCurrent || !freshness.AdviceReusePermitted {
		result.Reason = "recalculated-advice-evidence-degraded"
		result.RecommendedAction = "collect-measured-placement-evidence"
	} else if supersession.Action == ActionNone && supersession.RecommendedNodeID != "" {
		candidate, ok := placementResourceHeadroomCandidateByNode(
			envelope.Candidates,
			supersession.RecommendedNodeID,
		)
		if !ok {
			return PlacementRecalculatedAdviceRevalidationGate{}, fmt.Errorf(
				"%w: recommended node missing from headroom envelope",
				ErrInvalidRecommendation,
			)
		}
		result.SafetyScoreBand = candidate.ScoreBand
		result.EffectiveSafetyMarginPct = candidate.EffectiveSafetyMarginPercent
		if candidate.Eligible &&
			candidate.ScoreBand == PlacementSafetyHeadroom &&
			candidate.EffectiveSafetyMarginPercent >
				floatTolerance(candidate.EffectiveSafetyMarginPercent) {
			result.Status = PlacementRecalculatedAdviceCurrent
			result.FreshReuseDecisionEligible = true
			result.Reason = "recalculated-advice-current-with-safe-headroom"
			result.RecommendedAction = "evaluate-fresh-reuse-decision"
		} else {
			result.Reason = "recalculated-advice-safety-headroom-not-positive"
		}
	}

	result.RevalidationID = placementRecalculatedAdviceRevalidationID(result)
	if err := validatePlacementRecalculatedAdviceRevalidationGate(result); err != nil {
		return PlacementRecalculatedAdviceRevalidationGate{}, err
	}
	return result, nil
}

// ValidatePlacementRecalculatedAdviceRevalidationGate reconstructs the exact
// verdict from immutable supersession lineage and current freshness inputs.
func ValidatePlacementRecalculatedAdviceRevalidationGate(
	gate PlacementRecalculatedAdviceRevalidationGate,
	supersession PlacementRecalculatedAdviceSupersessionReceipt,
	request PlacementRequest,
	derived PlacementAdviceDerivedEvidenceSnapshot,
	envelope PlacementResourceHeadroomEnvelope,
	inputs []PlacementNodeDerivationInput,
	checkedAt time.Time,
) error {
	expected, err := RevalidatePlacementRecalculatedAdviceSupersession(
		supersession,
		request,
		derived,
		envelope,
		inputs,
		checkedAt,
	)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(expected, gate) {
		return fmt.Errorf(
			"%w: recalculated advice revalidation gate was modified",
			ErrInvalidRecommendation,
		)
	}
	return nil
}

func hasStalePlacementResourceEvidence(gate PlacementResourceHeadroomFreshnessGate) bool {
	for _, candidate := range gate.Candidates {
		if candidate.Status == PlacementResourceEvidenceStale {
			return true
		}
	}
	return false
}

func placementResourceHeadroomCandidateByNode(
	candidates []PlacementResourceHeadroomCandidate,
	nodeID string,
) (PlacementResourceHeadroomCandidate, bool) {
	for _, candidate := range candidates {
		if candidate.NodeID == nodeID {
			return candidate, true
		}
	}
	return PlacementResourceHeadroomCandidate{}, false
}

func validatePlacementRecalculatedAdviceRevalidationGate(
	gate PlacementRecalculatedAdviceRevalidationGate,
) error {
	if gate.SchemaVersion != PlacementRecalculatedAdviceRevalidationSchemaV1 ||
		gate.SourceReuseAuthorized || gate.ReuseAuthorized || gate.PlacementAuthorized ||
		!gate.AdvisoryOnly || gate.ProductionMutation {
		return fmt.Errorf(
			"%w: unsafe recalculated advice revalidation",
			ErrInvalidRecommendation,
		)
	}
	if gate.RevalidationID == "" || gate.SupersessionID == "" ||
		gate.DerivedSnapshotID == "" || gate.PlacementSnapshotID == "" ||
		gate.AdviceID == "" || gate.HeadroomEnvelopeID == "" ||
		gate.ResourceFreshnessGateID == "" || gate.ScopeID == "" ||
		gate.CheckedAt.IsZero() || gate.CheckedAt.Location() != time.UTC ||
		gate.Reason == "" || gate.RecommendedAction == "" {
		return fmt.Errorf(
			"%w: incomplete recalculated advice revalidation",
			ErrInvalidRecommendation,
		)
	}

	switch gate.Status {
	case PlacementRecalculatedAdviceCurrent:
		if !gate.FreshReuseDecisionEligible || gate.RecommendedNodeID == "" ||
			gate.SafetyScoreBand != PlacementSafetyHeadroom ||
			gate.EffectiveSafetyMarginPct <= floatTolerance(gate.EffectiveSafetyMarginPct) ||
			gate.Reason != "recalculated-advice-current-with-safe-headroom" ||
			gate.RecommendedAction != "evaluate-fresh-reuse-decision" {
			return fmt.Errorf(
				"%w: inconsistent current recalculated advice",
				ErrInvalidRecommendation,
			)
		}
	case PlacementRecalculatedAdviceStale:
		if gate.FreshReuseDecisionEligible ||
			gate.Reason != "recalculated-advice-evidence-stale" ||
			gate.RecommendedAction != "collect-current-placement-evidence" {
			return fmt.Errorf(
				"%w: inconsistent stale recalculated advice",
				ErrInvalidRecommendation,
			)
		}
	case PlacementRecalculatedAdviceBlocked:
		if gate.FreshReuseDecisionEligible ||
			gate.RecommendedAction == "evaluate-fresh-reuse-decision" {
			return fmt.Errorf(
				"%w: inconsistent blocked recalculated advice",
				ErrInvalidRecommendation,
			)
		}
	default:
		return fmt.Errorf(
			"%w: invalid recalculated advice status",
			ErrInvalidRecommendation,
		)
	}

	if placementRecalculatedAdviceRevalidationID(gate) != gate.RevalidationID {
		return fmt.Errorf(
			"%w: recalculated advice revalidation integrity mismatch",
			ErrInvalidRecommendation,
		)
	}
	return nil
}

func placementRecalculatedAdviceRevalidationID(
	gate PlacementRecalculatedAdviceRevalidationGate,
) string {
	canonical := gate
	canonical.RevalidationID = ""
	encoded, _ := json.Marshal(canonical)
	return "prar-" + placementEvidenceSHA256Hex(encoded)[:24]
}
