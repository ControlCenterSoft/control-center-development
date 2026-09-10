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
	ErrInvalidLeaderTransferReceipt = errors.New("invalid HA leader transfer receipt")
	ErrStaleLeaderTransferReceipt   = errors.New("stale HA leader transfer receipt")
)

const LeaderTransferReceiptSchemaV1 = "clusterha.leader-transfer-receipt/v1"

// LeaderTransferReceipt is immutable planning evidence that a reconciler-observed
// leader transfer advanced the HA journal and leader epoch exactly once. It does
// not execute a transfer, fence a node, or authorize lifecycle mutation.
type LeaderTransferReceipt struct {
	SchemaVersion              string             `json:"schema_version"`
	ReceiptID                  string             `json:"receipt_id"`
	PreflightPlanID            string             `json:"preflight_plan_id"`
	ClusterID                  string             `json:"cluster_id"`
	ClusterGeneration          uint64             `json:"cluster_generation"`
	PreviousLeaderID           string             `json:"previous_leader_id"`
	NextLeaderID               string             `json:"next_leader_id"`
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

// LeaderTransferReceiptRequest contains only trusted planner/reconciler evidence.
// AfterObservation must be CAS-linked to BeforeEvidence. The constructor rejects
// topology, generation or member-state changes so the receipt proves exactly one
// leader transfer rather than an unrelated failover or membership mutation.
type LeaderTransferReceiptRequest struct {
	Preflight        LifecyclePreflightPlan        `json:"preflight"`
	BeforeMembership Snapshot                      `json:"before_membership"`
	BeforeEvidence   TransitionRevisionEvidence    `json:"-"`
	NextLeaderID     string                        `json:"next_leader_id"`
	AfterObservation ReconcilerRevisionObservation `json:"after_observation"`
}

type LeaderTransferReceiptValidation struct {
	ReceiptID                 string             `json:"receipt_id"`
	PreflightPlanID           string             `json:"preflight_plan_id"`
	ClusterID                 string             `json:"cluster_id"`
	ClusterGeneration         uint64             `json:"cluster_generation"`
	NextLeaderID              string             `json:"next_leader_id"`
	AfterMembershipSnapshotID string             `json:"after_membership_snapshot_id"`
	AfterRevisionEvidenceID   string             `json:"after_revision_evidence_id"`
	AfterRevision             TransitionRevision `json:"after_revision"`
	Revalidated               bool               `json:"revalidated"`
	ProofOnly                 bool               `json:"proof_only"`
	ExecutionAuthorized       bool               `json:"execution_authorized"`
	HostMutation              bool               `json:"host_mutation"`
	StateMutation             bool               `json:"state_mutation"`
}

func BuildLeaderTransferReceipt(request LeaderTransferReceiptRequest) (LeaderTransferReceipt, error) {
	if err := validatePreflightEvidence(request.Preflight); err != nil {
		return LeaderTransferReceipt{}, fmt.Errorf("%w: preflight: %v", ErrInvalidLeaderTransferReceipt, err)
	}
	if !request.Preflight.RequiresLeaderTransfer || request.Preflight.RequiresStandaloneDowntime || request.Preflight.ClusterProfile == ProfileStandalone {
		return LeaderTransferReceipt{}, fmt.Errorf("%w: preflight does not require a multi-node leader transfer", ErrInvalidLeaderTransferReceipt)
	}
	if request.Preflight.ClusterID != request.BeforeMembership.ClusterID || request.Preflight.ClusterGeneration != request.BeforeMembership.Generation {
		return LeaderTransferReceipt{}, fmt.Errorf("%w: preflight does not match the before membership identity", ErrInvalidLeaderTransferReceipt)
	}
	if request.Preflight.NodeID != request.BeforeMembership.LeaderID {
		return LeaderTransferReceipt{}, fmt.Errorf("%w: preflight target is not the current leader", ErrInvalidLeaderTransferReceipt)
	}
	if err := validateIdentifier("next_leader_id", request.NextLeaderID); err != nil {
		return LeaderTransferReceipt{}, fmt.Errorf("%w: %v", ErrInvalidLeaderTransferReceipt, err)
	}
	if request.NextLeaderID == request.BeforeMembership.LeaderID {
		return LeaderTransferReceipt{}, fmt.Errorf("%w: next leader must differ from the current leader", ErrInvalidLeaderTransferReceipt)
	}

	_, beforeMembers, err := validateSnapshot(request.BeforeMembership)
	if err != nil {
		return LeaderTransferReceipt{}, fmt.Errorf("%w: before membership: %v", ErrInvalidLeaderTransferReceipt, err)
	}
	nextLeader, exists := beforeMembers[request.NextLeaderID]
	if !exists || nextLeader.Kind != MemberController || !memberReady(nextLeader) {
		return LeaderTransferReceipt{}, fmt.Errorf("%w: next leader %q is not a ready controller", ErrInvalidLeaderTransferReceipt, request.NextLeaderID)
	}
	if err := validateTransitionRevisionEvidence(request.BeforeEvidence); err != nil {
		return LeaderTransferReceipt{}, fmt.Errorf("%w: before revision evidence: %v", ErrInvalidLeaderTransferReceipt, err)
	}
	beforeSnapshotID, err := membershipSnapshotID(request.BeforeMembership)
	if err != nil {
		return LeaderTransferReceipt{}, err
	}
	if err := validateRevisionEvidenceBinding(request.BeforeEvidence, request.BeforeMembership, beforeSnapshotID); err != nil {
		return LeaderTransferReceipt{}, fmt.Errorf("%w: before revision binding: %v", ErrInvalidLeaderTransferReceipt, err)
	}

	afterMembership := request.AfterObservation.Membership
	if afterMembership.ClusterID != request.BeforeMembership.ClusterID || afterMembership.Generation != request.BeforeMembership.Generation {
		return LeaderTransferReceipt{}, fmt.Errorf("%w: leader transfer must not change cluster identity or generation", ErrInvalidLeaderTransferReceipt)
	}
	if afterMembership.LeaderID != request.NextLeaderID {
		return LeaderTransferReceipt{}, fmt.Errorf("%w: observed leader %q does not match next leader %q", ErrInvalidLeaderTransferReceipt, afterMembership.LeaderID, request.NextLeaderID)
	}
	if !sameMemberSetAndState(request.BeforeMembership.Members, afterMembership.Members) {
		return LeaderTransferReceipt{}, fmt.Errorf("%w: leader transfer must not change membership or member readiness", ErrInvalidLeaderTransferReceipt)
	}
	if request.AfterObservation.ResourceVersion == request.BeforeEvidence.Revision().ResourceVersion {
		return LeaderTransferReceipt{}, fmt.Errorf("%w: leader transfer must advance resource version", ErrInvalidLeaderTransferReceipt)
	}

	afterEvidence, err := AdvanceTransitionRevisionEvidence(request.BeforeEvidence, request.AfterObservation)
	if err != nil {
		return LeaderTransferReceipt{}, fmt.Errorf("%w: revision advance: %v", ErrInvalidLeaderTransferReceipt, err)
	}
	beforeRevision := request.BeforeEvidence.Revision()
	afterRevision := afterEvidence.Revision()
	if beforeRevision.LeaderEpoch == math.MaxUint64 || beforeRevision.JournalSequence == math.MaxUint64 {
		return LeaderTransferReceipt{}, fmt.Errorf("%w: before revision cannot advance", ErrInvalidLeaderTransferReceipt)
	}
	if afterRevision.LeaderEpoch != beforeRevision.LeaderEpoch+1 || afterRevision.JournalSequence != beforeRevision.JournalSequence+1 {
		return LeaderTransferReceipt{}, fmt.Errorf("%w: leader transfer must advance leader epoch and journal exactly once", ErrInvalidLeaderTransferReceipt)
	}

	afterSnapshotID, err := membershipSnapshotID(afterMembership)
	if err != nil {
		return LeaderTransferReceipt{}, err
	}
	if err := validateRevisionEvidenceBinding(afterEvidence, afterMembership, afterSnapshotID); err != nil {
		return LeaderTransferReceipt{}, fmt.Errorf("%w: after revision binding: %v", ErrInvalidLeaderTransferReceipt, err)
	}

	receipt := LeaderTransferReceipt{
		SchemaVersion:              LeaderTransferReceiptSchemaV1,
		PreflightPlanID:            request.Preflight.PlanID,
		ClusterID:                  request.Preflight.ClusterID,
		ClusterGeneration:          request.Preflight.ClusterGeneration,
		PreviousLeaderID:           request.BeforeMembership.LeaderID,
		NextLeaderID:               request.NextLeaderID,
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
	receiptID, err := leaderTransferReceiptID(receipt)
	if err != nil {
		return LeaderTransferReceipt{}, err
	}
	receipt.ReceiptID = receiptID
	return receipt, nil
}

// RevalidateLeaderTransferReceipt proves that the exact post-transfer snapshot
// and sealed HA revision are still current. Any later journal entry, failover,
// health change or topology change makes the receipt stale.
func RevalidateLeaderTransferReceipt(receipt LeaderTransferReceipt, currentMembership Snapshot, currentEvidence TransitionRevisionEvidence) (LeaderTransferReceiptValidation, error) {
	if err := validateLeaderTransferReceipt(receipt); err != nil {
		return LeaderTransferReceiptValidation{}, err
	}
	if err := validateTransitionRevisionEvidence(currentEvidence); err != nil {
		return LeaderTransferReceiptValidation{}, fmt.Errorf("%w: current revision evidence: %v", ErrInvalidLeaderTransferReceipt, err)
	}
	currentSnapshotID, err := membershipSnapshotID(currentMembership)
	if err != nil {
		return LeaderTransferReceiptValidation{}, fmt.Errorf("%w: current membership: %v", ErrStaleLeaderTransferReceipt, err)
	}
	if err := validateRevisionEvidenceBinding(currentEvidence, currentMembership, currentSnapshotID); err != nil {
		return LeaderTransferReceiptValidation{}, fmt.Errorf("%w: current revision binding: %v", ErrStaleLeaderTransferReceipt, err)
	}
	if currentMembership.ClusterID != receipt.ClusterID || currentMembership.Generation != receipt.ClusterGeneration || currentMembership.LeaderID != receipt.NextLeaderID {
		return LeaderTransferReceiptValidation{}, fmt.Errorf("%w: current cluster identity or leader changed", ErrStaleLeaderTransferReceipt)
	}
	if currentSnapshotID != receipt.AfterMembershipSnapshotID || currentEvidence.EvidenceID() != receipt.AfterRevisionEvidenceID || currentEvidence.Revision() != receipt.AfterRevision {
		return LeaderTransferReceiptValidation{}, fmt.Errorf("%w: post-transfer membership or HA revision changed", ErrStaleLeaderTransferReceipt)
	}

	return LeaderTransferReceiptValidation{
		ReceiptID:                 receipt.ReceiptID,
		PreflightPlanID:           receipt.PreflightPlanID,
		ClusterID:                 receipt.ClusterID,
		ClusterGeneration:         receipt.ClusterGeneration,
		NextLeaderID:              receipt.NextLeaderID,
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

func validateLeaderTransferReceipt(receipt LeaderTransferReceipt) error {
	if receipt.SchemaVersion != LeaderTransferReceiptSchemaV1 || receipt.ReceiptID == "" || receipt.PreflightPlanID == "" || receipt.ClusterID == "" || receipt.ClusterGeneration == 0 || receipt.PreviousLeaderID == "" || receipt.NextLeaderID == "" || receipt.BeforeMembershipSnapshotID == "" || receipt.AfterMembershipSnapshotID == "" || receipt.BeforeRevisionEvidenceID == "" || receipt.AfterRevisionEvidenceID == "" {
		return fmt.Errorf("%w: receipt identity is incomplete", ErrInvalidLeaderTransferReceipt)
	}
	if receipt.PreviousLeaderID == receipt.NextLeaderID {
		return fmt.Errorf("%w: previous and next leader must differ", ErrInvalidLeaderTransferReceipt)
	}
	if !receipt.ProofOnly || receipt.ExecutionAuthorized || receipt.HostMutation || receipt.StateMutation {
		return fmt.Errorf("%w: receipt safety flags are invalid", ErrInvalidLeaderTransferReceipt)
	}
	if err := validateTransitionRevision(receipt.BeforeRevision); err != nil {
		return fmt.Errorf("%w: before revision: %v", ErrInvalidLeaderTransferReceipt, err)
	}
	if err := validateTransitionRevision(receipt.AfterRevision); err != nil {
		return fmt.Errorf("%w: after revision: %v", ErrInvalidLeaderTransferReceipt, err)
	}
	if receipt.BeforeRevision.LeaderEpoch == math.MaxUint64 || receipt.BeforeRevision.JournalSequence == math.MaxUint64 || receipt.AfterRevision.LeaderEpoch != receipt.BeforeRevision.LeaderEpoch+1 || receipt.AfterRevision.JournalSequence != receipt.BeforeRevision.JournalSequence+1 || receipt.AfterRevision.ResourceVersion == receipt.BeforeRevision.ResourceVersion {
		return fmt.Errorf("%w: revision lineage is not one exact leader-transfer step", ErrInvalidLeaderTransferReceipt)
	}
	for field, value := range map[string]string{
		"preflight_plan_id":             receipt.PreflightPlanID,
		"cluster_id":                    receipt.ClusterID,
		"previous_leader_id":            receipt.PreviousLeaderID,
		"next_leader_id":                receipt.NextLeaderID,
		"before_membership_snapshot_id": receipt.BeforeMembershipSnapshotID,
		"after_membership_snapshot_id":  receipt.AfterMembershipSnapshotID,
		"before_revision_evidence_id":   receipt.BeforeRevisionEvidenceID,
		"after_revision_evidence_id":    receipt.AfterRevisionEvidenceID,
	} {
		if err := validateIdentifier(field, value); err != nil {
			return fmt.Errorf("%w: %v", ErrInvalidLeaderTransferReceipt, err)
		}
	}
	expectedID, err := leaderTransferReceiptID(receipt)
	if err != nil {
		return err
	}
	if expectedID != receipt.ReceiptID {
		return fmt.Errorf("%w: receipt fingerprint mismatch", ErrInvalidLeaderTransferReceipt)
	}
	return nil
}

func sameMemberSetAndState(left, right []Member) bool {
	if len(left) != len(right) {
		return false
	}
	leftCanonical := canonicalMembers(left)
	rightCanonical := canonicalMembers(right)
	for i := range leftCanonical {
		if leftCanonical[i] != rightCanonical[i] {
			return false
		}
	}
	return true
}

func leaderTransferReceiptID(receipt LeaderTransferReceipt) (string, error) {
	input := struct {
		SchemaVersion              string             `json:"schema_version"`
		PreflightPlanID            string             `json:"preflight_plan_id"`
		ClusterID                  string             `json:"cluster_id"`
		ClusterGeneration          uint64             `json:"cluster_generation"`
		PreviousLeaderID           string             `json:"previous_leader_id"`
		NextLeaderID               string             `json:"next_leader_id"`
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
		PreviousLeaderID:           receipt.PreviousLeaderID,
		NextLeaderID:               receipt.NextLeaderID,
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
		return "", fmt.Errorf("%w: receipt fingerprint: %v", ErrInvalidLeaderTransferReceipt, err)
	}
	digest := sha256.Sum256(encoded)
	return "chlr-" + hex.EncodeToString(digest[:])[:24], nil
}
