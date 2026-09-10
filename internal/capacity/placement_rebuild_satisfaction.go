package capacity

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"time"

	"control-center/internal/agent"
)

const PlacementRebuildSatisfactionSchemaV1 = "capacity.placement-rebuild-satisfaction/v1"

const (
	PlacementRebuildSatisfactionSatisfied = "satisfied"
	PlacementRebuildSatisfactionBlocked   = "blocked"
)

type PlacementRebuildSatisfactionCheck struct {
	NodeID           string                    `json:"node_id"`
	ConstraintID     string                    `json:"constraint_id"`
	Metric           agent.CapacityMetric      `json:"metric"`
	TargetID         string                    `json:"target_id"`
	RequiredEvidence string                    `json:"required_evidence"`
	ObservedEvidence agent.ObservationEvidence `json:"observed_evidence"`
	Satisfied        bool                      `json:"satisfied"`
	Reason           string                    `json:"reason"`
}

// PlacementRebuildSatisfactionGate proves that a previously blocked placement
// recommendation has received a complete fresh/measured input set. It only
// permits recalculating new advisory advice; it never revives reuse of the old
// recommendation and never authorizes placement or production mutation.
type PlacementRebuildSatisfactionGate struct {
	SchemaVersion                string                              `json:"schema_version"`
	GateID                       string                              `json:"gate_id"`
	ReceiptID                    string                              `json:"receipt_id"`
	SourceResourceReuseGateID    string                              `json:"source_resource_reuse_gate_id"`
	SourceHeadroomEnvelopeID     string                              `json:"source_headroom_envelope_id"`
	SourceDerivedSnapshotID      string                              `json:"source_derived_snapshot_id"`
	ScopeID                      string                              `json:"scope_id"`
	CheckedAt                    time.Time                           `json:"checked_at"`
	Checks                       []PlacementRebuildSatisfactionCheck `json:"checks"`
	Status                       string                              `json:"status"`
	Reason                       string                              `json:"reason"`
	RecommendedAction            string                              `json:"recommended_action"`
	AllRequirementsSatisfied     bool                                `json:"all_requirements_satisfied"`
	AdviceRecalculationPermitted bool                                `json:"advice_recalculation_permitted"`
	ReuseAuthorized              bool                                `json:"reuse_authorized"`
	PlacementAuthorized          bool                                `json:"placement_authorized"`
	AdvisoryOnly                 bool                                `json:"advisory_only"`
	ProductionMutation           bool                                `json:"production_mutation"`
}

// BuildPlacementRebuildSatisfactionGate validates the sealed rebuild receipt
// against its exact source evidence, then verifies a complete same-scope set of
// current node derivations. Every source resource constraint must still exist,
// every supplied derivation must be exactly reproducible at checkedAt, and all
// observations must be measured. A satisfied gate grants only permission to
// recalculate new advisory placement advice.
func BuildPlacementRebuildSatisfactionGate(
	receipt PlacementRebuildEvidenceReceipt,
	sourceGate PlacementResourceReuseGate,
	sourceFreshness PlacementResourceHeadroomFreshnessGate,
	sourceEnvelope PlacementResourceHeadroomEnvelope,
	currentInputs []PlacementNodeDerivationInput,
	checkedAt time.Time,
) (PlacementRebuildSatisfactionGate, error) {
	if err := ValidatePlacementRebuildEvidenceReceipt(
		receipt,
		sourceGate,
		sourceFreshness,
		sourceEnvelope,
	); err != nil {
		return PlacementRebuildSatisfactionGate{}, err
	}
	if checkedAt.IsZero() {
		return PlacementRebuildSatisfactionGate{}, fmt.Errorf(
			"%w: checked_at is required",
			ErrInvalidRecommendation,
		)
	}
	checkedAt = checkedAt.UTC()
	if checkedAt.Before(receipt.CheckedAt) {
		return PlacementRebuildSatisfactionGate{}, fmt.Errorf(
			"%w: satisfaction check precedes rebuild receipt",
			ErrInvalidRecommendation,
		)
	}
	if len(currentInputs) != len(sourceEnvelope.Candidates) {
		return PlacementRebuildSatisfactionGate{}, fmt.Errorf(
			"%w: current input set changed placement candidate cardinality",
			ErrInvalidRecommendation,
		)
	}

	required := make(map[string]PlacementRebuildEvidenceRequirement, len(receipt.Requirements))
	for _, requirement := range receipt.Requirements {
		key := placementRebuildSatisfactionKey(
			requirement.NodeID,
			requirement.ConstraintID,
		)
		if _, duplicate := required[key]; duplicate {
			return PlacementRebuildSatisfactionGate{}, fmt.Errorf(
				"%w: rebuild receipt repeats a requirement",
				ErrInvalidRecommendation,
			)
		}
		required[key] = requirement
	}

	sourceByNode := make(map[string]PlacementResourceHeadroomCandidate, len(sourceEnvelope.Candidates))
	for _, candidate := range sourceEnvelope.Candidates {
		if _, duplicate := sourceByNode[candidate.NodeID]; duplicate {
			return PlacementRebuildSatisfactionGate{}, fmt.Errorf(
				"%w: source envelope repeats a candidate",
				ErrInvalidRecommendation,
			)
		}
		sourceByNode[candidate.NodeID] = candidate
	}

	checks := make([]PlacementRebuildSatisfactionCheck, 0)
	seenNodes := make(map[string]struct{}, len(currentInputs))
	allSatisfied := true
	for _, input := range currentInputs {
		profile, err := NormalizeProfile(input.Profile)
		if err != nil {
			return PlacementRebuildSatisfactionGate{}, err
		}
		nodeID := profile.Subject.ID
		if _, duplicate := seenNodes[nodeID]; duplicate {
			return PlacementRebuildSatisfactionGate{}, fmt.Errorf(
				"%w: duplicate current input for node %q",
				ErrInvalidRecommendation,
				nodeID,
			)
		}
		seenNodes[nodeID] = struct{}{}

		sourceCandidate, ok := sourceByNode[nodeID]
		if !ok {
			return PlacementRebuildSatisfactionGate{}, fmt.Errorf(
				"%w: current input introduced node %q outside rebuild scope",
				ErrInvalidRecommendation,
				nodeID,
			)
		}
		if len(profile.Constraints) != len(sourceCandidate.Resources) {
			return PlacementRebuildSatisfactionGate{}, fmt.Errorf(
				"%w: node %q changed resource constraint cardinality",
				ErrInvalidRecommendation,
				nodeID,
			)
		}

		canonicalTelemetry, err := normalizeNodeProjectionTelemetry(input.Telemetry, profile, checkedAt)
		if err != nil {
			return PlacementRebuildSatisfactionGate{}, fmt.Errorf(
				"current telemetry %q: %w",
				nodeID,
				err,
			)
		}
		expectedDerivation, err := DeriveNodeProjection(profile, canonicalTelemetry, checkedAt)
		if err != nil {
			return PlacementRebuildSatisfactionGate{}, fmt.Errorf(
				"current derivation %q: %w",
				nodeID,
				err,
			)
		}
		if expectedDerivation != input.Evidence {
			return PlacementRebuildSatisfactionGate{}, fmt.Errorf(
				"%w: node %q current derivation evidence does not match checked_at inputs",
				ErrInvalidRecommendation,
				nodeID,
			)
		}

		constraintByID := make(map[string]Constraint, len(profile.Constraints))
		for _, constraint := range profile.Constraints {
			constraintByID[constraint.ID] = constraint
		}
		observationByKey := make(map[string]agent.CapacityObservation, len(canonicalTelemetry.Observations))
		for _, observation := range canonicalTelemetry.Observations {
			key := projectionConstraintKey(observation.Metric, observation.TargetID)
			observationByKey[key] = observation
		}

		for _, resource := range sourceCandidate.Resources {
			constraint, ok := constraintByID[resource.ConstraintID]
			if !ok || constraint.Metric != resource.Metric || constraint.TargetID != resource.TargetID ||
				constraint.Unit != resource.Unit {
				return PlacementRebuildSatisfactionGate{}, fmt.Errorf(
					"%w: node %q changed constraint lineage for %q",
					ErrInvalidRecommendation,
					nodeID,
					resource.ConstraintID,
				)
			}
			observation, ok := observationByKey[projectionConstraintKey(resource.Metric, resource.TargetID)]
			if !ok {
				return PlacementRebuildSatisfactionGate{}, fmt.Errorf(
					"%w: node %q lacks current observation for %q",
					ErrInvalidRecommendation,
					nodeID,
					resource.ConstraintID,
				)
			}

			requirement, explicitlyRequired := required[placementRebuildSatisfactionKey(nodeID, resource.ConstraintID)]
			requiredEvidence := PlacementRebuildEvidenceCurrentTelemetry
			if explicitlyRequired {
				if requirement.Metric != resource.Metric || requirement.TargetID != resource.TargetID {
					return PlacementRebuildSatisfactionGate{}, fmt.Errorf(
						"%w: rebuild requirement lineage mismatch",
						ErrInvalidRecommendation,
					)
				}
				requiredEvidence = requirement.RequiredEvidence
			}
			satisfied := observation.Evidence == agent.EvidenceMeasured
			reason := "exact-current-measured-evidence"
			if !satisfied {
				reason = "current-evidence-not-measured"
				allSatisfied = false
			}
			checks = append(checks, PlacementRebuildSatisfactionCheck{
				NodeID:           nodeID,
				ConstraintID:     resource.ConstraintID,
				Metric:           resource.Metric,
				TargetID:         resource.TargetID,
				RequiredEvidence: requiredEvidence,
				ObservedEvidence: observation.Evidence,
				Satisfied:        satisfied,
				Reason:           reason,
			})
		}
	}
	if len(seenNodes) != len(sourceByNode) {
		return PlacementRebuildSatisfactionGate{}, fmt.Errorf(
			"%w: current input set omitted a rebuild-scope node",
			ErrInvalidRecommendation,
		)
	}

	sort.Slice(checks, func(i, j int) bool {
		if checks[i].NodeID == checks[j].NodeID {
			return checks[i].ConstraintID < checks[j].ConstraintID
		}
		return checks[i].NodeID < checks[j].NodeID
	})
	result := PlacementRebuildSatisfactionGate{
		SchemaVersion:                PlacementRebuildSatisfactionSchemaV1,
		ReceiptID:                    receipt.ReceiptID,
		SourceResourceReuseGateID:    sourceGate.GateID,
		SourceHeadroomEnvelopeID:     sourceEnvelope.EnvelopeID,
		SourceDerivedSnapshotID:      sourceEnvelope.DerivedSnapshotID,
		ScopeID:                      receipt.ScopeID,
		CheckedAt:                    checkedAt,
		Checks:                       checks,
		Status:                       PlacementRebuildSatisfactionBlocked,
		Reason:                       "current-rebuild-evidence-incomplete",
		RecommendedAction:            "collect-current-measured-evidence",
		AllRequirementsSatisfied:     false,
		AdviceRecalculationPermitted: false,
		ReuseAuthorized:              false,
		PlacementAuthorized:          false,
		AdvisoryOnly:                 true,
		ProductionMutation:           false,
	}
	if allSatisfied {
		result.Status = PlacementRebuildSatisfactionSatisfied
		result.Reason = "exact-current-rebuild-evidence-satisfied"
		result.RecommendedAction = "recalculate-advisory-placement"
		result.AllRequirementsSatisfied = true
		result.AdviceRecalculationPermitted = true
	}
	result.GateID = placementRebuildSatisfactionGateID(result)
	if err := validatePlacementRebuildSatisfactionGate(result); err != nil {
		return PlacementRebuildSatisfactionGate{}, err
	}
	return result, nil
}

func ValidatePlacementRebuildSatisfactionGate(
	gate PlacementRebuildSatisfactionGate,
	receipt PlacementRebuildEvidenceReceipt,
	sourceGate PlacementResourceReuseGate,
	sourceFreshness PlacementResourceHeadroomFreshnessGate,
	sourceEnvelope PlacementResourceHeadroomEnvelope,
	currentInputs []PlacementNodeDerivationInput,
	checkedAt time.Time,
) error {
	expected, err := BuildPlacementRebuildSatisfactionGate(
		receipt,
		sourceGate,
		sourceFreshness,
		sourceEnvelope,
		currentInputs,
		checkedAt,
	)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(expected, gate) {
		return fmt.Errorf(
			"%w: placement rebuild satisfaction gate was modified",
			ErrInvalidRecommendation,
		)
	}
	return nil
}

func validatePlacementRebuildSatisfactionGate(gate PlacementRebuildSatisfactionGate) error {
	if gate.SchemaVersion != PlacementRebuildSatisfactionSchemaV1 || !gate.AdvisoryOnly ||
		gate.ReuseAuthorized || gate.PlacementAuthorized || gate.ProductionMutation {
		return fmt.Errorf("%w: unsafe placement rebuild satisfaction gate", ErrInvalidRecommendation)
	}
	if gate.GateID == "" || gate.ReceiptID == "" || gate.SourceResourceReuseGateID == "" ||
		gate.SourceHeadroomEnvelopeID == "" || gate.SourceDerivedSnapshotID == "" ||
		gate.ScopeID == "" || gate.CheckedAt.IsZero() || gate.CheckedAt.Location() != time.UTC ||
		len(gate.Checks) == 0 || gate.Reason == "" || gate.RecommendedAction == "" {
		return fmt.Errorf("%w: incomplete placement rebuild satisfaction gate", ErrInvalidRecommendation)
	}
	allChecksSatisfied := true
	for _, check := range gate.Checks {
		if check.NodeID == "" || check.ConstraintID == "" || check.Metric == "" ||
			check.TargetID == "" || check.RequiredEvidence == "" || check.ObservedEvidence == "" ||
			check.Reason == "" {
			return fmt.Errorf("%w: incomplete rebuild satisfaction check", ErrInvalidRecommendation)
		}
		if check.RequiredEvidence != PlacementRebuildEvidenceCurrentTelemetry &&
			check.RequiredEvidence != PlacementRebuildEvidenceLineage {
			return fmt.Errorf("%w: invalid rebuild satisfaction evidence requirement", ErrInvalidRecommendation)
		}
		wantSatisfied := check.ObservedEvidence == agent.EvidenceMeasured
		wantReason := "current-evidence-not-measured"
		if wantSatisfied {
			wantReason = "exact-current-measured-evidence"
		} else {
			allChecksSatisfied = false
		}
		if check.Satisfied != wantSatisfied || check.Reason != wantReason {
			return fmt.Errorf("%w: inconsistent rebuild satisfaction check", ErrInvalidRecommendation)
		}
	}
	if gate.AllRequirementsSatisfied != allChecksSatisfied ||
		gate.AdviceRecalculationPermitted != allChecksSatisfied {
		return fmt.Errorf("%w: rebuild satisfaction verdict does not match checks", ErrInvalidRecommendation)
	}
	switch gate.Status {
	case PlacementRebuildSatisfactionSatisfied:
		if !gate.AllRequirementsSatisfied || !gate.AdviceRecalculationPermitted ||
			gate.Reason != "exact-current-rebuild-evidence-satisfied" ||
			gate.RecommendedAction != "recalculate-advisory-placement" {
			return fmt.Errorf("%w: inconsistent satisfied rebuild gate", ErrInvalidRecommendation)
		}
	case PlacementRebuildSatisfactionBlocked:
		if gate.AllRequirementsSatisfied || gate.AdviceRecalculationPermitted ||
			gate.Reason != "current-rebuild-evidence-incomplete" ||
			gate.RecommendedAction != "collect-current-measured-evidence" {
			return fmt.Errorf("%w: inconsistent blocked rebuild gate", ErrInvalidRecommendation)
		}
	default:
		return fmt.Errorf("%w: invalid rebuild satisfaction status", ErrInvalidRecommendation)
	}
	if placementRebuildSatisfactionGateID(gate) != gate.GateID {
		return fmt.Errorf("%w: rebuild satisfaction gate integrity mismatch", ErrInvalidRecommendation)
	}
	return nil
}

func placementRebuildSatisfactionKey(nodeID, constraintID string) string {
	return nodeID + "\x00" + constraintID
}

func placementRebuildSatisfactionGateID(gate PlacementRebuildSatisfactionGate) string {
	canonical := gate
	canonical.GateID = ""
	encoded, _ := json.Marshal(canonical)
	return "prsg-" + placementEvidenceSHA256Hex(encoded)[:24]
}
