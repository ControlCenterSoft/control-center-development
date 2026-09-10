package clusterha

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"control-center/internal/nodelifecycle"
)

var (
	ErrInvalidLifecycleRecoveryRollbackCompletion      = errors.New("invalid cluster lifecycle recovery rollback completion")
	ErrStaleLifecycleRecoveryRollbackCompletion        = errors.New("stale cluster lifecycle recovery rollback completion")
	ErrLifecycleRecoveryRollbackReconciliationRequired = errors.New("cluster lifecycle recovery rollback reconciliation required")
)

const LifecycleRecoveryRollbackCompletionReceiptSchemaV1 = "clusterha.lifecycle-recovery-rollback-completion-receipt/v1"

// LifecycleRecoveryRollbackCompletionObservation binds the exact rollback
// admission to the post-CAS lifecycle state and a fresh HA observation. The
// source request remains the immutable pre-CAS lineage used to prove that the
// single-use rollback admission was valid when the typed CAS was attempted.
type LifecycleRecoveryRollbackCompletionObservation struct {
	Admission         LifecycleRecoveryRollbackAdmission        `json:"admission"`
	Source            LifecycleRecoveryRollbackAdmissionRequest `json:"source"`
	PostCAS           LifecycleCASObservation                   `json:"post_cas"`
	CurrentMembership Snapshot                                  `json:"current_membership"`
	CurrentEvidence   TransitionRevisionEvidence                `json:"-"`
}

// LifecycleRecoveryRollbackCompletionReceipt is immutable evidence that one
// exact Draining -> Ready recovery CAS completed while quorum, minimum-ready,
// membership and HA journal lineage stayed unchanged. It is evidence only and
// cannot authorize another lifecycle, membership, failover or host action.
type LifecycleRecoveryRollbackCompletionReceipt struct {
	SchemaVersion                    string                       `json:"schema_version"`
	ReceiptID                        string                       `json:"receipt_id"`
	RollbackAdmissionID              string                       `json:"rollback_admission_id"`
	RollbackPlanID                   string                       `json:"rollback_plan_id"`
	RecoveryHandoffID                string                       `json:"recovery_handoff_id"`
	OriginalAdmissionID              string                       `json:"original_admission_id"`
	NodeID                           string                       `json:"node_id"`
	FromState                        nodelifecycle.State          `json:"from_state"`
	TargetState                      nodelifecycle.State          `json:"target_state"`
	TransitionType                   nodelifecycle.TransitionType `json:"transition_type"`
	PreviousLifecycleGeneration      uint64                       `json:"previous_lifecycle_generation"`
	AppliedLifecycleGeneration       uint64                       `json:"applied_lifecycle_generation"`
	PreviousLifecycleResourceVersion string                       `json:"previous_lifecycle_resource_version"`
	AppliedLifecycleResourceVersion  string                       `json:"applied_lifecycle_resource_version"`
	ClusterID                        string                       `json:"cluster_id"`
	ClusterGeneration                uint64                       `json:"cluster_generation"`
	MembershipSnapshotID             string                       `json:"membership_snapshot_id"`
	RevisionEvidenceID               string                       `json:"revision_evidence_id"`
	Revision                         TransitionRevision           `json:"revision"`
	QuorumRequired                   int                          `json:"quorum_required"`
	HealthyVotesObserved             int                          `json:"healthy_votes_observed"`
	MinimumReadyNodes                int                          `json:"minimum_ready_nodes"`
	ReadyNodesObserved               int                          `json:"ready_nodes_observed"`
	StandaloneDowntimeBound          bool                         `json:"standalone_downtime_bound"`
	CompletionVerified               bool                         `json:"completion_verified"`
	ReconciliationRequired           bool                         `json:"reconciliation_required"`
	FurtherLifecycleAuthorized       bool                         `json:"further_lifecycle_authorized"`
	MembershipMutationAuthorized     bool                         `json:"membership_mutation_authorized"`
	FailoverAuthorized               bool                         `json:"failover_authorized"`
	GenericCommandAuthorized         bool                         `json:"generic_command_authorized"`
	HostMutationAuthorized           bool                         `json:"host_mutation_authorized"`
}

// BuildLifecycleRecoveryRollbackCompletionReceipt seals completion only when
// the post-CAS state proves the exact single-use rollback and a fresh HA read
// still satisfies the original quorum/minimum-ready boundary. Any uncertain or
// superseded outcome is routed to reconciliation instead of retrying the CAS.
func BuildLifecycleRecoveryRollbackCompletionReceipt(
	observation LifecycleRecoveryRollbackCompletionObservation,
) (LifecycleRecoveryRollbackCompletionReceipt, error) {
	if observation.Admission.RollbackAdmissionID == "" || observation.Source.Plan.RollbackPlanID == "" {
		return LifecycleRecoveryRollbackCompletionReceipt{}, fmt.Errorf(
			"%w: rollback admission and source plan are required",
			ErrInvalidLifecycleRecoveryRollbackCompletion,
		)
	}
	if err := RevalidateLifecycleRecoveryRollbackAdmission(observation.Admission, observation.Source); err != nil {
		return LifecycleRecoveryRollbackCompletionReceipt{}, classifyLifecycleRecoveryRollbackCompletionError(err)
	}
	if err := revalidateRollbackCompletionHA(observation); err != nil {
		return LifecycleRecoveryRollbackCompletionReceipt{}, err
	}
	if err := validateLifecycleCASObservation(observation.PostCAS); err != nil {
		return LifecycleRecoveryRollbackCompletionReceipt{}, fmt.Errorf(
			"%w: post-CAS observation: %v",
			ErrInvalidLifecycleRecoveryRollbackCompletion,
			err,
		)
	}
	if !rollbackCompletionPostCASMatchesAdmission(observation.PostCAS, observation.Admission) {
		return LifecycleRecoveryRollbackCompletionReceipt{}, fmt.Errorf(
			"%w: post-CAS state does not prove the exact admitted rollback",
			ErrLifecycleRecoveryRollbackReconciliationRequired,
		)
	}

	admission := observation.Admission
	currentQuorum := quorum(len(observation.CurrentMembership.Members))
	currentHealthy := healthyVotes(observation.CurrentMembership.Members)
	currentReady := readyVotes(observation.CurrentMembership.Members)
	if currentQuorum != admission.QuorumRequired || currentHealthy < admission.QuorumRequired ||
		currentReady < admission.MinimumReadyNodes {
		return LifecycleRecoveryRollbackCompletionReceipt{}, fmt.Errorf(
			"%w: fresh quorum or minimum-ready evidence no longer satisfies the rollback boundary",
			ErrStaleLifecycleRecoveryRollbackCompletion,
		)
	}

	receipt := LifecycleRecoveryRollbackCompletionReceipt{
		SchemaVersion:                    LifecycleRecoveryRollbackCompletionReceiptSchemaV1,
		RollbackAdmissionID:              admission.RollbackAdmissionID,
		RollbackPlanID:                   admission.RollbackPlanID,
		RecoveryHandoffID:                admission.RecoveryHandoffID,
		OriginalAdmissionID:              admission.OriginalAdmissionID,
		NodeID:                           admission.NodeID,
		FromState:                        admission.FromState,
		TargetState:                      admission.TargetState,
		TransitionType:                   admission.TransitionType,
		PreviousLifecycleGeneration:      admission.ExpectedLifecycleGeneration,
		AppliedLifecycleGeneration:       observation.PostCAS.Generation,
		PreviousLifecycleResourceVersion: admission.ExpectedLifecycleResourceVersion,
		AppliedLifecycleResourceVersion:  observation.PostCAS.ResourceVersion,
		ClusterID:                        admission.ClusterID,
		ClusterGeneration:                admission.ClusterGeneration,
		MembershipSnapshotID:             admission.MembershipSnapshotID,
		RevisionEvidenceID:               admission.RevisionEvidenceID,
		Revision:                         admission.Revision,
		QuorumRequired:                   admission.QuorumRequired,
		HealthyVotesObserved:             currentHealthy,
		MinimumReadyNodes:                admission.MinimumReadyNodes,
		ReadyNodesObserved:               currentReady,
		StandaloneDowntimeBound:          admission.StandaloneDowntimeBound,
		CompletionVerified:               true,
		ReconciliationRequired:           false,
		FurtherLifecycleAuthorized:       false,
		MembershipMutationAuthorized:     false,
		FailoverAuthorized:               false,
		GenericCommandAuthorized:         false,
		HostMutationAuthorized:           false,
	}
	id, err := lifecycleRecoveryRollbackCompletionReceiptID(receipt)
	if err != nil {
		return LifecycleRecoveryRollbackCompletionReceipt{}, err
	}
	receipt.ReceiptID = id
	return receipt, nil
}

// RevalidateLifecycleRecoveryRollbackCompletionReceipt proves that stored
// completion evidence still matches the exact post-CAS result and fresh HA
// lineage. It never converts completion evidence into execution authority.
func RevalidateLifecycleRecoveryRollbackCompletionReceipt(
	receipt LifecycleRecoveryRollbackCompletionReceipt,
	observation LifecycleRecoveryRollbackCompletionObservation,
) error {
	if err := validateLifecycleRecoveryRollbackCompletionReceipt(receipt); err != nil {
		return err
	}
	rebuilt, err := BuildLifecycleRecoveryRollbackCompletionReceipt(observation)
	if err != nil {
		return err
	}
	if rebuilt != receipt {
		return fmt.Errorf(
			"%w: immutable rollback completion evidence changed",
			ErrStaleLifecycleRecoveryRollbackCompletion,
		)
	}
	return nil
}

func revalidateRollbackCompletionHA(observation LifecycleRecoveryRollbackCompletionObservation) error {
	recoveryObservation := observation.Source.PlanRequest.RecoveryObservation
	recoveryObservation.CurrentMembership = observation.CurrentMembership
	recoveryObservation.CurrentEvidence = observation.CurrentEvidence
	if err := RevalidateLifecycleExecutionRecoveryHandoff(
		observation.Source.PlanRequest.RecoveryHandoff,
		recoveryObservation,
	); err != nil {
		return classifyLifecycleRecoveryRollbackCompletionError(err)
	}
	return nil
}

func rollbackCompletionPostCASMatchesAdmission(
	post LifecycleCASObservation,
	admission LifecycleRecoveryRollbackAdmission,
) bool {
	return post.NodeID == admission.NodeID &&
		post.State == admission.TargetState &&
		post.Generation == admission.PlannedLifecycleGeneration &&
		post.ResourceVersion != admission.ExpectedLifecycleResourceVersion
}

func classifyLifecycleRecoveryRollbackCompletionError(err error) error {
	if errors.Is(err, ErrStaleLifecycleRecoveryRollbackAdmission) ||
		errors.Is(err, ErrStaleLifecycleRecoveryRollbackPlan) ||
		errors.Is(err, ErrStaleLifecycleExecutionRecovery) ||
		errors.Is(err, ErrStaleLifecycleExecutionAdmission) ||
		errors.Is(err, ErrStaleLifecycleHandoff) {
		return fmt.Errorf("%w: %v", ErrStaleLifecycleRecoveryRollbackCompletion, err)
	}
	return fmt.Errorf("%w: %v", ErrInvalidLifecycleRecoveryRollbackCompletion, err)
}

func validateLifecycleRecoveryRollbackCompletionReceipt(
	receipt LifecycleRecoveryRollbackCompletionReceipt,
) error {
	if receipt.SchemaVersion != LifecycleRecoveryRollbackCompletionReceiptSchemaV1 || receipt.ReceiptID == "" ||
		receipt.RollbackAdmissionID == "" || receipt.RollbackPlanID == "" || receipt.RecoveryHandoffID == "" ||
		receipt.OriginalAdmissionID == "" || receipt.NodeID == "" || receipt.ClusterID == "" ||
		receipt.ClusterGeneration == 0 || receipt.MembershipSnapshotID == "" || receipt.RevisionEvidenceID == "" ||
		receipt.PreviousLifecycleGeneration == 0 || receipt.AppliedLifecycleGeneration == 0 ||
		receipt.PreviousLifecycleResourceVersion == "" || receipt.AppliedLifecycleResourceVersion == "" {
		return fmt.Errorf(
			"%w: completion identity or lifecycle evidence is incomplete",
			ErrInvalidLifecycleRecoveryRollbackCompletion,
		)
	}
	if receipt.FromState != nodelifecycle.StateDraining || receipt.TargetState != nodelifecycle.StateReady ||
		receipt.TransitionType != nodelifecycle.TransitionDesired ||
		receipt.AppliedLifecycleGeneration != receipt.PreviousLifecycleGeneration+1 ||
		receipt.AppliedLifecycleResourceVersion == receipt.PreviousLifecycleResourceVersion {
		return fmt.Errorf(
			"%w: completed rollback lifecycle boundary is invalid",
			ErrInvalidLifecycleRecoveryRollbackCompletion,
		)
	}
	if receipt.QuorumRequired <= 0 || receipt.MinimumReadyNodes != receipt.QuorumRequired ||
		receipt.HealthyVotesObserved < receipt.QuorumRequired || receipt.ReadyNodesObserved < receipt.MinimumReadyNodes {
		return fmt.Errorf(
			"%w: completion quorum or minimum-ready evidence is invalid",
			ErrInvalidLifecycleRecoveryRollbackCompletion,
		)
	}
	if !receipt.CompletionVerified || receipt.ReconciliationRequired || receipt.FurtherLifecycleAuthorized ||
		receipt.MembershipMutationAuthorized || receipt.FailoverAuthorized || receipt.GenericCommandAuthorized ||
		receipt.HostMutationAuthorized {
		return fmt.Errorf(
			"%w: completion safety flags are invalid",
			ErrInvalidLifecycleRecoveryRollbackCompletion,
		)
	}
	if err := validateTransitionRevision(receipt.Revision); err != nil {
		return fmt.Errorf("%w: revision: %v", ErrInvalidLifecycleRecoveryRollbackCompletion, err)
	}
	for field, value := range map[string]string{
		"receipt_id":                          receipt.ReceiptID,
		"rollback_admission_id":               receipt.RollbackAdmissionID,
		"rollback_plan_id":                    receipt.RollbackPlanID,
		"recovery_handoff_id":                 receipt.RecoveryHandoffID,
		"original_admission_id":               receipt.OriginalAdmissionID,
		"node_id":                             receipt.NodeID,
		"cluster_id":                          receipt.ClusterID,
		"membership_snapshot_id":              receipt.MembershipSnapshotID,
		"revision_evidence_id":                receipt.RevisionEvidenceID,
		"previous_lifecycle_resource_version": receipt.PreviousLifecycleResourceVersion,
		"applied_lifecycle_resource_version":  receipt.AppliedLifecycleResourceVersion,
	} {
		if err := validateIdentifier(field, value); err != nil {
			return fmt.Errorf("%w: %v", ErrInvalidLifecycleRecoveryRollbackCompletion, err)
		}
	}
	expectedID, err := lifecycleRecoveryRollbackCompletionReceiptID(receipt)
	if err != nil {
		return err
	}
	if expectedID != receipt.ReceiptID {
		return fmt.Errorf(
			"%w: rollback completion fingerprint mismatch",
			ErrInvalidLifecycleRecoveryRollbackCompletion,
		)
	}
	return nil
}

func lifecycleRecoveryRollbackCompletionReceiptID(
	receipt LifecycleRecoveryRollbackCompletionReceipt,
) (string, error) {
	input := receipt
	input.ReceiptID = ""
	encoded, err := json.Marshal(input)
	if err != nil {
		return "", fmt.Errorf(
			"%w: fingerprint: %v",
			ErrInvalidLifecycleRecoveryRollbackCompletion,
			err,
		)
	}
	digest := sha256.Sum256(encoded)
	return "chrrc-" + hex.EncodeToString(digest[:])[:24], nil
}
