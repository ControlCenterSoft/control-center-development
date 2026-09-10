package clusterha

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
)

var (
	ErrInvalidFencingReceipt = errors.New("invalid HA fencing receipt")
	ErrStaleFencingReceipt   = errors.New("stale HA fencing receipt")
)

const FencingReceiptSchemaV1 = "clusterha.fencing-receipt/v1"

// ReconcilerFencingObservation is trusted evidence from the reconciler/fencing
// adapter boundary. FenceObservationID must identify the exact completed fencing
// event reported by that adapter. The contract never invokes a fencing backend.
type ReconcilerFencingObservation struct {
	Membership              Snapshot `json:"membership"`
	ExpectedResourceVersion string   `json:"expected_resource_version"`
	ResourceVersion         string   `json:"resource_version"`
	TargetNodeID            string   `json:"target_node_id"`
	FenceObservationID      string   `json:"fence_observation_id"`
	Fenced                  bool     `json:"fenced"`
}

// FencingReceipt is immutable proof-only evidence that one unavailable
// controller was observed fenced and that the HA journal advanced exactly once.
// It grants no execution authority and performs no host or state mutation.
type FencingReceipt struct {
	SchemaVersion              string             `json:"schema_version"`
	ReceiptID                  string             `json:"receipt_id"`
	PreflightPlanID            string             `json:"preflight_plan_id"`
	ClusterID                  string             `json:"cluster_id"`
	ClusterGeneration          uint64             `json:"cluster_generation"`
	TargetNodeID               string             `json:"target_node_id"`
	FenceObservationID         string             `json:"fence_observation_id"`
	BeforeMembershipSnapshotID string             `json:"before_membership_snapshot_id"`
	AfterMembershipSnapshotID  string             `json:"after_membership_snapshot_id"`
	BeforeRevisionEvidenceID   string             `json:"before_revision_evidence_id"`
	AfterRevisionEvidenceID    string             `json:"after_revision_evidence_id"`
	BeforeRevision             TransitionRevision `json:"before_revision"`
	AfterRevision              TransitionRevision `json:"after_revision"`
	ProofOnly                  bool               `json:"proof_only"`
	ExecutionAuthorized        bool               `json:"execution_authorized"`
	HostMutation               bool               `json:"host_mutation"`
	StateMutation              bool               `json:"state_mutation"`
}

type FencingReceiptRequest struct {
	Preflight        LifecyclePreflightPlan       `json:"preflight"`
	PreflightRequest LifecyclePreflightRequest    `json:"preflight_request"`
	BeforeMembership Snapshot                     `json:"before_membership"`
	BeforeEvidence   TransitionRevisionEvidence   `json:"-"`
	AfterObservation ReconcilerFencingObservation `json:"after_observation"`
}

type FencingReceiptValidation struct {
	ReceiptID                 string             `json:"receipt_id"`
	PreflightPlanID           string             `json:"preflight_plan_id"`
	ClusterID                 string             `json:"cluster_id"`
	ClusterGeneration         uint64             `json:"cluster_generation"`
	TargetNodeID              string             `json:"target_node_id"`
	FenceObservationID        string             `json:"fence_observation_id"`
	AfterMembershipSnapshotID string             `json:"after_membership_snapshot_id"`
	AfterRevisionEvidenceID   string             `json:"after_revision_evidence_id"`
	AfterRevision             TransitionRevision `json:"after_revision"`
	Revalidated               bool               `json:"revalidated"`
	ProofOnly                 bool               `json:"proof_only"`
	ExecutionAuthorized       bool               `json:"execution_authorized"`
	HostMutation              bool               `json:"host_mutation"`
	StateMutation             bool               `json:"state_mutation"`
}

func BuildFencingReceipt(request FencingReceiptRequest) (FencingReceipt, error) {
	if err := validatePreflightEvidence(request.Preflight); err != nil {
		return FencingReceipt{}, fmt.Errorf("%w: preflight: %v", ErrInvalidFencingReceipt, err)
	}
	if !request.Preflight.RequiresFencing || request.Preflight.ClusterProfile == ProfileStandalone {
		return FencingReceipt{}, fmt.Errorf("%w: preflight does not require multi-node fencing", ErrInvalidFencingReceipt)
	}
	if request.Preflight.RequiresStandaloneDowntime {
		return FencingReceipt{}, fmt.Errorf("%w: fencing receipt is not valid for standalone downtime", ErrInvalidFencingReceipt)
	}

	rebuilt, err := BuildLifecyclePreflight(request.PreflightRequest)
	if err != nil {
		return FencingReceipt{}, fmt.Errorf("%w: preflight request no longer validates: %v", ErrInvalidFencingReceipt, err)
	}
	if !sameLifecyclePreflightEvidence(request.Preflight, rebuilt) || rebuilt.RequiresFencing != request.Preflight.RequiresFencing {
		return FencingReceipt{}, fmt.Errorf("%w: preflight evidence does not match supplied request", ErrInvalidFencingReceipt)
	}
	if !request.PreflightRequest.FencingConfirmed {
		return FencingReceipt{}, fmt.Errorf("%w: preflight request lacks fencing confirmation", ErrInvalidFencingReceipt)
	}
	if !sameSnapshot(request.PreflightRequest.Membership, request.BeforeMembership) {
		return FencingReceipt{}, fmt.Errorf("%w: preflight request does not match before membership", ErrInvalidFencingReceipt)
	}
	if request.Preflight.ClusterID != request.BeforeMembership.ClusterID || request.Preflight.ClusterGeneration != request.BeforeMembership.Generation {
		return FencingReceipt{}, fmt.Errorf("%w: preflight does not match cluster identity", ErrInvalidFencingReceipt)
	}
	if request.Preflight.NodeID != request.AfterObservation.TargetNodeID {
		return FencingReceipt{}, fmt.Errorf("%w: fencing target does not match lifecycle target", ErrInvalidFencingReceipt)
	}
	if err := validateIdentifier("target_node_id", request.AfterObservation.TargetNodeID); err != nil {
		return FencingReceipt{}, fmt.Errorf("%w: %v", ErrInvalidFencingReceipt, err)
	}
	if err := validateIdentifier("fence_observation_id", request.AfterObservation.FenceObservationID); err != nil {
		return FencingReceipt{}, fmt.Errorf("%w: %v", ErrInvalidFencingReceipt, err)
	}
	if !request.AfterObservation.Fenced {
		return FencingReceipt{}, fmt.Errorf("%w: fencing adapter did not confirm completion", ErrInvalidFencingReceipt)
	}

	_, beforeMembers, err := validateSnapshot(request.BeforeMembership)
	if err != nil {
		return FencingReceipt{}, fmt.Errorf("%w: before membership: %v", ErrInvalidFencingReceipt, err)
	}
	target, exists := beforeMembers[request.AfterObservation.TargetNodeID]
	if !exists || target.Kind != MemberController || target.Healthy {
		return FencingReceipt{}, fmt.Errorf("%w: fencing target must be an unavailable controller", ErrInvalidFencingReceipt)
	}
	if err := validateTransitionRevisionEvidence(request.BeforeEvidence); err != nil {
		return FencingReceipt{}, fmt.Errorf("%w: before revision evidence: %v", ErrInvalidFencingReceipt, err)
	}
	beforeSnapshotID, err := membershipSnapshotID(request.BeforeMembership)
	if err != nil {
		return FencingReceipt{}, err
	}
	if err := validateRevisionEvidenceBinding(request.BeforeEvidence, request.BeforeMembership, beforeSnapshotID); err != nil {
		return FencingReceipt{}, fmt.Errorf("%w: before revision binding: %v", ErrInvalidFencingReceipt, err)
	}

	afterMembership := request.AfterObservation.Membership
	if afterMembership.ClusterID != request.BeforeMembership.ClusterID || afterMembership.Generation != request.BeforeMembership.Generation {
		return FencingReceipt{}, fmt.Errorf("%w: fencing must not change cluster identity or generation", ErrInvalidFencingReceipt)
	}
	if afterMembership.LeaderID != request.BeforeMembership.LeaderID {
		return FencingReceipt{}, fmt.Errorf("%w: fencing receipt cannot include a leader transition", ErrInvalidFencingReceipt)
	}
	if !sameSnapshot(request.BeforeMembership, afterMembership) {
		return FencingReceipt{}, fmt.Errorf("%w: fencing must not mutate membership or readiness", ErrInvalidFencingReceipt)
	}
	if request.AfterObservation.ResourceVersion == request.BeforeEvidence.Revision().ResourceVersion {
		return FencingReceipt{}, fmt.Errorf("%w: fencing must advance resource version", ErrInvalidFencingReceipt)
	}

	afterEvidence, err := AdvanceTransitionRevisionEvidence(request.BeforeEvidence, ReconcilerRevisionObservation{
		Membership:              afterMembership,
		ExpectedResourceVersion: request.AfterObservation.ExpectedResourceVersion,
		ResourceVersion:         request.AfterObservation.ResourceVersion,
	})
	if err != nil {
		return FencingReceipt{}, fmt.Errorf("%w: revision advance: %v", ErrInvalidFencingReceipt, err)
	}
	beforeRevision := request.BeforeEvidence.Revision()
	afterRevision := afterEvidence.Revision()
	if beforeRevision.JournalSequence == math.MaxUint64 {
		return FencingReceipt{}, fmt.Errorf("%w: before journal cannot advance", ErrInvalidFencingReceipt)
	}
	if afterRevision.LeaderEpoch != beforeRevision.LeaderEpoch || afterRevision.JournalSequence != beforeRevision.JournalSequence+1 {
		return FencingReceipt{}, fmt.Errorf("%w: fencing must advance journal exactly once without changing leader epoch", ErrInvalidFencingReceipt)
	}

	afterSnapshotID, err := membershipSnapshotID(afterMembership)
	if err != nil {
		return FencingReceipt{}, err
	}
	if err := validateRevisionEvidenceBinding(afterEvidence, afterMembership, afterSnapshotID); err != nil {
		return FencingReceipt{}, fmt.Errorf("%w: after revision binding: %v", ErrInvalidFencingReceipt, err)
	}

	receipt := FencingReceipt{
		SchemaVersion:              FencingReceiptSchemaV1,
		PreflightPlanID:            request.Preflight.PlanID,
		ClusterID:                  request.Preflight.ClusterID,
		ClusterGeneration:          request.Preflight.ClusterGeneration,
		TargetNodeID:               request.AfterObservation.TargetNodeID,
		FenceObservationID:         request.AfterObservation.FenceObservationID,
		BeforeMembershipSnapshotID: beforeSnapshotID,
		AfterMembershipSnapshotID:  afterSnapshotID,
		BeforeRevisionEvidenceID:   request.BeforeEvidence.EvidenceID(),
		AfterRevisionEvidenceID:    afterEvidence.EvidenceID(),
		BeforeRevision:             beforeRevision,
		AfterRevision:              afterRevision,
		ProofOnly:                  true,
		ExecutionAuthorized:        false,
		HostMutation:               false,
		StateMutation:              false,
	}
	receiptID, err := fencingReceiptID(receipt)
	if err != nil {
		return FencingReceipt{}, err
	}
	receipt.ReceiptID = receiptID
	return receipt, nil
}

// RevalidateFencingReceipt proves that the exact post-fencing membership and
// HA journal revision remain current. Later failover, health, membership or
// journal changes make the receipt stale.
func RevalidateFencingReceipt(receipt FencingReceipt, currentMembership Snapshot, currentEvidence TransitionRevisionEvidence) (FencingReceiptValidation, error) {
	if err := validateFencingReceipt(receipt); err != nil {
		return FencingReceiptValidation{}, err
	}
	if err := validateTransitionRevisionEvidence(currentEvidence); err != nil {
		return FencingReceiptValidation{}, fmt.Errorf("%w: current revision evidence: %v", ErrInvalidFencingReceipt, err)
	}
	currentSnapshotID, err := membershipSnapshotID(currentMembership)
	if err != nil {
		return FencingReceiptValidation{}, fmt.Errorf("%w: current membership: %v", ErrStaleFencingReceipt, err)
	}
	if err := validateRevisionEvidenceBinding(currentEvidence, currentMembership, currentSnapshotID); err != nil {
		return FencingReceiptValidation{}, fmt.Errorf("%w: current revision binding: %v", ErrStaleFencingReceipt, err)
	}
	if currentMembership.ClusterID != receipt.ClusterID || currentMembership.Generation != receipt.ClusterGeneration {
		return FencingReceiptValidation{}, fmt.Errorf("%w: current cluster identity changed", ErrStaleFencingReceipt)
	}
	if currentSnapshotID != receipt.AfterMembershipSnapshotID || currentEvidence.EvidenceID() != receipt.AfterRevisionEvidenceID || currentEvidence.Revision() != receipt.AfterRevision {
		return FencingReceiptValidation{}, fmt.Errorf("%w: post-fencing membership or HA revision changed", ErrStaleFencingReceipt)
	}

	return FencingReceiptValidation{
		ReceiptID:                 receipt.ReceiptID,
		PreflightPlanID:           receipt.PreflightPlanID,
		ClusterID:                 receipt.ClusterID,
		ClusterGeneration:         receipt.ClusterGeneration,
		TargetNodeID:              receipt.TargetNodeID,
		FenceObservationID:        receipt.FenceObservationID,
		AfterMembershipSnapshotID: receipt.AfterMembershipSnapshotID,
		AfterRevisionEvidenceID:   receipt.AfterRevisionEvidenceID,
		AfterRevision:             receipt.AfterRevision,
		Revalidated:               true,
		ProofOnly:                 true,
		ExecutionAuthorized:       false,
		HostMutation:              false,
		StateMutation:             false,
	}, nil
}

func validateFencingReceipt(receipt FencingReceipt) error {
	if receipt.SchemaVersion != FencingReceiptSchemaV1 || receipt.ReceiptID == "" || receipt.PreflightPlanID == "" || receipt.ClusterID == "" || receipt.ClusterGeneration == 0 || receipt.TargetNodeID == "" || receipt.FenceObservationID == "" || receipt.BeforeMembershipSnapshotID == "" || receipt.AfterMembershipSnapshotID == "" || receipt.BeforeRevisionEvidenceID == "" || receipt.AfterRevisionEvidenceID == "" {
		return fmt.Errorf("%w: receipt identity is incomplete", ErrInvalidFencingReceipt)
	}
	if !receipt.ProofOnly || receipt.ExecutionAuthorized || receipt.HostMutation || receipt.StateMutation {
		return fmt.Errorf("%w: receipt safety flags are invalid", ErrInvalidFencingReceipt)
	}
	if err := validateTransitionRevision(receipt.BeforeRevision); err != nil {
		return fmt.Errorf("%w: before revision: %v", ErrInvalidFencingReceipt, err)
	}
	if err := validateTransitionRevision(receipt.AfterRevision); err != nil {
		return fmt.Errorf("%w: after revision: %v", ErrInvalidFencingReceipt, err)
	}
	if receipt.BeforeRevision.JournalSequence == math.MaxUint64 || receipt.AfterRevision.LeaderEpoch != receipt.BeforeRevision.LeaderEpoch || receipt.AfterRevision.JournalSequence != receipt.BeforeRevision.JournalSequence+1 || receipt.AfterRevision.ResourceVersion == receipt.BeforeRevision.ResourceVersion {
		return fmt.Errorf("%w: revision lineage is not one exact fencing journal step", ErrInvalidFencingReceipt)
	}
	for field, value := range map[string]string{
		"receipt_id":                    receipt.ReceiptID,
		"preflight_plan_id":             receipt.PreflightPlanID,
		"cluster_id":                    receipt.ClusterID,
		"target_node_id":                receipt.TargetNodeID,
		"fence_observation_id":          receipt.FenceObservationID,
		"before_membership_snapshot_id": receipt.BeforeMembershipSnapshotID,
		"after_membership_snapshot_id":  receipt.AfterMembershipSnapshotID,
		"before_revision_evidence_id":   receipt.BeforeRevisionEvidenceID,
		"after_revision_evidence_id":    receipt.AfterRevisionEvidenceID,
	} {
		if err := validateIdentifier(field, value); err != nil {
			return fmt.Errorf("%w: %v", ErrInvalidFencingReceipt, err)
		}
	}
	expectedID, err := fencingReceiptID(receipt)
	if err != nil {
		return err
	}
	if expectedID != receipt.ReceiptID {
		return fmt.Errorf("%w: receipt fingerprint mismatch", ErrInvalidFencingReceipt)
	}
	return nil
}

func fencingReceiptID(receipt FencingReceipt) (string, error) {
	input := struct {
		SchemaVersion              string             `json:"schema_version"`
		PreflightPlanID            string             `json:"preflight_plan_id"`
		ClusterID                  string             `json:"cluster_id"`
		ClusterGeneration          uint64             `json:"cluster_generation"`
		TargetNodeID               string             `json:"target_node_id"`
		FenceObservationID         string             `json:"fence_observation_id"`
		BeforeMembershipSnapshotID string             `json:"before_membership_snapshot_id"`
		AfterMembershipSnapshotID  string             `json:"after_membership_snapshot_id"`
		BeforeRevisionEvidenceID   string             `json:"before_revision_evidence_id"`
		AfterRevisionEvidenceID    string             `json:"after_revision_evidence_id"`
		BeforeRevision             TransitionRevision `json:"before_revision"`
		AfterRevision              TransitionRevision `json:"after_revision"`
		ProofOnly                  bool               `json:"proof_only"`
		ExecutionAuthorized        bool               `json:"execution_authorized"`
		HostMutation               bool               `json:"host_mutation"`
		StateMutation              bool               `json:"state_mutation"`
	}{
		SchemaVersion:              receipt.SchemaVersion,
		PreflightPlanID:            receipt.PreflightPlanID,
		ClusterID:                  receipt.ClusterID,
		ClusterGeneration:          receipt.ClusterGeneration,
		TargetNodeID:               receipt.TargetNodeID,
		FenceObservationID:         receipt.FenceObservationID,
		BeforeMembershipSnapshotID: receipt.BeforeMembershipSnapshotID,
		AfterMembershipSnapshotID:  receipt.AfterMembershipSnapshotID,
		BeforeRevisionEvidenceID:   receipt.BeforeRevisionEvidenceID,
		AfterRevisionEvidenceID:    receipt.AfterRevisionEvidenceID,
		BeforeRevision:             receipt.BeforeRevision,
		AfterRevision:              receipt.AfterRevision,
		ProofOnly:                  receipt.ProofOnly,
		ExecutionAuthorized:        receipt.ExecutionAuthorized,
		HostMutation:               receipt.HostMutation,
		StateMutation:              receipt.StateMutation,
	}
	encoded, err := json.Marshal(input)
	if err != nil {
		return "", fmt.Errorf("%w: receipt fingerprint: %v", ErrInvalidFencingReceipt, err)
	}
	digest := sha256.Sum256(encoded)
	return "chfr-" + hex.EncodeToString(digest[:])[:24], nil
}
