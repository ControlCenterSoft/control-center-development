package capacity

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"time"

	"control-center/internal/agent"
)

const PlacementRebuildEvidenceReceiptSchemaV1 = "capacity.placement-rebuild-evidence-receipt/v1"

const (
	PlacementRebuildEvidenceCurrentTelemetry = "current-measured-telemetry"
	PlacementRebuildEvidenceLineage          = "profile-and-telemetry-lineage"
)

const (
	PlacementRebuildArtifactNodeProjections  = "node-projections"
	PlacementRebuildArtifactDerivedEvidence  = "derived-placement-evidence"
	PlacementRebuildArtifactHeadroomEnvelope = "resource-headroom-envelope"
	PlacementRebuildArtifactFreshnessGate    = "resource-freshness-gate"
	PlacementRebuildArtifactPlacementAdvice  = "placement-advice"
)

// PlacementRebuildEvidenceRequirement explains which exact resource evidence
// must be refreshed before a blocked placement recommendation may be rebuilt.
type PlacementRebuildEvidenceRequirement struct {
	NodeID           string                          `json:"node_id"`
	ConstraintID     string                          `json:"constraint_id"`
	Metric           agent.CapacityMetric            `json:"metric"`
	TargetID         string                          `json:"target_id"`
	CurrentStatus    PlacementResourceEvidenceStatus `json:"current_status"`
	Reason           string                          `json:"reason"`
	RequiredEvidence string                          `json:"required_evidence"`
}

// PlacementRebuildEvidenceReceipt is deterministic, explanatory rebuild
// evidence. It deliberately carries no authority to rebuild or place anything.
type PlacementRebuildEvidenceReceipt struct {
	SchemaVersion           string                                `json:"schema_version"`
	ReceiptID               string                                `json:"receipt_id"`
	ResourceReuseGateID     string                                `json:"resource_reuse_gate_id"`
	ResourceFreshnessGateID string                                `json:"resource_freshness_gate_id"`
	HeadroomEnvelopeID      string                                `json:"headroom_envelope_id"`
	DerivedSnapshotID       string                                `json:"derived_snapshot_id"`
	PlacementSnapshotID     string                                `json:"placement_snapshot_id"`
	AdviceID                string                                `json:"advice_id"`
	ScopeID                 string                                `json:"scope_id"`
	CheckedAt               time.Time                             `json:"checked_at"`
	BlockedBy               string                                `json:"blocked_by"`
	Requirements            []PlacementRebuildEvidenceRequirement `json:"requirements"`
	RequiredArtifacts       []string                              `json:"required_artifacts"`
	RecommendedAction       string                                `json:"recommended_action"`
	RebuildRequired         bool                                  `json:"rebuild_required"`
	ReuseAuthorized         bool                                  `json:"reuse_authorized"`
	RebuildAuthorized       bool                                  `json:"rebuild_authorized"`
	PlacementAuthorized     bool                                  `json:"placement_authorized"`
	AdvisoryOnly            bool                                  `json:"advisory_only"`
	ProductionMutation      bool                                  `json:"production_mutation"`
}

// BuildPlacementRebuildEvidenceReceipt converts a blocked resource/lineage
// reuse verdict into an exact evidence acquisition plan. The result only tells
// a caller what must be refreshed; it cannot trigger collection, rebuild, or
// placement by itself.
func BuildPlacementRebuildEvidenceReceipt(
	gate PlacementResourceReuseGate,
	freshness PlacementResourceHeadroomFreshnessGate,
	envelope PlacementResourceHeadroomEnvelope,
) (PlacementRebuildEvidenceReceipt, error) {
	if err := validatePlacementResourceReuseGate(gate); err != nil {
		return PlacementRebuildEvidenceReceipt{}, err
	}
	if gate.Status != PlacementAdviceReuseBlocked || !gate.RebuildRequired || gate.ReuseAllowed {
		return PlacementRebuildEvidenceReceipt{}, fmt.Errorf(
			"%w: rebuild receipt requires a blocked reuse gate",
			ErrInvalidRecommendation,
		)
	}
	if gate.BlockedBy != PlacementResourceReuseBlockResource && gate.BlockedBy != PlacementResourceReuseBlockLineage {
		return PlacementRebuildEvidenceReceipt{}, fmt.Errorf(
			"%w: rebuild receipt only accepts resource or lineage blocks",
			ErrInvalidRecommendation,
		)
	}
	if err := validateRebuildFreshnessEvidence(gate, freshness, envelope); err != nil {
		return PlacementRebuildEvidenceReceipt{}, err
	}

	requirements, err := placementRebuildEvidenceRequirements(gate, freshness, envelope)
	if err != nil {
		return PlacementRebuildEvidenceReceipt{}, err
	}
	artifacts := []string{
		PlacementRebuildArtifactNodeProjections,
		PlacementRebuildArtifactDerivedEvidence,
		PlacementRebuildArtifactHeadroomEnvelope,
		PlacementRebuildArtifactFreshnessGate,
		PlacementRebuildArtifactPlacementAdvice,
	}
	result := PlacementRebuildEvidenceReceipt{
		SchemaVersion:           PlacementRebuildEvidenceReceiptSchemaV1,
		ResourceReuseGateID:     gate.GateID,
		ResourceFreshnessGateID: freshness.GateID,
		HeadroomEnvelopeID:      envelope.EnvelopeID,
		DerivedSnapshotID:       gate.DerivedSnapshotID,
		PlacementSnapshotID:     gate.PlacementSnapshotID,
		AdviceID:                gate.AdviceID,
		ScopeID:                 gate.ScopeID,
		CheckedAt:               gate.CheckedAt,
		BlockedBy:               gate.BlockedBy,
		Requirements:            requirements,
		RequiredArtifacts:       artifacts,
		RecommendedAction:       "collect-evidence-and-rebuild-advice",
		RebuildRequired:         true,
		ReuseAuthorized:         false,
		RebuildAuthorized:       false,
		PlacementAuthorized:     false,
		AdvisoryOnly:            true,
		ProductionMutation:      false,
	}
	result.ReceiptID = placementRebuildEvidenceReceiptID(result)
	if err := validatePlacementRebuildEvidenceReceipt(result); err != nil {
		return PlacementRebuildEvidenceReceipt{}, err
	}
	return result, nil
}

// ValidatePlacementRebuildEvidenceReceipt reconstructs the expected receipt
// from exact blocked evidence and rejects changed requirements or authority.
func ValidatePlacementRebuildEvidenceReceipt(
	receipt PlacementRebuildEvidenceReceipt,
	gate PlacementResourceReuseGate,
	freshness PlacementResourceHeadroomFreshnessGate,
	envelope PlacementResourceHeadroomEnvelope,
) error {
	expected, err := BuildPlacementRebuildEvidenceReceipt(gate, freshness, envelope)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(expected, receipt) {
		return fmt.Errorf(
			"%w: placement rebuild evidence receipt was modified",
			ErrInvalidRecommendation,
		)
	}
	return nil
}

func validateRebuildFreshnessEvidence(
	gate PlacementResourceReuseGate,
	freshness PlacementResourceHeadroomFreshnessGate,
	envelope PlacementResourceHeadroomEnvelope,
) error {
	if freshness.SchemaVersion != PlacementResourceHeadroomFreshnessSchemaV1 ||
		!freshness.AdvisoryOnly || freshness.PlacementAuthorized || freshness.ProductionMutation ||
		freshness.CheckedAt.IsZero() || freshness.CheckedAt.Location() != time.UTC ||
		freshness.GateID != placementResourceHeadroomFreshnessGateID(freshness) {
		return fmt.Errorf("%w: invalid resource freshness evidence", ErrInvalidRecommendation)
	}
	if envelope.SchemaVersion != PlacementResourceHeadroomSchemaV1 || !envelope.AdvisoryOnly ||
		envelope.PlacementAuthorized || envelope.ProductionMutation ||
		envelope.EnvelopeID != placementRebuildHeadroomEnvelopeID(envelope) {
		return fmt.Errorf("%w: unsafe resource headroom envelope", ErrInvalidRecommendation)
	}
	if len(freshness.Candidates) != len(envelope.Candidates) {
		return fmt.Errorf("%w: freshness candidate cardinality mismatch", ErrInvalidRecommendation)
	}
	if gate.ResourceFreshnessGateID != freshness.GateID || gate.HeadroomEnvelopeID != envelope.EnvelopeID ||
		freshness.HeadroomEnvelopeID != envelope.EnvelopeID || !gate.CheckedAt.Equal(freshness.CheckedAt) ||
		gate.DerivedSnapshotID != freshness.DerivedSnapshotID ||
		gate.PlacementSnapshotID != freshness.PlacementSnapshotID ||
		gate.AdviceID != freshness.AdviceID || gate.ScopeID != freshness.ScopeID ||
		gate.DerivedSnapshotID != envelope.DerivedSnapshotID ||
		gate.PlacementSnapshotID != envelope.PlacementSnapshotID ||
		gate.AdviceID != envelope.AdviceID || gate.ScopeID != envelope.ScopeID {
		return fmt.Errorf("%w: rebuild evidence lineage mismatch", ErrInvalidRecommendation)
	}
	return nil
}

func placementRebuildEvidenceRequirements(
	gate PlacementResourceReuseGate,
	freshness PlacementResourceHeadroomFreshnessGate,
	envelope PlacementResourceHeadroomEnvelope,
) ([]PlacementRebuildEvidenceRequirement, error) {
	freshnessByNode := make(map[string]PlacementResourceHeadroomFreshnessCandidate, len(freshness.Candidates))
	for _, candidate := range freshness.Candidates {
		if _, exists := freshnessByNode[candidate.NodeID]; exists {
			return nil, fmt.Errorf("%w: duplicate freshness candidate", ErrInvalidRecommendation)
		}
		freshnessByNode[candidate.NodeID] = candidate
	}

	requirements := make([]PlacementRebuildEvidenceRequirement, 0)
	for _, candidate := range envelope.Candidates {
		assessment, ok := freshnessByNode[candidate.NodeID]
		if !ok {
			return nil, fmt.Errorf(
				"%w: candidate %q has no freshness evidence",
				ErrInvalidRecommendation,
				candidate.NodeID,
			)
		}
		if gate.BlockedBy == PlacementResourceReuseBlockResource &&
			assessment.Status == PlacementResourceEvidenceCurrent {
			continue
		}

		stale := placementRebuildStringSet(assessment.StaleConstraintIDs)
		nonMeasured := placementRebuildStringSet(assessment.NonMeasuredConstraintIDs)
		for _, resource := range candidate.Resources {
			reason := "lineage-revalidation-required"
			requiredEvidence := PlacementRebuildEvidenceLineage
			status := assessment.Status
			if gate.BlockedBy == PlacementResourceReuseBlockResource {
				switch {
				case stale[resource.ConstraintID]:
					reason = "constraint-observation-stale"
					requiredEvidence = PlacementRebuildEvidenceCurrentTelemetry
				case nonMeasured[resource.ConstraintID]:
					reason = "constraint-evidence-not-measured"
					requiredEvidence = PlacementRebuildEvidenceCurrentTelemetry
				case assessment.Status == PlacementResourceEvidenceDegraded:
					reason = "candidate-confidence-degraded"
					requiredEvidence = PlacementRebuildEvidenceCurrentTelemetry
				default:
					continue
				}
			}
			requirements = append(requirements, PlacementRebuildEvidenceRequirement{
				NodeID:           candidate.NodeID,
				ConstraintID:     resource.ConstraintID,
				Metric:           resource.Metric,
				TargetID:         resource.TargetID,
				CurrentStatus:    status,
				Reason:           reason,
				RequiredEvidence: requiredEvidence,
			})
		}
	}
	if len(requirements) == 0 {
		return nil, fmt.Errorf("%w: rebuild receipt has no evidence requirements", ErrInvalidRecommendation)
	}
	sort.Slice(requirements, func(i, j int) bool {
		if requirements[i].NodeID == requirements[j].NodeID {
			if requirements[i].ConstraintID == requirements[j].ConstraintID {
				return requirements[i].RequiredEvidence < requirements[j].RequiredEvidence
			}
			return requirements[i].ConstraintID < requirements[j].ConstraintID
		}
		return requirements[i].NodeID < requirements[j].NodeID
	})
	return requirements, nil
}

func placementRebuildHeadroomEnvelopeID(envelope PlacementResourceHeadroomEnvelope) string {
	canonical := envelope
	canonical.EnvelopeID = ""
	encoded, _ := json.Marshal(canonical)
	return "prh-" + placementEvidenceSHA256Hex(encoded)[:24]
}

func placementRebuildStringSet(values []string) map[string]bool {
	result := make(map[string]bool, len(values))
	for _, value := range values {
		result[value] = true
	}
	return result
}

func validatePlacementRebuildEvidenceReceipt(receipt PlacementRebuildEvidenceReceipt) error {
	if receipt.SchemaVersion != PlacementRebuildEvidenceReceiptSchemaV1 || !receipt.AdvisoryOnly ||
		receipt.ReuseAuthorized || receipt.RebuildAuthorized || receipt.PlacementAuthorized ||
		receipt.ProductionMutation || !receipt.RebuildRequired {
		return fmt.Errorf("%w: unsafe rebuild evidence receipt", ErrInvalidRecommendation)
	}
	if receipt.CheckedAt.IsZero() || receipt.CheckedAt.Location() != time.UTC ||
		receipt.ReceiptID == "" || receipt.ResourceReuseGateID == "" ||
		receipt.ResourceFreshnessGateID == "" || receipt.HeadroomEnvelopeID == "" ||
		receipt.DerivedSnapshotID == "" || receipt.PlacementSnapshotID == "" ||
		receipt.AdviceID == "" || receipt.ScopeID == "" || len(receipt.Requirements) == 0 {
		return fmt.Errorf("%w: incomplete rebuild evidence receipt", ErrInvalidRecommendation)
	}
	if receipt.BlockedBy != PlacementResourceReuseBlockResource &&
		receipt.BlockedBy != PlacementResourceReuseBlockLineage {
		return fmt.Errorf("%w: invalid rebuild block reason", ErrInvalidRecommendation)
	}
	if receipt.RecommendedAction != "collect-evidence-and-rebuild-advice" ||
		!reflect.DeepEqual(receipt.RequiredArtifacts, []string{
			PlacementRebuildArtifactNodeProjections,
			PlacementRebuildArtifactDerivedEvidence,
			PlacementRebuildArtifactHeadroomEnvelope,
			PlacementRebuildArtifactFreshnessGate,
			PlacementRebuildArtifactPlacementAdvice,
		}) {
		return fmt.Errorf("%w: invalid rebuild action plan", ErrInvalidRecommendation)
	}
	for _, requirement := range receipt.Requirements {
		if requirement.NodeID == "" || requirement.ConstraintID == "" || requirement.Metric == "" ||
			requirement.TargetID == "" || requirement.Reason == "" {
			return fmt.Errorf("%w: incomplete rebuild evidence requirement", ErrInvalidRecommendation)
		}
		if requirement.RequiredEvidence != PlacementRebuildEvidenceCurrentTelemetry &&
			requirement.RequiredEvidence != PlacementRebuildEvidenceLineage {
			return fmt.Errorf("%w: invalid required evidence kind", ErrInvalidRecommendation)
		}
	}
	if placementRebuildEvidenceReceiptID(receipt) != receipt.ReceiptID {
		return fmt.Errorf("%w: rebuild evidence receipt integrity mismatch", ErrInvalidRecommendation)
	}
	return nil
}

func placementRebuildEvidenceReceiptID(receipt PlacementRebuildEvidenceReceipt) string {
	canonical := receipt
	canonical.ReceiptID = ""
	encoded, _ := json.Marshal(canonical)
	return "prer-" + placementEvidenceSHA256Hex(encoded)[:24]
}
