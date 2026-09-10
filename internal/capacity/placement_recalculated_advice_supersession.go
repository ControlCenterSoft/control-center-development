package capacity

import (
	"encoding/json"
	"fmt"
	"reflect"
	"time"
)

const PlacementRecalculatedAdviceSupersessionSchemaV1 = "capacity.placement-recalculated-advice-supersession/v1"

// PlacementRecalculatedAdviceSupersessionReceipt seals the transition from a
// previously blocked placement recommendation to a newly recalculated advisory
// recommendation built from a fully satisfied fresh-evidence gate. It never
// restores reuse authority for the old recommendation and never authorizes a
// placement or production mutation.
type PlacementRecalculatedAdviceSupersessionReceipt struct {
	SchemaVersion             string               `json:"schema_version"`
	SupersessionID            string               `json:"supersession_id"`
	SourceReceiptID           string               `json:"source_receipt_id"`
	SatisfactionGateID        string               `json:"satisfaction_gate_id"`
	SourceDerivedSnapshotID   string               `json:"source_derived_snapshot_id"`
	SourcePlacementSnapshotID string               `json:"source_placement_snapshot_id"`
	SourceAdviceID            string               `json:"source_advice_id"`
	NewDerivedSnapshotID      string               `json:"new_derived_snapshot_id"`
	NewPlacementSnapshotID    string               `json:"new_placement_snapshot_id"`
	NewAdviceID               string               `json:"new_advice_id"`
	NewHeadroomEnvelopeID     string               `json:"new_headroom_envelope_id"`
	ScopeID                   string               `json:"scope_id"`
	RecalculatedAt            time.Time            `json:"recalculated_at"`
	RecommendedNodeID         string               `json:"recommended_node_id,omitempty"`
	Action                    RecommendationAction `json:"action"`
	AdviceChanged             bool                 `json:"advice_changed"`
	PlacementSnapshotChanged  bool                 `json:"placement_snapshot_changed"`
	SupersedesSource          bool                 `json:"supersedes_source"`
	FreshEvidenceRecalculated bool                 `json:"fresh_evidence_recalculated"`
	SourceReuseAuthorized     bool                 `json:"source_reuse_authorized"`
	ReuseAuthorized           bool                 `json:"reuse_authorized"`
	PlacementAuthorized       bool                 `json:"placement_authorized"`
	AdvisoryOnly              bool                 `json:"advisory_only"`
	ProductionMutation        bool                 `json:"production_mutation"`
	Reason                    string               `json:"reason"`
	RecommendedAction         string               `json:"recommended_action"`
}

// BuildPlacementRecalculatedAdviceSupersessionReceipt consumes only a satisfied
// rebuild gate, re-derives placement advice from the exact fresh derivation
// inputs and seals a new resource-headroom envelope. The result supersedes the
// old blocked recommendation as evidence, but grants no reuse or execution
// authority. Any later reuse decision must pass a fresh explicit reuse gate.
func BuildPlacementRecalculatedAdviceSupersessionReceipt(
	gate PlacementRebuildSatisfactionGate,
	receipt PlacementRebuildEvidenceReceipt,
	sourceGate PlacementResourceReuseGate,
	sourceFreshness PlacementResourceHeadroomFreshnessGate,
	sourceEnvelope PlacementResourceHeadroomEnvelope,
	request PlacementRequest,
	currentInputs []PlacementNodeDerivationInput,
	recalculatedAt time.Time,
) (PlacementRecalculatedAdviceSupersessionReceipt, error) {
	if err := ValidatePlacementRebuildSatisfactionGate(
		gate,
		receipt,
		sourceGate,
		sourceFreshness,
		sourceEnvelope,
		currentInputs,
		recalculatedAt,
	); err != nil {
		return PlacementRecalculatedAdviceSupersessionReceipt{}, err
	}
	if gate.Status != PlacementRebuildSatisfactionSatisfied ||
		!gate.AllRequirementsSatisfied || !gate.AdviceRecalculationPermitted {
		return PlacementRecalculatedAdviceSupersessionReceipt{}, fmt.Errorf(
			"%w: recalculated advice requires a satisfied rebuild gate",
			ErrInvalidRecommendation,
		)
	}

	derived, err := CapturePlacementAdviceFromDerivations(request, currentInputs, recalculatedAt)
	if err != nil {
		return PlacementRecalculatedAdviceSupersessionReceipt{}, err
	}
	if derived.Placement.Advice.ScopeID != receipt.ScopeID ||
		derived.Placement.Advice.ScopeID != gate.ScopeID {
		return PlacementRecalculatedAdviceSupersessionReceipt{}, fmt.Errorf(
			"%w: recalculated placement scope changed rebuild lineage",
			ErrInvalidRecommendation,
		)
	}
	headroom, err := BuildPlacementResourceHeadroomEnvelope(
		request,
		derived,
		currentInputs,
		recalculatedAt,
	)
	if err != nil {
		return PlacementRecalculatedAdviceSupersessionReceipt{}, err
	}
	if derived.SnapshotID == receipt.DerivedSnapshotID || headroom.EnvelopeID == sourceEnvelope.EnvelopeID {
		return PlacementRecalculatedAdviceSupersessionReceipt{}, fmt.Errorf(
			"%w: rebuild did not produce fresh placement evidence lineage",
			ErrInvalidRecommendation,
		)
	}

	advice := derived.Placement.Advice
	result := PlacementRecalculatedAdviceSupersessionReceipt{
		SchemaVersion:             PlacementRecalculatedAdviceSupersessionSchemaV1,
		SourceReceiptID:           receipt.ReceiptID,
		SatisfactionGateID:        gate.GateID,
		SourceDerivedSnapshotID:   receipt.DerivedSnapshotID,
		SourcePlacementSnapshotID: receipt.PlacementSnapshotID,
		SourceAdviceID:            receipt.AdviceID,
		NewDerivedSnapshotID:      derived.SnapshotID,
		NewPlacementSnapshotID:    derived.Placement.SnapshotID,
		NewAdviceID:               advice.AdviceID,
		NewHeadroomEnvelopeID:     headroom.EnvelopeID,
		ScopeID:                   receipt.ScopeID,
		RecalculatedAt:            recalculatedAt.UTC(),
		RecommendedNodeID:         advice.RecommendedNodeID,
		Action:                    advice.Action,
		AdviceChanged:             advice.AdviceID != receipt.AdviceID,
		PlacementSnapshotChanged:  derived.Placement.SnapshotID != receipt.PlacementSnapshotID,
		SupersedesSource:          true,
		FreshEvidenceRecalculated: true,
		SourceReuseAuthorized:     false,
		ReuseAuthorized:           false,
		PlacementAuthorized:       false,
		AdvisoryOnly:              true,
		ProductionMutation:        false,
		Reason:                    "fresh-evidence-advice-recalculated",
		RecommendedAction:         "review-recalculated-advisory-placement",
	}
	result.SupersessionID = placementRecalculatedAdviceSupersessionID(result)
	if err := validatePlacementRecalculatedAdviceSupersessionReceipt(result); err != nil {
		return PlacementRecalculatedAdviceSupersessionReceipt{}, err
	}
	return result, nil
}

// ValidatePlacementRecalculatedAdviceSupersessionReceipt reconstructs the exact
// receipt from the source block and fresh derivation inputs. This makes the
// supersession deterministic and tamper-evident without granting placement
// authority.
func ValidatePlacementRecalculatedAdviceSupersessionReceipt(
	supersession PlacementRecalculatedAdviceSupersessionReceipt,
	gate PlacementRebuildSatisfactionGate,
	receipt PlacementRebuildEvidenceReceipt,
	sourceGate PlacementResourceReuseGate,
	sourceFreshness PlacementResourceHeadroomFreshnessGate,
	sourceEnvelope PlacementResourceHeadroomEnvelope,
	request PlacementRequest,
	currentInputs []PlacementNodeDerivationInput,
	recalculatedAt time.Time,
) error {
	expected, err := BuildPlacementRecalculatedAdviceSupersessionReceipt(
		gate,
		receipt,
		sourceGate,
		sourceFreshness,
		sourceEnvelope,
		request,
		currentInputs,
		recalculatedAt,
	)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(expected, supersession) {
		return fmt.Errorf(
			"%w: recalculated placement supersession receipt was modified",
			ErrInvalidRecommendation,
		)
	}
	return nil
}

func validatePlacementRecalculatedAdviceSupersessionReceipt(
	receipt PlacementRecalculatedAdviceSupersessionReceipt,
) error {
	if receipt.SchemaVersion != PlacementRecalculatedAdviceSupersessionSchemaV1 ||
		!receipt.SupersedesSource || !receipt.FreshEvidenceRecalculated ||
		receipt.SourceReuseAuthorized || receipt.ReuseAuthorized ||
		receipt.PlacementAuthorized || !receipt.AdvisoryOnly || receipt.ProductionMutation {
		return fmt.Errorf("%w: unsafe recalculated advice supersession", ErrInvalidRecommendation)
	}
	if receipt.SupersessionID == "" || receipt.SourceReceiptID == "" ||
		receipt.SatisfactionGateID == "" || receipt.SourceDerivedSnapshotID == "" ||
		receipt.SourcePlacementSnapshotID == "" || receipt.SourceAdviceID == "" ||
		receipt.NewDerivedSnapshotID == "" || receipt.NewPlacementSnapshotID == "" ||
		receipt.NewAdviceID == "" || receipt.NewHeadroomEnvelopeID == "" ||
		receipt.ScopeID == "" || receipt.RecalculatedAt.IsZero() ||
		receipt.RecalculatedAt.Location() != time.UTC || receipt.Reason == "" ||
		receipt.RecommendedAction == "" || string(receipt.Action) == "" {
		return fmt.Errorf("%w: incomplete recalculated advice supersession", ErrInvalidRecommendation)
	}
	if receipt.NewDerivedSnapshotID == receipt.SourceDerivedSnapshotID {
		return fmt.Errorf("%w: supersession must use fresh derived evidence", ErrInvalidRecommendation)
	}
	if receipt.Reason != "fresh-evidence-advice-recalculated" ||
		receipt.RecommendedAction != "review-recalculated-advisory-placement" {
		return fmt.Errorf("%w: invalid recalculated advice disposition", ErrInvalidRecommendation)
	}
	if placementRecalculatedAdviceSupersessionID(receipt) != receipt.SupersessionID {
		return fmt.Errorf("%w: recalculated advice supersession integrity mismatch", ErrInvalidRecommendation)
	}
	return nil
}

func placementRecalculatedAdviceSupersessionID(
	receipt PlacementRecalculatedAdviceSupersessionReceipt,
) string {
	canonical := receipt
	canonical.SupersessionID = ""
	encoded, _ := json.Marshal(canonical)
	return "pras-" + placementEvidenceSHA256Hex(encoded)[:24]
}
