package clusterha

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
)

var (
	ErrInvalidLifecycleHandoff = errors.New("invalid cluster lifecycle handoff")
	ErrStaleLifecycleHandoff   = errors.New("stale cluster lifecycle handoff")
)

const LifecycleHandoffSchemaV1 = "clusterha.lifecycle-handoff/v1"

// LifecycleHandoff is immutable proof-only evidence presented at the bounded
// lifecycle executor boundary. It binds the exact lifecycle preflight to the
// current HA membership/journal revision and to any typed leader-transfer or
// fencing receipts required by that preflight. It never executes or authorizes
// host/state mutation by itself.
type LifecycleHandoff struct {
	SchemaVersion           string             `json:"schema_version"`
	HandoffID               string             `json:"handoff_id"`
	PreflightPlanID         string             `json:"preflight_plan_id"`
	LifecyclePlanID         string             `json:"lifecycle_plan_id"`
	NodeID                  string             `json:"node_id"`
	ClusterID               string             `json:"cluster_id"`
	ClusterGeneration       uint64             `json:"cluster_generation"`
	MembershipSnapshotID    string             `json:"membership_snapshot_id"`
	RevisionEvidenceID      string             `json:"revision_evidence_id"`
	Revision                TransitionRevision `json:"revision"`
	LeaderTransferReceiptID string             `json:"leader_transfer_receipt_id,omitempty"`
	FencingReceiptID        string             `json:"fencing_receipt_id,omitempty"`
	StandaloneDowntimeBound bool               `json:"standalone_downtime_bound"`
	ReadyForBoundedExecutor bool               `json:"ready_for_bounded_executor"`
	ExecutionAuthorized     bool               `json:"execution_authorized"`
	HostMutation            bool               `json:"host_mutation"`
	StateMutation           bool               `json:"state_mutation"`
}

type LifecycleHandoffRequest struct {
	Preflight             LifecyclePreflightPlan     `json:"preflight"`
	PreflightRequest      LifecyclePreflightRequest  `json:"preflight_request"`
	CurrentMembership     Snapshot                   `json:"current_membership"`
	CurrentEvidence       TransitionRevisionEvidence `json:"-"`
	LeaderTransferReceipt *LeaderTransferReceipt     `json:"leader_transfer_receipt,omitempty"`
	FencingReceipt        *FencingReceipt            `json:"fencing_receipt,omitempty"`
}

type LifecycleHandoffValidation struct {
	HandoffID               string             `json:"handoff_id"`
	PreflightPlanID         string             `json:"preflight_plan_id"`
	LifecyclePlanID         string             `json:"lifecycle_plan_id"`
	NodeID                  string             `json:"node_id"`
	ClusterID               string             `json:"cluster_id"`
	ClusterGeneration       uint64             `json:"cluster_generation"`
	MembershipSnapshotID    string             `json:"membership_snapshot_id"`
	RevisionEvidenceID      string             `json:"revision_evidence_id"`
	Revision                TransitionRevision `json:"revision"`
	LeaderTransferReceiptID string             `json:"leader_transfer_receipt_id,omitempty"`
	FencingReceiptID        string             `json:"fencing_receipt_id,omitempty"`
	Revalidated             bool               `json:"revalidated"`
	ReadyForBoundedExecutor bool               `json:"ready_for_bounded_executor"`
	ExecutionAuthorized     bool               `json:"execution_authorized"`
	HostMutation            bool               `json:"host_mutation"`
	StateMutation           bool               `json:"state_mutation"`
}

func BuildLifecycleHandoff(request LifecycleHandoffRequest) (LifecycleHandoff, error) {
	if err := validatePreflightEvidence(request.Preflight); err != nil {
		return LifecycleHandoff{}, fmt.Errorf("%w: preflight: %v", ErrInvalidLifecycleHandoff, err)
	}
	if !request.Preflight.ControllerBound {
		return LifecycleHandoff{}, fmt.Errorf("%w: handoff is only valid for controller-bound lifecycle", ErrInvalidLifecycleHandoff)
	}
	rebuilt, err := BuildLifecyclePreflight(request.PreflightRequest)
	if err != nil {
		return LifecycleHandoff{}, fmt.Errorf("%w: preflight request no longer validates: %v", ErrInvalidLifecycleHandoff, err)
	}
	if !sameLifecycleHandoffPreflight(request.Preflight, rebuilt) {
		return LifecycleHandoff{}, fmt.Errorf("%w: preflight evidence does not match supplied request", ErrInvalidLifecycleHandoff)
	}
	if request.Preflight.RequiresLeaderTransfer && request.Preflight.RequiresFencing {
		return LifecycleHandoff{}, fmt.Errorf("%w: combined leader-transfer and fencing proof requires an ordered composite transition", ErrInvalidLifecycleHandoff)
	}
	if err := validateTransitionRevisionEvidence(request.CurrentEvidence); err != nil {
		return LifecycleHandoff{}, fmt.Errorf("%w: current revision evidence: %v", ErrInvalidLifecycleHandoff, err)
	}
	currentSnapshotID, err := membershipSnapshotID(request.CurrentMembership)
	if err != nil {
		return LifecycleHandoff{}, fmt.Errorf("%w: current membership: %v", ErrInvalidLifecycleHandoff, err)
	}
	if err := validateRevisionEvidenceBinding(request.CurrentEvidence, request.CurrentMembership, currentSnapshotID); err != nil {
		return LifecycleHandoff{}, fmt.Errorf("%w: current revision binding: %v", ErrInvalidLifecycleHandoff, err)
	}
	if request.CurrentMembership.ClusterID != request.Preflight.ClusterID ||
		request.CurrentMembership.Generation != request.Preflight.ClusterGeneration {
		return LifecycleHandoff{}, fmt.Errorf("%w: current cluster identity changed", ErrStaleLifecycleHandoff)
	}
	if healthyVotes(request.CurrentMembership.Members) < quorum(len(request.CurrentMembership.Members)) {
		return LifecycleHandoff{}, fmt.Errorf("%w: current quorum is lost", ErrStaleLifecycleHandoff)
	}

	leaderReceiptID := ""
	fencingReceiptID := ""
	switch {
	case request.Preflight.RequiresLeaderTransfer:
		if request.LeaderTransferReceipt == nil || request.FencingReceipt != nil {
			return LifecycleHandoff{}, fmt.Errorf("%w: exact leader-transfer receipt is required and fencing receipt is not", ErrInvalidLifecycleHandoff)
		}
		receipt := *request.LeaderTransferReceipt
		if receipt.PreflightPlanID != request.Preflight.PlanID || receipt.PreviousLeaderID != request.Preflight.NodeID {
			return LifecycleHandoff{}, fmt.Errorf("%w: leader-transfer receipt is bound to different lifecycle evidence", ErrInvalidLifecycleHandoff)
		}
		if _, err := RevalidateLeaderTransferReceipt(receipt, request.CurrentMembership, request.CurrentEvidence); err != nil {
			return LifecycleHandoff{}, fmt.Errorf("%w: leader-transfer receipt: %v", ErrStaleLifecycleHandoff, err)
		}
		leaderReceiptID = receipt.ReceiptID
	case request.Preflight.RequiresFencing:
		if request.FencingReceipt == nil || request.LeaderTransferReceipt != nil {
			return LifecycleHandoff{}, fmt.Errorf("%w: exact fencing receipt is required and leader-transfer receipt is not", ErrInvalidLifecycleHandoff)
		}
		receipt := *request.FencingReceipt
		if receipt.PreflightPlanID != request.Preflight.PlanID || receipt.TargetNodeID != request.Preflight.NodeID {
			return LifecycleHandoff{}, fmt.Errorf("%w: fencing receipt is bound to different lifecycle evidence", ErrInvalidLifecycleHandoff)
		}
		if _, err := RevalidateFencingReceipt(receipt, request.CurrentMembership, request.CurrentEvidence); err != nil {
			return LifecycleHandoff{}, fmt.Errorf("%w: fencing receipt: %v", ErrStaleLifecycleHandoff, err)
		}
		fencingReceiptID = receipt.ReceiptID
	default:
		if request.LeaderTransferReceipt != nil || request.FencingReceipt != nil {
			return LifecycleHandoff{}, fmt.Errorf("%w: transition receipt supplied when preflight requires none", ErrInvalidLifecycleHandoff)
		}
		if !sameSnapshot(request.PreflightRequest.Membership, request.CurrentMembership) {
			return LifecycleHandoff{}, fmt.Errorf("%w: membership changed after receipt-free preflight", ErrStaleLifecycleHandoff)
		}
	}

	if request.Preflight.RequiresStandaloneDowntime {
		if request.Preflight.ClusterProfile != ProfileStandalone || !request.PreflightRequest.StandaloneDowntimeAcknowledged {
			return LifecycleHandoff{}, fmt.Errorf("%w: standalone downtime evidence is incomplete", ErrInvalidLifecycleHandoff)
		}
		if leaderReceiptID != "" || fencingReceiptID != "" {
			return LifecycleHandoff{}, fmt.Errorf("%w: standalone handoff cannot carry HA transition receipts", ErrInvalidLifecycleHandoff)
		}
	} else if request.Preflight.ClusterProfile == ProfileStandalone {
		return LifecycleHandoff{}, fmt.Errorf("%w: standalone controller requires bounded downtime evidence", ErrInvalidLifecycleHandoff)
	}

	handoff := LifecycleHandoff{
		SchemaVersion:           LifecycleHandoffSchemaV1,
		PreflightPlanID:         request.Preflight.PlanID,
		LifecyclePlanID:         request.Preflight.LifecyclePlanID,
		NodeID:                  request.Preflight.NodeID,
		ClusterID:               request.Preflight.ClusterID,
		ClusterGeneration:       request.Preflight.ClusterGeneration,
		MembershipSnapshotID:    currentSnapshotID,
		RevisionEvidenceID:      request.CurrentEvidence.EvidenceID(),
		Revision:                request.CurrentEvidence.Revision(),
		LeaderTransferReceiptID: leaderReceiptID,
		FencingReceiptID:        fencingReceiptID,
		StandaloneDowntimeBound: request.Preflight.RequiresStandaloneDowntime,
		ReadyForBoundedExecutor: true,
		ExecutionAuthorized:     false,
		HostMutation:            false,
		StateMutation:           false,
	}
	handoffID, err := lifecycleHandoffID(handoff)
	if err != nil {
		return LifecycleHandoff{}, err
	}
	handoff.HandoffID = handoffID
	return handoff, nil
}

// RevalidateLifecycleHandoff proves the exact current membership and HA journal
// still match the sealed executor handoff. Any later health, topology, leader,
// journal or CAS movement makes the handoff stale and requires a new preflight.
func RevalidateLifecycleHandoff(
	handoff LifecycleHandoff,
	currentMembership Snapshot,
	currentEvidence TransitionRevisionEvidence,
) (LifecycleHandoffValidation, error) {
	if err := validateLifecycleHandoff(handoff); err != nil {
		return LifecycleHandoffValidation{}, err
	}
	if err := validateTransitionRevisionEvidence(currentEvidence); err != nil {
		return LifecycleHandoffValidation{}, fmt.Errorf("%w: current revision evidence: %v", ErrInvalidLifecycleHandoff, err)
	}
	currentSnapshotID, err := membershipSnapshotID(currentMembership)
	if err != nil {
		return LifecycleHandoffValidation{}, fmt.Errorf("%w: current membership: %v", ErrStaleLifecycleHandoff, err)
	}
	if err := validateRevisionEvidenceBinding(currentEvidence, currentMembership, currentSnapshotID); err != nil {
		return LifecycleHandoffValidation{}, fmt.Errorf("%w: current revision binding: %v", ErrStaleLifecycleHandoff, err)
	}
	if currentMembership.ClusterID != handoff.ClusterID || currentMembership.Generation != handoff.ClusterGeneration {
		return LifecycleHandoffValidation{}, fmt.Errorf("%w: current cluster identity changed", ErrStaleLifecycleHandoff)
	}
	if healthyVotes(currentMembership.Members) < quorum(len(currentMembership.Members)) {
		return LifecycleHandoffValidation{}, fmt.Errorf("%w: current quorum is lost", ErrStaleLifecycleHandoff)
	}
	if currentSnapshotID != handoff.MembershipSnapshotID ||
		currentEvidence.EvidenceID() != handoff.RevisionEvidenceID ||
		currentEvidence.Revision() != handoff.Revision {
		return LifecycleHandoffValidation{}, fmt.Errorf("%w: membership or HA revision changed", ErrStaleLifecycleHandoff)
	}

	return LifecycleHandoffValidation{
		HandoffID:               handoff.HandoffID,
		PreflightPlanID:         handoff.PreflightPlanID,
		LifecyclePlanID:         handoff.LifecyclePlanID,
		NodeID:                  handoff.NodeID,
		ClusterID:               handoff.ClusterID,
		ClusterGeneration:       handoff.ClusterGeneration,
		MembershipSnapshotID:    handoff.MembershipSnapshotID,
		RevisionEvidenceID:      handoff.RevisionEvidenceID,
		Revision:                handoff.Revision,
		LeaderTransferReceiptID: handoff.LeaderTransferReceiptID,
		FencingReceiptID:        handoff.FencingReceiptID,
		Revalidated:             true,
		ReadyForBoundedExecutor: true,
		ExecutionAuthorized:     false,
		HostMutation:            false,
		StateMutation:           false,
	}, nil
}

func validateLifecycleHandoff(handoff LifecycleHandoff) error {
	if handoff.SchemaVersion != LifecycleHandoffSchemaV1 || handoff.HandoffID == "" ||
		handoff.PreflightPlanID == "" || handoff.LifecyclePlanID == "" || handoff.NodeID == "" ||
		handoff.ClusterID == "" || handoff.ClusterGeneration == 0 || handoff.MembershipSnapshotID == "" ||
		handoff.RevisionEvidenceID == "" {
		return fmt.Errorf("%w: handoff identity is incomplete", ErrInvalidLifecycleHandoff)
	}
	if !handoff.ReadyForBoundedExecutor || handoff.ExecutionAuthorized || handoff.HostMutation || handoff.StateMutation {
		return fmt.Errorf("%w: handoff safety flags are invalid", ErrInvalidLifecycleHandoff)
	}
	if handoff.LeaderTransferReceiptID != "" && handoff.FencingReceiptID != "" {
		return fmt.Errorf("%w: handoff cannot carry unordered leader-transfer and fencing receipts", ErrInvalidLifecycleHandoff)
	}
	if err := validateTransitionRevision(handoff.Revision); err != nil {
		return fmt.Errorf("%w: revision: %v", ErrInvalidLifecycleHandoff, err)
	}
	for field, value := range map[string]string{
		"handoff_id":             handoff.HandoffID,
		"preflight_plan_id":      handoff.PreflightPlanID,
		"lifecycle_plan_id":      handoff.LifecyclePlanID,
		"node_id":                handoff.NodeID,
		"cluster_id":             handoff.ClusterID,
		"membership_snapshot_id": handoff.MembershipSnapshotID,
		"revision_evidence_id":   handoff.RevisionEvidenceID,
	} {
		if err := validateIdentifier(field, value); err != nil {
			return fmt.Errorf("%w: %v", ErrInvalidLifecycleHandoff, err)
		}
	}
	if handoff.LeaderTransferReceiptID != "" {
		if err := validateIdentifier("leader_transfer_receipt_id", handoff.LeaderTransferReceiptID); err != nil {
			return fmt.Errorf("%w: %v", ErrInvalidLifecycleHandoff, err)
		}
	}
	if handoff.FencingReceiptID != "" {
		if err := validateIdentifier("fencing_receipt_id", handoff.FencingReceiptID); err != nil {
			return fmt.Errorf("%w: %v", ErrInvalidLifecycleHandoff, err)
		}
	}
	expectedID, err := lifecycleHandoffID(handoff)
	if err != nil {
		return err
	}
	if expectedID != handoff.HandoffID {
		return fmt.Errorf("%w: handoff fingerprint mismatch", ErrInvalidLifecycleHandoff)
	}
	return nil
}

func sameLifecycleHandoffPreflight(left, right LifecyclePreflightPlan) bool {
	if !sameLifecyclePreflightEvidence(left, right) ||
		left.ClusterProfile != right.ClusterProfile ||
		left.CurrentQuorum != right.CurrentQuorum ||
		left.CurrentHealthyVotes != right.CurrentHealthyVotes ||
		left.HealthyVotesAfterDrain != right.HealthyVotesAfterDrain ||
		left.RequiresStandaloneDowntime != right.RequiresStandaloneDowntime ||
		left.RequiresLeaderTransfer != right.RequiresLeaderTransfer ||
		left.RequiresFencing != right.RequiresFencing ||
		left.MembershipPlanID != right.MembershipPlanID ||
		len(left.RequiredChecks) != len(right.RequiredChecks) {
		return false
	}
	for i := range left.RequiredChecks {
		if left.RequiredChecks[i] != right.RequiredChecks[i] {
			return false
		}
	}
	return true
}

func lifecycleHandoffID(handoff LifecycleHandoff) (string, error) {
	input := struct {
		SchemaVersion           string             `json:"schema_version"`
		PreflightPlanID         string             `json:"preflight_plan_id"`
		LifecyclePlanID         string             `json:"lifecycle_plan_id"`
		NodeID                  string             `json:"node_id"`
		ClusterID               string             `json:"cluster_id"`
		ClusterGeneration       uint64             `json:"cluster_generation"`
		MembershipSnapshotID    string             `json:"membership_snapshot_id"`
		RevisionEvidenceID      string             `json:"revision_evidence_id"`
		Revision                TransitionRevision `json:"revision"`
		LeaderTransferReceiptID string             `json:"leader_transfer_receipt_id,omitempty"`
		FencingReceiptID        string             `json:"fencing_receipt_id,omitempty"`
		StandaloneDowntimeBound bool               `json:"standalone_downtime_bound"`
		ReadyForBoundedExecutor bool               `json:"ready_for_bounded_executor"`
		ExecutionAuthorized     bool               `json:"execution_authorized"`
		HostMutation            bool               `json:"host_mutation"`
		StateMutation           bool               `json:"state_mutation"`
	}{
		SchemaVersion:           handoff.SchemaVersion,
		PreflightPlanID:         handoff.PreflightPlanID,
		LifecyclePlanID:         handoff.LifecyclePlanID,
		NodeID:                  handoff.NodeID,
		ClusterID:               handoff.ClusterID,
		ClusterGeneration:       handoff.ClusterGeneration,
		MembershipSnapshotID:    handoff.MembershipSnapshotID,
		RevisionEvidenceID:      handoff.RevisionEvidenceID,
		Revision:                handoff.Revision,
		LeaderTransferReceiptID: handoff.LeaderTransferReceiptID,
		FencingReceiptID:        handoff.FencingReceiptID,
		StandaloneDowntimeBound: handoff.StandaloneDowntimeBound,
		ReadyForBoundedExecutor: handoff.ReadyForBoundedExecutor,
		ExecutionAuthorized:     handoff.ExecutionAuthorized,
		HostMutation:            handoff.HostMutation,
		StateMutation:           handoff.StateMutation,
	}
	encoded, err := json.Marshal(input)
	if err != nil {
		return "", fmt.Errorf("%w: handoff fingerprint: %v", ErrInvalidLifecycleHandoff, err)
	}
	digest := sha256.Sum256(encoded)
	return "chh-" + hex.EncodeToString(digest[:])[:24], nil
}
